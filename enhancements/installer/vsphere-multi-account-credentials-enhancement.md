---
title: vsphere-multi-account-credential-management
authors:
  - "@rvanderp"
reviewers:
  - "@jcpowermac, for vSphere platform expertise, please review privilege definitions and vCenter integration"
  - "@patrickdillon, for installer team review, please review installation workflow changes"
  - "@joelspeed, for cloud expertise, please review credential management approach"
  - "@gnufied, for storage team review, please review CSI driver credential separation"
approvers:
  - TBD
api-approvers:
  - TBD
creation-date: 2026-01-28
last-updated: 2026-01-28
status: provisional
tracking-link:
  - TBD
see-also:
  - "/enhancements/installer/vsphere-ipi.md"
  - "/enhancements/cloud-integration/cloud-credential-operator.md"
replaces: []
superseded-by: []
---

# vSphere Multi-Account Credential Management

## Summary

This enhancement proposes support for administrator-provisioned, component-specific vCenter credentials for OpenShift on vSphere. Rather than having the cloud-credential-operator (CCO) create vCenter accounts (which would require administrative privileges), this enhancement enables administrators to pre-provision separate vCenter service accounts for each OpenShift component and provide those credentials to OpenShift. CCO validates the credentials have sufficient privileges and distributes them to the appropriate components. This approach supports enterprise security requirements where account provisioning is controlled by infrastructure teams, while still achieving least-privilege isolation between components.

## Motivation

OpenShift on vSphere currently uses a single vCenter account for all operations across all components (installer, machine-api-operator, CSI driver, diagnostics). This creates several problems:

1. **Excessive Privilege Exposure:** Every component has access to all privileges, even those it doesn't need
2. **Large Blast Radius:** A compromised credential in any component grants full cluster and potentially vSphere infrastructure access
3. **Poor Auditability:** vCenter audit logs cannot distinguish which OpenShift component performed an action
4. **Compliance Challenges:** Single-account architecture conflicts with SOC2 separation of duties and PCI-DSS least privilege requirements
5. **Credential Rotation Complexity:** Rotating credentials requires updating all components simultaneously

Analysis of OpenShift source code across seven repositories (installer, machine-api-operator, vmware-vsphere-csi-driver, cluster-storage-operator, govmomi, vsphere-problem-detector, cloud-credential-operator) reveals that each component requires distinct privilege subsets:

| Component | Required Privileges | Current State |
|-----------|---------------------|---------------|
| Installer | ~45 (full set) | Uses shared credentials |
| Machine API | ~35 (VM lifecycle) | Uses shared credentials |
| CSI Driver | ~10-15 (storage) | Uses shared credentials |
| Cloud Controller | ~10 (read-only) | Uses shared credentials |
| Diagnostics | ~5 (read-only) | Uses shared credentials |

### Why Administrator-Provisioned (Not CCO-Minted)

Having CCO automatically create vCenter accounts would require:
- Administrative privileges on vCenter (Global.Licenses, Admin role)
- Access to vCenter SSO or identity source management
- Elevated trust in OpenShift to manage vCenter accounts

This conflicts with enterprise security practices where:
- Account provisioning is controlled by dedicated infrastructure/security teams
- Service accounts go through approval workflows
- Account creation is audited separately from account usage
- Identity management is centralized (Active Directory, LDAP)

**This enhancement takes the administrator-provisioned approach:** Administrators create the accounts using their existing processes, then provide the credentials to OpenShift.

### User Stories

* As a **security-conscious cluster administrator**, I want to provide each OpenShift component with separate vCenter credentials that I've pre-provisioned, so that I maintain control over account creation while achieving least-privilege isolation.

* As a **compliance officer**, I want OpenShift to support separation of duties for vCenter access using accounts provisioned through our standard identity management processes, so that our OpenShift deployment meets SOC2 and PCI-DSS requirements.

* As a **vSphere administrator**, I want to create vCenter service accounts with specific privileges for each OpenShift component, following my organization's account provisioning procedures, and then configure OpenShift to use these accounts.

* As a **platform operator**, I want OpenShift to validate that each component's credentials have the required privileges before accepting them, so that I can catch configuration errors during deployment rather than at runtime.

* As a **security team member**, I want documentation of exactly what privileges each OpenShift component requires, so that I can create appropriately-scoped vCenter roles and accounts before cluster deployment.

* As a **day-2 operations engineer**, I want to rotate credentials for individual OpenShift components independently by updating secrets, so that credential rotation follows my organization's rotation policies without affecting other components.

### Goals

1. **Support per-component credential configuration:** Enable administrators to provide distinct vCenter credentials for each component (machine-api, CSI driver, diagnostics).

2. **Document precise privilege requirements:** Provide authoritative documentation of privileges required by each component, organized by vSphere object scope.

3. **Validate credential privileges:** CCO validates that provided credentials have sufficient privileges for the component's operations.

4. **Provide tooling for role/account creation:** Offer scripts and documentation for creating vCenter roles and accounts with correct privileges.

5. **Enable independent credential rotation:** Support updating credentials for one component without affecting others.

6. **Maintain backward compatibility:** Continue supporting single shared credential (passthrough mode) for simpler deployments.

### Non-Goals

1. **Automatic vCenter account creation:** CCO will NOT create vCenter accounts; this requires admin privileges the operator should not have.

2. **Direct identity source integration:** CCO will not integrate with AD/LDAP to create accounts.

3. **Privilege escalation or modification:** CCO will not modify roles or add privileges to existing accounts.

4. **External secret manager integration:** Integration with HashiCorp Vault, CyberArk, etc. is out of scope.

5. **Runtime privilege discovery:** Automatic detection of what privileges a component actually uses is not in scope.

