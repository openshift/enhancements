---
title: disable-builder-sa
authors:
  - "@adambkaplan"
reviewers:
  - "@sayan-biswas" # Team lead, Pipeline Integrations
  - "@siamaksade" # Product manager, Builds for OpenShift
  - "@coreydaley" # Former team lead and BuildConfig maintainer
  - "@apporvajagtap"
  - "@mfojtik"
approvers:
  - "@soltysh"
api-approvers:
  - "@soltysh"
creation-date: 2024-02-06
last-updated: 2024-02-19
tracking-link:
  - "https://issues.redhat.com/browse/BUILD-730"
see-also:
  - "https://issues.redhat.com/browse/RHDP-732"
  - "https://issues.redhat.com/browse/OCPSTRAT-890"
replaces: []
superseded-by: []
---

# Disable Builder Service Account

## Summary

Provide cluster configuration options to disable the auto-creation of the
`builder` service account. When this behavior is disabled, the `builder`
service account and its associated RBAC should not be created in new
namespaces, and cluster admins can delete `builder` service accounts in
existing namespaces.

## Motivation

In OCP 4.14, `Build` and `DeploymentConfig` were added as optional install
capabilities to OpenShift [1]. When `Build` and `DeploymentConfig` capabilities
are not enabled, the APIs and respective controllers are not enabled on the
cluster. Cluster admins can enable these capabilites after installation, but
they cannot disable these capabilities once enabled. Long-lived clusters
upgrading to 4.14 cannot disable the `Build` capability.

This feature will allow cluster administrators to disable the `builder` service
account while keeping other components of the BuildConfig system available.
When disabled, cluster administrators will be responsible for configuring a
service account that can perform actions that typically occur during builds.
Most notably, these service accounts will need permission to push to the
OpenShift internal registry if that feature is enabled. The builder service
account does not need permission to create pods with elevated pod security
permissions, as this has been delegated to the build controller's service
account.

This feature will address the following use cases:

- Cluster administrators (or equvalent) upgrading large fleets of "application"
  clusters that do not wish to run `Build` workloads alongside applications.
  In the current state, disabling the `builder` service account and its RBAC is
  not an option in OCP for clusters upgrading from 4.13 or earlier.
- Cluster administrators (or equivalent) who want more fine grained control
  over the permissions granted to service accounts.

[1] https://issues.redhat.com/browse/WRKLDS-695

### User Stories

- As an enterprise platform engineer, I want a mechanism to disable the builder
  service account - even if the “Build” capability is enabled on the cluster -
  so that I can provide my own RBAC for builds in the “golden path” namespace
  template for dev teams.
- As an information security officer, I want to disable the builder service
  account as part of our process to limit access to the OCP Build system on
  production/application clusters so that only service accounts related to
  applications are deployed, and they have the minimum permissions necessary.
- As a software architect/platform engineer, I want to change the default
  "golden path" service account used for builds so I can customize its
  permissions.
- As a product manager, I want to know how many OpenShift clusters are
  disabling the builder service account so that I can understand the impact of
  this feature.

### Goals

- On existing clusters that have the `Build` capability enabled, cluster admins
  can disable the creation of the `builder` service account.
- On existing clusters that the `builder` and service accounts are disabled,
  cluster admins can enable creation of these service accounts in every
  namespace.
- When the `builder` service account is disabled, existing `builder` service
  accounts already created in namespaces should remain intact.
- When the `builder` service account is disabled, manual deletion of existing
  `builder` service accounts should not lead to recreation of these service
  accounts.

### Non-Goals

- Cleaning up and deleting existing builder service accounts in namespaces
  after the SA generation controller is disabled.
- Enable/disable the `deployer` service account. This will require a different
  implementation/API.
- Disable the `BuildConfig` API and controllers on clusters that have the `Build`
  capability enabled at install time, or turn on the Build capability after
  installation.
- Refactoring “system:*” bootstrap roles and rolebindings related to BuildConfigs.
- Add Service Accounts as a `buildDefault`/`buildOverride` feature.
- Fine tune the RBAC of the _generated_ `builder` service account.
- Improve metrics related to the success rate of `BuildConfig`-driven builds

## Proposal

The general idea is to expose a new cluster-level configuration that controls
if the `builder` service account is created in each namespace.

### Workflow Description

flowchart TD
    A[Install] -->|Enable `Build` Capability| B(Build cluster config)
    B --> C{spec.builderServiceAccunt}
    C --> D[Empty]
    D --> H[Set to `Generate`] 
    H --> |ocm-o| C
    C --> E[Other]
    E --> H
    C --> F[Generate]
    C --> G[Disable]
    F --> |ocm-o| I[Turn on `builder` controllers]
    G --> |ocm-o| J[Turn off `builder` controllers]

1. The cluster is created or upgaded with the `Build` capability enabled.
2. On install or upgrade with the `Build` capability enabled, the `Build`
   cluster configuration CRD will be installed. The CRD will have a new field
   to declare that the `builder` service account should be generated:

   ```yaml
   apiVersion: config.openshift.io/v1
   kind: Build
   spec:
     buildDefaults:
       ...
     buildOverrides:
       ...
     builderServiceAccount: Generate   
   ```

