---
title: AWS EFS CSI driver operator via OLM
authors:
  - "@jsafrane"
reviewers:
  - 2uasimojo # from OCP Dedicated team (i.e. "the customer")
  - bertinatto # from OCP storage
approvers:
  - bbennett # As pillar lead?

creation-date: 2021-03-09
last-updated: 2021-06-24
status: implementable
see-also:
  - "/enhancements/storage/csi-driver-install.md"
replaces:
superseded-by:
---

# AWS EFS CSI driver operator via OLM

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [x] Design details are appropriately documented from clear requirements
- [x] Test plan is defined
- [x] Operational readiness criteria is defined
- [x] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

AWS EFS CSI driver (+ corresponding operator) is an optional OCP component. It should be installed through OLM, when
users opts-in.

This document describes existing (unsupported) solution and how to turn it into a supported one, both from code (and
build and shipment), and from user perspective.

## Motivation

Some of our customers want to use Amazon Elastic FileSystem in their clusters. At the same time, not all customers want
to use it, so it should not be installed by default as our AWS EBS CSI driver.

Despite written by Red Hat employees, the existing CSI driver operator
[aws-efs-operator](https://github.com/openshift/aws-efs-operator) uses upstream CSI driver, upstream CSI sidecars and
[the operator itself](https://quay.io/repository/app-sre/aws-efs-operator?tab=tags) is not released through
registry.redhat.com and therefore cannot be supported by us.

### Goals

* Allow users to use AWS EFS volumes in a supported way.
* Allow existing users of community-supported aws-efs-operator to update to Red Hat supported solution.
* Keep existing community community-supported aws-efs-operator features (namely, `SharedVolume` in
  `aws-efs.managed.openshift.io` API group). See below.

### Non-Goals

* Change the EFS CSI driver in any way. All missing functionality there must go through RFE process.

## Existing situation

### [aws-efs-operator](https://github.com/openshift/aws-efs-operator)

Summary of current [aws-efs-operator](https://github.com/openshift/aws-efs-operator) (no judging, just stating the facts):

* It's written using `controller-runtime`.
* Once the operator is installed by OLM, the operator automatically installs the CSI driver, without any CRD/CR.
  * As consequence, user cannot un-install the CSI driver easily.
* By default, it is installed in `openshift-operators` namespace and the CSI driver runs there too.
* It does not create EFS volumes in AWS! It's up to the cluster admin to create EFS volume in AWS, figure in which VPC
  it should be available and set correct security group. See [this KB article](https://access.redhat.com/articles/5025181)
  for suggested procedure.
* It offers `SharedVolume` CRD in `aws-efs.managed.openshift.io/v1alpha1` API group to create PV + PVC for an EFS
  volume. The EFS volume still needs to be created manually, see the previous bullet!
  * All that `SharedVolume` really does is that it creates PV with proper settings + a PVC. This allows unprivileged
    users to create PVs. Cluster admins need to be careful to give permissions to use this CRD only to trusted users.
  * Using `SharedVolume` is purely optional! Cluster admin may create PVs for EFS volumes manually, it's not harder
    than using `SharedVolume`.

### AWS EFS CSI driver

AWS EFS CSI driver is different from the other CSI drivers we ship. Listing the main differences here for completeness.

* It implements dynamic provisioning in non-standard way. Cluster admin must manually create an EFS share in the cloud
  (and figure out related networking and firewalls). The EFS CSI driver will provision volumes out of it as
  *subdirectories* of it. The CSI driver cannot create EFS shares on its own.
  * AWS has a limit of 120 Access Points per EFS volume, i.e. only 120 PVs can be created for a single StorageClass.
    [Upstream issue](https://github.com/kubernetes-sigs/aws-efs-csi-driver/issues/310).
  * Proper dynamic provisioning can come in the future, dealing with the networking and security groups is the hardest
    part there.
* AWS EFS volumes do not have any size enforcement. They can hold as much data as apps put there and someone will have to
  pay for the storage. OCP offers standard metrics for amount of data on PVCs, it's up to the cluster admin to create
  alerts if the consumption gets too high.
* It uses [efs-utils](https://github.com/aws/efs-utils) to provide encryption in transport, which is not included in
  RHEL. We will need to package it, either as an image or RPM package.

## Proposal
High level only, see design details below.

1. Rewrite current aws-efs-operator to use [CSI driver installation functions of library-go](https://github.com/openshift/library-go/tree/master/pkg/operator/csi/csicontrollerset).
   * This allows us to share well tested code to install CSI drivers and implement bugfixes / new features at a single
     place.
   * It will add common features of CSI driver operators which are missing in the current community operator, such as
     proxy injection and restarting the drivers when cloud credentials change.

2. Update (rewrite) current aws-efs-operator to watch `ClusterCSIDriver` CR named `efs.csi.aws.com`.
   * `ClusterCSIDriver` CRD is already provided by OCP and is used to install all other CSI drivers that OCP ships.
     * Every CSI driver operator watches only its CR, with [a defined name](https://github.com/openshift/api/blob/4b79815405ec40f1d72c3a74bae0ae7da543e435/operator/v1/0000_90_cluster_csi_driver_01_config.crd.yaml#L39).
       They don't act on or modify CRs of other CSI drivers.
   * Users must explicitly create the CR to install the CSI driver to allow users to uninstall the driver if they don't
     like it.

3. Keep existing functionality:
   * `SharedVolume` CRD in `aws-efs.managed.openshift.io/v1alpha1` API group keeps working as it is now.
   * Update from the community operator to the supported operator should be as seamless as possible.
   * The operator is still installed via OLM.

4. Ship the operator + CSI driver through ART pipeline.
   * Most of the operator just installs / removes CSI driver, i.e. manages driver's RBAC rules, CredentialsRequest,
     DaemonSet, Deployment, CSIDriver and StorageClasses. See existing
     [AWS EBS operator](https://github.com/openshift/aws-ebs-csi-driver-operator/tree/master/assets) for objects it
     manages, EFS should be very similar in this regard.
   * `SharedVolume` processing on top of it is very trivial, it just creates PV + PVC.

5. Ship [efs-utils](https://github.com/aws/efs-utils) as a base image for the EFS CSI driver image.

6. Update installer to delete Access Points and EFS volumes that have `owner` tag when destroying a cluster.
   * Existence of EFS volume using a VPC / Subnets actively prevents installer to delete them.
     `openshift-install destroy cluster` gets stuck at VPC (or subnet?) deletion, because AWS does not allow to delete
     a network that's used.
   * This way, cluster admins can tag the volumes to be deleted when the cluster is deleted (this is opt in!).
     At least CI should use this approach to ensure there are no leftovers.

### Implementation Details/Notes/Constraints [optional]

### Risks and Mitigations

Currently, all cloud-based CSI drivers shipped by OCP are installed when a cluster is installed on the particular cloud.
AWS EFS will not be installed by default. We will include a note about opt-in installation in docs.

After installation of the new operators, either as a fresh install or upgrading the existing one, users must create the
CR when the operator is installed on a new cluster
([example of CR for AWS EBS](https://github.com/openshift/cluster-storage-operator/blob/master/assets/csidriveroperators/aws-ebs/10_cr.yaml),
EFS will be very similar). This is something that was not necessary in the previous releases of
the operator. We will add a release note and introduce an (info) alert to remind admins to create the CR.

## Design Details

### Rewrite to library-go & ClusterCSIDriver CRD

This should be pretty straightforward. We already ship number of CSI drivers that use library-go. Example usage:
[AWS EBS CSI driver](https://github.com/openshift/aws-ebs-csi-driver-operator/blob/383a7638b37a4b9a9831c7747e8c499eedcf030f/pkg/operator/starter.go#L67).
EFS CSI driver operator should not be much more complicated.

Library-go does not support un-installation of CSI driver (and static resources created by `StaticResourcesController`),
this must be implemented there.

Note: When the *operator* is un-installed, nothing happens to the CRD, CR, the installed CSI driver, existing PVs and
PVCs. Everything is working, just not managed by the operator. When the *operand* is un-instaled by removing the
operator CR, the driver will be removed. Existing PVs, PVCs and SharedVolumes will be still present in the API server,
however, they won't be really usable. Nobody can mount / unmount EFS volumes without the driver.

### `SharedVolume` CRD

Since `SharedVolume` is trivial to handle, just creates PV + PVC, we leave handling of the CRD as a implementation
detail. We may try to copy code from the community operator, but if it's hard to combine library-go and
controller-runtime in a single binary, we may as well reimplement the handling to library-go style controller.

### Upgrade from the community operator

Prerequisites:
* The community aws-efs-operator is installed in OCP 4.8 cluster in `openshift-operators` namespace. The CSI driver runs
in the same namespace too.
  * The operator `Subscription` contains the community catalog as the operator source.

Expected workflow (confirmed with OLM team):

1. User upgrades to 4.9. Nothing interesting happens at this point, because the operator `Subscription` still points
   to the community source. The "old" operator is still running.
2. User edits the `Subscription` and updates the `source:` to the Red Hat certified catalog.
   (Alternatively, user may un-install the community operator and install the supported one.)
3. OLM reconciles this change, checks the new catalog and if there is a newer operator there, it will install it as an
   update. In other words, we need to ship a newer version in the Red Hat catalog than in the community one.
4. The "new" CSI operator starts. It does not see `ClusterCSIDriver` CR for EFS and only emits Info level alert that
   there is no CR and does nothing. This means the old CSI driver is still running (installed by the "old" operator)
   and any apps that use it are still running.
5. User reads release notes, documentation or alert and creates th CR.
6. The "new" operator "adopts" the CSI driver objects. Namely, it updates DaemonSet of the CSI driver, which then does
   rolling update of the driver pods, using supported images. We won't move the driver across namespaces to
   `openshift-cluster-csi-drivers` to have the "adoption" possible.

To adopt the old objects, the new operator must use the same names for all objects created by the old operator.

### [efs-utils](https://github.com/aws/efs-utils)

We need to get efs-utils into the container with the EFS CSI driver to enable data encryption in transport. efs-utils
are just two Python scripts, which create / monitor stunnel connections between OCP nodes and NFS server somewhere in
AWS cloud.

We've chosen to use base image approach. There are two reasons: 1) we know the drill, and 2) we don't want
to support a generic RPM that can be used outside of OCP.

1. Fork `github.com/aws/efs-utils` into `github.com/openshift/aws-efs-utils`.
2. Create a new base image with the utilities, say `ose-aws-efs-utils-base`:
   ```Dockerfile
   FROM: ocp-4.9:base
   RUN yum install stunnel python <and any other deps>
   COPY <the utilities> /usr/bin
   ```
3. The EFS CSI driver then uses it as the base image:
   ```Dockerfile
   FROM registry.ci.openshift.org/openshift/release:golang-1.15 AS builder
   ... build the image ...

   FROM ose-aws-efs-utils-base
   COPY --from=builder <the driver binary> /usr/bin/
   ```

It would be nice if we did not ship `ose-aws-efs-utils-base` anywhere, so people can consume the EFS utils only through
the EFS operator. **We do not want to maintain it for everyone.**

### Code organization

* Create new  `github.com/openshift/aws-efs-csi-driver-operator` repository for the operator code and start from
  scratch there.
* Leave existing `github.com/openshift/aws-efs-operator` untouched, for the community version that may be maintained
  for older OCP releases.
* Fork the CSI driver into `github.com/openshift/aws-efs-csi-driver`.
* Fork `github.com/aws/efs-utils` into `github.com/openshift/aws-efs-utils`.

### Open Questions

**How to actually hide the community operator in 4.9 and newer operator catalogs?** We don't want to have two operators
in the catalog there. We still want to offer the community operator in 4.8 and earlier though.

  * Right now it seems there will be two operators available in OLM and it's not that bad.

### Test Plan

* Automatic jobs:
  * Run CSI certification tests with the CSI driver:
     1. Install OCP on AWS.
     2. Install the operator.
     3. Create AWS EFS share (this may require extra privileges in CI and figure out the networking!)
     4. Set up dynamic provisioning in the driver (by creating a StorageClass).
     5. Run CSI certification tests.
  * Run upgrade tests:
     1. Install OCP on AWS.
     2. Install the operator.
     3. Create AWS EFS share (this may require extra privileges in CI and figure out the networking!)
     4. Create an dummy app that uses EFS share.
     5. Upgrade the EFS operator (via OLM).
     6. Check the app survived.

* Manual tests:
  * Upgrade from the community CSI driver. With some application pods using volumes provided by the community driver.
    There should be no (long) disruption of the application.

### Graduation Criteria

#### Dev Preview -> Tech Preview

* The functionality described above is implemented.
* At least manual testing passed (CI may need dynamic provisioning in the driver).
* End user documentation exists.

#### Tech Preview -> GA

TODO. Mainly all tests in CI & feedback from the users (OSD!)

- More testing (upgrade, downgrade, scale)
- Sufficient time for feedback
- Available by default
- Backhaul SLI telemetry
- Document SLOs for the component
- Conduct load testing

#### Removing a deprecated feature

TODO: shall we deprecate `SharedVolume`?

### Upgrade / Downgrade Strategy

TODO. Nothing special expected here. No downgrade in OLM!

If applicable, how will the component be upgraded and downgraded? Make sure this
is in the test plan.

Consider the following in developing an upgrade/downgrade strategy for this
enhancement:
- What changes (in invocations, configurations, API use, etc.) is an existing
  cluster required to make on upgrade in order to keep previous behavior?
- What changes (in invocations, configurations, API use, etc.) is an existing
  cluster required to make on upgrade in order to make use of the enhancement?

Upgrade expectations:
- Each component should remain available for user requests and
  workloads during upgrades. Ensure the components leverage best practices in handling [voluntary disruption](https://kubernetes.io/docs/concepts/workloads/pods/disruptions/). Any exception to this should be
  identified and discussed here.
- Micro version upgrades - users should be able to skip forward versions within a
  minor release stream without being required to pass through intermediate
  versions - i.e. `x.y.N->x.y.N+2` should work without requiring `x.y.N->x.y.N+1`
  as an intermediate step.
- Minor version upgrades - you only need to support `x.N->x.N+1` upgrade
  steps. So, for example, it is acceptable to require a user running 4.3 to
  upgrade to 4.5 with a `4.3->4.4` step followed by a `4.4->4.5` step.
- While an upgrade is in progress, new component versions should
  continue to operate correctly in concert with older component
  versions (aka "version skew"). For example, if a node is down, and
  an operator is rolling out a daemonset, the old and new daemonset
  pods must continue to work correctly even while the cluster remains
  in this partially upgraded state for some time.

Downgrade expectations:
- If an `N->N+1` upgrade fails mid-way through, or if the `N+1` cluster is
  misbehaving, it should be possible for the user to rollback to `N`. It is
  acceptable to require some documented manual steps in order to fully restore
  the downgraded cluster to its previous state. Examples of acceptable steps
  include:
  - Deleting any CVO-managed resources added by the new version. The
    CVO does not currently delete resources that no longer exist in
    the target version.

### Version Skew Strategy

N/A, it's managed by OLM and allows to run on any OCP within the boundaries set by the operator metadata.

## Implementation History

## Drawbacks

## Alternatives

## Infrastructure Needed [optional]

We need to create / delete EFS shares in CI.

* This may require new permissions in CI.

