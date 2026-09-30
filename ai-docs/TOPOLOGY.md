# OpenShift Deployment Topology Reference

**Purpose**: Reference guide for understanding OpenShift deployment topologies and their constraints. Used by EP authors, reviewers, and AI agents working on enhancement proposals.

**Source**: Distilled from the enhancement template's Topology Considerations section and real enhancement proposals in this repository.

---

## Quick Decision Tree

When writing or reviewing an enhancement proposal, determine which topologies are affected:

| Your Change Affects... | Must Address |
|------------------------|-------------|
| Control plane components (kube-apiserver, etcd, controllers) | Hypershift control/data plane split |
| Node-level resources (DaemonSets, kubelet config) | SNO resource constraints |
| New APIs or operators | MicroShift compatibility |
| Configuration or platform features | OKE feature availability |
| None of the above | Document WHY each topology is N/A |

---

## Hypershift / Hosted Control Planes

**Architecture**: Control plane runs in a management cluster; workloads run in a separate guest cluster.

### When to Consider

- Changes to control plane components (kube-apiserver, etcd, controllers)
- New operators or controllers
- API extensions (CRDs, webhooks)
- Certificate or authentication changes
- Networking between control plane and data plane

### Key Questions for EP Authors

| Question | Why It Matters |
|----------|----------------|
| Where does this component run? (management vs guest) | Determines RBAC, networking, resource accounting |
| Does it need cross-cluster communication? | Impacts networking policies, certificate management |
| What RBAC is needed in each cluster? | Management cluster may need federated identity |
| How does upgrade orchestration work? | HCP upgrades management-side separately from guest |
| Are there separate control/data plane versions? | Version skew tolerance becomes critical |

### Good vs Bad EP Examples

**Good**: "This enhancement is HyperShift-specific. The `controlPlaneVersion` field is added to `HostedClusterStatus`. Reconciliation runs in the CPO on the management cluster. Management cluster impact: +50MB per hosted cluster. Guest cluster impact: None."

**Bad**: "This works with Hypershift." (Missing: WHERE components run, networking implications, upgrade orchestration)

### Authoritative Sources

| Topic | Enhancement Path |
|-------|-----------------|
| HCP upgrade orchestration | `enhancements/hypershift/hypershift-control-plane-version-status.md` |
| HCP networking | `enhancements/hypershift/hosted-control-plane-metrics-exposure.md` |
| HCP operator placement | `enhancements/hypershift/node-tuning.md` |

---

## Standalone Clusters

**Architecture**: Traditional self-hosted control plane; all components in same cluster.

### When to Consider

- Always applicable unless enhancement is Hypershift-specific
- Most enhancements should work here by default

### Key Questions

