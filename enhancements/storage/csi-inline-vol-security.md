---
title: csi-inline-vol-security
authors:
  - "@adambkaplan"
  - "@dobsonj"
reviewers:
  - "@deads2k"
  - "@gnufied"
approvers:
  - "@jsafrane"
api-approvers:
  - "@deads2k"
  - "@sttts"
creation-date: 2022-01-10
last-updated: 2022-09-08
tracking-link:
  - https://issues.redhat.com/browse/STOR-746
see-also: 
  - "/enhancements/subscription-content/subscription-content-access.md"
  - "/enhancements/cluster-scope-secret-volumes/csi-driver-host-injections.md"
  - "/enhancements/authentication/pod-security-admission.md"
  - "/enhancements/cluster-scope-secret-volumes/shared-resource-validation.md"
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

### User Stories

As an OCP cluster administrator, I want to indicate that a CSI driver
should not be used by the restricted profile so that a particular CSI driver is
not used to mount CSI ephemeral volumes in restricted namespaces.

As an OCP cluster administrator, I want to prevent pods from using CSI
ephemeral volumes unless I have deemed the underlying CSI driver “safe.”

As a maintainer who distributes CSI drivers that support CSI ephemeral volumes,
I want to indicate that my driver is not suitable for restricted/
security-conscious use cases.

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

A new validating admission webhook - `CSIVolumeAdmission` - will inspect pod
volumes on pod creation. If a pod uses a `csi` volume, the webhook will look up
the CSIDriver object and inspect the `csi-ephemeral-volume-profile` label. This
`CSIVolumeAdmission` webhook will then use the label’s value in its enforcement,
warning, and audit decisions.

Because pod volumes are considered immutable, this admission webhook will only
run its checks on pod creation. Existing pods that use `csi` volumes will not
be affected once the admission webhook is enabled.

### Workflow Description

#### Pod Security Profile Enforcement

When a `CSIDriver` has the `csi-ephemeral-volume-profile` label, pods using the
CSI driver to mount CSI ephemeral volumes must run in a namespace that enforces
a pod security standard of equal or greater permission. If the namespace
enforces a more restrictive standard, the `CSIVolumeAdmission` admission webhook
should deny admission.

The following table demonstrates the pod admission behavior for a pod that
mounts a CSI ephemeral volume with the given pod security enforcement profile
and CSI ephemeral volume profile:

| PodSecurity Profile | driver label: restricted | driver label: baseline | driver label: privileged |
| ------------------- | ----------------- | --------------- | ----------------- |
| restricted | Allowed | Denied  | Denied  |
| baseline   | Allowed | Allowed | Denied  |
| privileged | Allowed | Allowed | Allowed |

#### Pod Security Profile Warning

The `CSIVolumeAdmission` admission webhook can also provide user-facing warnings
(via appropriate HTTP response headers) if the CSI driver’s effective profile
is more permissive than the pod security warning profile for the pod namespace.

The following table demonstrates the pod admission behavior with the given pod
security warning profile and CSI driver effective profile:

| PodSecurity Profile | driver label: restricted | driver label: baseline | driver label: privileged |
| ------------------- | ----------------- | --------------- | ----------------- |
| restricted | No Warning | Warning    | Warning  |
| baseline   | No Warning | No Warning | Warning  |
| privileged | No Warning | No Warning | No Warning |

#### Pod Security Profile Audit

The `CSIVolumeAdmission` admission webhook can also apply audit annotations to
the pod if the CSI driver’s effective profile is more permissive than the pod
security audit profile for the pod namespace.

The following table demonstrates the pod admission behavior with the given pod
security audit profile and CSI driver effective profile:

| PodSecurity Profile | driver label: restricted | driver label: baseline | driver label: privileged |
| ------------------- | ----------------- | --------------- | ----------------- |
| restricted | No Audit | Audit    | Audit  |
| baseline   | No Audit | No Audit | Audit  |
| privileged | No Audit | No Audit | No Audit |

