# Metadata Controller

The metadata controller provides capability to add, update, delete and reconcile metadata required for cloud resources.
The scope of the present proposal is to enable continuous management of user-defined tags for cloud resources. The APIs in the
proposal have been deliberately designed to be modular for extensibility in the early versions. The API will be refined and updated
based on use-cases towards beta versions.

## Summary

Tagging cloud resources enable users to perform administrative operations like, organize the resources, 
apply security policies, optimize operations, etc. The cloud resources can be tagged using cloud service provider tools, 
kubernetes controllers that create resources via cloud service provider api and other open source tools. A reconciliation 
using a generic metadata controller helps to keep the tags synchronized for cloud resources. 

In the following proposal, the controller supports mainly 
1. Day 2 operations and tag management for cloud resources selected using classifiers.
2. Own the tag list and reconcile to replace any edits performed external to the controller.
3. Opcodes in specification for handling user request as an operation on tag list.
4. Extensible APIs and segregated configuration for controller and cloud provider specific configurations.

## Motivation

Users should be able to maintain lifecycle of tags for cloud resources created during installation or by other day-2 kubernetes
controllers and operators using kubernetes API.

### User Stories

As a cluster admin, I want to add tags to the cloud resources created at cluster installation,  which help
for cost calculation and reports.

As a cluster admin, I want tags to be added to the cloud resources created post cluster installation which help to include
cloud resources for other tag processors.

As a cluster admin, I want all tags to be added via metadata CR, which helps to reconcile the tag key and values.

As a cluster admin, I want to add tags on selected resources, which are identified using specified labels/classifiers.

As a cluster admin, I want the tag list to be lexicographically sorted, to have a deterministic set of tags applied
when tag list size exceeds the maximum allowed limit on cloud resource.

As a cluster admin, I want the tag list to be lexicographically sorted, to have a deterministic set of tags appended to
existing tags for cloud resources when the tag list exceeds the maximum allowed limit on cloud resource.

As a cluster admin, I want to delete tags, which are not required on the cloud resources.

As a cluster admin, I want to be able to ignore cloud resource from all tag operations, which helps to avoid overwriting values 
set by cloud service provider's policies.

### Goals

1. Enable continuous management operations (create, append , update and delete) applicable for tagging 
 cloud resources created by kubernetes services.
2. Provide a new custom resource of kubernetes api for specification and status of tags.
3. Enable continuous monitoring of tags applied and reconcile when there are changes made external to 
the controller's API.
4. Provide CLI for operations, cloud service provide authentication and ability to edit custom resource with ease.

### Non-Goals

1. Managing metadata which are not related to cloud resource tagging.
2. Sub-set of input tags to be applied on selected cloud resource.
3. Any conflict with cloud service provider tagging policies will not be managed by controller.
4. Regular expression or wildcard characters for classifier tag.
5. Metrics collection and audit logging features for controller and user operations.
6. UI frontend for managing tags.

## Proposal 

A new kubernetes controller to generates actions based on tag specification defined by a custom resource. The controller
reads existing data, applies tags based on opcode and maximum limit policy. The controller would require an initial tag or label, 
that can be used to identify cloud resources to apply and manage tags. Cloud resources can be ignored from being managed 
by the controller by removing the classifier tag on the cloud resource. Controller reconciles the tag list on 
cloud resources periodically at intervals configured by sync period for the controller. 

### Controller configuration

#### Maximum limit policy

Maximum limit policy indicates whether the input tag list can be applied partially when the length of the list exceeds the 
allowed limit on the cloud resource. The result might differ when applied along overwrite policy. Following are the list of valid values.

1. POLICY_APPLY_PARTIAL - Allows input tag list applied to be partial on cloud resource. 
2. POLICY_APPLY_STRICT - Applies input tag list strictly in complete.

### Opcodes

Controller supports different opcodes for actions to add, update and delete tags. Following are the supported opcodes
1. "add" - Controller adds tags to the cloud resources. In case of an existing entry for the tag, controller reports failure.
2. "update" - Controller updates existing tags for the cloud resources. If there is no existing entry, controller reports failure.
3. "delete" - Controller removes tags from the cloud resources. In case of specific errors, user intervention may be required to remove tags manually
from the cloud resources.

### Workflow Description

#### Bootstrap
1. User starts the controller with cloud service provider credentials and sync period set.
2. Controller starts to list/watch custom resource.
3. User creates `AWSMetadata` object with tag list details.
4. User creates `CloudMetadata` configuration with controller configuration.
5. Based on the configuration, controller queries and lists all resources based on classifier tag.
6. Creates list of existing tags on cloud resource.
7. Add/update tag workflow is initiated.

#### Add/Update tags

