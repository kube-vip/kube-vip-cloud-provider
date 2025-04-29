package provider

import (
	"context"
	"fmt"
	"reflect"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	servicehelper "k8s.io/cloud-provider/service/helpers"
	"k8s.io/klog"
)

const (
	controllerName = "service-lbc-controller"
)

// loadbalancerClassServiceController starts a controller that reconcile type loadbalancer service with
// loadbalancerclass set to kube-vip.io/kube-vip-class.
// no need to add node controller since kube-vip-cp itself doesn't use node info to update loadbalancer
type loadbalancerClassServiceController struct {
	kubeClient          kubernetes.Interface
	serviceInformer     cache.SharedIndexInformer
	serviceLister       corelisters.ServiceLister
	serviceListerSynced cache.InformerSynced

	recorder  record.EventRecorder
	workqueue workqueue.TypedRateLimitingInterface[any]

	cmName      string
	cmNamespace string
	lbClass     string
}

func newLoadbalancerClassServiceController(
	sharedInformer informers.SharedInformerFactory,
	kubeClient kubernetes.Interface,
	cmName, cmNamespace, lbClass string,
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
		workqueue: workqueue.NewNamedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[any](), "Services"),

		cmName:      cmName,
		cmNamespace: cmNamespace,
		lbClass:     lbClass,
	}

	_, _ = serviceInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(cur interface{}) {
			if svc, ok := cur.(*corev1.Service); ok && wantsLoadBalancer(svc, lbClass) {
				c.enqueueService(svc)
			}
		},
		UpdateFunc: func(old interface{}, cur interface{}) {
			oldSvc, ok1 := old.(*corev1.Service)
			curSvc, ok2 := cur.(*corev1.Service)
			if ok1 && ok2 && wantsLoadBalancer(curSvc, lbClass) && (c.needsUpdate(oldSvc, curSvc) || needsCleanup(curSvc)) {
				c.enqueueService(curSvc)
			}
		},
		// Delete is handled in the UpdateFunc
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

// Run starts the worker to process service updates
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
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return err
	}

	svc, err := c.serviceLister.Services(namespace).Get(name)

	switch {
	case apierrors.IsNotFound(err):
		// service absence in store means watcher caught the deletion, ensure LB info is cleaned
		return nil
	case err != nil:
		utilruntime.HandleError(fmt.Errorf("unable to retrieve service %v from store: %v", key, err))
		return err
	default:
		klog.Infof("Reconcile service %s/%s, since loadbalancerClass match", svc.Namespace, svc.Name)
		if err = c.processServiceCreateOrUpdate(svc); err != nil {
			return err
		}
	}

	return nil
}

func (c *loadbalancerClassServiceController) processServiceCreateOrUpdate(svc *corev1.Service) error {
	startTime := time.Now()
	defer func() {
		klog.Infof("Finished processing service %s/%s (%v)", svc.Namespace, svc.Name, time.Since(startTime))
	}()

	// if it's getting deleted, remove the finalizer
	if !svc.DeletionTimestamp.IsZero() {
		if err := c.removeFinalizer(svc); err != nil {
			klog.Infof("Error removing finalizer from service %s/%s", svc.Namespace, svc.Name)
			return err
		}
		c.recorder.Event(svc, corev1.EventTypeNormal, "LoadBalancerDeleted", "Deleted load balancer")
		return nil
	}

	c.recorder.Event(svc, corev1.EventTypeNormal, "EnsuringLoadBalancer", "Ensuring load balancer")

	if err := c.addFinalizer(svc); err != nil {
		klog.Infof("Error adding finalizer to service %s/%s", svc.Namespace, svc.Name)
		return err
	}

	if _, err := syncLoadBalancer(context.Background(), c.kubeClient, svc, c.cmName, c.cmNamespace); err != nil {
		c.recorder.Eventf(svc, corev1.EventTypeWarning, "syncLoadBalancer", "Error syncing load balancer: %v", err)
		return err
	}

	c.recorder.Event(svc, corev1.EventTypeNormal, "EnsuredLoadBalancer", "Ensured load balancer")

	return nil
}

// addFinalizer patches the service to add finalizer.
func (c *loadbalancerClassServiceController) addFinalizer(service *corev1.Service) error {
	if servicehelper.HasLBFinalizer(service) {
		return nil
	}

	// Make a copy so we don't mutate the shared informer cache.
	updated := service.DeepCopy()
	updated.ObjectMeta.Finalizers = append(updated.ObjectMeta.Finalizers, servicehelper.LoadBalancerCleanupFinalizer)

	klog.Infof("Adding finalizer to service %s/%s", updated.Namespace, updated.Name)
	_, err := servicehelper.PatchService(c.kubeClient.CoreV1(), service, updated)
	return err
}

// removeFinalizer patches the service to remove finalizer.
func (c *loadbalancerClassServiceController) removeFinalizer(service *corev1.Service) error {
	if !servicehelper.HasLBFinalizer(service) {
		return nil
	}

	// Make a copy so we don't mutate the shared informer cache.
	updated := service.DeepCopy()
	updated.ObjectMeta.Finalizers = removeString(updated.ObjectMeta.Finalizers, servicehelper.LoadBalancerCleanupFinalizer)

	klog.Infof("Removing finalizer from service %s/%s", updated.Namespace, updated.Name)
	_, err := servicehelper.PatchService(c.kubeClient.CoreV1(), service, updated)
	return err
}

