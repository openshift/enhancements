---
title: IBM Public Cloud Support
authors:
  - "@csrwng"
reviewers:
  - "@derekwaynecarr"
  - "@sttts"
  - "@deads2k"
  - "@mfojtik"
  - "@ironcladlou"
  - "@crawford"
  - "@abhinavdahiya"
  - "@spadgett"
  - "@miabbott"
approvers:
  - "@derekwaynecarr"
creation-date: 2020-02-04
last-updated: 2020-08-05
status: implemented
see-also:
  - "/enhancements/update/cluster-profiles.md"
replaces:
superseded-by:
---

# IBM Public Cloud Support

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [x] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [x] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

IBM Public Cloud deploys Kubernetes/OpenShift clusters by hosting their control plane
as pods in a central management cluster. OpenShift v3.11 is currently offered as a managed
service using this model. This type of setup allows the hosting of many
cluster control planes on a single Kubernetes cluster. Only the worker nodes are exclusive
to an individual user cluster and may exist in a different account than the central
management cluster.

Supporting this type of deployment in OpenShift 4.x requires a different approach to
installing OpenShift and managing its control plane. Instead of bootstrapping a cluster 
control plane from scratch on virtual machines, a cluster control plane can be created 
by applying a set of manifests to an existing cluster in a new namespace. 

Changes are required in different areas of the product in order to make clusters deployed 
using this method viable. These include changes to the cluster version operator (CVO), web 
console, second level operators (SLOs) deployed by the CVO, and RHCOS.

## Motivation

Enable OpenShift 4.x deployment on IBM Cloud.

### Goals

- Enable the deployment of OpenShift 4.x on IBM Public Cloud by providing the IBM
  Public Cloud team the necessary tools to generate manifests needed for a hosted
  control plane.
- Ensure that this deployment model remains functional through regular e2e testing
  on IBM Public Cloud.
- Make the necessary product changes to make IBM Public Cloud a fully supported 
  cloud provider.
- Ensure that clusters running on IBM Public Cloud are conformant OpenShift clusters
  (pass Kubernetes and OpenShift conformance e2e tests). Fully document any tests that
  must be skipped due to differences from a traditional deployment.

### Non-Goals

- Make hosted control planes a supported deployment model outside of IBM Public Cloud.
- Define the automation needed on the IBM Public Cloud side to provision new clusters.
- Create an alternate installer for OpenShift.

## Proposal

Support for IBM Public Cloud will be added in 3 phases:

1. Beta: Initial rollout. Enables users to create OpenShift clusters
   using RHEL 7 worker nodes. Nodes are labeled as both worker and master to support
   SLOs that have masters in their node selectors. IBM provides out of tree cloud support
   for storage, service load balancers. Ingress, image registry, storage and auth are 
   configured after initial provisioning (day 2). Calico is used for the SDN.
2. GA: In-tree support for ingress, image registry, storage, auth, load balancers.
3. Post-GA: Add support for RHCOS workers

In more detail:

### Beta

Enables an OpenShift cluster to be hosted on top of a Kubernetes/OpenShift cluster. 

Given a release image, a CLI tool generates manifests that instantiate the control plane
components so that they can be applied to a namespace. IBM provides the automation
that applies these manifests and instantiates worker nodes that add themselves to 
the new cluster. Minting of kubelet certificates for these worker nodes is handled
by IBM automation.

Components that run on the management cluster include:
- etcd\+
- kubernetes apiserver
- kubernetes controller manager
- kubernetes scheduler
- openshift apiserver
- openshift controller manager
- cluster version operator
- control plane operator(s)\*
- oauth server\+
- vpn server\+

\* - new component

\+ - provisioned by IBM

Once a control plane is running, the Kubernetes API server connects to a VPN server to
which worker nodes also connect. This enables the API server to access the pod and service
network of the worker nodes. This is needed for aggregated API services running on the
worker nodes.

The cluster version operator running on the management cluster connects to the Kubernetes
API server via the service network and starts applying manifests for SLOs that must run
on the worker nodes. These include (among others):

- network (dns, multus)
- ingress
- image registry
- OLM
- samples
- console

The CVO needs to skip manifests that instantiate control plane operators, machine API related
operators, and others that do not apply to this topology. These include:

- cluster-version-operator
- cluster-kube-apiserver-operator
- cluster-kube-controller-manager-operator
- cluster-kube-scheduler-operator
- cluster-openshift-apiserver-operator
- cluster-openshift-controller-manager-operator
- cluster-autoscaler-operator
- cluster-machine-approver
- insights-operator
- machine-config-operator
- cloud-credential-operator
- cluster-authentication-operator

