---
title: kubelet-haproxy-routing-bare-metal
authors:
  - "@jparrill"
reviewers:
  - "@csrwng"
  - "@sdminonne"
  - "@muraee"
  - "@bryan-cox"
approvers:
  - "@csrwng"
api-approvers:
  - "@everettraven"
creation-date: 2026-10-20
last-updated: 2026-10-20
status: provisional
tracking-link:
  - https://issues.redhat.com/browse/CNTRLPLANE-4732
  - https://issues.redhat.com/browse/CNTRLPLANE-4807
see-also: []
replaces: []
superseded-by: []
---

# Route Kubelet Through HAProxy on All Public HCP Clusters

## Summary

On public Hosted Control Planes, kubelet's bootstrap kubeconfig points directly
to the KAS LoadBalancer IP. This makes kubelet unable to survive a Day 2 LB IP
change because the old IP is hardcoded in the kubeconfig. This enhancement
changes the bootstrap kubeconfig generation to point kubelet at the HAProxy
`kube-apiserver-proxy` static pod address (`172.20.0.1:6443` by default,
configurable via `hcluster.Spec.Networking.APIServer.AdvertiseAddress`).
The change is gated behind the `KubeletHAProxyRouting`
FeatureGate (`TechPreviewNoUpgrade`) to enable iterative Day 1 development and
PoC validation before building Day 2 functionality on top
([CNTRLPLANE-4807](https://issues.redhat.com/browse/CNTRLPLANE-4807)).

## Motivation

The [Deterministic LoadBalancer IP Management](deterministic-loadbalancer-ip-management.md)
enhancement introduces Day 2 IP migration capabilities. Those mechanisms
assume all worker-to-KAS traffic flows through HAProxy so that updating
HAProxy's configuration atomically switches the target IP for all connections.

Today, kubelet on public clusters bypasses HAProxy and connects directly to the
KAS LoadBalancer IP. If the LB IP changes, kubelet retains the old IP in its
kubeconfig and becomes unresponsive — even if HAProxy is updated to point to the
new IP. This makes Day 2 IP migration impossible without full node replacement.

Private clusters route kubelet through an internal DNS name
(`api.<hcp-name>.hypershift.local`) rather than the LB IP, which sidesteps the
staleness problem. IBM Cloud ROKS follows a similar pattern — kubelet is directed
to a stable internal endpoint rather than the raw LB IP. This enhancement brings
the same resilience to all public HyperShift clusters by routing kubelet through
the node-local HAProxy static pod.

### User Stories

- As a platform operator, I want kubelet to connect through HAProxy so that a
  Day 2 LB IP change does not require full node replacement.

- As a Telco platform engineer, I want kubelet routing to be consistent across
  public and private clusters so that Day 2 IP migration works identically in
  all topologies.

### Goals

- Route kubelet through HAProxy (`172.20.0.1`) on all public HCPs.
- Maintain existing behavior on private clusters (routed via internal DNS, no change needed).
- Enable Day 2 IP migration mechanisms from the parent enhancement.

### Non-Goals

- Changing the HAProxy static pod configuration or deployment mechanism.
- Implementing Day 2 IP migration mechanisms (covered by
  [CNTRLPLANE-4799](https://issues.redhat.com/browse/CNTRLPLANE-4799) and
  [CNTRLPLANE-4800](https://issues.redhat.com/browse/CNTRLPLANE-4800)).
- DNS indirection or HAProxy resolvers (future work in parent enhancement).

## Proposal

Change `adaptBootstrapKubeconfigSecret()` in `kubeconfig.go` (lines 170-186) so
that for all public HostedClusters, the kubeconfig server URL uses
`172.20.0.1:6443` instead of the value returned by `externalURL(infraStatus)`.

The IP `172.20.0.1` (`NodeInternalAPIServerIP`) is already present in the KAS TLS
certificate SANs (`support/pki/kas.go:38`), so no certificate changes are needed.

### Workflow Description

**Before (current)**:
1. CPO generates bootstrap kubeconfig with `server: https://<KAS-LB-IP>:6443`.
2. Worker node boots, kubelet reads kubeconfig, connects directly to KAS LB IP.
3. If LB IP changes → kubelet has stale IP → node becomes NotReady.

**After (proposed, with `KubeletHAProxyRouting` FeatureGate enabled)**:
1. CPO checks if `KubeletHAProxyRouting` gate is enabled.
2. CPO generates bootstrap kubeconfig with `server: https://172.20.0.1:6443` for
   all public HCs.
3. Worker node boots, kubelet reads kubeconfig, connects to `172.20.0.1:6443`.
4. HAProxy static pod forwards the connection to the KAS LB IP.
5. If LB IP changes → HAProxy config is updated (via ignition rollout) → kubelet
   connection follows automatically.

No user action required beyond enabling the FeatureGate via
`TechPreviewNoUpgrade`. The change is transparent to the cluster administrator.

The kubeconfig change triggers a worker rollout whose behavior depends on the
NodePool upgrade strategy:
- **Replace** (`UpgradeType: Replace`): nodes are drained and replaced. The
  rollout is paced by `maxUnavailable` and `maxSurge`.
- **InPlace** (`UpgradeType: InPlace`): existing machines are upgraded in place,
  paced by `maxUnavailable`. No node replacement occurs.

### API Extensions

- **FeatureGate**: `KubeletHAProxyRouting` registered with `TechPreviewNoUpgrade`
  lifecycle in the HyperShift feature gate registry
  ([CNTRLPLANE-4807](https://issues.redhat.com/browse/CNTRLPLANE-4807)).

No new API fields, CRDs, webhooks, or finalizers beyond the FeatureGate
registration.

### Topology Considerations

#### Hypershift / Hosted Control Planes

This enhancement is exclusively for HyperShift. Only the CPO kubeconfig
generation logic is modified. The change applies to all public clusters
regardless of platform.

#### Standalone Clusters

Not applicable. Standalone clusters do not use HyperShift.

#### Single-node Deployments or MicroShift

Not applicable. SNO and MicroShift do not use HyperShift.

#### OpenShift Kubernetes Engine

No impact. OKE clusters using HyperShift benefit from this change identically.

### Implementation Details/Notes/Constraints

#### Code Change

The change is localized to `adaptBootstrapKubeconfigSecret()` in
`control-plane-operator/controllers/hostedcontrolplane/v2/kas/kubeconfig.go`:

```go
// Current (lines 170-186):
// Uses externalURL(infraStatus) for all public clusters.

// Proposed:
// When KubeletHAProxyRouting gate is enabled AND cluster is public,
// use NodeInternalAPIServerIP (172.20.0.1) instead of externalURL()
// for all platform types.
```

Public/private detection uses `netutil.IsPrivateHCP(hcp)`, already available in
the CPO reconciler context. The FeatureGate check uses the standard HyperShift
gate plumbing (`util.IsFeatureGateEnabled()`). No platform type switch is needed
— the change applies to all non-private clusters uniformly.

#### HAProxy Dependency

HAProxy is already used by pods on hosted clusters for KAS connectivity.
Making kubelet also route through HAProxy aligns both paths and makes HAProxy a
dependency for all worker-to-KAS traffic, not just pod egress. This is
acceptable because:

- HAProxy is a static pod managed by kubelet from local manifests — it starts
  without KAS connectivity and runs as long as kubelet runs.
- Private clusters avoid the staleness problem via internal DNS; this change
  brings the same resilience to public clusters via HAProxy.
- The static pod lifecycle is independent of the KAS connection — kubelet starts
  HAProxy from disk before needing API access.

#### NodeInternalAPIServerIP

`172.20.0.1` is a well-known IP in HyperShift assigned to the
`kube-apiserver-proxy` HAProxy static pod. It is:
- Already in the KAS TLS certificate SANs (`support/pki/kas.go:38`).
- Already used on bare-metal platforms (Agent, None, KubeVirt) since our
  initial implementation, now extended to all public platforms.
- Stable across cluster lifecycle — not affected by LB provider changes.

### Risks and Mitigations

| Risk | Mitigation |
|------|------------|
| HAProxy static pod fails to start → kubelet cannot reach KAS | HAProxy is a static pod that starts from local manifests before kubelet needs API access. This is the same pattern used for all static pods on OpenShift nodes. Failure is node-local; other nodes are unaffected. |
| Existing public clusters upgrade → kubeconfig changes → node rollout | The kubeconfig change produces a new ignition hash, triggering a controlled rollout (drain + replace, paced by MaxUnavailable). This is the standard upgrade path. |

### Drawbacks

- Adds HAProxy as a hard dependency for kubelet on all public clusters.
- Triggers a one-time node rollout on upgrade for existing public clusters. This
  is unavoidable for any kubeconfig change and is paced by `MaxUnavailable`.

## Alternatives (Not Implemented)

### 1. DNS-Based Endpoint

Use a DNS name instead of an IP in the kubeconfig, with CoreDNS or node-local DNS
resolving to the current KAS LB IP. Discarded:
- CoreDNS runs in the guest cluster — unavailable at first node boot
  (chicken-and-egg problem).
- Requires additional infrastructure (node-local DNS cache) not universally
  present across HyperShift platforms.

### 2. Kubelet Kubeconfig Hot-Reload

Implement a sidecar or controller that watches for KAS IP changes and rewrites
kubelet's kubeconfig on disk, then signals kubelet to reload. Discarded:
- Kubelet does not support kubeconfig hot-reload without restart.
- Restarting kubelet causes pod disruption on the node.
- Adds operational complexity for a problem that HAProxy routing solves cleanly.

## Open Questions

None. The approach is proven by private clusters (via internal DNS) and IBM Cloud
ROKS (via stable internal endpoints) and is now being extended uniformly to all
public HyperShift clusters.

## Test Plan

### Unit Tests

- `adaptBootstrapKubeconfigSecret()` returns `172.20.0.1:6443` for all public
  HCs (any platform) when `KubeletHAProxyRouting` gate is enabled.
- `adaptBootstrapKubeconfigSecret()` returns `externalURL()` for all public HCs
  when gate is disabled (existing behavior preserved).
- `adaptBootstrapKubeconfigSecret()` returns the internal DNS URL for private
  HCs regardless of gate (existing behavior preserved).

### E2E Tests

See the parent test plan ([plans/ocpstrat-2653.md](https://github.com/hypershift-community/test-results-verification/blob/main/plans/ocpstrat-2653.md),
PR #4) for the full E2E test case coverage. Relevant test cases for this
enhancement: TC-001 through TC-004, TC-033, TC-034.

## Graduation Criteria

Gated behind `KubeletHAProxyRouting` (`TechPreviewNoUpgrade`) to enable
iterative development and PoC validation in Day 1 before Day 2 work begins.

### Dev Preview -> Tech Preview

- FeatureGate `KubeletHAProxyRouting` registered (CNTRLPLANE-4807).
- `adaptBootstrapKubeconfigSecret()` routes through `172.20.0.1` when gate is on.
- Unit tests covering all platform/topology/gate combinations.
- E2E: public Agent HC with gate enabled → worker joins via HAProxy.

### Tech Preview -> GA

- Sufficient soak time on TP clusters without regressions.
- Day 2 IP migration validated on top of this change.
- User-facing documentation updated.
- Gate graduates to `Default` (always on) — the routing becomes the standard
  behavior for all public clusters.

### Removing a deprecated feature

Not applicable.

## Upgrade / Downgrade Strategy

**Upgrade**: When the `KubeletHAProxyRouting` gate is enabled on a public
cluster, the next CPO reconciliation generates the new kubeconfig.
The kubeconfig change alters the ignition content hash, triggering a worker
rollout. During rollout:
- Old workers: kubelet → KAS LB IP (direct, still works as long as IP hasn't
  changed).
- New workers: kubelet → 172.20.0.1 → HAProxy → KAS LB IP.

If the gate is not enabled, no change occurs — existing behavior preserved.

The rollout is paced by `MaxUnavailable` and follows the standard node replacement
flow.

**Downgrade**: Rolling back CPO restores `externalURL()` in the kubeconfig. A
worker rollout replaces nodes with the old-style kubeconfig. Workers revert to
direct KAS LB IP connectivity. No data loss.

## Version Skew Strategy

During upgrade, old and new workers coexist:
- Old workers: direct connection to KAS LB IP.
- New workers: HAProxy-routed connection.

Both paths reach the same KAS pods. No coordination needed. The KAS TLS
certificate already includes both the LB IP and `172.20.0.1` in its SANs.

## Operational Aspects of API Extensions

Not applicable. No API extensions introduced.

## Support Procedures

- **Detect**: If a worker node is NotReady after upgrade, check kubelet logs for
  connection errors to `172.20.0.1:6443`. Verify the HAProxy static pod is
  running: `crictl ps | grep haproxy`.
- **Diagnose**: `curl -k https://172.20.0.1:6443/healthz` from the worker node.
  If HAProxy is down, check `/etc/kubernetes/manifests/` for the static pod
  manifest.
- **Remediate**: HAProxy failures are local to the node. Restarting kubelet
  restarts the static pod. If the manifest is missing, the ignition content is
  corrupt — re-provision the node.

## Infrastructure Needed

None.
