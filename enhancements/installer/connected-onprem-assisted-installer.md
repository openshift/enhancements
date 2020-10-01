---
title: assisted-installer-on-prem
authors:
  - "@mhrivnak"
reviewers:
  - "@avishayt"
  - "@hardys"
  - "@dhellmann"
  - "@beekhof"
  - "@ronniel1"
  - "@filanov"
approvers:
  - "@crawford"
  - "@abhinavdahiya"
  - "@eparis"
creation-date: 2020-07-23
last-updated: 2020-10-06
status: implementable
see-also:
  - "/enhancements/installer/connected-assisted-installer.md"  
---

# Assisted Installer On-Prem

## Release Signoff Checklist

- [X] Enhancement is `implementable`
- [X] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

The assisted installer, [previously proposed and
accepted](connected-assisted-installer.md), includes a service running on
cloud.redhat.com to which agents connect and wait for orders on how to
A) provision the host they are running on, and B) join or form a cluster.

This enhancement proposes to enable a user to run that "assistant" service
on-premise, inside their own network and on their own infrastructure, for
one-time-use to create a new cluster.

## Motivation

Many users will not want a service that can provision hosts inside their
network to be run by a third party outside of their network. Reasons can
include trust, control, and reproducibility. Those users will prefer to run the
service on-premise.

### Goals

* Users can run the entire assisted install workflow with software that runs inside their network.

### Non-Goals

* Disconnected and air-gapped environments are not included in this enhancement.
* Adding Nodes on day 2 will be addressed separately.
* Utilizing a local container registry for mirrored content will be addressed in a future enhancement.
* Running assisted-installer on one of the control plane hosts will be addressed in a future enhancement. For now the assisted-installer must run on a separate host.

## Proposal

A user will visit cloud.redhat.com to download a live ISO with which they can
run the assisted installer on-premise. The RHCOS-based ISO will include:

* the user's pull secret
* a systemd unit file that runs all containers necessary to run the assisted installer

Similar to downloading the current installer, the download will default to the
latest available version of the assisted installer, RHCOS and OpenShift.

### User Stories [optional]

#### Story 1

I want to install OpenShift while running the assisted installer and the entire
installation workflow on infrastructure that I control. I do not want a third
party cloud service to control the provisioning of my hosts from across the
internet. I don't mind downloading content over the internet during the install
workflow.

#### Story 2

I want to automate use of the assisted installer and expect reproducible
results. Utilizing a cloud service does not provide sufficient control or
guarantees of reproducibility.

#### Story 3

I want to test the assisted installer prior to production use and expect
reproducible results. Utilizing a cloud service does not provide sufficient
control or guarantees of reproducibility.

### Implementation Details/Notes/Constraints [optional]

#### RHCOS Live ISO

The user will download an RHCOS-based ISO from cloud.redhat.com, and it will
already have embedded ignition including their pull secret and the systemd unit
files required to run the assisted installer.

The user will boot the live ISO and connect to it with their web browser. The
on-premise assisted installer will itself be run with podman inside the live
ISO.

The user will need to boot the live ISO in a networking environment where one
of these is true:

1. The virtualization host has a L3 address that is routable from the hosts
that will form the new cluster, and the host is configured to port-forward to
the live ISO's VM.

2. The live ISO VM will have a L3 address that is routable from the hosts that
will form the new cluster. This is often referred to as "bridge networking".
Notably, bridge networking is not the default if using libvirt on a RHEL or
similar host; the default is for VMs to be on a subnet behind NAT. When Red Hat
provides documentation on how to use the live ISO, it should reference or
include guidance on how to accomplish such a networking setup.

3. The live ISO is booted on bare metal where it gets an L3 address that is
routable from the hosts that will form the new cluster.

#### Single Use on Day 1 Only

The on-premise assisted installer will run once per cluster. As a live ISO, it
will not utilize any persistent storage. Upon completion of a cluster
installation, the live ISO will be shut down and all state will vanish.

#### Creating the Agent ISO

The assisted installer generates a live ISO that can be booted on hosts and then
used to provision them. It runs the Agent that communicates with the assisted
installer service. The ISO needs to include:

* a URL to where the assisted installer is running
* a CA certificate so it can trust the assisted installer

