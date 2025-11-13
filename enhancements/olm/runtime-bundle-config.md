---
title: runtime-bundle-config
authors:
  - perdasilva
reviewers:
  - joelanford
approvers:
  - joelanford
api-approvers: # In case of new or modified APIs or API extensions (CRDs, aggregated apiservers, webhooks, finalizers). If there is no API change, use "None"
  - everettraven 
creation-date: 2025-11-05
last-updated: 2025-11-05
tracking-link: # link to the tracking ticket (for example: Jira Feature or Epic ticket) that corresponds to this enhancement
  - https://issues.redhat.com/browse/OPRUN-4133
---

# OLM v1 Runtime Bundle Configuration

## Summary

This enhancement proposes the addition of a configuration layer to the ClusterExtension API surface. The configuration
layer allows users to set bundle configuration, which itself is defined by the bundle authors through a configuration
schema. It provides a single mechanism for configuring bundles independently of the underlying bundle packaging format, 
which could be either registry+v1, Helm, or, a future registry+v2 format.

## Motivation

OLMv0's strongly opinionated configuration layer has made it difficult for authors to provide content customization to
their users, and has meant that new configuration knobs had to be added as features to OLM on the OLM team's timeline. 
OLMv1 aims to mitigate this by allowing content authors to define a configuration schema directly in the bundle. 

OLMv1 also aims to support content packaged in different formats (i.e. registry+v1, Helm, registry+v2) but wouldn't like
any format specific concerns to be surfaced at the API level. Users care about content, and not its format.

### User Stories

#### Story 1: Bundle Configuration
As a cluster administrator installing a bundle on my cluster, I'd like to configure the bundle in ways allowed by the 
author in order to make the content best fit my cluster and the desired usage.

#### Story 2: Bundle Configuration Update
As a cluster administrator, I'd like to update the configuration of an installed bundle in ways allowed by the 
author in order to make the content best fit my cluster and the desired usage as my those needs change over time without
having to re-install the bundle.

#### Story 3: Bundle Configuration Validation
As a cluster administrator, when I add or update configuration, I don't want the installation/update to progress unless
the provided configuration meets the configuration schema provided by the author in order to avoid wrongly
configured or faulty content from being installed on my cluster.

#### Story 4: Bundle Configuration Consistency Across Upgrades
As a cluster administrator, when OLM automatically progresses my bundle to its next version, if my bundle 
configuration no longer fits the schema provided by the author, the upgrade should not progress until the configuration
is fixed in order to avoid wrongly configured or faulty content from being installed.

#### Story 5: Bundle Configuration Across Formats
As a cluster administrator, when my installed content migrates between formats, I want my upgrades to progress as
they normally would without intervention in order to not be burdened with migration steps. I care about the content,
not the format it is packaged in.

### Goals

- Introduce the notion of bundle configuration into the ClusterExtension API surface in a format agnostic way s.t. it can also be used for other bundle formats such as Helm Charts

### Non-Goals

- Support for configuration defined in sources outside the ClusterExtension API (e.g. `Secrets` and/or `ConfigMaps`), though this may come in the future if necessary
- Update the catalog server to expose bundle configuration schemas to users, though this may come in the near future
- Define the registry+v1 bundle configuration schema. This will come through subsequent enhancements tackling specific configuration needs
- Define registry+v2 bundle configuration handling
- Occlude secret information
- Support for default configurations outside what can be set in a JSON-schema defined field default values

## Proposal

### 1. Expand ClusterExtension API to Include Bundle Configuration

The ClusterExtension API surface will be expanded to ingest opaque bundle configuration in the form of arbitrary json 
objects. This enables bundles to provide their own configuration schema as opposed to OLM having to define one as in 
OLMv0. The raw input utilized by the bundle rendering layer independently of the underlying bundle format, e.g. it 
can be the values.json for Helm.

The configuration will be supplied inline directly in the ClusterExtension API. The changes to the API will be designed
in such a way that other sources of configuration (e.g. ConfigMaps, or Secrets) can be subsequently added if needed
in the future.

### 2. Update Bundle Rendering Layer

The bundle rendering layer is currently responsible for producing the manifests that should be applied to the cluster
as part of the application. This interface will be expanded to accept the arbitrary configuration, validate it, and
account for it when generating the final manifests. The given configuration must conform to the configuration schema
provided by the bundle. If the bundle cannot be configured, or the configuration does not strictly comply with the
schema, an error is raised that prevents installation/upgrade/update from proceeding until the user can address the 
issue, e.g.:
- User provides configuration but the bundle does not support configuration
- User does not provide required configuration
- User provides configuration keys that are not existent
- Bundle does not provide a configuration schema

