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

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
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

func TestProcessServiceCreateOrUpdate(t *testing.T) {
	testCases := []struct {
		desc               string
		svcs               []*corev1.Service
		svcUpdate          []*corev1.Service
		expectedNumPatches int
		expectIPAllocated  bool
	}{
		{
			desc:               "create service with correct lbclass, reconcile",
			svcs:               []*corev1.Service{service("t1s1", ptr.To(LoadbalancerClass)), service("t1s2", ptr.To(LoadbalancerClass))},
			expectedNumPatches: 2,
			expectIPAllocated:  true,
		},
		{
			desc:               "create service with incorrect lbclass, skip reconciling",
			svcs:               []*corev1.Service{service("t2s1", ptr.To("wrong")), service("t2s2", ptr.To("wrong"))},
			expectedNumPatches: 0,
			expectIPAllocated:  true,
		},
	}

	// create ip pool for service to use
	client := fake.NewSimpleClientset()
	ctx := context.Background()
	cm := poolConfigMap()
	if _, err := client.CoreV1().ConfigMaps(cm.Namespace).Create(ctx, cm, metav1.CreateOptions{}); err != nil {
		t.Errorf("Failed to prepare configmap %s for testing: %v", cm.Name, err)
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			c := newController(client)
			c.serviceLister = newFakeServiceLister(nil, tc.svcs...)
			// create service
			for _, svc := range tc.svcs {
				if _, err := client.CoreV1().Services(svc.Namespace).Create(ctx, svc, metav1.CreateOptions{}); err != nil {
					t.Errorf("Failed to prepare service %s for testing: %v", svc.Name, err)
				}
			}
			client.ClearActions()

			// run process processServiceCreateOrUpdate
			for _, svc := range tc.svcs {
				if err := c.syncService(svc.Name); err != nil {
					t.Errorf("failed to update service %s: %v", svc.Name, err)
				}
			}

			// verify the number of patch request to ippool to update spec section
			actions := client.Actions()
			numPatches := 0
			for _, a := range actions {
				if a.Matches("update", "services") {
					numPatches++
				}
			}
			if tc.expectedNumPatches != numPatches {
				t.Errorf("expectedPatch %d doesn't match number of patches %d", tc.expectedNumPatches, numPatches)
			}
		})
	}
}

func service(name string, lbc *string) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: corev1.ServiceSpec{
			LoadBalancerClass: lbc,
		},
	}
}

func poolConfigMap() *corev1.ConfigMap {
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

type fakeServiceLister struct {
	cache []*corev1.Service
	err   error
}

func newFakeServiceLister(err error, svcs ...*corev1.Service) *fakeServiceLister {
	ret := &fakeServiceLister{}
	ret.cache = svcs
	ret.err = err
	return ret
}

// List lists all Services in the indexer.
// Objects returned here must be treated as read-only.
func (l *fakeServiceLister) List(selector labels.Selector) (ret []*corev1.Service, err error) {
	return l.cache, l.err
}

// Services retrieves the ServiceNamespaceLister for a namespace.
// Objects returned here must be treated as read-only.
func (l *fakeServiceLister) Services(namespace string) corelisters.ServiceNamespaceLister {
	res := &fakeServiceNamespaceLister{
		cache: []*corev1.Service{},
	}
	for _, svc := range l.cache {
		if svc.Namespace == namespace {
			res.cache = append(res.cache, svc)
		}
	}
	return res
}

type fakeServiceNamespaceLister struct {
	cache []*corev1.Service
	err   error
}

// List lists all Services in the indexer.
// Objects returned here must be treated as read-only.
func (l fakeServiceNamespaceLister) List(selector labels.Selector) (ret []*corev1.Service, err error) {
	return l.cache, l.err
}

// Get retrieves the Service from the index for a given name.
// Objects returned here must be treated as read-only.
func (l fakeServiceNamespaceLister) Get(name string) (*corev1.Service, error) {
	for _, svc := range l.cache {
		if svc.Name == name {
			return svc, nil
		}
	}
	return nil, nil
}
