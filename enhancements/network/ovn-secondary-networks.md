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
- IPv6 support is welcome (not an immediate priority)
- optional connection to the external networks

### Non-Goals
- network policy concerns. There isn't a requirement for having network
  policies for the secondary networks. There is an
  [enhancement proposal](enhancements/network/multi-networkpolicy.md) about
  this subject, not scoped around OVN.
- ingress into the OVN secondary networks. This is not a current objective,
  and should be treated in a separate enhancement proposal, which would depend
  on the [services on secondary networks feature](TODO: add link to PR) availability.

## Proposal

### User Stories

#### Isolated tenant network
A customer can specify a layer 2 overlay network with a subnet via a
`network-attachment-definition`, which is provisioned without the intervention
of a system administrator. You can refer to the
[network attachment definition section](#net-attach-def-samples) for examples.

Afterwards, the users can then request their pods - and / VMs - to connect to
those overlays as they would for any other secondary network.

### Implementation Details/Notes/Constraints
A new CRD will be introduced, named `OVNSecondaryNetwork`. When this CRD is
provisioned, a `NetworkAttachmentDefinition` object will be rendered.

The new CRD will look like:

```golang
type OVNSecondaryNetwork struct {
    metav1.TypeMeta   `json:",inline"`
    metav1.ObjectMeta `json:"metadata,omitempty"`

    Spec OVNSecondaryNetworkSpec `json:"spec,omitempty"`
}

type OVNSecondaryNetworkSpec struct {
    // Subnet is a RFC 4632/4291-style string that represents an IP address and prefix length in CIDR notation
    Subnet                  string `json:"subnet"`
    HasExternalConnectivity bool   `json:"hasExternalConnectivity,omitempty"`
    MTU                     uint16 `json:"mtu,omitempty"`
}
```

A controller will watch out for the creation / deletion of these
`OVNSecondaryNetwork`s, and will render and provision the corresponding
`NetworkAttachmentDefinition`s.

**Note**: the subnet attribute requires some validation, especially *if* the
subnet has external connectivity: we need to assure the uniqueness of the
subnet on these overlay networks, and when the provisioned overlay has external
connectivity, we need to furthermore assure that its range does not clash with
the node CIDRs, cluster CIDR, and service CIDR. An error will be thrown when
the provided range clashes with the aforementioned CIDRs.

#### External connectivity considerations
When an overlay network with external connectivity is requested, the traffic
will be directly dumped onto the host's underlay via a localnet port.

#### Network-attachment-definition provisioning
A removed `Network-Attachment-definition` with present attachment will
trigger a reconciliation loop, which will cause the net-attach-def entity to be
reprovisioned; if the network attachment definition is removed and the
corresponding OVN logical switch does not have any logical ports attached
triggers the logical switch to be deleted.

#### Adding / removing a pod using an OVN secondary network
When a pod connected to an overlay network is scheduled, the logical switch
that will implement the layer 2 network will be (lazily) created if it does not
already exist. The logical switch name will be
`secondary_<namespace>_<networkName>`, and it will assign IP addresses in the
configured range (defined in the `OverlayNetwork::Subnet` attribute).

A logical switch port will also be created (or removed), on the corresponding
logical switch whenever a pod is scheduled (or removed).

### Requesting static IP / MAC
A static MAC / IP address can be requested for the pods by specifying those in
the annotations field. The requested IP address **must** be in range of the
corresponding `OVNSecondaryNetwork`, otherwise the pod creation will fail.

The usage of static MAC / IPs must be validated: once an IP is requested, we
must make sure it is not in use within any logical port connected to that
logical switch. If it is, we fail the pod creation with an error.
OVN is smart enough to not assign via DHCP addresses that were previously
statically configured on a logical switch port connected to the switch.

Static MAC and IP addresses can be injected into each pod via
[runtime config](https://github.com/containernetworking/cni/blob/master/CONVENTIONS.md#dynamic-plugin-specific-fields-capabilities--runtime-configuration)
; the user would use the static IPAM plugin to provide the desired IP / MAC
address to the pod. Other IPAM plugins could be used, but it wouldn't make
much sense - the users should omit the IPAM section, and rely instead on OVN's
DHCP to get addresses from. An argument could even be made to disallow the
usage of any IPAM plugin other than the `static` one.

Check the example below for examples on how to configure a static MAC and IP
address for an imaginary pod.

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: samplepod
  annotations:
    k8s.v1.cni.cncf.io/networks: '[
      {
        "name": "tenantA-overlay",
        "ips": [ "192.168.2.205/24" ],
        "mac": "CA:FE:C0:FF:EE:00"
      }
    ]'
...
```

The rendered `network-attachment-definition` to allow both IP / MAC to be set
for these networks would have to look something like:

```yaml
apiVersion: "k8s.cni.cncf.io/v1"
kind: NetworkAttachmentDefinition
metadata:
  name: tenantA-overlay
spec:
  config: '{
      "cniVersion": "0.3.1",
      "plugins": [{
          "type": "ovn-k8s-cni-overlay",
          "capabilities": { "ips": true },
          "subnet": "10.10.10.0/24",
          "ipam": {
            "type": "static"
          }
        }, {
          "capabilities": { "mac": true },
          "type": "tuning"
        }]
    }'
```

### Risks and Mitigations
There is [ongoing upstream work](https://github.com/ovn-org/ovn-kubernetes/pull/2301)
by NVIDIA that extends OVN-Kubernetes and implements the required use-cases.
More information about their proposal can be found in this
[slide deck](https://docs.google.com/presentation/d/1bUtdNF--ydHukw4dwBDrvHv6Jh0NwOrN2JjfSzJNgq4/edit#slide=id.p).

Their proposal implements the following topologies for OVN secondary networks:
  - routed L3 networks (same as OVN-Kubernetes primary network)
  - flat L2 networks (our requirement)
  - flat L2 networks with external network connectivity (our requirement)

As such, there is the risk that the upstream community decides to accept
NVIDIA's contribution but Red Hat decides to ship this feature via a dedicated
CNI plugin, as described in the [alternatives section](#alternatives).

## Design Details

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
            "hasExternalConnectivity": true,
        }'
```

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

### Test Plan
The first layer of OVN secondary networks will happen on the upstream repo
`ovn-kubernetes` e2e tests, ran using kind.

We will additionally going to use baremetal CI job to assert the feature works
as intended.

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

#### Removing a deprecated feature

### Upgrade / Downgrade Strategy
Secondary OVN networks just uses the `net-attach-def` CRD and do not rely on
any Kubernetes object is used, hence we could say that it is not kubernetes
version sensitive.

### Version Skew Strategy

## Implementation History

## Drawbacks
We are proposing implementing secondary OVN networks in the `ovn-kubernetes`
repo. This will potentially cause the downstream test matrix around
ovn-kubernetes to bloat.

## Alternatives
This enhancement can also be implemented as a dedicated CNI plugin + controller.

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

## Infrastructure Needed
OpenShift 4.x cluster

