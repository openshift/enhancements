---
title: cpumanager-alpha-options-enablement
authors:
  - "@ffromani"
reviewers: # Include a comment about what domain expertise a reviewer is expected to bring and what area of the enhancement you expect them to focus on. For example: - "@networkguru, for networking aspects, please look at IP bootstrapping aspect"
  - "@mrunalp"
  - "@haircommander"
  - "@rphilips"
  - "@kannon92"
approvers:
  - "@mrunalp"
api-approvers: # In case of new or modified APIs or API extensions (CRDs, aggregated apiservers, webhooks, finalizers). If there is no API change, use "None"
  - None
creation-date: 2024-12-04
last-updated: 2024-12-04
tracking-link: # link to the tracking ticket (for example: Jira Feature or Epic ticket) that corresponds to this enhancement
  - https://issues.redhat.com//browse/OCPBUGS-44786
---

# Add support for cpumanager policy alpha option `prefer-align-cpus-by-uncorecache`

## Summary

Enable selected users to consume the upstream LLC (Last-Level Cache) cpu
alignment feature introduced as alpha quality in kubernetes 1.32.
This otherwise simple process encompasses the handling of per-group feature gates.

## Motivation

The upstream kubernetes communities added options to fine-tune the behavior
of some kubelet resource managers, notably cpumanager and topology manager.
These options have different levels of maturity and follows roughly the expected
feature lifecycle `alpha->beta->GA`, with one key difference: the feature gates
introduced govern a group of options, grouped by maturity, and the options
transition from a group to another while they mature.

In other words, the feature gate controls the availability of the maturity level
of options, and the set can be empty. Instead of having a 1:1 relationship between
a feature gate and a feature, we now have a 1:N relationship, and that relationship
is time variant.

The alpha-grade feature gate may control features A,B,C say in version 1.30 and
features C,D in version 1.32.

This approach was introduced with [KEP-2625](https://github.com/kubernetes/enhancements/issues/2625)
after engaging with [sig-arch](https://groups.google.com/g/kubernetes-sig-architecture/c/Nxsc7pfe5rw/m/vF2djJh0BAAJ)
precisely with the purpose to avoid feature gate proliferation.

In order to allow selected users to consume alpha-grade feature gates,
the trivial and unlikely desirable approach would be to just add the
`CPUManagerPolicyAlphaOptions` to the relevant OpenShift profile.
The side effect would be that all the alpha options at any given time will
be thus available by default.

We want to enable some key users to consume the alpha-grade option `prefer-align-cpus-by-uncorecache`,
but we don't want them to be able to access all the others unless explicitely granted.

In the rest of the document we focus on how we can enable a single alpha-level
feature gates in OpenShift.

### User Stories

See upstream [KEP-4800](https://github.com/kubernetes/enhancements/issues/4800).

### Goals

- Enable key users to access just and only the alpha-grade option `prefer-align-cpus-by-uncorecache`
- See also upstream [KEP-4800](https://github.com/kubernetes/enhancements/issues/4800).

### Non-Goals

- Allow any users to access any other alpha-grade option in supported mode

## Proposal

The backport of the feature per se in largely linear and straightforward
(proof: [initial PR](https://github.com/openshift/kubernetes/pull/2136))
so the bulk of this document will focus on how to unpack the cpumanager alpha
options feature gate. See Implementation details below.

### Workflow Description

See Implementation Details below.
See also the upstream [KEP-4800](https://github.com/kubernetes/enhancements/issues/4800).

### API Extensions

N/A

### Topology Considerations

#### Hypershift / Hosted Control Planes

N/A

#### Standalone Clusters

N/A

#### Single-node Deployments or MicroShift

N/A

### Implementation Details/Notes/Constraints

#### per-option extra enablement file

The idea behind this approach was already used for [workload partitioning](https://github.com/openshift/enhancements/pull/1421).
In this case, this will replace the feature gate check in the cpumanager policy options
setup logic: instead of a feature gate check, we will check for the presence
of a enablement file.

The enablement file is a well known path which has to exist in the node
filesystem. If present (content is ignored), the feature is enabled; otherwise is not.
This goes in addition (not replacing) the value of the feature gate.

This approach will require a minimal openshift-only change to replace the
guard in the policy option kubelet setup logic.

This approach enables per-policy granularity by checking per-policy enablement files.

### Risks and Mitigations

* Users may forget to set the enablement file.
  The mitigation is to have clear documentation about the requisite to set the feature to on.
* Worse visibility from support perspective about the fact the user
  is opting in, being this not being obvious from the openshift profile.
  The mitigation is to make sure the support tools, first and foremost must-gather, capture
  the enablement file base directory so we can safely detect the feature state.

### Drawbacks

The most obvious option would to _not_ implement any of the approaches
described above. We can just wait for policy options to transition to beta.
This minimizes the maintenance burden but increases the time to market for users.
In addition, the overarching problem still remains but to a lesser extent:
there's no provision to _disable_ a single option, but again only a group
of them.

## Test Plan

No special requirements. For the specific LLC alignment feature,
the Telco QE team already has a test plan ongoing.

## Graduation Criteria

See upstream [KEP-4800](https://github.com/kubernetes/enhancements/issues/4800)
The Openshift specific changes will be dropped once the upstream feature matures to Beta grade or better.

### Dev Preview -> Tech Preview

N/A

### Tech Preview -> GA

N/A

### Removing a deprecated feature

N/A

## Upgrade / Downgrade Strategy

See upstream [KEP-4800](https://github.com/kubernetes/enhancements/issues/4800)

## Version Skew Strategy

See upstream [KEP-4800](https://github.com/kubernetes/enhancements/issues/4800)

## Operational Aspects of API Extensions

No API extension required besides the feature gate management described in Implementation Details above

## Support Procedures

N/A

## Alternatives

#### 1: Enable the `CPUManagerPolicyAlphaOptions` Feature Gate in selected openshift profiles

Enabling the `CPUManagerPolicyAlphaOptions` FeatureGate in `DevPreview` and/or
`TechPreview` profiles will grant users access to _all_ the alpha grade policy options,
which is something undesirable because enable unexpected interactions.
But it's by far and large the simplest option, so is presented for completeness.

The risk is that users can mix and match unsupported options they gain access to
by side effect. While these options can be documented as unsupported, we enable
users to run into avoidable issues.

There's no possible mitigation about this risk beside documentation and clear support
boundaries.

#### 2: Option 1 plus per-option extra enablement file

We can check the presence of the feature gate and in addition also check the
presence of the enablement file (see design details above) like that:

- feature gate off, enablement file absent: option not available
- feature gate off, enablement file present: option not available
- feature gate on, enablement file absent: option not available
- feature gate on, enablement file present: option available

In this case we augment, not replace, the feature gate check.
This approach adds complexity from both software maintenance and operational sides,
and the benefits are questionable because the enablement file already provide
per-option granularity and already conveys quite clearly the users are opting in
an experimental code path.
