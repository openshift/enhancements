---
title: vrf-metaplugin-cni
authors:
  - "@fedepaol"
reviewers:
  - "@dougbtv"
  - "@zshi-redhat"
approvers:
  - TBD
creation-date: 2020-07-21
last-updated: 2020-07-21
status: implementable
---

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [x] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

To allow isolation, and make it possible to have overlaps between the CIDRs of secondary networks (and between them and the pod's address space), we want to introduce a VRF meta cni plugin. Taking advantage of the chaining mechanism, it can be used to assign a secondary interface to a custom [VRF](https://access.redhat.com/documentation/en-us/red_hat_enterprise_linux/8/html/configuring_and_managing_networking/configuring-virtual-routing-and-forwarding-vrf_configuring-and-managing-networking).

## Motivation

Telco customers maintain strict separation of multiple networks for different services or functionalities. When a device connects to multiple networks, the practice is to maintain strict separation of these networks.

Moreover, the multiple networks a pod is connected to may have overlapping CIDRs, and those CIDRs can also overlap with the sdn’s CIDR. To maintain this kind of isolation, they typically use VRFs. Currently this is done by granting the containers the NET_ADMIN capabilities.

### Goals

Being able to assign a secondary interface to a particular vrf, and eventually having more secondary interfaces of the same pod assigned to the same VRF.

### Non-Goals

Assigning host level interfaces to VRFs.

## Proposal

The proposal is to introduce an additional CNI plugin that takes the vrf name as an input, and can be chained to the “main” cni used for the secondary interface, and that has the effect of creating the requested vrf (if it does not exist) and to add the interface to that vrf.

In addition to the name of the vrf, an optional routing table id can be added, with the effect of setting the routing table used by the VRF.

In this way, the network interface returned by any CNI can be isolated in its own VRF, satisfying the above requirements without the need of additional capabilities.

### User Stories [optional]

#### Story 1

As a user, I want to be able to optionally specify a VRF which the secondary nic is attached to.

### Implementation Details/Notes/Constraints

By adding several networks with the same `vrf` name, the CNI will ensure that they are all added to the same vrf, creating it only the first time.

A `NetworkAttachmentDefinition` for MacVlan would look like:

```yaml
apiVersion: "k8s.cni.cncf.io/v1"
kind: NetworkAttachmentDefinition
metadata:
  name: macvlan-vrf-conf
spec:
  config: '{
              "cniVersion": "0.3.1",
              "plugins": [
                {
                  "type": "macvlan",
                  "mode": "bridge",
                  "ipam": {
                    "type": "host-local",
                    "subnet": "192.168.1.0/24",
                    "rangeStart": "192.168.1.200",
                    "rangeEnd": "192.168.1.216",
                  }
                },
                {
                  "type": "vrf",
                  "vrfname": "blue"
                }
              ]
            }'
```

The same example for a SR-IOV device, enforcing the routing table id, would look like:

```yaml
apiVersion: "k8s.cni.cncf.io/v1"
kind: NetworkAttachmentDefinition
metadata:
  name: sriov-vrf-conf
  annotations:
    k8s.v1.cni.cncf.io/resourceName: openshift.io/resource_id
spec:
  config: '{
              "cniVersion": "0.3.1",
              "plugins": [
                {
                  "type": "sriov",
                  "ipam": {
                    "type": "host-local",
                    "subnet": "192.168.1.0/24",
                    "rangeStart": "192.168.1.200",
                    "rangeEnd": "192.168.1.216",
                  }
                },
                {
                  "type": "vrf",
                  "vrfname": "blue",
                  "table": 1001
                }
              ]
            }'
```

This can be supported at `NetworkAttachmentDefinition` level, or by documenting (and validating) chained CNIs in the `Network` CRD of `operator.openshift.io/v1`.

At the same time, the `SriovNetwork` type from the SR-IOV operator needs to be extended for chaining meta CNIs.

The CNIs we intend to support are SR-IOV and MacVlan.

### Risks and Mitigations

## Design Details

This would be shipped as a new CNI plugin, ideally being part of the container networking plugins [repo](https://github.com/containernetworking/plugins),
which is then consumed in our [openshift variant](https://github.com/openshift/containernetworking-plugins). If not feasible, other alternatives
may be considered for shipping it.

The CNI implementation is pretty simple: it takes the result of the main CNI, creates the VRF in the pod's namespace and assignes the interface to the VRF.

If the VRF already exists, it only adds the interface. When removing an interface, it does the same, checking if the interface is the last removed from the
VRF and eventually delete the VRF.

An optional routing table id can be added. If a VRF with the given name but with a different routing table already exists in the namespace of the pod, the CNI will return an error.

If the routing table is not specified, the CNI will choose a new one, making sure that it is different from those already used by other VRFs inside the pod's namespace.

### Test Plan

Apart from local unit tests, e2e tests will be added in the [cnf-features-deploy](https://github.com/openshift-kni/cnf-features-deploy) e2e integration test suite.
Basic tests can be added under openshift/origin test suite too.

### Graduation Criteria

##### Dev Preview -> Tech Preview

- Ability to utilize the enhancement end to end (at least, with bare `Network Attachment Definition`)
- End user documentation, relative API stability
- Sufficient test coverage

##### Tech Preview -> GA

- More testing (e2e)
- Sufficient time for feedback
- Available by default
- Available as part of OpenShift Network / SR-IOV

## Implementation History

## Alternatives

When granted enough privileges, the application is able to set the VRF by itself.
