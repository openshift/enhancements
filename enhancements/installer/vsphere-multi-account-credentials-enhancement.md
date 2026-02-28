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
last-updated: 2026-02-13
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

This enhancement proposes support for administrator-provisioned, component-specific vCenter credentials for OpenShift on vSphere. Rather than having the cloud-credential-operator (CCO) create vCenter accounts (which would require administrative privileges), this enhancement enables administrators to pre-provision separate vCenter service accounts for each OpenShift component and provide those credentials to OpenShift. The installer creates per-component credential secrets in the cluster and CCO distributes them to the appropriate components, falling back to shared credentials when dedicated secrets are not provided. This approach supports enterprise security requirements where account provisioning is controlled by infrastructure teams, while still achieving least-privilege isolation between components.

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

* As a **security team member**, I want documentation of exactly what privileges each OpenShift component requires, so that I can create appropriately-scoped vCenter roles and accounts before cluster deployment.

* As a **day-2 operations engineer**, I want to rotate credentials for individual OpenShift components independently by updating secrets, so that credential rotation follows my organization's rotation policies without affecting other components.

### Goals

1. **Support per-component credential configuration:** Enable administrators to provide distinct vCenter credentials for each component (machine-api, CSI driver, diagnostics).

2. **Document precise privilege requirements:** Provide authoritative documentation of privileges required by each component, organized by vSphere object scope.

3. **Provide tooling for role/account creation:** Offer scripts and documentation for creating vCenter roles and accounts with correct privileges.

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
3. **Secret generation:** The installer creates per-component credential secrets in `kube-system` keyed by vCenter FQDN
4. **CCO secret distribution:** CCO discovers dedicated component secrets via annotation or name-based lookup and syncs them to target namespaces
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
│  kube-system namespace (source secrets):                                         │
│  ┌──────────────────┐ ┌──────────────────┐ ┌──────────────────┐                 │
│  │ vsphere-creds    │ │ vsphere-creds-   │ │ vsphere-creds-   │  ...            │
│  │ (root/shared)    │ │ machine-api      │ │ csi-driver       │                 │
│  └────────┬─────────┘ └────────┬─────────┘ └────────┬─────────┘                 │
│           │                    │                     │                            │
│           └────────────────────┴─────────────────────┘                            │
│                                │                                                  │
│  ┌─────────────────────────────┴───────────────────────────────────────────────┐  │
│  │                    cloud-credential-operator                               │  │
│  ├─────────────────────────────────────────────────────────────────────────────┤  │
│  │                                                                             │  │
│  │  For each CredentialsRequest:                                               │  │
│  │  1. Search kube-system for dedicated secret by annotation                   │  │
│  │     (cloudcredential.openshift.io/credentials-request = ns/name)            │  │
│  │  2. If not found, look up dedicated secret by hardcoded name mapping        │  │
│  │  3. If not found, fall back to root vsphere-creds secret                    │  │
│  │  4. Sync chosen secret data to target namespace/secret                      │  │
│  │                                                                             │  │
│  └─────────────────────────────────────────────────────────────────────────────┘  │
│                                │                                                  │
│      ┌─────────────────────────┼──────────────────────────┐                      │
│      ▼                         ▼                          ▼                      │
│  ┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐              │
│  │ machine-api     │    │ csi-driver      │    │ diagnostics     │              │
│  │ credentials     │    │ credentials     │    │ credentials     │              │
│  │ (target ns)     │    │ (target ns)     │    │ (target ns)     │              │
│  └─────────────────┘    └─────────────────┘    └─────────────────┘              │
│                                                                                   │
└─────────────────────────────────────────────────────────────────────────────────┘
```

### Workflow Description

**Actors:**
- **vSphere administrator:** Creates vCenter accounts and roles
- **cluster administrator:** Provides credentials to OpenShift, manages cluster
- **cloud-credential-operator:** Discovers and distributes credentials to target namespaces
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
2. The installer merges credentials (install-config takes precedence over credentials file)
3. The installer creates per-component credential secrets in `kube-system` with the key format `<vcenter-fqdn>.username` / `<vcenter-fqdn>.password`
4. CCO discovers dedicated secrets for each CredentialsRequest (via annotation or name-based lookup)
5. CCO syncs the dedicated secret data to each component's target namespace/secret
6. If no dedicated secret exists for a component, CCO falls back to the root `vsphere-creds` secret

#### Alternative: Post-Installation Configuration

For existing clusters migrating to per-component credentials:

1. The cluster administrator creates per-component secrets in `kube-system` with multi-vCenter support:
   ```yaml
   apiVersion: v1
   kind: Secret
   metadata:
     name: vsphere-creds-machine-api
     namespace: kube-system
     labels:
       cloudcredential.openshift.io/credentials-request: "yes"
     annotations:
       cloudcredential.openshift.io/credentials-request: "openshift-cloud-credential-operator/openshift-machine-api-vsphere"
   type: Opaque
   stringData:
     # Credentials for each vCenter
     vcenter1.example.com.username: "ocp-machine-api@vsphere.local"
     vcenter1.example.com.password: "<password-vc1>"
     vcenter2.example.com.username: "ocp-machine-api@vsphere.local"
     vcenter2.example.com.password: "<password-vc2>"
   ```
2. CCO automatically discovers the new secrets via annotation or name-based lookup and syncs them to the component's target namespace
3. If the dedicated secret is removed, CCO falls back to distributing the root `vsphere-creds` secret

#### CCO Credential Lookup Workflow

When CCO processes a CredentialsRequest for a vSphere component, it determines which credential secret to use through a multi-step lookup:

1. **Annotation-based lookup (primary):** CCO lists all secrets in `kube-system` with the label `cloudcredential.openshift.io/credentials-request: "yes"` and finds the one whose `cloudcredential.openshift.io/credentials-request` annotation matches the CredentialsRequest's `namespace/name`.

2. **Name-based lookup (fallback):** If no annotated secret is found, CCO maps the CredentialsRequest name to a hardcoded dedicated secret name:

   | CredentialsRequest Name | Dedicated Secret Name |
   |---|---|
   | `openshift-machine-api-vsphere` | `vsphere-creds-machine-api` |
   | `openshift-vmware-vsphere-csi-driver-operator` | `vsphere-creds-csi-driver` |
   | `openshift-vsphere-cloud-controller-manager` | `vsphere-creds-cloud-controller` |
   | `openshift-vsphere-problem-detector` | `vsphere-creds-diagnostics` |

3. **Root credential fallback:** If no dedicated secret exists, CCO falls back to the shared `vsphere-creds` secret in `kube-system`.

4. **Sync to target:** CCO copies the chosen secret's data to the target secret specified in the CredentialsRequest's `spec.secretRef`.

**Cache selector:** The vSphere CCO cache watches all secrets in the `kube-system` namespace (scoped by namespace rather than a single secret name) to ensure it can discover dedicated component secrets.

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

#### Per-Component Secret Format (Multi-vCenter)

Each component secret contains credentials for each vCenter, keyed by vCenter server FQDN:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: vsphere-creds-machine-api
  namespace: kube-system
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

#### Existing VSphereProviderSpec for CredentialsRequest

The existing `VSphereProviderSpec` in CCO already defines a `Permissions` field that lists the privileges required by each component. This is used to document privilege requirements in each component's CredentialsRequest manifest:

```go
// VSphereProviderSpec contains the privilege requirements for a component.
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type VSphereProviderSpec struct {
    metav1.TypeMeta `json:",inline"`

    // Permissions contains the list of permission sets required by this component.
    Permissions []VSpherePermission `json:"permissions"`
}