#### Default Behavior

If the referenced CSIDriver for a CSI ephemeral volume does not have the
`csi-ephemeral-volume-profile` label, the `CSIVolumeAdmission` admission webhook
will consider the driver to have the `privileged` profile for enforcement,
warning, and audit behaviors. Likewise, if the pod’s namespace does not have
the pod security admission label set, the admission webhook will assume the
`restricted` profile is allowed for enforcement, warning, and audit decisions.

This means that if no labels are set, CSI ephemeral volumes using that CSIDriver
will _only_ be usable in privileged namespaces by default. This restrictive
default policy prevents risky drivers from being used as inline volumes in
unprivileged namespaces, unless the cluster administrator makes a conscious
decision to allow it.

Like other admission webhooks, this webhook will have its own configuration API
which allows this behavior to be tuned. The following example sets the
`restricted` profile on CSIDriver objects by default, and the `privileged`
profile as the default for namespaces. This configuration reverses the default
behavior and will effectively run in a "no-op" mode.

```yaml
defaults:
  …
  csi-ephemeral-volume-profile: restricted
  pod-security-enforce-profile: privileged
  pod-security-warn-profile: privileged
  pod-security-audit-profile: privileged
```

### API Extensions

This adds a new validating admission webhook which can prevent pods from being
created - `CSIVolumeAdmission`. This admission webhook should only be invoked
when a pod is being created. Pod volumes are considered immutable, therefore
this admission webhook should not prevent pods from being updated under any
circumstance.

This admission webhook will impact any workload that can add a `csi` volume to
a Pod. No OpenShift component is known to directly consume `csi` volumes today.
Any pod-specable workload (`Deployment`, `DeploymentConfig`, etc.) and
OpenShift Builds can consume `csi` volumes with approriate user specifications.

