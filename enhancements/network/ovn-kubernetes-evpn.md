---
title: ovn-kubernetes-evpn
authors:
  - "@jcaamano"
reviewers:
  - "@anuragthehatter"
  - "@jechen0648"
  - "@kyrtapz"
  - "@maiqueb"
  - "@mattedallo"
  - "@pperiyasamy"
  - "@tssurya"
approvers:
  - "@tssurya"
api-approvers:
  - None
creation-date: 2025-10-14
last-updated: 2025-10-24
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
circumstances of the inadequacy and the best context to resolve it:
* Amend the upstream enhancement if still open.
* Work on a follow up upstream enhancement while keeping this one open.
* Work on a new downstream enhancement that either replaces or follows this one. 

## Motivation

The motivation for this feature is aligned to the one described in the upstream
[enhancement][2].

### User Stories

The user stories for this feature are aligned to the ones described in the
upstream [enhancement][3].

### Goals

The goals for this feature are aligned to the ones described in the upstream
[enhancement][4].

### Non-Goals

The non-goals for this feature are aligned to the ones described in the upstream
[enhancement][5].

## Proposal

Ths section requires a general understanding of the overall proposal described
in the upstream[enhancement][6].

The EVPN feature is mainly driven by the already existing OVN-Kubernetes
RouteAdvertisements CRD that has no proposed changes. The `routeAdvertisements`
configuration flag in the OVN-Kubernetes CNO configuration will need to be set
to `Enabled` to be able to use the feature. Additionally, the EVPN feature
requires FRR-k8s. FRR-k8s is deployed by CNO when the
`additionalRoutingCapabilities` `providers` includes `FRR` in the operator
configuration, which is required to enable `routeAdvertisements`.

The EVPN feature is only supported when `routingViaHost` is set to `true`, also
known as local gateway mode. CNO will perform no validation, OVN-Kubernetes will
reject invalid configurations in this regard reporting it through the status of
the appropriate resources.

The EVPN feature impacts FRR-k8s and OVN-Kubernetes specific APIs in the form of
new and updated CRDs. A new feature gate will be introduced for the EVPN feature
and CNO will deploy these CRD updates only if the feature gate is enabled,
making it impossible to use the feature if the feature gate is not enabled.
Worth noting that FRR-k8s APIs allow the possibility to inject raw FRR
configuration and that this capability might be used during the development
process until proper structured APIs are introduced.

Downstream FRR-k8s image consumes the FRR version from a [RHEL 9.6 base
image][7] which currently provides FRR v8. The EVPN feature requires FRR v9+
which we assume available by the time this feature becomes GA. In the meantime,
during development and tech preview, it is feasible to use a workflow where the
downstream image is replaced with an upstream one of the appropriate version.

### Workflow Description

In no particular order, a cluster administrator enables FRR and
RouteAdvertisements, and the EVPN feature gate if not available through the
default feature set:

```shell
oc patch featuregate cluster --type=merge -p='{"spec"{"featureSet":"TechPreviewNoUpgrade"}'
...
oc patch Network.operator.openshift.io cluster --type=merge -p='{"spec":{"additionalRoutingCapabilities": {"providers": ["FRR"]}, "defaultNetwork":{"ovnKubernetesConfig":{"routeAdvertisements":"Enabled"}}}}'
```

During development or tech preview, replace the FRR version with an upstream one
that can support the EVPN feature:

```shell
oc patch Network.operator.openshift.io cluster --type=merge -p='{"spec":{"managementState": "Unmanaged"}}'
...
oc set image -n openshift-frr-k8s ds/frr-k8s frr=quay.io/frrouting/frr:10.4.1 reloader=quay.io/frrouting/frr:10.4.1
```

Then, a cluster administrator proceeds with the OVN-Kubernetes [workflow][8] to
configure EVPN. What follows is an example of that workflow in no particular
order.

* Configure FRR-K8s defining the routers to establish the underlay BGP peering
   and the routers that correspond to the desired IP-VRFs:

