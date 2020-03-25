---
title: restart-metrics
authors:
  - "@rphillips"
reviewers:
  - "@smarterclayton"
  - "@sjenning"
  - "@mrunalp"
approvers:
  - "@brancz"
  - "@bparees"
  - "@squat"
  - "@s-urbaniak"
  - "@metalmatze"
  - "@paulfantom"
  - "@LiliC"
  - "@pgier"
  - "@simonpasquier"
creation-date: 2020-03-23
last-updated: 2020-03-23
status: implementable
see-also:
replaces:
superseded-by:
---

# Service Restart Metrics

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

There is not any insight with systemd units restarting. Metrics of this sort are
important to know if Kubelet, crio, or other system services are crashing and
restarting. Restart metrics are going to be extremely important since the
Kubelet is going to be changed to exit on a crash, thus restarting. Previous
behavior of the Kubelet is to recover() from the crash and not exit.

## Motivation

Systemd unit restart metrics of a unit are vitally important for system
administrators to see the health of the Kubelet, crio, and other vitally important
system services.

### Goals

- Systemd unit restart metrics need to be propogated through to monitoring, and
  the alerting system to alert system administrators to problems within the
  system.

### Non-Goals

- Defining alerts is not in the scope of this proposal. Alerts can be defined
  once we see some sample cluster data.

### Current Behavior

node_exporter is running in a non-privileged container and cannot connect to the
DBUS socket to communicate with systemd because of an selinux denial. Monitoring
has concerns the underlying [systemd
collector](https://github.com/prometheus/node_exporter/blob/master/collector/systemd_linux.go)
is not performant enough.

## Proposal

- This proposal is to add a privileged sidecar running systemd_exporter
  [systemd_exporter](https://github.com/povilasv/systemd_exporter) to the
  node_exporter [daemonset](https://github.com/openshift/cluster-monitoring-operator/blob/master/assets/node-exporter/daemonset.yaml#L17).

- systemd_exporter will be configured to write metrics to node_exporters
  text_collector.

- A whitelist will be enabled in systemd_exporter to enable metrics from the
  following core services, using the built in command-line argument
  ([collector.unit-whitelist](https://github.com/povilasv/systemd_exporter/blob/master/systemd/systemd.go#L25)):
  - kubelet
  - crio
  - sshd
  - chronyd
  - dbus
  - getty
  - irqbalance
  - NetworkManager
  - rpc-statd
  - rpcbind
  - sssd
  - systemd-hostnamed
  - systemd-journald
  - systemd-logind
  - systemd-udevd

### Risks and Mitigations

- Node team will own the systemd_exporter image. The daemonset that configures
  and deploys the image will be owned by the cluster-monitoring-operator.

## Design Details

- Explained above

### Test Plan

- Validate we see system dbus metrics from the whitelisted services

### Graduation Criteria

Delivered in 4.5.

### Upgrade / Downgrade Strategy

N/A

### Version Skew Strategy

N/A

## Implementation History

## Drawbacks

## Alternatives
