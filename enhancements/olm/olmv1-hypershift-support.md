---
title: olmv1-hypershift-support
authors:
  - "@jkeister"
reviewers:
  - "@joelanford, OLM architect"
  - "@jparrill, for HyperShift component integration patterns"
approvers:
  - "@joelanford, for OLM"
  - "@oceanc80, for OLM"
  - "@jparrill, for HyperShift"
api-approvers:
  - None
creation-date: 2026-03-26
last-updated: 2026-04-02
status: provisional
tracking-link:
  - https://redhat.atlassian.net/browse/OPRUN-4317
see-also: []
replaces: []
superseded-by: []
---

# OLMv1 HyperShift Support

## Summary

This enhancement adds HyperShift support to OLMv1 components (catalogd and operator-controller), enabling hosted clusters to install and manage operators via ClusterExtension resources. Two viable architectural approaches are provided: Control Plane Placement (catalogd in management cluster) and Data Plane Placement (catalogd on worker nodes). Both approaches use existing CRDs, require no code changes to controllers (leveraging standard KUBECONFIG environment variable support), and maintain operational parity with standalone clusters including centralized TLS certificate management and consistent upgrade condition reporting.

## Terminology and Components
- management cluster: the cluster used for hypershift for identified control-plane components
- hosted cluster: the cluster used for hypershift for all other components; data-plane components execute physically here, control-plane components for the hosted cluster are logically here
- hosted cluster namespace: the namespace created on the _management cluster_ in which all the control-plane components for a hosted cluster execute, plus KAS, ETCD and other core kubernetes components for the hosted cluster's operation
- cluster-olm-operator: the 2nd-level component, which installs operator-controller and catalogd; in standalone topology this is managed by CVO
- operator-controller: the 3rd-level component responsible for continuously reconciling cluster installables from catalogd in the form of the ClusterExtension resource
- catalogd: the 3rd-level component responsible for continuously reconciling cluster installable catalogs in the form of the ClusterCatalog resource, and for serving catalog metadata to operator-controller and the OpenShift console

## Motivation

HyperShift is the multi-tenancy approach endorsed by OpenShift. Currently, OLMv1 components (cluster-olm-operator, catalogd, and operator-controller) only support standalone cluster deployments. Without explicit HyperShift support, OLMv1 would only be deployable as data-plane components on the hosted cluster, where they could not be guaranteed to continue to work if worker nodes there are paused/scaled-down.


### User Stories

- As a hosted cluster admin, I want to install content from ClusterCatalogs via OLMv1 using ClusterExtension resources in my hosted cluster API, so that I can manage operators the same way as in standalone clusters.

- As a hosted cluster admin, I want to have access to default ClusterCatalog resources in my hosted cluster API, so that I can install ClusterExtensions from a common set of catalogs.

- As a hosted cluster admin, I want to create custom ClusterCatalog resources in my hosted cluster API, so that I can add private catalogs specific to my tenant.

- As a platform admin, I want each hosted cluster to automatically receive version-appropriate default catalogs (matching its OCP version including patch level), so that operators are compatible with the cluster version.

- As a platform admin, I want centralized TLS algorithm management for catalogd to work identically in both standalone and HyperShift deployments, so that I can maintain consistent security posture across deployment models without special-casing HyperShift.

- As a cluster admin, I want cluster-olm-operator to report upgradeable==false conditions consistently in both standalone and HyperShift configurations, so that upgrade orchestration can detect and respect OLMv1 health status regardless of topology.

### Goals

- Support OLMv1 (cluster-olm-operator, catalogd, operator-controller) in HyperShift hosted clusters
- Minimize customization of existing components and creation of bespoke components
- Maintain operational parity with standalone clusters (TLS management, upgrade conditions)
- Ensure complete tenant isolation between hosted clusters

### Non-Goals

- Implementing a single centralized catalogd serving all hosted clusters (scalability and security concerns)
- Creating namespace-scoped Catalog CRDs
- Adding support to catalogd to pull content from both management and hosted clusters, for cached default catalogs and custom catalogs, respectively
- Changing the deployment model to support multiple catalogd deployments per operator-controller deployment (i.e. management and hosted clusters)
- Modifying the ClusterCatalog or ClusterExtension API

## Proposal