### Workflow Description

**content manager** is the human that has write access to the ClusterExtension API and wants to install an application
on the cluster using OLM. This could be for example the cluster administrator, or a delegate.

#### 1. Initial Install - Happy Path

Content manager wants to install new content and apply customizations via the package/bundle provided configuration options:

1. Content manager searches catalog for content
2. Content manager reads documentation and understands the configuration options for that content
3. Content manager creates a ClusterExtension CR to install that content and adds the configuration
4. OLM resolves the bundle
5. OLM generates the desired manifests accounting for the given configuration
6. OLM applies generated manifests to cluster
7. ClusterExtension conditions report success

#### 2. Upgrade - Happy Path

Content manager wants to upgrade a package, or OLM detects an opportunity to auto-upgrade and existing configuration
gets carried over:

1. A catalog is updated on the cluster making an update available
2. OLM sees the catalog update and reconciles the ClusterExtensions
3. OLM resolves content to new bundle
4. OLM generates the desired manifests accounting for the given configuration
5. OLM applies generated manifests to cluster
6. ClusterExtension conditions report success

#### 3. Installation/Upgrade - Configuration Validation Failure

Content manager wants to install/upgrade a package, or OLM detects an opportunity to auto-upgrade and the given configuration
does not conform to the bundle provided configuration schema:

1. OLM resolves bundle that needs to be installed from catalogs
2. OLM attempts to generate the bundle manifests with the given configuration
3. Configuration violates the schema provided by the bundle
4. ClusterExtension conditions report configuration error
5. Content manager observes error
6. Content manager consults documentation
7. Content manager rectifies configuration error
8. OLM detects configuration changes
9. ClusterExtension conditions report success and content is installed/updated

Alternatively, the user can return to the previous happy state by rolling back to a previous version using the rollback
flow described at the bottom of this section.

Note: this is the same flow for when a configuration is provided but the bundle cannot be configured, or
does not provide a configuration schema

#### 4. Upgrade Across Non-Detectable Breaking Changes

Some breaking API changes such as changes in default behavior, or semantic meaning of fields, can be hard, or impossible,
to detect and _could_ potentially have a large blast radius. By their nature, they are hard to detect. Such an issue would
represent a bug for the content author to address. If the user detects and issue and suspects this to be the case, they can 
follow the rollback flow described at the bottom of this section.

#### Rollback Flow

First, one must determine the version to rollback to.
If the ClusterExtension attempted an upgrade and is in a failed state, `.status.install` should report the version
of the installed bundle. If the current version was successfully installed in a way that violates the user intent, e.g.
there was a non-detectable breaking change in configuration behavior between bundle versions, then one needs to 
consult the catalog and walk the upgrade graph backwards from the currently installed version.

Once the target version is determined, a rollback can be done by updating the desired version in the target 
ClusterExtension and using the `SelfCertified` upgrade policy in the catalog source config, e.g.:

```
apiVersion: olm.operatorframework.io/v1
kind: ClusterExtension
metadata:
  name: some-operator
spec:
  namespace: some-namespace
  serviceAccount:
    name: some-service-account
  config:
    configType: Inline
    inline:
        ...
  source:
    sourceType: Catalog
    catalog:
      packageName: some-package
      # --- force rollback to a previous version ---
      version: <previous version>
      upgradeConstraintPolicy: SelfCertified
      # -------------------------------------------- 
```

Note: OLMv1 aims to introduce a ClusterExtensionRevision API which would provide a historical record of the different
revisions of the application applied to the cluster over time. This will help the user to identify the previous happy
state.

### API Extensions

The ClusterExtension API will be expanded to contain a discriminated union `.spec.config`:

