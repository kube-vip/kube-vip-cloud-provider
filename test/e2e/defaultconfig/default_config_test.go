//go:build e2e

package defaultconfig

import (
	"context"
	"fmt"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/require"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	tu "github.com/kube-vip/kube-vip-cloud-provider/pkg/testutil"
	"github.com/kube-vip/kube-vip-cloud-provider/test/e2e"
)

var f = e2e.NewFramework()

func TestDeployWithDifferentConfig(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "deploy with default config")
}

var _ = Describe("Default config", func() {
	Context("Deploy service in default namespace", func() {
		var (
			testsvc          = "svc-default"
			testsvcNamespace = "default"
		)

		BeforeEach(func() {
			// By default, kube-vip-cloud-provider will provide ip for service in any namespaces.
			require.NoError(f.T(), f.Deployment.EnsureResources())
		})

		f.NamespacedTest("create-services", func(namespace string) {
			Specify("Service should be reconciled, has ip assigned and correct label", func() {
				ctx := context.TODO()
				By("Create a service type LB")
				svc := tu.NewService(testsvc, tu.TweakNamespace(testsvcNamespace))
				_, err := f.Client.CoreV1().Services(svc.Namespace).Create(ctx, svc, meta_v1.CreateOptions{})
				require.NoError(f.T(), err)

				By("Service should have a valid IP assigned, and related kube-vip annotations and labels")
				require.Eventually(f.T(), func() bool {
					svc, err = f.Client.CoreV1().Services(svc.Namespace).Get(ctx, svc.Name, meta_v1.GetOptions{})
					if err != nil {
						return false
					}
					return e2e.ServiceIsReconciled(svc) && e2e.ServiceHasIPAssigned(svc)
				}, 30*time.Second, time.Second, fmt.Sprintf("Service is not successfully reconciled %v, with error %v", svc, err))
			})
		})

		AfterEach(func() {
			err := f.Client.CoreV1().Services(testsvcNamespace).Delete(context.TODO(), testsvc,
				meta_v1.DeleteOptions{PropagationPolicy: ptr.To(meta_v1.DeletePropagationBackground)})
			require.NoError(f.T(), err)

			// Reset resource requests for other tests.
			require.NoError(f.T(), f.Deployment.DeleteResources())
		})
	})

	Context("Deploy service in namespace that kube-vip-cloud-provider is not configured to", func() {
		BeforeEach(func() {
			// Update configmap to only allocate ip for service in test-2 namespace
			f.Deployment.ConfigMap.Data = map[string]string{
				"cidr-test-2": "10.0.0.1/24",
			}
			require.NoError(f.T(), f.Deployment.EnsureResources())
		})

		f.NamespacedTest("create-services-in-different-namespace", func(namespace string) {
			Specify("Service not be reconcile if namespace is not configured namespace test-2, service in default namespace should be reconciled", func() {
				ctx := context.TODO()
				By("Create a service type LB in namespace that's not test-2")
				svc := tu.NewService("test1", tu.TweakNamespace(namespace))
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

				By("Create a service type LB in test-2 namespace")
				svc = tu.NewService("test2", tu.TweakNamespace("test-2"))
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
			})
		}, "test-2")

		AfterEach(func() {
			// Reset resource requests for other tests.
			require.NoError(f.T(), f.Deployment.DeleteResources())
		})
	})
})
