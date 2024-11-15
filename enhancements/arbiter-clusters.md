---
title: arbiter-clusters
authors:
  - "@eggfoobar"
reviewers:
  - "@tjungblu"
  - "@patrickdillon"
  - "@racedo"
  - "@deads2k"
  - "@jerpeter1"
  - "@sjenning"
  - "@yuqi-zhang"
  - "@zaneb"
  - "@rphillips"
  - "@joelanford"
approvers:
  - "@jerpeter1"
api-approvers:
  - "@JoelSpeed"
creation-date: 2024-08-27
last-updated: 2024-10-24
tracking-link:
  - https://issues.redhat.com/browse/OCPEDGE-1191
see-also: []
replaces: []
superseded-by: []
---

# Support 2 Node + 1 Arbiter Node HA Cluster

## Summary

This enhancement describes an ability to install OpenShift with a control plane
that consists of at minimum 2 normal sized nodes, and at least 1 node that can
be less powerful than the recommended node size. This 1 arbiter node will only
be running critical components for maintaining HA to allow the arbiter node size
to be as small and as low cost as possible with in reason.

## Motivation

Customers at the edge are requiring a more economical solution for HA
deployments at the edge. They can support running 2 node clusters for redundancy
but would like the option to deploy a lower cost node as an arbiter to supply
the 3 nodes for ETCD quorum.

### User Stories

- As a solutions architect for a retail organization, I want to deploy OpenShift
  at n number of store locations at the edge with only 2 regular sized nodes and
  1 lower cost node to maintain HA and keep compute costs down.
- As a solutions architect for cloud infrastructures, I want to offer low cost
  OpenShift deployments on purpose built hardware for a 2 + 1 configuration.
- As an OpenShift cluster admin I want non-critical applications deployed to my
  2 + 1 arbiter node cluster to not be scheduled to run on the arbiter node.
- As an OpenShift cluster admin I want to be able to allow deployments with
  proper tolerations or explicitly defined node in the `spec` to be able to be
  scheduled on the arbiter node.

### Goals

- Provide a new arbiter node role type that achieves HA but does not act as a
  full master node.
- HA for a 2+1 arbiter node should match the HA guarantees of a 3 Node Cluster
  deployment.
- Support installing OpenShift with 2 master nodes and 1 arbiter node on
  baremetal only for the time being.
- Add a new `ControlPlaneTopology` enum in the Infrastructure API
- The arbiter node hardware requirements will be lower than regular nodes in
  both cost and performance. Customers can use devices on the market from OEMs
  like Dell that supply an all in one unit with 2 compute and 1 lower powered
  compute for this deployment scenario.

### Non-Goals

The below goals are not intended to be worked on now, but might be expansion
ideas for future features.

- Running the arbiter node offsite.
- Running a virtualized arbiter node on the same cluster.
- Having a single arbiter supporting multiple clusters.
- Moving from 2 + 1 to a conventional 3 node cluster.
- Arbiter nodes are never intended to be of worker type.
- We will not be implementing cloud deployments for now, this is not due to a
  technical limitation but rather practicality of scope, validation, and
  testing. With out a clear customer use case is too much change, this is
  something we can revisit if there is a customer need.

## Proposal

The main focus of the enhancement is to support edge deployments of individual
OpenShift HA clusters at scale, and to do so in a cost effective way. We are
proposing doing this through the creation of a new node role type called an
`node-role.kubernetes.io/arbiter` node as a heterogenous quasi control plane
configuration. The arbiter will run the minimum components that help maintain an
HA cluster, things like MCD, monitoring and networking. Other platform pods
should not be scheduled on the arbiter node. The arbiter node will be tainted to
make sure that only deployments that tolerate that taint are scheduled on the
arbiter.

We think creating this new node role would be the best approach for this
feature. Having a new node role type allows our bootstrap flow to leverage the
existing MCO and Installer systems to supply specific Ignition files for a
smaller footprint device. Existing deployments will essentially be blind to this
node role unless they need to be made aware of it, but critical workloads that
allow all taints can still run on the node, thus making it easier for smaller
arbiter nodes to be deployed with out higher memory and cpu requirements.

