---
title: on-prem-dual-stack-vips
authors:
  - "@cybertron"
  - "@creydr"
reviewers:
  - "@creydr"
  - "@dougsland"
approvers:
  - "@patrickdillon"
  - "@kirankt"
  - "@shardy"
  - "@cgwalters"
  - "@kikisdeliveryservice"
api-approvers:
  - "@danwinship"
  - "@aojea"
creation-date: 2022-03-01
last-updated: 2022-08-02
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
nodes, it is not necessarily true of the applications that run in or out of
the cluster and may need to interact with the API or ingress services.

This design is proposing the addition of a second pair of VIPs for dual stack
clusters so API and ingress are accessible from either ip version.

## Motivation

Customer applications on dual stack clusters may only have ipv6 connectivity,
but the cluster VIPs will be ipv4 (at this time ipv6 VIPs are not supported
in dual stack clusters).

### Goals

Create both ipv4 and ipv6 VIPs on dual stack clusters so single stack
applications have the connectivity they require.

### Non-Goals

* Making the `kubernetes.default` Service dual stack is not supported and will
not be addressed by this work.

* Configuration of any VIPs beyond a second ipv6 one for dual stack clusters.
MetalLB is a better solution to creating arbitrary loadbalancers.

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

### Drawbacks

Nothing significant. A very small amount of compute resources will be used
to manage the new VIPs. Even this can be avoided by simply not specifying
the second VIP if it is not needed.

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

assisted-service - the hive API extension is used for agent-based cluster creation as a stand-alone tool ("Infrastructure Operator") and as part of RHACM. It will need new API fields that correspond to the changes in the install-config.

### install-config

Currently the VIPs are specified as follows:

```yaml
platform:
  baremetal:
    apiVIP: 192.168.111.5
    ingressVIP: 192.168.111.4
```

After this change it will look more like this:

```yaml
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

The old single-value fields will be replaced by fields that take a list of
VIPs. This allows us to have primary and secondary VIPs based on the order
of the list and is also trivially extensible if we ever need more VIPs.


### openshift/api

We will need to make changes to the
[baremetal platformStatus](https://github.com/openshift/api/blob/354aa98a475c1fcd60b41aaee735da95d7318b42/config/v1/0000_10_config-operator_01_infrastructure.crd.yaml#L311)
to add new fields for the new VIPs.

The new structure of platformStatus would look like this:

```yaml
baremetal:
  description: BareMetal contains settings specific to the BareMetal platform.
  type: object
  properties:
    # Deprecated
    apiServerInternalIP:
      description: apiServerInternalIP is an IP address...
      type: string
    apiServerInternalIPs:
      description: apiServerInternalIPs is a list of IP addresses...
      type: array
    # Deprecated
    ingressIP:
      description: ingressIP is an external IP...
      type: string
    ingressIPs:
      description: ingressIPs is a list of  external IPs...
      type: array
    nodeDNSIP:
    ...
