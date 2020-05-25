---
title: manual-cloud-credentials
authors:
  - "@dgoodwin"
reviewers:
  - @joelddiaz
  - @smarterclayton
  - @abhinavdahiya
  - @derekwaynecarr
  - @soltysh
approvers:
  - @joelddiaz
  - @abhinavdahiya
  - @soltysh
creation-date: 2020-05-14
last-updated: 2020-05-14
status: provisional
---

# Support Manual Provisioning of Cloud Credentials

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

This proposal outlines a plan for fully supporting the pre-existing process for disabling the cloud-credential-operator (still running, but in a no-op mode) and manually provisioning the cloud credentials for your cluster. It allows users to avoid broken upgrades when credentials have changed, and provides them with the tools to detect and reconcile the problem.

## Motivation

The current best practice for installing OpenShift 4 involves providing openshift-install with `root` cloud credentials powerful enough to mint additional fine grained credentials for each component in the cluster that needs them. The root credentials are stored in a secret in the cluster. This mechanism allows each component to have only the permissions it needs, and the required permissions are defined in the release image and automatically upgraded with the cluster itself.

However some customers are unable or unwilling to do this and would prefer to disable automatic credential provisioning, and instead manage it themselves. It is possible to disable the cloud-credential-operator today by injecting a `ConfigMap` into the openshift-install manifests directory, along with several `Secrets` containing the cloud provider credentials the user manually configured. While this process exists today, it is not officially supported as we believe there are situations that could arise during upgrade where credentials have changed, and the upgrade will block and go into a failed state, with little visibility as to why or how it can be fixed.

### Goals

List the specific goals of the proposal. How will we know that this has succeeded?

Goals for users who have chosen to disable the cloud credential operator:

  * Users can easily mint and inject their own cloud credential secrets at install time.
  * Users will be prevented from upgrading *if* credentials have changed in an available release image update.
  * Users can upgrade without failures when the new release image contains credential changes.
  * Users have tooling at their disposal to aid them in detecting what credentials changes have occurred in a release image.
  * Users have tooling at their disposal to aid them in updating their credentials with new permissions.

### Non-Goals

What is out of scope for this proposal? Listing non-goals helps to focus discussion
and make progress.

  * Providing a mechanism to disable the cloud credential operator, this mechanism already exists.

## Proposal

### Implementation Details/Notes/Constraints [optional]

This proposal requires the CCO to begin reporting Upgradable=False in a number of scenarios, unless an admin informs us they have taken action to reconcile new permissions and it is safe to upgrade.

CLI tooling will be implemented to help users who wish to disable the CCO to perform their initial installations, update credentials prior to upgrade for release images available for update, and reconcile credentials against current CredentialsRequests in the cluster on an on-going basis. Users must take this manual step to clear the Upgradable=False condition if credentials have changed in any available update.

Cloud credential operator reconcile code will be refactored to be a little more friendly to use as a vendored library, for use in the CLI.

Note that when we say CCO is disabled, it is still running, just not minting credentials, and does not have access to the powerful "root" credentials for the cloud provider account.

### Risks and Mitigations


## Design Details

Core of this proposal will be CLI tooling which vendors code from the credentials operator and allows for single use execution of the reconcile loop. Provising CLI tooling allows users to bypass the security concerns of storing powerful cloud credentials in the cluster and instead keep them entirely locally.

### Upgradable False Condition

A field will be added to the cloud credential operator ConfigMap (possibly becoming a separate API type in another enhancement) storing a list of Updates (from ClusterVersion.AvailableUpdates) that have been reconciled, confirming that we have minted credentials and it is safe to upgrade to this release.

```yaml
  reconciledAvailableUpdates:
  - version: 4.4.1
    image: quay.io/openshift-release-dev/ocp-release:4.4.1-x86_64
  - version: 4.4.2
    image: quay.io/openshift-release-dev/ocp-release:4.4.2-x86_64
```

Entries will be added to this list by the CLI tooling below, however an administrator could do this by hand if desired.

If the CCO is enabled per normal best practice install, this field will be ignored, and should be absent.

