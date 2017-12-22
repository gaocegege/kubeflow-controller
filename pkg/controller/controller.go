package controller

import (
	"fmt"
	"reflect"
	"time"

	api "github.com/caicloud/kubeflow-clientset/apis/kubeflow/v1alpha1"
	kubeflowalpha1 "github.com/caicloud/kubeflow-clientset/apis/kubeflow/v1alpha1"
	kubeflowscheme "github.com/caicloud/kubeflow-clientset/clientset/scheme"
	clientset "github.com/caicloud/kubeflow-clientset/clientset/typed/kubeflow/v1alpha1"
	informers "github.com/caicloud/kubeflow-clientset/informers"
	listers "github.com/caicloud/kubeflow-clientset/listers/kubeflow/v1alpha1"
	"github.com/golang/glog"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"
	clientv1 "k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	kubernetesapi "k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/api/v1"
	kubernetes "k8s.io/kubernetes/pkg/client/clientset_generated/clientset"
	kubeinformers "k8s.io/kubernetes/pkg/client/informers/informers_generated/externalversions"
	kubelisters "k8s.io/kubernetes/pkg/client/listers/core/v1"
	"k8s.io/kubernetes/pkg/controller"

	"github.com/caicloud/kubeflow-controller/pkg/checker"
	"github.com/caicloud/kubeflow-controller/pkg/composer"
)

const (
	controllerName = "kubeflow-controller"
	// SuccessSynced is used as part of the Event 'reason' when a TFJob is synced
	SuccessSynced = "Synced"

	// MessageResourceSynced is the message used for an Event fired when a TFJob
	// is synced successfully
	MessageResourceSynced = "TFJob synced successfully"
)

var (
	groupVersionKind = schema.GroupVersionKind{
		Group:   api.GroupName,
		Version: api.GroupVersion,
		Kind:    api.TFJobResourceKind,
	}
)

// Controller is the type for TFJob controller.
type Controller struct {
	// kubeclientset is a standard kubernetes clientset
	kubeclientset kubernetes.Interface
	// tfjobclientset is a clientset for our own API group
	tfJobclientset clientset.KubeflowV1alpha1Interface

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
	podControl   controller.PodControlInterface
	podLister    kubelisters.PodLister

	// TODO(gaocegege): Add a map o keep track of all trainers.
}

// NewController returns a new tfJob controller.
func NewController(
	kubeclientset kubernetes.Interface,
	tfJobclientset clientset.KubeflowV1alpha1Interface,
	kubeInformerFactory kubeinformers.SharedInformerFactory,
	tfJobInformerFactory informers.SharedInformerFactory) *Controller {

	// obtain references to shared index informers for the tfJob type

	tfJobInformer := tfJobInformerFactory.Kubeflow().V1alpha1().TFJobs()

	// Create event broadcaster
	// Add tfJob-controller types to the default Kubernetes Scheme so Events can be
	// logged for tfJob-controller types.
	kubeflowscheme.AddToScheme(kubernetesapi.Scheme)
	glog.V(4).Info("Creating event broadcaster")
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(glog.Infof)
	eventBroadcaster.StartRecordingToSink(&v1core.EventSinkImpl{Interface: v1core.New(kubeclientset.Core().RESTClient()).Events("")})
	recorder := eventBroadcaster.NewRecorder(kubernetesapi.Scheme, clientv1.EventSource{Component: controllerName})

	controller := &Controller{
		kubeclientset:  kubeclientset,
		tfJobclientset: tfJobclientset,
		podControl: controller.RealPodControl{
			KubeClient: kubeclientset,
			Recorder:   eventBroadcaster.NewRecorder(kubernetesapi.Scheme, clientv1.EventSource{Component: controllerName}),
		},
		tfJobLister:  tfJobInformer.Lister(),
		expectations: controller.NewControllerExpectations(),
		tfJobSynced:  tfJobInformer.Informer().HasSynced,
		workqueue:    workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "tfJobs"),
		recorder:     recorder,
	}

	glog.Info("Setting up event handlers")
	// Set up an event handler for when tfJob resources change
	tfJobInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: controller.handleObject,
		UpdateFunc: func(old, new interface{}) {
			glog.Info("Update tfJob")
			newTFJob := new.(*kubeflowalpha1.TFJob)
			oldTFJob := old.(*kubeflowalpha1.TFJob)
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
	podInformer := kubeInformerFactory.Core().V1().Pods()
	podInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    controller.addPod,
		UpdateFunc: controller.updatePod,
		// DeleteFunc: controller.deletePod,
	})
	controller.podLister = podInformer.Lister()

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

	pods, err := c.getPodsForJob(tfJob)
	if err != nil {
		return err
	}

	activePods := controller.FilterActivePods(pods)
	succeeded, _ := getStatus(pods)
	// TODO(gaocegege): Check if the TFJob is valid.

	if jobNeedsSync {
		c.manageTFJob(activePods, succeeded, tfJob)
	}

	complete := false
	if succeeded == 1 {
		complete = true
	}

	if complete {
		// job.Status.Conditions = append(job.Status.Conditions, newCondition(batch.JobComplete, "", ""))
		tfJob.Status.Phase = api.TFJobSucceeded
	}

	// Update the status.
	// TODO(gaocegege): Check first.
	state := api.TFReplicaLocal
	for _, pod := range activePods {
		tfJob.Status.TFReplicaStatuses = make([]*api.TFReplicaStatus, 0)
		replicaStatus := &api.TFReplicaStatus{
			Type:  &state,
			State: api.TFReplicaState(pod.Status.Phase),
		}
		tfJob.Status.TFReplicaStatuses = append(tfJob.Status.TFReplicaStatuses, replicaStatus)
	}

	if err := c.updateTFJob(tfJob); err != nil {
		return err
	}

	glog.V(4).Infof("Sync TFJob: %s", tfJob)

	return nil
}

