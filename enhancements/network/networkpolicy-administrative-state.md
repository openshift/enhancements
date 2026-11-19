---
title: networkpolicy-administrative-state
authors:
  - "@dbirefne"
reviewers: # Include a comment about what domain expertise a reviewer is expected to bring and what area of the enhancement you expect them to focus on.
  - "@trozet, for OVN-Kubernetes CNI aspects, please review the annotation-handling logic in the network policy controller"
  - "@openshift/console-team, for OpenShift Console UI aspects, please review the toggle UX workflow"
approvers:
  - "@dbirefne"
api-approvers: # In case of new or modified APIs or API extensions (CRDs, aggregated apiservers, webhooks, finalizers). If there is no API change, use "None".
  - "None"
creation-date: 2026-07-19
last-updated: 2026-07-19
tracking-link:
  - "https://redhat.atlassian.net/browse/RFE-9203"
status: provisional
see-also:
  - "/enhancements/network/admin-network-policy.md"
replaces: []
superseded-by: []
---

# NetworkPolicy Administrative State (Enable/Disable)

## Summary

This enhancement introduces an annotation-based mechanism to administratively disable a NetworkPolicy without deleting the resource from the cluster. A new well-known annotation (`networkpolicy.openshift.io/administrative-state`) will signal OVN-Kubernetes to skip enforcement of a policy's ingress and egress rules while preserving the full resource definition in etcd. A corresponding toggle in the OpenShift Console will allow administrators to enable or disable policies with a single click, eliminating the need to delete and recreate policies during troubleshooting, staging, or compliance workflows.

## Motivation

NetworkPolicies in OpenShift are currently binary: they are either active (created) or non-existent (deleted). There is no native mechanism to temporarily suspend enforcement without losing the resource definition. This creates friction for administrators who must delete policies to troubleshoot connectivity issues, stage policies before maintenance windows, or maintain audit trails of intended-but-inactive rules.

### User Stories

* As a cluster administrator, I want to disable a NetworkPolicy during a production outage investigation so that I can quickly isolate whether the policy is causing the connectivity failure without losing the policy's YAML configuration.

* As a security engineer, I want to pre-deploy NetworkPolicies in a disabled state so that I can stage them for review and activate them during a scheduled maintenance window without manual YAML application.

* As a compliance auditor, I want to see disabled NetworkPolicies in the cluster so that I can maintain a clear audit trail of what security rules are intended versus what is actively enforced.

* As a DevOps engineer, I want to maintain multiple versions of a NetworkPolicy and toggle between them so that I can perform A/B testing of different security postures without managing local file backups.

* As an SRE operating at scale, I want the system to emit metrics about disabled policies so that I can monitor for policies that remain disabled longer than expected and remediate drift.

### Goals

- Allow administrators to disable a NetworkPolicy without deleting the resource from the API server.
- Ensure disabled policies remain visible in the OpenShift Console with a clear "Disabled" visual indicator.
- Provide a one-click toggle in the Console UI to switch between enabled and disabled states.
- Reduce Mean Time to Recovery (MTTR) during network-related outages by enabling non-destructive fault isolation.
- Eliminate the need for local backups of policy YAML during troubleshooting.

### Non-Goals

- Modifying the upstream Kubernetes NetworkPolicy API (e.g., adding a new `spec.enabled` field). This proposal is intentionally scoped to an OpenShift-specific annotation.
- Supporting multi-cluster policy synchronization of the administrative state.
- Providing scheduled enable/disable (cron-based policy toggling). This may be addressed in a future enhancement.
- Adding RBAC beyond what already exists for `patch` operations on NetworkPolicy resources.

## Proposal

This enhancement introduces three coordinated changes:

1. **A well-known annotation** recognized by the OVN-Kubernetes CNI plugin:
   ```yaml
   metadata:
     annotations:
       networkpolicy.openshift.io/administrative-state: "disabled"
   ```
   When this annotation is absent or set to any value other than `"disabled"`, the policy is enforced normally (backward-compatible default).

2. **OVN-Kubernetes behavior change**: The NetworkPolicy controller in OVN-Kubernetes will check for this annotation during reconciliation. When set to `"disabled"`, the controller will skip generating the corresponding OVN ACL (Access Control List) logical flows. When the annotation is removed or changed back to `"enabled"`, the controller will regenerate the flows.

3. **OpenShift Console UI change**: A toggle button will be added to the NetworkPolicy detail and list views under Networking > NetworkPolicies. The button issues a JSON Patch request to add or remove the annotation.

### Workflow Description

**Cluster administrator** is a human user responsible for managing network security in the cluster.

**OVN-Kubernetes controller** is the software component that translates NetworkPolicy resources into OVN logical flows.

