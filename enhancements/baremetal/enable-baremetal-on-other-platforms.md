---
title: enable-baremetal-on-other-platforms
authors:
  - "@asalkeld"
  - "@sadasu"
reviewers:
  - "@hardy"
  - "@romfreiman"
  - "@dhellman"
approvers:
  - "@hardy"
creation-date: 2021-08-20
last-updated: 2021-08-20
status: implementable
see-also:
  - "/enhancements/baremetal-provisiong-config.md"
replaces:
superseded-by:
---

# Enable baremetal on other Platforms to support centralized host management

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [x] Design details are appropriately documented from clear requirements
- [x] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

Baremetal Host API is only available when deploying an OpenShift cluster with the baremetal
platform (via the IPI or AI (Assisted Installer) workflow). Having the ability to
manage baremetal hosts from clusters without requiring the cluster to be on baremetal
would be beneficial to customers.

## Motivation

An initial driver of this feature are the centralized host management use cases
in edge topologies, which without this feature, is restricted to having the
central OpenShift cluster deployed on baremetal.

See:
- https://github.com/openshift/enhancements/blob/master/enhancements/installer/agent-based-installation-in-hive.md
- https://github.com/openshift/assisted-service/tree/master/docs/hive-integration

### Goals

The specific goals of this proposal are to:

Support the centralized host management use case by partially enabling Baremetal Host API
on the following platforms:
- None
- OpenStack
- vSphere

We will be successful when:

Centralized host management can deploy clusters when running on the above platforms.

### Non-Goals

Allow Baremetal Host API to be fully enabled on all platforms.

Note that the scope of this epic is only enablement of the Metal3
BaremetalHost API and associated controller/services - the centralized host management flow
currently interacts directly via this API without any Machine API integration.

## Proposal

BMO (baremetal-operator) provides the Baremetal Host API, it in turn is configured
and managed by CBO (cluster-baremetal-operator).

CBO reads the Provisioning CR that is created by the installer on baremetal platforms
and uses that to configure and deploy BMO. In the case of non-baremetal platforms
the user (or automation) will need to define the Provisioning CR.

Currently CBO checks the platform and if it is not baremetal it will be in a "disabled" state i.e. it will
1. set status.conditions Disabled=true and
2. not read or process the Provisioning CR and thus not deploy baremetal-operator.

This proposal is to allow CBO to be enabled on the Platforms: None, OpenStack or vSphere.

Further (to restrict the testing matrix) the allowed configuration options
of the Provisioning CR will be restricted to exactly those required by centralized host management.

*Only spec.provisioningNetwork=Disabled mode will be accepted in the Provisioning CR.*

If any other provisioningNetwork mode is set, the CBO webhook will refuse the change
in the usual case, but if defined before upgrading the operator, the Reconcile loop
must always validate the Provisioning CR.

Note:

1. when the Provisioning CR is set to provisioningNetwork=Disabled mode, worker
nodes would be booted via virtual media. This removes the requirement for the
Provisioning Network which can be expected to be available only in Baremetal platform types.

2. documentation will need to be added to the centralized host management documentation
explaining how to create a Provisioning CR. Current documentation is here:
https://github.com/openshift/assisted-service/blob/8880093ef5ce041d4c1951ffd5ea1096991ec3ee/docs/user-guide/assisted-service-on-openshift.md#configure-bare-metal-operator


### User Stories

#### Story 1 - Current IPI baremetal platform use case

No change.

#### Story 2 - centralized host management use case

As a user of a hub cluster that performs central infrastructure management, and
optionally zero-touch provisioning, I need to provision hosts using the k8s-native
API (Baremetal Hosts CR) even when the hub cluster has a platform of None, OpenStack, or vSphere.


### Risks and Mitigations

There is concern that *random* customers will use this feature out of context
and create support burden. This is why we have not suggested enabling CBO on
all platforms and with full feature set. However it is still a potential issue.

## Design Details

### Open Questions [optional]

1. Where will the feature be e2e tested?

### Test Plan

#### Unit Testing

We will add unit tests to confirm that cluster-baremetal-operator:
* is enabled on the required platforms.
* will restrict functionality on these platforms to ProvisioningNetork=Disabled.

#### Functional Testing

An e2e test will be written in the Assisted Installer CI that will:
1. create one of the platforms above (SNO Platform=None might be the easiest) with Assisted Service.
2. confirm that CBO is enabled
3. create a Provisioning CR and confirm that BMO is running
4. provision a baremetal cluster

QE will validate the remaining platforms that are supported to reduce the load
on CI.

### Graduation Criteria

#### Dev Preview -> Tech Preview

#### Tech Preview -> GA

The feature will go to GA without tech preview

#### Removing a deprecated feature

### Upgrade / Downgrade Strategy

cluster-baremetal-operator will upgrade as it currently does, this is only a
minor change in functionality.

On the platforms (None, OpenStack and vSphere) where the operator was in a disabled
state, after been upgraded it will move into an enabled state. However in all but
centralized host management use cases nothing will change as there is no Provisioning CR.

### Version Skew Strategy

None required as this is not dependant on other components.

## Implementation History

This PR is the current WIP implementation: https://github.com/openshift/cluster-baremetal-operator/pull/189

## Drawbacks

There is concern that *random* customers will use this feature out of context
and create support burden.

## Alternatives

Customers can instead create a dedicated baremetal cluster to use as the hub
cluster.

Another alternative is to additionally distribute baremetal-operator as an optional
operator with OLM. It's probably not a good idea, but worth enumerating as an alternative.
The main downsides are the complexity of releasing and distributing the same project
two different ways, and the potential for install-time confusion or conflict over
which method should be used to install it.