```yaml
apiVersion: frrk8s.metallb.io/v1beta1
kind: FRRConfiguration
metadata:
  name: evpn
  namespace: openshift-frr-k8s
  labels:
    advertise: evpn
spec:
  bgp:
    routers:
    - asn: 64512
      neighbors:
      - address: 172.18.0.5
        asn: 64513
```

* Configure a VTEP:

```yaml
apiVersion: k8s.ovn.org/v1
kind: VTEP
metadata:
  name: evpn-vtep
spec:
  cidr: 100.64.0.0/24
  mode: managed
```

* Configure RouteAdvertisements targeting the reference FRRConfiguration to use
   and the CUDNs to be advertised in an EVPN configuration:

```yaml
apiVersion: k8s.ovn.org/v1
kind: RouteAdvertisements
metadata:
  name: advertise-cudns-evpn
spec:
  targetVRF: auto
  advertisements:
  - PodNetwork
  networkSelectors:
  - networkSelectionType: ClusterUserDefinedNetworks
    clusterUserDefinedNetworkSelector:
      networkSelector:
        matchLabels:
          advertise: evpn
  frrConfigurationSelector:
    matchLabels:
      advertise: evpn
  nodeSelector: {}
```

* Configure the L2 and/or L3 CUDNs with EVPN

```yaml
apiVersion: k8s.ovn.org/v1
kind: ClusterUserDefinedNetwork
metadata:
  name: l3-cudn
  labels:
    advertise: evpn
spec:
  namespaceSelector:
    matchLabels:
      kubernetes.io/metadata.name: l3-cudn
  topology: Layer3
  layer3:
    role: Primary
    subnets:
      - cidr: "22.100.0.0/16"
        hostSubnet: 24
  transport: EVPN
  evpnConfiguration:
    vtep: evpn-vtep
    ipVRF:
      vni: 100
---
apiVersion: k8s.ovn.org/v1
kind: ClusterUserDefinedNetwork
metadata:
  name: l2-cudn
  labels:
    advertise: evpn
spec:
  namespaceSelector:
    matchLabels:
      kubernetes.io/metadata.name: l2-cudn
  network:
    topology: Layer2
    layer2:
      role: Primary
      subnets:
      - "22.100.0.0/16"
  transport: EVPN
  evpnConfiguration:
    vtep: evpn-vtep
    macVRF:
      vni: 111
    ipVRF:
      vni: 101
```

### API Extensions

There are no changes required to OCP specific APIs.


### Topology Considerations

#### Hypershift / Hosted Control Planes

There is no specific technical reason by which Hosted Control Planes couldn't be
supported. However this enhancement does not consider any testing on such
platforms and does not claim support on them.

#### Standalone Clusters

No special considerations for standalone clusters.

#### Single-node Deployments or MicroShift

There is no specific technical reason by which Single-node Deployments or
MicroShift couldn't be supported. However this enhancement does not consider any
testing on such platforms and does not claim support on them.

#### OpenShift Kubernetes Engine

No special considerations for OpenShift Kubernetes Engine.

### Implementation Details/Notes/Constraints

As a recap, these are the changes proposed by this enhancement:

* Introduce all relevant updates of OVN-Kubernetes and FRR-k8s APIs.
* Introduce a CNO feature gate for the EVPN feature that will be required to
  deploy the specific changes of the APIs to configure EVPN.
* New OVN-Kubernetes CRDs introduced by this feature will only be deployed if
  both EVPN feature gate and RouteAdvertisements are enabled.
* Updates to existing OVN-Kubernetes and FRR-k8s CRDs introduced by this feature
  will only be deployed if the EVPN feature gate is enabled.

### Risks and Mitigations

Development of this feature is subject to the availability of FRR v9+. The
problem and possible mitigations have been described in the Proposal section of
this enhancement.

Otherwise, the risk and mitigations for this feature are aligned to the ones
described in the upstream [enhancement][9].

### Drawbacks

The drawbacks of this feature are aligned to the ones described in the upstream
[enhancement][9].

