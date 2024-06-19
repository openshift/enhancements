---
title: subprovisioner-csi-driver-integration-into-lvms
authors:
  - "@jakobmoellerdev"
reviewers:
  - "CNV Team"
  - "LVMS Team"
approvers:
  - "@DanielFroehlich"
  - "@jerpeter1"
  - "@suleymanakbas91"
api-approvers:
  - "@DanielFroehlich"
  - "@jerpeter1"
  - "@suleymanakbas91"
creation-date: 2024-05-02
last-updated: 2024-05-02
status: discovery
tracking-link:
  - https://issues.redhat.com/browse/OCPEDGE-1147
---

# Subprovisioner CSI Driver Integration into LVMS

[Subprovisioner](https://gitlab.com/subprovisioner/subprovisioner) 
is a CSI plugin for Kubernetes that enables you to provision Block volumes 
backed by a single, cluster-wide, shared block device (e.g., a single big LUN on a SAN).

Logical Volume Manager Storage (LVMS) uses the TopoLVM CSI driver to dynamically provision local storage on the OpenShift Container Platform clusters.

This proposal is about integrating the Subprovisioner CSI driver into the LVMS operator to enable the provisioning of 
shared block devices on the OpenShift Container Platform clusters. 

This enhancement will significantly increase scope of LVMS, but allows LVMS to gain the unique value proposition
of serving as a valid layered operator that offers LUN synchronization and provisioning capabilities.

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

This is a proposal to
- Create an enhancement to the "LVMCluster" CRD that is able to differentiate a deviceClass into a new
  type of shared storage that can be provisioned side-by-side or in alternative to regular LVMS device-classes managed by TopoLVM.
- Create a productization for a LUN-backed CSI driver alternative to TopoLVM that allows for shared vg usage, especially in the context of virtualization.


## Motivation

TopoLVM as our existing in-tree driver of LVMS is a great solution for local storage provisioning, but it lacks the ability to provision shared storage.
This is a significant limitation for virtualization workloads that require shared storage for their VMs that can dynamically be provisioned and deprovisioned 
on multiple nodes. Since OCP 4.15, LVMS support Multi-Node Deployments as a Topology, but without Replication or inbuilt resiliency behavior.

The Subprovisioner CSI driver is a great solution for shared storage provisioning, but it is currently not productized as part of OpenShift Container Platform.

### Goals

- Extension of the LVMCluster CRD to support a new deviceClass policy field that can be used to provision shared storage via Subprovisioner.
- Find a way to productize the Subprovisioner CSI driver as part of OpenShift Container Platform and increasing the Value Proposition of LVMS.
- Allow provisioning of regular TopoLVM deviceClasses and shared storage deviceClasses side-by-side in the same cluster.

### Non-Goals

- Compatibility with other CSI drivers than Subprovisioner. 
- Switching the default CSI driver for LVMS from TopoLVM to Subprovisioner or the other way around.
- Implementing a new CSI driver from scratch.
- Integrating the Subprovisioner CSI driver into TopoLVM.

### User Stories

As a Data Center OCP Admin:
- I want to seamlessly add my existing SAN infrastructure to OCP nodes to host VM workloads, enabling better (live) migration of VMs from vSphere to OCP Virt and from one OCP node to another.
- I want to provision shared block storage across multiple nodes, ensuring high availability and resiliency for my virtualization workloads.
- I want to manage both local and shared storage within the same OpenShift cluster to optimize resource utilization and simplify storage management.

As a Developer:
- I want to deploy applications that require shared storage across multiple pods and nodes, ensuring data consistency and high availability.
- I want to use a single, unified API to provision and manage both local and shared storage classes, reducing complexity in my deployment scripts.
- I want to benefit from the unique capabilities of Subprovisioner for shared storage without having to manage separate storage solutions, both TopoLVM and Subprovisioner use lvm2 under the hood.

As a Storage Administrator:
- I want to easily configure and manage volume groups using the new deviceClass policy field in the LVMCluster CRD, ensuring that my storage setup is consistent and efficient.
- I want to monitor the health and status of my volume groups, receiving alerts and logs for any issues that arise with the shared storage.
- I want to leverage existing expensive SAN infrastructure to provide shared storage, maximizing the return on investment for our hardware.

As an IT Operations Engineer:
- I want to ensure that upgrades and downgrades of the LVMS operator and Subprovisioner CSI driver are seamless and do not cause downtime for my existing workloads.
- I want to follow clear guidelines and best practices for managing version skew between LVMS and Subprovisioner, ensuring compatibility and stability.
- I want detailed documentation and troubleshooting guides to help resolve any issues that arise during the deployment and operation of shared storage.

As a Quality Assurance Engineer:
- I want to execute comprehensive integration and end-to-end tests that validate the functionality of shared storage provisioning with Subprovisioner.
- I want to conduct performance and stress tests to ensure that the solution can handle high load and failure conditions without degradation of service.
- I want to gather and analyze feedback from early adopters to improve the stability and performance of the integrated solution before general availability.

As a Product Manager:
- I want to offer a unique value proposition with LVMS by integrating Subprovisioner, enabling OCP customers to use shared block storage seamlessly.
- I want to ensure that the solution meets the needs of our enterprise customers, providing high availability, resiliency, and performance for their critical workloads.
- I want to manage the roadmap and release cycles effectively, ensuring that each phase of the project is delivered on time and meets quality standards.

### Risks and Mitigations
- There is a risk of increased maintenance burden by integrating a new CSI driver into LVMS without gaining traction
  - tested separately in the Subprovisioner project as pure CSI Driver similar to TopoLVM and within LVMS with help of QE
    - we will not GA the solution until we have a clear understanding of the maintenance burden. The solution will stay in TechPreview until then.
- There is a risk that Subprovisioner is so different from TopoLVM that behavior changes can not be accomodated in the current CRD
  - we will scrap this effort for integration and look for alternative solutions if the integration is not possible with reasonable effort.
- There is a risk that Subprovisioner will break easily as its a really young project
  - we will not GA the solution until we have a clear understanding of the stability of the Subprovisioner project. The solution will stay in TechPreview until then.

## Proposal

The proposal is to extend the LVMCluster CRD with a new deviceClass policy field that can be used to provision shared storage via Subprovisioner.
We will use this field as a hook in lvm-operator, our orchestrating operator, to provision shared storage via Subprovisioner instead of TopoLVM.
Whenever LVMCluster discovers a new deviceClass with the Subprovisioner associated policy, it will create a new CSI driver deployment for Subprovisioner and configure it to use the shared storage deviceClass.
As such, it will handover the provisioning of shared storage to the Subprovisioner CSI driver. Also internal engineering such as sanlock orchestration will be managed by the driver.

### Workflow Description

#### Subprovisioner instantiation via LVMCluster

1. The user is informed of the intended use case of Subprovisioner, and decides to use it for its multi-node capabilities before provisioning Storage
2. The user configures LVMCluster with non-default values for the Volume Group and the deviceClass policy field
3. The lvm-operator detects the new deviceClass policy field and creates a new CSI driver deployment for Subprovisioner.
4. The Subprovisioner CSI driver is configured to use the shared storage deviceClass, initializes the global lock space, and starts provisioning shared storage.
5. The user can now provision shared storage via Subprovisioner on the OpenShift Container Platform cluster.
6. The user can also provision regular TopoLVM deviceClasses side-by-side with shared storage deviceClasses in the same cluster. Then, TopoLVM gets provisioned side-by-side.

### API Extensions

#### Design Details for `LVMCluster CR extension`

API scheme for `LVMCluster` CR:

```go

+ // The DeviceAccessPolicy type defines the accessibility of the lvm2 volume group backing the deviceClass. 
+ type DeviceAccessPolicy string
  
+ const (
+   DeviceAccessPolicyShared DeviceAccessPolicy = "shared"
+   DeviceAccessPolicyNodeLocal DeviceAccessPolicy = "nodeLocal"
+ )

  // LVMClusterSpec defines the desired state of LVMCluster
  type LVMClusterSpec struct {
    // Important: Run "make" to regenerate code after modifying this file
    
    // Tolerations applied to CSI driver pods
    // +optional
    Tolerations []corev1.Toleration `json:"tolerations,omitempty"`
    // Storage describes the deviceClass configuration for local storage devices
    // +Optional
    Storage Storage `json:"storage,omitempty"`
  }
  type Storage struct {
    // DeviceClasses are a rules that assign local storage devices to volumegroups that are used for creating lvm based PVs
    // +Optional
    DeviceClasses []DeviceClass `json:"deviceClasses,omitempty"`
  }
  
  type DeviceClass struct {
    // Name of the class, the VG and possibly the storageclass.
    // Validations to confirm that this field can be used as metadata.name field in storageclass
    // ref: https://github.com/kubernetes/apimachinery/blob/de7147/pkg/util/validation/validation.go#L209
    // +kubebuilder:validation:MaxLength=245
    // +kubebuilder:validation:MinLength=1
    // +kubebuilder:validation:Pattern="^[a-z0-9]([-a-z0-9]*[a-z0-9])?$"
    Name string `json:"name,omitempty"`
    
    // DeviceSelector is a set of rules that should match for a device to be included in the LVMCluster
    // +optional
    DeviceSelector *DeviceSelector `json:"deviceSelector,omitempty"`
    
    // NodeSelector chooses nodes on which to create the deviceclass
    // +optional
    NodeSelector *corev1.NodeSelector `json:"nodeSelector,omitempty"`
    
    // ThinPoolConfig contains configurations for the thin-pool. 
+   // MUST NOT be set for shared deviceClasses.
    // +optional
    ThinPoolConfig *ThinPoolConfig `json:"thinPoolConfig,omitempty"`
    
    // Default is a flag to indicate whether the device-class is the default.
    // This will mark the storageClass as default.
    // +optional
    Default bool `json:"default,omitempty"`
    
    // FilesystemType sets the filesystem the device should use.
    // For shared deviceClasses, this field must be set to "" or none.
    // +kubebuilder:validation:Enum=xfs;ext4;none;""
    // +kubebuilder:default=xfs
    // +optional
    FilesystemType DeviceFilesystemType `json:"fstype,omitempty"`
    
+   // Policy defines the policy for the deviceClass.
+   // TECH PREVIEW: shared will allow accessing the deviceClass from multiple nodes.
+   // The deviceClass will then be configured via shared volume group.
+   // +optional	  
+   // +kubebuilder:validation:Enum=shared;local
+   DeviceAccessPolicy DeviceAccessPolicy `json:"deviceAccessPolicy,omitempty"`
  }

  type ThinPoolConfig struct {
    // Name of the thin pool to be created. Will only be used for node-local storage, 
    // since shared volume groups will create a thin pool with the same name as the volume group.
    // +kubebuilder:validation:Required
    // +required
    Name string `json:"name"`
    
    // SizePercent represents percentage of space in the volume group that should be used
    // for creating the thin pool.
    // +kubebuilder:default=90
    // +kubebuilder:validation:Minimum=10
    // +kubebuilder:validation:Maximum=90
    SizePercent int `json:"sizePercent,omitempty"`
    
    // OverProvisionRatio is the factor by which additional storage can be provisioned compared to
    // the available storage in the thin pool. Only applicable for node-local storage.
    // +kubebuilder:validation:Minimum=1
    // +kubebuilder:validation:Required
    // +required
    OverprovisionRatio int `json:"overprovisionRatio"`
  }

  // DeviceSelector specifies the list of criteria that have to match before a device is assigned
  type DeviceSelector struct {
    // A list of device paths which would be chosen for creating Volume Group.
    // For example "/dev/disk/by-path/pci-0000:04:00.0-nvme-1"
    // We discourage using the device names as they can change over node restarts.
+   // For multiple nodes, all paths MUST be present on all nodes.
    // +optional
    Paths []string `json:"paths,omitempty"`
  
    // A list of device paths which could be chosen for creating Volume Group.
    // For example "/dev/disk/by-path/pci-0000:04:00.0-nvme-1"
    // We discourage using the device names as they can change over node restarts.
+	// For multiple nodes, all paths SHOULD be present on all nodes.
    // +optional
    OptionalPaths []string `json:"optionalPaths,omitempty"`
  
    // ForceWipeDevicesAndDestroyAllData runs wipefs to wipe the devices.
    // This can lead to data lose. Enable this only when you know that the disk
    // does not contain any important data.
    // +optional
    ForceWipeDevicesAndDestroyAllData *bool `json:"forceWipeDevicesAndDestroyAllData,omitempty"`
  }
```

### Implementation Details/Notes/Constraints

#### Design Details on Volume Group Orchestration and Management via vgmanager

The `vgmanager` component will be responsible for managing volume groups (VGs) and coordinating the orchestration between TopoLVM and Subprovisioner CSI drivers. This includes:

1. **Detection and Configuration**:
  - Detecting devices that match the `DeviceSelector` criteria specified in the `LVMCluster` CR.
  - Configuring volume groups based on the `DeviceAccessPolicy` (either `shared` for Subprovisioner or `local` for TopoLVM).
  - Ensuring that shared volume groups are correctly initialized and managed across multiple nodes.

2. **Dynamic Provisioning**:
  - Creating and managing VGs dynamically based on incoming requests and the policy defined in the CR.
  - For shared deviceClasses, ensure that the VG is accessible and consistent across all nodes in the cluster.
  - For shared volume groups mandated by a shared deviceClass, the VG will be created in shared mode and a SAN lock might need to be initialized
  
3. **Monitoring and Maintenance**:
  - Continuously monitor the health and status of the VGs.
  - Handling any required maintenance tasks, such as resizing, repairing, or migrating VGs must be performed manually for shared Volume Groups.

4. **Synchronization**:
  - Ensure synchronization mechanisms (such as locks) are in place for shared VGs to prevent data corruption and ensure consistency.
  - Utilize `sanlock` or similar technologies to manage and synchronize access to shared storage at all times.
  - For SAN lock initialization, a race-free initialization of the lock space will be required. This can be achieved by using a Lease Object,
    which is a Kubernetes object that can be used to coordinate distributed systems. The Lease Object will be used to ensure that only one node
    can initialize the lock space at a time. The Lease will be owned on a first-come-first-serve basis, and the node that acquires the Lease will
    will be used for shared lockspace initialization. [A sample implementation can be found here](https://github.com/openshift/lvm-operator/commit/8ba6307c7bcaccc02953e0e2bdad5528636d5e2d)


#### Design Details for Status Reporting

The status reporting will include:

1. **VG Status**:
  - Report the health and state of each VG managed by `vgmanager`.
  - Include details such as size, available capacity, and any errors or warnings.
  - Health reporting per node is still mandatory.

2. **Node-Specific Information**:
  - Report node-specific information related to the VGs, such as which nodes have access to shared VGs.
  - Include status of node-local VGs and any issues detected.

3. **CSI Driver Status**:
  - Provide status updates on the CSI drivers (both TopoLVM and Subprovisioner) deployed in the cluster.
  - Include information on driver health, performance metrics, and any incidents.
  - Ideally, subprovisioner implements Volume Health Monitoring CSI calls.

4. **Event Logging**:
  - Maintain detailed logs of all events related to VG management and CSI driver operations.
  - Ensure that any significant events (such as failovers, recoveries, and maintenance actions) are logged and reported.


### Drawbacks

- Increased complexity in managing both node-local and shared storage.
- Potential for increased maintenance burden with the integration of a new CSI driver.
- Risks associated with the stability and maturity of the Subprovisioner project.
- Complex testing matrix and shared volume group use cases can be hard to debug / troubleshoot.

### Topology Considerations

* The primary use case for Subprovisioner is to enable shared storage across multiple nodes. This capability is critical for environments where high availability and data redundancy are required.
* Ensure that all nodes in the cluster can access the shared storage devices consistently and reliably. This may involve configuring network settings and storage paths appropriately.

#### Hypershift / Hosted Control Planes

N/A

#### Standalone Clusters

LVMS can be installed on standalone clusters, but the shared storage provisioning will only work in a multi-node environment.

#### Single-node Deployments or MicroShift

* While LVMS can be installed on single-node deployments and MicroShift, the shared storage provisioning feature enabled by Subprovisioner is designed for multi-node environments. Single-node setups can still use local storage provisioning through TopoLVM.
* MicroShift deployments will include the Subprovisioner binaries but will not use shared storage provisioning due to the single-node nature of MicroShift.

## Test Plan

- **Integration Tests**:
  - Update existing LVMS integration tests to include scenarios for shared storage provisioning with Subprovisioner.
  - Ensure that device detection and VG management are functioning correctly with both TopoLVM and Subprovisioner.
  - QE will be extending the existing test suites to include shared storage provisioning and synchronization tests.

- **E2E Tests**:
  - Implement end-to-end tests to validate the complete workflow from device discovery to VG provisioning and usage.
  - Include multi-node scenarios to test shared storage provisioning and synchronization.

- **Performance and Stress Tests**:
  - Conduct performance tests to assess the scalability and robustness of the VG management and CSI driver operations.
  - The performance tests will have the same scope as the existing TopoLVM performance tests, mainly provisioning times and I/O
  - Perform stress tests to evaluate system behavior under high load and failure conditions.
  - We will run these tests before any graduation to GA at the minimum.

## Graduation Criteria

### Dev Preview -> Tech Preview

- **Developer Preview (Early Evaluation and Feedback)**:
  - Initial implementation with basic functionality for shared and node-local VG provisioning.
  - Basic integration and E2E tests in place.
  - Feedback from early adopters and stakeholders collected.
  - No official Product Support.
  - Functionality is provided with very limited, if any, documentation. Documentation is not included as part of the product’s documentation set.

- **Technology Preview**:
  - Feature-complete implementation with all planned functionality.
  - Comprehensive test coverage including performance and stress tests.
  - Functionality is documented as part of the products documentation set (on the Red Hat Customer Portal) and/or via the release notes.
  - Functionality is provided with LIMITED support by Red Hat. Customers can open support cases, file bugs, and request feature enhancements. However, support is provided with NO commercial SLA and no commitment to implement any changes.
  - Functionality has undergone more complete Red Hat testing for the configurations supported by the underlying product.
  - Functionality is, with rare exceptions, on Red Hat’s product roadmap for a future release.

### Tech Preview -> GA

- **GA**:
  - Proven stability and performance in production-like environments.
  - Positive feedback from initial users.
  - Full documentation, including troubleshooting guides and best practices.
  - Full LVMS Support Lifecycle

### Removing a deprecated feature

N/A

## Upgrade / Downgrade Strategy

- **Upgrade**:
  - Ensure that upgrades are seamless with no downtime for existing workloads. Migrating to a subprovisioner enabled version is a no-break operation
  - Test upgrade paths thoroughly to ensure compatibility and data integrity. The subprovisioner to topolvm (or vice versa) switch should be excluded and forbidden explicitly.
  - The "default" deviceClass cannot be changed as well and changeing from shared to local or vice versa is not supported without resetting the LVMCluster.
  - New deviceClasses with the shared policy should be able to be added to existing LVMClusters without affecting existing deviceClasses.

- **Downgrade**:
  - Allow safe downgrades by maintaining backward compatibility. Downgrading from a subprovisioner enabled version to a purely topolvm enabled version should be a no-break operation for the topolvm part. For the subprovisioner part, the operator should ensure that the shared VGs can be cleaned up manually
  - Provide rollback mechanisms and detailed instructions to revert to previous versions. Ensure that downgrades do not result in data loss or service interruptions.
    The operator should ensure that the shared VGs can be cleaned up manually.
  - Ensure that downgrades do not result in data loss or service interruptions. The operator should ensure that the shared VGs can be cleaned up without data loss on other device classes.

## Version Skew Strategy

- Ensure compatibility between different versions of LVMS and the integrated Subprovisioner CSI driver.
  - Implement version checks and compatibility checks in the `vgmanager` component.
  - Ensure that the operator can handle version skew between the LVMS operator and the Subprovisioner CSI driver where required.
  - Provide clear guidelines on how to manage version skew and perform upgrades in a controlled manner.
  - One version of LVMS should be able to handle one version of the Subprovisioner CSI driver.
- Document supported version combinations and any known issues with version mismatches.
- Provide clear guidelines on how to manage version skew and perform upgrades in a controlled manner.

## Operational Aspects of API Extensions

The integration of the Subprovisioner CSI driver into LVMS introduces several new API extensions, primarily within the LVMCluster CRD. These extensions include new fields for the deviceClass policy, specifically designed to support shared storage provisioning. The operational aspects of these API extensions are as follows:

* Configuration and Management:
  * Administrators can configure shared storage by setting the DeviceAccessPolicy field in the DeviceClass section of the LVMCluster CRD to shared.
  * The API ensures that only valid configurations are accepted, providing clear error messages for any misconfigurations, such as setting a filesystem type for shared device classes.

* Validation and Enforcement:
  * The operator will enforce constraints on shared storage configurations, such as requiring shared volume groups to use the shared policy and prohibiting thin pool configurations.
  * The vgmanager component will validate device paths and ensure that they are consistent across all nodes in the cluster.

* Dynamic Provisioning:
  * When a shared device class is configured, the operator will dynamically create and manage the corresponding Subprovisioner CSI driver deployment, ensuring that the shared storage is properly initialized and synchronized across nodes.

Monitoring and Reporting:
  * The status of the shared storage, including health and capacity metrics, will be reported through the LVMCluster CRD status fields.
  * Node-specific information and events related to the shared storage will be logged and made available for troubleshooting and auditing purposes.

## Support Procedures

Regular product support for LVMS will continue to be established through the LVMS team. In addition, Subprovisioner will receive upstream issues through consumption in the LVMS project and will serve as a repackaging customer for the Subprovisioner project.

## Security Considerations

- **Access Control**:
  - Ensure that access to shared storage is controlled and restricted to authorized users. Node-level access control should be enforced similarly to TopoLVM.
  - Implement RBAC policies to restrict access to VGs and CSI drivers based on user roles and permissions.
  - Ensure that shared VGs are only accessible by nodes that are authorized to access them.
- **CVE Scanning**:
  - Ensure that the Subprovisioner CSI driver is regularly scanned for vulnerabilities and that any identified issues are addressed promptly.
  - Implement a process for CVE scanning and remediation for the Subprovisioner CSI driver.
  - Fixes for CVEs should be handled in a dedicated midstream openshift/subprovisioner for critical CVEs when Red Hat decides to no longer solely own the project. Until then, the fixes will be handled by the Red Hat team and a midstream is optional.

## Implementation Milestones

- **Phase 1**: Initial design and prototyping. Basic integration with Subprovisioner and updates to the LVMCluster CR.
- **Phase 2**: Development of `vgmanager` functionalities for VG orchestration and management. Integration and E2E testing.
- **Phase 3**: Performance testing, bug fixes, and documentation. Preparing for Alpha release.
- **Phase 4**: Developer Preview release with comprehensive manual and QE testing. Gathering user feedback and making improvements.
- **Phase 5**: Technology PReview with Documentation Extension and preparation of GA.
- **Phase 6**: General Availability (GA) release with proven stability and performance in production environments.


## Alternatives

- Continue using TopoLVM exclusively for local storage provisioning.
- Evaluate and integrate other CSI drivers that support shared storage.
- Develop a custom CSI driver to meet the specific needs of LVMS and OpenShift.
- Move Subprovisioner to CNV and package it in a separate product.
