---
title: kube-apiserver-Certificates
authors:
  - "@deads2k"
reviewers:
  - "@sttts"
approvers:
  - "@deads2k"
creation-date: 2019-09-27
last-updated: 2019-09-27
status: implemented
see-also:
replaces:
superseded-by:
---

# kube-apiserver Certificates

This is documentation about the certificates that Kubernetes has and how the certificates
for the kube-apiserver in particular are managed.

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift/docs]

## Summary

This is documentation about the certificates that Kubernetes has and how the certificates
for the kube-apiserver in particular are managed.

## Motivation

We face many questions about 
1. What a specific certificate/flag is for.
2. How a specific certificate is created.
3. What breaks when certificate fails.
4. How often a certificate is rotated.

This document attempts to answer these questions.


### Goals

### Non-Goals

## Proposal

### TLS related flags for individual binaries

#### kube-apiserver
| Flag | What’s it for |
|---|---|
| `--etcd-cafile`  | CA bundle used to verify the etcd server really is the etcd server |
| `--etcd-certfile` | Client cert used to identify KAS to the etcd server |
| `--etcd-keyfile` | Client key used to identify KAS to the etcd server |
| `--tls-cert-file` | Serving cert used to serve requests not matching SNI.  Must be verifiable with `kube-controller-manager --root-ca-file` or service must be handled with SNI. |
| `--tls-private-key-file` | Serving key used to serve requests not matching SNI |
| `--tls-sni-cert-key` | Special flag format to specify hostname-pattern,cert,key tuples to serve matching SNI requests.  If used for kubernetes.default.service, must be verifiable with `kube-controller-manager --root-ca-file`.. |
| `--client-ca-file` | CA bundle used to verify client certificate connections from clients and identify users. (I am Bob).  Must be able to verify `kube-controller-manager --cluster-signing-cert-file` or `kubelet --rotate-certificates` will fail. |
| `--requestheader-client-ca-file` | CA bundle used to verify client certificate connections from front proxies that are asserting the identity of user. (This request is from Bob).  Must be able to verify `kube-apiserver --proxy-client-cert-file` or aggregation in the cluster will fail by default. |
| `--kubelet-certificate-authority` | CA bundle used to verify kubelets for connections from KAS to kubelet.  (Think logs,exec,etc).  Must be able to verify `kubelet --tls-cert-file`.  Must be able to verify `kube-controller-manager --cluster-signing-cert-file` or `kubelet --rotate-server-certificates` will fail. |
| `--kubelet-client-certificate` | Client cert used to identify KAS to the kubelets.  Must be verifiable by `kubelet --client-ca-file`. |
| `--kubelet-client-key` | Client key used to identify KAS to the kubelets |
| `--proxy-client-cert-file` | Client cert used to identify KAS to aggregated API servers as a front proxy.  Must be verifiable by `kube-apiserver --requestheader-client-ca-file` or aggregation in the cluster will fail by default |
| `--proxy-client-key-file` | Client key used to identify KAS to aggregated API servers as a front proxy |
| `--service-account-key-file` | RSA keys used to verify ServiceAccount tokens.  Must be able to verify `kube-controller-manager --service-account-private-key-file` for all keys you want to continue working. |
|  |  |
|  |  |
|  |  |

#### kubelet
| Flag | What’s it for |
|---|---|
| `--client-ca-file` | CA bundle used to verify client certificate connections from clients and identify users. (I am Bob).  Must be able to verify `kube-apiserver --kubelet-client-certificate` or some endpoints will fail. |
| `--tls-cert-file` | Serving cert used to serve requests. Must be verifiable by  `kube-apiserver--kubelet-certificate-authority` or some endpoints will fail. |
| `--tls-private-key-file` | Serving key used to serve requests |
| `--rotate-certificates` | If true, get client cert/key pairs for authentication to kube-apiserver from the CSR API.  This interacts with `kube-apiserver --client-ca-file` and `kube-controller-manager --cluster-signing-cert-file`. |
| `--rotate-server-certificates` | If true, get serving cert/key pairs (--tls-cert-file/--tls-private-key-file) from the CSR API.   This interacts with `kube-apiserver--kubelet-certificate-authority` and `kube-controller-manager --cluster-signing-cert-file`. |

