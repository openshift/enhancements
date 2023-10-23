---
title: expose-cluster-publishing-status
authors:
  - "@flavianmissi"
reviewers: # Include a comment about what domain expertise a reviewer is expected to bring and what area of the enhancement you expect them to focus on. For example: - "@networkguru, for networking aspects, please look at IP bootstrapping aspect"
  - "@patrickdillon, installer"
  - "@Miciah, edge networking"
approvers: # A single approver is preferred, the role of the approver is to raise important questions, help ensure the enhancement receives reviews from all applicable areas/SMEs, and determine when consensus is achieved such that the EP can move forward to implementation.  Having multiple approvers makes it difficult to determine who is responsible for the actual approval.
  - TBD
api-approvers: # In case of new or modified APIs or API extensions (CRDs, aggregated apiservers, webhooks, finalizers). If there is no API change, use "None"
  - "@JoelSpeed"
creation-date: yyyy-mm-dd
last-updated: yyyy-mm-dd
tracking-link: # link to the tracking ticket (for example: Jira Feature or Epic ticket) that corresponds to this enhancement
  - https://issues.redhat.com/browse/IR-302
---

# Expose Cluster Publishing Status

## Summary

IPI clusters can be configured to be public or private. This is done in the
install-config.yaml using the `Publish` parameter. There is no current standard
defined for how components should access this parameter to bootstrap themselves
as public (External) or private (Internal).

At the moment of writing, provisioning a private cluster means the cluster
will be configured with a private DNS zone, internal (non internet facing)
ingress, and internal API server.

Exposing the cluster's publishing status empowers other components to bootstrap
themselves as public or private according to the user request at install time.

## Motivation

A component should be able to know how to bootstrap itself according to user
preference. A central place providing the cluster publishing status at install
time makes more sense than the installer having to individually configure
various components.

### User Stories

* As the author of the image registry operator, I want to easily discover the
cluster publishing status, so that I can configure access the cloud storage objects
accordingly.


In the specific case of OpenShift's internal image registry, when a cluster is
provisioned as internal, the registry operator will provision the required cloud
storage assets without consideration for the cluster publishing status. In Azure
specifically, customers find this problematic as their cloud console will show
them a security warning about the exposed storage account.

To circumvent this problem, the registry operator needs to know when to provision
the storage account behind a private endpoint. Currently there is no supported
way for components to know whether they should configure themselves as internal
or not.

### Goals

Expose the cluster publishing status used at install time in the Infrastructure
object.

### Non-Goals

This enhancement does not cover support for updating the cluster publishing
status on running clusters. This property is meant to reflect the `publish`
parameter as it was set in the install-config.yaml.

## Proposal

The Infrastructure API is extended, adding the `Publish` field to the
`InfrastructureStatus`.

```go
type InfrastructureStatus struct {
	// ...

	// publish indicates the publishing status of the cluster at install time.
	// The default is 'External', which means the cluster will be publicly
    // available on the internet.
	// The 'Internal' mode indicates a private cluster, and operators should
    // configure operands accordingly.
    // The empty value means the publishing status of the cluster at install
    // time is unknown.
    // +kubebuilder:validation:Enum="";Internal;External;
    // +kubebuilder:default=""
	// +optional
	Publish PublishMode `json:"publish,omitempty"`
}
```

To reflect the install-config publishing options, `PublishMode` will only
support to valid options.

```
// PublishMode defines the publish mode of the cluster at install time.
type PublishMode string

const (
	// "Internal" is for operators to configure their asserts as private
    // when appropriate.
	InternalPublishMode PublishMode = "Internal"

	// "External" indicates that the cluster is publicly available on the internet.
	ExternalPublishMode PublishMode = "External"
)
```

### Workflow Description

1. The cluster creator install a cluster via IPI, setting `publish: Internal` in installer-config.yaml
2. The installer bootstraps the cluster, and sets `Publish: Internal` in the Infrastructure status
3. Operators use this value to configure operands accordingly.

### API Extensions

### Implementation Details/Notes/Constraints [optional]

The `Publish` field in the infrastructure status will only reflect the initial
status of the cluster. Individual components (like ingress) may be changed on
day-2 to for example turn a public cluster private, and the `Publish` field in
the Infrastructure status will remain untouched.

#### Hypershift [optional]

Hypershift publishing options are not an exact match for the ones provided by
the installer in installer-config.yaml.

Hypershift options (limited to AWS) are:

* `Public`
* `PublicAndPrivate`
* `Private`

`PublicAndPrivate` does not translate well to OpenShift. It allows public API
server access and private node communication with the control plane, making
the cluster appear public to end-users, while securing control plane and
nodes communication.

The suggested approach to Hypershift is to translate the modes as follows:

* `Public` -> `Public`
* `PublicAndPrivate` -> `Public`
* `Private` -> `Private`

### Risks and Mitigations

Users may interpret the `Publish` field in the Infrastructure status as a way
to change the publishing status of their clusters at run time, which is not
its intention.

### Drawbacks

## Design Details

### Open Questions [optional]

### Test Plan

The installer needs to ensure that the value set in the Infrastructure status
is the same as what the user set in the install-config.yaml.

### Graduation Criteria

- N/A

#### Dev Preview -> Tech Preview

- N/A

#### Tech Preview -> GA

- N/A

#### Removing a deprecated feature

N/A

### Upgrade / Downgrade Strategy

Upgrading a cluster will not affect the publishing status of the cluster.
The `Publish` field is set by the installer in the Infrastructure object at
cluster install time - no value will be set during upgrades or downgrades.

Components consuming this value should be prepared for the case when it is unset.

### Version Skew Strategy

The `Publish` field in the Infrastructure status should only be used by
operators at bootstrap time. It does not affect upgrades or version skew.

### Operational Aspects of API Extensions

#### Failure Modes

#### Support Procedures

## Implementation History

## Alternatives

Similar to the `Drawbacks` section the `Alternatives` section is used to
highlight and record other possible approaches to delivering the value proposed
by an enhancement.

### Operators consume 'publish' field from the "cluster-config" config map

The kube-system namespace contains a cluster-config config map, which contains
the install-config.yaml used to bootstrap the cluster.
The cluster-config config map is deprecated.

### Use operator-specific APIs to manage the behaviors

Operators would specify their own version of the `public` field in their own
configurations.
This is not scalable, as new or removed operators would have to be handled
individually.

## Infrastructure Needed [optional]