This proposal enables OLMv1 HyperShift support through per-hosted-cluster catalogd deployments that serve both default and custom catalogs. 

- catalogd runs in management cluster (`clusters-{name}` namespace)
- operator-controller has local access to catalogd ClusterCatalog service API
- Aligns with HyperShift default (90%+ deployments)
- Requires external route for console-operator access
- Simplest deployment model
- Use existing ClusterCatalog and ClusterExtension CRDs (cluster-scoped)
- Store all ClusterCatalogs in hosted cluster's API server (default + custom)
- Single catalogd instance serves both catalog types
- catalogd version exactly matches hosted cluster OCP version (including patch)
- Zero code changes to catalogd/operator-controller (standard KUBECONFIG env var)
- Complete tenant isolation

### Workflow Description

**hosted cluster admin** is a user responsible for managing operators in their hosted cluster.

1. The hosted cluster admin sits down at the OpenShift console with credentials for their hosted cluster API
2. console-operator lists ClusterCatalogs from hosted cluster API and discovers both default (platform-managed) and custom (tenant-created) catalogs
3. For each catalog, console-operator reads `ClusterCatalog.Status.URLs.Base` and makes HTTP request to catalogd
4. catalogd returns catalog content
5. console-operator composes a catalog inventory page to hosted cluster admin
6. The hosted cluster admin selects a catalog entity to install
7. console-operator creates a new ClusterExtension resource in the hosted cluster API
8. operator-controller (running in management cluster, watching hosted cluster API via KUBECONFIG) sees the new ClusterExtension
9. operator-controller lists ClusterCatalogs from hosted cluster API and discovers both default (platform-managed) and custom (tenant-created) catalogs
10. operator-controller reads `ClusterCatalog.Status.URLs.Base` and makes HTTP request to catalogd
11. catalogd returns catalog content
12. operator-controller resolves the bundle and creates Deployment in hosted cluster API
13. Hosted cluster scheduler places operator pods on worker nodes
14. operator-controller updates ClusterExtension status
15. The hosted cluster admin verifies installation via console page or `kubectl get clusterextensions`
16. The operator runs as a pod in the hosted cluster's data-plane, visible to the admin

### API Extensions

This enhancement does not introduce new APIs, CRDs, admission webhooks, conversion webhooks, aggregated API servers, or finalizers. It uses the existing ClusterCatalog and ClusterExtension CRDs:

- **ClusterCatalog**: Cluster-scoped CRD storing catalog references (both default platform-managed and custom tenant-created)
- **ClusterExtension**: Cluster-scoped CRD for operator installation requests

Both CRD instances are stored in the hosted cluster's API server. The enhancement modifies deployment patterns for existing OLMv1 components but does not change their API surface.

### Topology Considerations

#### Hypershift / Hosted Control Planes

This enhancement is HyperShift-specific.

#### Standalone Clusters

This enhancement does not affect standalone cluster deployments. OLMv1 continues to operate in the `olmv1-system` namespace with catalogd and operator-controller using in-cluster configuration.

#### Single-node Deployments or MicroShift

Not applicable to single-node or MicroShift deployments. This enhancement is HyperShift-specific.

#### OpenShift Kubernetes Engine

Not applicable to OKE deployments. This enhancement is HyperShift-specific.

### Implementation Details/Notes/Constraints

#### Operational Parity with Standalone Clusters

This enhancement ensures HyperShift deployments maintain the same operational characteristics as standalone clusters:

**TLS Certificate Management**

catalogd uses OpenShift service-ca for TLS certificates via the `service.beta.openshift.io/serving-cert-secret-name` annotation on the catalogd service. This works identically in both standalone (`olmv1-system` namespace) and HyperShift (`clusters-{name}` or `openshift-catalogd` namespace) deployments. The service-ca operator automatically provisions and rotates certificates regardless of topology.

Example service annotation:
```yaml
metadata:
  annotations:
    service.beta.openshift.io/serving-cert-secret-name: catalogd-service-cert
```

**Upgrade Condition Reporting**

cluster-olm-operator requires dual-API access in HyperShift deployments:

- **Hosted Cluster API**: Manage ClusterCatalog and ClusterExtension resources, monitor operator installation health
- **Management Cluster API**: Report ClusterOperator conditions (Upgradeable, Available, Degraded) to the management cluster