3. cluster-openshift-controller-manager-operator (ocm-o) ensures that this
   field is populated with a valid value. If none is set, or it is set to an
   invalid value, the operator sets this to `Generate`.
4. The cluster administrator edits the cluster build system configuration to
   disable the generation of the builder serice account.

   ```yaml
   apiVersion: config.openshift.io/v1
   kind: Build
   spec:
     ...
     builderServiceAccount: Disable
   ```

5. When configuration is updated to `builderServiceAccount: Disable`, ocm-o
   disables the controllers that generate the RBAC for the `builder` service
   account. The mechanism should be similar to what is employed to turn off the
   `Build` capability at install.

### API Extensions

Modified extension:
- apiVersion: `config.openshift.io/v1`
- kind: `Build`

This proposal will update an existing cluster configuration CRD. ocm-o will use
_runtime validation_ to ensure a valid value is set. Defaulting will not be
used because we do not have full confidence that `Generate` will be the accepted
long-term behavior. Runtime validation by ocm-o provides flexibility to change
the "empty"/"default" behavior without breaking API contracts.

The following values will be considered valid:

- `Generate`
- `Disable`

Other values - including null/empty - should be considered invalid and reverted
to `Generate` by ocm-o if detected. Follow up features may add additional
supported values that provided additional functionality (ex: `Remove` to clean
up `builder` service accounts in existing namespaces gracefully).

### Topology Considerations

#### Hypershift / Hosted Control Planes

For hosted control planes, we currently do not allow the `Build` subsystem to
be modified for hosted clusters. This could change in the future. [1]

If these configurations are exposed in the future, Hypershift will need
to ensure appropriate behavior of openshift-controller-manager for each "guest
cluster." Refactoring decisions - such as running the build-related controllers
in their own `Deployment` - may be considered at such time.

[1] https://hypershift-docs.netlify.app/reference/api/#hypershift.openshift.io/v1beta1.ClusterConfiguration

#### Standalone Clusters

Adminstrators for standalone OCP will be able to modify the `Build`
configuration resource if the cluster enables the `Build` capability.

For clusters installed without the `Build` capability [1], the system build
configuration CRD is not installed and its default instance is not created.
The cluster admin must first enable the `Build` capability, at which point
the configuration CRD and `cluster` instance are created.

[1] https://docs.openshift.com/container-platform/4.14/post_installation_configuration/enabling-cluster-capabilities.html#setting_additional_enabled_capabilities_enabling-cluster-capabilities

#### Single-node Deployments or MicroShift

Single-node deployments will behave like standalone OCP with respect to the
`Build` configuration resource. It is up to the cluster administrator to enable
the `Build` capability at install time, or as a day 2 action.

For Microshift/edge clusters, the `Build` APIs are not enabled [1]. This is
less likely to change due to the smaller form factor of Microshift.

[1] https://github.com/openshift/microshift/blob/4.15.0-rc.5-202402022103.p0/docs/contributor/enabled_apis.md

### Implementation Details/Notes/Constraints

Many aspects of of the Build system and its user experience assume the
`builder` service account is created in every namespace. Disabling the auto
generation of this account, its RBAC, and its push secret may cause builds to
fail. To address this, builds should "fail fast" if the service account
specified for the build does not exist at pod creation time.

For platform teams that want to "bring their own builder" service account, OCP
should provide detailed documentation describing the RBAC that is generated for
the `builder` service account.

### Risks and Mitigations

* Risk that builds fail if the service account for the build pod does not
  exist. This can be mitigated by ensuring builds fail fast in this scenario,
  with an actionable error message.
* Risk that "bring your own" service account builds fail, especially pushing to
  the internal registry. We may not be able to easily mitigate this, as the UX
  should be no different for failing to push to an external registry like
  quay.io.
* Risk that this feature is enabled on clusters with a high magnitude of
  namespaces (10k), resulting in a large number of objects being created at
  once. This is mitigated by the rate limiters baked into the
  `openshift-controller-manager` controllers and the QPS limits for the
  controller clients. The current controller defaults (10 QPS, 100 burst) imply
  an upper bound of ~40 objects created per second, each one of fairly small
  size (under 1 KiB). At 10k namespaces, this represents at most 40 MiB of
  storage - small in comparison to the etcd limit of 8GiB.

### Drawbacks

OCP has hesitated to implement this feature for a very long time because it
adds significant burden to developers, platform engineers, and cluster admins
whose tenants use `BuildConfigs` to build applications. Without the `builder`
service account, teams must configure their own service account with the
correct RBAC controls in every desired namespace.

In the past this was pretty difficult, but today many large enterprises either
have their own in-house tooling to provision OpenShift namespaces, or are
adopting IDPs like Red Hat Developer Hub to provide approved software templates
and environments to engineering teams. This has simplified the process for
teams to onboard to OpenShift in a controlled, "best practices" manner.

