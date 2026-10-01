# Topology Considerations for Enhancement Proposals

**Purpose**: Guide enhancement authors and reviewers through the Topology Considerations section of enhancement proposals. Every EP must address how changes affect each OpenShift deployment topology.

**Key Principle**: "Not applicable" without explanation is insufficient. Authors must articulate WHY each topology is or isn't affected.

## Quick Decision Tree

| Does your enhancement affect... | Then you MUST address... |
|--------------------------------|--------------------------|
| Control plane components? | Hypershift/HCP control-plane split |
| Node-level resources? | SNO resource constraints |
| New APIs or operators? | MicroShift compatibility |
| Configuration changes? | OKE feature availability |
| None of the above? | Document why all are N/A with rationale |

## Hypershift / Hosted Control Planes

**Architecture**: Control plane runs in a management cluster; workloads run in a guest cluster.

**When to consider**: Changes to control plane components (kube-apiserver, etcd, controllers), new operators/controllers, API extensions (CRDs, webhooks), certificate/auth changes, control-plane/data-plane networking.

**Key questions**:

| Question | Why It Matters |
|----------|----------------|
| Where does this component run (management vs guest)? | Determines RBAC, networking, resource accounting |
| Does it need cross-cluster communication? | Impacts network policies and certificate management |
| What RBAC is needed in each cluster? | Management may need federated identity |
| How does upgrade orchestration work? | HCP upgrades management-side separately from guest |
| Version skew tolerance? | N→N+1 skew during rolling upgrade is critical |

**Good example**: "This enhancement adds a field to `HostedClusterStatus`. Reconciliation runs in the CPO on the management cluster. Management cluster impact: CPO resource consumption increases by ~50MB per hosted cluster. Guest cluster impact: None."

**Bad example**: "This works with Hypershift." (Missing: WHERE components run, networking implications, upgrade orchestration)

## Standalone Clusters

**Architecture**: Traditional self-hosted control plane; all components in the same cluster.

**When to consider**: Always applicable unless the enhancement is Hypershift-specific.

**Key question**: Is this Hypershift-only? If so, standalone won't have the relevant CRDs/controllers. If not, explain how the component runs identically or differently.

**Good example**: "This change applies to standalone clusters. The webhook runs as a pod in the openshift-authentication namespace. No control-plane split to handle."

## Single-Node Deployments (SNO)

**Architecture**: Single node acts as both control plane and worker. No high availability.

**Resource constraints**: ~8 vCPUs, 16-32 GB RAM (vs 3x4 vCPUs, 3x16 GB in HA). Storage depends on the storage driver and deployment configuration — SNO supports CSI-backed remote storage in addition to local volumes.

**When to consider**: New daemons/operators consuming resources, features requiring quorum or leader election, storage-intensive workloads, features assuming multiple nodes exist.

**Key questions**:

| Question | Why It Matters |
|----------|----------------|
| Per-node resource overhead (MB/CPU)? | In SNO, ALL overhead is on one node |
| Assumes multiple nodes? | Pod anti-affinity, zone spreading, quorum can't work |
| Requires high availability? | SNO has no HA; single point of failure |
| Uses local storage? | SNO often uses hostPath or local PVs |
| Behavior during node reboot? | Entire cluster goes down |

**Common scenarios**:

| Scenario | SNO Impact | Mitigation |
|----------|------------|------------|
| New DaemonSet | N*overhead becomes 1*overhead (good) | Document actual resource usage |
| Replica requirements | Can't schedule 3 replicas | Allow replica=1 via SNO detection |
| Distributed quorum | No quorum with 1 member | Use single-member mode or disable |

**Topology detection**: `oc get infrastructure cluster -o jsonpath='{.status.controlPlaneTopology}'` — a value of `SingleReplica` indicates SNO.

**Good example**: "SNO impact: This adds a DaemonSet consuming 100MB RAM and 50m CPU per node. In SNO, this is 100MB total. The feature detects single-node topology and reduces replica count from 3 to 1."

## MicroShift

**Architecture**: Minimal OpenShift for edge; subset of operators and APIs. Optimized for 2-4 GB RAM.

