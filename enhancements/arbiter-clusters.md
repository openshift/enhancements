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
approvers:
  - "@tjungblu"
  - "@patrickdillon"
  - "@racedo"
  - "@jerpeter1"
  - "@deads2k"
api-approvers:
  - "@JoelSpeed"
creation-date: 2024-08-27
last-updated: 2024-08-27
tracking-link:
  - https://issues.redhat.com/browse/OCPEDGE-1191
see-also: []
replaces: []
superseded-by: []
---

# Support 2 Node + 1 Arbiter Node HA Cluster

## Summary

This enhancement describes an ability to install OpenShift with a control plane
that consists of 2 normal sized nodes, and 1 node that can be less powerful than
the recommended node size. This 1 arbiter node will only be running critical
components for maintaining HA to allow the arbiter node size to be as small and
as low cost as possible with in reason.

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
- Support installing OpenShift with 2 master nodes and 1 arbiter node.
- The arbiter node hardware requirements will be lower than regular nodes in
  both cost and performance. Customers can use devices on the market from OEMs
  like Dell that supply an all in one unit with 2 compute and 1 lower powered
  compute for this deployment scenario.
- Moving from 2 + 1 to a conventional 3 node cluster

### Non-Goals

The below goals are not intended to be worked on now, but might be expansion
ideas for future features.

- Running the arbiter node offsite.
- Running a virtualized arbiter node on the same cluster.
- Having a single arbiter supporting multiple clusters.

## Proposal

The main focus of the enhancement is to support edge deployments of individual
OpenShift HA clusters at scale, and to do so in a cost effective way. We are
proposing doing this through the incorporation of an arbiter node as a quasi
heterogenous control plane configuration. The arbiter will run the critical
components that help maintain an HA cluster, but other platform pods should not
be scheduled on the arbiter node. The arbiter node will be tainted to make sure
that only deployments that tolerate that taint are scheduled on the arbiter.

Functionality that we are proposing to change:

- Update MCO MachinePool Validation Webhook to support `master/arbiter`
  configuration.
  - Currently MCO blocks custom machine pools that target non `master/worker`
    roles we will need to update these to allow the `arbiter` role as well.
- Update MCO to contain ignition files for the `arbiter` node
  - MCO will need to generate and provide the data for initial configuration for
    the `arbiter` node. This also gives us more flexibility to make sure the
    node configurations for the `arbiter` are more tuned for the smaller
    footprint.
- We will add support to the OCP installer to provide a way of setting up the
  initial manifests, taints, and node roles.
  - We will need to support a path for customers to indicate the desire for a
    2+1 arbiter install configuration.
  - This will also be used to apply the taint to the machineset manifest.
- Alter Cluster ETCD Operator (CEO) to be aware of the arbiter node role type
  and allow it to treat it as if it were a master node.
  - We will need CEO to create an ETCD member on the arbiter node to allow
    quarum to happen
- Update the tolerations of any critical or desired component that should be
  running on the arbiter node.

### MCO Changes

