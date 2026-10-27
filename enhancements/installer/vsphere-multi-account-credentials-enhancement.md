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
last-updated: 2026-10-27
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

Analysis of OpenShift source code across vSphere integration components reveals that each component requires distinct privilege subsets. The full candidate privilege matrix, including per-object scopes, conditions, and source references, is maintained in `docs/user/vsphere/component-privileges.md` in the installer repository:

| Component | Target Role / Scope | Privileges | Current State |
|-----------|---------------------|------------|---------------|
| Machine Management | `machineManagement` | Installer provisioning matrix (installer, destroyer, CAPV, Machine API); `Host.Inventory.EditCluster` conditionally for HostGroup topology | Uses shared credentials |
| Storage | `storage` | CSI driver + CSI operator combined: CNS/storage-policy access, datastore file management, node VM disk attach/detach, conditional vSAN file-service and tagging | Uses shared credentials |
| Cloud Controller Manager | `cloudControllerManager` | Read-only inventory visibility (node VM discovery; conditional tag-based zone lookup) | Uses shared credentials |
| Problem Detector | `vsphereProblemDetector` | Read-only diagnostics (VM/ESXi/datastore/task properties, `Datastore.Browse`, conditional `StorageProfile.View`) | Uses shared credentials |

The matrix is explicitly a **review and human-test matrix, not a certified minimum role**: privileges are inferred from the vSphere API calls made by each consumer, with "candidate" (strongly suggested but unverified minimum) and "conditional" (only for installations using that operation) designations. A privilege held by one account does not satisfy a requirement of another, and a privilege on one vSphere object does not imply a grant on another unless the assignment propagates.

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

3. **Document precise privilege requirements:** Provide a candidate privilege matrix per component, organized by vSphere object scope, with conditions and a human-test checklist (`component-privileges.md`), plus conversion documentation for existing clusters.

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

1. **API Schema Extension:** `platform.vsphere.credentialType` controls whether credentials are `global` or `component-scoped`. Component-scoped credentials are protected by the `VSpherePerComponentScopedCreds` feature gate, enabled through the `DevPreviewNoUpgrade` or `CustomNoUpgrade` feature set.
2. **Component Credentials Structure:** Under `platform.vsphere.vcenters[]`, `componentCredentials` contains `machineManagement`, `storage`, `cloudControllerManager`, and `vsphereProblemDetector` credentials.
3. **Installer & CAPI Integration:** Installer-side VM operations (such as OVA validation, template cloning, and CAPI cluster asset generation) use the `machineManagement` credential.
4. **Manual Credentials Mode Secrets:** Component-scoped credentials require `credentialsMode: Manual`; the installer generates component-specific Secrets in target namespaces (`openshift-machine-api`, `openshift-machine-config-operator`, `openshift-cluster-csi-drivers`, `openshift-cloud-controller-manager`, `openshift-cluster-storage-operator`).
5. **Multi-vCenter Support:** Each component secret contains `<vcenter-server>.username` and `<vcenter-server>.password` key-value pairs for every configured vCenter.
6. **Install-Time Privilege Validation:** The installer validates each component credential against its own privilege matrix at install time, so mis-scoped accounts fail fast.

### Workflow Description

1. **vSphere Administrator** creates roles and service accounts in vCenter corresponding to each component (`machineManagement`, `storage`, `cloudControllerManager`, `vsphereProblemDetector`).
2. **Cluster Administrator** creates an `install-config.yaml` with `featureSet: DevPreviewNoUpgrade` (or `CustomNoUpgrade`), `credentialsMode: Manual`, `platform.vsphere.credentialType: component-scoped`, and `vcenters[].componentCredentials` populated for each vCenter server.
3. **OpenShift Installer** parses `install-config.yaml`, validates that the `VSpherePerComponentScopedCreds` feature gate is enabled, that `credentialsMode` is `Manual`, that the installation is not agent-based, that top-level `user`/`password` fields are omitted, and that all four component credentials are non-empty strings.
4. **Installer Pre-flight & Pre-installation** uses the `machineManagement` credential for all vCenter client interactions (e.g., verifying datacenters, datastores, networks, and cloning templates), validates the `machineManagement` privileges against the installer privilege matrix, and separately connects with the `storage`, `cloudControllerManager`, and `vsphereProblemDetector` credentials to validate each against its own component privilege matrix.
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
    // Protected by the VSpherePerComponentScopedCreds feature gate.
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

