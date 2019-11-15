---
title: simplify-olm-apis
authors:
  - "@njhale"
reviewers:
  - "@ecordell"
  - "@dmesser"
approvers:
  - TBD
creation-date: 2019-09-05
last-updated: 2019-10-02
status: provisional
see-also:
  - "http://bit.ly/rh-epic_simplify-olm-apis"
---

# simplify-olm-apis

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

This enhancement iterates on OLM APIs to reduce the number of resource types and provide a smaller surface by which to manage an operator's lifecycle. We propose adding a new cluster-scoped resource type to OLM that acts as a single entry point for the installation and management of an operator.

## Motivation

Operator authors perceive OLM/Marketplace v1 APIs as difficult to use. This is thought to stem from three primary sources of complexity:

1. Too many types
2. Redundancies (e.g. OperatorSource and CatalogSource)
3. Effort to munge native k8s resources into OLM resources (e.g. Deployment/RBAC to CSV)
4. Configuration of operator permissions (i.e how do we gate which operators are installed to a cluster and what permissions they are given)

Negative perceptions stunt community adoption and cause a loss of mindshare, while complexity impedes product stability and feature delivery. Reducing OLM's API surface will help to avert these scenarios by making direct interaction with OLM more straightforward. A simplified user experience will encourage operator development and testing.

### Goals

- Define an API resource that aggregates the complete state of an operator
- Define a single API resource that allows an authorized user to:
  - install an operator without ancillary setup (e.g `OperatorGroup` not required)
  - subscribe to installation of operator updates from a bundle index (e.g. `CatalogSource`)
- Simplify OLM's permission model
- Be resilient to future changes
- Remain backwards compatible with operators installed by previous versions of OLM
- Retain all of OLM's current features

### Non-Goals

- Define the implementation of an operator bundle
- Describe how bundle images or indexes are pulled, unpacked, or queried
- Deprecate `OperatorSource`
- Customization/configuration of an operator

## Proposal

Introduce a new cluster scoped API resource that represents an operator as a set of component resources selected with a unique label.

At a High level, an `Operator` spec will specify:

- a source operator bundle (optional)
- a source package, channel, and bundle index (optional)
- an update policy (optional)

While its status will surface:

- info about the operator, such its name, version, and the APIs it provides and requires
- the label selector used to gather its components
- a set of status enriched references to its components
- top-level conditions that summarize any abnormal state
- the bundle image or package, channel, bundle index its components were resolved from
- the packages and channels within the referenced bundle index that contain updates for the operator

An example of an `Operator`:

```yaml
apiVersion: operators.coreos.com/v2alpha1
kind: Operator
metadata:
  name: plumbus
spec:
  updates:
    type: CatalogSource
    catalogSource:
      package: plumbus
      channel: stable
      entrypoint: plumbus.v2.0.0
      approval: Automatic
      ref:
        name: community
        namespace: my-ns

status:
  metadata:
    displayName: Plumbus
    description: Welcome to the exciting world of Plumbus ownership! A Plumbus will aid many things in life, making life easier. With proper maintenance, handling, storage, and urging, Plumbus will provide you with a lifetime of better living and happiness.
    version: 2.0.0-alpha
    apis:
      provides:
      - group: how.theydoit.com
        version: v2alpha1
        kind: Plumbus
        plural: plumbai
        singular: plumbus
      requires:
      - group: manufacturing.how.theydoit.com
        version:
        kind: Grumbo
        plural: grumbos
        singular: grumbo
  
  updates:
    available:
    - name: community
      channel: beta

  conditions:
  - kind: UpdateAvailable
    status: True
    reason: CrossChannelUpdateFound
    message: pivoting between versions
    lastTransitionTime: "2019-09-16T22:26:29Z"

  components:
   matchLabels:
      operators.coreos.com/operator:v2alpha1/plumbus: ""
   refs:
    - kind: ClusterServiceVersion
      namespace: operators
      name: plumbus.v2.0.0-alpha
      uid: d70a53b5-d06a-11e9-821f-9a2e3b9d8156
      apiVersion: operators.coreos.com/v1alpha1
      resourceVersion: 109811
      conditions:
      - type: Installing
        status: True
        reason: AllPreconditionsMet
        message: deployment rolling out
        lastTransitionTime: "2019-09-16T22:26:29Z"
    - kind: CustomResourceDefinition
      name: plumbai.how.dotheydoit.com
      uid: d680c7f9-d06a-11e9-821f-9a2e3b9d8156
      apiVersion: apiextensions.k8s.io/v1beta1
      resourceVersion: 75396
    - kind: ClusterRoleBinding
      namespace: operators
      name: rb-9oacj
      uid: d81c24d6-d06a-11e9-821f-9a2e3b9d8156
      apiVersion: rbac.authorization.k8s.io/v1
      resourceVersion: 75438
```