This ensures the HyperShift control plane operator can detect OLMv1 health and block upgrades when `Upgradeable==False`, maintaining the same upgrade safety guarantees as standalone clusters.

Implementation pattern:
- Use in-cluster config for management cluster API access
- Use KUBECONFIG environment variable for hosted cluster API access
- Report conditions to management cluster's ClusterOperator resource
- Block hosted cluster upgrades when OLMv1 is degraded or not upgradeable

#### KUBECONFIG Environment Variable Support

Both catalogd and operator-controller use standard controller-runtime configuration which automatically respects the `KUBECONFIG` environment variable. No code changes are required.

**How it works:**
- `ctrl.GetConfigOrDie()` checks `KUBECONFIG` environment variable first
- Falls back to in-cluster config if `KUBECONFIG` is not set
- Standard client-go behavior, already implemented

**HyperShift deployment pattern:**
1. Mount admin-kubeconfig secret as volume at `/etc/openshift/kubeconfig/kubeconfig`
2. Set `KUBECONFIG` environment variable to mounted path
3. Do NOT use `--kubeconfig` flag (rely on standard client-go behavior)

Example deployment configuration:
```yaml
env:
- name: KUBECONFIG
  value: /etc/openshift/kubeconfig/kubeconfig
volumeMounts:
- name: kubeconfig
  mountPath: /etc/openshift/kubeconfig
  readOnly: true
volumes:
- name: kubeconfig
  secret:
    secretName: admin-kubeconfig
```

#### Per-Hosted-Cluster catalogd Deployment

Each hosted cluster receives its own catalogd instance whose version exactly matches the hosted cluster's OCP version (including patch level). This ensures:

- Perfect version alignment (catalogd v1.10.0 for OCP 4.22.1, catalogd v1.10.2 for OCP 4.22.3)
- Complete isolation (no shared state between hosted clusters)
- Simpler architecture (no cross-namespace routing)

**Why per-hosted-cluster instead of shared:**

Version skew problem: Hosted clusters run different OCP versions, including different patch versions. Catalogd controller versions may differ between patch releases due to backported security fixes and bug patches. A shared catalogd cannot be guaranteed to serve version-appropriate catalogs for multiple OCP patch versions simultaneously.

Resource overhead: ~50-100Mi memory per hosted cluster vs hypothetical shared model. This trade-off is accepted for perfect version alignment and complete isolation.

#### Default Catalog Provisioning

The HyperShift control plane operator extracts default catalog information from the hosted cluster's release image and provisions ClusterCatalog resources in the hosted cluster API.

**Version-to-catalog-image mapping:**
1. Read `HostedCluster.spec.release.image` (e.g., `quay.io/openshift-release-dev/ocp-release:4.15.1-x86_64`)
2. Extract version from release metadata
3. Generate default catalog images using version-based naming:
   - `registry.redhat.io/redhat/redhat-operator-index:v4.15.1`
   - `registry.redhat.io/redhat/certified-operator-index:v4.15.1`
   - `registry.redhat.io/redhat/community-operator-index:v4.15.1`
4. Create ClusterCatalog resources in hosted cluster API with extracted image references
5. catalogd unpacks and serves these catalogs

**Example ClusterCatalog (platform-managed):**
```yaml
apiVersion: olm.operatorframework.io/v1
kind: ClusterCatalog
metadata:
  name: redhat-operators
  labels:
    olm.operatorframework.io/default-catalog: "true"
    olm.operatorframework.io/provided-by: "platform"
spec:
  source:
    type: Image
    image:
      ref: registry.redhat.io/redhat/redhat-operator-index:v4.15.1
```

#### Component Specifications

**catalogd Deployment**

- Location: `clusters-{name}` namespace
- KUBECONFIG env var points to hosted cluster admin kubeconfig
- Watches ClusterCatalog resources from hosted cluster API
- Serves HTTP at `https://catalogd.{namespace}.svc:8443/catalogs/{catalog-name}/api/v1/all`
- Updates `ClusterCatalog.Status.URLs.Base` with service endpoint
- TLS certificates managed by service-ca via annotation

**operator-controller Deployment**

