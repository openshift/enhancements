---
title: gcp-custom-endpoints
authors:
  - "@barbacbd"
reviewers:
  - "@patrickdillon"
approvers:
  - "@zaneb"
  - "@patrickdillon"
api-approvers:
   - "@JoelSpeed"
creation-date: 2025-01-14
last-updated: 2025-01-14
tracking-link:
   - https://issues.redhat.com/browse/CORS-2389
---

# GCP Custom API Endpoint Support

## Release Signoff Checklist

- [ ]  Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Open Questions

## Summary

For users with strict regulatory policies, Private Service Connect allows private consumption of services across
VPC networks that belong to different groups, teams, projects, or organizations. Supporting OpenShift to consume
these private endpoints is key for these customers to be able to deploy the platform on GCP and be compliant with
their regulatory policies.

As an administrator, I would like to deploy OpenShift 4 clusters to GCP, utilizing custom GCP API
endpoints (private and restricted).

## Motivation

### Goals

1. Allow users to specify API endpoints for some or all required GCP services.

### Non-Goals

1. There are no plans to allow self-signed server certificates for endpoints specifically.

## Proposal

To support custom endpoints for GCP APIs, the install-config would
allow the users to provide a list of endpoints for various
services. When the custom endpoints are set, the installer will
validate that the endpoints are reachable. The endpoints will be used
by the installer to call GCP APIs for performing various actions like
validations, creating the MachineSet objects, and also configuring the
cluster api GCP provider.

When custom endpoints are provided for GCP APIs, the cluster operators also need to discover the 
information, therefore, the installer will make these available using the `config.openshift.io/v1` `Infrastructure` object. 
The cluster operators will use the custom endpoints from the object to configure the GCP SDK accessing the 
corresponding endpoints for communications (rather than the default endpoints).

### User Stories

1. As a user I want to be able to use GCP Private API endpoints while deploying OpenShift, so I can be compliant with my company security policies.
2. As an administrator I want to be able to use GCP Private API endpoints while deploying OpenShift, so I can be compliant with my company security policies.

### Implementation Details/Notes/Constraints

See below

### Workflow Description

#### Install Config

The user can provide a list of API endpoints for various services.

```go
type Platform struct {
   ...
   // ServiceEndpoints list contains custom endpoints which will override default
	// service endpoint of GCP Services.
   // There must be only one ServiceEndpoint for a service.
   // +optional
   ServiceEndpoints []configv1.GCPServiceEndpoint `json:"serviceEndpoints,omitempty"`
   ...
}
```

The yaml representation by the user would look like:

```yaml
platform:
  gcp:
    serviceEndpoints:
    - name: Compute
      url: https://compute-exampleendpoint.p.googleapis.com
    - name: IAM
      url: https://iam-exampleendpoint.p.googleapis.com
```

#### Validations for service endpoints

1. The installer must ensure that only one override per service is specified by the user.

2. The URL for the service endpoint must be `https`.

#### Configuring the cluster api GCP with service overrides

