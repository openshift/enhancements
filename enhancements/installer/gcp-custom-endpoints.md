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

When custom endpoints are provided for GCP APIs, the cluster operators also need the discover the 
information, therefore, the installer will make these available using the `config.openshift.io/v1` `Infrastructure` object. 
The cluster operators can then use the custom endpoints from the object to configure the GCP SDK to use the 
corresponding endpoints for communications.

Similar to the [AWS Custom Endpoints](./aws-custom-region-and-endpoints.md) enhancement, this enhancement proposes using
the `.spec.cloudConfig` Config Map reference for cloud provider specific configurations. The same controller 
`cluster-kube-cloud-config-operator` will be used to perform the task of stitching the custom endpoints
with the rest of the cloud config, such that all the kubernetes components can continue to directly consume a
Config Map for configuration. This controller will also perform the specialized stitching on the bootstrap host for
control-plane kubelet and also actively reconcile the state in the cluster.

### User Stories

1. As a user I want to be able to use GCP Private API endpoints while deploying OpenShift, so I can be compliant with my company security policies.
2. As an administrator I want to be able to use GCP Private API endpoints while deploying OpenShift, so I can be compliant with my company security policies.

### Implementation Details/Notes/Constraints

See below

### Workflow Description

#### Install Config

The user can provide a list of API endpoints for various services.

```go
// ServiceEndpoint store the configuration of a url to
// override existing defaults of GCP Services.
type ServiceEndpoint struct {
    Name string `json:"name"`

    // This must be a HTTPS URL
    URL     string `json:"url"`
}
```

```go
type Platform struct {
    ...
    // ServiceEndpoints list contains custom endpoints which will override default
    // service endpoint of GCP Services.
    // There must be only one ServiceEndpoint for a service.
    // +optional
    ServiceEndpoints []ServiceEndpoint `json:"serviceEndpoints,omitempty"`
    ...
}
```

The yaml representation by the user would look like:

```yaml
platform:
  gcp:
    serviceEndpoints:
    - name: compute
      url: https://compute.custom.url
    - name: cloud storage
      url: https://cloud.custom.url
```

#### Validations for service endpoints

1. The installer must ensure that only one override for service is specified by the user.

2. The URL for the service endpoint must be `https` and the host should trust the certificate.

##### Optional Validation for service endpoints

1. The installer should ensure that the service endpoint is at least reachable from the host.

#### Configuring the cluster api GCP with service overrides

