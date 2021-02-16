---
title: code-ready-containers
authors:
reviewers:
approvers:
creation-date: 2020-03-02
last-updated: 2020-03-02
status: implemented
see-also:
  - "/enhancements/this-other-neat-thing.md"
replaces:
superseded-by:
---

# Code Ready Containers (CRC)

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Open Questions

This is where to call out areas of the design that require closure before deciding
to implement the design.  For instance,
 > 1. This requires exposing previously private resources which contain sensitive
  information.  Can we do this?

## Summary

CRC functions by doing roughly....
 1. Running a libvirt install, which is for ["development only"](https://github.com/openshift/installer/blob/ffc34e32fe4a71560f59312384daa87b401d6ec9/README.md) meaning OpenShift development only.
 2. On a single node cluster, which is below the minimum of [three that are required](https://github.com/openshift/installer/blob/c904277e59dd947a8884265b2511034b05c38644/upi/openstack/inventory.yaml#L35) for a functional cluster.
 3. Disable the CVO, which prevents all reconciliation of operators, top level status, and upgrades.
 4. Deletes the monitoring stack, but not all monitor resources.
     Removes the aggregated apiserver as well.
 5. Scale insights to zero.
 6. Force the router down to a single node.  Preventing smooth upgrades.
 7. Twiddle the registry operator.
    Changes for routes and PVs.
 8. Disable the cloud credential operator.  Preventing reporting of failures to create and maintain creds for other operators.
 10. All other operators and operands are expected to function properly on a single node libvirt installation.

Every one of these step must be considered supported when run by CRC in order to consider CRC a product.
Every single one of these steps is, at the time of merging this enhancement, considered unsupported by the teams that own the components.

## Motivation

This is an attempt to describe the current state of CRC, not debate the motivations that lead to it.

### Goals

### Non-Goals

## Proposal

### Implementation Details/Notes/Constraints

### Risks and Mitigations

## Design Details

### Test Plan

#### Removing a deprecated feature

### Upgrade / Downgrade Strategy

Upgrades and downgrades are not supported.

### Version Skew Strategy

No known version skew is present.

## Drawbacks

This adds a novel platform type and release topology to the product without consulting any of the teams that purportedly make it work.

## Alternatives

1. Build from kube-control-plane out.
   This approach makes the openshift org work for you:
   OLM has a mission to support bare kube,
   operators installed via OLM have a mission to run on kube,
   openshift as a whole had a mission to install everything via OLM (back in 4.0 and for our future),
   openshift already has operators available which can work via OLM.
   1. Take an established upstream project that starts a kube control plane.  Even kube-up could do this.
   2. Shim in the openshift built binaries for kube-control-plane (our kube-apiserver, kube-controller-manager, etcd)
   3. Install OLM
   4. Create variants of everything else you want to install that come in via OLM.
