---
title: runtime-bundle-configuration
authors:
  - anbhatta
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

This enhancement proposes the addition of 
1. A configuration layer to the ClusterExtension API surface. 
2. An initial definition for a registry+v1 bundle configuration schema.

The configuration layer allows users to set bundle configuration whose structure is defined by the bundle authors through
a configuration schema. It provides a single mechanism for configuring bundles independently of the underlying
bundle packaging format. Currently, only the registry+v1 format is supported. But, the Helm and registry+v2 formats
will be supported in the future.

As registry+v1 bundles don't provide a bundle configuration schema, this enhancement proposes that the schema be
derived by OLM by introspecting the bundle. The enhancement also proposes a first iteration of the registry+v1 bundle
configuration schema that will enable the installation of registry+v1 bundles with Single- and/or OwnNamespace install mode.
The registry+v1 bundle configuration schema will be expanded in a future enhancement to add support for configuration 
compatible with OLMv0's `Subscription` `.spec.config` while maintaining backwards compatibility.

## Motivation

OLMv0's strongly opinionated configuration layer has made it difficult for authors to provide content customization to
their users, and has meant that new configuration knobs had to be added as features to OLM placing the OLM team on
the critical path of authors trying to deliver value to their users. OLMv1 aims to mitigate this by allowing content
authors to define a configuration schema directly in the bundle.

OLMv1 also aims to support content packaged in different formats (i.e. registry+v1, Helm, registry+v2) but wouldn't like
any format specific concerns to be surfaced at the API level. Users care about content, and not its format. Therefore,
a generic bundle configuration API surface is desired.

