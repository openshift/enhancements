---
title: build-csi-volumes
authors:
  - "@adambkaplan"
reviewers:
  - "@deads2k"
  - "@slaskawi"
  - "@s-urbaniak"
  - "@Anandnatraj"
approvers:
  - "@bparees"
creation-date: 2021-09-10
last-updated: 2021-09-10
status: implementable
see-also:
  - "/enhancements/builds/volume-mounted-resources.md"
  - "/enhancements/cluster-scope-secret-volumes/csi-driver-host-injections.md"
  - "/enhancements/subscription-content/subscription-content-access.md"
replaces: []
superseded-by: []
---

Start by filling out this header template with metadata for this enhancement.

* `reviewers`: This can be anyone that has an interest in this work.

* `approvers`: All enhancements must be approved, but the appropriate people to
  approve a given enhancement depends on its scope.  If an enhancement is
  limited in scope to a given team or component, then a peer or lead on that
  team or pillar is an appropriate approver.  If an enhancement captures
  something more broad in scope, then a member of the OpenShift architects team
  or someone they delegate would be appropriate.  Examples would be something
  that changes the definition of OpenShift in some way, adds a new required
  dependency, or changes the way customers are supported.  Use your best
  judgement to determine the level of approval needed.  If you’re not sure,
  just leave it blank and ask for input during review.

# Build CSI Volumes

This is the title of the enhancement. Keep it simple and descriptive. A good
title can help communicate what the enhancement is and should be considered as
part of any review.

The YAML `title` should be lowercased and spaces/punctuation should be
replaced with `-`.

To get started with this template:
1. **Pick a domain.** Find the appropriate domain to discuss your enhancement.
1. **Make a copy of this template.** Copy this template into the directory for
   the domain.
1. **Fill out the "overview" sections.** This includes the Summary and
   Motivation sections. These should be easy and explain why the community
   should desire this enhancement.
1. **Create a PR.** Assign it to folks with expertise in that domain to help
   sponsor the process.
1. **Merge at each milestone.** Merge when the design is able to transition to a
   new status (provisional, implementable, implemented, etc.). View anything
   marked as `provisional` as an idea worth exploring in the future, but not
   accepted as ready to execute. Anything marked as `implementable` should
   clearly communicate how an enhancement is coded up and delivered. If an
   enhancement describes a new deployment topology or platform, include a
   logical description for the deployment, and how it handles the unique aspects
   of the platform. Aim for single topic PRs to keep discussions focused. If you
   disagree with what is already in a document, open a new PR with suggested
   changes.
1. **Keep all required headers.** If a section does not apply to an
   enhancement, explain why but do not remove the section. This part
   of the process is enforced by the linter CI job.

The `Metadata` section above is intended to support the creation of tooling
around the enhancement process.

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

