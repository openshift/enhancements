---
title: dual-stack-vips
authors:
  - @cybertron
reviewers:
  - @creydr
approvers:
  - @patrickdillon
  - @kirankt
  - @shardy
  - someone from MCO
api-approvers:
  - ???
creation-date: 2022-03-01
last-updated: 2022-03-02
tracking-link:
  - https://issues.redhat.com/browse/SDN-2213
see-also:
  - "/enhancements/on-prem-service-load-balancers.md"
replaces:
superseded-by:
---

# Dual Stack VIPs

## Summary

Originally the on-prem loadbalancer architecture supported only single-stack
ipv4 or ipv6 deployments. When dual stack support was added for the baremetal
platform, we carried over the VIP configuration that only allowed for a single
VIP to be specified. The thinking was that in dual stack clusters every node
would have access to either ipv4 or ipv6 endpoints. While this is true of the
nodes, it is not necessarily true of the applications that run in the cluster
and may need to interact with the API or ingress services.

This design is proposing the addition of a second VIP for dual stack clusters
so API and ingress are accessible from either ip version.

## Motivation

Customer applications on dual stack clusters may only have ipv6 connectivity,
but the cluster VIPs will be ipv4 (at this time ipv6 VIPs are not supported
in dual stack clusters).

### Goals

Create both ipv4 and ipv6 VIPs on dual stack clusters so single stack
applications have the connectivity they require.

### Non-Goals

Nothing in particular. The problem being solved here is fairly contained, so
scope creep should not be a problem.

## Proposal

### User Stories

As a deployer of a dual stack OpenShift cluster, I want to run ipv6-only
applications that need access to the API and ingress VIPs.

### API Extensions

