//go:build e2e

package multipleranges

import (
	"context"
	"testing"
	"time"

	tu "github.com/kube-vip/kube-vip-cloud-provider/pkg/testutil"
	"github.com/kube-vip/kube-vip-cloud-provider/test/e2e"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/require"
	core_v1 "k8s.io/api/core/v1"
	api_errors "k8s.io/apimachinery/pkg/api/errors"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

var f = e2e.NewFramework()

func TestDeployWithMultipleRanges(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "deploy with multiple IP ranges per namespace")
}

var _ = BeforeSuite(func() {
	// Delete "cidr-testing" because it takes precedence over "range-testing" 
	// and prevents us from testing the multiple range functionality.
	delete(f.Deployment.ConfigMap.Data, "cidr-testing")
	f.Deployment.ConfigMap.Data["range-testing"] = "192.168.10.10-192.168.10.11,192.168.10.20-192.168.10.20"
	require.NoError(f.T(), f.Deployment.EnsureResources())
})

var _ = AfterSuite(func() {
	require.NoError(f.T(), f.Deployment.DeleteResources())
})

var _ = Describe("Multiple IP Ranges Assignment", func() {
	Context("When multiple comma-separated ranges are set in the ConfigMap for a namespace", func() {
		f.NamespacedTest("multi-range-ns", func(namespace string) {
			Specify("Services should sequentially receive IPs traversing across the defined ranges", func() {
				ctx := context.TODO()

				// Create three services. We expect the first two to consume the first range,
				// and the third service to bridge the gap and take the IP from the second range.
				svcs := []*core_v1.Service{
					tu.NewService("multi-range-svc-1", tu.TweakNamespace("testing")),
					tu.NewService("multi-range-svc-2", tu.TweakNamespace("testing")),
					tu.NewService("multi-range-svc-3", tu.TweakNamespace("testing")),
				}

				for _, svc := range svcs {
					By("Creating a service type LB in the testing namespace")
					_, err := f.Client.CoreV1().Services(svc.Namespace).Create(ctx, svc, meta_v1.CreateOptions{})
					require.NoError(f.T(), err)
				}

				By("Waiting for the IPs to be assigned by the kube-vip controller")
				// We poll the API Server until the controller has reconciled our Services.
				for i, svc := range svcs {
					require.Eventually(f.T(), func() bool {
						var err error
						svcs[i], err = f.Client.CoreV1().Services(svc.Namespace).Get(ctx, svc.Name, meta_v1.GetOptions{})
						if err != nil {
							return false
						}
						return e2e.ServiceIsReconciled(svcs[i]) && e2e.ServiceHasIPAssigned(svcs[i])
					}, 30*time.Second, time.Second, "Service "+svc.Name+" did not get an IP")
				}

				By("Verifying the assigned IPs bridge across both defined ranges")
				Expect(svcs[0].Spec.LoadBalancerIP).To(Equal("192.168.10.10"))
				Expect(svcs[1].Spec.LoadBalancerIP).To(Equal("192.168.10.11"))
				Expect(svcs[2].Spec.LoadBalancerIP).To(Equal("192.168.10.20")) // Successfully grabbed from the second range

				By("Cleaning up the services before the controller is killed")
				for _, svc := range svcs {
					err := f.Client.CoreV1().Services(svc.Namespace).Delete(context.TODO(), svc.Name, meta_v1.DeleteOptions{PropagationPolicy: ptr.To(meta_v1.DeletePropagationBackground)})
					require.NoError(f.T(), err)

					// Wait for the deletion finalizers to be processed.
					require.Eventually(f.T(), func() bool {
						_, err := f.Client.CoreV1().Services(svc.Namespace).Get(context.TODO(), svc.Name, meta_v1.GetOptions{})
						return api_errors.IsNotFound(err)
					}, 30*time.Second, time.Second, "Service failed to delete (Finalizer stuck)")
				}
			})
		}, "testing") // We pass "testing" here to instruct the framework to create this namespace.
	})
})