- Is this Hypershift-only? (Standalone won't have HCP CRDs/controllers)
- Does it depend on Hypershift-specific features?

**Good**: "This change applies to standalone clusters. The new webhook runs as a pod in `openshift-authentication` and validates authentication CRs."

**Bad**: "No special considerations for standalone clusters." (Better: explain WHY, e.g., "Component runs identically; no control-plane split to handle.")

---

## Single-Node Deployments (SNO)

**Architecture**: Single node acts as both control plane and worker; no high availability.

### Resource Constraints

| Resource | SNO Typical | HA Typical (3-node) |
|----------|-------------|---------------------|
| CPU | 8 vCPUs | 3 x 4 vCPUs |
| Memory | 16-32 GB | 3 x 16 GB |
| Storage | Local disk only | May use distributed |

### When to Consider

- New daemons or operators that consume memory/CPU
- Features requiring quorum or leader election
- Storage-intensive workloads
- Features assuming multiple nodes exist

### Key Questions for EP Authors

| Question | Why It Matters |
|----------|----------------|
| What is the per-node resource overhead? | ALL overhead is on one node |
| Does it assume multiple nodes? | Pod anti-affinity, zone spreading, quorum won't work |
| Does it require high availability? | SNO has no HA; single point of failure |
| Does it use local storage? | Can't assume network storage |
| How does it behave during node reboot? | Entire cluster goes down |

### Common Scenarios

| Scenario | SNO Impact | Mitigation |
|----------|------------|------------|
| New DaemonSet | N x overhead becomes 1 x overhead | Document actual resource usage |
| Replica requirements | Can't schedule 3 replicas | Allow replica=1 via SNO detection |
| Distributed quorum | No quorum with 1 member | Use single-member mode or disable |
| Network partitions | Can't happen (1 node) | Simplifies some failure modes |

### Topology Detection

```bash
# Label-based detection
oc get node -l node.openshift.io/single-node-cluster

# Infrastructure topology API
oc get infrastructure cluster -o jsonpath='{.status.controlPlaneTopology}'
# Returns: SingleReplica
```

**Good**: "SNO: This adds a DaemonSet consuming 100MB RAM and 50m CPU per node. In SNO, this is 100MB total. The feature detects single-node topology and reduces replica count from 3 to 1."

**Bad**: "No special considerations for single-node deployments." (Missing: memory/CPU overhead, replica adjustments, HA assumptions)

---

## MicroShift

**Architecture**: Minimal OpenShift for edge; subset of operators and APIs; optimized for 2-4 GB RAM.

### Key Differences from OCP

| Feature | OCP | MicroShift |
|---------|-----|------------|
| Network config | CNO + `network.config.openshift.io` | Config file: `/etc/microshift/config.yaml` |
| Updates | CVO orchestration | rpm-ostree |
| Operator set | Full platform | Essential only |
| Configuration | CR-driven | `/etc/microshift/config.yaml` |
| Memory target | Not constrained | < 500 MB for platform |

### Operators Present in MicroShift

| Present | Absent |
|---------|--------|
| Service CA operator | CNO (cluster-network-operator) |
| DNS operator | CVO (cluster-version-operator) |
| Ingress operator (route controller) | Marketplace |
| | Monitoring stack (Prometheus) |
| | Machine API |
| | Console |

### Key Questions

- Does this depend on an operator not in MicroShift?
- Does this add a new API? (MicroShift has limited CRD set)
- Can config be exposed via `/etc/microshift/config.yaml`?
- What is the memory overhead? (Target: < 500 MB total)

**Good**: "MicroShift: This feature depends on CNO, which does not run in MicroShift. MicroShift users configure networking via `/etc/microshift/config.yaml`. This enhancement does not apply to MicroShift."

**Config file alternative**: "MicroShift: The `maxPods` field is added to kubelet configuration. In OCP, this is set via `KubeletConfig` CR. In MicroShift, users set `kubelet.maxPods: 250` in `/etc/microshift/config.yaml`."

### Reference

- [MicroShift repository](https://github.com/openshift/microshift)
- `enhancements/microshift/` for MicroShift-specific proposals

---

## OpenShift Kubernetes Engine (OKE)

**Architecture**: Upstream Kubernetes with minimal OpenShift additions; no platform operators.

### Key Differences from OCP

- No platform operators: monitoring, console, registry, etc.
- Upstream Kubernetes components
- Minimal `config.openshift.io` API surface

### When to Consider

- Features depending on OCP-specific operators (console, monitoring, etc.)
- Features using `config.openshift.io` APIs not in OKE
- Features requiring OpenShift-specific integrations

**Good**: "OKE: This enhancement adds a console plugin, which depends on the OpenShift web console. OKE does not include the console, so this feature is not available in OKE."

### Reference

- [OKE comparison documentation](https://docs.redhat.com/en/documentation/openshift_container_platform/latest/html/overview/oke-about)

---

## Anti-Patterns in Topology Sections

### "Not applicable" Without Explanation

**Bad**: "Not applicable to Hypershift."
**Good**: "Not applicable to Hypershift. This enhancement modifies the in-cluster storage operator, which runs identically in both standalone and Hypershift guest clusters. The management cluster is unaffected."

### "No Special Considerations"

**Bad**: "No special considerations for SNO."
**Good**: "SNO is supported. This adds a Deployment (not DaemonSet), consuming 200MB RAM total regardless of node count."

### Ignoring Resource Constraints

**Bad**: "Adds a new monitoring sidecar to all pods."
**Good**: "Adds a 50MB monitoring sidecar to all pods in `openshift-*` namespaces. In SNO with ~100 system pods, this is ~5GB additional memory (~30% increase on 16GB node). Mitigated by making the sidecar opt-in via annotation."

---

## Reviewer Checklist

### Hypershift
- [ ] Identifies which cluster(s) components run in
- [ ] Addresses cross-cluster communication if applicable
- [ ] Considers version skew during upgrades
- [ ] Quantifies management cluster resource impact

### SNO
- [ ] Quantifies per-node memory/CPU overhead
- [ ] Addresses replica count assumptions
- [ ] Documents behavior if HA is assumed

### MicroShift
- [ ] Lists operator dependencies (are they in MicroShift?)
- [ ] Proposes config file alternative if adding new API
- [ ] Quantifies memory overhead (target < 500MB)

### OKE
- [ ] Identifies OCP-specific dependencies
- [ ] Links to OKE comparison doc if unclear

### General
- [ ] No section says "N/A" without explanation
- [ ] Author can articulate WHY each topology is/isn't affected