OpenShift components that add `CSIDriver` objects that provide the `Ephemeral`
volume capability should add the appropriate security profile label. As of 4.12,
only the [Shared Resource CSI Driver](https://github.com/openshift/csi-driver-shared-resource)
implements this capability in OpenShift. Other third party CSI drivers may
implement this capability, notably the [Secret Store CSI Driver](https://secrets-store-csi-driver.sigs.k8s.io/).
Inline volumes using third party CSI drivers that do not have an appropriate
security profile label can be used only in privileged namespaces, as described
in the [Default Behavior](#default-behavior) section.

### Implementation Details/Notes/Constraints [optional]

N/A

### Risks and Mitigations

Adding a new validating admission webhook inherently risks the cluster’s
availability and responsiveness. At its most extreme, this proposal will
require the admission webhook to fetch a `CSIDriver` object on every pod
admission request, adding additional strain on the kubernetes apiserver. A
lister for `CSIDriver` objects can mitigate this, and can be particularly
effective because most clusters do not have a large number of `CSIDriver`
objects, and these objects do not change frequently.

If the webhook fails, it could have a major impact on cluster availability,
specifically when starting new pods. To prevent such a failure from impacting
the rest of the control plane, the webhook configuration will exempt control
plane pods and the pods in the namespace hosting the webhook. It should also
be possible to manually disable the webhook if desired.

Implementing this as an admission webhook also means incurring extra network
hops on every pod creation request. This can have a significant performance
impact on OpenShift. This performance impact needs to be measured and well
understood to avoid serious regressions before this feature moves to GA. We
may need some assistance from the performance and scale team to quantify this.

The webhook will run in a privileged namespace (`openshift-cluster-csi-drivers`),
requires permissions to read all `CSIDriver` objects on the cluster, and is
designed to review pod admission requests and allow or deny those requests.
The permissions will be limited to only what is required by the webhook, and
it will go through the standard security review by the Product Security team
to ensure we are limiting the security risks in this design.

The default behavior of this webhook will have an impact on user experience
for any customers using inline volumes, in that the cluster administrator will
have to decide on a policy for each CSI driver that they intend to use for
inline volumes in unprivileged namespaces. This is a tradeoff between security
and usability, and the default behavior favors a secure-by-default configuration
over a more permissive configuration. This deserves some scrutiny during
enhancement review, and approval from the product experience team.

### Drawbacks

Aside from the risks and mitigations described in the previous section, this
proposal adds a new admission webhook with behavior and functionality that is
related to the existing PodSecurityAdmission plugin. One challenge of this
approach is the handling of defaults when the CSI driver does not have an
effective profile label, the namespace does not have a pod security enforcement
label, or both. The admission webhook would need its own configuration for
default behavior, and skew between its behavior and the PodSecurityAdmission
admission plugin could lead to undesirable outcomes. For example,
PodSecurityAdmission could be configured to enforce the `restricted` profile by
default in all namespaces, whereas this new admission webhook could be
configured to assume the `privileged` namespace profile is enforced by default.
This could cause the admission webhook to allow admission of a pod that uses a
`privileged` CSI driver in a namespace that has the `restricted` profile enforced.

## Design Details

### Open Questions [optional]

1. Is the default behavior reasonable, given the security and usability trade-offs?
2. Is a webhook an acceptable choice, given the availability requirements during pod creation?

### Test Plan

- Unit testing in the repo with the webhook.
- Run e2e suite of the Shared Resource CSI driver with the webhook enabled.
- End to end testing in openshift/origin, tested against the following:
  - Build pods that utilize the Shared Resource CSI driver
  - Workloads that utilize a `privileged` CSI driver in a `restricted` and
    `baseline` namespace.

### Graduation Criteria

| OpenShift | Maturity |
| --------- | -------- |
| 4.12 | Tech Preview |
| 4.13 | GA |

#### Dev Preview -> Tech Preview

- Admission webhook implementation behind feature gate
- Test coverage for pods with CSI volumes
- Documentation for the admission plugin - use and configuration
- Verify use with the Shared Resource CSI Driver

#### Tech Preview -> GA

- Seamless upgrade experience for the plugin, enabled by default.
- Pod creation scale testing within acceptable limits.
- End to end testing with pods using volumes from the Shared Resource CSI driver
- Sufficient time for feedback
- User facing documentation created in [openshift-docs](https://github.com/openshift/openshift-docs/)

#### Removing a deprecated feature

N/A

### Upgrade / Downgrade Strategy

**TODO**

### Version Skew Strategy

N/A

### Operational Aspects of API Extensions

**TODO**

[Shared Resource Validation](https://github.com/openshift/enhancements/blob/master/enhancements/cluster-scope-secret-volumes/shared-resource-validation.md#operational-aspects-of-api-extensions) can serve as an example here.

#### New SLIs for the new admission validations to help cluster administrators and support

#### Impact of existing API Server SLIs

#### Measuring / Verifying impact on existing API Server SLIs


#### Failure Modes

**TODO**

<!--
- Describe the possible failure modes of the API extensions.
- Describe how a failure or behaviour of the extension will impact the overall cluster health
  (e.g. which kube-controller-manager functionality will stop working), especially regarding
  stability, availability, performance and security.
- Describe which OCP teams are likely to be called upon in case of escalation with one of the failure modes
  and add them as reviewers to this enhancement.
-->

#### Support Procedures

**TODO**

## Implementation History

- 2022-01-12: Initial draft
- 2022-09-08: Updates to original proposal and use webhook

## Alternatives

### Implement as an admission plugin

This was originally proposed as an admission plugin, rather than a webhook.
However, this would require a carry patch in `openshift/kubernetes` which
we want to avoid. Validating webhooks were designed with use cases like
this in mind, and it results in a cleaner implementation without needing
to maintain a carry patch indefinitely. There is also a precedent for
implementing a validating webhook for inline volumes in the
[Shared Resource Validation](/enhancements/cluster-scope-secret-volumes/shared-resource-validation.md) enhancement.

## Infrastructure Needed [optional]

N/A