## Proposal

### Overview

This enhancement extends OpenShift to support administrator-provisioned, per-component vCenter credentials with multi-vCenter support:

1. **Credential storage:** Credentials are stored in install-config.yaml or a hidden file in the user's home directory (`~/.vsphere/credentials`)
2. **Multi-vCenter support:** Each component can have separate credentials for each vCenter in a multi-vCenter topology
3. **Privilege validation:** CCO validates credentials have required privileges before provisioning to components
4. **CredentialsRequest specifications:** Define precise privilege requirements per component
5. **Tooling and documentation:** Provide scripts to create vCenter roles matching component requirements
6. **Graceful degradation:** Fall back to shared credentials if per-component credentials not provided

### Component Architecture

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│                       Administrator Workflow (Per vCenter)                       │
├─────────────────────────────────────────────────────────────────────────────────┤
│                                                                                   │
│  1. Review privilege      2. Create vCenter       3. Create vCenter              │
│     documentation            roles (govc/UI)         accounts                    │
│         │                        │                       │                        │
│         ▼                        ▼                       ▼                        │
│  ┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐            │
│  │ Privilege Docs  │     │ govc role.create│     │ govc sso.user   │            │
│  │ per component   │     │ PowerCLI        │     │ AD/LDAP         │            │
│  └─────────────────┘     └─────────────────┘     └─────────────────┘            │
│         │                        │                       │                        │
│         └────────────────────────┴───────────────────────┘                        │
│                                  │                                                │
│                    ┌─────────────┴─────────────┐                                 │
│                    ▼                           ▼                                 │
│           ┌─────────────────┐         ┌─────────────────┐                        │
│           │   vCenter 1     │         │   vCenter 2     │                        │
│           │   (roles +      │         │   (roles +      │                        │
│           │    accounts)    │         │    accounts)    │                        │
│           └─────────────────┘         └─────────────────┘                        │
│                                  │                                                │
│                                  ▼                                                │
│                    4. Store credentials in:                                       │
│                       - install-config.yaml, OR                                   │
│                       - ~/.vsphere/credentials                                    │
│                                  │                                                │
└──────────────────────────────────┼────────────────────────────────────────────────┘
                                   │
                                   ▼
┌─────────────────────────────────────────────────────────────────────────────────┐
│                          OpenShift Cluster                                       │
├─────────────────────────────────────────────────────────────────────────────────┤
│                                                                                   │
│  ┌───────────────────────────────────────────────────────────────────────────┐  │
│  │                    cloud-credential-operator                               │  │
│  ├───────────────────────────────────────────────────────────────────────────┤  │
│  │                                                                             │  │
│  │  ┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐        │  │
│  │  │CredentialsRequest│   │ Multi-vCenter   │    │ Credential      │        │  │
│  │  │  Controller      │──▶│ Privilege       │──▶│ Distributor     │        │  │
│  │  │                  │   │ Validator       │    │                 │        │  │
│  │  └─────────────────┘    └─────────────────┘    └─────────────────┘        │  │
│  │                                                        │                    │  │
│  └────────────────────────────────────────────────────────┼────────────────────┘  │
│                                                           │                        │
│      ┌────────────────────────────────────────────────────┼────────────────────┐  │
│      │                                                    │                    │  │
│      ▼                                                    ▼                    ▼  │
│  ┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐              │
│  │ machine-api     │    │ csi-driver      │    │ diagnostics     │              │
│  │ credentials     │    │ credentials     │    │ credentials     │              │
│  │ (all vCenters)  │    │ (all vCenters)  │    │ (all vCenters)  │              │
│  └─────────────────┘    └─────────────────┘    └─────────────────┘              │
│                                                                                   │
└─────────────────────────────────────────────────────────────────────────────────┘
```

### Workflow Description

**Actors:**
- **vSphere administrator:** Creates vCenter accounts and roles
- **cluster administrator:** Provides credentials to OpenShift, manages cluster
- **cloud-credential-operator:** Validates and distributes credentials
- **OpenShift components:** Consume credentials for vCenter operations

#### Pre-Installation Workflow

1. The vSphere administrator reviews the privilege documentation for each component
2. The vSphere administrator creates custom roles in vCenter using provided scripts:
   ```bash
   # Example: Create machine-api role
   govc role.create openshift-machine-api \
       Sessions.ValidateSession \
       VirtualMachine.Config.AddNewDisk \
       VirtualMachine.Interact.PowerOn \
       # ... (all required privileges)
   ```
3. The vSphere administrator creates service accounts for each component:
   ```bash
   # Using vCenter SSO
   govc sso.user.create -p 'SecurePassword123!' ocp-cluster1-machine-api
   govc sso.user.create -p 'SecurePassword456!' ocp-cluster1-csi-driver
   ```
4. The vSphere administrator assigns roles to accounts on appropriate vSphere objects:
   ```bash
   govc permissions.set -principal 'ocp-cluster1-machine-api@vsphere.local' \
       -role openshift-machine-api \
       -propagate=true \
       /Datacenter/vm/openshift-cluster1
   ```
5. The vSphere administrator provides credentials to the cluster administrator

#### Installation Workflow (Per-Component Credentials Mode)

Credentials can be provided in two ways:

**Option A: install-config.yaml (Recommended for automation)**

The cluster administrator includes per-component credentials directly in install-config.yaml:

```yaml
apiVersion: v1
baseDomain: example.com
metadata:
  name: my-cluster