func (c *Controller) manageTFJob(activePods []*v1.Pod, succeeded int32, tfJob *api.TFJob) (int32, error) {
	active := int32(len(activePods))
	jobKey, err := controller.KeyFunc(tfJob)
	if err != nil {
		runtime.HandleError(fmt.Errorf("Couldn't get key for job %#v: %v", tfJob, err))
		return 0, nil
	}

	if checker.IsLocalJob(tfJob) {
		wantActive := int32(1)
		// Had succeeded result, not need to create new pod.
		if succeeded > 0 {
			wantActive = active
		}
		if active < wantActive {
			glog.V(4).Infof("Need %d active pods but got %d", wantActive, active)
			c.expectations.ExpectCreations(jobKey, int(wantActive-active))

			// TODO: If failed to create, it will be executed multiple times.
			composer := composer.New(tfJob)
			composer.Compose()

			err := c.podControl.CreatePodsWithControllerRef(
				tfJob.Namespace, tfJob.Spec.Specs[0].Template, tfJob,
				newControllerRef(tfJob))
			if err != nil && errors.IsTimeout(err) {
				// Pod is created but its initialization has timed out.
				// If the initialization is successful eventually, the
				// controller will observe the creation via the informer.
				// If the initialization fails, or if the pod keeps
				// uninitialized for a long time, the informer will not
				// receive any update, and the controller will create a new
				// pod when the expectation expires.
			}
			if err != nil {
				defer runtime.HandleError(err)
				glog.V(2).Infof("Failed creation, decrementing expectations for job %q/%q", tfJob.Namespace, tfJob.Name)
				c.expectations.CreationObserved(jobKey)
			}
		} else {
			glog.V(4).Info("wantActive = active, nothing to do.")
		}
	}
	return 0, nil
}

func (c *Controller) addPod(obj interface{}) {
	pod := obj.(*v1.Pod)
	// if pod.DeletionTimestamp != nil {
	// 	// on a restart of the controller controller, it's possible a new pod shows up in a state that
	// 	// is already pending deletion. Prevent the pod from being a creation observation.
	// 	c.deletePod(pod)
	// 	return
	// }

	glog.V(4).Infof("Now pod added: %v", pod)

	// If it has a ControllerRef, that's all that matters.
	if controllerRef := controller.GetControllerOf(pod); controllerRef != nil {
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

	glog.V(4).Infof("pod updated: %v, %v", curPod, oldPod)

	curControllerRef := controller.GetControllerOf(curPod)
	oldControllerRef := controller.GetControllerOf(oldPod)
	controllerRefChanged := !reflect.DeepEqual(curControllerRef, oldControllerRef)
	if controllerRefChanged && oldControllerRef != nil {
		// The ControllerRef was changed. Sync the old controller, if any.
		if job := c.resolveControllerRef(oldPod.Namespace, oldControllerRef); job != nil {
			c.enqueueTFJob(job)
		}
	}

	// If it has a ControllerRef, that's all that matters.
	if curControllerRef != nil {
		job := c.resolveControllerRef(curPod.Namespace, curControllerRef)
		if job == nil {
			return
		}
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

func (c *Controller) getPodsForJob(tfJob *api.TFJob) ([]*v1.Pod, error) {
	// TODO(gaocegege): It is a hack, we definitely should replace it with a graceful way.
	selector, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{
		MatchLabels: map[string]string{
			"kubeflow.caicloud.io": "",
			"job_type":             string(*tfJob.Spec.Specs[0].TFReplicaType),
			"runtime_id":           tfJob.Spec.RuntimeID,
			"tf_job_name":          tfJob.Name,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("couldn't convert Job selector: %v", err)
	}
	// List all pods to include those that don't match the selector anymore
	// but have a ControllerRef pointing to this controller.
	pods, err := c.podLister.Pods(tfJob.Namespace).List(labels.Everything())
	if err != nil {
		return nil, err
	}
	// If any adoptions are attempted, we should first recheck for deletion
	// with an uncached quorum read sometime after listing Pods (see #42639).
	canAdoptFunc := controller.RecheckDeletionTimestamp(func() (metav1.Object, error) {
		fresh, err := c.tfJobclientset.TFJobs(tfJob.Namespace).Get(tfJob.Name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		if fresh.UID != tfJob.UID {
			return nil, fmt.Errorf("original Job %v/%v is gone: got uid %v, wanted %v", tfJob.Namespace, tfJob.Name, fresh.UID, tfJob.UID)
		}
		return fresh, nil
	})
	cm := controller.NewPodControllerRefManager(c.podControl, tfJob, selector, groupVersionKind, canAdoptFunc)
	return cm.ClaimPods(pods)
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
	_, err := c.tfJobclientset.TFJobs(tfJob.Namespace).Update(tfJob)
	if err != nil {
		return fmt.Errorf("failed to update TFJob: %v", err)
	}
	return nil
}
