---
title: Host Port Registry
authors:
  - "@squeed"
reviewers:
  - "@danw"
  - "@jsafrane"
  - "@wking"
approvers:
  - "@russelb"
creation-date: 2020-08-26 
last-updated: 2020-08-26
status: informational
---

# Host Port Registry

OpenShift has a number of host-level services that listen on a socket. Since they
are not isolated by network namespace, they can conflict if both listen on the same
port.

This document serves as the authoritative list of port assignments.

## Background

Ports 9000-9999 are available for general use by system services. We require that
customers open this range [and several others](https://github.com/openshift/openshift-docs/blob/master/modules/installation-network-user-infra.adoc)
between nodes.

The user-facing documentation, informed by the installer's
terraform templates, is the authoritative source for usable port ranges.

These ports are used by certain services that, for whatever reason, must run
in the host network (or use host-port forwarding).

The Kubernetes scheduler is aware of host ports, and will fail to schedule pods
on a node where there would be a port conflict.

### Localhost

Separately, it is a common pattern for a pod to have one or more processes only
listening on localhost. Since all HostNetwork pods share a network namespace,
coordination is also required in this case.

## Requirements

All processes that wish to listen on a host port MUST

- Have an entry in the table below
- be in a [documented range](https://github.com/openshift/openshift-docs/blob/master/modules/installation-network-user-infra.adoc), 
  if they are intended to be reachable 
- declare that port in their `Pod.Spec`

Localhost-only ports SHOULD be outside of this range, to leave room.

# Port Registry

## External

Ports are assumed to be used on all nodes in all clusters unless otherwise specified.


### TCP

| Port  | Process   | Owning Team | Since | Notes |
|-------|-----------|-------------|-------|-------|
| 80    | haproxy   | net edge    | 3.0 | HTTP routes; baremetal only; only on nodes running router pod replicas |
| 443   | haproxy   | net edge    | 3.0 | HTTPS routes; baremetal only; only on nodes running router pod replicas |
| 1936  | openshift-router | net edge | 3.0 | healthz/stats; baremetal only; only on nodes running router pod replicas |
| 2379  | etcd      | etcd || control plane only |
| 2380  | etcd      | etcd || control plane only |
| 3306  | mariadb   | kni | 4.4 | baremetal ironic DB, control plane only |
| 5050  | ironic-inspector | kni | 4.4 | baremetal provisioning, control plane only |
| 6180  | httpd     | kni | 4.4 | baremetal provisioning server, control plane only |
| 6181  | httpd     | kni | 4.7 | baremetal image cache, control plane only |
| 6385  | ironic-api   | kni | 4.4 | baremetal provisioning, control plane only |
| 6443  | kube-apiserver | apiserver || control plane only |
| 8089  | ironic-conductor | kni | 4.4 | baremetal provisioning, control plane only |
| 9001  | machine-config-daemon oauth proxy | node || metrics |
| 9100  | node-exporter | monitoring || metrics |
| 9101  | openshift-sdn kube-rbac-proxy | sdn || metrics, openshift-sdn only |
| 9101  | kube-proxy | sdn || metrics, third-party network plugins only, deprecated |
| 9102  | ovn-kubernetes master kube-rbac-proxy | sdn || metrics, control plane only, ovn-kubernetes only |
| 9102  | kube-proxy | sdn | 4.7 | metrics, third-party network plugins only |
| 9103  | ovn-kubernetes node kube-rbac-proxy | sdn || metrics |
| 9537  | crio      | node || metrics |
| 9641  | ovn-kubernetes northd | sdn | 4.3 | control plane only, ovn-kubernetes only |
| 9642  | ovn-kubernetes southd | sdn | 4.3 | control plane only, ovn-kubernetes only |
| 9643  | ovn-kubernetes northd | sdn | 4.3 | control plane only, ovn-kubernetes only |
| 9644  | ovn-kubernetes southd | sdn | 4.3 | control plane only, ovn-kubernetes only |
| 9978  | etcd      | etcd || metrics, control plane only |
| 9979  | etcd      | etcd || ?, control plane only |
| 10010 | crio | node || stream port|
| 10250 | kubelet | node || kubelet api |
| 10251 | kube-scheduler | apiserver || healthz, control plane only |
| 10255 | kube-proxy | sdn | 4.7 | healthz, third-party network plugins only |
| 10256 | openshift-sdn | sdn || healthz, openshift-sdn only |
| 10256 | kube-proxy | sdn || healthz, third-party network plugins only, deprecated |
| 10257 | kube-controller-manager | apiserver || metrics, healthz, control plane only |
| 10259 | kube-scheduler | apiserver || metrics, control plane only |
| 10357 | cluster-policy-controller | apiserver || healthz, control plane only |
| 10443 | haproxy   | net edge    | 3.0 | HAProxy internal `fe_no_sni` frontend; localhost only; baremetal only; only on nodes running router pod replicas |
| 10444 | haproxy   | net edge    | 3.0 | HAProxy internal `fe_sni` frontend; localhost only; baremetal only; only on nodes running router pod replicas |
| 17697 | kube-apiserver | apiserver || ?, control plane only |
| 22623 | machine-config-server | node || control plane only |
| 22624 | machine-config-server | node || control plane only |
| 29101 | openshift-sdn | sdn || metrics |
| 60000 | baremetal-operator | kni || metrics, 4.6+, control plane only |


### UDP

| Port  | Process   | Owning Team | Since | Notes |
|-------|-----------|-------|-------|-------|
| 4789  | openshift-sdn vxlan | sdn | 3.0 | openshift-sdn only |
| 6081  | ovn-kubernetes geneve | sdn | 4.3 | ovn-kubernetes only |


## Localhost-only
| Port  | Process   | Owning Team | Since | Notes |
|-------|-----------|-------------|-------|-------|
| 4180  | machine-config-daemon oauth-proxy | node ||
| 8797  | machine-config-daemon | node |4.0| metrics |
| 9443 | kube-controller-manager | workloads || recovery-controller|
| 9977  | etcd | etcd || ? |
| 10248 | kubelet | node || healthz |
| 10300 | various CSI drivers | storage | 4.6 | healthz |
| 10301 | various CSI drivers | storage | 4.6 | healthz |
| 11443 | kube-scheduler | workloads || recovery-controller|
| 29102 | ovn-kubernetes | sdn || metrics, ovn-kubernetes only |
| 29103 | ovn-kubernetes | sdn || metrics, ovn-kubernetes only |


## Previously allocated

If a feature is completely removed, (not just deprecated), then any now-free
ports should be noted here, along with the version in which they were removed.


## Future 

We can enforce this in an automated fashion in the future. We should write tests
that ensure

- pods opening host ports declare those ports in their Pod.Spec (host-level processes excluded)
- pods that declare host ports are in the port registry. The registry will need to be machine-readable
