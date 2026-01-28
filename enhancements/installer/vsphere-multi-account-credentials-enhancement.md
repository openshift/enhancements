---
title: vsphere-multi-account-credentials-enhancement
authors:
  - "@rvanderp"
reviewers:
  - "@jcpowermac, for vSphere platform expertise, please review privilege definitions and vCenter integration"
  - "@patrickdillon, for installer team review, please review installation workflow changes"
  - "@everettraven, for API review"
approvers:
  - "@jcpowermac"
api-approvers:
  - "@everettraven"
creation-date: 2026-01-28
last-updated: 2026-09-18
status: provisional
tracking-link:
  - "https://issues.redhat.com/browse/SPLAT-2964"
  - "https://github.com/openshift/installer/pull/10895"
see-also:
  - "/enhancements/installer/vsphere-ipi.md"
  - "/enhancements/cloud-integration/cloud-credential-operator.md"
replaces: []
superseded-by: []
---

# vSphere Multi-Account Credential Management

## Summary

This enhancement proposes support for administrator-provisioned, component-specific vCenter credentials for OpenShift on vSphere. Rather than having the cloud-credential-operator (CCO) create vCenter accounts (which would require administrative privileges), this enhancement enables administrators to pre-provision separate vCenter service accounts for each OpenShift component and provide those credentials to OpenShift via `install-config.yaml`. The installer uses `platform.vsphere.credentialType: component-scoped` to emit per-component credential secrets in the cluster, which are consumed by installer-side VM lifecycle operations and distributed to the appropriate OpenShift component operators.

## Motivation

OpenShift on vSphere currently uses a single vCenter account for all operations across all components (installer, machine-api-operator, CSI driver, cloud controller manager, diagnostics). This creates several problems:

1. **Excessive Privilege Exposure:** Every component has access to all privileges, even those it doesn't need.
2. **Large Blast Radius:** A compromised credential in any component grants full cluster and potentially vSphere infrastructure access.
3. **Poor Auditability:** vCenter audit logs cannot distinguish which OpenShift component performed an action.
4. **Compliance Challenges:** Single-account architecture conflicts with SOC2 separation of duties and PCI-DSS least privilege requirements.
5. **Credential Rotation Complexity:** Rotating credentials requires updating all components simultaneously.

Analysis of OpenShift source code across vSphere integration components reveals that each component requires distinct privilege subsets:

| Component | Target Role / Scope | Required Privileges | Current State |
|-----------|---------------------|---------------------|---------------|
| Machine Management | `machineManagement` | ~35 (VM lifecycle & provisioning) | Uses shared credentials |
| Storage | `storage` | ~10-15 (CSI storage) | Uses shared credentials |
| Cloud Controller Manager | `cloudControllerManager` | ~10 (node & topology read-only) | Uses shared credentials |
| Problem Detector | `vsphereProblemDetector` | ~5 (diagnostics read-only) | Uses shared credentials |

### Why Administrator-Provisioned (Not CCO-Minted)

Having CCO automatically create vCenter accounts would require:
- Administrative privileges on vCenter (`Global.Licenses`, `Admin` role)
- Access to vCenter SSO or identity source management
- Elevated trust in OpenShift to manage vCenter accounts

This conflicts with enterprise security practices where:
- Account provisioning is controlled by dedicated infrastructure/security teams
- Service accounts go through approval workflows
- Account creation is audited separately from account usage
- Identity management is centralized (Active Directory, LDAP)

**This enhancement takes the administrator-provisioned approach:** Administrators create the accounts using their existing processes, then provide the credentials to OpenShift via `install-config.yaml`.

### User Stories

* As a **security-conscious cluster administrator**, I want to provide each OpenShift component with separate vCenter credentials that I've pre-provisioned, so that I maintain control over account creation while achieving least-privilege isolation.

* As a **compliance officer**, I want OpenShift to support separation of duties for vCenter access using accounts provisioned through our standard identity management processes, so that our OpenShift deployment meets SOC2 and PCI-DSS requirements.

* As a **vSphere administrator**, I want to create vCenter service accounts with specific privileges for each OpenShift component, following my organization's account provisioning procedures, and then configure OpenShift to use these accounts.

