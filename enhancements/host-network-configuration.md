---
title: host-network-configuration
authors:
  - "@russellb"
reviewers:
  - “@derekwaynecarr”
  - “@crawford”
  - “@cgwalters”
approvers:
  - “@celebdor”
  - “@bcrochet”
  - “@squeed”
  - “@danwinship”
  - “@dcbw”
  - “@knobunc”
  - “@abhinavdahiya”
  - “@sdodson”
  - “@yboaron”
  - “@qinqon”
  - “@phoracek”
  - “@EdDev”
  - “@ashcrow”
  - “@dhellmann”
  - “@hardys”
  - “@stbenjam”
  - “@dustymabe”
  - “@miabbott”
  - “@dougbtv”
  - “@eparis”
  - “@markmc”
  - “@jwforres”
creation-date: 2020-06-30
last-updated: 2020-06-30
status: implemented
see-also:
  - "/enhancements/rhcos/static-networking-enhancements.md"
  - "/enhancements/network/20190903-SRIOV-GA.md"
  - "/enhancements/network/host-level-openvswitch.md"
  - "/enhancements/baremetal/baremetal-provisioning-config.md"
  - "/enhancements/baremetal/baremetal-provisioning-optional.md"
---

# Host Network Configuration

## Summary

This document serves as a high level overview of host network configuration for
OpenShift 4.x clusters.  Host network configuration spans multiple features in
multiple components.  This enhancement aims to describe use cases and point to
various references for current or future solutions that help satisfy those use
cases.

## Motivation

Configuring host network devices on an OpenShift cluster looks a bit different
than a standalone Linux host.  This document aims to be a starting reference
for learning more about how this works right now and where to look to learn
more.

### Goals

* Document the current configuration interfaces available for host networking
* Provide references to other proposals

### Non-Goals

* Proposing new enhancements.  Instead, those should be covered in other
  enhancements that this one may reference.


## Use Cases

The purpose of this section is not to enumerate every possible use case, but it
should cover enough use cases to demonstrate the range and flexibility of host
network configuration needed in OpenShift.

This section could use several more additions - contributions welcome!

### Use Case #1 - Bare Metal, Primary Network VLAN

Consider an on-premise environment where the network administrator has isolated
their provisioning infrastructure on a separate network from where the cluster
is expected to operate.  It would be wasteful to dedicate an entire NIC just
for provisioning and no other purpose.  Instead, the administrator would like
to bring up the cluster’s primary network as a VLAN on the same network
interface that was used for provisioning.

* Bare metal servers with a single network interface
* We desire PXE booting these servers on the untagged network for provisioning
* We must configure a VLAN on this interface to access the primary external
  network for the cluster
* Access to this network must be configured very early, before ignition will be
  able to reach the machine config server

### Use Case #2 - Extra VLAN, Post Install Configuration

Consider an on-premise environment with workloads that must be able to access
resources on multiple networks that must be directly connected to the cluster.
One example could be an isolated network running database services that are not
routeable from any other network.  Instead, we’ll attach the Nodes to that VLAN
and limit which pods can access it.

* After cluster installation, configure another VLAN on a network interface
  that specific workloads will need access to

### Use Case #3 - Static Network Configuration Only

Consider a lab environment where DHCP services either aren’t available at all,
or the person trying out an OpenShift cluster does not have access to it to
configure DHCP address reservations.  As long as the cluster administrator has
a pool of addresses they are allowed to assign manually, a cluster with fully
static network configuration would allow them to install OpenShift without
needing to work with the network administrator on DHCP server configuration.

* Must be able to override default RHCOS behavior of requiring DHCP to be
  present on at least one interface
* Must be able to configure static network configuration for every interface
  used by the cluster


## Existing Configuration Interfaces

### Summary

First, here’s a summary of how an administrator may configure host networking
in a cluster.  More details about each interface are in the following sections.

#### First Boot