The [GCP Cluster Spec Structure](https://github.com/kubernetes-sigs/cluster-api-provider-gcp/blob/main/api/v1beta1/gcpcluster_types.go#L31) should be
edited to include a map.

```go
// ServiceEndpoints contains the endpoint overrides when the user would like to use custom enpoints for GCP APIs.
// +optional
ServiceEndpoints map[string]string `json:"serviceEndpoints,omitempty"`
```

The keys for the map are the service names, and only specific service names should be accepted. Currently the supported
GCP services include:

- compute
- iam

The cluster API GCP Provider creates the all the clients for each service in a [common location](https://github.com/kubernetes-sigs/cluster-api-provider-gcp/blob/main/cloud/scope/clients.go).
To ensure that the APIs use the custom endpoints, the endpoints are passed to the functions that create the clients for the
services.

```go
	opts = append(opts, option.WithEndpoint(endpoint))
	client, err := service.NewClient(ctx, opts...)
	if err != nil {
		return nil, errors.Errorf("failed to create gcp example client: %v", err)
	}
```

There is currently an [open CAPG issue](https://github.com/kubernetes-sigs/cluster-api-provider-gcp/issues/1391) to address these required
API changes for CAPG.

#### Destroying clusters

The `metadata.json` needs to store the custom endpoints from the `install-config.yaml` so that the users can delete the clusters without any previous state.

## Operational Aspects of API Extensions

### API Extensions

Since almost of the cluster operators that communicate to the GCP API need the use the API endpoints provided by 
the user, the `Infrastructure` global configuration seems like the best place to store and discover this information.

And in contrast to the current configuration in the `Infrastructure` object like `region` is not editable as day-2
operation, the users should be able to edit the GCP endpoints day-2, therefore this configuration best belongs in
the `spec` section.

```go
// InfrastructureSpec contains settings that apply to the cluster infrastructure.
type InfrastructureSpec struct {
  // platformSpec holds configuration specific to the underlying
  // infrastructure provider.
  // +optional
  PlatformSpec *PlatformSpec `json:"platformSpec,omitempty"`
}

// PlatformSpec holds some configuration to the underlying infrastructure provider
// of the current cluster. It is supposed that only one of the spec structs is set.
type PlatformSpec struct {
  Type PlatformType `json:"type"`

  // GCP contains settings specific to the Amazon Web Services infrastructure provider.
  // +optional
  GCP *GCPPlatformSpec `json:"gcp,omitempty"`

  ...
}
```

```go
// GCPPlatformSpec holds the current status of the GCP infrastructure provider.
type GCPPlatformSpec struct {
  // ServiceEndpoints list contains custom endpoints which will override default
  // service endpoint of GCP Services.
  // There must be only one ServiceEndpoint for a service.
  // +optional
  ServiceEndpoints []GCPServiceEndpoint `json:"serviceEndpoints,omitempty"`
}

// GCPServiceEndpoint store the configuration of a custom url to
// override existing defaults of GCP Services.
// See the full list of supported GCP service endpoints here: https://developers.google.com/apis-explorer
type GCPServiceEndpoint struct {
  Name string `json:"name"`

  // This must be a HTTPS URL
  URL     string `json:"url"`
}
```

The users are going to be specifying the service endpoints for APIs, there is chance of user error and operators
picking up invalid or incorrect information. Therefore, the service endpoints will be mirrored to the `status` section
after basic validations by a controller and the cluster operators will use the information from the `status` section.

```go
// GCPPlatformStatus holds the current status of the GCP Services infrastructure provider.
type GCPPlatformStatus struct {
  // ServiceEndpoints list contains custom endpoints which will override default
  // service endpoint of GCP Services.
  // There must be only one ServiceEndpoint for a service.
  // +optional
  ServiceEndpoints []GCPServiceEndpoint `json:"serviceEndpoints,omitempty"`
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

### Cluster Kube Cloud Config Operator

Since various kubernetes components like the kube-apiserver, kubelet
(machine-config-operator), kube-controller-manager,
cloud-controller-managers use the `.spec.cloudConfig` Config Map
reference from the global `Infrastructure` configuration for cloud
provider specific configurations. Using the controller
`cluster-kube-cloud-config-operator` allows performing the task of
stitching the custom endpoints with the rest of the cloud config, such
that all the kubernetes components can continue to directly consume a
Config Map for configuration.

#### Boostrap host control flow

The controller reads the on disk `Infrastructure` object, and the cloud config Config Map from disk to

1. Create a new cloud config Config Map, stitching the existing cloud config and service endpoints for GCP.

2. Writing that Config Map to disk for use by other operators and also push to the cluster

The bootstrap control flow should not modify the existing `Infrastructure` object on the disk.

#### Infrastructure global configuration updates for cloud configurations

```go
// InfrastructureSpec contains settings that apply to the cluster infrastructure.
type InfrastructureSpec struct {
  // cloudConfig is a reference to a ConfigMap containing the cloud provider configuration file.
  // This configuration file is used to configure the Kubernetes cloud provider integration
  // when using the built-in cloud provider integration or the external cloud controller manager.
  // The namespace for this config map is openshift-config.
  // +optional
  CloudConfig ConfigMapFileReference `json:"cloudConfig"`
}
```

```go
// InfrastructureStatus describes the infrastructure the cluster is leveraging.
type InfrastructureStatus struct {
  // cloudConfig is a reference to a ConfigMap containing the cloud provider configuration file.
  // This configuration file is used to configure the Kubernetes cloud provider integration
  // when using the built-in cloud provider integration or the external cloud controller manager.
  // The namespace for this config map is openshift-config-managed.
  // +optional
  CloudConfig ConfigMapFileReference `json:"cloudConfig"`
}
```

#### Validations

1. Fail out with an error, when the user has set the service endpoints in the cloud config.
   As the service endpoints are already controlled by other fields in the `Infrastructure` object, trying to merge
   2 sources of information would be erroneous.

2. Ensure that service endpoints are valid and reachable by cluster components.

#### New Operator Alternatives

1. Previous versions recommended that the kube-controller-manager and machine-config-operator perform the
   stitching for cloud config. The kube-controller-manager-operator owners do not want to understand or handle
   the vagaries of various cloud providers. The operator allows those components to use the Config Map
   as-is. See [comment](https://github.com/openshift/enhancements/pull/163#discussion_r359962825)

2. The operator could also be named Infrastructure Config Operator and the Kube Cloud Config controller could 
   become a sub controller. The operator could perform other functions related to the Infrastructure object and
   cloud providers.

### Cluster Operators

Almost all the cluster operators will have to read the api endpoints from the `infrastructures.config.openshift.io`
`cluster` object's `status` for GCP and make sure the operators are watching for changes.

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

#### Cluster API Provider GCP

The GCP Cluster spec should be configured to use the service endpoint overrides from the `Infrastructure` object.
It makes sense to configure the controller itself because per machine configuration for API endpoints would not be
ergonomic for two reasons:

1. The Machines / MachineSets are created by the installer or users once, while the service endpoints can be updated by
   users later and then the Machine objects will have to be updated out of band to reflect the change.

2. The requirement for different service endpoint per Machine doesn't seem to provide major value.

## Test Plan

The Openshift Installer Team and the QE team will work together to make a permanent set of custom APIs that are
accessible in one of the CI projects for GCP. The custom endpoints will be used for new CI jobs.

## Graduation Criteria

Feature is complete and the Jobs in the test plan are consistently passing.

### Dev Preview -> Tech Preview

The Openshift API will introduce the Dev and Tech Preview tags to protect the release from the additions of the feature.

### Tech Preview -> GA

The Openshift Installer and Openshift API will drop the tags when the feature is considered GA. 

### Removing a deprecated feature

N/A

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

The feature will be introduced as `techPreview`. After the feature is considered complete, the `techPreview` tag is removed. This may or
may not span across multiple releases.

## Alternatives

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

N/A

#### Standalone Clusters

N/A

#### Single-node Deployments or MicroShift

N/A

### Risks and Mitigations

N/A


