---
title: cluster-api-upgradeability-preflight
authors:
  - "@damdo"
  - "@mdbooth"
reviewers:
  - "@JoelSpeed"
  - "@wking"
  - "@sdodson"
approvers:
  - "@JoelSpeed"
api-approvers:
  - "N/A"
creation-date: 2026-10-14
last-updated: 2026-10-14
status: provisional
tracking-link:
  - https://redhat.atlassian.net/browse/OCPCLOUD-3330
see-also:
  - /enhancements/cluster-api/crd-compatibility-checker.md
  - /enhancements/cluster-api/installing-cluster-api-components-in-ocp.md
  - /enhancements/update/targeted-update-edge-blocking.md
  - /enhancements/update/accepted-risks.md
replaces: []
superseded-by: []
---

# Cluster API Upgradeability Preflight

## Summary

Throughout this document, **Release N** is the minor release that implements this enhancement, and **Release N+1** is the next minor release stream (not its next patch).

External components such as HyperShift, ACM/MCE, or customer tooling can install Cluster API CRDs on a cluster. Release N+1 will enable OpenShift-managed Cluster API components that may need the same CRDs. Because CRDs are cluster-scoped, OpenShift and external managers must share the same installed definitions, and OpenShift must know which CRDs it may manage.

An upgrade to N+1 can therefore fail, or break external components, because of CRD ownership or schema-compatibility conflicts that only become apparent once the upgrade is underway.

This proposal detects those conflicts before the upgrade, giving administrators advance notice and an opportunity to remediate.

## Motivation

