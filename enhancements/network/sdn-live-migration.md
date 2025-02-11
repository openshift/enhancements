---
title: sdn-live-migration
authors:
  - "@pliurh"
reviewers:
  - "@danwinship"
  - "@trozet"
  - "@dcbw"
  - "@russellb"
approvers:
  - "@danwinship"
  - "@trozet"
  - "@dcbw"
  - "@russellb"
api-approvers:
  - "@danwinship"
  - "@trozet"
  - "@dcbw"
  - "@russellb"
creation-date: 2022-03-18
last-updated: 2023-09-25
tracking-link:
  - https://issues.redhat.com/browse/SDN-2612
status: implementable
---

# SDN Live Migration

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)


## Summary
Migrating the CNI network provider network of a running cluster from
OpenShift SDN to OVN Kubernetes without service interruption. During the
migration, we will partition the cluster into two sets of nodes controlled by
different network plugins. We will utilize the Hybrid overlay feature of
OVN Kubernetes to connect the networks of the two CNI network plugins. So that
pods on each side can still talk to pods on the other side.

## Motivation

For some Openshift users, they have very high requirements on service
availability. The current SDN migration solution, which will cause a service
interruption, is not acceptable.

### Goals

- Migrate the cluster network provider from OpenShift SDN to OVN Kubernetes for
  an existing cluster.
  - The OVN Kubernetes will be running in the IC multi-zone mode.
- This solution will work on all platforms managed by the SD team.
  - ARO (Azure Red Hat OpenShift)
  - OSDv4 (OpenShift Dedicated) on GCP
  - OSDv4 (OpenShift Dedicated) on AWS
  - ROSA Classic (Red Hat OpenShift Service on AWS)
- This is an in-place migration without requiring extra nodes.
- The impact on workload of the migration will be similar to that of an OCP
  upgrade.
- The solution can work at scale, e.g., in a large cluster with 100+ nodes.
- The migration operation will be able to be rolled back if needed.
- The migration is fully automated with zero human interaction
- The migration shall be triggered by a declarative network API
- There shall be failure indicator and corresponding documentation on what to do
  next.

### Non-Goals

- Support for migration to other network providers
- The necessary GUI change in Openshift Cluster Manager
- Ensure the following features remain working during a live migration:
  - Egress IP
  - Egress Router
  - Multicast
- Migration from SDN Multi-tenant mode
- Support Hypershift clusters

## Pre-requisites

- OVN-IC multi-zone mode in OCP is dev-complete.

## Proposal

The key problem with doing a live SDN migration is that we need to maintain the
connectivity of the cluster network during the migration when pods are attached
to different networks. We propose to utilize the OVN Kubernetes hybrid overlay
feature to connect the networks owned by OpenShift SDN and OVN Kubernetes.

- We will run different plugins on different nodes, but both plugins will know
  how to reach pods owned by the other plugin, so all pods, services, etc.
  remain connected.
- During migration, CNO will take original-plugin nodes one by one and convert
  them to destination-plugin nodes, rebooting them in the process.
- The cluster network CIDR will remain unchanged, as will the node host subnet
  of each node.
- NetworkPolicy will work correctly throughout the migration.

### Limitations

- The following features, which are only supported by OpenShift SDN but not by
  OVN Kubernetes, will stop working when the migration is started.
  - Multitenant Isolation
- The Egress Router feature is supported by both OpenShift SDN and OVN
  Kubernetes, but with different designs and implementations. Therefore, even
  with the same name, it has different APIs and configuration logic between
  OpenShift SDN and OVN Kubernetes. Also, the supported platforms and modes are
  different between the two network providers. So users need to evaluate before
  conducting the live migration and have to migrate the configuration manually
  after the migration is complete.
- We have supported automated conversion for the configuration of the following
  features. When the migration is done, these features will remain functional.
  - Multicast
  - Egress IP
  - Egress Firewall
- During the migration, when the cluster is running with both OVN Kubernetes and
  OpenShift SDN,
  - Multicast and Egress IP will be temporarily disabled for both CNIs.
  - Egress Firewall shall remain functional, as the implementation of this
    feature is per-node-based in both CNIs.

### User Stories

The service delivery (SD) team (which manages OpenShift services ARO, OSD, ROSA)
has a unique set of requirements around downtime, node reboots, and a high
degree of automation. Specifically, SD needs a way to migrate their managed
fleet in a way that is no more impactful to the customer's workloads than an OCP
upgrade and that can be done at scale in a safe, automated way that can be made
self-service and not require SD to negotiate maintenance windows with customers.
The current migration solution needs to be revisited to support these
(relatively) more stringent requirements.

