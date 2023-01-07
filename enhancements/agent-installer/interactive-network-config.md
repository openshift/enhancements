---
title: interactive-network-config
authors:
  - "@zaneb"
reviewers: # Include a comment about what domain expertise a reviewer is expected to bring and what area of the enhancement you expect them to focus on. For example: - "@networkguru, for networking aspects, please look at IP bootstrapping aspect"
  - "@andfasano"
  - "@rwsu"
  - "@bfournie"
  - "@cgwalters"
  - "@patrickdillon"
approvers:
  - "@celebdor"
api-approvers: # In case of new or modified APIs or API extensions (CRDs, aggregated apiservers, webhooks, finalizers). If there is no API change, use "None"
  - None
creation-date: 2023-01-06
last-updated: 2023-01-06
tracking-link:
  - https://issues.redhat.com/browse/AGENT-385
see-also:
  - "/enhancements/agent-installer/automated-workflow-for-agent-based-installer.md"
replaces: N/A
superseded-by: N/A
---

# Interactive Network Configuration

## Summary

Enable the user, after booting the agent ISO, to interactively add or correct
any neccessary network configuration. Since we cannot rely on network
connectivity for this step, configuration will be via a text user interface
(TUI) displayed on the console.

## Motivation

Providing network configuration up-front, in the absence of the actual hosts to
be used, is onerous for users. Names of NIC interfaces are difficult to predict
in advance, and often MAC addresses are required. Even the simplified NMState
format is difficult to write by hand.

Correcting any error in the configuration passed into the installer currently
requires returning to the beginning of the process, generating a new ISO and
booting the hosts anew with the updated image. There is no guarantee of getting
it right, so there could be multiple iterations of this process.

### User Stories

* As a cluster administrator, I want to install a cluster into a network
  environment requiring custom network configuration (e.g. static IPs, NIC
  bonds, VLANs) without needing any information from the physical servers in
  advance.
* As a cluster administrator, I want to fix a cluster installation that has
  stalled due to incorrect network configuration without having to regenerate
  the installation image.

### Goals

* Allow the user to configure static IPs, NIC bonds, and VLANs.
* Make available all necessary information about the hardware and network
  environment to do this configuration.
* Provide an indication of when sufficient connectivity is or is not available.
* Enable the user to debug connectivity problems by narrowing down the cause
  (DNS, reachability, &c.).
* Work for both the existing automation workflow and a future fully-interactive
  workflow.

### Non-Goals

* Interactive (re)configuration of any other aspect of the installation.
* Add interactive network configuration to CoreOS in general.
* Allow interactive network configuration during IPI baremetal installs.
* Create a new implementation of the network configuration available in
  `nmtui`.

## Proposal

### Workflow Description

The user will generate the agent ISO using `openshift-install` as usual. The
`networkConfig` section in the `agent-config` (or the NMStateConfig ZTP
manifests) may _or may not_ contain network configuration.

The user will then boot the ISO. Any network configuration for each current
host contained in the ISO will be applied at startup.

Prior to the login prompt, a TUI will appear on the hardware console. It will
display the current status of connectivity to the registry, which will be
continuously updated. It will also give the user the option to interactively
reconfigure the network, or accept the current configuration and exit.

If there is connectivity available to the registry, the TUI will automatically
exit within 20s if there is no user interaction.

Services that require pulling a container image will not start until the TUI
exits.

Once the TUI exits, the login prompt will be displayed on the console as usual.

### API Extensions

N/A

### Implementation Details/Notes/Constraints

The TUI will be implemented as a golang binary and delivered in a container
added to the release payload. The actual network configuration will be done
through the existing `nmtui` binary, which is already present in CoreOS.

To allow it to run before the network is configured, the TUI binary will be
extracted from the release payload by the installer process, and embedded in
the CoreOS ISO (which is also extracted from the release payload). Since even a
modest golang binary is likely too large for the pre-allocated Ignition config
area, the binary will be added to the initramfs along with a script in the
`/usr/lib/dracut/hooks/pre-pivot/` directory to copy it from the initramfs to
the system root. To add these files to the initramfs in the ISO, they will be
added to a new CPIO archive, and this archive will be appended to the initrd
volume. In PXE installs, the installer will append the new CPIO archive to the
initrd file, immediately after the CPIO archive containing the Ignition config.

