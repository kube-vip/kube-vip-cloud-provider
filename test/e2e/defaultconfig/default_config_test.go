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

	tu "github.com/kube-vip/kube-vip-cloud-provider/pkg/testutil"
	"github.com/kube-vip/kube-vip-cloud-provider/test/e2e"
)

// Each suite load default manifest from scratch, so that changes on manifest objects won't impact other tests suites.
var f = e2e.NewFramework()

func TestDeployWithDefaultConfig(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "deploy with default config")
}

var _ = BeforeSuite(func() {
	// By default, configMap only contains below 3 cidr
	// cidr-default: 192.168.0.200/29
	// cidr-plunder: 192.168.0.210/29
	// cidr-testing: 192.168.0.220/29

	require.NoError(f.T(), f.Deployment.EnsureResources())
})

var _ = AfterSuite(func() {
	// Reset resource requests for other tests.
	require.NoError(f.T(), f.Deployment.DeleteResources())
})

var watchedNamespace = "testing"

var _ = Describe("Default config", func() {
	Context("Deploy service in namespace that kube-vip-cloud-provider is configured and is not configured", func() {
		f.NamespacedTest("create-services-in-different-namespace", func(namespace string) {
			Specify("Service not be reconcile if namespace is not configured namespace testing, service in default namespace should be reconciled", func() {
				ctx := context.TODO()
				By("Create a service type LB in namespace that's not testing")
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

				By("Create a service type LB in testing namespace")
				svc = tu.NewService("test2", tu.TweakNamespace("watchedNamespace"))
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
		}, watchedNamespace)
	})
})
