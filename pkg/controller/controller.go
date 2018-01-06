/*
Copyright 2018 Caicloud Inc. All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"fmt"
	"reflect"
	"time"

	"github.com/caicloud/kubeflow-controller/pkg/controller/control"
	"github.com/caicloud/kubeflow-controller/pkg/controller/updater"
	"github.com/caicloud/kubeflow-controller/pkg/tensorflow"

	"github.com/caicloud/kubeflow-controller/pkg/checker"

	api "github.com/caicloud/kubeflow-clientset/apis/kubeflow/v1alpha1"
	clientset "github.com/caicloud/kubeflow-clientset/clientset/versioned"
	kubeflowscheme "github.com/caicloud/kubeflow-clientset/clientset/versioned/scheme"
	informers "github.com/caicloud/kubeflow-clientset/informers/externalversions"
	listers "github.com/caicloud/kubeflow-clientset/listers/kubeflow/v1alpha1"
	"github.com/golang/glog"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	kubeinformers "k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/kubernetes/pkg/controller"
)

const (
	controllerName = "kubeflow-controller"
	// SuccessSynced is used as part of the Event 'reason' when a TFJob is synced
	SuccessSynced = "Synced"

	// MessageResourceSynced is the message used for an Event fired when a TFJob
	// is synced successfully
	MessageResourceSynced = "TFJob synced successfully"
)

// Controller is the type for TFJob controller.
type Controller struct {
	// kubeclientset is a standard kubernetes clientset
	kubeclientset kubernetes.Interface
	// tfjobclientset is a clientset for our own API group
	tfJobclientset clientset.Interface

	tfJobLister listers.TFJobLister
	tfJobSynced cache.InformerSynced

	// workqueue is a rate limited work queue. This is used to queue work to be
	// processed instead of performing it as soon as a change happens. This
	// means we can ensure we only process a fixed amount of resources at a
	// time, and makes it easy to ensure we are never processing the same item
	// simultaneously in two different workers.
	workqueue workqueue.RateLimitingInterface
	// recorder is an event recorder for recording Event resources to the
	// Kubernetes API.
	recorder record.EventRecorder

	// A TTLCache of pod creates/deletes each rc expects to see
	expectations controller.ControllerExpectationsInterface

	helper HelperInterface

	// TODO(gaocegege): Add a map o keep track of all trainers.
}

// NewController returns a new tfJob controller.
func NewController(
	kubeclientset kubernetes.Interface,
	tfJobclientset clientset.Interface,
	kubeInformerFactory kubeinformers.SharedInformerFactory,
	tfJobInformerFactory informers.SharedInformerFactory) *Controller {

	// obtain references to shared index informers for the tfJob type

	tfJobInformer := tfJobInformerFactory.Kubeflow().V1alpha1().TFJobs()
	podInformer := kubeInformerFactory.Core().V1().Pods()
	serviceInformer := kubeInformerFactory.Core().V1().Services()

	// Create event broadcaster
	// Add tfJob-controller types to the default Kubernetes Scheme so Events can be
	// logged for tfJob-controller types.
	kubeflowscheme.AddToScheme(scheme.Scheme)
	glog.V(4).Info("Creating event broadcaster")
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(glog.Infof)
	eventBroadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{Interface: kubeclientset.CoreV1().Events("")})
	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, v1.EventSource{Component: controllerName})

	podControl := controller.RealPodControl{
		KubeClient: kubeclientset,
		Recorder:   eventBroadcaster.NewRecorder(scheme.Scheme, v1.EventSource{Component: controllerName}),
	}
	serviceControl := control.RealServiceControl{
		KubeClient: kubeclientset,
		Recorder:   eventBroadcaster.NewRecorder(scheme.Scheme, v1.EventSource{Component: controllerName}),
	}
	podLister := podInformer.Lister()
	serviceLister := serviceInformer.Lister()

	helper := NewHelper(tfJobclientset, podLister, podControl, serviceLister, serviceControl)

	controller := &Controller{
		kubeclientset:  kubeclientset,
		tfJobclientset: tfJobclientset,
		helper:         helper,
		tfJobLister:    tfJobInformer.Lister(),
		expectations:   controller.NewControllerExpectations(),
		tfJobSynced:    tfJobInformer.Informer().HasSynced,
		workqueue:      workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "tfJobs"),
		recorder:       recorder,
	}

	glog.Info("Setting up event handlers")
	// Set up an event handler for when tfJob resources change
	tfJobInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: controller.handleObject,
		UpdateFunc: func(old, new interface{}) {
			glog.Info("Update tfJob")
			newTFJob := new.(*api.TFJob)
			oldTFJob := old.(*api.TFJob)
			if newTFJob.ResourceVersion == oldTFJob.ResourceVersion {
				glog.Infof("ResourceVersion not changed: %s", newTFJob.ResourceVersion)
				// Periodic resync will send update events for all known tfJobes.
				// Two different versions of the same tfJob will always have different RVs.
				return
			}
			controller.handleObject(newTFJob)
		},
		DeleteFunc: controller.handleObject,
	})

	podInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    controller.addPod,
		UpdateFunc: controller.updatePod,
		DeleteFunc: controller.deletePod,
	})

	serviceInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    controller.addService,
		UpdateFunc: controller.updateService,
		DeleteFunc: controller.deleteService,
	})

	return controller
}

// Run will set up the event handlers for types we are interested in, as well
// as syncing informer caches and starting workers. It will block until stopCh
// is closed, at which point it will shutdown the workqueue and wait for
// workers to finish processing their current work items.
func (c *Controller) Run(threadiness int, stopCh <-chan struct{}) error {
	defer runtime.HandleCrash()
	defer c.workqueue.ShutDown()

	// Start the informer factories to begin populating the informer caches
	glog.Info("Starting kubeflow controller")

	// Wait for the caches to be synced before starting workers
	glog.Info("Waiting for informer caches to sync")
	if ok := cache.WaitForCacheSync(stopCh, c.tfJobSynced); !ok {
		return fmt.Errorf("failed to wait for caches to sync")
	}

	glog.Info("Starting workers")
	// Launch two workers to process tfjob resources
	for i := 0; i < threadiness; i++ {
		go wait.Until(c.runWorker, time.Second, stopCh)
	}

	glog.Info("Started workers")
	<-stopCh
	glog.Info("Shutting down workers")

	return nil
}

// runWorker is a long-running function that will continually call the
// processNextWorkItem function in order to read and process a message on the
// workqueue.
func (c *Controller) runWorker() {
	for c.processNextWorkItem() {
	}
}

// processNextWorkItem will read a single work item off the workqueue and
// attempt to process it, by calling the syncHandler.
func (c *Controller) processNextWorkItem() bool {
	obj, shutdown := c.workqueue.Get()

	if shutdown {
		return false
	}

	// We wrap this block in a func so we can defer c.workqueue.Done.
	err := func(obj interface{}) error {
		// We call Done here so the workqueue knows we have finished
		// processing this item. We also must remember to call Forget if we
		// do not want this work item being re-queued. For example, we do
		// not call Forget if a transient error occurs, instead the item is
		// put back on the workqueue and attempted again after a back-off
		// period.
		defer c.workqueue.Done(obj)
		var key string
		var ok bool
		// We expect strings to come off the workqueue. These are of the
		// form namespace/name. We do this as the delayed nature of the
		// workqueue means the items in the informer cache may actually be
		// more up to date that when the item was initially put onto the
		// workqueue.
		if key, ok = obj.(string); !ok {
			// As the item in the workqueue is actually invalid, we call
			// Forget here else we'd go into a loop of attempting to
			// process a work item that is invalid.
			c.workqueue.Forget(obj)
			runtime.HandleError(fmt.Errorf("expected string in workqueue but got %#v", obj))
			return nil
		}
		// Run the syncHandler, passing it the namespace/name string of the
		// TFJob resource to be synced.
		if err := c.syncHandler(key); err != nil {
			return fmt.Errorf("error syncing '%s': %s", key, err.Error())
		}
		// Finally, if no error occurs we Forget this item so it does not
		// get queued again until another change happens.
		c.workqueue.Forget(obj)
		glog.Infof("Successfully synced '%s'", key)
		return nil
	}(obj)

	if err != nil {
		runtime.HandleError(err)
		return true
	}

	return true
}

// syncHandler compares the actual state with the desired, and attempts to
// converge the two. It then updates the Status block of the TFJob resource
// with the current status of the resource.
func (c *Controller) syncHandler(key string) error {
	// Convert the namespace/name string into a distinct namespace and name
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		runtime.HandleError(fmt.Errorf("invalid resource key: %s", key))
		return nil
	}
	if len(namespace) == 0 || len(name) == 0 {
		return fmt.Errorf("invalid job key %q: either namespace or name is missing", key)
	}

	// Check the expectations of the job before counting active pods, otherwise a new pod can sneak in
	// and update the expectations after we've retrieved active pods from the store. If a new pod enters
	// the store after we've checked the expectation, the job sync is just deferred till the next relist.
	jobNeedsSync := c.expectations.SatisfiedExpectations(key)

	// Get the TFJob resource with this namespace/name
	tfJob, err := c.tfJobLister.TFJobs(namespace).Get(name)
	if err != nil {
		if errors.IsNotFound(err) {
			glog.V(4).Infof("Job has been deleted: %v", key)
			c.expectations.DeleteExpectations(key)
			return nil
		}
		return err
	}

	var workerPods []*v1.Pod
	var psPods []*v1.Pod
	var workerServices []*v1.Service
	var psServices []*v1.Service

	isLocalJob := checker.IsLocalJob(tfJob)

	if isLocalJob {
		workerPods, err = c.helper.GetPodsForTFJob(tfJob, api.TFReplicaLocal)
		if err != nil {
			return err
		}
	} else {
		workerPods, err = c.helper.GetPodsForTFJob(tfJob, api.TFReplicaWorker)
		if err != nil {
			return err
		}
		psPods, err = c.helper.GetPodsForTFJob(tfJob, api.TFReplicaPS)
		if err != nil {
			return err
		}
		workerServices, err = c.helper.GetServicesForTFJob(tfJob, api.TFReplicaWorker)
		if err != nil {
			return err
		}
		psServices, err = c.helper.GetServicesForTFJob(tfJob, api.TFReplicaPS)
		if err != nil {
			return err
		}
	}

	activeWorkerPods := controller.FilterActivePods(workerPods)
	succeededWorkerPods, _ := getStatus(workerPods)

	activePSPods := controller.FilterActivePods(psPods)

	// TODO(gaocegege): Check if the TFJob is valid.

	if jobNeedsSync && tfJob.DeletionTimestamp == nil {
		_, _, err := c.manageTFJob(activeWorkerPods, activePSPods, workerServices, psServices, succeededWorkerPods, tfJob)
		if err != nil {
			return err
		}
	}

	shouldUpdateStatus := false
	if isLocalJob {
		updater, err := updater.NewLocal(tfJob, int(succeededWorkerPods), workerPods)
		if err != nil {
			return err
		}
		shouldUpdateStatus = updater.ShouldUpdate()
	} else {
		updater := updater.NewDistributed(tfJob, int(succeededWorkerPods), workerPods, psPods)
		shouldUpdateStatus = updater.ShouldUpdate()
	}

	if shouldUpdateStatus {
		if err := c.updateTFJob(tfJob); err != nil {
			return err
		}
	}

	glog.V(4).Infof("Sync TFJob: %s", tfJob)

	return nil
}

func (c *Controller) manageTFJob(activeWorkerPods []*v1.Pod, activePSPods []*v1.Pod, workerServices []*v1.Service, psServices []*v1.Service, succeeded int32, tfJob *api.TFJob) (int, int, error) {
	jobKey, err := controller.KeyFunc(tfJob)
	if err != nil {
		runtime.HandleError(fmt.Errorf("Couldn't get key for job %#v: %v", tfJob, err))
		return 0, 0, err
	}
	glog.V(4).Infof("Manage the TFJob %s, active workers: %d, active parameter servers: %d", tfJob.Name, len(activeWorkerPods), len(activePSPods))

	if checker.IsLocalJob(tfJob) {
		localJob := tensorflow.NewLocalJob(tfJob, activeWorkerPods, succeeded)
		event := localJob.Action()
		switch event.Action {
		case tensorflow.ActionShouldAddWorker:
			glog.V(4).Infof("Expect to create %d pod", event.Number)
			c.expectations.ExpectCreations(jobKey, event.Number)
			if err := c.helper.CreatePod(tfJob, tfJob.Spec.Specs[0].Template); err != nil {
				defer runtime.HandleError(err)
				glog.V(2).Infof("Failed creation, decrementing expectations for TFJob %q/%q", tfJob.Namespace, tfJob.Name)
				c.expectations.CreationObserved(jobKey)
				return 0, 0, err
			}
		case tensorflow.ActionNothing:
			glog.V(4).Info("Nothing to do.")
		}
		return 1, 0, nil
	}

	// Deal with the logic about distributed jobs.
	distributedJob := tensorflow.NewDistributedJob(tfJob, activeWorkerPods, activePSPods, workerServices, psServices, succeeded)
	currentWorkerPods := 0
	currentPSPods := 0

	events := distributedJob.Action()
	for _, event := range events {
		switch event.Action {
		case tensorflow.ActionShouldAddWorkerService:
			glog.V(4).Infof("Need create %d service(s)", event.Number)
			c.expectations.ExpectCreations(jobKey, event.Number)
			for i := 0; i < event.Number; i++ {
				if err := c.helper.CreateService(tfJob, distributedJob.GetService(api.TFReplicaWorker, i)); err != nil {
					defer runtime.HandleError(err)
					glog.V(2).Infof("Failed creation, decrementing expectations for TFJob %q/%q", tfJob.Namespace, tfJob.Name)
					c.expectations.CreationObserved(jobKey)
				}
			}
		case tensorflow.ActionShouldAddPSService:
			glog.V(4).Infof("Need create %d service(s)", event.Number)
			c.expectations.ExpectCreations(jobKey, event.Number)
			for i := 0; i < event.Number; i++ {
				if err := c.helper.CreateService(tfJob, distributedJob.GetService(api.TFReplicaPS, i)); err != nil {
					defer runtime.HandleError(err)
					glog.V(2).Infof("Failed creation, decrementing expectations for TFJob %q/%q", tfJob.Namespace, tfJob.Name)
					c.expectations.CreationObserved(jobKey)
				}
			}
		case tensorflow.ActionShouldAddWorker:
			glog.V(4).Infof("Expect to create %d pod(s)", event.Number)
			c.expectations.ExpectCreations(jobKey, event.Number)
			currentWorkerPods = len(activeWorkerPods) + event.Number

			for i := 0; i < event.Number; i++ {
				if err := c.helper.CreatePod(tfJob, distributedJob.GetSpec(api.TFReplicaWorker, i).Template); err != nil {
					defer runtime.HandleError(err)
					glog.V(2).Infof("Failed creation, decrementing expectations for TFJob %q/%q", tfJob.Namespace, tfJob.Name)
					c.expectations.CreationObserved(jobKey)
					return 0, 0, err
				}
			}
		case tensorflow.ActionShouldAddPS:
			glog.V(4).Infof("Expect to create %d PS(es)", event.Number)
			c.expectations.ExpectCreations(jobKey, event.Number)
			currentPSPods = len(activePSPods) + event.Number

			for i := 0; i < event.Number; i++ {
				if err := c.helper.CreatePod(tfJob, distributedJob.GetSpec(api.TFReplicaPS, i).Template); err != nil {
					defer runtime.HandleError(err)
					glog.V(2).Infof("Failed creation, decrementing expectations for TFJob %q/%q", tfJob.Namespace, tfJob.Name)
					c.expectations.CreationObserved(jobKey)
					return 0, 0, err
				}
			}
		case tensorflow.ActionNothing:
			glog.V(4).Info("Nothing to do.")
		}
	}
	return currentWorkerPods, currentPSPods, nil
}

func (c *Controller) addPod(obj interface{}) {
	pod := obj.(*v1.Pod)
	// if pod.DeletionTimestamp != nil {
	// 	// on a restart of the controller controller, it's possible a new pod shows up in a state that
	// 	// is already pending deletion. Prevent the pod from being a creation observation.
	// 	c.deletePod(pod)
	// 	return
	// }

	glog.V(4).Infof("New pod added: %v", pod)

	// If it has a ControllerRef, that's all that matters.
	if controllerRef := metav1.GetControllerOf(pod); controllerRef != nil {
		tfJob := c.resolveControllerRef(pod.Namespace, controllerRef)
		if tfJob == nil {
			return
		}
		jobKey, err := controller.KeyFunc(tfJob)
		if err != nil {
			return
		}
		c.expectations.CreationObserved(jobKey)
		c.enqueueTFJob(tfJob)
		return
	}
}

func (c *Controller) updatePod(old, cur interface{}) {
	curPod := cur.(*v1.Pod)
	oldPod := old.(*v1.Pod)
	if curPod.ResourceVersion == oldPod.ResourceVersion {
		// Periodic resync will send update events for all known pods.
		// Two different versions of the same pod will always have different RVs.
		return
	}
	// if curPod.DeletionTimestamp != nil {
	// 	// when a pod is deleted gracefully it's deletion timestamp is first modified to reflect a grace period,
	// 	// and after such time has passed, the kubelet actually deletes it from the store. We receive an update
	// 	// for modification of the deletion timestamp and expect an job to create more pods asap, not wait
	// 	// until the kubelet actually deletes the pod.
	// 	jm.deletePod(curPod)
	// 	return
	// }

	curControllerRef := metav1.GetControllerOf(curPod)
	oldControllerRef := metav1.GetControllerOf(oldPod)
	controllerRefChanged := !reflect.DeepEqual(curControllerRef, oldControllerRef)
	if controllerRefChanged && oldControllerRef != nil {
		// The ControllerRef was changed. Sync the old controller, if any.
		if job := c.resolveControllerRef(oldPod.Namespace, oldControllerRef); job != nil {
			glog.V(4).Infof("pod ControllerRef updated: %v, %v", curPod, oldPod)
			c.enqueueTFJob(job)
		}
	}

	// If it has a ControllerRef, that's all that matters.
	if curControllerRef != nil {
		job := c.resolveControllerRef(curPod.Namespace, curControllerRef)
		if job == nil {
			return
		}
		glog.V(4).Infof("pod has a ControllerRef: %v, %v", curPod, oldPod)
		c.enqueueTFJob(job)
		return
	}

	// // Otherwise, it's an orphan. If anything changed, sync matching controllers
	// // to see if anyone wants to adopt it now.
	// if controllerRefChanged {
	// 	for _, job := range c.getPodJobs(curPod) {
	// 		c.enqueueTFJob(job)
	// 	}
	// }
}

func (c *Controller) deletePod(obj interface{}) {
	glog.Errorln("To Be Implemented.")
}

func (c *Controller) addService(obj interface{}) {
	service := obj.(*v1.Service)
	// if pod.DeletionTimestamp != nil {
	// 	// on a restart of the controller controller, it's possible a new pod shows up in a state that
	// 	// is already pending deletion. Prevent the pod from being a creation observation.
	// 	c.deletePod(pod)
	// 	return
	// }

	glog.V(4).Infof("New service added: %v", service)

	// If it has a ControllerRef, that's all that matters.
	if controllerRef := metav1.GetControllerOf(service); controllerRef != nil {
		tfJob := c.resolveControllerRef(service.Namespace, controllerRef)
		if tfJob == nil {
			return
		}
		jobKey, err := controller.KeyFunc(tfJob)
		if err != nil {
			return
		}
		c.expectations.CreationObserved(jobKey)
		c.enqueueTFJob(tfJob)
		return
	}
}

func (c *Controller) updateService(old, cur interface{}) {
	curService := cur.(*v1.Service)
	oldService := old.(*v1.Service)
	if curService.ResourceVersion == oldService.ResourceVersion {
		// Periodic resync will send update events for all known services.
		// Two different versions of the same service will always have different RVs.
		return
	}
	// if curService.DeletionTimestamp != nil {
	// 	// when a service is deleted gracefully it's deletion timestamp is first modified to reflect a grace period,
	// 	// and after such time has passed, the kubelet actually deletes it from the store. We receive an update
	// 	// for modification of the deletion timestamp and expect an job to create more services asap, not wait
	// 	// until the kubelet actually deletes the service.
	// 	jm.deleteService(curService)
	// 	return
	// }

	curControllerRef := metav1.GetControllerOf(curService)
	oldControllerRef := metav1.GetControllerOf(oldService)
	controllerRefChanged := !reflect.DeepEqual(curControllerRef, oldControllerRef)
	if controllerRefChanged && oldControllerRef != nil {
		// The ControllerRef was changed. Sync the old controller, if any.
		if job := c.resolveControllerRef(oldService.Namespace, oldControllerRef); job != nil {
			glog.V(4).Infof("service ControllerRef updated: %v, %v", curService, oldService)
			c.enqueueTFJob(job)
		}
	}

	// If it has a ControllerRef, that's all that matters.
	if curControllerRef != nil {
		job := c.resolveControllerRef(curService.Namespace, curControllerRef)
		if job == nil {
			return
		}
		glog.V(4).Infof("service has a ControllerRef: %v, %v", curService, oldService)
		c.enqueueTFJob(job)
		return
	}

	// // Otherwise, it's an orphan. If anything changed, sync matching controllers
	// // to see if anyone wants to adopt it now.
	// if controllerRefChanged {
	// 	for _, job := range c.getServiceJobs(curService) {
	// 		c.enqueueTFJob(job)
	// 	}
	// }
}

func (c *Controller) deleteService(obj interface{}) {
	glog.Errorln("To Be Implemented.")
}

// resolveControllerRef returns the controller referenced by a ControllerRef,
// or nil if the ControllerRef could not be resolved to a matching controller
// of the correct Kind.
func (c *Controller) resolveControllerRef(namespace string, controllerRef *metav1.OwnerReference) *api.TFJob {
	// We can't look up by UID, so look up by Name and then verify UID.
	// Don't even try to look up by Name if it's the wrong Kind.
	if controllerRef.Kind != api.TFJobResourceKind {
		return nil
	}
	job, err := c.tfJobLister.TFJobs(namespace).Get(controllerRef.Name)
	if err != nil {
		return nil
	}
	if job.UID != controllerRef.UID {
		// The controller we found with this Name is not the same one that the
		// ControllerRef points to.
		return nil
	}
	return job
}

func (c *Controller) handleObject(obj interface{}) {
	glog.V(4).Infof("Handle TFJob: %s", obj)

	c.enqueueTFJob(obj)
}

func (c *Controller) enqueueTFJob(obj interface{}) {
	var key string
	var err error
	if key, err = cache.MetaNamespaceKeyFunc(obj); err != nil {
		runtime.HandleError(err)
		return
	}
	c.workqueue.AddRateLimited(key)
}

// updateTFJob updates the tfjob in Kubernetes.
func (c *Controller) updateTFJob(tfJob *api.TFJob) error {
	_, err := c.tfJobclientset.KubeflowV1alpha1().TFJobs(tfJob.Namespace).Update(tfJob)
	if err != nil {
		return fmt.Errorf("failed to update TFJob: %v", err)
	}
	return nil
}