**Key differences from OCP**:
- **No CNO**: Network config via `/etc/microshift/config.yaml`, not `network.config.openshift.io`
- **No CVO**: Updates via rpm-ostree, not cluster-version-operator
- **Subset of operators**: Only essential operators run (Service CA, DNS, Ingress route controller)
- **Missing operators**: CNO, CVO, Marketplace, Monitoring (Prometheus)
- **Config file driven**: `/etc/microshift/config.yaml` replaces many CRs

**Key questions**: Does this depend on an operator not in MicroShift? Does this add a new API (MicroShift has limited CRDs)? Can config be exposed via config file? Memory overhead acceptable (<500MB)?

**Good example**: "MicroShift: This feature depends on CNO, which does not run in MicroShift. MicroShift users configure networking via `/etc/microshift/config.yaml`. This enhancement does not apply to MicroShift."

**Config file alternative**: "In OCP, this is set via `KubeletConfig` CR. In MicroShift, users set `kubelet.maxPods: 250` in `/etc/microshift/config.yaml`."

## OpenShift Kubernetes Engine (OKE)

**Architecture**: OKE is the OpenShift Container Platform distribution with a subscription-limited feature set. It includes the administrator console and cluster monitoring, but excludes the developer console and user-workload monitoring.

**When to consider**: Features depending on OCP-specific capabilities not included in OKE (developer console, user-workload monitoring), features using `config.openshift.io` APIs not in OKE, features requiring OpenShift-specific integrations beyond the OKE scope.

**Good example**: "OKE: This enhancement adds a developer console plugin, which depends on the OpenShift developer console. OKE includes only the administrator console, so this feature is not available in OKE."

**Reference**: [OKE comparison doc](https://docs.redhat.com/en/documentation/openshift_container_platform/latest/html/overview/oke-about)

## Reviewer Checklist

**Hypershift**:
- [ ] Identifies which cluster(s) components run in
- [ ] Addresses cross-cluster communication if applicable
- [ ] Considers version skew during upgrades
- [ ] Quantifies management cluster resource impact

**SNO**:
- [ ] Quantifies per-node memory/CPU overhead
- [ ] Addresses replica count assumptions
- [ ] Documents behavior if HA is assumed

**MicroShift**:
- [ ] Lists operator dependencies (are they in MicroShift?)
- [ ] Proposes config file alternative if adding new API
- [ ] Quantifies memory overhead (is it <500MB?)

**OKE**:
- [ ] Identifies OCP-specific dependencies
- [ ] States availability or non-availability clearly

**General**:
- [ ] No section says "N/A" without explanation
- [ ] Author articulates WHY each topology is/isn't affected

## Common Anti-Patterns

| Anti-Pattern | Problem | Better |
|-------------|---------|--------|
| "Not applicable" | No explanation given | "Not applicable — this modifies the in-cluster storage operator, which runs identically in both standalone and Hypershift guest clusters" |
| "No special considerations" | Doesn't address constraints | "SNO is supported. This Deployment consumes 200MB RAM regardless of node count. No replica adjustments needed" |
| Ignoring resource impact | Hides real cost | "Adds a 50MB sidecar to all openshift-* pods. In SNO with ~100 system pods, this is ~5GB additional. Mitigated by making sidecar opt-in" |

## Authoritative Sources

| Topic | Where to Look |
|-------|--------------|
| HCP upgrade orchestration | `enhancements/hypershift/` |
| HCP networking | `enhancements/hypershift/` (metrics exposure, monitoring EPs) |
| SNO constraints | Topology sections in various EPs |
| MicroShift differences | `enhancements/microshift/`, [openshift/microshift](https://github.com/openshift/microshift) |
| Enhancement template | [guidelines/enhancement_template.md](../guidelines/enhancement_template.md) |

## For Agentic Documentation Contributors

When documenting form factor behavior, ALWAYS cite authoritative enhancements as primary sources. Do not rely on training data for topology specifics — read relevant enhancements, extract facts, cite sources, and distinguish verified facts from logical inferences.
