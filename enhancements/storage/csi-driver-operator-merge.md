---
title: csi-driver-operator-merge
authors:
  - jsafrane
reviewers:
  - "@storage-team"
  # TODO: someone from ART
  # TODO: staff engineer for new image names
approvers:
  - TBD
api-approvers:
  - None
creation-date: 2023-09-05
last-updated: 2023-09-05
tracking-link:
  - https://issues.redhat.com/browse/STOR-1437
see-also:
  - "/enhancements/storage/csi-driver-install.md"
replaces:
superseded-by:
---

# Merge CSI driver operators into a single git repository

## Summary

CSI driver operators are OCP specific operators that install and manage CSI drivers shipped by OCP either as part of
payload or as optional operators installed by OLM.

In this document we propose that all these operators are compiled from a single git repository, as they're mostly
copy-paste.

## Motivation

OpenShift comes with these CSI driver operators built by ART and either shipped as part of the OpenShift
payload or via OLM:

* [Alibaba Disk](https://github.com/openshift/alibaba-disk-csi-driver-operator/) (*)
* [AWS EBS](https://github.com/openshift/aws-ebs-csi-driver-operator/)
* [AWS EFS](https://github.com/openshift/aws-efs-csi-driver-operator/) (OLM)
* [Azure Disk](https://github.com/openshift/azure-disk-csi-driver-operator/)
* [Azure File](https://github.com/openshift/azure-file-csi-driver-operator/)
* [GCP PD](https://github.com/openshift/gcp-pd-csi-driver-operator/)
* [GCP Filestore](https://github.com/openshift/gcp-filestore-csi-driver-operator/) (OLM)
* [IBM PowerVS](https://github.com/openshift/ibm-powervs-block-csi-driver-operator/)
* [IBM VPC](https://github.com/openshift/ibm-vpc-block-csi-driver-operator/)
* [OpenStack Cinder](https://github.com/openshift/openstack-cinder-csi-driver-operator/)
* [OpenStack Manila](https://github.com/openshift/csi-driver-manila-operator/)
* [oVirt Disk](https://github.com/openshift/ovirt-csi-driver-operator/) (*)
* [Secret Store](https://github.com/openshift/secrets-store-csi-driver-operator/) (OLM)
* [Shared Resource](https://github.com/openshift/csi-driver-shared-resource-operator/)
* [VMware vSphere](https://github.com/openshift/vmware-vsphere-csi-driver-operator/)

Operators marked by (*) will be removed soon, as OCP stops supporting the particular cloud provider.

All these operators have their own separate github repository (i.e. 15 of them). All of them share the
same [CSI driver operator library in library-go](https://github.com/openshift/library-go/tree/master/pkg/operator/csi),
so their code is quite small and mostly just configure the shared library.

* When a CVE is found in the shared library, it needs to be fixed in all 15 repositories. Similarly, Kubernetes go
  packages (like k8s.io/client-go) needs to be bumped in each of them. This is a lot of work, and it's easy to miss one
  of the repositories.

* Recently we have started adding HyperShift support to these operators (AWS EBS first). This means that we need to add
  a new functionality to all these operators and it's easier to do it in a single repository.

### User Stories

* As an OpenShift engineer, I want to fix a CVE in a vendored package in all CSI driver operators at once, so that I
  don't have to fix it in their individual git repositories separately.
  
* As an OpenShift cluster admin, I don't see any difference in my cluster. All CSI drivers + their operators work as
  before and use the same API.
  
* Explicitly, as OpenShift cluster admin, I am still able to install and uninstall AWS EBS and GCP filestore CSI drivers
  from OLM as before.
  
### Goals

* Simplify maintenance of CSI driver operators that are part of OCP and built + shipped by ART.

### Non-Goals

* Create a generic framework for CSI driver operators to be used by 3rd party vendors.

## Proposal

1. Merge all CSI driver operators listed above into a single repository, github.com/openshift/csi-operator, together
   with the shared CSI driver operator library (i.e. move it from library-go to this repository). This will be a gradual
   process over several OCP releases - we will move few operator at a time.
1. Until all CSI driver operators are merged into the common repository, update them in their existing repos to vendor
   the shared library from github.com/openshift/csi-operator instead of library-go, so we can fix bugs only on a single
   place. Remove all CSI controllers from library-go, if possible.
1. During development, we want to keep using the existing (old) CSI driver operators and their images as the default.
   The new operators will be behind a feature gate (name TBD). `cluster-storage-operator`, observing the feature gate,
   will run a CSI driver operator Deployment with either the old or the new CSI driver operator image name. Both images
   will have the operator binary with the same name, accepting the same cmdline arguments (if
   not, `cluster-storage-operator` can manage the differences based on the feature gate).
1. When merging the operators, share even more code between them. For example, the AWS EBS and GCP PD operators are
   almost identical, so they can share the same code. Individual operators will still have enough flexibility to
   run extra library-go style controllers, e.g. to install a separate Deployment for a webhook or to sync Secrets from a
   different namespace or to change its format to be usable by a CSI driver.

We still want to have separate binary + image for each operator, now built from
github.com/openshift/csi-operator instead of their own repository.

### Workflow Description

The usage of the new CSI driver operator image will be controlled by a feature gate (name TBD) and observed by
`cluster-storage-operator` (CSO).

When CSO syncs (e.g. during cluster installation or after FeatureGates are changed), it will check if the feature gate is
enabled. If so, it will update the CSI driver operator Deployment to use the new image. If not, it will update the CSI
driver operator Deployment to use the old image. If the new image needs different cmdline arguments, CSO will need to
have logic for that, but we currently do not expect any differences between the new and old images except for the image
name and source repository.

When the new image is declared GA, we will remove any logic from CSO and only the new image will be used. 

All failure conditions are the same as with the old CSI driver operator images - CSO will become unavailable / degraded.

#### Variation and form factor considerations [optional]

Standalone OCP (incl. single-node): It will work as before, only the CSI driver operator images will come from a
different repository.

HyperShift: See Implementation Details below.

MicroShift: None of the CSI driver operators listed above are shipped as part of MicroShift. Even though LVM Operator
installs a CSI driver in MicroShift, it is a separate operator + image, not using our code in library-go. We do not
propose any changes in it.

### API Extensions

None. We will reuse existing ClusterCSIDriver object in `operator.openshift.io/v1`.

### Implementation Details/Notes/Constraints [optional]

#### HyperShift [optional]

We already have HyperShift support for AWS EBS CSI driver. I.e. its operator runs control plane of the CSI driver in a
hosted control plane and the managed cluster runs only the node plugin. We're not exactly happy about the operator code,
there is a lot of `if hypershift { /* something special */ }`. We want to refactor it and make it more generic, so that
it can be used by other CSI drivers as well. Azure Disk and Azure File are the first candidates.

In the end, it should be simple to add HyperShift support to a CSI driver operator.

#### Reusing an old repository

https://github.com/openshift/csi-operator repository already exists and contains a failed experiment to have a
monolithic operator for all CSI driver for OCP 3.11. AFAIK, it was never part of ART pipeline. It is configured in
openshift/release (e.g. to run unit tests), but no image is built there. We will completely remove existing code from
the repository and start from scratch.

#### Building and shipping the operators

Right now, ART pipeline builds a separate image for each CSI driver operator:

| Source repository                                                                                                      | ART name*                                    |
|------------------------------------------------------------------------------------------------------------------------|----------------------------------------------|
| [openshift/alibaba-disk-csi-driver-operator](https://github.com/openshift/alibaba-disk-csi-driver-operator/)           | ose-alibaba-disk-csi-driver-operator.yml     | 
| [openshift/aws-ebs-csi-driver-operator](https://github.com/openshift/aws-ebs-csi-driver-operator/)                     | ose-aws-ebs-csi-driver-operator.yml          |
| [openshift/aws-efs-csi-driver-operator](https://github.com/openshift/aws-efs-csi-driver-operator/)                     | ose-aws-efs-csi-driver-operator.yml          |
| [openshift/azure-disk-csi-driver-operator](https://github.com/openshift/azure-disk-csi-driver-operator/)               | ose-azure-disk-csi-driver-operator.yml       |
| [openshift/azure-file-csi-driver-operator](https://github.com/openshift/azure-file-csi-driver-operator/)               | azure-file-csi-driver-operator.yml           |
| [openshift/gcp-pd-csi-driver-operator](https://github.com/openshift/gcp-pd-csi-driver-operator/)                       | ose-gcp-pd-csi-driver-operator.yml           |
| [openshift/gcp-filestore-csi-driver-operator](https://github.com/openshift/gcp-filestore-csi-driver-operator/)         | ose-gcp-filestore-csi-driver-operator.yml    |
| [openshift/ibm-powervs-block-csi-driver-operator](https://github.com/openshift/ibm-powervs-block-csi-driver-operator/) | ose-powervs-block-csi-driver-operator.yml    |
| [openshift/ibm-vpc-block-csi-driver-operator](https://github.com/openshift/ibm-vpc-block-csi-driver-operator/)         | ose-ibm-vpc-block-csi-driver-operator.yml    |
| [openshift/openstack-cinder-csi-driver-operator](https://github.com/openshift/openstack-cinder-csi-driver-operator/)   | ose-openstack-cinder-csi-driver-operator.yml |
| [openshift/csi-driver-manila-operator](https://github.com/openshift/csi-driver-manila-operator/)                       | csi-driver-manila-operator.yml               |
| [openshift/ovirt-csi-driver-operator](https://github.com/openshift/ovirt-csi-driver-operator/)                         | ose-cluster-ovirt-csi-operator.yml           |
| [openshift/secrets-store-csi-driver-operator](https://github.com/openshift/secrets-store-csi-driver-operator/)         | ose-secrets-store-csi-driver-operator.yml    |
| [openshift/csi-driver-shared-resource-operator](https://github.com/openshift/csi-driver-shared-resource-operator/)     | ose-csi-driver-shared-resource-operator.yml  |
| [openshift/vmware-vsphere-csi-driver-operator](https://github.com/openshift/vmware-vsphere-csi-driver-operator/)       | ose-vmware-vsphere-csi-driver-operator.yml |

*) ART name is the name of metadata file in https://github.com/openshift-eng/ocp-build-data/tree/openshift-4.14/images

During development of an OCP release, we want ART to build both old and new operator image, as the new one is used only
when the feature gate (name TBD) is enabled. This will give us opportunity to test the new operator image in CI and by
QE. Before OCP feature freeze, we must decide which operator image will be shipped in the release. The other one will be
removed from the release, i.e. from CI, from ART pipeline and from payload.

Example (using BREW package names + AWS EBS CSI driver operator + 4.15):

* In OCP 4.14, ART builds
  only [`ose-aws-ebs-csi-driver-operator-container`](https://brewweb.engineering.redhat.com/brew/packageinfo?packageID=74505)
  from https://github.com/openshift/aws-ebs-csi-driver-operator repository.

* During OCP 4.15 development, we want ART to build both `ose-aws-ebs-csi-driver-operator-container` (as today)
  and say `ose-aws-ebs-csi-driver-operator-v2-container` from https://github.com/openshift/csi-operator.
  TODO: better name for the new images?
  We will file these tickets / PRs ([source](https://art-dash.engineering.redhat.com/self-service/new-content)):
  * Comet:
    * New _build repository_ for `ose-aws-ebs-csi-driver-operator-v2-container`.
    * New _delivery repository_ for `ose-aws-ebs-csi-driver-operator-v2-container`.
    * Shall we follow the same brew / dist-git / image names as the old one, just add `-v2-`? Or shall we fix the inconsistencies (e.g. `azure-file-...` vs `ose-azure-disk-...`)? 
  * openshift/release:
    * build the image in CI + promote to 4.15.
  * ART:
    * Follow https://art-dash.engineering.redhat.com/self-service/new-content, using almost the same data as in the existing image (e.g. multiarch support)
      * Exception: we will not perform threat model assessment - we're merging code from existing repos.

* Before OCP 4.15 feature freeze we will decide if the `-v2-` image is good enough and file tickets to
  disable builds of either the old or new image. Similarly, we will update `cluster-storage-operator` and its feature gate
  to use only one of these images.
  We will file these tickets + PRs ([source](https://docs.ci.openshift.org/docs/how-tos/onboarding-a-new-component/#removing-a-component-from-the-openshift-release-payload)):
  * PR to update `cluster-storage-operator` and its related-images, so only the "good" image is used.
    * This can be merged before anything below.
  * Jira for ART to stop building the "bad" image, https://issues.redhat.com/browse/ART-1443
    * They will create a PR to update ocp-build-data.
  * When removing the old image: PR to openshift/release to stop building + promoting the image in CI.
  * On slack, coordinate with ART merging of all these PRs
  
* At 4.15 GA (and in the whole 4.15.z stream), ART will build and ship only the "good" image.

* In 4.16, if the "good" image is the old one, we re-submit all tickets to enable building + shipping of the new image
  and continue testing.
  * Drawback: we will have to stop its development + testing until OCP 4.16 is branched, so we can start building the
    image + testing it in 4.16, keeping 4.15 with only a single image.

TBD: is there a better way? This one is quite coordination and/or ticket-heavy. Can ART remove unused images from
payload automatically when no `related-images` refers to them?

### Gradual merge

We will merge the operators one by one, starting with AWS EBS. Second will be Azure Disk and Azure File, as we need to
support HyperShift for them and we want to share code with AWS EBS. The rest will follow as time allows, possibly over
multiple OCP releases. Very provisional plan is to merge AWS EBS, Azure Disk and Azure File in 4.15 and the rest in
4.16.

### Risks and Mitigations

* Churn around adding / removing images in ART pipeline and in Comet may be too much.
* Since each CSI driver operator is different:
  * Shared Resource (and maybe others) deploy an extra validating webhook.
  * Cinder (and maybe others) syncs Secrets to a different format.
  * Manila installs the CSI driver only optionally.
  * AWS sync CA-bundle from a different namespace.
  * vSphere interacts with vSphere API a lot.
  * And many others.
  
  All the operators use shared code from library-go already, so merging them to a single repository will not make them
  worse. Still, it may be more difficult to share even more code e.g. for HyperShift.

### Drawbacks

* We will need to support old images + github repos in all supported z-streams for quite some time, so the real benefit
  will be visible only after few years. Until then, we will have to maintain repositories with the old CSI driver
  operators _and_ the new merged repository. 

* While each CSI driver operator will be a separate binary and a separate image, all their dependencies will be in a
  single repository. `vendor/` dir of this repository will be large, as it will contain many (all?) SDKs of clouds that
  we support.

## Design Details

### Open Questions [optional]

### Test Plan

We have a solid CI to test that CVO runs CSO and CSO runs corresponding CSI driver operator.
We will add CI jobs that enables the feature gate (name TBD) and runs the new CSI driver operator image.

### Graduation Criteria

#### Dev Preview -> Tech Preview

When both QE and CI are happy with the new CSI driver operator image, we will disable the old image and call the new
operator **GA** directly. This will be separate for each CSI driver operator, potentially spanning multiple OCP releases.

No OCP release should ship two CSI driver operators, users can't really test it before GA. They might get a `-ec.x`
release with two images.

#### Tech Preview -> GA

N/A, i.e. go directly to GA

#### Removing a deprecated feature

Just switch to the old images.

### Upgrade / Downgrade Strategy

During OCP upgrade, `cluster-storage-operator` will use a different image names for CSI driver operators for the new
release. This should be transparent to users.

### Version Skew Strategy

N/A

### Operational Aspects of API Extensions

N/A

#### Failure Modes

N/A

#### Support Procedures

Same as today.

## Implementation History

Major milestones in the life cycle of a proposal should be tracked in `Implementation
History`.

## Alternatives

### Merge operators in a single binary

We can easily build all CSI driver operators into a single binary in a single image. This would simplify the build,
but it would make it more difficult to switch between the old and new CSI driver operator images during development
and testing.

The resulting image would be quite large, as it would contain all the dependencies of all CSI driver operators.

## Infrastructure Needed [optional]

N/A (other than the usual CI + QE)