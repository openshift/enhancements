---
title: csi-inline-vol-security
authors:
  - "@adambkaplan"
  - "@dobsonj"
reviewers:
  - "@deads2k"
  - "@jsafrane"
approvers:
  - "@jsafrane"
api-approvers:
  - "@deads2k"
creation-date: 2022-01-10
last-updated: 2022-09-30
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

A new validating admission plugin - `CSIVolumeAdmission` - will inspect pod
volumes on pod creation. If a pod uses a `csi` volume, the plugin will look up
the CSIDriver object and inspect the `csi-ephemeral-volume-profile` label. This
`CSIVolumeAdmission` plugin will then use the label’s value in its enforcement,
warning, and audit decisions.

Because pod volumes are considered immutable, this admission plugin will only
run its checks on pod creation. Existing pods that use `csi` volumes will not
be affected once the admission plugin is enabled.

### Workflow Description

#### Pod Security Profile Enforcement

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

#### Pod Security Profile Warning

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

#### Pod Security Profile Audit

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

#### Default Behavior

If the referenced CSIDriver for a CSI ephemeral volume does not have the
`csi-ephemeral-volume-profile` label, the `CSIVolumeAdmission` admission plugin
will consider the driver to have the `privileged` profile for enforcement,
warning, and audit behaviors. Likewise, if the pod’s namespace does not have
the pod security admission label set, the admission plugin will assume the
`restricted` profile is allowed for enforcement, warning, and audit decisions.

This means that if no labels are set, CSI ephemeral volumes using that CSIDriver
will _only_ be usable in privileged namespaces by default. This restrictive
default policy prevents risky drivers from being used as inline volumes in
unprivileged namespaces, unless the cluster administrator makes a conscious
decision to allow it.

### API Extensions

This adds a new validating admission plugin which can prevent pods from being
created - `CSIVolumeAdmission`. This admission plugin should only be invoked
when a pod is being created. Pod volumes are considered immutable, therefore
this admission plugin should not prevent pods from being updated under any
circumstance.

This plugin must evaluate all pods and pod-specable workloads (`Deployment`,
`DeploymentConfig`, `Job`, etc.), and the plugin is required for cluster
security and basic cluster functionality. Given the critical nature of this
component, it will be implemented as and admission plugin rather than a webhook
to reduce network round-trips and improve overall stability/reliability.
To do this, we will add a carry-patch to `openshift/kubernetes` containing
the admission plugin logic--similar to SCC.

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

Adding a new validating admission plugin inherently risks the cluster’s
availability and responsiveness. At its most extreme, this proposal will
require the admission plugin to fetch a `CSIDriver` object on every pod
admission request, adding additional strain on the kubernetes apiserver. A
lister for `CSIDriver` objects can mitigate this, and can be particularly
effective because most clusters do not have a large number of `CSIDriver`
objects, and these objects do not change frequently.

The default behavior of this plugin will have an impact on user experience
for any customers using inline volumes, in that the cluster administrator will
have to decide on a policy for each CSI driver that they intend to use for
inline volumes in unprivileged namespaces. This is a tradeoff between security
and usability, and the default behavior favors a secure-by-default configuration
over a more permissive configuration. This deserves some scrutiny during
enhancement review, and approval from the product experience team.

### Drawbacks

