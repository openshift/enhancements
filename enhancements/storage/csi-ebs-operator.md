---
title: csi-ebs-operator
authors:
  - "@fbertina"
reviewers:
  - "@jsafrane”
  - "@hekumar"
  - "@chuffman"
approvers:
  - "@..."
creation-date: 2020-03-09
last-updated: 2020-03-13
status: implementable
see-also:
replaces:
superseded-by:
---

# CSI Operator for AWS EBS

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

In the past storage drivers have been provided either in-tree or via external provisioners. With the move to advocating CSI as the way to deliver storage drivers, Red Hat needs to move the in-tree drivers (and others that our customers need) to use CSI. With the focus on operators in Red Hat OCP 4.x these CSI drivers should be installed in the form of an Operator.

## Motivation

* In-tree drivers will be removed from Kubernetes, we need to continue to make the drivers available and CSI is the way to do this.
* AWS is a key cloud provider for OpenShift and use of EBS has been supported in both OpenShift 3.x and 4.x, we need to move this driver to be CSI, provided by an operator.
* CSI drivers (incl. AWS EBS) will enable new storage features, not possible with in-tree volume plugins, like snapshots and volume cloning.

### Goals

* Create an operator to install the AWS EBS CSI driver.
* Publish the operator on OperatorHub.
* Package and ship downstream image of AWS EBS CSI driver (upstream: https://github.com/kubernetes-sigs/aws-ebs-csi-driver)

### Non-Goals

* Driver creation (upstream: https://github.com/kubernetes-sigs/aws-ebs-csi-driver)

## Proposal

OCP ships a new operator called aws-ebs-csi-driver-operator.

1. The operator installs the AWS EBS CSI driver following the [recommended guidelines (currently work-in-progress)](https://github.com/openshift/enhancements/pull/139/files).
2. The operator deploys the all the objects required by the AWS EBS CSI driver:
   2.1 A namespace called `openshift-aws-ebs-csi-driver`.
   2.2 Two ServiceAccounts: one for the Controller Service and other for the Node Service of the CSI driver.
   2.3 The RBAC rules to be used by the sidecar containers.
   2.4 The CSIDriver object representing the CSI driver.
   2.5 A Deployment that runs the driver's Controller Service.
   2.7 A DaemonSet that runs the driver's Node Service.
   2.8 A non-default StorageClass that uses the CSI driver as a provisioner.
3. The operator deploys all the CSI driver objects in the namespace `openshift-aws-ebs-csi-driver`.
   3.1 This is true regardless the namespace where the operator itself is deployed.
4. The operator itself is installed by OLM once the user opts-in.
   4.1 The operator should be deployed in the namespace `openshift-aws-ebs-csi-driver-operator`.
   4.2 The CR which the operator reacts to is *non-namespaced* and is named `cluster`.
5. The operator leverages the AWS credentials available in the `kube-system/aws-creds` secret.

### API

Given that no additional fields are needed, other than the standard ones, the following API should is provided by the operator:

```go
// EBSCSIDriver is a specification for a EBSCSIDriver resource
type EBSCSIDriver struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   EBSCSIDriverSpec   `json:"spec"`
	Status EBSCSIDriverStatus `json:"status"`
}

// EBSCSIDriverSpec is the spec for a EBSCSIDriver resource
type EBSCSIDriverSpec struct {
	operatorv1.OperatorSpec `json:",inline"`
}

// EBSCSIDriverStatus is the status for a EBSCSIDriver resource
type EBSCSIDriverStatus struct {
	operatorv1.OperatorStatus `json:",inline"`
}

// EBSCSIDriverList is a list of EBSCSIDriver resources
type EBSCSIDriverList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []EBSCSIDriver `json:"items"`
}
```

The CRD and its `cluster` instance is created by OLM from the operator manifest.


### User Stories

#### Story 1

### Implementation Details/Notes/Constraints

### Risks and Mitigations

* The upstream CSI driver might have bugs that are currently unknown.

## Design Details

### csi-snapshot-controller-operator

The manages one Deployment that runs the driver controller and one DaemonSet that run the CSI driver in nodes (plus related service account and RBAC role bindings).

### Test Plan

* Both upstream and downstream CSI driver repositories will have a pre-submit job to run the Kubernetes external storage tests.
* The operator will have unit tests and E2E tests, also running as pre-submit jobs.
* The operator will also have a pre-submit job to run the Kubernetes external storage tests against the CSI driver installed by it.

### Graduation Criteria

There is no dev-preview phase.

#### Tech Preview

#### Tech Preview -> GA

#### Removing a deprecated feature

### Upgrade / Downgrade Strategy

### Version Skew Strategy

## Implementation History

## Drawbacks

## Alternatives

## Infrastructure Needed

* aws-ebs-csi-driver GitHub repository (forked from upstream).
* aws-ebs-csi-driver-operator GitHub repository.
* aws-ebs-csi-driver and aws-ebs-csi-driver-operator images.
