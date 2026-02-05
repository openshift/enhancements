---
title: preflight-from-target-release
authors:
  - "@wking"
  - "@fao89"
reviewers: # Include a comment about what domain expertise a reviewer is expected to bring and what area of the enhancement you expect them to focus on. For example: - "@networkguru, for networking aspects, please look at IP bootstrapping aspect"
  - "@hongkailiu, for accepted risks integration and conditional update aspects"
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
* **Flexible execution model**: Support both one-time preflight validation and continuous preflight monitoring for target releases.

Success criteria:
- Administrators can run `oc adm upgrade --preflight --to=<version>` to check compatibility.
- Preflight results appear in ClusterVersion `status` alongside other conditional update risks.
- Component maintainers can write compatibility checks into the target release, without backporting logic to earlier releases.

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

### Topology Considerations

#### Hypershift / Hosted Control Planes

HyperShift is out of scope for now, as we rush to get something tech-preview for standalone.
We'll come back later and figure out how this could fit into the HostedCluster APi.

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

Cluster adminstrators can request preflight checks via [the new `mode` property](#clusterversion-spec-desiredupdate-mode).
The `mode` property will also be wrapped in the existing `oc adm upgrade` command, so cluster administrators can use `oc adm upgrade --mode=preflight ...` to request preflight updates.

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
The results will be JSON.
Because they will be [propagated into `conditionalUpdateRisks`](#retrieving-preflight-check-results), we'll use that structure:

```json
{
  "format": "FIXME: media type for preflight v1 JSON",
  "preflightID": ...
  "risks": [
    {
      "name": "ConcerningThingA",
      "message": "FIXME: Upgrade can get stuck on clusters that use multiple networks together with dual stack.
      "url": "FIXME: https://issues.redhat.com/browse/SDN-3996
      "matchingRules": FIXME probably don't set this, because components may not want to write PromQL to help the current cluster check whatever the future-CVO found
      "conditions": FIXME probably don't set this.  If we return a risk, it's because we think it applies to this cluster.
      "cacheTime": TIMESTAMP?  Do components get to tell us how long the results are valid for?
    },
    ...more concerning things, if found...
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

```yaml
  conditionalUpdateRisks:  # include every risk in the conditional updates (moved up and renamed)
  - name: ConcerningThingA
    message: Upgrade can get stuck on clusters that use multiple networks together with dual stack.
    url: https://issues.redhat.com/browse/SDN-3996
    matchingRules:
    - type: Always
    conditions:
    - status: True  # Apply=True if the risk is applied to the current cluster
      type: Applies
      reason: MatchingRule
      message: The matchingRules[0] matches
      lastTransitionTime: 2021-09-13T17:03:05Z
```

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
- Integration with existing `conditionalUpdateRisks` API established
- `oc adm upgrade` command integration stable for Tech Preview usage

**Documentation and Testing:**
- End-user documentation for preflight workflow created in [openshift-docs](https://github.com/openshift/openshift-docs/)
- Integration tests covering end-to-end preflight execution
- Unit tests for CVO preflight mode processing and API validation
- Performance and resource usage testing on representative cluster configurations

**Component Integration:**
- At least one OpenShift operator (e.g., networking, machine-config) implements preflight compatibility checks to demonstrate the framework
- Integration with the accepted-risks workflow tested and validated
- Basic error handling and failure recovery mechanisms implemented

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

1. **Micro version upgrades**: Preflight functionality operates at the minor version boundary (4.21 to 4.22), so micro version updates (4.22.1 to 4.22.3) do not affect preflight compatibility.

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
- **Namespace isolation**: Preflight operations contained within existing CVO namespace boundaries with additional isolation for operator checks

**Attack surface analysis:**
- **Minimal API surface expansion**: Single optional field with enum validation
- **No network exposure**: Preflight operations entirely internal to cluster
- **Image security**: Target release images undergo same security scanning as update images

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

1. **Clear preflight request:**
   ```bash
   oc patch clusterversion version --type='merge' -p '{"spec":{"desiredUpdate":{"mode":null}}}'
   ```

1. **Force cleanup of preflight resources:**
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
