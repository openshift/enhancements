---
title: gcp_user_defined_labels
authors:
  - "@bhb"
reviewers:
  - "@patrickdillon" ## reviewer for installer component
  - "@barbacbd" ## reviewer for installer component
  - "@JoelSpeed" ## reviewer for api and machine-api-provider-gcp components
  - "@flavianmissi" ## reviewer for cluster-image-registry-operator component
  - "@tsmetana" ## reviewer for storage component
approvers:
  - "@jerpeter1" ## approver for CFE
api-approvers:
  - "@JoelSpeed" ## approver for api component
creation-date: 2022-07-12
last-updated: 2023-08-01
tracking-link:
  - https://issues.redhat.com/browse/OCPPLAN-8155
  - https://issues.redhat.com/browse/CORS-2455
see-also:
  - "enhancements/api-review/custom-tags-aws.md"
  - "enhancements/api-review/azure_user_defined_tags.md"
replaces:
  - N/A
superseded-by:
  - N/A
---

# Apply user defined labels to all GCP resources created by OpenShift

## Summary

This enhancement describes the proposal to allow an administrator of OpenShift to
have the ability to apply user defined labels and tags to those resources created
by OpenShift in GCP.

## Motivation

Motivations include but are not limited to:

- Allow admin, compliance, and security teams to keep track of assets and objects
  created by OpenShift in GCP.

### User Stories

- As an OpenShift administrator, I want to have labels added to all resources created
  in GCP by OpenShift, so that I can query resources by the labels or filter resources
  by label in cloud billing.
- As an OpenShift administrator, I want to have tags added to all resources created
  in GCP by OpenShift, so that I can use it for creating IAM policies or to configure 
  organization policy constraints or to refine resource usage and cost data.

### Goals

- The administrator or service (in the case of Managed OpenShift) installing OpenShift
  can configure a list of up to 32 user-defined labels in the OpenShift installer generated
  install config, which is referenced and applied by installer and the in-cluster operators
  on the GCP resources during creation.
- Labels must be applied at creation time, in an atomic operation. Labels will not be added post cluster creation.
- The administrator or service (in the case of Managed OpenShift) installing OpenShift can configure up to 50 tags
  in the install config. The tags are added by installer and the in-cluster operators while creating GCP resources.

### Non-Goals

- Support for update or delete operation on labels or tags after creation of GCP resource.

## Proposal

***GCP Labels*** 

A label of the form `kubernetes-io-cluster-<cluster_id>:owned` will be added to every
resource created by OpenShift to enable administrator to differentiate the resources
created for OpenShift cluster. An administrator is not allowed to add or modify the label
having the prefix `kubernetes-io` or `openshift-io` in the key. The same can be found
applied to other cloud platform resources which supports labels/tags for ex: AWS, Azure.

New `userLabels` field will be added to `platform.gcp` of install-config for the user
to define the labels to be added to the resources created by installer and in-cluster operators.

If `platform.gcp.userLabels` in the install-config has any labels defined, then these labels
will be added to all the GCP resources created by OpenShift, provided all the defined
labels meet all the below conditions or else the cluster creation will fail.
1. A label key and value must have minimum of 1 character and can have maximum of 63 characters.
2. A label key and value must contain only lowercase letters, numeric characters,
   underscores, and dashes.
3. A label key must start with a lowercase letter.
4. Each resource can have multiple labels, up to a maximum of 32.
   - GCP supports a maximum of 64 labels per resource. Restricting the number of
    user defined labels to 32 and reserving 32 for Openshift's internal use.

All in-cluster operators that create GCP resources (Cluster Infrastructure ,Storage) will
apply these labels during resource creation.

The userLabels field is intended to be set at install time and is considered immutable.
Components that respect this field must only ever add labels that they retrieve from this
field to cloud resources, they must never remove labels from the existing underlying cloud
resource even if the labels are removed from this field(despite it being immutable).

If the userLabels field is changed post-install, there is no guarantee about how an
in-cluster operator will respond to the change. Some operators may reconcile the
change and update the labels on the GCP resource. Some operators may ignore the change.
However, if labels are removed from userLabels, the label will not be removed from the
GCP resource.

***GCP Tags*** 

