---
title: console-expose-network-features
authors:
- "@mariomac"
- "@jotak"
reviewers:
- "@abhat"
- "@jotak"
approvers:
- "@abhat"
creation-date: 2021-07-07
last-updated: 2021-07-07
status: provisional
see-also:
replaces:
superseded-by:
---

# Console: expose network features

## Release Signoff Checklist

- [X] Enhancement is `implementable`
- [X] Design details are appropriately documented from clear requirements
- [X] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

The Console may need to show some fields that are exclusive to a given CNI type, and hide the fields that are not
provided by it. Then the console needs visibility about which CNI (`OpenShiftSDN` or `OVNKubernetes`) features
are available, depending on each customer's configuration.

This information should be available to all users (or, at this moment, to any user able to create
a Network Policy).

## Motivation

[In the new network policy creation forms, some fields may depend on the type
of cluster network type](https://issues.redhat.com/browse/NETOBSERV-16).
For example, `OpenShiftSDN` neither supports egress
network policies nor ingress exceptions. The related fields should be only visible
when the cluster network is `OVNKubernetes`.

### Goals

* The console can fetch a set of CNI features, which would depend on the CNI type, whichever user
  is logged in.

### Non-Goals

N/A.

## Proposal

The Console UI should be able to query the SDN component for a set of available features. For
example, the SDN component would return a list of named features, e.g.:

```json
{
  "features": {
    "network_policy_egress": true,
    "network_policy_peer_ipblock_exceptions": true
  }
}
```

or:

```json
{
  "features": {
    "network_policy_egress": false,
    "network_policy_peer_ipblock_exceptions": false
  }
}
```

The decision of exposing the features as a `string` -> `boolean` map instead of an array is
argumented in the [Version Skew Strategy](#version-skew-strategy) section.

### User Stories

* As an Openshift Console developer, I would like to get visibility about the features provided
  by the CNI type that runs in the customer cluster, so I can conditionally hide network policy fields
  that do not apply for the user's network type.
  
### Implementation Details/Notes/Constraints [optional]

To be decided.

### Risks and Mitigations

To be evaluated.

## Design Details

We will expose the network capabilities as an `sdn-public` Config Map, writeable only by the SDN,
readable by any `system:authenticated` user.

The [Cluster Network Operator](https://github.com/openshift/cluster-network-operator) will implement
the logic for exposing the network capabilities.

After the data is accessible, we will modify the `useClusterNetworkFeatures` function
from the console to load the network features from the previously created config map.

### Open Questions [optional]

N/A

### Test Plan

Unit tests to validate the exposed map values according to the known features of the diverse
SDN types.

Automated tests for the UI side: check that, according to the selected CNI type, the Network Policy
creation form will selectively show/hide sections that are supported/unsupported by the CNI.

Integration tests: install the cluster network operator with different CNI types and verify
that the values of the exposed config maps vary.

QE manual tests to validate the final implementation.

### Graduation Criteria

To be decided.

#### Dev Preview -> Tech Preview

#### Tech Preview -> GA

#### Removing a deprecated feature

### Upgrade / Downgrade Strategy

### Version Skew Strategy

As long as this feature depends on a stable and documented Openshift API feature,
there are no dependencies that might cause such version skew.

However, as long as we require new features for future versions of the Console,
it might happen that an outdated "features provider" may not provide information about
all those _latest features_.

In that case, we may interpret the `string` -> `boolean` features map the following
way. For each feature `F` in the `features` map:

* If `features[F] === true`, then CNI type supports the feature.
* If `features[F] === false`, then the CNI type does not support the feature.
* If `features[F] === undefined`, then the feature' provider does not know
  about `C` and the console will take a default action (show it, hide it, show it with a warning message...)

## Implementation History

N/A

## Drawbacks

N/A

## Alternatives

* [Access the K8s API directly from the console](https://github.com/mariomac/console/pull/1).
  - Drawback 1: this feature would be available only to administrator users.
  - Drawback 2: probably, the console frontend is not the best place to implement the feature's logic.
  
* Expose the feature from Console backend.
  - Drawback 1: it would require adding new permissions to the console.
  - Drawback 2: probably, the console backend _bridge_ is not the best place to implement the feature's logic.

## Infrastructure Needed [optional]

N/A
