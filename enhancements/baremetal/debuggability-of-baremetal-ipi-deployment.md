---
title: debuggability-of-baremetal-ipi-deployment
authors:
  - "@stbenjam"
reviewers:
  - "@abhinavdahiya"
  - "@dtantsur"
  - "@enxebre"
  - "@hardys"
  - "@juliakreger"
  - "@markmc"
  - "@sadasu"
approvers:
  - TBD
creation-date: 2020-05-15
last-updated: 2020-05-15
status: provisional
see-also:
- https://github.com/openshift/installer/pull/3535
- https://github.com/openshift/enhancements/pull/212
replaces:
superseded-by:
---

# Improve debuggability of baremetal IPI deployment failures

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Open Questions

1. Should the installer error or warn when compute replicas quantity is
   not met, even if at lest 2 workers deployed (enough to get a
   functional cluster)?
2. In order to bubble up information about worker failures to the
   installer, the most likely solution to me seems to use operator
   status as Degraded, with a relevant error message. Could we mark
   machine-api-operator when workers fail to roll out?

## Summary

In OpenShift 4.5, we improved the existing installer validations for
baremetal IPI to identify early problems. Those include identifying
duplicate baremetal host records, insufficient hardware resources to
deploy the requested cluster size, reachability of RHCOS images, and
networking misconfiguration such as overlapping networks or DNS
misconfiguration.

However, a variety of situations exist where deployments fail for
reasons that were not preventable during the pre-install validations.
These failures in baremetal IPI are hard to diagnose. Errors from
baremetal-operator and ironic are often not presented to the user, and
even when they are the installer doesn't provide context about what
action to take.

This enhancement request is a broad attempt at categorizing the types of
deployment failures, and what information we could present to the user
to make identifying root causes easier.

## Motivation

The goal of this enhancement is to improve the day 1 install experience
and reduce the perception of complexity in baremetal IPI deployments.

### Goals

- Any deployment that ends in an unsuccessful install must provide the
  user clear and actionable information to diagnose the problem.

### Non-Goals

- Addressing the underlying causes of the failures is not the goal of
  this enhancement.

## Proposal

Broadly, deployments fail due to problems encountered during these
installation activities:

  - Pre-bootstrap (image downloading, manifest creation, etc)
  - Infrastructure automation (Terraform)
  - Bootstrap
  - Bare Metal Host Provisioning (Control Plane and Workers)
  - Operator Deployment (i.e., those rolled out by CVO)

We believe that since 4.5, pre-bootstrap errors are usually detected,
and useful information is presented to the user about how to rectify the
problem, so this enhancement request will focus on failures that occur
from terraform onward.

### Kinds of deployment failure

#### Infrastructure Automation (Terraform)

Baremetal IPI relies on terraform to provision a libvirt bootstrap
virtual machine, and bare metal control plane hosts. We use
terraform-provider-libvirt and terraform-provider-ironic to accomplish
those goals.

terraform-provider-ironic reports failures when it cannot reach the
Ironic API, or a control plane host fails to provision. In both cases,
we do not provide useful information to the user about what to do.

#### Bootstrap Failures

The bootstrap runs a couple of baremetal-specific services, including
Ironic as well as a utility that populates introspection data for
the control plane.

Bootstrap typically fails for baremetal when we can't download the
machine-os image into our local HTTP cache.  Less common, but still
sometimes seen are that services such as dnsmasq, mariadb, ironic-api,
ironic-conductor, or ironic-inspector fail.

Failures on bootstrap services rarely result in any indication to the
user that something went wrong other than that there was a timeout.

The installer has a feature for log gathering on bootstrap failure that
does not work on baremetal. This should be the first priority, but even
in this case a user still needs to look into an archive containing many
logs to identify a failure.

Ideally there would be some mechanism to identify and extract useful
information and display it to the user.

#### Bare Metal Host Provisioning

Whether the control plane or worker nodes, provisioning of bare metal
hosts can fail in the same ways, although the communication path to
provide feedback is different in each case. For the control plane,
information about failure is presented to the user via terraform. For
workers, it would be through information on the `BareMetalHost`
resource, and the baremetal-operator logs.

Provisioning can fail in many ways. The most difficult to troubleshoot
are simply when we fail to hear back from a host. Buggy UEFI firmware
may prevent PXE, a kernel could panic, or even a network cable may be
unplugged. In these cases, we should inform the user what little
information Ironic was able to discern, but also provide a suggestion
that the most effective way to troubleshoot the problem is examination
of the console of the host.

An infrequent, but possible outcome of deployment to bare metal hosts,
is that Ironic is successful in cleaning, inspecting, and deploying a
host. After Ironic lays down an image on disk and reboots, Ironic marks
the host ‘active’.  However, when the host boots again it’s possible that
there’s a catastrophic problem such as a kernel panic or fail to
configure with ignition. From Ironic's perspective, it's done it's duty,
and is unaware the host failed to come up. The feedback to the user is
only that there was a timeout.

#### Operator Deployment

Operator deployment failures are rarely platform-specific, although
there is one case that should be addressed. When worker deployment
fails, possibly due to provisioning issues like those described above, a
variety of operators may report failures such as ingress, console, and
others that cannot run on the control plane.

When this happens, the installer times out, reports to the user a large
number of operators failed to roll out, and no useful context about what
to do or why the operators failed.

#### User Stories

##### Show more information from terraform

  - As a user, I want terraform to report last_error and status from
    ironic in case of deployment failure.

  - As a user, I want the installer to provide suggestions for causes
    of failure. See the existing work for translating terraform error
    messages that is being done in https://github.com/openshift/installer/pull/3535.

#### Extract relevant logs from the bootstrap

  - As a user, I would like the installer to extract and display error
    messages from bootstrap journal when relevant errors can be
    identified.

#### Implement bootstrap gather

  - As a user, I want the installer to automatically gather logs when
    bootstrap fails on the baremetal IPI platform, like it does for other
    platforms.

See also:
 - https://github.com/openshift/installer/issues/2009

#### Show errors from machine controllers

  - As a user, I want the installer logs to bubble information up from
    either machine-api-operator or cluster-baremetal-operator about why
    workers failed to deploy. These operators should be degraded when
    machine provisioning fails.

#### Callback to Metal3

  - As a user, I want my host to callback to Metal3/Ironic from ignition
    when RHCOS boots.

### Implementation Details/Notes/Constraints



### Risks and Mitigations

Some stories may impact the design of software managed by teams other
than the baremetal IPI team. These including the installer and
machine-api-operator teams, for example.

## Design Details

### Test Plan

**Note:** *Section not required until targeted at a release.*

### Graduation Criteria

**Note:** *Section not required until targeted at a release.*

### Upgrade / Downgrade Strategy

Upgrades/downgrades are not applicable, as these are day 1
considerations only. There is no impact on upgrades or downgrades.

### Version Skew Strategy

As these are day 1 considerations for greenfield deployments, no version
skew strategy is needed.

## Implementation History


## Drawbacks

## Alternatives

An alternative approach would be to provide troubleshooting
documentation and leave users to uncover the root causes of failures on
their own, which is largely what happens today.
