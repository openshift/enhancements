---
title: csi-certification
authors:
  - "@fbertina"
reviewers:
  - "@jsafrane"
  - "@gnufied”
  - "@chuffman"
approvers:
  - "@..."
creation-date: 2019-12-18
last-updated: 2019-12-18
status: provisional
see-also:
replaces:
superseded-by:
---

# CSI Certification

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

We want to have a CSI certification suite that our storage vendors can use to certify their CSI drivers.

## Motivation

As storage vendors want to include their solutions into the OpenShift ecosystem, they will need to make their CSI drivers installable via an operator. To get support for their drivers, they need to certify them and we need to provide a way to test that the driver they provide meets the key requirements.

### Goals

The goal is to allow a storage vendor to run a test that validates their CSI drivers for use with OpenShift as a step in the process to certify their operators for the OperatorHub.

### Non-Goals

* Validate CSI driver operators, this proposal targets CSI drivers only.
* Troubleshoot or debug CSI drivers. This is the responsibility of the CSI driver vendor.

## Proposal

We propose that in order to be certified, a CSI driver should meet the following requirement(s):

* The driver should pass all tests contained in the `openshift/csi` test suite that is part of the `openshift-tests` utility.
  * The cluster should not have custom settings aiming the tests to pass, like SELinux disabled.
  * The driver must be installed via the operator that will be available in our marketplace.
* Once all tests pass, the CSI driver vendor should provide our certification team with the output of the tests in order to prove they passed.

### User Stories

#### Story 1

As a storage vendor, I want to make my CSI driver available to OCP users so that they can use my storage backend in their applications.

#### Story 2

As an OCP user, I want to store my application data to a given storage backend using a certified CSI driver.

### Implementation Details/Notes/Constraints

No technical work needs to be done in the `openshift-tests` utility, as it already contains the `openshift/csi` test suite.

### Risks and Mitigations

## Design Details

In order to run all tests of the `openshift/csi` test suite, CSI driver vendors should follow these steps:

* Install the CSI driver through the operator available in our marketplace.
* Prepare the manifest file for the CSI certification tests.
  * This file should describe the supported features, so that the `openshift-tests` utility knows which tests should run.
	* Here is the [upstream documentation](https://github.com/kubernetes/kubernetes/blob/master/test/e2e/storage/external/README.md) that describe its format.
* Find out the location of the `tests` image in the cluster.
  * The command `oc adm release info` can be used to get the `tests` image address. In the following example, `myPullSecret.txt` contains the pull secret used to install the cluster:
	```
	$ oc adm release info --pullspecs -a myPullSecret.txt | grep tests
	tests                                         quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:c89de58c5a2ea4ce9bffabf54f2c74d0805c39d6baa12754990fa3a2f1203856
	```
* Run the tests.
  * Since the utility `openshift-tests` is going to be executed from a container, it's necessary to map some local files into the container:
	* A `kubeconfig.yaml` file with credentials to access the cluster.
	* The CSI driver manifest file created in the second step above.
  * Assuming that both `kubeconfig.yaml` and `manifest.yaml` files are in the current working directory, this is how the tests can be executed:
	```
	$ podman run \
         --authfile=myPullSecret.txt \
	     -v `pwd`:/data:z \
		 --rm -it \
		 quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:c89de58c5a2ea4ce9bffabf54f2c74d0805c39d6baa12754990fa3a2f1203856 \
		 sh -c "KUBECONFIG=/data/kubeconfig TEST_CSI_DRIVER_FILES=/data/manifest.yaml /usr/bin/openshift-tests run openshift/csi"
	```
* After all tests pass, the CSI driver vendor should send the test results to our certification team.

### Test Plan

* We want the `openshift/csi` test suite running in our CI for at least one CSI driver, possibly the EBS driver.
  * This will guarantee that the test suite is in constant use by us, and if something breaks we'll see it right away.
* We would like QA to run this process against one of our CSI drivers, like `ember` or `ceph-csi`.

### Graduation Criteria

##### Tech Preview

* The `openshift/csi` test suite running against a CSI driver in our CI.
* The `openshift-tests` utility is available to storage vendors.
* Documentation on how CSI driver vendors could run the test suite and certify their drivers.

##### Removing a deprecated feature

### Upgrade / Downgrade Strategy

### Version Skew Strategy

## Implementation History

## Drawbacks

## Alternatives

## Infrastructure Needed
