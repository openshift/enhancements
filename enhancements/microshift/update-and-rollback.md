---
title: update-and-rollback
authors:
  - oglok 
reviewers:
  - "@copejon"
  - "@fzdarsky"
  - "@ggiguash"
  - "@majopela"
  - "@dhellmann"
  - "@zshi-redhat"
  - "@pacevedom"
approvers:
  - "@dhellmann"
api-approvers:
  - None
creation-date: 2023-01-09
last-updated: 2023-01-09
tracking-link:
  - https://issues.redhat.com/browse/USHIFT-518
---

# MicroShift Upgrades and Rollbacks

## Summary

Allow successful upgrades of MicroShift running on Edge Devices and enable
the ability of going back to a previous working state. To support this process,
MicroShift relies on the transactional features of OSTree by atomically
switching between states.

## Motivation

Managing the life cycle of Edge Devices at scale brings certain challenges
regarding configuration and maintenance. By leveraging the features of OSTree for
upgrades and rollbacks, Edge Devices can automatically detect if a new version
of MicroShift has been provided and upgrade accordingly.

This enhancenment covers the mechanisms to save the state of a running device
by performing a backup of all required MicroShift resources and restoring it in
the case of a broken state.

### User Stories

As an application developer, I want to focus on releasing new stable versions of
my application that work in production.

As an application deployer, I want to focus only on the lifecycle management for
my application and forget about how MicroShift is upgraded.

As an application deployer, I can provide new versions of my application as part
of the OSTree, and enforce its upgrade

As a device administrator, I can provide a new version of the Operating System
OSTree and the edge device will upgrade automatically.

As a device administrator, I need to ensure a MicroShift upgrade works and a
rollback mechanism is in place in case something goes wrong.

### Goals

The goals of this proposal are:
- How to implement a successful MicroShift upgrade on systems based on `rpm-ostree`
- Describe the process of creating a MicroShift's data backup
- Identify what files need to be saved as part of a MicroShift's data backup
- Describe the process of rolling back to a previous working state

### Non-Goals

