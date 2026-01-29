---
title: configure-ovs-replacement
authors:
  - bnemec
reviewers:
  - mko # OPNET
  - jcaamano # OVNK
approvers:
  - knobunc
api-approvers:
  - None
creation-date: 2026-01-28
last-updated: 2026-01-28
status: implementable
tracking-link:
  - https://issues.redhat.com/browse/OPNET-696
see-also:
  - "/enhancements/network/configure-ovs-alternative.md"
replaces:
  - NA
superseded-by:
  - NA
---

About the enhancement process:
1. **Iterate.** Some sections of the enhancement do not make sense to fill out in the first pass.
   We expect enhancements to be merged with enough detail to implement tech preview, and be updated later
   ahead of promoting to GA.
1. **Build consensus.** The enhancement process is a way to build consensus between multiple stakeholders
   and align on the design before implementation begins. It is the responsibility of the author to drive
   the process. This means that you must find stakeholders, request their review, and work with them to
   address their concerns and get their approval. If you need help finding stakeholders, try asking in
   #forum-ocp-arch or taking your proposal to the OCP arch call or a staff engineer.
1. **Document decisions.** The enhancements act as our record of previous conversations and the decisions
   that were made. It is important that these EPs are merged so that we can build a library of references
   for future engineers/technical writers/support engineers to be able to understand the history of our
   designs and the rationale behind them.
   **Please find the time to make sure that these PRs are merged.** If you are struggling to reach consensus,
   or you are not getting the reviews you need, please reach out to a staff engineer or your team lead to help you.

To get started with this template:
1. **Pick a domain.** Find the appropriate domain to discuss your enhancement.
1. **Make a copy of this template.** Copy this template into the directory for
   the domain.
1. **Fill out the metadata at the top.** The embedded YAML document is
   checked by the linter.
1. **Fill out the "overview" sections.** This includes the Summary and
   Motivation sections. These should be easy and explain why the community
   should desire this enhancement.
1. **Create a PR.** Assign it to folks with expertise in that domain to help
   sponsor the process.
1. **Merge after reaching consensus.** Merge when there is consensus
   that the design is complete enough for implementation to begin.
   It is ok to have some details missing, these should be captured in the open questions.
   Come back and update the document if important details (API field names, workflow, etc.)
   change during implementation.
1. **Keep all required headers.** If a section does not apply to an
   enhancement, explain why but do not remove the section. This part
   of the process is enforced by the linter CI job.

See ../README.md for background behind these instructions.

Start by filling out the header with the metadata for this enhancement.

# Configure-ovs Replacement

## Summary

Provide an NMState-based replacement for the configure-ovs.sh script currently
used to configure the br-ex bridge in OVNKubernetes clusters.

## Motivation

Configure-ovs.sh has historically been problematic for a couple of reasons:

1. It's a large, complex bash script that is difficult to test and is a source
   of quite a few bugs.

2. It conflicts with other network configuration mechanisms, most notably the
   Kubernetes-NMState operator.

NMState is a tool developed and tested by the NetworkManager team so it has
better test coverage. It is also the same tool preferred for day 2 network
configuration changes, which should significantly reduce conflicts.

### User Stories

- As a cluster administrator, I want to deploy a cluster with no special
  network configuration, but still be able to make changes to the network
  after deployment.

- As a cluster administrator, I want to deploy a cluster with a second
  bridge named br-ex1.

- As a cluster administrator, I want to deploy a cluster where br-ex is on
  a different interface from the one selected by the nodeip-configuration
  service.

### Goals

- Make NMState the default method for configuring br-ex.

- Make the switchover entirely invisible to most, if not all, users.

### Non-Goals

Handling complex network configurations. We already have the NMState-based
configuration method which allows custom, more complex configs. We are
only looking to do a 1:1 replacement (or something close to it) of
configure-ovs.sh functionality.

We also want to avoid as much custom code around NMState as possible. For
example, there has been some investigation into converting the existing
configure-ovs script to use NMState instead of nmcli calls, but this
leaves the problem of complex logic in bash, which we'd really like to
avoid for this implementation.

## Proposal