```
// ClusterExtensionConfig is a discriminated union which selects the source configuration values to be merged into
// the ClusterExtension's rendered manifests.
//
// +kubebuilder:validation:XValidation:rule="has(self.configType) && self.configType == 'Inline' ?has(self.inline) : !has(self.inline)",message="inline is required when configType is Inline, and forbidden otherwise"
// +union
type ClusterExtensionConfig struct {
	// configType is a required reference to the type of configuration source.
	//
	// Allowed values are "Inline"
	//
	// When this field is set to "Inline", the cluster extension configuration is defined inline within the
	// ClusterExtension resource.
	//
	// +unionDiscriminator
	// +kubebuilder:validation:Enum:="Inline"
	// +kubebuilder:validation:Required
	ConfigType ClusterExtensionConfigType `json:"configType"`

	// inline contains JSON or YAML values specified directly in the
	// ClusterExtension.
	//
	// inline must be set if configType is 'Inline'.
	// inline accepts arbitrary JSON/YAML objects.
	// inline is validation at runtime against the schema provided by the bundle if a schema is provided.
	//
	// +kubebuilder:validation:Type=object
	// +optional
	Inline *apiextensionsv1.JSON `json:"inline,omitempty"`
}
```

Example:

```
apiVersion: olm.operatorframework.io/v1
kind: ClusterExtension
metadata:
  name: operator
spec:
  namespace: argocd
  serviceAccount:
    name: operator-sa
  config:
    inline:
      foo: bar
  source:
    sourceType: Catalog
    catalog:
      packageName: package-name
      version: 1.0.0
```

Initially, only the `Inline` configType will be available. However, we leave it expandable in case further 
configuration sources (e.g. ConfigMaps, Secrets, etc.) become needed.

### Topology Considerations

#### Hypershift / Hosted Control Planes

OLMv1 does not yet support Hypershift. Although no aspects of this feature's implementation stands out as at odds with 
the topology of Hypershift, it should be reviewed when OLMv1 is ready to be supported in Hypershift clusters.

#### Standalone Clusters

No specific considerations needed

#### Single-node Deployments or MicroShift

No specific considerations needed

### Implementation Details/Notes/Constraints

The OLMv1 runtime will operate over a bundle interface that will be common to any/all supported bundle formats. It won't treat
different formats differently for the purposes of life-cycling. It cares only about generating the required manifests,
and organizing them in a way that it can lifecycle the application. As such, the interface should be generic and not
leak underlying format specific details. The bundle rendering engine should consume opaque configuration, the bundle
interface should provide a configuration schema in JSON Schema format, and rendering will only take place if the
configuration **strictly** adheres to the provided schema, i.e. no additional keys, required fields are set, field value
constraints are observed, etc.

Taking this approach also means that the configuration schema should be treated like an API surface by the authors
which should ensure it is not broken between minor and patch versions. Breaking changes detectable by the schema will 
always stop the install/upgrade/update operation from continuing until the underlying configuration issue is addressed.

Such issues will be treated as bugs for the content author as opposed to OLM, except in the case of registry+v1 bundles
whose schema is controlled by the OLM team. The OLM will ensure there are no breaking changes throughout the development 
of the registry+v1 configuration schema.

Any validation errors will be surfaced to the user via the `Progressing` condition with a clear message, e.g.:
- invalid bundle configuration: unknown key 'foo'
- invalid bundle configuration: invalid value for field 'bar' must be less than 10 but got 11
- invalid bundle configuration: invalid type for field 'fizz' got boolean expected string
- invalid bundle configuration: missing required field 'buzz'
- invalid bundle configuration: bundle 'package.v1.0.0' does not support configuration

### Risks and Mitigations

#### 1. Unknown keys

Unknown keys in configuration can have undesired effects especially between bundle versions. If a configuration key
that is not part of the schema in one version but present in a subsequent version of the bundle is present, this
could violate user intent and potentially place the cluster in an undesired state.

*Mitigation*: Strict schema validation is applied. Unknown keys cannot be present in the supplied configuration.

#### 2. Configuration Changes in Detectably Breaking Ways

If there's a detectable breaking change in the configuration schema, it could be possible to push undesirable changes
onto the cluster or even cause an outage.

*Mitigation*: Install/Update/Upgrade operations are only allowed to continue if the configuration strictly respects
the configuration schema

#### 3. Non-existing Configuration Schema

If a bundle does not define a configuration schema rendering _could_ still take place on a best-effort basis. However,
the system would be susceptible to undesirable changes being applied as no breaking changes could be detected.

*Mitigation*: For a bundle to be configurable, it must provide a configuration schema. If it does not, if any
configuration is supplied, the operation will not progress.

Note: it's possible that in the future users can override this behavior if they are willing to accept the consequences.

#### 4. Secret Information