Currently MCO blocks machine config pools that target non master/worker roles in
a [validation
webhook](https://github.com/openshift/machine-config-operator/blob/7c5ae75515fe373e3eecee711c18b07d01b5e759/manifests/machineconfigcontroller/custom-machine-config-pool-selector-validatingadmissionpolicy.yaml#L20-L25).
We will need to update this to allow the arbiter role.

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

`/templates/arbiter/**/kubelet.service.yaml`

```
        --node-labels=node-role.kubernetes.io/arbiter,node.openshift.io/os_id=${ID} \
...
        --register-with-taints=node-role.kubernetes.io/arbiter=:NoSchedule \
```

### Installer Changes

We will need to update the installer to have awareness of the explicit intent to
setup the cluster with an arbiter node. Adding a new machine type similar to the
flow for `master` and `workers` machines. The intent will be to generate 1
`arbiter` machine and 2 `master` machines when the user specifies a
`MachinePool` `install-config.arbiterNode` field.

The `arbiterNode` will be a `MachinePool` object that will enforce 1 replica,
and users will supply 2 replicas for `controlPlane`, when no `arbiterNode` field
is supplied regular flow will validate replica count. When `arbiterNode` is
supplied 2 replicas will be valid for for `controlPlane` as long as an
`arbiterNode` is specified.

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
arbiterNode:
  platform:
    aws:
      type: m6a.xlarge
controlPlane:
  architecture: amd64
  hyperthreading: Enabled
  name: master
  platform:
    aws:
      type: m6a.4xlarge
  replicas: 2
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

### ETCD Operator Change

CEO(cluster-etcd-operator) will need to be made aware of the new arbiter role.
There are some challenges posed by this since CEO uses the node informer for
`master` in a lot of their controllers to manage the etcd cluster. We also have
a challenge in that the `node-role` labels are just empty keys, meaning that a
label selector like `node-role in (master,arbiter)` will not work for
`node-role` since we always just match on the key's existence. Furthermore,
labels selectors by design do not support an `or` operation on keys
([see](https://github.com/kubernetes/kubernetes/issues/90549#issuecomment-620625847)).
We will need to wrap the informer and lister interfaces to allow much of the
logic not to change for CEO, this does also offer a benefit since it will be
easier to FeatureGate during our TechPreview phase.

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

`/ceohelpers/informers.go`

```go
// New lister for multiple selectors.
func NewMultiSelectorNodeLister(indexer cache.Indexer, extraSelectors ...labels.Selector) corev1listers.NodeLister {
	return &mergedNodeLister{indexer: indexer, extraSelectors: extraSelectors}
}
// New informer for multiple selectors
func NewMultiSelectorNodeInformer(client kubernetes.Interface, resyncPeriod time.Duration, indexers cache.Indexers, selectors ...string) cache.SharedIndexInformer {
	return cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				var filteredNodes *corev1.NodeList
				for _, selector := range selectors {
					options.LabelSelector = selector
					nodes, err := client.CoreV1().Nodes().List(context.TODO(), options)
					if err != nil {
						return nil, err
					}
					if filteredNodes == nil {
						filteredNodes = nodes
					} else {
						filteredNodes.Items = append(filteredNodes.Items, nodes.Items...)
					}
				}
				return filteredNodes, nil
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				var filteredNodeWatchers mergedWatchFunc
				for _, selector := range selectors {
					options.LabelSelector = selector
					nodeWatcher, err := client.CoreV1().Nodes().Watch(context.TODO(), options)
					if err != nil {
						return nil, err
					}
					filteredNodeWatchers = append(filteredNodeWatchers, nodeWatcher)
				}
				return filteredNodeWatchers, nil
			},
		},
		&corev1.Node{},
		resyncPeriod,
		indexers,
	)
}

// Wrap node lister
type mergedNodeLister struct {
	indexer        cache.Indexer
	extraSelectors []labels.Selector
}
func (s *mergedNodeLister) List(selector labels.Selector) (ret []*corev1.Node, err error) {
	err = cache.ListAll(s.indexer, selector, func(m interface{}) {
		ret = append(ret, m.(*corev1.Node))
	})
	for _, selector := range s.extraSelectors {
		err = cache.ListAll(s.indexer, selector, func(m interface{}) {
			ret = append(ret, m.(*corev1.Node))
		})
	}
	return ret, err
}
func (s *mergedNodeLister) Get(name string) (*corev1.Node, error) {
	obj, exists, err := s.indexer.GetByKey(name)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errors.NewNotFound(corev1.Resource("node"), name)
	}
	return obj.(*corev1.Node), nil
}

// Wrap watch func
type mergedWatchFunc []watch.Interface
func (funcs mergedWatchFunc) Stop() {
	for _, fun := range funcs {
		fun.Stop()
	}
}
func (funcs mergedWatchFunc) ResultChan() <-chan watch.Event {
	out := make(chan watch.Event)
	for _, fun := range funcs {
		go func(eventChannel <-chan watch.Event) {
			for v := range eventChannel {
				out <- v
			}
		}(fun.ResultChan())
	}
	return out
}
```

### Workflow Description

#### For Cloud Installs

1. The user creates an `install-config.yaml`.
2. The user defines the `install-config.controlPlane` field with `2` replicas.
3. The user then enters the new field `install-config.arbiterNode` defines the
   arbiter node.
4. The user generates the manifests with this install config via the
   `openshift-install create manifests`
5. The installer creates a new `arbiter` MachineSet with a replica of 1 and
   checks the control plane replicas is `2` or higher
6. With the object `arbiterNode` in the install config, the installer creates
   the machine object with the correct node labels and machine config resources
   for ignition bootstrap files.
7. The installer applies the taint to the arbiter MachineSet
8. The user can make any alterations to the node machine type to use less
   powerful machines.
9. The user then begins the install via `openshift-install create cluster`

#### For Baremetal Installs

1. The user creates an `install-config.yaml` like normal.
2. The user defines the `install-config.controlPlane` field with `2` replicas.
3. The user then enters the new field `install-config.arbiterNode` defines the
   arbiter node.
4. The user then enters the machine information for
   `install-config.controlPlane.platform.baremetal`
5. The installer creates a new `arbiter` MachineSet with a replica of 1 and
   checks the control plane replicas is `2` or higher
6. With the object `arbiterNode` in the install config, the installer creates
   the machine object with the correct node labels and machine config resources
   for ignition bootstrap files.
7. The installer applies the taint to the arbiter MachineSet
8. The user can make any alterations to the node machine type to use less
   powerful machines.
9. The user then begins the install via `openshift-install create cluster`

#### During Install

1. The CEO will watch for new masters and the arbiter role
2. CEO will create the operand for the etcd deployments that have tolerations
   for the arbiter
3. Operators that have tolerations for the arbiter should be scheduled on the
   node
4. The install should proceed as normal

### API Extensions

The `installConfig` will include a `install-config.controlPlane.arbiterNode`
bool flag to denote arbiter infra structure.

### Topology Considerations

#### Hypershift / Hosted Control Planes

At the time being there is no impact on Hypershift since this edge deployment
will require running the control plane.

#### Standalone Clusters

This change is relevant to standalone deployments of OpenShift at the edge or
datacenters. This enhancement specifically deals with this type of deployment.

#### Single-node Deployments or MicroShift

This change does not effect Single Node or MicroShift.

### Implementation Details/Notes/Constraints

Currently there are some behavior unknowns, we will need to put out a POC to
validate some of the desires in this proposal. In it's current version this
proposal is not exhaustive but will be filled out as we implement these goals.

We currently expect this feature to mainly be used by `baremetal` installs, or
specialized hardware that is built to take advantage of this type of
configuration. In the current design we make two paths for cloud and baremetal
installs in the installer. However, the cloud install is primarily for testing,
this might mean that we simplify the installer changes if we are the only ones
using cloud installs, since we can simply alter the manifests in the pipeline
with out needing to change the installer.

### Risks and Mitigations

The main risk in this enhancement is that because we are treating one of the
master nodes in a 3 node cluster as an arbiter, we are explicitly evicting
processes that would otherwise be a normal supported upstream configuration such
as a compact cluster. We run the risk of new components being critical to HA not
containing the proper tolerations for running on the arbiter node. One of the
mitigations we can take against that is to make sure we are testing installs and
updates.

Another risk we run is customers using an arbiter node with improper disk speeds
below that recommended for etcd, since etcd is sensitive to latency between
members, we should provide proper guidance so that the arbiter node must meet
minimum requirements for ETCD to function properly. ([ETCD
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

## Test Plan

- We will create a CI lane to validate install and fail over scenarios such as
  loosing a master or swaping out an arbiter node.
- CI lane for e2e conformance testing, tests that explicitly test 3 node masters
  will need to be altered to accommodate the different topology.

## Graduation Criteria

### Dev Preview -> Tech Preview

N/A

### Tech Preview -> GA

- More testing (upgrade, downgrade, scale)
- E2E test additions for this feature
- Sufficient time for feedback
- Available by default
- Backhaul SLI telemetry
- Document SLOs for the component
- Conduct load testing
- User facing documentation created in
  [openshift-docs](https://github.com/openshift/openshift-docs/)

Before going GA we need to have a proper idea of the role that the
[control-plane machine-set
operator](https://github.com/openshift/cluster-control-plane-machine-set-operator/tree/main/docs/user#overview)
plays in this type of configuration. In TechPreview this will be made `InActive`
but it's role should be well defined for the `arbiter` before going GA.

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
