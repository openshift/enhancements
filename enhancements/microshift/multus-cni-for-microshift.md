---
title: multus-cni-for-microshift
authors:
  - pmtk
reviewers:
  - s1061123, Multus expert
  - pliurh, Networking expert
  - dhellmann, MicroShift architect
  - jerpeter1, Edge Enablement Staff Engineer
  - pacevedom, MicroShift team lead
approvers:
  - dhellmann
api-approvers:
  - None
creation-date: 2024-01-16
last-updated: 2024-02-06
tracking-link:
  - https://issues.redhat.com/browse/OCPSTRAT-473
# see-also:
#   - "/enhancements/this-other-neat-thing.md"
# replaces:
#   - "/enhancements/that-less-than-great-idea.md"
# superseded-by:
#   - "/enhancements/our-past-effort.md"
---

# Multus CNI for MicroShift

## Summary

Currently MicroShift ships [ovn-kubernetes](https://github.com/openshift/ovn-kubernetes) (ovn-k)
CNI responsible for connectivity within and outside the cluster.
There are users that have needs beyond what ovn-k offers like adding more interfaces to the Pods.
Some example requirements are connecting Pods to the host's bridge interface or setting up complex networking based on VLAN.
This functionality is Multus' trademark - adding additional interfaces to Pods.

This enhancement explores providing Multus CNI as an optional component to MicroShift.

## Motivation

Providing Multus CNI for MicroShift will help users:
- wanting to integrate MicroShift into existing environments
- wanting to slowly, step by step, migrate to MicroShift without being required to refactor everything at once
- wanting to add additional interfaces because of newly discovered requirements

### User Stories

* As a MicroShift admin, I want to add additional interfaces to certain Pods so that I can
  make them accessible over networks that should not be available to rest of the cluster.
* As a MicroShift admin, I want to add additional interfaces to certain Pods so that I can
  access them directly (from outside the cluster) without using Kubernetes' networking such as
  NodePorts, Load Balancers, Ingresses, etc.
* As a MicroShift admin, I want to add additional interfaces to certain Pods so that I can
  start slowly migrating an existing solution to MicroShift.

### Goals

- Provide optional Multus CNI for MicroShift clusters that can be added to existing clusters
- Provide container network plugins that are planned to be supported with MicroShift and Multus,
  meaning:
  - IPAMs: host-local, dhcp, static
  - CNIs:
    - bridge - must have
    - macvlan, ipvlan - stretch goal

### Non-Goals

- Automatically removing Multus from the cluster upon RPM uninstall
- Support Multus for multi-node deployments of MicroShift
- Providing network policies, support for Services via CNIs other than ovn-kubernetes,
  or admission webhook for Multus

## Proposal

Deliver Multus for MicroShift as an optional RPM containing required manifests that will be applied
during MicroShift's start. There should be little to no changes to MicroShift itself as we want
Multus CNI for MicroShift to fit optional components pattern.

Manifests for deploying Multus on MicroShift will be based on existing manifests for OpenShift,
but they will differ because OpenShift uses thick architecture Multus whereas MicroShift will use
thin architecture Multus.

How should cleanup of the Multus artifacts look like is an open question (see Open Questions section).

### Workflow Description

**User** is a human user responsible for setting up and managing Edge Devices.
**Application** is user's workload that intends to use additional interfaces.

#### Installation and usage on RHEL For Edge (ostree)

> In this workflow, it doesn't matter if the device is already running R4E with existing MicroShift cluster.
> Deployment of new commit requires reboot which will force recreation of the Pod networking after adding Multus.

1. User gathers all information about the networking environment of the edge device.
1. User prepares NetworkAttachmentDefinition (NAD) CRs that will allow Application to be part of the specified network.
1. User prepares ostree commit that contains:
   - (optional) Init procedures to configure OS for usage of the additional network
   - MicroShift RPMs
   - Multus for MicroShift RPM
   - NetworkAttachmentDefinition CRs
   - Application using mentioned NetworkAttachmentDefinition CRs
1. User deploys the ostree commit onto the edge device.
1. Edge device boots:
1. (optional) Init procedures are configuring OS and networks
1. MicroShift starts
1. MicroShift applies Multus' manifests
1. MicroShift applies Application's manifests that include NetworkAttachmentDefinitions
1. Application's Pod are created, Multus inspects Pod's annotations and sets up 
   additional interfaces based on matching NetworkAttachmentDefinitions
1. Application's containers are running, they can utilize additional interfaces

#### Installation and usage on RHEL (rpm)

##### Adding to existing MicroShift cluster

1. MicroShift already runs on the device.
1. User installs `microshift-multus` RPM
1. User reboots the host
1. Host boots, MicroShift starts and deploys Multus from the manifests.d.
1. User creates NetworkAttachmentDefinition CRs
1. User deploys application that uses NetworkAttachmentDefinitions
1. When network for Pods is created, Multus calls additional CNIs according to annotations
   and NetworkAttachmentDefinitions
1. Application's containers are running, they can utilize additional interfaces

##### Adding to MicroShift cluster before first start

1. MicroShift is not installed. MicroShift's database (`/var/lib/microshift`) does not exist.
1. User installs `microshift` and `microshift-multus` RPMs.
1. User enables and starts `microshift.service`
1. MicroShift starts and deploys Multus from the manifests.d.
1. User creates NetworkAttachmentDefinition CRs
1. User deploys application that uses NetworkAttachmentDefinitions
1. When network for Pods is created, Multus calls additional CNIs according to annotations
   and NetworkAttachmentDefinitions
1. Application's containers are running, they can utilize additional interfaces

### API Extensions

Multus is an established project with already existing API extensions.
Following paragraphs does not present brand new CRDs or APIs, it only aims to summarize how adding
Multus will affect MicroShift's API. For more information see
[Multus CNI repository](https://github.com/openshift/multus-cni/).

Multus is configured in following ways:
- CNI configuration created when Multus' DaemonSet starts, it includes the primary CNI (ovn-kubernetes for MicroShift)
  that is default delegate (CNI invoked for all Pods). It can be crafted manually but usually it's
  autogenerated based on current primary CNI and options provided to the script.
- NetworkAttachmentDefinition (net-attach-def) CR
- Annotations

NetworkAttachmentDefinition (NAD) is a simple CR which only contains single string field called `config` which
contains a JSON-formatted CNI configuration.
Exact schema of the CNI config depends on the specific CNI and its documentation must be consulted.
Exact values depend on the runtime environment as configs may require name of the specific host interface.
For these reasons, we can only provide examples and guidance on crafting CNI configs, therefore we
cannot package a default configuration that could work on every host in `microshift-networking-multus` RPM.

Below is example of NAD to use the `bridge` CNI with host bridge named `test-bridge`:
```yaml
apiVersion: "k8s.cni.cncf.io/v1"
kind: NetworkAttachmentDefinition
metadata:
  name: bridge-conf
spec:
  config: '{
      "cniVersion": "0.3.0",
      "type": "bridge",
      "bridge": "test-bridge",
      "mode": "bridge",
      "ipam": {
        "type": "host-local",
        "subnet": "192.168.20.0/24",
        "rangeStart": "192.168.20.200",
        "rangeEnd": "192.168.20.216",
        "routes": [
          { "dst": "0.0.0.0/0" }
        ],
        "gateway": "192.168.20.1"
      }
    }'
```

There are two Pod annotation that can be used to instruct Multus on how the networks should be set up.
Main one is `k8s.v1.cni.cncf.io/networks` which specifies which NAD should be added to the Pod.
Multiple NADs can be specified by separating them with comma. NADs can be even reused.
Examples from Multus docs:
```yaml
apiVersion: v1
kind: Pod
metadata:
  name: samplepod
  annotations:
    k8s.v1.cni.cncf.io/networks: bridge-conf
---
apiVersion: v1
kind: Pod
metadata:
  name: samplepod
  annotations:
    k8s.v1.cni.cncf.io/networks: bridge-conf,bridge-conf
```

Information about the Pod's interfaces are reported back by Multus also as an annotation `k8s.v1.cni.cncf.io/network-status`.
Example below shows annotations of a Pod with two `bridge` CNI interfaces:
```yaml
Annotations:      k8s.v1.cni.cncf.io/network-status:
                    [{
                        "name": "ovn-kubernetes",
                        "interface": "eth0",
                        "ips": [
                            "10.42.0.14"
                        ],
                        "mac": "0a:58:0a:2a:00:0e",
                        "default": true,
                        "dns": {}
                    },{
                        "name": "default/bridge-conf",
                        "interface": "net1",
                        "ips": [
                            "192.168.20.202"
                        ],
                        "mac": "b2:6a:5c:ba:34:9a",
                        "dns": {},
                        "gateway": [
                            "\u003cnil\u003e"
                        ]
                    },{
                        "name": "default/bridge-conf",
                        "interface": "net2",
                        "ips": [
                            "192.168.20.203"
                        ],
                        "mac": "7a:ca:cf:19:64:e8",
                        "dns": {},
                        "gateway": [
                            "\u003cnil\u003e"
                        ]
                    }]
                  k8s.v1.cni.cncf.io/networks: bridge-conf,bridge-conf
```

There is an annotation for NetworkAttachmentDefinition - `k8s.v1.cni.cncf.io/resourceName`.
It is used when some CNI requires information about specific device that is prepared by device plugin.
It is primarily used with SR-IOV and as such out of scope for this enhancement mentioned only for completeness.


### Implementation Details/Notes/Constraints [optional]

First, it must be noted that Multus CNI itself is a meta-CNI. From a high level perspective, its
purpose is to call other CNIs according to the CNI configs supplied by the user in form of
NetworkAttachmentDefinitions and Pod annotations.
Any specific actions are related to the delegate CNIs. For example: creating veth pair,
attaching one end of veth to the bridge and making the other end available within the Pod is
responsibility of `bridge` CNI is this example.
MicroShift team will create tests for CNIs that will be declared as supported. However these tests
will not explore the breadth and depth of possible network setups, so ultimately the responsibility
for correctness of the configuration is up to the user.

#### Manifests

MicroShift will provide Multus based on thin architecture because of the resource consumption
(see alternatives for more information).
Because of the differences with OpenShift (which uses thick Multus plugin), existing OpenShift
manifests will require changes to make them suitable for MicroShift. These manifests will reside
in MicroShift repository.

Because one of the required IPAMs is the `dhcp` (dynamic), manifests will also include
a DHCP server DaemonSet.

Updating necessary image references will be part of existing rebase procedure.

#### RPM package

RPM spec to build `microshift-multus` and `microshift-multus-release-info` RPMs will be part of
existing `microshift.spec` file.
The RPM will include:
- manifests required to deploy Multus on MicroShift
- CRI-O drop-in config to use Multus instead of ovn-kubernetes (which will require reorganization
  of currently existing MicroShift's CRI-O configs)
- greenboot healthcheck script
- cleanup script plugin

#### Container images: Multus and network plugins

Because the Multus image used by OpenShift has a large size that is not acceptable for edge devices, new image
will be prepared and will only include relevant artifacts such as entrypoint script/binary and
Multus CNI binary (which is copied to host's `/opt/cni/bin`).

To supply network plugins (CNIs) such as `bridge`, `ipvlan`, and `macvlan` a new image will be prepared
so MicroShift uses the same binaries as OpenShift (alternative is using RHEL's networkplugins RPM - see alternatives).
This image will also contain IPAM binaries such as `static`, `dynamic (DHCP)`, and `host-local`.

Both of these images will be part of the OpenShift payload which MicroShift references during rebase procedure.

#### Hypershift [optional]

No, enhancement is MicroShift specific.

### Risks and Mitigations

There may be a race condition between Multus on MicroShift and other services on the host that also
configure the host's networking
Taking `bridge` CNI as an example: when the bridge interface does not exist, it will be created
when a Pod requiring that interface is created.
If user expects something else will create the interface, they will need to configure system to start
MicroShift after other services.

Multus using thin plugin architecture creates a kubeconfig when the DaemonSet starts and copies it
out of Pod to `/etc/cni/net.d/multus.d/multus.kubeconfig` on the host filesystem so the Multus CNI
binary can use it to get
NetworkAttachmentDefinitions and Pod annotations. The file is owned and readable only by root, so
the risk of someone gaining access to the cluster after logging into the host can be compared to
getting access to `/var/lib/microshift/resources/kubeadmin/kubeconfig`.

Currently MicroShift runs as a single-node cluster but in future releases there might be an effort
to allow for multi-node clusters. For this reason, during implementation of this enhancement, no
assumption that MicroShift will always run as a single-node should be made and potential multi-node
should be kept in mind. We will not build the solution to support multi-node, but want to avoid
making decisions that make it harder to do so in the future.

Another potential risk was investigated but judged as not a problem: could a race condition between
Multus being ready and application's Pods starting impact the application, by for example not having
relevant parts of network set up.
This should not be an issue because Pods that do not use `hostNetwork` will have networking setup
after the CNI is ready. Both ovn-kubernetes and Multus use `hostNetwork` so they start before other
Pods. If CRI-O is configured to use the Multus regardless of files in /etc/cni/net.d, CRI-O will wait
for the Multus. We can also use the Multus' config option of `--readiness-indicator-file` to make
sure the Multus waits for the ovn-kubernetes.

### Drawbacks

This section includes limitations of the Multus itself, not its integration with MicroShift.
However, these drawbacks should be documented nonetheless.

Multus does not actively watch NetworkAttachmentDefinitions or annotations therefore to make changes
to these resources effective the Pod must be re-created. This behavior is reasonable because it does not
disrupt the Pod's networking and should not be a problem in production environments where we expect to
have stable configurations.

Multus also does not observe the underlying bridge interfaces, therefore if one is rebuilt, the Pod's
interface might stop working (see [BZ #2066351](https://bugzilla.redhat.com/show_bug.cgi?id=2066351)).
If these limitations are ever addressed (see [NP-606](https://issues.redhat.com/browse/NP-606) and 
[NP-608](https://issues.redhat.com/browse/NP-608)), they would, most likely, be part of the thick
Multus plugin.

When installing Multus CNI, CRI-O will be configured to use Multus CNI meaning that CRI-O will
wait for Multus and Multus will wait for ovn-kubernetes. This can result in slight increase of startup
time for Pods using CNI network as more preconditions are added (waiting for two CNIs in sequence).

## Design Details

### Open Questions [optional]

1. Clean up of Multus artifacts on disk during system rollback or when `microshift-cleanup-data` is executed:
   Should we invest in creating a lightweight plugin architecture to avoid adding cleanup of Multus
   to MicroShift's core cleanup script? This architecture could be based on existing
   [kubectl plugin](https://kubernetes.io/docs/tasks/extend-kubectl/kubectl-plugins/) architecture
   where plugins start with `kubectl-`, so we could use `microshift-cleanup-plugin-` prefix.
1. Should we enable namespace isolation? This requires NetworkAttachmentDefinition and Pod to be in the same
   namespace.
   - There is an additional option `--global-namespaces` where we can define a namespace so that NAD residing
     in the namespace can be referenced in Pod from any other namespace.

### Test Plan

Multus CNI for MicroShift will be tested using existing test harness. New test suite will be created
with simple test for each CNI we want to declare as supported.

Starting with `bridge` test shall: install Multus, create NetworkAttachmentDefinition and deploy
a Pod that should have additional interface attached. After verifying that the interface is present,
test will try to access Pod's application using the bridge interface on the host to make sure there
is a connectivity.

Adding Multus to existing cluster will be tested by adding this scenario to existing upgrade test
on rpm-based RHEL.

Tests for other networking plugins will be designed and implemented when plugins are planned for support.

### Graduation Criteria

Multus CNI for MicroShift is targeted to be GA next release.

#### Dev Preview -> Tech Preview

N/A

#### Tech Preview -> GA

N/A

#### Removing a deprecated feature

N/A

### Upgrade / Downgrade Strategy

Because both Multus and MicroShift RPMs will be built from the same spec file, they will share the
same version and it is expected that they are updated together following MicroShift upgrade rules
depending on type of operating system (ostree-based or regular RPM).

Considering only the manifests, we know that on each start MicroShift will apply manifests forcefully
overwriting any differences. However, MicroShift does not have any uninstall capabilities.
If manifests ever change, for example some ConfigMap is renamed, then these old parts will
keep existing in the database. This could have undesirable consequences of having two Multus DaemonSets
if the name or the namespace of original DaemonSet changes. To make the transition to thick Multus
smooth, we should not deviate from already existing resource names present in OpenShift's Multus
manifests. This problem is not Multus specific - it is how MicroShift works and it is not part of this
enhancement addressing this shortcoming.

Because of the way Multus works (checks Annotations when needed, executed when networking must be
set up, does not keep its own database beside cache) we can consider it to be mostly stateless.
When Multus CNI binary is executed by kubelet/CRI-O it will setup Pod's network according to the
configs and Annotations (not a database with "expected states"), therefore updating Multus does not
require migration strategies.

If the schema of NetworkAttachmentDefinitions CRD changes, instead of deploying a mutating webhook
(which would use resources idling but actually perform actions on first start), we can just suggest
users to update their CRs.

### Version Skew Strategy

Building Multus and MicroShift RPMs from the same spec file means Multus should be updated together
with MicroShift which means there should not be any version skew between MicroShift and Multus.
This might change with introduction of multi-node deployments of MicroShift.

### Operational Aspects of API Extensions

N/A

#### Failure Modes

If Multus (or any delegate CNIs it executes) fails, a new Pod will be stuck in "ContainerCreating" status
and none of the Pod's containers will start. This can happen if the CNI configuration provided in
NetworkAttachmentDefinition is incorrect or when Pod's Annotation contains NAD that does not exist.
In such cases, user needs to verify its manifests.

Pods without Multus' Annotation will be set up with the default CNI (ovn-kubernetes) and should not
have increased CNI failure rate.

#### Support Procedures

If Multus cannot configure a Pod's networking according to the annotations (any of the CNIs fail),
the Pod will not start and its events should contain error from the Multus. For example:
```
Warning  NoNetworkFound          0s                 multus             cannot find a network-attachment-definition (asdasd) in namespace (default): network-attachment-definitions.k8s.cni.cncf.io "asdasd" not found
```

To address such issues, user can:
- verify values in both NetworkAttachmentDefinitions and Annotations,
- remove Annotation to verify if the Pod is created successfully with just the default network.

Other support procedure, more intended for administrators of the device, is inspecting logs
of `crio.service` or `microshift.service` (especially those coming from `kubelet` component).

For example, following error from kubelet informs that there the primary CNI is not running.
It can be because the Pods are not starting or because CRI-O misconfiguration (wrong
`cni_default_network` setting).

```
Feb 06 13:47:31 dev microshift[1494]: kubelet E0206 13:47:31.163290    1494 pod_workers.go:1298] "Error syncing pod, skipping" err="network is not ready: container runtime network not ready: NetworkReady=false reason:NetworkPluginNotReady message:Network plugin returns error: No CNI configuration file in /etc/cni/net.d/. Has your network provider started?" pod="default/samplepod" podUID="fe0f7f7a-8c47-4488-952b-8abc0d8e2602"
```

Below is an example log when Pod cannot be created because the annotations reference
NetworkAttachmentDefinition that doesn't exist.

> Relevant log:
>
> cannot find a network-attachment-definition (bad-conf) in namespace (default): network-attachment-definitions.k8s.cni.cncf.io \"bad-conf\" not found" pod="default/samplepod"`

```
Feb 06 13:51:11 dev microshift[1476]: kubelet I0206 13:51:11.604745    1476 util.go:30] "No sandbox for pod can be found. Need to start a new one" pod="default/samplepod"
Feb 06 13:51:11 dev microshift[1476]: kubelet E0206 13:51:11.696487    1476 remote_runtime.go:193] "RunPodSandbox from runtime service failed" err="rpc error: code = Unknown desc = failed to create pod network sandbox k8s_samplepod_default_5fa13105-1bfb-4c6b-aee7-3437cfb50e25_0(7517818bd8e85f07b551f749c7529be88b4e7daef0dd572d049aa636950c76c6): error adding pod default_samplepod to CNI network \"multus-cni-network\": plugin type=\"multus\" name=\"multus-cni-network\" failed (add): Multus: [default/samplepod/5fa13105-1bfb-4c6b-aee7-3437cfb50e25]: error loading k8s delegates k8s args: TryLoadPodDelegates: error in getting k8s network for pod: GetNetworkDelegates: failed getting the delegate: getKubernetesDelegate: cannot find a network-attachment-definition (bad-conf) in namespace (default): network-attachment-definitions.k8s.cni.cncf.io \"bad-conf\" not found"
Feb 06 13:51:11 dev microshift[1476]: kubelet E0206 13:51:11.696543    1476 kuberuntime_sandbox.go:72] "Failed to create sandbox for pod" err="rpc error: code = Unknown desc = failed to create pod network sandbox k8s_samplepod_default_5fa13105-1bfb-4c6b-aee7-3437cfb50e25_0(7517818bd8e85f07b551f749c7529be88b4e7daef0dd572d049aa636950c76c6): error adding pod default_samplepod to CNI network \"multus-cni-network\": plugin type=\"multus\" name=\"multus-cni-network\" failed (add): Multus: [default/samplepod/5fa13105-1bfb-4c6b-aee7-3437cfb50e25]: error loading k8s delegates k8s args: TryLoadPodDelegates: error in getting k8s network for pod: GetNetworkDelegates: failed getting the delegate: getKubernetesDelegate: cannot find a network-attachment-definition (bad-conf) in namespace (default): network-attachment-definitions.k8s.cni.cncf.io \"bad-conf\" not found" pod="default/samplepod"
Feb 06 13:51:11 dev microshift[1476]: kubelet E0206 13:51:11.696565    1476 kuberuntime_manager.go:1172] "CreatePodSandbox for pod failed" err="rpc error: code = Unknown desc = failed to create pod network sandbox k8s_samplepod_default_5fa13105-1bfb-4c6b-aee7-3437cfb50e25_0(7517818bd8e85f07b551f749c7529be88b4e7daef0dd572d049aa636950c76c6): error adding pod default_samplepod to CNI network \"multus-cni-network\": plugin type=\"multus\" name=\"multus-cni-network\" failed (add): Multus: [default/samplepod/5fa13105-1bfb-4c6b-aee7-3437cfb50e25]: error loading k8s delegates k8s args: TryLoadPodDelegates: error in getting k8s network for pod: GetNetworkDelegates: failed getting the delegate: getKubernetesDelegate: cannot find a network-attachment-definition (bad-conf) in namespace (default): network-attachment-definitions.k8s.cni.cncf.io \"bad-conf\" not found" pod="default/samplepod"
Feb 06 13:51:11 dev microshift[1476]: kubelet E0206 13:51:11.696625    1476 pod_workers.go:1298] "Error syncing pod, skipping" err="failed to \"CreatePodSandbox\" for \"samplepod_default(5fa13105-1bfb-4c6b-aee7-3437cfb50e25)\" with CreatePodSandboxError: \"Failed to create sandbox for pod \\\"samplepod_default(5fa13105-1bfb-4c6b-aee7-3437cfb50e25)\\\": rpc error: code = Unknown desc = failed to create pod network sandbox k8s_samplepod_default_5fa13105-1bfb-4c6b-aee7-3437cfb50e25_0(7517818bd8e85f07b551f749c7529be88b4e7daef0dd572d049aa636950c76c6): error adding pod default_samplepod to CNI network \\\"multus-cni-network\\\": plugin type=\\\"multus\\\" name=\\\"multus-cni-network\\\" failed (add): Multus: [default/samplepod/5fa13105-1bfb-4c6b-aee7-3437cfb50e25]: error loading k8s delegates k8s args: TryLoadPodDelegates: error in getting k8s network for pod: GetNetworkDelegates: failed getting the delegate: getKubernetesDelegate: cannot find a network-attachment-definition (bad-conf) in namespace (default): network-attachment-definitions.k8s.cni.cncf.io \\\"bad-conf\\\" not found\"" pod="default/samplepod" podUID="5fa13105-1bfb-4c6b-aee7-3437cfb50e25"
```

## Implementation History

<!-- Major milestones in the life cycle of a proposal should be tracked in `Implementation
History`. -->

## Alternatives

### Thick plugin architecture

In 2022 Multus was recreated in a different way called "thick plugin" which changed its mode of operation significantly.
Major difference is that DaemonSet is no longer dummy (i.e. Thin: creates kubeconfig, config, copies Multus
CNI binary to the host, and finally sleeps or watches files to update the kubeconfig and/or config).
Instead it is the brain of the operation: CNI binary on the host is only a shim that forwards the request
to the DaemonSet which executes all of the delegates. It also exports a metric, but it was not
deemed useful.

The decision to use thin instead of thick architecture is mostly driven by resource consumption:
Multus CNI binary in thin mode only uses resources when it runs and DaemonSet idles (or is close to it),
whereas thick Multus' DaemonSet is an application that uses resources even if there are no new Pods.

If we would decide to use the thick plugin, we still would need to create a new image as the one used in OCP
is not suitable for edge deployments (1.2GB). Another pro for using the thick plugin is that is has better
test coverage as it is used in OpenShift. Also, if there will be a new CNI spec that includes UPDATE
command the thick plugin has better chance of supporting that.

Even though thin plugin suits MicroShift needs better, we should strive toward making as little
breaking changes as possible compared to OpenShift's thick multus when preparing manifests for
MicroShift's thin Multus.

See [Multus Thick Plugin](https://github.com/openshift/multus-cni/blob/master/docs/thick-plugin.md).

### Using network plugins from RHEL repositories

RHEL ships network plugins RPM that includes delegate CNIs we aim to support like `bridge`, `macvlan`, etc.
Originally that RPM was meant for Podman networking, but Podman shipped with RHEL9 does not use them anymore.
This means they can exist only for compatibility and are not actively maintained.
On the other hand, OpenShift networking team (with whom we have ongoing cooperation) are actively
maintaining these binaries and quickly addressing any CVEs or bugs. These binaries are packaged in
a container image as part of the OpenShift payload which will ensure that we do not have version
skew with MicroShift or Multus and they will match binaries shipped with OpenShift.

### Building an operator based on Cluster Network Operator

In OpenShift Multus manifests can be templated according to the needs. This includes working with
SDN or OVN-K CNI, deploying optional DHCP server, or deploying opetional whereabouts reconciler.
Rendering Multus manifests is responsibility of Cluster Network Operator (CNO).

During review of this enhancement a question was asked: should we create an operator with subset of
CNO's functionality?
Since CNO mostly renders manifests, there is no real need to add additional runtime component
that would do only that. If we ever find that we don't want to deploy DHCP server or we need
whereabouts reconciler, we can think of another way to compose Multus and accompanying addons,
for example providing several RPMs such as `microshift-multus`, `microshift-multus-dhcp`,
`microshift-multus-whereabouts`, or think of other way, e.g. something resembling helm's
`values.yaml` that could be user supplied to configure behavior of RPM supplied by MicroShift team.

## Infrastructure Needed [optional]

N/A
