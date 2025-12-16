---
title: crd-compatibility-checker
authors:
  - "@mdbooth"
  - "@JoelSpeed"
reviewers:
  - "@JoelSpeed" # original feature architect
  - "@damdo" # Cluster Infrastructure team lead
  - "@csrwng" # integration with HyperShift
  - "@rokej" # integration with HyperShift
  - "@muraee" # integration with HyperShift
  - "@bryan-cox" # integration with HyperShift
approvers:
  - "@JoelSpeed"
api-approvers:
  - "@JoelSpeed"
creation-date: 2025-09-10
last-updated: 2025-09-10
tracking-link:
  - "https://issues.redhat.com/browse/OCPCLOUD-3005"
  - "https://issues.redhat.com/browse/OCPCLOUD-3006"
see-also: []
---

# CRD Compatibility Checker

## Summary

We are on the cusp of introducing a new suite of APIs into OpenShift, the Cluster API or CAPI APIs.
We know already that OpenShift clusters are running these APIs, be that within HyperShift, MCE or even as customer workloads today.
As we introduce these APIs into OpenShift's core payload, we need to ensure that we do not break existing use cases,
and that we allow these use cases to continue to manage CRDs, in a way that does not break our core usage.

## Motivation

CRDs required by the OpenShift payload are typically managed by CVO, or by operators which are managed by CVO. Changes to them are not normally permitted as this could undermine OpenShift stability.
However, we are encountering cases where we want to use APIs in the OpenShift payload that other workloads may also want to use.
We cannot assume that the versions required by the workload and OpenShift itself are the same.

In general, later versions of well-behaved kube APIs are compatible with earlier versions.
An earlier version of a controller *should* operate correctly with a later version of the CRD, as long as the CRD remains compatible.
In practice this will usually work, but some care needs to be taken.

### User Stories

* As an OpenShift cluster administrator using the latest upstream Cluster API, I need to disable OpenShift's management of Cluster API CRDs so that it does not downgrade or otherwise make a breaking change to the CRDs for my existing workloads.

* As a member of the HyperShift team, I need to be able to disable OpenShift's management of Cluster API CRDs, so that I can install newer CRDs with newer features into HyperShift management clusters within ROSA and ARO.

* As an OpenShift operator developer, I want to ensure that external CRD managers cannot install incompatible versions of CRDs that my operator depends on so that my operator continues to function correctly.

* As an OpenShift cluster administrator using the latest upstream Cluster API, I want the cluster to give me a clear signal if a cluster upgrade would be incompatible with the CRDs I am using.

* As a HyperShift administrator, I want to check if a planned CRD upgrade will be permitted by the management cluster before starting the upgrade.

* As a cluster administrator, I want to be sure that the Cluster API objects I use to manage the cluster itself will not change behaviour on upgrade.

### Goals

* Prevent the GA of Cluster API within OpenShift from impacting existing Cluster API users
* Allow actors external to the core payload (HyperShift, MCE, Customers) to take responsibility for the management of a subset of Cluster API CRDs from the core OpenShift product
* Build tooling to prevent CRDs being upgraded in a way that would break the payload-installed controllers
* Build tooling to prevent OpenShift upgrade when it would not be compatible with unmanaged CRDs
* Build tooling to ensure objects managed by core OpenShift controllers still conform to the required OpenShift schema when the CRD is unmanaged
* Define how CRD upgrades will be expected to work when unmanaged
* Minimize operational changes required for existing external workloads

### Non-Goals

* Allow unmanaged CRDs to later become managed again
* Enforcing single ownership of CRDs (must be ensured out-of-band)
* Providing a user interface for managing compatibility requirements
* Resolving compatibility conflicts between different actors

## Proposal

We will allow non-payload actors to take full responsibility for the lifecycle management of certain CRDs.
We will provide additional tooling to facilitate this.

We define the following 3 kinds of actors, each of whom has modified responsibilities under this proposal.