// VSpherePermission defines a set of privileges for a specific vSphere object scope.
type VSpherePermission struct {
    // Privileges is the list of vCenter privilege IDs required.
    Privileges []string `json:"privileges"`
}
```

Note: In the current implementation, CCO does not actively validate these privileges against vCenter. The `Permissions` field serves as declarative documentation of what each component requires. See [Future Work](#future-work) for planned privilege validation.

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

2. **Credential Discovery:** CCO discovers dedicated component secrets via annotation or name-based lookup in `kube-system` and syncs them to component target namespaces.

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

#### Installer: Per-Component Secret Generation

When the installer detects that any `VCenter` in the install-config has `ComponentCredentials` set, it generates separate secret manifests for each component instead of the single shared `vsphere-creds` secret:

- `99_vsphere-creds-machine-api.yaml` - Secret `vsphere-creds-machine-api` in `kube-system`
- `99_vsphere-creds-csi-driver.yaml` - Secret `vsphere-creds-csi-driver` in `kube-system`
- `99_vsphere-creds-cloud-controller.yaml` - Secret `vsphere-creds-cloud-controller` in `kube-system`
- `99_vsphere-creds-diagnostics.yaml` - Secret `vsphere-creds-diagnostics` in `kube-system`

For each component, the installer uses the component-specific credentials if provided, otherwise falls back to the main `User`/`Password` on the `VCenter` entry. Each secret uses the multi-vCenter key format:

```yaml
data:
  vcenter1.example.com.username: <base64>
  vcenter1.example.com.password: <base64>
  vcenter2.example.com.username: <base64>
  vcenter2.example.com.password: <base64>
