---
title: etcd-as-a-transient-systemd-unit
authors:
  - dusk125
  - hasbro17
reviewers:
  - "@tjungblu, etcd Team"
  - "@Elbehery, etcd Team"
  - "@fzdarsky"
  - "@deads2k"
  - "@derekwaynecarr"
  - "@mangelajo"
  - "@pmtk"
approvers:
  - "@dhellmann"
api-approvers:
  - None
creation-date: 2022-09-16
last-updated: 2022-10-19
tracking-link:
  - https://issues.redhat.com/browse/ETCD-318
#see-also:
#  - "/enhancements/this-other-neat-thing.md"
---

# Decoupling etcd from the MicroShift binary

## Summary

The enhancement proposes moving etcd out of the MicroShift binary and into a transient systemd unit that is launched and managed by MicroShift.

## Motivation

Currently MicroShift [bundles upstream etcd](https://github.com/openshift/microshift/blob/f7f2260e4fbff61654f478fc149cb7052261f87f/go.mod#L21) into its binary and runs it as a goroutine.
This results in etcd dependencies being coupled with MicroShift which makes it harder to maintain and upgrade etcd independently of the platform like it is done with a separate openshift/etcd repo in the Openshift Container Platform (OCP).

This enhancement outlines an update to MicroShift to change how etcd is bundled and deployed with MicroShift in order to minimize risk of shared dependencies between etcd and OpenShift/Kubernetes leading to hard-to-trace bugs.

Changing the deployment of etcd, outlined below, would alleviate concerns about building etcd with a version of shared dependencies - mainly grpc - that etcd is not currently built against, and therefore, not tested against.

Additionally moving the etcd deployment into its own process would make it easier to debug and process logs via journatlctl.

### User Stories

1. "As an etcd developer, I want to maintain etcd versions independently of microshift dependencies."

2. "As a MicroShift Device Administrator, I want to observe and debug the etcd server logs separately from other microshift processes."

### Goals

- The etcd binary is built as a separate binary to ease dependency management and shipped together with MicroShift binary in single RPM to ease deployment process.
- The etcd server logs are easier to observe.

### Non-Goals

- Certificate management for etcd client authentication needs to be updated regardless of the delivery/execution implementation so this will be out of scope for this enhancement.
- Creating an RPM build root specifically for MicroShift-etcd; this should be eventually needed so we're building etcd with the Golang version it's expecting (1.16), instead of the same that MicroShift is built with (1.19).
  - This enhancement discusses this further in the 'etcd Binary from an RPM' section.
  - This work should be done in the future, but is out of scope for this enhancement.

## Proposal
    
### Running etcd as a transient systemd unit
In order to address the concerns brought up in the Motivation section, the MicroShift binary will need to be updated to remove the currently bundled etcd server and replaced with a forked process running under systemd; using the `systemd-run` command. Only the execution of etcd will change - from goroutine to external process - MicroShift's general usage of etcd will not be changed.
    
Additionally, this external process will need to be shutdown when MicroShift exits. In an ungraceful termination of MicroShift, it is possible that the etcd systemd unit will continue execution, so it may be necessary to add code to detect this on MicroShift startup so that another etcd launch is not attempted: this would be a port conflict and cause the second etcd instance to fail to start.
Further investigation is needed to learn if a transient systemd unit's lifecycle can be tied to another non-transient systemd unit (MicroShift) so that if the latter exits ungracefully, the former will be gracefully shutdown.

This etcd systemd unit will also need to be provided with the necessary CA certs and signed key pairs for both server and client authentication. 

There should be a mechanism to allow for running the binary more directly in a development environment; one that potentially bypasses the scenarios below.
    
The actual execution of etcd in the transient systemd unit will also be considered in this enhancement; there are currently two scenarios (in detail below): running the etcd container image under podman, and running the etcd binary installed via an RPM.
    
#### etcd Binary from an RPM
This will lay the etcd binary down into the same directory as the MicroShift binary so that in a development environment a local build of etcd will be used.

The RPM will be built in the same build root as MicroShift as a go submodule. This will resolve the dependency issues, but will cause etcd to be built with an unexpected version of Golang - Upstream/OpenShift etcd expects Go1.16, MicroShift is built with Go1.19.
While this is not ideal, decoupling etcd into its own process to resolve the dependency conflict is the first step and getting the etcd Go build version aligned back to 1.16 can be a future goal.

##### RPM Pros
* This scenario should incur the least amount of non-etcd-related overhead since the binary is being run directly.
* Integration is straight-forward; the etcd rpm is another MicroShift dependency, embedded into the rpm-ostree and distributed the same way as other dependencies.
* Clear, consistent architecture: There is only either RPM-installed content or content hosted on the cluster - no third way of installing/running things.
    
##### RPM Cons
* New plumbing will need to be created to (or potentially existing will need to be updated/refurbished to) build etcd and package it into an RPM; addressing this issue is outside the scope of this enhancement.
* etcd will need to be built with a newer version of Golang (1.19) than is currently expected from upstream (1.16); etcd will be built under the MicroShift build root, locking the Golang version.

### Workflow Description

For the end user, starting and stopping MicroShift (and etcd) won't change due to this enhancement. They will run MicroShift with `systemctl start microshift`; as a part of the MicroShift boot up, etcd will be automatically brought up as well (using `systemd-run etcd`).
The execution lifetime of the etcd service will be tied to that of MicroShift, so it will be automatically stopped if MicroShift is stopped by the user `systemctl stop microshift` and if MicroShift has an unexpected shutdown.
The execution of etcd should be completely transparent to the user; they would be able to see it running under systemd `systemctl status microshift-etcd` and collect logs from it `journalctl -u microshift-etcd`.

#### Variation [optional]

For the developer who wishes to run and debug MicroShift, microshift-etcd will detect that MicroShift is being run locally (not from a systemd unit) and change its execution of etcd to a direct binary execution; it will expect to find the etcd binary in the same directory as the MicroShift binary. This will allow the developer to build and debug both MicroShift and etcd locally.

### API Extensions

N/A

### Implementation Details/Notes/Constraints [optional]

We have agreed on a multi-step approach, starting with the process management change (this enhancements change), then adding etcd, in a go submodule, to the MicroShift repository. Once this is in place, we might move the etcd build out to its own RPM, but that is outside of the scope for this enhancement.
This stepped approach will help keep these fundamental changes more manageable for reviewers, and will help decrease the likelihood of disruption due to these changes.

### Risks and Mitigations

The etcd execution lifetime should be bound to that of MicroShift (through systemd 'BindsTo' property); however, it may be possible that MicroShift exits and etcd is left running.
If this occurs, the user can also bring down etcd with `systemctl stop microshift-etcd`; if we find instances of this happening, we can add a check to the microshift-etcd startup to shutdown a running instance of microshift-etcd, if it exists.

### Drawbacks

We will have a more complicated build and an extra package to manage. We consider those acceptable tradeoffs to be able to deliver etcd fixes quickly and to decouple the dependencies that are causing MicroShift build issues.

## Design Details

We will need to update MicroShift's automated rebase process to include the etcd go submodule as a seperate step.

### Open Questions

None

### Test Plan

**Note:** *Section not required until targeted at a release.*

In addition to standard end-to-end tests, we should also test for the main MicroShift binary dying/being killed and ensuring that etcd also comes down. This could be as simple as starting MicroShift via systemd, getting its pid, killing that pid, checking `systemctl status microshift-etcd` and ensuring it comes down.

Another test could be if etcd has an ungraceful shutdown; do the same steps as above, but using microshift-etcd's pid. Etcd should automatically come back up via the same systemd unit.

### Graduation Criteria

TODO

#### Dev Preview -> Tech Preview

TODO

#### Tech Preview -> GA

TODO

#### Removing a deprecated feature

TODO

### Upgrade / Downgrade Strategy

TODO

### Version Skew Strategy

Currently, there is no need for a version skew strategy as etcd will be built and delivered as a part of the MicroShift RPM, so it will not be possible for the MicroShift and etcd versions to get out of sync.

This may need to be revisited if we build and deliver etcd in its own RPM, but that is out of scope for this enhancement.

### Operational Aspects of API Extensions

N/A

#### Failure Modes

TODO

#### Support Procedures

##### Reading etcd Logs
With this enhancement, etcd logs will no longer show up in the MicroShift log stream, they will be in their own systemd log stream. The user/support can get these logs with `journalctl -u microshift-etcd` or similar command used for MicroShift itself.

##### Backup and Restore
Backup and restore of etcd may be different since there is no Cluster Etcd Operator; in OCP, the CEO is what handles the backup and restore of etcd. However, since etcd will not be running in a container, an admin could just run the `etcdctl snapshot save` command directly and etcd should snapshot like in OCP.

More investigation should be done for what pieces from the etcd [backup](https://github.com/openshift/cluster-etcd-operator/blob/2272cc785bcba7a5b84c015481705e0dbe64cf8c/bindata/etcd/cluster-backup.sh) and [restore](https://github.com/openshift/cluster-etcd-operator/blob/2272cc785bcba7a5b84c015481705e0dbe64cf8c/bindata/etcd/cluster-restore.sh) scripts are needed.

## Implementation History

TODO

## Alternatives

### etcd Logs
If we decide not to run etcd in a transient systemd unit, we'll need to make updates to the etcd Zap logger to have it write out logs in a consistent format to the other modules in MicroShift - currently, etcd writes logs in JSON format.
* If this isn't supported with current etcd logger configuration, we may have to patch our downstream logger to achieve this.

### etcd as Go Plugin
Compile etcd into a go plugin and continue to execute it in a gorountine. This would allow for a separate build chain for the binary, but does not change how etcd runs.

This would not solve the dependency issue as you can still only have one version of each dependency otherwise the symbols would conflict. Also, the plugin would need to be built with the same runtime version of Go as MicroShift.

### etcd Container Image with Podman
This scenario would run the etcd server as a container executed via a `podman` command.
This was rejected and moved to an alternative because it would incur an additional, large, dependency on the system and on the customer.
    
#### Podman Pros
* This would be similar to how etcd is built and shipped for OCP and would reuse the existing build machinery for openshift/etcd.
    
#### Podman Cons
* The current etcd image is too large, ~400MB, either it will need to be shrunk or a new image will need to be created with the bare minimum in it; the etcd binary alone is about 30MB.
* This scenario places a new runtime dependency on podman; it would need to be installed and ready before MicroShift could be used.
* MicroShift and its dependencies (incl. microshift-networking) are deliberately delivered as RPMs, so they natively fit into customers' build pipeline and content delivery for the RHEL4Edge rpm-ostrees they're building. Introducing non-RPM dependencies for MicroShift will complicate this.
* A (small) risk of conflicts when customers use their own management agent for managing Podman workloads.

## Infrastructure Needed [optional]

TODO
