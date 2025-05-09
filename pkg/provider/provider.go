package provider

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"

	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog"

	cloudprovider "k8s.io/cloud-provider"
)

// OutSideCluster allows the controller to be started using a local kubeConfig for testing
var OutSideCluster bool

const (
	// ProviderName is the name of the cloud provider
	ProviderName = "kubevip"

	// KubeVipClientConfig is the default name of the load balancer config Map
	KubeVipClientConfig = "kubevip"

	// KubeVipClientConfigNamespace is the default namespace of the load balancer config Map
	KubeVipClientConfigNamespace = "kube-system"

	// KubeVipServicesKey is the key in the ConfigMap that has the services configuration
	KubeVipServicesKey = "kubevip-services"

	// CustomLoadbalancerClassEnvKey environment key for custom loadbalancerclass name.
	// A LoadbalancerClass could be set in service.spec.loadbalancerclass.
	// If the service has this value, then service controller will reconcile the service.
	CustomLoadbalancerClassEnvKey = "KUBEVIP_CUSTOM_LOADBALANCERCLASS_NAME"

	// DefaultLoadbalancerClass is the default loadbalancerClass name if no custom loadbalancerClass name is
	// supplied by CustomLoadbalancerClassEnvKey
	DefaultLoadbalancerClass = "kube-vip.io/kube-vip-class"

	// EnableLoadbalancerClassEnvKey environment key for enabling loadbalancerclass.
	// This should be enabled if CustomLoadbalancerClassNameEnvKey is not empty
	EnableLoadbalancerClassEnvKey = "KUBEVIP_ENABLE_LOADBALANCERCLASS"
)

func init() {
	cloudprovider.RegisterCloudProvider(ProviderName, newKubeVipCloudProvider)
}

// KubeVipCloudProvider - contains all of the interfaces for the cloud provider
type KubeVipCloudProvider struct {
	lb            cloudprovider.LoadBalancer
	kubeClient    kubernetes.Interface
	namespace     string
	configMapName string
	enableLBClass bool
	lbClass       string
}

var _ cloudprovider.Interface = &KubeVipCloudProvider{}

func newKubeVipCloudProvider(io.Reader) (cloudprovider.Interface, error) {
	ns := os.Getenv("KUBEVIP_NAMESPACE")
	cm := os.Getenv("KUBEVIP_CONFIG_MAP")
	lbc := os.Getenv(EnableLoadbalancerClassEnvKey)
	cbc := os.Getenv(CustomLoadbalancerClassEnvKey)

	if cm == "" {
		cm = KubeVipClientConfig
	}

	if ns == "" {
		ns = KubeVipClientConfigNamespace
	}

	var (
		enableLBClass bool
		lbClass       string
		err           error
	)

	if len(lbc) > 0 {
		klog.Infof("Checking if loadbalancerClass is enabled: %s", lbc)
		enableLBClass, err = strconv.ParseBool(lbc)
		if err != nil {
			return nil, fmt.Errorf("error parsing value of %s: %s", EnableLoadbalancerClassEnvKey, err.Error())
		}
	}
	klog.Infof("starting with enable loadbalancerClass flag set to: %t", enableLBClass)

	lbClass = DefaultLoadbalancerClass
	if cbc != "" {
		lbClass = cbc
	}
	klog.Infof("loadbalancerClass value set to: %s", lbClass)

	klog.Infof("Watching configMap for pool config with name: '%s', namespace: '%s'", cm, ns)

	var cl *kubernetes.Clientset
	if !OutSideCluster {
		// This will attempt to load the configuration when running within a POD
		cfg, err := rest.InClusterConfig()
		if err != nil {
			return nil, fmt.Errorf("error creating kubernetes client config: %s", err.Error())
		}
		cl, err = kubernetes.NewForConfig(cfg)
		if err != nil {
			return nil, fmt.Errorf("error creating kubernetes client: %s", err.Error())
		}
		// use the current context in kubeconfig
	} else {
		config, err := clientcmd.BuildConfigFromFlags("", filepath.Join(os.Getenv("HOME"), ".kube", "config"))
		if err != nil {
			panic(err.Error())
		}
		cl, err = kubernetes.NewForConfig(config)
		if err != nil {
			return nil, fmt.Errorf("error creating kubernetes client: %s", err.Error())
		}
	}
	return &KubeVipCloudProvider{
		lb:            newLoadBalancer(cl, ns, cm, lbClass),
		kubeClient:    cl,
		namespace:     ns,
		configMapName: cm,
		enableLBClass: enableLBClass,
		lbClass:       lbClass,
	}, nil
}

// Initialize - starts the clound-provider controller
func (p *KubeVipCloudProvider) Initialize(clientBuilder cloudprovider.ControllerClientBuilder, _ <-chan struct{}) {
	klog.Info("Initing Kube-vip Cloud Provider")

	clientset := clientBuilder.ClientOrDie("do-shared-informers")
	sharedInformer := informers.NewSharedInformerFactory(clientset, 0)

	if p.enableLBClass {
		klog.Info("starting a separate service controller that only monitors service with loadbalancerClass")
		klog.Info("default cloud-provider service controller will ignore service with loadbalancerClass")
		controller := newLoadbalancerClassServiceController(sharedInformer, p.kubeClient, p.configMapName, p.namespace, p.lbClass)
		go controller.Run(context.Background().Done())
	}

	sharedInformer.Start(nil)
	sharedInformer.WaitForCacheSync(nil)
}

// LoadBalancer returns a loadbalancer interface. Also returns true if the interface is supported, false otherwise.
func (p *KubeVipCloudProvider) LoadBalancer() (cloudprovider.LoadBalancer, bool) {
	return p.lb, true
}

// ProviderName returns the cloud provider ID.
func (p *KubeVipCloudProvider) ProviderName() string {
	return ProviderName
}
