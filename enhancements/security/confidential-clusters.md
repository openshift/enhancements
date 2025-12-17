---
title: confidential-clusters-enhancement-proposal
authors:
- "@uril"
- "@travier"
reviewers:
- "@confidential-cluster-team" # for the Confidential Cluster operator
- "@coreos-team"               # for RHCOS changes
- "TBD" # Someone from the @mco team
- "TBD" # Someone from the @installer team
approvers:
- "@sdodson"
- "TBD"
api-approvers:
- "TBD" # c.f. Confidential Cluster Operator API
creation-date: 2025-10-23
last-updated: 2025-10-23
status: implementable
tracking-link:
- "https://issues.redhat.com/browse/OCPSTRAT-2023"
- "https://issues.redhat.com/browse/OCPSTRAT-2316"
- "https://issues.redhat.com/browse/OCPSTRAT-1940"
see-also:
- "https://github.com/confidential-computing"
replaces:
- N/A
superseded-by:
- N/A
---

# OpenShift Enhancement: Confidential Clusters

## Summary

This enhancement proposes the integration of **confidential computing**
capabilities into **OpenShift cluster**, enabling the deployment of
**Confidential Clusters**. A confidential cluster is an OpenShift cluster where
all nodes run on Confidential Virtual Machines (CVMs) and are remotely attested
before they join the cluster. By leveraging CVMs, the memory for all workloads
and their management services is automatically shielded from the underlying host
infrastructure and each node disk is encrypted. This provides a foundational
layer of protection for sensitive data in memory. All nodes of the cluster are
also remotely attested to be running valid versions of RHCOS before they join
the cluster and on every boot.

## Motivation

In today's cloud-first world, organizations are increasingly migrating sensitive
workloads to public cloud environments. While cloud providers offer significant
scalability and flexibility, concerns around data confidentiality and integrity,
from the cloud provider itself or other unauthorized parties, remain a
significant barrier for highly regulated industries.

**Confidential Computing** protects data in use by processing it within a
hardware-isolated Trusted Execution Environment (TEE).
Confidential Virtual Machines (CVMs), are such TEEs, utilizing hardware
technologies like AMD SEV-SNP and Intel TDX, to strictly isolate the VM's
resources so that even the privileged hypervisor cannot view or modify the
active workload.

Trust in this environment is established via Remote Attestation.
This is a cryptographic process where the hardware generates a digitally signed
**attestation quote** proving that the CVM is running on genuine, security-
enabled hardware.
Relying parties verify this quote before releasing sensitive secrets to
the protected environment.

Confidential Clusters address these concerns by ensuring all OpenShift
nodes run on CVMs, automatically encrypting and protecting, from the host,
memory of workloads and management services as well as the content on the disk.

In Confidential Clusters, all the nodes are required to go through
remote attestation, by sending hardware signed quotes to a remote attestation
server to validate the confidential computing features enabled for the virtual
machines and to verify the version of the operating system that is booted.

Those added security layers enhance the security posture of OpenShift
deployments, making it a viable platform for even the most sensitive
applications. This enhancement proposal is meant to explain how we can integrate
confidential computing technology with OpenShift and expose this capability for
the management cluster's lifecycle.

### User Stories

Here are several scenarios where Confidential Clusters would provide immense
value:

* As a regulated company (Finance, Healthcare, etc), I want to run my
  applications and data on OpenShift in the cloud, knowing that the data in
  memory and on the disk is protected from the cloud provider and unauthorized
  access.

* As a company manager, I want to provision separated, isolated confidential
  OpenShift clusters for each department, such that strict data segregation and
  protection is maintained for their highly sensitive operations.

* As a data scientist or an AI developer, I want to run OpenShift AI workloads,
  including training models and processing proprietary datasets, while being
  confident that my data and models are protected in memory and on the disk
  throughout their lifecycle.

### Goals

* Enable the deployment of OpenShift clusters where all nodes operate as
  Confidential VMs (CVMs), minimizing exposure risk to the cloud provider or
  other unauthorized entities.

* Implement a robust remote attestation process for CVM nodes to verify their
  trustworthiness before sharing secrets and joining the cluster. The remote
  attestation process ensures that the software running on the node (kernel,
  operating system binaries, etc.) is exactly what is configured for the cluster
  and that the nodes operate in confidential mode.

