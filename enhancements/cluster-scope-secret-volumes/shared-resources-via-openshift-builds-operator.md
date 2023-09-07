---
title: shared-resources-via-openshift-builds-operator
authors:
  - "@adambkaplan"
  - "@gabemontero"
reviewers: # Include a comment about what domain expertise a reviewer is expected to bring and what area of the enhancement you expect them to focus on. For example: - "@networkguru, for networking aspects, please look at IP bootstrapping aspect"
  - "@cdaley"
  - "@rgormley"
approvers: # A single approver is preferred, the role of the approver is to raise important questions, help ensure the enhancement receives reviews from all applicable areas/SMEs, and determine when consensus is achieved such that the EP can move forward to implementation.  Having multiple approvers makes it difficult to determine who is responsible for the actual approval.
  - "@bparees"
api-approvers: # In case of new or modified APIs or API extensions (CRDs, aggregated apiservers, webhooks, finalizers). If there is no API change, use "None"
  - "@bparees"
creation-date: 2023-08-17
last-updated: 2023-08-22
tracking-link: # link to the tracking ticket (for example: Jira Feature or Epic ticket) that corresponds to this enhancement
  - https://issues.redhat.com/browse/RHDP-701
see-also:
  - "enhancements/cluster-scope-secret-volumes/csi-driver-host-injections.md"
  - "enhancements/cluster-scope-secret-volumes/shared-resource-validation.md"
replaces:
  - "/enhancements/subscription-content/subscription-injection.md"
superseded-by: []
---

# Shared Resources via OpenShift Builds Operator

## Summary

Remove the Shared Resources CSI driver from OpenShift, and deliver it to customers with the
forthcoming OpenShift Builds OLM Operator.


## Motivation

The Shared Resources CSI driver provides a novel way for sharing information across OpenShift
namespaces. Though there was hope this capability would be generally useful, in practice this
feature is mainly needed for Red Hat build workloads.

Packaging this driver with the forthcoming OpenShift Builds operator provides several advantages:

- Provides an extension to OpenShift Builds, which is based on the upstream [Shipwright](https://shipwright.io) project.
- Allows release outside of OpenShift's cadence.
- Potential to fully support customers on older versions of OCP.
- Pathway to adoption on hosted control plane clusters. The original GA of this component was
  blocked due to cumbersome work-arounds needed for the CSI driver's associated webhooks.
- Improve general composability of OpenShift (see [OCPSTRAT-841](https://issues.redhat.com/browse/OCPSTRAT-841)).


### User Stories


- As a developer installing RHEL RPMs in my container image, I want to use the Shared Resource CSI
  driver to mount my cluster's entitlement into my build.
- As a cluster admin of a build/CI cluster, I want to manage the Shared Resource CSI driver via an
  operator so that its installation, custom resources, and lifecycle are managed by OLM.
- As an SRE operating hosted control plane clusters, I want the Shared Resource CSI driver to be
  managed by OLM so that its admission webhooks are not run on the hosted control plane.
- As a Red Hat software engineer, I want to release the Shared Resource CSI Driver via OLM so that
  I can fully support the driver on more versions of OCP.


### Goals

- Deploy the Shared Resource CSI Driver via OLM
- Deprecate and remove the Shared Resource CSI Driver as a tech preview feature in OpenShift.


### Non-Goals

- Automatically mount RHEL entitlements into builds.
- Deploy OLM operator webhooks on hosted control planes.
- Describe the low level implementation of the OpenShift Builds Operator.
- Management of the Build capability in OCP.


## Proposal

The `CSIDriverSharedResource` feature will be officially deprecated in OCP 4.15, and removed fully
in OCP 4.16. A blog post will announce this deprecation and its replacement with the OpenShift Builds operator.

The OpenShift Builds operator will be enhanced to support the lifecycle and management of the
Shared Resource CSI Driver and its associated components. This will be in addition to the
lifecycle management of Shipwright, whose logic is implemented in the upstream
[Shipwright Operator](https://github.com/shipwright-io/operator).


### Workflow Description

1. Cluster admin installs the OpenShift Builds operator via OLM, using one of the following:
   1. OperatorHub web console in OpenShift.
   2. Creating an appropriate OLM `Subscription` object via `oc apply`, OpenShift GitOps, and so
      forth.
2. Once the operator is deployed, it creates an `OpenShiftBuild` custom resource instance on the
   cluster with the following specification:

   ```yaml
   apiVersion: operator.build.openshift.io/v1alpha1
   kind: OpenShiftBuild
   metadata:
     name: cluster
   spec:
     shipwright:
       build:
         managementState: Enabled
     sharedResources:
       managementState: Enabled
   ```

3. Each component has a `managementState` field that indicates if it should be disabled, enabled,
   or removed. These states implement the following state machine:
   1. `Disabled` components can only transition to the `Enabled` state.
   2. `Enabled` components can only transition to the `Removed` state.
   3. `Removed` components can only transition to the `Enabled` state.
3. The OpenShift Builds Operator reconciles the `OpenShiftBuild` instance (singleton), and reports
   an appropriate status. If `spec.sharedResources.managementState` is `Enabled`, the operator deploys the
   Shared Resource CSI driver, custom resource definitions, and webhook.
4. The operator will likewise deploy and manage Shipwright components using the `spec.shipwright.*`
   fields, respecting the `managementState: Enabled|Disabled|Removed` value for each.
   Reconciliation will reuse upstream Shipwright operator CRDs and controllers.
5. **The operator will enable Shipwright Builds and the Shared Resources CSI driver by default.**


### Variations

#### Shipwright Builds

- To disable Shipwright Builds by default, set the `SHIPWRIGHT_BUILD_DEFAULT_DISABLED` envionrment
  variable to `true` in the operator `Subscription` object:

  ```yaml
  kind: Subscription
  apiVersion: operators.coreos.com/v1alpha1
  metadata:
    name: openshift-build
    namespace: openshift-operators
  spec:
    ...
    config:
      env:
        - name: SHIPWRIGHT_BUILD_DEFAULT_DISABLED
          value: "true"
  ```

- If OpenShift Pipelines is not installed and `spec.shipwright.build.managementState` is set to
  `Enabled`, the operator does not install Shipwright Builds and reports that OpenShift Pipelines
  should be installed in its status.
- If Shipwright Builds is enabled and then later disabled, the following should occur:
  - Shipwright build controllers should be deleted.
  - CRDs and associated admission webhooks should remain in place.


#### Shared Resource CSI Driver

- To disable the Shared Resource CSI Driver by default, set the `SHARED_RESOURCES_DEFAULT_DISABLED`
  environment variable to `true` in the operator `Subscription` object:

  ```yaml
  kind: Subscription
  apiVersion: operators.coreos.com/v1alpha1
  metadata:
    name: openshift-build
    namespace: openshift-operators
  spec:
    ...
    config:
      env:
        - name: SHARED_RESOURCES_DEFAULT_DISABLED
          value: "true"
  ```

- If the CSI driver is disabled after it has been enabled, the following should occur:
  - The `CSIDriver` object for the driver should be deleted.
  - The `DaemonSet` for the CSI driver should be deleted.
  - CRDs and associated admission webhooks should remain in place.


### API Extensions

The OpenShift Builds will introduce a new configuration CRD, `OpenShiftBuild`. It will contain 
`spec` fields that configure the Shared Resource CSI driver in addition to Shipwright-related
components. The operator will also deploy (directly or indirectly) the following:

- Custom resources for the Shipwright Operator
- Custom resources and webhooks for Shipwright Build
- Custom resources and webhooks for the Shared Resource CSI Driver


### Implementation Details/Notes/Constraints [optional]

#### Update domain name for Shared Resource CSI Driver

The root domain/api group used for the Shared Resource CSI driver components will be changed from
`sharedresource.openshift.io` to `sharedresource.dev`, impacting the registered name of the CSI
driver and resource CRDs. To keep the existing feature in OCP, the Shared Resource CSI driver code
may need to be forked to a new GitHub repo/organization.


#### Dependency Resolution with OpenShift Pipelines

Notably, the design of this operator does **_not_** intend to use OLM API dependency resolution
to automate the deployment and management of OpenShift Pipelines. Cluster admins will need to
separately (and manually) install OpenShift Pipelines. This ensures that OpenShift Pipelines is not
installed if a cluster admin does not want Shipwright Builds.


### Risks and Mitigations

#### Duplicate CSI Drivers

On a tech preview cluster, the same CSI driver could be deployed twice on tech preview clusters.
Changing the root domain of the driver and CRDs avoids this problem - the driver and CRDs have
different names/APIGroups, and this are distinct entities in OpenShift. The driver DaemonSet should
likewise not collide, since it will be deployed in a different namespace.


#### Unwanted components/CRDs

Admins or users may want to use the Shared Resource CSI Driver without Shipwright or Tekton - for
example, developers or teams that heavily rely on the `BuildConfig` APIs today. To address this
concern, the OpenShift Builds operator will not rely on OLM API resolution to automate the
installation and deployment of OpenShift Pipelines. The operator will instead report an appropriate
status message if `spec.shipwright.build.enabled` is set to `true` and it detects that OpenShift
Pipelines has not been installed.

The operator will also allow admins to opt out of enabling Shipwright Builds and/or the Shared
Resource CSI Driver through environment variable declarations in the `Subscription` object. Doing
so blocks the default creation of the CRDs for the Shared Resource CSI driver and Shipwright.


### Drawbacks

- Shared resources no longer a part of OpenShift, impacting groups that had hypothetical use cases.
- Customers who need entitlements in builds will need to install a new operator, or resort to
  current cumbersome work-arounds. OCP will not support this feature "out of the box."
- Potential impact on Red Hat components that need shared resources and/or RHEL entitlements.
- Manual installation of OpenShift Pipelines is required to deploy Shipwright Builds. Shipwright's
  upstream operator currently installs Tekton via [OLM API dependency resolution](https://olm.operatorframework.io/docs/concepts/olm-architecture/dependency-resolution/).
  This proposal will deviate from the "upstream" behavior to support customers who want the Shared
  Resource CSI Driver, but are not interested in Shipwright or Tekton.


## Design Details


### Test Plan

- Shared resource logic in the OCP Builds [tech preview suite](https://github.com/openshift/origin/blob/master/test/extended/builds/volumes.go#L134)
  will need to be migrated to the OpenShift Builds operator test suites.
- OpenShift Builds operator tests will need an OCP techpreview variant to ensure the Shared Resource
  CSI drivers (plural) can coexist. This variant can be removed once OCP 4.15 reaches end of life.
- Tests for [inline CSI volumes](https://github.com/openshift/origin/blob/master/test/extended/storage/inline.go)
  will need to updated to either:
  - Utilize a different driver that supports CSI inline (ephemeral) volumes.
  - Install the OpenShift Builds operator prior to test execution.


### Graduation Criteria

#### Dev Preview -> Tech Preview

N/A

#### Tech Preview -> GA

N/A

#### Removing a deprecated feature

- Deprecation announcement for OCP 4.15
- Removal of `CSIDriverSharedResource` feature for OCP 4.16


### Upgrade / Downgrade Strategy

No concerns for upgrade/downgrade, as `CSIDriverSharedResource` is a tech preview feature and does
not support upgrades.


### Version Skew Strategy

OpenShift Builds operator will be responsible for managing upgrades of the csi driver.


### Operational Aspects of API Extensions

API extensions are currently managed by OLM and the operator. Webhooks related to these
custom resources are the responsibility of the respective operator or operand, and generally should
not validate pod admission.

The exception is the pod admission webhook for the Shared Resource CSI driver, which has been
discussed previously (see [shared resource validation](enhancements/cluster-scope-secret-volumes/shared-resource-validation.md)).


#### Failure Modes

Out of scope - will be described by the OpenShift Builds operator design.


#### Support Procedures

Out of scope - will be described by the OpenShift Builds operator documentation.


## Implementation History

TBD


## Alternatives


### Remain in OCP

The CSI driver could remain in OpenShift, but would require significant changes to its validating
admission webhook. Validating admission webhooks proved challenging to deploy with hosted control
planes ("hypershift") when deployed in the OCP payload. Support for these webhooks exists today for
OLM operators, as these run in the "guest" cluster and are not part of the hosted control plane.
Future enhancements may allow OLM operators to generally deploy webhooks onto the hosted control plane.

Replacing the current validating webhook with
[CRD CEL expressions](https://kubernetes.io/docs/tasks/extend-kubernetes/custom-resources/custom-resource-definitions/#validation-rules)
(available in OCP 4.12) would prove to be challenging for the following reasons:

- The webhook validates pod admission to check that `readOnly: true` is set on the referenced
  volume. This is mainly to improve user experience, and could be removed due to scalability
  concerns.
- The webhook ensures that Secrets and ConfigMaps in any `openshift-*` namespace are not shared,
  with the exception of those explicitly allowed by the driver webhook's
  [configuration](https://github.com/openshift/csi-driver-shared-resource-operator/blob/master/assets/webhook/deployment.yaml#L37).


### Deploy Shared Resources as a Separate Operator

Rather than bundle the Shared Resources CSI driver with OpenShift Builds, the driver could be
deployed with its own OLM operator. The primary issue with this approach is that OpenShift Builds
would still want to deploy and manage this driver, since this is part of our strategy to make use
of entitlements in builds simpler. This would likely add a lot of unnecessary complexity:

- An additional operator CRD to reconcile (ex: `SharedResourceConfig`)
- Inability to manage which version of the Shared Resource CSI Driver is installed. Operators have
  limited ability to declare the exact version of a dependant operator (especially when APIs are
  used for dependency resoution).
- Separate deployment/management of the CSI driver, with added "Day 2" actions required by cluster
  admins.

The OpenShift Builds operator already has to address these complexities for Tekton, since it has
an indirect dependency on the OpenShift Pipelines operator. Adding another operator that has loose
management in OLM would significantly expand the potential support matrix.


## Infrastructure Needed [optional]

- New GitHub repository (and organization?) for the Shared Resource CSI driver.
- New GitHub repository for the OpenShift Builds operator (likely to live in the `redhat-developer`
  org).
- CI infrastructure - either reuse the existing OpenShift CI, "dogfood" with its own CI cluster, or
  something in between (ex: share with OpenShift Pipelines dogfooding CI).
