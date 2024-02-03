package provider

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog"
)

const (
	controllerName = "service-with-loadbalancerclass-controller"
)

type loadbalancerClassServiceController struct {
	kubeClient          kubernetes.Interface
	serviceInformer     cache.SharedIndexInformer
	serviceLister       corelisters.ServiceLister
	serviceListerSynced cache.InformerSynced

	recorder  record.EventRecorder
	workqueue workqueue.RateLimitingInterface

	cmName      string
	cmNamespace string
}

func newLoadbalancerClassServiceController(
	sharedInformer informers.SharedInformerFactory,
	kubeClient kubernetes.Interface,
	cmName, cmNamespace string,
) *loadbalancerClassServiceController {
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(klog.Infof)
	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: controllerName})
	serviceInformer := sharedInformer.Core().V1().Services().Informer()
	c := &loadbalancerClassServiceController{
		serviceInformer:     serviceInformer,
		serviceLister:       sharedInformer.Core().V1().Services().Lister(),
		serviceListerSynced: serviceInformer.HasSynced,
		kubeClient:          kubeClient,

		recorder:  recorder,
		workqueue: workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "Nodes"),

		cmName:      cmName,
		cmNamespace: cmNamespace,
	}

	_, _ = serviceInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(cur interface{}) {
			s := cur.(*corev1.Node).DeepCopy()
			c.enqueueService(s)
		},
		UpdateFunc: func(old interface{}, new interface{}) {
			c.enqueueService(new)
		},
		// DeleteFunc: ,
	})

	return c
}

func (c *loadbalancerClassServiceController) enqueueService(obj interface{}) {
	var key string
	var err error
	if key, err = cache.MetaNamespaceKeyFunc(obj); err != nil {
		utilruntime.HandleError(err)
		return
	}
	c.workqueue.Add(key)
}

// Run starts the worker to process node updates
func (c *loadbalancerClassServiceController) Run(stopCh <-chan struct{}) {
	defer utilruntime.HandleCrash()
	defer c.workqueue.ShutDown()

	klog.V(4).Info("Waiting cache to be synced.")

	if !cache.WaitForNamedCacheSync("service", stopCh, c.serviceListerSynced) {
		return
	}

	klog.V(4).Info("Starting service workers for loadbalancerclass.")
	go wait.Until(c.runWorker, time.Second, stopCh)

	<-stopCh
}

// runWorker is a long-running function that will continually call the
// processNextWorkItem function in order to read and process a message on the
// workqueue.
func (c *loadbalancerClassServiceController) runWorker() {
	for c.processNextWorkItem() {
	}
}

// processNextWorkItem will read a single work item off the workqueue and
// attempt to process it, by calling the syncHandler.
func (c *loadbalancerClassServiceController) processNextWorkItem() bool {
	obj, shutdown := c.workqueue.Get()
	if shutdown {
		return false
	}

	// We wrap this block in a func so we can defer c.workqueue.Done.
	err := func(obj interface{}) error {
		defer c.workqueue.Done(obj)

		var key string
		var ok bool
		if key, ok = obj.(string); !ok {
			c.workqueue.Forget(obj)
			utilruntime.HandleError(fmt.Errorf("expected string in workqueue but got %#v", obj))
			return nil
		}

		// Run the syncHandler, passing it the key of the
		// IPPool resource to be synced.
		if err := c.syncService(key); err != nil {
			// Put the item back on the workqueue to handle any transient errors.
			c.workqueue.AddRateLimited(key)
			return fmt.Errorf("error syncing '%s': %s, requeuing", key, err.Error())
		}

		// Finally, if no error occurs we Forget this item so it does not
		// get queued again until another change happens.
		c.workqueue.Forget(obj)
		return nil
	}(obj)
	if err != nil {
		utilruntime.HandleError(err)
		return true
	}

	return true
}

// syncService will sync the Service with the given key if it has had its expectations fulfilled,
// meaning it did not expect to see any more of its pods created or deleted. This function is not meant to be
// invoked concurrently with the same key.
func (c *loadbalancerClassServiceController) syncService(key string) error {
	startTime := time.Now()
	defer func() {
		klog.V(4).Infof("Finished syncing service %q (%v)", key, time.Since(startTime))
	}()

	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return err
	}

	svc, err := c.serviceLister.Services(namespace).Get(name)

	switch {
	case err != nil:
		utilruntime.HandleError(fmt.Errorf("unable to retrieve service %v from store: %v", key, err))
	case loadbalancerClassMatch(svc):
		klog.V(4).Infof("Reconcile service %s/%s, since loadbalancerClass match", svc.Namespace, svc.Name)
		err = c.processServiceCreateOrUpdate(svc)
	default:
		klog.V(4).Infof("Skip reoconciling service %s/%s, since loadbalancerClass doesn't match", svc.Namespace, svc.Name)
	}

	return err
}

func (c *loadbalancerClassServiceController) processServiceCreateOrUpdate(svc *corev1.Service) error {
	// ctx := context.Background()
	_, err := syncLoadBalancer(context.Background(), c.kubeClient, svc, c.cmName, c.cmNamespace)
	c.recorder.Eventf(svc, corev1.EventTypeWarning, "syncLoadBalancer", "Error syncing load balancer: %v", err)
	return err
}

func loadbalancerClassMatch(svc *corev1.Service) bool {
	return svc != nil && svc.Spec.LoadBalancerClass != nil && *svc.Spec.LoadBalancerClass == LoadbalancerClass
}
