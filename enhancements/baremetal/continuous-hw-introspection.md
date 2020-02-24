---
title: Continuous hardware introspecton of Baremetal IPI hosts
authors:
  - "@sadasu"
reviewers:
  - "@dhellmann"
  - "@enxebre"
  - "@hardys"
  - "@stbenjam"
  - "@juliakreger"
approvers:
  - "@dhellmann"
  - "@enxebre"
creation-date: 2020-02-18
last-updated: 2020-02-17
status: provisional|implementable|implemented|deferred|rejected|withdrawn|replaced
---

# Continuous hardware introspection of Baremetal IPI hosts

This proposal talks about how performing continuous hardware introspection on all master
and worker nodes in a Baremetal IPI deployment would get us important data about baremetal
hosts which can be used to monitor changes in hardware capabilities of the server.

This enhancement will not result in a new CRD or API to be written but will detail
data that would become available in the Status field of the BareMetalHost CRD.

## Release Signoff Checklist

- [X] Enhancement is `implementable`
- [X] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

Continuous hardware inspection on all the master and worker nodes that are a part
of a Baremetal IPI deployment, allow the user to detect changes in server hardware
and become aware of any failures seen on the servers. This information can be used
to report current status of hardware to the user and/or glean metrics about the
health of the deployment.

At the end of 4.4, a Baremetal IPI deployment does not have continuous hardware introspection
running on its master or worker nodes. The underlying provisioning service already has the
capability to perform hardware inspection on each of the servers periodically. During 4.5, the
goal is to enable continuous hardware introspection all master and worker nodes by running the
above mentioned service in a DameonSet. Another piece of this solution would be to enable the
baremetal-operator (BMO) to read this information and update the "status" field of the
BareMetalHost CRD. Please refer to [1] for the latest BaremetalHost CRD.

## Motivation

This enhancement will allow the user to get continuous updates on all the baremetal
servers in their Metal3 deployment providing useful information about the hardware
inventory on the servers and the current status of their hardware. This data would
be used to generate alerts regarding the current state of hardware but will not be
used to make any decisions regarding workload placements or application of labels on
these nodes.

This enhancement talks about the infrastructure pieces required to collect data and
the enhancement [5] talks about how this data is integrated with the monitoring stack
to generate alerts regarding hardware state on baremetal hosts managed by metal3.

### Goals

This enhancement will allow the baremetal host CR associated with each baremetal
server in a metal3 deployment to contain up-to-date information about its hardware and
operational status of a server.

### Non-Goals

This proposal does not include any changes to the BareMetalHost CRD.

## Proposal

This enhancement proposes to add a new DaemonSet started by the machine-api-operator
only for the BareMetal platform. This DaemonSet would run a new container image
(ironic-hardware-inventory-recorder) on all master and worker nodes. With the new
addition, it would become possible to have detailed information regarding the
hardware connected to both master and worker nodes in a metal3 deployment and not
just when the worker nodes are provisioned. This also allows the user to keep
track of any changes in hardware capabilities of servers in a metal3 deployment.

Additional details regarding enhancements made to the underlying service required
for this feature can be found in [3].

### User Stories [optional]

#### Story 1

As an administrator tasked with managing the physical infrastructure, I would like to
know the current status of the hardware and inventory data for each of the baremetal
servers in my Baremetal IPI deployment.

#### Story 2

As an administrator tasked with managing the physical infrastructure, I would like to know
of any changes in hardware status and inventory data of each baremetal server while it is part
of my Baremetal IPI deployment.

### Implementation Details/Notes/Constraints [optional]

Continuous hardware introspection involves periodically reading information about baremetal
server hardware by talking to the server directly. This information includes details
about devices connected to each server like its CPUs, NICs and storage devices. The
actual task of hardware introspection is performed by an agent that needs to collect
information on all master and worker nodes and hence needs to run as a DaemonSet.

