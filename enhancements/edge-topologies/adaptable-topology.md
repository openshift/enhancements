---
title: adaptable-topology
authors:
  - "@jaypoulz"
  - "@jeff-roche"
reviewers:
  - "@tjungblu, for cluster-etcd-operator"
  - "@joelanford, for OLM"
  - "@zaneb, for bare metal platform and infrastructure"
  - "@Miciah, for ingress"
  - "@yuqi-zhang, for machine-config-operator"
  - "@carbonin, for assisted-installer"
  - "@patrickdillon, for installer"
  - "@joelspeed, for API and infrastructure config"
  - "@spadgett, for console"
  - "@cybertron, for bare metal networking"
  - "@sjenning, for control plane"
  - "@Jan-Fajerski, for monitoring"
  - "@bbennett, for networking"
  - "@p0lyn0mial, for library-go"
  - "@jsafrane, for storage and registry"
  - "@TrevorKing, for cluster-version-operator and OTA"
  - "@damdo, for cloud-credential-operator"
  - "@jerpeter, for OpenShift architecture"
approvers:
  - TBD
api-approvers:
  - TBD
creation-date: 2025-12-10
last-updated: 2025-12-10
tracking-link:
  - https://issues.redhat.com/browse/OCPEDGE-2280
see-also:
  - "/enhancements/edge-topologies/single-node/cluster-high-availability-mode-api.md"
  - "/enhancements/edge-topologies/two-node/two-node-fencing.md"
  - "/enhancements/edge-topologies/two-node/two-node-arbiter.md"
  - "/enhancements/monitoring/integration-with-topology-modes.md"
replaces: []
superseded-by: []
---

# Adaptable Topology

## Terms

**Adaptable Topology** - A cluster-topology mode that adjusts cluster control-plane and infrastructure behavior based on the current number of control-plane and worker nodes in the cluster.

**AutomaticQuorumRecovery (AQR)** - A future extension enabling automatic failover for two-node clusters via fencing and pacemaker. While not implemented in this enhancement, Adaptable topology is designed with AQR in mind as the mechanism needed to support DualReplica transitions.

**Control Plane Topology** - The cluster-topology mode describing how control-plane nodes are deployed and managed (SingleReplica, DualReplica, HighlyAvailable, HighlyAvailableArbiter, or Adaptable). Control-plane nodes are nodes labeled with `node-role.kubernetes.io/control-plane` or `node-role.kubernetes.io/master`.

**Infrastructure Topology** - The cluster-topology mode describing how infrastructure workloads are distributed (SingleReplica, HighlyAvailable, or Adaptable). When there are no worker nodes, control-plane nodes serve as workers.

**Topology Transition** - A Day 2 operation that changes the values of controlPlaneTopology and infrastructureTopology in the cluster's Infrastructure config. Adaptable topology proposes allowing this as a one-way transition from existing topology modes to Adaptable.

## Summary

This enhancement introduces Adaptable topology,
a cluster-topology mode that adjusts behavior based on node count.
The controlPlaneTopology responds to control-plane and arbiter node counts.
The infrastructureTopology responds to worker node counts.
Clusters can install with Adaptable topology or transition to it from
selected topologies as a one-way Day 2 operation.
The initial implementation supports SingleReplica-to-Adaptable transitions.
Future stages will add AutomaticQuorumRecovery (AQR) for DualReplica
behavior on two-node configurations.

## Motivation

Cluster demands change over time.
Changes may be customer-driven, such as growing service demands.
Changes may be architectural,
such as colocating services to optimize communications.
OpenShift offers flexibility for capacity needs through adding or removing
nodes, autoscaling, and similar capabilities.
These operate within a fixed cluster topology.
OpenShift does not offer flexibility when high availability requirements change.
Customers who start with Single Node OpenShift (SNO)
and later need high availability must redeploy their cluster.

Adaptable topology addresses this by enabling clusters to adjust
as nodes are added or removed, allowing growth without redeployment.

### User Stories

* As a cluster administrator running Single Node OpenShift (SNO) at an edge location, I want to add control-plane nodes to my cluster to achieve high availability so that I can handle node failures without service disruption as workloads become more critical.

* As a solutions architect deploying OpenShift clusters at scale, I want to start with minimal footprint deployments that can grow into highly available clusters so that I can reduce initial costs while maintaining scalability.

* As a cluster administrator managing a fleet of edge deployments, I want the cluster topology to automatically adapt as I add or remove nodes so that I don't need to reconfigure cluster behavior or redeploy clusters when my infrastructure changes.

* As a platform engineer, I want a topology framework that reduces the number of special cases in my automation and tooling so that I can simplify operations across cluster configurations.

* As an OpenShift operator developer, I want an API to detect the topology behavior so that my operator can make decisions about replica counts and placement strategies as the cluster scales.

* As an operator author, I want to declare my operator's compatibility with Adaptable topology in my operator metadata so that cluster administrators can make informed decisions about installing my operator on clusters using this topology mode.

* As a cluster administrator preparing to transition to Adaptable topology, I want to see my installed operators' compatibility status so that I can assess risks before approving the transition.

### Goals

* Provide a new Adaptable topology mode that can be set at installation time
  or transitioned to as a Day 2 operation
* Enable SingleReplica clusters to transition to Adaptable topology,
  scaling to multi-node configurations without redeployment
* Automatically adjust control-plane behavior based on the number of
  control-plane and arbiter nodes
* Automatically adjust infrastructure workload distribution based on
  the number of worker nodes
* Provide a mechanism for operators to detect the topology behavior
  through the Infrastructure API
* Provide shared utilities in library-go to reduce the implementation burden
  for operator authors implementing support for Adaptable topology
* Support OLM operator compatibility declarations for Adaptable topology
  to enable informed transition decisions
* Establish the architectural foundation for future topology transitions,
  including AutomaticQuorumRecovery (AQR) for DualReplica support
* Maintain backward compatibility with existing clusters using
  fixed topology modes

### Non-Goals

* Implementing AutomaticQuorumRecovery (AQR) or DualReplica-based fencing
  mechanisms in this enhancement
