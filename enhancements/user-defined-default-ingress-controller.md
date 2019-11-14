---
title: user-defined-default-ingress-controller
authors:
  - "@ironcladlou"
reviewers:
  - "@ironcladlou"
  - "@Miciah"
  - "@danehans"
approvers:
  - "@ironcladlou"
  - "@Miciah"
  - "@danehans"
creation-date: 2019-09-08
last-updated: 2019-09-09
status: implementable
see-also:
replaces:
superseded-by:
---

# User-defined Default Ingress Controller

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

This proposal is to enable users to define the default
IngressController<sup>[1](https://docs.openshift.com/container-platform/4.1/networking/configuring-ingress-cluster-traffic/configuring-ingress-cluster-traffic-ingress-controller.html#configuring-ingress-cluster-traffic-ingress-controller),[2](https://github.com/openshift/api/blob/master/operator/v1/types_ingress.go)</sup>
during OpenShift installation.

Changes to the OpenShift [Installer](https://github.com/openshift/installer) and
[Ingress Operator](https://github.com/openshift/cluster-ingress-operator) are
proposed which will provide the basis for a supportable customization procedure.
The procedure is proposed for incorporation into the OpenShift
[documentation](https://github.com/openshift/openshift-docs).

## Motivation

OpenShift's default IngressController is automatically created based on the
cluster platform and cluster-global configuration. The default is reliable for
installation, but typically requires reconfiguration soon after installation.

However, there are some updates to an existing IngressController which are
disruptive or destructive and which users would prefer to configure during
installation.

Because of the broad range of ingress needs, it's difficult to anticipate what
subset of IngressController API fields should be exposed for configuration at
the time of installation  Allowing the user to replace the default
IngressController in its entirety provides maximum expressiveness and
flexibility without introducing new configuration API surface area.

### Goals

* Create a procedure which safely allows users to provide a specific default
  IngressController resource to the installer.

### Non-Goals

* No changes to the `config.openshift.io` or `operator.openshift.io` APIs are
  proposed.

## Proposal

The following specific changes are proposed.

1. Add a `render` command to the Ingress Operator binary which emits the
   following manifests:
    * The IngressController [Custom Resource Definition](https://github.com/openshift/cluster-ingress-operator/blob/master/manifests/00-custom-resource-definition.yaml).
    * The `openshift-ingress-operator`
      [Namespace](https://github.com/openshift/cluster-ingress-operator/blob/master/manifests/00-namespace.yaml).
2. Update the [Installer manifest rendering](https://github.com/openshift/installer/blob/master/data/data/bootstrap/files/usr/local/bin/bootkube.sh.template)
   system to execute the Ingress Operator `render` command and move the output
   into the correct location, causing the installer to apply the manifests
   before the Ingress Operator's lifecycle begins.
3. Add a installation procedure to the
   [documentation](https://github.com/openshift/openshift-docs) describing how
   to provide a default IngressController and reasons why one might want to do
   so.

### User Stories

Generally speaking, the proposed capability generically improves support for
many post-installation IngressController use cases. This section describes two
possible use cases.

#### Create an internal (non-public) cluster

>As an OpenShift administrator, I want to create a new cluster which is never
>exposed to the public internet.

To accomplish this goal today on cloud platforms, users must (in addition to
other non-ingress tasks):

1. Install a cluster which unconditionally exposes ingress with a public load
   balancer.
2. Replace the default IngressController with an internal variant.

This replacement procedure violates the constraints of the goal (as a public
load balancer exists for some time). The procedure is also disruptive:

1. During the replacement operation, default ingress is completely uninstalled
   and then reinstalled, resulting in complete (but temporary) ingress
   disruption.
    * The old cloud load balancer is destroyed and a new load balancer is
      provisioned.
    * DNS records for the old cloud load balancer are destroyed and then
      re-created for the new load balancer.

If this proposal is implemented, during installation the user can provide the
following IngressController manifest which satisfies the constraint as well as
eliminating disruption and unnecessary (and possibly expensive) cloud resource
provisioning.

```yaml
apiVersion: operator.openshift.io/v1
kind: IngressController
metadata:
  namespace: openshift-ingress-operator
  name: default
spec:
  endpointPublishingStrategy:
    type: LoadBalancerService
    loadBalancer:
      scope: Internal
```

#### Pre-configuring shards

>As an OpenShift administrator, I want to the default IngressController to only
>admits OpenShift's default Routes

To accomplish this goal today, users must:

1. Install a cluster which by default produces an IngressController which will
   admit any Route resource in any namespace.
2. Update the default IngressController to specify Route selector criteria which
   exclude user defined Routes (where user defined Route is a Route labeled to
   indicate the Route is user-defined).

The update procedure has at least two notable drawbacks:

1. Changing the IngressController will result in a rollout (which today can be
   disruptive).
2. The IngressController may already have admitted some Routes which would have
   been excluded had the IngressController been created with the desired
   selection criteria.

If this proposal is implemented, during installation the user can provide an
IngressController manifest which has the desired Route selection criteria,
eliminating both drawbacks.

### Implementation Details

The IngressController resource is a cluster-scoped Custom Resource Definition.
For the cluster to support creation of the default IngressController resource,
the following must be true:

1. The IngressController Custom Resource Definition must be persisted and
   admitted into the cluster.
2. The `openshift-ingress-operator` namespace must exist to contain the
   `default` IngressController resource.

The Custom Resource Definition and Namespace manifests are part of the Ingress
Operator image, and are normally created and managed by the [Cluster Version Operator](https://github.com/openshift/cluster-version-operator). Creation order of these resources
is significant, and it is unsafe for users to reorder or modify them.

To ensure a safe and supportable path for creating these resources at
installation time and outside the context of the CVO, the proposed Ingress
Operator `render` command emits these manifests with the correct order and
content, and users are warned that modifying them results in an unsupportable
cluster.

The user provides _only_ the IngressController resource.

Constraints:

* **After installation has completed, OpenShift has no memory of the
  IngressController manifest provided during installation**. If a user provides
  a default IngressController at installation time, deletes the default
  IngressController after installation, and doesn't replace the default the
  Ingress Operator will create a new default IngressController as already
  documented.

### Risks and Mitigations

* Risk: User confusion over advanced installation procedures.
  * Mitigation: Ensure high quality documentation exists.
* Risk: User has the opportunity to modify resources owned by the platform
  outside the CVO  lifecycle.
  * Mitigation: Ensure the Ingress Operator recognizes and reconciles over any
    user modifications to operator-owned resources.
* Risk: Documentation falls out of date as API evolves.
  * Mitigation: Minimize the number and detail of things the user must provide
    at installation (e.g. relying on defaults where possible in a manifest
    definition).
* Risk: Easy for users to provide a default IngressController which breaks installation.
  * Mitigation: Ensure high quality documentation exists.

## Design Details

### Test Plan

TODO

### Graduation Criteria

TODO

### Upgrade / Downgrade Strategy

This proposal is for install-time capabilities which have no impact on upgrades
or downgrades.

### Version Skew Strategy

This proposal doesn't introduce any operator API behavioral changes which could
be affected by version skew.

## Implementation History

TODO

## Drawbacks

The [Implementation Details/Notes/Constraints](#implementation-details) section
contains detailed coverage of constraints and risks which in this case can be
reasonably framed as drawbacks.

Some broad reasons this proposal might _not_ be acceptable are:

* The end-user installation complexity introduced does not offset the utility.
* The general mechanism for implementation (extracting CVO manifests and
  preempting the CVO) seems underspecified and too risky to form a stable
  foundation for additional such procedures.

## Alternatives

* Introduce additional `config.openshift.io` API surface generic enough to
  improve defaulting in such a way that a custom IngressController is
  unnecessary. For example, a [generic "internal" cluster status](https://github.com/openshift/enhancements/pull/25)
  could inform the Ingress Operator to use an internal load balancer by default
  for new IngressControllers, solving at least one use case.
  * Drawback: Requires new configuration API and the attendant enhancement
    proposal process.
  * Drawback: Solves use cases somewhat piecemeal; may require a new proposal
    discussion for each use case to determine how defaulting can help.

  As defaulting evolves, defaults may progressively minimize but not eliminate
  the need for the advanced procedure described in this proposal.

* Expose IngressController fields through the `config.openshift.io` Ingress API.
  For example, adding a load balancer scope field.
  * Advantage: Eliminates the need for Ingress Operator manifest extraction.
  * Advantage: Creates opportunities for installer UX integration (e.g. the wizard).
  * Drawback: Scope is overly broad — these defaults apply to _all_ new
    IngressControllers, not just the default.
  * Drawback: Taken to a logical extreme, IngressControllerSpec becomes embedded
    in `config.openshift.io`.

  Several specific variants of API additions have been considered and not
  documented here; this is an overview of the properties common to the general
  approach.

## Infrastructure Needed [optional]

TODO
