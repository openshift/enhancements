---
title: ovn-kubernetes-ga
authors:
  - "@pecameron"
  - "@squeed"
  - "@knobunc"
  - "@dcbw"
reviewers:
  - TBD
approvers:
  - TBD
creation-date: 2019-09-19
last-updated: 2019-09-19
status: implementable
see-also: []
replaces: []
Superseded-by: []
---

# OVN-Kubernetes

## Release Signoff Checklist
- [X] Enhancement is `implementable`
- [X] Design details are appropriately documented from clear requirements
- [X] Test plan is defined
- [X] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift/docs]

## Summary
Makes OVN-Kubernetes production-ready for OpenShift’s use-cases.

### Motivation
OVN centralizes engineering efforts between teams, as well as allowing us
to iterate much more quickly for advanced networking features.

OVN-Kubernetes allows us to engage an existing upstream community, rather
than developing an openshift-specific network plugin in isolation.

## Proposal
OVN-Kubernetes has been in production in non-OpenShift clusters for some
time. It consists of a central controller, a data store, and individual
node-level processes.

The central controller has two components: the ovn-kubernetes controller,
which reconciles Kubernetes objects in to logical OVN objects, and
ovn-northd, which converts logical objects in to flows.

The data store is a replicated database, with a traditional master-backup
architecture.

The node-level process (ovn-southd) is responsible for programming the
flows from ovn-northd in to the vswitch on the local node. It also handles
some bookkeeping.

#### Controller
There will be a controller daemonset on every master node. They will
perform leader election directly via the Kubernetes API. The ovn-kubernetes
process will, when it achieves leadership, will be responsible for
starting the ovn-north process.

#### Data store
We will write a small controller that, based on the result of the leader
election process, configures the databases (ovn-ndbd and ovn-sbdb) as
master or backup. The master will be distributed via an endpoint. If a
node is backup, it will watch for endpoint changes and update replication
configuration as fit.

#### Node-level
The node-level components are configured as a daemonset. They run on all
nodes, including the control-plane. They watch for endpoint changes,
watch the sbdb for changes, and configure the nodes.

### Test Plan
OVN-Kubernetes must pass all standard QE tests, as well as the full e2e
suite. It does not need any special testing, as the GA release does not
enable any new features.

### Graduation Criteria
GA:
- OVN-Kubernetes is stable
- The control-plane is low-latency: changes are installed on the nodes in under 10 seconds
- Reliable control plane: losing a master node does not cause more than a few seconds of extra latency, and no loss of network connectivity
- Reliable node: Upgrading ovn-kubernetes components

Default: For OVN-Kubernetes to be the default in new installations, it
should implement all openshift-sdn-only features (that we do not wish
to deprecate).

### Upgrade / Downgrade Strategy
This is ultimately a version-skew question. We will not support converting
existing openshift-sdn clusters at this time.

### Version Skew Strategy
OVN-kubernetes is not particularly kubernetes-version sensitive. The
OVN team does perform upgrade tests between minor versions. We will have
to make sure their test plan matches how we intend to upgrade.

In a rolling update some nodes will have the current version and some
will have the new version. Since all nodes interact with the master,
version skew can occur between nodes.

## Implementation History
- 3.11 dev preview
- 4.1 n/a
- 4.2 tech preview
- 4.3 GA

## Drawbacks

## Alternatives
There are many. But we’ve been involved in the OVN and OVN-Kubernetes
communities for a long time. We have the expertise, and we think it’s
worth the investment.

