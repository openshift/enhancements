---
title: containerize-tuned
authors:
  - "@yanirq"
  - "@jmencak"  
reviewers:
  - "@MarSik"
  - "@ffromani"
  - "@Tal-or"
  - "@browsell"
  - "@bwensley"
  - "@swatisehgal"
  - "@mrunalp"  
approvers:
  - "@MarSik"
  - "@browsell"
  - "@mrunalp"
api-approvers:
  - "None"
creation-date: 2024-03-11
last-updated: 2024-03-14
tracking-link:
  - https://issues.redhat.com/browse/PSAP-1236
---

# Containerize Tuned

## Summary

The proposed enhancement is to restructure cluster-node-tuning-operator (NTO) to run TuneD from a container (e.g. using podman from a systemd service) instead of running in daemon mode so TuneD will initially set defaults and hand off ownership to other services that apply tunings.

## Motivation

With the current implementation of [cluster-node-tuning-operator](https://github.com/openshift/cluster-node-tuning-operator) (NTO) having TuneD running in a daemonset there is an overarching issue where we have multiple services manipulating tuning data - tuned, crio and some system services for irqs.
Since the order of setting tunings is not guaranteed with the current implementation of TuneD there are scenarios where the TuneD tunings are applied too late (especially on restarts) causing tuning discrepancies.

This is manifested already several issues: 

* Order of pods start up ([OCPBUGS-26401](https://issues.redhat.com/browse/OCPBUGS-26401)) : In case of node reboot, all pods restart again in random order. Since there's no control on pod restart order, it is possible that TuneD pod will start after the workloads. This means the workload pods start with partial tuning which can affect performance or even cause the workload to crash. One of the reasons being partial tuning and/or dynamic tuning done by crio hooks is undone.

* Tuned restarts / reapplication ([OCPBUGS-26400](https://issues.redhat.com/browse/OCPBUGS-26400) , [OCPBUGS-27778](https://issues.redhat.com/browse/OCPBUGS-27778) , [OCPBUGS-22474](https://issues.redhat.com/browse/OCPBUGS-22474).

* Tuning changes impact running applications ([OCPBUGS-28647](https://issues.redhat.com/browse/OCPBUGS-28647))

* Historical issues:
  * Tuned overwriting IRQBALANCE_BANNED_CPUS ([OCPBUGSM-46474](https://issues.redhat.com/browse/OCPBUGSM-46474))
  * Tuned affining containers to house keeping cpus ([OCPBUGSM-31773](https://issues.redhat.com/browse/OCPBUGSM-31773))


### User Stories

No new user stories added in this EP.

### Goals

* Tuning integrity is kept and not corrupted when node restarts occur
* Tuning integrity is kept and not corrupted when pod restarts occur
* Tuning will be done as “one shot” to the majority of use cases

### Non-Goals

* Centralize ALL tuning into NTO 
  * Move all tuning currently delegated to MCO into TuneD
  * Considered as best effort for this proposal
* Complete removal of TuneD daemon

## Proposal

The proposal is to change the way TuneD is currently running and apply tunings which are done in daemon mode (daemonset) to a containerized manner where TuneD will be run via podman from a systemd service.
This enhancement will remove the main issue of reapplying tunings on restart events which can cause inconsistencies to tuning data since there are other elements such as crio and other system services that also apply tunings and there is no control on pods order while restarting (TuneD vs workload).

### Implementation Details

* The tuned Daemon set which is run and controlled via the [cluster node tuning operator](https://github.com/openshift/cluster-node-tuning-operator) **(NTO)** will be stripped from the responsibility of running tuned.
The operand will keep the following responsibilities:
  * It will act as the node agent to dump files and read logs of the “podmanized” tuned service:
    * Data write: Read all the Tuned objects and dump the profiles to disk
    * Profile selection: Select the right profile for the node via Tuned.spec.recommend
    * Status reporting: Read tuned logs to report tuning status via Tunes.status
  * It will preserve the support for [rtentsk plugin](https://github.com/redhat-performance/tuned/blob/master/tuned/plugins/plugin_rtentsk.py) (opening a socket with timestamping enabled and holds this socket open).
* [Machine config operator](https://github.com/openshift/machine-config-operator) (MCO) will have a systemd service file to start tuned at boot via podman run using the NTO [image](https://github.com/openshift/cluster-node-tuning-operator/blob/eb35a4ef7ef6072e29faf383078562a70302ee1e/Dockerfile.rhel9#L30):

```yaml
name: ocp-tuned.service
enabled: true
contents: |
  [Unit]
  Description=TuneD service from NTO image
  After=firstboot-osupdate.target systemd-sysctl.service network.target polkit.service
  # Requires is necessary to start this unit before kubelet-dependencies.target
  Requires=kubelet-dependencies.target
  Before=kubelet-dependencies.target
  ConditionPathExists=/var/lib/ocp-tuned/image.env
  
  [Service]
  Type=exec
  Restart=on-failure
  RestartSec=10
  ExecReload=/bin/pkill --signal HUP --pidfile /run/tuned/tuned.pid
  #TimeoutStartSec=5
  ExecStartPre=/bin/bash -c " \
    mkdir -p /run/tuned "
  ExecStart=/usr/bin/podman run \
      --rm \
      --name openshift-tuned \
      --privileged \
      --authfile /var/lib/kubelet/config.json \
      --net=host \
      --pid=host \
      --security-opt label=disable \
      --volume(s)
	… 
      --entrypoint '["/usr/bin/cluster-node-tuning-operator","openshift-tuned","--in-cluster=false","-v=1"]' \
      $NTO_IMAGE
  Environment=PODMAN_SYSTEMD_UNIT=%n
  EnvironmentFile=-/var/lib/ocp-tuned/image.env
  ExecStop=/usr/bin/podman stop -t 20 openshift-tuned
  ExecStopPost=/usr/bin/podman rm -f --ignore openshift-tuned

```

This approach would work even for deployments such as HyperShift, openshift-ansible and likely even IBM ROKS; this still wouldn't work for custom deployments with RHEL.

* Podmanized TuneD will be started via a systemd.path based on a presence of an environment file containing the NTO release payload image (once NTO operand extracts it and writes it to disk).

* Prior to the tuned main process, all necessary tuned directories and known profiles will be created and placed on the node.

* This containerized approach requires having a path to custom tuned profiles directory configurable or at least in subdirectory /etc/tuned/profiles, in order to mount them as a whole in containerized environments. 
This will have to be implemented on [Tuned](https://github.com/redhat-performance/tuned) - [RHEL-26157](https://issues.redhat.com/browse/RHEL-26157)


### Workflow Description

N/A

### API Extensions

No API changes

### Topology Considerations

#### Hypershift / Hosted Control Planes

TBD

#### Standalone Clusters

#### Single-node Deployments or MicroShift

How does this proposal affect the resource consumption of a
single-node OpenShift deployment (SNO), CPU and memory?

TBD

### Implementation Details/Notes/Constraints

* “Accumulating” NTO images during upgrades: by using podman to run TuneD from the NTO image that comes from the release payload,this old payload image will not be removed from the cluster during upgrades as it is used by the systemd unit running podman.
* Even with “podmanized” TuneD, we’ll likely not be able to resolve issues such as [these](https://github.com/openshift/machine-config-operator/pull/2944#issuecomment-1029739561) and centralized the tuning into NTO – chicken-and-egg with default openshift TuneD profiles. Rendering them via MCO/installer, doing some built-in NTO bootstrap or using host-native TuneD would help.
* Even with this approach, TuneD still must finish tuning before Kubelet starts or there is a chance the workloads will start to a partially configured system state.
Options such as reduce the TuneD startup time and/or block the kubelet with timeout on TuneD failures can be taken into consideration.
* Note: To remove the need of having a daemon for keeping open files for some tuned plugins (rtentsk,cpu) [RHEL-23928](https://issues.redhat.com/browse/RHEL-23928) and [RHEL-23926](https://issues.redhat.com/browse/RHEL-23926) will have to be implemented first.

### Risks and Mitigations

* The [pod label based matching](https://docs.openshift.com/container-platform/4.15/scalability_and_performance/using-node-tuning-operator.html#accessing-an-example-node-tuning-operator-specification_node-tuning-operator) must stay working as it was never officially deprecated although the official warning exists.
  * Disabling that functionality when PerformanceProfile is in place should be considered as a viable option.

* Telco Day-0:
  * Stock tuned profiles and all the profiles provided via Tuned.spec.profile are kept in an in memory overlay of the daemonset container.
  We might need to store the profiles files on the node via MachineConfig to support Day 0 tuning (= there no NTO daemon during bootstrap).


### Drawbacks

The implementation is invasive and refactors a prominent part of NTO in the way it used to apply tunings via the tuned daemonset. 
It will not be possible to downport such a solution and the reported issues mentioned at the start will have to be resolved in another manner for older versions.

## Open Questions [optional]

 > 1. Will control plane upgrade (new NTO image reference) cause immediate reboot of all workers? 
 **Requirement:** a new NTO image could be deployed without forcing all workers to reboot.
 
## Test Plan

Full e2e regression tests on new OCP installs and upgrades.

## Graduation Criteria

### Dev Preview -> Tech Preview

### Tech Preview -> GA

### Removing a deprecated feature

- The [pod label based matching](https://docs.openshift.com/container-platform/4.15/scalability_and_performance/using-node-tuning-operator.html#accessing-an-example-node-tuning-operator-specification_node-tuning-operator) needs to be officially deprecated although the official warning exists.

## Upgrade / Downgrade Strategy

* Upgrade NTO from version with tuned daemonset to version with containerized tuned - TBD
* Upgrade NTO with both versions supporting containerized tuned - TBD
* IBU (Image Based Upgrades) for SNO where new release has containerized tuned - TBD

## Support Procedures

## Version Skew Strategy

All MCO and Tuned required changes mentioned above must ship with the main change in NTO

 - [Tuned](https://github.com/redhat-performance/tuned) - [RHEL-26157](https://issues.redhat.com/browse/RHEL-26157) , [RHEL-23926](https://issues.redhat.com/browse/RHEL-23926) (optional)
 - RHEL + RHCOS - [RHEL-23928](https://issues.redhat.com/browse/RHEL-23928)

## Operational Aspects of API Extensions

## Alternatives

#### Move TuneD to RHCOS

The benefits of moving TuneD to RHCOS:
* This resolves the issue of pod startup order (OCPBUGS-26401). TuneD would run as a systemd service and could be set to run before kubelet started any pods.

Issues:
* Lack of Python in RHCOS - but this has been added in recent RHCOS releases.
* TuneD would require other dependencies to be pulled into RHCOS (e.g. hdparm was mentioned). These additions would need to be approved by the RHCOS team.
* It was mentioned that we may need to reduce the TuneD footprint in order to add it to RHCOS - what would that entail?

This wouldn't resolve the issues of TuneD restarts - we'd still need to either allow TuneD to run in one-shot mode or we'd need to allow it to restart safely.

To support running TuneD as a systemd service, we would need changes in NTO to generate the TuneD configuration files and make them available to the TuneD service. A restart would be required to apply the configuration after it was generated by NTO.

#### Do not allow TuneD to do restarts (run one shot)  -  do not reapply

* There were two reasons mentioned why we currently run TuneD in daemon mode (instead of allowing it to apply the setting and exit):
  * [rtentsk plugin](https://github.com/redhat-performance/tuned/blob/master/tuned/plugins/plugin_rtentsk.py): keeps a socket open to avoid static key IPIs. If TuneD didn't do this we'd need another process to do it. In the past, there was such a binary, called [rt-entsk and it was part of the rt-setup package](https://www.redhat.com/sysadmin/real-time-kernel). It was removed in favor of the rtentsk_plugin.
  * [cpu plugin](https://github.com/redhat-performance/tuned/blob/bcfbd5de1163f95deb45b5a8319aff99020ccef9/tuned/plugins/plugin_cpu.py#L497): keeps the /dev/cpu_dma_latency file open to control the maximum c-state. It may be possible to eliminate this by disabling specific c-states for each CPU (i.e. writing to /sys/devices/system/cpu/cpuX/cpuidle/stateX/disable) for each unwanted c-state.

* Even if we eliminated the above and allowed TuneD to just run, apply settings and then exit, we'd still have an issue if the TuneD pod restarted. In that case we would need a way to determine that the configuration had already been applied and avoid doing it again.

* One drawback to preventing TuneD from re-applying configuration on a TuneD restart would be that there would be no way to make any kind of (TuneD controlled) configuration change without a full node restart. Do we rely on this behavior in OCP?

#### Taint nodes via NTO

Use a taint to prevent application pods from running before the TuneD pod has applied the configuration (and then NTO would remove the taint and unblock the application pods). There are some concerns with this approach:

* Platform pods would require this taint to be tolerated, possibly requiring changes to a lot of pod specs.
* How would this work when the node was rebooted and the pods are already scheduled?

#### Layered approach

This would involve adding a layer to RHCOS to add TuneD. Moving TuneD into RHCOS would be preferable.

Potential issues:
* we had issues having multi layers in the past
* can cause issues with upgrades

#### Pod ordering problems in general

Alternative ways that pod startup could be ordered - potentially with some upstream k8s changes:

* QoS classes method [https://github.com/kubernetes/enhancements/pull/3004](https://github.com/kubernetes/enhancements/pull/3004) - RT sensitive pods could in future mention an opaque QoS class name. The exact meaning is then left to be interpreted by the platform components. Possibly opening a way for defining a dependency that is fulfilled later. If accepted upstream this could mitigate RH-only behavior concerns.
* Special device plugin that only exposes a “rt-ready” device once the tuning is complete. The customer workload pods would have to be updated to request this though and the SNO restart with pods already scheduled concern still applies.
