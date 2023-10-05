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
last-updated: 2022-10-26
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

| Port  | Process   | Protocol | Control-plane only | Owning Team | Since | Notes |
|-------|-----------|----------|--------------------|-------------|-------|-------|
| 80    | haproxy   ||| net edge    | 3.0 | HTTP routes; baremetal only; only on nodes running router pod replicas |
| 443   | haproxy   ||| net edge    | 3.0 | HTTPS routes; baremetal only; only on nodes running router pod replicas |
| 1936  | openshift-router ||| net edge | 3.0 | healthz/stats; baremetal only; only on nodes running router pod replicas |
| 2041  | konnectivity-agent ||| hypershift | 4.10 | Only for Hypershift guest clusters not on IBM cloud |
| 2379  | etcd      || yes | etcd |||
| 2380  | etcd      || yes | etcd |||
| 5050  | ironic-inspector || yes | metal | 4.4 | baremetal provisioning |
| 6080  | cluster-kube-apiserver-operator || yes | apiserver |||
| 6180  | httpd     || yes | metal | 4.4 | baremetal provisioning server |
| 6181  | httpd     || yes | metal | 4.7 | baremetal image cache |
| 6183  | httpd     || yes | metal | 4.10 | baremetal provisioning server (TLS) |
| 6385  | ironic    || yes | metal | 4.4 | baremetal provisioning |
| 6388  | ironic (internal) || yes | metal | 4.12 | baremetal provisioning |
| 6443  | kube-apiserver || yes | apiserver |||
| 9001  | machine-config-daemon oauth proxy ||| node || metrics |
| 9099  | cluster-version operator | HTTPS | yes | updates || metrics |
| 9100  | node-exporter || no | monitoring || metrics |
| 9101  | openshift-sdn kube-rbac-proxy ||| sdn || metrics, openshift-sdn only |
| 9101  | kube-proxy ||| sdn || metrics, third-party network plugins only, deprecated |
| 9102  | ovn-kubernetes master kube-rbac-proxy || yes | sdn || metrics, ovn-kubernetes only |
| 9102  | kube-proxy ||| sdn | 4.7 | metrics, third-party network plugins only |
| 9103  | ovn-kubernetes node kube-rbac-proxy ||| sdn || metrics |
| 9105  | ovn-kubernetes node kube-rbac-proxy-ovn-metrics ||| sdn | 4.10 | metrics |
| 9106  | sdn controller kube-rbac-proxy || yes | sdn | 4.10 | sdn only |
| 9107  | ovn-kubernetes node ||| sdn | 4.12 | egressip-node-healthcheck-port, sdn interface only, ovn-kubernetes only |
| 9108 | ovn-kubernetes kube-rbac-proxy || yes | sdn | 4.14 | ovnkube-control-plane |
| 9120  | metallb ||| sdn | 4.9 | metrics|
| 9121  | metallb ||| sdn | 4.9 | metrics|
| 9122  | metallb ||| sdn | 4.9 | leader election protocol |
| 9191  | cluster-machine-approver ||| cluster infra | 4.3 | metrics |
| 9192  | cluster-machine-approver ||| cluster infra | 4.3 | metrics |
| 9193  | cluster-machine-approver-capi ||| cluster infra | 4.11 | metrics |
| 9194  | cluster-machine-approver-capi ||| cluster infra | 4.11 | metrics |
| 9200-9219  | various CSI drivers ||| storage | 4.8 | metrics |
| 9258  | cluster-cloud-controller-manager-operator ||| cluster infra | 4.9 | metrics, control plane only |
| 9300  | kube-proxy ||| sdn | 4.12 | metrics, ingress node firewall |
| 9301  | kube-proxy ||| sdn | 4.12 | metrics, ingress node firewall |
| 9444  | haproxy ||| sdn | 4.7 | on-prem internal loadbalancer, healthcheck port |
| 9445  | haproxy ||| sdn | 4.7 | on-prem internal loadbalancer |
| 9446  | baremetal-operator || yes | metal | 4.9 | healthz; baremetal provisioning |
| 9447  | baremetal-operator || yes | metal | 4.10 | webhook; baremetal provisioning |
| 9448  | run-once-duration-override-operator ||| workloads | 4.13 | webhook; run-once-duration-override |
| 9537  | crio      ||| node || metrics |
| 9641  | ovn-kubernetes northd || yes | sdn | 4.3 | ovn-kubernetes only |
| 9642  | ovn-kubernetes southd || yes | sdn | 4.3 | ovn-kubernetes only |
| 9643  | ovn-kubernetes northd || yes | sdn | 4.3 | ovn-kubernetes only |
| 9644  | ovn-kubernetes southd || yes | sdn | 4.3 | ovn-kubernetes only |
| 9978  | etcd      || yes | etcd || metrics |
| 9979  | etcd      || yes | etcd || ? |
| 9980  | etcd      || yes | etcd || healthz, readyz |
| 10010 | crio ||| node || stream port |
| 10250 | kubelet ||| node || kubelet api |
| 10251 | kube-scheduler || yes | apiserver || healthz |
| 10255 | kube-proxy ||| sdn | 4.7 | healthz, third-party network plugins only |
| 10256 | openshift-sdn ||| sdn || healthz, openshift-sdn only |
| 10256 | kube-proxy ||| sdn || healthz, third-party network plugins only, deprecated |
| 10257 | kube-controller-manager || yes | apiserver || metrics, healthz |
| 10258 | cloud-controller-manager || yes | cluster infra | 4.9 | metrics, healthz |
| 10259 | kube-scheduler || yes | apiserver || metrics |
| 10263 | cloud-node-manager ||| cluster infra | 4.9 | metrics, healthz, some platforms only |
| 10357 | cluster-policy-controller || yes | apiserver || healthz |
| 10443 | haproxy   ||| net edge    | 3.0 | HAProxy internal `fe_no_sni` frontend; localhost only; baremetal only; only on nodes running router pod replicas |
| 10444 | haproxy   ||| net edge    | 3.0 | HAProxy internal `fe_sni` frontend; localhost only; baremetal only; only on nodes running router pod replicas |
| 17697 | kube-apiserver || yes | apiserver || ? |
| 22623 | machine-config-server || yes | node |||
| 22624 | machine-config-server || yes | node |||
| 60000 | baremetal-operator || yes | metal | 4.6 | metrics |