```

If we go with the array format for install-config, it likely makes sense to do
the same in the api. This way we can just loop over the specified VIPs and
do configuration in one block rather than having a separate one for each IP
version.

### machine-config-operator

We will need to wire in the new VIPs to the manifests and configuration
templates. The primary VIP will always be configured, the secondary only if
one was specified.

I believe haproxy already listens on both ipv4 and ipv6 so there should be no
changes needed in that configuration. We'll need to verify that as part of
this implementation though.

We could also modify the apiserver configuration so it listens on both v4 and
v6, but since there's currently no way to set up the `kubernetes.default`
Service as dual stack this doesn't accomplish much. Traffic to the
secondary IP version will have to come in via the VIP, which already has
haproxy to handle both versions.

### baremetal-runtimecfg

There are a number of places in runtimecfg where a VIP is used to determine
which ip version to use (for example, when deciding whether to use the ipv4
or ipv6 addresses in the unicast_peer list). This logic will need to be
updated to handle the fact that there may be both v4 and v6 VIPs.

Additionally, the new VIPs will need to be wired in to the rendering code.

### assisted-service

The new fields in the REST and Kubernetes APIs will need to be handled.
Like install-config and api, assisted will need backward compatibility logic
to handle the migration from the deprecated fields to the new ones. A database
migration will also be needed to deal with the fact that there will be a new
structure for VIP entries. Currently for every cluster in the DB there is a
single-value column in the DB for api_vip and ingress_vip. The new schema will
require us to create a new table with the schema {cluster_id, api_vip}
allowing for multiple entries with the same cluster_id but different IPs.

These are all things that have been done before so they should not be a
problem.

### Open Questions [optional]

How do we add new VIPs to an existing dual stack cluster or to a cluster that
is converted from single stack to dual? We will need to work with the
machine-config-operator team to determine the right way to populate the
necessary fields in the appropriate records. We can still get the feature
implemented for new clusters without solving this, however. The method
of modifying on-prem configuration values will be essentially the same
regardless of what format we choose.

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

CNO takes care updating the new fields (`apiServerInternalIPs` &
`ingressInternalIPs`) and old fields (`apiServerInternalIP` &
`ingressInternalIP`) in [openshift/api](#openshiftapi) to have a consistent API
between versions and therefore keeping clients consuming the old API functional.

The following table shows the rules how the fields are set:

| Case | Initial value of new field | Initial value of old field | Resulting value of new field | Resulting value of old field | Description |
| ---- | -------------------------- | -------------------------- | ---------------------------- | ---------------------------- | ----------- |
| 1    | _empty_                    | foo                        | [0]: foo                     | foo                          | `new` field is empty, `old` with value: set `new[0]` to value from `old` |
| 2    | [0]: foo <br />[1]: bar    | _empty_                    | [0]: foo <br />[1]: bar      | foo                          | `new` contains values, `old` is empty: set `old` to value from `new[0]` |
| 3    | [0]: foo <br />[1]: bar    | foo                        | [0]: foo <br />[1]: bar      | foo                          | `new` field contains values, `old` contains `new[0]`: we are fine, as `old` is part of `new` |
| 4    | [0]: foo <br />[1]: bar    | bar                        | [0]: foo <br />[1]: bar      | foo                          | `new` contains values, `old` contains `new[1]`: as `new[0]` contains the clusters primary IP family, new values take precedence over old values, so set `old` to value from `new[0]` |
| 5    | [0]: foo <br />[1]: bar    | baz                        | [0]: foo <br />[1]: bar      | foo                          | `new` contains values, `old` contains a value which is not included in `new`: new values take precedence over old values, so set `old` to value from `new[0]` (and log a warning) |

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

## Alternatives

Implement this somewhat like the
[existing workaround](https://github.com/yboaron/kepalived-v6) - create a
separate instance of keepalived that is responsible for ipv6 only. This
results in some duplication of containers and configs, but it would probably
simplify the code changes.

### Alternative install-config layouts

#### Just add v6 versions of the VIP fields
```yaml
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
handle older Infrastructure and ControllerConfigs.

#### Deprecate the old fields and add new v4 and v6 fields
```yaml
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

#### Add Secondary fields
```yaml
platform:
  baremetal:
    apiVIP: 192.168.111.5
    # Optional, and only for dual stack clusters
    apiVIPSecondary: fd2e:6f44:5dd8:c956::5
    ingressVIP: 192.168.111.4
    # Optional, and only for dual stack clusters
    ingressVIPSecondary: fd2e:6f44:5dd8:c956::4
```

This format allows us to keep backward compatibility with existing
install-configs, and also allows us to specify that one VIP is primary and
one is secondary for circumstances where that may be important. We will need
validations to ensure that the VIP choices are sane - i.e. right now in a dual
stack cluster the primary VIP must be ipv4, both VIPs should not be the same
IP version, etc. Some amount of new validations are going to be required
regardless of what format we pick though.

### Alternative api structures

#### Add v4 and v6 fields, deprecate existing fields

```yaml
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

#### Add Secondary field

```yaml
baremetal:
  description: BareMetal contains settings specific to the BareMetal platform.
  type: object
  properties:
    apiServerInternalIP:
      description: apiServerInternalIP is an IP address...
      type: string
    apiServerInternalIPSecondary:
      description: apiServerInternalIPSecondary is an additional IP address...
      type: string
    ingressIP:
      description: ingressIP is an external IP...
      type: string
    ingressIPSecondary:
      description: ingressIPSecondary is an additional external IP...
      type: string
    nodeDNSIP:
    ...
```

This has the benefit of not requiring any deprecations. It should also
provide automatic backward compatibility with existing configuration -
in existing clusters the VIP in the original field will by definition
be the primary, whether v4 or v6. Any other proposed structure for these
values would require some amount of migration for existing clusters. This
one does not.

While this does not explicitly state which address is v4 and which is v6,
that shouldn't matter. All we need to do is make sure the ip versions for
things like unicast keepalived match the associated VIP. It doesn't matter
to the consuming code which version that is.

## Infrastructure Needed [optional]

None