* Supporting bidirectional topology transitions
  (e.g., transitioning from Adaptable back to SingleReplica)
* Supporting transitions from all existing topology modes
  (initial implementation focuses on SingleReplica to Adaptable)
* Automatic node provisioning or deprovisioning based on workload demands
* Workload-aware topology decisions
  (e.g., transitioning based on application requirements rather than node count)
* Supporting topology transitions for HyperShift clusters
* Implementing topology transitions for MicroShift deployments

## Proposal

The Adaptable topology introduces a new cluster-topology mode that adjusts
cluster behavior based on node composition.
When both controlPlaneTopology and infrastructureTopology are set to
`Adaptable`, the cluster evaluates the number and type of nodes present
and adopts the behavior of the fixed topology mode.

### Topology Behavior Matrix

The Adaptable topology determines the effective behavior based on these rules:

**Control Plane Topology Behavior:**

*Note: When AutomaticQuorumRecovery (AQR) is implemented in a future enhancement, the two control-plane node case will transition to DualReplica behavior when AQR is enabled.*

| Control-Plane Nodes | Arbiter Nodes | Effective controlPlaneTopology Behavior |
|:-------------------:|:-------------:|:---------------------------------------:|
| 1                   | 0             | SingleReplica                           |
| 2                   | 0             | SingleReplica (AQR not implemented)     |
| 2                   | 1+            | HighlyAvailableArbiter                  |
| 3+                  | any           | HighlyAvailable                         |

**Infrastructure Topology Behavior:**

*Note: When there are no worker nodes, control-plane nodes serve as workers.*

| Worker Nodes | Effective infrastructureTopology Behavior |
|:------------:|:-----------------------------------------:|
| 0 or 1       | SingleReplica                             |
| 2+           | HighlyAvailable                           |

### How Adaptable Topology Works

Operators watch the Infrastructure config and adjust their behavior.

The cluster-etcd-operator (CEO) maintains quorum safety through
topology transitions.
In Adaptable topology without AQR,
there is no quorum safety when fewer than 3 control-plane nodes are present.
A single etcd instance runs whether the cluster has one or
two control-plane nodes.

When scaling up to 3 control-plane nodes,
CEO starts two learner etcd instances on the nodes not running etcd.
CEO then coordinates an atomic transition to promote both learners together.
When scaling down below 3 control-plane nodes,
CEO coordinates two etcd instances to leave together, leaving a single instance.

When nodes are added or removed,
operators detect the change through the Infrastructure config and adjust
their deployment strategies, replica counts, and placement policies.
This happens without cluster reconfiguration.

### Workflow Description

#### Installing a Cluster with Adaptable Topology

**cluster creator** is a human user responsible for deploying a cluster.

1. The cluster creator prepares an `install-config.yaml` with the desired initial node count
2. The cluster creator sets `adaptableTopology: true` in the `install-config.yaml` (optional, defaults to `false`)
3. The cluster creator runs `openshift-install create cluster` to complete the installation
4. The installer validates the configuration and sets both `controlPlaneTopology` and `infrastructureTopology` to `Adaptable` in the Infrastructure config
5. The cluster installs with behavior matching the effective topology for the initial control-plane, arbiter, and worker node counts
6. After installation completes, the cluster is ready to scale by adding or removing nodes

*Note: The `adaptableTopology` flag is optional and defaults to `false`. Future releases may default to `true` once the feature matures.*

#### Transitioning an Existing Cluster to Adaptable Topology

**cluster administrator** is a human user responsible for managing an existing cluster.

1. The cluster administrator runs `oc adm topology transition adaptable --dry-run` to check for operator compatibility issues
2. The CLI scans installed operators and reports their compatibility status:
   - Compatible: Operators with Adaptable topology support
   - Warning: Operators with unknown compatibility status (no declaration)
   - Error: Operators explicitly incompatible with Adaptable topology
3. If errors are present, the cluster administrator must remediate incompatible operators (updates, replacements, or removal) before proceeding
4. The cluster administrator runs `oc adm topology transition adaptable` to initiate the transition
5. The CLI warns that this is a one-way transition and asks for confirmation
6. If warnings (unknown compatibility) are present, the CLI asks for additional confirmation to proceed anyway
7. If errors are present, the transition is blocked unless `--force` is provided (unsupported, for development purposes only)
8. Upon confirmation, the CLI updates both `controlPlaneTopology` and `infrastructureTopology` fields to `Adaptable` in the Infrastructure config
9. The API validates that both fields are being set to `Adaptable` together (enforced by ValidatingAdmissionPolicy)
10. The cluster transitions to Adaptable topology, maintaining current effective behavior based on existing node count
11. The cluster is now ready to scale by adding or removing nodes

#### Scaling a Cluster Running Adaptable Topology

**cluster administrator** is managing a cluster already running Adaptable topology.

##### Scaling Control-Plane Nodes

1. The cluster administrator adds a new control-plane node to the cluster
2. The node joins the cluster and receives the control-plane labels
3. Operators watching the Infrastructure config detect the node count change
4. When crossing the 2→3 control-plane node threshold:
   - CEO initiates the etcd scaling process, starting learner instances on the two control-plane nodes not running etcd
   - CEO coordinates an atomic transition to promote both learner instances together
   - Other operators adjust their behavior to match HighlyAvailable control-plane topology
5. When scaling down and crossing the 3→2 control-plane node threshold:
   - CEO coordinates two etcd instances to atomically leave the cluster together
   - The cluster continues operating with a single etcd instance
   - Other operators adjust their behavior to match SingleReplica control-plane topology

##### Scaling Worker Nodes

1. The cluster administrator adds or removes worker nodes
2. Infrastructure operators detect the worker node count change
3. When crossing the 1→2 worker node threshold:
   - Infrastructure operators adjust their behavior to match HighlyAvailable infrastructure topology
   - Replica counts and placement strategies are updated accordingly
4. When scaling down and crossing the 2→1 worker node threshold:
   - Infrastructure operators adjust their behavior to match SingleReplica infrastructure topology