* The adopting manager: assumes responsibility for the CRD from Cluster CAPI Operator.
* Cluster CAPI Operator: relinquishes responsibility for the CRD from the core payload.
* CRD user: runs controllers which use the API.

All of these actors are cluster admins, but in practise their actions may be carried out by different teams.

### Actor responsibilities

The responsibilities of each are as follows:

#### Adopting Manager

* Provides a CRD compatible with the requirements of all other CRD users in the cluster.
* If multiple API versions are required, runs a conversion webhook.
* Runs storage migrations when changing storage version.
* Communicates API version deprecation lifecycles to other CRD users.
* Runs validating and mutating webhooks for all namespaces appropriate to the Adopting manager's storage version[^adopting-manager-webhooks].
[^adopting-manager-webhooks]: Although note that the CRD user may run additional validating and mutating webhooks against objects in their own namespace.

Note that the Adopting Manager is also expected to be a CRD User.

#### Cluster CAPI Operator

* Stops applying updates to the CRD on behalf of the core payload.

Note that Cluster CAPI Operator is also a CRD User.

#### CRD User

Note that actors with another role may also be CRD Users.
There may also be additional CRD Users.
For example when the adopting manager is ACM and deploys Hypershift, all of the following are CRD Users:
- ACM
- Hypershift
- Cluster CAPI Operator

* Configures controllers carefully to reconcile only their own objects, e.g. by namespace.
* Configures CRD Compatibility Requirements for their own controllers.

Note that the Adopting Manager MAY opt out of the latter requirement, assuming it loads the precise CRD that it requires.

### Implementation

The Cluster CAPI Operator will implement a way to yield management of any CRD that it would typically manage.
The API is described in this document.
The Adopting Manager will need to, prior to upgrading to a version of OpenShift that includes the Cluster CAPI Operator, configure the operator CRD to disable management of the CRDs that they wish to manage themselves.

We will introduce a new component, the CRD Compatibility Checker, which can enforce compatibility requirements on CRDs at admission.
These compatibility requirements will be configured by the CRD user based on the requirements of the controllers they are running.
Cluster CAPI Operator will use this to ensure that unmanaged CAPI CRDs remain compatible with the CAPI controllers executed by the core payload.

The CRD Compatibility Checker can also perform schema validation on objects against a custom schema.
The Cluster CAPI Operator will configure CRD Compatibility Checker to perform schema validation on objects of unmanaged CRDs used by its operands.
This will ensure objects presented to Cluster CAPI Operator operands conform to the expected schema rather than a later version of the schema which may permit, for example, additional fields or values.

### Scope of compatibility requirements

A CRD user MUST add a compatibility requirement sufficient to cover the direct API usage of its controllers.
For example, if a CAPI CRD defines both `v1beta1` and `v1beta2` but its controllers only read and write `v1beta2` (relying on a conversion webhook for `v1beta1` support), the CRD user MUST add a compatiblity requirement for `v1beta2`.
It MAY add a compatibility requirement for `v1beta1`, but note that this constrains the Adopting Manager to continue providing `v1beta1`, and may prevent them from upgrading to the latest version which removes it.

The CRD user SHOULD NOT assert compatibility requirements for parts of the API which it does not strictly need, as this would constrain the Adopting Manager from upgrading the CRD to a version which no longer needs them.
For example, in the above example the CRD user SHOULD NOT add a compatibility requirement for `v1beta1`, as it does not use it directly.

This also applies to deprecated fields.
If the Compatibility CRD provided by the CRD user contains deprecated fields which are no longer used by the version of the controller being deployed, the CRD user SHOULD explicitly exclude them from compatibility requirements by listing them in `excludedFields`.

For its own operands, the Cluster CAPI Operator will add a compatibility requirement for:
* the storage version defined in its operand CRD, which we assume is the API version directly used by the operand's controllers.
* any version specified in a `deps.v1.cluster-capi.operator.openshift.io/<crd name>` annotation on any object in any transport configmap.

