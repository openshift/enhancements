---
title: nested-spire-support
authors:
  - "@rausingh-rh"
reviewers:
  - "@nhegde07"
  - "@PillaiManish"
  - "@tgeer"
approvers:
  - "@tgeer"
api-approvers:
  - "@tgeer"
creation-date: 2026-09-18
last-updated: 2026-10-14
status: provisional
tracking-link:
  - https://redhat.atlassian.net/browse/SPIRE-625
---

# Nested SPIRE Support for Zero Trust Workload Identity Manager

## Summary

This enhancement introduces new configuration fields to the Zero Trust Workload Identity Manager (ZTWIM) operator to support [nested SPIRE](https://spiffe.io/docs/latest/planning/scaling_spire/#nested-spire) topologies. With this enabled, a downstream server in one OpenShift cluster obtains its intermediate signing CA from an upstream server in another cluster, rather than generating its own CA. All servers in the topology share a single trust domain and form a certificate hierarchy: the upstream server holds the top-level signing CA (self-signed or chained to an external PKI), each downstream server holds an intermediate CA signed by the upstream server, and X.509 workload SVIDs chain back to a common root of trust. JWT-SVIDs are verified using signing keys propagated across the topology.

## Motivation

Organizations running multiple OpenShift clusters need workload identities that chain to a single root of trust so that cross-cluster workload authentication works transparently. Peers verify each other's SVIDs against one CA hierarchy without requiring SPIFFE federation between separate trust domains.

Support for nested SPIRE topologies enables scalable, highly available workload identity management across multi-cluster environments without a shared global datastore. It allows downstream servers to issue identities under a single trust domain, improving resilience and segmenting failure domains across distinct infrastructure boundaries.

### User Stories

- As an OpenShift cluster administrator, I want to configure a local SPIRE server to use an upstream server as its authority, so that workload identities across clusters chain back to a common root of trust instead of using isolated self-signed CAs.
- As an OpenShift cluster administrator, I want the operator to manage all infrastructure that nested SPIRE requires, so that I do not have to hand-assemble and maintain those resources myself.
- As an OpenShift cluster administrator of the upstream cluster, I want to onboard a downstream cluster declaratively, so that the upstream server issues an intermediate signing CA to that cluster.
- As an SRE, I want to see the health of nested SPIRE through status conditions on each cluster, so that I can detect and remediate configuration or connectivity problems at scale.
- As an OpenShift security engineer, I want enabling nested SPIRE to maintain the least-privilege posture for all components involved, so that the feature does not introduce unnecessary privilege escalation.

### Goals

1. Allow a **downstream cluster** to obtain its intermediate signing CA from an upstream server, with the operator managing all required infrastructure end to end.
2. Allow the **upstream cluster's operator** to expose the upstream server's gRPC API so downstream clusters can reach it over the network.
3. Allow an **upstream cluster's operator** to register one or more downstream clusters for CA chaining through CR configuration.
4. Support **N-level nesting**: a cluster may be both upstream and downstream simultaneously.
5. Garbage-collect all operator-created nested SPIRE resources when the feature is removed from the CR.
6. Report nested SPIRE health through status conditions so administrators can detect and remediate problems.

### Non-Goals

1. **Automating cross-cluster credential distribution using ZTWIM operator.** The administrator manually provisions the required credentials and trust material on each cluster. See [Workflow Description](#workflow-description) for details.
2. **Node attestors other than** `k8s_psat`**.** Only projected service account token attestation is supported in this iteration.
3. **Automatic cross-cluster discovery.** Each upstream-downstream link must be configured explicitly on both clusters.

## Proposal

**Key terms**

1. *Upstream cluster*: a cluster whose SPIRE server signs intermediate CAs for downstream servers.
2. *Downstream cluster*: a cluster whose SPIRE server holds an intermediate CA from an upstream server.
3. *Mid-tier cluster*: a cluster that is both upstream and downstream simultaneously.
4. *Upstream agent*: a dedicated SPIRE agent deployed on the downstream cluster, attested against the upstream server. It is distinct from the existing `spire-agent` DaemonSet that serves workloads.

**Supported topologies.** A nested topology is a tree of SPIRE servers sharing one trust domain. Each downstream chains to exactly one upstream; an upstream may register many downstreams. All clusters share the same trust bundle, so workloads in any tier can authenticate workloads in any other tier. The root server's CA may also come from external PKI (cert-manager, Vault) rather than being self-signed; in that case, the external CA's path length constraint must allow at least one subordinate CA per nesting level.

**On the upstream cluster**, the operator exposes the SPIRE server's gRPC API over the network, configures the server to recognize agents from each registered downstream cluster, so downstream cluster's SPIRE server can request an intermediate CA.

**On the downstream cluster**, the operator:

1. Deploys a dedicated upstream agent that authenticates with the upstream server.
2. Connects the upstream agent's Workload API socket to the downstream server via a second CSI driver.
3. Configures the downstream server to obtain its signing CA from the upstream server.

A dedicated agent and a second CSI driver are needed because the existing workload-facing agent is attested to the local server (not the upstream one). The full list of resources and the design rationale are in [Implementation Details](#implementation-detailsnotesconstraints) and [Alternatives](#alternatives-not-implemented).

### Workflow Description

**Starting state:** Both clusters have ZTWIM installed.

The workflow has four phases. Phases 1 and 3 are **manual** administrator steps. Phases 2 and 4 are **operator-driven** reconciliation.

**Phase 1 - Manual: prepare cross-cluster credentials**

1. Create a kubeconfig that allows the upstream cluster's SPIRE server to reach the downstream cluster's API server with sufficient permissions for TokenReview and `get` on `pods` and `nodes`.
2. On the upstream cluster, store that kubeconfig in a Secret in the operator namespace.

**Phase 2 - Operator-driven: configure the upstream cluster**

1. Administrator sets `spec.grpcEndpoint` and `spec.nestedSpire` on the upstream SpireServer CR, referencing the kubeconfig Secret for each downstream cluster.
2. The operator exposes the upstream server's gRPC API over the network, configures it to recognize agents from the downstream cluster, creates registration entries that authorize the downstream cluster to request an intermediate CA, and restarts the upstream server to apply the changes.

**Phase 3 - Manual: bootstrap trust bundle (one-time)**

1. Copy the upstream cluster's root trust bundle from the `spire-bundle` ConfigMap.
2. On the downstream cluster, create a Secret containing the trust bundle PEM.

**Phase 4 - Operator-driven: configure the downstream cluster**

1. Administrator sets `spec.upstreamAuthority.spire` on the downstream SpireServer CR, referencing the trust bundle Secret and upstream server address.
2. The operator deploys the upstream agent on the downstream cluster, connects the upstream agent's Workload API socket to the downstream server, configures the downstream server to obtain its signing CA from the upstream server, and restarts the downstream server to apply the changes.

**Runtime (automatic):**

1. The upstream agent attests to the upstream server using `k8s_psat`. The upstream server validates the agent's projected SA token via TokenReview against the downstream cluster's API server.
2. The downstream server's UpstreamAuthority spire plugin obtains an SVID from the upstream agent's Workload API socket and uses it to authenticate a gRPC call to the upstream server, requesting an intermediate CA.
3. The downstream server activates the upstream-signed CA and issues workload SVIDs chaining to the shared root.

```mermaid
sequenceDiagram
    participant Admin as Administrator (manual)
    participant UpOp as Upstream Operator
    participant UpSrv as Upstream SPIRE Server
    participant DownOp as Downstream Operator
    participant UpAgent as Upstream Agent
    participant DownSrv as Downstream SPIRE Server

    rect rgb(255, 245, 230)
    Note over Admin: Phase 1 - Manual
    Admin->>Admin: Create kubeconfig for upstream-to-downstream API access
    Admin->>Admin: Store kubeconfig as Secret on upstream cluster
    end

    rect rgb(230, 245, 255)
    Note over UpOp: Phase 2 - Operator-driven
    Admin->>UpOp: Set spec.grpcEndpoint + spec.nestedSpire on upstream CR
    UpOp->>UpOp: Expose upstream server gRPC, configure downstream attestation
    UpOp->>UpSrv: Restart upstream server
    end

    rect rgb(255, 245, 230)
    Note over Admin: Phase 3 - Manual (one-time)
    Admin->>Admin: Copy root trust bundle from upstream cluster
    Admin->>Admin: Store trust bundle as Secret on downstream cluster
    end

    rect rgb(230, 245, 255)
    Note over DownOp: Phase 4 - Operator-driven
    Admin->>DownOp: Set spec.upstreamAuthority.spire on downstream CR
    DownOp->>DownOp: Deploy upstream agent, connect socket to downstream server
    DownOp->>DownSrv: Restart downstream server
    end

    rect rgb(230, 255, 230)
    Note over UpAgent,DownSrv: Runtime - Automatic
    UpAgent->>UpSrv: Upstream agent sends projected SA token (k8s_psat attestation)
    UpSrv->>DownSrv: Upstream server sends TokenReview to downstream API server
    DownSrv-->>UpSrv: Downstream API server validates token
    UpSrv-->>UpAgent: Upstream server issues agent SVID
    DownSrv->>UpAgent: Downstream server fetches SVID from upstream agent socket
    DownSrv->>UpSrv: Downstream server requests intermediate CA from upstream server
    UpSrv-->>DownSrv: Upstream server returns signed intermediate CA + trust bundle
    DownSrv->>DownSrv: Downstream server activates upstream-signed CA
    end
```

**Day-2 operations:**

- **Adding another downstream cluster.** Append an entry to `spec.nestedSpire.downstreamClusters`. Regenerating `k8s_psat` configuration restarts the upstream server briefly; existing downstreams keep issuing from their cached intermediate CAs.
- **Removing a downstream cluster.** On the downstream cluster, clearing `spec.upstreamAuthority.spire` removes all operator-created nested SPIRE resources. On the upstream cluster, the operator removes the downstream cluster's attestation configuration and registration entries.
- **Upstream outage.** The downstream server keeps issuing from its cached intermediate CA. SPIRE retries CA preparation automatically until upstream returns.
- **Node drain.** Both `spire-server` and the upstream agent are evicted. When both are rescheduled, pod affinity naturally places the agent on the same node as the server. No operator intervention is needed.
- **Server-only reschedule.** If only `spire-server` moves to a different node (e.g., pod eviction without node drain), the upstream agent stays running on the old node. The operator detects the node mismatch during reconciliation, deletes the agent pod, and pod affinity places the replacement on the server's new node. CA rotation stalls during this window but workload issuance continues from the cached intermediate CA.

### API Extensions

This enhancement modifies the existing cluster-scoped `SpireServer` CRD. It does not add new CRDs, admission webhooks, conversion webhooks, aggregated API servers, or finalizers.

#### Downstream cluster: `spec.upstreamAuthority.spire`

`UpstreamAuthorityConfig` today requires exactly one of `certManager` or `vault`. This adds `spire` as a third option.

```go
// UpstreamAuthorityConfig selects and configures an UpstreamAuthority plugin.
// Exactly one of certManager, vault, or spire must be set.
// +kubebuilder:validation:XValidation:rule="(has(self.certManager) && !has(self.vault) && !has(self.spire)) || (!has(self.certManager) && has(self.vault) && !has(self.spire)) || (!has(self.certManager) && !has(self.vault) && has(self.spire))",message="exactly one of certManager, vault, or spire must be set"
type UpstreamAuthorityConfig struct {
    // certManager and vault are unchanged.

    // spire configures the SPIRE UpstreamAuthority plugin.
    // +kubebuilder:validation:Optional
    Spire *UpstreamAuthoritySpire `json:"spire,omitempty"`
}

// UpstreamAuthoritySpire configures nested SPIRE on a downstream cluster.
type UpstreamAuthoritySpire struct {
    // serverAddress is the DNS hostname of the upstream SPIRE server's gRPC endpoint.
    // Must not include a URL scheme, port, or path.
    // +kubebuilder:validation:Required
    // +kubebuilder:validation:MinLength=1
    // +kubebuilder:validation:MaxLength=253
    // +kubebuilder:validation:Pattern=`^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$`
    // +kubebuilder:validation:XValidation:rule="!self.contains('://') && !self.contains(':') && !self.contains('/')",message="serverAddress must be a hostname without scheme, port, or path"
    ServerAddress string `json:"serverAddress"`

    // serverPort is the port of the upstream gRPC endpoint.
    // +kubebuilder:default=443
    // +kubebuilder:validation:Minimum=1
    // +kubebuilder:validation:Maximum=65535
    // +kubebuilder:validation:Optional
    ServerPort int32 `json:"serverPort,omitempty"`

    // trustBundleSecret is the name of a Secret in the operator namespace that
    // holds the upstream SPIRE server's trust bundle. The Secret must contain
    // data key bundle.crt with PEM-encoded X.509 root certificate(s).
    // +kubebuilder:validation:Required
    TrustBundleSecret string `json:"trustBundleSecret"`
}
```

#### Upstream cluster: `spec.grpcEndpoint` and `spec.nestedSpire`

```go
type GRPCEndpointConfig struct {
    // managedRoute enables or disables automatic Route creation for the gRPC endpoint.
    // +kubebuilder:default:="true"
    // +kubebuilder:validation:Enum:="true";"false"
    // +kubebuilder:validation:Optional
    ManagedRoute string `json:"managedRoute,omitempty"`
}

// +kubebuilder:validation:XValidation:rule="!has(self.nestedSpire) || has(self.grpcEndpoint)",message="grpcEndpoint is required when nestedSpire is set"
type SpireServerSpec struct {
    // ... existing fields ...

    // grpcEndpoint exposes the SPIRE server's gRPC API.
    // +kubebuilder:validation:Optional
    GRPCEndpoint *GRPCEndpointConfig `json:"grpcEndpoint,omitempty"`

    // nestedSpire registers downstream clusters for node attestation.
    // Requires grpcEndpoint to be set.
    // +kubebuilder:validation:Optional
    NestedSpire *NestedSpireConfig `json:"nestedSpire,omitempty"`
}

type NestedSpireConfig struct {
    // downstreamClusters enumerates clusters whose SPIRE servers chain to this one.
    // +listType=map
    // +listMapKey=name
    // +kubebuilder:validation:Optional
    DownstreamClusters []DownstreamCluster `json:"downstreamClusters,omitempty"`
}

type DownstreamCluster struct {
    // name identifies the downstream cluster. Immutable once set.
    // Must match the downstream cluster's ZeroTrustWorkloadIdentityManager.spec.clusterName.
    // +kubebuilder:validation:Required
    // +kubebuilder:validation:MinLength=1
    // +kubebuilder:validation:MaxLength=63
    // +kubebuilder:validation:Pattern=`^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$`
    // +kubebuilder:validation:XValidation:rule="self == oldSelf",message="name is immutable"
    Name string `json:"name"`

    // kubeConfigSecret is the name of a Secret in the operator namespace
    // holding a kubeconfig for the downstream cluster's API server.
    // +kubebuilder:validation:Required
    KubeConfigSecret string `json:"kubeConfigSecret"`
}
```

### Topology Considerations

#### Hypershift / Hosted Control Planes

Not supported.

#### Standalone Clusters

Supported. This is the primary target topology.

#### Single-node Deployments or MicroShift

Not supported.

#### OpenShift Kubernetes Engine

Not applicable.

### Implementation Details/Notes/Constraints

#### Why a dedicated upstream agent and a second CSI driver

The `UpstreamAuthority spire` plugin fetches an SVID from a local Workload API Unix socket. There is no option to bypass the socket. The existing `spire-agent` DaemonSet cannot provide that SVID because it is attested to the *local* SPIRE server. A second agent, attested to the upstream server, is required.

Because the upstream agent runs in a separate pod, a second CSI driver (`upstream.csi.spiffe.io`) bind-mounts the socket into the SPIRE server pod, following the same pattern ZTWIM already uses for workload sockets. This keeps the SPIRE server pod on the `restricted` SCC. Alternative approaches (sidecar, shared `hostPath`) are in the [Alternatives](#alternatives-not-implemented) section.

#### Upstream agent co-location with SPIRE server

The upstream agent is a one-replica Deployment with `requiredDuringSchedulingIgnoredDuringExecution` pod affinity to the SPIRE server pod. Kubernetes does not support `RequiredDuringExecution` for pod affinity, so if the server moves to a different node, the agent stays behind.

The operator restores co-location automatically:

1. The `SpireServer` controller watches the `StatefulSet`. When `spire-server-0` is recreated, the StatefulSet status change triggers reconciliation.
2. The controller compares the server pod's `spec.nodeName` with the upstream agent pod's `spec.nodeName`.
3. On mismatch, the controller deletes the agent pod. Pod affinity places the replacement on the server's current node.

During the reschedule window, CA rotation stalls but workload issuance from the cached intermediate continues.

#### Resources created by the operator on the downstream cluster


| Resource                           | Kind                         | Purpose                                                                | SCC                     |
| ---------------------------------- | ---------------------------- | ---------------------------------------------------------------------- | ----------------------- |
| `spire-agent-upstream`             | ServiceAccount               | Upstream agent identity                                                |                         |
| `spire-agent-upstream`             | ClusterRole + Binding        | `get` on `pods`, `nodes`, `nodes/proxy`                                |                         |
| `spire-agent-upstream`             | ConfigMap                    | Generated `agent.conf`                                                 |                         |
| `spire-agent-upstream`             | Deployment (1 replica)       | Upstream agent, pod affinity to SPIRE server                           | `spire-agent-upstream`  |
| `spire-agent-upstream`             | SecurityContextConstraints   | Same privileges as existing `spire-agent`, scoped to upstream agent SA |                         |
| `upstream.csi.spiffe.io`           | CSIDriver                    | Second driver, `csi-ephemeral-volume-profile: restricted`              |                         |
| `spire-spiffe-csi-driver-upstream` | SA + RoleBinding + DaemonSet | Projects upstream agent socket into SPIRE server pod                   | `privileged` (existing) |
| `spire-server`                     | StatefulSet (modified)       | Gains CSI volume at `/run/spire/upstream-agent`                        | `restricted`, unchanged |


#### Resources created by the operator on the upstream cluster

When `managedRoute` is `"true"`, the operator creates a passthrough Route with host `grpc.<clusterName>.<trustDomain>`. The Route must use passthrough TLS because SPIRE requires mutual TLS with SPIFFE IDs; edge or re-encrypt termination would present the router's certificate instead of the SPIRE server's SVID. When `"false"`, the administrator manages the Route.


| Resource                                          | Kind                   | Purpose                                                        |
| ------------------------------------------------- | ---------------------- | -------------------------------------------------------------- |
| `spire-server-grpc`                               | Route                  | Passthrough Route to `spire-server` Service `grpc` port        |
| `spire-server`                                    | ConfigMap (modified)   | `k8s_psat` clusters map gains one entry per downstream cluster |
| `spire-server`                                    | StatefulSet (modified) | Mounts kubeconfig Secrets for each downstream cluster          |
| `nested-spire-<downstreamClusterName>-node-alias` | ClusterStaticEntry     | Node alias for the downstream cluster's upstream agents        |
| `nested-spire-<downstreamClusterName>-downstream` | ClusterStaticEntry     | `downstream: true` entry for intermediate CA minting           |


#### Generated registration entries

For each downstream cluster (e.g., `cluster01` in trust domain `example.com`), the operator generates two `ClusterStaticEntry` objects. `ClusterStaticEntry` is an existing CRD owned by `spire-controller-manager`.

**Node alias** (`nested-spire-cluster01-node-alias`): provides a stable parent SPIFFE ID so that child entries do not track per-node agent IDs that change on node replacement.

```yaml
apiVersion: spire.spiffe.io/v1alpha1
kind: ClusterStaticEntry
metadata:
  name: nested-spire-cluster01-node-alias
spec:
  className: zero-trust-workload-identity-manager-spire
  parentID: spiffe://example.com/spire/server
  spiffeID: spiffe://example.com/downstream/cluster01
  selectors:
  - k8s_psat:cluster:cluster01
  - k8s_psat:agent_ns:<operator-namespace>
  - k8s_psat:agent_sa:spire-agent-upstream
```

**Downstream server entry** (`nested-spire-cluster01-downstream`): authorizes the downstream server to request an intermediate CA. The `downstream: true` flag grants the SPIRE `Downstream` role.

```yaml
apiVersion: spire.spiffe.io/v1alpha1
kind: ClusterStaticEntry
metadata:
  name: nested-spire-cluster01-downstream
spec:
  className: zero-trust-workload-identity-manager-spire
  parentID: spiffe://example.com/downstream/cluster01
  spiffeID: spiffe://example.com/downstream/cluster01/spire-server
  selectors:
  - k8s:ns:<operator-namespace>
  - k8s:sa:spire-server
  - k8s:pod-name:spire-server-0
  - k8s:container-name:spire-server
  downstream: true
  x509SVIDTTL: 1h
```

#### Status conditions

Downstream:

- `UpstreamAgentAvailable`: upstream agent Deployment has an available replica, trustBundleSecret exists. Reasons: `Ready`, `DeploymentNotReady`, `TrustBundleMissing`.
- `UpstreamCSIDriverAvailable`: upstream CSI DaemonSet is fully ready. Reasons: `Ready`, `DaemonSetNotReady`.

Upstream:

- `GRPCEndpointAvailable`. Route exists with the expected spec. Reasons: `RouteCreated`, `RouteCreationFailed`, `RouteDisabled`.
- `DownstreamClustersValid`: for each `downstreamClusters[]` entry, kubeconfig Secret exists, operator-created `ClusterStaticEntry` objects exist, and `spire-controller-manager` has set `status.rendered=true` and `status.masked=false` on both. Reasons: `Ready`, `KubeConfigMissing`, `EntryNotApplied` (`status.rendered=false`), `EntryMasked` (`status.masked=true`)

#### Constraints

1. Every cluster in a hierarchy must share the same `trustDomain`.
2. `k8s_psat` only in this iteration. Each upstream cluster must reach every registered downstream cluster's API server.
3. Kubeconfigs and trust bundles are administrator-provided Secrets, not generated by the operator.
4. When the root CA comes from external PKI, its path length constraint must allow at least one subordinate CA per nesting level.
5. **Day-0 enablement is recommended.** Nested SPIRE is best enabled before the downstream server issues any workload SVIDs (day 0). A server that already holds a valid self-signed CA (day 2) continues using it until the next CA rotation. This is SPIRE's standard behavior and applies equally to the `certManager` and `vault` plugins. While the stale self-signed CA is active, workloads receive SVIDs that do not chain to the shared root. To force immediate use of the upstream-signed CA on a day-2 cluster, re-initialize the datastore: scale the StatefulSet to zero, delete its PVC, and scale back to one. The operator does not perform this destructive step automatically.
6. The plugin propagates X.509 CA chain and JWT signing keys only, not federated trust bundles ([spiffe/spire#4128](https://github.com/spiffe/spire/issues/4128)). Downstream clusters needing federation must configure their own `ClusterFederatedTrustDomain`.

#### Network policies for nested SPIRE

**Upstream agent** (`spire-agent-upstream`, downstream cluster): default-deny baseline plus egress to the upstream server gRPC port (`serverPort`, default 443/TCP), Kubernetes API server (6443/TCP), kubelet (10250/TCP for workload attestation), and DNS (5353 TCP+UDP). Ingress for metrics (9402/TCP from openshift-monitoring). No other ingress needed; the downstream server accesses the upstream agent via Unix socket (CSI mount).

**Upstream CSI driver**: no NetworkPolicy required (Unix domain sockets only).

**SpireServer - new capability gates:**

| Capability | Gate | Effect | Ports |
|------------|------|--------|-------|
| Upstream authority (downstream) | `spec.upstreamAuthority.spire != nil` | Feature-egress | `serverPort` (default 443/TCP) |
| gRPC endpoint (upstream) | `spec.grpcEndpoint != nil` | Ingress from `openshift-ingress` | 8081/TCP |
| Downstream attestation (upstream) | `spec.nestedSpire.downstreamClusters` non-empty | Feature-egress | Downstream API server ports (typically 6443/TCP) |

### Risks and Mitigations
| Risk                                                                                                                                                         | Impact                                        | Mitigation                                                                                                                                                           |
| ------------------------------------------------------------------------------------------------------------------------------------------------------------ | --------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Upstream cluster holds `TokenReview` + pod/node read credentials on every downstream cluster                                                                 | High: lateral movement across fleet           | Bind kubeconfig to a SA with exactly `system:auth-delegator` + `get` on `pods` and `nodes`                                                                           |
| Upstream outage stops CA rotation on downstream clusters                                                                                                     | High, time-delayed                            | Downstream keeps issuing from cached intermediate until it expires; SPIRE retries automatically                                                                      |
| Trust domain or cluster name mismatch across clusters                                                                                                        | Medium: opaque attestation failure            | Three distinct failure modes with diagnosable errors (see [Support Procedures](#support-procedures))                                                                 |
| SPIRE server rescheduled while upstream agent stays behind                                                                                                   | Medium: CA rotation stalls                    | Operator detects node mismatch during reconcile and deletes upstream agent pod                                                                                       |
| Upstream agent cannot schedule on SPIRE server's node                                                                                                        | Medium: CA rotation blocked                   | `UpstreamAgentAvailable` reports status                                                                                                                              |
| Kubeconfig Secret targets wrong cluster's API server                                                                                                         | Medium: `invalid bearer token` at TokenReview | Document that kubeconfig must match `downstreamClusters[].name`; error is diagnosable via server logs                                                                |
| No enforced limit on nesting depth. Each tier adds an intermediate CA to the chain, increasing TLS handshake size and blast radius of a mid-tier compromise. | Medium                                        | SPIRE imposes no depth limit. With external PKI, the root certificate's path length constraint caps the maximum depth; set it to at least the number of nesting levels required by the topology. Tested up to 2 levels of nesting depth. Deeper topologies are untested. |
| Each `downstreamClusters` change rolls the upstream server                                                                                                   | Low: brief unavailability                     | Batch onboarding during maintenance windows                                                                                                                          |
| Impersonation of downstream server via `pods/create`                                                                                                         | High: intermediate CA minting                 | `k8s:pod-name:spire-server-0` blocks duplicates while real pod exists; restrict `pods/create` in operator namespace                                                  |

### Drawbacks

**Operational complexity.** Each upstream-downstream link requires configuration on both clusters and manual cross-cluster credential management. An upstream cluster holds N kubeconfig credentials and restarts its SPIRE server when the downstream list changes, increasing the blast radius.

**Maintenance burden.** The operator must track SPIRE's nesting behavior across upstream SPIRE versions. Changes to the UpstreamAuthority plugin, k8s_psat attestor, or downstream registration semantics could require operator updates.

## Alternatives (Not Implemented)

### 1. Sidecar container

Upstream agent as a second container in the `spire-server` pod, sharing an `emptyDir` for the socket. Removes the Deployment, CSI driver, and pod affinity. **Not chosen**: both containers share the `spire-server` ServiceAccount. The upstream agent needs `pods`, `nodes`, and `nodes/proxy` RBAC for attestation, which would grant those permissions to the SPIRE server SA that currently does not need them. A separate Deployment keeps two ServiceAccounts, stable selectors, and independent lifecycle.

### 2. Raw `hostPath`

Simplest socket delivery. **Rejected**: a `hostPath` mount moves the SPIRE server off `restricted` SCC; SELinux MCS labels prevent cross-pod socket access without broader SCC grants.

### 3. Reuse the existing `spire-agent` DaemonSet

**Not possible**: it is attested to the local SPIRE server, which is the server that needs the upstream-signed CA.

### 4. Share the existing `spire-agent` SCC

**Rejected**: the `spire-agent` SCC's `Users` list is reconciled by the `SpireAgent` controller. If the `SpireServer` controller adds the upstream agent SA to the same SCC, the `SpireAgent` controller would overwrite the `Users` list on its next reconcile and remove it. Avoiding this would require both controllers to coordinate writes to a shared SCC, adding coupling between the two controllers.

### 5. `x509pop` node attestation

**Deferred**: needs certificate provisioning, distribution, and rotation on both clusters.

### 6. `join_token` node attestation

**Rejected**: tokens are single-use and produce a non-re-attestable agent.

### 7. Upstream agent as a DaemonSet

**Rejected**: only the agent on the SPIRE server's node is useful. N-1 extra agents waste resources and multiply attestation load.

### 8. SPIRE federation instead of nesting

**Different capability**: federation joins distinct trust domains with separate roots; nesting produces one trust domain with one root.

## Open Questions [optional]

1. **Upstream agent as Deployment (1 replica) vs. DaemonSet.** The current proposal uses a single-replica Deployment with pod affinity to co-locate with the SPIRE server pod, plus operator-assisted restoration when co-location breaks. A DaemonSet is simpler (no co-location logic) but wastes resources on every node where no SPIRE server runs and multiplies attestation load on the upstream server. The Deployment approach is more resource-efficient but adds operator complexity.
2. **Maximum downstream clusters per upstream.** Each downstream cluster adds a kubeconfig Secret mount and a `k8s_psat` entry to the upstream server, and every change to the `downstreamClusters` list rolls the StatefulSet. Is there a practical ceiling that should be documented or enforced?
3. **Dedicated** `spire-agent-upstream` **SCC vs. sharing existing** `spire-agent` **SCC.** Current proposal: dedicated SCC to avoid cross-controller coordination. Consolidation possible later.

## Test Plan

### Unit tests

- CEL validation: exactly one of `certManager`/`vault`/`spire`; `nestedSpire` requires `grpcEndpoint`; `downstreamClusters[].name` immutability.
- Config generation: downstream `UpstreamAuthority` block, upstream `k8s_psat` clusters map, node alias and downstream entry pairs.
- Status condition transitions.

### Integration tests

- CRUD lifecycle for `spec.upstreamAuthority.spire`, `spec.grpcEndpoint`, and `spec.nestedSpire`: assert resource creation, update, and garbage collection.
- SCC lifecycle: enable creates `spire-agent-upstream` SCC; disable removes it.

### End-to-end tests

Requires at least two clusters; mid-tier scenarios need three.

- **Happy path**: upstream + downstream; assert intermediate CA, shared-root SVIDs, cross-cluster authentication.
- **Multiple downstreams**: two downstreams on one upstream; assert independent CA issuance.
- **Mid-tier**: A → B → C; assert B is both upstream and downstream, cross-tier authentication works.
- **Cleanup**: remove config; assert all operator-created resources deleted.
- **Node drain**: assert server and agent reschedule together, CA rotation continues.
- **Server-only reschedule**: assert operator detects mismatch, deletes agent pod, pod affinity places replacement on server's node, CA rotation resumes.
- **Upstream outage**: assert downstream keeps issuing from cached CA.
- **Misconfiguration**: trust domain mismatch, cluster name mismatch, missing trust bundle, invalid kubeconfig.

## Graduation Criteria

### Dev Preview

- Downstream receives intermediate CA from upstream (certificate chain verification).
- Mid-tier topology works (three-cluster chain).
- Multiple downstreams per upstream.
- Resource cleanup on removal.
- Status conditions report health.

### Dev Preview -> Tech Preview

- Upgrade and failure scenario test coverage.
- End user documentation.
- Open questions settled; `k8s_psat`-only scope confirmed.
- Enumerate SLIs (e.g., CA rotation latency, upstream agent attestation success rate)
- Gather feedback from users deploying multi-cluster topologies.

### Tech Preview -> GA

- End-to-end tests pass for all supported topologies.
- Load testing at maximum downstream cluster count.
- All open questions resolved.
- Upgrade and downgrade tested.
- Document SLOs for CA rotation and upstream agent availability.
- Alerts and metrics implemented.
- User-facing documentation created for single-upstream and multi-tier procedures.

### Removing a deprecated feature

Not applicable.

## Upgrade / Downgrade Strategy

**Upgrade**: no action required. New fields are optional and absent by default.

**Adopting the feature**: set `spec.upstreamAuthority.spire` and wait for `UpstreamAgentAvailable=True`. If the server still logs `self_signed=true`, re-initialize the datastore (scale StatefulSet to 0, delete PVC, scale to 1). This invalidates all SVIDs and is a maintenance-window operation. The same applies to `certManager` and `vault` plugins.

**Downgrade**: orphans generated resources. The older operator regenerates the ConfigMap without the plugin; the server self-signs after the current CA expires. Delete orphaned resources by label and re-initialize if immediate self-signed CA is needed.

## Version Skew Strategy

Within a cluster, the upstream agent uses the same SPIRE image the operator ships. The consequential skew is between clusters: SPIRE requires [agents not newer than the oldest server, and at most one minor version older](https://github.com/spiffe/spire/blob/main/doc/upgrading.md). The upstream agent runs in the downstream cluster but talks to the upstream server, so **upgrade from the root outward** (each tier before the one below it).

## Operational Aspects of API Extensions

- **CRD changes**: three optional fields on `SpireServer`.
- `ClusterStaticEntry`: two per downstream cluster. Processed by existing `spire-controller-manager`.
- **Route**: one passthrough Route per upstream cluster.

Health indicators (SLIs):

- `UpstreamAgentAvailable` and `UpstreamCSIDriverAvailable` conditions on the downstream `SpireServer` CR.
- `GRPCEndpointAvailable` and `DownstreamClustersValid` conditions on the upstream `SpireServer` CR.

Impact is validated by e2e tests in CI that assert these conditions reach `Ready` across supported topologies.

Failure modes:

- Externally deleted `ClusterStaticEntry` objects are recreated on next reconcile. Downstream CA minting pauses until restored; workload issuance continues from cached CAs.
- Externally deleted Route is recreated. Downstream clusters cannot reach upstream until restored.
- Externally deleted `spire-agent-upstream` SCC prevents upstream agent startup until recreated.

The ZTWIM team owns all components and is the escalation point.

## Support Procedures

Primary diagnostic, downstream server CA report:

```bash
oc logs spire-server-0 -c spire-server -n zero-trust-workload-identity-manager | grep "X509 CA"
```

A healthy nested downstream logs `X509 CA prepared` with `self_signed=false` and a non-empty `upstream_authority_id`. After initial setup, verify every tier's logs for `self_signed=false` .

### Symptom → Cause → Action


| Symptom                                             | Likely Cause                                       | Action                                                              |
| --------------------------------------------------- | -------------------------------------------------- | ------------------------------------------------------------------- |
| `self_signed=true` in server logs                   | Server holds valid self-signed CA, ignoring plugin | Re-initialize datastore (scale to 0, delete PVC, scale to 1)        |
| `no X.509 bundle found for trust domain`            | Trust domain mismatch between clusters             | Ensure all clusters share the same `trustDomain`                    |
| `not configured for cluster "<name>"`               | Cluster name not registered on upstream            | Verify `downstreamClusters[].name` matches downstream `clusterName` |
| `invalid bearer token`                              | Kubeconfig points to wrong API server              | Fix kubeconfig Secret to target correct downstream cluster          |
| `certificate contains no URI SAN`                   | Route hostname mismatch or not passthrough         | Ensure Route host matches `serverAddress`, passthrough TLS          |
| `authentication handshake failed: EOF`              | Upstream server unreachable                        | Check upstream server and Route                                     |
| `403 Forbidden` from workload attestor              | SA lacks `nodes/proxy`                             | Verify `spire-agent-upstream` ClusterRole                           |
| Pod in `ContainerCreating`                          | CSI DaemonSet not running or no agent socket       | Check `UpstreamCSIDriverAvailable` and agent pod                    |
| `too many intermediates for path length constraint` | External PKI CA has pathLen 0                      | Reissue CA with pathLen >= nesting levels                           |

### Disabling nested SPIRE

Removal and fallback behavior is described in [Workflow Description](#workflow-description) and [Upgrade / Downgrade Strategy](#upgrade--downgrade-strategy). Impact on workloads:

- **Existing workloads**: SVIDs remain valid until their individual TTL expires. No disruption.
- **New workloads**: receive SVIDs signed by the self-signed CA once the current intermediate expires.
- **Re-enabling**: operator recreates all resources; SPIRE re-establishes the CA chain on the next rotation cycle without manual intervention.

## Infrastructure Needed [optional]

Multi-cluster CI lane: two clusters for single-upstream scenarios, three clusters for mid-tier (A → B → C). 