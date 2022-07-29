---
title: advertise-version-details
authors:
  - "@dhellmann"
reviewers: # Include a comment about what domain expertise a reviewer is expected to bring and what area of the enhancement you expect them to focus on. For example: - "@networkguru, for networking aspects, please look at IP bootstrapping aspect"
  - "@spadgett, console team"
  - "@wking, upgrades team"
  - "@LalatenduMohanty, upgrades team"
  - "@soltysh, workloads team"
approvers:
  - "@sdodson"
api-approvers: # In case of new or modified APIs or API extensions (CRDs, aggregated apiservers, webhooks, finalizers). If there is no API change, use "None"
  - None
creation-date: 2022-07-29
last-updated: 2022-07-29
tracking-link: # link to the tracking ticket (for example: Jira Feature or Epic ticket) that corresponds to this enhancement
  - https://issues.redhat.com/browse/USHIFT-233
  - https://bugzilla.redhat.com/show_bug.cgi?id=1850656
---

# Advertise Version Details

## Summary

We have several use cases for allowing authenticated users to discover
whether they are communicating with full OpenShift or MicroShift, including the
version of the product. We already have the ClusterVersion resource
for OCP, but that requires special privileges to read and is not
present in a MicroShift deployment. This enhancement describes a
standard that both platforms can use to share platform and version
information with API callers.

## Motivation

Users configuring applications to deploy on different versions of
OpenShift may need to adjust their configuration to work differently
on OCP and MicroShift. Ideally this is not needed because
configuration tools would adjust based on the APIs present in the
cluster, without needing to know more detail about the product. In
practice it is useful and often required, because not all cluster
behaviors can be discovered via Kubernetes API availability or version
(for example, the recent service account secret change, or the
impending pod security admission and default SCC changes.)

The built-in Kubernetes API endpoint `/version` reports some version
details, but not the name of the product or distribution, which is
also often useful for making installation-time or configuration-time
choices.

```console
$ oc version
Client Version: 4.11.0-0.nightly-2022-06-23-153912
Kustomize Version: v4.5.4
Server Version: 4.11.0-0.nightly-2022-06-23-153912
Kubernetes Version: v1.24.0+284d62a
```

