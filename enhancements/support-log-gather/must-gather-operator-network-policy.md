---
title: must-gather-operator-network-policy
authors:
  - "@shivprakashmuley"
reviewers:
  - "@TrilokGeer"
  - "@Prashanth684"
approvers:
  - "@TrilokGeer"
  - "@Prashanth684"
api-approvers:
  - "@Prashanth684"
creation-date: 2026-09-16
last-updated: 2026-09-23
status: provisional
tracking-link:
  - https://redhat.atlassian.net/browse/MG-365
see-also:
  - "/enhancements/support-log-gather/must-gather-operator.md"
  - "/enhancements/support-log-gather/must-gather-custom-images.md"
---

# NetworkPolicy for Must-Gather Operator

## Summary

This enhancement adds Kubernetes NetworkPolicy resources to the must-gather-operator to restrict network access for both the operator pod and the must-gather job pods it creates. The operator pod receives a static NetworkPolicy deployed alongside its manifests. Job pods receive a two-tier dynamic NetworkPolicy model: a long-lived namespace-level default policy for base traffic (DNS, API server), and short-lived per-job policies for SFTP/proxy egress and admin-declared custom image egress. The design follows a least-privilege approach where all traffic not explicitly allowed is denied.

## Motivation

Today, the must-gather-operator and the job pods it creates have unrestricted network access. This is a security concern in hardened environments where default-deny network policies are enforced at the cluster level. Auditors increasingly require per-pod network policies to demonstrate least-privilege network access.

Without NetworkPolicy:
- The operator pod can reach any endpoint in the cluster or externally, even though it only needs the Kubernetes API server and an SFTP server for pre-flight validation.
- Must-gather job pods have full network access, even though the default image only needs the Kubernetes API server.
- In clusters with default-deny policies, the operator and its jobs may break silently because no allow-list policies exist.

### User Stories

- As a cluster administrator, I want the must-gather-operator and its job pods to have least-privilege network access so that I can meet security and compliance requirements in hardened environments.
- As a security auditor, I want to see explicit NetworkPolicy resources for must-gather pods so that I can verify the operator follows the principle of least privilege.
- As a cluster administrator running custom must-gather images, I want to declare the network requirements for each custom image so that the operator can create appropriate NetworkPolicies without opening all egress.
- As a cluster administrator, I want the operator to automatically manage NetworkPolicy lifecycle (create, update, cleanup) so that I do not need to manually create or maintain policies for must-gather jobs.

### Goals

1. Restrict the operator pod's network access to only the ports it needs (DNS, API server, SFTP validation, metrics, health probes).
2. Restrict must-gather job pods to the minimum egress required for the default image (DNS, API server).
3. Dynamically add SFTP and proxy egress rules to job pods only when the MustGather CR specifies an upload target or when proxy environment variables are configured.
4. Provide a mechanism for administrators to declare custom egress rules for custom must-gather images via the admin config resource (future).
5. Ensure concurrent must-gather jobs in the same namespace do not interfere with each other's NetworkPolicies.

### Non-Goals

1. Restricting network access for the ServiceAccount or RBAC associated with must-gather jobs (handled separately).
2. Providing a mechanism for end users (MustGather CR creators) to control NetworkPolicy rules directly.
3. Providing an "allow all egress" escape hatch in the admin config.

## Proposal

The design introduces NetworkPolicy at two layers:

1. **Operator Pod** -- A single static NetworkPolicy manifest deployed alongside the operator. It declares `policyTypes: [Ingress, Egress]` with explicit allow rules; Kubernetes implicitly denies all other traffic.

2. **Must-Gather Job Pods** -- A two-tier dynamic model managed by the operator's reconciler:
   - **Tier 1 (Namespace Default)**: A long-lived NetworkPolicy per namespace with base egress rules (DNS, API server). Created once, persists until the namespace is deleted.
   - **Tier 2 (Per-Job / Per-Image)**: Short-lived NetworkPolicies for SFTP/proxy egress and admin-declared custom image egress. Created per-job, cleaned up on MustGather CR completion or deletion.

### Workflow Description

**Cluster administrator** deploys the operator, which includes the static operator NetworkPolicy.

**User** creates a MustGather CR to trigger a must-gather collection.

