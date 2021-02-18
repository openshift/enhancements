---
title: ovn-kubernetes-ipsec
authors:
  - "@mdgray"
reviewers:
  - TBD
  - "@dcbw"
approvers:
  - TBD
creation-date: 2020-09-08
last-updated: 2021-02-18
status: implemented
see-also:
  - "/enhancements/network/20190919-OVN-Kubernetes.md"
replaces:
  - N/A
superseded-by:
  - N/A
---

# OVN Kubernetes IPsec Enablement

## Release Signoff Checklist

- [X] Enhancement is `implementable`
- [X] Design details are appropriately documented from clear requirements
- [X] Test plan is defined
- [X] Graduation criteria for dev preview, tech preview, GA
- [X] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

The OVN Kubernetes network plugin uses [OVN](https://www.ovn.org) to instantiate
virtual networks for Kubernetes. These virtual networks use Geneve, a network
virtualization overlay encapsulation protocol, to tunnel traffic across the
underlay network between Kubernetes Nodes.

IPsec is a protocol suite that enables secure network communications between
IP endpoints by providing the following services:
- Confidentiality
- Authentication
- Data Integrity

OVN tunnel traffic is transported by physical routers and switches. These
physical devices could be untrusted (devices in public network) or might be
compromised. Enabling IPsec encryption for this tunnel traffic can prevent the
traffic data from being monitored and manipulated.

The scope of this work is to encrypt all traffic between pods on the cluster
network when that traffic leaves the node (and correspondingly decrypt traffic
that enters the node) using IPsec. It will *not* encrypt traffic between pods on
the host network, as this traffic does not traverse OVN. However, as a side
effect, this will also encrypt pod traffic that originates from pods on the host
network and is destined for the cluster network as this network does traverse
OVN. In order to simplify terminology, this encrypted traffic will be referred
to as inter-node traffic throughout the document.

## Motivation

Encryption services are recommended when traffic is traversing an untrusted
network. Encryption services may also be required for regulatory or compliance
reasons (e.g. FIPS compliance).

### Goals

- Provide an option to enable IPsec encryption of all inter-node traffic across
an entire Kubernetes cluster that has been configured with OVN Kubernetes.
  - Assume that all nodes will be provisioned with certificates that have been
  signed by a OpenShift Container Platform internal certificate authority
  (CA).
- This option will only be available at cluster installation time.
- All components used to implement this must be FIPS-compliant cryptographic
components.
- Provide an implementation that will be deployable on both Public and Private
Cloud infrastructure. This may require an option to enable IPsec NAT traversal
techniques.

### Non-Goals

- Will not support mixed clusters of RHEL7 and RHEL8 nodes. This can be
supported in a later enhancement.
- Will not provide an option to enable tunnels that have been configured with
a self-signed certificate as the authenticating certificate. However,
authentication by certificates signed by a self-signed CA will be supported.
- Will not provide an option to enable tunnels that have been configured with
pre-shared keys.
- Will not provide an option to enable encryption after cluster installation.
- Will not do any explicit performance optimization of the datapath. As the
implementation of this enhancement will primarily require changes to the control
and management path, it is expected that this will not alter the performance of
the current OVS/Kernel IPsec implementation.
- Will not provide any interface to allow a cluster operator to specify
preferred encryption, integrity, authentication algorithms. However, we will
ensure that the defaults are sensible. Configuration of this could be targeted
at a later enhancement, if required.
- Will not explicitly encrypt cluster management traffic. Cluster management
traffic has already been encrypted using TLS.
- Will not allow granular configuration of which nodes will have IPsec enabled
(for example Namespace to Namespace encryption) or how IPsec is configured
between specific nodes. This enhancement will enable IPsec on all inter-node
traffic indiscriminately.
- Will not provide IPsec connectivity between nodes on the host network.

## Proposal

The following changes are proposed to the [network.operator.openshift.io API](https://github.com/openshift/api/blob/master/operator/v1/types_network.go). Please note that the change is the
addition of an optional `ipsecConfig` structure in the spec.

Specifying `ipsecConfig`, even if empty, will enable IPsec. This will
future-proof the possibility of adding additional configuration fields
in this structure.

```go
// ovnKubernetesConfig contains the configuration parameters for networks
// using the ovn-kubernetes network project
type OVNKubernetesConfig struct {
	// mtu is the MTU to use for the tunnel interface. This must be 100
	// bytes smaller than the uplink mtu.
	// Default is 1400
	// +kubebuilder:validation:Minimum=0
	// +optional
	MTU *uint32 `json:"mtu,omitempty"`
	// geneve port is the UDP port to be used by geneve encapulation.
	// Default is 6081
	// +kubebuilder:validation:Minimum=1
	// +optional
	GenevePort *uint32 `json:"genevePort,omitempty"`
	// HybridOverlayConfig configures an additional overlay network for peers that are
	// not using OVN.
	// +optional
	HybridOverlayConfig *HybridOverlayConfig `json:"hybridOverlayConfig,omitempty"`
	// ipsecConfig enables and configures IPsec for pods on the pod network within the
	// cluster.
	// +optional
	IPsecConfig *IPsecConfig `json:"ipsecConfig,omitempty"`
}
```

This will result in an updated OVNKubernetes Operator configuration object:
(.spec.defaultNetwork.ovnKubernetesConfig) which will enable cluster-wide
configuration of IPsec at cluster installation time:

```yaml
spec:
  defaultNetwork:
    type: OVNKubernetes
    ovnKubernetesConfig:
      mtu: 1400
      genevePort: 6081
      ipsecConfig: {}
```

### Implementation Details/Notes/Constraints

It is expected that the following changes will need to be made in order to
enable this enhancement:

- OVNKubernetes pod Dockerfiles will need to be updated in order to install
  necessary components required by OVN/OVS to enable IPsec (e.g.
  openvswitch-ipsec, libreswan, etc).
- Modify Cluster Network Operator manifests to configure start up of packages
  required by OVN/OVS in order to enable IPsec (e.g. ovs-monitor-ipsec, pluto,
  etc) and determine how to containerize these components.
- Ensure that all versions of all components are at the correct level required
  by OVN/OVS. This may required back-porting of patches into RHEL. It has
  already been identified that OVS and the Kernel will each require a patch to
  enable this enhancement.
- Ensure correct generation of the Certificate Authority (CA) certificate and
  all node private keys and CA-signed certificates.
- Modify Cluster Network Operator manifests to ensure that all OVS/OVN
  components are configured to use host private key, host certificate and CA
  certificate.
- Modify OVNKubernetes operator configuration object in order to allow
  enablement of IPsec for the cluster.
- Certificate rotation will be handled in the same as it is with the current
  OVN TLS implementation.

### Risks and Mitigations

#### Upstreaming

A number of changes will need to be made to upstream communities including the
Linux kernel. As these may need time to execute, it is imperative that we
identify them early and start the process.

#### Performance

Enablement of IPsec will cause degradation of overall networking throughput and
latency due to the additional overhead of the IPsec protocol. Some early
performance testing will be done in a non-production cluster in order to get
an approximation of the expected performance degradation. These results will be
posted at a later date. After implementation of the enhancement, it is expected
that comprehensive benchmarking and profiling will take place. Based on the
output of this activity, it may be decided to start some optimization activities
in parallel to improve performance.

#### Security Review

## Design Details

### Open Questions

### Test Plan

It will expected to add a number of Continuous Integration jobs for IPsec:

- All OpenShift e2e tests should be replicated in a cluster that has been
  installed with IPsec enabled. This should ensure IPsec has no regressions with
  respect to a cluster that has not been enabled with IPsec.
- In order to test any issues with version skew, an IPsec-enabled cluster should
  be upgraded while active.

Additional CI jobs could potentially be added at a later stage.

- Functional tests
  - Ensure certificate expiration and rotation happens correctly.
- Non-functional tests
  - Performance tests (scale tests)
  - Endurance tests (traffic generation over a long period)
  - Recovery tests (what happens when node IPsec components such as pluto or
    ovs-monitor-ipsec are killed)

It should also be noted that, some upstream OVS/OVN unit and integration tests
will also need to be developed.

### Graduation Criteria

##### Dev Preview -> Tech Preview

- Ability to utilize the enhancement end to end
- End user documentation, relative API stability
- Sufficient test coverage
- Gather feedback from users rather than just developers
- Performance measurement

##### Tech Preview -> GA

- More testing (upgrade, downgrade, scale)
- Sufficient time for feedback
- Available by default

##### Removing a deprecated feature

N/A

### Upgrade / Downgrade Strategy

Currently, it is proposed that this feature can only be enabled at cluster
installation time.

### Version Skew Strategy


However, it will need to support version skew within RHEL8 components. The exact
details of this will be understood, when it is determined how IPsec components
will be containerized but some considerations are noted below:

- Internode libreswan compatibility
  - Each nodes' libreswan version will need to be compatible with other nodes'
    libreswan version
- ovs-monitor-ipsec/libreswan compatibility
  - ovs-monitor-ipsec creates libreswan configuration files at runtime.
    Therefore, the ovs-monitor-ipsec version will need to track libreswan
    version changes.
- ovs-monitor-ipsec/ovsdb compatibility
  - ovs-monitor-ipsec reads ovsdb tables. Therefore, ovs-monitor-ipsec will need
    to track OVN/OVS versions.
- libreswan/kernel compatibility
  - libreswan will need to be compatible with the underlying Kernel IPsec
    implementation.

## Implementation History

TBD

## Drawbacks

N/A

## Alternatives

N/A

## Infrastructure Needed

N/A