However if CCO is disabled, it will compare the ClusterVersion.Spec.AvailableUpdates to this list of ReconciledAvailableUpdates. If *any* available update is not present in the reconciled list, the CCO will report Upgradable=False.

### Initial Installation

```bash
$ oc adm credentials reconcile --manifests=/path/to/installer/manifests
```

This command would parse the CredentialsRequests in the manifests directory, mint credentials in the cloud using the local users environment variables or credentials file, store the status on the CredentialsRequsts (which are yet to be created), write Secret manifests containing the cloud credentials, and disable the CCO.

### Upgrade Reconciliation

```bash
$ oc adm credentials reconcile --available-updates
```

This command will iterate all currently available updates in ClusterVersion.AvailableUpdates. For each given release image we will extract each CredentialsRequest and use the local users environment variables/credentials file to compare and adjust permissions as necessary.

This command reconciles permissions *in an additive fashion*. We do not want to reconcile any reduction in permissions as that would affect the running components in the cluster right now. The command would print a warning if this situation is encountered, and it can be resolved after upgrade with the next command.

For net new CredentialsRequests in the update, we have a problem. We cannot assume to store a secret in the target namespace as it may be for a component that does not exist in the currently running OpenShift release. To work around this problem we propose copying net new CredentialsRequests into the cluster (all CredentialsRequests live in the openshift-cloud-credential-operator namespace today), and introducing the concept of temporary secret storage. This would allow us to mint the credentials when we have the root creds available, grab the secrets which we typically only get at creation time and cannot retrieve later, and transfer them to the correct namespace when it is available.

Each available update we process successfully is added to the reconciledAvailableUpdates list.

### On-Going Reconciliation

```bash
$ oc adm credentials reconcile
```

This command would re-reconcile against the CredentialsRequests currently in the cluster. This is more of an on-going configuration management to ensure something doesn't change unexpectedly on us due to external factors. It would however support reducing permissions as a result of an additive only reconcile pre-upgrade.

It should be noted that running this command after reconciling --available-updates, but before updating, would undo the pre-upgrade changes. As such this command should clear the reconciledAvailableUpdates list.


## Open Questions [optional]

 > 1. How can we handle net new CredentialsRequests in incoming release images?
   * We could create the CredentialsRequest during the pre-upgrade reconciliation, but this does not guarantee that the target namespace for the component (where the Secret/ServiceAccount would be stored) exists.
   * If we were to take the incoming CredentialsRequests, mind the credentials, and store in a temporary secret in the CCO namespace, we could later transfer it to the correct namespace when it exists.
 > 1. Can the installer/CVO install manifests that contain status?
   * If not we may need to hack around this by storing details on the minted credentials prior to installation as an annotation on the CredentialsRequest, and have the CCO reconcile it to it's final location in Status once we have a live object.
   * Is it safe to edit a manifest from the release image prior to install?
 > 1. Should the reconcile against current version command (i.e. no --available-updates flag) use the current CredentialsRequests in the cluster, or by extracting the current release image? The latter would allow us to prune net new CredentialsRequests processed for availableUpdates that were never installed.

### Test Plan

**Note:** *Section not required until targeted at a release.*

Consider the following in developing a test plan for this enhancement:
- Will there be e2e and integration tests, in addition to unit tests?
- How will it be tested in isolation vs with other components?

No need to outline all of the test cases, just the general strategy. Anything
that would count as tricky in the implementation and anything particularly
challenging to test should be called out.

All code is expected to have adequate tests (eventually with coverage
expectations).

## Implementation History

Major milestones in the life cycle of a proposal should be tracked in `Implementation
History`.

## Drawbacks

Copying net new CredentialsRequests from future upgrades, minting credentials, storing them in a temporary location, and copying them to their final location during the upgrade is particularly nebulous.

Involves a significant amount of new code.

If included in "oc adm", would require vendoring and compiling basically all of cred operator so we can run the reconcile loop. (including all cloud provider libraries)

## Alternatives

The possibility of requiring second level operators to gracefully handle the absence of new permissions was discussed. However we felt we should avoid placing the onus for this on SLOs in a difficult to test scenario when we could instead more reliably offer it as part of the CCO itself.