### Risks and Mitigations

## Design Details

The existing OVN Kubernetes hybrid overlay feature was developed for hybrid
Windows/Linux clusters. Each OVN Kubernetes node manages an external-to-OVN OVS
bridge, named br-ext, which acts as the VXLAN source and endpoint for packets
moving between pods on the node and their cluster-external destination. The
br-ext SDN switch acts as a transparent gateway and routes traffic towards
Windows nodes.

In the SDN live migration use case, we can enhance this feature to connect the
nodes managed by different CNI plugins. To minimize the implementation effort
and maintainability of the code, we will try to reuse the hybrid overlay code
and only make necessary changes to both CNI plugins. OVN Kubernetes nodes
(OVN-IC zones) will have full-mesh connections to all OpenShift SDN Nodes
through VXLAN tunnel.

On the OVN Kubernetes side, all the cross-CNI traffic shall follow the same path as the current hybrid overlay implementation. For OVN Kubernetes, we need to make the following enhancements:

1. We need to prevent OVN Kubernetes from allocating a subnet for each host. We
   need to reuse the host subnet allocated by OpenShift SDN.
2. We need to modify OVN Kubernetes to allow overlapping between the cluster
   network and the Hybrid overlay CIDR.
3. OVN Kubernetes shall be able to handle adding/removing hybrid overlay nodes
   on the fly.
4. We need to allow `hybrid-overlay-node` to run on the Linux nodes using
   OpenShift SDN as the CNI plugin; currently, it is designed to only run on
   Windows nodes. It is responsible for:
   - collecting the MAC address of the host primary interface and setting the
    node annotation `k8s.ovn.org/hybrid-overlay-distributed-router-gateway-mac`.
   - removing the pod annotations added by OVN Kubernetes for pods running on
    the local node.

On the OpenShift SDN side, when a node is converted to OVN Kubernetes, it will
be almost transparent to the control-plane of OpenShift SDN. But we still need
to introduce a 'migration mode' for OpenShift SDN by:

1. Change ingress NetworkPolicy processing to be based entirely on pod IPs
   rather than using namespace VNIDs, since packets from OVN nodes will have the
   VNID 0 set.
2. To be compatible with Windows node VXLAN implementation, OVN Kubernetes
   hybrid overlay uses the peer host interface MAC as the VXLAN inner dest MAC
   for egress traffic. Therefore, when packets arrive at the br0 of the SDN
   node, they cannot be forwarded to the pod interface correctly. We need to add
   a flow to change the dst MAC to the pod interface MAC for such ingress
   traffic.

### MTU Consideration

There are two related but distinct MTU values to consider: the hardware MTU and
the cluster MTU. The cluster MTU is the MTU value for pod interfaces. It is
always less than your hardware MTU to account for the cluster network overlay
overhead. The overhead is 100 bytes for OVN Kubernetes and 50 bytes for SDN. For
example, if the hardware MTU is 1500, then the auto-probed cluster MTU would be
1400 for OVN Kubernetes and 1450 for OpenShift SDN. During the migration, this
MTU mismatch would break some cross-CNI traffic.

To resolve this issue, during the live migration process, CNO will update the
routable MTU to make sure the 2 CNI shares the same overlay MTU.

### The Traffic Path

#### Packets going from OpenShift SDN to OVN Kubernetes

On the SDN side, it doesn't need to know if the peer node is a SDN node or an
OVN node. We reuse the existing VXLAN tunnel rules on the SDN side.
- Egress NetworkPolicy rules and service proxy happen as normal.
- When the packet reaches table 90, it will hit a "send via vxlan" rule that was
  generated based on a HostSubnet object.

On the OVN side:
- OVN accepts the packet via the VXLAN tunnel, ignores the VNID set by SDN, and
  then just routes it normally.
- Ingress NetworkPolicy processing will happen when the packet reaches the
  destination pod's switch port, just like normal.
  - Our NetworkPolicy rules are all based on IP addresses, not "logical input
    port", etc., so it doesn't matter that the packets came from outside OVN and
    have no useful OVN metadata.

#### Packets going from OVN Kubernetes to OpenShift SDN

On the OVN side:
- The packet just follows the same path as the hybrid overlay.

On the SDN side:
- We have to change ingress NetworkPolicy processing to be based entirely on pod
  IPs rather than using namespace VNIDs since packets from OVN nodes won't have
  the VNID set. There is already code to generate the rules that way, though,
  because egress NetworkPolicy already works that way.

### Workflow Description

