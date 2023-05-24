# kube-vip-cloud-provider

The [Kube-Vip](https://kube-vip.io) cloud provider is a general purpose cloud-provider for on-prem bare-metal or virtualised environments. It's designed to work with the [kube-vip](https://kube-vip.io) project however if a load-balancer solution follows the [Kubernetes](https://kubernetes.io) conventions then this cloud-provider will provide IP addresses that another solution can advertise.

## Architecture

The `kube-vip-cloud-provider` will only implement the `loadBalancer` functionality of the out-of-tree cloud-provider functionality. The design is to keep be completely decoupled from any other technologies other than the Kubernetes API, this means that the only contract is between the kube-vip-cloud-provider and the kubernetes services schema. The cloud-provider wont generate configuration information in any other format, it's sole purpose is to ensure that a new service of type:`loadBalancer` has been assigned an address from an address pool. It does this by updating the `<service>.annotations.kube-vip.io/loadbalancerIPs` and `<service>.spec.loadBalancerIP` with an address from it's IPAM, the responsibility of advertising that address **and** updating the `<service>.status.loadBalancer.ingress.ip` is left to the actual load-balancer such as [kube-vip.io](https://kube-vip.io).

`<service>.spec.loadBalancerIP` [is deprecated](https://github.com/kubernetes/kubernetes/pull/107235) in k8s 1.24, kube-vip-cloud-provider will only updates the annotations `<service>.annotations.kube-vip.io/loadbalancerIPs` in the future.

## IP address functionality

- IP address pools by CIDR.
- IP ranges [start address - end address].
- Multiple pools by CIDR per namespace.
- Multiple IP ranges per namespace (handles overlapping ranges).
- Setting of static addresses through `--load-balancer-ip=x.x.x.x` or through annotations `kube-vip.io/loadbalancerIPs: x.x.x.x`.
- Setting the special IP `0.0.0.0` for DHCP workflow.
- Support single stack IPv6 or IPv4.
- Support loadbalancerClass `kube-vip.io/kube-vip-class` or custom loadbalancerClass name.

## Installing the `kube-vip-cloud-provider`

We can apply the controller manifest directly from this repository to get the latest release:

```
$ kubectl apply -f https://raw.githubusercontent.com/kube-vip/kube-vip-cloud-provider/main/manifest/kube-vip-cloud-controller.yaml
```

It uses a `Deployment` and can always be viewed with the following command:

```
kubectl describe deployment/kube-vip-cloud-provider -n kube-system
POD_NAME=$(kubectl get po -n kube-system | grep kube-vip-cloud-provider | cut -d' ' -f1)
kubectl describe pod/$POD_NAME -n kube-system
```

## Global and namespace pools

### Global pool

Any service in any namespace will take an address from the global pool `cidr/range`-global.

### Namespace pool

A service will take an address based upon its namespace pool `cidr/range`-`namespace`. These would look like the following:

```
$ kubectl get configmap -n kube-system kubevip -o yaml

apiVersion: v1
kind: ConfigMap
metadata:
  name: kubevip
  namespace: kube-system
data:
  cidr-default: 192.168.0.200/29
  cidr-development: 192.168.0.210/29
  cidr-finance: 192.168.0.220/29
  cidr-testing: 192.168.0.230/29
  cidr-ipv6: 2001::10/127
```

## Create an IP pool using a CIDR

```
kubectl create configmap --namespace kube-system kubevip --from-literal cidr-global=192.168.0.220/29
```

## Create an IP range

```
kubectl create configmap --namespace kube-system kubevip --from-literal range-global=192.168.0.200-192.168.0.202
```

## Multiple pools or ranges

We can apply multiple pools or ranges by seperating them with commas.. i.e. `192.168.0.200/30,192.168.0.200/29` or `2001::12/127,2001::10/127` or `192.168.0.10-192.168.0.11,192.168.0.10-192.168.0.13` or `2001::10-2001::14,2001::20-2001::24`

## Special DHCP CIDR

Set the CIDR to `0.0.0.0/32`, that will make the controller to give all _LoadBalancers_ the IP `0.0.0.0`.

## LoadbalancerClass support

If users only want kube-vip-cloud-provider to allocate ip for specific set of services, they can pass `KUBEVIP_ENABLE_LOADBALANCERCLASS: true` as an environment variable to kube-vip-cloud-provider. kube-vip-cloud-provider will only allocate ip to service with `spec.loadBalancerClass: kube-vip.io/kube-vip-class`.

If users want to use a custom loadbalancerClass name, they can pass `KUBEVIP_CUSTOM_LOADBALANCERCLASS_NAME: <custom name>` as an environment variable, without setting `KUBEVIP_ENABLE_LOADBALANCERCLASS`. They set `spec.loadBalancerClass: <custom name>` on the service.

## Debugging

The logs for the cloud-provider controller can be viewed with the following command:

```
kubectl logs -n kube-system kube-vip-cloud-provider-0 -f
```