1. The operator reconciles the MustGather CR.
2. If no namespace-level default NetworkPolicy exists in the target namespace, the operator creates one (Tier 1).
3. If the MustGather CR has `spec.uploadTarget` (SFTP) or the operator has proxy environment variables, the operator creates a short-lived NetworkPolicy with SFTP/proxy egress rules (Tier 2).
4. If the MustGather CR references a custom image with a matching `imageNetworkPolicies` entry in the admin config (future), the operator creates a per-image NetworkPolicy with admin-declared egress rules (Tier 2).
5. The operator creates the Job. Job pods are labeled for NetworkPolicy selection.
6. On MustGather CR completion or deletion, short-lived (Tier 2) NetworkPolicies are cleaned up. The namespace-level default (Tier 1) persists.

### API Extensions

This enhancement does not introduce new CRDs. It adds NetworkPolicy resources managed by the operator.

The future admin config resource (`MustGatherConfig`) will include an `imageNetworkPolicies` field for declaring custom image egress rules (see Layer 2 Tier 2 section below). The API shape for this field is illustrative and will be finalized when the admin config resource is implemented.

### Topology Considerations

#### Hypershift / Hosted Control Planes

No unique considerations. NetworkPolicy is a standard Kubernetes resource and applies equally in hosted control plane environments.

#### Standalone Clusters

The change is relevant for standalone clusters. The NetworkPolicy manifests are deployed alongside the operator and apply to the operator's namespace.

#### Single-node Deployments or MicroShift

No additional resource consumption. NetworkPolicy resources are lightweight metadata objects. The actual enforcement is handled by the CNI plugin, which is already running.

#### OpenShift Kubernetes Engine

No special considerations. NetworkPolicy is a standard Kubernetes API and does not depend on features excluded from the OKE product offering.

### Implementation Details/Notes/Constraints

#### Layer 1: Operator Pod NetworkPolicy (Static Manifest)

A single static NetworkPolicy is deployed in `deploy/10_must-gather-operator.NetworkPolicy.yaml`:

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: must-gather-operator
  namespace: must-gather-operator
  labels:
    app: must-gather-operator
spec:
  podSelector:
    matchLabels:
      name: must-gather-operator
  policyTypes:
    - Ingress
    - Egress
  egress:
    # DNS resolution
    - ports:
        - protocol: TCP
          port: 5353
        - protocol: UDP
          port: 5353
        - protocol: TCP
          port: 53
        - protocol: UDP
          port: 53
    # Kubernetes API server
    - ports:
        - protocol: TCP
          port: 6443
    # SFTP pre-flight credential validation
    - ports:
        - protocol: TCP
          port: 22
  ingress:
    # Prometheus metrics scraping
    - from:
        - namespaceSelector:
            matchLabels:
              network.openshift.io/policy-group: monitoring
      ports:
        - protocol: TCP
          port: 8080
    # Health/readiness probes
    - ports:
        - protocol: TCP
          port: 8081
```

The operator's allowed traffic:

| Direction | Port | Protocol | Purpose |
|-----------|------|----------|---------|
| Egress | 53, 5353 | TCP/UDP | DNS resolution (CoreDNS / openshift-dns) |
| Egress | 6443 | TCP | Kubernetes API server |
| Egress | 22 | TCP | SFTP pre-flight credential validation (unconditionally allowed) |
| Ingress | 8080 | TCP | Prometheus metrics scraping (from monitoring namespace) |
| Ingress | 8081 | TCP | Health/readiness probes |

All other ingress and egress traffic is implicitly denied.

#### Layer 2: Must-Gather Job NetworkPolicy (Two-Tier Model)

##### Tier 1: Namespace-Level Default Policy (Long-Lived)

A single "default" NetworkPolicy is created in a namespace the first time a must-gather job runs there. This policy persists until the namespace itself is deleted.

- Selects pods with label `must-gather.openshift.io/network-policy: default`
- Contains only the static base egress rules: DNS + API server
- Default-image job pods receive this label
- SFTP and proxy egress are NOT included (they vary per CR and belong in Tier 2)

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: must-gather-default
  namespace: <job-namespace>
spec:
  podSelector:
    matchLabels:
      must-gather.openshift.io/network-policy: default
  policyTypes:
    - Ingress
    - Egress
  egress:
    - ports:
        - protocol: TCP
          port: 53
        - protocol: UDP
          port: 53
        - protocol: TCP
          port: 5353
        - protocol: UDP
          port: 5353
    - ports:
        - protocol: TCP
          port: 6443
```

**Why only DNS + API server?** All must-gather data collection (`oc get`, `oc adm inspect`, `oc exec`, `oc cp`, `oc adm node-logs`, `oc debug`, `oc get --raw`) flows through the Kubernetes API server on port 6443. Kubelet interactions (logs, exec, cp) are proxied by the API server. The gather container does not make direct TCP connections to kubelet port 10250.

