---
title: preflight-from-target-release
authors:
  - "@wking"
  - "@fao89"
reviewers: # Include a comment about what domain expertise a reviewer is expected to bring and what area of the enhancement you expect them to focus on. For example: - "@networkguru, for networking aspects, please look at IP bootstrapping aspect"
  - "@hongkailiu, for accepted risks integration and conditional update aspects"
  - "@enxebre, for HyperShift compatibility assessment and future integration planning"
  - "@csrwng, for HyperShift architecture alignment and hosted control plane considerations"
approvers:
  - "@PratikMahajan"
api-approvers:
  - "@JoelSpeed"
creation-date: 2026-01-14
last-updated: 2026-01-16
status: provisional
tracking-link:
  - https://issues.redhat.com/browse/OCPSTRAT-2843
see-also:
  - "/enhancements/update/accepted-risks.md"
replaces:
superseded-by:
---

# Preflight checks from target release

## Summary

Give cluster administrators a way to run preflight checks from a target release before launching an update.

## Motivation

The existing cluster update validation mechanisms have limitations that this enhancement addresses:

1. **Skip-level update challenges**: With skip-level updates on the horizon [KEP-4330](https://github.com/kubernetes/enhancements/tree/master/keps/sig-architecture/4330-compatibility-versions), the existing `Upgradeable` tooling does not allow a 5.0 cluster to distinguish between risks for updating to 5.1 and risks for updating skip-level directly to 5.2.
    This preflight enhancement would allow that 5.0 cluster to run checks from the 5.2.z target release to report about any concerns that target release had with the current version's state.
1. **Component maintainer workflow**: Previously explored in [PR #363](https://github.com/openshift/enhancements/pull/363), operators needed complex backporting strategies to warn about future incompatibilities.
    This approach allows components to define compatibility checks in their target release rather than backporting knowledge to previous versions.

This preflight enhancement allows clusters to run compatibility checks from a target release without committing to the update, enabling administrators to understand and plan for potential issues before beginning an update.

### User Stories

#### As a cluster administrator operating a production OpenShift cluster

I want to proactively check compatibility with a target release without committing to an update, so that I can:
- Assess risks for skip-level updates (e.g., from 5.0 directly to 5.2).
- Validate that my cluster configuration and workloads are compatible before scheduling an update, optionally well before, so I have plenty of time to calmly address any detected issues.
- Review specific risk names that can be accepted using the accepted-risks mechanism introduced in [accepted-risks](accepted-risks.md)

#### As a component maintainer developing OpenShift operators

I want to write compatibility checks in my target release rather than backporting compatibility logic, so that I can:
- Define what configurations from previous releases are incompatible with my new version.
- Leverage the latest understanding of compatibility requirements without backporting knowledge to older releases.
- Reduce the maintenance burden of keeping compatibility checks synchronized across multiple release branches.
- Focus compatibility validation logic in the release where breaking changes are introduced, which may include additional context like release manifest YAML that is not available in the older release.

**Example**: A 5.2 networking operator can check if a 5.0 cluster's dual-stack configuration is compatible with 5.2's networking changes, without requiring the 5.0 operator to know about future 5.2 requirements.

#### As a cluster lifecycle engineer

I want to integrate preflight checks into automated update workflows, so that I can:
- Run preflight validations as part of CI/CD pipelines before approving cluster updates.
- Generate reports on fleet-wide compatibility for upcoming releases.
- Implement automated update policies that only proceed when preflight checks pass.

### Goals

* **Proactive risk assessment**: Enable cluster administrators to identify potential update risks before committing to an update, particularly for skip-level updates.
    This aligns with upstream Kubernetes work on compatibility versions in [KEP-4330](https://github.com/kubernetes/enhancements/tree/bb6bf298fdc524454b6fd477c84f5760b0f98c40/keps/sig-architecture/4330-compatibility-versions).
* **Target release compatibility checks**: Allow components to define compatibility checks in their target release rather than requiring backports to previous releases.
* **Integration with accepted-risks workflow**: Results from preflight checks should integrate with the existing `conditionalUpdateRisks` and accepted-risks mechanism to provide a unified risk management experience.
* **Non-disruptive validation**: Preflight checks must be read-only operations that do not modify cluster state or affect running workloads.
  The preflight CVO deployment runs with restricted RBAC permissions that prevent write access to cluster resources, ensuring enforcement at the API level.
* **Flexible execution model**: Support both one-time preflight validation and continuous preflight monitoring for target releases.

Success criteria:
- Administrators can run `oc adm upgrade --mode=preflight --to=<version>` to check compatibility.
- Administrators can cancel preflight checks using `oc adm upgrade --clear-preflight`.
- Preflight results appear in ClusterVersion `status` alongside other conditional update risks.
- Preflight results are automatically cleared after cluster upgrades to prevent stale data confusion.
- Partial or failed preflight results are clearly marked to distinguish from complete assessments.
- Component maintainers can write compatibility checks into the target release, without backporting logic to earlier releases.

### Timeline and Skip-Level Update Dependencies

This enhancement is targeting an aggressive timeline to support upcoming skip-level update capabilities:

**Skip-level update timeline**: OpenShift is planning to support skip-level updates (e.g., 5.0 → 5.2) starting with standalone clusters. This capability depends on having robust preflight validation to warn administrators about compatibility issues before launching skip-level updates.

**Critical path dependencies**:
- **4.22 Tech Preview requirement**: To enable skip-level updates in OpenShift 5.0, preflight functionality must reach Tech Preview status in 4.22
- **5.0 GA target**: Skip-level support requires preflight to GA by OpenShift 5.0 to serve as the launch point for skip-level validation
- **Limited development time**: Missing the 4.22 Tech Preview window significantly risks the 5.0 GA timeline

**Consequence of delays**: If preflight functionality is delayed:
- Skip-level updates for 5.0 → 5.2 may not be possible, forcing customers through intermediate 5.1 updates
- Alternative solutions like `UpgradeableNextEUS` conditions would need development, adding complexity
- The window for skip-level support may shift to later major versions (5.1 → 5.3 or beyond)

**Alignment with upstream Kubernetes**: This timeline also aligns with [KEP-4330 compatibility versions](https://github.com/kubernetes/enhancements/blob/4970b6fabfcf821b732ca24bdccad1d92fc7f208/keps/sig-architecture/4330-compatibility-versions/kep.yaml#L25), which remains in alpha status, making early OpenShift implementation valuable for upstream feedback.

The aggressive timeline reflects the strategic importance of skip-level updates for enterprise customers who prefer fewer, more significant updates over frequent minor version updates.

### Non-Goals

* **Operator-level preflight framework**: This enhancement focuses on cluster-level preflight orchestration.
    Individual operator preflight implementations are out of scope (those would be developed separately by component teams).
    * For the initial enhancement, even the cluster-level interface between the target-release CVO and the target-release operators is out of scope.
      We need rapid agreement on the interface between the user and the cluster-managing CVO, and between the cluster-managing CVO and the target-release CVO to set a solid launch pad in the initial release.
      The details of the interface bewtween the target-release CVO and target-release operators can be deferred to the target release, and we have more time to plan that out.
* **Automatic remediation**: Preflight checks identify risks but do not automatically fix configuration issues.
    Remediation remains a manual administrative task.
* **Performance impact analysis**: This enhancement identifies compatibility risks but does not assess performance impact or resource consumption changes in target releases.
* **Rollback planning**: While preflight checks may identify update risks, planning rollback strategies for failed updates is out of scope.
* **HyperShift** or **Web-console integration**: For the initial implementation, we will focus on standalone clusters, the API, and `oc`.
    Integration with HyperShift and the in-cluster web console can happen in subsequent phases.
* **Managed OpenShift integration**: For the 4.22 tech-preview, integration with managed OpenShift service workflows and customer isolation requirements is out of scope due to time constraints.
    This includes testing integration with managed service update automation and ensuring preflight checks do not access sensitive managed service configurations.
* **External plugins**: This enhancement does not give cluster admins the ability to plug in additional checks specific to a given target version.
    They retain the ability to:
    * [Create `critical` platform alerts][create-platform-alert] which [existing checks will surface pre-update][recommend-critical-alert].
    * [Create a custom ClusterOperator with an `Upgradeable=False` condition][ClusterOperator-Upgradeable] which existing logic will propagate through to major and minor updates (`Upgradeable` does not block patch updates from x.y.z to x.y.z' within the current z stream).

## Proposal

[The accepted-risks proposal](accepted-risks.md) added `clusterversion.status.conditionalUpdateRisks` to ClusterVersion to discuss risks that the cluster is concerned about.
This gives us an existing location where we can discuss any concerns a preflight turns up.
The remaining piece, proposed in this enhancement, is a way to request a preflight for a specific target release.
We will add a new `mode` property to `spec.desiredUpdate` to mark preflight requests.

### Workflow Description

**Cluster Administrator** is responsible for managing OpenShift cluster updates and maintenance.

**Component Developer** writes OpenShift operators and defines compatibility checks for their components.

#### Requesting a Preflight Check

1. **Starting State**: A cluster administrator wants to evaluate risks for upgrading from version 5.0.0 to version 5.2.0 (skip-level update) before scheduling a maintenance window.
1. **Request Preflight Check**: Administrator uses `oc` to request a preflight check: `oc adm upgrade --mode=preflight --to 5.2.0`.
1. **CVO Processes Request**: The Cluster Version Operator detects the preflight request and:
    - Launches target CVO as a Deployment with `preflight` argument, instead of performing an actual update.
    - Uses a shared volume to share preflight results between the preflight CVO and the cluster-managing CVO.
1. **Target Release Validation**: The target release CVO (5.2.0) runs in preflight mode:
    - Examines current cluster configuration, operators, and workloads.
    - Executes compatibility checks defined by operators in the 5.2.0 release.
    - Generates risk assessment without modifying cluster state.
1. **Results Integration**: Preflight results are reported back to the running CVO and integrated into the ClusterVersion status:
    ```yaml
    status:
      conditionalUpdateRisks:
      - name: "DualStackIncompatible"
        message: "Cluster uses dual-stack networking configuration incompatible with 5.2.0."
        conditions:
        - type: Applies
          status: True
          reason: "PreflightValidation"
          message: "Risk identified during preflight check for 5.2.0."
    ```
    Preflight results included in ClusterVersion status will be automatically reported to Red Hat Insights as part of standard ClusterVersion telemetry, providing visibility for support and engineering teams without requiring separate reporting mechanisms.
1. **Administrator Review**: Administrator reviews risks and can either:
   - Address configuration issues before updating.
   - Accept specific risks using [the established accepted-risks workflow](accepted-risks.md).
   - Choose a different update path.

### API Extensions

#### ClusterVersion spec.desiredUpdate.mode

The existing [ClusterVersion `spec.desiredUpdate` property][ClusterVersion-desiredUpdate] would have its [`Update` type][Update-API] extended with a new `mode` property:

```go
// mode allows an update to be checked for compatibility without committing to updating the cluster.
// When omitted (default), the update request proceeds normally.
// When set to "Preflight", compatibility checks are performed without updating the cluster.
// +kubebuilder:validation:Enum:=Preflight
// +optional
mode UpdateModePolicy `json:mode,omitempty`
```

allowing preflight requests like:

```yaml
spec:
  desiredUpdate:
    mode: Preflight
    version: 5.2.0
```

**Version parameter validation**: The `version` field in preflight requests follows the same validation rules as regular update requests:

- **Available versions**: The version must be available through the cluster's configured Cincinnati update service
- **Release image accessibility**: The specified version's release image must be accessible from the cluster
- **Not-recommended versions allowed**: Preflight checks can target versions marked as "not recommended" in Cincinnati, enabling risk assessment even for potentially problematic updates
- **Signature verification**: Same signature and verification requirements apply as regular updates (can be bypassed with existing `--allow-explicit-upgrade` mechanisms for testing)
- **Version format**: Must follow semantic versioning format (X.Y.Z or X.Y.Z-suffix)

**Validation behavior**: Invalid version requests result in standard CVO error handling:
- Version not found in Cincinnati: Preflight request fails with descriptive error message
- Inaccessible release image: Deployment fails with appropriate error conditions
- Invalid version format: API validation rejects the request

This validation approach ensures preflight checks use the same version resolution and security policies as actual cluster updates.

**Note on multiple preflight checks**: The current design supports one active preflight check at a time. To compare results across multiple target versions, administrators would need to run sequential preflight checks. Future enhancements could extend the `mode` field to support multiple concurrent checks (e.g., `mode: "Preflight-test1"` and `mode: "Preflight-test2"`), but this complexity is deferred from the initial Tech Preview implementation to focus on core functionality.

### Topology Considerations

#### Hypershift / Hosted Control Planes

HyperShift is out of scope for the initial Tech Preview release (OpenShift 4.22) to focus implementation efforts on standalone clusters first.
This aligns with the strategic priority of supporting upcoming skip-level updates ([OCPSTRAT-2843](https://issues.redhat.com/browse/OCPSTRAT-2843)) which primarily target standalone cluster deployments.

**Compatibility assessment**: The chosen design approach should be compatible with future HyperShift integration:

- **Existing patterns**: HyperShift already runs CVO as a child Deployment, so the preflight CVO deployment pattern should be adaptable
- **API surface**: The ClusterVersion API extensions are minimal and could be exposed through HostedCluster or HostedControlPlane CRDs
- **Isolation model**: The read-only RBAC approach aligns with HyperShift's hosted control plane isolation requirements
- **Result propagation**: The file-based result sharing mechanism can be adapted to HyperShift's control plane architecture

**Future integration considerations**: HyperShift support will require:

- **HostedCluster API extensions**: Adding preflight-related fields to HostedCluster or HostedControlPlane CRDs
- **Control plane deployment**: Adapting preflight CVO deployment to hosted control plane constraints
- **Result aggregation**: Ensuring preflight results surface appropriately in hosted cluster management workflows
- **RBAC adaptation**: Aligning preflight permissions with HyperShift's security boundaries

**Risk mitigation**: If design decisions prove incompatible with HyperShift:
- The ClusterVersion API extensions are minimal and could be refactored without major breaking changes
- The CVO-to-CVO communication pattern provides a clean abstraction layer for adaptation
- Alternative result delivery mechanisms could be implemented without changing user-facing APIs

Future enhancements will evaluate detailed integration with the HostedCluster API once the core preflight infrastructure is proven with standalone clusters.

#### Standalone Clusters

Yes, standalone is the focus.

#### Single-node Deployments or MicroShift

Single-node will have the same support as standalone.
Running preflight checks will come with the usual resource cost of long-running workload.
But cluster-admins have the ability to clear `desiredUpdate` if they want to stop running preflights, and they can enable or disable preflights as they see fit, to balance the cost vs. the benefit.

MicroShift is out of scope, because it doesn't run a cluster-version operator.
I'm not sure if MicroShift has a mechanism for checking for update compatibility or conditional update issues or not.

#### OpenShift Kubernetes Engine

This functionality will be implemented in component layers that are part of the OpenShift Kubernetes Engine (OKE), so it will function there the same way it does in OCP.

### Implementation Details/Notes/Constraints

#### Requesting preflight checks

Cluster administrators can request preflight checks via [the new `mode` property](#clusterversion-spec-desiredupdate-mode).
The `mode` property will also be wrapped in the existing `oc adm upgrade` command, so cluster administrators can use `oc adm upgrade --mode=preflight ...` to request preflight updates.

#### CLI Command Extensions

The `oc adm upgrade` command will be extended with preflight-specific subcommands for improved usability:

**Starting preflight checks:**
```bash
oc adm upgrade --mode=preflight --to=5.2.0
```

**Cancelling active preflight checks:**
```bash
oc adm upgrade --clear-preflight
```
This command provides a user-friendly alternative to manual patch operations, similar to the existing `--clear` command for cancelling non-started upgrades.

**Checking preflight status:**
```bash
oc adm upgrade --status-preflight
```
Enhanced to display preflight execution status when active, including target version and completion progress.

#### Evaluating preflight checks

When the cluster-version operator (CVO) sees a `mode: Preflight` request, it retrieves the target release pullspec in the usual way as for an update request.
But instead of launching a `version-*` Pod to retrieve release manifests from the target release (FIXME: https://github.com/openshift/cluster-version-operator/blob/83243780aed4e0d9c4ebff528e54b918d4170fd3/pkg/cvo/updatepayload.go#L189-L297), it runs the target release as a long-running Deployment with `args` set to `preflight`.

The preflight CVO Deployment will be configured with resource limits lower than the standard CVO to prevent cluster resource exhaustion:
- **CPU limits**: Reduced from standard CVO allocation to support read-only operations
- **Memory limits**: Lower than production CVO to account for preflight-specific workload patterns
- **Resource quotas**: Configurable limits to allow administrators to control resource consumption based on cluster capacity
- **Scheduling constraints**: Optional node affinity and tolerations to isolate preflight workloads

This way, the old CVO doesn't need to understand the details of how to query components for preflight checks; that's all deferred to the target CVO.

When the target release CVO is invoked with the `preflight --format=preflight-v1-json` argument, it runs preflight checks, and reports the results to the cluster's running CVO via a host-mounted volume (just like the `version-*` Pod FIXME https://github.com/openshift/cluster-version-operator/blob/83243780aed4e0d9c4ebff528e54b918d4170fd3/pkg/cvo/updatepayload.go#L271-L275).

**Design rationale**: The preflight CVO reports results indirectly through the current cluster CVO rather than updating ClusterVersion status directly to maintain clear ownership boundaries. The current cluster CVO owns the ClusterVersion resource and is responsible for coordinating all cluster update activities. Having the target release CVO directly update the resource would create potential conflicts and complexity in resource ownership, especially during concurrent operations or when the preflight CVO is a different version than the current CVO.

**Server-side apply considerations**: An alternative approach would use server-side apply with structured field management to allow both CVOs to safely write to different parts of the ClusterVersion status. This would require:

- **List field annotations**: `conditionalUpdateRisks` would need `+listType=map` and `+listMapKey` annotations to enable safe concurrent updates
- **Field manager separation**: Each CVO would act as a separate field manager, owning distinct entries in the list
- **Conflict resolution**: When both CVOs write entries with the same key, conflicts would be rejected requiring explicit conflict resolution

**Current CVO compatibility limitations**: The existing CVO codebase uses traditional Update operations rather than server-side apply patterns:

- **Legacy update patterns**: Current CVO code would require significant refactoring to adopt applyconfiguration patterns
- **Cache invalidation complexity**: Distinguishing between "append this new conditional update" and "append this new one but clear stale entries" becomes complex even with server-side apply
- **Version skew issues**: Target CVO (potentially N+1 or N+2) and current CVO (N) may have different understandings of list structure and conflict resolution

The chosen indirect approach avoids these technical complications while maintaining clear ownership semantics and backward compatibility with existing CVO patterns.
The results will be JSON.
Because they will be [propagated into `conditionalUpdateRisks`](#retrieving-preflight-check-results), we'll use that structure:

```json
{
  "format": "preflight-v1-json",
  "preflightID": "2026-02-06T15:30:00Z-preflight-5.2.0",
  "targetVersion": "5.2.0",
  "executionStatus": "completed|failed|running",
  "risks": [
    {
      "name": "ConcerningThingA",
      "message": "Cluster uses dual-stack networking configuration incompatible with 5.2.0.",
      "url": "https://issues.redhat.com/browse/SDN-3996",
      "targetVersion": "5.2.0"
    }
  ]
}
```

The preflight CVO runs as a constantly-running Deployment while `mode: Preflight` is set, continuously monitoring cluster state and updating risk assessments as conditions change. This eliminates the need for result caching since risks are evaluated in real-time.

When preflight risks are identified, the cluster's running CVO lifts those identified risks up into ClusterVersion's `status.conditionalUpdateRisks`, merging with risks detected via other mechanisms (the OpenShift Update Service, etc.).
It also updates `status.conditionalUpdates` to set the preflight risk names in `status.conditionalUpdates([version==checkedVersion]).riskNames` for the version that was checked.

The preflight Deployment continues running until the administrator either:
- Clears the `desiredUpdate.mode: Preflight` setting
- Initiates an actual update by removing the `mode` field
- Sets a different target version for preflight evaluation

#### Retrieving preflight check results

**Successful preflight results:**
```yaml
  conditionalUpdateRisks:  # include every risk in the conditional updates (moved up and renamed)
  - name: ConcerningThingA
    message: Cluster uses dual-stack networking configuration incompatible with 5.2.0.
    url: https://issues.redhat.com/browse/SDN-3996
    matchingRules:
    - type: Always
    conditions:
    - status: True  # Apply=True if the risk is applied to the current cluster
      type: Applies
      reason: PreflightValidation
      message: Risk identified during preflight check for 5.2.0
      lastTransitionTime: 2021-09-13T17:03:05Z
```

**Failed or incomplete preflight results:**

When preflight checks fail or are still running, partial results are stored in `conditionalUpdateRisks` with special conditions indicating their incomplete status:

```yaml
  conditionalUpdateRisks:
  - name: PreflightIncomplete
    message: Preflight check for 5.2.0 failed to complete. Partial results may not reflect all compatibility risks.
    url: https://docs.openshift.com/preflight-troubleshooting
    conditions:
    - status: True
      type: Applies
      reason: PreflightFailed
      message: Preflight execution failed after checking 3 of 7 operators. Results are incomplete.
      lastTransitionTime: 2021-09-13T17:08:15Z
  - name: NetworkingCompatibilityUnknown
    message: Networking compatibility could not be determined due to preflight failure.
    conditions:
    - status: True
      type: Applies
      reason: PreflightPartial
      message: Risk assessment incomplete - networking operator preflight check failed
      lastTransitionTime: 2021-09-13T17:08:15Z
```

**Running preflight status:**

While preflight checks are actively running, interim results may be populated with progress indicators:

```yaml
  conditionalUpdateRisks:
  - name: PreflightInProgress
    message: Preflight check for 5.2.0 in progress. Results will be updated as checks complete.
    conditions:
    - status: True
      type: Applies
      reason: PreflightRunning
      message: 5 of 7 operators checked successfully, 2 remaining
      lastTransitionTime: 2021-09-13T17:05:30Z
```

**Result completeness indicators:**

The `reason` field in risk conditions clearly indicates the nature of preflight results:
- `PreflightValidation`: Complete, successful preflight check
- `PreflightFailed`: Preflight execution failed, results incomplete
- `PreflightPartial`: Some components checked successfully, others failed
- `PreflightRunning`: Check in progress, interim results only

#### Historical results retention policy

**Source version consistency**: Preflight results are maintained as long as the cluster remains on the same source version, allowing administrators to compare compatibility across multiple target versions for planning purposes:

- **Multiple target analysis**: Administrators can run sequential preflight checks against different target versions (5.1.0, 5.2.0, 5.3.0) from the same source version
- **Historical comparison**: Previous preflight results for different target versions are preserved in `conditionalUpdateRisks` until the cluster source version changes
- **Planning workflow**: Enables administrators to evaluate multiple upgrade paths before making decisions

**Result accumulation behavior**:
- **Additive storage**: Each new target version checked adds its results to the existing set
- **Target version identification**: Results are tagged with target version information for clear identification

**Example behavior**:
1. Administrator runs `oc adm upgrade --mode=preflight --to=5.1.0`
2. Results for 5.1.0 compatibility appear in `conditionalUpdateRisks`
3. Administrator runs `oc adm upgrade --mode=preflight --to=5.2.0`
4. Results for 5.2.0 compatibility are **added** alongside existing 5.1.0 results
5. Both sets of results remain available for comparison until cluster upgrades

**Rationale for accumulation approach**:
- **Comprehensive planning**: Administrators can compare risks across multiple target versions simultaneously
- **Workflow efficiency**: Avoids needing to re-run earlier preflight checks when evaluating additional targets
- **Decision support**: Side-by-side comparison of preflight results aids in choosing optimal upgrade paths

**Example ClusterVersion status with accumulated results**:
```yaml
apiVersion: config.openshift.io/v1
kind: ClusterVersion
spec:
  clusterID: 8c065c5e-97b5-4d8a-8ae5-b26f54bb9c7f
  desiredUpdate:
    mode: Preflight
    version: "5.3.0"
status:
  conditionalUpdateRisks:
  # Results from preflight check targeting 5.1.0
  - name: "NetworkingDualStackDeprecated"
    message: "Dual-stack networking configuration will be deprecated in 5.1.0. Migration to single-stack recommended."
    url: "https://docs.openshift.com/container-platform/5.1/networking/dual-stack-migration.html"
    conditions:
    - type: Applies
      status: True
      reason: PreflightValidation
      message: "Risk identified during preflight check for 5.1.0"
      lastTransitionTime: "2026-02-09T10:30:00Z"
  # Results from preflight check targeting 5.2.0
  - name: "StorageCSIDriverIncompatible"
    message: "Current CSI driver version 2.3.1 incompatible with OpenShift 5.2.0. Minimum version 2.4.0 required."
    url: "https://issues.redhat.com/browse/STOR-4521"
    conditions:
    - type: Applies
      status: True
      reason: PreflightValidation
      message: "Risk identified during preflight check for 5.2.0"
      lastTransitionTime: "2026-02-09T11:15:00Z"
  - name: "MachineConfigKernelVersionMismatch"
    message: "Current kernel version 4.18.0 may have performance degradation with 5.2.0 container runtime."
    url: "https://docs.openshift.com/container-platform/5.2/updating/kernel-compatibility.html"
    conditions:
    - type: Applies
      status: True
      reason: PreflightValidation
      message: "Risk identified during preflight check for 5.2.0"
      lastTransitionTime: "2026-02-09T11:15:00Z"
  # Results from preflight check targeting 5.3.0 (currently running)
  - name: "PreflightInProgress"
    message: "Preflight check for 5.3.0 in progress. Results will be updated as checks complete."
    conditions:
    - type: Applies
      status: True
      reason: PreflightRunning
      message: "6 of 8 operators checked successfully, 2 remaining"
      lastTransitionTime: "2026-02-09T12:05:30Z"
```

#### Preflight data lifecycle and cluster upgrades

**Problem addressed**: Preflight results that persist after cluster upgrades can cause confusion about their relevance to the current cluster state. For example, if a cluster upgrades from 5.0.0 → 5.1.0 after running preflight checks for 5.2.0, administrators might incorrectly assume the 5.2.0 preflight results are still valid for the upgraded cluster.

**Solution**: Automatic preflight data cleanup ensures results remain current and relevant:

**Behavior after cluster upgrades**: When a cluster upgrades (e.g., 5.0.0 → 5.1.0), preflight results from the previous version are automatically cleared from `conditionalUpdateRisks` to prevent confusion. This ensures administrators see only results that apply to the current cluster state.

**Automatic cleanup**: Preflight results are automatically cleared when:
- **Cluster upgrade completes**: When the actual cluster version changes (e.g., 5.0.0 → 5.1.0), all preflight results from the previous source version are cleared since they are no longer relevant to the new cluster state
- **Source version detection**: The CVO detects the cluster version has changed since preflight results were generated and automatically removes stale results
- **Manual clearing**: Administrator explicitly clears preflight mode (`desiredUpdate.mode` removed) or requests different target version evaluation

### Risks and Mitigations

#### Security Risks

**Risk**: Running target release CVO with cluster access could potentially expose sensitive information or allow unintended modifications.
**Mitigation**: The preflight CVO will use the same system-admin ServiceAccount as the standard CVO, enabling it to launch secondary Deployments and interact with 2nd-level operators (etcd operator, Kube API server operator, etc.) for comprehensive compatibility checks. This approach recognizes that preflighting requires complete trust in the target release, since administrators are evaluating that release for a subsequent update. Security review will be conducted by the OCP Security team during implementation.

**Risk**: Target release images might contain vulnerabilities or malicious code.
**Mitigation**: Preflight checks only run against official OpenShift release images that have passed the same security scanning and approval process as regular updates.

#### Resource and Performance Risks

**Risk**: Preflight checks could consume significant cluster resources, impacting production workloads.
**Mitigation**:
- Implement resource limits and quotas for preflight CVO deployments
- Provide configurable timeout controls
- Allow administrators to schedule preflight checks during maintenance windows
- Monitor resource usage during beta testing

**Risk**: Long-running preflight checks could interfere with normal cluster operations.
**Mitigation**:
- Design preflight checks to be read-only and non-blocking
- Implement proper cleanup mechanisms for failed or interrupted preflight runs
- Provide clear documentation on expected execution times

#### Operational Risks

**Risk**: Complex error scenarios during preflight execution could confuse administrators.
**Mitigation**:
- Comprehensive error handling and reporting
- Clear distinction between preflight infrastructure failures and compatibility risks
- Detailed documentation and troubleshooting guides

**Risk**: Inconsistent or unreliable preflight results could undermine trust in the update process.
**Mitigation**:
- Extensive testing across different cluster configurations
- Component maintainer guidelines for writing reliable compatibility checks
- Fallback mechanisms when preflight checks cannot complete

#### API and Compatibility Risks

**Risk**: Changes to the preflight API could break existing automation tools.
**Mitigation**:
- Follow standard OpenShift API versioning and deprecation policies
- Provide clear migration guides for API changes
- Maintain backward compatibility during maturation phases

**Review Process**:
- **Security Review**: OCP Security team will review RBAC models and preflight execution sandbox
- **API Review**: Kubernetes API team will review ClusterVersion API extensions
- **UX Review**: OpenShift UX team will review `oc` command integration and error messaging

### Drawbacks

#### Resource Overhead
**Drawback**: Running preflight checks requires downloading and executing target release images, which consumes additional storage, network bandwidth, and compute resources.
**Impact**: Clusters with limited resources may experience performance degradation during preflight execution.
**Consideration**: The benefits of proactive risk assessment may not justify the resource cost for all cluster types, especially in resource-constrained environments.

#### Implementation Complexity
**Drawback**: This enhancement adds significant complexity to the cluster version operator and introduces new failure modes.
**Impact**: Increased maintenance burden for the OTA team and potential for new bugs affecting core update functionality.
**Consideration**: The complexity may outweigh the benefits if existing `Upgradeable` mechanisms prove sufficient for most update scenarios.

#### False Positives and User Experience
**Drawback**: Preflight checks may generate false positive warnings that could discourage administrators from performing necessary updates.
**Impact**: Risk aversion based on inaccurate preflight results could lead to clusters remaining on vulnerable or obsolete versions.
**Consideration**: The accuracy of component compatibility checks depends on the quality of implementation by individual operator teams.

#### Limited Scope of Initial Implementation
**Drawback**: The initial implementation focuses only on cluster-level orchestration without defining specific operator-level compatibility checks.
**Impact**: The actual utility of preflight checks will depend on future operator development work that is not guaranteed to happen.
**Consideration**: The infrastructure investment may not deliver immediate value to cluster administrators.

#### Alternative Workflow Disruption
**Drawback**: Introducing a new preflight workflow may complicate existing update automation and administrative procedures.
**Impact**: Teams will need to modify their update practices to incorporate preflight checks, adding procedural overhead.
**Consideration**: The additional step in the update workflow may slow down routine maintenance windows without proportional benefit.

## Alternatives (Not Implemented)

### Dedicated preflight object

An alternative approach suggested during review would introduce a separate preflight Custom Resource that could be created independently for update validation:

```yaml
apiVersion: config.openshift.io/v1alpha1
kind: PreflightCheck
metadata:
  name: test-5-2-upgrade
spec:
  targetVersion: "5.2.0"
status:
  phase: Running|Completed|Failed
  results:
    risks: [...]
    conditions: [...]
```

**Why this approach was not chosen:**
- **API proliferation**: Introduces a new API resource when existing ClusterVersion is the natural place for update-related operations
- **User workflow complexity**: Forces administrators to manage separate objects instead of extending familiar `oc adm upgrade` workflow
- **Lifecycle management**: Requires additional cleanup logic for abandoned preflight objects vs. automatic cleanup with ClusterVersion integration
- **Consistency**: Update-related operations should be centralized in the ClusterVersion API for operational simplicity
- **Multiple preflights**: While this approach would enable multiple concurrent preflight checks (addressing the review comment about comparisons), the additional complexity outweighs the benefit for the initial implementation

**Future enhancement**: Support for multiple concurrent preflight checks could be added to the chosen approach by allowing the `version` field to accept a list of target versions (e.g., `version: ["5.1.0", "5.2.0", "5.3.0"]`) without requiring a separate API resource.

### Multiple concurrent preflight checks for version comparison

During review, a use case was raised about comparing preflight results between different target versions (e.g., comparing 5.9 vs 6.0 when approaching end-of-support for version 5.x). An alternative approach would allow multiple concurrent preflight checks by accepting a list of target versions:

```yaml
spec:
  desiredUpdate:
    mode: Preflight
    version: ["5.9.12", "6.0.3"]
```

**Why this approach was not chosen for initial implementation:**

- **Implementation complexity**: Supporting concurrent preflight deployments for multiple target versions requires complex orchestration and resource coordination
- **Resource contention**: Multiple preflight CVOs running simultaneously compete for cluster resources and may interfere with each other
- **Status management**: Tracking progress and results for multiple concurrent preflight operations adds complexity to the ClusterVersion status model
- **Error handling complexity**: Failures in one target version check affect the overall preflight operation, requiring sophisticated partial failure handling

**Serial workflow alignment**: The chosen serial approach aligns well with existing OpenShift channel management workflows. Administrators comparing versions like 5.9 vs 6.0 typically:

1. Set channel to `*-5.9` to see available 5.9.z versions and run preflight checks
2. Capture preflight results and assess risks
3. Set channel to `*-6.0` to see available 6.0.z versions and run preflight checks
4. Compare captured results to make informed upgrade decisions

This channel-switching workflow already exists for version comparison, and serial preflight execution fits naturally into this pattern without adding API complexity.

**Future consideration**: If concurrent preflights become a common need, support could be added by allowing the `version` field to accept a list of target versions while maintaining the simple single-version API for the majority use case.

### One-shot preflight checks

An alternative approach considered was implementing one-time preflight checks instead of long-running continuous validation. This approach would use a unique identifier for each preflight request:

```go
// preflight is an identifier for a preflight attempt, typically a timestamp
// +kubebuilder:validation:Pattern:=^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z$
// +optional
preflight string `json:"preflight,omitempty"`
```

allowing one-time preflight requests like:

```yaml
spec:
  desiredUpdate:
    preflight: "2026-01-23T15:30:00Z"
    version: 5.2.0
```

Results would be cached temporarily in ClusterVersion status:

```yaml
conditions:
- status: True
  type: PreflightComplete
  reason: ValidationFinished
  message: "Preflight check 2026-01-23T15:30:00Z completed. Results valid until 2026-01-24T15:30:00Z."
  lastTransitionTime: 2026-01-23T15:35:05Z
```

**Why this approach was not chosen:**
- **Cache complexity**: Requires complex cache invalidation logic to determine when cluster state changes affect preflight results
- **Stale results**: Risk of administrators relying on outdated preflight results that no longer reflect current cluster state
- **Resource inefficiency**: Repeated one-shot checks consume more resources than a single long-running validation process
- **User experience**: Administrators must manually re-run checks to ensure current compatibility, adding operational overhead

### Backporting compatibility checks to current releases

This alternative follows the pattern suggested in [PR #363](https://github.com/openshift/enhancements/pull/363), where operators backport compatibility logic to earlier releases rather than running checks from target releases.

**How it would work:**
- When a breaking change is introduced in version N+1, operators backport detection logic to version N
- The current cluster's operators evaluate their own compatibility with future versions
- No target release execution required

**Why this approach was not chosen:**
- **Maintenance burden**: Requires maintaining compatibility checks across multiple release branches
- **Incomplete knowledge**: Earlier releases may not have complete understanding of future requirements
- **Release coordination**: Complex synchronization needed between component teams for backported checks
- **Skip-level limitations**: Cannot easily check compatibility for multiple skip-level targets (N+2, N+3)
- **Community resistance**: Previous attempt in [PR #363](https://github.com/openshift/enhancements/pull/363) was rejected by the community for these reasons

### External preflight plugins

An alternative approach would allow cluster administrators to register custom preflight validation plugins for specific target versions.

**How it would work:**
- Provide a plugin framework for custom compatibility checks
- Allow registration of external validation tools
- Execute third-party checks alongside OpenShift component validation

**Why this approach was not chosen:**
- **Security concerns**: External code execution requires complex sandboxing and validation
- **Support complexity**: Red Hat support cannot troubleshoot customer-specific validation plugins
- **Scope limitation**: Enhancement focuses on OpenShift component compatibility, not general-purpose validation framework
- **Maintenance overhead**: Plugin compatibility across OpenShift versions would create additional burden
- **Existing alternatives**: Administrators can already create custom ClusterOperators with `Upgradeable=False` conditions for custom blocking logic


## Open Questions [optional]

### Component Developer Interface for Preflight Checks

**Question**: As a developer on a team that is not CVO, how am I expected to write a preflight check and have CVO execute it? *(raised by @JoelSpeed)*

**Current Status**: The interface between the target-release CVO and target-release operators must be defined during Tech Preview development to enable component teams to implement preflight checks in time for GA.

**Proposed Direction**: Three candidate approaches will be evaluated during early Tech Preview development:

1. **Standardized preflight command**: Operators implement a `--preflight` flag similar to the CVO:
   ```bash
   # Example for the machine-config-operator
   machine-config-operator --preflight --format=preflight-v1-json
   ```

2. **CRD-based interface**: Target release operators create preflight-specific CRDs that the target CVO queries:
   ```yaml
   apiVersion: operator.openshift.io/v1
   kind: PreflightResult
   metadata:
     name: machine-config-preflight
   status:
     risks: [...]
   ```

3. **Shared library approach**: Common preflight utility functions that operators import and use in a standardized way.

**Selection Timeline**: The final interface will be chosen by OpenShift 4.22-beta based on:
- Technical feasibility and implementation complexity
- Component team feedback and adoption readiness
- Integration reliability with CVO orchestration
- Alignment with existing OpenShift operator patterns

**Risk Mitigation**: If interface consensus cannot be reached, the CVO will implement critical compatibility checks directly for Tech Preview, with operator delegation added incrementally post-GA.

### Multiple Concurrent Preflight Checks

**Question**: Is there a way I could request multiple pre-flights? Perhaps I want to compare results? *(raised by @JoelSpeed)*

**Current Approach**: Serial preflight execution with result accumulation. Administrators can run sequential preflight checks against different target versions, and results are preserved for comparison.

**Future Enhancement Possibility**: Support for multiple concurrent preflight checks could be added by allowing the `version` field to accept a list of target versions (e.g., `version: ["5.1.0", "5.2.0", "5.3.0"]`), but this adds implementation complexity for resource coordination and status management.

**Design Trade-off**: The initial implementation prioritizes simplicity and reliability over concurrent execution, aligning with existing OpenShift channel management workflows where administrators typically evaluate versions sequentially.

### HyperShift Compatibility

**Question**: Are we likely to design something here that doesn't work for HyperShift later? *(raised by @JoelSpeed)*

**Assessment**: The chosen design should be compatible with future HyperShift integration, but formal review from HyperShift maintainers (@enxebre @csrwng) is requested.

**Compatibility Factors**:
- HyperShift already runs CVO as a child Deployment, so preflight CVO deployment pattern should be adaptable
- ClusterVersion API extensions are minimal and could be exposed through HostedCluster CRDs
- File-based result sharing can be adapted to hosted control plane architecture

**Risk Mitigation**: If design proves incompatible, the minimal API surface allows refactoring without major breaking changes to user-facing workflows.

## Test Plan

### Unit Tests
- **CVO preflight mode processing**: Test API validation, request parsing, and result integration
- **ClusterVersion API extensions**: Test new `mode` field validation and status updates
- **Error handling**: Test various failure scenarios and error propagation

### Integration Tests
- **End-to-end preflight workflow**: Test complete workflow from `oc adm upgrade --mode=preflight` to result display, including integration with accepted-risks workflow
- **RBAC and security**: Verify preflight CVO runs with system-admin ServiceAccount permissions and can launch secondary Deployments for 2nd-level operator compatibility checks
- **Resource management**: Test resource limits, timeouts, and cleanup mechanisms
- **Multi-version scenarios**: Test one representative preflight scenario end-to-end (e.g., minor version upgrade) and exercise key failure modes to ensure proper error handling
- **Failure recovery**: Test behavior when preflight checks fail or time out

### Testing Challenges
1. **Simulating target release compatibility issues**: Creating test scenarios that reliably reproduce specific compatibility problems
2. **Version skew testing**: Testing across multiple OpenShift versions requires maintaining test environments with different release versions
3. **Resource exhaustion scenarios**: Safely testing resource limit enforcement without impacting test infrastructure
4. **Component maintainer testing**: Ensuring individual operators implement reliable preflight checks requires coordination across multiple teams


## Graduation Criteria

This enhancement follows the standard OpenShift maturity progression from Tech Preview to GA, with specific criteria tailored to preflight functionality.

The initial target for OpenShift 4.22 is **Tech Preview**, as outlined in [OCPSTRAT-2843](https://issues.redhat.com/browse/OCPSTRAT-2843).

### Dev Preview -> Tech Preview

*This enhancement is targeting Tech Preview directly for OpenShift 4.22, skipping the Dev Preview phase due to the critical nature of cluster update functionality and the need for rapid deployment to support upcoming skip-level updates.*

For reference, if this enhancement had followed a Dev Preview phase, the following criteria would have been required:

- **Core preflight orchestration**: Basic `oc adm upgrade --mode=preflight` functionality working end-to-end for next-minor version targets
- **API stability**: `ClusterVersion.spec.desiredUpdate.mode` field finalized and integrated with existing update workflow
- **Minimal operator integration**: At least one core OpenShift operator implementing basic preflight compatibility checks to validate the framework
- **Safety validation**: Confirmed read-only operation with no cluster state modification during preflight execution
- **Resource management**: Basic resource limits and cleanup mechanisms preventing preflight operations from impacting cluster stability
- **Developer feedback**: Validation from OTA team and component maintainers on preflight execution model and result integration with `conditionalUpdateRisks`

Since we are proceeding directly to Tech Preview, these criteria are incorporated into the Tech Preview requirements below.

### Tech Preview (OpenShift 4.22)

**Core Functionality:**
- Cluster administrators can successfully execute `oc adm upgrade --mode=preflight --to=<version>` for next-minor version targets (e.g., 5.0 → 5.1)
- Preflight results integrate with `ClusterVersion.status.conditionalUpdateRisks` and accepted-risks workflow
- Target release CVO runs in preflight mode without cluster state modification
- Basic resource limits and cleanup mechanisms function correctly
- **Limited scope**: Skip-level updates (e.g., 5.0 → 5.2) and CI/CD integration are future enhancements beyond Tech Preview

**API Stability:**
- `ClusterVersion.spec.desiredUpdate.mode` API extension approved and implemented
- Integration with existing `conditionalUpdateRisks` API established with automatic cleanup after cluster upgrades
- `oc adm upgrade` command integration stable for Tech Preview usage, including `--mode=preflight`, `--clear-preflight`, and enhanced `--status-preflight` commands

**Documentation and Testing:**
- End-user documentation for preflight workflow created in [openshift-docs](https://github.com/openshift/openshift-docs/)
- Integration tests covering end-to-end preflight execution
- Unit tests for CVO preflight mode processing and API validation
- Performance and resource usage testing on representative cluster configurations

**Component Integration:**
- **Component Developer Interface Defined**: Standard interface for operators to implement preflight checks established, including:
  - Technical specification for one of the proposed patterns (command-line flag, CRD-based, or shared library)
  - Developer documentation and guidelines for component maintainers
  - Reference implementation and testing framework for operator preflight checks
- At least two OpenShift operators (e.g., networking, machine-config) implement preflight compatibility checks using the defined interface
- Integration with the accepted-risks workflow tested and validated
- Comprehensive error handling and failure recovery mechanisms implemented with detailed troubleshooting guides

### Tech Preview -> GA

**Production Readiness:**
- Extensive testing across different cluster topologies (single-node, multi-node, various cloud providers)
- Performance analysis and optimization for resource consumption during preflight execution
- Complete security review including RBAC model for preflight CVO execution
- Load testing with multiple concurrent preflight requests

**Operator Ecosystem Adoption:**
- Multiple core OpenShift operators implement preflight checks (minimum 5 operators)
- Component maintainer guidelines and best practices documentation published
- Operator preflight check reliability validated across upgrade scenarios

**Operational Stability:**
- SLI metrics defined and exposed for preflight execution success/failure rates
- Monitoring and alerting for preflight infrastructure health
- Troubleshooting runbooks and support procedures documented
- Failure mode analysis completed with documented recovery procedures

**Enhanced User Experience:**
- Console integration for preflight results viewing (may be post-GA)
- Enhanced `oc` output formatting and error messaging
- Integration with automated update workflows tested
- Fleet management tooling compatibility validated

**API Maturation:**
- API version stability guaranteed for GA lifecycle
- Deprecation policy established for any Tech Preview API changes
- Backward compatibility testing across supported OpenShift version skew

**Field Testing:**
- Minimum 6 months of Tech Preview usage with customer feedback incorporation
- Validation in diverse customer environments including edge deployments
- Performance characteristics documented under various cluster conditions
- Integration with Red Hat support processes established

### Future Enhancements (Post-GA)

**Extended Topology Support:**
- HyperShift/Hosted Control Planes integration ([deferred from initial scope](#hypershift--hosted-control-planes))
- Managed OpenShift service workflow integration
- MicroShift compatibility analysis

**Advanced Features:**
- Skip-level update preflight validation (N+2, N+3 versions)
- Scheduled/continuous preflight monitoring capabilities
- Integration with CI/CD pipeline automation tools

### Removing a deprecated feature

*This section addresses potential deprecation scenarios for preflight functionality, though such deprecation is not currently planned given the enhancement's role in supporting upcoming skip-level updates and safe cluster modernization.*

Given the strategic importance of preflight checks for OpenShift's update ecosystem, deprecation would only be considered if:

**Replacement technology emerges:**
- A fundamentally superior approach to update risk assessment is developed
- Upstream Kubernetes provides equivalent functionality that supersedes OpenShift's implementation
- New OpenShift architecture makes cluster-level preflight checks obsolete

**Deprecation process if required:**

1. **Extended notice period**: Minimum 3 major version deprecation timeline (longer than standard 2 versions) due to critical update infrastructure dependency:
   - Announce in version N (e.g., 4.28)
   - Mark deprecated in version N+1 (e.g., 4.29)
   - Remove in version N+3 (e.g., 4.31)

1. **Migration strategy**:
   - Develop enhanced `ClusterOperator.Upgradeable` conditions to replace preflight functionality
   - Provide automated migration tools for existing operator preflight implementations
   - Ensure skip-level update support is maintained through alternative mechanisms

1. **Ecosystem coordination**:
   - Work with all operator teams implementing preflight checks to plan removal timeline
   - Coordinate with Red Hat managed services team to update automation dependencies
   - Update cluster update service integration points

1. **Backward compatibility**:
   - `ClusterVersion.spec.desiredUpdate.mode` field remains accepted but ignored during deprecation period
   - Existing `oc adm upgrade --mode=preflight` commands return helpful migration guidance
   - No disruption to standard update workflows that don't use preflight mode

[maturity-levels]: https://git.k8s.io/community/contributors/devel/sig-architecture/api_changes.md#alpha-beta-and-stable-versions
[deprecation-policy]: https://kubernetes.io/docs/reference/using-api/deprecation-policy/

## Upgrade / Downgrade Strategy

The preflight enhancement introduces new API fields and behavior that must be handled gracefully during cluster upgrades and downgrades.

### Upgrading TO a cluster with preflight support

**From clusters without preflight (pre-4.22) to clusters with preflight (4.22+):**

1. **API Compatibility**: The new `ClusterVersion.spec.desiredUpdate.mode` field is optional and backward-compatible. Existing update requests without the `mode` field continue to work as before.

1. **No automatic activation**: Preflight functionality is not enabled automatically. Administrators must explicitly request preflight checks using `oc adm upgrade --mode=preflight` or by setting the API field directly.

1. **Existing behavior preserved**: All existing update workflows (`oc adm upgrade`, direct API manipulation) continue to function identically to pre-preflight behavior when the `mode` field is not specified.

1. **CVO compatibility**: The cluster-version-operator in 4.22+ gracefully ignores preflight requests when targeting pre-4.22 releases that don't support preflight mode, logging an appropriate warning.

### Upgrading FROM a cluster with preflight support

**During cluster upgrades while preflight is active:**

1. **Preflight termination**: When a real update is initiated (by clearing `mode: Preflight`), any running preflight CVO deployment is gracefully terminated before the actual update begins.

1. **Resource cleanup**: Preflight-related deployments, volumes, and temporary resources are cleaned up as part of the normal CVO update process.

1. **State preservation**: Preflight results in `conditionalUpdateRisks` are preserved during the upgrade and remain available for historical reference.

1. **Version skew handling**: During upgrades, the newer CVO version supports both preflight and non-preflight update requests, maintaining compatibility during the rollout.

### Downgrade Strategy

**From clusters with preflight (4.22+) to clusters without preflight (pre-4.22):**

1. **API field removal**: When downgrading, the `mode` field in `ClusterVersion.spec.desiredUpdate` will be ignored by the older CVO version. Administrators should manually clear any preflight configurations before downgrading.

1. **Resource cleanup**: Any running preflight CVO deployments must be manually terminated before downgrade:
   ```bash
   oc patch clusterversion version --type='merge' -p '{"spec":{"desiredUpdate":{"mode":null}}}'
   oc delete deployment cluster-version-operator-preflight -n openshift-cluster-version-operator --ignore-not-found
   ```

1. **Graceful degradation**: Older CVO versions ignore unknown API fields, so the presence of preflight-related status fields doesn't cause upgrade failures.

1. **Manual intervention**: If downgrade fails due to preflight-related resources, administrators can manually clean up:
   - Remove preflight deployments from `openshift-cluster-version-operator` namespace
   - Clear preflight-specific conditions from ClusterVersion status (these will be automatically removed by the older CVO)

### Version Skew Considerations

1. **Patch version upgrades**: Preflight functionality operates at the minor version boundary (4.21 to 4.22), so patch version updates (4.22.1 to 4.22.3) do not affect preflight compatibility.

1. **Minor version upgrades**: Preflight checks can target next-minor versions (5.0 checking 5.1) but may have limited functionality when checking much newer versions that introduce preflight protocol changes.

1. **Component compatibility**: Individual operators implementing preflight checks must handle version skew gracefully, providing clear error messages when preflight checks cannot be performed due to version incompatibilities.

### Feature Gate Dependencies

1. **Tech Preview feature gate**: The initial 4.22 implementation requires the `TechPreviewNoUpgrade` feature gate to be enabled.

1. **Dedicated feature gate**: Future versions may introduce a specific `ClusterUpdatePreflight` feature gate for more granular control.

1. **Upgrade path**: When the feature graduates from Tech Preview to GA, the feature gate requirement will be removed, but the functionality remains opt-in via explicit API usage.

## Version Skew Strategy

The preflight enhancement must handle version skew scenarios gracefully, particularly since it involves running target release code alongside current cluster components.

### CVO Version Skew

**Current cluster CVO vs. Target release CVO:**
- The preflight feature runs the target release CVO as a separate deployment, creating intentional version skew
- The current cluster CVO (N) orchestrates the target release CVO (N+1 or N+2) execution
- Communication between CVOs happens via shared volumes and API objects, not direct process communication
- The preflight JSON format includes versioning (`"format": "preflight-v1-json"`) to handle protocol evolution

**Compatibility matrix:**
- **Current 4.22+ CVO, Target 4.22+ release**: Full preflight support with all features
- **Current 4.22+ CVO, Target pre-4.22 release**: Graceful degradation, logs warning that target doesn't support preflight mode
- **Current pre-4.22 CVO**: No preflight support - feature requires CVO upgrade to 4.22+

### Node Component Skew

**Kubelet compatibility:**
- Preflight checks are control plane operations and do not directly interact with kubelet APIs
- Node-level compatibility checks (container runtime versions, kernel compatibility) are performed by reading cluster state, not by coordinating with individual kubelets
- The preflight CVO deployment uses standard Kubernetes scheduling and does not require specific kubelet versions

**Container runtime and networking:**
- Preflight checks examine container runtime configurations through Kubernetes APIs (ImageContentSourcePolicy, etc.)
- Networking compatibility checks query existing CNI configurations rather than installing new network components
- No direct coordination with CSI, CRI, or CNI components during preflight execution

### Operator Version Skew

**Target release operators vs. current cluster operators:**
- Preflight checks run target release operators in read-only mode without replacing current operators
- Target release operators examine current cluster configurations but do not modify them
- Current cluster operators continue normal operations unaffected by preflight execution
- Resource conflicts avoided by running preflight operations in isolated namespaces or with dedicated service accounts

**Progressive operator updates:**
- During actual cluster upgrades, operators update independently, creating temporary skew
- Preflight results remain valid during operator skew periods since they represent point-in-time compatibility assessments
- Long-running preflight deployments re-evaluate compatibility as cluster state changes during upgrades

### API Version Skew

**ClusterVersion API compatibility:**
- The new `mode` field is optional and backward-compatible with existing API consumers
- Older `oc` clients ignore the unknown `mode` field without errors
- Newer `oc` clients gracefully handle older CVO versions that don't support the `mode` field

**Custom Resource compatibility:**
- Preflight checks may encounter CRDs that don't exist in the target release
- Target release operators must handle missing CRDs gracefully, reporting compatibility concerns rather than failing
- Schema evolution between releases is handled through standard CRD versioning mechanisms

### Error Handling for Version Skew

**Incompatible target releases:**
- When targeting releases more than 2 major versions newer, preflight may not be supported
- Clear error messages indicate version compatibility limitations
- Fallback to existing `Upgradeable` conditions when preflight cannot execute

**Partial functionality:**
- Preflight may provide limited results when significant version skew prevents complete evaluation
- Administrators receive clear indications of which checks succeeded vs. which failed due to version compatibility

**Testing requirements:**
- Version skew scenarios included in integration test matrix
- Specific test cases for N-1, N, N+1, N+2 version combinations
- Error path validation for unsupported version combinations

## Operational Aspects of API Extensions

The preflight enhancement extends the existing ClusterVersion API with a new optional field, which has minimal operational impact compared to introducing new CRDs or admission webhooks.

### API Extension: ClusterVersion.spec.desiredUpdate.mode

**Type**: Extension to existing core OpenShift API resource
**Scope**: Single optional field addition to ClusterVersion specification
**Impact**: Minimal - leverages existing kube-apiserver infrastructure

### Service Level Indicators (SLIs)

**Health monitoring for preflight operations:**
- **CVO condition monitoring**: `ClusterVersion.status.conditions` includes preflight-specific conditions:
  - `type: PreflightRunning` - indicates active preflight deployment
  - `type: PreflightCompleted` - indicates successful preflight execution
  - `type: PreflightFailed` - indicates preflight execution failures

- **Deployment health**: Monitor preflight CVO deployment in `openshift-cluster-version-operator` namespace:
  ```bash
  oc get deployment cluster-version-operator-preflight -n openshift-cluster-version-operator
  oc describe pods -l app=cluster-version-operator-preflight
  ```

- **Resource consumption metrics**: Standard Kubernetes metrics for preflight deployments:
  - CPU/memory usage of preflight CVO pods
  - Volume mount success/failure for preflight result sharing
  - Pod scheduling and execution times

### Impact on Existing SLIs

**API throughput and availability:**
- **Minimal impact**: The `mode` field adds ~10 bytes to ClusterVersion API requests
- **No webhook dependencies**: Uses standard kube-apiserver field validation only
- **No additional API calls**: Preflight execution does not generate additional API traffic beyond normal CVO operations
- **Expected usage**: <1000 preflight requests per cluster per day, well within ClusterVersion API capacity

**Cluster component performance:**
- **CVO scheduling overhead**: Launching preflight CVO deployments adds minimal scheduling load (~1 deployment per preflight request)
- **Storage I/O impact**: Preflight result sharing uses hostPath volumes, adding minimal disk I/O
- **Network impact**: Target release image pulls required, similar to existing update process

**Critical path isolation:**
- **Update path separation**: Preflight operations do not block normal cluster updates
- **Resource isolation**: Preflight CVO runs with constrained resource limits to prevent interference
- **Namespace isolation**: Preflight operations use standard CVO namespace without additional privilege escalation

### Impact Measurement and Testing

**Continuous monitoring:**
- **CI integration**: Automated testing in OpenShift CI measures API response times with and without preflight API changes
- **Performance regression testing**: QE team validates ClusterVersion API throughput remains within established SLOs
- **Resource usage baselines**: Platform team establishes resource consumption baselines for preflight CVO deployments

**Responsible teams:**
- **OTA team**: Primary ownership of preflight functionality and performance characteristics
- **API machinery team**: Reviews ClusterVersion API extension for compliance with OpenShift API standards
- **QE performance team**: Validates preflight operations don't regress cluster update performance

### Failure Modes and Impact Analysis

**API field validation failures:**
1. **Failure mode**: Invalid `mode` field values rejected by kube-apiserver
1. **Impact**: Client receives 400 Bad Request, no cluster functionality affected
1. **Recovery**: Client corrects request syntax, no administrator intervention required

**Preflight CVO deployment failures:**
1. **Failure mode**: Target release CVO fails to start (image pull errors, resource constraints)
1. **Impact**: Preflight request fails, but cluster updates and normal operations unaffected
1. **Recovery**: Automatic cleanup after timeout, administrator can retry preflight or proceed with update

**Resource exhaustion:**
1. **Failure mode**: Preflight CVO consumes excessive CPU/memory
1. **Impact**: Potential performance degradation, but resource limits prevent cluster instability
1. **Recovery**: Automatic termination when resource limits exceeded, deployment cleaned up

**Shared volume failures:**
1. **Failure mode**: Result sharing between CVOs fails due to volume mount issues
1. **Impact**: Preflight results unavailable, but cluster operations continue normally
1. **Recovery**: Preflight deployment restart, temporary result loss acceptable

### Security and Privilege Impact

**RBAC requirements:**
- **Orchestrator privileges**: Preflight CVO deployment uses same system-admin ServiceAccount as standard CVO for orchestration and operator coordination
- **Restricted operator access**: Individual target release operator preflight checks run with cluster-reader equivalent permissions to ensure read-only cluster assessment
  - Explicit deny rules for `create`, `update`, `patch`, `delete` operations on cluster-scoped resources
  - Read-only access to `get`, `list`, `watch` operations for compatibility assessment
  - Audit logging enabled for all preflight ServiceAccount activities to ensure compliance
- **Namespace isolation**: Preflight operations contained within existing CVO namespace boundaries with additional isolation for operator checks
- **API enforcement**: kube-apiserver RBAC provides technical enforcement of read-only restrictions, preventing accidental cluster state modification even if preflight code attempts write operations

**Resource management and cleanup:**
- **Deployment tracking**: CVO maintains a registry of all preflight-related deployments and pods created during validation
  - Preflight deployments use consistent labeling (`app=cluster-version-operator-preflight`, `preflight-target=<version>`) for identification
  - CVO monitors and cleans up preflight resources when checks complete or are cancelled
  - Timeout handling ensures abandoned preflight resources are cleaned up automatically (default 1-hour timeout)
- **Centralized write prevention**: RBAC enforcement occurs at multiple levels:
  - **ServiceAccount restrictions**: Preflight deployments use dedicated ServiceAccounts with explicit deny rules for write operations
  - **API server enforcement**: Kubernetes RBAC denies write attempts at the API level, providing technical enforcement regardless of code behavior
  - **Admission controller validation**: Any attempted write operations by preflight code trigger RBAC denials and audit log entries
- **Resource isolation**: Each preflight check runs in isolated context with dedicated ServiceAccount and resource limits to prevent interference

**Attack surface analysis:**
- **Minimal API surface expansion**: Single optional field with enum validation
- **No network exposure**: Preflight operations entirely internal to cluster
- **Image security**: Target release images undergo same security scanning as update images
- **Controlled execution environment**: Preflight deployments run with same isolation as standard OpenShift operators but with read-only RBAC constraints

### Escalation and Support Procedures

**Primary escalation path**: OTA team
**Secondary escalation**:
- **API issues**: Platform API team
- **Performance regression**: QE performance team
- **Security concerns**: OCP Security team

**Team expertise areas:**
- **OTA team**: Preflight orchestration, CVO integration, update workflow
- **Component teams**: Individual operator preflight check implementation
- **Platform team**: ClusterVersion API compliance and performance impact

## Support Procedures

### Detecting Preflight Failures

**Symptoms of preflight execution problems:**

1. **Preflight deployment failures:**
   - **ClusterVersion status**: Check for conditions with `type: PreflightFailed`
   ```bash
   oc get clusterversion version -o yaml | grep -A5 'type: PreflightFailed'
   ```
   - **CVO logs**: Look for preflight-related errors in cluster-version-operator logs
   ```bash
   oc logs deployment/cluster-version-operator -n openshift-cluster-version-operator | grep preflight
   ```
   - **Deployment status**: Check if preflight CVO deployment exists and is running
   ```bash
   oc get deployment cluster-version-operator-preflight -n openshift-cluster-version-operator
   oc describe pods -l app=cluster-version-operator-preflight -n openshift-cluster-version-operator
   ```

1. **Image pull failures:**
   - **Event logs**: Check for ImagePullBackOff events in CVO namespace
   ```bash
   oc get events -n openshift-cluster-version-operator --field-selector reason=Failed
   ```
   - **Pod status**: Examine preflight pod status for image pull errors
   ```bash
   oc describe pod -l app=cluster-version-operator-preflight -n openshift-cluster-version-operator
   ```

1. **Resource exhaustion:**
   - **Pod status**: Check for OOMKilled or resource limit violations
   - **Metrics**: Monitor CPU/memory usage of preflight pods through cluster monitoring
   - **Node capacity**: Verify sufficient node resources for preflight deployment scheduling

1. **Volume mount failures:**
   - **Pod events**: Look for volume mount errors in preflight pod events
   - **Storage logs**: Check for hostPath volume permission or space issues
   - **CVO logs**: Look for preflight result retrieval failures

### Disabling Preflight Functionality

**To disable an active preflight check:**

1. **Clear preflight request (recommended):**
   ```bash
   oc adm upgrade --clear-preflight
   ```

2. **Alternative: Manual API patch:**
   ```bash
   oc patch clusterversion version --type='merge' -p '{"spec":{"desiredUpdate":{"mode":null}}}'
   ```

3. **Force cleanup of preflight resources (if needed):**
   ```bash
   oc delete deployment cluster-version-operator-preflight -n openshift-cluster-version-operator --ignore-not-found
   oc delete pod -l app=cluster-version-operator-preflight -n openshift-cluster-version-operator --ignore-not-found
   ```

**Consequences of disabling preflight functionality:**

**Cluster health impact:**
- **No cluster degradation**: Disabling preflight has no impact on cluster operations
- **Update capability preserved**: Normal cluster updates continue to function normally
- **Component operators unaffected**: All running workloads and operators continue without interruption

**Running workload impact:**
- **No disruption**: Existing applications and workloads continue running without any changes
- **No service interruption**: All OpenShift services (API, console, routing) remain fully functional
- **No data loss**: Preflight operations are read-only and don't modify cluster state

**New workload impact:**
- **No functional changes**: New pod creation, deployments, and services work normally
- **Update risk assessment unavailable**: Administrators lose proactive compatibility checking capability
- **Fallback to existing mechanisms**: Standard `Upgradeable` conditions and conditional update risks remain available

### Graceful Failure and Recovery

**Preflight failure behavior:**

1. **Automatic cleanup**: Failed preflight deployments are automatically cleaned up after timeout

1. **State preservation**: Partial preflight results are preserved in ClusterVersion status for troubleshooting

1. **Non-blocking operation**: Preflight failures do not prevent normal cluster updates from proceeding

1. **Clear error reporting**: Failure reasons are clearly documented in ClusterVersion conditions and CVO logs

**Recovery procedures:**

1. **Automatic retry**: Clear the preflight request and re-submit after addressing underlying issues
   ```bash
   # Clear current preflight request
   oc patch clusterversion version --type='merge' -p '{"spec":{"desiredUpdate":{"mode":null}}}'

   # Wait for cleanup, then retry
   oc adm upgrade --mode=preflight --to=<target-version>
   ```

1. **Resource constraint resolution**: Increase cluster resources or wait for resource availability

1. **Image access resolution**: Verify network connectivity and image registry access for target releases

1. **Consistency guarantees**: No risk of cluster state corruption since preflight operations are read-only

**Manual override procedures:**

If preflight checks repeatedly fail but administrators want to proceed with updates:

1. **Use existing update mechanisms**: Normal updates bypass preflight entirely
   ```bash
   oc adm upgrade --to=<target-version>  # No --mode flag
   ```

1. **Accept known risks**: Use the accepted-risks workflow to acknowledge specific compatibility concerns
   ```bash
   oc adm upgrade accept <risk-name>
   oc adm upgrade --to=<target-version>
   ```

1. **Force updates**: Use existing `--force` flag for emergency update scenarios (not recommended)

The preflight enhancement is designed to fail gracefully without impacting cluster stability or update capabilities.

## Enhanced Troubleshooting and Recovery Procedures

### Common Preflight Failure Scenarios

#### Scenario 1: Target Release Image Pull Authentication Failures

**Symptoms:**
- Preflight deployment shows `ImagePullBackOff` status
- CVO logs contain registry authentication errors
- `oc describe pod` shows "unauthorized: authentication required"

**Root Causes:**
- Expired pull secrets for target release registry
- Network connectivity issues to release registry
- Target release not yet available in configured registry

**Recovery Steps:**
1. **Verify pull secret validity:**
   ```bash
   oc get secret pull-secret -n openshift-config -o jsonpath='{.data.\.dockerconfigjson}' | base64 -d | jq
   ```
2. **Test registry connectivity:**
   ```bash
   oc debug node/<node-name> -- curl -I https://quay.io/v2/
   ```
3. **Update pull secrets if expired:**
   ```bash
   oc set data secret/pull-secret -n openshift-config --from-file=.dockerconfigjson=<new-pull-secret>
   ```
4. **Retry preflight after pull secret update:**
   ```bash
   oc adm upgrade --clear-preflight
   sleep 30
   oc adm upgrade --mode=preflight --to=<target-version>
   ```

**Escalation:** If registry connectivity persists, escalate to Red Hat Support with pull secret and network configuration details.

#### Scenario 2: Component Operator Preflight Check Failures

**Symptoms:**
- Preflight completes but reports multiple `PreflightPartial` conditions
- Specific operator compatibility checks return "unable to determine"
- Target CVO logs show individual operator check timeouts

**Root Causes:**
- Component operator preflight implementation bugs
- Resource constraints preventing operator check execution
- Incompatible CRD versions between releases

**Recovery Steps:**
1. **Identify failing components:**
   ```bash
   oc get clusterversion version -o yaml | grep -A10 "PreflightPartial"
   ```
2. **Review target CVO logs for operator-specific errors:**
   ```bash
   oc logs deployment/cluster-version-operator-preflight -n openshift-cluster-version-operator | grep -E "(operator|component)"
   ```
3. **Check resource availability for preflight operations:**
   ```bash
   oc describe nodes | grep -E "(cpu|memory)" | tail -10
   ```
4. **For known operator issues, check component maintainer guidance:**
   - Network operator: Check dual-stack and CNI configuration compatibility
   - Machine-config operator: Verify kernel and container runtime compatibility
   - Storage operator: Validate CSI driver version compatibility

**Escalation Path:**
- **Component-specific issues:** Contact individual component team (networking, machine-config, storage)
- **Cross-component coordination:** Escalate to OTA team for orchestration issues
- **Critical blocker:** Escalate to OpenShift Engineering Manager with impact assessment

#### Scenario 3: Partial Results Due to Version Skew

**Symptoms:**
- Preflight reports `PreflightVersionSkew` conditions
- Some operators successfully checked, others marked as "incompatible version"
- Target release is more than 2 minor versions newer than current

**Root Causes:**
- Attempting to check compatibility across unsupported version gaps
- API incompatibilities between current and target releases
- Preflight protocol version mismatches

**Recovery Steps:**
1. **Verify target version compatibility:**
   ```bash
   oc adm upgrade --to=<intermediate-version> --mode=preflight
   ```
2. **Use intermediate version stepping:**
   - For 5.0 → 5.3 checks, try 5.0 → 5.1 → 5.2 → 5.3 sequence
   - Document compatibility findings at each step
3. **Accept partial results for planning:**
   - Use available compatibility checks for initial risk assessment
   - Plan for additional validation after intermediate upgrades

**Escalation:** For business-critical version skew scenarios, escalate to OTA team for guidance on supported compatibility checking patterns.

### Component Maintainer Guidelines

#### Implementing Reliable Preflight Checks

**Design Principles for Component Teams:**

1. **Fail-Safe Operation:**
   ```go
   func (c *ComponentOperator) RunPreflightCheck() (*PreflightResult, error) {
       // Always default to conservative risk assessment
       if err := c.validateConfiguration(); err != nil {
           return &PreflightResult{
               Risks: []Risk{{
                   Name: "ComponentCheckFailed",
                   Message: fmt.Sprintf("Unable to validate compatibility: %v", err),
                   Severity: "warning",
               }},
           }, nil // Return success with warning, not error
       }
       // ... rest of check logic
   }
   ```

2. **Resource Awareness:**
   - Implement timeout handling (maximum 5 minutes per component check)
   - Use memory-efficient data structures for large cluster analysis
   - Gracefully degrade functionality under resource constraints

3. **Clear Error Messaging:**
   - Include actionable remediation steps in risk messages
   - Provide documentation links for complex compatibility issues
   - Use consistent severity levels: `critical`, `warning`, `info`

#### Component Testing Requirements

**Component teams must provide:**
1. **Unit tests** covering preflight check logic
2. **Integration tests** with various cluster configurations
3. **Failure mode testing** with resource constraints and API errors
4. **Version skew testing** across supported OpenShift version ranges

#### Component Check Performance Standards

- **Maximum execution time:** 300 seconds per component
- **Memory usage limit:** 100MB per component check process
- **API call efficiency:** Batch operations where possible, respect rate limits
- **Error handling:** Graceful degradation, never crash the preflight CVO

### Escalation Matrix and Procedures

#### Tier 1 - Operational Issues (SRE/Admin Response)

**Scope:** Infrastructure failures, resource exhaustion, authentication issues
**Response Time:** 30 minutes during business hours
**Team:** OpenShift SRE, Customer SRE teams

**Common Issues:**
- Preflight deployment scheduling failures
- Registry authentication problems
- Resource quota exceeded
- Volume mount permission errors

**Resolution Authority:**
- Restart preflight deployments
- Adjust resource quotas within approved limits
- Apply authentication fixes
- Clear transient preflight state

#### Tier 2 - Component Integration Issues (Component Teams)

**Scope:** Individual operator preflight check failures, component-specific compatibility issues
**Response Time:** 4 business hours
**Team:** Component team engineers (networking, machine-config, storage, etc.)

**Common Issues:**
- Operator preflight implementation bugs
- Component-specific compatibility edge cases
- CRD version conflicts
- Component resource requirement changes

**Resolution Authority:**
- Fix component preflight check implementation
- Provide compatibility workarounds
- Update component-specific documentation
- Coordinate with OTA team for CVO integration issues

#### Tier 3 - Architecture/Design Issues (OTA Team)

**Scope:** CVO integration problems, preflight protocol issues, cross-component coordination
**Response Time:** 1 business day for critical, 3 business days for standard
**Team:** OTA team lead engineers

**Common Issues:**
- Target CVO communication failures
- Result integration with conditionalUpdateRisks
- Preflight orchestration logic bugs
- API field validation problems

**Resolution Authority:**
- Modify preflight CVO orchestration logic
- Update ClusterVersion API integration
- Coordinate cross-component compatibility standards
- Escalate to OpenShift architecture team for design changes

#### Tier 4 - Strategic Issues (OpenShift Engineering Leadership)

**Scope:** Fundamental design flaws, cross-product compatibility, business impact decisions
**Response Time:** 2 business days for critical assessment
**Team:** OpenShift Engineering Managers, Principal Engineers

**Common Issues:**
- Preflight approach incompatible with major OpenShift changes
- Business timeline conflicts with technical implementation
- Cross-product integration blockers (HyperShift, managed services)
- Customer escalations requiring strategic decisions

**Resolution Authority:**
- Approve design changes requiring cross-team coordination
- Make business priority decisions affecting delivery timelines
- Authorize resource allocation for critical preflight issues
- Coordinate with product management for customer impact mitigation

#### Emergency Escalation Procedures

**Critical Production Impact (P1)**
- **Preflight blocking customer updates in production**
- **Immediate contact:** OTA team on-call → OpenShift Engineering Manager
- **Response time:** 1 hour for initial assessment
- **Authority:** Disable preflight functionality if necessary to unblock critical customer updates

**High Business Impact (P2)**
- **Preflight functionality completely broken for major release**
- **Immediate contact:** OTA team → Component teams → Engineering leadership
- **Response time:** 4 hours for comprehensive response plan
- **Authority:** Coordinate rapid fix deployment and customer communication

### Support Documentation and Knowledge Management

#### Customer-Facing Documentation

**Required documentation for Tech Preview:**
- **Getting started guide:** Basic preflight workflow examples
- **Troubleshooting runbook:** Common failure scenarios and solutions
- **CLI reference:** Complete command syntax and options
- **Best practices:** Recommended preflight usage patterns for different cluster types

#### Internal Support Documentation

**For Red Hat Support teams:**
- **Escalation flowcharts:** Decision trees for routing preflight issues
- **Log analysis guides:** How to interpret CVO and component logs for preflight issues
- **Known issues database:** Tracked problems and workarounds
- **Version compatibility matrix:** Supported preflight check combinations

## Infrastructure Needed [optional]

No additional infrastructure is required for this enhancement. The preflight functionality uses existing OpenShift components and infrastructure:

- **Existing Cluster-Version-Operator**: Extended to support preflight mode
- **Existing ClusterVersion API**: Extended with optional `mode` field
- **Existing Container Registry**: Target release images come from standard OpenShift release registry
- **Standard Kubernetes Deployment**: Preflight CVO runs as a standard Kubernetes deployment
- **Existing CI/CD Pipeline**: Testing will use existing OpenShift CI infrastructure

[ClusterOperator-Upgradeable]: https://docs.redhat.com/en/documentation/openshift_container_platform/4.20/html/updating_clusters/understanding-openshift-updates-1#understanding_clusteroperator_conditiontypes_understanding-openshift-updates
[ClusterVersion-desiredUpdate]: https://github.com/openshift/api/blob/6fb7fdae95fd20a36809d502cfc0e0459550d527/config/v1/types_cluster_version.go#L56-L81
[create-platform-alert]: https://docs.redhat.com/en/documentation/monitoring_stack_for_red_hat_openshift/4.20/html/managing_alerts/managing-alerts-as-an-administrator#creating-new-alerting-rules_managing-alerts-as-an-administrator
[recommend-critical-alert]: https://github.com/openshift/oc/blob/345800dc3c4164fbca313c1cbfb383f262547903/pkg/cli/admin/upgrade/recommend/alerts.go#L109-L124
[Update-API]: https://github.com/openshift/api/blob/6fb7fdae95fd20a36809d502cfc0e0459550d527/config/v1/types_cluster_version.go#L704-L763