### UDP

| Port  | Process   | Protocol | Control-plane only | Owning Team | Since | Notes |
|-------|-----------|----------|--------------------|-------------|-------|-------|
| 500   | ovn-kubernetes IPsec ||| sdn | 4.7 | ovn-kubernetes only |
| 4500  | ovn-kubernetes IPsec ||| sdn | 4.7 | ovn-kubernetes only |
| 4789  | openshift-sdn / ovn-kubernetes VXLAN ||| sdn | 3.0 | openshift-sdn always, ovn-kubernetes when using Windows hybrid networking |
| 6081  | ovn-kubernetes geneve ||| sdn | 4.3 | ovn-kubernetes only |
| 9122  | metallb ||| sdn | 4.9 | leader election protocol |

## Localhost-only

| Port  | Process   | Protocol | Control-plane only | Owning Team | Since | Notes |
|-------|-----------|----------|--------------------|-------------|-------|-------|
| 4180  | machine-config-daemon oauth-proxy ||| node ||
| 8797  | machine-config-daemon ||| node | 4.0 | metrics |
| 9259  | cluster-cloud-controller-manager-operator || yes | cluster infra | 4.9 | healthz |
| 9260  | cluster-cloud-controller-manager-operator-config-sync || yes | cluster infra | 4.10 | healthz |
| 9443  | kube-controller-manager ||| workloads || recovery-controller |
| 9977  | etcd ||| etcd || ? |
| 10248 | kubelet ||| node || healthz |
| 10300 | various CSI drivers ||| storage | 4.6 | healthz |
| 10301 | various CSI drivers ||| storage | 4.6 | healthz |
| 10302 | various CSI drivers ||| storage | 4.7 | healthz |
| 10303 | various CSI drivers ||| storage | 4.9 | healthz |
| 11443 | kube-scheduler ||| workloads || recovery-controller |
| 29100 | openshift-sdn ||| sdn |4.10| metrics |
| 29101 | openshift-sdn ||| sdn || metrics |
| 29102 | ovn-kubernetes ||| sdn || metrics, ovn-kubernetes only |
| 29103 | ovn-kubernetes ||| sdn || metrics, ovn-kubernetes only |
| 29105 | ovn-kubernetes ||| sdn |4.10| metrics, ovn-kubernetes only|
| 29108 | ovn-kubernetes || yes | sdn |4.14| metrics, ovn-kubernetes only |
| 29150 | metallb ||| sdn | 4.9 | metrics |
| 29151 | metallb ||| sdn | 4.9 | metrics |
| 29445 | haproxy ||| sdn | 4.7 | on-prem internal loadbalancer, stats port |
| 39300 | manager ||| sdn | 4.12 | metrics, ingress node firewall |
| 39301 | daemon ||| sdn | 4.12 | metrics, ingress node firewall |

## Previously allocated

If a feature is completely removed, (not just deprecated), then any now-free
ports should be noted here, along with the version in which they were removed.

| Port  | Process   | Protocol | Control-plane only | Owning Team | Since | Notes |
|-------|-----------|----------|--------------------|-------------|-------|-------|
| 3306  | mariadb   || yes | metal | 4.4 | 4.11 | baremetal ironic DB |
| 8089  | ironic-conductor || yes | metal | 4.4 | 4.11 | baremetal provisioning |

## Future

We can enforce this in an automated fashion in the future. We should write tests
that ensure

- pods opening host ports declare those ports in their Pod.Spec (host-level processes excluded)
- pods that declare host ports are in the port registry. The registry will need to be machine-readable