New fields will have to be added to the
[platform status section](https://github.com/openshift/api/blob/24043075985b3dc87190174241f3c4d0416b8ea2/config/v1/types_infrastructure.go#L465)
of the infrastructure object to persist the specified additional VIPs.

### Implementation Details/Notes/Constraints [optional]

This should be a relatively simple feature to implement since most of the
new code, manifests, and templates will be slight modifications of the
existing loadbalancer ones. The main thing we need to determine is what the
structure of the install-config and api changes will be. That will have an
impact on the implementation of these new VIPs, but any of the designs
proposed below should be doable. It's mostly a question of picking the one
that will provide the best user experience and, secondarily, the simplest
implementation.

### Risks and Mitigations

Minimal risk. This is just adding another VIP, something we already have in
our deployments. The main concern would be logic errors arising out of
the need to handle multiple VIPs, which (if they happen) will need to be
addressed as bugs.

## Design Details

This feature will require changes in a few different components:

Installer - New fields will need to be added to install-config for deployers
to specify the additional VIPs.

API - New fields will need to be added to the platform status section of the
infrastructure object.

machine-config-operator - The on-prem loadbalancer templates and manifests
will need to be updated to support the additional VIPs.

baremetal-runtimecfg - There is logic that depends on the ip version of the
VIP. If there are multiple VIPs, this will have to be modified to account for
that. In addition, the render code will need the new values wired in.

### install-config

Currently the VIPs are specified as follows:

```
platform:
  baremetal:
    apiVIP: 192.168.111.5
    ingressVIP: 192.168.111.4
```

There are a few options for how we could add the new VIPs (we need to pick
one and move the others to the alternatives section).

#### Just add v6 versions of the VIP fields
```
platform:
  baremetal:
    apiVIP: 192.168.111.5
    apiVIPv6: f00::5
    ingressVIP: 192.168.111.4
    ingressVIPv6: f00::4
```
This is a minimal change, but it leaves us with somewhat of a problem in that
we can't assume the old fields always contain a v4 address since previous
releases allowed either v4 or v6. We may be able to deal with that by
detecting the appropriate ip version for the old fields and populating new
fields on the api side. E.g. if apiVIP is v4, we populate
`platformStatus.apiServerInternalIPv4` and if it is v6 we populate
`apiServerInternalIPv6` instead. That way the only place we need to worry about
the difference is in the installer. We would need a validation to ensure a v6
VIP isn't specified in both fields.

Note that this assumes we replace the existing api fields with two new ones
per VIP and do not reuse the existing ones at all. I think we're going to want
to do that regardless of what install-config layout we use so we don't need
logic on the backend in machine-config-operator and baremetal-runtimecfg to
handle older cluster configs.

#### Deprecate the old fields and add new v4 and v6 fields
```
platform:
  baremetal:
    # Deprecated
    apiVIP: 192.168.111.5
    # Deprecated
    ingressVIP: 192.168.111.4

    apiVIPv4: 192.168.111.5
    apiVIPv6: f00::5
    ingressVIPv4: 192.168.111.4
    ingressVIPv6: f00::4
```
This is more of a 1-1 mapping to what I anticipate the platformStatus api
will look like. We would still need compatibility logic to handle old-style
install-configs that set the deprecated fields, but having no overlap between
the old and new-style configs might simplify the logic somewhat.

#### Add plural fields that take a list of VIPs
```
platform:
  baremetal:
    # Deprecated
    apiVIP: 192.168.111.5
    # Deprecated
    ingressVIP: 192.168.111.4

    apiVIPs:
    - 192.168.111.5
    - f00::5
    ingressVIPs:
    - 192.168.111.4
    - f00::4
```
This is essentially a variation on the previous option where the two separate
fields per VIP are combined into a list field with a length of up to 2. In
this case we'd need some logic to determine whether the VIPs in the list are
v4 or v6, but I expect that would only be a little more complicated than the
other options. This has the advantage of being extensible if we ever wanted
to add more addresses, but I admit to having a hard time coming up with a use
case where more than an ipv4 and ipv6 address would be needed.

### openshift/api

We will need to make changes to the
[baremetal platformStatus](https://github.com/openshift/api/blob/354aa98a475c1fcd60b41aaee735da95d7318b42/config/v1/0000_10_config-operator_01_infrastructure.crd.yaml#L311)
to add new fields for the new VIPs. Unlike the install-config layout, I think
it is clearly better to add discrete new fields for this feature. That way,
backend code in machine-config-operator and baremetal-runtimecfg will not
need to detect whether the values in the old fields are from a new (where
the field will have to be v4) or old (where the existing field can be either
v4 or v6) configuration.

Ideally we would transition existing values to the new fields on upgrade.
I'm unsure whether a mechanism exists for us to do that, however. If not, then
some logic (probably in MCO) will be needed to map values from the old fields
to the appropriate ip version.

The new structure of platformStatus would look like this:

```
baremetal:
  description: BareMetal contains settings specific to the BareMetal platform.
  type: object
  properties:
    # Deprecated
    apiServerInternalIP:
      description: apiServerInternalIP is an IP address...
      type: string
    apiServerInternalIPv4:
      description: apiServerInternalIPv4 is an IP address...
      type: string
    apiServerInternalIPv6:
      description: apiServerInternalIPv6 is an IP address...
      type: string
    # Deprecated
    ingressIP:
      description: ingressIP is an external IP...
      type: string
    ingressIPv4:
      description: ingressIPv4 is an external IP...
      type: string
    ingressIPv6:
      description: ingressIPv6 is an external IP...
      type: string
    nodeDNSIP:
    ...
```

### machine-config-operator

We will need to wire in the new VIPs to the manifests and configuration
templates. There should be conditionals around the v4 and v6 sections
so only the appropriate ones are enabled based on the provided VIPs.

I believe haproxy already listens on both ipv4 and ipv6 so there should be no
changes needed in that configuration. We'll need to verify that as part of
this implementation though. Note that as of this writing, the apiserver
does not support dual stack anyway so all traffic through haproxy will end
up on one or the other ip version.

As noted above, it would be best to migrate the existing VIP fields to the new
v4 and v6 fields to simplify the logic in consumers. We may be able to do that
in the
[merge code run on upgrade](https://github.com/openshift/machine-config-operator/blob/080919dc992d600706b679eb118393ee293496f7/lib/resourcemerge/machineconfig.go#L68).
I'm unsure whether that can modify the actual infrastructure object, but
it can massage the data put into controllerconfig, which is where we actually
get the values.

### baremetal-runtimecfg

There are a number of places in runtimecfg where a VIP is used to determine
which ip version to use (for example, when deciding whether to use the ipv4
or ipv6 addresses in the unicast_peer list). This logic will need to be
updated to handle the fact that there may be both v4 and v6 VIPs.

Additionally, the new VIPs will need to be wired in to the rendering code.

### Open Questions [optional]

- See above options for install-config structure
- If we add the v4 and v6 VIP fields to the api as entirely new fields, is
  there a way for us to take the values from the old fields in existing
  clusters and map them to the appropriate v4 or v6 field on upgrade?

### Test Plan

Most of this feature will be covered by the existing dual stack test jobs.
A few added tests will be needed to verify the functionality of the new VIPS.

### Graduation Criteria

We do not anticipate needing a graduation process for this feature. The
internal loadbalancer implementation has been around for a number of
releases at this point and we are just extending it.

#### Dev Preview -> Tech Preview

NA

#### Tech Preview -> GA

NA

#### Removing a deprecated feature

NA

### Upgrade / Downgrade Strategy

Upgrades and downgrades will be handled the same way they are for the current
internal loadbalancer implementation. On upgrade, existing VIP configurations
will be maintained. We will not (and cannot) automatically add new VIPs to a
cluster on upgrade. If a deployer of an older dual stack cluster wants the new
VIP functionality that will have to be a separate operation from upgrade.

### Version Skew Strategy

This will also be handled as it is today. The keepalived configuration used
for the new version must work with the old version until all nodes have been
upgraded. If breaking changes are needed, there will be a migration mechanism
that runs after upgrade is complete (similar to what we did in the multicast
to unicast migration).

### Operational Aspects of API Extensions

NA

#### Failure Modes

NA

#### Support Procedures

NA

## Implementation History

4.11: Initial implementation

## Drawbacks

Nothing significant. A very small amount of compute resources will be used
to manage the new VIPs. Even this can be avoided by simply not specifying
the second VIP if it is not needed.

## Alternatives

Implement this somewhat like the
[existing workaround](https://github.com/yboaron/kepalived-v6) - create a
separate instance of keepalived that is responsible for ipv6 only. This
results in some duplication of containers and configs, but it would probably
simplify the code changes.

## Infrastructure Needed [optional]

None
