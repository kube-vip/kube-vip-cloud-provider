package provider

import (
	"context"
	"testing"
	"time"

	clientgotesting "k8s.io/client-go/testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	servicehelper "k8s.io/cloud-provider/service/helpers"
	klog "k8s.io/klog/v2"
	"k8s.io/utils/ptr"

	tu "github.com/kube-vip/kube-vip-cloud-provider/pkg/testutil"
)

func alwaysReady() bool { return true }

func newController(kubeClient *fake.Clientset) *loadbalancerClassServiceController {
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(klog.Infof)
	informerFactory := informers.NewSharedInformerFactory(kubeClient, 0)
	serviceInformer := informerFactory.Core().V1().Services()

	c := &loadbalancerClassServiceController{
		serviceInformer:     serviceInformer.Informer(),
		serviceLister:       serviceInformer.Lister(),
		serviceListerSynced: alwaysReady,
		kubeClient:          kubeClient,
		cmName:              KubeVipClientConfig,
		cmNamespace:         KubeVipClientConfigNamespace,

		recorder:  record.NewFakeRecorder(100),
		workqueue: workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "Nodes"),
	}
	kubeClient.ClearActions()
	return c
}

func newIPPoolConfigMap() *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      KubeVipClientConfig,
			Namespace: KubeVipClientConfigNamespace,
		},
		Data: map[string]string{
			"cidr-global": "10.0.0.1/24",
		},
	}
}

func TestSyncLoadBalancerIfNeeded(t *testing.T) {
	testCases := []struct {
		desc              string
		service           *corev1.Service
		expectNumOfUpdate int
		expectNumOfPatch  int
	}{
		{
			desc:              "udp service that wants LB",
			service:           tu.NewService("udp-service", tu.TweakAddPorts(corev1.ProtocolUDP, 80, 0), tu.TweakAddLBClass(ptr.To(LoadbalancerClass))),
			expectNumOfUpdate: 1,
			expectNumOfPatch:  1,
		},
		{
			desc:              "tcp service that wants LB",
			service:           tu.NewService("basic-service1", tu.TweakAddLBClass(ptr.To(LoadbalancerClass))),
			expectNumOfUpdate: 1,
			expectNumOfPatch:  1,
		},
		{
			desc:              "sctp service that wants LB",
			service:           tu.NewService("sctp-service", tu.TweakAddPorts(corev1.ProtocolSCTP, 80, 0), tu.TweakAddLBClass(ptr.To(LoadbalancerClass))),
			expectNumOfUpdate: 1,
			expectNumOfPatch:  1,
		},
		{
			desc:             "service that needs cleanup",
			service:          tu.NewService("basic-service2", tu.TweakAddLBIngress("8.8.8.8"), tu.TweakAddFinalizers(servicehelper.LoadBalancerCleanupFinalizer), tu.TweakAddDeletionTimestamp(time.Now()), tu.TweakAddLBClass(ptr.To(LoadbalancerClass))),
			expectNumOfPatch: 1,
		},
		{
			desc:              "service with finalizer that wants LB",
			service:           tu.NewService("basic-service3", tu.TweakAddFinalizers(servicehelper.LoadBalancerCleanupFinalizer), tu.TweakAddLBClass(ptr.To(LoadbalancerClass))),
			expectNumOfUpdate: 1,
		},
	}

	// create ip pool for service to use
	client := fake.NewSimpleClientset()
	ctx := context.Background()
	cm := newIPPoolConfigMap()
	if _, err := client.CoreV1().ConfigMaps(cm.Namespace).Create(ctx, cm, metav1.CreateOptions{}); err != nil {
		t.Errorf("Failed to prepare configmap %s for testing: %v", cm.Name, err)
	}
	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			c := newController(client)
			// create service
			if _, err := client.CoreV1().Services(tc.service.Namespace).Create(ctx, tc.service, metav1.CreateOptions{}); err != nil {
				t.Errorf("Failed to prepare service %s for testing: %v", tc.service, err)
			}
			client.ClearActions()

			// run process processServiceCreateOrUpdate
			if err := c.processServiceCreateOrUpdate(tc.service); err != nil {
				t.Errorf("failed to update service %s: %v", tc.service.Name, err)
			}
			actions := client.Actions()
			updateNum := 0
			patchNum := 0
			for _, action := range actions {
				if action.Matches("update", "services") {
					updateNum++
				}
				if action.Matches("patch", "services") {
					patchNum++
				}
			}
			if updateNum != tc.expectNumOfUpdate {
				t.Errorf("expect %d updates, got %d updates.", tc.expectNumOfUpdate, updateNum)
			}
			if patchNum != tc.expectNumOfPatch {
				t.Errorf("expect %d patches, got %d patches.", tc.expectNumOfPatch, patchNum)
			}
		})
	}
}