platform:
  vsphere:
    vcenters:
      - server: vcenter1.example.com
        user: ocp-installer@vsphere.local
        password: <installer-password>
        datacenters:
          - DC1
        componentCredentials:
          machineAPI:
            user: ocp-machine-api@vsphere.local
            password: <machine-api-password>
          csiDriver:
            user: ocp-csi@vsphere.local
            password: <csi-password>
          cloudController:
            user: ocp-ccm@vsphere.local
            password: <ccm-password>
          diagnostics:
            user: ocp-diagnostics@vsphere.local
            password: <diagnostics-password>
      - server: vcenter2.example.com
        user: ocp-installer@vsphere.local
        password: <installer-password-vc2>
        datacenters:
          - DC2
        componentCredentials:
          machineAPI:
            user: ocp-machine-api@vsphere.local
            password: <machine-api-password-vc2>
          csiDriver:
            user: ocp-csi@vsphere.local
            password: <csi-password-vc2>
          cloudController:
            user: ocp-ccm@vsphere.local
            password: <ccm-password-vc2>
          diagnostics:
            user: ocp-diagnostics@vsphere.local
            password: <diagnostics-password-vc2>
    failureDomains:
      - name: zone-a
        server: vcenter1.example.com
        # ...
      - name: zone-b
        server: vcenter2.example.com
        # ...
```

**Option B: Hidden credentials file (Recommended for interactive use)**

The cluster administrator creates a credentials file at `~/.vsphere/credentials`:

```ini
# ~/.vsphere/credentials
# Supports per-vCenter, per-component credentials

[vcenter1.example.com]
# Default credentials (used by installer)
user = ocp-installer@vsphere.local
password = <installer-password>

# Per-component credentials
machine-api.user = ocp-machine-api@vsphere.local
machine-api.password = <machine-api-password>
csi-driver.user = ocp-csi@vsphere.local
csi-driver.password = <csi-password>
cloud-controller.user = ocp-ccm@vsphere.local
cloud-controller.password = <ccm-password>
diagnostics.user = ocp-diagnostics@vsphere.local
diagnostics.password = <diagnostics-password>

[vcenter2.example.com]
user = ocp-installer@vsphere.local
password = <installer-password-vc2>
machine-api.user = ocp-machine-api@vsphere.local
machine-api.password = <machine-api-password-vc2>
csi-driver.user = ocp-csi@vsphere.local
csi-driver.password = <csi-password-vc2>
cloud-controller.user = ocp-ccm@vsphere.local
cloud-controller.password = <ccm-password-vc2>
diagnostics.user = ocp-diagnostics@vsphere.local
diagnostics.password = <diagnostics-password-vc2>
```

The installer reads from `~/.vsphere/credentials` when:
- Credentials are not specified in install-config.yaml
- The `VSPHERE_CREDENTIALS_FILE` environment variable points to a custom location

**Installation Process:**

1. The installer reads credentials from install-config.yaml or ~/.vsphere/credentials
2. The installer creates per-component credential secrets in the cluster
3. CCO validates each credential has required privileges for each vCenter
4. If validation fails, CCO reports which privileges are missing and on which vCenter
5. Components start using their designated credentials

#### Alternative: Post-Installation Configuration

For existing clusters migrating to per-component credentials:

1. The cluster administrator creates per-component secrets with multi-vCenter support:
   ```yaml
   apiVersion: v1
   kind: Secret
   metadata:
     name: vsphere-creds-machine-api
     namespace: openshift-config
   type: Opaque
   stringData:
     # Credentials for each vCenter
     vcenter1.example.com.username: "ocp-machine-api@vsphere.local"
     vcenter1.example.com.password: "<password-vc1>"
     vcenter2.example.com.username: "ocp-machine-api@vsphere.local"
     vcenter2.example.com.password: "<password-vc2>"
   ```
2. The cluster administrator updates the infrastructure configuration:
   ```yaml
   apiVersion: config.openshift.io/v1
   kind: Infrastructure
   metadata:
     name: cluster
   spec:
     platformSpec:
       vsphere:
         credentialsMode: PerComponent
         componentCredentials:
           machineAPI:
             secretRef:
               name: vsphere-creds-machine-api
               namespace: openshift-config
           csiDriver:
             secretRef:
               name: vsphere-creds-csi-driver
               namespace: openshift-config
           cloudController:
             secretRef:
               name: vsphere-creds-cloud-controller
               namespace: openshift-config
           diagnostics:
             secretRef:
               name: vsphere-creds-diagnostics
               namespace: openshift-config
   ```
3. CCO reconciles the new configuration and distributes credentials to components

#### Credential Validation Workflow

When CCO receives credentials for a component (per vCenter):

1. For each vCenter configured in the cluster:
   a. CCO extracts the component credentials for that vCenter
   b. CCO connects to the vCenter using the provided credentials
   c. CCO calls `AuthorizationManager.FetchUserPrivilegeOnEntities()` on relevant objects
   d. CCO compares returned privileges against required privileges for the component
2. If all required privileges are present on all vCenters, CCO provisions the credential to the component
3. If privileges are missing on any vCenter, CCO:
   - Sets condition `CredentialsProvisionFailed` on the CredentialsRequest
   - Logs detailed message listing missing privileges and the vCenter(s) affected
   - Does NOT provision incomplete credentials

### API Extensions

#### install-config.yaml Extension (Installer API)

The installer API is extended to support per-component credentials within each vCenter definition:

```go
// VCenter stores the vCenter connection fields and per-component credentials.
// This is part of the installer's install-config API.
type VCenter struct {
    // Server is the FQDN or IP address of the vCenter server.
    Server string `json:"server"`

    // Port is the TCP port that will be used to connect to vCenter.
    // +optional
    Port int32 `json:"port,omitempty"`

    // User is the username to use when connecting to vCenter.
    User string `json:"user"`

    // Password is the password for the user.
    Password string `json:"password"`

    // Datacenters is the list of datacenters to use within this vCenter.
    Datacenters []string `json:"datacenters"`

    // ComponentCredentials specifies per-component credentials for this vCenter.
    // If not specified, the main User/Password is used for all components.
    // +optional
    ComponentCredentials *VCenterComponentCredentials `json:"componentCredentials,omitempty"`
}