1. User creates `AWSMetadata` object with tag list details.
2. User creates `CloudMetadata` configuration with controller configuration.
3. Controller updates ready condition to false in `MetadataStatus.status`.
4. Controller validates tag entries from `AWSMetadata.spec.resourcetags` and populates a new list of tags to be added to cloud resource.
5. Controller appends tags to existing list and trims as per maximum limit policy specific to cloud resource.
6. Controller performs lexicographic sorting on the trimmed tag list.
7. Controller adds/updates the tag list on the cloud resource.
8. Controller updates the tag list to `AWSMetadata.status`.
9. Controller updates ready condition to true in `MetadataStatus.status`.

#### Delete tags

1. User adds tag key list (optionally, value, for validation) to custom resource specification.
2. User specifies operation to be performed on the tag list by specific opcode.
3. Controller updates ready condition to false in `MetadataStatus.status`.
4. Controller gets the active tag list, removes requested tags from the list and populates a new list of tags.
5. Controller replaces the tag list on the cloud resource.
6. Controller updates the tag list to `AWSMetadata.status`.
7. Controller updates ready condition to true in `MetadataStatus.status`.

#### Handling of pre-existing tags or tags added using other tools

In case of add operation, controller appends tags to existing tag list. If there is any tag pre-existing with same key name,
there will be an error reported for the operation. While, in case of update operation, the pre-existing tag is updated. There is
no distinction made between pre-existing tags and tags in `AWSMetadataSpec` for reconciliation. The active tag list is listed in the status.

#### Classifier tag behaviour

Classifier tags are used to identify cloud resources for which the controller should manage the tags. Classifier tags are mandatory and should
be added to cloud resource. Controller does not support any wildcard characters in classifier strings.
When multiple classifiers are used, a logical OR condition is applied for the classifiers.

#### Reconciliation

Reconciliation of tags is based on active tag list in `AWSMetadata.status.resourcetags`. Reconciliation of tags is ignored when ready condition is set to false at `CloudMetadata.MetadataStatus.status`. On deletion of AWSMetadataRef
there is no reconciliation of tags. If AWSMetadataRef refers to non-existing `AWSMetadata` object , controller fails with error condition and updates `CloudMetadata.MetadataStatus.status` to false.

#### No cloud provider configured

When there is no cloud provider object configured, there will be no operation by default.

#### Deleting classifier tag

Controller does not delete, update or reconcile classifier tags. Controller refers them as read-only tags. 

### Cloud provider authentication

TBD

### API Extensions

#### Example

```yaml
apiVersion: metadata-controller.cloud.io/v1alpha1
kind: CloudMetadata 
metadata: 
  name : metadata
  namespace: metadata-controller
spec:
  cloudprovider:
    awsref:
      name: awstags
      namespace: metadata-controller
  classifiers:
    "kubernetes.io/cluster/test-7lpkm-crhfx" : "owned"
  controllerconfig:
    limit: "POLICY_APPLY_PARTIAL"
    syncevery: "10m"
status:
  type: ready
  status: true
  reason: "none"
  message: "tags are applied successfully and can be referred by users"
```