* Provide a seamless integration of confidential computing and remote
  attestation from the cluster admin perspective.

* Support cluster upgrades and other lifecycle operations while preserving
  cluster confidentiality.

### Non-Goals

* This enhancement does not aim to protect from a malicious cluster operator or
  from an attacker that managed to elevate their privileges to cluster admin.

* This enhancement does not aim to provide data encryption outside of the
  confidential computing environment (for example network encryption, additional
  disk encryption), though existing OpenShift mechanisms to do that are
  available.

* This enhancement does not cover changes to application-level data
  encryption. It focuses on protecting data in memory and on the disk at the
  infrastructure layer.

* This enhancement does not address the security of the underlying cloud
  provider's hardware or hypervisor outside of the CVM's confidential execution
  environment.

## Proposal

Run all OpenShift nodes on Confidential VMs (CVMs). Use remote attestation to
verify the integrity and authenticity of a new node's hardware and software
before sharing secrets with that node and allowing it to join the cluster.

This implementation will happen in two phases.

* In the first phase, we will consider the bootstrap node and the first boot of
  each new node to be trusted. In this phase, only the confidentiality of the
  cluster will be guaranteed. We will assume the attacker can read data but not
  write data (to the disk, cloud metadata config, etc.).

* In the second phase, we will remove the need to trust the bootstrap node and
  the first boot of each node. Once completed, both confidentiality and
  integrity will be guaranteed.

We are working on a more detailed threat model, which will be submitted in a
later stage.

In the first phase (confidentiality), the following changes are needed to those
components:

* OpenShift API
  * Allow nodes to be marked as confidential. This is specific per cloud
    provider and per Hardware manufacturer.
  * Request/Instruct cloud providers to run nodes as CVMs.

* Installer
  * Allow users to specify they want to run OpenShift as a Confidential Cluster
    (cloud provider specific).
  * Deploy the Confidential Cluster Operator on the bootstrap node

* Confidential Cluster Operator
  * Setup a Trustee (attestation service) instance in the cluster to attest
    nodes.
  * Setup attestation and resource access policies in Trustee.
  * Provide a registration server for new nodes to trigger the provisioning of
    secrets.
  * Setup a MachineConfig to instruct new nodes to attest themselves.
  * Watch for cluster or OS image updates, compute and update the set of
    reference-values (expected "correct" values) in Trustee.

* RHEL CoreOS
  * Add support for composefs (native), UKI, and systemd-boot to bootc (Bootable
    Containers).
  * Build and upload disk images using UKI and systemd-boot to cloud providers.
  * Add attestation client to the operating system, such that nodes can request
    attestation and fetch secrets upon a successful attestation.
  * Add a clevis trustee pin to fetch LUKS passphrase upon a successful
    attestation and encrypt/decrypt the disk.
  * Modify Ignition to support clevis trustee pin.

In the second phase (integrity), the following changes are needed to those
components:

* Installer
  * Generate Trustee configuration and reference values to let administrators
    setup an external Trustee instance used by the bootstrap node.

* Confidential Cluster Operator
  * Support syncing secrets and reference values to an out of cluster Trustee
    instance

* RHEL CoreOS
  * Support verifying the integrity of the disk content during re-partitioning
    on first boot.
  * Set the PK/KEK/db/dbx configuration when uploading disk images to cloud
    providers
  * Modify Ignition to support fetching configs from a Trustee resource after
    remote attestation.
  * Measure Ignition config in a PCR value, before parsing it

* Machine Config Operator
  * Ensure that MachineConfigs are only served to attested nodes
  * Option: Store MachineConfigs as Trustee resources, stop serving configs via
    the MCS

* Cluster Machine Approver
  * Ensure that the logic in the CMA guarantees that only nodes passing
    attestation can get their CSR signed.

### Workflow Description

#### Cluster Administrator Workflow

The changes in the workflow for cluster creation differ based on the phase
implemented.

##### Cluster creation for the first phase

1. The cluster creator selects the Confidential Cluster option in the OpenShift
installer.
1. The rest of the installation process should not differ from the cluster
creation perspective.