// VCenterComponentCredentials defines per-component credentials for a single vCenter.
type VCenterComponentCredentials struct {
    // MachineAPI specifies credentials for machine-api-operator on this vCenter.
    // +optional
    MachineAPI *VCenterCredential `json:"machineAPI,omitempty"`

    // CSIDriver specifies credentials for the vSphere CSI driver on this vCenter.
    // +optional
    CSIDriver *VCenterCredential `json:"csiDriver,omitempty"`

    // CloudController specifies credentials for the cloud controller manager on this vCenter.
    // +optional
    CloudController *VCenterCredential `json:"cloudController,omitempty"`

    // Diagnostics specifies credentials for vsphere-problem-detector on this vCenter.
    // +optional
    Diagnostics *VCenterCredential `json:"diagnostics,omitempty"`
}

// VCenterCredential stores username and password for a vCenter account.
type VCenterCredential struct {
    // User is the username for the account.
    User string `json:"user"`

    // Password is the password for the account.
    Password string `json:"password"`
}
```

#### ~/.vsphere/credentials File Format

The credentials file follows an INI-style format with per-vCenter sections:

```ini
# ~/.vsphere/credentials
# File permissions should be 0600 (readable only by owner)

# Section name is the vCenter server FQDN or IP
[vcenter1.example.com]
# Default credentials (used by installer and as fallback)
user = admin-user@vsphere.local
password = secret-password

# Per-component credentials (optional)
# Format: component-name.user and component-name.password
machine-api.user = ocp-machine-api@vsphere.local
machine-api.password = machine-api-password
csi-driver.user = ocp-csi@vsphere.local
csi-driver.password = csi-password
cloud-controller.user = ocp-ccm@vsphere.local
cloud-controller.password = ccm-password
diagnostics.user = ocp-diagnostics@vsphere.local
diagnostics.password = diagnostics-password

[vcenter2.example.com]
user = admin-user@vsphere.local
password = secret-password-vc2
machine-api.user = ocp-machine-api@vsphere.local
machine-api.password = machine-api-password-vc2
# ... other components
```

The installer reads credentials in this order of precedence:
1. Explicit credentials in install-config.yaml
2. `VSPHERE_CREDENTIALS_FILE` environment variable path
3. `~/.vsphere/credentials` default location

#### VSpherePlatformSpec Extension (Infrastructure API)

```go
// VSpherePlatformSpec holds configuration for the vSphere platform.
type VSpherePlatformSpec struct {
    // ... existing fields ...

    // CredentialsMode specifies how vSphere credentials are managed.
    // Valid values are:
    //   "Passthrough" - Single credential used for all components (default)
    //   "PerComponent" - Separate credentials per component
    // +optional
    CredentialsMode VSphereCredentialsMode `json:"credentialsMode,omitempty"`

    // ComponentCredentials specifies per-component credential references.
    // Only used when CredentialsMode is "PerComponent".
    // Each secret contains credentials for all vCenters.
    // +optional
    ComponentCredentials *VSphereComponentCredentials `json:"componentCredentials,omitempty"`
}

// VSphereCredentialsMode defines how credentials are managed.
// +kubebuilder:validation:Enum=Passthrough;PerComponent
type VSphereCredentialsMode string

const (
    // VSphereCredentialsModePassthrough uses single shared credentials.
    VSphereCredentialsModePassthrough VSphereCredentialsMode = "Passthrough"
    // VSphereCredentialsModePerComponent uses separate credentials per component.
    VSphereCredentialsModePerComponent VSphereCredentialsMode = "PerComponent"
)

// VSphereComponentCredentials defines credential references for each component.
type VSphereComponentCredentials struct {
    // MachineAPI specifies credentials for machine-api-operator.
    // The referenced secret contains keys in the format:
    //   <vcenter-server>.username and <vcenter-server>.password
    // +optional
    MachineAPI *SecretReference `json:"machineAPI,omitempty"`

    // CSIDriver specifies credentials for the vSphere CSI driver.
    // +optional
    CSIDriver *SecretReference `json:"csiDriver,omitempty"`

    // CloudController specifies credentials for the cloud controller manager.
    // +optional
    CloudController *SecretReference `json:"cloudController,omitempty"`

    // Diagnostics specifies credentials for vsphere-problem-detector.
    // +optional
    Diagnostics *SecretReference `json:"diagnostics,omitempty"`
}

// SecretReference identifies a secret in a namespace.
type SecretReference struct {
    // Name is the name of the secret.
    Name string `json:"name"`
    // Namespace is the namespace of the secret.
    Namespace string `json:"namespace"`
}
```

#### Per-Component Secret Format (Multi-vCenter)

Each component secret contains credentials for each vCenter, keyed by vCenter server FQDN:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: vsphere-creds-machine-api
  namespace: openshift-config
type: Opaque
stringData:
  # Format: <vcenter-fqdn>.username and <vcenter-fqdn>.password
  vcenter1.example.com.username: "ocp-machine-api@vsphere.local"
  vcenter1.example.com.password: "password-for-vc1"
  vcenter2.example.com.username: "ocp-machine-api@vsphere.local"
  vcenter2.example.com.password: "password-for-vc2"
```

This format supports:
- **Different identity sources per vCenter:** Each vCenter can use different SSO domains or identity providers
- **Different account names:** While not recommended, different usernames can be used per vCenter
- **Independent password rotation:** Passwords can be rotated on one vCenter without affecting others
- **Flexible deployment:** Easily add or remove vCenters from the topology

