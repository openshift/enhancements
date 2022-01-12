---
title: csi-inline-vol-security
authors:
  - "@adambkaplan"
reviewers:
  - "@deads2k"
  - "@stlatz"
approvers:
  - "@jsafrane"
api-approvers:
  - "@deads2k"
  - "@sttts"
creation-date: 2022-01-10
last-updated: 2022-01-10
tracking-link: # link to the tracking ticket (for example: Jira Feature or Epic ticket) that corresponds to this enhancement
  - https://issues.redhat.com/browse/STOR-746
see-also: 
  - "/enhancements/subscription-content/subscription-content-access.md"
  - "/enhancements/cluster-scope-secret-volumes/csi-driver-host-injections.md"
  - "/enhancements/authentication/pod-security-admission.md"
replaces: []
superseded-by: []
---

# CSI Inline Ephemeral Volume Security

## Summary

Provide a mechanism where the use of an individual CSI driver capable of
provisioning CSI ephemeral volumes (also known as inline ephemeral volumes or
inline CSI volumes) can be restricted on pod admission.

## Motivation

The original set of Pod Security Standards permits the use of CSI ephemeral
volumes by the restricted profile - i.e. any workload. The justification was
listed as follows:

> - Inline CSI volumes should only be used for ephemeral volumes.
> - The `CSIDriver` object spec controls whether a driver can be used inline, and
>   can be modified without binary changes to disable inline usage.
> - Risky inline drivers should already use a 3rd party admission controller,
>   since they are usable by the baseline policy.
> - We should thoroughly document safe usage, both on the documentation for this
>   (pod security admission) feature, as well as in the CSI driver documentation.

This position provides the maximum amount of flexibility for workloads (any
workload can use CSI ephemeral volumes), however it forces administrators to
take an all or nothing approach with respect to CSI drivers that provide this
feature. As more CSI drivers implement this capability, admins may consider
some drivers safe for the `restricted` profile, but consider others only safe
for `baseline` or `privileged` workloads.

### Goals

- Allow cluster administrators to audit, warn, or block the use of a particular
  CSI driver as the provider of a CSI ephemeral volume.
- Allow CSI driver maintainers to recommend that their driver is suitable for a
  given security profile.
- Allow cluster administrators to default the audit, warning, or blocking of
  CSI drivers when used to provide an inline CSI volume.

### Non-Goals

- Let cluster administrators audit, warn, or block the use of a CSI driver as
  the provider for a persistent volume claim.