#### kube-controller-manager
| Flag | What’s it for |
|---|---|
| `--client-ca-file` | CA bundle used to verify client certificate connections from clients and identify users. (I am Bob) |
| `--tls-cert-file` | Serving cert used to serve requests |
| `--tls-private-key-file` | Serving key used to serve requests |
| `--cluster-signing-cert-file` | Signing cert used to issue approved CSR requests.  Must be verifiable with `kube-apiserver --kubelet-client-certificate` and `kube-apiserver --client-ca-file` or `kubelet --rotate-certificates` will fail. |
| `--cluster-signing-key-file` | Signing key used to issue approved CSR requests |
| `--requestheader-client-ca-file` | CA bundle used to verify client certificate connections from front proxies that are asserting the identity of user. (This request is from Bob) |
| `--root-ca-file` | CA bundle injected into ServiceAccount token secrets.  It is only intended to be used to verify a connection to the kube-apiserver on the service network.  All other uses are either wrong or coincidence.  Must be able to verify `kube-apiserver --tls-cert-file` |
| `--service-account-private-key-file` | RSA key used to sign ServiceAccount tokens.  Must be verifiable by `kube-apiserver --service-account-key-file` or ServiceAccounts will not be able to authenticate |


### What is exposed by operators?
Resources in `openshift-config` and `openshift-config-managed` are soft API contracts to operators and 
`openshift-config-managed` is a soft API contract from operators.  
We’ll try not to break anything, but the guarantee isn’t strong.  
These are for coordinating our operators, not apps in a cluster.

Resources in other `openshift-*-operator` namespaces of `openshift-*` namespaces have no guarantees at all.  
They can change at any time, for any reason, without any notice.

#### openshift-config-managed
| Resource | What’s it for | Who produces it | Who consumes it |
|---|---|---|---|
| cm/kube-apiserver-client-ca | `kube-apiserver --client-ca-file` | kube-apiserver-operator | openshift-apiserver, kube-controller-manager |
| cm/kubelet-serving-ca | `kube-apiserver --kubelet-certificate-authority` | kube-apiserver-operator |  |
| cm/kube-apiserver-serving-ca | `kube-controller-manager --root-ca` | kube-apiserver-operator |  |
| cm/ kube-apiserver-aggregator-ca | `kube-apiserver --requestheader-client-ca-file` | kube-apiserver-operator | openshift-apiserver, kube-controller-manager |
| cm/csr-controller-ca | `kube-apiserver --kubelet-certificate-authority` and input to cm/kube-apiserver-client-ca | kube-controller-manager-operator | kube-apiserver |
| cm/sa-token-signing-certs | `kube-apiserver --service-account-key-file` | kube-controller-manager-operator | kube-apiserver |
| secret/kube-controller-manager-client-cert-key | `kube-controller-manager --kubeconfig` | kube-apiserver | kube-controller-manager |
| secret/kube-scheduler-client-cert-key | `kube-scheduler --kubeconfig` | kube-apiserver | kube-scheduler |

#### openshift-config
| Resource | What’s it for | Who produces it | Who consumes it |
|---|---|---|---|
| cm/etcd-serving-ca | `kube-apiserver --etcd-cafile` | installer | kube-apiserver, openshift-apiserver |
| secret/etcd-client | `kube-controller-manager --etcd-certfile` | installer | kube-apiserver, openshift-apiserver |


### Implementation Details/Notes/Constraints [optional]

### Risks and Mitigations

## Design Details

### Test Plan

### Graduation Criteria

### Upgrade / Downgrade Strategy

### Version Skew Strategy

## Implementation History

## Drawbacks

## Alternatives

