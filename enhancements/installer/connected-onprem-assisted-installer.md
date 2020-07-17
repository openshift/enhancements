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
last-updated: 2020-07-24
status: implementable
see-also:
  - "/enhancements/installer/connected-assisted-installer.md"  
---

# Assisted Installer On-Prem

## Release Signoff Checklist

- [X] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
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

The assistant needs local access to a base RHCOS ISO in order to generate the
Agent ISO. It will then run the following command, passing ignition either via
`stdin` or as an additional argument.

```
coreos-installer iso embed -o /<output-path>/agent.iso /path/to/base/rhcos.iso
```

To facilitate local access, the base ISO image will be included inside the
assisted installer's container image.

#### S3

The cloud implementation stores each generated ISO in AWS S3. This enables the
service to scale its storage requirements as it generates many ISOs for the
creation of many clusters.

The on-premise implementation will only be used once to create a single
cluster, so it does not require a scalable object store. Instead of running an
S3-like service in the on-premise implementation, the assisted installer
service will write the generated ISO to its filesystem and serve that file
through its API directly from the filesystem.

### Risks and Mitigations

Communication between the Agent and the assisted installer must be protected
with TLS. The Agent must be able to trust the assisted installer service so
that it does not receive rogue provisioning instructions.

## Design Details

### Open Questions [optional]

#### RHCOS ISO image delivery

Is there a better way to make the base RHCOS ISO image available locally than
including it in the assistant's container image? The user will end up
downloading RHCOS twice; once for the live ISO that runs the assisted
installer, and once as part of the assisted installer's container image. It is
possible that the former will be in a different format, for example if they
will run the live ISO on a particular virtualization platform.

When run inside a RHCOS live ISO, `coreos-installer` will by default [install
RHCOS using local data
only](https://github.com/coreos/coreos-installer/pull/197); it does not need to
download an image. Is a similar approach possible when running
`coreos-installer iso embed`?

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

How will the component handle version skew with other components?
What are the guarantees? Make sure this is in the test plan.

Consider the following in developing a version skew strategy for this
enhancement:
- During an upgrade, we will always have skew among components, how will this impact your work?
- Does this enhancement involve coordinating behavior in the control plane and
  in the kubelet? How does an n-2 kubelet without this feature available behave
  when this feature is used?
- Will any other components on the node change? For example, changes to CSI, CRI
  or CNI may require updating that component before the kubelet.

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
* less stuff to download
* no need to support a number of potential virtualization platforms
* exposing the assisted installer's API service from a container may be easier in many cases than exposing it from a VM

Downsides:
* the user may not have podman available, even though it ships in RHEL, Fedora, and other distributions
* running containers with podman may be less familiar to users than running a live ISO

## Infrastructure Needed [optional]

Use this section if you need things from the project. Examples include a new
subproject, repos requested, github details, and/or testing infrastructure.

Listing these here allows the community to get the process for these resources
started right away.