As previously mentioned, the EVPN feature is only supported when
`routingViaHost` is set to `true`, also known as local gateway mode. Support for
shared gateway mode configuration will be introduced in a future enhancement.

Also worth noting that the EVPN feature is not compatible with other
OVN-Kubernetes features like EgressIP, IPsec or No-Overlay. The compatibility
matrix is described with more detail in the upstream [enhancement][10]. Further
enhancements will be required to resolve these compatibility issues as needed.

Among those limitations is the lack of support to advertise service IPs. MetalLB
would be the primary candidate to support this functionality and while it
supports unicast advertisements, it does not currently support the EVPN address
family. Thus, the advertisement of load balancer IPs on EVPN must be configured
manually until support is added to MetalLB. 

## Alternatives (Not Implemented)

The alternatives of this feature are aligned to the ones described in the
upstream [enhancement][11].

## Open Questions [optional]

N/A

## Test Plan

### E2E tests

There already exists a dual stack CI lane for BGP in local gateway mode,
`e2e-metal-ipi-ovn-dualstack-bgp-local-gw`, mostly defined with the appropriate
configuration to run EVPN test cases. The job should be modified to:
* Enable the EVPN feature gate.
* Enable the internal BGP fabric (introduced in the no-overlay
  [enhancement][12]) if tests want to leverage it.

[OpenShift Testing Extensions (OTE)][13] will be used for the implementation,
along with the appropriate infrastructure provider, enabling upstream test cases
to be reused downstream while maintaining equivalent coverage. This coverage is
expected to match:
* the existing coverage for P-CUDNs when configured normally, except for
  features explicitly identified as unsupported in the EVPN upstream
  [enhancement][10].
* testing both strict vs loose [OVN-Kubernetes BGP isolation modes][18]: in an
  scenario when routes are leaked across two CUDNs, loose mode will allow the
  interconnect between the two while strict mode will prevent it
* An scenario where IPv4 overlay is advertised on an IPv6 underlay, and
  vice versa.

### QE Testing

In a similar vein to E2E tests, QE coverage should constitute a regression of
the existing P-CUDN coverage under a EVPN configuration except for those features
explicitly called out as not supported in the EVPN upstream [enhancement][13].

Additionally, QE testing should include:
* An scenario where the EVPN underlay is carried over a secondary
  interface instead of br-ex.
* Combined BFD and EVPN configuration testing route updates when BFD reports as
  broken link.
* Testing upgrades from a cluster already making use of the EVPN feature.
* Day-0 EVPN configuration.
* MetalLB integration: test the reachability of load balancer IPs allocated by
  MetalLB when manually configured to be advertised through EVPN.
* Kubevirt integration: test VM migration.
* Stand alone usage of FRR-K8s EVPN features.

QE testing will need to emulate a BGP/EVPN fabric. While any kind of custom and
simplified setup is acceptable, there might be interest in using third party
projects like [containerlab][14] which facilitate the orchestration of an
emulated fabric with several different implementations, including FRR.

### Performance & Scale Testing

Performance & Scale Testing should be conducted for the OVN-Kubernetes control
plane with EVPN configurations as it is already conducted today with unicast BGP
configurations.

EVPN uses VxLAN on the dataplane instead of Geneve. Throughput testing with
VxLAN needs to be considered if not already performed.

## Graduation Criteria

The EVPN feature is planned to be provided with technical preview availability
first and then with general availability in a later release. While the main
development effort will take place in context of upstream projects, the
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
- User facing documentation available in [openshift-docs][15]

### Removing a deprecated feature

NA

## Upgrade / Downgrade Strategy

This feature has no impacts on upgradeability.

EVPN can only be enabled on newly defined CUDNs. EVPN cannot be enabled on
existing CUDNs as their specification is immutable by design.

## Version Skew Strategy

N/A

## Operational Aspects of API Extensions

N/A

## Support Procedures

