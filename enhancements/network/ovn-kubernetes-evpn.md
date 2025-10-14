---
title: OVN-Kubernetes support for EVPN
authors:
  - @jcaamano
reviewers:
  - @arghosh93
  - @asood-rh
  - @jechen0648
  - @martinkennelly
  - @maiqueb
  - @pperiyasamy
  - @tssurya
  - @zhaozhanqi
approvers:
  - @tssurya
api-approvers:
  - None
creation-date: 2025-10-14
last-updated: 2025-10-15
tracking-link:
  - https://issues.redhat.com/browse/CORENET-6429
see-also:
  - https://github.com/ovn-kubernetes/ovn-kubernetes/pull/5089

---

# OVN-Kubernetes support for EVPN

## Summary

This feature allows exposing primary Cluster User Defined Networks (P-CUDNs)
externally via a VPN to other entities either inside, or outside the cluster;
using BGP and EVPN as the common and native networking standard that will enable
integration into user networks without SDN specific network protocol
integration, and providing an industry standardized way to achieve network
segmentation between sites.

This enhancement is aligned and being worked on in tandem to a corresponding
OVN-Kubernetes upstream [enhancement][1]. As such, there will be references to
it for much of the content. The intention of this enhancement is to outline the
necessary changes to consume and integrate that functionality in OCP, including
the interaction with the Cluster Network Operator (CNO) and our test plan for
this feature. However, in case the upstream enhancement is found to be
inadequate, one of the following outcomes is possible depending on the
circumstances of the inadequacy:
* Amend the upstream enhancement if still open.
* Work on a follow up upstream enhancement while keeping this one open.
* Work on a new downstream enhancement that either replaces or follows this one. 

[1]: https://github.com/ovn-kubernetes/ovn-kubernetes/pull/5089


## Motivation

The motivation for this feature is aligned to the one described in the upstream
[enhancement][2].

[2]: https://github.com/ovn-kubernetes/ovn-kubernetes/blob/8461a5526377488bc643bd4eb7024b0b735e8830/docs/okeps/okep-50088-evpn.md#introduction


### User Stories

The user stories for this feature are aligned to the ones described in the
upstream [enhancement][3].

[3]: https://github.com/ovn-kubernetes/ovn-kubernetes/blob/8461a5526377488bc643bd4eb7024b0b735e8830/docs/okeps/okep-50088-evpn.md#user-storiesuse-cases

### Goals

The goals for this feature are aligned to the ones described in the upstream
[enhancement][4].

[4]: https://github.com/ovn-kubernetes/ovn-kubernetes/blob/8461a5526377488bc643bd4eb7024b0b735e8830/docs/okeps/okep-50088-evpn.md#goals

### Non-Goals

The non-goals for this feature are aligned to the ones described in the upstream
[enhancement][5].

[5]: https://github.com/ovn-kubernetes/ovn-kubernetes/blob/8461a5526377488bc643bd4eb7024b0b735e8830/docs/okeps/okep-50088-evpn.md#non-goals

## Proposal

Ths section requires a general understanding of the overall proposal described
in the upstream[enhancement][6].

[6]: https://github.com/ovn-kubernetes/ovn-kubernetes/blob/8461a5526377488bc643bd4eb7024b0b735e8830/docs/okeps/okep-50088-evpn.md#proposed-solution

The EVPN feature is mainly driven by the already existing OVN-Kubernetes
RouteAdvertisements CRD. The `routeAdvertisements` configuration flag in the
OVN-Kubernetes CNO configuration will need to be set to `Enabled` to be able to
use the feature.

Additionally, the EVPN feature is used through OVN-Kubernetes specific APIs in
the form of new and updated CRDs. A new feature gate will be introduced for the
EVPN feature and CNO will deploy these CRD updates only if the feature gate is
enabled making it impossible to use the feature if the feature gate is not
enabled.

The EVPN feature is only supported when `routingViaHost` is set to `true`, also
known as local gateway mode. CNO will perform no validation, OVN-Kubernetes will
[reject][13] invalid configurations in this regard.