Once such configuration option was provided by OLMv0 for registry+v1 bundles that could be installed in one of four "install modes", used to configure the namespace(s) the
operator should watch for their CR events:
- AllNamespaces: the whole cluster
- SingleNamespace: a single namespace that is _not_ the operator's namespace (i.e. the install namespace)
- OwnNamespace: the operator's namespace
- MultiNamespace: more than one namespace (the operator's namespace can also be used if OwnNamespace is supported by the bundle)

Currently, OLMv1 only supports registry+v1 bundles with AllNamespaces install mode enabled. Telco use-cases rely on
some operators that only support Single and/or OwnNamespace install modes, or prefer to install in these modes due to
security or resource utilization concerns.

Install modes are part of the OLMv0 multi-tenancy concept that will not be a part of OLMv1. They enable multiple
instances of the same operator to be installed and reconcile different namespaces. In OLMv1 it will not be possible
to install the same operator multiple times. Therefore, registry+v1 bundle install modes will be treated as
bundle specific configuration.


### User Stories

#### Story 1: Bundle Configuration
As a cluster administrator installing a bundle on my cluster, I'd like to configure the bundle in ways allowed by the
author in order to make the content best fit my cluster and the desired usage.

#### Story 2: Bundle Configuration Update
As a cluster administrator, I'd like to update the configuration of an installed bundle in ways allowed by the
author in order to make the content best fit my cluster and the desired usage as those needs change over time without
having to re-install the bundle.

#### Story 3: Bundle Configuration Validation
As a cluster administrator, when I add or update configuration, I don't want the installation/update/upgrade to progress
unless the provided configuration meets the configuration schema provided by the author in order to avoid wrongly
configured or faulty content from being installed on my cluster.

#### Story 4: Bundle Configuration Consistency Across Upgrades
As a cluster administrator, when OLM automatically progresses my bundle to its next version, if my bundle
configuration no longer fits the schema provided by the author, the upgrade should not progress until the configuration
is fixed in order to avoid wrongly configured or faulty content from being installed.

#### Story 5: Bundle Configuration Across Formats
As a cluster administrator, when my installed content migrates between formats, I want my upgrades to progress as
they normally would without intervention in order to not be burdened with migration steps. I care about the content,
not the format it is packaged in.

#### Story 6: Legacy Operator Migration
As a cluster administrator migrating from OLM v0 to OLM v1, I want to install operators that only support SingleNamespace or
OwnNamespace install modes so that I can continue using existing operator content without requiring operator authors to modify
their bundles.

#### Story 7: Operator Author Requirements
As an operator developer, I have existing registry+v1 bundles that only support Single or OwnNamespace install modes, and I
want my customers to be able to deploy these operators in OpenShift with OLM v1 so that the bundle content can be properly
rendered and installed without requiring me to modify my existing bundle format during the migration to OLM v1.

### Goals

- Introduce the notion of bundle configuration into the ClusterExtension API surface in a format agnostic way such that it can also be used for other bundle formats such as Helm Charts
- Enable support/rendering for registry+v1 bundles that only support SingleNamespace or OwnNamespace install modes
- Define the registry+v1 bundle schema to support configuration of bundles with Single- and/or OwnNamespace install modes
- Maintain backward compatibility with existing registry+v1 bundle content from OLM v0 catalogs
- Generate appropriate RBAC resources scoped to the target namespaces for registry+v1 bundles only

### Non-Goals

- Support for configuration defined in sources outside the ClusterExtension API (e.g. `Secrets` and/or `ConfigMaps`), though this may come in the future if necessary
- Update the catalog server to expose bundle configuration schemas to users, though this may come in the near future
- Define registry+v2 bundle configuration handling
- Occlude secret information
- Support for default configurations outside what can be set in a JSON-schema defined field default values
- Support OLMv0 SubscriptionConfig-type behavior, enabling broader operator configuration compatibility, though this will come in the future
- Support the MultiNamespace install mode: this mode can severally impact api server resource utilization and cause cluster instability. Even in OLMv0 it is documented as unwise to use and that it will likely be removed in the future (OLMv1 being that future).

## Proposal

### 1. Expand ClusterExtension API to Include Bundle Configuration

The ClusterExtension API surface will be expanded to ingest opaque bundle configuration in the form of arbitrary JSON
objects. This moves the concern of bundle configuration from OLM to the bundle author, paving the way for bundles
to provide their own configuration schema. This opaque configuration is utilized by the bundle rendering layer independently
of the underlying bundle format. For example, it can serve as the values.json for Helm bundles.

The configuration will be supplied inline directly in the ClusterExtension API. The changes to the API will be designed
in such a way that other sources of configuration (e.g. ConfigMaps, or Secrets) can be subsequently added if needed
in the future.

### 2. Update Bundle Manifest Provisioning Layer

The bundle manifest provisioning layer is currently responsible for providing the bundle manifests that should be applied
to the cluster. This interface will be expanded to accept the arbitrary configuration coming from the API, validate it, and
account for it when generating the final set of manifests. The given configuration must conform to the configuration schema
provided by the bundle. If the bundle cannot be configured, or the configuration does not strictly comply with the
schema, an error is raised that prevents installation/upgrade/update from proceeding until the user can address the
issue, e.g.:
- User provides configuration but the bundle does not support configuration
- User does not provide required configuration fields
- User provides configuration keys that are not existent
- User provides configuration but bundle does not provide a configuration schema - note that 
while for registry+v1 bundles this is deemed a good level of restriction to ship the configuration feature with initially, for future bundle formats this restriction may be loosed (eg OLMv1 could allow users to provide configuration for "Helm bundles" without any configuration schema). Users will likely be required to express their intent to configure such bundles explicity and have OLM install anyway, possibly via a ClusterExtension API field (eg `dangerouslyAcceptBundlesWithoutConfigSchema`). Such considerations however are for future considerations. 

### registry+v1 Bundle Configuration Generation, Validation and Handling

registry+v1 bundles do not carry a configuration schema and expecting the schema to exist in the FBC would mean only 
those catalogs that have been updated to carry this information would benefit from the changes proposed by this enhancement. 
Therefore, the decision was made to have OLM derive the configuration by observing the bundle configuration. 
It is possible that this logic moves to the catalog layer. But, since other bundle sources are envisioned (e.g. direct bundle install), 
for a first take, the following changes will go in the runtime:

1. Update the registry+v1 manifest provisioning layer to generate a configuration schema for the bundle
2. Update the registry+v1 manifest provisioning layer to validate user provided configuration against the generated schema returning any failures (and thereby aborting the operation)
3. Update the registry+v1 bundle rendering layer to accept the `watchNamespace` configuration for those bundles that support it and render the manifests appropriately

### Workflow Description

The standard OLMv1 installation and upgrade workflows stay largely the same. Where it will differ is:
- users will need to consult the [OpenShift documentation](https://docs.redhat.com/en/documentation/openshift_container_platform/4.20/html-single/extensions/index#olmv1-deploying-a-ce-in-a-specific-namespace_managing-ce) to understand how packages/bundles can be configured. The OpenShift documentation will include a capability matrix (derived from the table in the "registry+v1 Configuration Schema" section below) that maps bundle install mode support to configuration requirements, specifically:
  - Whether the `watchNamespace` configuration field is required or optional for a given bundle based on its install mode capabilities
  - Valid values and constraints for the `watchNamespace` field (e.g., must equal install namespace, must differ from install namespace, or can be any namespace)
  - Practical examples showing how to configure bundles for different install mode scenarios
  - The different configuration options for bundles given their install mode support will be enumerated with examples. All future updates to the registry+v1 bundle configuration surface will also be enumerated in the OpenShift docs, until there is a better way to surface the configuration schemas to users for individual packages.
- users will be able to specify inline bundle configuration on `.spec.config.inline`.
- install/upgrade operations will be halted if the provided configuration does not meet the bundle provided configuration schema. Errors will be surfaced through the `Progressing` condition outlining the issue, e.g. `invalid bundle configuration: unknown key 'foo'`, or `invalid bundle configuration: missing required key 'bar'`.
- users will be able to update configuration for currently installed content.
- the installer service account must have sufficient permissions for the target namespace.

Below are outlined the general bundle configuration error flows, and the specific flows for installing bundles in Single- or OwnNamespace mode.

**user** is the human that has write access to the ClusterExtension API and wants to install an application
on the cluster using OLM. This could be for example the cluster administrator, or a delegate.

#### 1. Installation/Upgrade - Configuration Validation Failure

The user wants to install/upgrade a package, or OLM detects an opportunity to auto-upgrade and the given configuration
does not conform to the bundle provided configuration schema:

1. OLM resolves bundle that needs to be installed from catalogs
2. OLM attempts to generate the bundle manifests with the given configuration
3. Configuration violates the schema provided by the bundle
4. ClusterExtension `Progressing` condition reports bundle configuration error
5. user observes error
6. user consults documentation
7. user rectifies configuration error
8. OLM detects configuration changes
9. ClusterExtension conditions report success and content is installed/updated

Bundle schema validation can fail for several reasons including:
- configuration was provided by the user, but the resolved bundle does not provide a configuration schema
- configuration includes keys that do not exist in the bundle configuration schema
- configuration does not set a value for a required field
- configuration sets a value of the wrong type (e.g. string where boolean is expected)
- configuration sets a value outside the allowable range (e.g. value must be <= 10 but got 11) or enum set (must be one of \["Turtle", "Elephant", "Capybara"\] but got "Lion")
- configuration sets a value that does not respect the desired format (e.g. value must be a valid url)

Alternatively, the user can address the issue by returning to the previous happy state by rolling back to a previous version using the
rollback flow described at the bottom of this section.

#### 2. Upgrade Across Non-Detectable Breaking Changes

Some breaking API changes such as changes in default behavior, or semantic meaning of fields, can be hard, or impossible,
to detect and _could_ potentially have a large blast radius. By their nature, they are hard to detect. Such an issue would
represent a bug for the content author to address. If the user detects an issue and suspects this to be the case, they can
follow the rollback flow described at the bottom of this section.

#### 3. Installing an Operator in SingleNamespace or OwnNamespace Modes

1. user searches catalog for content
2. user reads documentation and understands `watchNamespace` can be used to specify a namespace the operator should watch and whether it can or not be the install namespace
3. user creates a ClusterExtension CR to install that content specifying `.spec.config.inline.watchNamespace` to the desired namespace
4. OLM resolves the bundle
5. The manifest provisioning layer gets the bundle configuration schema and validates the configuration:
    1. `watchNamespace` is a valid namespace name (follows the right format)
    2. `watchNamespace` is not the install namespace if the bundle does not support OwnNamespace
    3. `watchNamespace` is the install namespace if the bundle does not support SingleNamespace
6. OLM applies generated manifests to cluster
7. ClusterExtension conditions report success

#### Rollback Flow

First, one must determine the version to rollback to.
If the ClusterExtension attempted an upgrade and is in a failed state, `.status.install` should report the version
of the installed bundle. If the current version was successfully installed in a way that violates the user intent, e.g.
there was a non-detectable breaking change in configuration behavior between bundle versions, then, at present, one needs
to consult the catalog and walk the upgrade graph backwards from the currently installed version. 
The [opm tool](https://docs.redhat.com/en/documentation/openshift_container_platform/4.20/html/cli_tools/opm-cli) can
be used to help facilitate this task. The undocumented `opm alpha render-graph` command can also be used to provide a 
visual representation of the upgrade graph for a particular package and channel.

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
state without having to manually walk the upgrade graph in reverse.

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
    // inline is used to specify arbitrary configuration values for the ClusterExtension.
    // It must be set if configType is 'Inline' and must be a valid JSON/YAML object.
    // The configuration values are validated at runtime against a JSON schema provided by the bundle.
    //
    // +kubebuilder:validation:Type=object
    // +kubebuilder:validation:MinProperties=1
    // +optional
    // +unionMember
	Inline *apiextensionsv1.JSON `json:"inline,omitempty"`
}
```

Example:

```
apiVersion: olm.operatorframework.io/v1
kind: ClusterExtension
metadata:
  name: argocd
spec:
  namespace: argocd
  serviceAccount:
    name: argocd-installer
  config:
    configType: Inline
    inline:
      watchNamespace: argocd-pipelines
  source:
    sourceType: Catalog
    catalog:
      packageName: argocd-operator
      version: 0.6.0
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

The OLMv1 runtime will operate over a bundle interface that will be common to the supported bundle formats. It won't treat
different formats differently for the purposes of life-cycling. It cares only about generating the required manifests,
and organizing them in a way that it can lifecycle the application. As such, the interface should be generic and not
leak underlying format specific details. The bundle rendering engine should consume opaque configuration, and each bundle
format will provide a configuration schema in whatever format is appropriate for that bundle type. OLMv1 will use the
appropriate validation libraries for the schema format provided by the bundle format (e.g., JSON Schema validators for
JSON Schema, CUE validators for CUE, Rego evaluators for Rego, etc.). Rendering will only take place if the configuration
**strictly** adheres to the provided schema, i.e. no additional keys, required fields are set, field value constraints
are observed, etc.

For bundle formats that use JSON Schema (such as registry+v1 bundles as described in this enhancement), the JSON Schema
format is most widely used in its [draft-07](https://json-schema.org/draft-07) specification and boasts wide coverage
across tools, libraries and ecosystems. The latest specification, [2020-12](https://json-schema.org/draft/2020-12), is
slowly growing in popularity. Therefore, additional JSON Schema specifications may also need to be supported in the future
to meet author needs.

Taking this approach also means that the configuration schema should be treated like an API surface by the authors
which should ensure it is not broken between minor and patch versions. Breaking changes detectable by the schema will
always stop the install/upgrade/update operation from continuing until the underlying configuration issue is addressed.

Such issues will be treated as bugs for the content author as opposed to OLM, except in the case of registry+v1 bundles
whose schema is controlled by the OLM team. The OLM team will ensure there are no breaking changes throughout the development
of the registry+v1 configuration schema.

**Handling Breaking Schema Changes:**

If a change in the JSON Schema specification forces a breaking change in the configuration API (for future bundle formats that provide their own schemas), bundle authors have two options:

1. **Continue with the current schema version**: Maintain backward compatibility by not adopting the new schema specification version that would cause breaking changes.

2. **Accept breaking changes and handle appropriately**: If breaking changes are necessary, authors should handle this like any other breaking API change:
   - Do not carry upgrade edges between breaking versions in the catalog
   - Document the migration to the breaking version, instructing users on how to modify their configuration successfully for the new schema
   - Consider this a major version change requiring explicit user action

**Unsupported JSON Schema Specifications:**

If OLM encounters a bundle that publishes a configuration schema using an unsupported JSON Schema specification, the bundle will be deemed invalid and an error will be surfaced to the user through the ClusterExtension status. If support for a specification needs to be dropped, the deprecation will follow standard OpenShift deprecation conventions and provide authors sufficient time to migrate to a more recent supported specification. In the event that a bundle is still installed using an unsupported specification version across an OLM upgrade, the same error will be surfaced to the user and upgrades and reconfigurations will not be possible until manual intervention occurs (e.g., manual migration of the configuration and bundle to a supported specification version).

Any validation errors will be surfaced to the user via the `Progressing` condition with a clear message, e.g.:
- invalid bundle configuration: unknown key 'foo'
- invalid bundle configuration: invalid value for field 'bar' must be less than 10 but got 11
- invalid bundle configuration: invalid type for field 'fizz' got boolean expected string
- invalid bundle configuration: missing required field 'buzz'
- invalid bundle configuration: bundle 'package.v1.0.0' does not support configuration

#### registry+v1 Configuration Schema

Since registry+v1 bundles don't provide configuration schemas, OLM will internally generate them for the bundle. This
enhancement introduces the initial schema which only contains the `watchNamespace` field. A future enhancement will
introduce additional fields without breaking compatibility (i.e. optional fields) to enable `Subscription` `.spec.config`
like customization.

For the purposes of this enhancement, whether a registry+v1 bundle provides a configuration schema, and whether
`watchNamespace` is required, or optional, or has any value restrictions depends on the bundle's install mode support.

The following table summarizes this relationship. Each row represents a bundle's supported install modes (marked with ✓ or -), and the resulting `watchNamespace` configuration requirements:

| AllNamespaces | SingleNamespace | OwnNamespace | WatchNamespace Configuration                                |
|---------------|-----------------|--------------|-------------------------------------------------------------|
| -             | -               | -            | undefined/error (no supported install modes)                |
| -             | -               | ✓            | required and must be install namespace                      |
| -             | ✓               | -            | required and must not be install namespace                  |
| -             | ✓               | ✓            | required and can be any namespace                           |
| ✓             | -               | -            | no configuration                                            |
| ✓             | -               | ✓            | optional and must be install namespace (default: unset)     |
| ✓             | ✓               | -            | optional and must NOT be install namespace (default: unset) |
| ✓             | ✓               | ✓            | optional and can be any namespace (default: unset)          | 

Given the tight scope and the small configuration surface, the validation may be done programmatically rather than through
the actual generation of a JSON-Schema. However, once this surface expands this will be the direction we'll take and
there should be no difference in behavior beyond the possible wording and structure of error messages.

Below are examples for what the JSON-Schemas may look like. Since OLM generates these schemas at runtime for registry+v1 bundles, it knows the install namespace from the ClusterExtension's `.spec.namespace` field and can directly embed namespace constraints in the schema using standard JSON Schema constructs:

**watchNamespace field required (can be any namespace)**

```json
{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "type": "object",
  "properties": {
    "watchNamespace": {
      "type": "string",
      "pattern": "^[a-z0-9]([-a-z0-9]*[a-z0-9])?$"
    }
  },
  "required": ["watchNamespace"]
}
```

**watchNamespace field optional (can be any namespace)**

```json
{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "type": "object",
  "properties": {
    "watchNamespace": {
      "type": ["string", "null"],
      "pattern": "^[a-z0-9]([-a-z0-9]*[a-z0-9])?$"
    }
  }
}
```

**watchNamespace cannot be the install namespace** (example where install namespace is "argocd")

```json
{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "type": "object",
  "properties": {
    "watchNamespace": {
      "type": ["string", "null"],
      "pattern": "^[a-z0-9]([-a-z0-9]*[a-z0-9])?$",
      "not": {
        "const": "argocd"
      }
    }
  }
}
```

**watchNamespace is optional and can only be the install namespace** (example where install namespace is "argocd")

```json
{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "type": "object",
  "properties": {
    "watchNamespace": {
      "type": ["string", "null"],
      "enum": ["argocd", null]
    }
  }
}
```

**Custom JSON Schema Formats for Future Bundle Formats:**

For future bundle formats where bundle authors provide their own configuration schemas (such as Helm charts with values.schema.json or registry+v2 bundles), OLM may provide custom JSON Schema format validators to help bundle authors create more robust configuration schemas:

- `namespaceName`: Would validate that the input is a valid Kubernetes namespace name (follows DNS-1123 subdomain format). This would be useful for any configuration field that accepts a namespace name.
- `isInstallNamespace`: Would validate that the input is a valid namespace name and matches the ClusterExtension's install namespace (`.spec.namespace`). Bundle authors could use this format to ensure a configuration value must be the install namespace.
- `isNotInstallNamespace`: Would validate that the input is a valid namespace name and does not match the install namespace. Bundle authors could use this format to ensure a configuration value must be different from the install namespace.

These custom formats would not be part of the standard JSON Schema specification but would be provided by OLM as helpers for bundle authors. Whether to implement these helpers will be decided when bundle formats that support author-provided schemas are implemented.

Example of potential future usage by bundle authors:
```json
{
  "type": "object",
  "properties": {
    "targetNamespace": {
      "type": "string",
      "format": "isNotInstallNamespace",
      "description": "The namespace to watch (must be different from install namespace)"
    }
  }
}
```

**Note**: For the registry+v1 bundle format described in this enhancement, OLM generates the schema at runtime and uses standard JSON Schema constructs directly (as shown in the examples above), so these custom formats are not used. 

#### registry+v1 Bundle Renderer Changes

Currently, OLM only supports AllNamespaces mode and promotes all `permissions` entries from the ClusterServiceVersion to `ClusterRole` and `ClusterRoleBinding` resources. This enhancement introduces namespace-scoped RBAC generation when `watchNamespace` is configured.

**Changes to RBAC Resource Generation:**

- **ClusterRole/ClusterRoleBinding**: `clusterPermissions` entries in the `ClusterServiceVersion` will continue to be created as `ClusterRole` and `ClusterRoleBinding` resources regardless of install mode

- **Role/RoleBinding**: `permissions` entries in the `ClusterServiceVersion` will be handled differently based on configuration:
  - **When `watchNamespace` is configured** (Single/OwnNamespace modes): Create `Role` and `RoleBinding` resources in the watch namespace (this is the new behavior introduced by this enhancement)
  - **When `watchNamespace` is not configured** (AllNamespaces mode): Continue current behavior of promoting to `ClusterRole` and `ClusterRoleBinding` resources

- **Operator Configuration**: The `olm.targetNamespaces` annotation will be set in the operator deployment's pod template to the value of `watchNamespace`, instructing the operator how to configure itself for the target namespace scope

#### Install Mode Upgrade Behavior Edge Case

An important edge case exists around bundle upgrades and install mode behavior consistency. Consider this scenario:

1. **Initial State**: An operator bundle (v1.0) supports only `OwnNamespace` install mode
2. **Installation**: User installs the operator without explicit `watchNamespace` configuration
3. **Bundle Update**: Operator author releases bundle (v1.1) that adds `AllNamespaces` support alongside existing `OwnNamespace` support
4. **Unintended Behavior**: Without proper validation, the system could automatically switch from `OwnNamespace` to `AllNamespaces` mode during upgrade, granting the operator cluster-wide permissions without user consent

This behavior is problematic because:
- **Security Implications**: Operators suddenly gain broader permissions than originally intended
- **Predictability**: Install mode changes occur without explicit administrator action
- **Consistency**: The same bundle configuration produces different permission scopes across versions

To address this edge case, the implementation requires explicit `watchNamespace` configuration for namespace-scoped bundles
(those not supporting `AllNamespaces` mode), ensuring that install mode selection is always deliberate and consistent across
bundle upgrades. We understand this doesn't produce the best UX for this particular case, but such bundles are rare, and
we'll address it in the future with bundle provided default configurations.

### Risks and Mitigations

#### 1. Unknown keys

Unknown keys in configuration can have undesired effects especially between bundle versions. If a configuration key
that is not part of the schema in one version but present in a subsequent version of the bundle is present, this
could violate user intent and potentially place the cluster in an undesired state.

*Mitigation*: For registry+v1 bundles (covered by this enhancement), OLM generates the configuration schemas and will ensure they reject unknown keys by omitting or setting `additionalProperties: false`. For future bundle formats where bundle authors provide their own schemas, whether unknown keys are rejected depends on how the author defines their schema. OLM will validate configuration strictly according to the provided schema. Documentation will encourage bundle authors to use strict schemas that reject unknown properties to prevent unintended configuration drift.

#### 2. Configuration Changes in Detectably Breaking Ways

If there's a detectable breaking change in the configuration schema, it could be possible to push undesirable changes
onto the cluster or even cause an outage.

*Mitigation*: Install/Update/Upgrade operations are only allowed to continue if the configuration strictly respects
the configuration schema

#### 3. Non-existing Configuration Schema

If a bundle does not define a configuration schema rendering _could_ still take place on a best-effort basis. However,
the system would be susceptible to undesirable changes being applied as no breaking changes could be detected.

*Mitigation*: For a bundle to be configurable, it must provide a configuration schema. If it does not, and
configuration is supplied, the operation will not progress.

Note: it's possible that in the future users can override this behavior if they are willing to accept the consequences.

#### 4. Secret Information

Often times, users might need to supply an application with secret/privileged information such as passwords, or
api keys. With the current inline approach, if this information *must* be entered in as plain text (as opposed to as
a reference to an existing secret, for instance), the privileged information would be reveled to anyone that can read
the ClusterExtension, or has access to etcd's data store or any backup of etcd's data store. It could also cause replication headaches as the privileged information might already exist in a
Secret somewhere, and if the information changes, it would need to be manually updated.

*Mitigations*:
- Don't give read access to ClusterExtensions, or ClusterExtensions with privileged information to anyone without access to the secret information.
- Don't install the application and contact the supplier and ask for a change in the configuration s.t. a secret reference can be given instead. This also solves the replication issue and ensures no additional copies of the information exist on the cluster between the source (Secret) and the point of use.

#### 5. Lack of Bundle Supplied Default Configuration

A bundle may not only want to provide a configuration schema, but also a default configuration beyond what schema
defined default values can offer. For instance, a Helm Chart would contain both a values.json (default configuration) and
a values.schema.json (bundle configuration schema). The lack of support for default configuration makes it hard to
detect drift in default configuration behavior that can cause undesired changes to the cluster state, and force users
to supply configuration even if it doesn't make immediate sense. For instance,

v1.0.0 of a hypothetical bundle supports only a single "behavior mode" (whatever that is) called `Simple`. When the user
installs v1.0.0, it is installed in the `Simple` behavior mode. Then, v1.1.0 comes along and adds a new behavior
mode `Awesome`, which becomes the new default. The upgrade to v1.1.0 would silently move to the new (default) behavior mode,
which could be undesirable. With default configuration support, we could detect a breaking change in the default behavior
and stop the flow until the user explicitly adds the configuration selecting the desired behavior mode.

*Mitigations*:
Require configuration from the user even when it feels superfluous. The configuration schema would always need to be 
designed to allow for changes without breaking the API. The lack of default configuration would make the case above
a bit jarring for the user because they would need to specify a behavior mode even though there's only one to choose from.
While OLMv1 only has support for registry+v1 bundles, this is a limitation we can live with. That is, there are very few, 
if any, bundles that _only_ support OwnNamespace and that would require users to specify the watchNamespace even though,
 logically, it could only be the install namespace.
However, support default values will be added by a future enhancement when we start to roll out Helm content support.

#### 6. Silent Switch in Default on Downgrade

Downgrading the cluster from the version that immediately introduces this feature might silently re-install
bundles that were installed in `Single-` or `OwnNamespace` mode, if they also support `AllNamespaces` mode.
Because the configuration won't be applied to the bundle, it will be installed with its default configuration.
Which for registry+v1 bundles is `AllNamespaces` mode.

*Mitigation*: Cluster downgrades are not a supported scenario. If a downgrade occurs (unsupported scenario), administrators should be aware that bundles previously configured with `watchNamespace` may revert to AllNamespaces mode. Administrators would need to manually verify operator permissions and behavior after any unsupported downgrade operation.

### Drawbacks

- Configuration validation happens at runtime and not at ingress-time by the kube apiserver
- Bundle configuration schema may evolve in a breaking way requiring manual intervention during auto-upgrade flows
- Bundle configuration schema may evolve in a non-detectable breaking way, e.g. changes in default behavior
- No support for default bundle configuration
- Secret information isn't occluded
- No way to inspect bundle configuration schema outside what will be provided by documentation
- Unideal install UX for registry+v1 bundles that only support OwnNamespaces

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

Once this is introduced, we can also smooth out the install experience for OwnNamespace only registry+v1 bundles. Rather
than expecting the user to set the watchNamespace to the install namespace, that can be the default configuration provided by the bundle.

### 4. Helm content support

OLMv1 would like to meet authors where they are and add support for Helm content. Handling custom configuration (values.json)
will be a requirement for this story.

### 5. registry+v2

OLMv1 will eventually add the next iteration of registry+v2. Author provided configuration schemas will be a requirement for this format.

### 6. Bundle configuration schema discoverability (outside the runtime)

In the future, we'd like an easy way to surface the bundle configuration schema to users via the console to help users
understand the configuration affordances for the different catalog packages, and bundles.

### 7. Operational risks of Single/OwnNamespace mode installations

Because we are enabling namespace-scoped operator installations, there are operational implications that could impact cluster management. These risks are mitigated by:

- **RBAC Misconfiguration**: Install mode validation ensures operators only receive permissions appropriate for their scope
- **Namespace Dependency**: Clear error messages when target namespaces don't exist or aren't accessible
- **Migration Complexity**: Comprehensive documentation and examples for transitioning between install modes
- **Permission Escalation**: ServiceAccount validation ensures adequate permissions without over-privileging
- **Unintended Install Mode Changes**: Requiring explicit `watchNamespace` configuration for namespace-scoped bundles prevents automatic permission scope escalation when bundle upgrades add new install mode capabilities

Currently, admins control the scope of operator installations through ClusterExtension RBAC. This enhancement adds namespace-level controls while maintaining existing security boundaries.

The feature is alpha and feature-gated, allowing administrators to:
- Control adoption timeline through feature gate management
- Test namespace-scoped installations in non-production environments
- Gradually migrate from AllNamespaces to more targeted install modes

Operators installed in Single/OwnNamespace modes have reduced blast radius compared to AllNamespaces installations, potentially improving cluster security posture.

| Risk                                | Impact | Mitigation                                                                                                              |
|-------------------------------------|--------|-------------------------------------------------------------------------------------------------------------------------|
| **Increased Complexity**            | Medium | Feature is alpha and feature-gated; clear documentation emphasizes this is transitional                                 |
| **RBAC Misconfiguration**           | High   | Comprehensive validation and clear error messages; documentation provides RBAC examples                                 |
| **Installation Failures**           | Medium | Detailed preflight checks and validation; clear error reporting                                                         |
| **Security Boundaries**             | Medium | Explicit validation of namespace permissions; RBAC properly scoped                                                      |
| **Feature Proliferation**           | Low    | Clear documentation that this is for legacy compatibility only                                                          |
| **Unintended Install Mode Changes** | High   | Mandatory `watchNamespace` configuration for namespace-scoped bundles prevents automatic mode switching during upgrades |

#### 7. JSON Schema 2020-12 Specification Support

The latest specification of the JSON Schema format is slowly gaining popularity though it does not quite have the same
tooling and library support as draft-07. In the future, we might want to add support for the latest specification if
there is sufficient demand for users. This would be an additive change to the architecture that would only require
the jsonschema library to understand the latest specification.

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
- [ ] Basic functionality for Single and OwnNamespace modes
- [ ] Unit and integration test coverage
- [ ] Documentation for configuration and usage

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
- **Configuration Migration**: No automatic migration from installed workloads currently being managed by OLMv0; users must explicitly install using OLMv1 ClusterExtension and configure `watchNamespace`.

### Downgrade Strategy

Remove the `.spec.config` fields from the CRs prior to upgrade. Keep in mind that installations would revert to their default
configuration, e.g.: a bundle that supports `AllNamespaces`, `SingleNamespace` is installed as `SingleNamespace`. It would be
reinstalled as `AllNamespaces` upon downgrade due to the configuration not being applied.

### Version Compatibility
- **Minimum Version**: Requires OpenShift 4.22+
- **Configuration Schema**: Uses existing ClusterExtension configuration schema for forward compatibility

## Operational Aspects of API Extensions

### Impact of Install Mode Extensions

**ClusterExtension API Enhancement:**
- **Architectural Impact:** The `config.inline.watchNamespace` field enables runtime install mode selection, moving from compile-time (bundle) to runtime (installation) configuration
- **Operational Impact:**
    - Administrators must understand namespace relationships and RBAC implications
    - Troubleshooting requires awareness of install vs watch namespace distinctions
    - Monitoring and alerting must account for namespace-scoped operator deployments

**RBAC Resource Generation:**
- **Architectural Impact:** Dynamic RBAC generation based on install mode creates different permission patterns for the same operator
- **Operational Impact:**
    - Permission debugging requires understanding of install mode impact on RBAC scope
    - Security auditing must consider namespace-level vs cluster-level permission grants

### Impact on Existing SLIs

With the removal of the install mode concept in OLMv1, operator packages that want to continue to use this stop-gap feature are expected to surface configuration documentation, and call out if the `watchNamespace` parameter is a part of it, along with usage examples etc. Until a broad percentage of operator packages, especially those that don't support `AllNamespaces` mode, take action to make such documentation available, a spike in bad installations is expected.

**Installation Success Rate:**

*   **RBAC Validation Complexity:** Namespace-scoped installations require more complex RBAC validation to ensure the ServiceAccount has appropriate permissions for the target namespace. RBAC misconfigurations that work in AllNamespaces mode may fail in Single/OwnNamespace modes.
    *   Example: ServiceAccount has cluster-wide read permissions but lacks namespace-specific write permissions, causing installation to fail.
*   **Bundle Compatibility Validation:** Additional validation layer to confirm bundles support the requested install mode. Bundles that only support AllNamespaces will fail when Single/OwnNamespace is requested.
    *   Example: Attempting to install a bundle with `watchNamespace: "test"` when the bundle CSV only declares support for AllNamespaces install mode.

**Installation Time:**

*   **RBAC Generation Complexity:** Converting CSV `permission` to namespace-scoped Role/RoleBinding resources requires additional processing time. Complex operators with extensive permission requirements will see increased installation duration.

**Operator Availability:**

*   **ServiceAccount Permission Dependencies:** Namespace-scoped operators depend on ServiceAccount permissions that may be modified by namespace administrators, creating additional failure points not present in cluster-scoped installations.
    *   Example: Namespace admin removes critical RoleBinding, causing operator to lose access to required resources.

### Possible Failure Modes

- Invalid bundle configuration

**Configuration Issues:**
- Invalid watchNamespace specification (DNS1123 validation failures)
- Target namespace doesn't exist or isn't accessible
- ServiceAccount lacks sufficient permissions for namespace access
- Bundle configuration does not include `watchNamespace`
- Bundle configuration requires `watchNamespace`
- Given `watchNamespace` cannot be used, e.g. it is set to the install namespace but `OwnNamespace` is not supported by the bundle

**Runtime Issues:**
- Operator deployed in install namespace but cannot access watch namespace
- RBAC resources incorrectly scoped for actual operator requirements
- Network policies preventing cross-namespace access when needed

### OCP Teams Likely to be Called Upon in Case of Escalation

1. OLM Team (primary)
2. OpenShift API Server Team
3. Networking Team (cross-namespace connectivity)
4. Authentication & Authorization Team (ServiceAccount/RBAC)
5. Layered Product Team

## Support Procedures

If there are problems with namespace-scoped operator installations:

1. **Check Namespace Existence**: Confirm target watch namespace exists and is accessible
2. **Validate ServiceAccount Permissions**: Verify ServiceAccount has required permissions for target namespace
3. **Review Bundle Compatibility**: Confirm bundle CSV supports the requested install mode
4. **Examine RBAC Resources**: Check generated Role/RoleBinding resources are correctly scoped

Common troubleshooting scenarios:
- **Installation Stuck**: Check namespace availability and ServiceAccount permissions
- **Operator Not Functioning**: Verify RBAC resources are correctly scoped to watch namespace
- **Permission Denied Errors**: Review ServiceAccount permissions and namespace access rights

For persistent issues, administrators can:
- If the bundle supports AllNamespaces mode: update the configuration by removing the watchNamespace config to fall back to AllNamespaces mode
- If AllNamespaces is not supported: remove the extension and contact the bundle author as this likely indicates a problem with the content
- Modify watchNamespace configuration to change install mode
- Scale down operator-controller to manually intervene if needed

## Version Skew Strategy

This feature is isolated to the operator-controller component, managed by cluster-olm-operator. Version skew strategy is not required.

### Component Interactions

The operator-controller is deployed by the cluster-olm-operator. There are no other component interactions. This
enhancement does not impact that interaction.

### API Compatibility
- **ClusterExtension API**: Uses existing configuration schema;
- **Bundle Format**: Works with existing registry+v1 bundles without modification
- **Status Reporting**: Uses existing condition and status mechanisms

### Deployment Considerations
- **Configuration Validation**: API server validates configuration schema regardless of feature gate state
- **Runtime Behavior**: Feature gate only affects installation behavior, not API acceptance

#### Feature Dependencies
- **Configuration Support**: This feature builds upon a ClusterExtension configuration infrastructure
- **RBAC Generation**: Leverages existing rukpak RBAC generation capabilities with enhanced scoping logic
- **Feature Gate Framework**: Uses established feature gate patterns for controlled rollout
