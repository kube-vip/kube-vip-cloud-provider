//go:build e2e

package loadbalancerclass

import (
	"context"
	"fmt"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/require"
	core_v1 "k8s.io/api/core/v1"
	api_errors "k8s.io/apimachinery/pkg/api/errors"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	"github.com/kube-vip/kube-vip-cloud-provider/pkg/provider"
	tu "github.com/kube-vip/kube-vip-cloud-provider/pkg/testutil"
	"github.com/kube-vip/kube-vip-cloud-provider/test/e2e"
)

// Each suite load default manifest from scratch, so that changes on manifest objects won't impact other tests suites.
var f = e2e.NewFramework()

func TestDeployWithLoadBalancerClass(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "deploy with loadbalancerclass")
}

var _ = BeforeSuite(func() {
	// By default, configMap only contains below 3 cidr:
	// cidr-default: 192.168.0.200/29
	// cidr-plunder: 192.168.0.210/29
	// cidr-testing: 192.168.0.220/29

	// enable loadbalancerclass
	f.Deployment.Deployment.Spec.Template.Spec.Containers[0].Env = append(f.Deployment.Deployment.Spec.Template.Spec.Containers[0].Env,
		core_v1.EnvVar{
			Name:  provider.EnableLoadbalancerClassEnvKey,
			Value: "true",
		},
		core_v1.EnvVar{
			Name:  provider.CustomLoadbalancerClassEnvKey,
			Value: "kube-vip.io/custom-class",
		},
	)
	require.NoError(f.T(), f.Deployment.EnsureResources())
})

var _ = AfterSuite(func() {
	// Reset resource requests for other tests.
	require.NoError(f.T(), f.Deployment.DeleteResources())
})

var watchedNamespaces = []string{"default", "testing", "plunder"}

var _ = Describe("Loadbalancerclass enabled", func() {
	Context("Deploy service with loadbalancerclass in different namespaces that kube-vip-cloud-provider is configured to watch and is not configured to watch", func() {
		f.NamespacedTest("create-services-in-different-namespace-with-loadblaancerclass", func(namespace string) {
			Specify("Service not be reconcile if namespace is not configured namespace testing, service in default namespace should be reconciled", func() {
				ctx := context.TODO()
				By("Create a service type LB in namespace that's not testing")
				svc := tu.NewService("test1", tu.TweakNamespace(namespace), tu.TweakAddLBClass(ptr.To(provider.LoadbalancerClass)))
				_, err := f.Client.CoreV1().Services(svc.Namespace).Create(ctx, svc, meta_v1.CreateOptions{})
				require.NoError(f.T(), err)

				By("Service should not have IP assigned, it shouldn't have kube-vip annotations and labels")
				require.Eventually(f.T(), func() bool {
					svc, err = f.Client.CoreV1().Services(svc.Namespace).Get(ctx, svc.Name, meta_v1.GetOptions{})
					if err != nil {
						return false
					}
					return !e2e.ServiceIsReconciled(svc) && !e2e.ServiceHasIPAssigned(svc)
				}, 30*time.Second, time.Second, fmt.Sprintf("Service is not supposed to have label or annotation %v, with error %v", svc, err))

				for _, ns := range watchedNamespaces {
					By("Create a service type LB in watched namespace")
					svc = tu.NewService(fmt.Sprintf("test-%s", ns), tu.TweakNamespace(ns), tu.TweakAddLBClass(ptr.To(provider.LoadbalancerClass)))
					_, err = f.Client.CoreV1().Services(svc.Namespace).Create(ctx, svc, meta_v1.CreateOptions{})
					require.NoError(f.T(), err)

					By("Service should have a valid IP assigned, and related kube-vip annotations and labels")
					require.Eventually(f.T(), func() bool {
						svc, err = f.Client.CoreV1().Services(svc.Namespace).Get(ctx, svc.Name, meta_v1.GetOptions{})
						if err != nil {
							return false
						}
						return e2e.ServiceIsReconciled(svc) && e2e.ServiceHasIPAssigned(svc)
					}, 30*time.Second, time.Second, fmt.Sprintf("Service is not successfully reconciled %v, with error %v", svc, err))

					By("Clean up the service")
					err = f.Client.CoreV1().Services(ns).Delete(context.TODO(), svc.Name,
						meta_v1.DeleteOptions{PropagationPolicy: ptr.To(meta_v1.DeletePropagationBackground)})
					require.Eventually(f.T(), func() bool {
						svc, err = f.Client.CoreV1().Services(svc.Namespace).Get(ctx, svc.Name, meta_v1.GetOptions{})
						if api_errors.IsNotFound(err) {
							return true
						}
						return false
					}, 30*time.Second, time.Second, fmt.Sprintf("Service is not successfully deleted %v, with error %v", svc, err))
				}
			})
		}, watchedNamespaces[1:]...) // Don't delete default namespace.
	})

	Context("Deploy service without loadbalancerclass in different namespaces that kube-vip-cloud-provider is configured to watch", func() {
		f.NamespacedTest("create-services-namespace-without-loadblaancerclass", func(namespace string) {
			Specify("Service not be reconcile if service doesn't have loadbalancer class even though namespace is configured to be watched", func() {
				ctx := context.TODO()

				for _, ns := range watchedNamespaces {
					By("Create a service type LB in watched namespace")
					svc := tu.NewService("test1", tu.TweakNamespace(ns))
					_, err := f.Client.CoreV1().Services(svc.Namespace).Create(ctx, svc, meta_v1.CreateOptions{})
					require.NoError(f.T(), err)
					By("Service should not have IP assigned, it shouldn't have kube-vip annotations and labels")
					require.Eventually(f.T(), func() bool {
						svc, err = f.Client.CoreV1().Services(svc.Namespace).Get(ctx, svc.Name, meta_v1.GetOptions{})
						if err != nil {
							return false
						}
						return !e2e.ServiceIsReconciled(svc) && !e2e.ServiceHasIPAssigned(svc)
					}, 30*time.Second, time.Second, fmt.Sprintf("Service is not supposed to have label or annotation %v, with error %v", svc, err))

					By("Clean up the service")
					err = f.Client.CoreV1().Services(ns).Delete(context.TODO(), svc.Name,
						meta_v1.DeleteOptions{PropagationPolicy: ptr.To(meta_v1.DeletePropagationBackground)})
					require.Eventually(f.T(), func() bool {
						svc, err = f.Client.CoreV1().Services(svc.Namespace).Get(ctx, svc.Name, meta_v1.GetOptions{})
						if api_errors.IsNotFound(err) {
							return true
						}
						return false
					}, 30*time.Second, time.Second, fmt.Sprintf("Service is not successfully deleted %v, with error %v", svc, err))
				}
			})
		}, watchedNamespaces[1:]...) // Don't delete default namespace.
	})
})
