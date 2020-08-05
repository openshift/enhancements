---
title: subscription-content-access
authors:
  - @adambkaplan
reviewers:
  - @gabemontero
  - @sbose
  - @abhgupta
  - @luciddreamz
  - @sideangleside
  - Barnaby Court
  - Chris Snyder
approvers:
  - @bparees
  - @derekwaynecarr
creation-date: 2019-06-19
last-updated: 2019-08-03
Status: implementable
see-also:
  - /enhancements/cluster-scope-secret-volumes/csi-driver-host-injections.md
replaces:
  - https://github.com/openshift/enhancements/pull/214
superseded-by:
---

# Subscription Content Access

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in
      [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

Accessing RHEL subscription content in workloads is a challenge on OpenShift 4.x. Containers can
download RHEL content via `yum install` if their hosts have subscriptions attached. In 4.x our
preferred host OS (RHCOS) is not capable of attaching subscriptions individually. This enhancement
proposal aims to restore the ability for containers to download RHEL content with minimal action
from the developer or cluster admin.

## Motivation

Containers can download RHEL content if their hosts have subscriptions attached. In 3.x, all nodes
were assumed to be RHEL 7 nodes that were subscribed individually. The information needed to access
subscription content (entitlement keys and redhat.repo configurations) was symbolically linked into
the running container at a well-known location that yum/dnf could find. This capability existed in
our patch of Docker, and was carried over into Red Hat’s container toolchain used on OpenShift 4
(cri-o, podman, buildah, etc.).

In OpenShift 4 the preferred operating system (RHCOS) is not capable of attaching subscriptions
individually. This means that by default containers running on OpenShift 4 cannot download RHEL
content. There are at least two use cases where this capability is relevant:

Builds which download RHEL libraries - particularly those that are not available to the UBI images
Kernel modules which need access to RHEL developer libraries.

### Goals

Identify the requirements needed to access subscription content on OpenShift. Describe the
architectural components needed to provide seamless access to subscription content on OpenShift.
Identify dependencies on non-OpenShift components that are needed to make goal #2 achievable.
Provide a rough roadmap for implementation.

### Non-Goals

Describe the detailed implementation of the components described in the architecture. This will be
accomplished in subsequent enhancement proposals.

## Proposal

### User Stories

#### Access RHEL Content in OpenShift Builds

As a developer using OpenShift to build my application I want access to RHEL subscription content in
builds So that I can download RPMs to compile my application

#### Access RHEL Content in Containers

As an OpenShift cluster administrator I want access to RHEL subscription content in containers So
that I can download RPMs to run specific workloads, such as debug utilities or build kernel modules

#### Access Appropriate RHEL Content for Container

As a developer using OpenShift I want to be able to access appropriate RHEL content for my container
So that I download RHEL 8 content in UBI 8 containers And download RHEL 7 content in UBI 7
containers

#### Provide Subscriptions to Multiple Clusters

As a multi-cluster administrator, I want to enable my cluster for entitlements via a declarative API
that works well with GitOps (or similar patterns) so that granting access to subscription content is
not a burden as I scale the number of clusters under management.

### Implementation Details/Notes/Constraints [optional]

To consume RHEL content in OpenShift, the running container must have access to the following:

One or more entitlement keys, which are PEM-encoded certificates. A yum repository definition,
which tells yum/dnf where to access content from.

Yum repository definitions are specific to the major version of RHEL that the running container is
based on - a RHEL 8-based container is not compatible with RHEL 7 content.

This enhancement proposal will detail at a high level the functions of:

1. A cluster-wide subscription manager to generate subscription content access data
2. A projected resource CSI driver to share resources across OpenShift namespaces
3. A subscription content webhook to generate the necessary volume mounts to add subscription
   content access data to containers
4. Extensions to propagate subscription content access data into OpenShift builds.

#### Cluster Subscription Manager

The cluster subscription manager will take as input the `cloud.redhat.com` pull secret, which
already provides authentication for the cluster and provides the means to register clusters with
Red Hat’s cloud manager. The pull secret is then used to obtain the following:

1. An entitlement key associated with the subscription for the OpenShift cluster.
2. For Satellite clusters, the `redhat.repo` yum repo configurations for all supported major
   versions of RHEL. Today these will be RHEL 7 and 8 - future major versions of RHEL must be
   anticipated.

Like RHSM, the cluster subscription manager obtains the entitlement keys associated with the
attached OpenShift subscription. It is assumed that OpenShift entitlements grant access to RHEL
content, but may not grant access to layered content such as EAP or Application Runtimes. As an
alternative, the subscription manager may try to obtain a Simple Content Access certificate which
globally grants access to yum repos available to the customer account. This content must be saved
in a Secret within the `openshift-config` namespace, named `etc-pki-entitlement`.

The cluster subscription manager must also manage entitlement key rotation events. In the event a
cluster's entitlement key is changed, the cluster subscription manager must update the key stored
in `openshift-config/etc-pki-entitlement`.

For clusters linked to a Satellite instance, the cluster subscription manager should also be
responsible for generating `redhat.repo` definitions which point to the Satellite instance.
Separate `redhat.repo` files may need to be generated for RHEL7 and RHEL8 content. Each
`redhat.repo` file will be saved in a ConfigMap within the `openshift-config` namespace, named
`redhat-repo-rhel${n}`, where `${n}` is the major version of RHEL associated with
the `redhat.repo` file.

The full details of this component will be specified in a subsequent enhancement proposal.

#### Projected-resource CSI Driver

Once the entitlement key(s) and yum repository definitions are stored on the cluster, the next step
is to share these definitions across namespaces. This task will fall to the
[projected-resource](/enhancements/cluster-scope-secret-volumes/csi-driver-host-injections.md)
containers storage interface (CSI) driver. This will be a special storage driver that allows
ConfigMaps and Secrets to be shared across namespaces via standard pod volume mounts. Access to
shared Secrets and ConfigMaps will be controlled via cluster-scoped Custom Resources. The
projected-resource CSI driver will be responsible for conducting a subject access review (SAR) check
to ensure that the service account for the pod has permission to mount any projected resources.

The full details of this component are specified in the
[CSI Driver Host Injections proposal](/enhancements/cluster-scope-secret-volumes/csi-driver-host-injections.md).

#### Subscription Injection Operator

The projected-resource CSI driver and associated custom resource are not sufficient to make it
simple to access subscription content in containers. The task of streamlining access to this content
will fall to the Subscription Injection Operator. This mutating admission webhook and associated
components will do the following:

1. Provide a CRD for defining subscription bundles, which consist of
1. A `Secret` with entitlement keys
1. A `ConfigMap` with a `redhat.repo` yum repo configuration
1. Create a set of default subscription bundles for RHEL7 and RHEL8 content, with data sourced from
   the `openshift-config` namespace.
1. Create appropriate Projected Resource shares, RBAC roles, and role aggregations which allow the
   subscription bundle data to be shared across namespaces.
1. Define an annotation that can be added to any pod template to consume subscription content. The
   value for this annotation will determine which RHEL content will be available. For example, a
   `Deployment` with the following pod template:

```yaml
kind: Deployment
apiVersion: apps/v1
metadata:
  name: entitled-deployment
  labels:
    app: entitled
spec:
  replicas: 3
  selector:
    matchLabels:
      app: entitled
  template:
    metadata:
      annotations:
        subscription.openshift.io/bundle: rhel8
      labels:
        app: entitled
    spec: ...
```

would have RHEL8 content available to the pod’s containers. The webhook will add the projected
resource CSI volume and volume mounts to all containers such that RHEL content can be consumed.

The full details of this component are specified in the
[Subscription Injection Operator enhancement proposal](https://github.com/openshift/enhancements/pull/389).

#### Extensions for OpenShift Builds

OpenShift builds will be enhanced to support a narrow set of annotations, which are then passed
through to the build pod. The annotation for the subscription content webhook will be one of the
annotations included in the allowed list - others may also be considered.

### Risks and Mitigations

**Risk:** Sensitive information can be leaked to unauthorized users.

_Mitigations:_ The proposal for the Projected-resource CSI driver includes RBAC protections and
SELinux controls to ensure secrets are protected.

**Risk:** Allowing arbitrary annotations in builds can lead to privilege escalations.

_Mitigation:_ Builds will use an allowlist to block arbitrary annotations from being passed through
to build pods.

**Risk:** Containers on RHEL7 nodes can consume different subscription content. If a RHEL7 node has
a subscription attached, pods that don't use the CSI driver subscription mechanism can access a
different set of content than those pods which do use the driver mechanism.

_Mitigation:_ The subscription injection operator could expose a configuration which applies the
the same injection annotation to every pod. This would ensure every pod consumes the same content.

## Design Details

### Test Plan

N/A - each individual component will have their own test plan.

### Graduation Criteria

The following is a proposed release timeline:

1. The Projected-resource CSI driver needs to reach maturity via a Tech Preview -> GA cycle.
2. The subscription content webhook will explicitly depend on the Projected-resource CSI driver.
   It’s lifecycle is tied down wrt Tech Preview -> GA Builds will require 2 and 3 in order to pass
   subscription content through.
3. The cluster subscription manager can be released as an independent component on its own cadence.
   However, it should employ the conventions of the subscription content webhook to ensure seamless
   behavior.

### Upgrade / Downgrade Strategy

Generally speaking, each of the components in this meta-enhancement will be add-ons, deliverable via
OLM. That said, a case can be made that these should be added to the core OpenShift payload.

### Version Skew Strategy

Each component will be responsible for managing version skew during upgrade.

## Implementation History

Major milestones in the life cycle of a proposal should be tracked in `Implementation History`.

## Drawbacks

Consuming subscription content was a core feature of OCP 3.x, and was provided by subscribing every
node in the OpenShift cluster. If these components are delivered via OLM, then consuming
subscription becomes an “extra” feature.

The projected resource driver also introduces a potential attack vector. Without proper precautions,
an attacker can gain access to sensitive information.

## Alternatives

### Use MachineConfigs to Mount Entitlements

As a work-around, customers can create a MachineConfig that adds the same entitlement keys to
/etc/pki/entitlement/ on every node [1]. CRI-O and buildah are configured by default to mount these
entitlement keys into all running containers.

Instead of using a projected resource storage driver, we could create a mechanism where entitlements
are added to the cluster, and then all MachineConfigs/MachineSets are updated accordingly. This has
a downside of forcing all nodes to be restarted when subscriptions are added or updated. The current
proposal does not have this drawback.

[1] https://access.redhat.com/solutions/4908771

### Automate Copying of ConfigMaps/Secrets

Instead of using a CSI driver to propagate subscription content data, a controller could copy
`Secrets`and `ConfigMaps` directly, without using a CSI driver for indirection. With this approach,
every copy would create another record in etcd, with the associated data taking up additional
etcd storage. This does not scale with the number of namespaces, depending on how many namespaces
need this data.

### Add Subscription Manager Support to RHCOS

Red Hat CoreOS (RHCOS) does not ship with the standard Red Hat Subscription Manager. Attaching
subscriptions via the traditional approach would eliminate the need for all of these components -
the default container mounts in RHCOS and cri-o would automatically propagate the entitlement data
to all pods.

This proposal has been rejected out of the desire to make OpenShift nodes as ephemeral as
Kubernetes Pods. For OpenShift, the subscription belongs to the cluster, and concerns regaring use
and billing are addressed via monitoring and telemetry.

## Infrastructure Needed [optional]

- New Github repos will be needed for each component above.
- New CI templates needed to install the respective components for e2e testing.
- Ultimately this will be introduced as an OLM operator, with attendant images produced by CPaaS


## Open Questions [optional]

- How can the cluster subscription manager be informed of key rotation/revocation events?
