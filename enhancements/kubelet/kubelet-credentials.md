---
title: master-bootstrap-credentials
authors:
  - "@deads2k"
reviewers:
  - "@rphillips"
approvers:
  - "@rphillips"
creation-date: 2020-01-31
last-updated: 2020-11-18
status: implementable
see-also:
  - https://bugzilla.redhat.com/show_bug.cgi?id=1693951
replaces:
superseded-by:
---

# Kubelet Authentication and Authorization

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Open Questions [optional]

## Summary

Kubelets need to be individually authenticated so that they can be individually authorized.
Individual authentication and authorization allows for fine-grained API access control which limits API access to only
those resources required to run the pods scheduled on a particular node.
For example, kubelets can only read secrets that are mounted by pods scheduled to the node running the kubelet. 

## Motivation

### Goals

1. bootstrap credentials should be 
    1. long lived.  It should not expire on short time frames.
    2. revokable.  We must be able to revoke this credential if it is compromised.
2. kubelets need to be individually authenticated
3. kubelets are individually authorized for least privilege on a particular node.

### Non-Goals

## Proposal

### Authentication versus Authorization

In Kubernetes, authentication and authorization are separate steps.
1. Authentication identifies the client making a request.  This is where the username and list of groups comes from.
   See the [Kubernetes Authentication documentation](https://kubernetes.io/docs/reference/access-authn-authz/authentication/)
   for full details.
    1. Client certificates authentication maps the common name to a username and maps the organization fields to the 
       groups for that user.
       See the [x509 client certificate documentation](https://kubernetes.io/docs/reference/access-authn-authz/authentication/#x509-client-certs)
       for full details. 
2. Authorization determines whether a particular username, groups, extra-info (things like scopes) tuple is allowed to
   perform the requested action.
   OpenShift always runs with [RBAC](https://kubernetes.io/docs/reference/access-authn-authz/rbac/), 
   the [node authorizer](https://kubernetes.io/docs/reference/access-authn-authz/node/), 
   and a scope limiting module specific to the OpenShift OAuth tokens.
   See the [Kubernetes Authorization documentation](https://kubernetes.io/docs/reference/access-authn-authz/authorization/)
   for full details.
    1. The node authorizer limits kubelet actions to the minimal set required to run pods scheduled to their nodes
       (more on this later).
       To do that, it requires being able to identify specific which individual kubelet is making an API request.

### Overall Authentication Flow

Kubelet credential flows are not obvious, so we will document them here.  At a high level the flow is
1. MachineConfig provides a bootstrap credential on the host for the kubelet to used to create a CertificateSigningRequest
   on the kube-apiserver.
2. machine-approver (or a cluster-admin) checks to see if the request CertificateSigningRequest is valid and if so approves.
3. kube-controller-manager signs the approved CertificateSigningRequest.
4. kubelet uses the client cert/key pair to authenticate to the kube-apiserver for its API requests. 

#### Kubelet Bootstrap Credentials

There are two different kinds of bootstrap credentials: 1) a master node client certificate that are created before service account
tokens are available, and 2) worker (and all other) node credentials that are a service account token.

Master node bootstrap credentials are created and used as follows.
1. The installer produces a single-purpose signer called `kubelet-bootstrap-kubeconfig-signer`.
2. A ca-bundle is created to identify certificates signed by this signer.
   The CA bundle is called `kubelet-bootstrap-kubeconfig-ca-bundle`.
3. The installer places the `kubelet-bootstrap-kubeconfig-ca-bundle` into `ns/openshift-config`
   `configmap/kubelet-bootstrap-kubeconfig`.
4. The installer signs the master kubelet bootstrap credentials for `system:serviceaccount:openshift-machine-config-operator:node-bootstrapper`
   with group `system:serviceaccounts:openshift-machine-config-operator`.
   Notice that this matches the service account that will eventually be used.
   The master kubelet bootstrap credentials are signed for 10 years of validity.
5. In the `cluster-kube-apiserver-operator`, combined the content from `configmap/kubelet-bootstrap-kubeconfig` with other
   valid client certificate CA bundles to identify clients.
6. The master MachineConfig includes the master kubelet bootstrap credentials.

To invalidate the master node bootstrap credential created by the installer
1. Delete `configmap/kubelet-bootstrap-kubeconfig` in `ns/openshift-config`.
2. The `cluster-kube-apiserver-operator` removes the CA bundle from the trusted client certificate CA bundles.
3. The master node bootstrap credential created by the installer is no longer valid.
4. In the MCO, if the `configmap/kubelet-bootstrap-kubeconfig` is deleted, use the "normal" service account token
   logic that provides bootstrap authentication credentials for workers.
5. The MCO rollouts the new machine configs.

Worker node credentials are easier and look like this.
1. The MCO retrieves a serviceaccount token for `serviceaccount/node-bootstrapper` in `ns/openshift-machine-config-operator`.
2. The MCO includes this node bootstrap credential (the service account token) in MachineConfigs.
   This ensures the content is distributed to each node during ignition.

To invalidate the worker node credentials
1. Delete the service account token: `oc -n openshift-machine-config-operator delete secret/node-bootstrapper-token`.
2. The machine-config-operator will create a new, empty `node-bootstrapper-token` and the kube-controller-manager will
   fill in a token value.
3. The machine-config-operator will read the new token value and update the MachineConfig to distribute it to the nodes.

### How Kubelets Create Credentials

1. If a current kubelet client certificate exists and is valid, use it and skip the rest.
2. Use the bootstrap kubelet credentials to create a client CertificateSigningRequest (CSR) on the kube-apiserver.
   The username is in the form `system:node:<nodeName>` and groups contain `system:nodes`
3. machine-approver (or cluster-admin) approves the CSR created by the kubelet's bootstrap credentials.
4. Kubelet uses bootstrap kubelet credentials to retrieve the signed certificate from the client CSR.
5. Given the signed client certificate and the local key (which never left the host), the kubelet communicate with the kube-apiserver

### How machine-approver Decides to Approve Client Certificates

1. If the CSR isn't a well-formed kubelet client certificate, don't approve.
2. If the CSR is for a renewal, don't approve.
3. If the CSR is not created by the correct user (`system:serviceaccount:openshift-machine-config-operator:node-bootstrapper`), don't approve.
4. If the node resource already exists, don't approve.
   In such a case, it should be a renewal, not a new request: see [code](https://github.com/openshift/cluster-machine-approver/blob/dfdf6e570465e6182bf7377067c7511e61c9dc81/csr_check.go#L261)
5. If machine doesn't exist for node, don't approve.
6. If the request doesn't closely conform to expected time for the machine creation, don't approve.
7. If the CSR is created by the correct user, for a node that isn't yet registered, for a recently created machine, then approve.

### How Kubelets Are Authorized

Kubelets are individually authorized based on which node are running on.
Since the kubelet client certificates have a CommonName (which maps to kubernetes username) of `system:node:<nodeName>`,
it is possible to identify which node a kubelet is running on.

Given a particular node, the node authorizer restricts API access to only those API resources that are required to run
pods scheduled to that node.
This list isn't exhaustive, but it gives an idea of what the node authorizer does.
1. kubelets can update pods/status for pods that are running on it, but not for any other pods.
2. kubelets can read configmaps that are mounted by pods running on it, but not any other configmaps.
3. kubelets can read persistentvolumes, but only for persistentvolumeclaims that are referenced by pods running on it.

If a kubelet requests or attempts to modify content that is not strictly required to run the pods scheduled to the node,
then the API requests are denied.

### Test Plan

### Upgrade / Downgrade Strategy

### Version Skew Strategy

How will the component handle version skew with other components?
What are the guarantees? Make sure this is in the test plan.

Consider the following in developing a version skew strategy for this
enhancement:
- During an upgrade, we will always have skew among components, how will this impact your work?
- Does this enhancement involve coordinating behavior in the control plane and
  in the kubelet? How does an n-2 kubelet without this feature available behave
  when this feature is used?
- Will any other components on the node change? For example, changes to CSI, CRI
  or CNI may require updating that component before the kubelet.

## Drawbacks


## Alternatives

1. Use a service-account token from the beginning.
   This is impractical because service account tokens are signed JWTs that contain a UID claim.
   The UID of the service account is not known until the service account is actually created.
   This means that a machine config could not be produced until the service account is created.
   This would mean re-writing the way that bootstrapping happens to allow for this ordering dependency.
2. Replace the machine config that has a bootstrap credential with a service account token after installation.
   One way would be to create a configmap that is deleted four hours after it was created.
   If the configmap is removed, then the machineconfig for masters is updated.
   This leaves us with a four hour gap or a manual step to delete the configmap and wait for a rollout of all masters.