func newSmallIPPoolConfigMap() *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      KubeVipClientConfig,
			Namespace: KubeVipClientConfigNamespace,
		},
		Data: map[string]string{
			"cidr-global":        "10.0.0.0/30,2001::0/48",
			"allow-share-global": "true",
		},
	}
}

func TestSyncLoadBalancerIfNeededWithMultipleIpUse(t *testing.T) {
	testCases := []struct {
		desc              string
		service           *corev1.Service
		expectIP          string
		expectNumOfUpdate int
		expectNumOfPatch  int
		expectError       bool
	}{
		{
			desc:              "udp service that wants LB",
			service:           tu.NewService("udp-service", tu.TweakDualStack(), tu.TweakAddPorts(corev1.ProtocolUDP, 123, 123), tu.TweakAddLBClass(ptr.To(LoadbalancerClass))),
			expectIP:          "10.0.0.1,2001::",
			expectNumOfUpdate: 1,
			expectNumOfPatch:  1,
		},
		{
			desc:              "tcp service that wants LB",
			service:           tu.NewService("basic-service1", tu.TweakDualStack(), tu.TweakAddPorts(corev1.ProtocolTCP, 345, 345), tu.TweakAddLBClass(ptr.To(LoadbalancerClass))),
			expectIP:          "10.0.0.1,2001::1",
			expectNumOfUpdate: 1,
			expectNumOfPatch:  1,
		},
		{
			desc:              "sctp service that wants LB",
			service:           tu.NewService("sctp-service", tu.TweakAddPorts(corev1.ProtocolSCTP, 1234, 1234), tu.TweakAddLBClass(ptr.To(LoadbalancerClass))),
			expectIP:          "10.0.0.1",
			expectNumOfUpdate: 1,
			expectNumOfPatch:  1,
		},
		{
			desc:             "service that needs cleanup",
			service:          tu.NewService("basic-service2", tu.TweakAddLBIngress("8.8.8.8"), tu.TweakAddFinalizers(servicehelper.LoadBalancerCleanupFinalizer), tu.TweakAddDeletionTimestamp(time.Now()), tu.TweakAddLBClass(ptr.To(LoadbalancerClass))),
			expectNumOfPatch: 1,
		},
		{
			desc:              "service with finalizer that wants LB",
			service:           tu.NewService("basic-service3", tu.TweakAddFinalizers(servicehelper.LoadBalancerCleanupFinalizer), tu.TweakAddLBClass(ptr.To(LoadbalancerClass))),
			expectIP:          "10.0.0.1",
			expectNumOfUpdate: 1,
		},
		{
			desc:              "now there is not enough ip, another tcp service that wants LB, but still could share ip with existing service",
			service:           tu.NewService("basic-service4", tu.TweakAddPorts(corev1.ProtocolTCP, 8080, 8080), tu.TweakAddLBClass(ptr.To(LoadbalancerClass))),
			expectNumOfUpdate: 1,
			expectNumOfPatch:  1,
			expectIP:          "10.0.0.1",
			expectError:       true,
		},
		{
			desc:              "another service who wants same port, get a new IP address",
			service:           tu.NewService("basic-service5", tu.TweakAddPorts(corev1.ProtocolTCP, 80, 80), tu.TweakAddLBClass(ptr.To(LoadbalancerClass))),
			expectNumOfUpdate: 1,
			expectNumOfPatch:  1,
			expectIP:          "10.0.0.2",
			expectError:       true,
		},
	}

	// create ip pool for service to use
	client := fake.NewSimpleClientset()
	ctx := context.Background()
	// This pool has 4 ipv4 addresses and 8 ipv6 address
	cm := newSmallIPPoolConfigMap()
	if _, err := client.CoreV1().ConfigMaps(cm.Namespace).Create(ctx, cm, metav1.CreateOptions{}); err != nil {
		t.Errorf("Failed to prepare configmap %s for testing: %v", cm.Name, err)
	}
	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			c := newController(client)
			// create service
			if _, err := client.CoreV1().Services(tc.service.Namespace).Create(ctx, tc.service, metav1.CreateOptions{}); err != nil {
				t.Errorf("Failed to prepare service %s for testing: %v", tc.service, err)
			}
			client.ClearActions()

			// run process processServiceCreateOrUpdate
			if err := c.processServiceCreateOrUpdate(tc.service); err != nil {
				if !tc.expectError {
					t.Errorf("failed to update service %s: %v", tc.service.Name, err)
				}
			}
			actions := client.Actions()
			updateNum := 0
			patchNum := 0
			lbIP := ""
			for _, action := range actions {
				switch a := action.(type) {
				case clientgotesting.UpdateActionImpl:
					s := a.Object.(*corev1.Service)
					lbIP = s.ObjectMeta.Annotations["kube-vip.io/loadbalancerIPs"]
					updateNum++
				case clientgotesting.PatchActionImpl:
					patchNum++
				}
			}
			if updateNum != tc.expectNumOfUpdate {
				t.Errorf("expect %d updates, got %d updates.", tc.expectNumOfUpdate, updateNum)
			}
			if patchNum != tc.expectNumOfPatch {
				t.Errorf("expect %d patches, got %d patches.", tc.expectNumOfPatch, patchNum)
			}
			if lbIP != tc.expectIP {
				t.Errorf("expect '%s' LbIP, got '%s'.", tc.expectIP, lbIP)
			}
		})
	}
}

