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

### Where to host the code
This enhancement can happen in two ways:
- introduce a new dedicated CNI plugin + controller
- add a sort of plugin into OVN-Kubernetes allowing flat layer 2 networks to
  be created

There are pros and cons to each of the options, which will be captured below:
- new cni plugin
  - pros
    - independent release cycle / development / review process / test matrix
    - clear scopes of responsibility (who maintains what)
    - re-use the ovn-k deployed components
  - cons
    - support nightmare (which team handles a customer case)
      - co-existance of the DBs
    - duplication of effort (external network connectivity, etc)
- ovn-kubernetes
  - pros
    - unified delivery / deployment
    - connectivity to external networks implemented for free
    - NVIDIA plans to contribute these topologies (our use cases are the
      first two):
      - flat L2
      - flat L2 w/ external network connectivity
      - routed L3 (one logical switch per node, inter-connected by a logical
        router)
  - cons
    - bloating the test matrix of ovn-kubernetes

Independently of where the code will live, there are things we can define as
of now:
- a logical switch per network. We will define an external id "owners" field
  so other ovn related projects can keep away from our stuff (e.g. avoid ovn-k
  reconciliation loops)
  - for IPv6 subnets we would need to also create a logical router and
    logical router port, to be able to send the RAs. Would we want
    these routers for other scenarios ? I.e. is there any reason to create
    them for IPv4 subnets ?
- MTU propagation via dhcp options (ipv4)
- a controller, which will watch out for `net-attach-def` and create the
  logical switch encoding the network. It must also reconcile the logical
  switch ports, which would be left behind when nodes are deleted
- active port security on the logical switch ports(preventing both MAC and
  IP spoofing).

### Impacts in multus
No changes are required in multus, since the feature would only consume the
`network-attachment-definition` CRD and does not change it.

### Limitations

### Net-attach-def samples
This is an example of a network attachment definition that will create an
isolated flat L2 network.

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
TODO

### Graduation Criteria

#### Dev Preview
- defined user stories validated by the upstream project CI
- minor documentation (how to use & contribute / examples / roadmap)

#### Dev Preview -> Tech Preview
- end user documentation, relative API stability
- sufficient test coverage

#### Tech Preview -> GA
- integrated with baremetal OCP CI
- upgrade & perf testing
- sufficient time for feedback

## Infrastructure Needed
OpenShift 4.x cluster