**OpenShift Console** is the web-based UI for cluster management.

#### Disabling a Policy via Console

1. The cluster administrator navigates to Networking > NetworkPolicies in the OpenShift Console.
2. The administrator selects a NetworkPolicy and clicks the "Disable" toggle button.
3. The Console sends a JSON Patch request to the Kubernetes API server:
   ```json
   [{"op": "add", "path": "/metadata/annotations/networkpolicy.openshift.io~1administrative-state", "value": "disabled"}]
   ```
4. The API server persists the annotation update and emits a watch event.
5. OVN-Kubernetes receives the watch event, detects the `administrative-state: disabled` annotation, and removes (or skips regenerating) the ACL flows associated with that policy.
6. Traffic previously restricted by the policy is now allowed.
7. The Console UI updates the policy's status indicator to show "Disabled."

#### Re-enabling a Policy via Console

1. The cluster administrator clicks "Enable" on the disabled policy.
2. The Console sends a JSON Patch request to remove the annotation:
   ```json
   [{"op": "remove", "path": "/metadata/annotations/networkpolicy.openshift.io~1administrative-state"}]
   ```
3. OVN-Kubernetes receives the watch event, detects the annotation is absent, and regenerates the ACL flows.
4. The policy is enforced again.

#### Disabling a Policy via CLI

1. The administrator runs:
   ```bash
   oc annotate networkpolicy <name> -n <namespace> networkpolicy.openshift.io/administrative-state=disabled
   ```
2. The same OVN-Kubernetes reconciliation triggers as described above.

#### Error Handling

- If the annotation contains an invalid value (e.g., `"maybe"`), OVN-Kubernetes treats it as enabled (enforce the policy) and emits a warning event on the NetworkPolicy resource.
- If OVN-Kubernetes is temporarily unavailable when the annotation changes, normal controller resync will reconcile the state on restart.

### API Extensions

This enhancement does not introduce any new CRDs, webhooks, aggregated API servers, or finalizers.

It modifies the behavior of OVN-Kubernetes with respect to existing `networking.k8s.io/v1/NetworkPolicy` resources. Specifically, OVN-Kubernetes will conditionally skip flow generation based on the presence of the `networkpolicy.openshift.io/administrative-state` annotation. Since this is an annotation (not a spec field), it does not change the Kubernetes API schema.

The annotation key `networkpolicy.openshift.io/administrative-state` uses the `openshift.io` domain to clearly signal this is an OpenShift-specific extension.

### Topology Considerations

#### Hypershift / Hosted Control Planes

No unique considerations. In Hypershift deployments, OVN-Kubernetes runs in the guest cluster where NetworkPolicy resources reside. The annotation is evaluated locally within the guest cluster's CNI, so management cluster components are unaffected.

#### Standalone Clusters

This change is fully relevant and functional for standalone clusters. It is the primary deployment topology for this feature.

#### Single-node Deployments or MicroShift

The annotation check adds negligible CPU and memory overhead (a single string comparison per NetworkPolicy during reconciliation). For SNO deployments this is acceptable.

For MicroShift: MicroShift uses OVN-Kubernetes as its CNI, so this feature will be available if the annotation-checking code is present. No additional MicroShift-specific configuration is needed. The feature can be exposed via `oc` CLI; a MicroShift-specific UI is not in scope.

#### OpenShift Kubernetes Engine

This feature depends on OVN-Kubernetes (included in OKE) and the OpenShift Console (included in OKE). It does not depend on any OCP-only features excluded from OKE. The feature is applicable to OKE deployments.

### Implementation Details/Notes/Constraints

**OVN-Kubernetes Changes:**

The primary change is in the NetworkPolicy controller's reconcile loop (located in `go-controller/pkg/ovn/policy.go` or equivalent). Before generating Address Sets and ACL rules for a given NetworkPolicy, the controller will:

1. Read the annotation `networkpolicy.openshift.io/administrative-state` from the NetworkPolicy's metadata.
2. If the value equals `"disabled"` (case-insensitive), skip all ACL and Address Set generation for this policy and ensure any previously created flows for this policy are removed.
3. If the annotation is absent or has any other value, proceed with normal flow generation.

The controller already watches for NetworkPolicy changes (including annotation updates), so no additional informer or watch setup is needed.

**OpenShift Console Changes:**

- Add a toggle button (or dropdown action) to the NetworkPolicy detail page (`frontend/packages/console-app/src/components/network-policies/`).
- The toggle calls a standard Kubernetes JSON Patch on the resource's annotations.
- The NetworkPolicy list view should display a badge or icon indicating "Disabled" state for annotated policies.
- The feature should be gated behind the same feature gate as the backend (if a gate is used for Dev Preview).