// needsUpdate checks if load balancer needs to be updated due to change in attributes.
func (c *loadbalancerClassServiceController) needsUpdate(oldService *corev1.Service, newService *corev1.Service) bool {
	if wantsLoadBalancer(newService, c.lbClass) && !reflect.DeepEqual(oldService.Spec.LoadBalancerSourceRanges, newService.Spec.LoadBalancerSourceRanges) {
		c.recorder.Eventf(newService, corev1.EventTypeNormal, "LoadBalancerSourceRanges", "%v -> %v",
			oldService.Spec.LoadBalancerSourceRanges, newService.Spec.LoadBalancerSourceRanges)
		return true
	}

	if !portsEqualForLB(oldService, newService) || oldService.Spec.SessionAffinity != newService.Spec.SessionAffinity {
		return true
	}

	if !reflect.DeepEqual(oldService.Spec.SessionAffinityConfig, newService.Spec.SessionAffinityConfig) {
		return true
	}
	if !loadBalancerIPsAreEqual(oldService, newService) {
		c.recorder.Eventf(newService, corev1.EventTypeNormal, "LoadbalancerIP", "%v -> %v",
			oldService.Spec.LoadBalancerIP, newService.Spec.LoadBalancerIP)
		return true
	}
	if len(oldService.Spec.ExternalIPs) != len(newService.Spec.ExternalIPs) {
		c.recorder.Eventf(newService, corev1.EventTypeNormal, "ExternalIP", "Count: %v -> %v",
			len(oldService.Spec.ExternalIPs), len(newService.Spec.ExternalIPs))
		return true
	}
	for i := range oldService.Spec.ExternalIPs {
		if oldService.Spec.ExternalIPs[i] != newService.Spec.ExternalIPs[i] {
			c.recorder.Eventf(newService, corev1.EventTypeNormal, "ExternalIP", "Added: %v",
				newService.Spec.ExternalIPs[i])
			return true
		}
	}
	if !reflect.DeepEqual(oldService.Annotations, newService.Annotations) {
		return true
	}
	if oldService.UID != newService.UID {
		c.recorder.Eventf(newService, corev1.EventTypeNormal, "UID", "%v -> %v",
			oldService.UID, newService.UID)
		return true
	}
	if oldService.Spec.ExternalTrafficPolicy != newService.Spec.ExternalTrafficPolicy {
		c.recorder.Eventf(newService, corev1.EventTypeNormal, "ExternalTrafficPolicy", "%v -> %v",
			oldService.Spec.ExternalTrafficPolicy, newService.Spec.ExternalTrafficPolicy)
		return true
	}
	if oldService.Spec.HealthCheckNodePort != newService.Spec.HealthCheckNodePort {
		c.recorder.Eventf(newService, corev1.EventTypeNormal, "HealthCheckNodePort", "%v -> %v",
			oldService.Spec.HealthCheckNodePort, newService.Spec.HealthCheckNodePort)
		return true
	}

	// User can upgrade (add another clusterIP or ipFamily) or can downgrade (remove secondary clusterIP or ipFamily),
	// but CAN NOT change primary/secondary clusterIP || ipFamily UNLESS they are changing from/to/ON ExternalName
	// so not care about order, only need check the length.
	if len(oldService.Spec.IPFamilies) != len(newService.Spec.IPFamilies) {
		c.recorder.Eventf(newService, corev1.EventTypeNormal, "IPFamilies", "Count: %v -> %v",
			len(oldService.Spec.IPFamilies), len(newService.Spec.IPFamilies))
		return true
	}

	return false
}

// only return service that's service type loadbalancer and loadbalancerclass match
func wantsLoadBalancer(svc *corev1.Service, loadbalancerClass string) bool {
	return svc != nil && svc.Spec.Type == corev1.ServiceTypeLoadBalancer && svc.Spec.LoadBalancerClass != nil && *svc.Spec.LoadBalancerClass == loadbalancerClass
}

// removeString returns a newly created []string that contains all items from slice that
// are not equal to s.
func removeString(slice []string, s string) []string {
	var newSlice []string
	for _, item := range slice {
		if item != s {
			newSlice = append(newSlice, item)
		}
	}
	return newSlice
}

// needsCleanup checks if load balancer needs to be cleaned up as indicated by finalizer.
func needsCleanup(service *corev1.Service) bool {
	if !servicehelper.HasLBFinalizer(service) {
		return false
	}

	if !service.ObjectMeta.DeletionTimestamp.IsZero() {
		return true
	}

	return false
}

func loadBalancerIPsAreEqual(oldService, newService *corev1.Service) bool {
	return oldService.Spec.LoadBalancerIP == newService.Spec.LoadBalancerIP
}

func portsEqualForLB(x, y *corev1.Service) bool {
	xPorts := getPortsForLB(x)
	yPorts := getPortsForLB(y)
	return portSlicesEqualForLB(xPorts, yPorts)
}

func getPortsForLB(service *corev1.Service) []*corev1.ServicePort {
	ports := []*corev1.ServicePort{}
	for i := range service.Spec.Ports {
		sp := &service.Spec.Ports[i]
		ports = append(ports, sp)
	}
	return ports
}

func portSlicesEqualForLB(x, y []*corev1.ServicePort) bool {
	if len(x) != len(y) {
		return false
	}

	for i := range x {
		if !portEqualForLB(x[i], y[i]) {
			return false
		}
	}
	return true
}

func portEqualForLB(x, y *corev1.ServicePort) bool {
	// TODO: Should we check name?  (In theory, an LB could expose it)
	if x.Name != y.Name {
		return false
	}

	if x.Protocol != y.Protocol {
		return false
	}

	if x.Port != y.Port {
		return false
	}

	if x.NodePort != y.NodePort {
		return false
	}

	if x.TargetPort != y.TargetPort {
		return false
	}

	if !reflect.DeepEqual(x.AppProtocol, y.AppProtocol) {
		return false
	}

	return true
}