Introduce a new cluster-scoped API resource that will drive the installation of bundles resolved for an `Operator`:

An `Install` spec will specify:

- a list of content to install (e.g. bundle images)
- a namespace mapping used to replace namespace fields in the content
- user approval of the install plan

While its status will surface:

- a list of resolved content
- the `ServiceAccount` used to install resolved content
- top-level conditions that summarize any abnormal state
- the state of approval
- the **unsatisfied** permissions pending approval that are required to install the resolved content

An example of an `Install`:

```yaml
apiVersion: operators.coreos.com/v2alpha1
kind: Install
metadata:
  name: plumbus-plan
spec:
 namespaces:
 - from: default
   to: operators

 approval: Unapproved

 content: # must specify at least one thing to install
  - type: ManifestImage # union type
    manifestImage: quay.io/howtheydoit/plum:latest
  - type: ManifestImage
    manifestImage: quay.io/howtheydoit/bus@sha256:def456...

 status:
  serviceAccountRef:
    name: plumbus-plan
    namespace: olm

  conditions:
  - kind: PendingApproval
    status: True
    reason: UnsatisfiedPermissions
    message: pending approval of unsatisfied permissions
    lastTransitionTime: "2019-09-16T22:26:29Z"

  resolved:
  - type: ManifestImage
    manifestImage: quay.io/howtheydoit/plum@sha256:abc123...
  - type: ManifestImage
    manifestImage: quay.io/howtheydoit/bus@sha256:def456...

  pending:
    permissions:
    - apiGroups:
      - ""
      resources:
      - secrets
      verbs:
      - get
      - list
      - create
      - update
      namespaces:
      - operators
```

To simplify operator permission management, OLM will adopt a permission approval model similar to that used by Android and iOS, wherein:

- a proposed installation surfaces a set of required install permissions
- the initial installation is subject to the approval of a user w/ the required install permissions
- subsequent updates are subject to re-approval when the required install permissions exceed the set approved for its predecessors

In order to drive this UX, when an `Install` resource is created, OLM will associate it with a `ServiceAccount`, evaluate the `spec.contents` field, identify the required install permissions, and surface any permissions the `ServiceAccount` is missing via the `status.pending.permissions` field. Whenever pending permissions exist, OLM will set `spec.approval` to `Unapproved` and gate content application. To approve an installation, a user with the required install permissions must set its `spec.approval` field to `Approved`. OLM will use a `ValidatingAdmissionWebhook` to ensure a user has these permissions before allowing the field to be set. As side effect of approval, OLM will generate and bind the pending permissions to the `Install`'s `ServiceAccount`. After, OLM will attempt to apply the `Install`'s contents while authenticated as its `ServiceAccount`, repeating the approval process whenever permissions are found pending.

### User Stories

#### As a Cluster Admin responsible for configuring OLM, I want to

- restrict the resources OLM will apply from a bundle image/index when installing an operator
- restrict the bundle content that a user can install
- restrict the bundle indexes a given user can use to resolve operator bundles
- require approval for updates to operators that require more permission than earlier versions requested

#### As a Cluster Admin responsible for installing an operator, I want to

- view the state of an operator by inspecting the status of a single API resource
- deploy an operator by applying its manifests directly
- deploy an operator using a bundle image
- deploy and upgrade an operator by referencing a bundle index

#### As a Cluster User, I want to

- view the operators I can use on a cluster

### Implementation Details/Notes/Constraints

#### The `Operator` and `Install` Resources

The `operators.coreos.com/v2alpha1` API group version will be added to OLM.

The cluster-scoped resources, `Operator` and `Install` will be added to `v2alpha1`:

- cluster scoped resources can be listed without granting namespace list, but do require a `ClusterRole`
- namespace scoped resources can specify references to cluster scoped resources in their owner references, which lets us use garbage collection to clean up resources across mutliple namespaces; e.g. `RoleBindings` copied for an `OperatorGroup`

#### Component Selection

Components can be associated with an `Operator` by a label following the key convention `operators.coreos.com/operator:v2alpha1/<operator-name>`. This choice of convention has the following benefits:

- using a unique label key helps avoid collisions
- using a deterministic label key lets users know the exact label used in advance
- including the API version can help OLM handle migrations to future API iterations

