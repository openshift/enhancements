---
title: ovn-secondary-networks
authors:
  - "@maiqueb"
reviewers:
  - "@dcbw"
  - "@dougbtv"
  - "@squeed"
  - "@trozet"
  - "@abhat"
  - "@alexanderConstantinescu"
  - TBD
approvers:
  - TBD
creation-date: 2021-06-15
last-updated: yyyy-mm-dd
status: provisional
---

# OVN secondary networks

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary
This enhacement proposal studies and describes how to support OVN as the
networking provider for a Multus secondary interface.

## Motivation
The networking plugins that can be used as secondary interfaces are currently
quite simple, and are unable to provide advanced networking requirements.

Furthermore, virtualization users are used to being able to define isolated
tenant networks that form a single L2 domain. OVN excels at this, by creating
simple L2 overlays without the need for a network administrator.

### Goals
- create flat layer 2 overlays
- connect a newly created pod to these overlay networks
- these overlays can optionally be a subnet
- optional connection to the external networks

### Non-Goals
- network policy concerns

## Proposal

### User Stories

#### Isolated tenant network
A customer can specify a layer 2 overlay network with a subnet via a
`network-attachment-definition`, which is provisioned without the intervention
of a system administrator.

Afterwards, the users can then request their pods - and / VMs - to connect to
those overlays as they would for any other secondary network.

### Implementation Details/Notes/Constraints

## Design Details
TODO

### Impacts in multus
No changes are required in multus, since the feature would only consume the
`network-attachment-definition` CRD and does not change it.

### Net-attach-def samples

```yaml
apiVersion: "k8s.cni.cncf.io/v1"
kind: NetworkAttachmentDefinition
metadata:
  name: tenant-network-1
  namespace: testns1
spec:
  config: '{
            "cniVersion": "0.3.1",
            "type": "ovn-kubernetes",
            "name": "tenant-overlay",
            "subnet": "10.10.10.0/24",
        }'
```

This is an example of a network attachment definition that will create a flat
L2 network connected to the external network.
```yaml
apiVersion: "k8s.cni.cncf.io/v1"
kind: NetworkAttachmentDefinition
metadata:
  name: tenant-network-2
  namespace: testns1
spec:
  config: '{
            "cniVersion": "0.3.1",
            "type": "ovn-kubernetes",
            "name": "tenant-overlay-ext-network",
            "subnet": "10.10.20.0/24",
            "has_external_connectivity": true,
        }'
```

### Test Plan

### Graduation Criteria

#### Dev Preview

#### Dev Preview -> Tech Preview

#### Tech Preview -> GA

### Upgrade / Downgrade Strategy

### Version Skew Strategy

## Implementation History

## Drawbacks

## Alternatives

## Infrastructure Needed
OpenShift 4.x cluster

