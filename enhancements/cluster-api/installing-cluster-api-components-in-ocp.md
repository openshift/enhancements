---
title: installing-cluster-api-components-in-ocp
authors:
  - "@mdbooth"
  - "@damdo"
  - "@nrb"
reviewers:
  - "@JoelSpeed"
  - "@damdo"
  - "@sdodson"
approvers:
  - "@JoelSpeed"
  - "@sdodson"
api-approvers:
  - None
creation-date: 2023-08-25
last-updated: 2026-01-08
tracking-link:
  - "https://issues.redhat.com/browse/OCPCLOUD-1910"
  - "https://issues.redhat.com/browse/OCPCLOUD-3344"
see-also:
  - "/enhancements/machine-api/converting-machine-api-to-cluster-api.md"
replaces:
  - "https://github.com/openshift/enhancements/blob/master/enhancements/machine-api/cluster-api-integration.md"
superseded-by:
---

# Installing Cluster API components in OpenShift

## Summary

This enhancement describes an enhancement to how Cluster API components are installed in an OpenShift cluster.
This feature is gated by the ClusterAPIMachineManagement feature gate.
As of 5.0.0 this feature gate is in the TechPreviewNoUpgrade feature set.

## Motivation

Installing CAPI has some unique requirements which stem from 3 constraints:
* It does not 'own' its operands.
  CAPI providers are separate projects with their own unique requirements.
  The CAPI operator must be able to install them all.
* It must be able to select the set of operands at runtime based on the configuration of the running cluster.
  Currently this is just the configured cloud platform.
* Other workloads, including RH products (Hypershift, ACM) and user workloads also use CAPI.
  This means that CAPI operator must allow those products and end-users to install cluster-scoped CAPI resources (specifically CRDs) which would otherwise conflict with those managed by CAPI operator.

The latter constraint is described in more detail in the [CRD Compatibility Checker enhancement](crd-compatibility-checker.md).

Architecturally, an important consequence is that, although we try to prevent it through other means, it is possible for a user to upgrade to a release where CAPI cannot be installed due to incompatible existing CRDs.
In these circumstances the CAPI operator must be able to continue to reconcile the last known working version of its operands.
This is in addition to normal desirable properties of an installer, including:
* Phased installation of components, with gates between phases ensuring the previous phase is working as expected
* Ability to remove assets previously installed by a CAPI provider

Additionally the installer must directly support CRD Compatibility Requirements be being able to replace unmanaged CRDs with Compability Requirements.

### User Stories

As an administrator I want to _have a CAPI-on-OpenShift architecture as minimally complicated as possible_ so that I can _easily understand what is going on and debug potential issues on my cluster_.

As an OpenShift engineer I want to _have a CAPI-on-OpenShift architecture as minimally complicated as possible_ so that I can _more easily maintain and extend it_.

As an OpenShift engineer I want to _have a way to atomically apply a change to any provider_ so that I can avoid _payload breakages_.

As an OpenShift engineer I want to _have a way to load and customize provider manifests before applying them_ so that I can _template the manifests payload with image references and other runtime tweaks_.

## Proposal

