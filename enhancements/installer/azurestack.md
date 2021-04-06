---
title: azurestack
authors:
  - "@patrickdillon"
reviewers:
  - "@staebler"
  - "@jhixson74"
approvers:
  - "@staebler"
creation-date: 2021-03-10
last-updated: 2021-03-10
status: implementable
see-also:
  - "/enhancements/installer/azure-support-known-cloud-environments.md"
---

## Release Signoff Checklist

- [X] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

This enhancement covers adding support to install OpenShift clusters to Azure Stack Hub (ASH).
Azure Stack Hub looks and feels like standard Azure, but differs significantly in terms of
implemenetation and technical details. Azure Stack Hub shares characteristics from [Azure cloud
environments](azure-support-known-cloud-environments.md) and on-prem platforms. This enhancement
covers details for the Installer to create infrastructure and run an OpenShift cluster on ASH.

## Motivation

Currently, Azure Stack Hub is not supported but is a significant on-prem platform.

### Goals

1. Installer creates infrastructure and installs OpenShift on Azure Stack Hub.
2. All standard support that entails, such as destroy.

### Non-Goals

1. ASH will not necessarily support all Azure public cloud features.
## Proposal


### User Stories

#### Story 1
As a cluster admin, I want to be able to use `openshift-install` to create and destroy a cluster on
Azure Stack Hub.

#### Story 2
As an OpenShift developer, I want Azure Stack Hub to be managed and released like other platforms and covered by CI tests.

### Implementation Details/Notes/Constraints [optional]

Primary ways in which Azure Stack differs from Azure:
- users must specify API endpoint(s)
- utilizes different API versions than public Azure
- requires different rhcos image
- limited instance metadata service (IMDS) for VMs
- does not support private DNS zones
- limited subset of Azure infrastructure available (ergo different Terraform provider)

### Risks and Mitigations

The risk of adding ASH support is somewhere between adding a new Azure environment and an entirely new platform.

## Design Details

### Infrastructure Global Configuration