**Cluster Network Operator:**

No changes required. The annotation is transparent to CNO — it is a passthrough annotation on the NetworkPolicy resource that OVN-K interprets directly.

**Metrics:**

- A new Prometheus metric `ovnkube_networkpolicy_administrative_state{namespace, name, state}` will report the current administrative state of each policy, enabling alerting on policies that remain disabled.

### Risks and Mitigations

| Risk | Mitigation |
|------|-----------|
| Accidental disabling of critical policies in production | Standard Kubernetes RBAC applies — only users with `patch` permission on NetworkPolicy resources can toggle the annotation. Organizations can restrict this with additional RBAC rules. |
| Policy modified while disabled leads to unexpected behavior on re-enable | OVN-K re-reads the full spec on annotation removal and generates flows from the current spec, so any modifications made while disabled are applied correctly. |
| Confusion about which policies are active | Console shows a clear "Disabled" visual indicator. The metric enables monitoring and alerting. |
| Upstream Kubernetes eventually adds a native `enabled` field | If upstream adds native support, this annotation becomes redundant. The migration path is straightforward: deprecate the annotation, add a controller that reconciles annotation-to-field, and eventually remove annotation support. |
| Security audit tools may not understand the annotation | Documentation and release notes will clearly explain the annotation's semantics. |

### Drawbacks

- This introduces an OpenShift-specific behavior that diverges from upstream Kubernetes semantics where a present NetworkPolicy always enforces rules. Tools and operators that assume this invariant may be surprised.
- The annotation approach is less discoverable than a dedicated API field. Users must know the specific annotation key.
- If upstream Kubernetes eventually implements a native disable mechanism with different semantics, there will be a migration burden.

## Alternatives (Not Implemented)

### Upstream Kubernetes Enhancement Proposal (KEP)

Adding a `spec.enabled` boolean field to the NetworkPolicy API would be the cleanest solution. However, this requires a Kubernetes Enhancement Proposal, cross-SIG consensus (sig-network, sig-api-machinery), and multiple release cycles. The timeline is impractical for addressing current operational pain.

### CRD-based PolicyOverride Resource

A dedicated `NetworkPolicyOverride` CRD could reference a NetworkPolicy and specify `state: disabled`. This approach adds API surface complexity, requires a new controller, and is harder to discover. The annotation approach is simpler and maps directly to the existing resource.

### Webhook-based Admission Control

A mutating admission webhook could intercept NetworkPolicy creation/updates and inject "allow-all" rules when a disable annotation is detected. This approach is fragile (webhook availability becomes a dependency), does not cleanly remove rules, and is harder to reason about than skipping flow generation entirely.

## Open Questions [optional]

1. Should we support a `"suspended"` state in addition to `"disabled"` with different semantics (e.g., suspended = temporarily off with auto-re-enable after a TTL)?
2. Should there be an audit log event emitted specifically when the administrative state changes, beyond the standard Kubernetes audit log for annotation patches?
3. Should the feature be gated behind a FeatureGate for Dev Preview, or can it ship directly as Tech Preview given the annotation is opt-in?

## Test Plan

**End-to-End Tests:**

- Create a NetworkPolicy that denies all ingress to a test pod.
- Verify that traffic to the pod is blocked.
- Annotate the policy with `networkpolicy.openshift.io/administrative-state: disabled`.
- Verify that traffic to the pod is now allowed.
- Remove the annotation.
- Verify that traffic is blocked again.
- Repeat the above with egress policies.

**Unit Tests (OVN-Kubernetes):**

- Test that the reconcile loop skips ACL generation when the annotation is present.
- Test that previously generated ACLs are removed when the annotation is added.
- Test that invalid annotation values (e.g., `"maybe"`) result in normal enforcement and a warning event.
- Test that annotation removal triggers full ACL regeneration.

**Integration Tests (Console):**

- Test that the toggle button correctly patches the annotation.
- Test that the UI reflects the disabled state after toggling.
- Test that the "Disabled" badge appears in the list view.

**Upgrade Tests:**

- Verify that a cluster upgraded from a version without this feature correctly ignores the annotation if it was manually applied (policies remain enforced by the old OVN-K version, and on upgrade the new OVN-K version picks up the annotation).

## Graduation Criteria

### Dev Preview -> Tech Preview

- OVN-Kubernetes recognizes and respects the annotation behind a feature gate.
- End-to-end test coverage for enable/disable/re-enable lifecycle.
- Console toggle available when the feature gate is enabled.
- API stability: annotation key is finalized.
- Metrics exposed for disabled policy count.
- Documentation published describing the annotation and CLI workflow.

### Tech Preview -> GA

