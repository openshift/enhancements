---
title: Credentials Management outside OpenShift Cluster
authors:
  - "@akhil-rane"
reviewers:
  - "@abhinavdahiya"
  - "@dgoodwin"
  - "@joelddiaz"
approvers:
  - "@derekwaynecarr"
  - "@sdodson"
creation-date: 2021-03-25
last-updated: 2021-03-25
status: provisional
---

# Credentials Management outside OpenShift Cluster

## Release Signoff Checklist

- [x] Enhancement is `partially implemented`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

The intent of this enhancement is to take the process of credentials management outside the OpenShift cluster for new 
platforms. We propose to make *manual* mode as default for clusters on new platforms, optionally supported by CLI tooling
that follows the best practices of the hosting infrastructure at that time.

## Motivation

The main motivation behind this enhancement is to satisfy the customer requirement to follow the best security practices 
by not storing admin credentials inside the cluster

### Goals

As part of this enhancement we plan to do the following:
* Guide new platforms to start with Manual mode rather than Mint mode.
* Provide optional tooling (ccoctl) for administrators to manage their credentials outside the cluster. (prep for install, reconcile, prep for upgrade)
* Establish precedent for future direction of preferring *Manual* over *Mint*.

### Non-Goals

* Removing Mint mode for any currently supported cloud. (AWS, Azure, GCP)
* Changing the default for any provider currently defaulting to Mint mode, though we may revisit in the future.

## Proposal

Currently, *mint* is a default credentials mode for OpenShift. In this mode we run OpenShift installer with an admin 
level cloud credential. The admin credential is stored in kube-system namespace and then used by the cloud credential 
operator to process the CredentialRequests in the cluster and create new users with fine-grained permissions.
The customers have reported that the need to store admin credentials inside the cluster is a major disadvantage. Based 
on the feedback, we propose to run credentials related setup outside a cluster and then start installation process in a 
*manual* mode without providing admin credentials to the installer.

### User Stories

#### Story 1
As an OpenShift cluster administrator I should be able to extract the CredentialsRequest manifests from the release image 
and create required manifests/infrastructure to satisfy all in-cluster component's CredentialsRequests. This has been 
already [implemented](https://github.com/openshift/cloud-credential-operator/blob/master/docs/ccoctl.md).

#### Story 2
As an OpenShift cluster administrator, I should be able to reconcile my credentials manually using local CLI tooling on 
an on-going basis. This is in [backlog](https://issues.redhat.com/browse/CCO-106) and targeted for 4.9.

#### Story 3
As an OpenShift cluster administrator, I should be able to prep credentials for a cluster upgrade. This is in [backlog](https://issues.redhat.com/browse/CCO-106) 
and targeted for 4.9.

### Risks and Mitigations

## Design Details

We intend to build an optional tool **ccoctl** which will handle credentials management of the cluster in *manual* mode. 
The following is the set of requirements for the current prototype (for AWS). We can have something similar for other 
platforms.

* ccoctl should be able to setup Identity Provider to authenticate OpenShift components
* ccoctl should be able to take a list of CredentialsRequests from the release image and create/update 
  Roles/Users/ServicePrincipals/ServiceAccounts with appropriate permissions.
* ccoctl should be able to take list of CredentialsRequests for a release image, and the Identity Provider URL to generate 
  the objects that need to be passed to the installer for successful installation
* ccoctl should be able to delete all the resources that it had created in the cloud

We envision **ccoctl** as a recommended tool to setup credentials on new platforms but customers are free to use other 
tools like Terraform/AWS CloudFormation to do the above mentioned tasks. Read more about AWS implementation details [here](https://github.com/openshift/cloud-credential-operator/blob/master/docs/ccoctl.md)

We also plan to have a detailed documentation in place to guide new cloud providers to implement *manual* mode.

To upgrade a cluster we need to execute following steps:
* Examine the CredentialsRequests in the new OpenShift release. Check if permissions in the existing CredentialsRequest
  have changed.
* Create/update credentials in the underlying cloud provider, and also create/update Kubernetes Secrets in the correct
  namespaces to satisfy all CredentialsRequests in the new release.
* CCO sets `Upgradeable=False` when in manual mode. Set an appropriate annotation `cloudcredential.openshift.io/upgradeable-to` 
  to a new upgradable version to allow upgrade.

All the above task will also be executable by *ccoctl*. Details in [this](https://issues.redhat.com/browse/CCO-84) card.

### Open Questions

### Test Plan

We plan to have a e2e test that will externally set up a credentials management infrastructure and then kickstart 
install in a *manual* mode.

### Graduation Criteria

### Upgrade / Downgrade Strategy

### Version Skew Strategy

## Implementation History

We currently have a work-in-progress CLI tool [ccoctl](https://github.com/openshift/cloud-credential-operator/blob/master/docs/ccoctl.md) 
to create and manage cloud credentials outside the cluster for AWS cloud. The design details of this tool is discussed above.

## Drawbacks

* Taking the credentials management outside the cluster will create additional overhead for the customer to make sure all
  the required infrastructure is in place before starting the installation process. Current tooling we have only supports
  the AWS cloud, we do not have anything planned for other cloud providers.
* Push-button upgrades will not work in *manual* mode as the cluster no longer has the admin credentials to mint credentials 
  (with fine-grained permissions) for in-cluster components.

## Alternatives

## Infrastructure Needed [optional]

