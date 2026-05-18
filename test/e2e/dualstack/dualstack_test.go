//go:build e2e

package dualstack

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
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

var f = e2e.NewFramework()

func TestDeployWithDualStack(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "deploy with dual-stack IPAM")
}

var _ = BeforeSuite(func() {
	// Configure dual-stack pools (both CIDR and range style are supported)
	delete(f.Deployment.ConfigMap.Data, "cidr-testing")
	f.Deployment.ConfigMap.Data["range-testing"] = "192.168.10.10-192.168.10.20,2001:db8::10-2001:db8::20"
	
	// With CIDR style:
	// f.Deployment.ConfigMap.Data["cidr-testing"] = "192.168.10.0/28,2001:db8::/126"

	require.NoError(f.T(), f.Deployment.EnsureResources())
})

var _ = AfterSuite(func() {
	require.NoError(f.T(), f.Deployment.DeleteResources())
})

var _ = Describe("Dual-Stack IP Assignment", func() {
	f.NamespacedTest("dualstack-ns", func(namespace string) {
		Context("when a dual-stack pool is configured", func() {
			var ctx = context.Background()

			Specify("RequireDualStack service gets both IPv4 and IPv6", func() {
				svc := tu.NewService("require-dualstack",
					tu.TweakNamespace("testing"),
					tu.TweakDualStack(), // sets RequireDualStack + IPv4,IPv6 order
				)

				By("Creating the service")
				_, err := f.Client.CoreV1().Services(svc.Namespace).Create(ctx, svc, meta_v1.CreateOptions{})
				require.NoError(f.T(), err)

				By("Waiting for IP assignment")
				require.Eventually(f.T(), func() bool {
					svc, err = f.Client.CoreV1().Services(svc.Namespace).Get(ctx, svc.Name, meta_v1.GetOptions{})
					return err == nil && e2e.ServiceIsReconciled(svc) && e2e.ServiceHasIPAssigned(svc)
				}, 45*time.Second, time.Second, "Service did not get dual-stack IPs")

				By("Verifying both IPs are present and in correct order")
				Expect(svc.Annotations).To(HaveKeyWithValue("kube-vip.io/loadbalancerIPs", MatchRegexp(`^192\.168\.10\.\d+,2001:db8::1[0-9]$`)))
				Expect(svc.Spec.LoadBalancerIP).To(MatchRegexp(`^192\.168\.10\.\d+$`))
			})

			Specify("PreferDualStack with IPv6 first prefers IPv6", func() {
				svc := tu.NewService("prefer-ipv6-first",
					tu.TweakNamespace("testing"),
					func(s *core_v1.Service) {
						s.Spec.IPFamilyPolicy = ptr.To(core_v1.IPFamilyPolicyPreferDualStack)
						s.Spec.IPFamilies = []core_v1.IPFamily{core_v1.IPv6Protocol, core_v1.IPv4Protocol}
					},
				)

				_, err := f.Client.CoreV1().Services(svc.Namespace).Create(ctx, svc, meta_v1.CreateOptions{})
				require.NoError(f.T(), err)

				require.Eventually(f.T(), func() bool {
					svc, err = f.Client.CoreV1().Services(svc.Namespace).Get(ctx, svc.Name, meta_v1.GetOptions{})
					
					return err == nil && e2e.ServiceIsReconciled(svc) && e2e.ServiceHasIPAssigned(svc)
				}, 30*time.Second, time.Second)

				Expect(svc.Annotations["kube-vip.io/loadbalancerIPs"]).To(MatchRegexp(`^2001:db8::1[0-9],192\.168\.10\.\d+$`))
			})

			Specify("Single-stack IPv4 service still works", func() {
				svc := tu.NewService("single-ipv4",
					tu.TweakNamespace("testing"),
					tu.TweakSetIPFamilies(core_v1.IPv4Protocol),
				)

				_, err := f.Client.CoreV1().Services(svc.Namespace).Create(ctx, svc, meta_v1.CreateOptions{})
				require.NoError(f.T(), err)

				require.Eventually(f.T(), func() bool {
					svc, err = f.Client.CoreV1().Services(svc.Namespace).Get(ctx, svc.Name, meta_v1.GetOptions{})
					return err == nil && e2e.ServiceIsReconciled(svc) && e2e.ServiceHasIPAssigned(svc)
				}, 30*time.Second, time.Second)

				Expect(svc.Spec.LoadBalancerIP).To(MatchRegexp(`^192\.168\.10\.\d+$`))
				Expect(svc.Annotations["kube-vip.io/loadbalancerIPs"]).To(MatchRegexp(`^192\.168\.10\.\d+$`))
			})

			Specify("Single-stack IPv6 service still works", func() {
				svc := tu.NewService("single-ipv6",
					tu.TweakNamespace("testing"),
					tu.TweakSetIPFamilies(core_v1.IPv6Protocol),
				)

				_, err := f.Client.CoreV1().Services(svc.Namespace).Create(ctx, svc, meta_v1.CreateOptions{})
				require.NoError(f.T(), err)

				require.Eventually(f.T(), func() bool {
					svc, err = f.Client.CoreV1().Services(svc.Namespace).Get(ctx, svc.Name, meta_v1.GetOptions{})
					return err == nil && e2e.ServiceIsReconciled(svc) && e2e.ServiceHasIPAssigned(svc)
				}, 30*time.Second, time.Second)

				Expect(svc.Spec.LoadBalancerIP).To(MatchRegexp(`^2001:db8::1[0-9]$`))
			})

			AfterEach(func() {
				// Cleanup services created in this context
				svcs, _ := f.Client.CoreV1().Services(namespace).List(ctx, meta_v1.ListOptions{})
				for _, s := range svcs.Items {
					_ = f.Client.CoreV1().Services(namespace).Delete(ctx, s.Name, meta_v1.DeleteOptions{
						PropagationPolicy: ptr.To(meta_v1.DeletePropagationBackground),
					})
				}
			})
		})
	}, "testing")
})