Lifecycle:
- **Created**: when the first must-gather job runs in the namespace
- **Never updated**: contains only static rules
- **Deleted**: only when the namespace is deleted

##### Tier 2: Short-Lived Policies (Per-Job and Per-Image)

Short-lived NetworkPolicies are created for:
1. **SFTP/proxy egress** -- when the MustGather CR has `spec.uploadTarget` or proxy environment variables are set
2. **Custom image egress** (future) -- when a custom image has an `imageNetworkPolicies` entry in the admin config

All Tier 2 policies are named using the MustGather CR name (e.g., `must-gather-<cr-name>-netpol`). Since the MustGather CR name is unique within a namespace and the spec is immutable, this guarantees each policy is unique per job. No hashing, random numbers, or shared-policy reference counting is needed.

###### SFTP/Proxy Egress (Per-Job)

When a MustGather CR specifies an SFTP upload target, the operator creates a short-lived NetworkPolicy named `must-gather-<cr-name>-sftp` with the SFTP egress rule. If proxy environment variables are set, the proxy port(s) are included in the same policy.

- The SFTP port defaults to 22 but respects a custom port in `spec.uploadTarget.sftp.host` (e.g., `sftp.example.com:2222`).
- Proxy ports are extracted from `HTTP_PROXY` / `HTTPS_PROXY` URLs. Defaults: http -> 3128, https -> 3129. Duplicate ports are deduplicated.

###### Custom Image Egress (Future -- with Admin Config Resource)

When the admin config resource (`MustGatherConfig`) is available, the admin can declare custom egress rules per image:

```yaml
apiVersion: operator.openshift.io/v1
kind: MustGatherConfig
metadata:
  name: cluster
spec:
  imageNetworkPolicies:
    - name: storage-vendor-gather
      imageStreamRef:
        name: storage-must-gather
        tag: latest
      egress:
        - ports:
            - protocol: TCP
              port: 443
    - name: network-debug-gather
      imageStreamRef:
        name: network-must-gather
        tag: v1.0
      egress:
        - ports:
            - protocol: TCP
              port: 10250
```

When a MustGather CR references a custom image matching an `imageNetworkPolicies` entry, the operator creates a per-job NetworkPolicy named `must-gather-<cr-name>-netpol` with:
- Base rules (DNS + API server)
- Admin-declared custom egress from the matching `imageNetworkPolicies` entry
- SFTP/proxy egress (if applicable)

Custom-image pods that have no matching `imageNetworkPolicies` entry get the `default` label value and fall under the Tier 1 namespace policy.

##### Per-CR Naming and Cleanup

All Tier 2 NetworkPolicies are named after the MustGather CR that triggered them. This simplifies lifecycle management:

| Policy | Name Pattern | Delete on CR completion? |
|--------|-------------|--------------------------|
| Tier 1 default | `must-gather-default` | Never |
| Tier 2 SFTP/proxy | `must-gather-<cr-name>-sftp` | Yes |
| Tier 2 per-image (future) | `must-gather-<cr-name>-netpol` | Yes |

On MustGather CR completion or deletion, the cleanup logic is:

1. Delete `must-gather-<cr-name>-sftp` if it exists
2. Delete `must-gather-<cr-name>-netpol` if it exists
3. Leave `must-gather-default` alone

This is safe for concurrent jobs because each CR has a unique name. Two CRs using the same custom image each get their own Tier 2 policy (with identical rules but different names). The slight duplication of NetworkPolicy objects is negligible -- they are lightweight metadata objects and must-gather jobs are short-lived.

Lifecycle:
- **Created**: before the job is created
- **Deleted**: when the MustGather CR completes or is deleted (simple delete by deterministic name)

##### Pod Label Assignment and Policy Routing

| Scenario | `must-gather.openshift.io/network-policy` value | NetworkPolicies Applied |
|----------|--------------------------------------------------|------------------------|
| Default image, no upload | `default` | `must-gather-default` (Tier 1) |
| Default image, with SFTP upload | `default` | `must-gather-default` (Tier 1) + `must-gather-<cr-name>-sftp` (Tier 2) |
| Custom image WITH imageNetworkPolicies entry | `default` | `must-gather-default` (Tier 1) + `must-gather-<cr-name>-netpol` (Tier 2) |
| Custom image WITHOUT imageNetworkPolicies entry | `default` | `must-gather-default` (Tier 1) |