Components/Changes needed for this to work:

#### OpenShift ROKS Toolkit CLI

CLI tool that takes a release image and a configuration file as input and produces a set
of manifests that provision a control plane that runs on an existing Kubernetes cluster.
PKI certificates are not generated by the tool. Manifests of control plane components 
expect certificates to have been generated before provisioning and to exist as secrets
and configmaps on the control plane namespace following a predefined contract.

This tool is invoked by IBM Public Cloud provisioning code to when instantiating a new cluster.

#### Control Plane Operator
An operator that:
- Projects global configuration into the target cluster. Watches
  and overwrites any changes to global configuration initiated inside the user
  cluster.
- Ensures that the Kubernetes Controller Manager CA bundle includes self-signed CAs
  generated by the router and the service CA operator.

#### Cluster Version Operator Changes

> :warning: NOTE: Use of the exclude annotation is deprecated in favor of 
  [Cluster Profiles](https://github.com/openshift/enhancements/blob/master/enhancements/update/cluster-profiles.md)

In phase I the cluster version operator excludes operator manifests that
have an annotation in the form:

`exclude.release.openshift.io/[identifier]=true`

whenever a global variable in the form

`EXCLUDE_MANIFESTS=[identifier]`

is present. For IBM public cloud, the value of \[identifier\] is 
`internal-openshift-hosted`

#### Excluded Manifests

> :warning: NOTE: Use of the exclude annotation is deprecated in favor of 
  [Cluster Profiles](https://github.com/openshift/enhancements/blob/master/enhancements/update/cluster-profiles.md)

Manifests for the components that should be skipped by the CVO whenever
the `EXCLUDE_MANIFESTS=internal-openshift-hosted` environment variable is 
present, should include the annotation to exclude them. In most cases, only the
minimum set of manifests to allow skipping the component should be annotated.
However, in the case of the Machine API and Machine Configuration operators, 
the CRDs that represent machines, machinesets and autoscalers should also be
skipped. Monitoring alerts for components that do not get installed in the user
cluster should also be skipped where possible.

#### Cluster Profiles Support

Manifests that should be rendered in an IBM Cloud managed cluster must include 
the following annotation:
```
include.release.openshift.io/ibm-cloud-managed=true
```
Manifests that should be excluded such as control plane operators will not 
include that annotation.

#### Console Changes
The console should not report the control plane as being down if no metrics
datapoints exist for control plane components in this configuration.

### GA

For GA, there is more native support for IBM Public Cloud as a supported
cloud provider in the product. Operators for control plane
components (based on the control plane self-hosted operators) are deployed on 
the management cluster.

Components and changes required for this phase:

#### Control Plane Operators
Break up the single Beta control plane operator into different operators that
vendor code from the self-hosted control plane operators in the product. The vendored
code should include config observers that assemble a new configuration for their
respective control plane components. This will ensure that drift in future versions
is kept under control and that a single code base is used to manage control plane
configuration.

#### Cluster Version Operator Changes
The CVO should support the concept of an install profile that allows selecting manifests
based on which profile is in effect. This should make it possible to modify the node
selector of SLOs that are installed on IBM Public Cloud clusters to target worker nodes.
(See separate proposal for installation profiles).

#### Ingress Operator
Add native support for IBM Public Cloud as a cloud provider.

#### Storage Operator
Create storage classes needed for IBM Public Cloud.

#### Image Registry Operator
Support IBM Public Cloud natively by provisioning the appropriate storage (RWO PVC)

#### OAuth Server
Support IBM public cloud as a provider.

### Post-GA

Add support for managed RHCOS nodes.

#### Managed Workers
RHCOS adds support for bootstrapping on IBM Public Cloud. The MCO is added to 
the components that get installed on the management cluster.  This enables upgrading 
of RHCOS nodes using the same mechanisms as in self-hosted OpenShift.

## Design Details

### Test Plan


### Graduation Criteria


### Upgrade / Downgrade Strategy


### Version Skew Strategy

## Implementation History

* Teach cluster-version operator about the new annotation, [cvo#252](https://github.com/openshift/cluster-version-operator/pull/252), merged 2019-11-13.
* Add the exclude annotation to the cluster-version operator manifests, [cvo#269](https://github.com/openshift/cluster-version-operator/pull/269), merged 2019-11-14.
* Add the exclude annotation to the machine-API operator manifests, [mao#437](https://github.com/openshift/machine-api-operator/pull/437), merged 2019-11-15.
* Many more pull requests adding exclusion annotations.
* Feature went live in 4.3 with 51 excluded manifests in 4.3.0.
