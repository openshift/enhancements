---
title: crd-compatibility-checker
authors:
  - "@mdbooth"
reviewers:
  - "@JoelSpeed"
  - "@damdo"
  - "@csrwng"
approvers:
  - "@JoelSpeed"
api-approvers:
  - "@JoelSpeed"
creation-date: 2025-09-10
last-updated: 2025-09-10
tracking-link:
  - "https://issues.redhat.com/browse/OCPCLOUD-3005"
  - "https://issues.redhat.com/browse/OCPCLOUD-3006"
see-also:
  # Link to parent safety enhancement
  - "/enhancements/this-other-neat-thing.md"
---

# CRD Compatibility Checker

This enhancement proposes a tool to implement lifecycle management of Cluster API CRDs within OpenShift, enabling multiple actors to specify compatibility requirements for CRDs while ensuring stability of OpenShift's own usage.

## Summary

The CRD Compatibility Checker provides a mechanism for OpenShift to allow non-payload workloads to manage CRDs that are also used by OpenShift payload components, while ensuring api compatibility between different versions.
This addresses scenarios where OpenShift payload components (like cluster-capi-operator) and non-payload workloads (like Hosted Control Planes or Advanced Cluster Management) need to use the same CRDs but may require different versions.

The tool provides admission webhooks to enforce compatibility policies.
Non-payload components can have a certain degree of confidence that their updates don't break payload components if the CRD change is admitted.
Ideally this would be picked up during a testing phase rather than, for example, during an upgrade.

## Motivation

CRDs required by the OpenShift payload are typically managed by CVO, or by operators which are managed by CVO. Changes to them are not normally permitted as this could undermine OpenShift stability.
However, we are encountering cases where we want to use APIs in the OpenShift payload that other workloads may also want to use.
We cannot assume that the versions required by the workload and OpenShift itself are the same.

In general, later versions of well-behaved kube APIs are compatible with earlier versions.
An earlier version of a controller *should* operate correctly with a later version of the CRD, as long as the CRD remains compatible.
So in practice this will usually work, but some care needs to be taken.

### User Stories

* As a cluster administrator, I want to install Hosted Control Planes (HCP) alongside OpenShift's cluster-capi-operator and use both systems without version conflicts.

* As a cluster administrator, I want to install additional Gateway API providers beyond those in the OpenShift payload so that I can use the specific providers that meet my networking requirements.

* As an OpenShift operator developer, I want to ensure that external CRD managers cannot install incompatible versions of CRDs that my operator depends on so that my operator continues to function correctly.

* As an external workload developer, I want to check if my planned CRD upgrade will be compatible with OpenShift's requirements so that I can provide clear feedback to administrators about upgrade feasibility.

* As a cluster administrator, I want OpenShift to ensure that objects created for OpenShift do not change in behaviour on upgrade.

* As a platform operator, I want to coordinate multiple compatibility requirements from different actors so that all stakeholders can safely use the same CRDs.

### Goals

- Enable OpenShift to safely allow external workloads to manage CRDs that are also used by OpenShift payload components
- Provide a mechanism for multiple actors to specify compatibility requirements for the same CRD
- Prevent installation of incompatible CRD versions through admission webhooks
- Allow controllers to programmatically check compatibility before attempting upgrades
- Enable object schema validation in specific namespaces to maintain upgradeability
- Minimize operational changes required for existing external workloads

### Non-Goals

- Defining the specific algorithm for determining CRD compatibility (assumed to be described elsewhere)
- Enforcing single ownership of CRDs (must be ensured out-of-band)
- Providing a user interface for managing compatibility requirements
- Resolving compatibility conflicts between different actors

## Proposal

The proposal introduces a new admission webhook for CRDs.
We introduce a new cluster-scoped CRD called `CRDCompatibilityRequirement` that allows actors to express compatibility requirements for target CRDs.
The admission webhook ensures that subsequent changes to referenced CRDs are compatible with all requirements specified in a `CRDCompatibilityRequirement`.

