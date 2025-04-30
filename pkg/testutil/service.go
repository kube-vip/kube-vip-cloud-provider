package testutil

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// ServiceTweak is a func that could be used to modify Service object
type ServiceTweak func(s *corev1.Service)

// NewService returns a service type LB in default namespace
func NewService(name string, tweaks ...ServiceTweak) *corev1.Service {
	s := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
		Spec: corev1.ServiceSpec{
			Type:  corev1.ServiceTypeLoadBalancer,
			Ports: makeServicePort(corev1.ProtocolTCP, 80, 0),
		},
	}
	for _, tw := range tweaks {
		tw(s)
	}
	return s
}

// TweakNamespace returns a func that changes the namespace of a service
func TweakNamespace(ns string) ServiceTweak {
	return func(s *corev1.Service) {
		s.Namespace = ns
	}
}

// TweakAddETP returns a func that changes the ExternalTrafficPolicyType of a service
func TweakAddETP(etpType corev1.ServiceExternalTrafficPolicyType) ServiceTweak {
	return func(s *corev1.Service) {
		s.Spec.ExternalTrafficPolicy = etpType
	}
}

// TweakAddLBIngress returns a func that changes the Ingress of a service
func TweakAddLBIngress(ip string) ServiceTweak {
	return func(s *corev1.Service) {
		s.Status.LoadBalancer.Ingress = []corev1.LoadBalancerIngress{{IP: ip}}
	}
}

func makeServicePort(protocol corev1.Protocol, sourcePort, targetPort int) []corev1.ServicePort {
	sp := corev1.ServicePort{Port: int32(sourcePort), Protocol: protocol}
	if targetPort > 0 {
		sp.TargetPort = intstr.FromInt32(int32(targetPort))
	}
	return []corev1.ServicePort{sp}
}

// TweakAddPorts returns a func that changes the ServicePort of a service
func TweakAddPorts(protocol corev1.Protocol, sourcePort, targetPort int) ServiceTweak {
	return func(s *corev1.Service) {
		s.Spec.Ports = makeServicePort(protocol, sourcePort, targetPort)
	}
}

// TweakAddLBClass returns a func that changes the loadbalancerClass a service
func TweakAddLBClass(loadBalancerClass *string) ServiceTweak {
	return func(s *corev1.Service) {
		s.Spec.LoadBalancerClass = loadBalancerClass
	}
}

// TweakAddFinalizers returns a func that changes the Finalizers a service
func TweakAddFinalizers(finalizers ...string) ServiceTweak {
	return func(s *corev1.Service) {
		s.Finalizers = finalizers
	}
}

// TweakAddDeletionTimestamp returns a func that changes the DeletionTimestamp a service
func TweakAddDeletionTimestamp(time time.Time) ServiceTweak {
	return func(s *corev1.Service) {
		s.DeletionTimestamp = &metav1.Time{Time: time}
	}
}

// TweakAddAppProtocol returns a func that changes the AppProtocol a service
func TweakAddAppProtocol(appProtocol string) ServiceTweak {
	return func(s *corev1.Service) {
		s.Spec.Ports[0].AppProtocol = &appProtocol
	}
}

// TweakSetIPFamilies returns a func that changes the IPFamilies a service
func TweakSetIPFamilies(families ...corev1.IPFamily) ServiceTweak {
	return func(s *corev1.Service) {
		s.Spec.IPFamilies = families
	}
}

// TweakSetLoadbalancerIP returns a func that changes the LoadBalancerIP a service
func TweakSetLoadbalancerIP(ip string) ServiceTweak {
	return func(s *corev1.Service) {
		s.Spec.LoadBalancerIP = ip
	}
}

func ipFamilyPolicyPtr(p corev1.IPFamilyPolicy) *corev1.IPFamilyPolicy {
	return &p
}

func TweakDualStack() ServiceTweak {
	return func(s *corev1.Service) {
		s.Spec.IPFamilyPolicy = ipFamilyPolicyPtr(corev1.IPFamilyPolicyRequireDualStack)
		s.Spec.IPFamilies = []corev1.IPFamily{corev1.IPv4Protocol, corev1.IPv6Protocol}
	}
}