#### Extended VSphereProviderSpec for CredentialsRequest

```go
// VSphereProviderSpec contains the privilege requirements for a component.
// Used by CCO to validate that provided credentials have sufficient privileges.
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type VSphereProviderSpec struct {
    metav1.TypeMeta `json:",inline"`

    // Permissions contains the list of permission sets required by this component.
    // CCO validates that the provided credentials have these privileges.
    Permissions []VSpherePermission `json:"permissions"`
}

// VSpherePermission defines a set of privileges for a specific vSphere object scope.
type VSpherePermission struct {
    // Privileges is the list of vCenter privilege IDs required.
    // Example: "VirtualMachine.Config.AddNewDisk", "Datastore.AllocateSpace"
    Privileges []string `json:"privileges"`

    // Scope specifies where these privileges are required.
    Scope VSpherePermissionScope `json:"scope"`

    // Propagate indicates whether privileges must propagate to child objects.
    // +optional
    Propagate bool `json:"propagate,omitempty"`
}

// VSpherePermissionScope defines the vSphere object(s) where privileges are checked.
type VSpherePermissionScope struct {
    // Type specifies the type of vSphere object.
    // Valid values: "vCenter", "Datacenter", "Cluster", "ResourcePool",
    //               "Folder", "Datastore", "Network"
    Type string `json:"type"`

    // InferFromClusterConfig indicates the path should be derived from
    // the cluster's infrastructure configuration (failure domains).
    // +optional
    InferFromClusterConfig bool `json:"inferFromClusterConfig,omitempty"`
}
```

### Privilege Requirements by Component

Based on comprehensive code analysis of OpenShift repositories:

#### Machine API Operator

**vCenter Root (no propagation):**
```
Sessions.ValidateSession
InventoryService.Tagging.AttachTag
InventoryService.Tagging.CreateTag
InventoryService.Tagging.EditTag
InventoryService.Tagging.DeleteTag
```

**Cluster (with propagation):**
```
Resource.AssignVMToPool
VApp.AssignResourcePool
```

**VM Folder (with propagation):**
```
VirtualMachine.Config.AddExistingDisk
VirtualMachine.Config.AddNewDisk
VirtualMachine.Config.AddRemoveDevice
VirtualMachine.Config.AdvancedConfig
VirtualMachine.Config.Annotation
VirtualMachine.Config.CPUCount
VirtualMachine.Config.DiskExtend
VirtualMachine.Config.EditDevice
VirtualMachine.Config.Memory
VirtualMachine.Config.RemoveDisk
VirtualMachine.Config.Rename
VirtualMachine.Config.ResetGuestInfo
VirtualMachine.Config.Resource
VirtualMachine.Config.Settings
VirtualMachine.Interact.GuestControl
VirtualMachine.Interact.PowerOff
VirtualMachine.Interact.PowerOn
VirtualMachine.Interact.Reset
VirtualMachine.Inventory.Create
VirtualMachine.Inventory.CreateFromExisting
VirtualMachine.Inventory.Delete
VirtualMachine.Provisioning.Clone
VirtualMachine.Provisioning.DeployTemplate
VirtualMachine.State.CreateSnapshot
VirtualMachine.State.RemoveSnapshot
InventoryService.Tagging.ObjectAttachable
```

**Datastore (no propagation):**
```
Datastore.AllocateSpace
Datastore.Browse
Datastore.FileManagement
```

**Network (no propagation):**
```
Network.Assign
```

**Total: ~35 privileges**

#### CSI Driver

**vCenter Root (no propagation):**
```
Cns.Searchable
StorageProfile.View
Sessions.ValidateSession
```

**VM Folder (with propagation):**
```
VirtualMachine.Config.AddExistingDisk
VirtualMachine.Config.AddRemoveDevice
```

**Datastore (no propagation):**
```
Datastore.AllocateSpace
Datastore.Browse
Datastore.FileManagement
```

**Total: ~10 privileges**

#### Cloud Controller Manager (cloud-provider-vsphere)

The Cloud Controller Manager is a **read-only** component that handles node discovery, zone/region topology, and instance metadata. It never creates, modifies, or deletes vSphere objects.

**vCenter Root (no propagation):**
```
Sessions.ValidateSession
System.Read
InventoryService.Tagging.ObjectAttachable
```

**Datacenter (with propagation):**
```
System.Read
```

**VM Folder (with propagation):**
```
VirtualMachine.Config.Query
```

**Cluster/ComputeResource (no propagation):**
```
Host.Inventory.View
Resource.QueryVMotion
```

**Datastore (no propagation):**
```
Datastore.Browse
```

**Total: ~10 privileges (read-only)**

*Note: The built-in "Read-only" role provides all necessary privileges for this component.*

#### Diagnostics (vsphere-problem-detector)

**vCenter Root (no propagation):**
```
Sessions.ValidateSession
System.Read
```

**Datacenter (no propagation):**
```
System.Read
```

**Datastore (no propagation):**
```
Datastore.Browse
```

**Total: ~5 privileges (read-only)**

#### Installer

Uses the full privilege set as defined in `installer/pkg/asset/installconfig/vsphere/permissions.go` (~45 privileges).

### Tooling for Administrators

#### govc Script for Role Creation (Multi-vCenter)