Often times, users might need to supply an application with secret/privileged information such as passwords, or
api keys. With the current inline approach, if this information *must* be entered in as plain text (as opposed to as 
a reference to an existing secret, for instance), the privileged information would be reveled to anyone that can read
the ClusterExtension. It could also cause replication headaches as the privileged information might already exist in a
Secret somewhere, and if the information changes, it would need to be manually replicated.

*Mitigations*:
- Don't give read access to ClusterExtensions, or ClusterExtensions with privileged information to anyone
- Don't install the application and contact the supplier and ask for a change in the configuration s.t. a secret reference can be given instead. This also solves the replication issue and ensures no additional copies of the information exist on the cluster outside the Secret and the point of use.

#### 5. Lack of Bundle Supplied Default Configuration

A bundle may not only want to provide a configuration schema, but also a default configuration beyond what schema
defined values can offer. For instance, a Helm Chart would contain both a values.json (default configuration) and
a values.schema.json (bundle configuration schema). The lack of support for default configuration make it hard to 
detect drift in default configuration behavior that can cause undesired changes to the cluster state, and force users
to supply configuration (demanded by the bundle) even if it doesn't make immediate sense. For instance,

v1.0.0 of a hypothetical bundle supports only a single "behavior mode" (whatever that is) called `Simple`. When the user 
installs v1.0.0, it is installed in the `Simple` behavior mode. Then, v1.1.0 comes along and adds a new behavior 
mode `Awesome`, which becomes the new default. The upgrade to v1.1.0 would silently move to the new (default) behavior mode, 
which could be undesirable. With default configuration support, we could detect a breaking change in the default behavior 
and stop the flow until the user explicitly adds the configuration selecting the desired behavior mode.

*Mitigations*:
Design the configuration schema for the bundle in a way that it breaks if the default behavior changes. E.g. in the
hypothetical case above, the bundle configuration schema would have a required field `behaviorMode` that can only take
the value `Simple`.

Note: While OLMv1 only has support for registry+v1 bundles, this is a limitation we can live with. However, this 
support will be added by a future enhancement when we start to roll out Helm content support.

#### 6. Lack of registry+v1 Bundle Configuration Schema

Adding bundle configuration support to registry+v1 bundles is out of the scope of this enhancement, which basically
relegates this enhancement to the changes in the ClusterExtension API spec and returning configuration errors should
any configuration be set.

*Mitigation*: We'll introduce another enhancement in tandem with this one for supporting registry+v1 bundles with
Single-/OwnNamespace install mode which will introduce the first configuration surface for registry+v1 bundles.

### Drawbacks

- Configuration validation happens at runtime and not at ingress-time by the kube apiserver
- Bundle configuration schema may evolve in a breaking way requiring manual intervention during auto-upgrade flows
- Bundle configuration schema may evolve in a non-detectable breaking way, e.g. changes in default behavior
- No support for default bundle configuration
- Secret information isn't occluded
- No way to inspect bundle configuration schema outside what will be provided by documentation

## Alternatives (Not Implemented)

### Alternative 1: Do nothing
**Description**: Do not implement configuration support.

**Why Not Selected**:
Bundle configuration is a highly sought after and necessary feature for OLM as a package manager. Without it, 
for instance, users wouldn't be able to affect where the operator's pods are schedule on the cluster, or add
additional annotations to bundle resources, pass environment variables, etc.

### Alternative 2: Format-specific Bundle Configuration
**Description**: Provide different configuration surfaces for different bundle formats

For instance, registry+v1 bundles could be configured with the `registryV1Configuration` config type, Helm/registry+v2
could use the `OpaqueConfiguration` type, etc.

**Why Not Selected**:
- One of the goals of OLM is to provide a single interface across bundle formats
- OLM does not want the user to care about the format types, only about content
- Transitions between format types are visible to the end-user

## Future Work

### 1. registry+v1 bundle configuration

