---
title: local-device-health-monitoring
authors:
  - "@rohantmp"
reviewers:
  - "@jan--f"
  - "@simonpasquier”
  - "@sp98" 
  - "@dhellman"

approvers:
  - "@simonpasquier”
  - "@dhellman"
creation-date: 2021-07-16
last-updated: 2021-09-01
status: implementable
see-also:
replaces:
superseded-by:
---

# Local Device Health Monitoring

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [x] Design details are appropriately documented from clear requirements
- [x] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

This enhancement proposes that we export health metrics for each local device on each node.

## Motivation

Local device vendors expose health information about storage devices via APIs like [SMART](https://en.wikipedia.org/wiki/S.M.A.R.T.). It very useful to know when a storage is about to fail, to differentiate between storage hardware related errors vs purely software ones, etc.

### Goals

- Expose health information about local storage devices in a way that can generate alerts

### Non-Goals

- Align with sig-storage [health monitoring for persistent volumes](https://kubernetes.io/blog/2021/04/16/volume-health-monitoring-alpha-update/)
  - justification: it is also useful to monitor system drives and CSI only deals with persitent volumes, and migrating to CSI is an extremely long term effort.

## Proposal

[node_exporter](https://github.com/openshift/node_exporter) currently exports per-node metrics, we will use it to collect and export metrics for each device.

### User Stories

- As an admin, I would like to be alerted when a storage device is unhealthy and be linked to remediation steps.
- As an admin, I would like if components (such as OpenShift Data Foundation) could access storage device health and failover faster.

### Implementation Details/Notes/Constraints

Implement a sidecar in the [node_exporter](https://github.com/openshift/node_exporter) component under [Cluster Monitoring Operator](https://github.com/openshift/cluster-monitoring-operator) that will scrape the metrics from the node and export them via the [textfile exporter](https://github.com/openshift/node_exporter/tree/master/text_collectors) (an exporter that scans `.prom` files in the
`/var/node_exporter/textfile` directory and exports them.

The Cluster Monitoring Operator will deploy the sidecar when the platform is Baremetal or None by modifying the `node_exporter` [daemonset](https://github.com/openshift/cluster-monitoring-operator/blob/master/assets/node-exporter/daemonset.yaml). The platform will be detected by querying the `infrastructure` resource.

After that, the maintainance effort will mostly centered on the code the image runs to get health and update its `.prom` file. The logic for this collector can be in a subdirectory of CMO and have it's own OWNERS file allowing the ODF team to own it.

We will be collecting metrics via [libstoragemgmt](https://github.com/libstorage/libstoragemgmt) and calling it via [libstoragemgmt-golang](https://github.com/libstorage/libstoragemgmt-golang/).
If a block device is a partion or logical volume, we will only expose health for the parent device.


### Ownership and Maintenance

- The `node_exporter` daemonset creation is owned by the Openshift-Monitoring team. The PR for modifying the daemonset based on infrastructure to include the sidecar can be contributed by ODF, but Monitoring team will have to review.
- The collector sidecar image and logic will live in a separate repository owned and maintained by the ODF team.
- A new bugzilla subcomponent will be created under openshift-monitoring, but the default assignee will be ODF and ODF will be responsible for triaging the issues.

### Risks and Mitigations

- The golang bindings for libstoragemgmgt need some review and polish.
  - Mitigation: The ODF(OpenShift Data Foundation) team will contribute to and help the RHEL team maintain libstoragemgmt golang

## Design Details

Each device will have a metric `storage_device_health` metric associated with it that has a value of 0,1, or 2. Where 0 is healthy state, 1 is a warning state, and 2 is a failed state. Each metric will be labeled by node, device-name, and device-id (where available).

| storage_device_health value | Meaning |
| --------------------------- | ------- |
| 0                           | Healthy |
| 1                           | Warning |
| 2                           | Failed  |

### Open Questions

### Test Plan

**Note:** *Section not required until targeted at a release.*

- E2E tests will deploy the component and verify that metrics are being emitted per device

### Graduation Criteria

**Note:** *Section not required until targeted at a release.*

**For non-optional features moving to GA, the graduation criteria must include
end to end tests.**

#### Dev Preview -> Tech Preview

- Ability to utilize the enhancement end to end
- End user documentation, relative API stability
- Sufficient test coverage
- Gather feedback from users rather than just developers
- Write symptoms-based alerts for the component(s)

#### Tech Preview -> GA

- More testing (upgrade, downgrade, scale)
- Sufficient time for feedback

#### Removing a deprecated feature

Does not apply

### Upgrade / Downgrade Strategy



### Version Skew Strategy



## Implementation History


## Drawbacks

Because writing files to be read by the textfile collector cannot be done on-demand in response to a request from prometheus, we will have to poll at our own frequency.
However, the frequency can be made tunable.

## Alternatives

- Implement in Local Storage Operator:
  - `LocalVolumeDisocveryResults` already enumerate and refresh information about the disk in a per-node `LocalVolumeDiscoveryResult`, and a [PR](https://github.com/openshift/local-storage-operator/pull/249) is in progress to add metrics to each of the daemonsets that update the CR.
  - `LocalVolumeDiscovery` can be configured via `nodeSelector` and `toleration` to match any set of nodes or all nodes. We could have the metrics be for all nodes or just for the ones that LocalVolumeDiscovery is configured to watch. Whether this should be controlled by the same (singleton) CR is an [open question](#Open-Questions    )

- We *could* create a new component for this.

Disqualified alternatives:
- We considered node-problem-detector, but it doesn't export metrics only sets node conditions and creates events.

## Infrastructure Needed

- New repository for collector image.
- New bugzilla subcomponent with default assignee managed by ODF team.