#### CAPI Cluster Assets

For installer-provisioned (CAPI-based) installations, the generated `vsphere-creds-*` identity Secrets use the `machineManagement` credential resolved via `Platform.CredentialsForVCenter(server)`.

#### UPI Installations

UPI is supported with component-scoped credentials (`docs/user/vsphere/install_upi.md`, "Installing with component-scoped credentials"). The standard UPI steps — load balancers, DNS records, VMs, and Ignition configuration — apply unchanged; only the install config and the post-install verification differ:

1. **Prerequisites:** all four component accounts must exist per configured vCenter before running the installer. The installer connects to each vCenter with the `machineManagement` account when generating the manifests (an unreachable vCenter or wrong password for that account is reported at `create manifests`). Privilege validation of all four component accounts only runs during `create cluster`, which UPI does not execute — the `storage`, `cloudControllerManager`, and `vsphereProblemDetector` accounts are first used by the cluster components themselves and must be verified after installation.
2. **Install config:** set `credentialsMode: Manual`, enable the `VSpherePerComponentScopedCreds` gate (either `featureSet: DevPreviewNoUpgrade`, or a minimal `featureSet: CustomNoUpgrade` with `featureGates: [VSpherePerComponentScopedCreds=true]`), and populate `componentCredentials` on each `vcenters` entry while omitting `user`/`password`. The gate is only read by the installer at install-config validation; nothing in the running cluster consumes it (though the feature set is rendered into the cluster's FeatureGate manifest).
3. **Provisioning the UPI VMs** uses the administrator's own vCenter credentials (for example the Terraform example's `vsphere_user`/`vsphere_password`), not any of the component accounts.
4. **Manifest generation:** `create manifests` emits the five component credential Secrets into `openshift/` (`99_vsphere-machine-api-credentials.yaml`, `99_vsphere-machine-config-operator-credentials.yaml`, `99_vsphere-csi-credentials.yaml`, `99_vsphere-cloud-controller-credentials.yaml`, `99_vsphere-problem-detector-credentials.yaml`). After the usual machineset/master-machine manifest removal and cloud-provider folder-name check, `create ignition-configs` embeds these Secrets into the bootstrap Ignition config, so the cluster comes up with its component credentials already in place and no post-installation Secret creation is needed.
5. **Credential mode:** the installer does not set the cluster's CloudCredential resource; the Cluster Version Operator creates it in the default mode (passthrough-behaving on vSphere). The cluster functions because CCO only propagates a root Secret that does not exist, but administrators should make the intended mode explicit post-install: `oc patch cloudcredential cluster --type merge -p '{"spec":{"credentialsMode":"Manual"}}'`.
6. **Verification:** confirm cluster operator health and exercise each component with its account (machine-set scale, PVC create/delete on a vSphere StorageClass, `vsphere://` node provider IDs, detector checks), as in the day-2 conversion procedure.

#### Standalone Clusters

Standalone IPI and UPI clusters on vSphere support `credentialType: component-scoped` directly via `install-config.yaml` (IPI requires `featureSet: DevPreviewNoUpgrade` or `CustomNoUpgrade` and `credentialsMode: Manual`). Agent-based installations (AI) are not supported. See "UPI Installations" under Implementation Details for the UPI-specific flow.

#### Single-node Deployments or MicroShift

For Single-Node OpenShift (SNO), per-component credentials reduce the privilege scope without affecting CPU/memory consumption. MicroShift does not use CCO or installer-generated component credential secrets.

#### OpenShift Kubernetes Engine

Compatible with OKE as it relies on standard vSphere integration components present across all OpenShift offerings.

### Implementation Details/Notes/Constraints

#### Validation and Mutual Exclusivity Rules

The installer validates `platform.vsphere` according to strict mutual exclusivity rules:

1. **`credentialType: component-scoped`**:
   - The `VSpherePerComponentScopedCreds` feature gate must be enabled (via `DevPreviewNoUpgrade` or `CustomNoUpgrade`); otherwise validation fails with a feature-gate error.
   - `credentialsMode` must be set to `Manual`; any other mode is rejected (`component-scoped credentials require credentialsMode "Manual"`).
   - Agent-based installations (AI/agent installer) are rejected (`component-scoped credentials are not supported for agent-based installations`).
   - `user` and `password` on every `vcenters[]` item **must be omitted**. Setting either results in a validation error (`must not be set when credentialType is component-scoped`).
   - `componentCredentials` on every `vcenters[]` item **is required**.
   - Inside `componentCredentials`, all four component fields (`machineManagement`, `storage`, `cloudControllerManager`, `vsphereProblemDetector`) are required, and both `user` and `password` must be non-empty strings.
   - At least one datacenter must be specified per vCenter.

2. **`credentialType: global` (or omitted)**:
   - `user` and `password` on every `vcenters[]` item **are required**.
   - `componentCredentials` on every `vcenters[]` item **must not be set**. Setting `componentCredentials` results in a validation error (`must not be set when credentialType is global`).

3. **Unknown values** of `credentialType` are rejected with a `NotSupported` error listing `global` and `component-scoped` as the supported values.

Note: `credentialsMode: Manual` is now an accepted credentials mode for the vSphere platform in install-config validation (previously only Passthrough was permitted).

#### Install-Time Component Privilege Validation

Beyond schema validation, the installer connects to each vCenter with each component credential and verifies the privileges actually held by that account against a per-component privilege matrix:

- **machineManagement:** validated against the same combined privilege matrix the installer has always required for global credentials, extended with `Host.Inventory.EditCluster` when any failure domain on that vCenter uses HostGroup zone topology (the installer and CAPV must reconfigure cluster VM groups). The HostGroup reconfiguration check only applies when the component-scoped credentials feature gate is enabled, so validation of existing global-credential install configs is unchanged.
- **storage (CSI):** `Cns.Searchable`, `StorageProfile.View` on vCenter; `Datastore.FileManagement`, `System.Read` on datastores; `Host.Config.Storage` on compute clusters; `VirtualMachine.Config.AddExistingDisk`, `VirtualMachine.Config.AddRemoveDevice` on folders (node VM access).
- **cloudControllerManager:** `System.Read` on vCenter (inventory access).
- **vsphereProblemDetector:** `System.Read`, `StorageProfile.View` on vCenter; `Datastore.Browse` on datastores.

Component privilege checks run against the objects of each failure domain topology (vCenter root folder, datacenter, datastores, compute cluster, resource pool, folder). Privileges unavailable on the target vCenter version are pruned from the check, as with the legacy matrix.

The install-time checks are a subset of the full **candidate component privilege matrix** documented in `docs/user/vsphere/component-privileges.md`. Notable details from that matrix:

- The `machineManagement` account is used by the installer, its destroyer, CAPV, and the in-cluster Machine API; the installer's validation of that account does not prove it covers every later CAPV, Machine API, or destroy operation.
- The `storage` account combines the needs of **both** the vSphere CSI driver and the OpenShift CSI operator (conditional tagging and storage-policy privileges for the managed default StorageClass) within a single account.
- The `cloudControllerManager` account needs only read visibility; upstream guidance recommends the ReadOnly role on node-VM ancestor entities. NSX permissions, if configured, are separate.
- The vSphere Problem Detector's current `CheckAccountPermissions` checks its own session against machine-installation privilege groups; a restricted detector account will produce misleading missing-machine-privilege reports until those checks are separated or redirected to the machine-management identity.
- CNS snapshot operations are not evidence for `VirtualMachine.State.*` privileges; they call CNS snapshot APIs.
- The matrix includes a review and human-test checklist: test wrong-password and wrong-scope cases, verify the identities actually delivered to CAPV (which has its own `identityRef` Secret in addition to the five component Secrets), cover multiple vCenters and non-propagating roles, and pin the shipped versions of CAPV, Machine API, CSI driver/operator, CCM, and detector before making the candidates a hard installation gate.

#### Documentation

The PR ships the following installer documentation:

- `docs/user/vsphere/customization.md` — "vCenter credential types" section: the `credentialType` model, feature-gate and `credentialsMode: Manual` requirements, a complete `componentCredentials` example, and the five generated Secrets table.
- `docs/user/vsphere/component-privileges.md` — the candidate component privilege matrix described above.
- `docs/user/vsphere/converting-to-component-credentials.md` — the day-2 conversion procedure described below.
- `docs/user/vsphere/install_upi.md` — UPI installation instructions for component-scoped credentials.

### Day-2 Conversion of Existing Clusters

Clusters installed with `global` credentials can be converted to `component-scoped` credentials as a day-2 operation performed entirely with `oc` on the running cluster (`docs/user/vsphere/converting-to-component-credentials.md`). The installer and the `VSpherePerComponentScopedCreds` feature gate are not involved; the procedure is:

1. **Discover the current configuration:** read `cloudcredential cluster` (`credentialsMode`) and the keys of the `kube-system/vsphere-creds` root Secret to determine the configured vCenter servers.
2. **Switch CCO to Manual mode:** `oc patch cloudcredential cluster --type merge -p '{"spec":{"credentialsMode":"Manual"}}'`. Ordering matters: while CCO runs in its default (passthrough-behaving) mode, it synchronizes each component Secret from the root Secret and would revert any replaced component Secret on the next sync.
3. **Remove the root Secret:** delete `kube-system/vsphere-creds` so no component can fall back to the global account.
4. **Replace the component Secrets:** recreate the five component Secrets (same names, namespaces, and account assignments as the installer-generated table above), each containing `<vcenter-server>.username`/`<vcenter-server>.password` key pairs for every configured vCenter. The Machine API and Machine Config Operator Secrets both use the `machineManagement` account.
5. **Verify:** confirm the affected ClusterOperators (`cloud-credential`, `machine-api`, `machine-config`, `storage`, `cloud-controller-manager`) report `Available=True`/`Degraded=False`; restart consuming deployments/daemonsets if mounted Secrets do not refresh promptly; exercise each component (machine-set scale, PVC create/delete on a vSphere StorageClass, `vsphere://` node provider IDs, detector checks).
6. **Retire the global account** once every component is verified.

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

When `credentialsMode: Manual` and `credentialType: component-scoped` (the required combination), the installer generates component-specific Secrets:

| Manifest Filename | Secret Name | Target Namespace | Component Credential Source |
|---|---|---|---|
| `99_vsphere-machine-api-credentials.yaml` | `vsphere-cloud-credentials` | `openshift-machine-api` | `machineManagement` |
| `99_vsphere-machine-config-operator-credentials.yaml` | `vsphere-cloud-credentials` | `openshift-machine-config-operator` | `machineManagement` |
| `99_vsphere-csi-credentials.yaml` | `vmware-vsphere-cloud-credentials` | `openshift-cluster-csi-drivers` | `storage` |
| `99_vsphere-cloud-controller-credentials.yaml` | `vsphere-cloud-credentials` | `openshift-cloud-controller-manager` | `cloudControllerManager` |
| `99_vsphere-problem-detector-credentials.yaml` | `vsphere-cloud-credentials` | `openshift-cluster-storage-operator` | `vsphereProblemDetector` |

Each Secret's data contains `<server>.username` and `<server>.password` keys per configured vCenter. Generation fails with an error if any vCenter is missing `componentCredentials` or has incomplete credentials for any component.

### Risks and Mitigations

| Risk | Mitigation |
|------|------------|
| Administrator provides insufficient privileges | Document precise candidate privilege matrix per component with conditions and a human-test checklist |
| Complex setup burden on administrators | Provide detailed documentation (credential model, privilege matrix, conversion procedure, UPI instructions) |
| Misconfiguration of component credentials | Enforce strict schema validation on `install-config.yaml` at install time, plus install-time privilege validation against per-component matrices |
| Misleading problem-detector self-privilege reports with a restricted detector account | Separate or redirect the detector's `CheckAccountPermissions` to the machine-management identity; documented as a known caveat in the privilege matrix |
| Converting a cluster while CCO is in default mode reverts component Secrets | Conversion procedure mandates switching CCO to Manual mode and deleting the root Secret before replacing component Secrets |
| Exposing secrets in `install-config.yaml` | Installer redacts component credentials from rendered install-config; enforce restricted file permissions |