```bash
#!/bin/bash
# create-openshift-roles.sh
# Creates vCenter roles for OpenShift components
# Supports multiple vCenters

set -e

# List of vCenter servers to configure
VCENTERS="${VCENTERS:-vcenter1.example.com vcenter2.example.com}"

# Create roles on each vCenter
for VCENTER in $VCENTERS; do
    echo "Creating roles on $VCENTER..."
    export GOVC_URL="$VCENTER"

    # Machine API Role
    govc role.create openshift-machine-api \
    Sessions.ValidateSession \
    InventoryService.Tagging.AttachTag \
    InventoryService.Tagging.CreateTag \
    InventoryService.Tagging.EditTag \
    InventoryService.Tagging.DeleteTag \
    Resource.AssignVMToPool \
    VApp.AssignResourcePool \
    VirtualMachine.Config.AddExistingDisk \
    VirtualMachine.Config.AddNewDisk \
    VirtualMachine.Config.AddRemoveDevice \
    VirtualMachine.Config.AdvancedConfig \
    VirtualMachine.Config.Annotation \
    VirtualMachine.Config.CPUCount \
    VirtualMachine.Config.DiskExtend \
    VirtualMachine.Config.EditDevice \
    VirtualMachine.Config.Memory \
    VirtualMachine.Config.RemoveDisk \
    VirtualMachine.Config.Rename \
    VirtualMachine.Config.ResetGuestInfo \
    VirtualMachine.Config.Resource \
    VirtualMachine.Config.Settings \
    VirtualMachine.Interact.GuestControl \
    VirtualMachine.Interact.PowerOff \
    VirtualMachine.Interact.PowerOn \
    VirtualMachine.Interact.Reset \
    VirtualMachine.Inventory.Create \
    VirtualMachine.Inventory.CreateFromExisting \
    VirtualMachine.Inventory.Delete \
    VirtualMachine.Provisioning.Clone \
    VirtualMachine.Provisioning.DeployTemplate \
    VirtualMachine.State.CreateSnapshot \
    VirtualMachine.State.RemoveSnapshot \
    InventoryService.Tagging.ObjectAttachable \
    Datastore.AllocateSpace \
    Datastore.Browse \
    Datastore.FileManagement \
    Network.Assign

# CSI Driver Role
govc role.create openshift-csi-driver \
    Sessions.ValidateSession \
    Cns.Searchable \
    StorageProfile.View \
    VirtualMachine.Config.AddExistingDisk \
    VirtualMachine.Config.AddRemoveDevice \
    Datastore.AllocateSpace \
    Datastore.Browse \
    Datastore.FileManagement

    # Cloud Controller Manager Role (read-only)
    govc role.create openshift-cloud-controller \
        Sessions.ValidateSession \
        System.Read \
        InventoryService.Tagging.ObjectAttachable \
        VirtualMachine.Config.Query \
        Host.Inventory.View \
        Resource.QueryVMotion \
        Datastore.Browse

    # Diagnostics Role (read-only)
    govc role.create openshift-diagnostics \
        Sessions.ValidateSession \
        System.Read \
        Datastore.Browse

    echo "Roles created on $VCENTER"
done

echo "All roles created successfully on all vCenters"
```

#### Script to Generate ~/.vsphere/credentials

```bash
#!/bin/bash
# generate-vsphere-credentials.sh
# Generates the ~/.vsphere/credentials file for per-component credentials

CREDS_FILE="${HOME}/.vsphere/credentials"
CREDS_DIR="${HOME}/.vsphere"

# Create directory with secure permissions
mkdir -p "$CREDS_DIR"
chmod 700 "$CREDS_DIR"

# Generate credentials file template
cat > "$CREDS_FILE" << 'EOF'
# vSphere Credentials for OpenShift
# Generated by generate-vsphere-credentials.sh
# File permissions: 0600 (readable only by owner)

# Add a section for each vCenter server
# Format: [vcenter-fqdn-or-ip]

[vcenter1.example.com]
# Default credentials (used by installer)
user = ocp-installer@vsphere.local
password = REPLACE_WITH_INSTALLER_PASSWORD

# Per-component credentials
machine-api.user = ocp-machine-api@vsphere.local
machine-api.password = REPLACE_WITH_MACHINE_API_PASSWORD
csi-driver.user = ocp-csi@vsphere.local
csi-driver.password = REPLACE_WITH_CSI_PASSWORD
cloud-controller.user = ocp-ccm@vsphere.local
cloud-controller.password = REPLACE_WITH_CCM_PASSWORD
diagnostics.user = ocp-diagnostics@vsphere.local
diagnostics.password = REPLACE_WITH_DIAGNOSTICS_PASSWORD

# Add additional vCenters as needed:
# [vcenter2.example.com]
# user = ...
# password = ...
# machine-api.user = ...
# ...
EOF

# Set secure permissions
chmod 600 "$CREDS_FILE"

echo "Credentials template created at $CREDS_FILE"
echo "Please edit the file and replace placeholder passwords"
```

#### PowerCLI Script for Role Creation

```powershell
# Create-OpenShiftRoles.ps1
# Creates vCenter roles for OpenShift components

param(
    [Parameter(Mandatory=$true)]
    [string]$VCenterServer
)

Connect-VIServer -Server $VCenterServer

# Machine API Role
$machineAPIPrivileges = @(
    "Sessions.ValidateSession",
    "InventoryService.Tagging.AttachTag",
    # ... (all privileges)
)
New-VIRole -Name "openshift-machine-api" -Privilege (Get-VIPrivilege -Id $machineAPIPrivileges)

# CSI Driver Role
$csiPrivileges = @(
    "Sessions.ValidateSession",
    "Cns.Searchable",
    "StorageProfile.View",
    # ... (all privileges)
)
New-VIRole -Name "openshift-csi-driver" -Privilege (Get-VIPrivilege -Id $csiPrivileges)

# Diagnostics Role
$diagPrivileges = @(
    "Sessions.ValidateSession",
    "System.Read",
    "Datastore.Browse"
)
New-VIRole -Name "openshift-diagnostics" -Privilege (Get-VIPrivilege -Id $diagPrivileges)

Write-Host "Roles created successfully"
```