##### Cluster creation for the second phase

1. The cluster creator chooses a domain name or IP which will be used to host
the initial, external, out of cluster, Trustee instance. This instance can be
hosted via a container on another system or using the Trustee operator in an
existing OpenShift cluster.
1. The cluster creator selects the Confidential Cluster option in the OpenShift
installer, passing in the URL of the external Trustee instance chosen above.
1. The OpenShift installer generates a set of configuration files for the
   external Trustee instance.
1. If the cluster creator adds/removes/modifies MachineConfigs, the
configurations above need to be re-generated again.
1. The cluster creator configures the external Trustee instance with those
   configuration files.
1. The cluster creator then resumes provisioning the cluster, starting with the
bootstrap node.
1. The cluster creator verifies that the bootstrap node has been properly
   attested.
1. The rest of the installation process should not differ from the cluster
creation perspective.

##### New node creation

The cluster administrator flow should not change when adding new nodes to the
cluster. The Confidential Cluster Operator will perform the necessary
configuration to allow new nodes to join the cluster.

##### Cluster update

The cluster administrator flow should not change when updating a cluster. The
Confidential Cluster Operator will perform the necessary configuration to allow
nodes to attest to the cluster using new version of RHCOS.

##### Shutting down and restarting Confidential Clusters

1. The cluster administrator synchronizes the policies and secrets configured in
   the Trustee instance to an external Trustee instance.
1. The cluster administrator verifies that all control plane nodes are
   configured to use the external Trustee instance as fallback in the Clevis
   Trustee PIN configuration.
1. Cluster shutdown
1. Before restarting any node, the Trustee instance must be made available at
   the domain or IP configured above.
1. The cluster administrator restarts the control plane nodes which attests
   themselves to the external Trustee instance.
1. The cluster administrator restarts the worker nodes which attests themselves
   to the internal or to the external Trustee instance.

#### User (Application Administrator) Workflow

This enhancement does not introduce any change to user workflows.

## API Extensions

This enhancement introduces some new API extensions:

* **Running nodes on cloud CVMs**:
For each supported cloud provider, confidential computing types and code need to
be added to
  * OpenShift API: types_<cloud_provider>.go
  * Cluster API: cluster-api-provider-<cloud_provider>
  * Machine API: machine-api-provider-<cloud_provider>
  * Machine API operator: add a webhook to validate confidential cluster
    configuration
  * OpenShift Installer: parse and setup confidential cluster configurations

* **ConfidentialCluster CRD**: This custom resource is used to configure the
  Confidential Cluster Operator and indirectly the Trustee instance that is used
  to attest nodes in the cluster and provide secrets.
  It is namespaced, versioned and contains:
  * TrusteeImage - the container image of Trustee attestation service
  * PcrsComputeImage - the container image for computing PCRs reference values
  * RegisterServerImage - the container image of node registration service
  * PublicTrusteeAddr - the IP address of Trustee attestation server, to be
    accessed by attesting nodes
  * TrusteeKbsPort - the port that Trustee serves on
  * RegisterServerPort - the port that the registration service serves on

* **Ignition spec changes**: The Ignition configuration specification will be
  extended to support:
  * configuring the Clevis trustee pin
  * enable fetching remote config after remote attestation

* **OpenShift CVO**: add a capability for confidential cluster
  * Disabled by default

* **Core Payload**: add Confidential Cluster Operator to Core Payload
  * It needs to be available during cluster installation
  * To be running iff the confidential-cluster CVO capability is enabled

#### Programming Languages
The Confidential Cluster Operator is written in Rust.

##### Why was Rust chosen

There are several reasons for chosing Rust, including

  * The Confidential Cluster operator is a critical element of the Confidential
    Cluster design. Any bugs in it might have a significant impact on the
    guarantees that it provides.
  * It has to be exposed to let new nodes register themselves when they come up.
    If it is unavailable, the cluster can not provision new nodes nor update
    them to a new OpenShift version.
  * It manages the reference values. If a bug enables unapproved reference
    value changes then the cluster immediately loses the guarantees of
    Confidential Computing.
  * Thus having it in Rust will reduce the risk of critical bugs and
    demonstrate to the customers that we are taking Confidential Clusters
    seriously.