- Location: `clusters-{name}` namespace (management cluster)
- KUBECONFIG env var points to hosted cluster admin kubeconfig
- Watches ClusterExtension resources from hosted cluster API
- Lists ClusterCatalog resources from hosted cluster API to discover catalogs
- Makes HTTP requests to catalogd
- Creates operator Deployment/StatefulSet in hosted cluster API
- Installed operators run on hosted cluster worker nodes

**cluster-olm-operator Deployment (Dual-API)**

- Location: `clusters-{name}` namespace (management cluster)
- **Hosted Cluster API access**: Via KUBECONFIG env var for ClusterCatalog management
- **Management Cluster API access**: Via in-cluster config for ClusterOperator status reporting
- Reports Upgradeable, Available, Degraded conditions to management cluster
- Provisions default ClusterCatalog resources in hosted cluster API
- Monitors OLMv1 health and blocks upgrades when necessary

#### HyperShift Component Integration

A shared kubeconfig injection utility provides consistent KUBECONFIG mounting for all OLMv1 components:

**Shared Utility** (`/control-plane-operator/controllers/hostedcontrolplane/v2/olm/util.go`):

```go
// InjectHostedClusterKubeconfig adds volume, volume mount, and KUBECONFIG env var
// to enable OLMv1 components to watch the hosted cluster's API server.
func InjectHostedClusterKubeconfig(ctx component.WorkloadContext, deployment *appsv1.Deployment) error {
    // Add admin-kubeconfig volume
    // Add volume mount to all containers
    // Set KUBECONFIG environment variable
    // ctrl.GetConfigOrDie() automatically uses this
}
```

**Component Registration:**
- `/control-plane-operator/controllers/hostedcontrolplane/v2/catalogd/component.go`
- `/control-plane-operator/controllers/hostedcontrolplane/v2/operatorcontroller/component.go`
- `/control-plane-operator/controllers/hostedcontrolplane/v2/clusterolmoperator/component.go`

Each component calls `InjectHostedClusterKubeconfig` in its adapt function to configure hosted cluster API access.

#### RBAC Requirements

**Management Cluster RBAC:**
- cluster-olm-operator ServiceAccount needs permissions to update ClusterOperator status

**Hosted Cluster RBAC:**
- catalogd ServiceAccount needs full permissions on ClusterCatalog resources
- operator-controller ServiceAccount needs permissions on ClusterExtension resources and operator installation (Deployments, Services, etc.)
- cluster-olm-operator ServiceAccount needs permissions on ClusterCatalog and ClusterExtension resources

RBAC resources are created in both the management cluster (for management API access) and hosted cluster API (for hosted cluster resource access).

### Risks and Mitigations

**Risk: catalogd as single point of failure**

Impact: If catalogd fails, catalog access is broken for one hosted cluster

Mitigation:
- Standard Kubernetes health checks and restart policies
- Could run multiple replicas if high availability is required
- Failure only affects one hosted cluster (complete isolation)

**Risk: Kubeconfig secret management**

Impact: catalogd and operator-controller need kubeconfigs for hosted cluster API access

Mitigation:
- Leverage HyperShift's existing secret management infrastructure
- Use hosted cluster admin kubeconfig (already managed by HyperShift)
- Document secret rotation procedures
- Standard Kubernetes secret management practices apply

**Risk: ClusterCatalog name collisions**

Impact: Tenant creates ClusterCatalog with same name as platform-managed default catalog

Mitigation:
- Platform-managed default catalog names are well-known and documented
- Best practice: custom catalogs avoid reserved names (`redhat-operators`, `certified-operators`, etc.)
- Platform could use admission webhooks to prevent collisions (optional)
- catalogd serves all ClusterCatalogs in the hosted cluster API without conflict

**Risk: Per-hosted-cluster catalogd resource usage**

Impact: Running separate catalogd instances per hosted cluster (instead of shared) increases resource footprint

Mitigation:
- Default catalogs use minimal resources (read-only FBC serving, typically <100Mi memory per instance)
- Resource requests/limits tuned for catalog workload
- Horizontal pod autoscaling not needed for read-only serving
- Trade-off accepted for perfect version alignment and isolation
- Resource overhead: ~50-100Mi per hosted cluster vs hypothetical shared model
- For 100 hosted clusters: ~5-10Gi additional memory total (acceptable in large management clusters)

**Risk: Version-to-catalog-image mapping accuracy**

Impact: Incorrect catalog image version could be used for a hosted cluster's OCP version