The latter allows a CAPI infrastructure provider to continue to require an older version of the core CAPI apis.
For example, Cluster API Provider AWS may continue to require the CAPI `v1beta1` api after core CAPI itself has updated its storage version to `v1beta2`.

Note that this may mean users of this API may have a shorter deprecation cycle than if the CRD was managed by Cluster CAPI Operator.
It is the responsibility of the Adopting Manager to communicate this clearly.

### Definition of compatible

A CRD is compatible with a compatibility requirement if:
* All fields defined in the compatibility requirement are present
* **TODO**: ...other things

### Workflow Description

#### For a cluster that already has Cluster API CRDs installed

We know that there are already clusters running Cluster API CRDs, be that within HyperShift, MCE, or even as customer workloads today.
For these clusters, we must allow them to continue to manage the CAPI CRDs that they have installed without the Cluster CAPI Operator trying to take ownership of them.

For the sake of the following example we will assume that 4.Y is the first version of OpenShift that includes the Cluster CAPI Operator in it's fully functional mode in the stable channel.

1. The cluster admin upgrades to version 4.Y-1.z, which includes the Cluster CAPI Operator in a reduced mode.
In this mode it implements pre-upgrade checks, but does not yet manage any CRDs or install any operands.
Version 4.Y-1.z includes the 'transport' configmaps from version 4.Y.
These contain the 4.Y operands, including CRDs.
1. The Cluster CAPI Operator finds a CRD which:
   * is also defined in a transport configmap
   * does not contain metadata identifying it as managed by Cluster CAPI Operator.
     **Open Question**: What does this metadata look like?
   * is not listed as an unmanaged CRD
1. The Cluster CAPI Operator marks itself as `Upgradeable=False`, with a message showing that one or more CRDs are already installed.
1. The cluster admin configures the Cluster CAPI Operator to disable management of the CRDs that are already installed.
1. The Cluster CAPI Operator now reports `Upgradeable=True`, and the cluster admin can proceed with the upgrade to 4.Y.
1. Once upgraded to 4.Y, the Cluster CAPI Operator will not attempt to manage the CRDs that are already installed, and listed in the `UnmanagedAPIs` field of the operator CRD.

Note that in practise marking CRDs as unmanaged is likely to be automated by the tool taking the role of Adopting Manager, e.g. ACM or Hypershift.

#### For a cluster that is installing Cluster API CRDs for the first time

In this example, we assume that the cluster is either installed with the Cluster CAPI Operator initially, or has been upgraded into a version of OpenShift that includes the Cluster CAPI Operator in a fully functional mode.

The cluster admin in this case has an operational CAPI environment within the cluster, and the cluster is managing its own CAPI CRDs.

The cluster admin now wishes to install a newer version of a CAPI CRD than the version that exists in the cluster payload.

1. The cluster admin adds the name of the CRD to the `UnmanagedAPIs` field of the Cluster CAPI Operator CRD spec.
1. The Cluster CAPI Operator configures CRD Compatibility Checker to ensure updates to this newly unmanaged CRD are validated.
1. The Cluster CAPI operator updates the `ObservedGeneration` field in the operator config status to reflect that it has acted upon the desired new configuration.
1. The cluster admin has now taken the role of Adopting Manager going forward.

#### When a new version of a CRD is introduced

In this example, we assume that the cluster is running, and that the admin has already marked a particular CRD as unmanaged.
They now wish to upgrade to a new version of the CRD, this time, with a v1 schema, where previously the CRD was v1alpha1.
A Cluster CAPI Operator operand requires v1alpha1.
We assume that they initially attempt to apply an incorrect update.

1. The cluster admin applies a new CRD to the cluster, which only has the v1 schema.
1. CRD Compatibility Checker's webhook detects that the new CRD does not contain the v1alpha1 schema, and rejects the update.
1. The cluster admin updates instead to an earlier version of the CRD which includes both v1 and v1alpha1 schemas, which succeeds.
1. The cluster admin deploys the required conversion webhook for the v1alpha1 to v1 conversion.
1. The cluster admin initiates a storage migration to `v1`.