### Drawbacks

1. **Increased Setup Complexity:** Administrators must create multiple accounts/roles in vCenter.
2. **Documentation Overhead:** Must maintain per-component privilege lists as vSphere features evolve.

## Alternatives (Not Implemented)

1. **CCO Account Minting Mode:** Having CCO mint vCenter accounts automatically was rejected because it requires full vCenter administrative privileges that security-conscious enterprise teams do not grant to Kubernetes workloads.
2. **Sidecar / File-based Credential File:** A separate `~/.vsphere/credentials` INI file was considered, but embedding the configuration within `install-config.yaml` under `platform.vsphere.credentialType` and `componentCredentials` keeps installation configuration declarative and consistent with other cloud platforms.

## Test Plan

- **Unit Tests:** Validate schema enums, mutual exclusivity rules, feature gate and credentialsMode requirements, agent-based rejection, `Platform.CredentialsForVCenter` credential selection, component privilege matrices, manual secret generation (including multi-vCenter key accumulation and MCO secret), CAPI credential secret generation, and password redaction.
- **Integration Tests:** Verify manifest generation for both global and component-scoped credentials in manual and automatic modes.
- **E2E Tests:** Execute full installation and day-2 cluster operations with `platform.vsphere.credentialType: component-scoped`.

## Graduation Criteria

### Dev Preview -> Tech Preview

- Implementation of `platform.vsphere.credentialType` and `componentCredentials` in `install-config.yaml`, gated behind the `VSpherePerComponentScopedCreds` feature gate (`DevPreviewNoUpgrade`).
- `credentialsMode: Manual` accepted for vSphere and required for component-scoped credentials.
- Support for `machineManagement` credentials in installer and CAPI cluster asset generation.
- Install-time privilege validation of all component credentials against per-component matrices.
- Support for generating component-scoped manual secrets in `openshift.go`, including the machine config operator secret.

### Tech Preview -> GA

- CI coverage for component-scoped vSphere installations.
- Official documentation in `openshift-docs` covering vCenter role and service account setup.

### Removing a deprecated feature

- No feature deprecation is involved; `credentialType: global` remains supported.

## Upgrade / Downgrade Strategy

- **Upgrade:** Existing clusters using global credentials continue functioning with `credentialType: global`. They can migrate to component-scoped credentials via the day-2 conversion procedure above. In Manual mode, CCO reports `Upgradeable=False` before minor version upgrades until an administrator confirms the component credentials satisfy the release being upgraded to; the acknowledgment is `oc patch cloudcredential cluster --type merge -p '{"metadata":{"annotations":{"cloudcredential.openshift.io/upgradeable-to":"<minor-version>"}}}'` after reviewing the candidate privilege matrix for privilege changes.
- **Downgrade / Rollback:** Rolling a converted cluster back to global credentials requires recreating the `kube-system/vsphere-creds` root Secret with the global account (which must still exist) and returning CCO to its default mode, after which CCO resumes passthrough propagation and overwrites the five component Secrets. Component-scoped API extensions are additive and backward-compatible for new installations.

## Version Skew Strategy

Older clients ignore `credentialType` and default to global credential handling. Component operators accept updated secrets independently during rolling updates.

### Feature Gate

`credentialType: component-scoped` is protected by the `VSpherePerComponentScopedCreds` feature gate, enabled through the `DevPreviewNoUpgrade` or `CustomNoUpgrade` feature set. Using it without the gate enabled is rejected at install-config validation. The gate requirement is also enforced during platform validation for provisioning (`ValidateForProvisioning`), which returns an explicit error naming the gate.

## Operational Aspects of API Extensions

- **SLI Impact:** No performance or API throughput impact.
- **Failure Modes:** If component credentials have invalid permissions, the corresponding component operator logs authentication or permission errors.

## Support Procedures

- **Diagnosis:** Inspect target Secrets in `openshift-machine-api`, `openshift-machine-config-operator`, `openshift-cluster-csi-drivers`, `openshift-cloud-controller-manager`, and `openshift-cluster-storage-operator`.
- **Remediation:** Update the relevant Secret with valid credentials for the failing component.
