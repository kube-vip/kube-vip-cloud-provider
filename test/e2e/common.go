//go:build e2e

package e2e

import (
	core_v1 "k8s.io/api/core/v1"
	servicehelper "k8s.io/cloud-provider/service/helpers"

	"github.com/kube-vip/kube-vip-cloud-provider/pkg/provider"
)

func ServiceIsReconciled(svc *core_v1.Service) bool {
	return svc.Labels[provider.ImplementationLabelKey] == provider.ImplementationLabelValue &&
		servicehelper.HasLBFinalizer(svc)
}

func ServiceHasIPAssigned(svc *core_v1.Service) bool {
	return svc.Annotations[provider.LoadbalancerIPsAnnotation] != "" &&
		svc.Spec.LoadBalancerIP != ""
}