func TestNeedsUpdate(t *testing.T) {
	testCases := []struct {
		desc    string
		service []*corev1.Service
		expect  bool
	}{
		{
			desc: "udp service that wants LB change protocol port",
			service: []*corev1.Service{
				tu.NewService("udp-service", tu.TweakAddPorts(corev1.ProtocolUDP, 80, 0)),
				tu.NewService("udp-service", tu.TweakAddPorts(corev1.ProtocolUDP, 80, 1)),
			},
			expect: true,
		},
		{
			desc: "service that get ingress update",
			service: []*corev1.Service{
				tu.NewService("ingress-update-service", tu.TweakAddLBIngress("8.8.8.8")),
				tu.NewService("ingress-update-service", tu.TweakAddLBIngress("1.1.1.1")),
			},
			expect: false,
		},
		{
			desc: "service that get app protocol update",
			service: []*corev1.Service{
				tu.NewService("app-protocol-service", tu.TweakAddAppProtocol(string(corev1.ProtocolUDP))),
				tu.NewService("app-protocol-service", tu.TweakAddAppProtocol(string(corev1.ProtocolSCTP))),
			},
			expect: true,
		},
		{
			desc: "service with update on externaltrafficpolicy",
			service: []*corev1.Service{
				tu.NewService("basic-etp", tu.TweakAddETP(corev1.ServiceExternalTrafficPolicyLocal)),
				tu.NewService("basic-etp"),
			},
			expect: true,
		},
		{
			desc: "service with update on ipfamily",
			service: []*corev1.Service{
				tu.NewService("basic-etp"),
				tu.NewService("basic-etp", tu.TweakSetIPFamilies(corev1.IPv4Protocol)),
			},
			expect: true,
		},
		{
			desc: "service with update on loadbalancerip",
			service: []*corev1.Service{
				tu.NewService("basic-etp"),
				tu.NewService("basic-etp", tu.TweakSetLoadbalancerIP("10.0.0.1")),
			},
			expect: true,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			client := fake.NewSimpleClientset()
			c := newController(client)
			nu := c.needsUpdate(tc.service[0], tc.service[1])
			if tc.expect != nu {
				t.Errorf("expect update to be %t, but get %t", tc.expect, nu)
			}
		})
	}
}

func TestNeedsCleanup(t *testing.T) {
	testCases := []struct {
		desc    string
		service *corev1.Service
		expect  bool
	}{
		{
			desc:    "service doesn't have finalizer or deletion timestamp",
			service: tu.NewService("service"),
			expect:  false,
		},
		{
			desc:    "service doesn't have finalizer, has deletion timestamp",
			service: tu.NewService("service", tu.TweakAddDeletionTimestamp(time.Now())),
			expect:  false,
		},
		{
			desc:    "service has finalizer, no deletion timestamp",
			service: tu.NewService("service", tu.TweakAddFinalizers(servicehelper.LoadBalancerCleanupFinalizer)),
			expect:  false,
		},
		{
			desc:    "service has finalizer and deletion timestamp",
			service: tu.NewService("service", tu.TweakAddFinalizers(servicehelper.LoadBalancerCleanupFinalizer), tu.TweakAddDeletionTimestamp(time.Now())),
			expect:  true,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			nc := needsCleanup(tc.service)
			if tc.expect != nc {
				t.Errorf("expect service clean up to be %t, but get %t", tc.expect, nc)
			}
		})
	}
}