The CAPI installer uses [Boxcutter](https://github.com/package-operator/boxcutter).
The installer is split into 2 controllers:
* The revision controller creates 'Revisions': a desired set of manifests to be installed
* The installer controller installs and deletes revisions which were created by the revision controller

An installer necessarily requires considerable privileges.
For improved security the installer controller runs in the separate `openshift-cluster-api-operator` namespace with its own RBAC and service account.
This allows us to greatly reduce the RBAC of the CAPI migration and sync controllers, which continue to run in the `openshift-cluster-api` namespace.

### Runtime flow

* At startup, CAPI operator reads a list of provider images associated with the current OpenShift release from the `capi-installer-images` ConfigMap in `openshift-cluster-api-operator`
* CAPI operator creates or updates the `capi-installer` deployment to mount all these images, along with any images still in use from previous releases
* CAPI installer extracts manifests and metadata from the `/capi-operator-manifests` directory of mounted images
* The revision controller creates a new Revision that references all manifests relevant to the current cluster version
* The installer controller installs the new Revision using Boxcutter
* Once successful, the installer controller deletes orphaned artifacts associated with previous Revisions

### Provider metadata

The CAPI operator includes a tool for generating its required manifest metadata: [manifests-gen](https://github.com/openshift/cluster-capi-operator/tree/main/manifests-gen).
At a high level, this tool builds a set of manifests and required metadata from a [kustomization.yaml](https://kustomize.io/).
Kustomize is chosen as the base as this is the tool most commonly used by existing CAPI providers, so extending it is relatively simple.

### API Extensions

This change is supported by a new `ClusterAPI` operator config CRD, proposed in [openshift/api#2564](https://github.com/openshift/api/pull/2564).
The API definition is available at [operator/v1alpha1/types_clusterapi.go](https://github.com/openshift/api/blob/master/operator/v1alpha1/types_clusterapi.go).

An example `ClusterAPI` in use defining 2 `Revision`s:

```yaml
apiVersion: operator.openshift.io/v1alpha1
kind: ClusterAPI
metadata:
  name: cluster
spec:
  unmanagedCustomResourceDefinitions:
  - machines.cluster.x-k8s.io
status:
  currentRevision: 4.22.5-0d2d314-1
  desiredRevision: 4.22.6-873bdf9-2
  revisions:
  - name: 4.22.5-0d2d314-1
    revision: 1
    contentID: 0d2d3148cd1faa581e3d2924cdd8e9122d298de41cda51cf4e55fcdc1f5d1463
    components:
    - type: Image
      image:
        ref: quay.io/openshift/cluster-api@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
        profile: default
    - type: Image
      image:
        ref: quay.io/openshift/cluster-api-provider-aws@sha256:fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210
        profile: default
  - name: 4.22.6-873bdf9-2
    revision: 2
    contentID: 873bdf9a2a6a324231a06ce04b4d52f781022493ca0480bfb2edcb8d22ae1c9b
    unmanagedCustomResourceDefinitions:
    - machines.cluster.x-k8s.io
    components:
    - type: Image
      image:
        ref: quay.io/openshift/cluster-api@sha256:1111111111111111111111111111111111111111111111111111111111111111
        profile: default
    - type: Image
      image:
        ref: quay.io/openshift/cluster-api-provider-aws@sha256:2222222222222222222222222222222222222222222222222222222222222222
        profile: default
```

In this example, CAPI installer is installing the `cluster-api` and `cluster-api-provider-aws` components.
We have just upgraded from 4.22.5 to 4.22.6, which included new images for both components.
Additionally, the user has indicated that `machines.cluster.x-k8s.io`, which was formerly managed by CAPI, will now be independently managed.
The 4.22.5 manifests are currently applied.

### Revision handling

In the above example, the newer revision was created by the revision controller after it observed a change in the desired installer content.
At a high level, the revision controller starts from the set of provider images mounted into the installer, reads each profile's `metadata.yaml` and `manifests.yaml`, and selects only the profiles which apply to the current cluster.
In particular, platform selection comes from the cluster's `Infrastructure` status and each profile's `ocpPlatform` metadata, while component ordering comes from `installOrder` in profile metadata.
The image reference and profile recorded in each component come directly from the mounted provider image and the selected profile name.

The revision controller then renders those selected profiles into a new desired revision.
During rendering it applies the configured manifest substitutions, replaces any provider `selfImageRef` placeholder with the actual mounted image reference, and records the resulting content as a new `contentID`.
If that rendered content differs from the latest known revision, the controller creates a new revision entry with the next revision number and makes it the `desiredRevision`.
Because the user has also declared `machines.cluster.x-k8s.io` unmanaged, that intent is carried into the newer revision so that the installer can treat that CRD differently from the older revision.

The installer controller then works from the revision list in order, starting with the desired revision.
For the newer revision, it first creates an initial phase containing `CompatibilityRequirement` objects for every unmanaged CRD.
That phase gates the rest of the revision, so the installer will not continue until each unmanaged CRD is present on the cluster and is compatible with what the new revision expects.
The installer then processes the remaining content in order, stripping unmanaged CRDs from any provider profile which contains them, and applying the rest of the revision phase by phase.

At the same time, the older revision remains available as the last known working state.
Until that has happened, the older revision is not merely retained as a fallback.
The installer controller continues to actively reconcile it until the newer revision has completed successfully.
While doing so, it ignores any individual resource which has already been updated to the newer revision, so progress to the new revision can happen incrementally without the older revision trying to take those resources back.

This means that, while the upgrade is still in progress, the example can legitimately show `currentRevision` pointing to `4.22.5-0d2d314-1` and `desiredRevision` pointing to `4.22.6-873bdf9-2`.
Until the newer revision has passed its gates and been fully reconciled, the installer controller continues to rely on the older revision as the currently applied revision.
Once the newer revision has completed successfully, the installer controller marks it as the `currentRevision` and tears down content which is only needed by the older revision.

The old revision is not kept forever.
After the installer controller has successfully moved `currentRevision` to the newer revision, the revision controller will eventually observe that the latest revision is now current and remove the older revision from the status.
At that point the example would collapse back down to a single active revision representing the current desired and applied state.

### ContentID

`contentID` identifies the installer content represented by a revision.
At a high level, it covers the fully rendered content of the revision rather than just the release version or revision number.
This means it changes when the set of selected provider profiles changes, when the rendered manifests from any selected profile change, or when manifest substitutions change the content which will be applied to the cluster.

In practice, this includes changes such as selecting a different platform-specific profile, updating manifests in a provider image, or changing rendering inputs such as substitutions and image references which affect the final rendered objects.
The purpose of `contentID` is simply to let the revision controller distinguish revisions which would install materially different content, even if they are otherwise similar at a glance.

### Provider manifest format

Manifests are embedded in provider images under the `/capi-operator-manifests` directory.
A provider image may contain arbitrarily many 'profiles', each with its own manifests and metadata.
A profile is located at `/capi-operator-manifests/<profile name>/` in the provider image.
e.g. the `default` profile is located at `/capi-operator-manifests/default/`.

A `profile` is simply a manifest bundle and metadata describing when to install it.
A provider can have arbitrarily many profiles in `/capi-operator-manifests`, but typically it will only have one, called `default`.
The `default` name is purely a convention, and has no semantics associated with it.
The installer will process every profile in every provider image, and will always install every profile whose selection criteria match the current cluster.

The primary purpose of supporting multiple profiles is to enable future selection of different manifests based on FeatureGates.
It also allows a provider (e.g. the CAPI operator image itself) to have multiple different platform-specific profiles.
Note that FeatureGate support is not intended for the initial implementation.

Each profile contains 2 files.
`manifests.yaml` contains a Kubernetes Resource Model(KRM)[^krm] formatted set of resources to be installed.
`metadata.yaml` contains metadata describing the manifests.
This metadata will be used to determine whether the profile will be installed.

[^krm] The format produced by kustomize: a set of kubernetes resources in YAML format in a single file, separated by YAML document separators.

Example `metadata.yaml`:
```yaml
attributes:
  type: infrastructure
  version: v2.10.0
installOrder: 20
name: cluster-api-provider-aws
ocpPlatform: AWS
selfImageRef: registry.ci.openshift.org/openshift:aws-cluster-api-controllers
```

### Managing provider updates

As there are many CAPI providers and they all have their own release schedules, we have implemented automation to manage most provider updates.
The flow is:
* A periodic ProwJob invokes [RebaseBot](https://github.com/openshift-eng/rebasebot)
* RebaseBot rebases the downstream repo if required
* RebaseBot invokes [manifests-gen](https://github.com/openshift/cluster-capi-operator/tree/main/manifests-gen) to generate a file containing the provider manifests with some OpenShift-specific modifications, and a metadata file
* The manifests and metadata file are added to the provider image in the `/capi-operator-manifests` directory

### Risks and Mitigations

#### Use as a vector for privilege escalation

The CAPI installer needs enough privilege to install arbitrary providers, including their RBAC.
With the ability to install RBAC, the CAPI installer is capable of granting privilege equivalent to Cluster Admin to any actor able to influence which manifests are installed, or to exploit some bug granting them the privileges of the CAPI installer.

We mitigate this risk in several ways.
Firstly the installer now runs in a separate deployment to the other functions of the CAPI operator including the migrations and sync controllers.
The installer runs in a separate namespace with a separate service account and separate RBAC.

The installer scans a fixed set of images for manifests.
These images are contained in a ConfigMap in the installer namespace which is managed by CVO.
The images in this ConfigMap are all specified by digest, and are part of the release payload.
Anybody who is able to both modify this ConfigMap and restart the installer would be able to load manifests from an arbitrary image.

As future work, it may be possible to grant the installer an aggregated role with minimal initial permissions.
In-payload providers would be responsible for defining a role with sufficient privilege to install the provider's payload, and adding that to the aggregated role.
This 'installation RBAC' would be installed by CVO.
I believe CAPI uses a similar model to enable infrastructure providers to allow core CAPI components to manage their custom resources.
We do not plan to implement this feature initially, but it may be a way to useful reduce the RBAC requirements of the installer.

### Drawbacks

- Not  using CVO to manage the entirety of the CAPI providers installation means there's an extra layer of operator management within the OpenShift cluster.
  That said this is not an uncommon pattern, but rather it is the norm when it comes to perform custom deploying behaviour in OpenShift. See for example MAO, CCMO, CSO, CNO.

## Design Details

### Open Questions

N/A - All addressed up until now.

### Test Plan

We plan to rely on the [existing E2Es](https://github.com/openshift/cluster-capi-operator/tree/main/e2e) which already cover the use cases we for the components we are refactoring. We plan to revisit them and add more where necessary.

### Graduation Criteria

#### Dev Preview -> Tech Preview

N/A

#### Tech Preview -> GA

The feature gate promotion tests will include at least:
* A 'validation' test in an upgrade job ensures that CAPI components are upgraded successfully
* Test coverage of the 'unsupported upgrade' scenario

HyperShift will have integrated support for CAPI operator into their CI pipeline.
This will involve specifying all CRDs they install as unmanaged.

#### Removing a deprecated feature

N/A

### Upgrade / Downgrade Strategy

N/A

### Version Skew Strategy

N/A

### Operational Aspects of API Extensions

N/A

#### Failure Modes

- Failure to apply manifests on installation
  - During installation, CAPI installer fails to install manifests for the current cluster.
  - This would result in an inability to provision Machines using Cluster API.
  - After 5 minutes, the `cluster-api` ClusterOperator will be marked Degraded with a Message indicating an installation failure.
  - The CAPI installer logs will contain details of the failure.

- Failure to apply manifests on upgrade
  - CAPI components are already installed.
    The cluster has been upgraded.
    The CAPI installer fails to apply the new manifests
  - Depending on the nature of the failure, it is possible that the CAPI components from the previous version will continue to operate correctly.
  - However, if the failure is because an upgraded Deployment has entered a crash loop, it will not be possible to provision new Machines in the cluster.
  - Either way, after 5 minutes the `cluster-api` ClusterOperator will be marked Degraded with a Message indicating an installation failure.
  - The CAPI installer logs will contain details of the failure.

- Failure to apply revision after some components successfully applied
  - While applying a revision, some components were installed but one failed.
  - Depending on the nature of the failure, the component that failed to apply may or may not be in a working state.
  - If it is not in a working state, this will most likely result in an inability to provision new Machines using Cluster API.
  - After 5 minutes, the `cluster-api` ClusterOperator will be marked Degraded with a Message indicating an installation failure.
  - The CAPI installer logs will contain details of the failure.

#### Support Procedures

To detect the failure modes in a support situation, admins can look at the cluster-capi-operator logs, and at its status and conditions during operations.

## Implementation History

* CAPI on OCP enhancements from the past:
  * [Cluster API Integration](https://github.com/openshift/enhancements/blob/master/enhancements/machine-api/cluster-api-integration.md)

## Alternatives (Not Implemented)

* **Place provider manifests in ConfigMaps**
  The previous implementation used this strategy (we called them 'transport' ConfigMaps).
  The primary reason for no longer doing this is the size limit of a ConfigMap.
  Many provider manifest bundles already exceed this size limit.
  We implemented compression for these cases, but this broke the release image substitution mechanism of `oc adm release`.
  The alternative of having many configmaps makes guaranteeing integrity complex during an upgrade.
  Extracting them from an image does not have this limitation.

* **Extend CVO capabilities to selectively deploy CAPI providers manifests**
  * *PROs*: this would allow us to directly apply the CAPI manifests from the CAPI providers repositories `/manifests` folder by leveraging CVO capabilities
  * *CONs*: this would need to expand the scope of CVO as it will require it to have knowledge of platform(s) the cluster is running on, and would still have the caveat of not being able to deploy multiple providers that are not strictly matching the platform the cluster is running on (i.e. missing desired providers-list)

* **Handling of multiple/custom CAPI providers**
  Installing multiple CAPI infrastructure providers to be able to launch MachineSets on a different cloud provider than the one set in the `Infrastructure` Object status, has been considered a non-goal for this enhancement.
  We don't want to tackle this now, as part of this work, as it would be a non neglegible task to take on that is not justified by any immediate business needs.
  Although this is an option that we don't want to completely inhibit as a potential future enhancement.
  In the future, if desirable, this can be implemented by exposing a new Custom Resource owned by the cluster-capi-operator, which instructs the operator on what providers to deploy.

* **Perform the CAPI providers apply to the cluster, through the clusterctl pkg library**
  * *PROs*: it would ensure compatibility with the clusterctl ecosystem and format/contract
  * *CONs*: it would require a lot of extra complexity (~500 more LOC) and dependencies to achieve a similar result