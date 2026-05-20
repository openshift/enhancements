# Design Document: vSphere Multi-Account Credential Management

**Document ID:** DD-SPLAT-2724  
**Version:** 1.0  
**Status:** Draft  
**Epic:** SPLAT-2724  
**Parent Feature:** OCPSTRAT-2933  
**Classification:** Internal — Engineering

---

## Table of Contents

1. [Architecture Overview](#1-architecture-overview)
2. [Component Diagram Descriptions](#2-component-diagram-descriptions)
3. [Data Models and Schemas](#3-data-models-and-schemas)
4. [API Contracts and Interfaces](#4-api-contracts-and-interfaces)
5. [Integration Points](#5-integration-points)
6. [Security Considerations](#6-security-considerations)
7. [Trade-offs and Alternatives Considered](#7-trade-offs-and-alternatives-considered)
8. [Implementation Phasing](#8-implementation-phasing)
9. [Open Questions and Blocking Dependencies](#9-open-questions-and-blocking-dependencies)
10. [Appendix](#10-appendix)

---

## 1. Architecture Overview

### 1.1 Problem Statement

The current OpenShift vSphere IPI installer uses a single vCenter credential set for both infrastructure provisioning (cluster creation) and all ongoing Day-2 operational activities (machine scaling, storage management, cloud controller operations, diagnostics). This creates an unacceptably large security blast radius: a compromised in-cluster credential grants an attacker the same privileges used to create — and therefore destroy — the entire vSphere infrastructure.

### 1.2 Solution Summary

This design introduces a **two-phase credential model** for vSphere IPI installations:

- **Phase 1 — Provisioning:** A high-privilege vCenter account is used exclusively by the OpenShift Installer (`openshift-install`) to create cluster infrastructure (VMs, folders, resource pools, networks, datastores). This credential is ephemeral with respect to the cluster: it is never written to any cluster-permanent configuration when component-specific operational credentials are also supplied.

- **Phase 2 — Operations (Day-2):** Up to four component-specific restricted vCenter accounts are used by in-cluster operators after the cluster is bootstrapped. Each account holds only the minimum vCenter permissions required by its corresponding operator.

The two phases are joined by an **atomic credential handoff**: the installer writes the per-component operational credentials to the `vsphere-cloud-credentials` Kubernetes Secret in `kube-system` before control transfers to the in-cluster operators. The provisioning credential is never written to this secret in multi-account mode.

### 1.3 High-Level Flow

```
User provides install-config.yaml
        │
        ▼
┌─────────────────────────────────┐
│  Installer: Schema Validation   │
│  (FR-01: Extended Schema)       │
└─────────────┬───────────────────┘
              │
              ▼
┌─────────────────────────────────┐
│  Installer: Credential          │
│  Permission Validation (FR-03)  │
│  - Validate provisioning creds  │
│  - Validate operational creds   │
└─────────────┬───────────────────┘
              │ [All validations pass]
              ▼
┌─────────────────────────────────┐
│  Installer: Infrastructure      │
│  Provisioning Phase             │
│  (Uses provisioning credentials)│
└─────────────┬───────────────────┘
              │
              ▼
┌─────────────────────────────────────────────┐
│  Installer: Atomic Credential Handoff (FR-04)│
│  - Write per-component operational creds     │
│    to vsphere-cloud-credentials secret       │
│  - Provisioning creds NOT written (FR-05)    │
└─────────────┬───────────────────────────────┘
              │
              ▼
┌─────────────────────────────────┐
│  Cluster Bootstrap Complete     │
│  In-cluster operators consume   │
│  vsphere-cloud-credentials      │
│  (operational credentials only) │
└─────────────────────────────────┘
```

### 1.4 Backward Compatibility Guarantee

All changes are strictly additive. If the user provides no component-specific credentials, the installer behaves identically to today: provisioning credentials are written to `vsphere-cloud-credentials` using the existing format. The new code paths are activated only when the extended credential fields are populated.

### 1.5 Scope Boundary

| In Scope | Out of Scope |
|----------|-------------|
| New IPI installations (greenfield) | Brownfield migration for existing clusters (OCPSTRAT-2933 separate) |
| vSphere platform only | AWS, Azure, GCP, bare metal |
| Installer and in-cluster credential bootstrap | Assisted Installer / Console UI (separate tracking) |
| Single vCenter instance | Cross-vCenter management |
| Credential validation (pre-install) | vCenter account/role creation |
| Credential lifecycle during install | Periodic credential rotation |

---

## 2. Component Diagram Descriptions

### 2.1 System Context Diagram

```
┌─────────────────────────────────────────────────────────────────────┐
│                        Administrator                                 │
│  (prepares install-config.yaml with provisioning + component creds) │
└────────────────────────────┬────────────────────────────────────────┘
                             │
                             ▼
┌─────────────────────────────────────────────────────────────────────┐
│                    openshift-install binary                          │
│                                                                      │
│  ┌──────────────────┐  ┌──────────────────┐  ┌──────────────────┐  │
│  │  Schema          │  │  Credential      │  │  Infrastructure  │  │
│  │  Validation      │→ │  Validation      │→ │  Provisioning    │  │
│  │  Module          │  │  Module (FR-03)  │  │  (Terraform/     │  │
│  │  (FR-01)         │  │                  │  │   vSphere SDK)   │  │
│  └──────────────────┘  └──────────────────┘  └────────┬─────────┘  │
│                                                         │            │
│  ┌──────────────────────────────────────────────────────▼─────────┐ │
│  │           Secret Population Module (FR-02, FR-04, FR-05)       │ │
│  │   Writes per-component credentials to vsphere-cloud-credentials │ │
│  └────────────────────────────────────────────────────────────────┘ │
└───────────────────────────────────────────┬─────────────────────────┘
                                            │
                    ┌───────────────────────┼────────────────────────┐
                    ▼                       ▼                         ▼
         ┌─────────────────┐    ┌─────────────────────┐   ┌──────────────────────┐
         │  VMware vCenter │    │  OpenShift Cluster  │   │  Kubernetes API      │
         │                 │    │  (kube-system ns)   │   │  Server              │
         │  Provisioning   │    │                     │   │                      │
         │  user account   │    │  vsphere-cloud-     │   │  RBAC enforcement    │
         │                 │    │  credentials Secret  │   │  on kube-system      │
         │  machine-api    │    │                     │   │  secrets             │
         │  user account   │    │  ┌───────────────┐  │   └──────────────────────┘
         │                 │    │  │machine-api    │  │
         │  storage user   │    │  │storage        │  │
         │  account        │    │  │diagnostics    │  │
         │                 │    │  │cloud-controller│ │
         │  diagnostics    │    │  └───────────────┘  │
         │  user account   │    │                     │
         │                 │    └─────────────────────┘
         │  cloud-         │
         │  controller     │
         │  user account   │
         └─────────────────┘
```

### 2.2 Installer Internal Component Decomposition

```
openshift-install (vSphere IPI)
│
├── pkg/types/vsphere/
│   └── types.go                    [Extended: new credential types]
│
├── pkg/types/vsphere/validation/
│   └── platform.go                 [Extended: schema validation FR-01]
│
├── pkg/asset/installconfig/
│   └── vsphere/
│       └── credentials.go          [New: credential parsing + dispatch]
│
├── pkg/vsphere/
│   └── validation/
│       └── credential_validator.go [New: permission validation module FR-03]
│       └── permissions_matrix.go   [New: per-component permission definitions]
│
├── pkg/asset/machines/vsphere/
│   └── secret.go                   [Extended: secret population logic FR-02]
│
└── pkg/infrastructure/vsphere/
    └── clusterapi/
        └── handoff.go              [New: atomic credential handoff FR-04]
```

### 2.3 In-Cluster Operator Integration

```
kube-system/vsphere-cloud-credentials (Kubernetes Secret)
│
├── [Key] machine-api-credentials       → consumed by: machine-api operator
├── [Key] storage-credentials           → consumed by: vSphere CSI driver / storage operator
├── [Key] diagnostics-credentials       → consumed by: diagnostics component
└── [Key] cloud-controller-credentials  → consumed by: cloud-controller-manager

Note: Key naming convention is TBD (see OQ-04). Names above are illustrative.

Legacy (single-credential) mode:
└── [Key] <existing-key-name>           → consumed by: all operators (unchanged)
```

### 2.4 Credential Validation Module Decomposition

```
CredentialValidationModule
│
├── ValidationOrchestrator
│   ├── Accepts: []CredentialValidationRequest
│   ├── Executes: parallel goroutines (one per credential)
│   ├── Enforces: 60-second total timeout (NFR-P-1)
│   └── Returns: []ValidationResult
│
├── ProvisioningCredentialValidator
│   ├── Connects to vCenter using provisioning creds
│   ├── Checks: VM, Folder, ResourcePool, Network, Datastore permissions
│   └── Returns: ValidationResult{Pass|Fail, []MissingPermission}
│
├── OperationalCredentialValidator (per-component)
│   ├── Instantiated once per component (machine-api, storage, diagnostics, CCM)
│   ├── Loads: ComponentPermissionSpec from permissions_matrix.go
│   ├── Checks: Minimum required permissions for component
│   └── Returns: ValidationResult{Pass|Fail, []MissingPermission}
│
└── ErrorFormatter
    ├── Accepts: ValidationResult
    └── Returns: Human-readable, actionable error string with remediation hint
```

---

## 3. Data Models and Schemas

### 3.1 Extended `install-config.yaml` Schema (vSphere Platform Section)

The existing `platform.vsphere` section is extended with a new optional `componentCredentials` block. All new fields are optional to preserve backward compatibility (NFR-B-1).

**Conceptual YAML representation:**

```yaml
platform:
  vsphere:
    # Existing fields — UNCHANGED
    vcenter: vcenter.example.com
    username: provisioning-user@vsphere.local   # Existing field: provisioning creds
    password: "<provisioning-password>"          # Existing field: provisioning creds
    datacenter: DC1
    defaultDatastore: datastore1
    folder: /DC1/vm/openshift-cluster
    resourcePool: /DC1/host/cluster/Resources/openshift
    network: VM Network
    diskType: thin

    # NEW OPTIONAL BLOCK — Multi-account credential support
    # When absent, installer behaves identically to current behavior.
    # When present, component credentials replace provisioning creds
    # in vsphere-cloud-credentials secret.
    componentCredentials:
      machineAPI:
        username: machine-api-user@vsphere.local
        password: "<machine-api-password>"
      storage:
        username: storage-user@vsphere.local
        password: "<storage-password>"
      diagnostics:
        username: diagnostics-user@vsphere.local
        password: "<diagnostics-password>"
      cloudController:
        username: cloud-controller-user@vsphere.local
        password: "<cloud-controller-password>"
```

**Design decisions embedded in this schema:**

1. The existing `username`/`password` fields at the `vsphere` level are reused for the provisioning credential. No new top-level provisioning field is introduced, preserving backward compatibility without field duplication.
2. The `componentCredentials` block is optional as a whole. Its sub-fields (per-component entries) follow a partial-configuration policy to be resolved via OQ-03.
3. All password fields are treated as sensitive; they must be redacted in any logging output (NFR-S-3).

### 3.2 Go Type Definitions (Installer)

```go
// File: pkg/types/vsphere/types.go

// Platform stores all the global configuration information for a vSphere platform.
// Existing type — shown with extension only.
type Platform struct {
    // ... existing fields unchanged ...

    // ComponentCredentials defines per-component vCenter credentials for Day-2
    // operations. This field is optional. When omitted, the top-level Username
    // and Password fields are used for all cluster operations (legacy behavior).
    // When provided, the top-level credentials are used only during provisioning
    // and are NOT persisted in the cluster.
    // +optional
    ComponentCredentials *ComponentCredentials `json:"componentCredentials,omitempty"`
}

// ComponentCredentials holds optional per-component vCenter credential sets
// for Day-2 operational use. All component fields are individually optional
// pending resolution of the partial-configuration policy (see OQ-03).
type ComponentCredentials struct {
    // MachineAPI holds credentials for the machine-api operator.
    // +optional
    MachineAPI *VSphereCredential `json:"machineAPI,omitempty"`

    // Storage holds credentials for the storage operator (vSphere CSI driver).
    // +optional
    Storage *VSphereCredential `json:"storage,omitempty"`

    // Diagnostics holds credentials for the diagnostics component.
    // +optional
    Diagnostics *VSphereCredential `json:"diagnostics,omitempty"`

    // CloudController holds credentials for the cloud-controller-manager.
    // +optional
    CloudController *VSphereCredential `json:"cloudController,omitempty"`
}

// VSphereCredential holds a vCenter username/password pair.
type VSphereCredential struct {
    // Username is the vCenter username for this credential.
    Username string `json:"username"`

    // Password is the vCenter password for this credential.
    // This value is treated as sensitive and must never be logged.
    Password string `json:"password"`
}
```

### 3.3 `vsphere-cloud-credentials` Secret Schema

#### 3.3.1 Multi-Account Mode (componentCredentials provided)

```
Secret: vsphere-cloud-credentials
Namespace: kube-system

Data:
  <machine-api-key>:      <base64(ini-formatted-credential)>
  <storage-key>:          <base64(ini-formatted-credential)>
  <diagnostics-key>:      <base64(ini-formatted-credential)>
  <cloud-controller-key>: <base64(ini-formatted-credential)>

Note: Exact key names to be resolved per OQ-04.
Provisioning credentials are NOT present.
```

#### 3.3.2 Legacy Mode (no componentCredentials)

```
Secret: vsphere-cloud-credentials
Namespace: kube-system

Data:
  <existing-key>: <base64(ini-formatted-credential)>

Structure is byte-for-byte identical to current implementation.
```

#### 3.3.3 Credential File Format (per entry)

Each credential entry within the secret follows the INI-style format already consumed by vSphere operators (exact format to be confirmed against current operator implementations):

```ini
[Global]
user = <username>
password = <password>
server = <vcenter-host>
port = 443
insecure-flag = false
```

> **Note:** The exact format must be confirmed with each operator team during the resolution of OQ-02. The format shown is representative of the existing `vsphere-cloud-credentials` structure.

### 3.4 Permissions Matrix Data Model

The minimum permission set per component is represented as a versioned data structure maintained alongside the codebase (NFR-M-1):

```go
// File: pkg/vsphere/validation/permissions_matrix.go

// ComponentRole defines the vCenter permissions required by a specific
// in-cluster component for Day-2 operations.
type ComponentRole struct {
    // ComponentName is the human-readable component identifier.
    ComponentName string

    // RequiredPrivileges is the list of vCenter privilege IDs required.
    // These must map to vCenter's privilege namespace (e.g., "VirtualMachine.Interact.PowerOn").
    RequiredPrivileges []string

    // Scope defines the vCenter object types and hierarchy levels
    // at which these privileges must be granted.
    Scope []PrivilegeScope
}

// PrivilegeScope defines where in the vCenter hierarchy a privilege
// must be granted (e.g., datacenter-level, folder-level).
type PrivilegeScope struct {
    ObjectType  string // e.g., "Folder", "Datastore", "ResourcePool"
    Propagation bool   // whether the privilege must propagate to children
}

// PermissionsMatrix is the authoritative, versioned source of truth
// for minimum required permissions per component.
// Version must be updated whenever permission requirements change.
var PermissionsMatrix = struct {
    Version    string
    Components map[string]ComponentRole
}{
    Version: "1.0.0",
    Components: map[string]ComponentRole{
        "machine-api":       { /* TBD — pending OQ-01 resolution */ },
        "storage":           { /* TBD — pending OQ-01 resolution */ },
        "diagnostics":       { /* TBD — pending OQ-01 resolution */ },
        "cloud-controller":  { /* TBD — pending OQ-01 resolution */ },
    },
}
```

> **Blocking Dependency:** The actual privilege lists for each component (OQ-01) must be defined and agreed upon by security stakeholders before this matrix can be populated and FR-03 validation logic can be implemented.

---

## 4. API Contracts and Interfaces

### 4.1 Credential Validation Module Interface

```go
// File: pkg/vsphere/validation/credential_validator.go

// CredentialValidator defines the interface for vCenter credential
// permission validation. Implemented as a discrete, independently
// testable module (NFR-M-2, FR-03 AC-03.5).
type CredentialValidator interface {
    // ValidateProvisioningCredential checks that the given credential
    // holds sufficient permissions for IPI cluster creation.
    // Must not create, modify, or delete any vCenter resource (FR-03 AC-03.4).
    // Returns nil on success; CredentialValidationError on failure.
    ValidateProvisioningCredential(ctx context.Context, cred VSphereCredential, vcenterHost string) error

    // ValidateOperationalCredential checks that the given credential
    // holds the minimum permissions required for the named component.
    // Must not create, modify, or delete any vCenter resource.
    // Returns nil on success; CredentialValidationError on failure.
    ValidateOperationalCredential(ctx context.Context, component string, cred VSphereCredential, vcenterHost string) error
}

// CredentialValidationError provides structured error information
// for a credential validation failure.
type CredentialValidationError struct {
    // CredentialType identifies which credential failed ("provisioning", "machine-api", etc.)
    CredentialType string

    // MissingPrivileges lists the specific vCenter privilege IDs that are absent.
    MissingPrivileges []string

    // AffectedObjects lists the vCenter objects on which the privilege check failed.
    AffectedObjects []string

    // Remediation provides a human-readable, actionable resolution suggestion.
    Remediation string
}

func (e *CredentialValidationError) Error() string {
    // Returns: "[CREDENTIAL ERROR] <CredentialType>: missing privileges [<list>]
    //           on objects [<list>]. Remediation: <Remediation>"
}
```

### 4.2 Secret Population Interface

```go
// File: pkg/asset/machines/vsphere/secret.go

// SecretPopulator defines the logic for building the vsphere-cloud-credentials
// Kubernetes Secret content. Behavior depends on whether component credentials
// are provided.
type SecretPopulator interface {
    // BuildSecret constructs the vsphere-cloud-credentials secret.
    // If componentCreds is non-nil, multi-account mode is used.
    // If componentCreds is nil, legacy single-credential mode is used.
    BuildSecret(
        provisioningCred VSphereCredential,
        componentCreds   *ComponentCredentials,
        vcenterHost      string,
    ) (*corev1.Secret, error)
}

// SecretMode describes which population mode is active.
type SecretMode int

const (
    SecretModeLegacy     SecretMode = iota // Single provisioning credential
    SecretModeMultiAccount                 // Per-component operational credentials
)
```

### 4.3 Credential Handoff Interface

```go
// File: pkg/infrastructure/vsphere/clusterapi/handoff.go

// CredentialHandoff orchestrates the atomic transition from provisioning
// to operational credentials (FR-04).
type CredentialHandoff interface {
    // Execute performs the atomic write of the vsphere-cloud-credentials
    // secret to the target cluster. The operation is designed to be
    // idempotent: a retry after partial failure must not leave the
    // cluster in an inconsistent state.
    //
    // On success: operational credentials are active; provisioning
    //             credentials are not present in any cluster secret.
    // On failure: returns error; cluster state is not partially mutated.
    Execute(ctx context.Context, secret *corev1.Secret, client kubernetes.Interface) error

    // Verify confirms post-handoff that:
    // (a) all component credential keys are present in the secret
    // (b) provisioning credentials are absent from the secret
    Verify(ctx context.Context, client kubernetes.Interface) error
}
```

### 4.4 Schema Validation Extension

The existing `ValidatePlatform` function in `pkg/types/vsphere/validation/platform.go` is extended:

```go
// ValidateComponentCredentials validates the optional ComponentCredentials block.
// Called from ValidatePlatform when ComponentCredentials is non-nil.
//
// Validates:
//   - No individual VSphereCredential has empty Username or Password
//   - Partial configuration policy is enforced per OQ-03 resolution
//     (either all-or-nothing, or defined fallback behavior per component)
//   - Returns field.ErrorList for integration with existing validation framework
func ValidateComponentCredentials(
    creds *ComponentCredentials,
    fldPath *field.Path,
) field.ErrorList
```

### 4.5 Installer CLI Interface (User-Facing)

No new CLI flags are required. All configuration is driven through `install-config.yaml`. The installer log output is extended per NFR-A-3:

```
[INFO]  Using multi-account credential mode for vSphere platform
[INFO]  Validating provisioning credential permissions...
[INFO]  Validating machine-api operational credential permissions...
[INFO]  Validating storage operational credential permissions...
[INFO]  Validating diagnostics operational credential permissions...
[INFO]  Validating cloud-controller operational credential permissions...
[INFO]  All credential validations passed
[INFO]  Infrastructure provisioning using provisioning credential (user: <username-only, no password>)
[INFO]  Writing per-component operational credentials to vsphere-cloud-credentials
[INFO]  Credential handoff complete; provisioning credentials not persisted in cluster
[WARN]  (Legacy mode only) Operating in single-credential mode. 
        Security recommendation: configure component-specific operational credentials 
        to reduce privilege exposure in the running cluster.
```

---

## 5. Integration Points

### 5.1 OpenShift Installer Schema Framework

**Repository:** `openshift/installer`  
**Integration type:** Schema extension (additive)  
**Change surface:**
- `pkg/types/vsphere/types.go` — new types
- `pkg/types/vsphere/validation/platform.go` — new validation rules
- `data/data/install.openshift.io/v1/InstallConfig.json` — JSON Schema extension
- `pkg/asset/installconfig/vsphere/` — credential parsing and dispatch logic

**Coordination required:** Installer team review for JSON Schema changes and validation framework integration.

### 5.2 vCenter API Integration (Credential Validation)

**Protocol:** vSphere REST API or govmomi (Go vSphere client library)  
**Operations used (read-only):**
- Session authentication (login)
- `AuthorizationManager.HasPrivilegeOnEntities` — checks whether the authenticated session holds specified privileges on specified managed object references
- Session termination (logout)

**Constraints:**
- No write operations of any kind (NFR-P-3, FR-03 AC-03.4)
- Must complete within 60-second total budget across all credential checks (NFR-P-1)
- Parallel execution across credential checks (NFR-P-2)
- Retry on transient 503/timeout errors with exponential backoff (NFR-R-4)

**Connection management:** Each credential validation opens a separate vCenter session using the credential under test, executes the privilege check, and closes the session. Sessions must not be reused between different credential sets.

### 5.3 machine-api Operator

**Repository:** `openshift/machine-api-operator` (or provider-specific)  
**Integration type:** Secret consumption  
**Change required (pending OQ-02):** Operator must read credentials from the per-component key within `vsphere-cloud-credentials` rather than (or in addition to) the legacy key.  
**Credential key:** TBD (OQ-04)  
**Impact if changes required:** Must be tracked as an explicit child issue of SPLAT-2724 per ASM-03.

### 5.4 cloud-controller-manager (CCM)

**Repository:** `openshift/cloud-provider-vsphere`  
**Integration type:** Secret consumption  
**Change required (pending OQ-02):** CCM must read the cloud-controller-specific credential key from `vsphere-cloud-credentials`.  
**Credential key:** TBD (OQ-04)  
**Impact if changes required:** Must be tracked as child issue.

### 5.5 vSphere CSI Driver / Storage Operator

**Repository:** `openshift/vmware-vsphere-csi-driver-operator`  
**Integration type:** Secret consumption  
**Change required (pending OQ-02):** Storage operator must read the storage-specific credential key from `vsphere-cloud-credentials`.  
**Credential key:** TBD (OQ-04)  
**Impact if changes required:** Must be tracked as child issue.

### 5.6 Diagnostics Component

**Repository:** TBD (component team to confirm)  
**Integration type:** Secret consumption  
**Change required (pending OQ-02):** Diagnostics component must read its specific credential key from `vsphere-cloud-credentials`.  
**Credential key:** TBD (OQ-04)  
**Impact if changes required:** Must be tracked as child issue.

### 5.7 Kubernetes Secret Management (kube-system)

**Integration type:** Kubernetes API — Secret create/update  
**Namespace:** `kube-system`  
**Secret name:** `vsphere-cloud-credentials` (existing; not renamed)  
**RBAC impact:** Existing RBAC bindings control which service accounts can read `vsphere-cloud-credentials`. Review required to ensure per-component consumers have read access to the secret but not cross-component access. This may require Secret restructuring or label-based access if strict per-component isolation is required (see Trade-offs, §7.3).

### 5.8 CI/CD Pipeline Integration

**Relevant closed issues:** SPLAT-2729 (pre-merge), SPLAT-2730 (e2e), SPLAT-2731 (CI implementation)  
**New test suites required:**
- Unit tests: schema validation, secret population, credential handoff logic (no vSphere required)
- Integration tests: validation module with mock vCenter (govmomi simulator)
- End-to-end tests: full IPI installation with multi-account credentials against live vSphere
- Failure scenario tests: atomic transition failure injection (FR-04 AC-04.5, AC-04.6)

---

## 6. Security Considerations

### 6.1 Credential Handling in Memory

All credential values (usernames and passwords) must be handled exclusively in memory during the install process. Specifically:

- Passwords extracted from `install-config.yaml` must never be written to any intermediate file, log output, error message, or standard output stream (NFR-S-3, FR-05 AC-05.3).
- The `install-config.yaml` file itself contains plaintext credentials at time of writing; this is an existing concern, not introduced by this feature. However, the feature must not create additional persistence vectors.
- In Go, `VSphereCredential.Password` fields should be treated as sensitive values. Consider implementing `fmt.Stringer` for `VSphereCredential` that redacts the password field to prevent accidental log exposure.

```go
// Prevents accidental logging of password in fmt.Printf / log statements
func (c VSphereCredential) String() string {
    return fmt.Sprintf("VSphereCredential{Username: %q, Password: [REDACTED]}", c.Username)
}
```

### 6.2 Provisioning Credential Non-Persistence

The most critical security invariant: in multi-account mode, provisioning credentials must never appear in `vsphere-cloud-credentials` or any other cluster-permanent store (FR-05, NFR-S-1).

**Implementation enforcement mechanism:**
- The `BuildSecret` function's multi-account code path must be structurally unable to receive the provisioning credential as input (enforced through type system separation rather than runtime checks where possible).
- Post-handoff verification (`CredentialHandoff.Verify`) actively checks for the absence of provisioning credentials by comparing the provisioning username against all values in the written secret.
- Integration test AC-05.2 and AC-05.1 validate this invariant against a live cluster.

### 6.3 Secret Access Control

The `vsphere-cloud-credentials` secret in `kube-system` is currently a single secret readable by multiple operators. In multi-account mode, all component credentials are co-located within this single secret. This means any operator that can read the secret can technically read all component credentials, not just its own.

**Current approach (accepted limitation):** This design maintains the existing single-secret structure because:
1. All consuming operators are trusted cluster components running with cluster-admin-equivalent privileges.
2. Restructuring to per-component secrets would require changes to every consuming operator and potentially the CCO, which is out of scope.
3. The threat model for credential separation is external (blast radius reduction for compromised in-cluster credentials), not internal (operator-to-operator credential theft within a compromised cluster).

**Future consideration:** If per-operator Secret isolation is required, it would be addressed in a follow-on enhancement by migrating to per-component Secrets with targeted RBAC bindings.

### 6.4 vCenter Permission Validation — Read-Only Guarantee

The credential validation module must be demonstrably non-destructive. Design constraints:
- Only `HasPrivilegeOnEntities` (or equivalent read/check operations) are invoked during validation.
- No Terraform or infrastructure creation operations are initiated until all credential validations pass.
- The validation module must have no access to infrastructure provisioning functions.
- Unit tests using the govmomi simulator must verify that no write operations are issued against the simulated vCenter during validation.

### 6.5 Security Review Gate

Per NFR-S-5, this design must be reviewed against OpenShift security policies before implementation is finalized. Specifically, the following must be reviewed:
- The decision to co-locate all component credentials in a single Secret vs. per-component Secrets.
- The in-memory credential handling approach within the installer binary.
- The atomic handoff implementation and its failure-mode