---
title: openstack-supported-versions
authors:
  - "@EmilienM"
  - "@mdbooth"
reviewers:
  - "@mandre"
  - "@pierreprinetti"
approvers:
  - "@mandre"
  - "@pierreprinetti"
creation-date: 2020-11-04
last-updated: 2020-11-17
status: implementable
---

# OpenStack supported versions

- [OpenStack supported versions](#openstack-supported-versions)
  - [Release Signoff Checklist](#release-signoff-checklist)
  - [Summary](#summary)
  - [Motivation](#motivation)
    - [Goals](#goals)
    - [Non-Goals](#non-goals)
  - [Proposal](#proposal)
    - [Design Details](#design-details)
    - [User Stories](#user-stories)
      - [Check the OpenStack version](#check-the-openstack-version)
      - [Ignore the version check](#ignore-the-version-check)
        - [Example Usage](#example-usage)
    - [Test Plan](#test-plan)
    - [Graduation Criteria](#graduation-criteria)
      - [Dev Preview -> Tech Preview](#dev-preview---tech-preview)
      - [Tech Preview -> GA](#tech-preview---ga)
    - [Infrastructure Needs](#infrastructure-needs)
    - [Drawbacks](#drawbacks)

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [x] Design details are appropriately documented from clear requirements
- [x] Test plan is defined
- [x] Graduation criteria
- [ ] User-facing documentation is created in [openshift/docs]

## Summary

OpenShift is not supported when running on top of an unsupported verison
of OpenStack.
While the [Support Matrix](https://access.redhat.com/articles/4679401) is
well documented, OpenShift doesn't check which version of
OpenStack is running before attempting an operation (e.g. installation or
upgrade). 
Running OpenShift over an unsupported version of OpenStack is problematic
and can lead to unstable infrastructure. To avoid that, it is proposed
to programmatically verify the version of OpenStack is supported
otherwise stop an installation or block and upgrade.

## Motivation

### Goals

- Disallow the installation or upgrade of OpenShift done via the some
  operator condition or check to run against a cloud where the OpenStack
  version is not supported.
- Allow an administrator to ignore the OpenStack version check and force the
  installer to proceed anyway. This can be useful if the OpenStack cloud
  is partially upgraded to a major version or if there is a Support
  Exception granted to run that deprecated version of OpenStack.

### Non-Goals

- Backport this feature to stable releases.

## Proposal

In order to implement this feature fully, the following changes must be made:

- Update gophercloud vendoring to (at least) [b56bf1](https://github.com/gophercloud/gophercloud/pull/2037/commits/5a1daf082451587459058cc539c53f2ae1aaaef3)
  so we can benefit from the apiversions package.
- Create a constant that will list all the supported versions
  of Compute API based on the upstream manual.
- The implementer might decide to use a regexp instead of an array.
- In the openshift-installer, add a new validation against the OpenStack
  cloud to make sure that the Compute API version is supported.
- Create a configuration flag `ignoreVersionCheck` that when set to
  true will send a warning if the OpenStack version isn't supported and
  proceed anyway.
- The upgrade of OpenShift should warn an admin about the incompatibility
  and possibly block the upgrade.

### Design Details

The OpenStack Compute service (also known as Nova) is always deployed
in OpenStack clouds. Also, it is one of the services that bumps the
microversions so regularly that is it possible to figure out
what version of OpenStack is running based on the maximum version of
Nova API that is available for a cloud.
There's a contractual given here, which is: Nova will not bump the microversion
of a stable release. There's also an assumption that Nova will always bump the
microversion between stable releases (i.e. add or change the public API).
This is a reasonable assumption which is unlikely to ever be broken.
If it were ever not true we would still have an opportunity to look for
alternatives.

Therefore, it is proposed that we use that information as a reliable
way of checking if the version of OpenStack that is running is
actually supported.
The version check should live in the Installer pre-flight checks near
the other verifications that we do.

The Nova API versions that are supported will be maintained in
the installer code and will follow the recommendations from
the [upstream Nova API manual](https://docs.openstack.org/nova/latest/reference/api-microversion-history.html).

There are multiple reasons where an administrator would want to ignore
the OpenStack version check:
- the administrators know what they are doing and understand that at this
  point they aren't supported anymore.
- the administrators have performed a major upgrade or Fast-Forward-Upgrade
  of OpenStack and the operation is partially finished; however
  they need to proceed to an OpenShift operation (e.g. fresh install, scale up).
  It it possible to run an OpenStack cloud with various versions of
  APIs, as long as compatibility constraints are respected (out of
  scope for this document).
- the administrators have a Support Exception and an OpenStack upgrade
  isn't possible at the moment of the operation.

In that case, we would use a new configuration flag named `ignoreVersionCheck`.

### User Stories

#### Check the OpenStack version

As an enterprise OpenShift cluster administrator, when I run an operation
against my OpenShift cluster, I want to make sure that the version of
OpenStack that is running, is actually supported.
If not, I would like to see the operation to fail early.

#### Ignore the version check

As an enterprise OpenShift cluster administrator, I know what I'm doing
and need to ignore the version check in a case where I absolutely need
to install or upgrade OpenShift.
Performing a scale operation of an unsupported OpenStack version would
require one of:

1. The underlying OpenStack has been downgraded
2. OCP has removed support for the underlying OpenStack
3. The underlying OpenStack has been upgraded to an unsupported version

We don't support (1), (2) isn't likely to happen and (3) would imply
an OpenStack regression.
Everything else being correct this should just be a sanity check so it
shouldn't hurt, but it does add a potential source of failure.
Therefore, ignoring the version check should only be used for a fresh
install of OCP.

##### Example Usage

If the administrator wants to ignore the version validation, they would
need to explicitly configure it, see the following example:

```yaml
apiVersion: v1
baseDomain: example.com
metadata:
  name: test-cluster
platform:
  openstack:
    cloud: mycloud
    ignoreVersionCheck: true
    ...
pullSecret: '{"auths": ...}'
sshKey: ssh-ed25519 AAAA...
```

### Test Plan   

Unit tests and validations for this feature will be added to the installer
to make sure that correct usage is enforced and that this feature does not
hinder the usage of other features. To ensure GA readiness, it will be
vetted by the QE team as well to make sure that it works with the following
use cases:
- running the installer against a supported version, deployment should work.
- running the installer against an unsupported version, deployment should
  not work.
- running the installer against an unsupported version and set
  `ignoreVersionCheck` to true, deployment should work.
- upgrading OpenShift against a supported version, upgrade should work.
- upgrading OpenShift against an unsupported version, upgrade
  should be blocked.
- upgrading OpenShift against an unsupported version, upgrade
  should be not blocked if `ignoreVersionCheck` is set to true.

### Graduation Criteria

This enhancement will follow standard graduation criteria.

#### Dev Preview -> Tech Preview

- Ability to utilize the enhancement end to end
- End user documentation, relative API stability
- Sufficient test coverage
- Gather feedback from users rather than just developers

#### Tech Preview -> GA

- Community Testing
- Sufficient time for feedback
- Upgrade testing from 4.6 clusters utilizing this enhancement to later releases
- Downgrade and scale testing are not relevant to this enhancement
- E2E testing is not necessary for this feature

### Infrastructure Needs

- Supported version of OpenStack cluster
  - PSI

### Drawbacks

There is a risk (small, but it exists) that one day an microversion of Nova API
is backported to an unsupported version of OpenStack and therefore the installer
would think that version of OpenStack is supported while it's not.
Hopefully, this has never happened and the frequency of OpenStack major releases
isn't that fast at this point.
