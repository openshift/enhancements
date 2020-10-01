---
title: rhcos-inject
authors:
  - "@crawford"
reviewers:
  - TBD
approvers:
  - TBD
creation-date: 2020-10-01
last-updated: 2020-10-01
status: provisional
---

# rhcos-inject

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

Throughout it's existence, OpenShift 4 has put a heavy influence on installation flows that make use of installer-provisioned infrastructure. This has largely been successful for predictable environments, such as AWS, Azure, or GCP, but it has (unsurprisingly) proven to make deployments into less predictable environments more difficult. These less predictable environments introduce a lot of variability into areas like network configuration, disk layout, and the life cycle of the machines and that makes it difficult or impossible for OpenShift to start from a foundation of shared assumption. To bridge this gap, admins need a way to inject a certain amount of customization before OpenShift installation can begin.

## Motivation

Admins need a way to inject customization into their RHCOS nodes before they are provisioned by the cluster. In most cases, this configuration is require in order for the provisioning process to complete, so the normal facilities (e.g. Machine Configuration Operator) are not yet available. Today, this is a very manual process involving an admin interactively providing that configuration at the point of running the RHCOS installer. This might work in some environments, but many others don't have easy interactive access due to technical or policy constraints.

### Goals

- Make it easy for an admin to add network configuration to each of the RHCOS nodes added both before and after OpenShift installation.
- Allow an admin to add custom services and miscellaneous configuration to their RHCOS nodes.

### Non-Goals

- Inventory management
- Bring-your-own-RHEL customization

## Proposal

Add a new, optional step to the installation process that allows an admin to inject network configuration and any other customization to the RHCOS installer image (and maybe other RHCOS images in the future) before invocation. This customization is performed by a new `rhcos-inject` utility that takes as input an RHCOS installer image (`rhcos-<version>-installer.<architecture>.iso`), network configuration, and a set of Ignition Configs and generates a new RHCOS installer image with all of the configuration and customization included. This new installer image can then be booted in an unsupervised manner and will complete the RHCOS installation along with any customization.

Initially, `rhcos-inject` will primarily focus on network configuration but as the needs of customers evolve, this can be expanded to include other common customization. As always, an escape hatch is needed for unanticipated cases and this will be in the form of raw Ignition Configs. If there is a need for additional customization beyond network configuration, the admin can include that in an Ignition Config and inject that alongside everything else.

An example of its invocation can be seen here:

```console
$ rhcos-inject \
	--input=rhcos-installer.x86_64.iso \
	--output=control-0.iso \
	--openshift-config=master.ign \
	--bond=bond0:em1,em2:mode=active-backup \
	--ip=10.10.10.2::10.10.10.254:255.255.255.0:control0.example.com:bond0:none
```

With a little scripting, a number of custom RHCOS installers can be quickly created:

```zsh
for i in {00..11}
do
	rhcos-inject \
		--input=rhcos-installer.x86_64.iso \
		--output=worker-${i}.iso \
		--openshift-config=worker.ign \
		--ip=10.10.10.$((i+10))::10.10.10.254:255.255.255.0:worker${i}.example.com:enp1s0:none
done
```

This network configuration applies to the post-pivot RHCOS installation environment and is copied to both the pre- and post-pivot installed environment by default. In the event that exotic configuration is required, this copying of configuration can be disabled and a service can be used instead (TODO(crawford) figure out systemd service ordering):

```console
$ cat rhcp.ign
{
  "ignition": { "version": "3.0.0"},
  "systemd": {
    "units": [{
      "name": "random-host-configuration-protocol.service",
      "enabled": true,
      "contents": "[Service]\nType=oneshot\nExecStart=/usr/bin/env nmcli ...\n\n[Install]\nWantedBy=pre-install.target"
    }]
  }
}

$ rhcos-inject \
	--input=rhcos-installer.x86_64.iso \
	--output=worker.iso \
	--openshift-config=worker.ign \
	--installer-config=rhcp.ign \
	--persist-networking=false
```

This custom service runs in the context of the RHCOS installer and is responsible for writing the network configuration for the installed system (e.g. based on an external lookup by MAC address). This escape hatch can be used in environments which employ an in-house host configuration procedure.

### User Stories

#### Static Network Configuration

An RHCOS node is deployed into an environment without DHCP and needs to be explicitly configured with an IP address/mask, gateway, and hostname:

```console
$ rhcos-inject \
	--input=rhcos-installer.x86_64.iso \
	--output=control-0.iso \
	--openshift-config=master.ign \
	--ip=10.10.10.2::10.10.10.254:255.255.255.0:control0.example.com:enp1s0:none
```

This static IP configuration is used for both the RHCOS installation and the post-installation system.

#### Dedicated Provisioning Network

An RHCOS node is deployed into an environment which makes use of a dedicated provisioning network which is only used during installation:

