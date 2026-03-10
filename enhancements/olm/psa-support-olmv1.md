---
title: psa-support-olmv1
authors:
  - "@ankithom"
reviewers:
  - "@perdasilva"
  - "@joelanford"
  - "@jkeiser"
  - "@camacedo"
approvers:
  - "@joelanford"
api-approvers:
  - None
creation-date: 2026-03-09
last-updated: 2026-03-09
status: provisional
tracking-link:
  - https://issues.redhat.com/browse/OCPSTRAT-2690
see-also:
  - https://docs.google.com/document/d/1o6U77GTGTD5jF5LVlN8m_RjrDC2mCgIeVIoPmwHk2F8
  - https://docs.google.com/document/d/1vNcjHB01Yhtar8h2TnHq9c3utzuflxfbZUVDjjaw3AI
---

# PSA Support for OLMv1

## Summary

This enhancement proposes a mechanism for OLMv1 to support Pod Security Admission (PSA) requirements for ClusterExtension bundles in the registry+v1 format via the existing `operatorframework.io/suggested-namespace-template` CSV annotation, and extends OpenShift Console to apply these namespace templates during both installation and upgrade of OLMv1 extensions.

## Motivation

As of OpenShift 4.11, all clusters have Pod Security Admission enforcement enabled by default. OLMv1 currently lacks a clear approach for handling PSA requirements, requiring cluster administrators to configure extensions to run on PSA-enforcing clusters.

### User Stories

* As an extension bundle author, I want to specify the PSA requirements for my extension workloads, so that users installing my extension have the correct namespace configuration.

* As a cluster administrator using Console to install an OLMv1 extension, I want the installation process to automatically configure the namespace with the appropriate PSA labels, so that extension workloads can run without PSA violations.

* As a cluster administrator upgrading an OLMv1 extension, I want the upgrade process to update namespace PSA labels if the new version requires different PSA levels, so that the upgraded extension continues to function correctly.

* As a cluster administrator using CLI or GitOps workflows, I want to discover PSA requirements, so that I can configure namespaces appropriately.

* As an extension author migrating from OLMv0 to OLMv1, I want to reuse my existing `operatorframework.io/suggested-namespace-template` CSV annotation, so that I can maintain PSA support with minimal changes to my bundle.

### Goals

- Enable bundle authors to specify PSA requirements for registry+v1 bundles
- Support automatic application of PSA labels during extension installation and upgrade via Console
- Provide clear documentation for bundle authors on PSA configuration

### Non-Goals

- Defining PSA support for non-registry+v1 bundle formats that may be supported by OLMv1 in the future
- Continuous reconciliation of namespace PSA labels after initial application
- Managing PSA for namespaces with multiple extensions

## Proposal

This proposal leverages the existing `operatorframework.io/suggested-namespace-template` CSV annotation to enable PSA support for OLMv1 extensions. The approach involves two parts:

1. **Bundle Author Specification**: For any bundle with a specific PSA requirement, bundle authors must include a namespace template in the CSV annotations with the expected PSA labels. This annotation is already supported in registry+v1 bundles and automatically rendered into FBC as part of the `olm.csv.metadata` property.

2. **Console Integration**: Extend Console's namespace template handling for OLMv1 to apply namespace templates during both installation and upgrade, regardless of whether the namespace already exists.

### Proposed Changes

#### 1. Bundle Author Workflow

Bundle authors include PSA requirements in their CSV using the existing annotation:

```yaml
apiVersion: operators.coreos.com/v1alpha1
kind: ClusterServiceVersion
metadata:
  annotations:
    operatorframework.io/suggested-namespace-template: |-
      {
        "apiVersion": "v1",
        "kind": "Namespace",
        "metadata": {
          "name": "sample",
          "labels": {
            "pod-security.kubernetes.io/enforce": "privileged",
            "pod-security.kubernetes.io/audit": "privileged",
            "pod-security.kubernetes.io/warn": "privileged"
          },
          "annotations": {
            "openshift.io/node-selector": ""
          }
        }
      }
```

This annotation automatically appears in the FBC catalog as part of the `olm.csv.metadata` property, making it accessible to Console during installation and upgrade.

#### 2. Console Workflow Changes