* As a **security team member**, I want documentation of exactly what privileges each OpenShift component requires, so that I can create appropriately-scoped vCenter roles and accounts before cluster deployment.

* As a **day-2 operations engineer**, I want to rotate credentials for individual OpenShift components independently by updating secrets, so that credential rotation follows my organization's rotation policies without affecting other components.

### Goals

1. **Support per-component credential configuration:** Enable administrators to provide distinct vCenter credentials for each component (`machineManagement`, `storage`, `cloudControllerManager`, `vsphereProblemDetector`) in `install-config.yaml`.

2. **Document precise privilege requirements:** Provide authoritative documentation of privileges required by each component, organized by vSphere object scope.

3. **Provide tooling for role/account creation:** Offer scripts and documentation for creating vCenter roles and accounts with correct privileges.

4. **Enable independent credential rotation:** Support updating credentials for one component without affecting others.

5. **Maintain backward compatibility:** Continue supporting single shared credential (`credentialType: global`) for existing or simpler deployments.

### Non-Goals

1. **Automatic vCenter account creation:** CCO will NOT create vCenter accounts; this requires admin privileges the operator should not have.

2. **Direct identity source integration:** CCO will not integrate with AD/LDAP to create accounts.

3. **Privilege escalation or modification:** CCO will not modify roles or add privileges to existing accounts.

4. **External secret manager integration:** Direct integration with HashiCorp Vault, CyberArk, etc. is out of scope.

5. **Runtime privilege discovery:** Automatic detection of what privileges a component actually uses is not in scope.

## Proposal

### Overview

This enhancement extends OpenShift to support administrator-provisioned, per-component vCenter credentials with multi-vCenter support:

1. **API Schema Extension:** `platform.vsphere.credentialType` controls whether credentials are `global` or `component-scoped`.
2. **Component Credentials Structure:** Under `platform.vsphere.vcenters[]`, `componentCredentials` contains `machineManagement`, `storage`, `cloudControllerManager`, and `vsphereProblemDetector` credentials.
3. **Installer & CAPI Integration:** Installer-side VM operations (such as OVA validation, template cloning, and CAPI cluster asset generation) use the `machineManagement` credential.
4. **Manual Credentials Mode Secrets:** In `credentialsMode: Manual`, the installer generates component-specific Secrets in target namespaces (`openshift-machine-api`, `openshift-cluster-csi-drivers`, `openshift-cloud-controller-manager`, `openshift-cluster-storage-operator`).
5. **Multi-vCenter Support:** Each component secret contains `<vcenter-server>.username` and `<vcenter-server>.password` key-value pairs for every configured vCenter.

### Workflow Description

1. **vSphere Administrator** creates roles and service accounts in vCenter corresponding to each component (`machineManagement`, `storage`, `cloudControllerManager`, `vsphereProblemDetector`).
2. **Cluster Administrator** creates an `install-config.yaml` specifying `platform.vsphere.credentialType: component-scoped` and populating `vcenters[].componentCredentials` for each vCenter server.
3. **OpenShift Installer** parses `install-config.yaml`, validates that top-level `user`/`password` fields are omitted and that all four component credentials are non-empty strings.
4. **Installer Pre-flight & Pre-installation** uses the `machineManagement` credential for all vCenter client interactions (e.g., verifying datacenters, datastores, networks, and cloning templates).
5. **Manifest Generation** creates Secrets in target namespaces for `credentialsMode: Manual` or generates CAPI identity secrets with `machineManagement` credentials.
6. **Day-2 Operation & Rotation** allows updating individual target Secrets in their respective namespaces independently.

### API Extensions

#### Go Struct Definitions (`pkg/types/vsphere/platform.go`)