- Feature gate removed; annotation is recognized by default.
- Upgrade and downgrade testing completed.
- Load testing with 1000+ NetworkPolicies (mix of enabled/disabled) shows no performance regression.
- Console UI finalized based on Tech Preview feedback.
- User-facing documentation created in [openshift-docs](https://github.com/openshift/openshift-docs/).
- SLO defined for OVN-K reconciliation latency when annotation state changes.

### Removing a deprecated feature

If upstream Kubernetes provides a native equivalent, the annotation will be deprecated:
- Announce deprecation with a minimum 2-release window.
- Provide migration documentation mapping annotation to the native mechanism.
- Remove annotation support after the deprecation window.

## Upgrade / Downgrade Strategy

**Upgrade:**

- No migration is required on upgrade. Existing clusters will not have the annotation on any NetworkPolicy, so all policies continue to be enforced normally.
- To use the feature after upgrade, administrators simply add the annotation to desired policies.
- The annotation is ignored by older OVN-Kubernetes versions that do not contain this feature's code, so a partially upgraded cluster (version skew) defaults to "enforce" — the safe behavior.

**Downgrade:**

- If a cluster is downgraded to a version without this feature, the annotation will be ignored by the older OVN-Kubernetes. All policies (including those annotated as disabled) will be enforced.
- This is the safe default: downgrade cannot accidentally leave policies disabled.
- No manual cleanup is required, though administrators may choose to remove stale annotations for clarity.

## Version Skew Strategy

During an upgrade, OVN-Kubernetes pods are rolled out across nodes. During the rollout window:

- Nodes running the **new** OVN-K version will respect the annotation (skip enforcement for disabled policies).
- Nodes running the **old** OVN-K version will ignore the annotation (enforce all policies).

This means that during the brief upgrade window, a "disabled" policy may still be partially enforced on nodes that have not yet been updated. This is acceptable because:
- The upgrade window is short (minutes).
- The safe default is "enforce," so no security gap is introduced during skew.
- Once all nodes are upgraded, the disabled state is uniformly respected.

No other components on the node are affected (no CSI, CRI changes). This is purely a CNI-level change within OVN-Kubernetes.

## Operational Aspects of API Extensions

This enhancement does not add CRDs, webhooks, aggregated API servers, or finalizers. It uses a standard annotation on an existing core resource. Therefore:

- **SLIs:** The existing OVN-Kubernetes health metrics apply. A new metric `ovnkube_networkpolicy_administrative_state` will indicate disabled policy count per namespace.
- **Impact on existing SLIs:** Negligible. The annotation check is a single string comparison per NetworkPolicy during reconciliation — no measurable impact on API throughput or OVN-K reconciliation latency.
- **Failure modes:** If OVN-K fails to process the annotation change (e.g., crash during reconciliation), the next resync will reconcile the state. During the window, the policy remains in its previous enforcement state (either enforced or not, depending on what was last applied).
- **Cluster health impact:** A bug that erroneously treats all policies as disabled would remove all network segmentation. This is mitigated by the explicit opt-in annotation requirement and E2E test coverage.
- **Escalation teams:** OVN-Kubernetes / Networking team, OpenShift Console team.

## Support Procedures

**Detecting issues:**

- If a customer reports unexpected traffic allowed through a policy, check for the annotation:
  ```bash
  oc get networkpolicy <name> -n <namespace> -o jsonpath='{.metadata.annotations.networkpolicy\.openshift\.io/administrative-state}'
  ```
- The metric `ovnkube_networkpolicy_administrative_state{state="disabled"}` will show all disabled policies.
- OVN-K logs will include messages like: `"Skipping ACL generation for NetworkPolicy %s/%s: administrative-state=disabled"`.

**Disabling the feature:**

- To re-enable a specific policy, remove the annotation:
  ```bash
  oc annotate networkpolicy <name> -n <namespace> networkpolicy.openshift.io/administrative-state-
  ```
- To disable the feature cluster-wide (if behind a feature gate), toggle the gate in the FeatureGate resource. All policies will be enforced regardless of annotation.
- Consequences of removing the feature gate while policies are annotated: OVN-K will re-enforce all policies on next resync (safe default).

**Impact on existing workloads:**

- Disabling a policy only affects traffic rules — no pods are restarted, no services are disrupted.
- Re-enabling a policy may cause existing connections to be dropped if they violate the re-applied rules (same behavior as creating a new policy).

**Graceful failure:**

- The annotation is purely opt-in. If the feature has a bug, removing all `administrative-state` annotations restores normal behavior with no additional action needed.

## Infrastructure Needed [optional]

No new subprojects, repositories, or testing infrastructure are needed. Changes will be submitted to existing repositories:
- `ovn-org/ovn-kubernetes` (backend logic)
- `openshift/console` (UI toggle)