**For Installation:**
- Console checks if the selected bundle contains an `operatorframework.io/suggested-namespace-template` annotation
- Apply namespace template (create namespace if it doesn't exist, or patch existing namespace with template metadata including PSA labels)

**For Upgrades:**
- Console checks if the target bundle version contains an `operatorframework.io/suggested-namespace-template` annotation
- If present, patch the namespace template to the extension's install namespace

### Workflow Description

**Bundle Author** is a developer creating an extension bundle that requires elevated PSA permissions.

**Cluster Administrator** is a user with cluster-admin permissions responsible for installing and managing extensions.

#### Scenario 1: Fresh Installation via Console

1. Bundle author determines their extension requires `privileged` PSA level
2. Bundle author adds `operatorframework.io/suggested-namespace-template` annotation to CSV with appropriate PSA labels
3. Bundle author publishes bundle to catalog; FBC automatically includes the annotation in `olm.csv.metadata` property
4. Cluster administrator navigates to Console and selects the extension to install
5. Console reads the bundle metadata and detects the namespace template
6. Console presents installation form
7. Cluster administrator chooses the install namespace name
8. Console applies the namespace template:
   - If namespace doesn't exist: Creates namespace using the template
   - If namespace exists: Patches namespace with the template
9. Console creates ClusterExtension CR; OLMv1 completes the installation
10. Extension workloads deploy successfully with appropriate PSA enforcement

#### Scenario 2: Upgrade Requiring PSA Change

1. Extension version 1.0 is installed with `baseline` PSA level
2. Extension version 2.0 requires `privileged` PSA level for new features
3. Bundle author updates namespace template in version 2.0 CSV with `privileged` PSA labels
4. Cluster administrator initiates upgrade to version 2.0 via Console
5. Console detects namespace template in version 2.0 bundle
6. Console automatically applies the namespace template, updating PSA labels to `privileged`
7. Upgrade proceeds and new workloads deploy successfully with elevated PSA permissions

#### Scenario 3: CLI/GitOps Installation (Non-Console)

1. Cluster administrator wants to install extension via CLI or GitOps
2. Cluster administrator queries the catalog to retrieve bundle metadata
3. Cluster administrator extracts `operatorframework.io/suggested-namespace-template` from `olm.csv.metadata` property
4. Cluster administrator manually creates/patches namespace with PSA labels from template
5. Cluster administrator creates ClusterExtension CR pointing to desired namespace
6. Extension workloads deploy successfully

**Alternative for CLI/GitOps:** Cluster administrator may instead set `security.openshift.io/scc.podSecurityLabelSync: "true"` on the namespace to leverage the OpenShift PSA label synchronization controller.

#### Failure Scenario: PSA Violation

1. Cluster administrator installs extension without applying namespace template
2. Extension workloads attempt to deploy
3. PSA admission controller blocks pod creation due to policy violation
4. Deployment status shows `ReplicaFailure` condition with detailed PSA violation message
5. Console displays warning about PSA violation
6. Cluster administrator reviews error message, identifies required PSA level
7. Cluster administrator manually patches namespace with correct PSA labels or reinstalls with namespace template applied
8. Workloads deploy successfully after namespace correction

### API Extensions

The proposed changes do not introduce new API extensions.

### Topology Considerations

#### Hypershift / Hosted Control Planes

The proposed changes should have no specific impact to HCP. 

#### Standalone Clusters

The proposed change should have no specific impact to standalone clusters. 

#### Single-node Deployments or MicroShift

The proposed change should have no specific impact to SNO/MicroShift clusters. 

#### OpenShift Kubernetes Engine

The proposed change is compatible with OpenShift Kubernetes Engine (OKE). PSA enforcement is a standard Kubernetes feature available in OKE. This enhancement does not force any OpenShift specific features onto how layered products manage PSA for their bundles.

### Implementation Details/Notes/Constraints

#### Assumptions

1. **Namespace templates are valid**: Bundle authors are responsible for providing valid namespace templates.

2. **One extension per namespace**: This design assumes at most one extension manages workloads in a given namespace. For multi-extension namespaces, cluster administrators must manually configure PSA labels.

3. **No continuous reconciliation**: Namespace templates are applied once during install/upgrade. There is no continuous reconciliation of PSA labels. If cluster administrators manually modify labels, OLM will not revert changes.

4. **User permissions**: Users installing extensions via Console are expected to have cluster-admin or equivalent permissions to create and modify namespaces.

5. **Console-driven workflow**: The primary workflow relies on Console for template application. CLI/GitOps users must extract and apply templates manually.

#### Technical Details

**FBC Property Structure**: The namespace template annotation is exposed in FBC as:

```yaml
schema: olm.bundle
image: sample/extension/bundle@sha256:6b3603c...
name: sample-bundle.v0.0.1
package: sample
properties:
- type: olm.csv.metadata
  value:
    annotations:
      operatorframework.io/suggested-namespace-template: |-
        {
          "apiVersion": "v1",
          "kind": "Namespace",
          "metadata": { ... }
        }
```

**Console Implementation**: Console must:
- Parse the namespace template JSON from the annotation
- Patch this template to either the selected namespace for a fresh extension install, or to an existing extension's install namespace for an upgrade.

**Namespace Patching**: When applying templates to existing namespaces, Console will use strategic merge patch to add or update labels/annotations without removing existing metadata not specified in the template.

**Compatibility with Label Synchronizer**: If a namespace has `security.openshift.io/scc.podSecurityLabelSync: "true"`, the label synchronizer will not overwrite manually-set PSA labels from the namespace template. Bundle authors can include this label in their template to opt into label synchronizer management.

### Risks and Mitigations

**Risk: Invalid namespace templates in bundles**
- *Impact*: Console fails to apply template, installation fails
- *Mitigation*: document requirements clearly for bundle authors; ensure namespace templates are valid

**Risk: Non-Console users (CLI, GitOps) unaware of PSA requirements**
- *Impact*: Deployments fail due to PSA violations
- *Mitigation*: Clear documentation on extracting namespace templates from FBC; error messages from PSA violations are descriptive; option to use label synchronizer

**Risk: Multiple extensions in same namespace with conflicting PSA requirements**
- *Impact*: One extension's PSA requirements may not satisfy another's
- *Mitigation*: Document assumption of one extension per namespace; recommend separate namespaces for extensions with different PSA needs

**Security Review**: PSA enforcement is a security feature. This enhancement does not weaken PSA enforcement, as:
- Bundle authors declare requirements explicitly
- PSA admission controller still enforces policies at pod creation time

**UX Review**: Console UX team should review:
- Upgrade workflow when PSA levels change
- Error messaging for PSA violations

### Drawbacks

**Increased user burden for CLI/GitOps workflows**: OLMv1 requires additional manual steps for non-Console users to extract required PSA levels from bundles. This is partially mitigated by the option to use the label synchronizer for non-`openshift-` namespaces.

**Limited to registry+v1 bundles**: This enhancement only addresses PSA for registry+v1 bundles. Future bundle formats will require separate consideration.

## Alternatives (Not Implemented)

### 1. Auto-labeling with Label Synchronizer

Automatically add the `security.openshift.io/scc.podSecurityLabelSync` label to namespaces watched by at least one extension (specified by `spec.namespace` on ClusterExtension).

**Pros:**
- Idempotent with OLMv0 behavior
- No changes required from bundle authors
- Behavior identical to other namespaces managed by label synchronizer (non-openshift, non-workload namespaces)

**Cons:**
- Continued dependence on OpenShift-specific label syncer (not portable)
- No intentionality in PSA level chosen by syncer—if bundles require elevated PSA, this should be an explicit choice
- Syncer may not choose appropriate PSA level for all workloads

**Why not chosen**: Bundle authors should explicitly declare PSA requirements rather than relying on automatic detection. This provides clarity and intentionality.

### 2. Specific PSA Properties on FBC

Add new FBC properties like `olm.psa` or `olm.psa.<enforce|audit|warn>[.version]` to contain PSA labels directly in FBC.

**Pros:**
- Properties tightly scoped to PSA purpose
- No reliance on CSV annotations

**Cons:**
- Proliferation of highly specific properties bloats FBC schema
- Redundant with existing namespace template mechanism
- Requires changes to FBC schema and tooling

**Why not chosen**: Reusing existing `suggested-namespace-template` annotation avoids schema changes and leverages established patterns.

### 3. Namespace Templates in ClusterExtension spec.config

Allow cluster administrators to specify namespace labels (including PSA) directly in ClusterExtension CR:

```yaml
apiVersion: olm.operatorframework.io/v1
kind: ClusterExtension
metadata:
  name: sample-ex
spec:
  config:
    configType: Inline
    inline: |
      {
        "watchNamespace": "samplens",
        "namespaceLabels": {
          "pod-security.kubernetes.io/enforce": "privileged",
          "pod-security.kubernetes.io/audit": "privileged",
          "pod-security.kubernetes.io/warn": "privileged",
          "security.openshift.io/scc.podSecurityLabelSync": "false"
        }
      }
```

OLM would be responsible for applying labels to the namespace.

**Pros:**
- Cluster admin controls namespace setup through OLM
- Does not rely on Console
- Applicable to all bundle formats

**Cons:**
- Requires cluster admin to know PSA requirements for every extension version
- More complex for users
- Shifts responsibility from bundle authors to cluster admins
- Requires OLM controller to manage namespace metadata (new responsibility)

**Why not chosen**: Bundle authors are best positioned to declare PSA requirements. Cluster admins should not need to research requirements for every extension version.

### 5. Namespace Manifests or Patches in Bundle

Include namespace manifests directly in the bundle. OLM ensures correct install order.

**Pros:**
- Manifests directly present in bundle
- OLM controls entire deployment including namespace

**Cons:**
- registry+v1 bundles don't support namespace manifests
- requires republishing bundles

**Why not chosen**: registry+v1 limitation and architectural concerns about bundles managing namespaces.

### 6. Limit Installation to Non-openshift Namespaces

Require extensions to install in non-`openshift-` namespaces, which are automatically managed by the PSA label synchronizer.

**Pros:**
- Non-openshift, non-workload namespaces already managed by label syncer
- No action required from bundle authors

**Cons:**
- Many Red Hat/OpenShift extensions use `openshift-*` namespaces
- Would break existing bundle conventions
- Doesn't solve the problem for vanilla Kubernetes clusters

**Why not chosen**: Not viable for the many extensions that require `openshift-*` namespaces.

### 7. Do Nothing

Leave namespace setup entirely to cluster administrators.

**Pros:**
- Cluster admin has full control over namespace configuration
- Can integrate into GitOps workflows independently
- May use label syncer, non-openshift namespaces, or manual PSA labels

**Cons:**
- Poor UX: Cluster admin must research and understand bundle PSA requirements for every extension and version

**Why not chosen**: Poor user experience and high operational burden on cluster administrators.

## Test Plan

### Unit Tests
- Validate namespace template JSON parsing in Console
- Test strategic merge patch logic for namespace metadata
- Test error handling for invalid namespace templates

### Integration Tests
- Install extension with namespace template on non-existent namespace (should create with labels)
- Install extension with namespace template on existing namespace (should patch with labels)
- Upgrade extension with changed PSA requirements (should update labels)
- Install extension without applying template (should succeed, workloads may fail PSA)
- Verify namespace template from FBC is correctly passed to Console

### E2E Tests
- End-to-end installation via Console with namespace template application
- End-to-end upgrade with PSA level change
- Verify workloads deploy successfully after template application
- Verify PSA violations are detected and surfaced when template not applied
- Test CLI installation workflow (manual template extraction and application)
- Test interaction with PSA label synchronization controller

### Managed OpenShift Testing
- Verify Console workflow on managed OpenShift (OSD, ROSA, ARO)
- Test with various PSA levels (`privileged`, `baseline`, `restricted`)
- Validate upgrade scenarios across versions with different PSA requirements

## Graduation Criteria

### Dev Preview -> Tech Preview

- Console implementation complete for installation and upgrade workflows
- Documentation for bundle authors on namespace template usage
- End-to-end tests passing
- Feedback gathered from early adopters (bundle authors and cluster administrators)

### Tech Preview -> GA

- Validation of CSV annotations integrated into OpenShift specific catalog validation 
- Console UX review completed and feedback addressed
- Security review completed
- Feedback from Tech Preview users addressed

## Upgrade / Downgrade Strategy

### Upgrade Strategy

No migration required. 

For Console:
- Existing extensions run unchanged.
- On extension install or upgrade, apply namespace template if specified on the CSV.

For bundle authors:
- Include PSA labels on bundle CSV if necessary.
- Have a new version of the bundle with the namespace template released as a patch version upgrade to allow users access to a bundle version that specified PSA requirements

### Downgrade Strategy

On console downgrade, namespace templates in extension bundles no longer apply while installing or upgrading an extension. Any pre-existing namespace labels will remain in place, but cluster administator will need to update labels as needed.

## Version Skew Strategy

### Console and OLM Version Skew

- Namespace template application is a Console-side operation
- Older Console + Newer OLM: Templates not applied, but extensions can still be installed (degraded UX)
- Newer Console + Older OLM: Console can apply templates; all bundles shipped with catalogs may not have namespace templates they need(requires manual fix).

### Component Version Skew During Upgrade

- During OpenShift cluster upgrade, Console may be upgraded before or after extension controllers
- Worst case: Template not applied during upgrade window; can be manually corrected afterward

## Operational Aspects of API Extensions

This enhancement does not introduce API extensions (CRDs, webhooks, aggregated API servers, finalizers).

**Operational considerations:**
- Console must have permissions to create and patch namespace resources
- Namespace patch operations are standard Kubernetes API calls with negligible performance impact
- No new controllers or reconciliation loops introduced