```go
// CredentialType determines how vCenter credentials are supplied.
// +kubebuilder:validation:Enum=global;component-scoped
type CredentialType string

const (
    // CredentialTypeGlobal uses top-level vCenter credentials on each VCenter.
    CredentialTypeGlobal CredentialType = "global"
    // CredentialTypeComponentScoped uses credentials scoped to each vSphere component.
    CredentialTypeComponentScoped CredentialType = "component-scoped"
)

// Credential contains a vCenter username and password.
type Credential struct {
    // User is the username used to authenticate to vCenter.
    User string `json:"user"`
    // Password is the password used to authenticate to vCenter.
    Password string `json:"password"`
}

// ComponentCredentials contains credentials for vSphere components.
type ComponentCredentials struct {
    // MachineManagement is used by the Machine API, Cluster API, and installer-side
    // operations to provision, configure, and remove virtual machines in vCenter.
    MachineManagement Credential `json:"machineManagement"`

    // Storage is used by the vSphere CSI driver to provision and manage storage
    // volumes and related vSphere storage resources.
    Storage Credential `json:"storage"`

    // CloudControllerManager is used by the vSphere Cloud Controller Manager to
    // manage cloud-provider integration, including node and load-balancer state.
    CloudControllerManager Credential `json:"cloudControllerManager"`

    // VSphereProblemDetector is used by the vSphere Problem Detector to inspect
    // the vSphere environment and report configuration and permission problems.
    VSphereProblemDetector Credential `json:"vsphereProblemDetector"`
}

type Platform struct {
    // CredentialType determines whether credentials are global or component-scoped.
    // When omitted, global credentials are used for backward compatibility.
    CredentialType CredentialType `json:"credentialType,omitempty"`

    // VCenters defines the vCenter instances.
    VCenters []VCenter `json:"vcenters,omitempty"`

    // ...
}

type VCenter struct {
    Server string `json:"server"`

    // User is the username to use when connecting to vCenter (used when credentialType is global).
    User string `json:"user,omitempty"`

    // Password is the password for the user (used when credentialType is global).
    Password string `json:"password,omitempty"`

    Datacenters []string `json:"datacenters"`

    // ComponentCredentials contains credentials for each vSphere component when
    // the platform uses component-scoped credentials.
    // +optional
    ComponentCredentials *ComponentCredentials `json:"componentCredentials,omitempty"`
}
```

### Topology Considerations

#### Hypershift / Hosted Control Planes

vSphere is not supported on HyperShift / Hosted Control Planes. Therefore, this enhancement does not apply to HyperShift.

#### Standalone Clusters

Standalone IPI and UPI clusters on vSphere support `credentialType: component-scoped` directly via `install-config.yaml`.

#### Single-node Deployments or MicroShift

For Single-Node OpenShift (SNO), per-component credentials reduce the privilege scope without affecting CPU/memory consumption. MicroShift does not use CCO or installer-generated component credential secrets.

#### OpenShift Kubernetes Engine

Compatible with OKE as it relies on standard vSphere integration components present across all OpenShift offerings.

### Implementation Details/Notes/Constraints

#### Validation and Mutual Exclusivity Rules

The installer validates `platform.vsphere` according to strict mutual exclusivity rules:

1. **`credentialType: component-scoped`**:
   - `user` and `password` on every `vcenters[]` item **must be omitted**. Setting either results in a validation error (`must not be set when credentialType is component-scoped`).
   - `componentCredentials` on every `vcenters[]` item **is required**.
   - Inside `componentCredentials`, all four component fields (`machineManagement`, `storage`, `cloudControllerManager`, `vsphereProblemDetector`) are required, and both `user` and `password` must be non-empty strings.

2. **`credentialType: global` (or omitted)**:
   - `user` and `password` on every `vcenters[]` item **are required**.
   - `componentCredentials` on every `vcenters[]` item **must not be set**. Setting `componentCredentials` results in a validation error (`must not be set when credentialType is global`).

#### Installer-Side Credential Selection

The installer resolves client credentials using `Platform.CredentialsForVCenter(server)`:

```go
func (p *Platform) CredentialsForVCenter(server string) (Credential, error) {
    for i := range p.VCenters {
        vcenter := &p.VCenters[i]
        if vcenter.Server != server {
            continue
        }

        if p.CredentialType == CredentialTypeComponentScoped {
            if vcenter.ComponentCredentials == nil {
                return Credential{}, fmt.Errorf("vcenter %s is missing componentCredentials", server)
            }
            credential := vcenter.ComponentCredentials.MachineManagement
            if credential.User == "" || credential.Password == "" {
                return Credential{}, fmt.Errorf("vcenter %s is missing machineManagement credentials", server)
            }
            return credential, nil
        }

        return Credential{User: vcenter.User, Password: vcenter.Password}, nil
    }
    return Credential{}, fmt.Errorf("vcenter %s not found", server)
}
```