5. Special case when crossing the 1→0 worker node threshold:
   - Infrastructure operators switch to scheduling on control-plane nodes
   - Some operators may increase replica counts since control-plane nodes are now eligible for infrastructure workloads

### API Extensions

#### Infrastructure Config Changes

The Infrastructure config will be updated to support `Adaptable` as a new value
for both `controlPlaneTopology` and `infrastructureTopology` fields:

```go
type TopologyMode string

const (
    HighlyAvailableTopologyMode         TopologyMode = "HighlyAvailable"
    SingleReplicaTopologyMode           TopologyMode = "SingleReplica"
    DualReplicaTopologyMode             TopologyMode = "DualReplica"
    HighlyAvailableArbiterTopologyMode  TopologyMode = "HighlyAvailableArbiter"
    AdaptableTopologyMode               TopologyMode = "Adaptable"
)
```

A ValidatingAdmissionPolicy will enforce that when either field is set to
`Adaptable`, both must be set to `Adaptable` together.
This prevents invalid configurations where control-plane and infrastructure
topologies are out of sync.

#### Shared Utilities in library-go

Shared utilities will be provided in library-go to ease implementation
for operator authors.
These utilities will:

- Check the `AdaptableTopology` feature gate to determine if
  Adaptable topology is enabled
- Provide a subscription function for topology changes that operators invoke
  to enable Adaptable topology behavior
- Subscribe to node count changes and provide notifications when
  behavior thresholds are crossed
- Provide consistent logic for operators to determine appropriate
  replica counts and placement strategies based on current node counts

Operators only need to watch for node changes when Adaptable topology is set.
The utilities handle feature gate checks automatically.

This modernizes operators to respond directly to node count changes
rather than relying on topology abstractions.

#### Feature Gate

A new feature gate `AdaptableTopology` will be added to gate this functionality. The feature gate will progress through the following stages:

- **Dev Preview**: Part of the `DevPreviewNoUpgrade` feature set
- **Tech Preview**: Moved to the `TechPreviewNoUpgrade` feature set
- **GA**: Moved to the `Default` feature set

#### Operator Compatibility Annotations

Operators declare Adaptable topology support using annotations. The implementation differs between in-payload operators and OLM-managed operators due to their distinct lifecycle management.

##### In-Payload Operator Compatibility

In-payload operators (core operators managed by cluster-version-operator) declare compatibility via annotations on their `ClusterOperator` resource:

```yaml
apiVersion: config.openshift.io/v1
kind: ClusterOperator
metadata:
  name: etcd
  annotations:
    operators.openshift.io/infrastructure-features: '["Adaptable"]'
spec:
  # ... ClusterOperator spec
```

**Implementation:**
- Each in-payload operator adds the annotation to the ClusterOperator resource it creates
- The annotation is added when the operator has been updated to support Adaptable topology
- The `oc adm topology transition` command reads ClusterOperator resources to check compatibility

**Rationale:**
In-payload operators do not have ClusterServiceVersion (CSV) resources.
They are deployed directly as part of the release payload and report status
via ClusterOperator resources.
The ClusterOperator resource is the natural location for capability declarations.

**Ownership:**
Individual operator teams are responsible for adding the annotation
to their ClusterOperator resources.
The API and Control Plane teams review any ClusterOperator API extensions
needed to support this annotation pattern.

##### OLM Operator Compatibility

OLM-managed operators (out-of-payload operators installed via OperatorHub) declare compatibility via annotations on their `ClusterServiceVersion` resource:

```yaml
apiVersion: operators.coreos.com/v1alpha1
kind: ClusterServiceVersion
metadata:
  name: my-operator.v1.0.0
  annotations:
    operators.openshift.io/infrastructure-features: '["Adaptable"]'
spec:
  # ... CSV spec
```

**Implementation:**
- Operator authors add the annotation to their CSV when publishing to OperatorHub
- OLM manages the CSV lifecycle
- The `oc adm topology transition` command and console read CSV resources
  to check compatibility

##### Compatibility Status Interpretation

The `oc adm topology transition` command interprets compatibility as follows:

| Annotation Present | Annotation Value    | Status       | Transition Behavior                       |
|:------------------:|:-------------------:|:------------:|:------------------------------------------|
| Yes                | `["Adaptable"]`     | Compatible   | Proceed (after user confirmation)         |
| No                 | N/A                 | Unknown      | Warning, requires additional confirmation |
| Yes                | `[]` or other value | Incompatible | Blocked (unless `--force` is used)        |

The console and CLI display compatibility information before
topology transitions, allowing cluster administrators to make informed decisions.

### Topology Considerations

#### Hypershift / Hosted Control Planes

Adaptable topology is not compatible with HyperShift clusters.
HyperShift uses `External` as its `controlPlaneTopology`,
and since both `controlPlaneTopology` and `infrastructureTopology` must be
set to `Adaptable` together, HyperShift clusters cannot use Adaptable topology.

#### Standalone Clusters

Standalone clusters are the primary target for Adaptable topology.
This enhancement enables standalone clusters to start with minimal footprints
and scale as requirements evolve, without requiring redeployment.

`platform: none` will be supported for all node configurations.

`platform: baremetal` presents a challenge for single-node clusters.
It sets up bare metal networking using keepalived for ingress load balancing,
which is not useful and creates a point of failure for SNO deployments.
The Bare Metal Networking team will be consulted to determine if this
networking setup can be disabled for single-node clusters.
The goal is to support `platform: baremetal` for all node configurations
rather than limiting support to deployments with two or more nodes.

#### Single-node Deployments or MicroShift

Single Node OpenShift (SNO) clusters are candidates for
transitioning to Adaptable topology.
The primary use case is enabling SNO deployments to scale to
multi-node highly available configurations as requirements change.

MicroShift is not affected by this enhancement as it doesn't use an
Infrastructure config to track topologies.
However, one motivating factor for Adaptable topology is bringing
MicroShift and OpenShift architectures closer together.
MicroShift would benefit from operators having an adaptable topology mode
that handles topology changes via node updates.
A follow-up enhancement will address MicroShift to SNO transitions,
which may leverage Adaptable topology at transition time.