Mitigation:
- Version extraction from release image is deterministic
- Use well-defined naming convention for catalog images (e.g., `v4.15.1`)
- Validate catalog image exists before creating ClusterCatalog
- E2E tests validate correct catalog images for each OCP version
- Clear error messages if catalog image cannot be determined or pulled

### Drawbacks

**Resource Usage Trade-off**

Per-hosted-cluster catalogd deployment consumes ~50-100Mi memory per hosted cluster vs a hypothetical shared model. This trade-off is accepted because:

- Perfect version alignment requires per-cluster catalogd (patch version differences)
- Complete tenant isolation is a security requirement
- Resource overhead scales linearly but remains acceptable for large deployments
- For 100 hosted clusters: ~5-10Gi additional memory total

**Approach-Specific Costs**

- Requires external route exposure for console-operator access
- Public catalog endpoint (security consideration for catalog content)
- DNS and load balancer dependencies

## Alternatives (Not Implemented)

### Alternative 1: Split Deployments across Control-Plane and Data-Plane

**Description:** Existing cluster-olm, operator-controller components would deploy to control-plane, catalogd deployment to data-plane

- catalogd deployment in `openshift-catalogd` namespace (worker nodes)
- operator-controller deployment in `clusters-{name}` namespace (management cluster)
- operator-controller uses Konnectivity sidecar to access catalogd on worker nodes
- console-operator has direct access to catalogd service API
- No external route required
- cluster-olm-operator uses dual-API access pattern

**Pros:**
- Use existing ClusterCatalog and ClusterExtension CRDs (cluster-scoped)
- Store all ClusterCatalogs in hosted cluster's API server (default + custom)
- Single catalogd instance serves both catalog types
- catalogd version exactly matches hosted cluster OCP version (including patch)
- Zero code changes to catalogd/operator-controller (standard KUBECONFIG env var)
- Complete tenant isolation

**Cons:**
- Consumes worker node resources (~200Mi per hosted cluster)
- Requires Konnectivity sidecar for operator-controller access
- More complex deployment (likely cannot re-use cluster-olm-operator, so we have divergent CVO, HCP integration)
- Does not align with HyperShift OLMv0 default (`olmCatalogPlacement: management`)

Rejected - complexity, component reuse, pause impacts

### Alternative 2: Single Centralized catalogd

**Description:** One catalogd in management cluster watches all hosted cluster APIs

**Pros:**
- Single component to manage
- Centralized metrics

**Cons:**
- Requires credentials for all hosted cluster APIs (complex secret management)
- Single point of failure for all tenants
- Scaling bottleneck with many hosted clusters
- Complex tenant filtering logic
- Version skew problem: Cannot serve different catalogd controller versions for different OCP patch versions

Rejected - scaling, security, and version skew concerns

### Alternative 3: Direct Multi-Backend Support in operator-controller

**Description:** operator-controller has built-in multi-catalogd client spanning management and hosted clusters where management catalogd instance governs shared default catalogs and hosted cluster catalogd instance governs custom catalogs

**Pros:**
- No proxy component needed
- Direct communication reduces hops

**Cons:**
- Requires operator-controller code changes
- Breaks single-cluster deployment model
- Each component needs multi-backend logic
- Violates "indistinguishable from standalone" requirement

Rejected - requires code changes, violates operator-controller <--> catalogd deployment alignment

### Alternative 4: Namespace-Scoped Catalog CRD

**Description:** New namespace-scoped Catalog CRD + federated client

**Pros:**
- Clean API model
- Native Kubernetes multi-tenancy

**Cons:**
- Requires new CRD and API version
- operator-controller needs federated client (code changes)
- Doesn't meet "indistinguishable from standalone" requirement
- More complex migration path

Rejected - requires new APIs and code changes

### Alternative 5: ClusterCatalog Mirroring Controller

**Description:** Controller mirrors management cluster ClusterCatalogs into each hosted cluster API as read-only references

**Pros:**
- operator-controller discovers all catalogs from single API
- Simple routing logic

**Cons:**
- **Version skew problem:** Cannot handle hosted clusters running different OpenShift versions
- Requires version-aware mirroring logic
- Adds mirroring controller component complexity
- Duplicate ClusterCatalog resources across APIs
- URL rewriting needed
- Potential name collision issues

