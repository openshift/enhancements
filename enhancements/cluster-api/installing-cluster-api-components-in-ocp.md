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
This feature is currently only available in Tech Preview clusters.

## Motivation

The benefits of this changes include:
* Reduced potential for using the CAPI installer as a means of privilege escalation to Cluster Admin
* Simplified handling of manifests too large to be nested into a ConfigMap
* Simplified CAPI manifest generation and review
* Move provider specific manifest generation (almost) entirely into CAPI provider repos

Additionally, the change supports a reimplementation of the CAPI installer controller to enable:
* Phased installation of CAPI providers, with gates between phases
* Ability to temporarily pin CAPI providers to a previous cluster version
* Ability to remove assets previously installed by a CAPI provider
* Ability to support generation of CRD Compatibility Requirements for unmanged CRDs

### User Stories

As an administrator I want to _have a CAPI-on-OpenShift architecture as minimally complicated as possible_ so that I can _easily understand what is going on and debug potential issues on my cluster_.

As an administrator I want to _have a CAPI-on-OpenShift architecture as minimally complicated as possible_ so that I can _extend my cluster by deploying a custom provider_.

As an OpenShift engineer I want to _have a CAPI-on-OpenShift architecture as minimally complicated as possible_ so that I can _more easily maintain and extend it_.

As an OpenShift engineer I want to _have a way to atomically apply a change to any provider_ so that I can avoid _payload breakages_.

As an OpenShift engineer I want to _have a way to load and customize provider manifests before applying them_ so that I can _template the manifests payload with image references and other runtime tweaks_.

As an OpenShift engineer I want to _have a way to load and customize provider manifests before applying them_ so that I can _future proof the system_ to be able to give users a way to selectively deploy one or more providers, even ones that are not matching the running platform.

## Proposal

We will implement a series of steps which allow us to transition smoothly from the current implementation to a new implementation without temporarily breaking the payload (see the **Interim flow** section below).
In summary:

* Add support for image-based CAPI manifests to the CAPI installer
* Update the manifests-gen tool to generate image-based manifests and metadata
* Update all providers to use image-based manifests
* Re-implement the CAPI installer, with the new version supporting only the new format