### Implementation Details/Notes/Constraints

#### Component Changes Summary

*Note: Some operators like cluster-etcd-operator, cluster-monitoring-operator, Console, and OLM appear both in the table below and in the Core Operators section because they have unique changes in addition to the common changes affecting all core operators.*

The following components require updates to support Adaptable topology:

| Component | Changes Required |
| --------- | ---------------- |
| [Infrastructure API](#infrastructure-config-changes) | Add `Adaptable` enum value, ValidatingAdmissionPolicy for paired field updates |
| [openshift/installer](#installer-changes) | Add `adaptableTopology` flag to install-config, validate platform support, set Infrastructure config fields |
| [openshift/oc](#oc-cli-changes) | New `oc adm topology transition` command with dry-run and compatibility checks |
| [library-go](#shared-utilities-in-library-go) | Shared utilities for feature gate checks, topology change subscriptions, node count watching, and threshold detection |
| [Core Operators](#core-operator-changes) | Node count awareness, dynamic replica/placement adjustments via library-go subscriptions |
| [All Operators](#operator-compatibility-annotations) | Add compatibility annotation to ClusterOperator resources |
| [cluster-etcd-operator](#how-adaptable-topology-works) | Enhanced scaling logic for 2↔3 node transitions |
| [cluster-monitoring-operator](#monitoring-operator-and-telemetry) | Collect and backhaul SLI telemetry for feature adoption and transition metrics |
| [OLM](#operator-compatibility-annotations) | Operator compatibility annotation support and filtering |
| [Console](#console-changes) | Operator compatibility display, marketplace filtering, restart with Adaptable topology |

#### Initial Topology Audit

*This data was gathered via GitHub API search queries for `ControlPlaneTopology` and `InfrastructureTopology` references in Go code
across the openshift organization. Test repositories (origin), documentation, enhancements, and non-operator repositories were filtered out.
Operators showing "No" for both fields had no search hits, indicating they do not currently reference these topology APIs.
Results reflect actual code usage as of December 2025.*

**In-Payload Operators**

| Operator Name                                | References ControlPlaneTopology | References InfrastructureTopology |
| -------------------------------------------- | ------------------------------- | --------------------------------- |
| cluster-authentication-operator              | Yes                             | No                                |
| cluster-autoscaler-operator                  | Yes                             | No                                |
| cluster-baremetal-operator                   | Yes                             | No                                |
| cluster-cloud-controller-manager-operator    | Yes                             | Yes                               |
| cluster-config-operator                      | Yes                             | No                                |
| cluster-control-plane-machine-set-operator   | Yes                             | No                                |
| cluster-csi-snapshot-controller-operator     | Yes                             | No                                |
| cluster-etcd-operator                        | Yes                             | Yes                               |
| cluster-image-registry-operator              | No                              | Yes                               |
| cluster-ingress-operator                     | Yes                             | Yes                               |
| cluster-kube-apiserver-operator              | Yes                             | Yes                               |
| cluster-monitoring-operator                  | Yes                             | Yes                               |
| cluster-network-operator                     | Yes                             | Yes                               |
| cluster-openshift-apiserver-operator         | Yes                             | No                                |
| cluster-storage-operator                     | Yes                             | No                                |
| cluster-version-operator                     | Yes                             | No                                |
| cloud-credential-operator                    | Yes                             | Yes                               |
| console-operator                             | Yes                             | Yes                               |
| csi-operator                                 | Yes                             | No                                |
| machine-api-operator                         | Yes                             | No                                |
| machine-config-operator                      | Yes                             | No                                |
| operator-framework-olm                       | Yes                             | Yes                               |
| operator-framework-operator-controller       | Yes                             | Yes                               |
| service-ca-operator                          | Yes                             | No                                |
| cluster-dns-operator                         | No                              | No                                |
| cluster-kube-controller-manager-operator     | No                              | No                                |
| cluster-kube-scheduler-operator              | No                              | No                                |
| cluster-machine-approver                     | No                              | No                                |
| cluster-samples-operator                     | No                              | No                                |
| insights-operator                            | No                              | No                                |

**Out-of-Payload Operators**

| Operator Name                       | References ControlPlaneTopology | References InfrastructureTopology |
| ----------------------------------- | ------------------------------- | --------------------------------- |
| cluster-nfd-operator                | Yes                             | No                                |
| cluster-node-tuning-operator        | Yes                             | No                                |
| csi-driver-shared-resource-operator | Yes                             | No                                |
| local-storage-operator              | Yes                             | No                                |
| node-observability-operator         | Yes                             | No                                |
| oadp-operator                       | Yes                             | No                                |
| ptp-operator                        | Yes                             | No                                |
| sriov-network-operator              | Yes                             | Yes                               |
| vmware-vsphere-csi-driver-operator  | Yes                             | No                                |

#### Reviewer's Guide

This section provides quick navigation to relevant content for each reviewing team:

**Control Plane Team:**
- [Operator Compatibility Annotations - In-Payload](#in-payload-operator-compatibility) - Review of ClusterOperator API extensions
- [API Extensions](#api-extensions) - Infrastructure config and library-go changes
- [Topology Behavior Matrix](#topology-behavior-matrix) - Behavior rules for control plane components
- [Initial Topology Audit](#initial-topology-audit) - CEO, API server, and control plane operator topology API usage
- [How Adaptable Topology Works](#how-adaptable-topology-works) - CEO's role in etcd scaling
- [Scaling Control-Plane Nodes](#scaling-control-plane-nodes) - Control plane and etcd scaling workflow
- [Core Operator Changes](#core-operator-changes) - CEO and control plane operator changes
- [oc CLI Changes](#oc-cli-changes) - CLI tooling for topology transitions
- [Open Questions - Control Plane](#control-plane-cluster-etcd-operator) - Questions for etcd team
- [Risks - etcd Data Loss](#risk-etcd-data-loss-if-transitions-are-not-atomic) - Atomic transition requirements
- [Test Plan - etcd quorum management](#post-transition-tests) - Testing etcd scaling
- [Service Level Objectives](#service-level-objectives) - SLOs for etcd operations
- [Operational Aspects](#operational-aspects-of-api-extensions) - Control plane operational considerations

**OLM Team:**
- [Operator Compatibility Annotations - OLM](#olm-operator-compatibility) - CSV annotation implementation for OLM-managed operators
- [Core Operator Changes](#core-operator-changes) - OLM-specific changes
- [Transitioning an Existing Cluster](#transitioning-an-existing-cluster-to-adaptable-topology) - CLI integration with OLM
- [Open Questions - Operators](#operators-optional--3rd-party) - Operator certification questions
- [User Stories](#user-stories) - Operator author perspectives

**Bare Metal Platform Team:**
- [Initial Topology Audit](#initial-topology-audit) - Bare metal operator topology API usage
- [Platform Support Constraints](#platform-support-constraints) - Bare metal limitations
- [Topology Considerations - Standalone Clusters](#standalone-clusters) - Platform support details
- [Risks - Platform Bare Metal](#risk-platform-bare-metal-may-not-support-single-node-clusters) - SNO networking challenge

**Core Operator Teams:**

This applies to operators that follow the standard pattern (library-go subscriptions, dynamic replicas/placement, and compatibility annotations):

- [Initial Topology Audit](#initial-topology-audit) - Topology API usage across operators
- [Operator Compatibility Annotations - In-Payload](#in-payload-operator-compatibility) - Adding compatibility annotations to ClusterOperator resources
- [Core Operator Changes](#core-operator-changes) - Standard operator pattern using library-go utilities
- [Topology Behavior Matrix](#topology-behavior-matrix) - Behavior rules
- [Scaling Control-Plane Nodes](#scaling-control-plane-nodes) - Control plane scaling workflow
- [Scaling Worker Nodes](#scaling-worker-nodes) - Infrastructure scaling workflow
- [Scaling Workflows](#scaling-a-cluster-running-adaptable-topology) - Node addition/removal

**Assisted Installer Team:**
- [Installing a Cluster](#installing-a-cluster-with-adaptable-topology) - Installation workflow
- [Installer Changes](#installer-changes) - Install-config flag

**Core Installer Team:**
- [Installing a Cluster](#installing-a-cluster-with-adaptable-topology) - Installation workflow
- [Installer Changes](#installer-changes) - Install-config flag and validation
- [Graduation Criteria](#entering-dev-preview) - Installer requirements per phase

**API Team:**
- [Infrastructure Config Changes](#infrastructure-config-changes) - API changes
- [Operator Compatibility Annotations - In-Payload](#in-payload-operator-compatibility) - ClusterOperator API extensions for compatibility declarations
- [Topology Behavior Matrix](#topology-behavior-matrix) - Behavior rules
- [Open Questions - Infrastructure Config](#infrastructure-config--api-updates) - API questions
- [Operational Aspects](#operational-aspects-of-api-extensions) - ValidatingAdmissionPolicy

**Console Team:**
- [Initial Topology Audit](#initial-topology-audit) - Console operator topology API usage
- [Console Changes](#console-changes) - UI updates
- [Core Operator Changes](#core-operator-changes) - Console-specific changes
- [Transitioning an Existing Cluster](#transitioning-an-existing-cluster-to-adaptable-topology) - Compatibility display workflow

**Bare Metal Networking Team:**
- [Platform Support Constraints](#platform-support-constraints) - Keepalived for SNO
- [Topology Considerations - Standalone Clusters](#standalone-clusters) - Bare metal networking details

**Monitoring Team:**
- [Initial Topology Audit](#initial-topology-audit) - Monitoring operator topology API usage
- [Core Operator Changes](#core-operator-changes) - Monitoring operator changes
- [Monitoring Operator and Telemetry](#monitoring-operator-and-telemetry) - Telemetry requirements
- [Scaling Control-Plane Nodes](#scaling-control-plane-nodes) - Monitoring behavior during transitions

**CLI Team:**
- [oc CLI Changes](#oc-cli-changes) - New topology transition command
- [Transitioning an Existing Cluster](#transitioning-an-existing-cluster-to-adaptable-topology) - CLI workflow
- [Transition Behavior](#transition-behavior) - Compatibility checking logic
- [Operational Aspects](#operational-aspects-of-api-extensions) - CLI operational considerations

**library-go Team:**
- [Shared Utilities in library-go](#shared-utilities-in-library-go) - New topology utilities and subscriptions
- [Core Operator Changes](#core-operator-changes) - How operators use library-go utilities
- [Infrastructure Config Changes](#infrastructure-config-changes) - API integration with utilities
- [Open Questions - Infrastructure Config](#infrastructure-config--api-updates) - Library-go implementation questions

**OTA/CVO Team:**
- [Initial Topology Audit](#initial-topology-audit) - CVO topology API usage
- [Core Operator Changes](#core-operator-changes) - CVO coordination role
- [Transitioning an Existing Cluster](#transitioning-an-existing-cluster-to-adaptable-topology) - Topology transition workflow
- [Upgrade / Downgrade Strategy](#upgrade--downgrade-strategy) - CVO upgrade handling
- [Version Skew Strategy](#version-skew-strategy) - Operator compatibility during upgrades

#### Installer Changes

The installer will support a new top-level `adaptableTopology` field in `install-config.yaml`:

```yaml
apiVersion: v1
baseDomain: example.com
adaptableTopology: true  # optional, defaults to false
controlPlane:
  name: master
  replicas: 1
```

When `adaptableTopology: true` is set, the installer validates platform support and sets both `controlPlaneTopology` and `infrastructureTopology` to `Adaptable` in the Infrastructure config.

#### oc CLI Changes

A new `oc adm topology transition` command will be added to facilitate safe topology transitions. The command provides:

- `--dry-run` flag to check operator compatibility without applying changes
- Scanning of installed operators for compatibility declarations
- Blocking on incompatible operators (error state)
- Warnings for operators with unknown compatibility
- `--force` flag to bypass blocks (unsupported, development only)
- Confirmation prompts before applying one-way transitions

#### Core Operator Changes

The following core operators require updates to support Adaptable topology. All will invoke library-go subscription functions and adjust behavior based on node counts:

| Operator | Specific Changes |
| -------- | ---------------- |
| cluster-etcd-operator | Enhanced scaling logic for 2↔3 node transitions (see [How Adaptable Topology Works](#how-adaptable-topology-works)) |
| cluster-authentication-operator | Adjust minimum kube-apiserver replica count checks based on node counts |
| cluster-ingress-operator | Adjust ingress controller replica counts and placement based on node counts |
| cluster-monitoring-operator | Adjust monitoring component replica counts and placement based on node counts |
| cluster-network-operator | Adjust network component replica counts based on node counts |
| OLM | Operator compatibility annotation support and filtering (see [Operator Compatibility Annotations](#operator-compatibility-annotations)) |
| Console | Operator compatibility display, marketplace filtering, restart with Adaptable topology (see [Console Changes](#console-changes)) |

See [Initial Topology Audit](#initial-topology-audit) for a comprehensive list of all operators that reference topology APIs.

#### Console Changes

The console will be updated to:
- Display operator compatibility status for Adaptable topology
- Provide a marketplace filter to show only operators that support Adaptable topology
- Restart with Adaptable topology set when the transition occurs

#### Platform Support Constraints

`platform: none` is supported for all node configurations.

`platform: baremetal` requires coordination with the Bare Metal Networking
team to disable keepalived networking for single-node clusters.
If this cannot be resolved, `platform: baremetal` support will be limited to
clusters with 2 or more nodes.

#### Transition Behavior

The topology transition command handles operator compatibility as follows:

**No warnings or errors**: The transition proceeds after user confirmation.

**Only warnings (unknown compatibility)**: The command encourages resolving warnings first but allows the transition to proceed with additional confirmation.

**At least one error (incompatible operator)**: The transition is blocked. The `--force` flag bypasses the block but is unsupported. A cluster-scoped event is posted to track forced transitions for support case detection.

#### Monitoring Operator and Telemetry

The cluster-monitoring-operator will be updated to collect and backhaul telemetry for Adaptable topology:

**SLI Metrics:**
- Cluster count using Adaptable topology
- Topology transition attempts (success/failure rates)
- Topology transition duration
- Installed operators with unknown compatibility status (helps identify popular operators needing self-certification)
- etcd scaling operation success rates
- Cluster API availability during transitions
- Node count distribution across Adaptable topology clusters

These metrics enable tracking feature adoption and identifying issues in production deployments.

#### Service Level Objectives

The following SLOs define success criteria for Adaptable topology:

| Objective | Target |
| --------- | ------ |
| Topology transition completion time | Within 2 seconds of CLI command approval |
| Operator health after transition | All compatible operators healthy within 3 minutes |
| Operator stability during transitions | No crashes for operators labeled as compatible |
| etcd scaling operation success rate | 99.9% success rate for 2↔3 node transitions |
| Cluster API availability | API remains available during transitions |

These SLOs will be validated during testing and monitored in production.

### Risks and Mitigations

#### Risk: Operators May Not Handle Dynamic Topology Changes Correctly

**Risk**: Operators that haven't been updated may not respond correctly to
node count changes, leading to incorrect replica counts or placement decisions.

**Mitigation**:
- OLM compatibility annotations allow operators to declare support
- The topology transition command scans for incompatibilities before
  allowing transitions
- Library-go utilities provide consistent implementation patterns
- All core operators will be updated before GA (this is a GA requirement)
- The primary risk is with optional operators and community/3rd party operators

#### Risk: Paradigm Shift May Confuse Users

**Risk**: Users accustomed to fixed topology definitions
may find the Adaptable topology paradigm shift confusing.

**Mitigation**:
- Documentation explaining how Adaptable topology works
- The cluster behaves correctly for the number of nodes provided
- The paradigm is simpler: add or remove nodes, cluster adapts automatically
- Transition is optional; existing topologies remain available

#### Risk: Confusion Between Adaptable 2-Node Behavior and DualReplica

**Risk**: Users may be confused when 2-node Adaptable topology operates like
SingleReplica rather than DualReplica,
since DualReplica is an existing topology mode.

**Mitigation**:
- Documentation explaining the difference and why AQR is needed
  for DualReplica behavior
- The topology transition command will warn users about this behavior
- Future AQR implementation will enable DualReplica behavior for
  2-node configurations

#### Risk: etcd Data Loss If Transitions Are Not Atomic

**Risk**: If CEO cannot make 1→3 or 3→1 etcd member transitions truly atomic,
data loss or corruption could occur.

**Mitigation**:
- CEO will coordinate atomic transitions when adding or removing
  two members simultaneously
- Learner instances are used before promoting members
- Extensive testing of atomic transition scenarios in CI
- If atomicity cannot be guaranteed, the feature will be blocked until resolved

#### Risk: Forced Transitions May Leave Clusters in Unsupported States

**Risk**: Users may use `--force` to bypass compatibility checks,
creating clusters that cannot be supported.

**Mitigation**:
- Forced transitions are explicitly marked as unsupported
- A cluster-scoped event is posted to enable detection in support cases
- Documentation warns against forcing transitions
- The `--force` flag is intended for development only
- Follow prior art from cluster version operator for handling forced operations

#### Risk: Platform Bare Metal May Not Support Single-Node Clusters

**Risk**: If keepalived networking cannot be disabled,
`platform: baremetal` will be limited to 2+ nodes,
reducing the value of Adaptable topology for this platform.

**Mitigation**:
- Early coordination with the Bare Metal Networking team
- Alternative is to document the limitation
- `platform: none` provides full support as a fallback

### Drawbacks

#### Increased Operator Complexity

Operators must be updated to handle dynamic topology changes based on node counts.
The logic for different node counts already exists in operators.
It's currently applied once at startup because Infrastructure config
topology values are static.
Operators must now watch for node count changes and adjust behavior dynamically.
Library-go utilities help, but operators still need to test and validate
behavior across all node count thresholds.

#### Expanded Test Matrix

Testing Adaptable topology requires validating behavior at each node count
threshold (1, 2, 3+ control-plane nodes; 0, 1, 2+ worker nodes).
This expands the test matrix compared to testing a single fixed topology.
CI infrastructure must handle additional test lanes for installations,
transitions, and scaling operations.

#### One-Way Transitions

Transitions to Adaptable topology are one-way.
Clusters cannot transition back to fixed topology modes.
This design simplifies implementation but means users must be certain
before transitioning.
Mistakes require cluster redeployment.

#### Coordination Across Teams

Adaptable topology requires updates across many teams:
installer, oc CLI, library-go, CEO, OLM, Console, and all core operator teams.
This coordination overhead is significant.
Any team falling behind delays the entire feature.

#### Limited Optional and 3rd Party Operator Support

Optional and 3rd party operators have no required timeline for
Adaptable topology support updates.
Users may encounter operators that don't handle topology transitions correctly.
Operator compatibility annotations help identify these cases,
but users may still need to wait for updates or choose alternative operators.

#### Potential for Unexpected Behavior

Users accustomed to fixed topologies may encounter unexpected behavior
when node counts cross thresholds.
The behavior is deterministic and documented, but differs from fixed topologies.
Troubleshooting may be more complex when behavior changes based on node count.

## Alternatives (Not Implemented)

### Direct Topology Transitions

An alternative is to allow direct transitions between topology modes
(e.g., SingleReplica → HighlyAvailable) without introducing Adaptable topology.

Transitioning from SingleReplica to HighlyAvailable signals the user wants
HA cluster behavior.

However, several problems emerge:

**Ambiguous Target State**: Should transitioning to HighlyAvailable create a
3-node compact cluster or provision compute nodes for a 5-node cluster?
The end state is unclear.

**Topology Mismatch**: The cluster configuration would claim HighlyAvailable
while still operating as SingleReplica until nodes are provisioned.
The declared topology differs from actual behavior.

**Complex Support Matrix**: Supporting arbitrary topology transitions creates
a complex matrix.
Only a subset would be feasible, requiring documentation.

### Why Adaptable Topology

Adaptable topology makes the transition a no-op until the user scales their cluster.
Operators determine correct behavior based on actual node counts.
This adds complexity to operator development but provides flexibility
for operators to adapt to cluster capabilities.

The one-way transition model simplifies testing.
We only test transitions into Adaptable topology and can stagger support
for different source topologies.

## Open Questions [optional]

### Operators (Core Payload)

1. **Core Operators Affected by Topology**: Which core operators have unique behavior based on controlPlaneTopology and infrastructureTopology values today?

   *See [Initial Topology Audit](#initial-topology-audit) for GitHub API search results showing 30 in-payload operators audited (24 reference topology APIs, 6 do not) and 9 out-of-payload operators that reference these topology fields.*

2. **infrastructureTopology Changes**: Is adding new infrastructureTopology values similar to adding controlPlaneTopology values, or are there special considerations?

### Operators (Optional & 3rd-Party)

3. **Self-Certification Process**: How can optional and 3rd-party operators self-certify Adaptable topology support? One approach: remain healthy while the AdaptableTopology suite scales nodes.

4. **Unknown Topology Behavior**: How do operators behave on unknown topologies? Most likely run in HighlyAvailable mode (default), but SingleReplica would be safer.

### Testing

5. **Transition Validation**: How do we validate operators handle transitions correctly end-to-end?

### Control Plane (cluster-etcd-operator)

6. **Learner Promotion After Voter Failure**: If we run a learner on a second control-plane node and the voter fails, can quorum restore promote the learner? Or can only former voters be restored with quorum?

7. **etcd Safety Enforcement**: Can CEO enforce etcd safety for 1↔3 node transitions? DualReplica offloads this to pacemaker. Do we need something similar?

8. **Atomic Member Changes**: Can we guarantee both learner promotions happen together (or neither happens)?

9. **Preventing Unsafe Transitions**: What other mechanisms are needed to prevent unsafe etcd transitions?

### Infrastructure Config & API Updates

10. **Paired Field Updates**: Is a ValidatingAdmissionPolicy sufficient to enforce that controlPlaneTopology and infrastructureTopology are always set together?

11. **Existing Policy Changes**: Do existing ValidatingAdmissionPolicies need changes to update the Infrastructure config?

12. **library-go Design Details**: What are the specific designs for library-go utilities that provide topology transition awareness and node-count change subscriptions?

## Test Plan

### CI Lanes

| Lane | Frequency | Description |
| ---- | --------- | ----------- |
| AdaptableTopology Suite on cluster installed with Adaptable topology | Nightly | Run AdaptableTopology test suite on a cluster installed with `adaptableTopology: true` in install-config |
| AdaptableTopology Suite on SNO-transitioned cluster | Nightly | Run AdaptableTopology test suite on a cluster transitioned from SingleReplica SNO to Adaptable topology |
| End-to-End tests (e2e) | Weekly | Standard test suite (openshift/conformance/parallel) on Adaptable topology clusters |
| Serial tests | Monthly | Standard test suite (openshift/conformance/serial) on Adaptable topology clusters |
| Upgrade between z-streams | Weekly | Test upgrades on clusters running Adaptable topology |
| Upgrade between y-streams | Weekly | Test upgrades across minor versions on clusters running Adaptable topology |

### CI Tests

The `AdaptableTopology` test suite contains pre-transition and post-transition tests.

#### Pre-Transition Tests

| Test | Description |
| ---- | ----------- |
| Operator compatibility check | Verify `oc adm topology transition` correctly reports operator compatibility status |
| Topology transition validation | Verify ValidatingAdmissionPolicy prevents setting controlPlaneTopology and infrastructureTopology independently |
| Dry-run transition check | Verify `oc adm topology transition --dry-run` correctly identifies incompatibilities without applying changes |

#### Post-Transition Tests

| Test | Description |
| ---- | ----------- |
| Scale 1→2→3 control-plane nodes | Verify cluster behavior crosses thresholds correctly when adding control-plane nodes |
| Scale 3→2→1 control-plane nodes | Verify cluster behavior crosses thresholds correctly when removing control-plane nodes |
| Scale 0→1→2 worker nodes | Verify infrastructure topology adjusts correctly when adding worker nodes |
| Scale 2→1→0 worker nodes | Verify infrastructure topology adjusts correctly when removing worker nodes |
| etcd quorum management | Verify CEO correctly manages etcd scaling at the 2→3 and 3→2 control-plane thresholds |
| SLO validation tests | Verify all [Service Level Objectives](#service-level-objectives) are met during transitions and scaling operations |

### QE Testing

Standard QE testing scenarios will include:
- Installation validation on supported platforms (`platform: none` and `platform: baremetal`)
- Topology transition validation from SingleReplica
- Operator compatibility validation across first-party operators

## Graduation Criteria

### Entering Dev Preview

- Developer documentation available
- All core operators remain stable during topology transitions (may continue in SingleReplica mode)
- Adaptable topology enum added to Infrastructure config
- ValidatingAdmissionPolicy enforces paired controlPlaneTopology and infrastructureTopology updates
- Library-go utilities implemented
- OLM operator compatibility annotations implemented
- CI lanes operational for installations, transitions, and scaling

### Dev Preview -> Tech Preview

- All core operators updated to support Adaptable topology with dynamic node count awareness
- AdaptableTopology test suite validates scaling behavior
- Tests verify all core operators have compatibility annotations
- Tests verify cluster stability during node scaling operations
- Installer supports `adaptableTopology` flag in install-config with validation
- Console displays compatibility status and provides marketplace filtering
- `oc adm topology transition` command implemented
- User-facing documentation in [openshift-docs](https://github.com/openshift/openshift-docs/)
- Platform bare metal single-node support resolved or limitation documented

### Tech Preview -> GA

- Full test coverage including upgrades (y-stream and z-stream)
- [Monitoring operator updates and backhaul SLI telemetry](#monitoring-operator-and-telemetry)
- [SLOs documented and validated](#service-level-objectives)
- Support procedures documented
- Feature gate moved to `Default` feature set

### Removing a deprecated feature

N/A

## Upgrade / Downgrade Strategy

### Upgrades

Clusters using Adaptable topology follow standard OpenShift upgrade procedures.
Topology transitions are kept separate from upgrades and do not affect
upgrade expectations.

**Operator Compatibility During Upgrades:**

When operators are upgraded to versions that support Adaptable topology,
they begin responding to node count changes.
This applies whether the operator was previously marked as unknown or incompatible.

The feature gate ensures Adaptable topology is only active in supported releases.

### Downgrades

Standard OpenShift downgrade procedures apply.
Downgrading to releases without Adaptable topology support requires redeployment.
The feature gate protects against usage in unsupported releases.

## Version Skew Strategy

Adaptable topology will not be supported until core in-payload operators
are updated and labeled to support it.

Version skew could occur when operators gain Adaptable topology support in
newer versions.
The topology transition command scans for incompatibilities before
allowing transitions.
Cluster administrators should upgrade their clusters first, then transition.

The operator compatibility label provides this protection.
This is a capability not available for existing topologies.

## Operational Aspects of API Extensions

Adaptable topology uses a ValidatingAdmissionPolicy to enforce paired
`controlPlaneTopology` and `infrastructureTopology` updates.
This policy is evaluated by the API server with no additional services required.
There are no unique operational concerns.

## Support Procedures

### Team Ownership

**Edge Enablement Team:**
- CLI (`oc adm topology transition` command)
- Infrastructure config API changes
- library-go shared utilities
- Installer `adaptableTopology` flag
- Implementation of Adaptable topology in core operators

**Control Plane Team:**
- cluster-etcd-operator (CEO) etcd scaling logic

**OLM Team:**
- Operator compatibility annotations

**Console Team:**
- Console UI changes

**Monitoring Team:**
- Telemetry collection and SLI metrics

**Bare Metal Networking Team:**
- Bare metal networking for SNO clusters

**Component Teams:**
- Define correct operator behavior for different control-plane and worker node counts
- Validate Adaptable topology implementation in their operators

### Detecting Issues

**Topology Transition Failures:**
- Symptom: `oc adm topology transition adaptable` fails with compatibility errors
- Check: Run `oc adm topology transition adaptable --dry-run` to see which operators are incompatible
- Resolution: Update or remove incompatible operators before retrying

**Operators Not Responding to Node Count Changes:**
- Symptom: Operator behavior doesn't change when crossing node count thresholds
- Check: Verify operator has compatibility annotation: `operators.openshift.io/infrastructure-features: '["Adaptable"]'`
- Check: Verify operator logs for threshold crossing events
- Resolution: File a bug in OCPBUGS project with the component matching the operator's team

**etcd Scaling Failures:**
- Symptom: etcd cluster unhealthy after adding/removing control-plane nodes
- Check: CEO logs for etcd scaling operations
- Check: etcd member list: `oc -n openshift-etcd exec <etcd-pod> -- etcdctl member list`
- Resolution: File a bug in OCPBUGS project with cluster-etcd-operator component

**Forced Transitions:**
- Symptom: Cluster transitioned with `--force` flag, operators misbehaving
- Check: Search for forced topology transition cluster-scoped event
- Resolution: Follow the pattern established by the OTA team for handling forced upgrades

### Recovery Procedures

**Rollback from Failed Transition:**
- Transitions to Adaptable topology are one-way and cannot be rolled back
- Recovery requires cluster redeployment

**Recovering from etcd Scaling Failure:**
- Follow standard etcd disaster recovery procedures
- CEO should handle scaling automatically; manual intervention indicates a bug
- File a bug in OCPBUGS project with cluster-etcd-operator component

## Infrastructure Needed [optional]

No additional infrastructure is required for this feature.

CI infrastructure will experience increased demand as new test lanes are introduced to support:
- Cluster installations with Adaptable topology
- Topology transitions from single-node clusters to Adaptable topology
- Scaling cluster nodes to verify operator behavior

---

*This enhancement proposal was collaboratively developed with AI assistance (Claude Sonnet 4.5).*