Rejected - doesn't solve version skew, adds unnecessary complexity

### Alternative 6: Version-Specific Shared Control Plane

**Description:** Shared catalogd instances per OCP version in `olmv1-system-{major}-{minor}-{patch}` namespaces for DEFAULT catalogs only. Each hosted cluster also gets separate catalogd for CUSTOM catalogs.

**Pros:**
- Resource efficiency for defaults (multiple HCs share catalogd)
- Deduplication of catalog image unpacking

**Cons:**
- **Dual catalogd instances required:** One shared (defaults) + one per-HC (customs)
- **Complex routing:** operator-controller must route based on catalog type
- **console-operator must aggregate:** Query TWO external endpoints
- **Different Status.URLs.Base:** Defaults point to `olmv1-system-*` namespace, customs to `clusters-*` namespace
- **Prohibitive console complexity:** ~2× latency, complex aggregation, mixed failure modes
- **Failure blast radius:** Shared catalogd failure affects all HCs at that version

Rejected - console-operator complexity is prohibitive, custom catalog isolation requirement defeats resource efficiency goals

### Alternative 7: Hybrid (Default Management, Custom Guest)

**Description:** DEFAULT catalogs served by shared version-specific catalogd in management cluster. CUSTOM catalogs served by per-HC catalogd in data plane.

**Pros:**
- Resource efficiency for defaults
- Custom catalog isolation

**Cons:**
- **Maximum complexity:** Two completely different deployment patterns
- **Dual routing logic:** operator-controller routes differently based on catalog type
- **Mixed network paths for console-operator:** External Route for defaults, cluster service for customs
- **Complex console-operator aggregation:** Must handle mixed URL types and latency profiles
- **Most failure modes:** External (defaults) + Konnectivity (operator-controller to customs)
- **Hardest to debug:** Catalog components in multiple locations with different access patterns

Rejected - complexity far outweighs benefits, combines worst aspects of both approaches

## Open Questions [optional]

Which is the greatest concern:  immunity to worker pause or avoiding an external route per catalogd instance? 
The answer determines whether the current approach or alternative 1 is selected.

## Test Plan

**Unit Tests:**
- KUBECONFIG environment variable support in catalogd (verify ctrl.GetConfigOrDie() behavior)
- KUBECONFIG environment variable support in operator-controller
- Version-to-catalog-image mapping logic
- Default ClusterCatalog provisioning in hosted cluster API

**Integration Tests:**
- catalogd serving both default and custom catalogs from single instance
- operator-controller discovering catalogs from hosted cluster API
- ClusterExtension creation and operator installation flow
- RBAC permissions for hosted cluster API access
- TLS certificate generation via service-ca

**E2E Tests (HyperShift CI):**
- Create hosted cluster with specific OCP version
- Verify default ClusterCatalogs created with correct image versions
- Install operator from default catalog via ClusterExtension
- Create custom ClusterCatalog in hosted cluster API
- Install operator from custom catalog
- Verify operator runs on hosted cluster worker nodes
- Verify ClusterExtension status updated correctly

**Version Skew Testing:**
- Create multiple hosted clusters at different OCP patch versions (4.15.1, 4.15.3, 4.16.0)
- Verify each gets catalogd matching its exact version
- Verify each gets version-appropriate default catalog images
- Install operators in each cluster and verify correct bundle versions
- Confirm complete isolation (no cross-cluster catalog visibility)

**Multi-Cluster Scale Testing:**
- Deploy 50+ hosted clusters in single management cluster
- Verify each gets isolated catalogd instance
- Measure resource consumption (memory, CPU)
- Verify operator installation latency remains acceptable
- Confirm no performance degradation with cluster count

**Operational Testing:**
- catalogd failure and recovery (verify only one hosted cluster affected)
- Service-ca certificate rotation (verify TLS certificates updated)
- cluster-olm-operator upgrade condition reporting (verify Upgradeable==False blocks upgrades)
- KUBECONFIG secret rotation (verify components reconnect)

## Graduation Criteria

### Dev Preview -> Tech Preview

- Ability to install operators in hosted clusters via ClusterExtension
- E2E tests in HyperShift CI covering basic operator installation
- End user documentation for hosted cluster admins
- Version skew testing with at least 2 different OCP versions
- Sufficient test coverage for catalogd and operator-controller KUBECONFIG support
- Gather feedback from early adopters