[13]: TODO add reference

The EVPN feature requires FRR-k8s. FRR-k8s is deployed by CNO when the
`additionalRoutingCapabilities` `providers` includes `FRR` in the operator
configuration, which is required to enable `routeAdvertisements`.

The upstream enhancement may introduce changes to the FRR-k8s APIs. These API
changes need not to be gated by the feature gate introduced above. Worth noting
that FRR-k8s APIs allow the possibility to inject raw FRR configuration and that
this capability might be used during the development process until proper
structured APIs are introduced.

Currently, CNO FRR-k8s includes FRR v8 but the EVPN feature needs FRR v9+ so it
will need to be updated. The problem is that FRR-k8s consumes the FRR version
provided by RHEL, specifically RHEL9 which is the current version in use. RHEL9
only provides FRR v8. RHEL10 will provide FRR v10. RHEL10 was to be
[available][7] as tech preview in OCP 4.21 and fully available in OCP 4.22 but
currently its real timeline is uncertain. These are our options in order of
preference:

[7]: https://docs.google.com/spreadsheets/d/1VO00pWkWf8Fr30PHl8mZFTK9ZnJO51BGXH4FT6efwp4/edit?gid=1551125754#gid=1551125754

* Request RHEL to package FRR v10 for RHEL 9.
* If RHEL 10 is available, build a FRR-k8s image against a RHEL10 stream.
* Consume FRR v10 from the FDP (Fast DataPath) project as we currently do with
  OVN and Libreswan until RHEL 10 is generally available for OCP.

A user of the EVPN feature may leverage the internal BGP fabric introduced in
the No-Overlay mode [enhancement][8], particularly for East/West L2 EVPN
configurations. No additional changes with respect to what is described in that
enhancement are required.

[8]: https://github.com/openshift/enhancements/pull/1859


### Workflow Description

In no particular order, a cluster administrator enables FRR and
RouteAdvertisements, and the EVPN feature gate if not available through the
default feature set:

```shell
oc patch featuregate cluster --type=merge -p='{"spec"{"featureSet":"TechPreviewNoUpgrade"}'
...
oc patch Network.operator.openshift.io cluster --type=merge -p='{"spec":{"additionalRoutingCapabilities": {"providers": ["FRR"]}, "defaultNetwork":{"ovnKubernetesConfig":{"routeAdvertisements":"Enabled"}}}}'
```

Then, a cluster administrator enables the internal BGP fabric if intended to be
used:
```shell
TODO
```

Then, a cluster administrator follows the OVN-Kubernetes [workflow][9] to configure EVPN.

[9]: https://github.com/ovn-kubernetes/ovn-kubernetes/blob/8461a5526377488bc643bd4eb7024b0b735e8830/docs/okeps/okep-50088-evpn.md#workflow-description

TODO: add example workflow


### API Extensions

There are no changes required to OCP specific APIs.


### Topology Considerations

#### Hypershift / Hosted Control Planes

No special considerations for hosted clusters.

#### Standalone Clusters

No special considerations for standalone clusters.

#### Single-node Deployments or MicroShift

No special considerations for single-node clusters.

### Implementation Details/Notes/Constraints

As a recap, these are the changes proposed by this enhancement:

* Introduce all relevant updates of OVN-Kubernetes and FRR-k8s APIs.
* Introduce a CNO feature gate for the EVPN feature that will be required to
  deploy the specific bits of the OVN-Kuberentes APIs that implement EVPN.
* Introduce changes to the OCP FRR-k8s build process to consume FRR v10.

### Risks and Mitigations

There is a risk related to the availability of FRR v9+ that has been described
in an earlier section of the enhancement. Not other mitigations proposed other
than the alternatives outline there.

Otherwise, the risk and mitigations for this feature are aligned to the ones
described in the upstream [enhancement][15].

[15]: https://github.com/ovn-kubernetes/ovn-kubernetes/blob/8461a5526377488bc643bd4eb7024b0b735e8830/docs/okeps/okep-50088-evpn.md#risks-known-limitations-and-mitigations

### Drawbacks