Unlike labels, Tags are not metadata of a resource but a resource itself. Tags are key-value
pairs. Tag keys, values, and bindings are all discrete resources. A tag key resource must be
created under an organization or project resource. A tag value created for each key is a list
of values that can be assigned to the key. A maximum of 1000 keys can be created under a given
organization or project and there can be a total of 1000 values created for each key and OpenShift
will only create tag bindings and not tag keys or values.

New `userTags` field will be added to `platform.gcp` of install-config for the user
to define the tags to be added to the resources created by installer and in-cluster operators.

If `platform.gcp.userTags` in the install-config has any tags defined, then these tags
will be added to all the GCP resources created by OpenShift.

A tag key resource can be created under an organization or project resource and since OpenShift does
not create or manage organization or project resource, tags must already exist for OpenShift to apply
on the created resources. Tags bindings are inherited by the children of the resources. Users can
define list of new tags to add or update the inherited tags with acceptable values.

Though tags are not created by OpenShift, validations are performed on the user defined tags to
catch any invalid configurations before requesting GCP service. User defined tags must meet all
the below conditions or else the cluster creation will fail.
1. A tag parentID can be either OrganizationID or ProjectID. An OrganizationID must consist of
   decimal numbers, and cannot have leading zeroes and a ProjectID must be 6 to 30 characters
   in length, can only contain lowercase letters, numbers, and hyphens, and must start with a
   letter, and cannot end with a hyphen.
2. A tag key and value must have minimum of 1 character and can have maximum of 63 characters.
3. A tag key must contain only uppercase and lowercase alphanumeric characters, hyphens,
   underscores, and periods.
4. A tag value must contain only uppercase and lowercase alphanumeric characters, hyphens,
   underscores, periods, at signs, percent signs, equals signs, plusses, colons, commas,
   asterisks, pound signs, ampersands, parentheses, square braces, curly braces, and spaces.
5. A tag key and value must begin and end with an alphanumeric character.
6. Tag key must already exist.
7. Tag value is scalar type and must be one of the pre-defined values for the key.
8. A maximum of 50 tags can be configured.
9. Tag key with same value shouldn't already exist on a resource(inherited from parent resource)

### Workflow Description

***GCP Labels***
- An OpenShift administrator requests to add required labels to all GCP resources
  created by OpenShift by adding it in `platform.gcp.userLabels`
- OpenShift installer validates the labels defined in `platform.gcp.userLabels` and
  adds these labels to all resources created during installation and also updates
  `.status.platformStatus.gcp.resourceLabels` of the `infrastructure.config.openshift.io`
- In-cluster operators refers `.status.platformStatus.gcp.resourceLabels` of the
  `infrastructure.config.openshift.io` to add labels to the resources created later.

***GCP Tags***
- An OpenShift administrator requests to add required tags to all GCP resources
  created by OpenShift by adding it in `platform.gcp.userTags`.
- OpenShift installer validates the tags defined in `platform.gcp.userTags` and
  adds these tags to all resources created during installation and also updates
  `.status.platformStatus.gcp.resourceTags` of the `infrastructure.config.openshift.io`
- In-cluster operators refers `.status.platformStatus.gcp.resourceTags` of the
  `infrastructure.config.openshift.io` to add tags to the resources created later.

#### Variation [optional]

### API Extensions
Enhancement requires modifications to the below mentioned CRDs.
- Add `userLabels` and `userTags` fields to `platform.gcp` of the
  `installconfigs.install.openshift.io`
