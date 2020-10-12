---
title: Baremetal-Keepalived-Unicast
authors:
  - "@bcrochet"
  - "@yboaron"
reviewers:
  - TBD
approvers:
  - TBD
creation-date: 2020-02-21
last-updated: 2020-02-21
status: implementable
---

# baremetal-keepalived-unicast

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

The bare metal installer-provisioned infrastructure (IPI) use VIPs (Virtual IP) and [Keepalived](https://www.keepalived.org/) to support high availability for API and default Ingress.
For more information in the matter of VIPs please use the following [link](https://github.com/openshift/enhancements/blob/master/enhancements/network/baremetal-networking.md).

In current implementation, Keepalived is configured to run [VRRP management protocol](https://tools.ietf.org/html/rfc3768#section-3) of the VIP addresses in a multicast mode, but since not all environments have multicast available, this proposal seeks to remedy that.

The suggested solution will change Keepalived to run in unicast mode by default and also support moving existing clusters to this mode.


## Motivation

As mentioned above, currently Keepalived sends the VRRP management protocol via multicast. This requires a
minimal amount of configuration, as the multicast peers do not need to be specified.
However, there are likely to be environments where multicast has been disabled because
of network policy. 

Additionally, moving to unicast will also solve the possible collisions when two or more clusters are on the same broadcast/multicast domain. This use case described in more detail in `user story #2` below.

In environments where there are lots of existing virtual routers managing Virtual IPs with multicast messaging or disallow the multicast VRRP traffic completely, it is necessary to configure Keepalived unicast communication  rather than making it an optional configuration. Making it default ensures a successful deployment, so It makes sense to make it the default and if we want to continue to support multicast VRRP, have it as an option.

### Goals

* Allow baremetal IPI deployment in networks where multicast VRRP traffic is forbidden or non-functional.
* No loss of functionality from the current multicast implementation.
* Prevent VRRP ID collisions during deployment.
* Moving existing clusters to unicast mode with minimal downtime.

### Non-Goals

* A switchable implementation. It does not need to be an installer choice.
* Allow masters to be deployed on different subnets (not on the same L2 segment).

## Proposal

Change the way we configure and update Keepalived to manage the VIPs using unicast peer lists.

### User Stories

#### Story 1 - Allow deployment where networking doesn't permit multicast

As an administrator, I want to deploy OpenShift on bare metal nodes with 
installer-provisioned infrastructure (IPI) where the networking doesn't permit multicast.
Some customers have restrictions on multicast network traffic (it impacts telco/edge customers since it’s not possible to use multicast between the regional and far-edge locations).
The current deployment supports high availability for API and default ingress using multicast traffic for VRRP management of the VIP addresses.


#### Story 2 - Allow multiple deployments on the same network

For PoC and test deployments we might need to run multiple clusters on the same network (requiring a dedicated network is a heavy dependency in non-virtualized environments), while we’d still recommend isolated networks for production it would be desirable to enable deployment of multiple clusters on a single broadcast domain.
Keepalived uses the `virtual_router_id` field to specify to which VRRP router the [instance](https://www.keepalived.org/doc/configuration_synopsis.html#vrrp-instance-definitions-synopsis) belongs,`virtual_router_id` range is 1 - 255.
Each baremetal deployment includes two virtual routers:
* API
* Ingress

Although the `virtual_router_id` values for both API and Ingress are calculated using [checksum function based on cluster name](https://github.com/openshift/baremetal-runtimecfg/blob/master/pkg/config/node.go#L162-#L171) we can still end up in a situation where virtual routers from different deployments or services share the same `virtual_router_id`.

### Implementation Details/Notes/Constraints

Unlike multicast mode, in unicast we should specify and keep up to date all the group members in 'unicast_peer' section at [Keepalived conf](https://www.keepalived.org/manpage.html) for each VIP. Lets first present briefly the supported VIPs and the group members for each one of them.

As mentioned above, `api-vip` and `ingress-vip` are supported to enable high availability.

The `api-vip` should resides on the bootstrap instance during the bootstrap phase and move to one of the control plane nodes as soon as one of the control plane nodes can host it.
The group members for the `api-vip` should include the bootstrap node and the control plane nodes.

As per the `ingress-vip`, it should resides on a node (either control or compute) that runs a router pod, so for that case the group members should include all control and compute nodes.

#### api-vip implementation

Since the bootstrap IP address is not known until the server is up, a method for 
retrieving that node will need to be implemented to allow the control plane nodes to include the 
bootstrap IP address in their Keepalived configuration. 

##### Bootstrap node

The bootstrap [Keepalived](https://github.com/openshift/machine-config-operator/blob/4c04978faf8ce36af046e18c7722d03f6e60133e/manifests/baremetal/keepalived.yaml) instance runs as static pod and the relevant [Keepalived configuration](https://github.com/openshift/machine-config-operator/blob/4c04978faf8ce36af046e18c7722d03f6e60133e/manifests/baremetal/keepalived.conf.tmpl) rendered by the [baremetal-runtimecfg](https://github.com/openshift/baremetal-runtimecfg).

The bootstrap Keepalived pod includes:
- [Keepalived container](https://github.com/openshift/machine-config-operator/blob/4c04978faf8ce36af046e18c7722d03f6e60133e/manifests/baremetal/keepalived.yaml#L35-#L93)
- [keepalived-monitor container](https://github.com/openshift/machine-config-operator/blob/4c04978faf8ce36af046e18c7722d03f6e60133e/manifests/baremetal/keepalived.yaml#L94-#L138) which is responsible for rendering Keepalived config file.

The keepalived-monitor container in bootstrap node will first render Keepalived cfg file with an empty 'unicast_peer' section, as a result of that the `api-vip` will be set to the bootstrap node by Keepalived.

At the next step, the bootstrap's keepalived-monitor container will repeatedly retrieve the control plane nodes details from the localhost:kube-apiserver and add them to 'unicast_peer' section.

The bootstrap node owns the `api-vip' during bootstrap phase because it's configured with [higher](https://github.com/openshift/machine-config-operator/blob/4c04978faf8ce36af046e18c7722d03f6e60133e/manifests/baremetal/keepalived.conf.tmpl#L13) VRRP priority than the control plane nodes [priority](https://github.com/openshift/machine-config-operator/blob/4c04978faf8ce36af046e18c7722d03f6e60133e/templates/master/00-master/baremetal/files/baremetal-keepalived-keepalived.yaml#L26).

##### Control plane nodes

The [Keepalived pod](https://github.com/openshift/machine-config-operator/blob/4c04978faf8ce36af046e18c7722d03f6e60133e/templates/common/baremetal/files/baremetal-keepalived.yaml) in control plane nodes already includes Keepalived and keepalived-monitor containers. 
To support unicast mode, the keepalived-monitor should be updated to include the bootstrap node IP address and IP addresses of control plane nodes.

The bootstrap IP address should be fetched from [etcd-endpoints ConfigMap annotation](https://github.com/openshift/cluster-etcd-operator/blob/abe09ece8184e1e18ca12872358bc5e1979e1286/pkg/operator/etcdendpointscontroller/etcdendpointscontroller.go#L96-#L97).

To avoid a circular dependency (Keepalived--> api-vip--> Keepalived) the control plane nodes details retrieved from localhost:kube-apiserver and not api-vip:kube-apiserver.

#### ingress-vip implementation

Add unicast support for the `ingress-vip` is simpler than the `api-vip`.

The following steps should be done to enable unicast for `ingress-vip`:
1. Update the Keepalived conf template files for both [control plane nodes](https://github.com/openshift/machine-config-operator/blob/4c04978faf8ce36af046e18c7722d03f6e60133e/templates/master/00-master/baremetal/files/baremetal-keepalived-keepalived.yaml#L49-#L73) and [compute nodes](https://github.com/openshift/machine-config-operator/blob/4c04978faf8ce36af046e18c7722d03f6e60133e/templates/worker/00-worker/baremetal/files/baremetal-keepalived-keepalived.yaml) to include the unicast details.
2. The keepalived-monitor container should be updated to [retrieve](https://github.com/openshift/baremetal-runtimecfg/blob/8242fd84d6c6d267d2d5551abf8e4f778b4f0615/pkg/config/node.go#L219-#L244) the nodes (both compute and control) IP addresses from the kube-apiserver and render them to Keepalived config.

### Risks and Mitigations

It is important to not destabilize our existing installation method, that uses
multicast successfully. Full parity is expected before landing.

Since upgrade may trigger a Keepalived configuration migration process (see upgrade section below), it's important to add upgrade coverage for bare metal IPI in CI.

## Design Details

### Test Plan

Because of the fundamental inclusion of this in Baremetal IPI installations,
any breakage by CI before landing can probably be assumed to be caused by
this work.

Take down nodes both in active as in passive mode and observe the VIP move correctly and without significantly increasing the current expected downtime in VIP changes.

Force a disaster recovery scenario and see that we can recover the cluster following the usual procedure.

Since the baremetal-runtimecfg is used by all on-prem platforms, we should verify that Ovirt, vSphere, and OpenStack still successfully deployed in Keepalived multicast mode.

### Upgrade / Downgrade Strategy

In case that the Keepalived config file was already generated by the previous SW version, Keepalived will continue first to run in a mode determined in accordance with the existing file.

The MCO should render a new file in all nodes, this file will trigger a synchronized reload migration, the content of the file will determine the desired mode.

More information regarding the migration process can be found in the next section.

#### Configuration migration process

Since multicast and unicast are considered as separate Keepalived domains and will not interoperate, the updated configuration should simultaneously reload in all the relevant Keepalived instances. This option is feasible since the nodes run NTP.

The following describes the required steps for updating Keepalived mode in the cluster:

1. A file with a predefined name should be created in all nodes.
   The file should include both the desired mode and planned reload time (optional)
2. Once the file has been identified by the [keepalived-monitor](https://github.com/openshift/machine-config-operator/blob/master/templates/common/baremetal/files/baremetal-keepalived.yaml#L89-#L113) the following actions will be carried out:
   - Verification that desired mode is different from current mode (otherwise exit)
   - Verification that upgrade process is completed, by the comparison between values of `machineconfiguration.openshift.io/currentConfig` and  `machineconfiguration.openshift.io/desiredConfig` in all nodes annotations.
    - Generation of updated configuration file and initiation of reload message at the desired calculated time.

## Implementation History

A PR with implementation in baremetal-runtimecfg can be found here:

* https://github.com/openshift/baremetal-runtimecfg/pull/65

The Machine Config operator portion can be found here:

* https://github.com/openshift/machine-config-operator/pull/1768

## Drawbacks

* Multicast is a simpler configuration with fewer moving parts. However, not every
  environment allows multicast, whereas all environments would allow unicast.

## Alternatives

An alternative would be to absolutely require multicast for installation and
operation.
