---
title: single-node-openshift-day2-workers
authors:
  - "@omertuc"
reviewers:
  - "@romfreiman"
  - "@eranco74"
  - "@tsorya"
  - TBD
approvers:
  - TBD
api-approvers: # in case of new or modified APIs or API extensions (CRDs, aggregated apiservers, webhooks, finalizers)
  - TBD
creation-date: 2021-01-05
last-updated: 2021-01-05
tracking-link: # link to the tracking ticket (for example: Jira Feature or Epic ticket) that corresponds to this enhancement
  - https://issues.redhat.com/browse/MGMT-8414
see-also:
  - "https://github.com/openshift/enhancements/tree/master/enhancements/single-node"
  - "https://github.com/openshift/enhancements/blob/master/enhancements/installer/single-node-installation-bootstrap-in-place.md"
replaces:
superseded-by:
---

# Single Node OpenShift Day-2 Workers

## Summary

This enhancemnet aims to enable adding workers to a single-node cluster by
dealing with a "floating ingress" issue encountered "none"-platform single
control-plane node clusters which have worker nodes added to them. It does so
by adjusting the installer to pin the default `IngressController` to the master
pool when installing single-node "none"-platform clusters.

## Motivation

There's been recent demand from OpenShift to support adding workers to existing
single-node clusters.

This is currently easily done on (unsupported) cloud IPI-deployed single node
clusters, by increasing the replica count of worker machinesets. Everything
there works as expected.

Even on Assisted Installer installed Single Node clusters it's trivial to add
more workers by leverging the Assisted Installer's day-2 worker installation
capabilities (after some minor DNS configurations issues which will be improved
by the Assisted-Installer team, separately from this enhancement).

The issue is that on "none"-platform single-node clusters, when adding workers,
the resulting cluster has a major issue that will be made clear in the
paragraphs below.

One of the benefits of installing single-node clusters is the simplicity of not
having to deal with load-balancing and virtual IPs, as these don't provide much
value when there's only a single node behind them.

As a result, current common ways of installing Single Node OpenShift today
(mainly the Assisted Installer) avoid the usage of load balancers or virtual
IPs for API and ingress.

A user installing Single-Node OpenShift on "none"-platform will be tempted to
simply point their DNS entries directly at the IP address of the single node
that they just installed.

Similarly, in the Assisted Installer, the user is able to complete the
installation without needing to define any DNS entries. This is currently
achieved by injecting a dnsmasq systemd service file into the Assisted
Installer's discovery ISO and also inside an installer `MachineConfig` manifest
targeted at the "master" pool. The node is then configured using
`/etc/resolv.conf` to use that dnsmasq server for DNS resolution. The dnsmasq
server is configured with DNS entries for `api.<cluster>.<base>`,
`api-int.<cluster>.<base>` and `*.apps.<cluster>.<base>` which all point to the
node's own IP address. This allows the installation process and the resulting
cluster to conveniently work without the user having to think about and
configure DNS (of course external access to the cluster requires the user to
configure DNS, but this can be done after the installation has completed).

The issue with that approach is that it assumes that `*.apps.<cluster>.<base>`
should always point at the single control-plane node's IP address. This is
of-course correct when there's just a single node, but once you start adding
worker nodes to the cluster it starts causing a potential problem - the
`router-default` deployment created by the Cluster Ingress Operator, which is
responsible for load balancing ingress traffic, targets the "worker" pool using
a node selector. As a result, under some circumstances, the deployment's pods
may find themselves on the newly added worker nodes, as those also belong to
the worker pool.

This is a problem because ingress traffic has to be directed at the node
running the `router-default` pods, and since the DNS entries have been
"naively" pointed at the original control-plane node which may no longer be
running those pods, ingress traffic can no longer work. This is until DNS is
adjusted (solving the problem temporarily) or a load-balancer / some virtual IP
solution is put in place.

We would like to avoid this complication.

### Goals

- Enable expansion of Single Node clusters by ensuring ingress traffic can always
  be directed at the single control-plane node successfully, even when new workers
  are added.

### Non-Goals

- Deal with expansion of the single-node control-plane by adding more control-plane nodes
- Deal with expansion of clusters that have been installed before the implementation
  of this enhancement (they can be take ncare of with a simple documentation on how it can be
  enabled with a few manual steps).
- Deal with "day 1" UPI installation of single-node control plane clusters
  that have additional non-control plane worker nodes
- Deal with cloud IPI installations of Single Node OpenShift. They do not suffer
  suffer from the floating ingress issue as they're using a cloud provided load
  balancer which is automatically configured (using a LoadBalancer type Service)
  to follow the nodes currently holding the router-default pods.
- Adjust Assisted Installer to use the baremetal platform for single-node installations.
  The baremetal platforms solves this issue with virtual IP addresses/keepalived.
- Deal with users who want their ingress traffic to not go through the single
  control-plane node.

## Proposal

Modify the installer's behavior during Single Node "none"-platform installation
to create an `IngressController` CR installer manifest targetting control-plane
nodes rather than worker nodes, which will in turn prevent the OpenShift
Ingress Operator from creating the default `IngressController` CR which targets
worker nodes. This will ensure that the `router-default` deployment created by
the OpenShift Cluster Ingress Operator will always run on the single control
plane node, and as a result any `*.apps.<cluster>.<base>` DNS entries which
originally pointed at the single control plane node will remain correct even in
the face of newly added worker nodes.