Today, it is possible for a cluster admin to read the ClusterVersion
resource in an OCP cluster to discover that it is OCP and what version
is running. Limiting ourselves to this approach has two
drawbacks. First, not all console users have cluster admin privileges,
which means that the console cannot show them the cluster version
details. Second, the ClusterVersion API is not present in a MicroShift
deployment, so instead [we created a
ConfigMap](https://github.com/openshift/microshift/pull/776) in the
`kube-public` namespace containing some basic version details. That
means a caller must know two different APIs to check the version and
product they are communicating with. This enhancement describes how we
can standardize on one to make it easier for all authenticated users
to obtain basic product version details for a cluster.

### User Stories

As a console user, I want to see the version of OCP I am using so that
I can refer to the correct version of the documentation and API
references.

As a console developer, I want to be able to obtain the version of OCP
that I am using so that the console app can use different
implementation choices for behaviors that cannot be discovered by
probing the Kubernetes API versions supported.

As an authenticated API caller, I want to obtain the product name and
version of the OpenShift variant I am communicating with so that I can
use it to make decisions on the client side.

As an application developer, I want to be able to determine the
Kubernetes distribution the application is being installed into at
runtime so that I only need to implement one set of installation
logic.

As an OpenShift user without admin privileges, I want to find
information about the version of the product I am using in a
consistent way, regardless of whether I am using standalone OpenShift,
HyperShift, or MicroShift.

### Goals

* Use a standard Kubernetes type, to keep access easy for arbitrary
  API clients. This also lets us avoid introducing a new CRD, so we do
  not increase the overhead for MicroShift.
* Define a standard location for version information that all
  OpenShift variants can support.

### Non-Goals

* This change does not introduce a new way for a cluster admin to
  update the version of a cluster (i.e., to perform an upgrade).
* This change does not introduce a new way for a client to discover
  that a cluster is being upgraded.
* This change does not introduce a new way for a client to discover
  the cluster topology (single-node, HA, external control-plane, etc.).
* This change does not introduce a new way for a client to discover
  other properties of the cluster that might currently only be visible
  using OpenShift-specific APIs.
* This change does not introduce a way for a cluster consumer to tell
  the difference between ROSA, ARO, OSD, HyperShift, or standalone
  OpenShift clusters because we want those to appear the same from the
  workload's perspective.

## Proposal

Because we want arbitrary clients to be able to read and understand
the data without requiring adopting an OpenShift-specific API, we will
use a standard Kubernetes ConfigMap.

Because we want all OpenShift variants to publish the information in
the same way and that the data should not be writable by API callers,
we will specify the name and namespace for the ConfigMap.

Because MicroShift exposes a limited number of namespaces by default,
we need to choose from one of those. The most suitable available
namespace is `openshift`, because it is available in all OpenShift
clusters, regardless of topology or form-factor and is readable by
authenticated users by design and includes other cluster-wide details.

Therefore, we will create the ConfigMap with name `version` in the
`openshift` namespace.

This choice will mean updating the implementation in MicroShift to
change the namespace from `kube-public`, but since the ConfigMap will
be OpenShift-specific we want to avoid collisions that might be
introduced by other distributions in the `kube-public` namespace.

Because we want all OpenShift variants to report the same type of
information, we will keep the contents of the ConfigMap brief and
specify the data key names in this enhancement:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: version
  namespace: openshift
data:
  product: ""
  major: ""
  minor: ""
  patch: ""
  version: ""
```

- `product` should hold a representation of the OpenShift variant
  name. For example, `OCP` for OpenShift in standalone or external
  control-plane configurations of all topologies and `MicroShift` for
  MicroShift instances.
- `major` should hold the major version number of the product as a
  string. For example, for `4.12.0` the value would be `"4"`.
- `minor` should hold the minor version number of the product as a
  string. For example, for `4.12.0` the value would be `"12"`.
- `patch` should hold the patch version number of the product as a
  string. For example, for `4.12.0` the value would be `"0"`.
- `version` should hold the complete version of the product as a
  string. For example, for `4.12.0` the value would be `"4.12.0"`. The
  version string may also include other qualifiers in nightly,
  sprintly, CI, or hot-fix builds. For example,
  `4.11.0-0.nightly-2022-06-23-153912`.

The `version` string is meant for display in user interface
situations.  The `major`, `minor`, and `patch` values are included
separately so that API users do not have to parse the `version` string
before applying rules based on the values.

### Workflow Description

#### Determining the Version of a Cluster via the API

API clients can read the `openshift/version` ConfigMap contents in the
normal way.

#### Determining the Version of a Cluster via the CLI

The `oc login` command reports the `message` value of the
`openshift/motd` ConfigMap when a user successfully authenticates. The
command should be extended to include the `version` string of the
`openshift/version` ConfigMap, when it is present.

The `oc version` command reports version details today. Some of the
information it reports comes from the ClusterVersion resource, which
is not visible to all users. The command should be updated to look at
the `openshift/version` ConfigMap when it is present and the
ClusterVersion resource cannot be read.

#### Publishing the Version in MicroShift

MicroShift will update the `openshift/version` ConfigMap contents on
startup using a value compiled into the main binary. Upgrading
MicroShift requires restarting the service, so there is no need to
continue reconciling the contents of the ConfigMap.

#### Publishing the Version in OCP

OpenShift running in standalone or external control-plane
configurations may be upgraded dynamically in place without loss of
API connectivity. The cluster-version-operator will therefore update
the contents of the ConfigMap as the version of the cluster
changes. During an upgrade, we want to report the current version as
the one from which the cluster is upgrading and not expose that an
upgrade is in progress. The ConfigMap should therefore always contain
the value of the most recent version marked as `Completed` in the
history.

### API Extensions

None

### Risks and Mitigations

There is some risk that an API caller could use the version
information to determine whether the cluster is vulnerable to a known
exploit. API callers will need to be authenticated to read the data,
and they could just try the exploit directly without the version
information.

It is possible on first launch of a cluster for the API server to
respond before the ConfigMap has been written, and therefore for the
caller to fail to detect that the product is an OpenShift
variant. This can be mitigated by populating the data as early as
possible using an empty ConfigMap with only the `product` field filled
in.

There is a similar race condition when restarting MicroShift after an
upgrade when the wrong version information may be presented. Clients
are not as likely to need to query the MicroShift version at runtime
in a production setting, because most deployments will either use
manifests built into the image or use an agent to control application
deployment from a central server. This race should therefore only
affect developer scenarios.

### Drawbacks

None

## Design Details

### Open Questions

None

### Test Plan

It should be possible to test this new behavior via functional tests
without a full end-to-end test suite.

### Graduation Criteria

#### Dev Preview -> Tech Preview

- Ability to utilize the enhancement end to end
- End user documentation, relative API stability
- Sufficient test coverage
- Gather feedback from users rather than just developers
- Enumerate service level indicators (SLIs), expose SLIs as metrics
- Write symptoms-based alerts for the component(s)

#### Tech Preview -> GA

- More testing (upgrade, downgrade, scale)
- Sufficient time for feedback
- Available by default
- Backhaul SLI telemetry
- Document SLOs for the component
- Conduct load testing
- User facing documentation created in [openshift-docs](https://github.com/openshift/openshift-docs/)

#### Removing a deprecated feature

N/A

### Upgrade / Downgrade Strategy

See the workflow section above.

### Version Skew Strategy

N/A

### Operational Aspects of API Extensions

N/A

#### Failure Modes

N/A

#### Support Procedures

N/A

## Implementation History

https://github.com/openshift/microshift/pull/776 implemented an early
version using a different ConfigMap in MicroShift.

https://github.com/openshift/microshift/pull/814 updates the
implementation in MicroShift to match this document.

## Alternatives

We could add the `ClusterVersion` API to MicroShift. This would add an
API owned by a component not present in the cluster
(cluster-version-operator), which would mean at least updating the
MicroShift startup code to write to the API instead of a ConfigMap. It
would also imply that a MicroShift cluster's version could be updated
by using the API, which is not true.

We could do nothing in standalone OpenShift or HyperShift
clusters. This would not solve the problem faced by the console of
presenting version details to users who do not have admin privileges,
and would introduce another way that MicroShift is different from
those configurations.

## Infrastructure Needed

None