The non-goals of this proposal are:
- How to implement MicroShift healthchecks using `greenboot`. This is covered by
[this](https://github.com/openshift/enhancements/pull/1306) other enhancement proposal
- How to implement upgrades of 3rd party applications running on MicroShift
- How to implement rollbacks of 3rd party applications running on MicroShift
- How to implement rollbacks without rpm-ostree

## Proposal

### Workflow Description

The Operating System of an Edge Device running MicroShift will be created using
Image Builder. This tool will allow `device administrators` to customize an edge
optimized version of RHEL by adding RPMs and configuration of choice as described
[here](https://github.com/openshift/enhancements/blob/master/enhancements/microshift/kubernetes-for-device-edge.md).

When a new version of MicroShift needs to be rolled out, a new image shall be created
and exposed to the edge devices. When working with thousands of Edge Devices,
automatic detection and enforcement of upgrades is required.  Hence, Edge Devices will
automatically detect the new exposed OSTree and perform a transactional upgrade.

Before pivoting to the new Operating System Tree, a data backup needs to be performed
in order to save the state of the running Edge Device. Due to the resource constrained
environments where these Edge Devices run on, only the N-1 state will be supported. That 
means that only one backup will be stored and when the upgrade is performed, the Edge
Device can only rollback to the previous state N-1.

The backup should consider all data required to save the state of MicroShift such as
certificates, configuration files, volumes

At every boot, `Greenboot` healthchecks will be executed as described in [this enhancement.](https://github.com/openshift/enhancements/blob/master/enhancements/microshift/microshift-greenboot.md)

If upgrading MicroShift to a new version results in a broken state, `Greenboot` scripts
will restore the previously saved backup, and perform an atomic rollback to the N-1 state.

It is important to highlight that MicroShift's upgrade and rollback processes need to
fit well into the mechanisms provided by the OSTree based version of Red Hat Enterprise
Linux, but it will not own any of these responsabilities. It is the `device administrator`
duty, to decide the upgrade/rollback workflow end to end.

### API Extensions

None

### Implementation Details/Notes/Constraints [Optional]

#### rpm-ostree Configuration

`rpm-ostree` provides a configuration option to check and stage new updates.
The file `/etc/rpm-ostreed.conf` provides `AutomaticUpdatePolicy` which defaults to `none`.

This option needs to be set to `stage` which downloads and unpacks the update performing
any package layering. Only a small amount of work is left to be performed at shutdown time
via the `ostree-finalize-staged.service` systemd unit.

Furthermore, in order to apply this policy, the `rpm-ostreed-automatic.service` systemd
service must be enabled.

This configuration is explained in the official documentation for RHEL 9. It can be found
[here](https://access.redhat.com/documentation/en-us/red_hat_enterprise_linux/9/html/composing_installing_and_managing_rhel_for_edge_images/managing-rhel-for-edge-images_composing-installing-managing-rhel-for-edge-images#enabling-rhel-for-edge-automatic-download-and-staging-of-updates_managing-rhel-for-edge-images).

This step is not strictly part of MicroShift's domain. However, in order to manage edge
devices at scale, this configuration is recommended.

#### MicroShift's Upgrade Systemd Service

A new systemd service file will be packaged into the MicroShift's main RPM. This systemd
service will monitor the `/run/ostree/staged-deployment` file that stores the staged update
deployment info by using the `"ConditionPathExists"` option. This condition will be the
trigger to run a custom script that saves the MicroShift state into a data backup file.

#### MicroShift Backup

Leveraging the ability to run any custom script before the new OSTree deployment becomes
active, allow us to perform the following tasks in order:
* Stop MicroShift systemd service (which does not cause interruption for applications)
* Calculate an estimate of storage space to avoid getting the device out of disk
* Save a tarball file of `/var/lib/microshift` to `/var/lib/microshift-backup`
* Save snapshots of any persistent information store in volumes
* Reboot to get the new OSTree deployment enabled

Note: MicroShift is not responsible for rebooting the system. It is the device
administrator's duty to design the upgrade and rollback workflow and decide whether the
reboot must be inmediate or schedule to a maintenance window.

#### MicroShift Rollback

`Greenboot` will perform different checks to verify if the Edge Device is in a healthy state.
If the result is declared as successful, MicroShift will run normally until a new OSTree
version is exposed to the device. If the result is declared as failed, `Greenboot` will
run custom scripts that will perform the following tasks in order:
* Stop MicroShift systemd service
* Restore all data from `/var/lib/microshift-backup` directory
* Execute `rpm-ostree rollback -r` in order to deploy the previous N-1 state

### Risks and Mitigations

MicroShift does not run the Cluster Version Operator to handle upgrades, and it relies
on OSTree capabilities for transactional updates and rollbacks. There could be occasions
when specific tasks are required to perform a successful upgrade of certain components
such as deprecated API removal, ETCD major versions bump, etc. In order to mitigate this,
every MicroShift version will have a specific custom upgrade script to jump to the next
version.

MicroShift is meant to run on Edge Devices with small footprint. Storing backups could
lead the device to run out of disk space. To mitigate this, we will only support only
N-1 states, which translates into saving only the backup of a previous state and not
several of them.

As mentioned in the MicroShift Greenboot enhancement, if network connectivity is slow or
unstable, it can lead to a failed state which will end up in a rollback. In this case,
increasing timers or reinstalling the upgrade when network improves will be necessary.

MicroShift depends on OpenVswitch for its networking infrastructure. The ovn-master should
maintain the database schema in sync with the specific OVN version. However, the OVN
database is regenerated from Kubernetes API server everytime `ovnkube-master` pod is
restarted, so there is no need to save it as part of MicroShift's data backup.

### Drawbacks

The way MicroShift is upgraded differs a lot from clustered OpenShift. The lack of Cluster
Operators allow the small footprint of MicroShift while it prevents a more standard way of
managing the lifecycle of this Kubernetes runtime.

## Design Details

### Open Questions

* Do we need to support MicroShift different z-stream to the next following major
version?
* How would MicroShift's backups be performed? Embedded service or external script/tool?
* Do we need to stop running workloads when an upgrade is triggered?
* Is there a need to implement post-boot scripts in case of success or failure?

### Test Plan

We will test upgrade procedure in CI per commit, from main branch to PR.
We will test rollback procedure in CI per commit, from PR to main branch.
We will have automated QE pipelines for upgrades and rollbacks among different versions.

### Graduation Criteria

#### Dev Preview -> Tech Preview

N/A

#### Tech Preview -> GA

- Sufficient test coverage
- Gather feedback from users

#### Full GA

- Available by default

#### Removing a deprecated feature

N/A

### Upgrade / Downgrade Strategy

This enhancement is all about upgrades and downgrades.

### Version Skew Strategy

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

N/A