The drawbacks of this feature are aligned to the ones described in the upstream
[enhancement][15].

Worth mentioning is the lack of support when `RoutingViaHost` is set to `false`,
also known as shared gateway mode. Support for this configuration will be
introduced in a future enhancement.

## Alternatives (Not Implemented)

The alternatives of this feature are aligned to the ones described in the
upstream [enhancement][16].

[16]: https://github.com/ovn-kubernetes/ovn-kubernetes/blob/8461a5526377488bc643bd4eb7024b0b735e8830/docs/okeps/okep-50088-evpn.md#alternatives

## Open Questions [optional]

N/A


## Test Plan

### E2E tests

There already exists a dual stack CI lane for BGP in local gateway mode,
`e2e-metal-ipi-ovn-dualstack-bgp-local-gw`, mostly defined with the appropriate
configuration to run EVPN test cases. The job should be modified to:
* Enable the EVPN feature gate.
* Enable the internal BGP fabric.

[OpenShift Testing Extensions (OTE)][14] will be used for the implementation
combined with the use of the appropriate infrastructure provider so that
upstream test cases can be used downstream as well. Ideally the coverage should
be the same as the existing coverage for P-CUDNs when configured normally except
for those features explicitly called out as not supported in the EVPN upstream
[enhancement][10].

[10]: TODO add link

[14]: https://github.com/openshift-eng/openshift-tests-extension

To test with the internal BGP fabric it would be enough to run selected
East/West test cases for a L2 EVPN as for the most part the internal BGP fabric
is no different from an externally provided one in terms of data plane.


### QE Testing

In a similar vein to E2E tests, QE coverage should constitute a regression of
the existing P-CUDN coverage under a EVPN configuration except for those features
explicitly called out as not supported in the EVPN upstream [enhancement][10].

QE testing should include testing upgrades from a cluster already making use of
the EVPN feature.

QE testing will need to emulate an BGP/EVPN fabric. While any kind of custom and
simplified setup is acceptable, there might be interest in using third party
projects like [containerlab][11].

[11]: https://github.com/srl-labs/containerlab

## Graduation Criteria

The EVPN feature is planned to be provided with technical preview availability
first and then with general availability in a later release. While the main
development effort will take place in context of the upstream project, the
following graduation criteria includes suggestions to prioritize that
development effort in case it can be accommodated.

### Dev Preview -> Tech Preview

- EVPN support for L2 and L3 P-CUDNs
- Use of raw configuration API in FRR-k8s
- Use of node IPs as VTEP IPs
- Sufficient test coverage (E2E, QE)

### Tech Preview -> GA

- Formal FRR-k8s APIs introduced
- Configurable VTEP IPs
- Complete testing, including upgrades
- User facing documentation available in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Upgrade / Downgrade Strategy

This feature has no impacts on upgradeability.

## Version Skew Strategy

N/A

## Operational Aspects of API Extensions

N/A

## Support Procedures

In general, support procedures will be based on the status reported on the
resource instances involved in the EVPN feature which are similar to those
already involved in the existing BGP features, including the status for
`RouteAdvertisements`, `ClusterUserDefinedNetwork` and FRR-k8s resources: any
invalid configuration shall be reported through appropriate status conditions on
those resources.

Those status conditions should have metrics associated, allowing the
configuration of alerts based on those metrics.

Other than that, and given the distributed nature of OVN-Kubernetes, the next
best troubleshoot method relies on the use tools like `iproute2`, `tcpdump`,
`ovn-trace`, `ovs-trace` and `vtysh`, mixing existing knowledge with additional
understanding of FRR and host configuration specific to EVPN that is detailed in
the upstream [enhancement][12]

[12]: https://github.com/ovn-kubernetes/ovn-kubernetes/blob/8461a5526377488bc643bd4eb7024b0b735e8830/docs/okeps/okep-50088-evpn.md#implementation-details

In the future, NetObserv might introduce insights into that host configuration
and facilitate troubleshooting.  

## Infrastructure Needed [optional]

The EVPN feature is only supported in baremetal platforms.