Add a default NMState configuration to each node that will be used if a
specific custom NMState file is not provided. This default configuration
will handle all of the work currently done by configure-ovs to set up the
br-ex bridge. It will use the "capture" feature of NMState to bring over
settings (such as static IPs) from the underlying interface to the bridge.

### Workflow Description

The workflow in the vast majority of cases should be exactly the same as it
is today. Users relying on configure-ovs to create br-ex will transparently
switch to using NMState when we ship this feature as GA.

It is _possible_ some users may need to use the custom br-ex feature instead
of custom configs that were previously passed to configure-ovs. At this time
it remains to be seen if it will be possible to write one NMState config that
handles every edge case configure-ovs currently does.

### API Extensions

NA

### Topology Considerations

#### Hypershift / Hosted Control Planes

TODO: Figure this out. I'm unclear how configure-ovs works in these clusters today
given that it is deployed by MCO and there is no MCO in HCP clusters. In theory
this feature should work the same way since it is also deployed by MCO.

#### Standalone Clusters

This would be the default way to deploy standalone clusters with OVNK.

#### Single-node Deployments or MicroShift

Nothing. This only affects initial boot and won't impact resource usage after
that.

#### OpenShift Kubernetes Engine

Nothing specific. The changes would also apply to OKE, but as this is a core
feature of the product there should be no differences.

### Implementation Details/Notes/Constraints

We should enable the nodeip-configuration service, currently only used for
on-prem and UPI clusters, on all platforms. This should be relatively safe
as it is already used in cluster types where we expect complex networking
environments. The remaining platforms are mostly cloud-based, which tend
to have simpler network configurations (at least in IPI form).

### Risks and Mitigations

There are a number of edge cases currently handled in configure-ovs, and we
will need to make sure we have a path forward for all of those in this new
method.

I propose that we enable this by default in TechPreview clusters for a couple
of releases before we flip the switch. That should allow customers with complex
configurations to easily test it before it ships as GA.

### Drawbacks

This affects a very fundamental part of the product that is used in most
existing clusters. Mistakes made in rolling it out could have significant,
widespread effects.

## Alternatives (Not Implemented)

- Leave configure-ovs.sh for existing clusters and only use this method for new
  ones. The downside of this is we have to keep maintaining configure-ovs
  indefinitely.

- Provide a flag to still allow use of configure-ovs for edge cases that we
  can't easily address with NMState. Same drawback as above.

- Don't replace configure-ovs.sh, but convert the nmcli calls in it to NMState.
  This doesn't address the complexity problems with the bash script, and
  potentially doesn't work better with day 2 configuration if we have to
  continue using ephemeral profiles.

## Open Questions [optional]

- For advanced edge cases currently handled in configure-ovs.sh, can we require
  a move to the custom NMState br-ex feature instead? For example, configure-ovs
  has a br-ex1 secondary bridge. Can we have those users switch to fully custom
  NMState?

- Configure-ovs has special route metrics (48) for routes it creates. Do we need
  something similar for for this, or does NMState just do the right thing?

- Currently configure-ovs.sh uses ephemeral profiles, which means they need to
  be recreated on each boot. Does moving to NMState and allowing modification
  via NMState eliminate the need for this? I think the ephemeral profiles were
  at least in part to allow modification of the underlying interfaces without
  breaking br-ex, but if br-ex and the interfaces can be modified directly,
  we may not need that.

## Test Plan

Since this feature should achieve feature parity with configure-ovs, it will be
imperative that all existing tests (except possibly any relying on implementation
details of configure-ovs) to pass with this enabled. I don't see much need for
additional testing beyond that as we are not planning to

## Graduation Criteria

We will initially turn this on in Tech Preview clusters to ensure it gets
exercised. During that phase, we need to reach out to some large customers
and ensure that their use cases are working with this before we GA it.

### Dev Preview -> Tech Preview

- Ability to utilize the enhancement end to end
- End user documentation, relative API stability
- Sufficient test coverage
- Gather feedback from users rather than just developers
- Enumerate service level indicators (SLIs), expose SLIs as metrics
- Write symptoms-based alerts for the component(s)

### Tech Preview -> GA