Over time, the admin upgrades various components and now decides it is time to remove the v1alpha1 schema from the CRD.
OpenShift 4.Y is the current version, which requires v1alpha1.
OpenShift 4.Y+1 requires v1 instead of v1alpha1.

1. The cluster admin again tries to apply the CRD to the cluster without the v1alpha1 schema.
1. CRD Compatibility Checker's validation webhook detects that the payload CAPI components still require the v1alpha1 schema, and rejects the update.
1. The cluster admin updates to the latest patch release of OpenShift 4.Y, which contains the CRD requirements for 4.Y+1.
1. The Cluster CAPI Operator runs pre-flight checks and detects that the current version of CRD contains a compatible version of the v1 API, and marks itself `Upgradeable=True`.
1. The cluster admin upgrades the cluster to 4.Y+1.
1. During the upgrade, Cluster CAPI Operator updates the CRD's compatibility requirement to be `v1`, as used by the new operand.
1. The cluster admin can now update the CRD to remove the v1alpha1 schema.

#### When fields are removed from the schema, without a version bump

Over time, it is possible that fields may be removed from schemas, without a version bump.
For example, Cluster API is introducing a `deprecated.v1beta1` field into the status of their core CRDs that will exist for some time, marked as deprecated, before being removed when the conversion between v1beta1 and v1beta2 is no longer required.
Without further measures, a CRD Compatiblity Requirement will prevent upgrading to a CRD which removes a field.
The `excludedFields` field of CRD Compatibility Requirement prevents validation of specific fields, so allows the Adopting Manager to upgrade to a CRD which removes the field.

In the case of `deprecated.v1beta1` in CAPI specifically, because this is a common pattern in CAPI, and controllers do not access the field directly, Cluster CAPI Operator will hard-code an exclusion for this field until it is no longer relevant to the current release.
Consequently, for `deprecated.v1beta1` specifically no further action is required.
We can take a similar approach for the deprecation or technically-incompatible modification of other fields.

#### Cluster CAPI Operator ensures cluster stability while Hypershift manages CAPI CRDs

This example goes into more detail on the precise interactions with CRD Compatibility Checker than the above.

In this example, Hypershift takes responsibility for the management of the cluster-api (CAPI) `Machine` CRD from cluster-capi-operator (CCAPIO).

CCAPIO deploys CAPI version X and uses it to manage CAPI resources used by the cluster itself.

Hypershift deploys CAPI version Y, where Y>X. Hypershift uses it to deploy external managed clusters.

As `Machine` can only have a single CRD in the management cluster, they must both use the same CRD.

* Hypershift will assert ownership of the `Machine` CRD
* CCAPIO will assert its requirements for the `Machine` CRD, but will no longer manage the CRD.
* Hypershift will upgrade the `Machine` CRD
* CRD Compatibility Checker ensures that the version Hypershift upgrades to does not break CCAPIO, and therefore the management cluster.

Hypershift asserts ownership of the `Machine` CRD by adding `machines.cluster.x-k8s.io` to CCAPIO's `UnmanagedAPIs`.
This is the only operational change required by Hypershift.

CCAPIO observes the change to UnmanagedAPIs, and ensures that a CompatibilityRequirement exists for every entry.
In the case of `Machine` this would be:

```yaml
apiVersion:	apiextensions.openshift.io/v1alpha1
kind:	CompatibilityRequirement
metadata:
  name:	cluster-api-machine-ccapio
spec:
  compatibilitySchema:
    customResourceDefinition:
      data: |
        ...
        <complete YAML document of Machine CRD from transport config map>
        ...
      type: YAML
    excludedFields:
    - path: "status.deprecated"
    requiredVersions:
      # The v1beta1 version was added because we observed an annotation on a Deployment in OpenStack's
      # transport configmap indicating that it is using CAPI Machine v1beta1.
      additionalVersions:
      - v1beta1
      defaultSelection: StorageOnly # Valid options are StorageOnly and AllServed.
  customResourceDefinitionSchemaValidation:
    action: Deny
  objectSchemaValidation:
    action: Deny
    namespaceSelector:
      matchLabels:
        "kubernetes.io/metadata.name": "openshift-cluster-api"
```