The running network configuration is copied to disk using the `--copy-network`
argument to `coreos-installer`, so no other changes are required to persist the
desired configuration to the network.

Access to the registry could be checked by simply telling podman to pull the
release image. However, if this fails then ideally we would be able to identify
the point where it stops working in the following steps:

* DNS lookup
* ping
* HTTP
* Registry login
* Pulling the release image

### Risks and Mitigations

Users with access to the hardware console will be able to manipulate the
network configuration without logging in. If an exploit is found they may be
able to upgrade this to shell access. To mitigate this, the service should not
run as the root user. (`nmtui` itself will need to run as root, since it has to
be able to write the network configuration.) However, a user with physical
access to the hardware or access to the BMC can probably do anything they want
anyway.

### Drawbacks

Since we do not have code built in to the installer binary for extracting files
from the release payload, the agent-based installer will become dependent on
the presence of the `oc` client binary in all cases. Previously we could avoid
this dependency in connected environments by falling back to downloading the
CoreOS ISO from the Internet.

## Design Details

### Open Questions

1. What is the safest way to rewrite the CoreOS ISO to include the TUI binary?

### Test Plan

CI testing will verify that this does not break the fully-automated flow (e.g.
by blocking services starting forever).

The TUI binary itself will have unit tests and integration tests defined in its
own repository.

It is extremely difficult to automate testing of a TUI over the hardware
console, so end-to-end operation will be tested manually by QE.

### Graduation Criteria

N/A

#### Dev Preview -> Tech Preview

N/A

#### Tech Preview -> GA

N/A

#### Removing a deprecated feature

N/A

### Upgrade / Downgrade Strategy

N/A

### Version Skew Strategy

N/A

### Operational Aspects of API Extensions

N/A

#### Failure Modes

N/A

#### Support Procedures

N/A

## Implementation History

Prototype systemd unit: [openshift/installer#6560](https://github.com/openshift/installer/pull/6560)
TUI proof-of-concept: <https://github.com/openshift-agent-team/tui>
Investigation of adding a binary to the ISO: [AGENT-469](https://issues.redhat.com/browse/AGENT-469)
Prototype of adding a binary to the ISO: <https://github.com/andfasano/gasoline>

## Alternatives

The `whiptail` binary shows a TUI that is consistent in style with `nmtui`, and
uses only command-line arguments as input. It is already present in CoreOS. In
principle, we could use a bash script (which would be small enough to embed in
the ignition) to invoke it and avoid the need to embed a binary in the ISO.
While `whiptail` takes over the whole screen, the `tmux` binary is also
available in CoreOS and could in theory be used to create multiple panes to
allow the connectivity status to be shown alongside the TUI. However, software
written in bash is generally much more brittle and difficult to maintain than
the equivalent golang with appropriate libraries.

We could package the TUI binary for RHEL and include it in the base CoreOS
image. In principle, with a configuration option to list the container
registries that are needed, this could make sense as a generic CoreOS feature
(including for upstream Fedora CoreOS). Ideally this would be exposed as an
option in Ignition, so that the work of creating a systemd service to run at
the correct time could happen automatically. While this could be a worthy goal,
it will be time consuming to complete. It will be better to get a working
concept in the agent installer first, and only later pursue more generic
options.

Requesting the rendezvousIP interactively prior to the network configuration
would allow us to check connectivity not only to the registry, but also to the
assisted-service, even in a future fully-interactive workflow. This addition
would not make sense in a generic CoreOS configuration feature, however. In the
current automation workflow, the rendezvousIP is always fully-determined when
the ISO is generated anyway, so there is no need to consider this yet.

## Infrastructure Needed

The TUI binary will need a new git repository, tentatively named agent-utils.

This binary will need to be built into a container image, and that image
shipped in the release payload.