```golang
type Platform struct {
	// userLabels has additional keys and values that the installer will add as
	// labels to all resources that it creates on GCP. Resources created by the
	// cluster itself may not include these labels. This is a TechPreview feature
	// and requires setting CustomNoUpgrade featureSet with GCPLabelsTags featureGate
	// enabled or TechPreviewNoUpgrade featureSet to configure labels.
	UserLabels []UserLabel `json:"userLabels,omitempty"`

	// userTags has additional keys and values that the installer will add as
	// tags to all resources that it creates on GCP. Resources created by the
	// cluster itself may not include these tags. Tag key and tag value should
	// be the shortnames of the tag key and tag value resource. This is a TechPreview
	// feature and requires setting CustomNoUpgrade featureSet with GCPLabelsTags
	// featureGate enabled or TechPreviewNoUpgrade featureSet to configure tags.
	UserTags []UserTag `json:"userTags,omitempty"`
}

// UserLabel is a label to apply to GCP resources created for the cluster.
type UserLabel struct {
	// key is the key part of the label. A label key can have a maximum of 63 characters
	// and cannot be empty. Label must begin with a lowercase letter, and must contain
	// only lowercase letters, numeric characters, and the following special characters `_-`.
	Key string `json:"key"`

	// value is the value part of the label. A label value can have a maximum of 63 characters
	// and cannot be empty. Value must contain only lowercase letters, numeric characters, and
	// the following special characters `_-`.
	Value string `json:"value"`
}

// UserTag is a tag to apply to GCP resources created for the cluster.
type UserTag struct {
	// parentID is the ID of the hierarchical resource where the tags are defined,
	// e.g. at the Organization or the Project level. To find the Organization ID or Project ID refer to the following pages:
	// https://cloud.google.com/resource-manager/docs/creating-managing-organization#retrieving_your_organization_id,
	// https://cloud.google.com/resource-manager/docs/creating-managing-projects#identifying_projects.
	// An OrganizationID must consist of decimal numbers, and cannot have leading zeroes.
	// A ProjectID must be 6 to 30 characters in length, can only contain lowercase letters,
	// numbers, and hyphens, and must start with a letter, and cannot end with a hyphen.
	ParentID string `json:"parentID"`

	// key is the key part of the tag. A tag key can have a maximum of 63 characters and
	// cannot be empty. Tag key must begin and end with an alphanumeric character, and
	// must contain only uppercase, lowercase alphanumeric characters, and the following
	// special characters `._-`.
	Key string `json:"key"`

	// value is the value part of the tag. A tag value can have a maximum of 63 characters
	// and cannot be empty. Tag value must begin and end with an alphanumeric character, and
	// must contain only uppercase, lowercase alphanumeric characters, and the following
	// special characters `_-.@%=+:,*#&(){}[]` and spaces.
	Value string `json:"value"`
}
```

- Add `resourceLabels` and `resourceTags` fields to `status.platformStatus.gcp`
  of the `infrastructure.config.openshift.io`
```golang
// GCPPlatformStatus holds the current status of the Google Cloud Platform infrastructure provider.
// +kubebuilder:validation:XValidation:rule="!has(oldSelf.resourceLabels) && !has(self.resourceLabels) || has(oldSelf.resourceLabels) && has(self.resourceLabels)",message="resourceLabels may only be configured during installation"
// +kubebuilder:validation:XValidation:rule="!has(oldSelf.resourceTags) && !has(self.resourceTags) || has(oldSelf.resourceTags) && has(self.resourceTags)",message="resourceTags may only be configured during installation"
type GCPPlatformStatus struct {
	// resourceLabels is a list of additional labels to apply to GCP resources created for the cluster.
	// See https://cloud.google.com/compute/docs/labeling-resources for information on labeling GCP resources.
	// GCP supports a maximum of 64 labels per resource. OpenShift reserves 32 labels for internal use,
	// allowing 32 labels for user configuration.
	// +kubebuilder:validation:MaxItems=32
	// +kubebuilder:validation:XValidation:rule="self.all(x, x in oldSelf) && oldSelf.all(x, x in self)",message="resourceLabels are immutable and may only be configured during installation"
	// +listType=map
	// +listMapKey=key
	// +optional
	ResourceLabels []GCPResourceLabel `json:"resourceLabels,omitempty"`

	// resourceTags is a list of additional tags to apply to GCP resources created for the cluster.
	// See https://cloud.google.com/resource-manager/docs/tags/tags-overview for information on
	// tagging GCP resources. GCP supports a maximum of 50 tags per resource.
	// +kubebuilder:validation:MaxItems=50
	// +kubebuilder:validation:XValidation:rule="self.all(x, x in oldSelf) && oldSelf.all(x, x in self)",message="resourceTags are immutable and may only be configured during installation"
	// +listType=map
	// +listMapKey=key
	// +optional
	ResourceTags []GCPResourceTag `json:"resourceTags,omitempty"`
}

// GCPResourceLabel is a label to apply to GCP resources created for the cluster.
type GCPResourceLabel struct {
	// key is the key part of the label. A label key can have a maximum of 63 characters and cannot be empty.
	// Label must begin with a lowercase letter, and must contain only lowercase letters, numeric characters,
	// and the following special characters `_-`.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	// +kubebuilder:validation:Pattern=`^[a-z][0-9a-z_-]+$`
	Key string `json:"key"`

	// value is the value part of the label. A label value can have a maximum of 63 characters and cannot be empty.
	// Value must contain only lowercase letters, numeric characters, and the following special characters `_-`.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	// +kubebuilder:validation:Pattern=`^[0-9a-z_-]+$`
	Value string `json:"value"`
}