- More testing (upgrade, downgrade, scale)
- Sufficient time for feedback
- Available by default
- Backhaul SLI telemetry
- Document SLOs for the component
- Conduct load testing
- User facing documentation created in [openshift-docs](https://github.com/openshift/openshift-docs/)

**For non-optional features moving to GA, the graduation criteria must include
end to end tests.**

### Removing a deprecated feature

NA

## Upgrade / Downgrade Strategy

Upgrades should be fairly seamless. Configure-ovs already uses ephemeral
NetworkManager profiles that disappear on reboot. When upgrading to a
release with this feature, after the node reboots it will switch to managing
br-ex with NMState.

## Version Skew Strategy

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

## Operational Aspects of API Extensions

Describe the impact of API extensions (mentioned in the proposal section, i.e. CRDs,
admission and conversion webhooks, aggregated API servers, finalizers) here in detail,
especially how they impact the OCP system architecture and operational aspects.

- For conversion/admission webhooks and aggregated apiservers: what are the SLIs (Service Level
  Indicators) an administrator or support can use to determine the health of the API extensions

  Examples (metrics, alerts, operator conditions)
  - authentication-operator condition `APIServerDegraded=False`
  - authentication-operator condition `APIServerAvailable=True`
  - openshift-authentication/oauth-apiserver deployment and pods health

- What impact do these API extensions have on existing SLIs (e.g. scalability, API throughput,
  API availability)

  Examples:
  - Adds 1s to every pod update in the system, slowing down pod scheduling by 5s on average.
  - Fails creation of ConfigMap in the system when the webhook is not available.
  - Adds a dependency on the SDN service network for all resources, risking API availability in case
    of SDN issues.
  - Expected use-cases require less than 1000 instances of the CRD, not impacting
    general API throughput.

- How is the impact on existing SLIs to be measured and when (e.g. every release by QE, or
  automatically in CI) and by whom (e.g. perf team; name the responsible person and let them review
  this enhancement)

- Describe the possible failure modes of the API extensions.
- Describe how a failure or behaviour of the extension will impact the overall cluster health
  (e.g. which kube-controller-manager functionality will stop working), especially regarding
  stability, availability, performance and security.
- Describe which OCP teams are likely to be called upon in case of escalation with one of the failure modes
  and add them as reviewers to this enhancement.

## Support Procedures

Describe how to
- detect the failure modes in a support situation, describe possible symptoms (events, metrics,
  alerts, which log output in which component)

  Examples:
  - If the webhook is not running, kube-apiserver logs will show errors like "failed to call admission webhook xyz".
  - Operator X will degrade with message "Failed to launch webhook server" and reason "WehhookServerFailed".
  - The metric `webhook_admission_duration_seconds("openpolicyagent-admission", "mutating", "put", "false")`
    will show >1s latency and alert `WebhookAdmissionLatencyHigh` will fire.

- disable the API extension (e.g. remove MutatingWebhookConfiguration `xyz`, remove APIService `foo`)

  - What consequences does it have on the cluster health?

    Examples:
    - Garbage collection in kube-controller-manager will stop working.
    - Quota will be wrongly computed.
    - Disabling/removing the CRD is not possible without removing the CR instances. Customer will lose data.
      Disabling the conversion webhook will break garbage collection.

  - What consequences does it have on existing, running workloads?

    Examples:
    - New namespaces won't get the finalizer "xyz" and hence might leak resource X
      when deleted.
    - SDN pod-to-pod routing will stop updating, potentially breaking pod-to-pod
      communication after some minutes.

  - What consequences does it have for newly created workloads?

    Examples:
    - New pods in namespace with Istio support will not get sidecars injected, breaking
      their networking.

- Does functionality fail gracefully and will work resume when re-enabled without risking
  consistency?

  Examples:
  - The mutating admission webhook "xyz" has FailPolicy=Ignore and hence
    will not block the creation or updates on objects when it fails. When the
    webhook comes back online, there is a controller reconciling all objects, applying
    labels that were not applied during admission webhook downtime.
  - Namespaces deletion will not delete all objects in etcd, leading to zombie
    objects when another namespace with the same name is created.

## Infrastructure Needed [optional]

Use this section if you need things from the project. Examples include a new
subproject, repos requested, github details, and/or testing infrastructure.