This is made possible due to [this](https://github.com/openshift/enhancements/blob/4938b44dc0373a032cae9a48dbe0f86e06f8a189/enhancements/ingress/user-defined-default-ingress-controller.md) previous enhancement.


### User Stories

- As an OpenShift Single Node administrator, I want to add worker nodes to my
  single-node cluster, so that it'll be able to handle growing computation demands.

### API Extensions

This enhancement does not modify/add any API

### Implementation Details/Notes/Constraints

This enhancement can be easily implemented by adjusting the installer's
`generateDefaultIngressController` method such that when `config.platform.none
!= nil`, it will return an `IngressController` CR struct with
`.spec.nodePlacement.nodeSelector.matchLabels` targetting
`node-role.kubernetes.io/master`. 

### Risks and Mitigations

This should make no noticable difference on "regular" single-node installations
which do not go through expansion (as the default node selector targets the
worker pool, and the single control plane is already both in the master and
worker pools), but it will fix the issue described above for single-node
installations which do go through expansion.

Users who *want* their traffic to not go through the single control-plane node
can still do so by following the [existing](https://docs.openShift.com/container-platform/4.9/machine_management/creating-infrastructure-machinesets.html#moving-resources-to-infrastructure-machinesets) OpenShift documentation on moving
infrastructure workloads to particular nodes.

I do not believe this enhancement has any security implications.

## Design Details

### Open Questions

None that I can think of at the moment

### Test Plan

- Add unit tests to the installer to make sure the IngressController manifest
  is generated as exepected.

- Add periodic nightly tests which install single-node in the cloud, add
  a few worker nodes to it, then run conformance tests to make sure we don't
  run into any problems not described in this enhancement.

- Add periodic nightly tests which install a single-node "none"-platform
  cluster, add worker nodes to it, and check that ingress traffic still works as
  expected and recovers even after the `router-default` pod gets deleted and
  rescheduled. Make sure this is still true even after upgrades.

- Add tests on both cloud / "none"-platform that check that a single-node
  cluster with additional workers recovers after the single control-plane node
  reboots by running conformance tests post-reboot.

- Add tests on both cloud / "none"-platform that check that a single-node
  cluster with additional workers recovers after an upgrade by running
  conformance tests post-upgrade.

### Graduation Criteria

**Note:** *Section not required until targeted at a release.*

TBD

#### Dev Preview -> Tech Preview

TBD

#### Tech Preview -> GA

TBD

#### Removing a deprecated feature

N/A

### Upgrade / Downgrade Strategy

In the non-goals section it's mentioned that this enhancement does not apply to
clusters which have been installed prior to the enhancement, so their upgrade
is not discussed.

This enhancement, to the best of my knowledge, should have no problems persisting
across any type cluster upgrades. The Test Plan section describes how this will be
tested.

### Version Skew Strategy

Does not apply, to the best of my understanding.

### Operational Aspects of API Extensions

This enhancement does not modify/add any API

#### Failure Modes

This enhancement does not modify/add any API

#### Support Procedures

This enhancement does not modify/add any API

## Implementation History

Not yet applicable

## Drawbacks

- This is another change which would make single-node clusters slightly different from
  multi-node clusters, and any such difference is naturally not ideal.

- The installer maintainers would have to make sure that the default
  `IngressController` resource generated for single-node bootstrap-in-place
  installations is similar enough to the `IngressController` generated by the
  Openshift Cluster Ingress Operator by default in the absence of an existing
  resource. i.e. our goal here was to simply change the node selector label
  from "worker" to "master", but to do that we had to create an entire
  `IngressController`, one that does not get merged with the default one that
  would've been created by the OpenShift Cluster Ingress Operator otherwise.
  Any future important additions to the default `IngressController` created by
  the OpenShift Cluster Ingress Operator would also have to be added to the
  installer code where it generated the manifest for single-node
  bootstrap-in-place installations.

- TBD

## Alternatives

- Implement this change in the OpenShift Cluster Ingress Operator rather than
  the installer. It wouldn't be enough for the Ingress Operator to simply inspect
  the control-plane and infrastructure topology fields, because those have the
  same value whether you're on a cloud platform (which doesn't suffer from the
  problems described in this enhancement) or a "none"-platform installation.
  Maybe it could also look at the "platform" field in the infrastructure CR, then
  only do this if the platform is "none". Doing this in the OpenShift Cluster
  Ingress Operator would allow us to avoid the second drawback bullet mentioned
  above. Otherwise, a choice between this alternative and doing it in the
  installer seems rather arbitrary.

- Adjust the "baremetal" platform to support single-node installations. The
  baremetal platform solves the issue described in this enhancement with
  virtual IP addresses/keepalived. This approach was dismissed due to much
  higher development efforts and additional processes that would need to run on
  the already resource constrained single control-plane node. Furthermore, even
  once the baremetal platform is adjusted to support single-node clusters, the
  Assisted Installer which is currently the main supported way with which users
  install Single Node Openshift would have to go through a lot of development
  effort in order to make it use the baremetal platform rather than the "none"
  platform currently used for single node installations. This may happen in the
  future.


## Infrastructure Needed [optional]

N/A