registry+v1 bundles can be configured in two ways: 
- `OperatorGroup` `.spec.targetNamespaces`: defines the namespaces the operator should watch for its CRs [ref](https://docs.okd.io/latest/rest_api/operatorhub_apis/operatorgroup-operators-coreos-com-v1.html#spec)
- `Subscription` `.spec.config`: allows users to modify the operator `Deployment`, e.g. add environment variables to the controller container, pod annotations/labels, volumes/volume mounts, etc. [ref](https://docs.okd.io/latest/rest_api/operatorhub_apis/subscription-operators-coreos-com-v1alpha1.html) 

The enhancement to introduce configuration of the target namespace will be introduced in tandem with this enhancement. 
Another enhancement for adding support for subscription configuration will be added in the near future.

### 2. Support to Secret as a bundle configuration resource

With this enhancement, users will be able to provide inline bundle configuration support. There could be a use-case
for reading configuration from other cluster resources such as Secret/ConfigMap. 

### 3. Support for bundle provided default configurations

Similarly to Helm's default values.json, a bundle could also provide a default configuration. This is desirable because:
- It gives authors more flexibility in defining default configuration / behavior
- Breaking changes to default configuration could be detected across bundle upgrades and stop them from being silently applied to the cluster

### 4. Helm support

OLMv1 would like to meet authors where they are and add support for Helm content. Configuration (values.json) will 
be required to enable this.

### 5. registry+v2

OLMv1 will eventually add the next iteration of registry+v2. Author provided configuration schemas will be a requirement for this format.

### 6. Bundle configuration schema discoverability (outside the runtime)

In the future, we'd like an easy way to surface the bundle configuration schema to users via the console to help users
understand the configuration affordances for the different catalog packages, and bundles.

## Test Plan

### ClusterExtension API Ingress Validation Tests
- Ensure only JSON objects be given as configuration (and not non-object but valid JSON, e.g. `true`, `1`, etc.)
- Ensure bundle configuration defined as either JSON or YAML can be ingested

### Integration Tests
- **Configuration Validation**: Test bundle configuration validation is being exercised when configuration is provided 

### Regression Tests

NA

## Graduation Criteria

### Dev Preview -> Tech Preview
- [ ] Feature-gated implementation behind `NewOLMOwnSingleNamespace`
- [ ] API changes present
- [ ] Bundle configuration validation

### Tech Preview -> GA
- [ ] 1 OCP release of alpha feedback
- [ ] Production deployment validation
- [ ] Complete documentation including best practices
- [ ] Established support and maintenance processes

### Removing a deprecated feature

NA

## Upgrade / Downgrade Strategy

### Upgrade Strategy

- **Backward Compatibility**: All existing ClusterExtension CRs will be compatible with the introduction of the API changes
- **Configuration Migration**: No automatic migration from installalled workloads currently being managed by OLMv0; users must explicitly install using OLMv1 ClusterExtension and configure `watchNamespace`.

### Downgrade Strategy

Downgrading Openshift without removing the field would lead to the field being preserved but ignored because of the following configuration of the `.spec.config` field: 

```
inline:
    type: object
    x-kubernetes-preserve-unknown-fields: true
```

### Version Compatibility
- **Minimum Version**: Requires OpenShift 4.20+ 
- **Configuration Schema**: Uses existing ClusterExtension configuration schema for forward compatibility

## Operational Aspects of API Extensions

### Impact on Existing SLIs

The introduction of the API change alone will not impact SLIs without the addition of bundle configuration support
for registry+v1 bundles.

### Possible Failure Modes

- Invalid bundle configuration

### OCP Teams Likely to be Called Upon in Case of Escalation

1. OLM Team (primary)
2. OpenShift API Server Team 
3. Networking Team (cross-namespace connectivity)
4. Authentication & Authorization Team (ServiceAccount/RBAC)
5. Layered Product Team

## Support Procedures

Without the introduction of registry+v1 bundle configuration support, or support for additional bundle formats, 
a validation error will always be thrown if any configuration is set. Remove the configuration and the error will go away.
Otherwise, please consult enhancements responsible for adding bundle configuration support to registry+v1 bundles and/or
that introduce other bundle format support.

## Version Skew Strategy

This feature is isolated to the operator-controller component, managed by cluster-olm-operator. Version skew strategy is not required. 

### Component Interactions
- **operator-controller**: Must support the `NewOLMOwnSingleNamespace` feature gate

### API Compatibility
- **ClusterExtension API**: Uses existing configuration schema;
- **Status Reporting**: Uses existing condition and status mechanisms

### Deployment Considerations
- **Feature Gate Synchronization**: All operator-controller replicas must have consistent feature gate configuration
- **Configuration Validation**: API server validates configuration schema regardless of feature gate state
- **Runtime Behavior**: Feature gate only affects installation behavior, not API acceptance

#### Feature Dependencies
- **Feature Gate Framework**: Uses established feature gate patterns for controlled rollout