### Tech Preview -> GA

- More testing (upgrade, downgrade, scale)
- Multi-cluster scale testing (50+ hosted clusters)
- Version skew testing across multiple patch versions
- Performance benchmarks documented
- Available by default in HyperShift deployments
- Production-ready monitoring and alerting
- User facing documentation created in openshift-docs
- cluster-olm-operator upgrade condition reporting verified
- Service-ca TLS certificate management verified across topologies

**For non-optional features moving to GA, the graduation criteria must include end to end tests.**

E2E tests required for GA:
- Operator installation from default catalogs in hosted cluster
- Operator installation from custom catalogs in hosted cluster
- Version skew scenarios with different OCP patch versions
- Multi-hosted-cluster scenarios on single management cluster
- cluster-olm-operator blocking upgrades when installed content has maxOcpVersion of current version

### Removing a deprecated feature

Not applicable. This is a new feature, not deprecating existing functionality.

## Upgrade / Downgrade Strategy

**Upgrade Strategy:**

catalogd version automatically tracks hosted cluster OCP version. When a hosted cluster is upgraded:

1. HyperShift control plane operator detects `HostedCluster.spec.release.image` change
2. Control plane operator extracts new OCP version from release image
2. Control plane operator updates cluster-olm-operator with new image matching new OCP version
3. cluster-olm-operator updates catalogd deployment with new image matching new OCP version
4. cluster-olm-operator updates default ClusterCatalog image references to match new version
5. New catalogd unpacks new catalog images and serves version-appropriate content
6. cluster-olm-operator updates operator-controller deployment with new image matching new OCP version
7. operator-controller discovers updated catalogs and uses new versions for subsequent installations
8. Existing installed operators continue running (not affected by catalog version change)

**Version Alignment:**

- catalogd image version extracted from OCP release payload or determined by version-based naming
- Default catalog images use version-based naming: `registry.redhat.io/redhat/redhat-operator-index:v4.22.3`
- No user intervention required for catalog version upgrades

**Downgrade Strategy:**

Downgrades are not typically supported in the HyperShift model. If a hosted cluster must be downgraded:

1. HyperShift control plane operator detects `HostedCluster.spec.release.image` change to older version
2. Control plane operator updates cluster-olm-operator with new image matching older OCP version
3. cluster-olm-operator updates catalogd deployment with new image matching older OCP version
4. cluster-olm-operator updates default ClusterCatalog image references to match older version
5. cluster-olm-operator updates operator-controller deployment with new image matching older OCP version
6. operator-controller discovers updated catalogs and uses new versions for subsequent installations
7. Risk: Operators installed from newer catalogs may not be compatible with older cluster version
8. Recommendation: Delete incompatible ClusterExtensions before downgrading hosted cluster

## Version Skew Strategy

**Key Feature: Patch-Level Version Alignment**

Each hosted cluster receives a catalogd instance whose version exactly matches the hosted cluster's OCP version, including patch level. This ensures:

- OCP 4.21.1 → catalogd v0.10.0 + catalog image `v4.21.1`
- OCP 4.21.3 → catalogd v0.10.2 + catalog image `v4.21.3`
- OCP 4.22.0 → catalogd v0.11.0 + catalog image `v4.22.0`

**Why Patch-Level Matters:**

Patch versions within the same minor release may have different catalogd controller versions due to:
- Backported security fixes
- Bug patches in catalogd controller
- Updated API schemas
- Performance improvements

A shared catalogd cannot serve different controller versions simultaneously. Per-hosted-cluster deployment ensures perfect alignment.

**Handling Skew During Upgrades:**

During a hosted cluster upgrade from 4.21.1 to 4.21.3:
- Old catalogd (v1.10.0) continues serving while upgrade proceeds
- New catalogd (v1.10.2) deployed in rolling fashion
- operator-controller tolerates brief catalog unavailability (retries)
- Installed operators continue running (unaffected by catalog version)
- New operator installations use new catalog version

**Component Version Skew:**

- catalogd version: Matches hosted cluster OCP version exactly
- operator-controller version: Matches hosted cluster OCP version exactly
- cluster-olm-operator version: Matches hosted cluster OCP version exactly
- All three components deployed in hosted control plane namespace, version-aligned

