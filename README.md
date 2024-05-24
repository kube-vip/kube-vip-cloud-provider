# kube-vip-cloud-provider

The [Kube-Vip](https://kube-vip.io) cloud provider is a general purpose cloud-provider for on-prem bare-metal or virtualised environments. It's designed to work with the [kube-vip](https://kube-vip.io) project however if a load-balancer solution follows the [Kubernetes](https://kubernetes.io) conventions then this cloud-provider will provide IP addresses that another solution can advertise.

## Architecture

The `kube-vip-cloud-provider` will only implement the `loadBalancer` functionality of the out-of-tree cloud-provider functionality. The design is to keep be completely decoupled from any other technologies other than the Kubernetes API, this means that the only contract is between the kube-vip-cloud-provider and the kubernetes services schema. The cloud-provider wont generate configuration information in any other format, it's sole purpose is to ensure that a new service of type:`loadBalancer` has been assigned an address from an address pool. It does this by updating the `<service>.annotations.kube-vip.io/loadbalancerIPs` and `<service>.spec.loadBalancerIP` with an address from it's IPAM, the responsibility of advertising that address **and** updating the `<service>.status.loadBalancer.ingress.ip` is left to the actual load-balancer such as [kube-vip.io](https://kube-vip.io).

`<service>.spec.loadBalancerIP` [is deprecated](https://github.com/kubernetes/kubernetes/pull/107235) in k8s 1.24, kube-vip-cloud-provider will only updates the annotations `<service>.annotations.kube-vip.io/loadbalancerIPs` in the future.

## IP address functionality

- IP address pools by CIDR (excluding the network(first) address and broadcast(last) address)
- IP ranges [start address - end address]
- Multiple pools by CIDR per namespace
- Multiple IP ranges per namespace (handles overlapping ranges)
- Support for mixed IP families when specifying multiple pools or ranges
- Setting of static addresses through `--load-balancer-ip=x.x.x.x` or through annotations `kube-vip.io/loadbalancerIPs: x.x.x.x`
- Setting the special IP `0.0.0.0` for DHCP workflow.
- Support single stack IPv6 or IPv4
- Support for dualstack via the annotation: `kube-vip.io/loadbalancerIPs: 192.168.10.10,2001:db8::1`
- Support ascending and descending search order when allocating IP from pool or range by setting search-order=desc
- Support loadbalancerClass `kube-vip.io/kube-vip-class`
- Support assigning multiple services on single VIP (IPv4 only, optional)
- Support specifying service interface per namespace or at global level

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

## Create an IP pool using a CIDR and descending search order

```
kubectl create configmap --namespace kube-system kubevip --from-literal cidr-global=192.168.0.220/29 --from-literal search-order=desc
```

## Create an IP range

```
kubectl create configmap --namespace kube-system kubevip --from-literal range-global=192.168.0.200-192.168.0.202
```

## Create an IP range and descending search order

```
kubectl create configmap --namespace kube-system kubevip --from-literal range-global=192.168.0.200-192.168.0.202 --from-literal search-order=desc
```

## Multiple pools or ranges

We can apply multiple pools or ranges by seperating them with commas.. i.e. `192.168.0.200/30,192.168.0.200/29` or `2001::12/127,2001::10/127` or `192.168.0.10-192.168.0.11,192.168.0.10-192.168.0.13` or `2001::10-2001::14,2001::20-2001::24` or `192.168.0.200/30,2001::10/127`

## Dualstack Services

Suppose a pool in the configmap is as follows: `range-default: 192.168.0.10-192.168.0.11,2001::10-2001::11`
and there are no IPs currently in use.

Then by creating a service with the following spec (with `IPv6` specified first in `ipFamilies`):
```yaml
apiVersion: v1
kind: Service
metadata:
  name: my-service
  labels:
    app.kubernetes.io/name: MyApp
spec:
  ipFamilyPolicy: PreferDualStack
  ipFamilies:
  - IPv6
  - IPv4
  selector:
    app.kubernetes.io/name: MyApp
  ports:
    - protocol: TCP
      port: 80
```

The service will receive the annotation `kube-vip.io/loadbalancerIPs:
2001::10,192.168.0.10` following the intent to prefer IPv6. Conversely, if
`IPv4` were specified first, then the IPv4 address will appear first in the
annotation.

With the `PreferDualStack` IP family policy, kube-vip-cloud-provider will make a
best effort to provide at least one IP in `loadBalancerIPs` as long as any IP family
in the pool has available addresses.

If `RequireDualStack` is specified, then kube-vip-cloud-provider will fail to
set the `kube-vip.io/loadbalancerIPs` annotation if it cannot find an available
address in each of both IP families for the pool.


## Special DHCP CIDR

Set the CIDR to `0.0.0.0/32`, that will make the controller to give all _LoadBalancers_ the IP `0.0.0.0`.


## LoadbalancerClass support

If users only want kube-vip-cloud-provider to allocate ip for specific set of services, they can pass `KUBEVIP_ENABLE_LOADBALANCERCLASS: true` as an environment variable to kube-vip-cloud-provider. kube-vip-cloud-provider will only allocate ip to service with `spec.loadBalancerClass: kube-vip.io/kube-vip-class`.

## Allow multiple IPv4 services to share a VIP

When enabled, kube-vip-cloud-provider tries to assign services to already used VIPs if the ports of the services
do not overlap.
If you want to enable VIP-sharing between services, you can set `allow-shared`-`namespace` to true. It follows the same rules as
the configuration for global and namespace pools.

Example-object with sharing enabled for namespace `development`:
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
  allow-share-development: true
```

### Namespace pool

Kube-vip 0.8.0 supports `kube-vip.io/serviceInterface` annotation on service type LB. Now user can specify a ip range/cidr at namespace level, we would assume these ips within a namespace should share the same interface, then we support specifying interface per namespace level by

```
$ kubectl get configmap -n kube-system kubevip -o yaml

apiVersion: v1
kind: ConfigMap
metadata:
  name: kubevip
  namespace: kube-system
data:
  cidr-default: 192.168.0.200/29
  interface-default: eth3
  cidr-development: 172.16.0.0/29
  interface-default: eth5
```

`interface-global` could be used to specify all ips would use this ip address. If there is no interface specified for a namespace, it will fall back to this `interface-global`. But this is usually not needed since kube-vip has `vip_servicesinterface` for user to define default interface for service type LB.

## Debugging

The logs for the cloud-provider controller can be viewed with the following command:

```
kubectl logs -n kube-system kube-vip-cloud-provider-0 -f
```