#### API
ASH will need to be added to the known [Azure Cloud Environments in the API](https://github.com/openshift/api/blob/master/config/v1/types_infrastructure.go#L339-L355):

```go
// AzureStackCloud is the Azure cloud environment used on-premises.
AzureStackCloud AzureCloudEnvironment = "AzureStackCloud"
```

#### Cluster Config Operator
Cluster config operator will [need to be updated](https://github.com/openshift/cluster-config-operator/blob/master/pkg/operator/kube_cloud_config/azure.go#L23) and the new API vendored in.

#### API endpoints
API endpoints vary between Azure Stack deployments and, therefore, must be provided to the installer by the user or
constructed by the installer based on user input.

The primary API endpoint is the Management Endpoint, which follows a naming pattern:

```shell
https://management.{region}.{Azure Stack Hub domain}
```
Once this management endpoint is determined, it can be used to determine other endpoints of the stack instance.
For example, with the `PPE3` region and `stackpoc.com` domain we can [query the metadata endpoint for endpoints](https://docs.microsoft.com/en-us/azure-stack/user/azure-stack-rest-api-use?view=azs-2008):

```shell
curl 'https://management.ppe3.stackpoc.com/metadata/endpoints?api-version=2015-01-01' | jq
{
  "galleryEndpoint": "https://providers.ppe3.local:30016/",
  "graphEndpoint": "https://graph.windows.net/",
  "portalEndpoint": "https://portal.ppe3.stackpoc.com/",
  "authentication": {
    "loginEndpoint": "https://login.microsoftonline.com/",
    "audiences": [
      "https://management.stackpoc.com/81c9b804-ec9e-4b5a-8845-1d197268b1f5"
    ]
  }
}

```

##### Kubelet, Cloud-provider endpoints and Azure/go-autorest

 When the cloud environment is set to
`AZURESTACKCLOUD`, the Kubelet, utilizing [Azure/go-autorest](https://github.com/Azure/go-autorest/blob/562d376/autorest/azure/environments.go#L207-L225) expects the environment variable `AZURE_ENVIRONMENT_FILEPATH` to point to a [json configuration file](https://kubernetes-sigs.github.io/cloud-provider-azure/install/configs/#azure-stack-configuration), which is [typically located at `/etc/kuberentes/azurestackcloud.json`](https://github.com/kubernetes-sigs/cloud-provider-azure/issues/151).

The installer can create this JSON file based on the metadata described above, and lay down the configuration file
and Kubelet environment variable with machineconfigs.

##### Operators
Many operators will also need to set the resource management (and perhaps other) endpoints. In order to provide the
endpoints to operators, the installer should add the endpoints file to the
[cloud provider configmap](https://github.com/openshift/installer/blob/master/pkg/asset/manifests/cloudproviderconfig.go#L126-L141):

```go
  cloudProviderEndpointsKey = "azurestackcloud.json"

  azureStackEndpoints, err := azure.stackEndpoints{
			resourceManagerEndpoint:          resourceManagerEndpoint,
			serviceManagementEndpoint:        serviceManagementEnpoint,
			...
		}.JSON()
		if err != nil {
			return errors.Wrap(err, "could not create cloud provider config endpoints")
		}
		cm.Data[cloudProviderEndpointsKey] = azureStackEndpoints

```

Operators can then [create clients](https://github.com/Azure/azure-sdk-for-go/blob/master/services/attestation/mgmt/2020-10-01/attestation/client.go#L45-L53) using the resource manager endpoint and subscription ID from the cloud provider
configs.
#### Installer Manifests & Cloud Provider Config
ASH should be able to reuse Azure machine manifests with proper validation.

The [Azure cloud-provider-config](https://github.com/openshift/installer/blob/master/pkg/asset/manifests/azure/cloudproviderconfig.go) will need to be slightly adapted. `LoadBalancerSku` for ASH will be `basic`. `UseInstanceMetadata` should probably be `false`. Service principal credentials will be included here [(see next section)](#identity-management).

The cluster [DNS manifest contains the private DNS zone](https://github.com/openshift/installer/blob/master/pkg/asset/manifests/dns.go#L110-L112). At a minimum this should be removed, but there is an [open question](#open-question) about how this should be handled.

### Cloud Provider Authentication (Identity Management)
Azure Stack does not support user-assigned identities. In public Azure, the Installer uses a user-assigned identity to
authenticate to the cloud provider. To overcome this lack of user-assigned identities, we can passthrough the service
principal used for Installer authentication to the cloud-provider config with `aadClientID` & `aadClientSecret`. We
must ensure that we are able to rotate the service principal secret in the cloud-provider config and that doing so
will cause the updated service principal to be used (presumably by writing a new machine config & causing a reboot).

### User Authentication

Users of the installer will authenticate with service principals. The standard Azure workflow works here.

### Lack of Private DNS Zones
ASH does not provide private DNS zones. In order to provide internal cluster access to `api-int`, an external and
internal load balancer will be created. A public A record for `api-int` will be created that points to the internal
load balancer while the`api` record points to the external load balancer. Internal cluster traffic will resolve
the `api-int` record from public DNS to the private IP address of the internal load balancer. Cluster traffic
to the (external) `api` will be sent off the cluster.

### Install Config
Users must provide a resource manager endpoint. A new field will need to be added to the install config for the URI.

As discussed above, the endpoint follows a pattern: `https://management.{region}.{Azure Stack Hub domain}`. As the install config contains only the region, we could add a field for only the Azure Stack domain and construct the endpoint. On the other hand, I have seen an example where the endpoint did not follow this pattern (`adminmanagement` instead of
`management`) but this was not a production environment so may not be an actual problem. For advanced use, this value could
be edited in the manifests. If we believe having users supply only the domain is more ergonomic, then this does seem like
a viable option that would capture most use cases.

Another option to consider, would be adding the ability to [supply all endpoints, similar to AWS](https://github.com/openshift/installer/blob/master/pkg/types/aws/platform.go#L39).
### Terraform
Azure Stack has [a stand-alone Terraform provider](https://registry.terraform.io/providers/hashicorp/azurestack/latest/docs).
The Azure Stack provider does not support the creation of images. We will work on adding this support.
### Alternative Considerations
Azure Stack is substantially different than Azure so it could be added as its own platform. The direction of upstream
seems to preserve Azure Stack as a cloud variant, and this allows the ability to reuse much of the existing Azure
code which seems largely applicable. When necessary we can deviate based on `cloud`. For these reasons, it seems
we should stick with AzureStack as part of the Azure platform.

System-assigned identities could be used in the absense of user-assigned identities. This would require adding roles
after the creation of the VMs and probably adding support for this use case in the Machine API operator.
Utilizing service principal secrets is simpler.

We have considered using [Baremetal Networking](../network/baremetal-networking.md) to overcome the lack of
private DNS support. We may revisit this idea if [the current design](#lack-of-private-dns-zones) is found wanting.
### Open Questions

1. Should ASH produce a cluster DNS manifest or follow the baremetal approach and not create one?
1. Suggestions for determining all operators that need to adapt for the ASH config.

### Test Plan

E2E tests should be created specifically for Azure Stack. An Azure Stack environment has been obtained for this purpose.

### Graduation Criteria

None. Same as any platform.

### Version Skew Strategy

None. Same as any platform.

## Implementation History

None

## Drawbacks

None

## Alternatives

These are primarily still open questions.
