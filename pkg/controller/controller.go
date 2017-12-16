package controller

import (
	"fmt"
	"time"

	"github.com/golang/glog"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	kubeinformers "k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"

	kubeflowalpha1 "github.com/caicloud/kubeflow-clientset/apis/kubeflow/v1alpha1"
	clientset "github.com/caicloud/kubeflow-clientset/clientset/versioned"
	kubeflowscheme "github.com/caicloud/kubeflow-clientset/clientset/versioned/scheme"
	informers "github.com/caicloud/kubeflow-clientset/informers/externalversions"
	listers "github.com/caicloud/kubeflow-clientset/listers/kubeflow/v1alpha1"
)

const (
	controllerName = "kubeflow-controller"
	// SuccessSynced is used as part of the Event 'reason' when a TFJob is synced
	SuccessSynced = "Synced"

	// MessageResourceSynced is the message used for an Event fired when a TFJob
	// is synced successfully
	MessageResourceSynced = "TFJob synced successfully"
)

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
}

// NewController returns a new tfJob controller.
func NewController(
	kubeclientset kubernetes.Interface,
	tfJobclientset clientset.Interface,
	kubeInformerFactory kubeinformers.SharedInformerFactory,
	tfJobInformerFactory informers.SharedInformerFactory) *Controller {

	// obtain references to shared index informers for the tfJob type

	tfJobInformer := tfJobInformerFactory.Kubeflow().V1alpha1().TFJobs()

	// Create event broadcaster
	// Add tfJob-controller types to the default Kubernetes Scheme so Events can be
	// logged for tfJob-controller types.
	kubeflowscheme.AddToScheme(scheme.Scheme)
	glog.V(4).Info("Creating event broadcaster")
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(glog.Infof)
	eventBroadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{Interface: kubeclientset.CoreV1().Events("")})
	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: controllerName})

	controller := &Controller{
		kubeclientset:  kubeclientset,
		tfJobclientset: tfJobclientset,
		tfJobLister:    tfJobInformer.Lister(),
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

	// Get the TFJob resource with this namespace/name
	tfJob, err := c.tfJobLister.TFJobs(namespace).Get(name)
	if err != nil {
		// The TFJob resource may no longer exist, in which case we stop
		// processing.
		if errors.IsNotFound(err) {
			runtime.HandleError(fmt.Errorf("tfJob '%s' in work queue no longer exists", key))
			return nil
		}

		return err
	}

	glog.V(4).Infof("Sync TFJob: %s", tfJob)

	// TODO: Create TFJob on Kubernetes.
	return nil
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