// GCPResourceTag is a tag to apply to GCP resources created for the cluster.
type GCPResourceTag struct {
	// parentID is the ID of the hierarchical resource where the tags are defined,
	// e.g. at the Organization or the Project level. To find the Organization or Project ID refer to the following pages:
	// https://cloud.google.com/resource-manager/docs/creating-managing-organization#retrieving_your_organization_id,
	// https://cloud.google.com/resource-manager/docs/creating-managing-projects#identifying_projects.
	// An OrganizationID must consist of decimal numbers, and cannot have leading zeroes.
	// A ProjectID must be 6 to 30 characters in length, can only contain lowercase letters, numbers,
	// and hyphens, and must start with a letter, and cannot end with a hyphen.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=32
	// +kubebuilder:validation:Pattern=`(^[1-9][0-9]{0,31}$)|(^[a-z][a-z0-9-]{4,28}[a-z0-9]$)`
	ParentID string `json:"parentID"`

	// key is the key part of the tag. A tag key can have a maximum of 63 characters and cannot be empty.
	// Tag key must begin and end with an alphanumeric character, and must contain only uppercase, lowercase
	// alphanumeric characters, and the following special characters `._-`.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	// +kubebuilder:validation:Pattern=`^[a-zA-Z0-9]([0-9A-Za-z_.-]{0,61}[a-zA-Z0-9])?$`
	Key string `json:"key"`

	// value is the value part of the tag. A tag value can have a maximum of 63 characters and cannot be empty.
	// Tag value must begin and end with an alphanumeric character, and must contain only uppercase, lowercase
	// alphanumeric characters, and the following special characters `_-.@%=+:,*#&(){}[]` and spaces.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	// +kubebuilder:validation:Pattern=`^[a-zA-Z0-9]([0-9A-Za-z_.@%=+:,*#&()\[\]{}\-\s]{0,61}[a-zA-Z0-9])?$`
	Value string `json:"value"`
}
```

### Implementation Details/Notes/Constraints [optional]
***GCP Labels*** 

Add a new field `resourceLabels` to `.status.platformStatus.gcp` of the
`infrastructure.config.openshift.io` type. Labels included in the `resourceLabels` field
will be applied to new resources created for the cluster by the in-cluster operators.

The `resourceLabels` field in `status.platformStatus.gcp` will be populated by the
installer using the entries from `platform.gcp.userLabels` field of `install-config`.

`status.platformStatus.gcp.resourceLabels` field of `infrastructure.config.openshift.io`
is immutable.

OpenShift operators(Cluster Infrastructure, Storage) that create GCP resources will apply
these labels to all GCP resources they create.

Below list of terraform GCP APIs to create resources should be updated to add user
defined labels and as well the OpenShift default label in the installer component.
`google_storage_bucket, google_compute_instance, google_dns_managed_zone, google_compute_image, google_compute_forwarding_rule`

And below list of terraform GCP APIs support labeling resources in beta version.
`google_compute_address`

Below list of GCP Resources does not support labeling.
`VPC Network, VM Instance Group, Subnetwork, Target Pool, DNS Zone, Records, Router, NAT Rules, Backend Service, Health Check`

API update example:
A local variable should be defined, which merges the default label and the user
defined GCP labels, which should be referred in the GCP resource APIs.
``` terraform
locals {
  labels = merge(
    {
      "kubernetes-io-cluster-${var.cluster_id}" = "owned"
    },
    var.gcp_extra_labels,
  )
}

resource "google_storage_bucket" "ignition" {
  ...
  labels = local.labels
}
```

For resources supporting labels in beta version below changes are required.
``` terraform
provider "googlebeta" {
  credentials = var.gcp_service_account
  project     = var.gcp_project_id
  region      = var.gcp_region
}