1. The admin kicks off the migration process by updating the custom resource
   `network.config`:
   
   ```bash
   $ oc patch Network.config.openshift.io cluster --type='merge' --patch '{"metadata":{"annotations":{"network.openshift.io/network-type-migration":""}},"spec":{"networkType":"OVNKubernetes"}}'
   ```
   
   CNO will set the `NetworkTypeMigrationInProgress` condition to TRUE in the
   `status` of the network.config CR. If there are any unsupported features
   (refer to the [Limitations](#limitations) section) enabled in the cluster,
   CNO will set an the `NetworkTypeMigrationInProgress` condition to `False`
   with warning message and won't start the live migration. User shall disable
   these unsupported features before starting the live migration.

2. CNO will check the current hardware MTU and the cluster MTU to determine a
   routable MTU during the migration. e.g.
   - If the current hardware MTU is 1500, and the cluster MTU is not specified
     explicitly. CNO will set `.spec.migration.mtu` of network.operator CR with
     MTU of OVN-K, 
     - `.spec.migration.mtu.network.from` 1450 `to` 1400
     - `.spec.migration.mtu.machine.from` 1500 `to` 1500
   - If the current hardware MTU is 9000, and the cluster MTU is specified
     explicitly to 1250 for OpenShiftSDN. CNO will set `.spec.migration.mtu` of
     network.operator CR with MTU of OVN-K,
     - `.spec.migration.mtu.network.from` 1250 `to` 1200
     - `.spec.migration.mtu.machine.from` 9000 `to` 9000
   
   This will trigger MCO to apply a new machine config to each MCP and reboots
   all the nodes.

3. CNO watches the MCP, wait until the MCs with routable MTU is applied to all
   the nodes. CNO then sets the `NetworkTypeMigrationMTUReady`
   condition to `TRUE` in the status of the `network.config` CR.

4. CNO patches the network.operator CR with
   `{"spec":{"migration":{"networkType":"OVNKubernetes","mode":"Live"}}}`

5. CNO then redeploys openshift-sdn in migration mode. It
   will add a condition check to the wrapper script of the sdn container to see
   if the bridge `br-ex` exists on the node. If `br-ex` exists, it means the
   node has already been updated by MCO, therefore it's ready for running OVN
   Kubernetes. The sdn pod would just sleep infinity rather than actually
   launching the `openshift-sdn-node` process.

   CNO will also deploy OVN Kubernetes to the cluster with hybrid overlay
   enabled. To avoid racing, the host subnet allocation will be disabled in
   OVN Kubernetes.
      
   The ovnkube-node container wrapper script will be rendered with the logic for
   migration:

   - If `br-ex` doesn't exist, it means the node has not yet been updated by
     MCO, so instead of starting the `ovnkube-node` process, it would run the
     `hybrid-overlay-node` process. `hybrid-overlay-node` can add the node
     annotation `k8s.ovn.org/hybrid-overlay-distributed-router-gateway-mac`,
     which is required by hybrid overlay.

     The script will also add the node annotation
     `k8s.ovn.org/hybrid-overlay-node-subnet`, according to the HostSubnet CR of
      this node.

   - If `br-ex` does exist, it means the node has been updated by MCO. So the
     `ovnkube-node` can be started.

     The script will also add the node annotation `k8s.ovn.org/node-subnets`
     according to the HostSubnet CR of this node.
     
   CNO will wait until OVN-K is deployed in the cluster and set the
   `NetworkTypeMigrationTargetCNIAvailable` condition to `TRUE` in the status of
   the network.config CR.

6. CNO updates the `.status.migration.networkType` of the network.config CR.
   This change will trigger MCO to apply a new MachineConfig to each MCP:

   - MCO will render a new MachineConfig for each MCP.
   - MCO will cordon, drain and reboot the node.
   - When the node is up, the `br-ex` bridge will be created. It means the node
     is ready for OVN Kubernetes. So the local zone OVN Kubernetes components
     can be started. The ovnkube-node container wrapper script will also add the
     node annotation `k8s.ovn.org/node-subnets`, according to the HostSubnet CR
     of this node. On the local node, OVN Kubernetes will add all SDN nodes as
     hybrid overlay nodes to the local zone and all OVN nodes as OVN-IC remote
     zones. On a remote OVN node, OVN Kubernetes will remove a hybrid overlay
     node and add an OVN-IC remote zone. Pods will be recreated on the node
     using ovn-kubernetes as the default CNI plugin.
   - MCO will uncordon the node. New pods can be scheduled to this node.

   The above process will be repeated for each node until all the nodes have
   been applied to the new MachineConfig and converted to OVN Kubernetes.

7. CNO watches the MCPs, wait until the new MCs is applied to all the nodes. Set
   the `NetworkTypeMigrationTargetCNIInUse` condition to `TRUE` in the status of
   the network.config CR.

8. CNO removes the `spec.migration` field of network.operator CR. It will
   trigger the CNO to:

   - delete the openshift-sdn DaemonSets and the related resources
     (CustomResources, ConfigMaps, etc.).
   - redeploy ovn-kubernetes in "normal" mode (no migration mode script, hybrid
     overlay disabled, host subnet allocation enabled etc.).
   - remove the hybrid overlay related labels and node hybrid overlay
     annotations from the nodes.

9. CNO wait until OVN-K is redeployed Set the
   `NetworkTypeMigrationOriginalCNIPurged` condition to `TRUE` in the status of
   the network.config CR. Then set the `NetworkTypeMigrationInProgress`
   condition to `FALSE` with the reason `NetworkTypeMigrationCompleted` in the
   status of the network.config CR. This indicates that the migration is
   completed.

### API

For `Network.config.openshift.io` CRD, we will use the CR annotation
`network.openshift.io/network-type-migration:` to gate the modification to
`spec.networkType` field. If the annotation absents (default), CNO will block
the change to take effect. If it exists, CNO takes it as the trigger of starting
SDN live migration.

The following conditions will be added to the `.status` of the
`Network.config.openshift.io` CR:

- NetworkTypeMigrationInProgress
- NetworkTypeMigrationMTUReady
- NetworkTypeMigrationTargetCNIAvailable
- NetworkTypeMigrationTargetCNIInUse
- NetworkTypeMigrationOriginalCNIPurged

These conditions will be utilized by CNO to assess the present status of live
migration. These conditions besides `NetworkTypeMigrationInProgress` will be set
to `FALSE` when a live migration is initiated and reset to `UNKNOWN` once the
live migration is successfully finished. Users can also employ these conditions
to track the progress of live migration and troubleshoot any errors that may
occur.

The `LiveMigrationProgressing` condition will be set to `TRUE` when a live
migration is initiated. It will be set to `FALSE` with with the reason
`NetworkTypeMigrationCompleted` when the live migration is successfully
completed. If the `LiveMigrationProgressing` condition remains stuck in `FALSE`,
and there is an error in the reason field. it indicates an error in the live
migration process. This serves as an indicator to the user that they need to
address and resolve the issue before CNO can rollback or proceed the live
migration.

A new field `spec.migration.mode` will be introduced to the CRD
`Network.operator.openshift.io`. The supported values are `Live` and `Offline`.
If unset, the default mode is Offline.

```json
{ 
  "spec": { 
    "migration": {
      "networkType": "OVNKubernetes",
      // mode indicates whether we're doing a live migration or not.
      "mode": "Live"
    }
  } 
}
```

### Rollback

Users shall be able to rollback to openshift-sdn after the migration is
complete. The migration is bidirectional, so users can follow the same procedure
as above to conduct the rollback.

The HostSubnet CRs will be removed when the migration is done. So during the
rollback, the HostSubnet CRs will be recreated by the wrapper script of
ovnkube-node according to the node annotation `k8s.ovn.org/node-subnets`.

### Lifecycle Management

This is a one-time operation for a cluster; therefore, there is no lifecycle
management.

### Test Plan

We need to setup a CI job that can run the sig-network test against a cluster
that is in an intermediate state of live migration; specifically, some nodes are
running with OVN Kubernetes and the rest are running with OpenShift SDN. So that
we can ensure the cluster network remains functioning during the migration.

We also need to work closely with the Performance & Scale test team and the SD
team to test the live migration on a large (100+ nodes) cluster.

### Graduation Criteria

Graduation criteria follows:

#### Dev Preview -> Tech Preview

- End user documentation, relative API stability
- Sufficient test coverage
- Gather feedback from users rather than just developers

#### Tech Preview -> GA

- More testing (upgrade, scale)
- Add gating CI jobs on the relevant GitHub repos
- Sufficient time for feedback

#### Removing a deprecated feature
N/A

### Upgrade / Downgrade Strategy

This is a one-time operation for a cluster; therefore, there is no upgrade /
downgrade strategy.

### Drawbacks
N/A

### Version Skew Strategy
N/A

### API Extensions
N/A

### Operational Aspects of API Extensions
N/A

#### Failure Modes
N/A

#### Support Procedures
N/A

## Implementation History
N/A

## Alternatives

Instead of switching the network provider for an existing cluster, we can spin
up a new cluster and move the workload to it.

## Infrastructure Needed
N/A