The resolved label selector for an `Operator` will be surfaced in the `status.components.matchLabels` field of its status. 

```yaml
status:
  components:
    matchLabels:
      operators.coreos.com/operator:v2alpha1/my-operator: ""
```

**Note:** *Both namespace and cluster scoped resources are gathered using this label selector. Namespace scoped components are selected across all namespaces.*

Once associated with an `Operator`, a component's reference will be listed in the `status.components.resource` field of that `Operator`. Component references will also be enriched with abnormal status conditions relevant to the operator. These conditions should follow [k8s status condition conventions](https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#typical-status-properties) and in some cases may be copied directly from the component status.

```yaml
status:
  components:
    matchLabels:
        operators.coreos.com/operator:v2alpha1/my-operator: ""
    resources:
    - kind: ClusterServiceVersion
      namespace: my-ns
      name: plumbus.v2.0.0-alpha
      uid: d70a53b5-d06a-11e9-821f-9a2e3b9d8156
      apiVersion: operators.coreos.com/v1alpha1
      resourceVersion: 109811
      conditions:
      - type: Installing
        status: True
        reason: AllPreconditionsMet
        message: deployment rolling out
        lastTransitionTime: "2019-09-16T22:26:29Z"
```

Not all components will need to have status conditions surfaced. Initially, it should be sufficient to add conditions for:

- `CustomResourceDefinitions` and `APIServices`
- `Deployments`, `Pods`, and `ReplicaSets`
- `ClusterServiceVersions`, `Subscriptions`, and `InstallPlans`

### Generated Component Selection

For `v1alpha1` and `v1` OLM resource types that result in the generation of new resources, OLM will project labels onto the generated resources when:

- they match the afforementioned label key convention
- an `Operator` resource exists for the label

This means that labels will be copied from:

- `Subscriptions` onto resolved `Subscriptions` and `InstallPlans`
- `InstallPlans` onto `ClusterServiceVersions`, `CustomResourceDefinitions`, RBAC, etc.
- `ClusterServiceVersions` onto `Deployments` and `APIServices`

#### Operator Metadata

The `Operator` resource will surface a `status.metadata` field that

- contains the `displayName`, `description`, and `version` of the operator
- contains the `apis` field which in turn contains the `required` and `provided` apis of the operator

```yaml
status:
  metadata:
    displayName: Plumbus
    description: Welcome to the exciting world of Plumbus ownership! A Plumbus will aid many things in life, making life easier. With proper maintenance, handling, storage, and urging, Plumbus will provide you with a lifetime of better living and happiness.
    version: 2.0.0-alpha
    apis:
      provides:
      - group: how.theydoit.com
        version: v2alpha1
        kind: Plumbus
        plural: plumbai
        singular: plumbus
      requires:
      - group: manufacturing.how.theydoit.com
        version:
        kind: Grumbo
        plural: grumbos
        singular: grumbo
```

OLM will have control logic, such that for each `Operator` resource

- the newest successful `ClusterServiceVersion` in the operator's component selection is picked
- the pick's `displayName`, `description`, and `version` are projected onto the operator's `spec.metadata` field
- the pick's `required` and `provided` APIs are projected onto the operator's `status.metadata.apis` field

#### Installing From A `CatalogSource`