resource "google_compute_address" "cluster_ip" {
  provider = googlebeta
  ...
  labels = local.labels
}
```

The label of the form `kubernetes-io-cluster-<cluster_id>:owned` added by OpenShift, where
cluster_id is a string generated by concatenating user inputted cluster name and a random
string will be limited to a maximum length of 27 characters by trimming long cluster name
to 21 characters.

***GCP Tags*** 

Add a new field `resourceTags` to `.status.platformStatus.gcp` of the
`infrastructure.config.openshift.io` type. Tags included in the `resourceLabels` field
will be applied to new resources created for the cluster by the in-cluster operators.

The `resourceTags` field in `status.platformStatus.gcp` will be populated by the
installer using the entries from `platform.gcp.userTags` field of `install-config`.

`status.platformStatus.gcp.resourceTags` field of `infrastructure.config.openshift.io`
is immutable.

OpenShift operators(Cluster Infrastructure, Storage) that create GCP resources will apply
these tags to all GCP resources they create.

Resources which support tags and is required by OpenShift are Compute Engine Instances,
Cloud Storage Buckets.

Tag operations are restricted through quotas and the quota for tag write operation 
`TagsWriteRequestsPerMinutePerProject` has a default limit of 600requests/minute. Rate limit
mechanism must be implemented by the operators to restrict the number requests to 8 per second
and retry logic on exceeding the rate limit with initial backoff of 90 seconds, for maximum of
5 minutes with retry period factor of 2.

API update example:
Tag key and value names should be fetched using the short names defined by the
user, which will be used to the tag binding for a resource. Both terraform and
GO SDK for GCP does not provide a corresponding batch API and each tag should
be applied to each resource one at a time.
``` terraform
resource "google_tags_location_tag_binding" "vm" {
  parent = "//compute.googleapis.com/projects/${var.gcp_project_id}/zones/${var.gcp_master_availability_zones[0]}/instances/${var.cluster_id}-bootstrap"
  tag_value = data.google_tags_tag_value.ocp_tag_dev_value.id
  location = "${var.gcp_master_availability_zones[0]}"
}
```

```golang
func() {
	credPath := <GCP_CREDENTIALS_FILEPATH>
	project := <PROJECT_ID>
	location := <LOCATION>
	instance := <VM_INSTANCE_NAME>
	tagValueNamespacedName := <ORGANIZATION_ID>/<TAG_KEY_SHORT_NAME>/<TAG_VALUE_SHORT_NAME>
	googleAPIHTTPErrFieldName := "httpErr"
	googleAPIHTTPCodeFieldName := "Code"

	ctx := context.Background()
	opts := []option.ClientOption{
		option.WithCredentialsFile(credPath),
		option.WithEndpoint(fmt.Sprintf("https://%s-cloudresourcemanager.googleapis.com", location)),
	}
	client, err := rscmgr.NewTagBindingsRESTClient(ctx, opts...)
	if err != nil {
		log.Errorf("failed to create client for tag binding operation: %v", err)
		return err
	}
	defer client.Close()

	tagBindingReq := &rscmgrpb.CreateTagBindingRequest{
		TagBinding: &rscmgrpb.TagBinding{
			Parent: fmt.Sprintf("//compute.googleapis.com/projects/%s/zones/%s/instances/%s",
				project, location, instance),
			TagValueNamespacedName: tagValueNamespacedName,
		},
	}

	opts := []gax.CallOption{
		gax.WithRetry(func() gax.Retryer {
			return gax.OnHTTPCodes(gax.Backoff{
				Initial:    90 * time.Second,
				Max:        5 * time.Minute,
				Multiplier: 2,
			},
				http.StatusTooManyRequests)
		}),
	}
	result, err := client.CreateTagBinding(ctx, tagBindingReq, opts...)
	if err != nil {
		e, ok := err.(*apierror.APIError)
		if ok && reflect.Indirect(
		reflect.ValueOf(e).Elem().FieldByName(googleAPIHTTPErrFieldName),
		).FieldByName(googleAPIHTTPCodeFieldName).Int() == http.StatusConflict {
			log.Printf("tag binding %s already exist", tag)
		} else {
			log.Errorf("tag binding request failed: %v", err)
			return err
		}
	}

	resp, err := result.Wait(ctx)
	if err != nil {
		log.Errorf("wait on tag binding operation failed: %v", err)
		return err
	}

	log.Printf("tag binding successful: %s: %s", resp.GetName(), resp.String())
}
```

Adding labels and tags to GCP resources is controlled using FeatureGate `GCPLabelsTags` and should
be enabled to activate processing of user defined labels and tags.
```golang
func() {
	select {
	case <-featureGateAccessor.InitialFeatureGatesObserved():
		featureGates, err := featureGateAccessor.CurrentFeatureGates()
	case <-time.After(1 * time.Minute):
		log.Error(nil, "timed out waiting for FeatureGate detection")
		return nil, fmt.Errorf("timed out waiting for FeatureGate detection")
	}

	gcpLabelsTagsEnabled := featureGates.Enabled(configv1.FeatureGateGCPLabelsTags)

	if gcpLabelsTagsEnabled {
		// add user defined labels and tags
	}
}
```

#### Caveats
1. Updating or removing resource labels/tags added by OpenShift using an external interface,
   may or may not be reconciled by the operator managing the resource.
2. Updating labels/tags of individual resources is not supported and any label or tag present
   in `.status.platformStatus.gcp.resourceLabels` or `.status.platformStatus.gcp.resourceTags`
   respectively of `infrastructure.config.openshift.io/v1` resource will result in adding 
   labels and tags to all OpenShift managed GCP resources.
3. Limitations for TechPreview
   - Worker compute machines created by `machine-api-provider-gcp` controller will not be tagged,
     controller will be replaced with cluster-api-provider-gcp, and requires tagging
     support to be added in upstream.
   - Disk, Images, Snapshots created by `gcp-pd-csi-driver` will not be tagged with the userTags,
     requires tagging support to be added in upstream.
   - Filestore Instance resource created by `gcp-filestore-csi-driver` will not be tagged with
     the userTags, requires tagging support to be added in upstream.

### Risks and Mitigations
- Risks in using google-beta provider:

  Except for the additional features/functionality available for preview in the beta version,
  google-beta provider is not different from the GA version google provider. google-beta
  provider binary is embedded into installer as part of the installer build process and there
  are no additional requirements made on the end user to use the beta version.

  Below are some of the risks in using the google-beta provider.
  1. google-beta is a preview version and doesn't always carry the SLAs or technical
  support obligations.
  2. google-beta provider always uses the beta API regardless of whether the
  request contains any beta features.
  3. google_compute_address API provides just the `labels` functionality in the beta version,
  which is in beta since the initial [commit](https://github.com/hashicorp/terraform-provider-google-beta/commit/0fdc262183ed82e78824e42d05042a7b0aba7c8b)
  in 2018, though the usual beta phase for a feature is six months.

### Drawbacks
- User-defined labels cannot be updated on an GCP resource which is not managed by an
  operator. In this proposal, the changes proposed and developed will be part of
  openshift-* namespace. External operators are not in scope.
  User-defined labels can be updated on the following GCP resources.
  - Compute Engine Instance
  - Compute Address
  - Compute Image
  - Compute Forwarding Rule
  - Storage Bucket
  - DNS Zone

- GCP Labels can be used as queryable annotations for resources, but can't be used
  to set conditions on policies.

- User-defined tags cannot be updated on a GCP resource which is not managed by an
  operator. In this proposal, the changes proposed and developed will be part of
  openshift-* namespace. External operators are not in scope.
  User-defined tags can be updated on the following GCP resources.
  - Compute Engine Instances
  - Cloud Storage Buckets

- Administrator will have to manually perform below label pertaining actions
    1. removing the undesired labels/tags from the required resources.
    2. update label/tags values of the required resources.
    3. update labels/tags of the resources which are not managed by an operator.
    4. update labels/tags of the resources for which update logic is not supported
       by an operator.

## Design Details

### Open Questions
- GCP supports 50 tags per resource and since OpenShift do not add any tags of its own,
  users are allowed to define upto 50 tags. But the terraform and GO APIs for binding the
  tags to the resources doesn't support batch operation and requires each tag to be added
  individually to each resource which will increase the cluster creation time. What would
  be the optimal number of tags a user can define?

  **[Solution]** : User can configure upto 50 tags. GCP has limit of 600 requests per minute and
  the operators will make use of rate limiting mechanism to limit the number requests
  generated per second and retry logic when rate limit is exceeded.
  
- GCP client libraries support two ways of providing tag details
  1. NamespacedName: Tag value could be fed to APIs in the format
     `<ORGANIZATION_ID>/<TAG_KEY_SHORT_NAME>/<TAG_VALUE_SHORT_NAME>` where
      - ORGANIZATION_ID: Organization ID which is hosting the project under which OCP cluster
     will be created and tag key, value resources exist.
      - TAG_KEY_SHORT_NAME: Tag key short name is the display name provided during tag key
     resource creation.
      - TAG_VALUE_SHORT_NAME: Tag value short name is the display name provided during tag value
     resource creation.
  2. Name: Tag value could also be fed to the APIs in the format `tagValues/<TAG_VALUE_ID>` where
      - TAG_VALUE_ID: Tag value ID is the generated numeric ID for the tag value resource using which
     the tag key it is linked to can be identified.

  GCP terraform API `google_tags_location_tag_binding` expects the tag value's `Name` whereas the GO API
  `CreateTagBinding` accepts both tag value's `Name` and `NamespacedName`. The current approach for user
  to input `ORGANIZATION_ID, TAG_KEY_SHORT_NAME, TAG_VALUE_SHORT_NAME` which has more readability, will require
  1. For Terraform API([API doc](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/tags_tag_binding))
      - To fetch the tag key's `Name` using the `short_name`.
      - To fetch the tag value's `Name` using the `short_name` and the tag key's `Name`.
      - Use the tags value's `Name` to create tag binding to a resource using `google_tags_location_tag_binding` API.
  2. For GO API([API doc](https://github.com/googleapis/google-cloud-go/blob/resourcemanager/v1.8.2/resourcemanager/apiv3/tag_bindings_client.go#L174))
      - To derive the tag value's `NamespacedName` using the aforementioned user provided parameters.
      - Use the derived tag value's `NamespacedName` to create tag binding to a resource using `CreateTagBinding` API.

  Another approach is for user to configure just the list of tag value's names of the form `tagValues/<TAG_VALUE_ID>`,
  and it can be used directly with both terraform and GO APIs, and no additional processing on tags would be required.
  But the tag values configured are in obscured form and it will be diffcult for mapping the tags configured and the
  tags present on resources.

  Would it be better to ask user to configure just the tag value name, and not Organization ID, tag key short name, though
  it reduces the readability?

  **[Solution]** : Maintaining the uniformity with solution provided for AWS, Azure tags, user will have to configure tag key and value,
  and OrganizationID/ProjectID required for obtaining the TagValue `Name`.

### Test Plan
- Upgrade/downgrade testing
- Sufficient time for feedback
- Available by default
- Stress testing for scaling and label/tag update scenarios

### Graduation Criteria
#### Dev Preview -> Tech Preview
- Feature available for end-to-end usage.
- Complete end user documentation.
- UTs and e2e tests are present.
- Gather feedback from the users.

#### Tech Preview -> GA
N/A. This feature is for Tech Preview, until decided for GA.

#### Removing a deprecated feature

### Upgrade / Downgrade Strategy

On upgrade:
- Scenario 1: Upgrade to OpenShift version having support for adding labels.
  The new status field won't be populated since it is only populated by the
  installer and that can't have happened if the cluster was installed from a
  prior version. Components that consume the new field should take no action
  since there won't be any labels to apply.
- Scenario 2: Upgrade from OpenShift version having support for adding labels to higher:
  Cluster operators that add labels to GCP resources created for the cluster
  should refer to the label field and take action. For any new resource created
  post-upgrade, the operator managing the resource will add the user-defined labels
  to the resource. But the same does not apply to already existing resources,
  components may or may not update the resources with the user defined labels.

On downgrade:
- Scenario 1: Cluster installed with OpenShift version not having support for adding
  labels, upgraded to a version supporting labels and later downgraded to installed version.
  The new status field won't be populated since it is only populated by the
  installer and that can't have happened if the cluster was installed from a
  earlier version and upgraded to version having support for labels and downgrade will
  have no impact with the labels functionality too.
- Scenario 2: Cluster installed with OpenShift version having support for adding labels,
  upgraded to higher version and later downgraded to a lower version supporting labels.
  The status field may remain populated, components may or may not continue to labelTo keep consistency across
  newly created resources with the additional labels depending on whether given component
  sill has the logic to add labels post downgrade.

### Version Skew Strategy

### Operational Aspects of API Extensions

#### Failure Modes

#### Support Procedures

## Implementation History

## Alternatives

## Infrastructure Needed [optional]