All job pods use the same label value (`default`) for Tier 1 coverage. Tier 2 policies also select on `default`, but are named per-CR for independent lifecycle management. Kubernetes unions all matching policies, so the pod's effective egress is the combination of all applicable policies.

The `app.kubernetes.io/name: must-gather` label remains on all job pods for general identification and `oc get` filtering, but is not used for NetworkPolicy pod selection.

#### Design Rationale

**Why not per-job policies for everything?** The default must-gather image is the common case. Creating and deleting a NetworkPolicy for every single run adds unnecessary API churn. A long-lived namespace-level policy handles this cleanly.

**Why per-CR naming instead of per-image shared policies?** The MustGather CR name is unique within a namespace and the spec is immutable, so per-CR naming guarantees each Tier 2 policy is unique per job. This eliminates the need for hashing, random numbers, or shared-policy reference counting (OwnerReferences). Two CRs using the same custom image each get their own policy with identical rules -- the slight duplication is negligible since NetworkPolicies are lightweight metadata objects.

**Why admin config and not user CR?** Users should not control NetworkPolicy. It is a security boundary. The admin who curates the ImageStream allowlist already controls which images can run and should also own the network posture for those images.

**Why SFTP/proxy in Tier 2, not Tier 1?** SFTP host/port and proxy configuration vary per MustGather CR. Baking them into the long-lived namespace default would either over-provision access for all jobs or require updating the default policy on every CR creation.

### Risks and Mitigations

- **Risk**: A custom must-gather image that needs network access beyond DNS + API server but has no `imageNetworkPolicies` entry will fail silently.
  - **Mitigation**: The default policy (DNS + API server) covers the most common case (API-only data collection). If the custom image needs more, the admin adds an `imageNetworkPolicies` entry. No warning is logged to avoid assumptions about what a custom image needs.

- **Risk**: The CNI plugin does not support NetworkPolicy (e.g., some bare-metal setups).
  - **Mitigation**: NetworkPolicy resources are no-ops when the CNI plugin does not enforce them. The operator and jobs continue to work as before.

- **Risk**: The operator lacks RBAC to create NetworkPolicy resources.
  - **Mitigation**: The operator's ClusterRole already includes `networking.k8s.io/networkpolicies` with verbs `create`, `delete`, `get`, `list`, `update`, `watch`.

### Drawbacks

- Adds complexity to the reconciliation loop with NetworkPolicy lifecycle management.
- The two-tier model introduces multiple NetworkPolicy resources per namespace, which may be confusing to inspect when debugging connectivity issues.

## Alternatives (Not Implemented)

| Approach | Why Not Chosen |
|----------|----------------|
| Put egress rules in the MustGather CR itself | Users should not control network access -- security boundary violation. The admin decides what a custom image is allowed to reach. |
| Container image labels/annotations for network requirements | Requires runtime image inspection, adds complexity. The image author (often a vendor) should not unilaterally decide cluster network policy. |
| Allow all egress for custom images | Defeats the purpose of NetworkPolicy. |
| No extra egress for custom images (deny all beyond base) without admin config | Custom images that need additional access would silently break with no recourse. The admin config provides a managed path. |
| Two separate policies (deny-all + allow-traffic) for the operator pod | A single policy with `policyTypes: [Ingress, Egress]` and explicit rules is functionally equivalent and simpler to manage. The deny-all is implicit when policyTypes are declared. |
| Per-image shared NetworkPolicy with OwnerReference counting | Avoids policy duplication when multiple CRs use the same image, but adds complexity (OwnerReference lifecycle management, hash-based label derivation). Per-CR naming is simpler and the duplication cost is negligible. |
| Random label per job for NetworkPolicy uniqueness | Guaranteed unique but breaks on operator restart (new random generated, label mismatch with existing pod). Requires persisting the random value. Per-CR naming is deterministic and stateless. |

## Resolved Decisions

1. **Operator SFTP egress (port 22)**: Unconditionally allowed in the operator's static NetworkPolicy. The operator validates SFTP credentials as a pre-flight check, and the port is harmless to leave open when no SFTP upload is configured.
2. **Custom image without `imageNetworkPolicies` entry**: No warning logged. The default policy (DNS + API server) is assumed sufficient. If the custom image needs more, the admin adds an `imageNetworkPolicies` entry.
3. **No "allow all egress" escape hatch**: This would defeat the purpose of NetworkPolicy. Environments that do not want restrictions can simply not deploy the NetworkPolicy manifests.
4. **Per-CR naming for Tier 2 policies**: All short-lived policies are named `must-gather-<cr-name>-*`. Since CR names are unique and specs are immutable, this avoids the need for hashing, random label generation, or OwnerReference-based shared policy lifecycle management.
5. **SFTP/proxy in Tier 2, not Tier 1**: SFTP host/port and proxy configuration vary per CR. They belong in short-lived per-job policies, not the long-lived namespace default.

