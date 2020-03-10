---
title:reporting-baremetal-hardware-state 
authors:
  - "@sadasu"
reviewers:
  - "@hardys"
  - "@enxebre"
  - "@dhellmann"
  - "@stbenjam"
  - "@juliakreger"

approvers:
  - "@enxebre"
  - "@dhellmann"

creation-date: 2020-03-05
last-updated: 2020-03-05
status: provisional|implementable|implemented|deferred|rejected|withdrawn|replaced
see-also:
  - "/enhancements/baremetal/continuous-hw-introspection.md"  
---

# Adding BareMetal Host Hardware Information to Prometheus Metrics 

## Release Signoff Checklist

- [*] Enhancement is `implementable`
- [*] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Open Questions [optional]

Prometheus node-exporter can be augmented to perform this function?

## Summary

This proposal talks about how hardware data collected from each master and worker
node in a Baremetal IPI deployment would be intergrated into the monitoring stack.
Information about the collection of hardware capabilities from all nodes in a "metal3"
cluster can be found in [1].
 
## Motivation

In a Baremetal IPI deployment it is important to monitor every baremetal server
for any changes in hardware and its capabilities. Towards that end, hardware
introspection needs to be performed on each of the baremetal hosts periodically as
detailed in [1] and this information is used to populate the BareMetal Host CR
corresponding to each baremetal host [2]. This data collected from each server
needs to be provided to Prometheus in addition to any appropriate alerts indicating
a change in hardware status.

### Goals

The goal of this proposal is to specify how hardware data about barametal servers
could be reported in the monitoring stack.

### Non-Goals

Augmenting any existing component that collects hardware data to include values
currently being collected by the service that performs continuous hardware
introspection and updated the BareMetal Host CRD.

## Proposal

It is important to report the current hardware status of baremetal servers in
an Baremetal IPI deployment. The machine-api-operator currently is responsible
for deploying the DaemonSet that collects this information and updates the
BareMetal Host CRD [2]. Both these belong to the openshift-machine-api namespace
and hence can be integrated with Prometheus.

The machine-api-operator already implements a service-monitor for this namespace.
So, this proposal will just focus on the details of implementing a Prometheus
Collector interface to report current hardware status. In addition, this proposal
lists alerts that could be generated with this data.

### User Stories [optional]

1. As a user, I would like to know the current hardware status and capabilities of all
the baremetal servers in my Baremetal IPI deployment.

2. As a user, I would like to receive alerts if any hardware has been swapped out and
if the configuration has changed.

### Implementation Details/Notes/Constraints [optional]

Implementing prometheus.collector

Expose different values for the hardware data collected from each server. Most of the
data collected cannot be categorized as metrics so all the collector would do is to 
report current values.

Generating Alerts

The current data collected from the servers gives a hardware inventory of the server.
Using these collected values it is possible to determine if a particular hardware has
been removed or replaced with a different one. Generate alerts when there is a change
in hardware inventory within a server.

### Risks and Mitigations

Low security risk.

## Design Details

### Test Plan

To completely test this feature e2e, make sure the following occur successfully:

1. A DaemonSet "metal3-daemon" is running on all master and worker nodes.
2. A BareMetal CR for the worker node contains "Hardware" information in the "Status"
field of the CR.
3. Prometheus displays the current hardware state of the master and worker nodes.
4. Manually change the IP address of a NIC on one of the servers and see if an alert
is generated.
 
### Upgrade / Downgrade Strategy

In the current release no changes are being made to the BareMetal Host CRD as
part of this proposal. So, the only change between releases 4.4 and 4.5 pertaining
to this feature are to do with the integration with Prometheus.

If upgraded to 4.5, then the user would notice a new set of hardware inventory
and status data being reported for all baremetal servers in a Baremetal IPI
deployment.
When downgraded to 4.4 from 4.5, this data in the CR would become stale and there
would also be no way for Prometheus to report this data or generate alerts. 

### Version Skew Strategy

During upgrades and downgrades it is possible that the version of machine-api-operator
could end up being out-of-sync with the version of baremetal-operator for a brief
period.

If the machine-api-operator is upgraded before the baremetal-operator, for that small
duration, the machine-api-operator would read stale data from BaremMetal Host CR. This
stale data corresponds to the hardware data collected from the server at the time it
was provisioned. This will be corrected as soon as the baremetal-operator is also updated.

## Implementation History

Implementation for the DaemonSet that runs the hardware introspection service on each
master and worker node is a pre-requisite for this.

[1] - https://github.com/openshift/enhancements/pull/229
[2] - https://github.com/openshift/machine-api-operator/blob/master/install/0000_30_machine-api-operator_08_baremetalhost.crd.yaml