### Credential File Security

The `~/.vsphere/credentials` file contains sensitive vCenter credentials. The following security measures are enforced:

1. **File Permissions:** The installer validates that the credentials file has mode `0600` (owner read/write only) and refuses to proceed if permissions are too open.

2. **Directory Permissions:** The `~/.vsphere/` directory should have mode `0700`.

3. **Environment Variable Override:** For CI/CD pipelines, use `VSPHERE_CREDENTIALS_FILE` to point to a credentials file managed by the pipeline's secret management.

4. **Credential Precedence:** Credentials in install-config.yaml take precedence over the credentials file, allowing pipeline automation to override user defaults.

5. **Cleanup:** The installer never modifies the credentials file. Administrators are responsible for credential rotation and cleanup.

### Multi-vCenter Considerations

OpenShift supports spanning multiple vCenters using failure domains. This enhancement ensures per-component credentials work correctly with multi-vCenter topologies:

1. **Per-vCenter Credentials in Each Component Secret:** Each component secret contains separate credentials for each vCenter, allowing:
   - Different identity sources per vCenter
   - Independent password management
   - Flexibility in account naming per vCenter

2. **Credential Validation:** CCO validates each component's credentials on each vCenter independently and reports which vCenter(s) have authentication failures or missing privileges.

3. **Role Consistency:** The same role should be created on each vCenter with the same privileges. The provided scripts handle multi-vCenter role creation.

4. **Secret Key Format:** Credentials are keyed by vCenter FQDN:
   ```yaml
   stringData:
     vcenter1.example.com.username: "user@domain"
     vcenter1.example.com.password: "password1"
     vcenter2.example.com.username: "user@domain"
     vcenter2.example.com.password: "password2"
   ```

5. **Credentials File Format:** The `~/.vsphere/credentials` file similarly organizes credentials by vCenter with per-component entries under each vCenter section.

### Topology Considerations

#### Hypershift / Hosted Control Planes

For Hypershift deployments:
- The management cluster administrator provides credentials for each hosted cluster
- Credentials are stored in the management cluster, scoped per hosted cluster
- Each hosted cluster's components use credentials scoped to that cluster's resources

#### Standalone Clusters

This enhancement fully applies to standalone clusters and is the primary target.

#### Single-node Deployments or MicroShift

For single-node OpenShift (SNO):
- The same per-component credential model applies
- Reduces blast radius even on single-node deployments

For MicroShift:
- This enhancement does not apply; MicroShift does not use CCO

#### OpenShift Kubernetes Engine

Compatible with OKE; does not depend on OCP-specific features beyond CCO.

### Implementation Details/Notes/Constraints

#### Privilege Validation Implementation

```go
// ValidateCredentialPrivileges checks that credentials have required privileges
func (a *VSphereActuator) ValidateCredentialPrivileges(
    ctx context.Context,
    creds *corev1.Secret,
    required []VSpherePermission,
) error {
    // Connect to vCenter with provided credentials
    client, err := a.connectWithCredentials(ctx, creds)
    if err != nil {
        return fmt.Errorf("failed to connect: %v", err)
    }
    defer client.Logout(ctx)

    authManager := object.NewAuthorizationManager(client.Client)
    sessionMgr := session.NewManager(client.Client)
    userSession, _ := sessionMgr.UserSession(ctx)

    var missingPrivileges []string

    for _, perm := range required {
        // Get the managed object reference for the scope
        moRef, err := a.resolveScopeToMoRef(ctx, client, perm.Scope)
        if err != nil {
            return fmt.Errorf("failed to resolve scope %v: %v", perm.Scope, err)
        }

        // Fetch user's privileges on this object
        results, err := authManager.FetchUserPrivilegeOnEntities(ctx,
            []types.ManagedObjectReference{moRef},
            userSession.UserName)
        if err != nil {
            return fmt.Errorf("failed to fetch privileges: %v", err)
        }

        userPrivs := sets.NewString()
        for _, result := range results {
            userPrivs.Insert(result.Privileges...)
        }

        // Check each required privilege
        for _, reqPriv := range perm.Privileges {
            if !userPrivs.Has(reqPriv) {
                missingPrivileges = append(missingPrivileges,
                    fmt.Sprintf("%s on %s", reqPriv, perm.Scope.Type))
            }
        }
    }

    if len(missingPrivileges) > 0 {
        return fmt.Errorf("missing privileges: %v", missingPrivileges)
    }

    return nil
}
```

#### Fallback to Passthrough Mode

If per-component credentials are not provided, CCO falls back to passthrough mode:

```go
func (a *VSphereActuator) GetCredentialsForComponent(
    ctx context.Context,
    component string,
) (*corev1.Secret, error) {
    // Check for per-component credentials
    componentCreds, err := a.getComponentCredentials(ctx, component)
    if err == nil && componentCreds != nil {
        return componentCreds, nil
    }

    // Fall back to root credentials (passthrough)
    return a.getRootCredentials(ctx)
}
```

### Risks and Mitigations

| Risk | Mitigation |
|------|------------|
| Administrator provides insufficient privileges | CCO validates privileges per vCenter and reports specific missing privileges |
| Complex setup burden on administrators | Provide scripts (govc, PowerCLI) and detailed documentation |
| Credential secrets accidentally deleted | Standard Kubernetes secret backup practices; CCO recreates from source |
| vCenter version has different privilege names | Use pruneToAvailablePermissions pattern; validate against actual vCenter |
| Migration from passthrough mode is disruptive | Support gradual migration; components fall back gracefully |
| ~/.vsphere/credentials file exposed | Enforce 0600 permissions; installer refuses to proceed if permissions too open |
| Credentials in install-config.yaml committed to git | Document best practices; warn users about including secrets in version control |
| Multi-vCenter credential mismatch | CCO validates credentials per vCenter before provisioning; clear error messages identify which vCenter failed |
| Inconsistent accounts across vCenters | Provide multi-vCenter scripts that configure all vCenters consistently |