- Let cluster administrators audit, warn, or block the use of a CSI driver as
  the provider for a [generic ephemeral volume claim](https://kubernetes.io/docs/concepts/storage/ephemeral-volumes/#csi-ephemeral-volumes).
- Audit, warn, or block specific use patterns for a particular CSI driver - for
  example, prevent a specific volume attribute from being set.

## Proposal

Admins or distributions can optionally add the
`security.openshift.io/csi-ephemeral-volume-profile` label to a `CSIDriver`
object. This will declare the driver’s effective pod security profile when it
is used to provide CSI ephemeral volumes. This “effective profile” communicates
that a pod can use the CSI driver to mount CSI ephemeral volumes when the pod’s
namespace is governed by a pod security standard:

```yaml
kind: CSIDriver
metadata:
  name: csi.mydriver.company.org
  labels:
    security.openshift.io/csi-ephemeral-volume-profile: restricted
```

A new validating admission plugin - `CSIVolumeAdmission` - will inspect pod
volumes on pod creation. If a pod uses a `csi` volume, the plugin will look up
the CSIDriver object and inspect the `csi-ephemeral-volume-profile` label. This
`CSIVolumeAdmission` plugin will then use the label’s value in its enforcement,
warning, and audit decisions.

Because pod volumes are considered immutable, this admission plugin will only
run its checks on pod creation. Existing pods that use `csi` volumes will not
be affected once the admission plugin is enabled.

### User Stories

As a Kubernetes cluster administrator, I want to indicate that a CSI driver
should not be used by the restricted profile so that a particular CSI driver is
not used to mount CSI ephemeral volumes in restricted namespaces.

As a Kubernetes cluster administrator, I want to prevent pods from using CSI
ephemeral volumes unless I have deemed the underlying CSI driver “safe.”

As a maintainer who distributes CSI drivers that support CSI ephemeral volumes,
I want to indicate that my driver is not suitable for restricted/
security-conscious use cases.

### Pod Security Profile Enforcement

When a `CSIDriver` has the `csi-ephemeral-volume-profile` label, pods using the
CSI driver to mount CSI ephemeral volumes must run in a namespace that enforces
a pod security standard of equal or greater permission. If the namespace
enforces a more restrictive standard, the `CSIVolumeAdmission` admission plugin
should deny admission.

The following table demonstrates the pod admission behavior for a pod that
mounts a CSI ephemeral volume with the given pod security enforcement profile
and CSI ephemeral volume profile:

| PodSecurity Profile | driver label: restricted | driver label: baseline | driver label: privileged |
| ------------------- | ----------------- | --------------- | ----------------- |
| restricted | Allowed | Denied  | Denied  |
| baseline   | Allowed | Allowed | Denied  |
| privileged | Allowed | Allowed | Allowed |

### Pod Security Profile Warning

The `CSIVolumeAdmission` admission plugin can also provide user-facing warnings
(via appropriate HTTP response headers) if the CSI driver’s effective profile
is more permissive than the pod security warning profile for the pod namespace.

The following table demonstrates the pod admission behavior with the given pod
security warning profile and CSI driver effective profile:

| PodSecurity Profile | driver label: restricted | driver label: baseline | driver label: privileged |
| ------------------- | ----------------- | --------------- | ----------------- |
| restricted | No Warning | Warning    | Warning  |
| baseline   | No Warning | No Warning | Warning  |
| privileged | No Warning | No Warning | No Warning |

### Pod Security Profile Audit

The `CSIVolumeAdmission` admission plugin can also apply audit annotations to
the pod if the CSI driver’s effective profile is more permissive than the pod
security audit profile for the pod namespace.

The following table demonstrates the pod admission behavior with the given pod
security audit profile and CSI driver effective profile:

| PodSecurity Profile | driver label: restricted | driver label: baseline | driver label: privileged |
| ------------------- | ----------------- | --------------- | ----------------- |
| restricted | No Audit | Audit    | Audit  |
| baseline   | No Audit | No Audit | Audit  |
| privileged | No Audit | No Audit | No Audit |

### Default Behavior

If the referenced CSIDriver for a CSI ephemeral volume does not have the
`csi-ephemeral-volume-profile` label, the `CSIVolumeAdmission` admission plugin
will consider the driver to have the `restricted` profile for enforcement,
warning, and audit behaviors. Likewise, if the pod’s namespace does not have
the pod security admission label set, the admission plugin will assume the
`privileged` profile is allowed for enforcement, warning, and audit decisions.
This will effectively run in a "no-op" mode

Like other admission plugins, the plugin will have its own configuration API
which allows this behavior to be tuned. The following example sets the
`privileged` profile on CSIDriver objects by default, and the `restricted`
profile as the default for namespaces:

```yaml
defaults:
  …
  csi-ephemeral-volume-profile: privileged
  pod-security-enforce-profile: restricted
  pod-security-warn-profile: restricted
  pod-security-audit-profile: restricted
```

### API Extensions

This adds a new validating admission plugin which can prevent pods from being
created - `CSIVolumeAdmission`. This admission plugin should only be invoked
when a pod is being created. Pod volumes are considered immutable, therefore
this admission plugin should not prevent pods from being updated under any
circumstance.

This will be implemented as and admission plugin rather than a webhook to
reduce network round-trips and improve overall stability/reliability.
To do this, we will add a carry-patch to `openshift/kubernetes` containing
the admission plugin logic.

This admission plugin will impact any workload that can add a `csi` volume to
a Pod. No OpenShift component is known to directly consume `csi` volumes today.
Any pod-specable workload (`Deployment`, `DeploymentConfig`, etc.) and
OpenShift Builds can consume `csi` volumes with approriate user specifications.

OpenShift components that add `CSIDriver` objects that provide the `Ephemeral`
volume capability should add the appropriate security profile label. As of this
writing, only the [Shared Resource CSI Driver](https://github.com/openshift/csi-driver-shared-resource)
implements this capability in OpenShift. Other third party CSI drivers may
implement this capability, notably the [Secret Store CSI Driver](https://secrets-store-csi-driver.sigs.k8s.io/).
These drivers do not need to add this label to be used, though over time we may
restrict use of volumes from these drivers to privileged namespaces.

**TODO - more here? Is this sufficient?**

> API Extensions are CRDs, admission and conversion webhooks, aggregated API servers,
> finalizers, i.e. those mechanisms that change the OCP API surface and behaviour.
>
> - Name the API extensions this enhancement adds or modifies.
> - Does this enhancement modify the behaviour of existing resources, especially those owned
>  by other parties than the authoring team (including upstream resources), and, if yes, how?
>   Please add those other parties as reviewers to the enhancement.
>
>  Examples:
>  - Adds a finalizer to namespaces. Namespace cannot be deleted without our controller running.
>  - Restricts the label format for objects to X.
>  - Defaults field Y on object kind Z.
>
> Fill in the operational impact of these API Extensions in the "Operational Aspects
> of API Extensions" section.

### Implementation Details/Notes/Constraints [optional]

This admission plugin is being actively discussed upstream in a draft KEP.
There is a chance that this admission plugin will be implemented upstream -
either concurrently with OpenShift or in a subsequent Kubernetes release.

**TODO - other bits here?**

> What are the caveats to the implementation? What are some important details that
> didn't come across above. Go in to as much detail as necessary here. This might
> be a good place to talk about core concepts and how they relate.

### Risks and Mitigations

Adding a new validating admission plugin inherently risks the cluster’s
availability and responsiveness. At its most extreme, this proposal will
require the admission plugin to fetch a `CSIDriver` object on every pod
admission request, adding additional strain on the kubernetes apiserver. A
lister for `CSIDriver` objects can mitigate this, and can be particularly
effective because most clusters do not have a large number of `CSIDriver`
objects, and these objects do not change frequently.

**TODO - review for the following**

- Security evaluation of CSI Drivers
- User experience review
- Impact on existing CSI drivers provided by OpenShift

## Design Details

### Open Questions [optional]

1. How/can we define SLIs for an admission plugin? Do we rely on perforamnce
   benchmarks for pod creation? Can we construct an SLO/alerts around admission
   plugins?
2. What happens if Kubernetes adopts this plugin at a future date?

### Test Plan

**Note:** *Section not required until targeted at a release.*

- Unit testing in tree, applied in a carry patch for maintainability.
- Run e2e suite of the Shared Resource CSI driver with the plugin enabled.
- End to end testing in openshift/origin, tested against the following:
  - Build pods that utilize the Shared Resource CSI driver
  - Workloads that utilize a `privileged` CSI driver in a `restricted` and
    `baseline` namespace (assumes the PodSecurity plugin is enabled).

### Graduation Criteria

**TODO** - tech preview for 4.11?

| OpenShift | Maturity |
| --------- | -------- |
| 4.11 | Tech Preview |
| 4.12 | GA |

**Note:** *Section not required until targeted at a release.*

Define graduation milestones.

These may be defined in terms of API maturity, or as something else. Initial proposal
should keep this high-level with a focus on what signals will be looked at to
determine graduation.

Consider the following in developing the graduation criteria for this
enhancement:

- Maturity levels
  - [`alpha`, `beta`, `stable` in upstream Kubernetes][maturity-levels]
  - `Dev Preview`, `Tech Preview`, `GA` in OpenShift
- [Deprecation policy][deprecation-policy]

Clearly define what graduation means by either linking to the [API doc definition](https://kubernetes.io/docs/concepts/overview/kubernetes-api/#api-versioning),
or by redefining what graduation means.

In general, we try to use the same stages (alpha, beta, GA), regardless how the functionality is accessed.

[maturity-levels]: https://git.k8s.io/community/contributors/devel/sig-architecture/api_changes.md#alpha-beta-and-stable-versions
[deprecation-policy]: https://kubernetes.io/docs/reference/using-api/deprecation-policy/

**If this is a user facing change requiring new or updated documentation in [openshift-docs](https://github.com/openshift/openshift-docs/),
please be sure to include in the graduation criteria.**

**Examples**: These are generalized examples to consider, in addition
to the aforementioned [maturity levels][maturity-levels].

#### Dev Preview -> Tech Preview

- Admission plugin added to our fork of kube-apiserver
- SLIs - do we have these for admission plugins? Alerts?
- Test coverage in openshift/origin for pods with CSI volumes
- Documentation for the admission plugin - use and configuration
- Verify use with the Shared Resource CSI Driver

#### Tech Preview -> GA

- Seamless upgrade experience for the plugin, enabled by default.
- Pod creation scale testing within acceptable limits.
- End to end testing with pods using volumes from the Shared Resource CSI driver
- Sufficient time for feedback
- Backhaul SLI telemetry - **TODO - what will these be?**
- Document SLOs for the component - **TODO - how/what will we alert on?**
- User facing documentation created in [openshift-docs](https://github.com/openshift/openshift-docs/)

**For non-optional features moving to GA, the graduation criteria must include
end to end tests.**

#### Removing a deprecated feature

N/A

### Upgrade / Downgrade Strategy

The new admission plugin should only impact a narrow subset of pods which use
`csi` volumes, with "no-op" defaults for `CSIDriver` objects on the cluster.
Since this plugin will not be available in previous versions, workloads should
be able to be scheduled.

### Version Skew Strategy

**TODO: is this not applicable?**

N/A

### Operational Aspects of API Extensions

**TODO**

This will be added to kube-apiserver, so we should have metrics/altering on kube-apiserver health

- Impact on pod creation - how much will this add to pod creation without `csi` volume?
- Impact on pod creation with `csi` volume?

#### Failure Modes

**TODO**

Could this fail as a kube-apiserver crashloop?

- Describe the possible failure modes of the API extensions.
- Describe how a failure or behaviour of the extension will impact the overall cluster health
  (e.g. which kube-controller-manager functionality will stop working), especially regarding
  stability, availability, performance and security.
- Describe which OCP teams are likely to be called upon in case of escalation with one of the failure modes
  and add them as reviewers to this enhancement.

#### Support Procedures

TODO

## Implementation History

- 2022-01-12: Initial draft

## Drawbacks

`CSIDrivers` that provide the `Ephemeral` volume capability should be able to
provision these volumes in a secure manner. There is some concern in upstream
Kubernetes that this plugin would give license for CSIDriver implementers to
allow risky behaviors, increasing the attack surface of Kubernetes as a whole.

This proposal adds a new admission plugin whose behavior and functionality is
related to the existing PodSecurityAdmission plugin. The challenge of this
approach is the handling of defaults when the CSI driver does not have an
effective profile label, the namespace does not have a pod security enforcement
label, or both. The admission plugin would need its own configuration for
default behavior, and skew between its behavior and the PodSecurityAdmission
admission plugin could lead to undesirable outcomes. For example,
PodSecurityAdmission could be configured to enforce the “restricted” profile by
default in all namespaces, whereas this new admission plugin could be
configured to assume the `privileged` namespace profile is enforced by default.
This could cause the admission plugin to allow admission of a pod that uses a
`privileged` CSI driver in a namespace that has the “restricted” profile
enforced.

## Alternatives

### Augment the PodSecurityAdmission Plugin

Rather than creating a new admission plugin, this capability could be added to
the extending the PodSecurityAdmission plugin, this admission logic could be
implemented in a separate validating admission admission plugin. A similar
labeling scheme could be applied, and the admission plugin would admit or deny
a pod based on the namespace’s pod security labels and the CSI driver’s
effective pod security profile.

This would greatly extend the scope of PodSecurityAdmission plugin beyond the
original pod security standards. The standards were explicitly designed to
allow additional admission controls for CSI ephemeral volumes to be built on
top of the PodSecurityAdmission plugin. Extending it in this way is a
non-starter.

### Implement as a Validating Webhook

The plugin could be implemented as a validating webhook, which would eliminate
the need to carry the plugin as a patch to Kubernetes proper. A separate
webhook would incur extra network hops on every pod creation request. This
would have significant performance impact on OpenShift - thus an admission
plugin invoked directly on the kube-apiserver is appropriate.

## Infrastructure Needed [optional]

N/A