The new installer will use [Boxcutter](https://github.com/package-operator/boxcutter).
The installer controller will be split into 2 controllers:
* The revision controller creates 'Revisions': a desired set of manifests to be installed
* The installer controller installs and deletes revisions which were created by the revision controller

An installer necessarily requires considerable privileges.
For improved security the installer controller will run in the separate `openshift-cluster-api-operator` namespace with its own RBAC and service account.
This allows us to greatly reduce the RBAC of the CAPI migration and sync controllers, which will continue to run in the `openshift-cluster-api` namespace.

## Transition from 'Transport ConfigMaps'

Although never released, a previous version of the CAPIO Operator used a scheme of 'Transport ConfigMaps' where manifests were stored in ConfigMaps in the `openshift-cluster-api` namespace.
The ConfigMaps themselves were installed by CVO.
The provider manifests they contained were installed by the CAPI Operator as required.

Although these scheme has never been released and we do not need to consider upgrades, it is currently deployed in TechPreview clusters, and all currently supported CAPI providers currently use this scheme.
To avoid disruption to TechPreview installations until all providers have been updated to use the new scheme, we will minimally enhance the current installer to support an interim state where both ConfigMaps and image-based manifests are present.
This will not be supported for longer than necessary during the transition period.
Specifically, the new installer controller described in this document will not support this interim state.

### Current flow

Build time:
* A periodic ProwJob invokes [RebaseBot](https://github.com/openshift-eng/rebasebot)
* RebaseBot rebases the downstream repo if required
* RebaseBot invokes [manifests-gen](https://github.com/openshift/cluster-capi-operator/tree/main/manifests-gen) to generate a transport ConfigMap containing the provider manifests with some OpenShift-specific modifications and some provider-specific modifications
* The transport ConfigMap is added to the Cluster Version Operator (CVO) manifests in the provider image

Runtime:
* CVO installs the transport ConfigMap
* CAPI installer selects a set of transport ConfigMaps to install based on metadata in labels
* CAPI installer applies all manifests contained in its selected set of transport ConfigMaps

### New flow

Build time:
* A periodic ProwJob invokes [RebaseBot](https://github.com/openshift-eng/rebasebot)
* RebaseBot rebases the downstream repo if required
* RebaseBot invokes [manifests-gen](https://github.com/openshift/cluster-capi-operator/tree/main/manifests-gen) to generate a file containing the provider manifests with some OpenShift-specific modifications, and a metadata file
* The manifests and metadata file are added to the provider image in the `/capi-operator-manifests` directory

Differences:
* `manifests-gen` no longer implements custom go logic to do provider-specific modifications to manifests.
  These modifications move from `manifests-gen` custom logic, which is in the `cluster-capi-operator` repo, to kustomize patches in the provider repo's which requires them. `manifests-gen` already invokes `kustomize` under the hood so no extra changes are going to be required.
* `manifests-gen` writes CAPI Operator assets (defined in more detail below) instead of a CVO asset.

Runtime:
* At startup, CAPI installer reads a list of provider images associated with the current OpenShift release
* CAPI installer pulls these images and extracts manifests and metadata from the `/capi-operator-manifests` directory
* The revision controller creates a new Revision that references all manifests relevant to the current cluster
* The installer controller installs the new Revision using Boxcutter
* Once successful, the installer controller deletes orphaned artifacts associated with previous Revisions

### Interim flow

This flow is expected to exist only temporarily for a period of a few weeks at most.
Its purpose is to avoid a 'flag day' when changing the metadata format supported by the CAPI installer.
It is not expected to be included in any release.

Build time:
* Providers use either the current or new flows as described above

Runtime:
* CVO installs transport ConfigMaps for providers which include them
* At startup, CAPI installer reads a list of provider images associated with the current OpenShift release
* CAPI installer pulls these images and extracts manifests and metadata from the `/capi-operator-manifests` directory
* CAPI installer selects a set of transport ConfigMaps to install based on metadata in labels
* CAPI installer select a set of image-based provider manifests relevant to the current cluster
* CAPI installer applies all manifests selected by either method

### API Extensions

This change is supported by a new `ClusterAPI` operator config CRD, proposed in [openshift/api#2564](https://github.com/openshift/api/pull/2564).

An example `ClusterAPI` in use defining 2 `Revision`s:

```yaml
apiVersion: operator.openshift.io/v1alpha1
kind: ClusterAPI
metadata:
  name: cluster
spec:
  unmanagedCustomResourceDefinitions:
  - machines.cluster-api.x-k8s.io
status:
  currentRevision: 4.22.5-0d2d314-1
  desiredRevision: 4.22.6-873bdf9-2
  revisions:
  - name: 4.22.5-0d2d314-1
    revision: 1
    contentID: 0d2d3148cd1faa581e3d2924cdd8e9122d298de41cda51cf4e55fcdc1f5d1463
    components:
    - image:
        digest: quay.io/openshift/cluster-api@sha256:00000000...
        profile: default
    - image:
        digest: quay.io/openshift/cluster-api-provider-aws@sha256:00000000...
        profile: default
  - name: 4.22.6-873bdf9-2
    revision: 2
    contentID: 873bdf9a2a6a324231a06ce04b4d52f781022493ca0480bfb2edcb8d22ae1c9b
    unmanagedCustomResourceDefinitions:
    - machines.cluster-api.x-k8s.io
    components:
    - image:
        digest: quay.io/openshift/cluster-api@sha256:11111111...
        profile: default
    - image:
        digest: quay.io/openshift/cluster-api-provider-aws@sha256:11111111...
        profile: default
```

In this example, CAPI installer is installing the `cluster-api` and `cluster-api-provider-aws` components.
We have just upgraded from 4.22.5 to 4.22.6, which included new images for both components.
Additionally, the user has indicated that `machine.cluster-api.x-k8s.io`, which was formerly managed by CAPI, will now be independently managed.
The 4.22.5 manifests are currently applied.

### Provider manifest format

Manifests are embedded in provider images under the `/capi-operator-manifests` directory.
A provider image may contain arbitrarily many 'profiles', each with its own manifests and metadata.
A profile is located at `/capi-operator-manifests/<profile name>/` in the provider image.
e.g. the `default` profile is located at `/capi-operator-manifests/default/`.

The installer will process every profile in every provider image.
The purpose of implementing multiple profiles is to enable future selection of different manifests based on FeatureGates.
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

### Risks and Mitigations

#### Use as a vector for privilege escalation

The CAPI installer needs enough privilege to install arbitrary providers, including their RBAC.
With the ability to install RBAC, the CAPI installer is capable of granting privilege equivalent to Cluster Admin to any actor able to influence which manifests are installed, or to exploit some bug granting them the privileges of the CAPI installer.

With this redesign, we mitigate this risk in several ways.
Firstly the installer now runs in a separate deployment to the other functions of the CAPI operator including the migrations and sync controllers.
The installer runs in a separate namespace with a separate service account and separate RBAC.
This means:
* The migration and sync controllers no longer need highly privileged RBAC.
* Users who need to perform machine operations no longer require any privileges in the installer namespace.

The installer now scans a fixed set of images for manifests.
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
- Not using Upstream Cluster API Operator (which uses the default clusterctl contract) to apply the CAPI providers might require extra downstream work in the future to adapt the manifests before applying them if their format/assumptions change. Worth noting here that clusterctl currently handles API storage version changes,
which will now need to be handled differently. Within OpenShift we can make use of the [kube-storage-version-migrator-operator](https://github.com/openshift/cluster-kube-storage-version-migrator-operator) crafting a [migration request](https://github.com/kubernetes-sigs/kube-storage-version-migrator/blob/60dee538334c2366994c2323c0db5db8ab4d2838/pkg/apis/migration/v1alpha1/types.go#L30)
to easily handle one off migrations of CAPI CRDs storage version, later tombstoning the migration request manifest.


## Design Details

### Open Questions

N/A - All addressed up until now.

### Test Plan

We plan to rely on the [existing E2Es](https://github.com/openshift/cluster-capi-operator/tree/main/e2e) which already cover the use cases we for the components we are refactoring. We plan to revisit them and add more where necessary.

### Graduation Criteria
#### Dev Preview -> Tech Preview
N/A

#### Tech Preview -> GA
N/A

#### Removing a deprecated feature
N/A

### Upgrade / Downgrade Strategy

This will be covered in a further enhancement about CAPI lifecycle on OpenShift.

### Version Skew Strategy

This will be covered in a further enhancement about CAPI lifecycle on OpenShift.

### Operational Aspects of API Extensions
N/A

#### Failure Modes

- Failure of generating the manifests at build time
  - This would result in a Rebase Bot step failure, resulting in a missing rebase PR
  - the Periodic Prow Job will be retried on failure.
    Also we already have a notification mechanism in place in the RebaseBot, that will alert us on Slack on failures and their related reasons

- Failure to apply manifests on installation
  - During installation, CAPI installer fails to install manifests for the current cluster.
  - This would result in an inability to provision Machines using Cluster API.
  - The `cluster-api` ClusterOperator will be marked Degraded with a Message indicating an installation failure.
  - The CAPI installer logs will contain details of the failure.

- Failure to apply manifests on upgrade
  - CAPI components are already installed.
    The cluster has been upgraded.
    The CAPI installer fails to apply the new manifests
  - Depending on the nature of the failure, it is possible that the CAPI components from the previous version will continue to operate correctly.
  - However, if the failure is because an upgraded Deployment has entered a crash loop, it will not be possible to provision new Machines in the cluster.
  - Either way, the `cluster-api` ClusterOperator will be marked Degraded with a Message indicating an installation failure.
  - The CAPI installer logs will contain details of the failure.

- Failure to apply revision after some components successfully applied
  - While applying a revision, some components were installed but one failed.
  - Depending on the nature of the failure, the component that failed to apply may or may not be in a working state.
  - If it is not in a working state, this will most likely result in an inability to provision new Machines using Cluster API.
  - The `cluster-api` ClusterOperator will be marked Degraded with a Message indicating an installation failure.
  - The CAPI installer logs will contain details of the failure.

#### Support Procedures

To detect the failure modes in a support situation, admins can look at the cluster-capi-operator logs, and at its status and conditions during operations.

## Implementation History

* CAPI on OCP enhancements from the past:
  * [Cluster API Integration](https://github.com/openshift/enhancements/blob/master/enhancements/machine-api/cluster-api-integration.md)


## Alternatives (Not Implemented)

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

## Reference

### Invoking manifests-gen

`manifests-gen` is typically invoked in a provider repo from a `ocp-manifests` target in `openshift/Makfile`.
For example, the invocation for cluster-api-provider-aws might look like this:
```make
.PHONY: ocp-manifests
ocp-manifests: | $(RELEASE_DIR) ## Builds openshift specific manifests
        # Generate provider manifests.
        cd tools && $(MANIFESTS_GEN) --base-path "../../" --manifests-path "../capi-operator-manifests" --kustomize-dir="openshift" \
                --provider-name cluster-api-provider-aws \
                --provider-type infrastructure \
                --provider-version "${PROVIDER_VERSION}" \
                --provider-image-ref registry.ci.openshift.org/openshift:aws-cluster-api-controllers \
                --platform AWS \
                --protect-cluster-resource awscluster
```

`base-path` indicates the root directory of the repo relative to the current working directory, which in this case is `openshift/tools`.

`manifests-path` specifies the directory where the manifests should be written.

`kustomize-dir` is a directory relative to the root directory, containing a `kustomization.yaml` which will be used to generate the manifests.
The form of this `kustomization.yaml` is expected to be common across all providers.
An example of the common parts is given below.

`provider-name`, `provider-type`, `provider-version`, and `platform` are all written as metadata.
This metadata is used by the installer to determine which manifests to install.

`provider-image-ref` is the image reference of the provider image, as referenced by the generated manifests.
The installer will substitute this reference with the actual release image during installation.
This must use the `registry.ci.openshift.org` registry.
`manifests-gen` will return an error if it detects any image reference which does not use `registry.ci.openshift.org`.

`protect-cluster-resource` indicates the name of the infrastructure cluster resource type which the CAPI operator will create for this provider.
`manifests-gen` will generate a VAP for this resource to ensure that it cannot be modified.

Each provider is expected to define a `kustomization.yaml` with a form similar to:

```yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

components:
- tools/vendor/github.com/openshift/cluster-capi-operator/manifests-gen

resources:
- ../config/default

images:
- name: gcr.io/k8s-staging-cluster-api-aws/cluster-api-aws-controller
  newName: registry.ci.openshift.org/openshift
  newTag: aws-cluster-api-controllers
```

It must include the kustomize `Component` provided in the `manifests-gen` go module.
If the `tools` module uses vendoring, this can be included directly as shown above.
If the `tools` module does not use vendoring, this will have to be dynamically substituted with the location of the `manifests-gen` go module using an appropriate invocation of `go list`.

Assuming the upstream provider uses the typical `kubebuilder` scaffolding, it should include `config/default` from the upstream repo as the base resource.

Whatever value the upstream manifests use as the default image reference should be substituted with a new image reference in `registry.ci.openshift.org`.

If a provider requires any provider-specific modifications to the upstream manifests, they should also be included in this `kustomization.yaml`.
The standard modifications made by `manifests-gen` are detailed below.

### Standard modifications made by manifests-gen

`manifests-gen` makes the following set of modifications to provider manifests automatically.

* Set the namespace of all namespaced objects to `openshift-cluster-api`
* Replace cert-manager with OpenShift Service CA:
  Upstream CAPI providers typically include `cert-manager` metadata and manifests for webhooks.
  `manifests-gen` will automatically translate `cert-manager` manifests and metadata to use OpenShift Service CA instead.
* Exclude `Namespace` and `Secret` objects from the manifests: we expect these to be created by other means.
* The following set of changes to all Deployments:
  * Set the annotation `target.workload.openshift.io/management: {"effect": "PreferredDuringScheduling"}`.
  * Set resource requests of all containers to `cpu: 10m` and `memory: 50Mi`.
  * Remove resource limits from all containers.
  * Set the terminationMessagePolicy of all containers to `FallbackToLogsOnError`.