## Test Plan

The implementation includes unit tests covering:
- Base egress rule construction (DNS + API server)
- Dynamic SFTP egress rule (default port 22, custom port from host field, nil host, IPv6)
- Dynamic proxy egress rule (HTTP/HTTPS proxy URLs, port extraction, deduplication, no-proxy)
- NetworkPolicy creation, update (drift detection via egress rule comparison), and cleanup
- Job pod template labeling for NetworkPolicy pod selection
- Egress rule equality comparison
- Concurrent job isolation (distinct labels prevent cross-job NetworkPolicy interference)

E2E tests will verify:
- Must-gather job with default image completes successfully with NetworkPolicy in place
- Must-gather job with SFTP upload target creates and cleans up the SFTP egress NetworkPolicy
- Must-gather job fails gracefully when NetworkPolicy blocks required traffic (negative test)

## Graduation Criteria

The operator is already GA. This enhancement is a post-GA hardening improvement shipped as a standard operator update.

### Dev Preview -> Tech Preview

Not applicable. The operator is already GA. NetworkPolicy support is delivered in phases as part of regular operator updates.

### Tech Preview -> GA

Not applicable. See phased delivery below.

#### Phase 1 (Current)

- Operator pod static NetworkPolicy deployed
- Tier 1 namespace-level default policy for job pods implemented and tested
- Tier 2 SFTP/proxy per-CR policies implemented and tested
- E2E test coverage for default image and SFTP upload scenarios

#### Phase 2 (With Admin Config Resource)

- Admin config resource (`MustGatherConfig`) available with `imageNetworkPolicies` support
- Tier 2 per-image policies for custom images implemented and tested
- E2E test coverage for custom image NetworkPolicy scenarios

### Removing a deprecated feature

Not applicable. This enhancement adds new functionality and does not deprecate any existing features.

## Upgrade / Downgrade Strategy

- **Upgrade**: The operator deploys the static NetworkPolicy manifest on upgrade. Existing must-gather jobs are unaffected (NetworkPolicy is additive). New jobs will have NetworkPolicies created by the reconciler.
- **Downgrade**: Removing the operator removes the static NetworkPolicy. Namespace-level default policies persist until the namespace is deleted or manually removed. Short-lived per-job policies are cleaned up with their MustGather CRs.

## Version Skew Strategy

NetworkPolicy is a stable Kubernetes API (networking.k8s.io/v1) and does not have version skew concerns. The operator creates standard NetworkPolicy resources that are compatible with all supported OpenShift versions.

## Operational Aspects of API Extensions

This enhancement does not introduce new API extensions (CRDs, webhooks, aggregated API servers). It creates standard Kubernetes NetworkPolicy resources.

- NetworkPolicy resources are lightweight metadata objects and have negligible impact on API throughput.
- The operator creates at most 1 long-lived NetworkPolicy per namespace (Tier 1) and 1-2 short-lived NetworkPolicies per MustGather CR (Tier 2).
- If the CNI plugin does not enforce NetworkPolicy, the resources are no-ops with no operational impact.

## Support Procedures

- To detect NetworkPolicy issues: check if the must-gather job pod can reach the API server (`oc logs <job-pod>`). If the gather container logs show connection refused or timeout errors to `kubernetes.default.svc:6443`, inspect the NetworkPolicy in the job's namespace (`oc get networkpolicy -n <namespace>`).
- To temporarily disable NetworkPolicy for a job: delete the relevant NetworkPolicy resources in the job's namespace. The operator will recreate them on the next reconciliation, so this is a temporary measure.
- To permanently disable NetworkPolicy: remove the static manifest from the operator's deployment and set the admin config to not provision NetworkPolicies (details TBD with admin config resource).

## Infrastructure Needed

None.

## Implementation History

1. Initial implementation: operator static NetworkPolicy + per-job dynamic NetworkPolicy with base rules (DNS + API server), SFTP egress, and proxy egress.
2. Future: Tier 2 per-image NetworkPolicy support with admin config resource (`MustGatherConfig`).
