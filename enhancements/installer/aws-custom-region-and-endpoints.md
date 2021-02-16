---
title: aws-custom-regions-and-endpoints
authors:
  - "@abhinavdahiya"
reviewers:
  - "@ironcladlou"
  - "@enxebre"
  - "@adambkaplan"
  - "@dgoodwin"
  - "@jstuever"
  - "@patrickdillon"
approvers:
  - "@deads"
  - "@sdodson"
  - "@crawford"
creation-date: 2019-12-13
last-updated: 2020-03-03
status: implementable
---

# AWS Custom Regions and Custom Endpoints Support

## Release Signoff Checklist

- [x]  Enhancement is `implementable`
- [x] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Open Questions

## Summary

As an administrator, I would like to deploy OpenShift 4 clusters to AWS GovCloud, "hidden" regions and outpost deployments. These environments require the cluster to use custom endpoints for AWS APIs. The users provide the custom endpoints for various AWS services and the installer and cluster operators will use the specified endpoints to access the AWS APIs.

## Motivation

### Goals

1. Allow users to specify API endpoints for some or all required AWS services.

2. Provide users capability to install in custom regions on AWS.

3. Provide automated handling for most users with respect to getting AMI available in the custom regions.

### Non-Goals

1. There are no plans to allow self-signed server certificates for endpoints specifically.

2. Allowing users to use un-encrypted AMIs for the ec2 instances.

## Proposal

To support custom endpoints for AWS APIs, the install-config would
allow the users to provide a list of endpoints for various
services. When the custom endpoints are set, the installer will
validate that the endpoints are reachable. The endpoints will be used
by the installer to call AWS APIs for performing various actions like
validations, creating the MachineSet objects, and also configuring the
terraform AWS provider.

When custom endpoints are provided for AWS APIs, the cluster operators also need the discover the information, therefore, the installer will make these available using the `config.openshift.io/v1` `Infrastructure` object. The cluster operators can then use the custom endpoints from the object to configure the AWS SDK to use the corresponding endpoints for communications.

Since various kubernetes components like the kube-apiserver, kubelet
(machine-config-operator), kube-controller-manager,
cloud-controller-managers use the `.spec.cloudConfig` Config Map
reference for cloud provider specific configurations, a new controller
`cluster-kube-cloud-config-operator` will be introduced. The
controller will perform the task of stitching the custom endpoints
with the rest of the cloud config, such that all the kubernetes
components can continue to directly consume a Config Map for
configuration. This controller will also perform the specialized
stitching on the bootstrap host for control-plane kubelet and also
actively reconcile the state in the cluster.

Currently the installer only allows users to specify the regions that has RHCOS published AMIs, and therefore to support custom regions, the installer will allow users to specify any region string as long as the users also provide the custom endpoints for some predetermined list of services that are necessary for successful installs.

To support booting machines in custom regions, the installer will
always copying AMIs from `us-east-1` to the region where the user is
installing, encrypting the AMI using the user's default account KMS
key. This is has known limitations for increased AMI copy times and
also increased cost of transfer across regions. If the target region
doesn't support copying AMI from `us-east-1` region, the users are
expected to provide the source AMI in the install-config using the
platform [configuration][install-config-aws-ami]. The installer will
use the AMI provided and copy-encrypt to an AMI for the cluster,
keeping the behavior consistent with other workflows.

### User Stories

#### US1: Using private VPC endpoints

#### US2: Special endpoints like FIPS endpoints for US Gov Cloud

#### US3: Installing to new public regions without native support for RHCOS or AWS SDK

#### US4: Installing to custom region

## Implementation Details/Notes/Constraints

### Installer

#### Install Config

The user can provide a list of API endpoints for various services.

```go
// ServiceEndpoint store the configuration of a url to
// override existing defaults of AWS Services.
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
    // service endpoint of AWS Services.
    // There must be only one ServiceEndpoint for a service.
    // +optional
    ServiceEndpoints []ServiceEndpoint `json:"serviceEndpoints,omitempty"`
    ...
}
```

The yaml representation by the user would look like:

```yaml
platform:
  aws:
    serviceEndpoints:
    - name: ec2
      url: https://ec2.custom.url
    - name: s3
      url: https://s3.custom.url
```