In general, support procedures will be based on the status reported on the
resource instances involved in the EVPN feature which are similar to those
already involved in the existing BGP features, including the status for
`RouteAdvertisements`, `ClusterUserDefinedNetwork` and FRR-k8s resources: any
invalid configuration shall be reported through appropriate status conditions
and events associated to those resources.

Conditions on the Status of the impacted CRDs should have metrics associated,
allowing the configuration of alerts based on those metrics. FRR-k8s is expected
to provide additional metrics equivalent to those already provided for the
[unicast address family][16]

Other than that, and given the distributed nature of OVN-Kubernetes, the next
best troubleshooting method relies on the use tools like `iproute2`, `tcpdump`,
`ovn-trace`, `ovs-trace` and `vtysh`, mixing existing knowledge with additional
understanding of FRR and host configuration specific to EVPN that is detailed in
the upstream [enhancement][17]

In the future, NetObserv might introduce insights into that host configuration
and facilitate troubleshooting.

The existing CUDN telemetry should further break down CUDNs by transport.

## Infrastructure Needed [optional]

The EVPN feature is only supported for baremetal platforms.

[1]: https://github.com/ovn-kubernetes/ovn-kubernetes/pull/5089
[2]: https://github.com/ovn-kubernetes/ovn-kubernetes/blob/4568650412f85cdcf7671c74d2e9b3bb67d3b366/docs/okeps/okep-5088-evpn.md#introduction
[3]: https://github.com/ovn-kubernetes/ovn-kubernetes/blob/4568650412f85cdcf7671c74d2e9b3bb67d3b366/docs/okeps/okep-5088-evpn.md#user-storiesuse-cases
[4]: https://github.com/ovn-kubernetes/ovn-kubernetes/blob/4568650412f85cdcf7671c74d2e9b3bb67d3b366/docs/okeps/okep-5088-evpn.md#goals
[5]: https://github.com/ovn-kubernetes/ovn-kubernetes/blob/4568650412f85cdcf7671c74d2e9b3bb67d3b366/docs/okeps/okep-5088-evpn.md#non-goals
[6]: https://github.com/ovn-kubernetes/ovn-kubernetes/blob/4568650412f85cdcf7671c74d2e9b3bb67d3b366/docs/okeps/okep-5088-evpn.md#proposed-solution
[7]: https://docs.google.com/spreadsheets/d/1OPj8bm8FB0B4GgVhYSQwC_r5pjIgKtO2QfTUAhIKlFg
[8]: https://github.com/ovn-kubernetes/ovn-kubernetes/blob/4568650412f85cdcf7671c74d2e9b3bb67d3b366/docs/okeps/okep-5088-evpn.md#workflow-description
[9]: https://github.com/ovn-kubernetes/ovn-kubernetes/blob/4568650412f85cdcf7671c74d2e9b3bb67d3b366/docs/okeps/okep-5088-evpn.md#risks-known-limitations-and-mitigations
[10]: https://github.com/ovn-kubernetes/ovn-kubernetes/blob/master/docs/okeps/okep-5088-evpn.md#feature-compatibility
[11]: https://github.com/ovn-kubernetes/ovn-kubernetes/blob/4568650412f85cdcf7671c74d2e9b3bb67d3b366/docs/okeps/okep-5088-evpn.md#alternatives
[12]: https://github.com/ovn-kubernetes/ovn-kubernetes/blob/6a4a58afadf53d4fa11f2ac6ccf8bad558470265/docs/okeps/okep-5259-no-overlay.md
[13]: https://github.com/openshift-eng/openshift-tests-extension
[14]: https://github.com/srl-labs/containerlab
[15]: https://github.com/openshift/openshift-docs/
[16]: https://github.com/openshift/frr/blob/release-4.22/frr-tools/metrics/collector/metrics.go
[17]: https://github.com/ovn-kubernetes/ovn-kubernetes/blob/4568650412f85cdcf7671c74d2e9b3bb67d3b366/docs/okeps/okep-5088-evpn.md#implementation-details
[18]: https://github.com/ovn-kubernetes/ovn-kubernetes/blob/master/go-controller/pkg/config/config.go#L1174-L1175
