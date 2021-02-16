---
title: azure-support-known-cloud-environments
authors:
  - "@abhinavdahiya"
reviewers:
  - TBD
creation-date: 2020-05-11
last-updated: 2020-05-11
status: implementable
see-also:
  - "/enhancements/installer/aws-custom-region-and-endpoints.md"
  - https://issues.redhat.com/browse/CORS-1288
replaces:
superseded-by:
---

# Azure Support Known Cloud Environments

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [x] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

As an administrator, I would like to deploy OpenShift 4 clusters to
non-public Azure clouds. There are various known Azure clouds in
addition to the `AzurePublicCloud` available to users, including but
not limited to `AzureUSGovernmentCloud` etc. and requires the cluster
to use different Azure API endpoints. The user should be allowed to
provide the name of the Azure cloud and the installer and the cluster
operators should use the correct API endpoints for that provided
cloud.

## Motivation

### Goals

1. Install cluster to known Azure clouds as defined in list found [here](https://github.com/Azure/go-autorest/blob/9132adfc9db3007653dccb091b6a6c8e09b74ef5/autorest/azure/environments.go#L34-L39).

### Non-Goals

1. Supporting custom Azure clouds deployments that are not defined in Azure SDKs, i.e. custom BaseURI for endpoints will not supported.
2. Supporting self-signed server certificates for endpoints specifically.

## Proposal

The installer will allow the users to specify the name of the Azure
cloud using the install-config.yaml. The value will be restricted to
one of the known
[names](https://github.com/Azure/go-autorest/blob/9132adfc9db3007653dccb091b6a6c8e09b74ef5/autorest/azure/environments.go#L34-L39). When
no cloud is specified the installer will default to
`AzurePublicCloud`. The installer will use the endpoints pre-defined
in the Azure SDK for the specified clouds to communicate with Azure
APIs to perform validations, create machineset objects, and
configuring the terraform `azurerm` provider.

When using different Azure cloud, the cluster operators also need the discover the information, therefore, the installer will make these available using the config.openshift.io/v1 Infrastructure object. The cluster operators can then use Azure cloud name from the object to configure the Azure SDK to use the corresponding endpoints for communications.

### User Stories

#### US1: Installer cluster in Microsoft Azure for Government

To create clusters in special US regions for `Microsoft Azure for Government` the users would set the `cloudName` field in `platform.azure` to `AzureUSGovernmentCloud` in the install-config.yaml

### Implementation Details/Notes/Constraints

Previously we enabled picking the service endpoints for AWS platform [here](./aws-custom-region-and-endpoints.md) since AWS handles these special regions with service API endpoints for the services but Azure SDK allows users to use _cloud names_ to define these special partitions. Therefore, it makes sense to design out implementation so that the Azure users feel similarity with other tools.

#### Infrastructure global configuration

Since almost all of the cluster operators that communicate to the the Azure API need the use the Azure cloud provided by the user, the Infrastructure global configuration seems like the best place to store and discover this information.

The Infrastructure object contains user-editable platform configurable in `spec`, and discovery information that is not user-editable in `status`. The Azure cloud is a one-time setting that the user shouldn't be allowed to change and therefore, this information will only be allowed in the `status` section.

```go
// AzurePlatformStatus holds the current status of the Azure infrastructure provider.
type AzurePlatformStatus struct {
	// resourceGroupName is the Resource Group for new Azure resources created for the cluster.
	ResourceGroupName string `json:"resourceGroupName"`

	// networkResourceGroupName is the Resource Group for network resources like the Virtual Network and Subnets used by the cluster.
	// If empty, the value is same as ResourceGroupName.
	// +optional
	NetworkResourceGroupName string `json:"networkResourceGroupName,omitempty"`

	// cloudName is the name of the Azure cloud environment which can be used to configure the Azure SDK
	// with the appropriate Azure API endpoints.
	// If empty, consumers should default to AzurePublicCloud`.
	// +optional
	CloudName AzureCloudEnvironment `json:"cloudName,omitempty"`
}

// +kubebuilder:validation:Enum="";AzurePublicCloud;AzureUSGovernmentCloud;AzureChinaCloud;AzureGermanCloud
type AzureCloudEnvironment string

const (
	AzurePublicCloud AzureCloudEnvironment = "AzurePublicCloud"
	AzureUSGovernmentCloud AzureCloudEnvironment = "AzureUSGovernmentCloud"
	AzureChinaCloud AzureCloudEnvironment = "AzureChinaCloud"
	AzureGermanCloud AzureCloudEnvironment = "AzureGermanCloud"
)
```

#### Kube Cloud Config Controller

Since various kubernetes components like the kube-apiserver, kubelet
(machine-config-operator), kube-controller-manager,
cloud-controller-managers use the `.spec.cloudConfig` Config Map
reference for cloud provider specific configurations, a new controller
kube-cloud-config was introduced previously. The controller will
perform the task of stitching configuration from Infrastructure object
with the rest of the cloud config, such that all the kubernetes
components can continue to directly consume a Config Map for
configuration.

The controller will use the `cloudName` from the Infrastructure object to set the [`cloud`](https://github.com/kubernetes/kubernetes/blob/89ba90573f163ee3452b526f30348a035d54e870/staging/src/k8s.io/legacy-cloud-providers/azure/auth/azure_auth.go#L45-L46) field for the Azure cloud config.

If the user has already set a `cloud` value in the `.spec.cloudConfig` which doesn't match the value provided by the Infrastructure object, the controller should return error detailing the conflict in values.

#### Installer

##### Install Config

The install config should allow the user to set the cloud name for Azure.

```go
// types/pkg/azure/platform.go
type Platform struct {
    ...
	// cloudName is the name of the Azure cloud environment which can be used to configure the Azure SDK
	// with the appropiate Azure API endpoints.
	// If empty, the value is equal to `AzurePublicCloud`.
	// +optional
	CloudName AzureCloudEnvironment `json:"cloudName,omitempty"`
}

// +kubebuilder:validation:Enum="";AzurePublicCloud;AzureUSGovernmentCloud;AzureChinaCloud;AzureGermanCloud
type AzureCloudEnvironment string

const (
	PublicAzureCloud AzureCloudEnvironment = "AzurePublicCloud"
	USGovernmentAzureCloud AzureCloudEnvironment = "AzureUSGovernmentCloud"
	AzureChinaCloud AzureCloudEnvironment = "AzureChinaCloud"
	AzureGermanCloud AzureCloudEnvironment = "AzureGermanCloud"
)
```

##### Configuring the Azure SDK

The installer uses the Authorizers generated from [`newSessionFromFile`](https://github.com/openshift/installer/blob/a11cc4d29231135cd889487275ee32a20546b7e2/pkg/asset/installconfig/azure/session.go#L50) to create clients for Azure APIs. Updating the function to accept Azure cloud environment name should allow correct initiation of the clients.

##### Configuring the internal terraform-provider-azurerm with environment

The AzureRM provider accepts [`environment`](https://www.terraform.io/docs/providers/azurerm/index.html#environment) field to configure the Azure SDK. The valid values are `public`, `usgovernment`, `german`, and `china`, the installer should convert the install-config.yaml `AzureCloudEnvironment` string to a value acceptable by the provider.

##### Configuring infrastructure object

The installer should transform the install-config.yaml `AzureCloudEnvironment` string to set the `.status.platformStatus.azure.cloudName` in the Infrastructure object as detailed [above](#Infrastructure-global-configuration)

##### Destroy

The metadata.json needs to store the cloud name from the install-config.yaml so that the users can delete the clusters without any previous state.

#### Cluster Operators

Almost of the cluster operators will have to read the cloud name from the Infrastructure's object's status for Azure. The operators are not required to watch for changes but highly recommended. The operators can sync this value once on startup and expect that the value will not change later on.

The cluster operators using the Azure Go SDK can utilize [`EnvironmentFromName`](https://github.com/Azure/go-autorest/blob/9132adfc9db3007653dccb091b6a6c8e09b74ef5/autorest/azure/environments.go#L211-L230) to load the Azure cloud environment by passing the `.status.platformStatus.azure.cloudName` as string to the function.

The cluster operators must make sure to use `AzurePublicCloud` as value when `.status.platformStatus.azure.cloudName` is empty.

##### Machine API

The Azure machine controller should be configured to use Azure cloud environment name from the Infrastructure object. It makes sense to configure the controller itself as per machine configuration would not be ergonomic for two reasons:

1. The Machines / MachineSets are created by the installer or users once but new objects would require the user to specify these values again.

2. The requirement for clouds per Machine doesn't seem to provide major value.

### Risks and Mitigations

1. Picking correct instance type defaults for various clouds. The user UX when the installer fails to find default instance types for the a cloud is not great as it just errors out, but I think a little higher bar for non-public Azure cloud environments should not be too hard.

### Test Plan

TODO

### Graduation Criteria

None

### Upgrade / Downgrade Strategy

The changes to the API objects are backwards compatible and therefore there shouldn't be any specific concerns w.r.t to upgrades or downgrades.

### Version Skew Strategy

The changes to the API objects are backwards compatible and therefore there shouldn't be any specific concerns w.r.t to upgrades or downgrades.

## Implementation History

None

## Alternatives

We could treat all the non-public Azure cloud environments as Custom Azure cloud environment requiring the users to provide resource management endpoint. But that would not be great UX because,

- the Azure SDKs support these known environments natively and it will be a lot more easy for users to say `UsGovernmentCloud` instead of knowing the URL.
- Extra validations are required for URLs provided.
- The installer can control the known cloud names easily.
- The installer users the azurerm terraform provider that doesn't have support of custom resource endpoint URLs and would require use of the azurestack terraform provider. A change like that would be a lot more involved and therefore supporting known cloud names is a lot more easier to satisfy the goals.

## Infrastructure Needed

1. Access to MAG environment to test the changes.