When an RHCOS host first boots, the initramfs environment must be able to reach
the [Machine Config Server
(MCS)](https://github.com/openshift/machine-config-operator/blob/master/docs/MachineConfigServer.md)
for Ignition to succeed.  The MCS starts by running on the Boostrap host while
the first hosts are being deployed.  Ignition will attempt to reach the Machine
Config Server via an included URL. By default, dracut will attempt to do DHCPv4
and DHCPv6 on all interfaces and wait for one of them to work.  To change this
behavior, an admin can:

* Change the `ip=<...>` kernel argument(s) via PXE configuration or at the grub
  command line during boot.  If doing a bare metal install, it is also possible
  to edit the arguments baked into the image using a tool such as
  `virt-customize`.  Note that this is really the interface of last resort and
  shouldn’t be used unless absolutely required.
* If at least one network has DHCP to allow dracut to succeed, custom ignition
  additions can configure additional networking.
* If using the RHCOS live ISO/PXE ramdisk environment with the
  coreos-installer, use the `nmtui` Network Manager text UI to configure
  networking before proceeding.  Those settings can be automatically propagated
  to the installed host by using the `--copy-network` option to the
  `coreos-installer`.

#### Further Network Customization

Beyond the minimum needed to bootstrap the cluster, further host network
configuration customizations can be applied in the following ways:

* Customize the ignition configuration at install time to include more
  NetworkManager configuration files
* Create `MachineConfig` resources with NetworkManager configuration files and
  have them applied by the installer at install time, or apply new
  `MachineConfig` resources to the cluster via the API any time post-install

### Dracut in initfamfs

In the initramfs environment at boot time,
[dracut](https://github.com/dracutdevs/dracut) is the first tool that sets up
networking on the host.  Dracut is not specific to RHCOS.  It used by Fedora
and RHEL, as well.

Configuring dracut is done via [kernel command line
arguments](https://mirrors.edge.kernel.org/pub/linux/utils/boot/dracut/dracut.html#_network).
The kernel command line arguments can be customized via PXE configuration or at
the grub command line during boot.

Networking at this stage of the installation process is important, as the
installer generates a “pointer ignition config”, or a small ignition stub that
just points to a URL for where to download the full ignition configuration from
the Machine Config Server.  In other words, when a new RHCOS host boots for the
first time, dracut must configure networking just enough for ignition to be
able to reach the Machine Config Server.

A [past bug against
dracut](https://bugzilla.redhat.com/show_bug.cgi?id=1787620) ensured that there
was a way to get dracut to attempt both DHCPv4 (IPv4) and DHCPv6 (IPv6) and not
fail if one of them worked.  We also [changed
OpenShift](https://bugzilla.redhat.com/show_bug.cgi?id=1793591) to use the
`ip=dhcp,dhcp6` configuration by default in RHCOS.  This at least got default
behavior far enough along that it will work in either an IPv4 or IPv6
environment.

Assuming dracut succeeds in at least one DHCP attempt, additional networking
can be configured via ignition if further customizations are needed to enable
the host to reach the Machine Config Server.

Note that as of OCP 4.6, networking in the initramfs is handled by
NetworkManager.  The user interface stays the same (network kargs defined in
`man dracut.cmdline`) but the implementation is the `35network-manager` dracut
module vs the `35network-legacy` dracut module, which uses the legacy
network-scripts. It has some implications on the behavior though. For example
[we get smarter timeout
behavior](https://bugzilla.redhat.com/show_bug.cgi?id=1836248#c40) among others
mentioned in [coreos/fedora-coreos-tracker#394
(comment)](https://github.com/coreos/fedora-coreos-tracker/issues/394#issuecomment-604598128).

### Ignition

[Ignition](https://github.com/coreos/ignition/) is the tool used by RHCOS to do
initial host configuration on first boot.  It runs in initramfs, so the
networking set up done by dracut is the network environment ignition runs in.

Currently there are two ignition configuration files for each role e.g
master/worker - a “pointer” ignition is created by the installer, and
this can be customized by the user via the installer `create ignition-configs`
interface.  This configuration then references the rendered configuration via
an ignition `append` directive, which downloads the configuration referenced
from the Machine Config Server (which requires network access).

Here is an example of a “pointer” ignition configuration as generated by the
OpenShift installer:

```
$ cat ocp/ostest/worker.ign | jq .
{
  "ignition": {
    "config": {
      "append": [
        {
          "source": "https://[fd2e:6f44:5dd8:c956::5]:22623/config/worker",
          "verification": {}
        }
      ]
    },
    "security": {
      "tls": {
        "certificateAuthorities": [
          {
            "source": "data:text/plain;charset=utf-8;base64,...==",
            "verification": {}
          }
        ]
      }
    },
    "timeouts": {},
    "version": "2.2.0"
  },
  "networkd": {},
  "passwd": {},
  "storage": {},
  "systemd": {}
}
```

By customizing the “pointer” ignition configuration generated by the installer,
additional network configuration can be done on the host prior to attempting to
reach the Machine Config Server URL.

#### Example

Consider a cluster with hosts that have many network interfaces.  We may want
to configure NetworkManager to not attempt to bring up interfaces that do not
have explicit configuration.

```
mkdir lab
cat << EOF > lab/etc/conf.d/10-disable-unconfigured-nics.conf
[main]
no-auto-default=*
EOF
```

Next we update the two ignition configuration files generated by the installer
to include our network configuration.   This example makes use of
[filetranspiler](https://github.com/ashcrow/filetranspiler).

```
mv master.ign master.ign.orig
mv worker.ign worker.ign.orig
for i in master worker; do python3 ~/filetranspile --pretty -i ${i}.ign.orig -f ./lab -o ${i}.ign; done
```

### MachineConfig Resource

Once a cluster is able to reach the Machine Config Server, an additional
interface is available for managing host configuration: the `MachineConfig`
resource.  The `MachineConfig` resource can be used to add additional host
configuration for a single Node or pool of Nodes.  The OpenShift documentation
provides some examples of using this interface, such as [this example of
configuring an NTP server for the chrony time
service](https://docs.openshift.com/container-platform/4.4/installing/install_config/installing-customizing.html#installation-special-config-crony_installing-customizing).

This same interface can be used to manage custom NetworkManager configuration
files.  As long as the network configuration is not needed during the initial
bootstrapping of a new Node, this interface provides access to any host network
configuration that’s doable through NetworkManager.

Note that `MachineConfig` resources apply to a `MachinePool` and not to a
single host.  That makes this configuration interface helpful only for certain
types of network configuration where the same configuration can be laid down
across multiple hosts.  This interface would not work for applying static IP
configuration across each individual host.  A future enhancement is needed to
better serve that use case through the OpenShift API.

#### Example

Consider a bare metal cluster where the administrator would like to configure a
VLAN interface after the cluster is up and running.  A `MachineConfig` resource
can be used to apply a new NetworkManager configuration file to do this.

First, we must craft a config file to drop into the
`/etc/NetworkManager/system-connections `directory on each host. For example
purposes, I used this config file, which configures a VLAN interface using
DHCP.

In this case I used a VLAN ID of 20, and the NIC is ens3.

```
[connection]
id=vlan-vlan20
type=vlan
interface-name=vlan20
autoconnect=true
[ipv4]
method=auto
[vlan]
parent=ens3
id=20
```

These contents must then be base64 encoded and placed into a `MachineConfig`
resource. Here's the resource I used:

```
apiVersion: machineconfiguration.openshift.io/v1
kind: MachineConfig
metadata:
  labels:
    machineconfiguration.openshift.io/role: master
  name: 00-ocs-vlan
spec:
  config:
    ignition:
      version: 2.2.0
    storage:
      files:
      - contents:
          source: data:text/plain;charset=utf-8;base64,W2Nvbm5lY3Rpb25dCmlkPXZsYW4tdmxhbjIwCnR5cGU9dmxhbgppbnRlcmZhY2UtbmFtZT12bGFuMjAKYXV0b2Nvbm5lY3Q9dHJ1ZQpbaXB2NF0KbWV0aG9kPW1hbnVhbAphZGRyZXNzZXM9MTcyLjUuMC4yLzI0CmdhdGV3YXk9MTcyLjUuMC4xClt2bGFuXQpwYXJlbnQ9ZW5zMwppZD0yMAo=
        filesystem: root
        mode: 0600
        path: /etc/NetworkManager/system-connections/ocs-vlan.conf

```

This resource can then be applied to a running cluster.

Note that it could also be dropped in the `openshift/` directory of installer
generated manifests if you’d like to have the installer apply a similar
manifest at install time.  This works fine as long as you don’t need these
network customizations applied before the host can reach the Machine Config
Server.

### RHCOS live ISO Environment

As of OCP 4.6, [RHCOS includes a new live ISO/PXE ramdisk
environment](https://github.com/openshift/enhancements/pull/210) for installing
RHCOS.  This environment is used to run
[coreos-installer](https://github.com/coreos/coreos-installer/). Following that
work, [RHCOS will include the ability to run a text-based UI to configure
networking](https://github.com/openshift/enhancements/pull/291) in the ramdisk:
`nmtui`.  This installation process also provides the ability to provide
networking configuration via Ignition if the administrator does not want to
follow an interactive configuration process.

The `coreos-installer` also includes a
[`--copy-network`](https://github.com/coreos/coreos-installer/pull/212) option
which can be used to persist whatever network configuration was done using
`nmtui` to the resulting installed host.  There is a [dracut
module](https://github.com/coreos/fedora-coreos-config/pull/346) which will
apply these settings in the initramfs.

These enhancements provide some excellent improvements to early network
configuration capabilities.  However, it is currently most beneficial to the
UPI bare metal flow.  The IPI bare metal flow will need to do more work to
figure out how to make use of these tools.  Bare Metal IPI makes use of a
different deployment ramdisk that is part of the Ironic project.  While
conceptually similar, it has some significant differences in implementation.
Future integration work may provide better alignment between the Bare Metal IPI
tooling and the RHCOS installer.

### SR-IOV Operator

Some clusters may want to allocate SR-IOV devices to workloads for a secondary
network interface with high performance.  These devices are managed by the
[SR-IOV Operator](https://github.com/openshift/sriov-network-operator).

### Privileged Pods

A use case came up where someone wanted to run a custom BGP routing daemon on
each host.  Previously they would have managed this directly via Ansible and
wondered how such a configuration could be done with OCP 4.  This sort of
custom networking setup is still possible if run in containers.  For example,
if containers are built including the required daemon(s), then a `DaemonSet`
could be applied to the cluster to run a privileged pod on each Node which
performs the desired host network management.

Care must be taken when taking an approach like this.  It’s critical to
understand how such an approach will interact with the network provider in use
to ensure unintended side effects do not occur.  While this capability is
perfectly valid to use, it should not be encouraged.  Instead, it is a
capability we keep “in our back pocket” in case it’s needed to help a given
user solve their unique requirements.

## Future Improvements

### Disabling initramfs Networking

Configuration of initramfs networking is lacking in some desired flexibility,
as shown by the discussion in [this ignition
issue](https://github.com/coreos/ignition/issues/979).  The default behavior in
RHCOS is to attempt to get a DHCPv4 or DHCPv6 lease on one of the host’s
interfaces.  This can cause a lot of undesirable delay if there are a lot of
network interfaces present and DHCP is not available.  It will also cause
booting to fail if DHCP never succeeds.

One improvement is the ability to disable initramfs networking completely,
which would be useful in combined with providing the host a full ignition
configuration without needing to retrieve the bulk of it from a network
location.  See “Flattened Ignition Configuration” for some more info on that
piece.

### MCO Managed Pointer Ignition Configuration

There is a WIP proposal to moving management of the “pointer” configuration to
the MCO, such that e.g upgrade of the ignition version would be possible
without manual fixes, ref
<https://github.com/openshift/machine-config-operator/pull/1792>

### Flattened Ignition Configuration

The per-role `MachineConfigPool` resources result in a `MachineConfig` resource
that is rendered by the MCO, and exposed via the MCS - this is the
configuration that’s referenced by the “pointer”  ignition resource.

One reason for this “pointer” ignition approach is that on many cloud platforms
the size limit for host initialization information through their API is very
low. To get around the size limit, we provide a small config that contains
instructions for how to download the full configuration at runtime. For
baremetal deployments this size limit is likely to be much higher, so it may be
possible to provide the entire configuration to each node, which potentially
removes the runtime requirement to download the rendered configuration.

There is more discussion on this topic on
<https://github.com/openshift/machine-config-operator/issues/1690>

### Kubernetes NMState

While post-install network configuration is possible using `MachineConfig`
resources, it is worth considering whether host network management deserves its
own domain specific API.  One approach that has been proposed is integrating
[kubernetes-nmstate](https://github.com/nmstate/kubernetes-nmstate), which is
discussed in [this
enhancement](https://github.com/openshift/enhancements/pull/161).

It’s possible that a more domain specific network API would provide a
configuration interface that is easier to work with than managing
NetworkManager config files directly with `MachineConfig` resources. It could
also provide some limits on the types of configurations allowed, if such limits
were desired.

`kubernetes-nmstate` was originally designed as a completely standalone
component that can manage networking on Nodes that run NetworkManager.  There
are further architectural considerations under discussion on the enhancement
for OpenShift integration for how this could work in the OpenShift architecture
where the Machine Config Operator and its sub components own management of host
state today.

### Bare Metal IPI Alignment with RHCOS Installer

[RHCOS live ISO Environment](#rhcos-live-iso-environement) discussed the
capabilities for early network configuration when using the RHCOS installer
environment.  These capabilities are not available with bare metal IPI because
RHCOS installation is handled by a different deployment ramdisk which comes
from the Ironic project.  In bare metal IPI, the bare metal hosts are treated
like cloud VMs, and a full RHCOS disk image is written to the host along with a
config drive partition to include ignition configuration.

Exploration of further alignment of the deployment ramdisk environment seems
worthwhile, both for alignment on network management, but also to increase
consistency across environments and reduce overlapping functionality between
different components.