CCAPIO marks itself Degraded if CompatibilityRequirement it owns becomes not Progressing, and:
* not Admitted - transport config map is invalid
* not Compatible - current CRD is incompatible with CCAPIO requirements

Hypershift proceeds normally and upgrades the `Machine` CRD.
Assuming that the CRD from version Y is compatible with the one for version X required by CCAPIO, the CRD is admitted and everything works as normal.

#### CCAPIO prevents cluster upgrade if it would conflict with an existing CRD

This example covers the scenario when CCAPIO is first enabled in OCP, but more generally if CCAPIO starts using a CRD which Hypershift is currently using.
We want to prevent upgrade if a CRD currently in use would be incompatible with the version required by CCAPIO after upgrade.

OCP 4.Y is the current version.
OCP 4.Y+1 is the upgraded version.

Some time during the OCP 4.Y lifecycle we add an additional set of transport config maps to 4.Y.z which contain the CRDs required by 4.Y+1.
CCAPIO scans these, but does not load them.
For each CRD it creates a new CompatibilityRequirement.
e.g. for the `Machine` CRD it would create:

```yaml
apiVersion:	apiextensions.openshift.io/v1alpha1
kind:	CompatibilityRequirement
metadata:
  name:	cluster-api-machine-ccapio
spec:
  compatibilitySchema:
    customResourceDefinition:
      data: |
        ...
        <complete YAML document of Machine CRD from transport config map>
        ...
      type: YAML
    excludedFields:
    - path: "status.deprecated"
    requiredVersions:
      # The v1beta1 version was added because we observed an annotation on a Deployment in OpenStack's
      # transport configmap indicating that it is using CAPI Machine v1beta1.
      additionalVersions:
      - v1beta1
      defaultSelection: StorageOnly
  customResourceDefinitionSchemaValidation:
    action: Warn
```

The differences from the previous example are:
* The requirement identifies itself as being related to a different version.
* `customResourceDefinitionSchemaValidation.action` is `Warn` instead of `Deny`, so this requirement will not prevent updates to the current CRD.
* There is no `objectSchemaValidation`.

CCAPIO will mark itself as not upgradeable if any CompatibilityRequirement created for an upgrade transport config map reports its `Compatible` condition as `False`.

### API Extensions

The following is an outline of the CompatibilityRequirement CRD:

```yaml
apiVersion:	apiextensions.openshift.io/v1alpha1
kind:	CompatibilityRequirement
metadata:
  name:	cluster-api-machine-ccapio
spec:
  #compatibilitySchema defines the schema used by customResourceDefinitionSchemaValidation and objectSchemaValidation.
  compatibilitySchema:
    # customResourceDefinition contains the complete definition of the CRD for schema and object validation purposes.
    customResourceDefinition:
      data: |
        ...
        <complete YAML document of Machine CRD from transport config map>
        ...
      type: YAML
    # excludedFields is a set of fields in the schema which will not be validated
    # by customResourceDefinitionSchemaValidation or objectSchemaValidation.
    excludedFields:
    - path: "status.deprecated"
      # versions are the API versions the field is excluded from.
      # When not specified, the field is excluded from all versions.
      # versions: []
    # requiredVersions specifies a subset of the CRD's API versions which will be
    # asserted for compatibility.
    requiredVersions:
      # additionalVersions specifies a set api versions to require in addition to
      # the default selection. It is explicitly permitted to specify a version in
      # additionalVersions which was also selected by the default selection. The
      # selections will be merged and deduplicated.
      additionalVersions:
      # - v1
      # defaultSelection specifies a method for automatically selecting a set of
      # versions to require.
      # Valid options are StorageOnly and AllServed.
      defaultSelection: StorageOnly
  # customResourceDefinitionSchemaValidation ensures that updates to the
  # installed CRD are compatible with this compatibility requirement. If not
  # specified, admission of the target CRD will not be validated.
  # This field is optional.
  customResourceDefinitionSchemaValidation:
    action: Deny # Valid options are Deny and Warn
  # objectSchemaValidation ensures that matching resources conform to
  # compatibilitySchema.
  objectSchemaValidation:
    action: Deny # Valid options are Deny and Warn
    # matchConditions defines the matchConditions field of the resulting
    # ValidatingWebhookConfiguration.
    matchConditions:
    - expression: <cel expression for evaluation>
      name: identifier-for-the-match-condition
    # namespaceSelector defines a label selector for namespaces.
    namespaceSelector:
      matchExpressions:
      - key: "kubernetes.io/metadata.name"
        operator: In # One of In, NotIn, Exists or DoesNotExist
        values:
        - openshift-cluster-api
      matchLabels:
        "kubernetes.io/metadata.name": "openshift-cluster-api"
    # objectSelector defines a label selector for objects.
    objectSelector:
      matchExpressions:
      - key: some-label # label key for the selector
        operator: In # One of In, NotIn, Exists or DoesNotExist
        values:
        - on-an-object
      matchLabels:
        some-label: on-an-object
status:
  # conditions is a list of conditions and their status.
  # Known condition types are Progressing, Admitted, and Compatible.
  conditions: []
  # crdName is the name of the target CRD. The target CRD is not required to
  # exist, as we may legitimately place requirements on it before it is
  # created.  The observed CRD is given in status.observedCRD, which will be
  # empty if no CRD is observed.
  crdName: machines.cluster-api.x-k8s.io
  # observedCRD documents the uid and generation of the CRD object when the
  # current status was written.
  observedCRD:
    generation: <generation>
    uid: <uuid>
```

#### Conditions

CompatibilityRequirement defines 3 conditions:

* `Progressing` -
	CompatibilityRequirementProgressing is false if the spec has been
	completely reconciled against the condition's observed generation.
	True indicates that reconciliation is still in progress and the current status does not represent
	a stable state. Progressing false with an error reason indicates that the object cannot be reconciled.

* `Admitted` -
	CompatibilityRequirementAdmitted is true if the requirement has been configured in the validating webhook,
	otherwise false.


* `Compatible` -
	CompatibilityRequirementCompatible is true if the observed CRD is compatible with the requirement,
	otherwise false. Note that Compatible may be false when adding a new requirement which the existing
	CRD does not meet.

The above conditions are always set on every reconcile, and include `observedGeneration`.

These conditions may have the following Reasons:

#### Progressing

| Reason | Associated Status | Description |
|---|---|---|
| `ConfigurationError` | `False` | This indicates that reconciliation cannot progress due to an invalid spec. |
| `TransientError` | `True` | This indicates that reconciliation failed due to an error that can be retried. |
| `UpToDate` | `False` | This indicates that reconciliation completed successfully. |

#### Admitted

| Reason | Associated Status | Description |
|---|---|---|
| `Admitted` | `True` | This indicates that the requirement has been configured in the validating webhook. |
| `NotAdmitted` | `False` | This indicates that the requirement has not been configured in the validating webhook. |

#### Compatible

| Reason | Associated Status | Description |
|---|---|---|
| `RequirementsNotMet` | `False` | This indicates that a CRD exists, and it is not compatible with this requirement. |
| `CRDNotFound` | `False` | This indicates that the referenced CRD does not exist. |
| `CompatibleWithWarnings` | `True` | This indicates that the CRD exists and is compatible with this requirement, but `Message` contains one or more warning messages. |
| `Compatible` | `True` | This indicates that the CRD exists and is compatible with this requirement. |

#### Validating webhooks

##### CustomResourceDefinition

Create, Update, and Delete operations on CRDs will either be denied or emit a warning depending on the setting of `customResourceDefinitionSchemaValidation.action`.

Create and Update will deny/warn if the created or updated CRD would not be compatible with any CompatibilityRequirement which references it in `status.crdName`.