```

#### Installer: Cloud Provider Config Update

When component credentials are present, the installer updates the cloud provider config to reference `vsphere-creds-cloud-controller` instead of the default `vsphere-creds` secret, so the cloud controller manager reads from its dedicated credential secret.

#### CCO: Dedicated Secret Lookup

The CCO vSphere actuator resolves credentials for each CredentialsRequest through a tiered lookup:

```go
// GetDedicatedCredentialsSecret finds a dedicated secret for a CredentialsRequest.
// Returns nil if no dedicated secret exists (caller should fall back to root secret).
func (a *VSphereActuator) GetDedicatedCredentialsSecret(ctx context.Context, cr *credreqv1.CredentialsRequest) (*corev1.Secret, error) {
    // Primary: look for secret annotated with this CredentialsRequest's namespace/name
    secret, err := a.findDedicatedSecretByAnnotation(ctx, cr)
    if err == nil && secret != nil {
        return secret, nil
    }

    // Fallback: look for secret by hardcoded name mapping
    return a.findDedicatedSecretByName(ctx, cr)
}
```

The annotation-based lookup lists secrets in `kube-system` with label `cloudcredential.openshift.io/credentials-request: "yes"` and matches the annotation `cloudcredential.openshift.io/credentials-request` against the CredentialsRequest's `namespace/name`.

The name-based lookup uses a hardcoded mapping from CredentialsRequest name to secret name:

```go
var crNameToSecretName = map[string]string{
    "openshift-machine-api-vsphere":                    "vsphere-creds-machine-api",
    "openshift-vmware-vsphere-csi-driver-operator":     "vsphere-creds-csi-driver",
    "openshift-vsphere-cloud-controller-manager":       "vsphere-creds-cloud-controller",
    "openshift-vsphere-problem-detector":               "vsphere-creds-diagnostics",
}
```

#### CCO: Cache Selector Change

The vSphere CCO field selector was changed from watching a single secret (the root `vsphere-creds`) to watching all secrets in the `kube-system` namespace. This is necessary for the actuator to discover the per-component dedicated secrets.

#### CCO: Sync Behavior

The CCO `sync()` method for vSphere no longer checks for the passthrough annotation on the source credential secret before syncing. This change was necessary because dedicated component secrets created by the installer may not carry the passthrough annotation. The actuator always syncs the resolved credential data to the CredentialsRequest's target secret.

### Risks and Mitigations

| Risk | Mitigation |
|------|------------|
| Administrator provides insufficient privileges | Document precise privilege requirements per component; provide role creation scripts |
| Complex setup burden on administrators | Provide scripts (govc, PowerCLI) and detailed documentation |
| Credential secrets accidentally deleted | CCO falls back to root `vsphere-creds`; administrator can recreate dedicated secrets |
| Migration from passthrough mode is disruptive | Support gradual migration; CCO falls back to root secret when dedicated secrets are absent |
| ~/.vsphere/credentials file exposed | Enforce 0600 permissions; installer refuses to proceed if permissions too open |
| Credentials in install-config.yaml committed to git | Document best practices; warn users about including secrets in version control |
| Inconsistent accounts across vCenters | Provide multi-vCenter scripts that configure all vCenters consistently |
| CCO cache watches all kube-system secrets | Scoped by namespace; no performance concern since kube-system secret count is bounded |

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

1. **Installer Credentials Lifecycle:** Should installer credentials be automatically disabled post-installation?

2. **Cross-vCenter Account Naming:** Should we recommend the same account name across all vCenters (simpler) or unique names per vCenter (more auditable)?

### Resolved Questions

1. **Credentials File Format:** INI format was chosen (`~/.vsphere/credentials`) using Go's `gopkg.in/ini.v1` parser.

2. **Partial Configuration:** Yes. CCO uses dedicated secrets for components that have them and falls back to the root `vsphere-creds` secret for components that don't. The installer similarly falls back to the main `VCenter.User`/`Password` for any component without explicit `ComponentCredentials`.

3. **Credential Precedence:** install-config.yaml > `VSPHERE_CREDENTIALS_FILE` environment variable > `~/.vsphere/credentials` default location.

## Test Plan

### Unit Tests

**Installer:**
- `~/.vsphere/credentials` file loading and parsing (INI format)
- File permission validation (reject if not 0600 or stricter)
- Single vCenter and multi-vCenter credential loading
- Partial component credentials (some components defined, others not)
- Credential merging precedence (install-config over file)
- Per-component secret generation with fallback to main credentials

**CCO:**
- Annotation-based dedicated secret lookup
- Name-based dedicated secret lookup (hardcoded mapping)
- Fallback to root `vsphere-creds` when no dedicated secret exists
- Multi-vCenter secret format handling (`<vcenter>.username`/`<vcenter>.password` keys)
- Sync behavior without passthrough annotation

### Integration Tests

- Per-component secret distribution end-to-end
- Mixed mode (some components with dedicated secrets, others using root)
- Secret content correctness with multi-vCenter topology

### E2E Tests

- Full installation with per-component credentials
- Verify components use correct credentials (audit log verification)
- Credential rotation for individual components
- Fallback behavior when dedicated secrets are removed

## Graduation Criteria

### Dev Preview -> Tech Preview

- Per-component credential configuration supported in install-config.yaml and `~/.vsphere/credentials`
- CCO dedicated secret lookup (annotation and name-based) implemented
- Documentation for creating roles/accounts
- Scripts for govc and PowerCLI

### Tech Preview -> GA

- E2E tests in CI
- Tested on vSphere 7.0 and 8.0
- User documentation in openshift-docs
- Migration guide from passthrough mode
- Support runbook

## Upgrade / Downgrade Strategy

### Upgrade (Shared → Per-Component)

1. Administrator creates per-component vCenter accounts with appropriate roles
2. Administrator creates dedicated secrets in `kube-system` with the annotation `cloudcredential.openshift.io/credentials-request` set to the CredentialsRequest's `namespace/name`, or uses the well-known secret names (`vsphere-creds-machine-api`, etc.)
3. CCO automatically discovers dedicated secrets and syncs them to component target namespaces
4. Components pick up new credentials on next reconciliation

### Downgrade (Per-Component → Shared)

1. Administrator deletes the dedicated per-component secrets from `kube-system`
2. CCO falls back to distributing the root `vsphere-creds` secret
3. Components continue operating with shared credentials

## Version Skew Strategy

- New CCO version is backward compatible with passthrough mode
- API extensions are additive (new fields, not breaking changes)
- Components that don't understand per-component mode use existing secret paths

## Operational Aspects of API Extensions

### New API Fields

- `VCenter.ComponentCredentials` in install-config.yaml: Per-component credential configuration within each vCenter entry

### Impact on SLIs

- CCO watches all secrets in `kube-system` instead of a single secret; bounded set so negligible impact
- No additional vCenter API calls from CCO (secret lookup is Kubernetes-only)

### Failure Modes

| Failure | Detection | Resolution |
|---------|-----------|------------|
| Dedicated secret missing | CCO falls back to root `vsphere-creds`; no error | Create dedicated secret if per-component is desired |
| Invalid credentials in dedicated secret | Component logs authentication failure to vCenter | Verify account and password in the dedicated secret |
| Incorrect secret key format | Component fails to extract credentials | Ensure keys use `<vcenter-fqdn>.username` / `<vcenter-fqdn>.password` format |
| Credentials file permissions too open | Installer refuses to load file | Set `chmod 600 ~/.vsphere/credentials` |

## Support Procedures

### Detecting Issues

```bash
# Check CredentialsRequest status
oc get credentialsrequest -n openshift-cloud-credential-operator -o yaml