Components that we are proposing to change:

| Component                                                     | Change                                                                                                          |
| ------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------- |
| [Infrastructure API](#infrastructure-api)                     | Add `HighlyAvailableArbiter` as a new value for `ControlPlaneTopology`                                          |
| [Installer](#installer-changes)                               | Update install config API to Support arbiter machine types                                                      |
| [MCO](#mco-changes)                                           | Update validation hook to support arbiter role and add bootstrapping configurations needed for arbiter machines |
| [Kubernetes](#kubernetes-change)                              | Update allowed `well_known_openshift_labels.go` to include new node role as an upstream carry                   |
| [ETCD Operator](#etcd-operator-change)                        | Update operator to deploy operands on both `master` and `arbiter` node roles                                    |
| [library-go](#library-go-change)                              | Update the underlying static pod controller to deploy static pods to `arbiter` node roles                       |
| [Authentication Operator](#authentication-operator-change)    | Update operator to accept minimum 2 kube api servers when `ControlPlaneTopology` is `HighlyAvailableArbiter`    |
| [Hosted Control Plane](#hosted-control-plane-change)          | Disallow HyperShift from installing on the `HighlyAvailableArbiter` and `SingleReplica` topology                |
| [Alternative Install Flows](#alternative-install-flow-change) | Update installation flow for new node role via tooling such as Assisted Installer, Assisted Service and ZTP     |
| [OLM Filtering](#olm-filter-addition)                         | Add support to OLM to filter operators based off of control plane topology                                      |

### Infrastructure API

We need an authoritative flag to make it easy for components and tools to
identify if they are running against an `arbiter` backed HA cluster. We are
proposing adding a new `ControlPlaneTopology` key
called `HighlyAvailableArbiter` to denote that the cluster is installed with an
arbiter.

Currently this field will not change, once a cluster is installed as an arbiter
HA cluster, it will remain that way. There has been discussion on making this
field mutable to allow a cluster to transition between topologies. However, that
conversation is beyond the scope of this enhancement, and requires its own
dedicated enhancement. We will keep this field as static and not support
topology migrations.

### Installer Changes

We will need to update the installer to have awareness of the explicit intent to
setup the cluster with an arbiter node. Adding a new machine type similar to the
flow for `master` and `workers` machines. As an example if the intent is to
generate 1 `arbiter` machine and 2 `master` machines, the user specifies a
`MachinePool` for `installConfig.arbiter` field with the desired replicas.

The `arbiter` will be a `MachinePool` object that will enforce at least 1
replica, and at least 2 replicas for the `controlPlane`. When no `arbiter` field
is supplied regular flow will validate. When `arbiter` is supplied 2 replicas
will be valid for for `controlPlane` as long as an `arbiter` is specified.

Some validation we will enforce:

1. Only on BareMetal installs.
2. Minimum 1 Arbiter replica if Arbiter is defined.
3. Minimum 2 Master replica when Arbiter is defined.

`installConfig.yaml`

```yaml
apiVersion: v1
baseDomain: devcluster.openshift.com
compute:
  - architecture: amd64
    hyperthreading: Enabled
    name: worker
    platform: {}
    replicas: 0
arbiter:
  architecture: amd64
  hyperthreading: Enabled
  replicas: 1
  name: arbiter
  platform:
    baremetal: {}
controlPlane:
  architecture: amd64
  hyperthreading: Enabled
  name: master
  platform:
    baremetal: {}
  replicas: 2
platform:
  baremetal:
    ...
    hosts:
      - name: cluster-master-0
        role: master
        ...
      - name: cluster-master-1
        role: master
        ...
      - name: cluster-arbiter-0
        role: arbiter
        ...

```

Ex. `/machines/arbiter.go`

```go
type Arbiter struct {
	UserDataFile           *asset.File
	MachineConfigFiles     []*asset.File
	MachineFiles           []*asset.File
	ControlPlaneMachineSet *asset.File
	IPClaimFiles           []*asset.File
	IPAddrFiles            []*asset.File

	// SecretFiles is used by the baremetal platform to register the
	// credential information for communicating with management
	// controllers on hosts.
	SecretFiles []*asset.File

	// NetworkConfigSecretFiles is used by the baremetal platform to
	// store the networking configuration per host
	NetworkConfigSecretFiles []*asset.File

	// HostFiles is the list of baremetal hosts provided in the
	// installer configuration.
	HostFiles []*asset.File
}
```

### MCO Changes

Currently MCO blocks machine config pools that target non master/worker roles in
a [validation
webhook](https://github.com/openshift/machine-config-operator/blob/7c5ae75515fe373e3eecee711c18b07d01b5e759/manifests/machineconfigcontroller/custom-machine-config-pool-selector-validatingadmissionpolicy.yaml#L20-L25).
We will need to update this to allow the new arbiter role.

`custom-machine-config-pool-selector-validatingadmissionpolicy.yaml`

```
          has(object.spec.machineConfigSelector.matchLabels) &&
          (
            (object.spec.machineConfigSelector.matchLabels["machineconfiguration.openshift.io/role"] == "master")
            ||
            (object.spec.machineConfigSelector.matchLabels["machineconfiguration.openshift.io/role"] == "worker")
            ||
            (object.spec.machineConfigSelector.matchLabels["machineconfiguration.openshift.io/role"] == "arbiter")
          )
```

We will also need to update the go logic in the renderer to allow the `arbiter`
role.

`render.go`

```go
// GenerateMachineConfigsForRole creates MachineConfigs for the role provided
func GenerateMachineConfigsForRole(config *RenderConfig, role, templateDir string) ([]*mcfgv1.MachineConfig, error) {
	rolePath := role
	//nolint:goconst
	if role != "worker" && role != "master" && role != "arbiter" {
		// custom pools are only allowed to be worker's children
		// and can reuse the worker templates
		rolePath = "worker"
	}
...
```

We then need to add new rendered initial files for the arbiter role so
bootstrapping occurs correctly. When MCO stands up the Machine Config Server
today, we supply initial configs from the
[template/<master/worker>/00-<master/worker>](https://github.com/openshift/machine-config-operator/tree/master/templates)
that are rendered in the [template
controller](https://github.com/openshift/machine-config-operator/blob/7c5ae75515fe373e3eecee711c18b07d01b5e759/pkg/controller/template/render.go#L102).
We will need to add the appropriate folder with the files relevant to the
arbiter, which in this iteration will be similar to master, except the kubelet
service will contain different initial
[node-roles](https://github.com/openshift/machine-config-operator/blob/7c5ae75515fe373e3eecee711c18b07d01b5e759/templates/master/01-master-kubelet/_base/units/kubelet.service.yaml#L31)
and
[taints](https://github.com/openshift/machine-config-operator/blob/7c5ae75515fe373e3eecee711c18b07d01b5e759/templates/master/01-master-kubelet/_base/units/kubelet.service.yaml#L43).
In the future we might need to alter these files further to accommodate smaller
node footprint.

Futhermore, when it comes to the arbiter resources, we need to make sure things
are not created when not needed. The resources should only be created when the
arbiter topology is explicitly chosen during install. The MCP and resources
described here in the MCO changes should only be applied if the arbiter topology
is explicitly turned on.

`/templates/arbiter/**/kubelet.service.yaml`

```
        --node-labels=node-role.kubernetes.io/arbiter,node.openshift.io/os_id=${ID} \
...
        --register-with-taints=node-role.kubernetes.io/arbiter=:NoSchedule \
```

The initial TechPreview of this feature will copy over the master configurations
with slight alterations for the `arbiter`. However, for GA we will need to
create a more appropriate flow for this so we're not duplicating so much.

### Kubernetes Change

Our fork of kubernetes contains a small addition for custom node roles thats
used to circumvent the [kubelet flag
validation](https://github.com/openshift/kubernetes/blob/6c76c890616c214538d2b5d664ccfcb4f5e460bd/cmd/kubelet/app/options/options.go#L158)
preventing a node from containing roles that use the prefix
`node-role.kubernetes.io`. We will need modify
[well_known_openshift_labels.go](https://github.com/openshift/kubernetes/blob/master/staging/src/k8s.io/kubelet/pkg/apis/well_known_openshift_labels.go)
to include the new `node-role.kubernetes.io/arbiter`. This will be an upstream
carry patch for this file, and from what we could identify this was the only
needed spot to modify in the openshift/kubernetes code.

`/well_known_openshift_labels.go`

```go
const (
	NodeLabelControlPlane = "node-role.kubernetes.io/control-plane"
  NodeLabelArbiter      = "node-role.kubernetes.io/arbiter"
	NodeLabelMaster       = "node-role.kubernetes.io/master"
	NodeLabelWorker       = "node-role.kubernetes.io/worker"
	NodeLabelEtcd         = "node-role.kubernetes.io/etcd"
)

var openshiftNodeLabels = sets.NewString(
	NodeLabelControlPlane,
  NodeLabelArbiter,
	NodeLabelMaster,
	NodeLabelWorker,
	NodeLabelEtcd,
)
```

### ETCD Operator Change

CEO(cluster-etcd-operator) will need to be made aware of the new arbiter role.
There are some challenges posed by this since CEO uses the node informer for
`master` in a lot of their controllers to manage the etcd cluster. We also have
a challenge in that the `node-role` labels are just empty keys, meaning that a
label selector like `node-role in (master,arbiter)` will not work for
`node-role` since we always just match on the key's existence. Furthermore,
labels selectors by design do not support an `or` operation on keys
([see](https://github.com/kubernetes/kubernetes/issues/90549#issuecomment-620625847)).
We will need to wrap the informer and lister interfaces to allow the logic not
to change for CEO.

When the feature is turned on, the custom informer and lister will be used to
watch nodes that have `node-role.kubernetes.io/master` or
`node-role.kubernetes.io/arbiter` labels.

`starter.go`

```go
...
  // We update the core master node informer and lister to use the new wrapper, and removed the master prefix in favor or `controlPlane`
	controlPlaneNodeInformer := ceohelpers.NewMultiSelectorNodeInformer(kubeClient, 1*time.Hour, cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc}, masterNodeLabelSelectorString, arbiterNodeLabelSelectorString)
	controlPlaneNodeLister := ceohelpers.NewMultiSelectorNodeLister(controlPlaneNodeInformer.GetIndexer(), arbiterNodeLabelSelector)
..
```

Lastly, we have a few smaller changes to the other components that do not use
the informers and listers in the `starter.go`. One is the
[etcdcli.go](https://github.com/openshift/cluster-etcd-operator/blob/df38d111413a02485ea83ed06a4735f77ffb22f5/pkg/etcdcli/etcdcli.go#L515-L525)
which is responsible for updating the `etcd-endpoints` configmap, which contains
the IPs for the etcd members. We will need to add a check for arbiter nodes as
well as the existing `master` to correctly fetch the `endpoints` and update the
configmap. We also need to do a small addition
[observe_control_plane_replicas_count.go](https://github.com/openshift/cluster-etcd-operator/blob/df38d111413a02485ea83ed06a4735f77ffb22f5/pkg/operator/configobservation/controlplanereplicascount/observe_control_plane_replicas_count.go#L62-L70)
controller, this is responsible for updating the cluster `ETCD` resource with
the correct `observedConfig.controlPlane.replicas`, this needs to be updated to
correctly reflect the `replicas` count that contains both `controlPlane` and
`arbiter` counts.

We have a few more changes in CEO related to the deligated tooling in
`library-go`, that will be covered in the next section.

### library-go Change

Continuing from the CEO change, some of the underlying controllers used by CEO
need small changes to support the arbiter node. The `StaticPod` controller and
the `Guard` controller will need to be updated to add a helper method that
allows CEO and other tools to easily turn on the `arbiter` node selector for
their internal node listers.

`ex:`

```go

func (c *Controller) WithArbiter() {
    c.withArbiter = true;
}
...

if c.withArbiter {
    arbiterselector, err := labels.NewRequirement("node-role.kubernetes.io/arbiter", selection.Equals, []string{""})
    arbiternodes, err := c.nodeLister.List(labels.NewSelector().Add(*arbiterselector))
    nodes = append(nodes, arbiternodes...)
}
...

```

### Authentication Operator Change

The authentication operator runs a [readiness check
controller](https://github.com/openshift/cluster-authentication-operator/blob/2d71f164af3f3e9c84eb40669d330df2871956f5/pkg/controllers/readiness/unsupported_override.go#L58-L70)
that enforces for all non `SingleReplicaTopologyMode` topologies or an
explicitly override, there must be 3 or more `kube-apiservers`. We need to add
another check to this switch statement to allow minimum 2 replicas for
`HighlyAvailableArbiter` topologies.

### Hosted Control Plane Change

We will not support running this type of topology for hosted control plane
installations. We need to update HyperShift to check for the
`HighlyAvailableArbiter` control plane topology and prevent HCP installations on
Arbiter clusters. This will need to be done at the `hypershift` cli level as
well as any bootstrap component that can be created outside of the CLI flow.

### Alternative Install FLow Change

We currently have a few different options for different needs when installing
OCP that need to also be updated. Work done in the installer should be reflected
on the Assisted Installer, Assisted Service and ZTP.

### OLM Filter Addition

With this change it would be prudent to add the ability to allow operators to
specify which control plane topology they support. This gives us more guards
against installing layered components on unsupported or unconsidered topologies
like the Master+Arbiter in this enhancement.

This is a nice to have before we go GA and we should be looking into adding it
to OLM.

### Workflow Description

#### For Baremetal IPI Installs

1. The user creates an `install-config.yaml` like normal.
2. The user defines the `installConfig.controlPlane` field with `2` replicas.
3. The user then enters the new field `installConfig.arbiter` defines the
   arbiter node pool and it's replicas set to `1` and the machine type desired
   for the platform chosen.
4. The user then enters the empty information for
   `installConfig.controlPlane.platform.baremetal` and
   `installConfig.arbiter.platform.baremetal`.
5. The user adds the appropriate hosts to the `platform.baremetal.hosts` array,
   2 master machines with the `role` of `master` and 1 arbiter machine with the
   `role` of `arbiter`, the typical flow for bare metal installs should be
   followed.
6. The user runs `openshift-install create manifests`
7. The installer creates a new `arbiter-machine-0` Machine same as
   `master-machine-{0-1}` is currently generated.
8. The arbiter machine is labeled with
   `machine.openshift.io/cluster-api-machine-role: arbiter` and
   `machine.openshift.io/cluster-api-machine-type: arbiter` and set with the
   taint for `node-role.kubernetes.io/arbiter=NoSchedule`.
9. Arbiter is treated like a master node, in that the installer creates the
   `arbiter-ssh.yaml` and the `arbiter-user-data-secret.yaml`, mirroring a
   normal master creation.
10. The installer sets the `ControlPlaneTopology` to `HighlyAvailableArbiter`
    and `InfrastructureTopology` to `HighlyAvailable`
11. The user then begins the install via `openshift-install create cluster`

#### During Install

1. The CEO will watch for new masters and the arbiter role
2. CEO will create the operand for the etcd deployments that have tolerations
   for the arbiter
3. Operators that have tolerations for the arbiter should be scheduled on the
   node
4. The install should proceed as normal

### API Extensions

#### Installer API Change

The `installConfig` will include a `installConfig.arbiter` object for a
`MachinePool` to configure arbiter infra structure.

#### OCP Config API Change

The Infrastructure config fields for `ControlPlaneTopology` will support the new
value of `HighlyAvailableArbiter`

`InfrastructureTopology` will be left as `HighlyAvailable` in this situation
since the existing rule is we only highlight when we are dealing with
`SingleReplica` workers. In this instance, at minimum 2 masters will be able to
schedule worker workloads, thus follow the same pattern as a 3 Node cluster.
This makes the most sense for now since having `InfrastructureTopology` denote a
halfway point between single and three might be a moot distinction for workers.
However, we might need to revisit this as a product team might assume a
`InfrastructureTopology: HighlyAvailable` == `3 Workers`.

### Topology Considerations

#### Hypershift / Hosted Control Planes

We will need to make sure that HyperShift itself can not be installed on the
`HighlyAvailableArbiter` topology. We should also take the opportunity in that
change to also disallow `SingleReplica`.

#### Standalone Clusters

This change is relevant to standalone deployments of OpenShift at the edge or
datacenters. This enhancement specifically deals with this type of deployment.

#### Single-node Deployments or MicroShift

This change does not effect Single Node or MicroShift.

### Implementation Details/Notes/Constraints

Currently there are some behavior unknowns, we will need to test out our POC to
validate some of the desires in this proposal. In it's current version this
proposal is not exhaustive but will be filled out as we implement these goals.

We currently expect this feature to mainly be used by `baremetal` installs, or
specialized hardware that is built to take advantage of this type of
configuration. In the current design we make two paths for cloud and baremetal
installs in the installer. However, the cloud install is primarily for testing,
this might mean that we simplify the installer changes if we are the only ones
using cloud installs, since we can simply alter the manifests in the pipeline
with out needing to change the installer.

In the current POC, a cluster was created on AWS with 2 masters running
`m6a.4xlarge` and 1 arbiter running a `m6a.large` instance. At idle the node is
consuming `1.41 GiB / 7.57 GiB	0.135 cores / 2 cores`. The cluster installs and
operates with the following containers running on the arbiter node. This seems
very promising, but we will need to identify any other resources we might want
to run or remove from this list. The list is small already so there might not be
a desire to pre-optimize just yet.

| Component                                                                                          | Component Outlook on Arbiter                                      |
| -------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------- |
| "openshift-cluster-csi-drivers/aws-ebs-csi-driver-node-qvvqc"                                      | Probably not needed, no required component provisions EBS volumes |
| "openshift-cluster-node-tuning-operator/tuned-q7f98"                                               | Might be useful for performance tuning                            |
| "openshift-dns/node-resolver-zcp7j"                                                                | Keep, DNS configuration                                           |
| "openshift-etcd/etcd-guard-ip-10-0-21-103.us-west-2.compute.internal"                              | Keep, Needed for HA                                               |
| "openshift-etcd/etcd-ip-10-0-21-103.us-west-2.compute.internal"                                    | Keep, Needed for HA                                               |
| "openshift-image-registry/node-ca-4cxgp"                                                           | Not needed, no pods use image registry                            |
| "openshift-machine-config-operator/kube-rbac-proxy-crio-ip-10-0-21-103.us-west-2.compute.internal" | Keep, Node configuration                                          |
| "openshift-machine-config-operator/machine-config-daemon-mhflm"                                    | Keep, Node configuration                                          |
| "openshift-monitoring/node-exporter-xzwc6"                                                         | Keep, Monitoring                                                  |
| "openshift-multus/multus-544fd"                                                                    | Keep, Networking                                                  |
| "openshift-multus/multus-additional-cni-plugins-wkp64"                                             | Keep, Networking                                                  |
| "openshift-multus/network-metrics-daemon-cpskx"                                                    | Keep, Monitoring                                                  |
| "openshift-network-diagnostics/network-check-target-fq57x"                                         | Keep, Networking                                                  |
| "openshift-ovn-kubernetes/ovnkube-node-gpj6p"                                                      | Keep, Networking                                                  |

### Risks and Mitigations

The main risk in this enhancement is that because we are treating one of the
master nodes in a 3 node cluster as an arbiter, we are explicitly evicting
processes that would otherwise be a normal supported upstream configuration such
as a compact cluster. We run the risk of new components being critical to HA not
containing the proper tolerations for running on the arbiter node. One of the
mitigations we can take against that is to make sure we are testing installs and
updates.

We also need to take stock of pod disruption budgets for platform deployments to
make sure they can function correctly in this environment. In that same vein we
also need to check the layered products and validate what we can to make sure
they can also function in this environment.

A couple of risks we run is customers using an arbiter node with improper disk
speeds below that recommended for etcd, or a bigger problem being network
latency. Since etcd is sensitive to latency between members (see:
[etcd-io/etcd#14501](https://github.com/etcd-io/etcd/issues/14501)), we should
provide proper guidance so that the arbiter node must meet minimum requirements
for ETCD to function properly, in disk and network speeds. ([ETCD
Recommendations](https://docs.openshift.com/container-platform/4.16/scalability_and_performance/recommended-performance-scale-practices/recommended-etcd-practices.html))

### Drawbacks

A few drawbacks we have is that we will be creating a new variant of OpenShift
that implements a new unique way of doing HA for kubernetes. This does mean an
increase in the test matrix and all together a different type of tests since the
addition of an arbiter node will require different validation scenarios for
failover.

## Open Questions [optional]

1. In the future it might be desired to add another master and convert to a
   compact cluster, do we want to support changing ControlPlaneTopology field
   after the fact?

2. Do we need to modify OLM to filter out deployments based on topology
   information?

3. Since the focus of this feature is BareMetal for GA, we don't need to worry
   about the [control-plane machine-set
   operator](https://github.com/openshift/cluster-control-plane-machine-set-operator/tree/main/docs/user#overview).
   However, we will need to revisit this so we a proper idea of the role that it
   plays in this type of configuration.

## Test Plan

- We will create a CI lane to validate install and fail over scenarios such as
  loosing a master or swaping out an arbiter node.
- We will create a CI lane to validate upgrades, given the arbiter's role as a
  quasi master role, we need to validate that MCO treats upgrades as expected
- Create complimentary lanes for serial/techpreview/no-capabilities.
- Create a lane for or some method for validating layered products.
- CI lane for e2e conformance testing, tests that explicitly test 3 node masters
  will need to be altered to accommodate the different topology.
- We will add e2e tests to specifically test out the expectations in this type
  of deployment.
  - Create tests to validate correct pods are running on the `arbiter`.
  - Create tests to validate no incorrect pods are running on the `arbiter`.
  - Create tests to validate routing and disruptions are with in expectations.
  - Create tests to validate proper usage of affinity and anti-affinity.
  - Create tests to validate pod disruption budgets are appropriate.

## Graduation Criteria

### Dev Preview -> Tech Preview

N/A

### Tech Preview -> GA

- MCO Changes need to be streamlined so we don't duplicate too much between
  master and arbiter configurations
- OLM Needs to be update to allow operator owners to explicitly add or remove
  support for HA Arbiter deployments
- More testing (upgrade, downgrade, scale)
- E2E test additions for this feature
- Sufficient time for feedback
- Available by default
- Backhaul SLI telemetry
- Document SLOs for the component
- Conduct load testing
- User facing documentation created in
  [openshift-docs](https://github.com/openshift/openshift-docs/)

### Removing a deprecated feature

N/A

## Upgrade / Downgrade Strategy

WIP

## Version Skew Strategy

N/A

## Operational Aspects of API Extensions

WIP

## Support Procedures

WIP

## Alternatives

We originally had tried using the pre-existing features in OCP, such as setting
a node as NoSchedule to avoid customer workloads going on the arbiter node.
While this whole worked as expected, the problem we faced is that the desire is
to use a device that is lower power and is cheaper as the arbiter. This method
would still run most of the OCP overhead on the arbiter node.

## Infrastructure Needed [optional]

N/A