```console
$ rhcos-inject \
	--input=rhcos-installer.x86_64.iso \
	--output=control-0.iso \
	--openshift-config=master.ign \
	--ip=10.10.10.2::10.10.10.254:255.255.255.0:control0.example.com:enp1s0:none \
	--ip=:::::enp1s1:none \
	--persist-network=false
```

This static IP configuration is used during the installation, but once the machine reboots into the running system, it uses a different configuration. This is likely to be paired with the following use case.

#### Custom Dynamic Host Configuration

An RHCOS node is deployed into an environment which uses an in-house IPAM implementation in lieu of DHCP:

```console
$ cat network.ign
{
  "ignition": { "version": "3.0.0"},
  "systemd": {
    "units": [{
      "name": "configure-networking.service",
      "enabled": true,
      "contents": "[Service]\nType=oneshot\nExecStart=/usr/bin/env nmcli ...\n\n[Install]\nWantedBy=pre-install.target"
    }]
  }
}

$ rhcos-inject \
	--input=rhcos-installer.x86_64.iso \
	--output=my-rhcos-installer.x86_64.iso \
	--openshift-config=master.ign \
	--installer-config=network.ign
```

This service can contain just about any piece of logic needed in order to statically configure the node based on a dynamic assignment. For example, a customer may use this mechanism to configure a link-local address, request an IP from a provisioning system, reconfigure the network interfaces, and then phone-home to acknowledge a successful configuration.

### Implementation Details/Notes/Constraints

As with all software, it's important to consider coupling and cohesion when looking at this solution and its alternatives. Defining clear API boundaries and building upon layers of abstraction are some of the most effective techniques for avoiding pitfalls. This solution chooses to make a distinction between the networking required for an individual node to operate and for the cluster itself; respectively, the machine network and the pod network. A functioning machine network is considered a prerequisite to installation, as is power, cooling, and many others. The pod network, on the other hand, is something created and managed by OpenShift. Working backward from this assumption, it's clear that the solution to the problem of pre-installation network configuration should not be solved by the cluster.

When thinking about where this functionality should live, `openshift-install` may seem like an obvious choice. This is a poor fit, however. `openshift-install` is only used during installation and destruction of the cluster, whereas this functionality would also be necessary post-installation (e.g. during a scaling event). Additionally, there are a number of existing and future components which would benefit from this functionality, but may not want to carry the full weight of `openshift-install` (368 MiB at the time writing). Even further, `openshift-install` needs to continue to support MacOS, but it wouldn't be feasible to do the necessary Linux file system operations from that environment. It's going to be most flexible to implement this new functionality in a stand-alone, Linux-only utility. Future improvements may include running this utility in the cluster so that an admin can perform customizations from the comfort of the browser and then simply downloading the result.

It hasn't been explicitly mentioned yet, but implicit in this proposal is a slight rework to the RHCOS installer. In order for a user to effectively be able to leverage custom systemd services, there will need to be a simple and well-defined ordering of targets. There will also likely be some amount of changes necessary so that operations which depend on one another can communicate success and failure and so that the overall installation can be easily monitored.

### Risks and Mitigations

Since this is a new component entirely, there is very little risk to the existing installation procedures. The biggest risk appears to be the escape hatch; the concern being that it will be heavily abused to solve any machine customization challenges, including ones that should be tackled by the Machine Config Operator.

## Design Details

### Open Questions

#### Flag Names

I hate the classic `ip=:::::::::::::` syntax but I didn't want to redesign that in this initial pass. We should rethink the specific representation of these options before implementation. This will also allow for a more expressive invocation that can be used for things like Dot1Q, VLAN-tagging, teaming, etc.

#### Predictable (vs Consistent) Interface Names

There are going to be a lot of environments where a machine only has the one NIC and the admin just wants to configure it. Rather than requiring that they know the exact name of the interface, it would be helpful if there was a more flexible specifier that we can use.

#### Service Orchestration

What systemd targets are needed to guide the ordering of services that are injected into the installation environment? I presume we'll want something to ensure services run before installation begins and after that completes.

#### Raw Ignition Configs

Support for injecting raw Ignition Configs is very much an escape hatch - no human should be spending any significant amount of time reading or writing Ignition configs. If users find utility in this mechanism (as I suspect they will), they are immediately going to be met with frustration when simple syntax errors prevent nodes from installing correctly. We should consider jumping directly to a more user-friendly approach of integrating a config transpiler (e.g. https://github.com/coreos/container-linux-config-transpiler) so that admins have a better experience. The obvious downside to this approach is that it will be easier to make customizations using this mechanism versus the preferred paradigm of Machine Configs.

## Implementation History

TBD

## Drawbacks

- This approach taints the pristine RHCOS assets that come from our build system. One of the early goals of OpenShift 4 was to avoid the use of "golden images" and to push customers to make use of declarative configuration instead. This is a notable departure from that stance and opens the door to misuse and obfuscation of the installation environment.

## Alternatives

- https://github.com/openshift/enhancements/pull/399
- https://github.com/openshift/enhancements/pull/467