This proposal extends the build [Volume Mounted Resources](volume-mounted-resources.md) feature to support
[CSI ephemeral volumes](https://kubernetes.io/docs/concepts/storage/ephemeral-volumes/#csi-ephemeral-volumes).

## Motivation

In the [Share Secrets and ConfigMaps Across Namespaces](../cluster-scope-secret-volumes/csi-driver-host-injections.md) feature,
OpenShift will add a CSI driver that shares Secrets and ConfigMaps through a CSI ephemeral volume.
Builds should support this so they can mount shared Secrets and ConfigMaps.
This is critical for consumption of RHEL subscription content - see the [Subscription Content Access](../subscription-content/subscription-content-access.md) proposal.
CSI ephemeral volumes can also be provided by useful 3rd party CSI drivers, such as the [Secret Store CSI Driver](https://secrets-store-csi-driver.sigs.k8s.io/).

### Goals

* Builds can mount a CSI ephmeral volume

### Non-Goals

* Provide admission control for CSI ephemeral volume usage

## Proposal

### User Stories

As a developer building applications on OpenShift,
I want to mount the cluster's RHEL entitlement into my builds
so that I can `dnf install` RHEL subscritpion content in my Docker strategy build.

As a developer building applications on OpenShift,
I want to mount sealed secrets into my builds
so that I can use credentials that are too sensitive to be exposed as a normal Kubernetes secret.

### Implementation Details/Notes/Constraints [optional]

The Source and Docker strategy APIs will be extended to support `csi` as a volume source option:

```yaml
spec:
  ...
  dockerStrategy: # also applies to sourceStrategy
    volumes:
    - name: csi-ephemeral
      mounts:
      - destinationPath: /etc/pki/entitlement
      source:
        type: CSI
        csi:
          driver: shared-resource.csi.storage.openshift.io # from the upstream CSI volume source
          fsType: "ext4" # optional, from upstream CSI volume source
          nodePublishSecretRef: null # optional, from upstream CSI volume source
          volumeAttributes: # inherited from the upstream CSI volume source
            share: etc-pki-entitlement
```

The fields within the `csi` object are inherited from the upstream volume source
[spec](https://kubernetes.io/docs/reference/kubernetes-api/config-and-storage-resources/volume/).
There is a `readOnly` field in the upstream spec which will be omitted from the build API because builds do not support writeable volumes at this time.

When the build pod is created, the fields in the `csi` object will be converted to a pod CSI volume mount.
The existin mechanisms in the Volume Mounted Resources feature will then wire the volume mount through to buildah's build environment.
Because writeable volumes are not supported, the `readOnly` field will be set to `true` when the build pod is created.

#### Tech Preview Feature Gate

Clusters must opt into enabling this capability by enabling the `BuildCSIVolumes` feature gate.
This feature gate will be added to OpenShift's tech preview set of features.
When a cluster is installed with (or later enables) tech preview features, `csi` volumes can be added to builds.

If the `BuildCSIVolumes` feature gate is not enabled, builds which use `csi` volumes should fail prior to the creation of the build pod.

### Risks and Mitigations

**Risk:** A vulnerability in an underlying CSI driver can let a process escape the build container.

*Mitigation:*
Upstream Kubernetes considers CSI ephemeral volumes safe, even for restricted users.
See the [Pod Security Admission Control KEP](https://github.com/kubernetes/enhancements/blob/master/keps/sig-auth/2579-psp-replacement/README.md#pod-security-standards) for justification.
Only users with elevated permissions (admin/cluster admin) are allowed to install CSI drivers.
The admin takes responsibility for ensuring the CSI driver is "safe".
We will only provide a limited set of CSI drivers in the OCP payload that support CSI ephemeral volumes, which we control and vet.

**Risk:** Builds can be stuck in a "Pending" state if the CSI driver fails/refuses to mount the volume.

*Mitigation:* The build needs to reflect the pod state faithfully, potentially drilling into the pod's state to determine if a failed volume mount is the reason for the pending status.

## Design Details

### Open Questions [optional]

1. Do we need admission control for CSI ephemeral volumes to consider this feature GA?

### Test Plan

TODO

**Note:** *Section not required until targeted at a release.*

Consider the following in developing a test plan for this enhancement:
- Will there be e2e and integration tests, in addition to unit tests?
- How will it be tested in isolation vs with other components?
- What additional testing is necessary to support managed OpenShift service-based offerings?

No need to outline all of the test cases, just the general strategy. Anything
that would count as tricky in the implementation and anything particularly
challenging to test should be called out.

All code is expected to have adequate tests (eventually with coverage
expectations).

### Graduation Criteria

#### Dev Preview -> Tech Preview

No Dev Preview/alpha phase is expected, this will be initially launched as tech preview.

#### Tech Preview -> GA

- API for build CSI volumes is finalized. Note that the tech preview API must be backwards compatible!
- Admission control strategy for CSI drivers that provide CSI ephemeral volumes.
- End to end tests verify the RHEL subscription content access use case works.

#### Removing a deprecated feature

N/A

### Upgrade / Downgrade Strategy

TBD

If applicable, how will the component be upgraded and downgraded? Make sure this
is in the test plan.

Consider the following in developing an upgrade/downgrade strategy for this
enhancement:
- What changes (in invocations, configurations, API use, etc.) is an existing
  cluster required to make on upgrade in order to keep previous behavior?
- What changes (in invocations, configurations, API use, etc.) is an existing
  cluster required to make on upgrade in order to make use of the enhancement?

Upgrade expectations:
- Each component should remain available for user requests and
  workloads during upgrades. Ensure the components leverage best practices in handling [voluntary disruption](https://kubernetes.io/docs/concepts/workloads/pods/disruptions/). Any exception to this should be
  identified and discussed here.
- Micro version upgrades - users should be able to skip forward versions within a
  minor release stream without being required to pass through intermediate
  versions - i.e. `x.y.N->x.y.N+2` should work without requiring `x.y.N->x.y.N+1`
  as an intermediate step.
- Minor version upgrades - you only need to support `x.N->x.N+1` upgrade
  steps. So, for example, it is acceptable to require a user running 4.3 to
  upgrade to 4.5 with a `4.3->4.4` step followed by a `4.4->4.5` step.
- While an upgrade is in progress, new component versions should
  continue to operate correctly in concert with older component
  versions (aka "version skew"). For example, if a node is down, and
  an operator is rolling out a daemonset, the old and new daemonset
  pods must continue to work correctly even while the cluster remains
  in this partially upgraded state for some time.

Downgrade expectations:
- If an `N->N+1` upgrade fails mid-way through, or if the `N+1` cluster is
  misbehaving, it should be possible for the user to rollback to `N`. It is
  acceptable to require some documented manual steps in order to fully restore
  the downgraded cluster to its previous state. Examples of acceptable steps
  include:
  - Deleting any CVO-managed resources added by the new version. The
    CVO does not currently delete resources that no longer exist in
    the target version.

### Version Skew Strategy

TBD

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

## Implementation History

- 2021-09-10: Initial draft

## Drawbacks

TODO

The idea is to find the best form of an argument why this enhancement should _not_ be implemented.

## Alternatives

TODO

Similar to the `Drawbacks` section the `Alternatives` section is used to
highlight and record other possible approaches to delivering the value proposed
by an enhancement.

## Infrastructure Needed [optional]

Use this section if you need things from the project. Examples include a new
subproject, repos requested, github details, and/or testing infrastructure.

Listing these here allows the community to get the process for these resources
started right away.
