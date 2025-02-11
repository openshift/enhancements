---
title: aws-imdsv2-support
authors:
  - "@lobziik"
reviewers:
  - "@patrickdillon"
  - "@JoelSpeed"
approvers:
  - "@patrickdillon"
  - "@JoelSpeed"
creation-date: 2022-03-28
last-updated: 2022-04-05
tracking-link: 
  - "https://issues.redhat.com/browse/OCPCLOUD-1436"
---

# Support AWS IMDSv2 for EC2 machines

## Summary

For increasing security and add protection against open firewalls, reverse proxies, and SSRF vulnerabilities Amazon introduced
[enhancements to the EC2 Instance Metadata Service](http://aws.amazon.com/blogs/security/defense-in-depth-open-firewalls-reverse-proxies-ssrf-vulnerabilities-ec2-instance-metadata-service/)
(IMDSv2). It is a session-based interaction method with Instance Metadata Service (IMDS), in other words,
all requests to `http://169.254.169.254` will require prior PUT requests for obtaining token and such token must be used in further requests.
This document describes and proposes several interfaces for configuring IMDS behaviour for newly created machines via Machine API and
during the installation procedure.

## Motivation

By adding mechanisms for configuring IMDS behaviour we will allow our end-users to enhance the security of their clusters and simplify Amazon [recommended best practices](https://docs.aws.amazon.com/securityhub/latest/userguide/securityhub-standards-fsbp-controls.html#fsbp-ec2-8) implementation.
At this point, IMDSv2 enablement might be done manually via AWS console, but providing such settings on Machine API is a quite loud request from our end-users.

### Goals

- provide mechanisms to enable/enforce IMDSv2 for newly created machines via Machine API
- provide mechanisms to enable/enforce IMDSv2 for master machines during installation procedure for newly created clusters

### Non-Goals

- perform automated migration between using IMDSv1 and IMDSv2 for existing infrastructure

## Proposal

- Add new options into install-config for the OCP installation program which will be passed as metadata service settings during virtual machines creation
- Extend [AWSMachineProviderConfig](https://github.com/openshift/api/blob/1a6fa2913810101176a1d776f899fc4781b3fa50/machine/v1beta1/types_awsprovider.go#L12) by adding metadata service parameters
- Pass metadata service parameters to AWS during new machines creation

### User Stories

- As an OpenShift cluster administrator, I would like to enforce IMDSv2 usage during an OCP cluster deployment procedure.
- As an OpenShift cluster administrator, I would like to enforce IMDSv2 usage for newly created machines in my cluster via MachineSets configuration.
- As an OpenShift cluster administrator, I would like to change IMDS interaction settings for newly created machines in my cluster.

### API Extensions

#### AWSMachineProviderConfig changes
A new optional field will be added to the `AWSMachineProviderConfig` struct `MetadataServiceOptions`.

`MetadataServiceOptions` will be a struct with single `Authentication` field at this point.
Also, a new `MetadataServiceAuthentication` type that describes the possible values for the `Authentication` field on `MetadataServiceOptions` will be added.

The concrete changes to the `AWSMachineProviderConfig` outlined below:
```go
type AWSMachineProviderConfig struct {
    // Existing fields will not be modified
    ...
    // MetadataServiceOptions allows users to configure instance metadata service interaction options.
    // If nothing specified, default AWS IMDS settings will be applied.
    // https://docs.aws.amazon.com/AWSEC2/latest/APIReference/API_InstanceMetadataOptionsRequest.html
    // +optional
    MetadataServiceOptions MetadataServiceOptions `json:"metadataServiceOptions,omitempty"`
}
```

```go
type MetadataServiceAuthentication string

const (
	// MetadataServiceAuthenticationRequired enforces sending of a signed token header with any instance metadata retrieval (GET) requests.
	// Enforces IMDSv2 usage.
	MetadataServiceAuthenticationRequired = "Required"
	// MetadataServiceAuthenticationOptional allows IMDSv1 usage along with IMDSv2
	MetadataServiceAuthenticationOptional = "Optional"
)

// MetadataServiceOptions defines the options available to a user when configuring
// Instance Metadata Service (IMDS) Options.
type MetadataServiceOptions struct {
	// Authentication determines whether or not the host requires the use of authentication when interacting with the metadata service.
	// When using authentication, this enforces v2 interaction method (IMDSv2) with the metadata service.
	// When omitted, this means the user has no opinion and the value is left to the platform to choose a good
	// default, which is subject to change over time. The current default is optional.
	// At this point this field represents `HttpTokens` parameter from `InstanceMetadataOptionsRequest` structure in AWS EC2 API
	// https://docs.aws.amazon.com/AWSEC2/latest/APIReference/API_InstanceMetadataOptionsRequest.html
	// +kubebuilder:validation:Enum=Required;Optional
	// +optional
	Authentication MetadataServiceAuthentication `json:"authentication,omitempty"`
}
```

### Implementation Details/Notes/Constraints

#### Install Config changes

For supporting IMDSv2 during installation we could extend install-config with aws specific `metadataService`
section in the installer configuration.

Example (origin is the [aws installation doc](https://docs.openshift.com/container-platform/4.10/installing/installing_aws/installing-aws-customizations.html#installation-aws-config-yaml_installing-aws-customizations)):

```yaml
apiVersion: v1
baseDomain: example.com 
controlPlane:   
  hyperthreading: Enabled 
  name: master
  platform:
    aws:
      zones:
      - us-west-2a
      - us-west-2b
      rootVolume:
        iops: 4000
        size: 500
        type: io1
      metadataService: # proposed section
        authentication: Required
      type: m5.xlarge
  replicas: 3
compute: 
- hyperthreading: Enabled 
  name: worker
  platform:
    aws:
      rootVolume:
        iops: 2000
        size: 500
        type: io1
      metadataService: # proposed section
        authentication: Required
      type: c5.4xlarge
      zones:
      - us-west-2c
  replicas: 3
metadata:
  name: test-cluster 
networking:
  clusterNetwork:
  - cidr: 10.128.0.0/14
    hostPrefix: 23
  machineNetwork:
  - cidr: 10.0.0.0/16
  networkType: OpenShiftSDN
  serviceNetwork:
  - 172.30.0.0/16
platform:
  aws:
    region: us-west-2 
    userTags:
      adminContact: jdoe
      costCenter: 7536
    amiID: ami-96c6f8f7 
    serviceEndpoints: 
      - name: ec2
        url: https://vpce-id.ec2.us-west-2.vpce.amazonaws.com
fips: false 
sshKey: ssh-ed25519 AAAA... 
pullSecret: '{"auths": ...}' 
```

#### Installer changes

In order to support the changes described in the paragraph above some changes in the installer should be made:
1. Internal structures should be extended for storing metadata service options from the installer config.
2. Values for control plane machines should be piped down to terraform configs for passing it to AWS API during master machines creation. AWS terraform provider already [support such parameters](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/instance#metadata-options)
3. Values for worker machines should be used for rendering proper MAPI MachineSet manifests in case of IPI installation.

#### AWS Machine API provider changes

Parameters described in *API Extensions* sections should be taken and passed to AWS API during new EC2 instance creation.
AWS SDK which we use already supports [such](https://pkg.go.dev/github.com/aws/aws-sdk-go@v1.38.23/service/ec2#RunInstancesInput) [parameters](https://pkg.go.dev/github.com/aws/aws-sdk-go@v1.38.23/service/ec2#InstanceMetadataOptionsRequest).

#### AWS Termination handler changes

AWS specific [terimination handler](https://docs.openshift.com/container-platform/4.6/machine_management/creating_machinesets/creating-machineset-aws.html#machineset-non-guaranteed-instance_creating-machineset-aws)
which relies on interaction with IMDS should be patched in order to support IMDSv2 for MAPI spot instances.

### Risks and Mitigations

#### Openshift components might rely on IMDS interactions
Some openshift components might rely on interactions with IMDS.
Enforcing a session-based interaction procedure might break it and required changes in order to support IMDSv2.

#### User workloads might rely on IMDS interactions
Some user workloads might not support IMDSv2. Such workloads will be broken on per-vm basis in the case of IMDSv2 enforcement. 

## Design Details

### Open Questions

- Should we enforce IMDSv2 (prohibit IMDSv1) for newly created clusters?

### Test Plan

Openshift e2e suite should be sufficient to prove the common safety of such changes.

Aws-specific tests for exercising new API fields should be added to the Machine API e2e test suite.
Specifically, a MachineSet creation with enabled IMDSv2 with a future check that machines are there and operable should be enough.

Presumably, an extra CI job should be required for exercise installer changes.
IMDSv2 should be enabled during installation time in the install-config, then the openshift conformance e2e test suite should be executed.

### Graduation Criteria

Addition of API fields to Machine API implies that the feature is GA from the beginning, no graduation criteria are required.
Addition of AWS-specific optional parameters to the installer config presumably does not require additional graduation criteria as well.

#### Dev Preview -> Tech Preview

N/A

#### Tech Preview -> GA

N/A

#### Removing a deprecated feature

This enhancement does not describe the removal of a deprecated feature.

### Upgrade / Downgrade Strategy

Existing clusters being upgraded will not have any undesired effect as this these features do not interfere with any other one and require additional configuration to be enabled.

Once configured, on a downgrade, the Machine API components will not know about the new fields, and as such, they will ignore IMDS parameters if specified.
Machines created with the IMDS parameters configured will be unaffected, persisted, and stay as it was.

### Version Skew Strategy

Since it is an optional aws-specific feature of Machine API, version discrepancy with other components should not have any negative effect. 

### Operational Aspects of API Extensions

#### Failure Modes

Enforcing IMDSv2 might lead to end-user workloads degradation in the case of such workloads relying on an old mechanism 
of interaction with the AWS Instance Metadata service.
This already was discussed in the Risks section.

#### Support Procedures

- If some component relies on IMDS interaction and does not support IMDSv2, it will receive `401 Unauthorized` response on each IMDS request.
  * Such errors handling expecting to have no difference with networking/bad requests issues and should be treated accordingly on a per-component basis.

## Implementation History

- https://github.com/coreos/ignition/pull/1154
- https://github.com/openshift/machine-config-operator/pull/2988
- https://github.com/openshift/api/pull/1156

## Future Implementation

In the future, we may wish to add additional [metadata service parameters](https://docs.aws.amazon.com/AWSEC2/latest/APIReference/API_ModifyInstanceMetadataOptions.html),
such as `HttpPutResponseHopLimit` and `InstanceMetadataTags`. 
This will require further API extension and respective controllers/installer changes.

## Drawbacks

N/A

## Alternatives

Do not provide any additional knobs in MachineAPI and in install-config, leave users to deal with it themselves manually
via AWS interfaces (web-console, cli)

## Infrastructure Needed

Additional CI job for exercise new options in install config.
