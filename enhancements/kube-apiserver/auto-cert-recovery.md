---
title: automatic-cert-recovery-for-kube-apiserver-kube-controller-manager
authors:
  - "@deads2k"
reviewers:
  - "@tnozicka"
  - "@sttts"
approvers:
  - "@sttts"
creation-date: 2019-08-22
last-updated: 2020-05-29
status: implemented
see-also:
replaces:
superseded-by:
---

# Automatic Cert Recovery for kube-apiserver and kube-controller-manager

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [x] Design details are appropriately documented from clear requirements
- [x] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

Fully automate the recovery of kube-apiserver and kube-controller-manager certificates currently documented [here](https://docs.openshift.com/container-platform/4.1/disaster_recovery/scenario-3-expired-certs.html).
Currently, there are helper commands to make the effort more practical, but we think we can fully automate the process
to avoid human error and intervention.


## Motivation

If the kube-apiserver and kube-controller-manager operators are offline for an extended period of time, the cluster
cannot automatically restart itself because the certificates are invalid.  This comes up in training clusters where
clusters are suspended frequently.  It also comes up for products like code-ready-containers which creates and suspends
VMs for later restart.  It is theoretically possible to automate the recovery steps, but they are slow and error prone.

### Goals

1. Provide a zero touch, always-on certificate recovery for the kube-apiserver and kube-controller-manager

### Non-Goals

1. Provide automation for any other part of disaster recovery.
2. Provide mechanisms to keep certificates up to date for any other component (kubelet for instance).
3. Provide mechanisms to approve CSRs.  That is still the domain of the cloud team.

## Proposal

We will take our existing `cluster-kube-apiserver-operator regenerated-certificates` command and create a simple
controller which will watch for expired certificates and regenerate them.  It will connect to the kube-apiserver using
localhost with an SNI name option wired to a 10 year cert.  When there is no work to do, this controller will do nothing.
This controller will run as another container in our existing static pods.
The recovery flow will look like this:

1. kas-static-pod/kube-apiserver starts with expired certificates
2. kas-static-pod/cert-syncer connects to localhost kube-apiserver with using a long-lived SNI cert (localhost-recovery).  It sees expired certs.
3. kas-static-pod/cert-regeneration-controller connects to localhost kube-apiserver with a long-lived SNI cert (localhost-recovery).
 It sees expired certs and refreshes them as appropriate.  Being in the same  repo, it uses the same logic.
 We will add an overall option to the library-go cert rotation to say, "only refresh on expired"
 so that it never collides with the operator during normal operation.  The library-go cert rotation impl is resilient to
 multiple actors already.
4. kas-static-pod/cert-syncer sees updated certs and places them for reload. (this already works)
5. kas-static-pod/kube-apiserver starts serving with new certs. (this already works)
6. kcm-static-pod/kube-controller-manager starts with expired certificates
7. kcm-static-pod/cert-syncer connects to localhost kube-apiserver with using a long-lived SNI cert (localhost-recovery).  It sees expired certs.
8. kcm-static-pod/recovery-controller connects to localhost kube-apiserver with a long-lived SNI cert (localhost-recovery).  It sees expired certs and refreshes them as appropriate.  Being in the same
 repo, it uses the same logic.  We will add an overall option to the library-go cert rotation to say, "only refresh on expired"
 so that it never collides with the operator during normal operation.  The library-go cert rotation impl is resilient to
 multiple actors already.
9. kcm-static-pod/cert-syncer sees updated certs and places them for reload. (this already works)
10. kcm-static-pod/kube-controller-manager live reloads new client certs and CA from the kubeconfig.
11. kcm-static-pod/kube-controller-manager wires up a library-go/pkg/controller/fileobserver to the CSR signer and suicides on the update

12. At this point, kas and kcm are both up and running with valid serving certs and valid CSR signers.
13. kcm creates pods for operators and workloads including sdn-o and sdn
14. Kubelets will start creating CSRs for signers, but the machine approver is down.
 **A cluster-admin must manually approve client CSRs for the master kubelets**
15. Master kubelets are able to communicate to the kas and get listings of pods. Network plugins are not ready and pods are not scheduled, so kubelet can't run them yet.
16. ks-static-pod/cert-syncer connects to localhost kube-apiserver with using a long-lived SNI cert (localhost-recovery). It syncs new client certs for kube-scheduler and places them for reload.
17. ks-static-pod/kube-scheduler live reloads new client certs and CA from the kubeconfig.
18. ks-static-pod/kube-scheduler schedules sdn-o and sdn pods
19. Master kubelets run the sdn, sdn-o, network plugins become ready and kubelet can run regular pods
20. Master kubelets run the kas-o, kcm-o, ks-o and the system re-bootstraps.


### Implementation Details/Notes/Constraints

This requires these significant pieces

- [x] kcm fileobserver
- [x] kcm-o to rewire configuration to auto-refresh CSR signer
- [x] kcm-o to provide a cert regeneration controller
- [x] kas-o to provide a cert regeneration controller
- [x] kas-o to create and wire a long-lived serving cert/key pair for localhost-recovery
- [x] kas-o, kcm-o, ks-o to create and wire client tokens for localhost-recovery
- [x] rewire kas-o, kcm-o, ks-o cert-syncers to use client tokens and connect to localhost-recovery
- [x] automatic kubeconfig cert reloading for kcm, ks
- [x] library-go cert rotation library to support an override for only rotating when certs are expired
- [ ] remove old manual recovery commands

### Risks and Mitigations

1. If we wire the communication unsafely we can get a CVE.
2. If we don't delay past "normal" rotation, the kas-o logs will be hard to interpret.
3. If something goes wrong, manual recovery may be harder.

## Design Details

### Test Plan

Disaster recovery tests are still outstanding with an epic that may not be approved.  Lack of testing here doesn't introduce
additional risk beyond that already accepted.

This will be tested as part of normal disaster recovery tests.  It's built on already unit tested libraries and affects
destination files already checked with unit and e2e tests.

### Graduation Criteria

This will start as GA.

### Upgrade / Downgrade Strategy

Being attached to the existing static pod, upgrades and downgrades will produce matching containers, so our producer and
consumers are guaranteed to match.

### Version Skew Strategy

Because each deployment in the payload is atomic, it will not skew.  There are no external changes.

## Implementation History

Major milestones in the life cycle of a proposal should be tracked in `Implementation
History`.

- 2020-05-29 [tnozicka] Updated the proposal to reflect implementation shipped in OCP 4.5

## Alternatives

This process can be run by a laborious and error prone manual process that three existent teams have already had trouble with.