### Drawbacks

1. **Increased Setup Complexity:** Administrators must create multiple accounts/roles
2. **Documentation Overhead:** Must maintain per-component privilege lists
3. **Potential for Misconfiguration:** More credentials means more chances for errors
4. **vCenter Administrative Burden:** More accounts to manage and audit

## Alternatives (Not Implemented)

### Alternative 1: CCO Creates vCenter Accounts (Mint Mode)

**Description:** CCO uses administrative credentials to create per-component accounts automatically.

**Pros:**
- Fully automated
- No manual account creation
- Consistent naming

**Cons:**
- Requires administrative vCenter privileges
- Conflicts with enterprise security practices
- CCO becomes a privileged identity management system

**Why not selected:** Enterprises typically have strict controls on account creation; giving CCO this power is a security concern.

### Alternative 2: Single Account with Multiple Roles

**Description:** Use one account but assign different roles on different vSphere objects.

**Pros:**
- Simpler credential management
- Still provides privilege scoping

**Cons:**
- Single point of compromise
- Cannot distinguish actions in audit logs
- Credential rotation affects all components

**Why not selected:** Does not achieve audit separation goal.

### Alternative 3: External Identity Provider Integration

**Description:** Integrate with external IdP (Vault, CyberArk) for dynamic credentials.

**Pros:**
- Dynamic, short-lived credentials
- Centralized secret management
- Built-in audit

**Cons:**
- Requires external infrastructure
- Adds complexity
- Dependency on third-party system

**Why not selected:** Out of scope; can be future enhancement.

## Open Questions

1. **Credentials File Format:** Should the `~/.vsphere/credentials` file use INI format (as proposed) or YAML for consistency with install-config.yaml?

2. **Validation Frequency:** Should CCO re-validate privileges periodically, or only on secret changes?

3. **Partial Configuration:** If only some component credentials are provided, should CCO use per-component for those and passthrough for others?

4. **Installer Credentials Lifecycle:** Should installer credentials be automatically disabled post-installation?

5. **Cross-vCenter Account Naming:** Should we recommend the same account name across all vCenters (simpler) or unique names per vCenter (more auditable)?

6. **Credentials File Discovery:** Should the installer search additional locations (e.g., `/etc/vsphere/credentials` for system-wide configuration)?

## Test Plan

### Unit Tests

- VSphereProviderSpec parsing and privilege list handling
- Privilege validation logic with mock AuthorizationManager
- Fallback to passthrough mode
- Error messaging for missing privileges

### Integration Tests

- Credential validation using govcsim
- Per-component secret distribution
- Mixed mode (some per-component, some passthrough)
- Privilege validation failure scenarios

### E2E Tests

- Full installation with per-component credentials
- Verify components use correct credentials (audit log verification)
- Credential rotation for individual components
- Migration from passthrough to per-component mode

## Graduation Criteria

### Dev Preview -> Tech Preview

- Per-component credential configuration supported
- Privilege validation implemented
- Documentation for creating roles/accounts
- Scripts for govc and PowerCLI

### Tech Preview -> GA

- E2E tests in CI
- Tested on vSphere 7.0 and 8.0
- User documentation in openshift-docs
- Migration guide from passthrough mode
- Support runbook

## Upgrade / Downgrade Strategy

### Upgrade (Passthrough → PerComponent)

1. Administrator creates per-component accounts
2. Administrator creates per-component secrets
3. Administrator updates Infrastructure CR to PerComponent mode
4. CCO validates and distributes new credentials
5. Components pick up new credentials

### Downgrade (PerComponent → Passthrough)

1. Administrator updates Infrastructure CR to Passthrough mode
2. CCO reverts to distributing root credentials
3. Per-component secrets remain but are unused

## Version Skew Strategy

- New CCO version is backward compatible with passthrough mode
- API extensions are additive (new fields, not breaking changes)
- Components that don't understand per-component mode use existing secret paths

## Operational Aspects of API Extensions

### New API Fields

- `VSpherePlatformSpec.CredentialsMode`: Enum, no webhook needed
- `VSpherePlatformSpec.ComponentCredentials`: Reference to secrets

### Impact on SLIs

- Minimal additional vCenter API calls for privilege validation
- Validation occurs once per secret change, not continuously

### Failure Modes

| Failure | Detection | Resolution |
|---------|-----------|------------|
| Missing privileges | CCO condition `CredentialsProvisionFailed` | Add missing privileges to vCenter role |
| Invalid credentials | CCO logs authentication failure | Verify account and password |
| Secret not found | CCO condition reports missing secret | Create the referenced secret |

## Support Procedures

### Detecting Issues

```bash
# Check CredentialsRequest status
oc get credentialsrequest -n openshift-cloud-credential-operator -o yaml

# Check CCO logs
oc logs -n openshift-cloud-credential-operator deployment/cloud-credential-operator

# Verify component secrets exist
oc get secret -n openshift-machine-api vsphere-cloud-credentials -o yaml
```

### Reverting to Passthrough Mode

```bash
# Update Infrastructure CR
oc patch infrastructure cluster --type=merge -p '
{
  "spec": {
    "platformSpec": {
      "vsphere": {
        "credentialsMode": "Passthrough"
      }
    }
  }
}'
```

## Infrastructure Needed

- vSphere test environments with configurable accounts
- CI integration for privilege validation testing
- Documentation site updates for per-component setup guide