### Workflow description

The following are 2 related example uses of the proposed API and controller.

#### CCAPIO ensures cluster stability while HCP manages CAPI CRDs

In this example, Hosted Control Planes (HCP) takes responsibility for the management of the cluster-api (CAPI) `Cluster` CRD from cluster-capi-operator (CCAPIO).

CCAPIO deploys CAPI version X and uses it to manage cluster resources.

HCP deploys CAPI version Y, where Y>X. HCP uses it to deploy external managed clusters.

As `Cluster` can only have a single CRD in the management cluster, they must both use the same CRD.

* HCP will assert ownership of the `Cluster` CRD
* CCAPIO will assert its requirements for the `Cluster` CRD, but will no longer manage the CRD.
* HCP will upgrade the `Cluster` CRD

CRD Compatibility Checker ensures that the version HCP upgrades to does not break CCAPIO, and therefore the management cluster.

During normal operation, CCAPIO applies sets of resources to the cluster which are defined in 'transport' config maps.
CCAPIO will be enhanced such that it will automatically create a CRDCompatibilityRequirement for any CRD it loads.
In the case of `Cluster` this would be:

```yaml
apiVersion: operator.openshift.io/v1alpha1
kind: CRDCompatibilityRequirement
metadata:
  name: cluster-api-cluster-ccapio
spec:
  crdRef: clusters.cluster.x-k8s.io
  creatorDescription: "OpenShift Cluster CAPI Operator"
  compatibilityCRD: |
    ...
    <complete YAML document of Cluster CRD from transport config map>
    ...
  crdAdmitAction: Enforce
  objectSchemaValidation:
    namespaceSelector:
      matchLabels:
        "kubernetes.io/metadata.name": "openshift-cluster-api"
    action: Enforce
```

CCAPIO marks itself Degraded if any CRDCompatibilityRequirement it creates during this process becomes not Progressing, and:
* not Admitted - transport config map is invalid
* not Compatible - current CRD is incompatible with CCAPIO requirements

HCP uses an as-yet-undefined mechanism to inform CCAPIO of the set of CRDs which are no longer managed by CCAPIO.
For CRDs in this list, CCAPIO creates a CRDCompatibilityRequirement but does not load or update the CRD.
This is the only operational change required by HCP.

HCP proceeds normally and upgrades the `Cluster` CRD.
Assuming that the CRD from version Y is compatible with the one for version X required by CCAPIO, the CRD is admitted and everything works as normal.

#### CCAPIO prevents cluster upgrade if it would conflict with an existing CRD

This example covers the scenario when CCAPIO is first enabled in OCP, but more generally if CCAPIO starts using a CRD which HCP is currently using.
We want to prevent upgrade if a CRD currently in use would be incompatible with the version required by CCAPIO after upgrade.

OCP 4.Y is the current version.
OCP 4.Y+1 is the upgraded version.

Some time during the OCP 4.Y lifecycle we add an additional set of transport config maps to 4.Y.z which contain the CRDs required by 4.Y+1.
CCAPIO scans these, but does not load them.
For each CRD it creates a new CRDCompatibilityRequirement.
e.g. for the `Cluster` CRD it would create:

```yaml
apiVersion: operator.openshift.io/v1alpha1
kind: CRDCompatibilityRequirement
metadata:
  name: cluster-api-cluster-ccapio-4.Y+1
spec:
  crdRef: clusters.cluster.x-k8s.io
  creatorDescription: "OpenShift Cluster CAPI Operator for OCP 4.Y+1"
  compatibilityCRD: |
    ...
    <complete YAML document of future Cluster CRD from transport config map>
    ...
  crdAdmitAction: Warn
```

The differences from the previous example are:
* The requirement identifies itself as being related to a different version.
* `crdAdmitAction` is `Warn` instead of `Enforce`, so this requirement will not prevent updates to the current CRD.
* There is no `objectSchemaValidation`.