**No skew between components:** All OLMv1 components for a hosted cluster are version-aligned with that cluster's OCP version.

## Operational Aspects of API Extensions

This enhancement does not introduce new API extensions (CRDs, admission webhooks, conversion webhooks, aggregated API servers, or finalizers). It uses existing ClusterCatalog and ClusterExtension CRDs without modification.

**Impact on Existing SLIs:**

- No impact on API throughput (uses existing ClusterCatalog/ClusterExtension APIs)
- No impact on API availability (no new webhooks or aggregated servers)
- No impact on API latency (no admission control added)

**Expected Use-Cases:**

- Hosted clusters typically create 1-10 custom ClusterCatalog resources
- Hosted clusters typically create 5-50 ClusterExtension resources
- Scale well below limits that would impact API throughput

## Support Procedures

**Detecting catalogd Failure:**

Symptoms:
- ClusterCatalog resources show `Unpacked=False` condition
- operator-controller logs show HTTP connection failures to catalogd
- ClusterExtension installations stuck in `Pending` phase
- Metric: `catalogd_http_request_duration_seconds` shows no recent activity

Commands:
```bash
# Check catalogd pod status (Approach 1)
kubectl -n clusters-customer1 get pods -l app=catalogd

# Check catalogd pod status (Approach 2, requires hosted cluster access)
kubectl -n openshift-catalogd get pods -l app=catalogd

# Check ClusterCatalog status
kubectl get clustercatalogs -o yaml

# Check catalogd logs
kubectl -n clusters-customer1 logs -l app=catalogd
```

**Detecting KUBECONFIG Issues:**

Symptoms:
- catalogd or operator-controller logs show "unable to connect to API server"
- Components fail to start with authentication errors
- ClusterCatalog resources not discovered

Commands:
```bash
# Verify KUBECONFIG secret exists
kubectl -n clusters-customer1 get secret admin-kubeconfig

# Check KUBECONFIG environment variable in pod
kubectl -n clusters-customer1 exec -it <pod> -- env | grep KUBECONFIG

# Verify KUBECONFIG file mounted
kubectl -n clusters-customer1 exec -it <pod> -- ls -la /etc/openshift/kubeconfig/
```

**Disabling OLMv1 in Hosted Cluster:**

To disable OLMv1 components for a specific hosted cluster:

1. Delete operator-controller deployment: `kubectl -n clusters-customer1 delete deployment operator-controller`
2. Delete catalogd deployment: `kubectl -n clusters-customer1 delete deployment catalogd`

Consequences:
- Only affects one hosted cluster (complete isolation)
- Existing installed operators continue running (no impact on running workloads)
- New operator installations fail (ClusterExtension resources stuck in `Pending`)
- catalog queries from console-operator fail (OperatorHub UI shows no catalogs)

**Recovering from catalogd Failure:**

1. Check catalogd pod logs for errors
2. Verify KUBECONFIG secret is valid and mounted correctly
3. Verify catalogd can connect to hosted cluster API server
4. Verify catalog images are pullable
5. Restart catalogd pod: `kubectl -n clusters-customer1 delete pod -l app=catalogd`

OLMv1 fails gracefully:
- operator-controller retries catalog queries with exponential backoff
- ClusterExtension installations queue until catalogd recovers
- No data loss (ClusterCatalog and ClusterExtension resources preserved in hosted cluster API)
- Installed operators continue running (no impact on running workloads)

**cluster-olm-operator Upgrade Blocking:**

To verify cluster-olm-operator is correctly blocking upgrades:

```bash
# Check ClusterOperator status in management cluster
kubectl get clusteroperator cluster-olm-operator -o yaml

# Look for Upgradeable condition
# Upgradeable=False should block hosted cluster upgrades
```

If upgrade blocking is not working:
1. Verify cluster-olm-operator has dual-API access (both management and hosted cluster)
2. Check cluster-olm-operator logs for ClusterOperator status update errors
3. Verify RBAC permissions for ClusterOperator resource updates

## Infrastructure Needed [optional]

No new infrastructure required. Uses existing HyperShift infrastructure:
- admin-kubeconfig secret management
- service-ca for TLS certificates
- Konnectivity (if using Approach 2)
- Standard HyperShift component registration