The [compatibility-checker proposal](https://github.com/openshift/enhancements/blob/master/enhancements/cluster-api/crd-compatibility-checker.md) describes the shared-API model in which OpenShift
can leave Cluster API CRDs under external management, provided the installed CRDs satisfy
the API requirements of the OpenShift controllers that use them. An upgrade to N+1 can
raise those requirements: components newly enabled by the upgrade may need CRDs that exist
on the cluster but are managed externally, with incompatible schemas, or with unresolved
ownership.

These conflicts should be detected in N, before an upgrade is initiated, rather than during it. Detection must be automatic: administrators of healthy clusters should not have to run a preflight workflow, and the checks must be repeated automatically whenever relevant CRDs or configuration change.

### User Stories

- As a cluster administrator running external Cluster API components, I want advance notice of ownership and compatibility conflicts so that I can remediate them before upgrading OpenShift.
- As a cluster administrator, I want assessment and recovery to run automatically so that routine upgrades do not require a separate preflight workflow.
- As an OpenShift developer, I want requirements to come from the target release payload so that future Cluster API component bundles do not have to be manually backported into the current release.
- As an OpenShift developer, I want eligible clusters to enable machine management after upgrading so that existing external CRD consumers can coexist with the new payload components.
- As a fleet administrator or support engineer, I want status to identify the evaluated release, conflicting CRDs, and failing consumers so that I can monitor and remediate across clusters.

### Goals

- Identify CRD ownership and schema-compatibility conflicts between existing externally managed CRDs and the Cluster API components in a discovered next-minor release payload, before every minor-release upgrade.
- Let compatible clusters follow the normal upgrade workflow with no additional preflight command.
- Provide actionable conflict diagnostics and automatic recovery once the conflicts are remediated.
- Preserve administrator-declared external CRD management.
- Run the checks in release N, before an upgrade to N+1 enables or changes OpenShift-managed Cluster API components.

### Non-Goals

- Certifying every selectable payload, atomically preventing every unsafe upgrade, guaranteeing upgrade completion, or proving that all workloads remain safe throughout an update.
- Automatically resolving conflicts among external CRD managers, managing their workloads, or transferring those workloads to OpenShift.
- Introducing a new manual preflight CLI, depending on `ClusterUpdateAcceptedRisks`, or adding a Cincinnati `ClusterAPICRDs` risk.
- Skip-level assessment. The scope is N to N+1 only.
- Installing target controllers or executing target binaries in N.
- A generic exemption for existing CRDs carrying adoption annotations, or a new administrator opt-in adoption contract. The latter remains a scope question.
- Replacing the existing compatibility API/checker with a second or reduced checking implementation.

## Proposal

The proposal has two distinct parts: component changes that make the assessment infrastructure available in Release N, and the preflight controller that uses that infrastructure to assess N+1.

### Context

As described in [Installing Cluster API components](https://github.com/openshift/enhancements/blob/master/enhancements/cluster-api/installing-cluster-api-components-in-ocp.md), Cluster API components are installed by 2 controllers running in the `cluster-api-operator` namespace:
* The revision controller: Constructs a revision definition for the current release defining which Cluster API manifests must be applied.
* The installer controller: Continuously reconciles the current revision against cluster state.

### Component changes that enable preflight

1. **Deploy the CAPI operator infrastructure.** A `ClusterAPIOperator` feature gate,
   enabled in the Default feature set in N, makes CAPI operator infrastructure available
   independently of machine-management enablement. The CAPI installer and revision
   controllers must support a **no-components mode** that reconciles successfully without
   installing Cluster API operands. While the provider-specific
   `ClusterAPIMachineManagement<Provider>` feature gates (where `<Provider>` is the
   provider suffix) are disabled in N, the installer runs in this mode.
2. **Deploy the compatibility checker.** The existing `CompatibilityRequirement` API and checker must be deployed in N's supported default configuration, independently of machine-management enablement. This is a release prerequisite.

### Preflight controller

1. **Add the Upgradeable controller.** A new controller runs alongside the CAPI installer and revision controllers, in the existing CAPI installer binary and Deployment.
2. **Discover the target release.** The controller watches `availableUpdates` and `conditionalUpdates` from `ClusterVersion/version.status`, and selects the newest listed patch release in the next minor stream, assessing one target at a time. Checking the selected release does not certify other releases.
3. **Read target requirements and create `CompatibilityRequirement` objects.** Using current-release code, the controller extracts the target's Cluster API component requirements (this works with disconnected environments too, details below).
4. **Collect results and set `Upgradeable`.** The controller combines the compatibility
   checker's results with its own CRD ownership checks to compute the `cluster-api`
   ClusterOperator's `Upgradeable` condition. Any ownership conflict or failed compatibility
   check sets `Upgradeable=False`, which blocks forward minor/major upgrades via the existing
   CVO precondition.[^cvo-upgradeable-precondition] The upgradeable controller is the **sole writer of `Upgradeable`**;
   the general status controller retains `Available`, `Progressing`, and `Degraded`, which is
   why it needs to land in release N.

### Workflow Description

**Cluster administrators** request upgrades and declare which CRDs must remain externally
managed. **External CRD managers** maintain those definitions. The **upgradeable controller**
discovers the target, orchestrates extraction, assesses results, and writes `Upgradeable`.
The **compatibility checker** reconciles the created requirements. The **CAPI installer and
revision controllers** manage current-release operands or reconcile in no-components mode.
The **CVO and upgrade clients** (`oc adm upgrade`, console) are unchanged.

#### Normal assessment and upgrade

1. The controller discovers a next-minor upgrade target in `availableUpdates` and records the release being assessed.
2. It starts a release extraction pod that mounts the target release payload image and copies the provider-image references from its `capi-installer-images` manifest into a ConfigMap (`upgradeable-images-<release-version>`, in `openshift-cluster-api-operator`).
3. It then starts a provider extraction pod that mounts the referenced provider images, selects the applicable content, and creates `CompatibilityRequirement` objects describing the CRD schemas, API versions, and field exclusions required by each target component.
4. The controller waits for extraction completion and for the asynchronous checker results. Pod success alone is not assessment success.
5. For each existing CRD that the target would manage but which is not already managed by the CAPI installer, the controller verifies the ownership and compatibility criteria below.
6. With no relevant conflicts, the controller sets the `Upgradeable=True` condition on the
   `cluster-api` ClusterOperator for the recorded target. Otherwise it sets
   `Upgradeable=False` on that ClusterOperator, identifying the CRD, the target, and the
   failing requirement.
7. The administrator follows the normal upgrade workflow. In N+1, the CAPI installer begins managing the applicable operands, leaving declared unmanaged CRDs externally managed.

#### Compatibility criteria

The upgradeable controller will verify, for each CRD it processes, the following compatibility criteria:
- A CRD that exists in release N, should be owned by the CAPI Installer or listed in `ClusterAPI/cluster.spec.unmanagedCustomResourceDefinitions`.
- A CRD that exists in release N, should be compatible with release N+1's new CRD definition even if listed in `ClusterAPI/cluster.spec.unmanagedCustomResourceDefinitions`. What "compatibility" means is defined in the [`crd-compatibility-checker`](https://github.com/openshift/enhancements/blob/master/enhancements/cluster-api/crd-compatibility-checker.md) enhancement 

A conflict can arise if any of the above criteria is not fulfilled.

#### Conflict remediation
To resolve such conflicts a cluster administrator must ensure that any CRD that would be installed by Release N+1 is either already owned by the CAPI Installer or is unmanaged (listed in the `ClusterAPI/cluster.spec.unmanagedCustomResourceDefinitions`) and already compatible with the Release N+1.

The message associated with the `Upgradeable=False` condition will detail all the conflicts and suggests remediations that the admin will need to perform.

#### Unavailable or incomplete assessment

Extraction failure, checker absence, pending reconciliation, missing coverage, and stale
observations do not establish compatibility; the failure policy for these states is an open
question. At startup the controller sets `Upgradeable=Unknown`, which does not block
upgrades. If no target release is available, `Upgradeable=True` means only "nothing to
check", not "verified compatible". Conversely, if a conflicting target later disappears
from `availableUpdates`, that disappearance does not resolve the conflict.

`Upgradeable=False` is a global guard for forward minor/major upgrades, not a per-target certification or a same-stream patch gate.

### API Extensions

No new configuration is selected. The proposal reuses:

- `operator.openshift.io` `ClusterAPI/cluster`, including `spec.unmanagedCustomResourceDefinitions`.
- `apiextensions.openshift.io` `CompatibilityRequirement`.
- `config.openshift.io` ClusterOperator and ClusterVersion.

### Topology Considerations

#### Hypershift / Hosted Control Planes

This feature is not applicable to Hosted Control Planes, as they do not run CAPI.

#### Standalone Clusters

This feature is applicable to this configuration.

#### Single-node Deployments or MicroShift

CAPI Operator does not run on these configurations, so this does not apply there.

#### OpenShift Kubernetes Engine

As with standalone clusters, the CAPI Operator runs on OKE, so this feature applies to this configuration.

### Implementation Details/Notes/Constraints

### CAPI IPAM CRDs adoption considerations

The two existing payload-provided IPAM CRDs, `ipaddressclaims.ipam.cluster.x-k8s.io` and
`ipaddresses.ipam.cluster.x-k8s.io`, have a one-off upgrade-preflight exemption for their
transition into operator installation.[^payload-ipam-crds]
This exemption extends to no other CRD; the existing `always`/`never` adoption policy is
not a generic preflight bypass or consent mechanism.[^capi-adoption-policy]

### Condition ownership and behavior

The checker's existing `CompatibilityRequirement` conditions (`Progressing`, `Admitted`,
`Compatible`) are reused unchanged. The upgradeable controller aggregates them with its
ownership checks into the ClusterOperator's `Upgradeable` condition. A successful aggregate
assessment requires completed extraction, the complete applicable requirement set, and
current observations for every requirement; absence of a failing result is not sufficient.

| Situation | Intended `Upgradeable` |
| --- | --- |
| Initialization | `Unknown` |
| No discoverable next-minor target | `True` with explanatory message |
| Complete, current assessment, no relevant conflict | `True` |
| Existing external target CRD lacks the unmanaged declaration | `False` |
| Existing external CRD fails an applicable target requirement | `False` |
| Pending, failed, unavailable, stale, or incomplete assessment | TBD (failure policy open) |


### Risks and Mitigations

- **Untrusted payload data or overly privileged extraction:**
  We may read any image published in `availableUpdates` or `conditionalUpdates` on the `ClusterVersion` object.
  These images came from the cluster's configured cincinnati endpoint, but have not yet been validated by CVO and we do not perform any additional checking on them.
  We perform the following operations based on this image:
  * Pull and mount the release image
  * Parse the release metadata json
  * Parse a manifest from the release
  * Pull and mount further payload images referenced from the release
  * Parse manifest yamls from payload images
  * Inject manifest yamls into a CompatibilityRequirement object

  In mitigation:
  * We do not execute code from the pulled release payload, or directly create any cluster resource from it
  * A false result from the preflight check is not fatal even if it causes to subsequently upgrade to an incompatible release; the future release still handles this safely.

  The most significant risk is likely that a malicious release payload published by cincinnati could exhaust disk space on a control plane node when it is pulled.

- **Snapshot weakness:** An external CRD manager can change a definition after preflight
  passes and the upgrade is accepted, but before target controllers start. Future
  requirements warn rather than enforce N+1's rules, so a change compatible with N can
  invalidate the earlier N+1 assessment. No mitigation is selected; whether the
  installation proposal's gating behavior covers this window must be verified before
  being relied upon ([R1](https://github.com/openshift/enhancements/blob/master/enhancements/cluster-api/crd-compatibility-checker.md),
  [R5](https://github.com/openshift/enhancements/blob/master/enhancements/cluster-api/installing-cluster-api-components-in-ocp.md)).
  If the CAPI Preflights fail to catch this, the Upgrade
  checks will be able to catch this. Less desirable but still safe.

### Drawbacks

This introduces a continuously running controller, transient extraction pods, a cross-release data contract, and mandatory checker infrastructure before machine management is enabled.


## Alternatives (Not Implemented)

- **Cincinnati risk plus accepted-risk automation.** Could represent target-specific concerns, but depends on accepted-risk availability in N and on a graph-data schema that supports the required release targeting.[^cincinnati-graph-data][^accepted-risks]
- **Manual preflight command.** Would reduce continuous orchestration but requires administrators to discover, invoke, and repeat it; does not meet the automation goals.
- **Backport future transport bundles into N.** The earlier model in the compatibility proposal; avoids the cross-release data contract but requires recurring synchronization and backport work.
- **Target-release checks at CVO acceptance.** Potential future integration[^target-release-preflight] does not currently provide an available contract; this enhancement does not depend on it.
- **Direct fetching of release payloads (no pods)** This requires implementing, and maintaining for the lifetime of the software, all mechanisms used by OpenShift to pull payload images in all supported configurations.
  Today this includes pull secret handling, mirroring, and signature verification, and this may be extended in the future.
  By using pods with ImageVolumes we delegate this work to CRI-O, which is already correctly configured.
- **No available update sets something other than Upgradeable=True** Setting Upgradeable=False when there is no available update is a safer option because it closes a window between a target release being made available and the upgradeability status being updated. However, this window is extremely short (typically ~1 minute), and it causes a warning to be displayed to the User in the console which is not actionable. The states we rejected are:
  - Upgradeable=False: Displays an incorrect and unactionable warning in the console.
  - Upgradeable=Unknown: Causes installation failure due to unexpected ClusterOperator status when there is no available update.
  - Upgradeable unset: Violates a general API principal for Conditions that if a Condition is expected to be set it will always be present.
- **Per platform filtering of manifests included in the preflight**
This is a desirable feature in theory.
In practice it requires parsing profile metadata from a future release.
We do not yet have enough experience of this interface to confidently assert a forward compatibility guarantee for it.
In addition, while the feature sounds intuitively useful, for the CAPI CRDs at least it is improbable that an affected cluster would ever be deployed in practice, although it is more likely that some non-CAPI resources such as Azure's ASO or OpenStack's ORC might generate undesirable conflicts.
We can address this in the future if it ever becomes a real issue, but in the meantime the workaround is for the administrator to either add entries to unmanagedCRDs, or override the warning after evaluating the reported conflict.
This will be easier to implement robustly in the future when we have CVO preflights and can also execute code from the future release.

## Open Questions [optional]

1. **Profile selection and coverage.** How does extraction select the provider profiles applicable to the target installation, ensuring every applicable consumer's requirements are covered without imposing unrelated ones?
    - we are not going to do selection, so you must be compatible with everything in the target release payload, whether it would be installed or not
1. **Failure policy.** Do pending, failed, stale, or incomplete assessments block with distinct `False` reasons, use a bounded grace period, or report nonblocking uncertainty? What constitutes freshness evidence?
    - ?
1. **Discovery race.** Is the window before a discovered conflict reaches CVO acceptable, and what exact guarantee is supported?
    - We can consider this acceptable

## Test Plan

This will be tested by a new set of tests which run by default in the serial suite, ensuring wide coverage. The test will create a fake cincinnati server exposing a single Y+1 update. The update will initially expose the cluster's current release because this is guaranteed to be available in current jobs without infrastructure changes, but in future we hope to use real Y+1 release in stable branches. It will wait for Upgradeable=True because a fresh install must always be compatible with release:next before doing one of the following:

  - If Cluster CRD is not currently present (upgrade to initial release containing CAPI)
    - Create it from the target payload with an incompatible schema
    - Wait for Upgradeable=False
    - Declare it unmanaged and wait for a new revision, still incompatible
    - Restore the CRD and
    - Wait for Upgradeable=True

  - If Cluster CRD is current present (upgrade from CAPI release to subsequent CAPI release):
    - Make the current CRD incompatible and (through shenanigans) no longer managed by capi-installer
    - Wait for Upgradeable=False
    - Declare it unmanaged and wait for a new revision, still incompatible
    - Restore the CRD to the target payload version
    - Wait for Upgradeable=True

In both cases we are able to restore the Cluster CRD and the ClusterAPI to their original states. Restoring ClusterAPI requires further shenanigans, see the code for detail.

## Graduation Criteria

### Dev Preview -> Tech Preview

- End-to-end preflight assessment and recovery are demonstrated against an N+1 payload on the agreed initial configurations.
- Conflict diagnostics and administrator remediation are validated, with feedback incorporated from cluster administrators and component owners.

### Tech Preview

- End-to-end extraction, assessment, conflict diagnostics, and automatic recovery after administrators resolve external conflicts work on agreed initial configurations.
- N's component prerequisites and no-components behavior are validated independently of machine-management enablement.

### Tech Preview -> GA

- Required APIs and dependencies have an approved support/maturity plan across the agreed support matrix.
- End-to-end coverage of N-to-N+1 upgrades.
- Disconnected access validated.
- Documentation published in `openshift-docs`, covering upgrade scope, conflict remediation, unmanaged-list irreversibility, diagnostics, and overrides.

### Removing a deprecated feature

Not applicable.

## Upgrade / Downgrade Strategy

N must deploy the infrastructure and checker prerequisites described in the component-changes section above before N+1 can enable machine management. A compatible cluster requires no extra action; a cluster with external target CRDs must supply the unmanaged declarations and remediation first. Direct installation of N+1 cannot retroactively benefit from an N preflight.

Only adjacent-minor assessment is initially supported; same-stream patches are unaffected.

## Version Skew Strategy

Only N->N+1 is supported, with N+1 being the latest available next-minor.

## Operational Aspects of API Extensions

N/A

## Support Procedures

1. Inspect `cluster-api` ClusterOperator conditions and the `ClusterVersion` available updates to identify the concern and candidate target.
2. Inspect `spec.unmanagedCustomResourceDefinitions`, live CRD identity, and each controller-owned requirement's conditions and observations.
3. For incomplete extraction, inspect the extraction pods, ConfigMap, controller logs, image pulls, and registry access.
4. For a compatibility failure, coordinate with the external manager. For an ownership conflict, confirm management intent before making the durable unmanaged declaration. Do not delete or replace shared CRDs merely to remove a warning.
5. Verify all applicable results pass and the condition recovers automatically.

No supported disable or bypass procedure is selected for the mandatory dependency. `--force` remains a hazardous existing override, not remediation.

## Infrastructure Needed [optional]

No new repository or binary. CI needs a controlled update service and payloads, cross-release assessment jobs, and appropriate CI/nightly trust stores.

## References

- [CRD Compatibility Checker enhancement](https://github.com/openshift/enhancements/blob/master/enhancements/cluster-api/crd-compatibility-checker.md)
- [Installing Cluster API components enhancement](https://github.com/openshift/enhancements/blob/master/enhancements/cluster-api/installing-cluster-api-components-in-ocp.md)

[^cvo-upgradeable-precondition]: [CVO upgradeability precondition](https://github.com/openshift/cluster-version-operator/blob/main/pkg/payload/precondition/clusterversion/upgradeable.go)
[^payload-ipam-crds]: [Payload IPAM CRDs](https://github.com/openshift/cluster-api/blob/main/openshift/manifests/0000_30_cluster-api_04_crd.core-cluster-api.yaml)
[^capi-adoption-policy]: [Local CAPI installer adoption policy](https://github.com/openshift/cluster-capi-operator/blob/main/pkg/revisiongenerator/validate.go)
[^cincinnati-graph-data]: [Cincinnati graph-data, including Block Edges](https://github.com/openshift/cincinnati-graph-data/blob/master/README.md)
[^accepted-risks]: [Accepted Risks enhancement](/enhancements/update/accepted-risks.md)
[^target-release-preflight]: [Target-release preflight proposal](https://github.com/openshift/enhancements/pull/1930)