OLM will allow a user to subscribe and install changes from `CatalogSources` via the `CatalogSource` variant of the `spec.updates` [union type](https://github.com/kubernetes/enhancements/blob/master/keps/sig-api-machinery/20190325-unions.md). A `package`, `channel`, and reference to a `CatalogSource` will be used identify the bundle to install.

```yaml
updates:
  type: CatalogSource
  catalogSource:
    package: my-operator
    channel: stable
    entrypoint: plumbus.v2.0.0
    ref:
      name: community
      namespace: my-ns
```

The optional `entrypoint` field specifies the name of the first bundle in the package/channel to install. Omitting `package` and `channel` will install the `entrypoint` from `ref` without subscribing to updates.

To provide access control for `CatalogSources`, OLM will use a `ValidatingAdmissionWebhook` to ensure that only users with the `install` verb bound on a `CatalogSource` may specify it when creating or updating an `Operator`.

An example of a `Role` that permits the use of `CatalogSources` in a specific namespace and `ClusterRole` that permits use of `CatalogSources` in any namespace:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: my-catalogs-installer
  namespace: my-catalogs
rules:
- apiGroups:
  - operators.coreos.com
  resources:
  - catalogsources
  verbs:
  - install
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: all-catalogs-installer
rules:
- apiGroups:
  - operators.coreos.com
  resources:
  - catalogsources
  verbs:
  - install
```

For an `Operator` that specifies valid `CatalogSource` type updates, OLM will resolve the updated [component](https://en.wikipedia.org/wiki/Component_(graph_theory)) of the cluster-wide operator dependency graph that contains it. In other words, during the install/upgrade of a subject operator, OLM considers the install/upgrade of all operators for which an undirected dependency path connects it to the subject.

In order to enable migration from `Subscriptions` to `Operators`, OLM will attempt to associate each existing `Subscription` with an `Operator` that specifies the same `package`, `channel`, and `CatalogSource`. If no matching `Operator` already exists, OLM will generate one with a matching `spec.updates` field of type `CatalogSource`. Once an association is made, the respective `Subscription` will be deleted.

**Note:** *For `Operators` that specify a `CatalogSource` shared by a `Subscription`, but only specify an `entrypoint`, OLM will query the `CatalogSource` for the `entrypoint`'s `package` and `channel`*

Similarly, to migrate away from `OperatorGroups`, an `Install` will inherit the `ServiceAccount` of its install namespace's `OperatorGroup`. If zero or more than one `OperatorGroup` exists, then the `Install` will be assigned an eponymous `ServiceAccount` that OLM generates in the namespace of OLM's controller deployments.

A top-level status condition, `ComponentsDiverge`,  will exist that lets a user know when the components that exist on-cluster diverge from those resolved from a `CatalogSource`.

```yaml
conditions:
- type: ComponentsDiverge
  status: True
  reason: ComponentsDivergeFromInstall
  message: on-cluster components diverge from install resource
  lastTransitionTime: "2019-09-16T22:26:29Z"
```

The `PendingApproval` condition will indicate when an operator's `Install` component is pending manual approval.

```yaml
conditions:
- type: InstallPendingApproval
  status: True
  reason: PrivilegeEscalationRequired
  message: one or more installs require privilege escalation approved by an authorized user
  lastTransitionTime: "2019-09-16T22:26:29Z"
```

Specifics around the `Installs` pending approval will be surfaced in the component conditions.

```yaml
- kind: Install
  name: plumbus-plan
  uid: d70a53b5-d06a-11e9-821f-9a2e3b9d8178
  apiVersion: operators.coreos.com/v2alpha1
  resourceVersion: 109888
  conditions:
  - type: PendingApproval
    status: True
    reason: UnsatisfiedPermissions
    message: pending approval of unsatisfied permissions
    lastTransitionTime: "2019-09-16T22:26:29Z"
```

When an update is available for an operator, the `UpdateAvailable` condition will be surfaced.

```yaml
status:
   conditions:
   - kind: UpdateAvailable
     status: True
     reason: UpdateFoundInAnotherChannel
     message: an upgrade has been found outside the specified channel
     lastTransitionTime: "2019-09-16T22:26:29Z"
```

More detailed information about the package and channel that updates are available in will be specified in the `status.updates.available` field.

```yaml
status:
  updates:
    available:
      - package: my-operator
        channel: alpha
      - package: my-operator
        channel: beta
```

`CatalogSources` will be restricted for use by default. A Cluster Admin can grant access to a `CatalogSource` by binding the custom `install` verb to the desired user. This can be done for all `CatalogSources` on the cluster by using a `ClusterRoleBinding`, for all in a namespace with a `RoleBinding`, and for a specific `CatalogSource` by specifying its name in a `Role` in the same namespace.

### Installing From A Bundle Image

OLM will enable a user to install a single operator from a bundle image via the `ManifestImage` variant of the `spec.source` union type. A fully qualified image (including image registry) will be used to identify the bundle to install.

```yaml
status:
  updates:
    type: ManifestImage
    manifestImage: quay.io/my-namespace/my-operator@sha256:123abc...
```

**Note:** *"Manifest" is used in place of "Bundle" to avoid confusing users with internal jargon*

An `Install` will be generated that includes this image in its `spec`.

```yaml
spec:
  content:
  - type: ManifestImage
    manifestImage: quay.io/my-namespace/my-operator@sha256:123abc...
```

Top-level status condition `ContentImportError`, with condition reason `ManifestImagePullFailed` will be surfaced if any issue is encountered while pulling the bundle image.

```yaml
- type: ContentImportError
  status: True
  reason: ManifestImagePullFailed
  message: a manifest image pull has failed
  lastTransitionTime: "2019-09-16T22:26:29Z"
```

Like with the `CatalogSource` variant, an `Install` will be used to both apply bundle contents and provide a content record for reference. The top-level status condition `ComponentsDiverge` will also be used, but with a different condition reason; `ComponentsDivergeFromResolvedContent`.

```yaml
conditions:
- type: ComponentsDiverge
  status: True
  reason: ComponentsDivergeFromResolvedContent
  message: on-cluster components diverge from resolved content
  lastTransitionTime: "2019-09-16T22:26:29Z"
```

#### Deprecate Copied `ClusterServiceVersions`

OLM will clean up copied `ClusterServiceVersions`. When a copy is encountered, OLM will check if the original is associated with an `Operator` and:

- if so, add `OwnerReferences` to that `Operator` on all resources in the namespace owned by the copy
- else, ignore

After an associate is made, OLM will delete the copy.

**Note:** *[OpenShift console](https://github.com/openshift/console) depends CSV copying and must be migrated to the new API resources before this feature can be deprecated*

#### Raw Manifests

To install manifests without a bundle image, the `Install` resource will allow a `Raw` content type.

```yaml
content:
- type: Raw
  raw: |-
    apiVersion: v1
    kind: Pod
    metadata:
      name: nginx
      namespace: operators
      labels:
        name: nginx
    spec:
      containers:
      - name: nginx
        image: nginx
        ports:
        - containerPort: 80
```

**Note:** *`Raw` will use [`RawExtension`](https://github.com/kubernetes/apimachinery/blob/082230a5ffdd4ae12f1204049ffb5a87a9a0cb72/pkg/runtime/types.go#L94) in it's go type definition*

OLM can also use the `Raw` content type to augment the content of a bundle install; e.g. adding generated `Roles` and `RoleBindings`.

### Risks and Mitigations

#### Operator Info Leak

**Problem:** With operators now being represented by a cluster scoped resource, information about installed operators is available outside the scope of the namespace it's deployed into.
**Mitigation:** Make operator get and list privileged operations. `ClusterRoles` can also be created which apply only to specific resources by name.

## Design Details

### Test Plan

Any e2e tests will live in OLM's repo as per convention and will be executed separately from openshift's e2e tests.

The following features should have distinct e2e tests:

- Component selection
- Component status conditions
- Generated resource label propagation
- `Subscription` adoption
- Install from `CatalogSource`
- Install from manifest image
- Copied `ClusterServiceVersion` cleanup

### Graduation Criteria

#### Dev Preview -> Tech Preview

- Support bundles coming from registries, and unpack them into an InstallPlan object. This means that even if all api work slips, we can still push the bundle format forward.
- Support the _observability_ via the `Operator` object -- detect installed operators and aggregate status in a useful way, but don’t yet allow user input (spec)
- Implement the `Install` resource and `Operator` input
- Sufficient test coverage
- End user documentation, relative API stability
- v2alpha1 is bumped to v2beta1
- Gather feedback from users rather than just developers

#### Tech Preview -> GA

- v2beta1 is bumped to v2
- Existing copied `ClusterServiceVersions` on a cluster are cleaned up
- More testing (upgrade, downgrade, scale)
- Sufficient time for feedback
- Feature flags removed
- Available by default

### Upgrade / Downgrade Strategy

The v2alpha1 will be disabled by default and can be turned on via a command option.

```bash
olm-operator --enable-v2alpha1
```

On upgrade, an existing cluster can continue to use OLM as it had before. OLM will infer and generate any new resources required.

Any downgrade procedure would need to take into account that using many different install `ServiceAccounts` in a single namespace is not possible in the previous version of OLM. This may require that some `Subscriptions` be moved to new namespaces since they could have different scoping requirements.

### Version Skew Strategy

The APIs being added are net-new, and only addative changes are being made to existing OLM resources. Handling skew for upgrades/downgrades is discussed in [that section](#upgrade-/-downgrade-strategy).

## Implementation History

N/A

## Drawbacks

- New API resources are required
- Listing operators becomes a privileged operation that leaks info about existing namespaces and resources
- Implict agreement that an operator is a cluster singleton

## Alternatives

We can define a namespace-scoped operator resource in addition to the cluster-scoped one. We can use the cluster-scoped resource to indicate an operator may span multiple namespaces, and a namespace scoped operator to indicate it should be installed only in a specific namespace.

We can use an aggregated API server to surface _virtual_ resources as an alternative for viewing installed operators. These resources are synthesized based on the request namespace. The drawback to this approach is that we're adding a new `APIService` to the cluster, which can cause issues with Kubernetes API discovery.