#### Validations for service endpoints

1. The installer must ensure that only one override for service is specified by the user.

2. The installer should ensure that the service endpoint is at least reachable from the host.

3. The URL for the service endpoint must be `https` and the host should trust the certificate.

#### Configuring the AWS SDK

There are 2 ways to configure the AWS SDK,

1. Use the sdk [resolver][aws-sdk-endpoints-resolver] like Kubernetes AWS [cloud-provider][k8s-aws-cloud-provider-get-resolver] so that the AWS SDK's default resolver is used for AWS APIs and override with the user provided ones when available.

2. Specifically override the AWS config when initializing the clients, for example the terraform-provider-aws's internal [configuration][terraform-provider-aws-config-endpoints]

#### Configuring the internal terraform-provider-aws with service overrides

The Terraform AWS provider doesn't accept random AWS API endpoint [overrides][terraform-aws-provider-api-endpoints], rather endpoint for each service needs to be explicitly specified. So the `tfvars` can pass a map object and the terraform `main.tf` can set each required endpoint override when certain key is present and set in the provide map object.

#### Destroying clusters

The `metadata.json` needs to store the custom endpoints from the `install-config.yaml` so that the users can delete the clusters without any previous state.

### Infrastructure global configuration for service endpoints

Since almost of the cluster operators that communicate to the the AWS API need the use the API endpoints provided by the user, the `Infrastructure` global configuration seems like the best place to store and discover this information.

And in contrast to the current configuration in the `Infrastructure` object like `region` is not editable as day-2 operation, the users should be able to edit the AWS endpoints day-2, therefore this configuration best belongs in the `spec` section.

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

  // AWS contains settings specific to the Amazon Web Services infrastructure provider.
  // +optional
  AWS *AWSPlatformSpec `json:"aws,omitempty"`

  ...
}
```

```go
// AWSPlatformSpec holds the current status of the Amazon Web Services infrastructure provider.
type AWSPlatformSpec struct {
  // ServiceEndpoints list contains custom endpoints which will override default
  // service endpoint of AWS Services.
  // There must be only one ServiceEndpoint for a service.
  // +optional
  ServiceEndpoints []AWSServiceEndpoint `json:"serviceEndpoints,omitempty"`
}

