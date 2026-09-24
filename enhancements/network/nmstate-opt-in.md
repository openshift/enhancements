---
title: nmstate-opt-in
authors:
  - cybertron
reviewers:
  - zaneb # Installer
  - mkowalski # On-Prem Networking
  - everettraven # API
approvers:
  - zaneb
api-approvers:
  - everettraven
creation-date: 2026-03-09
last-updated: 2026-09-22
status: implementable
tracking-link:
  - https://issues.redhat.com/browse/OPNET-763
see-also:
  - "/enhancements/network/configure-ovs-alternative.md"
replaces:
  - NA
superseded-by:
  - NA
---

# NMState Opt-In for Managing br-ex

## Summary

Provide a mechanism allowing opt-in to NMState managing br-ex, without
requiring any additional configuration. There is
[existing functionality](https://github.com/openshift/enhancements/blob/master/enhancements/network/configure-ovs-alternative.md)
that already allows using NMState, but it requires the user to write a full
NMState config representing the desired end state. This is a significant
increase in effort over the default behavior with configure-ovs.sh, so we want
to provide a simpler option that still allows use of NMState.

## Motivation

* Using NMState to manage br-ex provides much more flexibility in management
  of host networking, in particular the ability to use Kubernetes-NMState to
  make changes on day 2.
* In some instances, the existing functionality may require similar, but
  meaningfully different, NMState configuration in two different places. This
  is confusing, and providing a mechanism that eliminates the second explicit
  config improves the user experience.

### User Stories

* As a cluster administrator, I want to have NMState manage br-ex without
  having to learn the full NMState syntax to lower the barrier to entry for
  using the feature.
* As a cluster administrator, I don't want to specify host networking config
  in multiple places to avoid confusion about what belongs where.
* As a former VMWare administrator, I want a simple way to enable advanced
  networking configurations like I had on VMWare.

### Goals

Provide a simplified mechanism to use NMState for br-ex management.

### Non-Goals

Full replacement of configure-ovs.sh. By making this opt-in, we don't need
to handle every single existing edge case and can focus on new deployments.

## Proposal

### Workflow Description

Cluster deployers will no longer need to write full NMState configs
representing their desired br-ex config. Instead, they will set the
bridgeTopology field in install-config to the value that matches their
desired topology. This will trigger Machine Config Operator to generate
an NMState policy to create the bridge as specified. This will be done
automatically based on the existing network configuration (whether that
was applied by NMState or not), much like configure-ovs.sh works today.
However, NMState will make it easier to update after deployment using
Kubernetes-NMState.

### API Extensions

This enhancement adds two new fields. It does not add any CRDs, webhooks,
aggregated API servers, or finalizers.

* `bridgeTopology`, a new field under the existing `networking` stanza of
  the installer's install-config API. Initially accepts `Standard` or
  unset.
* `BridgeTopology`, a new field on the `status` stanza of the cluster-scoped
  `Infrastructure` resource (`config.openshift.io/v1`), alongside the other
  high-level networking topology fields already present there. Initially
  accepts `Disabled` (default) and `Standard`. This signals MCO which
  configuration, if any, it should generate for setting up br-ex on nodes.
  Placing it in the Infrastructure Status object is consistent with where
  other high-level networking topology fields already live.

  One noteworthy difference from existing on-prem fields is that this one is
  cross-platform rather than platform-specific. We do not intend for this to
  be a platform-specific feature, so we don't want to tie it to one, or even
  several, platforms.

### Topology Considerations

#### Hypershift / Hosted Control Planes

This feature will be delivered as one of the built-in MCO templates that are
delivered via Ignition, even on Hypershift nodes. The configure-ovs script is
deployed the same way, so this should work the same way.

#### Standalone Clusters

Yes

#### Single-node Deployments or MicroShift

This should be usable in SNO deployments, but should not affect resource usage.
I'm unsure whether it makes sense for MicroShift, but if it includes NMState in
the base OS image it should work.

#### OpenShift Kubernetes Engine

No difference.

### Implementation Details/Notes/Constraints

#### Install-config

Example install-config for enabling this feature:

```yaml
# ...
networking:
  networkType: OVNKubernetes
  bridgeTopology: Standard
# ...
```

#### Standard Topology Support

The `Standard` topology will support most common network architectures
currently in use. This includes single interfaces, bonds, and VLANs. Other
interface types should work too, as long as they behave in the same way as
the other supported ones.

All types of addressing (DHCP, DHCPv6, SLAAC, static) will be supported.

We expect that this will cover the vast majority of deployments.

#### Additional Topologies

We anticipate needing to support additional topologies beyond `Standard`.
These topologies will include architectures that cannot be reasonably
represented in a single common configuration, even with templating capability
from NMPolicy.

One example is the balance-slb bond mode. This requires two OVS bridges and
some additional configurations to connect them. There is currently no path to
deploying this architecture using a single configuration.

Another possible example is br-ex1. Currently, configure-ovs supports deploying
an additional OVS bridge named br-ex1. This is a somewhat obscure architecture
that is not widely used and could be replicated by deploying br-ex1 using
a different mechanism (possibly also NMState) and letting this feature handle
br-ex. However, if we look at this as a preliminary step to replacing
configure-ovs, we will need to replicate that functionality anyway.

#### NMState Details

The NMState YAML that will create the bridge selects the interface based on
an altname of "primary", which will be assigned by baremetal-runtimecfg as
part of the nodeip-configuration service. The YAML looks something like this:

```yaml
capture:
  # FIXME: This requires primary to be the first altname. Need NMState support to fix.
  # This is not a blocker problem, but it does introduce an unnecessary requirement.
  base-iface: interfaces.alt-names.0.name == "primary"
desiredState:
  interfaces:
  - name: {{`"{{ capture.base-iface.interfaces.0.name }}"`}}
    type: {{`"{{ capture.base-iface.interfaces.0.type }}"`}}
    state: up
# ...
  - name: br-ex
    type: ovs-interface
    state: up
    copy-mac-from: {{`"{{ capture.base-iface.interfaces.0.name }}"`}}
    mtu: {{`"{{ capture.base-iface.interfaces.0.mtu }}"`}}
    wait-ip: {{`"{{ capture.base-iface.interfaces.0.wait-ip }}"`}}
    ipv4: {{`"{{ capture.base-iface.interfaces.0.ipv4 }}"`}}
    ipv6: {{`"{{ capture.base-iface.interfaces.0.ipv6 }}"`}}
# ...
```

### Risks and Mitigations

* This is less flexible than specifying the entire NMState config. It may not
  work for every use case. The feature is designed to be extensible in the
  future, so we can add automatic configuration for more architectures if
  needed, however.

### Drawbacks

Some users may already have existing full NMState br-ex configs that they would
like to keep using. This is still possible because we are not removing the old
feature, but it also doesn't improve the old, bad interface.

## Alternatives (Not Implemented)

We considered adding an install-config interface to the existing NMState feature.
However, this does not address the duplication issues, and if we are able to
completely replace configure-ovs it may leave us with a useless field in the
install-config API. If we are unable to replace the free-form configuration with
this more templated feature, we may still revisit this.

## Open Questions [optional]

* Is selecting the primary interface based on the "primary" altname acceptable,
  or should we pick something less likely to conflict with existing configs?
  Currently we only support one altname because of a limitation in the NMState
  capture syntax, but if that gets fixed we may want/need to support more.

## Test Plan

We currently have a CI job that tests custom NMState config. We'll either
convert that to this new feature, or add a new, similar job.

## Graduation Criteria

### Dev Preview -> Tech Preview

We do not plan on a dev preview.

### Tech Preview -> GA

We need major customers to use this in their real world architectures to
ensure that it addresses their needs. We also need to ensure that it is
simple enough for more conservative customers to adopt.

### Removing a deprecated feature

NA

## Upgrade / Downgrade Strategy

There is already logic in nmstate-configuration to re-apply modified configs,
so if changes are needed as part of an upgrade, that should be handled already.
We will need to ensure that such changes do not cause any functional problems
on upgrade.

## Version Skew Strategy

Version skew will be handled the same way as any other machine config-based
feature. The node configurations are independent of each other, and as long as
they maintain connectivity it should not matter if the configurations are not
identical.

## Operational Aspects of API Extensions

Nothing new. The only API change here is to add a field for MCO to generate
the new config(s). It will work the same as the other similar fields used by
on-prem infrastructure today.

## Support Procedures

Largely the same as the existing NMState config. The only difference is that
the policy template might be slightly more complex to debug.

## Infrastructure Needed [optional]

NA