#### Generated Manifests for `credentialsMode: Manual`

When `credentialsMode: Manual` and `credentialType: component-scoped`, the installer generates component-specific Secrets:

| Manifest Filename | Secret Name | Target Namespace | Component Credential Source |
|---|---|---|---|
| `99_vsphere-machine-api-credentials.yaml` | `vsphere-cloud-credentials` | `openshift-machine-api` | `machineManagement` |
| `99_vsphere-csi-credentials.yaml` | `vmware-vsphere-cloud-credentials` | `openshift-cluster-csi-drivers` | `storage` |
| `99_vsphere-cloud-controller-credentials.yaml` | `vsphere-cloud-credentials` | `openshift-cloud-controller-manager` | `cloudControllerManager` |
| `99_vsphere-problem-detector-credentials.yaml` | `vsphere-cloud-credentials` | `openshift-cluster-storage-operator` | `vsphereProblemDetector` |

### Risks and Mitigations

| Risk | Mitigation |
|------|------------|
| Administrator provides insufficient privileges | Document precise privilege requirements per component; provide role creation scripts |
| Complex setup burden on administrators | Provide scripts (`govc`, PowerCLI) and detailed documentation |
| Misconfiguration of component credentials | Enforce strict schema validation on `install-config.yaml` at install time |
| Exposing secrets in `install-config.yaml` | Installer redacts component credentials from rendered install-config; enforce restricted file permissions |

### Drawbacks

1. **Increased Setup Complexity:** Administrators must create multiple accounts/roles in vCenter.
2. **Documentation Overhead:** Must maintain per-component privilege lists as vSphere features evolve.

## Alternatives (Not Implemented)

1. **CCO Account Minting Mode:** Having CCO mint vCenter accounts automatically was rejected because it requires full vCenter administrative privileges that security-conscious enterprise teams do not grant to Kubernetes workloads.
2. **Sidecar / File-based Credential File:** A separate `~/.vsphere/credentials` INI file was considered, but embedding the configuration within `install-config.yaml` under `platform.vsphere.credentialType` and `componentCredentials` keeps installation configuration declarative and consistent with other cloud platforms.

## Test Plan

- **Unit Tests:** Validate schema enums, mutual exclusivity rules, `Platform.CredentialsForVCenter` credential selection, manual secret generation, and password redaction.
- **Integration Tests:** Verify manifest generation for both global and component-scoped credentials in manual and automatic modes.
- **E2E Tests:** Execute full installation and day-2 cluster operations with `platform.vsphere.credentialType: component-scoped`.

## Graduation Criteria

### Dev Preview -> Tech Preview

- Implementation of `platform.vsphere.credentialType` and `componentCredentials` in `install-config.yaml`.
- Support for `machineManagement` credentials in installer and CAPI cluster asset generation.
- Support for generating component-scoped manual secrets in `openshift.go`.

### Tech Preview -> GA

- CI coverage for component-scoped vSphere installations.
- Official documentation in `openshift-docs` covering vCenter role and service account setup.

### Removing a deprecated feature

- No feature deprecation is involved; `credentialType: global` remains supported.

## Upgrade / Downgrade Strategy

- **Upgrade:** Existing clusters using global credentials continue functioning with `credentialType: global`. Migrating to component-scoped credentials involves updating target component Secrets.
- **Downgrade:** Component-scoped API extensions are additive and backward-compatible.

## Version Skew Strategy

Older clients ignore `credentialType` and default to global credential handling. Component operators accept updated secrets independently during rolling updates.

## Operational Aspects of API Extensions

- **SLI Impact:** No performance or API throughput impact.
- **Failure Modes:** If component credentials have invalid permissions, the corresponding component operator logs authentication or permission errors.

## Support Procedures

- **Diagnosis:** Inspect target Secrets in `openshift-machine-api`, `openshift-cluster-csi-drivers`, `openshift-cloud-controller-manager`, and `openshift-cluster-storage-operator`.
- **Remediation:** Update the relevant Secret with valid credentials for the failing component.