// AWSServiceEndpoint store the configuration of a custom url to
// override existing defaults of AWS Services.
// Currently Supports - EC2, IAM, ELB, S3 and Route53.
type AWSServiceEndpoint struct {
  Name string `json:"name"`

  // This must be a HTTPS URL
  URL     string `json:"url"`
}
```

But since the users are going to be specifying the service endpoints for APIs, there is chance of user error and operators picking up invalid or incorrect information. Therefore, the service endpoints will be mirrored to the `status` section after basic validations by a [controller][TODO-link-to-section] and the cluster operators will use the information from the `status` section.

```go
// AWSPlatformStatus holds the current status of the Amazon Web Services infrastructure provider.
type AWSPlatformStatus struct {
  // region holds the default AWS region for new AWS resources created by the cluster.
  Region string `json:"region"`

  // ServiceEndpoints list contains custom endpoints which will override default
  // service endpoint of AWS Services.
  // There must be only one ServiceEndpoint for a service.
  // +optional
  ServiceEndpoints []AWSServiceEndpoint `json:"serviceEndpoints,omitempty"`
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
   cluster-network-operator that might require access to the AWS APIs
   in the future to control the security group rules. Also various OLM
   operators that interact with AWS APIs will need their own
   configuration. Configuring all these separately is not a great UX
   for installer and a user who wants to modify the cluster to use API
   endpoints as day-2 operation.

3. AWS has a `cloud.conf` file as mentioned in [upstream
   docs][k8s-cloud-conf] which is logically already an API supported
   and the file format already represents the canonical representation
   of this information.

   The cloud configuration doesn't translate well to what most of the
   OpenShift operators need to configure the clients for cloud
   APIs. The cloud config includes a lot more information than just
   configuring the clients like instance prefixes for LBs, information
   about the vpc, subnets etc. which the operators operators don't
   need. These values in the configuration makes it very difficult to
   abstract away information which most of the operators do not need.
   Secondly, if there ends up an special configuration required to
   configure the OpenShift operators for a cloud, we will end up
   conflicting with kube-cloud-controller code to make the ask valid
   as since it's purpose upstream is to configure
   k8s-cloud-controllers only.

### Cluster Kube Cloud Config Operator

Since various kubernetes components like the kube-apiserver, kubelet
(machine-config-operator), kube-controller-manager,
cloud-controller-managers use the `.spec.cloudConfig` Config Map
reference from the global `Infrastructure` configuration for cloud
provider specific configurations. Introduction of new controller
`cluster-kube-cloud-config-operator` allows performing the task of
stitching the custom endpoints with the rest of the cloud config, such
that all the kubernetes components can continue to directly consume a
Config Map for configuration.

#### Boostrap host control flow

The controller reads the on disk `Infrastructure` object, and the cloud config Config Map from disk to

1. Create a new cloud config Config Map, stitching the existing cloud config and service endpoints for AWS.

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

1. Bail out with error, when the user has set the service endpoints in the cloud config.
As the service endpoints are already controls by other fields in the `Infrastructure` object, trying to merge 2 sources of information would be erroneous.

2. Ensure that service endpoints are valid and reachable by cluster components.

#### New Operator Alternatives

1. Previous versions recommended that the kube-controller-manager and machine-config-operator perform the stitching for cloud config. The kube-controller-manager-operator owners do not want to understand or handle the vagaries of various cloud providers. The operator allows those components to use the Config Map as-is. See [comment](https://github.com/openshift/enhancements/pull/163#discussion_r359962825)

2. The operator could also be named Infrastructure Config Operator and the Kube Cloud Config controller could become a sub controller. The operator could perform other functions related to the Infrastructure object and cloud providers.

### Cluster Operators

Almost of the cluster operators will have to read the api endpoints from the `infrastructures.config.openshift.io` `cluster` object's `status` for AWS and make sure the operators are watching for changes.

#### Configuring the AWS SDK for controllers

1. Use the sdk [resolver][aws-sdk-endpoints-resolver] like Kubernetes AWS [cloud-provider][k8s-aws-cloud-provider-get-resolver] so that the AWS SDK's default resolver is used for AWS APIs and override with the user provided ones when available.

2. Specifically override the AWS config when initializing the clients, for example the terraform-provider-aws's internal [configuration][terraform-provider-aws-config-endpoints]
Passing in empty string for Endpoints when initializing clients causes the SDK to use the internal default. So hopefully that reduces the conditional overhead.

The second options is going to be most useful for controllers because the services used by the controllers is finite/known.

#### Machine API

The AWS machine controller should be configured to use the service endpoint overrides from the `Infrastructure` object. It makes sense to configure the controller itself because per machine configuration for API endpoints would not be ergonomic for two reasons:

1. The Machines / MachineSets are created by the installer or users once, while the service endpoints can be updated by users later and then the Machine objects will have to be updated out of band to reflect the change.

2. The requirement for different service endpoint per Machine doesn't seem to provide major value.

### Custom regions

#### Installer validations for allowed regions

The installer currently only allows users to specify regions which have RHEL CoreOS AMI published. So to allow for users to specify custom regions or even new public regions that are known by the installer, the installer should allow users to specify any region string.

The installer although should keep track if the specified region is `known`. A region is `known` when

1. There is RHEL CoreOS AMI for the region known to the installer binary.

2. The regions is one of the known regions to the AWS SDK vendored into the binary.`

#### Service endpoints for unknown regions

The installer requires user specify service endpoints for a list of AWS services. The list of services are,

* ec2
* elasticloadbalancing
* s3
* iam
* route53
* tagging

If the user provides `unknown` region, and doesn't provide service endpoints for the services mentioned above the installer returns a validation error.

#### Default instances for custom regions

AWS now allows users to find all the available instance type based on some constraints using [DescribeInstanceTypeOfferings][aws-describe-instance-type-offerings]. The installer should,

1. Use this API to find m4/m5 instance types for the region that match the default CPU and memory requirements for control-plane and compute.

2. Error out requiring the users to explicitly specify the desired instance types in cases where (1) doesn't return successful results.

#### Picking the AMI for booting machines

Terraform AWS Provider has a resource [`aws_ami_copy`][terraform-aws-provider-ami-copy] that can copy AMI from one region to the current region setup for the provider. The installer will copy the AMI from `us-east-1` region while continuing to encrypt it with the default KMS key.

But if there is a restrictions that the AMI cannot be copied from the `us-east-1` like in sovereign cloud regions, the installer will try to create cluster but fail mid way. The expectation is that the user will provide the AMI using the platform [configuration][install-config-aws-ami].

For cases when the users will have to import the AMI to their accounts, the users would need to know which AMI to import. There are two different work flows,

1. The users can continue to use the same process as the AWS user-provided-infrastructure workflow to find the corresponding AMI

2. https://github.com/openshift/enhancements/pull/201

##### Custom AMI Alternatives

The installer could require users provide AMI whenever the target region is `unknown`, but that would put un-necessary over head for customers that don't have the restrictions of copying the AMI from `us-east-1`

#### Region list for terminal prompts

The installer will only list `known` regions for the terminal prompts. Users that want to use the custom region will be required to provide `install-config.yaml`

### Risks and Mitigation

There are certain risks for supporting custom regions,

1. AMI provided to the installer for custom regions can be incorrect and therefore solution similar to https://github.com/openshift/enhancements/pull/201 are necessary for the future.

2. Users that don't provide custom endpoints for all the required services might see verbose failures when trying to use default API endpoints.
The installer will force users to provide service endpoints for some basic services, but any day-2 operator or new operators that require new services in the cluster will break on upgrades. Since the global configuration allows providing new service endpoints as day-2 operation, any failure for missing service endpoints should be easy to fix.

3. Picking correct instance type defaults for custom regions.
The user UX when the installer fails to find default instance types for the custom region is not great as it just errors out, but I think a little higher bar for custom region should not be too hard.

### Test Plan

TODO

### Graduation Criteria

None

### Upgrade / Downgrade Strategy

#### Downgrades

The new controller for cloud config reconciliation will be added on upgrade to clusters, and if the users try to downgrade to previous version, cluster version operator will leave the controller running.

As part of the controller, the deliverable will also include upstream docs that details steps for user to take to remove the new controller like

1. oc delete the namespace
2. remove any cluster resources like clusterrolebindings

Any changes made to the `Infrastructure` object will be dropped as the CRD sets preserveUnknownFields to false.

## Implementation History

None

## Drawbacks

Already covered inline.

## Infrastructure Needed

1. Access to US Gov Cloud region to test the changes.

2. Networking environment to test custom API endpoints.

[aws-sdk-endpoints-resolver]: https://docs.aws.amazon.com/sdk-for-go/api/aws/endpoints/#Resolver
[aws-describe-instance-type-offerings]: https://aws.amazon.com/blogs/compute/it-just-got-easier-to-discover-and-compare-ec2-instance-types/
[install-config-aws-ami]: https://github.com/openshift/installer/blob/e8289c5ddef58e17bbf22e225e179cbe70ac4108/pkg/types/aws/platform.go#L6-L8
[k8s-aws-cloud-provider-get-resolver]: https://github.com/kubernetes/kubernetes/blob/5cb1ec5fea6c4cafee6b8de3d09ca65361063451/staging/src/k8s.io/legacy-cloud-providers/aws/aws.go#L656-L680
[k8s-cloud-conf]: https://kubernetes.io/docs/concepts/cluster-administration/cloud-providers/#cloud-conf
[k8s-aws-cloud-provider-service-overrides]: https://github.com/kubernetes/kubernetes/blob/5cb1ec5fea6c4cafee6b8de3d09ca65361063451/staging/src/k8s.io/legacy-cloud-providers/aws/aws.go#L595-L616
[terraform-provider-aws-config-endpoints]: https://github.com/terraform-providers/terraform-provider-aws/blob/63b09d73d017b47d16790db92ba81c4f3fd7606e/aws/config.go#L390-L549
[terraform-aws-provider-api-endpoints]: https://www.terraform.io/docs/providers/aws/guides/custom-service-endpoints.html
[terraform-aws-provider-ami-copy]: https://www.terraform.io/docs/providers/aws/r/ami_copy.html
