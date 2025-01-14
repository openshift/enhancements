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

1. Allow users to specify the name of a private service connect endpoint that will be used to create endpoints to the GCP APIs.

### Non-Goals

1. There are no plans to allow self-signed server certificates for endpoints specifically.
2. Handle overrides to OAUTH or STS GCP Services. These services do not have private service connect endpoint override options (contacted Google to verify).

## Proposal

To support custom endpoints for GCP APIs, the install-config would
allow the users to provide the name of a private service connect endpoint. The installer
will create endpoints to all the GCP APIs from the PSC endpoint name.
The installer will also create a second private DNS zone that will map the overridden enpoints
to the basic googleapis domain endpoints.

### User Stories

1. As a user I want to be able to use GCP Private API endpoints while deploying OpenShift, so I can be compliant with my company security policies.
2. As an administrator I want to be able to use GCP Private API endpoints while deploying OpenShift, so I can be compliant with my company security policies.

### Implementation Details/Notes/Constraints

See below

### Workflow Description

#### Install Config

The user can provide the name of a private service connect endpoint.

```go
// PSCEndpoint contains the information to describe a Private Service Connect
// endpoint.
type PSCEndpoint struct {
    // Name contains the name of the private service connect endpoint.
    Name string `json:"name"`
    // Region is the region where the endpoint resides.
    // When the region is empty, the location is assumed to be global.
    // +optional
    Region string `json:"region,omitempty"`

    // ClusterUseOnly should be set to true when the installer should use
    // the public api endpoints and all cluster operators should use the
    // api endpoint overrides. The value should be false when the installer
    // and cluster operators should use the api endpoint overrides.
    // +optional
    ClusterUseOnly bool `json:"clusterUseOnly,omitempty"`
}
```

The yaml representation by the user would look like:

```yaml
platform:
  gcp:
     endpoint: example
```

#### Validations for service endpoints

1. The installer will validate that the endpoint exists. When no region is provided, it is assumed to be a global endpoint.

#### Configuring the cluster api GCP with service overrides

The [GCP Cluster Spec Structure](https://github.com/kubernetes-sigs/cluster-api-provider-gcp/blob/main/api/v1beta1/gcpcluster_types.go#L31) should be edited to include a structure that contains the [service endpoints](https://github.com/kubernetes-sigs/cluster-api-provider-gcp/blob/main/api/v1beta1/endpoints.go#L21). The OpenShift Installer
will create an endpoint url for each API endpoint that CAPG will use. The required endpoints are added to the struct. The Cluster API GCP Provider creates the all the clients for each service in a [common location](https://github.com/kubernetes-sigs/cluster-api-provider-gcp/blob/main/cloud/scope/clients.go).
To ensure that the APIs use the custom endpoints, the endpoints are passed to the functions that create the clients for the services.

```go
	opts = append(opts, option.WithEndpoint(endpoint))
	client, err := service.NewClient(ctx, opts...)
	if err != nil {
		return nil, errors.Errorf("failed to create gcp example client: %v", err)
	}
```

All of the required changes for custom endpoint support in CAPG can be found in [PR 1391](https://github.com/kubernetes-sigs/cluster-api-provider-gcp/issues/1391).

#### Destroying clusters

The `metadata.json` needs to store the Private Service Connect endpoint information from the `install-config.yaml` so the users can delete the clusters without any previous state. There are no workarounds or alternative locations to store this information, so HIVE and the Installer team must be aware of these impacts to the metadata changes.

## Operational Aspects of API Extensions

### API Extensions

There are no API extensions necessary with the current configuration. 

The current solution utilizes a new private hosted zone that will forward any of the requests to the
API endpoints to the correct location (the overridden private endpoints).

If the solution involved setting the api endpoint override in each application, then changes to the openshift/api
would be necessary. The changes would involve adding endpoint overrides for each service (i.e. compute) that would be
set in the infrastructure object. These api endpoint overrides would be picked up by each application that needs the
information where it would be set during GCP service creation.

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


#### Boostrap host control flow

No changes are necessary.

#### Infrastructure global configuration updates for cloud configurations

No changes are necessary.

#### Validations

1. Ensure that private service connect endpoint is valid and reachable with a valid ip address. This is an indirect validation as all endpoints will be used for validations of other values.

### Cluster Operators

The CCM is the only cluster operator that will require API endpoint overrides. The CCM already contains fields for these
overrides. The OpenShift Installer will create the manifest that will set these fields. the fields are `APIEndpoint` and 
`ContainerAPIEndpoint`, and these correspond to the `compute` and `container` services respectively. The CCM requires the
api endpoint overrides, because CCM processes information while the OpenShift Installer is still executing and the new
private zone has not been established in time for the CCM to make use of its functionality.

```
var configTmpl = `[global]
...
{{- if ne .Global.APIEndpoint "" }}{{"\n"}}api-endpoint = {{.Global.APIEndpoint}}{{ end }}
{{- if ne .Global.ContainerAPIEndpoint "" }}{{"\n"}}container-api-endpoint = {{.Global.ContainerAPIEndpoint}}{{ end }}

`
```

#### Configuring the GCP SDK for controllers

There is an option to override the API using a custom endpoint for GCP.

```go
	opts = append(opts, option.WithEndpoint(endpoint))
	client, err := service.NewClient(ctx, opts...)
	if err != nil {
		return nil, errors.Errorf("failed to create gcp example client: %v", err)
	}
```

## Test Plan

The Openshift Installer Team and the QE team have created a test that dynamically creates the Private Service Connect Endpoint.

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

### API Additions

The alternative begins by adding the fields to the OpenShift API. This method includes a fields in the `PlatformStatus`
that contains an endpoint overrides for all the GCP Services.

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

### Installer

The OpenShift Installer would process the information below, verify/validate the provided information, and create manifests
to provide this information to cluster components through the infrastructure object.

#### Install Config

The install configuration file would contain a list of Service Endpoints (above) that the user could provide when
they need these api endpoints to be overridden.

### Cluster Components

Each cluster component that creates a GCP Service (i.e. Compute Service Client) would read the infrastructure object
and determine if the endpoints are overridden or not. When the endpoints should be overridden, the client creation in
the cluster component should include the api endpoint. 

```go
	opts = append(opts, option.WithEndpoint(endpoint))
	client, err := service.NewClient(ctx, opts...)
	if err != nil {
		return nil, errors.Errorf("failed to create gcp example client: %v", err)
	}
```

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

HIVE uses the `metadata.json` generated by the Openshift Installer. The endpoint information is required to be added to the `metadata.json` file, because the Installer requires this information for cluster destruction. Changes to the `metadata.json` file are treated as breaking API changes, and the HIVE team must be aware of these breaking changes.

#### Standalone Clusters

Not under consideration

#### Single-node Deployments or MicroShift

Not under consideration

### Risks and Mitigations

Not under consideration