This agent then periodically pushes data back to a service running with the metal3 pod. The
baremetal-operator, also running within the metal3 pod, needs to poll this service for
hardware introspection data when it is not busy reconciling. The baremetal-operator then
updates the status on their respective BareMetalHost CRs.

Hardware inspection is curently done on the worker nodes once when it is initially registered
with the cluster. This information could get outdated unless this introspection is performed at
regular intervals. Currently, there is no hardware introspection data on the master nodes and
implemnting this feature will give us access to that data. Details of the underlying service
used to perform the continuous hardware inspection can be found at [3]. Information on how
baremetal-operator interacts with this underlying service is detailed in [4].

The machine-api-operator would now be responsible for deploying the DaemonSet that would
run the service that performs continuous hardware introspection on baremetal hosts.The container
image for the DaemonSet can be found at [2]. Also, if for some reason, a previously "ready"
metal3 deployment becomes "unavailable", the DaemonSet can continue to keep running. This
will result in the lastest state of the baremetal servers in the baremetal CRs to
eventually become stale until metal3 transitions to "available"  operational state again.

### Risks and Mitigations

No technical risks forseen for this proposal.

## Design Details

This DaemonSet runs only on master and worker nodes in Baremetal IPI environments managed
by "metal3". The data collected from each baremetal host would be used only to generate
metrics and alerts that a cloud admin can use to get a snapshot of the state of hardware
in that cluster. The proposal to integrate this data with the monitoring stack is [5].

This data cannot and will not be used to make any decisions regarding driving workloads
to nodes. This data also would not be used to detemine placement of any specific labels
on these nodes. The understanding is that this data is collected only in Baremetal IPI
environments managed by metal3 and cannot be used to affect generic outcomes in an
OpenShift installation.

To ensure that the DaemonSet runs the ironic-hardware-inventory-recorder only on nodes
managed by "metal3", the "node-Selector" property would be specified in the pod template,
which is part of the DaemonSet definition. All nodes that would be part of a metal3 cluster
would have the label "metal3-node" set to true. This will ensure that the DaemonSet would
not run on any nodes that have been added to the cluster that are not managed by "metal3".

Since the solution involves sending periodic updates from each node in the cluster, it
has the potential to cause the thundering herd problem when the cluster size is non-trivial.
To mitigate this scenario, the agent running on each node to collect and report hardware
status has been updated to introduce jitter in its periodicity of updates. Implementation
for this enhancement can be found at [6].

With continuous hardware status updates from the agent, there is another potential issue
where the database storing the current hardware status of every node in the "metal3" cluster
gets overwhelmed. To avoid this scenario, the service with access to the database is
designed to hold just the latest status for each node. Every node in the cluster would 
have just one entry in this database.

### Test Plan

Any e2e tests would make sure that the new DaemonSet is running on all the master
and worker nodes in a Baremetal IPI deployment. The tests would make sure that
the hardware status expressed in the BareMetalHost CR is updated periodically indicating
that the different components of the underlying service are communicating with each other.
Lastly, any e2e tests will make sure that the DaemonSet is running even if "metal3"
is not.

### Graduation Criteria

This feature is intended to be in tech preview in 4.5.

### Upgrade / Downgrade Strategy

Upgrade to 4.5 and downgrade from 4.5 are both non-issues because there is no change to the
BareMetalHost CRD to implement this proposal. After a downgrade, the hardware introspection data
will eventually become stale.

### Version Skew Strategy

## Implementation History

[1] - https://github.com/openshift/machine-api-operator/blob/master/install/0000_30_machine-api-operator_08_baremetalhost.crd.yaml
[2] - https://quay.io/repository/openshift/origin-ironic-hardware-inventory-recorder
[3] - https://docs.openstack.org/ironic-inspector/latest/
[4] - https://github.com/metal3-io/metal3-docs/blob/master/design/hardware-status.md
[5] - https://github.com/openshift/enhancements/pull/244
[6] - https://review.opendev.org/#/c/715005/
[7] - https://en.wikipedia.org/wiki/Thundering_herd_problem