The assisted-service needs local access to a base RHCOS ISO in order to
generate the Agent ISO. As of RHCOS 4.6, coreos-installer [can use the live
ISO's own block device as the
source.](https://github.com/coreos/coreos-installer/pull/197) That block
device, found on the live ISO host at `/dev/disk/by-label/*coreos`, will be
mounted into the assisted-service's container. assisted-service will run the
following commands, passing ignition either via `stdin` or as an additional
argument.

```
cp /mnt/rhcos.iso ./agent.iso  # likely at some other mount point
coreos-installer iso embed ./agent.iso
```

A [proposal to
coreos-installer](https://github.com/coreos/coreos-installer/issues/382)
suggests adding the ability for it to send output to stdout instead of saving
it as a local file. That would enable assisted-service to avoid making a copy
on the local filesystem, which consumes disk space that in the on-prem case is
backed by RAM.

#### S3

The cloud implementation stores each generated ISO in AWS S3. This enables the
service to scale its storage requirements as it generates many ISOs for the
creation of many clusters.

The on-premise implementation will only be used once to create a single
cluster, so it does not require a scalable object store. Instead of running an
S3-like service in the on-premise implementation, the assisted installer
service will write the generated ISO to its filesystem and serve that file
through its API directly from the filesystem.

#### TLS Trust

Upon booting the assisted installer live ISO, it will generate a TLS
certificate to be used in serving the API and UI. The public part of that
certificate will be added to the ignition for ISOs that it generates, so that
any client connecting to the assisted-service API from a discovered host will
be able to trust the API.

The Common Name in the certificate will be `assisted-api.local.openshift.io`.
See the next section for details on how that will resolve.

#### Hostname or IP for Service URL

When the assisted installer generates Agent ISOs, it needs to include in that
ISO a URL for its own API. To do so, it must determine an IP address or FQDN.
Rather than make user provide that, it will use the following approach.

When the assisted installer live ISO boots, it may get more than one IP
address. It could get both an IPv4 and IPv6 address, or some users may find it
necessary to assign it multiple addresses of one version or the other. Asking
the user to pick just one address is undesirable because:

* it adds a step
* it may require the user to deeply investigate
* the user may need the API and UI to listen on multiple addresses

When ignition is generated for any host that needs to communicate back to the
assisted-service API, it will include an entry that adds each IP address to
that host's `/etc/hosts` file resolving to the name
`assisted-api.local.openshift.io`. That way any client on the host will try all
addresses if necessary to find one that it can route to. The addresses will be
determined by an equivalent of running `hostname -I` on the assisted installer
live ISO.

### Risks and Mitigations

Communication between the Agent and the assisted installer must be protected
with TLS. The Agent must be able to trust the assisted installer service so
that it does not receive rogue provisioning instructions. This is mitigated
as described in the "TLS Trust" section above.

## Design Details

### Open Questions [optional]

### Test Plan

**Note:** *Section not required until targeted at a release.*

### Graduation Criteria

**Note:** *Section not required until targeted at a release.*

#### Examples

These are generalized examples to consider, in addition to the aforementioned
[maturity levels][maturity-levels].

##### Dev Preview -> Tech Preview

- Ability to utilize the enhancement end to end
- End user documentation, relative API stability
- Sufficient test coverage
- Gather feedback from users rather than just developers

##### Tech Preview -> GA 

- More testing (upgrade, downgrade, scale)
- Sufficient time for feedback
- Available by default

**For non-optional features moving to GA, the graduation criteria must include
end to end tests.**

##### Removing a deprecated feature

- Announce deprecation and support policy of the existing feature
- Deprecate the feature

### Upgrade / Downgrade Strategy

Since the on-premise assisted installer is intended for short-term use just to get
a cluster installed, there is nothing stateful to upgrade or downgrade.

The user will retrieve a newer version, or potentially an older version if
older versions are offered, by downloading it from cloud.redhat.com.

### Version Skew Strategy

Not applicable. The on-premise modality of assisted installer does not have its
own versioning.

## Implementation History

Major milestones in the life cycle of a proposal should be tracked in `Implementation
History`.

## Drawbacks

Running the assisted installer on-premise is not as simple of an experience as
utilizing the service from cloud.redhat.com. It requires more work from the
user prior to beginning a cluster installation.

## Alternatives

### Containers Instead of ISO

Instead of distributing the assisted installer as a live ISO, it could be distributed
as a podman-compatible Pod definition that runs containers directly on the
user's machine. The user would copy a `podman` command from cloud.redhat.com and run it
locally.

Advantages include:
* no need to support a number of potential virtualization platforms
* exposing the assisted installer's API service from a container may be easier in many cases than exposing it from a VM

Downsides:
* the user may not have podman available, even though it ships in RHEL, Fedora, and other distributions
* running containers with podman may be less familiar to users than running a live ISO
* in the future the live ISO can run on one of the hosts and serve the bootstrap role. This requires an OS on that host, which the live ISO provides.

## Infrastructure Needed [optional]

Use this section if you need things from the project. Examples include a new
subproject, repos requested, github details, and/or testing infrastructure.

Listing these here allows the community to get the process for these resources
started right away.
