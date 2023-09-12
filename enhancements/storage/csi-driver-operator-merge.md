---
title: csi-driver-operator-merge
authors:
  - jsafrane
reviewers:
  - "@storage-team"
  - "joepvd" # ART
approvers:
  - TBD
api-approvers:
  - None
creation-date: 2023-09-05
last-updated: 2023-09-12
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

* When a feature or bugfix added to the shared library, it needs to be re-vendored in all 15 repositories. Similarly,
  Kubernetes go packages (like k8s.io/client-go) needs to be bumped in each of them. This is a lot of work, and it's
  easy to miss one of the repositories. For example, backporting a bugfix leads to a lot of PRs: (1 library-go + 15
  repos that vendor it) * X supported releases

* Recently we have started adding HyperShift support to these operators (AWS EBS first). This means that we need to add
  a new functionality to all these operators, and it's easier to do it in a single repository.

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

1. Merge all CSI driver operators listed above into a single repository, github.com/openshift/csi-operator. This will be
   a gradual process over several OCP releases - we will move few operators at a time.
1. After all operators are merged into csi-operator, move CSI controllers from library-go to the csi-operator. _We may
   do it earlier, if we need a bigger refactoring of the controllers, but we don't plan it right now._
1. When merging the operators, share even more code between them. For example, the AWS EBS and GCP PD operators are
   almost identical, so they can share the same code. Individual operators will still have enough flexibility to
   run extra library-go style controllers, e.g. to install a separate Deployment for a webhook, sync Secret from a
   different namespace or change the Secret format to be usable by a CSI driver.
1. We do not want to disrupt CI / nightlies. See "Building and shipping the operators" section how, together with ART
   team, we plan to switch building of the images from the old repository to openshift/csi-operator.
   
We want to keep existing behavior of the CSI driver operators as much as possible:

* All operators will use the same leader election locks as before.
* All operators will report errors to upper layers (cluster-storage-operator, OLM) in the same _style_, i.e. via
  conditions in the `ClusterCSIDriver` object. We may add / remove / rename some conditions though, clearing the old
  ones to make upgrade safe.
* We still want to have separate binary + image for each operator, now built from github.com/openshift/csi-operator
  instead of their own repository.

### Workflow Description

Workflow of existing components does not change at all. For CSI drivers that are part of payload,
cluster-version-operator (CVO) installs cluster-storage-operator (CSO). CSO checks the platform where OCP runs and
installs corresponding CSI driver operators. Only the image name may change here, depending on the option we choose in
"Building and shipping the operators" section.

All failure conditions are the same as with the old CSI driver operator images - CSO will become unavailable / degraded.

#### Variation and form factor considerations [optional]

Standalone OCP (incl. single-node): It will work as before, only the CSI driver operator images will come from a
different repository.

HyperShift: Only AWS EBS CSI driver runs its control plane in the managed control plane namespace. It will
work as before. Merging the operators will allow us to add HyperShift support to other CSI drivers as well.

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

`openshift4/csi-operator` is already listed in Comet as Operator image and Deprecated, but AFAIK Comet tracks images and
distgit repos, not github repos. We do not plan to re-use `csi-operator` _image_ nor distgit.

#### CSI operator maintenance

Right now, different CSI driver operators are co-maintained by different teams:

Shift on Stack (i.e. OpenStack):

* OpenStack Cinder
* OpenStack Manila

IBM:

* IBM PowerVS
* IBM VPC

oVirt:

* oVirt Disk (to be removed soon)

Alibaba:

* Alibaba Disk (to be removed?)

Build team:

* Shared Resource

OCP storage:

* All the rest.

Co-maintenance means that OCP storage team knows best how to run an operator in OCP (e.g. how to set replicas, their
topology, tolerations etc), how to report the operator status to upper layers (CSO, CVO or OLM) and how to integrate it
with other OCP components. The platform-specific teams know best how to run a CSI driver - what cmdline arguments and
env. variable it needs, what Secret or other configuration it needs etc.

We want to keep this co-maintenance model, however, we want to refactor the operators more aggressively to a common
code. For example, the `assets/` directory of each operator is mostly copy-paste from an earlier operator and fixing
anything there leads to too many PRs. Similarly, `starter.go` is often very similar to the other operators.