See [Risks and Mitigations](#risks-and-mitigations).

## Design Details

### Open Questions [optional]

N/A

### Test Plan

- Unit testing in tree, applied in a carry patch for maintainability.
- Run e2e suite of the Shared Resource CSI driver with the plugin enabled.
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

- Admission plugin added to our fork of kube-apiserver
- Admission plugin disabled by default, feature gate can be enabled with TechPreviewNoUpgrade
- Test coverage in openshift/origin for pods with CSI volumes
- Documentation for the admission plugin - use and configuration
- Verify use with the Shared Resource CSI Driver
- CSI Inline Volumes remains documented as a tech preview feature until admission plugin is GA

#### Tech Preview -> GA

- Seamless upgrade experience for the plugin, enabled by default.
- Pod creation scale testing within acceptable limits.
- End to end testing with pods using volumes from the Shared Resource CSI driver
- Sufficient time for feedback
- User facing documentation created in [openshift-docs](https://github.com/openshift/openshift-docs/)
- CSI Inline Volumes is promoted to GA in OCP along with the admission plugin

#### Removing a deprecated feature

N/A

### Upgrade / Downgrade Strategy

CSI drivers that are managed by OCP and support inline volumes will automatically
have a default CSI ephemeral volume profile applied to the `CSIDriver` object by
Cluster Storage Operator.

Users of inline volumes who upgrade to an OCP release with the admission plugin
enabled will need to add a CSI ephemeral volume profile for any unmanaged `CSIDriver`
objects that should allow the use of inline volumes in unprivileged namespaces.

The CSI Inline Volumes upstream feature graduated to GA in k8s 1.25, and was enabled
by default as a beta feature since k8s 1.15. However, we don't want to declare
this feature GA in OCP until the admission plugin is also GA and enabled by default.
The reason is: if a user already deployed a community driver to use with inline
volumes, and we say it's GA, then we shouldn't break it by enabling the admission
plugin later (assuming the community driver does not have the correct annotation).
This means we will still document CSI Inline Volumes as tech preview in OCP 4.12,
and then graduate both the upstream feature and the admission plugin to GA in 4.13.

### Version Skew Strategy

N/A

### Operational Aspects of API Extensions

#### Impact on existing API Server SLIs

This admission plugin must evaluate all pods and pod-specable workloads. While
the logic is not particularly complex (i.e. look for inline volumes and compare
labels between the namespace and `CSIDriver`), it could potentially impact:

- Latency measures for pod APIs
- Latency measures for kube apiserver
- Memory consumption of kube apiserver
- Number of pod security errors

#### Measuring impact on existing API Server SLIs

- Use `kubelet_pod_start_duration_seconds_bucket` to measure latency impact on pod APIs
- Use `apiserver_request_duration_seconds` to measure latency impact on kube apiserver
- Use `container_memory_working_set_bytes` to measure memory consumption of kube apiserver
- Use `pod_security_errors_total` to measure number errors preventing evaluation of pod security

#### Failure Modes

Possible failure modes:
1. Unanticipated bugs in the admission plugin could prevent Pods from being started (by incorrect validation responses).
2. Additional load of the admission plugin could increase duration of API server responses to Pod creation requests, and in the worst case lead to degradation of the API server.
3. A `CSIDriver` object managed by OCP that supports `Ephemeral` volumes could be missing the `csi-ephemeral-volume-profile` label, leading to an unexpected validation response.
4. API Server failures could prevent the admission plugin from being called.

OCP teams that may be involved with an escalation:
- OCP Storage team for issues caused by the admission plugin
- API Server team for issues invoking the plugin or creating / modifying objects

#### Support Procedures

##### Detecting failure modes

- Check the `CSIDriver` object and the pod's `Namespace` to ensure they have the correct labels as described in the [Proposal](#proposal).
- Check kube-apiserver logs for any errors relating to the `CSIInlineVolumeSecurity` admission plugin.
- Use the metrics described in [Operational Aspects of API Extensions](#operational-aspects-of-api-extensions) to evaluate performance, load, and error rate of the API server and pod start up latency.

##### Disable API extension

While the admission plugin is tech preview, it can be disabled by removing TechPreviewNoUpgrade. No admission checks will be done by this plugin while the feature flag is disabled. Once it graduates to GA, the admission plugin will be enabled as part of OCP without a user-facing way to disable it since this is considered a critical security feature.

## Implementation History

- 2022-01-12: Initial draft
- 2022-09-08: Updates to original proposal

## Alternatives

### Augment the PodSecurityAdmission Plugin

Rather than creating a new admission plugin, this capability could be added by
extending the PodSecurityAdmission plugin. A similar labeling scheme could be
applied, and the admission plugin would admit or deny a pod based on the namespace's
pod security labels and the CSI driver’s effective pod security profile.
However, this would greatly extend the scope of PodSecurityAdmission plugin beyond
the original pod security standards. The standards were explicitly designed to
allow additional admission controls for CSI ephemeral volumes to be built on
top of the PodSecurityAdmission plugin. Extending it in this way is a non-starter.

### Implement as a Validating Webhook

The plugin could be implemented as a validating webhook, which would eliminate
the need to carry the plugin as a patch to Kubernetes proper. However, a separate
webhook would incur extra network hops on every pod creation request, which would
have significant performance impact on OpenShift. Given the critical nature of this
component, an admission plugin invoked directly on the kube-apiserver is appropriate.

## Infrastructure Needed [optional]

N/A