## Open Questions [optional]

1. Should we also provide mechanisms for changing the name of the default
   service account used for BuildConfigs? This is not included to limit the
   scope of this feature.

## Test Plan

The current OpenShift builds test suite includes a set of `Serial` tests for
tuning cluster configuration. These should be augmented to test the new
`Disable` setting as follows:

1. Set the `builderServiceAccount` config setting to `Disable`
2. Create a new namespace and apply a `BuildConfig` that has the service
   account unset. This should be a "Hello world" style build that _does not_
   _push to the internal registry_ (no output image).
3. Run a build from this `BuildConfig`. This build should fail quickly (less
   than 1 minute) because the service account does not exist.
4. Update the `BuildConfig` to use the `default` service account. Run a build
   from this `BuildConfig` - it should succeed.

Existing testing infrastructure for openshift-controller-manager-operator and
openshift-controller-manager can address unit and integration testing.

The build tests should also verify that builds can succeed if an alternative
serivce account name is provided, and the service account has been granted
appropriate permissions per the OCP documentation. If these tests do not exist
already, they should be created.


## Graduation Criteria

This feature will be released as _Generally Available_, targeting OCP 4.16.

### Dev Preview -> Tech Preview

N/A

### Tech Preview -> GA

A new metric will need to be published, indicating if the cluster has disabled
the builder service account generator.

Documentation will be updated to describe the following:
- What the builder SA generator creates, especially RBAC
- Impact of disabling the builder SA generator
- How to change the service account for a Build, with examples for the cli and
  OpenShift web console (both Admin and Developer perspectives)


### Removing a deprecated feature

N/A

## Upgrade / Downgrade Strategy

**Upgrade**

The new field defaults to `Generate` through a value ocm-o applies if it
encounters the empty string. While the upgrade progresses, ocm-o will keep the
current logic of enabling the builder service account generators by default.
Only if the `Disable` value is set will the builder SA cease to be generated.

**Downgrade**

On downgrade, ocm-o will need to be rolled back to the version that does not
read the `builderServiceAccount` field. This version will continue to generate
the builder service account in all namespaces.

## Version Skew Strategy

Skew can happen if the `Build` CRD for `config.openshift.io` does not align
with ocm-o:

- CRD updated, but not ocm-o: builder service account should continue to be
  created.
- CRD not updated, but ocm-o is: builder service account should continue to be
  created. We may see the operator report errors trying update a the `Build`
  CRD instance, in which event the operator should re-queue and try again.

## Operational Aspects of API Extensions

This feature does not introduce new CRDs or admission webhooks. The default
value for `builderServiceAccount` is set by a cluster operator.

This feature could impact user experience if cluster admins/platform engineers
do not configure a `builder` service account on behalf of developers. This
would result in an increase in faild builds on the cluster. We do not have a
"good" SLI for `BuildConfig`-driven builds at present:

- The metric `openshift_build_status_phase_total` records the phase of the
  `Build` objects on the cluster, with labels for phase state (`error`,
  `completed`, `failed`, `canceled`, etc.).
- The metric is a `Gauge` type, calculated by querying the state of OpenShift
  when the metrics endpoint for `openshift-state-metrics` is hit.
- This metric tends to over-report failures as a percent of total builds.
  Builds have an integrated pruning mechanism that deletes `Build` objects over
  time. By default, the 5 most recent successful and failed builds are
  retained. Over time, the ratio of successful to "failed" category builds will
  converge to 1.
- `openshift_build_total` provides similar data, and suffers from the same
  limitations as `openshift_build_status_phase_total`. This is sourced from the
  openshift-controller-manager.

## Support Procedures

- Detection: ClusterOperator `openshift-controller-manager` reports itself
  `Progressing` or `Degraded`
- Support: analyze logs for `openshift-controller-manager-operator` pods.
  Check for errors updating the `openshift-controller-manager` deployment or
  the `Build` cluster config resource.

## Alternatives

Instead of disabling the service account, we could provide mechanisms for
cluster admins to tune the RBAC granted to the account. This would require a
much larger API surface. Furthermore, a misconfigured RBAC could substantially
weaken the security features of OpenShift. One of the primary motivations of
this feature is to reduce the attack surface for OpenShift - especially for
clusters that are not used to build container images.

We could also provide mechanisms to change the default service account name for
builds. Doing this in isolation would not address the security concerns that
the `builder` service account raises - security teams would still want ways to
to remove this service account if the cluster does not need build capabilities
OR if the security/platform engineering team wants to provide their own RBAC
for builds. Adding cluster options to set an alternative default builder
service account name could be addressed in a follow up enhancement.

Finally, clusters can simply be installed with the `Build` capability disabled.
This works for new clusters and deployments - however it does not help
enterprises with large fleets of existing (pre 4.14) clusters that are
upgrading over time. "Lift and shift" approaches may not scale in this context,
or may not be feasbile due to anticipated disruptions of mission-critical
applications.

## Infrastructure Needed [optional]

No new infrastructure anticipated.