The [GCP Cluster Spec Structure](https://github.com/kubernetes-sigs/cluster-api-provider-gcp/blob/main/api/v1beta1/gcpcluster_types.go#L31) should be
edited to include a structure that contains the [service endpoints](https://github.com/kubernetes-sigs/cluster-api-provider-gcp/blob/main/api/v1beta1/endpoints.go#L21). The Cluster API GCP Provider does not require the full set of endpoints and it cannot import the openshfit/api. The required endpoints are added to the struct, and should only be overridden when they differ from the base endpoint. The Cluster API GCP Provider creates the all the clients for each service in a [common location](https://github.com/kubernetes-sigs/cluster-api-provider-gcp/blob/main/cloud/scope/clients.go).
To ensure that the APIs use the custom endpoints, the endpoints are passed to the functions that create the clients for the
services.

```go
	opts = append(opts, option.WithEndpoint(endpoint))
	client, err := service.NewClient(ctx, opts...)
	if err != nil {
		return nil, errors.Errorf("failed to create gcp example client: %v", err)
	}
```

All of the required changes for custom endpoint support in CAPG can be found in [PR 1391](https://github.com/kubernetes-sigs/cluster-api-provider-gcp/issues/1391).

#### Destroying clusters

The `metadata.json` needs to store the custom endpoints from the `install-config.yaml` so the users can delete the clusters without any previous state. There are no workarounds or alternative locations to store this information, so HIVE and the Installer team must be aware of these impacts to the metadata changes.

## Operational Aspects of API Extensions

### API Extensions

All the cluster operators that communicate to the GCP API(s) need the use the API endpoints provided by 
the user, so the `Infrastructure` global configuration seems like the best place to store and discover this information.

The users should not be able to edit the GCP endpoints day-2, therefore this configuration best belongs in
the `status` section and _not_ the `spec`.

```go
// InfrastructureStatus describes the infrastructure the cluster is leveraging.
type InfrastructureStatus struct {
   // platformStatus holds status information specific to the underlying
   // infrastructure provider.
   // +optional
   PlatformStatus *PlatformStatus `json:"platformStatus,omitempty"`
}

// PlatformStatus holds the current status specific to the underlying infrastructure provider
// of the current cluster. Since these are used at status-level for the underlying cluster, it
// is supposed that only one of the status structs is set.
type PlatformStatus struct {
   // gcp contains settings specific to the Google Cloud Platform infrastructure provider.
   // +optional
   GCP *GCPPlatformStatus `json:"gcp,omitempty"`

  ...
}
```

```go
// GCPPlatformStatus holds the current status of the Google Cloud Platform infrastructure provider.
type GCPPlatformStatus struct {
	// serviceEndpoints specifies endpoints that override the default endpoints
	// used when creating clients to interact with GCP services.
	// When not specified, the default endpoint for the GCP region will be used.
	// Only 1 endpoint override is permitted for each GCP service.
	// The maximum number of endpoint overrides allowed is 11.
	// +listType=map
	// +listMapKey=name
	// +kubebuilder:validation:MaxItems=11
	// +kubebuilder:validation:MaxItems=9
	// +kubebuilder:validation:XValidation:rule="self.all(x, self.exists_one(y, x.name == y.name))",message="only 1 endpoint override is permitted per GCP service name"
	// +optional
	// +openshift:enable:FeatureGate=GCPCustomAPIEndpointsInstall
	ServiceEndpoints []GCPServiceEndpoint `json:"serviceEndpoints,omitempty"`
}


// GCPServiceEndpoint store the configuration of a custom url to
// override existing defaults of GCP Services.
type GCPServiceEndpoint struct {
  // name is the name of the GCP service whose endpoint is being overridden.
  // This must be provided and cannot be empty.
  //
  // Allowed values are Compute, Container, CloudResourceManager, DNS, File, IAM, ServiceUsage,
  // Storage, and TagManager.
  //
  // As an example, when setting the name to Compute all requests made by the caller to the GCP Compute
  // Service will be directed to the endpoint specified in the url field.
  //
  // +required
  Name GCPServiceEndpointName `json:"name"`

  // url is a fully qualified URI that overrides the default endpoint for a client using the GCP service specified
  // in the name field.
  // url is required, must use the scheme https, must not be more than 253 characters in length,
  // and must be a valid URL according to Go's net/url package (https://pkg.go.dev/net/url#URL)
  //
  // An example of a valid endpoint that overrides the Compute Service: "https://compute-myendpoint1.p.googleapis.com"
  //
  // +required
  // +kubebuilder:validation:MaxLength=253
  // +kubebuilder:validation:XValidation:rule="isURL(self)",message="must be a valid URL"
  // +kubebuilder:validation:XValidation:rule="isURL(self) ? (url(self).getScheme() == \"https\") : true",message="scheme must be https"
  // +kubebuilder:validation:XValidation:rule="url(self).getEscapedPath() == \"\" || url(self).getEscapedPath() == \"/\""
  URL string `json:"url"`
}
```

#### Global configuration Alternatives

1. Create another global configuration for Cloud API endpoints that stores information like the endpoints themselves, trusted bundles etc.

   Infrastructure global configuration already performs the function
   of tracking infrastructure related configuration and another global
   configuration that stores a part of the information doesn't seem
   like a great option. But it might allow validations and status
   observation by an independent controller.
   
2. Configure each individual cluster operator

   There are five cluster operators that would need to be configured
   namely, cluster-kube-controller-manager, cluster-ingress-operator,
   cluster-machine-api-operator, cluster-image-registry-operator,
   cluster-credential-operator. There might be more operators like
   cluster-network-operator that might require access to the GCP APIs
   in the future to control the security group rules. Also, various OLM
   operators that interact with GCP APIs will need their own
   configuration. Configuring all these separately is not a great UX
   for installer and a user who wants to modify the cluster to use API
   endpoints as day-2 operation.

3. Use a separate hosted zone.

   The new private hosted zone would forward any of the requests to the
   API endpoints to the correct location (the overridden private endpoints).
   Private Service Connect offers limitted functionality in this area, but
   it does not provide information on specific version such as `alpha` and
   `beta` versions required by the GCP Provider.


#### Boostrap host control flow

The controller reads the on disk `Infrastructure` object, and the cloud config Config Map from disk to

1. Create a new cloud config Config Map, stitching the existing cloud config and service endpoints for GCP.

2. Writing that Config Map to disk for use by other operators and also push to the cluster

The bootstrap control flow should not modify the existing `Infrastructure` object on the disk.

#### Infrastructure global configuration updates for cloud configurations

```go
// InfrastructureStatus describes the infrastructure the cluster is leveraging.
type InfrastructureStatus struct {
// platformStatus holds status information specific to the underlying
// infrastructure provider.
// +optional
PlatformStatus *PlatformStatus `json:"platformStatus,omitempty"`
}
```

#### Validations

1. Fail out with an error, when the user has set the service endpoints in the cloud config.
   As the service endpoints are already controlled by other fields in the `Infrastructure` object, trying to merge 2 sources of information would be erroneous.

2. Ensure that service endpoints are valid and reachable by cluster components. This is an indirect validation as all endpoints will be used for validations of other values. For instance, the endpoints are set prior to validating dns hosted zones in the openshift installer.

### Cluster Operators

Almost all the cluster operators will have to read the api endpoints from the `infrastructures.config.openshift.io`
`cluster` object's `status`. Changes are not allowed for GCP service endpoint overrides, so there is no need to watch
for changes to the `spec`.

#### Configuring the GCP SDK for controllers

There is an option to override the API using a custom endpoint for GCP.

```go
	opts = append(opts, option.WithEndpoint(endpoint))
	client, err := service.NewClient(ctx, opts...)
	if err != nil {
		return nil, errors.Errorf("failed to create gcp example client: %v", err)
	}
```

If the `ServiceEndpoints` contain the name and url for a supported GCP API, the endpoint is overwritten using the method above.

#### Machine API

The GCP machine controller should be configured to use the service endpoint overrides from the `Infrastructure` object.
It makes sense to configure the controller itself because per machine configuration for API endpoints would not be
ergonomic for two reasons:

1. The Machines / MachineSets are created by the installer or users once, while the service endpoints can be updated by
   users later and then the Machine objects will have to be updated out of band to reflect the change.

2. The requirement for different service endpoint per Machine doesn't seem to provide major value.

#### Cloud Credential Operator

The Cloud Credential Operator (CCO) should be configured to use the service endpoint overrides from the `Infrastructure` object. The CCO requires the Compute and IAM endpoint overrides.

##### CCOCTL

The CCOCTL requires the user to manually enter endpoint overrides as command line arguments. 

#### Cluster Image Registry Operator

The Cluster Image Registry Operator should be configured to use the service endpoint overrides from the `Infrastructure` object.

#### Ingress Operator

The Ingress Operator should be configured to use the service endpoint overrides from the `Infrastructure` object.

#### Cluster Network Operator

The Cluster Network Operator should be configured to use the service endpoint overrides from the `Infrastructure` object.

#### GCP PD CSI Driver Operator

The GCP PD CSI Driver Operator should be configured to use the service endpoint overrides from the `Infrastructure` object.

#### Cluster API Provider GCP

The GCP Cluster status should be configured to use the service endpoint overrides from the `Infrastructure` object.
It makes sense to configure the controller itself because per machine configuration for API endpoints would not be
ergonomic for two reasons:

1. The Machines / MachineSets are created by the installer or users once, while the service endpoints can be updated by
   users later and then the Machine objects will have to be updated out of band to reflect the change.

2. The requirement for different service endpoint per Machine doesn't seem to provide major value.

## Test Plan

The Openshift Installer Team and the QE team have created a test that dynamically creates the Private Service Connect Endpoint. The endpoint is used to create the urls for each service endpoint override that is added to the install-config file.

## Graduation Criteria

Feature is complete and the Jobs in the test plan are consistently passing. This should include that no logs show any traffic going to the Google Cloud Default Endpoints.

### Dev Preview -> Tech Preview

The Openshift API will introduce the Dev and Tech Preview tags to protect the release from the additions of the feature.

### Tech Preview -> GA

The Openshift Installer and Openshift API will drop the tags when the feature is considered GA. 

### Removing a deprecated feature

Not under consideration.

## Upgrade / Downgrade Strategy

### Downgrades

The controller for cloud config reconciliation will be added on upgrade to clusters, and if the users try to downgrade
to previous version, cluster version operator will leave the controller running.

As part of the controller, the deliverable will also include upstream docs that details steps for user to take to remove
the new controller like

1. oc delete the namespace
2. remove any cluster resources like clusterrolebindings

Any changes made to the `Infrastructure` object will be dropped as the CRD sets preserveUnknownFields to false.

## Version Skew Strategy

The feature will be introduced as `techPreview`. After the feature is considered complete, the `techPreview` tag is removed. This may or may not span across multiple releases.

## Alternatives (Not Implemented)

### Install Config

Begin by adding a struct to hold the Endpoint information in the GCP Platform

```go
// ServiceEndpoints contains the url endpoints for overriding GCP APIs. An unset field indicates that the
// default API endpoint will be used.
// +optional
ServiceEndpoints CustomServiceEndpoints `json:"serviceEndpoints,omitempty"`
```

Instead of using a map, the fields could be explicit. In the struct above, each service could have an associated string field
for the endpoint. The default for each is an empty string which is considered to be not overwritten.

```go
// CustomServiceEndpoints contains all the custom endpoints that the user may override. Each field corresponds to
// a service where the expected value is the url that is used to override the default API endpoint.
type CustomServiceEndpoints struct {

	// CloudResourceManagerServiceEndpoint is the custom endpoint url for the Cloud Resource Manager Service
	CloudResourceManagerServiceEndpoint string `json:"cloudResourceManager,omitempty"`

	// ComputeServiceEndpoint is the custom endpoint url for the Compute Service
	ComputeServiceEndpoint string `json:"compute,omitempty"`

	// DNSServiceEndpoint is the custom endpoint url for the DNS Service
	DNSServiceEndpoint string `json:"dns,omitempty"`

	// FileServiceEndpoint is the custom endpoint url for the File Service
	FileServiceEndpoint string `json:"file,omitempty"`

	// IAMServiceEndpoint is the custom endpoint url for the IAM Service
	IAMServiceEndpoint string `json:"iam,omitempty"`

	// ServiceUsageServiceEndpoint is the custom endpoint url for the Service Usage Service
	ServiceUsageServiceEndpoint string `json:"serviceUsage,omitempty"`

	// StorageServiceEndpoint is the custom endpoint url for the Storage Service
	StorageServiceEndpoint string `json:"storage,omitempty"`
}
```

The approach allows users to override the endpoints without worrying about changing map keys. 

**This would require API changes each time that an endpoint is added.** Unless the installer decides to handle taking the above
struct and turning it into the list or map that the openshift API structure contains.

## Support Procedures

The Openshift Installer team is the main focus for this enhancement.

### Implementation History

None

### Drawbacks

Already covered inline.

### Infrastructure Needed

1. Networking environment to test custom API endpoints.

### Topology Considerations

#### Hypershift / Hosted Control Planes

Not under consideration

#### HIVE

HIVE uses the `metadata.json` generated by the Openshift Installer. The endpoints are required to be added to the `metadata.json` file, because the Installer requires this information for cluster destruction. Changes to the `metadata.json` file are treated like breaking API changes, and the HIVE team must be aware of these breaking changes.

#### Standalone Clusters

Not under consideration

#### Single-node Deployments or MicroShift

Not under consideration

### Risks and Mitigations

Not under consideration