##### Making Confidential Cluster Operator fit OpenShift
The following steps are to be taken to make cocl-operator fit OpenShift
  * APIs and CRDs are to be written in Go
  * When the operator is built, the APIs/CRDs are translated to Rust
  * The system openssl library is to be used

#### Integration with Openshift tooling
To make Confidential Cluster Operator a first citizen in the Openshift
echosystem, interfaces are written in Go and generated with OpenShift tools.

When the operator is built, the interfaces are converted to Rust. COPY FROM Jakob


## Topology Considerations

### Hypershift / Hosted Control Planes

Initially, this enhancement will not support a hosted control plane
topology. However, the design can be extended to support it.

In a HCP scenario, the operator will only be responsible for the worker
nodes. As the Confidential Cluster Operator will be hosted in the control plane,
the nodes hosting those services are considered part of the Trusted Computing
Base (TCB).

As HCP operators are not allowed to set up MachineConfigs, we will need an
option during HCP cluster creation to set up a MachineConfig in the control
plane and tell the Confidential Cluster Operator to use it.

### Standalone Clusters

Standalone Clusters running on cloud providers supporting confidential virtual
machines are the primary target for this enhancement.

In the future, we might want to extend the remote attestation feature of this
enhancement to be able to use it for Bare Metal OpenShift clusters to get
stronger guarantees that nodes have not been tampered with (i.e. "Attested
Clusters"). In this case, the nodes would not be running as Confidential VMs and
their memory would not be encrypted, but the guarantees around which operating
system version is used on each node and its integrity would be provided to
cluster operators.

### Single-node Deployments or MicroShift

Initially, this enhancement will not support SNO & MicroShift
deployments. However, the design can be extended to support it.

Single node deployments require an external Trustee instance to be available
when the node boots. Thus in this scenario, the Confidential Cluster operator
and the Trustee instance would be running in a management cluster responsible
for multiple SNO/MicroShift deployments. The Operator would coordinate reference
value updates in tandem with the management tools (for example ACS).

Confidential Clusters run on confidential VMs, so they require running on VMs
and on special hardware.

## Implementation Details/Notes/Constraints

### Operating system integrity and confidentiality guarantees

To guarantee the integrity of the operating system, we are adding composefs, UKI
& systemd-boot support to bootc (Bootable Containers). Unified Kernel Images
(UKI) are bundling the kernel, initrd and kernel command line into a single PE
binary that is signed for Secure Boot. Each UKI also includes the hash of the
composefs image used for the operating system, thus strongly tying a booted UKI
with a version of the operating system.

To make sure only Red Hat signed (or eventually customer signed) UKIs can be
booted, we will set the Secure Boot configuration for cloud instances to only
trust Red Hat’s (or the customer’s) Secure Boot certificates.

In order to verify that those validations effectively took place, we are using a
remote attestation process which relies on the measurements of the boot chain
components via the TPM. The measurements are stored in PCR banks which are
signed by hardware components and sent to a remote Trustee instance for
validation.

### Adding a new node to the cluster

Each node of the cluster will be started as a confidential VM. As part of the
first boot process, in the initramfs, Ignition runs and fetches its
configuration from the cloud provider instance metadata service (user-data).

In phase 1, we will trust that this configuration has not been tampered with.

In phase 2, we will measure this configuration in a PCR value before processing
it.

The initial Ignition configuration mainly consists of a directive that asks
Ignition to replace the entire configuration with the content that it will fetch
over HTTPS from the Machine Config Server (MCS).

In phase 1, this will not be changed.

In phase 2, the initial configuration will be modified to tell Ignition to fetch
the new configuration from a remotely attested resource endpoint. The MCS will
not serve Ignition configs directly for nodes anymore but will store those as
resources in a Trustee instance. To access those configurations, the node will
have to successfully remotely attest itself first.

Included in the new configuration provided by the MCS, a directive tells
Ignition to fetch an additional element of configuration from a new service: the
registration server from the Confidential Cluster Operator.

What happens in the operator as part of this registration step is described in
<https://github.com/confidential-clusters/cocl-operator/blob/main/docs/design/boot-attestation.md>.

On first boot, the content of the operating system is in clear text on the
disk. The additional configuration fetched from the registration service
includes a directive that tells Ignition to encrypt the entire root disk using
LUKS. Ignition first reads the operating system content from the disk into
memory, then re-partitions the disk, sets up LUKS and then writes back the
content in the root partition.

When setting up the keys for unlocking the LUKS device, the configuration tells
Ignition to use the Clevis Trustee Pin which fetches a resource from a Trustee
instance that is used as secret to bind the LUKS device. To access this
resource, the node must pass remote attestation successfully. To ensure that a
node can only fetch a single secret at a time, a unique identifier is provided
in the additional Ignition configuration provided by the registration server and
this value is measured in a PCR value that is validated as part of the remote
attestation process.

In phase 1, the content read from the disk will not be fully verified for
integrity.

In phase 2, the content read from the disk will be verified for integrity.

Finally, once the content of the root partition has been written back to the
disk, the system resumes booting and later joins the cluster.

If any attestation step fails, the node keeps retrying indefinitely, in turn,
each Trustee server configured. This is required as a Trustee server may be
offline at any given point in time or because the reference values accepted by
Trustee have not yet been updated by the operator or the cluster
administrator. This infinite retry loop leaves the opportunity to the cluster
operator to investigate the failure and potentially manually update the
reference values accepted for the cluster. This is similar to how Ignition
retries infinitely until an error occurs.

The remote attestation flow is demonstrated in this presentation:

* <https://cfp.all-systems-go.io/all-systems-go-2025/talk/TNKPQS/>

* <https://media.ccc.de/v/all-systems-go-2025-362-uki-composefs-and-remote-attestation-for-bootable-containers>

### Second boot

On second boot, the initrd opens the LUKS device. The LUKS device header stores
the configuration needed for the Clevis Trustee Pin to perform the request to
the Trustee servers. The response to this request is the secret needed to unlock
the LUKS device and resume booting.

### Confidential Cluster Operator

The confidential cluster operator provides two services:

* A registration service which provides individualized Ignition configs to each
  node on first boot.

* A Trustee instance which stores secrets (LUKS root keys).

For each new machine registering to the service, the operator creates a CRD that
includes a uniquely generated UUID. This UUID is given back to the new node. The
operator watches for new Machine CRDs and sets up attestation and resource
policy in the Trustee instance, and generates random secret values to be used as
LUKS root keys.

For more details about this flow, see:
<https://github.com/confidential-clusters/cocl-operator/blob/main/docs/design/boot-attestation.md>

### Cluster installation

As part of the cluster installation process in cloud platforms, a bootstrap node
is created, which hosts a temporary control plane used to create the final
control plane and worker nodes of the cluster.

In phase 1, the Confidential Cluster Operator is deployed on this bootstrap
node, which is considered trusted and it is used to bootstrap the trust for the
rest of the cluster.

In phase 2, the bootstrap node itself must be attested to establish trust. It is
thus required to set up an external Trustee instance (outside of the cluster as
it does not exist yet) that is accessible from the bootstrap node to attest
itself. In the future, key material should be fetched from this external Trustee
server instead of being passed to the bootstrap node directly.

Once the Confidential Cluster Operator is running on the bootstrap node, the
rest of the cluster is bootstrapped using the flow described above.

### Cluster update & downgrade

The Confidential Cluster Operator watches for changes in the desired OpenShift
release payload. When a new update is selected, the Confidential Cluster
Operator gets the URL/sha256 that points to the new container image (of RHCOS)
that is part of the desired release payload.

It then computes the expected PCR values for this bootable container image. It
can either read a specific LABEL from the container image where those values
have been pre-computed and stored, or pull the container image itself and
directly compute the values.

The PCR pre-calculation flow is demonstrated in this presentation:

* <https://cfp.all-systems-go.io/all-systems-go-2025/talk/TNKPQS/>

* <https://media.ccc.de/v/all-systems-go-2025-362-uki-composefs-and-remote-attestation-for-bootable-containers>

Once the new expected values have been computed, the operator updates the
reference values configured in the Trustee instance for the cluster.

Initially, we will never remove previous reference values. Thus downgrading the
version of a node will not be an issue. In the future, reference values from
older versions of the cluster will progressively be garbage collected, to
prevent downgrade attacks.

## Risks and Mitigations

* **Performance Overhead**: The memory and disk encryption used for CVMs can
  introduce a slight performance overhead. This will be mitigated by providing
  clear guidance on performance expectations for confidential workloads.

* **Cost**: CVMs require support for features only present in newer, more
  powerful CPUs, which can lead to slightly higher costs. This is a trade-off
  for enhanced security that users must accept.

* **Attestation Complexity**: To be useful and offer real security guarantees,
  the remote attestation process must be as precise as possible (i.e. we must
  measure and verify as many elements as possible from the boot chain). The more
  elements measured, the more complex the implementation. If any part of the
  verification fails during remote attestation, the node must not be able to
  boot, otherwise it would compromise the integrity of the cluster. Any mistake
  thus significantly impacts the availability of the cluster.

While the remote attestation process is complex, the role of the operator is to
manage that complexity in order to free cluster administrators from having to
manually handle setting and updating reference values.

* **Cloud Provider Dependency**: This feature relies on underlying cloud
  provider CVM capabilities. The design aims for portability where possible but
  will initially target specific cloud environments with mature CVM offerings.

* **Debugging Challenges**: Debugging on CVMs can be difficult as some
  traditional methods may fail. For example, if attestation fails in the
  initramfs phase, gathering logs may be challenging. This can be mitigated by
  providing a way to enable debugging on a CVM that is not part of the
  cluster. We are also implementing KubeVirt support as a target for development
  clusters in order to let OpenShift developers reproduce potential issues
  locally without having to deploy an entire cluster in a cloud environment.

* **Trustee / Confidential Cluster Operator availability**: If one of Trustee
  or the Operator is unavailable, nodes will not be able to boot. This can be
  mitigated by using an external (out of cluster) Trustee instance, especially
  for scenarios where the cluster is expected to be shutdown completely.

* **Security review**: This enhancement will need careful security review. We
  are working on a more detailed threat model, which will be submitted in a
  later stage.

## Drawbacks

It introduces a lot of complexity, notably for the first boot and for
updates. While we will try to hide this complexity from the cluster
administrators as much as possible, bugs can always happen and debugging will be
harder.

## Alternatives (Not Implemented)

We chose to host the Operator in the cluster in order to be able to implement
the entire PCR pre-calculation logic and include it in the operator instead of
letting users compute and manage those themselves. This should make for a better
user experience. The alternative is to instead host the Trustee instance outside
of the cluster and have the Operator be a different component outside of the
cluster. This would require users to manage reference values on cluster updates
and node creations.

## Open Questions [optional]

To be updated with incoming questions.

## Test Plan

We will need E2E tests on all supported cloud platforms.

While we don’t want to support that for production, it should also be possible
to test adding confidential nodes to a non confidential cluster where the
Confidential Cluster Operator would be running, making testing easier.

We plan to support running on KubeVirt (CNV), at least for development and
testing, using TPM only (i.e. no Confidential Computing) remote attestation
checks.  Remote attestation support will also be tested independently as part of
FCOS/RHCOS and general Image Mode / bootc testing.

## Graduation Criteria

**Note:** *Section not required until targeted at a release.*

(Section not yet filled out)

## Upgrade / Downgrade Strategy

This component will (in the end) be part of the core OpenShift payload and
updated alongside the rest of the cluster.

## Version Skew Strategy

The protocol used by the nodes attestation client and the Trustee server must
match. This means that we may have to keep multiple versions of the Trustee
instance running in parallel until the boot image is updated in the cluster.

## Operational Aspects of API Extensions

* If the Confidential Cluster Operator is not available, new nodes will fail to
  boot and join the cluster.
* If the Confidential Cluster Operator is not available, the policy and
  reference values will not be updated in the cluster and updates can not be
  performed. Manual updates will be required
* If the Trustee instance is not available, nodes will fail to attest themselves
  on boot. Configuring a backup Trustee instance mitigates this.

## Support Procedures

(Section not yet filled out)

## Infrastructure Needed [optional]

(Section not yet filled out)