CCAPIO will mark itself as not upgradeable if any CRDCompatibilityRequirement created for an upgrade transport config map reports its `Compatible` condition as `False`.

### API Extensions

The following is an outline of the CRDCompatibilityRequirement CRD:

```yaml
apiVersion: operator.openshift.io/v1alpha1
kind: CRDCompatibilityRequirement
metadata:
  ...
spec:
  # Fields to be implemented initially
  crdRef: <the name of the target CRD>
  creatorDescription: |
    <human readable description of who created this requirement. displayed in errors and warnings the validation webhooks>
  compatibilityCRD: |
    <the complete CRD required by the creator of this requirement, as yaml>
  crdAdmitAction: <Enforce or Warn>

  # If defined, also validate that matching objects conform to the schema given in compatibilityCRD.
  # This will be implemented in a second phase. The initial API will not contain this field.
  objectSchemaValidation:
    # namespaceSelector, objectSelector, and matchConditions will be copied to
    # the corresponding ValidatingWebhookConfiguration. Their definitions and
    # semantics are therefore identical to those in
    # ValidatingWebhookConfiguration.
    namespaceSelector:
      matchLabels:
        "kubernetes.io/metadata.name": "openshift-cluster-api"
    objectSelector:
      ...
    matchConditions:
      ...
    action: <Enforce or Warn>
status:
  conditions:
    # Conditions are detailed further below
    ...

  # observedCRD is set only if the target CRD was observed.
  observedCRD:
    uid: <uid of the observed target CRD>
    generation: <generation of the observed target CRD>

```

#### Conditions

CRDCompatibilityRequirement defines 3 conditions:

* `Progressing` -
False if the spec has been completely reconciled.
True indicates that reconciliation is still in progress and the current status does not represent a stable state.
Progressing False with an error reason indicates that the object cannot be reconciled.

* `Admitted` -
True if the requirement has been configured in the validating webhook, otherwise False.

* `Compatible` -
True if the observed CRD is compatible with the requirement, otherwise False.
Note that Compatible may be False when adding a new requirement which the existing CRD does not meet.

The above conditions are always set on every reconcile, and include `observedGeneration`.

These conditions may have the following Reasons:

* `Progressing`
  * `ConfigurationError` -
  This indicates that reconciliation cannot progress due to an invalid spec.
  It is associated with a status of `False`.
  * `TransientError` -
  This indicates that reconciliation was prevented from completing due to an error that can be retried.
  It is associated with a status of `True`.
  * `UpToDate` -
  This indicates the reconciliation completed successfully.
  It is associated with a status of `False`.
* `Compatible`
  * `RequirementsNotMet` -
  This indicates that a CRD exists, and it is not compatible with this requirement.
  It is associated with a status of `False`.
  * `CRDDoesNotExist` -
  This indicates that the referenced CRD does not exist.
  It is associated with a status of `False`.
  * `CompatibleWithWarnings` -
  This indicates that the CRD exists and is compatible with this requirement, but `Message` contains one or more warning messages.
  It is associated with a status of `True`.
  * `Compatible` -
  This indicates that the CRD exists and is compatible with this requirement.
  It is associated with a status of `True`.

#### Validating webhooks

##### ClusterResourceDefinition

Create, Update, and Delete operations on CRDs will either be denied or emit a warning depending on the setting of `crdAdmitAction`.

Create and Update will deny/warn if the created or updated CRD would not be compatible with any CRDCompatibilityRequirement which references it in `crdRef`.

Delete will deny/warn if any CRDCompatibilityRequirement references it in `crdRef`.

##### Dynamically created by `objectSchemaValidation`

[!NOTE]
Implementation of `objectSchemaValidation` will be deferred to a second phase to be implemented after the CRD validation webhook is fully implemented and integrated.

If a CRDCompatibilityRequirement defines `objectSchemaValidation` we will dynamically create a new `ValidatingWebhookConfiguration` to implement it.
This webhook will ensure that matching objects conform to the CRD given in `compatibilityCRD`, which may be different to the current CRD version.
It will either warn or deny admission based on the setting of `objectSchemaValidation.action`.