# Check CCO logs
oc logs -n openshift-cloud-credential-operator deployment/cloud-credential-operator

# Verify dedicated component secrets exist in kube-system
oc get secret -n kube-system vsphere-creds-machine-api
oc get secret -n kube-system vsphere-creds-csi-driver
oc get secret -n kube-system vsphere-creds-cloud-controller
oc get secret -n kube-system vsphere-creds-diagnostics

# Verify component target secrets are synced
oc get secret -n openshift-machine-api vsphere-cloud-credentials -o yaml
```

### Reverting to Shared Credentials

```bash
# Delete dedicated secrets to revert to shared credentials
oc delete secret -n kube-system vsphere-creds-machine-api
oc delete secret -n kube-system vsphere-creds-csi-driver
oc delete secret -n kube-system vsphere-creds-cloud-controller
oc delete secret -n kube-system vsphere-creds-diagnostics

# CCO will automatically fall back to using the root vsphere-creds secret
```

## Future Work

The following capabilities are planned for future iterations but are not part of the current implementation:

### Privilege Validation

CCO could validate that each component's credentials have sufficient vCenter privileges before syncing them. This would involve:
- Connecting to each vCenter using the component credentials
- Calling `AuthorizationManager.FetchUserPrivilegeOnEntities()` to check privileges against `VSphereProviderSpec.Permissions`
- Setting conditions on CredentialsRequest to report missing privileges

This would catch configuration errors at deployment time rather than at runtime.

### Infrastructure API Extensions

A `VSpherePlatformSpec.CredentialsMode` field (Passthrough/PerComponent) and `VSpherePlatformSpec.ComponentCredentials` with secret references could be added to the Infrastructure CR. This would provide:
- A declarative API for post-installation migration to per-component credentials
- Status reporting for credential configuration
- Integration with the cluster configuration lifecycle

### Automated Credential Rotation Monitoring

CCO could monitor credential expiry and alert administrators when rotation is needed.

## Infrastructure Needed

- vSphere test environments with configurable accounts
- CI integration for per-component credential testing
- Documentation site updates for per-component setup guide