```yaml
apiVersion: metadata-controller.aws.io/v1alpha1
kind: AWSMetadata
metadata:
  name: awstags
  namespace: metadata-controller
spec:
  resourcetags:
    "env": "test"
    "centre": "eng"
status:
  type: applied
  status: true
  reason: "none"
  message: "tags are applied successfully to cloud resources"
  resourcetags:
    "env": "test"
    "centre": "eng"
```
### GO language structures
```go
type CloudMetadata struct {
    metav1.TypeMeta `json:",inline"`
    metav1.ObjectMeta `json:"metadata,omitempty"`
    
    Spec MetadataSpec `json:"spec"`
    
    Status MetadataStatus `json:status`
}

type MetadataSpec struct {
	// CloudProviderSpec defines cloudprovider specific configurations
	// to authenticate, apply metadata and applicable policies.
    CloudProviderSpec CloudProviderSpec `json:"cloudprovider"`
	
	// GlobalClassifiers are the tags applied on cloudresouces for 
	// identification by user. Controller uses classifiers to identify
	// cloud resources. GlobalClassifiers are common to all listed 
	// cloud providers in CloudProviderSpec.
    GlobalClassifiers ClassifierSpec `json:"classifier"`
	
	// GlobalControllerConfig are controller specific behavioral configurations.
    GlobalControllerConfig *ControllerConfig `json:"controllerconfig, omitempty"`
}

type CloudProviderSpec struct {
    // TODO: variable list of cloud type required
	// CloudProviderSpec uses reference to different cloud provider specific API objects.
    AWS *AWSMetadataRef `json:"awsmetadataref", omitempty`
    //... and other cloud providers
}

type AWSMetadataRef struct {
    Name string `json:"name"`
    Namespace string `json:"namespace"`
}

type AWSMetadata struct {
    metav1.TypeMeta `json:",inline"`
    metav1.ObjectMeta `json:"metadata,omitempty"`

    Spec AWSMetadataSpec `json:"spec"`

    Status CloudProviderStatus `json:status`
}

type AWSMetadataSpec struct {
	OpCode string `json:"opcode"`
    ResourceTags map[string]string `json:"resourcetags"`
}

type OpCodeType string

// Opcodes supported by controller
const (
	OpAdd OpCodeType = "add"
	OpUpdate OpCodeType = "update"
	OpDelete OpCodeType = "delete"
)

type CloudProviderConditionType string

const (
	// CloudProviderConditionApplied indicates tags add/update operation completion condition.
    CloudProviderConditionApplied MetadataConditionType = "applied"
	
	// CloudProviderConditionApproved signifies the tags validation condition.
    CloudProviderConditionApproved MetadataConditionType = "approved"
	
	// CloudProviderConditionPolicy indicates if there are any policy condition failures.
    CloudProviderConditionPolicy MetadataConditionType = "policy"
	
	// CloudProviderConditionRequest indicates any operation condition.
    CloudProviderConditionRequest MetadataConditionType = "request"
)

type CloudProviderStatus struct {
    Type CloudProviderConditionType `json:"type"`
    Status metav1.ConditionStatus `json:"status"`
    LastTransitionTime *metav1.Time `json:"lastTransitionTime,omitempty"`
    Reason string `json:"reason, omitempty"`
    Message string `json:"message, omitempty"`
}

type MetadataStatusConditionType string

// Extensible for cases when there are multiple cloud providers being 
// referred to tag management.
const (
    MetadataConditionReady MetadataStatusConditionType = "ready"
)

type MetadataStatus struct {
	// Type and status signify the condition type and success status.
    Type MetadataStatusCondtitionType `json:"type"`
    Status metav1.ConditionStatus `json:"status"`
	
	// LastTransitionTime provides the time for last operation handled by the controller.
    LastTransitionTime *metav1.Time `json:"lastTransitionTime,omitempty"`
	
	// Reason gives the error codes in string for failures.
    Reason string `json:"reason, omitempty"`
	
	// Message details the string error messages.
    Message string `json:"message, omitempty"`
	
	// Resourcestags is the active list of tags applied to cloud resources.
    ResourceTags map[string]string `json:"resourcetags"`
}

type ClassifierSpec struct {
    Classifiers map[string]string `json:"classifiers"`
}

type ControllerConfig struct {
    LimitPolicy LPolicy `json:"limit, omitempty"`
	SyncEvery  *string  `json:"syncevery, omitempty"`
}
```

### Risks and Mitigations

1. Any user with permission to edit custom resource can influence tags on cloud resources with wide-scoped classifiers.

### Drawbacks

1. User intervention is required in some cases for failure resolution of opcode driven operations.
2. HA is not supported for the controller.
3. Override of global configurations specific to cloud provider are not supported.
4. Pre-existing tags or tags added using external tools on cloud resources can be updated using the controller.
5. Cloud provider tag policies will conflict with operations supported by controller. A review of the policies is required 
before usage of the controller.

## Design Details

### Open questions

1. Should controller override tags added on machine set spec which supersedes infrastructure object?
2. Should the controller override new tags added using external controllers on a controller-managed cloud resource? 

### Test Plan

1. Unit tests.
2. Integration tests for resources created by OpenShift installer and operators. 

### Graduation Criteria

The proposal is for Dev Preview.

#### Dev Preview

1. A stand-alone controller that can be deployed on OpenShift.
2. Integration with existing implementation of tag management in OpenShift.

#### Dev Preview -> Tech Preview
N/A

#### Tech Preview -> GA
N/A

#### Removing a deprecated feature
N/A

### Upgrade / Downgrade Strategy
N/A

### Version Skew Strategy
N/A

#### Failure Modes

#### Support Procedures

### Operational Aspects of API Extensions

`CloudMetadata.status` represents operational aspects of the controller and `AWSMetadata.status` represents operational aspects of operations
defined in `AWSMetadata.spec.opcode`

`AWSMetadata.spec.resourcetags` lists tags that are considered for the opcode. It may not have all tags for cloud resource.
`AWSMetadata.status.resourcetags` lists all tags applied on cloud resource using controller.

## Implementation History

In OpenShift, the tags are added when the cloud resources are created by installer or the operator. The tags are maintained at `infrastructure.status.aws.resourcetags` and 
cannot be updated.

## Alternatives

1. Customers can alternatively use cloud service provider or external tools. For example, in AWS, AWS tag editor can be used to edit tags, configure 
EventBridge rules and processor services to handle tag updates.

## Future work
1. Controller is able to perform tag policy management.
2. Controller metrics collection and audit logging.
3. UI-based frontend integration to manage tags.