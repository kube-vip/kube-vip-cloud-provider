//go:build e2e

package descendingip

import (
	"context"
	"testing"
	"time"

	"github.com/kube-vip/kube-vip-cloud-provider/pkg/config"
	tu "github.com/kube-vip/kube-vip-cloud-provider/pkg/testutil"
	"github.com/kube-vip/kube-vip-cloud-provider/test/e2e"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/require"
	api_errors "k8s.io/apimachinery/pkg/api/errors"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

var f = e2e.NewFramework()

func TestDeployWithDescendingIP(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "deploy with descending IP assignment")
}

var _ = BeforeSuite(func() {
	f.Deployment.ConfigMap.Data[config.ConfigMapSearchOrderKey] = "desc"
	f.Deployment.ConfigMap.Data[config.ConfigMapSkipEndIPsKey] = "true"
	require.NoError(f.T(), f.Deployment.EnsureResources())
})

var _ = AfterSuite(func() {
	require.NoError(f.T(), f.Deployment.DeleteResources())
})

var _ = Describe("Descending IP Assignment", func() {
	Context("When search-order=desc is set in the ConfigMap", func() {
		f.NamespacedTest("dummyns", func(namespace string) {
			Specify("The assigned LoadBalancer IP should be the highest available in the pool", func() {
				ctx := context.TODO()

				By("Creating a service type LB in a watched namespace")
				svc := tu.NewService("desc-ip-svc", tu.TweakNamespace("testing"))
				_, err := f.Client.CoreV1().Services(svc.Namespace).Create(ctx, svc, meta_v1.CreateOptions{})
				require.NoError(f.T(), err)

				By("Waiting for the IP to be assigned")
				require.Eventually(f.T(), func() bool {
					svc, err = f.Client.CoreV1().Services(svc.Namespace).Get(ctx, svc.Name, meta_v1.GetOptions{})
					if err != nil {
						return false
					}
					return e2e.ServiceIsReconciled(svc) && e2e.ServiceHasIPAssigned(svc)
				}, 30*time.Second, time.Second, "Service did not get an IP")

				By("Verifying the assigned IP is the highest in the range")
				// For cidr-testing: 192.168.0.220/29, the highest IP is 192.168.0.223,
				// but we excluded end IPs, so we expect one less than that
				expectedIP := "192.168.0.222"
				Expect(svc.Spec.LoadBalancerIP).To(Equal(expectedIP))

				By("Cleaning up the service before the controller is killed")
				err = f.Client.CoreV1().Services(svc.Namespace).Delete(context.TODO(), svc.Name, meta_v1.DeleteOptions{PropagationPolicy: ptr.To(meta_v1.DeletePropagationBackground)})
				require.NoError(f.T(), err)

				require.Eventually(f.T(), func() bool {
					_, err := f.Client.CoreV1().Services(svc.Namespace).Get(context.TODO(), svc.Name, meta_v1.GetOptions{})
					return api_errors.IsNotFound(err)
				}, 30*time.Second, time.Second, "Service failed to delete (Finalizer stuck)")
			})
		}, "testing")
	})
})
