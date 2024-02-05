/*
Copyright 2021 The Kubernetes Authors.
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

package provider

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	servicehelper "k8s.io/cloud-provider/service/helpers"
	"k8s.io/utils/ptr"

	klog "k8s.io/klog/v2"
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
			service:           newService("udp-service", tweakAddPorts(corev1.ProtocolUDP, 0), tweakAddLBClass(ptr.To(LoadbalancerClass))),
			expectNumOfUpdate: 1,
			expectNumOfPatch:  1,
		},
		{
			desc:              "tcp service that wants LB",
			service:           newService("basic-service1", tweakAddLBClass(ptr.To(LoadbalancerClass))),
			expectNumOfUpdate: 1,
			expectNumOfPatch:  1,
		},
		{
			desc:              "sctp service that wants LB",
			service:           newService("sctp-service", tweakAddPorts(corev1.ProtocolSCTP, 0), tweakAddLBClass(ptr.To(LoadbalancerClass))),
			expectNumOfUpdate: 1,
			expectNumOfPatch:  1,
		},
		{
			desc:              "service specifies incorrect loadBalancerClass",
			service:           newService("with-external-balancer", tweakAddLBClass(ptr.To(LoadbalancerClass))),
			expectNumOfUpdate: 1,
			expectNumOfPatch:  1,
		},
		{
			desc:             "service that needs cleanup",
			service:          newService("basic-service2", tweakAddLBIngress("8.8.8.8"), tweakAddFinalizers(servicehelper.LoadBalancerCleanupFinalizer), tweakAddDeletionTimestamp(time.Now()), tweakAddLBClass(ptr.To(LoadbalancerClass))),
			expectNumOfPatch: 1,
		},
		{
			desc:              "service with finalizer that wants LB",
			service:           newService("basic-service3", tweakAddFinalizers(servicehelper.LoadBalancerCleanupFinalizer), tweakAddLBClass(ptr.To(LoadbalancerClass))),
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

func TestNeedsUpdate(t *testing.T) {
	testCases := []struct {
		desc    string
		service []*corev1.Service
		expect  bool
	}{
		{
			desc: "udp service that wants LB change protocol port",
			service: []*corev1.Service{
				newService("udp-service", tweakAddPorts(corev1.ProtocolUDP, 0)),
				newService("udp-service", tweakAddPorts(corev1.ProtocolUDP, 1)),
			},
			expect: true,
		},
		{
			desc: "service that get ingress update",
			service: []*corev1.Service{
				newService("ingress-update-service", tweakAddLBIngress("8.8.8.8")),
				newService("ingress-update-service", tweakAddLBIngress("1.1.1.1")),
			},
			expect: false,
		},
		{
			desc: "service that get app protocol update",
			service: []*corev1.Service{
				newService("app-protocol-service", tweakAddAppProtocol(string(corev1.ProtocolUDP))),
				newService("app-protocol-service", tweakAddAppProtocol(string(corev1.ProtocolSCTP))),
			},
			expect: true,
		},
		{
			desc: "service with update on externaltrafficpolicy",
			service: []*corev1.Service{
				newService("basic-etp", tweakAddETP(corev1.ServiceExternalTrafficPolicyLocal)),
				newService("basic-etp"),
			},
			expect: true,
		},
		{
			desc: "service with update on ipfamily",
			service: []*corev1.Service{
				newService("basic-etp"),
				newService("basic-etp", tweakSetIPFamilies(corev1.IPv4Protocol)),
			},
			expect: true,
		},
		{
			desc: "service with update on loadbalancerip",
			service: []*corev1.Service{
				newService("basic-etp"),
				newService("basic-etp", tweakSetLoadbalancerIP("10.0.0.1")),
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
			service: newService("service"),
			expect:  false,
		},
		{
			desc:    "service doesn't have finalizer, has deletion timestamp",
			service: newService("service", tweakAddDeletionTimestamp(time.Now())),
			expect:  false,
		},
		{
			desc:    "service has finalizer, no deletion timestamp",
			service: newService("service", tweakAddFinalizers(servicehelper.LoadBalancerCleanupFinalizer)),
			expect:  false,
		},
		{
			desc:    "service has finalizer and deletion timestamp",
			service: newService("service", tweakAddFinalizers(servicehelper.LoadBalancerCleanupFinalizer), tweakAddDeletionTimestamp(time.Now())),
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

type serviceTweak func(s *corev1.Service)

func newService(name string, tweaks ...serviceTweak) *corev1.Service {
	s := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
		Spec: corev1.ServiceSpec{
			Type:  corev1.ServiceTypeLoadBalancer,
			Ports: makeServicePort(corev1.ProtocolTCP, 0),
		},
	}
	for _, tw := range tweaks {
		tw(s)
	}
	return s
}

func tweakAddETP(etpType corev1.ServiceExternalTrafficPolicyType) serviceTweak {
	return func(s *corev1.Service) {
		s.Spec.ExternalTrafficPolicy = etpType
	}
}

func tweakAddLBIngress(ip string) serviceTweak {
	return func(s *corev1.Service) {
		s.Status.LoadBalancer.Ingress = []corev1.LoadBalancerIngress{{IP: ip}}
	}
}

func makeServicePort(protocol corev1.Protocol, targetPort int) []corev1.ServicePort {
	sp := corev1.ServicePort{Port: 80, Protocol: protocol}
	if targetPort > 0 {
		sp.TargetPort = intstr.FromInt32(int32(targetPort))
	}
	return []corev1.ServicePort{sp}
}

func tweakAddPorts(protocol corev1.Protocol, targetPort int) serviceTweak {
	return func(s *corev1.Service) {
		s.Spec.Ports = makeServicePort(protocol, targetPort)
	}
}

func tweakAddLBClass(loadBalancerClass *string) serviceTweak {
	return func(s *corev1.Service) {
		s.Spec.LoadBalancerClass = loadBalancerClass
	}
}

func tweakAddFinalizers(finalizers ...string) serviceTweak {
	return func(s *corev1.Service) {
		s.ObjectMeta.Finalizers = finalizers
	}
}

func tweakAddDeletionTimestamp(time time.Time) serviceTweak {
	return func(s *corev1.Service) {
		s.ObjectMeta.DeletionTimestamp = &metav1.Time{Time: time}
	}
}

func tweakAddAppProtocol(appProtocol string) serviceTweak {
	return func(s *corev1.Service) {
		s.Spec.Ports[0].AppProtocol = &appProtocol
	}
}

func tweakSetIPFamilies(families ...corev1.IPFamily) serviceTweak {
	return func(s *corev1.Service) {
		s.Spec.IPFamilies = families
	}
}

func tweakSetLoadbalancerIP(ip string) serviceTweak {
	return func(s *corev1.Service) {
		s.Spec.LoadBalancerIP = ip
	}
}