This webhook will have the same lifecycle as the CRDCompatibilityRequirement which created it.

The webhook will only be called for objects matching the selectors in `objectSchemaValidation`.

### Topology Considerations

#### Hypershift / Hosted Control Planes

The HCP management cluster is one of the primary intended users of this feature.

The feature should work correctly in a managed cluster, although we don't expect CAPI CRDs to be managed there so it wouldn't be an initial target.

#### Standalone Clusters

This enhancement is relevant for standalone clusters as it addresses scenarios where external workloads manage CRDs used by OpenShift payload components.

#### Single-node Deployments or MicroShift

As for managed clusters, the feature should work correctly on SNO but we would not initially target those deployments.

### Implementation Details/Notes/Constraints

N/A

### Risks and Mitigations

- **Risk**:
  Webhook failures could block CRD or object operations

  **Mitigation**:
  Run the typical deployment configuration with 2 members which coordinate via leader election.
  Ensure that the webhooks can safely run HA in both members.

- **Risk**:
  Performance impact from additional admission webhook calls
  
  **Mitigation**:
  CRD updates are infrequent enough that this is unlikely to be a concern.
  For object schema validation there is potential for impact during certain phases of cluster activity.
  We will optimize performance beyond the initial implementation only if it proves necessary.
  
- **Risk**:
  Complex coordination between multiple actors with conflicting requirements
  
  **Mitigation**:
  This tool is itself a mitigation of this risk.
  It provides a clear indication that this coordination has broken down before it impacts cluster stability.
  The tool will provide clear error messages identifying which actor's requirements are preventing operations.
  
- **Risk**:
  Object schema validation could break legitimate use cases.
  
  **Mitigation**:
  This would be a bug in the tool.
  We expect the initial and primary non-payload customer to be HCP.
  We will coordinate with HCP to ensure that potential issues show up early in their CI pipeline.

### Drawbacks

- Adds complexity to the OpenShift API surface with new CRDs and webhooks
- Requires external workloads to understand and work with the compatibility system
- May block legitimate CRD upgrades if compatibility requirements are too restrictive
- Introduces additional failure modes through webhook dependencies

The benefits of enabling safe multi-actor CRD management outweigh these drawbacks, and the system is designed to fail gracefully with clear error messages.

## Alternatives (Not Implemented)

- **Version pinning**:

  Pin CRD versions in OpenShift payload.
  External workloads are required to use the same CRDs as the OpenShift payload.

  This was considered operationally infeasible due to the different lifecycles of OCP and external workloads. It would be difficult to achieve even with HCP, where we have the opportunity to coordinate closely if required.

