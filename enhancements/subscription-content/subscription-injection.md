---
title: subscription-injection-operator
authors:
  - "@adambkaplan"
reviewers:
  - "@gabemontero"
  - "@coreydaley"
  - "@otaviof"
approvers:
  - "@bparees"
  - "@derekwaynecarr"
creation-date: 2020-06-25
last-updated: 2020-09-16
status: implementable
see-also:
  - /enhancements/cluster-scope-secret-volumes/csi-driver-host-injections.md
  - https://github.com/openshift/enhancements/pull/384
replaces: []
superseded-by: []
---

# Subscription Injection Operator

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [x] Design details are appropriately documented from clear requirements
- [x] Test plan is defined
- [x] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in
      [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

This enhancement will allow any workload to access RHEL subscription content via a pod annotation.

## Motivation

Containers can download RHEL content if their hosts have subscriptions attached. In 3.x, all nodes
were assumed to be RHEL 7 nodes that were subscribed individually. The information needed to access
subscription content (entitlement keys and `redhat.repo` configurations) was symbolically linked
into the running container at a well-known location that yum/dnf could find. This capability
existed in our patch of Docker, and was carried over into Red Hat’s container toolchain used on
OpenShift 4 (cri-o, podman, buildah, etc.).

In OpenShift 4 the preferred operating system (RHCOS) is not capable of attaching subscriptions
individually. To access subscription content, containers must be provided entitlement keys and
optionally `redhat.repo` configurations via other means. Enabling this capability needs to be
simple and straightforward.

### Goals

1. Inject entitlement keys, such as the Simple Content Access key, into a pod spec.
2. Allow overrides of the `redhat.repo` configuration for containers that need to connect to
   Satellite.
3. Define APIs and conventions that allow cluster admins to add the required information to a
   cluster (entitlement keys and `redhat.repo` configurations).
4. Allow multiple entitlements to be made available to a container.

### Non-Goals

1. Automatically install entitlement keys and `redhat.repo` configurations onto a subscribed
   cluster.
2. Automatically rotate entitlement keys and update `redhat.repo` configurations on a subscribed
   cluster.
3. Dynamically add additional entitlement keys to running containers by mounting an additional
   `Secret`

## Proposal

### User Stories

#### Add RHEL content to running containers

As a developer or application administrator
I want access to RHEL subscription content in my containers
So that I can install RHEL content within my container via `yum install`

#### Access RHEL content from a Satellite instance

As a developer using OpenShift 
I want to be able to access RHEL content from my Satellite instance
So that I download RHEL content from my Satellite instances instead of the Red Hat CDN.

### Implementation Details/Notes/Constraints [optional]

#### Installation and Setup

When the `subscription-injection-operator` is installed, it creates the subscription `Bundle` and
`ClusterBundle` custom resource definition. The operator deployment creates sample YAML files
which helps admins get started with default, cluster-wide and namespace-scoped subscription
bundles:

```yaml
apiVersion: console.openshift.io/v1
kind: ConsoleYAMLSample
metadata:
  name: subscription-cluster-bundle-example
spec:
  targetResource:
    apiVersion: subscription.openshift.io/v1
    kind: ClusterBundle
  title: Cluster-wide subscription bundle example
  description: | 
    An example of a subscription bundle that can be injected into any workload that has the
    `subscription.openshift.io/inject-cluster-bundle: cluster` annotation in its pod spec, and
    whose service account has the `edit` or `admin` role.
  yaml: |
    apiVersion: subscription.openshift.io/v1
    kind: ClusterBundle
    metadata:
      name: cluster
    spec:
      aggregateToClusterRoles:
      - edit
      - admin
      entitlements:
      - name: etc-pki-entitlement
        namespace: openshift-config
```

```yaml
apiVersion: console.openshift.io/v1
kind: ConsoleYAMLSample
metadata:
  name: subscription-namespaced-bundle-example
spec:
  title: Namespaced subscription bundle example
  description: |
    An example of a subscription bundle that can be injected into any workload in the bundle's
    namespace that has the `subscription.openshift.io/inject-bundle: subscription` annotation in
    its pod spec.
  targetResource:
    apiVersion: subscription.openshift.io/v1
    kind: Bundle
  yaml: |
    apiVersion: subscription.openshift.io/v1
    kind: Bundle
    metadata:
      name: subscription
    spec:
      entitlements:
      - name: etc-pki-entitlement
```

The `entitlements` object is a reference to the `Secrets` which contain the entitlement keys for
the cluster's subscription. The cluster administrator is responsible for obtaining the entitlement
keys and adding them to the cluster. Administrators can refer to existing guidance on how to obtain
this information.

The API specs will also include a `yumRepositories` field, which allows for a list of `ConfigMaps`
containing `.repo` configuration files that allow an entitlement to be downloaded from a Red Hat
Satellite instance.

#### Subscription Cluster Bundle Controller

When a `ClusterBundle` object is created, the Subscription Bundle Controller (a new controller)
will perform the necessary actions to make the bundle available to cluster workloads. This consists
of the following:

1. Verify that the referenced `entitlements` and optional `yumRepositories` resources exist.
2. Create the ProjectedResource CSI driver `Shares` for the `entitlements` and (optional)
  `yumRepositories` as declared in the Projected Resource CSI Driver proposal.
3. Create a `ClusterRole` associated with the bundle, which grants `get`, `watch`, and `list`
   permissions to the associated `Shares` and the `ClusterBundle` object.
4. Aggregate the cluster role to the provided list of system roles.
5. Add owner references to the generated `ClusterRole` and shares. This ensures the dependent
   objects are deleted when the `ClusterBundle` is deleted.

When a `ClusterBundle` object is updated, changes to the referenced `entitlements` and
`yumRepositories` are propagated to the associated `Share` objects. Likewise, changes to aggregated
cluster roles should be propagated to the associated `ClusterRole`. Non-terminated pods which have
the `ClusterBundle` injected are not altered - only new pods which inject the `ClusterBundle` will
receive the updated mounts for `entitlements` and `yumRepositories`.

When a `ClusterBundle` object is deleted, the owner references above will ensure deletion of the
associated objects. Non-terminated pods which inject the deleted `ClusterBundle` are not impacted.
New pods which inject the deleted `ClusterBundle` will be rejected by an admission webhook (see 
below).

#### Subscription Bundle Controller

`Bundle` objects to not require any special actions on creation. The escalation risk of a service
account listing the names of `Secrets` and `ConfigMaps` it does not have permission to list/read is
minimal. Consumers of `Bundle` objects are generally assumed to have the ability to create `Pods`
within a namespace, and therefore will likely have read/list permissions on `Secrets` and
`ConfigMaps`.

When a `Bundle` object is updated, new pods which inject the `Bundle` will mount in the updated
`entitlements` and `yumRepositories` references. Non-terminated pods will continue to run with
the previously referenced `entitlements` and `yumRepositories`. Changes to underlying `Secrets`
and `ConfigMaps` should be handled by respective volume mount controllers.

When a `Bundle` object is deleted, non-terminated pods which inject the deleted `Bundle` will
continue to run with the previously injected `entitlements` and `yumRepositories` mounts. New pods
which inject the deleted `Bundle` will be rejected by an admission webhook (see below).

#### Subscription Bundle Injection

Developers can inject subscription bundles localized to their namespace by adding the
`subscription.openshift.io/inject-bundle` annotation to a pod template spec. The value of the
annotation is a comma-separated list of bundles to inject. Any workload controller which
ultimately creates a `Pod` will be supported. For example, to inject multiple bundles into a
`Deployment`, use the following YAML:

```yaml
kind: Deployment
apiVersion: v1
metadata:
  name: my-rhel-deployment
spec:
  replicas: 2
  selector:
    matchLabels:
      app: "rhel-deployment"
  template:
    metadata:
      annotations:
        "subscription.openshift.io/inject-bundle": mybundle,otherbundle
      labels:
        app: "rhel-deployment"
    spec:
      containers:
      - name: run
        image: "registry.redhat.io/ubi8/ubi:latest"
        ...
```

The Subscription Injection mutating admission webhook will listen to all pod creation API calls.
The webhook will do the following:

1. Look for the `subscription.openshift.io/inject-bundle` annotation
2. Read the `Bundles` with names matching the annotation CSV values that exist in the pod's
   namespace. Reject admission if any of the bundles do not exist.
3. Read the `Bundles` in the namespace which have the
   `subscription.openshift.io/always-inject: "true"` label. Check these bundles against the allow/
   deny annotations on the pod.
4. Verify that the `Secrets` referenced in `entitlements` across eligible bundles do not contain
   overlapping keys. Reject admission if the secrets contain overlapping keys.
5. Add a `ProjectedVolume` in the pod for the `entitlements` referenced in the bundle.
6. Mount the entitlements volume into `/run/secrets/etc-pki-entitlement` for each container in
   the pod, including init and ephemeral containers.
7. [optional] Verify that the `ConfigMaps` referenced in `yumRepositories` across eligible bundles
   do not contain overlapping keys. Reject admission if the `ConfigMaps` contain overlapping keys.
8. [optional] Add a `ProjectedVolume` in the pod for the `yumRepositories` referenced in the bundle
   which sources data via the projected resource CSI driver.
9. [optional] Mount the yum repos volume into `/run/secrets` for each container in the pod,
   including init and ephemeral containers.
10. Record the generation of the injected bundles by adding or updating the generations JSON value
    in the `subscription.openshift.io/bundle-generations` annotation.

#### Cluster Subscription Bundle Injection

Developers can inject cluster-wide subscription bundles by adding the
`subscription.openshift.io/inject-cluster-bundle` annotation to a pod template spec. Any workload
controller which ultimately creates a `Pod` will be supported. For example, to inject a
subscription into a `Deployment`, use the following YAML:

```yaml
kind: Deployment
apiVersion: v1
metadata:
  name: my-rhel-deployment
spec:
  replicas: 2
  selector:
    matchLabels:
      app: "rhel-deployment"
  template:
    metadata:
      annotations:
        "subscription.openshift.io/inject-cluster-bundle": cluster
      labels:
        app: "rhel-deployment"
    spec:
      containers:
      - name: run
        image: "registry.redhat.io/ubi8/ubi:latest"
        ...
```

The Subscription Injection mutating admission webhook will watch all newly created pods. The
webhook will do the following:

1. Look for the `subscription.openshift.io/inject-cluster-bundle` annotation.
2. Read the `ClusterBundles` with names matching the annotation values. Reject admission if the
   bundle does not exist.
3. Read the `ClusterBundles` with the `subscription.openshift.io/always-inject: "true"` label.
   Check these bundles against the allow/deny annotations on the pod.
4. Conduct a subject access review for the referenced `ClusterBundles` against the pod service
   account. Return an error if the pod service account does not have `get` access to the
   `ClusterBundle`.
5. Verify that the `Secrets` referenced in `entitlements` across eligible `ClusterBundles` do not 
   contain overlapping keys. Reject admission if the secrets contain overlapping keys.
6. Add `Volumes` in the pod for the `entitlements` referenced in the bundle which sources data via
   the projected resource CSI driver.
7. Mount the `entitlements` volumes into `/run/secrets/etc-pki-entitlement` for each container in
   the pod, including init and ephemeral containers.
8. [optional] Verify that the `ConfigMaps` referenced in `yumRepositories` across elibilbe
   `ClusterBundles` do not contain overlapping keys. Reject admission if the `ConfigMaps` contain
   overlapping keys.
9. [optional] Add `Volumes` in the pod for the `yumRepositories` referenced in the bundle which
   sources data via the projected resource CSI driver.
10. [optional] Mount the `yumRepositories` volumes into `/run/secrets` for each container in the pod,
    including init and ephemeral containers.
11. Record the generation of the injected bundles by adding or updating the generations JSON value
    in the `subscription.openshift.io/bundle-generations` annotation.

#### Subscription Injection Mutating Admission Webhook

Per upstream docs, the mutating admission webhook will need to register itself with the cluster and
run as a service in its own namespace [1]. The operator installing the Subscription Injection
webhook will need to do the following:

1. Create a `Deployment` that runs the webhook in a highly available mode.
2. Create a `Service` to expose the webhook pods, with generated TLS cert/key pairs.
3. Create a `ConfigMap` where the service serving CA can be injected.
4. Create a `MutatingWebhookConfiguration` that fires the webhook for all created pods.
5. When the service serving CA in item 3 is injected or updated, the operator needs to update the
   caBundle provided to the webook configuration in 4.

[1]
https://kubernetes.io/blog/2019/03/21/a-guide-to-kubernetes-admission-controllers/#mutating-webhook-configuration

#### Bundle API

```yaml
kind: ClusterBundle
apiVersion: subscription.openshift.io/v1alpha1
metadata:
  name: <name>
  ... # standard metadata, cluster-scoped so no namespace
  labels:
    # Special label to auto inject a ClusterBundle
    subscription.openshift.io/always-inject: "true"
spec:
  aggregateToClusterRoles: # cluster roles which the ClusterBundle RBAC should aggregate to.
  - admin
  - edit
  entitlements:
  - name: <secret name with entitlement keys>
    namespace: <namespace containing the secret>
  yumRepositories:
  - name: <configMap name with yum repo definition>
    namespace: <namespace for the yum repo definition>
status:
  conditions:
  - type: Invalid
    status: "False" # standard k8s conditions, should obey the "abnormal-true" convention
  - ... # additional informational conditions
  clusterRole: <name>-subscription-bundle # clusterRole is cluster-scoped
  shares:
  - name: <name>-subscription-entitlement # shares are cluster-scoped
  - name: <name>-subscription-yumRepository
```

```yaml
kind: Bundle
apiVersion: subscription.openshift.io/v1alpha1
metadata:
  name: <name>
  namespace: <namespace>
  ... # standard metadata
  labels:
    # Special label to auto-inject a Bundle in a namespace
    subscription.openshift.io/always-inject: "true"
spec:
  entitlements:
  - name: <secret name with entitlement keys>
    namespace: <namespace containing the secret>
  yumRepositories:
  - name: <configMap name with yum repo definition>
    namespace: <namespace for the yum repo definition>
status:
  conditions:
  - type: Invalid
    status: "False" # standard k8s conditions, should obey the "abnormal-true" convention
  - ... # additional informational conditions
```

#### Pod Annotations and Mounts

Injecting a namespace-scoped `Bundle`:

```yaml
kind: Pod
apiVersion: core/v1
metadata:
  annotations:
    # Inject namespace-scoped bundles
    "subscription.openshift.io/inject-bundle": local,driver
    # Record the generation of the injected bundles
    "subscription.openshift.io/bundle-generations": |
      {
        "bundles": {
          "local": 2,
          "driver": 3
        }
      }
spec:
  containers:
  - name: main
    ...
    volumeMounts:
    # Mount yum-repos.d configs in /run/secrets
    - name: yum-repo
      mountPath: /run/secrets
    # Mount entitlements in /run/secrets/etc-pki-entitlement
    - name: etc-pki-entitlement
      mountPath: /run/secrets/etc-pki-entitlement
  ...
  volumes:
  # yumRepositories are consolidated into a single volume
  # Projected volumes with no key/path mappings require that all refereneced keys are unique
  - name: yum-repo
    projected:
      sources:
      - configMap:
          name: local-repo
      - configMap:
          name: driver-repo
  # Entitlement keys are likewise consolidated into a single volume
  # Projected volumes with no key/path mappings require that all referenced keys are unique
  - name: etc-pki-entitlement
    projected:
      sources:
      - secret:
          name: local-entitlement
      - secret:
          name: driver-entitlement
```

Injecting a cluster-scoped `ClusterBundle`:

```yaml
kind: Pod
apiVersion: core/v1
metadata:
  annotations:
    # Inject cluster-wide bundles
    "subscription.openshift.io/inject-cluster-bundle": cluster,extra
    # Record the generation of the injected bundles
    "subscription.openshift.io/bundle-generations": |
      {
        "clusterBundles": {
          "cluster": 1,
          "extra": 4
        }
      }
spec:
  containers:
  - name: main
    ...
    volumeMounts:
    # Mount yum-repos.d configs in /run/secrets
    - name: yum-repo
      mountPath: /run/secrets
    # Mount entitlements in /run/secrets/etc-pki-entitlement
    - name: etc-pki-entitlement
      mountPath: /run/secrets/etc-pki-entitlement
  ...
  volumes:
  # yumRepositories are consolidated into a single volume
  # Projected volumes with no key/path mappings require that all refereneced keys are unique
  - name: yum-repo
    csi:
      driver: projected-resource.storage.openshift.io
      volumeAttributes:
        sources:
        - share:
            name: cluster-repo
        - share:
            name: extra-repo
  # Entitlement keys are likewise consolidated into a single volume
  # Projected volumes with no key/path mappings require that all referenced keys are unique
  - name: etc-pki-entitlement
    csi:
      driver: projected-resource.storage.openshift.io
      volumeAttributes:
        sources:
        - share:
            name: local-entitlement
        - share:
            name: driver-entitlement
```

Allow/deny lists for bundles that are always injected. Note that if none of these annotations are
present, all available auto-injected bundles are added to the pod:

```yaml
kind: Pod
apiVersion: core/v1
metadata:
  annotations:
    # Allow some bundles in the namespace, deny all others
    subscription.openshift.io/allow-bundles: "bundle-A,bundle-B"
    # Deny some bundles in the namespace, but allow others
    subscription.openshift.io/deny-bundles: "bundle-B,bundle-C"
    # Deny all bundles in the namespace
    subscription.openshift.io/deny-bundles: "*"
    # Allow some cluster bundles, but deny others
    subscription.openshift.io/allow-cluster-bundles: "cluster-bundle-A"
    # Deny some cluster bundles, but allow others
    subscription.openshift.io/deny-cluster-bundles: "cluster-bundle-A,cluster-bundle-B"
    # Deny all cluster bundles
    subscription.openshift.io/deny-cluster-bundles: "*"
  ...
```

### Risks and Mitigations

**Risk:** Entitlement keys can leak outside of the `openshift-config` namespace to unauthorized
workloads.

_Mitigation:_ Access to the entitlement keys is gated by the cluster role generated for the
projected resource shares associated with the bundle. Cluster admins are responsible for either

1. Creating a `ClusterRoleBinding` for the service accounts that need access to the entitlement, OR
2. Declaring cluster roles to aggregate the `ClusterBundle` to via the `aggregateToClusterRoles`
   array, OR
3. Configuring the operator to aggregate the generated `ClusterRole` to appropriate system roles by
   default.

**Risk:** Shares and cluster roles are orphaned if the subscription `ClusterBundle` is deleted.

_Mitigation:_ Owner references are used to ensure proper cleanup. Note that `ClusterBundle` is
cluster-scoped, so it can create owner references on any object.

**Risk:** Containers run with outdated entitlement keys if they are rotated

_Mitigation:_ When an entitlement key is rotated, the secret containing the original key(s) should
be directly updated with the new entitlement key. This would be treated as a content change by the
the respective volume mount controller (kubelet for `Bundle`, projected resource csi driver for
`ClusterBundle`). Updating a `Bundle` or `ClusterBundle` by changing the referenced `entitlements`
and `yumRepositories` should be clearly discouraged in the documentation.

## Design Details

### Test Plan

This feature set can be tested as follows:

1. Download an OpenShift entitlement key set from the customer portal, and add it to the
   `openshift-config` namespace as the `etc-pki-entitlement` `Secret`.
2. Create a `ClusterBundle` object 
2. Run a pod with the `"subscription.openshift.io/inject-cluster-bundle": cluster` annotation, and
   a UBI8 container which performs a `yum install` of a RHEL package that requires a subscription.

### Graduation Criteria

This feature will likely require Dev Preview, Tech Preview, and GA maturity levels.

#### Feature Roadmap

1. `Bundle` supporting a single bundle injected into a workload within the same namespace, allowing
   one entitlement secret and one yum.repo configuration.
2. `Bundle` supporting multiple `entitlements` and `yumRepositories`.
3. Allow multiple `Bundles` to be injected into a workload within the same namespace.
4. Allow a `Bundle` to be automatically injected into a workload within a namespace.
5. `ClusterBundle` supporting a single bundle injected into a workload, allowing one entitlement
   secret and one yum.repo configuration.
6. `ClusterBundle` supporting multiple `entitlements` and `yumRepositories`.
7. Allow multiple `ClusterBundles` to be injected into a workload.
8. Allow auto-injection of `ClusterBundles` into all workloads. 

#### Dev Preview

- Bundle APIs at `v1alpha1`.
- Initial Dev preview release of just the `Bundle` object API and webhook.
- Subsequent dev preview release of `ClusterBundle` with dependency on the projected resource CSI
  driver (dev preview).

##### Dev Preview -> Tech Preview

- Projected resource CSI driver reaches tech preview state.
- APIs reach `v1beta1`
- Clear documentation on how to enable subscription injection across the cluster
- Initial scale testing
- Gather feedback for namespace-restricted subscription bundles
- Release as a tech preview OLM operator

##### Tech Preview -> GA

- All APIs reach `v1` stability
- Projected resource CSI driver reaches GA
- Security audit for these components and the projected resource CSI driver
- Scale testing (with potential mitigations like `HorizontalPodAutoscalers`)
- Disruption testing (with potential mitigations like `PodDisruptionBudgets`)

### Upgrade / Downgrade Strategy

Upgrades and downgrades will be handled via OLM. Each released version will specify:

- Minimum supported version of the projected resource CSI driver
- Minimum supported kube version of the cluster.

### Removal Strategy

As an OLM operator, removing the Subscription Injection operator will not remove the associated
custom resource definitions. As a result, existing `Bundle` and `ClusterBundle` objects will remain
on the cluster and the associated injected volumes will remain intact.

### Version Skew Strategy

- OLM addresses cluster version skew via the minimum supported Kubernetes version attribute as well
  as minimum supported `GroupVersionKind` for dependent CRDs.
- Version skew for the bundle controllers can be addressed via leader election.
- Version skew for the admission webhook can be addressed via standard `Deployment` rollouts.
  Temporary version skew for the admission webhook is acceptable.

### Open Questions

1. Is allowing an admission webhook which can read all `Secrets` and `ConfigMaps` acceptable?
   The assumption is yes - admission webhooks in general require a high level of privilege.
2. When a `Bundle`/`ClusterBundle` is updated or deleted, how can we evict pods in a way that is scalable?
   We won't evict pods - new pods receive updated mount specs, existing pods aren't changed.

## Implementation History

- 2020-06-25: Initial proposal
- 2020-09-16: Implementable version

## Drawbacks

- Since this is being delivered via OLM, customers will need to opt into capabilities that they may
  expect by default.
- Users will have to specify on a per-workload basis which subscriptions should be added,
  increasing toil.
- Cluster admins need to manually obtain their entitlement keys, add them to the cluster, and
  configure appropriate `ClusterBundle` objects.

## Alternatives

### Document what we have

Today, any UBI/RHEL-based container can access subscription content if the entitlement keys are
mounted into `/run/secrets/etc-pki-entitlement`. Clusters which access content via Satellite will
also need `redhat.repo` configurations mounted into `/run/secrets`. This is not well documented in
OpenShift. Customers will need to obtain entitlement keys and update their pods on their own.
Likewise, Satellite users will need to obtain a `redhat.repo` configurations and mount it into
their pods.

Builds have their own docs since they use a separate mechanism to add Secrets and ConfigMaps into
the build context.

### Mount subscription bundles through the node

As a work-around, customers can create a MachineConfig that adds the same entitlement keys to
/etc/pki/entitlement/ on every node [1]. CRI-O and buildah are configured by default to mount these
entitlement keys into all running containers.

There are two main downsides to this approach:

1. Nodes need to be restarted when the entitlement keys are added or updated/rotated.
2. Only one set of entitlements can be shared per node. Mutli-tenancy isn't feasible unless tenant
   workloads are tied to specific nodes.

[1] https://access.redhat.com/solutions/4908771

## Infrastructure Needed [optional]

To test this via CI, our CI clusters would need to obtain entitlement keys. The template used to
test this feature would need to add the entitlement keys after the cluster has been installed.