Delete will deny/warn if any CompatibilityRequirement references it in `status.crdName`.

##### Dynamically created by `objectSchemaValidation`

[!NOTE]
Implementation of `objectSchemaValidation` will be deferred to a second phase to be implemented after the CRD validation webhook is fully implemented and integrated.

If a CompatibilityRequirement defines `objectSchemaValidation` we will dynamically create a new `ValidatingWebhookConfiguration` to implement it.
This webhook will ensure that matching objects conform to the CRD given in `compatibilitySchema`, which may be different to the current CRD version.
It will either warn or deny admission based on the setting of `objectSchemaValidation.action`.
E.g. the `ValidatingWebhookConfiguration` will warn or deny a Machine object in the namespace `openshift-cluster-api`, if it has fields set that exist
in the CustomResourceDefinition, but not in the schema configured in the CompatibilityRequirement.

This webhook will have the same lifecycle as the CompatibilityRequirement which created it.

The webhook will only be called for objects matching the selectors in `objectSchemaValidation`.

### Topology Considerations

#### Hypershift / Hosted Control Planes

The Hypershift management cluster is one of the primary intended users of this feature.

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
  The kube-apiserver metric `apiserver_admission_webhook_admission_duration_seconds` can be considered to measure the performance.
  
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
  We expect the initial and primary non-payload customers to be ACM and Hypershift.
  We will coordinate with ACM and Hypershift to ensure that potential issues show up early in their CI pipeline.

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

### Removing a deprecated feature

None

## Upgrade / Downgrade Strategy

CRDCompatibilityChecker has no direct impact on cluster operations unless it is configured.
We handle upgrade and downgrade by altering the behaviour of cluster-capi-operator, which is responsible for the in-payload configuration.

The following uses an existing per-platform feature gate, `ClusterAPIMachineManagement<platform>`, and proposes a new featuregate `ClusterAPIMachineManagement<platform>Upgradeability`.

### `ClusterAPIMachineManagement<platform>Upgradeability`

This is a new feature gate proposed by this enhancement.

This should be enabled at least 1 release prior to `ClusterAPIMachineManagement<platform>`.

When enabled, cluster-capi-operator will:
* Create a `CompatibilityRequirement` for every CRD discovered in the 'future' transport config map if that CRD is also marked 'unmanaged'.
This requirement will `Warn`.
* Mark itself not upgradeable if any of these are marked `Compatible=False`.

### `ClusterAPIMachineManagement<platform>`

This is the existing feature gate which enables CAPI for a particular platform.

If it is enabled, cluster-capi-operator will:
* Create a `CompatibilityRequirement` for every CRD in the 'active' transport config map if that CRD is also marked 'unmanaged'.
This requirement will `Enforce`.

Upgrading to first version of OCP to include this feature:
- `ClusterAPIMachineManagement<platform>Upgradeability` was enabled in 4.Y-1.
- `ClusterAPIMachineManagement<platform>` is enabled in 4.Y, but not 4.Y-1.
- Users of unmanaged APIs must mark them 'unmanaged' in 4.Y-1.
- OCP will not permit upgrade if unmanaged CRDs are incompatible with 4.Y.
- On upgrade, OCP will enforce compatibility.

During subsequent upgrades:
- cluster-capi-operator ensures that its `active` and `future` reflect the respective transport config maps from the current OCP version.

We do not intend to support downgrades, although we believe the proposed solution provides enough context to implement them if required.

## Version Skew Strategy

The system must handle version skew during upgrades:
- Webhook and controllers must be compatible with both the current and previous API versions

## Operational Aspects of API Extensions

- Fails all Create, Update, and Delete operations on CRDs if the webhook is not running.
- Fails all Create, Update, and Delete operations on CAPI objects in the `openshift-cluster-api` namespace if the webhook is not running.
- Runs an informer cache of CRD objects, so potentially large memory usage.
- Expected use cases involve a static number of `CompatibilityRequirement` objects in the order of 10s.
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