- **CRD isolation**:

  For example isolating different CRDs for different workloads in different namespaces, or using a separate control plane for external workloads.

  Unfortunately kubernetes does not support namespaced CRDs. Using a separate control plane, for example [KCP](https://kcp.io/), would be ideal.
  However, it would require further architectural work to deploy and maintain with OCP.
  We should revisit this should it ever become feasible in the future.

- **Manual coordination**:
  Rely on out-of-band coordination between actors.
  
  This was considered unlikely to be a robust process over time.

## Open Questions [optional]

1. What is the specific algorithm for determining CRD compatibility?
2. How should the system handle CRD conversion webhooks during compatibility checks?
3. What is the performance impact of the admission webhooks on high-traffic clusters?

## Test Plan

**Note:** *Section not required until targeted at a release.*

The test plan should include:
- Unit tests for compatibility checking logic
- Integration tests for webhook admission scenarios
- E2E tests for complete workflows including upgrade scenarios
- Performance tests to measure webhook latency impact
- Tests for various failure modes and recovery scenarios

## Graduation Criteria

**Note:** *Section not required until targeted at a release.*

### Dev Preview -> Tech Preview

- Webhook admission working for CRD updates
- Feature integrated in cluster-capi-operator
- Initial documentation and API stability
- Comprehensive test coverage for all deployed features

### Tech Preview -> GA

- Object schema validation working end-to-end
- Comprehensive test coverage including upgrade/downgrade scenarios
- Performance validation showing acceptable webhook latency
- Production deployment validation
- Complete user documentation

## Upgrade / Downgrade Strategy

CRDCompatibilityChecker has no direct impact on cluster operations unless it is configured.
We handle upgrade and downgrade by altering the behaviour of cluster-capi-operator, which is responsible for the in-payload configuration.

The following requires 2 feature gates:
* CRDCompatibilityCheckerUpgradeCheck
* CRDCompatibilityCheckerEnforce

`CRDCompatibilityCheckerUpgradeCheck` enables the 'future version' check.
If `CRDCompatibilityCheckerUpgradeCheck` is enabled, cluster-capi-operator will:

* Create a `CRDCompatibilityRequirement` for every CRD discovered in the 'future' transport config map if that CRD is also marked 'unmanaged'.
This requirement will have `crdAdmitAction=Warn`.
* Mark itself not upgradeable if any of these are marked `Compatible=False`.

If `CRDCompatibilityCheckerUpgradeCheck` is not enabled, cluster-capi-operator will look for all 'future' `CRDCompatibilityRequirement`s and delete them.

If `CRDCompatibilityCheckerEnforce` is enabled, cluster-capi-operator will:
* Create a `CRDCompatibilityRequirement` for every CRD in the 'active' transport config map if that CRD is also marked 'unmanaged'.
This requirement will have `crdAdmitAction=Enforce`.

If `CRDCompatibilityCheckerEnforce` is not enabled, cluster-capi-operator will look for all 'active' `CRDCompatibilityRequirement`s and delete them.

Upgrading to first version of OCP to include this feature:
- `CRDCompatibilityCheckerUpgrade` was enabled in 4.N-1.
- `CRDCompatibilityCheckerEnforce` is enabled in 4.N, but not 4.N-1.
- Users of unmanaged APIs must mark them 'unmanaged' in 4.N-1.
- OCP will not permit upgrade if unmanaged CRDs are incompatible with 4.N.
- On upgrade, OCP will enforce compatibility.

Downgrading from first version of OCP to include this feature:
- `CRDCompatibilityRequirement`s created by 4.N will automatically be deleted by 4.N-1 due to the missing feature gate.

During subsequent upgrades and downgrades:
- cluster-capi-operator ensures that its `active` and `future` reflect the respective transport config maps from the current OCP version.

## Version Skew Strategy

The system must handle version skew during upgrades:
- Webhook and controllers must be compatible with both the current and previous API versions

## Operational Aspects of API Extensions

- Fails all Create, Update, and Delete operations on CRDs if the webhook is not running.
- Fails all Create, Update, and Delete operations on CAPI objects in the `openshift-cluster-api` namespace if the webhook is not running.
- Runs an informer cache of CRD objects, so potentially large memory usage.
- Expected use cases involve a static number of `CRDCompatibilityRequirement` objects in the order of 10s.
- Failure of the component will prevent:
  - Changes to CRD objects (unlikely to impact workloads).
  - Changes to CAPI objects, preventing cluster scale-up, scale-down, and reconfiguration.

## Support Procedures

Any failure of the tool will manifest as an admission failure.
If the tool is not running, the error message will indicate which webhook service had no members, which should identify the root cause.
If the tool is running but behaving incorrectly, all errors and warnings clearly indicate CRD Compatibility Checker as the source.

### Create/Update/Delete of CRDs not possible

If something was preventing the webhooks from running correctly, an operator would have to delete the `crd-compatibility-requirements` `ValidatingWebhookConfiguration`, and prevent the Cluster Version Operator from recreating it.

### Create/Update of OCP CAPI objects not possible

If something was preventing the webhooks from running correctly, an operator would have to delete all `ValidatingWebhookConfiguration`s owned by `cluster-capi-operator`.
They will not be recreated while the CRD compatibility checker is not running.