The other teams will still be responsible for their platform-specific code, e.g. adding extra controllers
to sync / modify driver's Secrets, running extra Deployment with a webhook, or even adding extra sidecars to the CSI
driver like IBM vpc-node-label-updater. We want to reduce copy-paste of the common code, which is easier if the operator
code is in a single repository.

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

During development of an OCP release, we will merge an CSI driver operator to github.com/openshift/csi-operator
without any testing in CI. We will keep as close to the original operator as possible, and we will test the new image
manually (or with some tricks in CI if possible, see below).

Once we're happy with the results, we will follow ART's guidelines and flip building of an existing image from the old
repo to github.com/openshift/csi-operator. The resulting image name will be the same. This will require a tight
cooperation with ART. We expect guidelines
like [Changing the component name of a second level operator](https://docs.ci.openshift.org/docs/how-tos/onboarding-a-new-component/#changing-the-component-name-of-a-second-level-operator) -
we're not changing name of an operator, but we want to change the source repository of the image.

Example:

* In OCP 4.14, ART builds
  only [`ose-aws-ebs-csi-driver-operator-container`](https://brewweb.engineering.redhat.com/brew/packageinfo?packageID=74505)
  from https://github.com/openshift/aws-ebs-csi-driver-operator repository.

* In 4.15, we merge the operator into github.com/openshift/csi-operator into `/legacy` directory, including
  its own `go.mod` and `vendor/`, and do not change any code of it. This way, we can ensure the code will be 100% the
  same as the
  old operator
  * We can test the new image pre-merge manually using:
    `oc adm release new --from=<the latest 4.15 nightly> aws-ebs-csi-driver-operator=quay.io/jsafrane/my-ebs-operator:1 --to=quay.io/jsafrane/test-release:1` and install / upgrade from `test-release:1`.
  * _We could trick CI to build `aws-ebs-csi-driver-operator` image from the new repo and use it in presubmit
    tests there. But we can't promote it anywhere, as it would overwrite the old image from
    github.com/openshift/aws-ebs-csi-driver-operator. Some experiments are needed here._

  The openshift/csi-operator repository should look like this at this point:
  ```
  ├── assets          # Common assets for all CSI drivers (empty now)
  ├── cmd             # All commands for all CSI driver operators (empty now)
  ├── pkg             # All common code for all CSI driver operators (empty now)
  ├── test            # All tests for all CSI driver operators (empty now)
  ├── vendor          # Global vendor dir (empty now)
  ├── legacy          # "Old" CSI driver operators that just merged here.
  │   └── aws-ebs-csi-driver-operator
  │       ├── assets
  │       │   ├── assets.go
  │       │   ├── cabundle_cm.yaml
  │       │   └── ... # all other assets
  │       ├── cmd
  │       │   └── aws-ebs-csi-driver-operator
  │       │       └── main.go
  │       ├── pkg
  │       │   ├── dependencymagnet
  │       │   │   └── dependencymagnet.go
  │       │   ├── operator
  │       │   │   ├── starter.go
  │       │   │   ├── starter_test.go
  │       │   │   ├── storageclasshook.go
  │       │   │   └── storageclasshook_test.go
  │       │   └── version
  │       │       └── version.go
  │       ├── test
  │       │   └── e2e
  │       │       └── manifest.yaml
  │       ├── vendor
  │       │   └── ... # all packages vendored by the old operator
  │       ├── go.mod  # go.mod + go.sum from the old operator
  │       └── go.sum
  ├── Dockerfile.aws-ebs
  └── Dockerfile.aws-ebs.test
  ```
  `Dockerfile.aws-ebs` + `Dockerfile.aws-ebs.test` will be in the root directory, so we can re-use it in the next step.
  They will build the operator + its test image from the `legacy/` directory at this point.

* We coordinate with ART to switch building of `ose-aws-ebs-csi-driver-operator-container` image from
  github.com/openshift/aws-ebs-csi-driver-operator / `Dockerfile.rhel7` to
  github.com/openshift/csi-operator / `Dockerfile.aws-ebs` in a semi-atomic way.
  * With a quick pre-merge test in CI, if possible.
  * Goal: nightlies should be green all the time.
  * TBD: exact procedure & tickets to file. Current high level idea:
    1. PR against openshift/release to stop promoting the CI operator image
       from `openshift/aws-ebs-csi-driver-operator` jobs and start promoting it from `openshift/csi-operator`.
    2. PR against openshift/ocp-build-data to switch the source github repo + Dockerfile
       for `ose-aws-ebs-csi-driver-operator-container` image.
    3. Somehow coordinated merge of these two.
      * _Can we merge 1. before 2. to see if / how it breaks CI builds? We could be able to revert back to the working
        config without any extra approvals._

* After the switch, we start actually refactoring and merging the operator code to shared packages and so on. At this
  time, we will have CI in place for all our PRs in the repo. We will re-use `Dockerfile.aws-ebs`
  and `Dockerfile.aws-ebs.test` to build the image, so we don't need to change anything in ART build data.

  After the refactoring, the repo should look like this:
  ```
  ├── assets
  │   ├── assets.go
  │   └── generated   # We want to generate the assets, see below.
  │       └── aws-ebs
  │           ├── cabundle_cm.yaml
  │           └── ... # other AWS EBS driver assets
  ├── cmd
  │   └── aws-ebs-csi-driver-operator
  │       └── main.go
  ├── pkg
  │   ├── aws-ebs
  │   │   └── # AWS EBS specific-code
  │   └── common
  │       └── starter.go # and any other code shared by all CSI drivers
  ├── test
  │   └── e2e
  │       └── aws-ebs
  │           └── manifest.yaml
  ├── vendor
  ├── Dockerfile.aws-ebs
  ├── Dockerfile.aws-ebs-test
  ├── go.mod
  └── go.sum
  ```

There will be two major points when things can break:

1. When we switch building of `ose-aws-ebs-csi-driver-operator-container` from openshift/aws-ebs-csi-driver-operator to
   openshift/csi-operator. Since the code will be 100% the same, we think it's safe to do. Any revert must be
   coordinated with ART in ocp-build-data and openshift/release repositories.
2. After our refactoring, we will switch building of `Dockerfile.aws-ebs` from `legacy/aws-ebs-csi-driver-operator/cmd/`
   to `cmd/`. At this time we will have CI that should catch any breakage. Any revert must be done in
   openshift/csi-operator repo, which is under our control.

All CSI driver operators will be merged in a similar way, i.e. in `legacy/` directory first.

Drawbacks:

* When anything goes wrong in switching the source repositories, it's ART who will need to roll back things, we cannot
  do it ourselves.

Advantages:

* Keeping image names, i.e. no new Comet repos (distgits, brew packages, etc.)
* Harder (but not impossible) to test before the switch. In ideal case, we're switching just the source repository
  of the same image, the actual operator code should be the same.

#### Gradual merge

We will merge the operators one by one, starting with AWS EBS. Second will be Azure Disk and Azure File, as we need to
support HyperShift for them and we want to share code with AWS EBS. The rest will follow as time allows, possibly over
multiple OCP releases.

Very provisional and optimistic plan:

* AWS EBS, Azure Disk and Azure File in 4.15.
* The rest in 4.16 or later.

#### Generated YAML files

All CSI driver YAML files look very similar today. We plan to generate them from a single set of templates, so that we
don't need to maintain them separately. Exact details about the generator are TBD. We want something like kustomize,
but better integrated with the operator code.

### Risks and Mitigations

* Since each CSI driver operator is different, it may be hard to merge all their code. Some differences are:
  * Shared Resource (and maybe others) deploy an extra validating webhook.
  * Cinder (and maybe others) syncs Secrets to a different format.
  * Manila installs the CSI driver only optionally.
  * AWS sync CA-bundle from a different namespace.
  * vSphere interacts with vSphere API a lot.
  * AWS EFS and GCP Filestore are OLM operators.
  * And many others.

  All the operators use shared code from library-go already, so merging them to a single repository will not make them
  worse. Still, it may be more difficult to share even more code e.g. for HyperShift.

* Since we plan to generate also YAML files for the CSI drivers, there is a risk that an exotic CSI driver will
  require a different YAML file than the others. We will keep possibility for a CSI driver to provide its own YAML file,
  not using the generator.

### Drawbacks

* We will need to support old images + github repos in all supported z-streams for quite some time, so the real benefit
  will be visible only after few years. Until then, we will have to maintain repositories with the old CSI driver
  operators _and_ the new merged repository.
  Listing nr. of merge commits (i.e. nr. of PRs) in each release branch:

  |                                       | 4.10 | 4.11 | 4.12 | 4.13 |
  |---------------------------------------|------|------|------|------|
  | alibaba-disk-csi-driver-operator      | 2    | 0    | 1    | 1    |
  | aws-ebs-csi-driver-operator           | 1    | 0    | 7    | 4    |
  | aws-efs-csi-driver-operator           | 4    | 1    | 1    | 1    |
  | azure-disk-csi-driver-operator        | 3    | 2    | 3    | 1    |
  | azure-file-csi-driver-operator        | 0    | 1    | 1    | 1    |
  | csi-driver-manila-operator            | 3    | 3    | 4    | 3    |
  | csi-driver-shared-resource-operator   | 0    | 0    | 2    | 1    |
  | gcp-filestore-csi-driver-operator     |      |      | 2    | 1    |
  | gcp-pd-csi-driver-operator            | 1    | 0    | 1    | 1    |
  | ibm-powervs-block-csi-driver-operator |      |      | 1    | 3    |
  | ibm-vpc-block-csi-driver-operator     | 4    | 0    | 1    | 1    |
  | openstack-cinder-csi-driver-operator  | 2    | 1    | 2    | 2    |
  | vmware-vsphere-csi-driver-operator    | 9    | 6    | 5    | 5    |

  Most of the PRs are CVEs and high severity bugs already. 

* While each CSI driver operator will be a separate binary and a separate image, all their dependencies will be in a
  single repository. `vendor/` dir of this repository will be large, as it will contain many (all?) SDKs of clouds that
  we support.
  * There is a risk that the operators will require different versions of a vendored package. So far, we kept library-go
    and k8s packages at the same version in all CSI driver operators without issues, but we did not monitor _all_ the
    packages.
    * Most cloud SDKs were part of github.com/kubernetes/kubernetes at some point, so we know it was possible (with a
      lot of effort, I guess).

## Design Details

### Open Questions [optional]

### Test Plan

We have a solid CI to test that CVO runs CSO and CSO runs corresponding CSI driver operator.
We will add CI jobs for csi-operator repository for all platforms, possibly with careful rules to trigger only the jobs
that a PR affects. With a possibility to run everything manually using `/test all`. 

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

### Merge operators in a single binary + image

We can easily build all CSI driver operators into a single binary in a single image. This would simplify the build,
but it would make it more difficult to switch between the old and new CSI driver operator images during development
and testing.

The resulting image would be quite large, as it would contain all the dependencies of all CSI driver operators.

### Build separate images and switch them using a feature gate

During development of an OCP release, we want ART to build both old and new operator image, as the new one is used only
when a feature gate (name TBD) is enabled. This will give us opportunity to test the new operator image in CI and by
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
  disable builds of either the old or `-v2-` image. Similarly, we will update `cluster-storage-operator` and its feature gate
  to use only one of these images.
  We will file these tickets + PRs ([source](https://docs.ci.openshift.org/docs/how-tos/onboarding-a-new-component/#removing-a-component-from-the-openshift-release-payload)):
  * PR to update `cluster-storage-operator` and its related-images, so only the "good" image is used.
    * This can be merged before anything below.
  * Jira for ART to stop building the "bad" image, https://issues.redhat.com/browse/ART-1443
    * They will create a PR to update ocp-build-data.
  * When removing the old image: PR to openshift/release to stop building + promoting the image in CI.
  * On slack, coordinate with ART merging of all these PRs

* At 4.15 GA (and in the whole 4.15.z stream), ART will build and ship only the "good" image.

* In 4.16, if the "good" image is the old one, we re-submit all tickets to enable building + shipping of the `-v2-` image
  again and continue testing.
  * Drawback: we will have to stop its development + testing until OCP 4.16 is branched, so we can keep 4.15 with only
    a single image until GA.

Drawbacks:

* New images need to be built, i.e. lot of Comet requests.
* More tickets.
* It's harder to track which image is used in which OCP release. For example we may end up with:
  * AWS EBS, Azure Disk, Azure File: -container repo for < 4.15 builds and -v2-container repo for >= 4.15 builds.
  * Say GCP PD, GCP Filestore: -container repo for < 4.16 builds and -v2-container repo for >= 4.16 builds.
  * Say OpenStack Cinder, Manila: -container repo for < 4.17 builds and -v2-container repo for >= 4.17 builds.
  * Etc.

  OCP release numbers are purely illustrative!

Advantages:

* Can be tested easily in CI and by QE.

## Infrastructure Needed [optional]

N/A (other than the usual CI + QE)