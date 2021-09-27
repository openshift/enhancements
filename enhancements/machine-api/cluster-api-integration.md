---
title: Cluster API Integration
authors:
  - "@JoelSpeed"
  - "@alexander-demichev"
reviewers:
  - "@elmiko"
  - "@Fedosin"
  - "@lobziik"
  - "@asalkeld"
  - "@hardys"
approvers:
  - "@elmiko"
  - "@asalkeld"
creation-date: 2021-09-16
last-updated: 2021-09-16
status: implementable
---

# Cluster API Integration

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

This enhancement describes the process of integrating the upstream [Cluster API](https://github.com/kubernetes-sigs/cluster-api)
project into OpenShift standalone clusters.

## Motivation

We would like to give users the ablility to use Cluster API for machine management, as an addition or supplement for Machine API.

### Goals

- Run Cluster API controllers for managing infrastructure in a similar way to Machine API.
- Provide forward compatibility between Machine API (MAPI) and Cluster API (CAPI).
- Ensure feature parity between MAPI and CAPI before migration.

### Non-Goals

- Deprecate or remove any existing APIs.
- Stop providing support for Machine API in near future.
- Provide any automated integration or migration between MAPI and CAPI resources.

## Proposal

This proposal is about introducing Cluster API alongside Machine API as a technical preview in OpenShift clusters. Cluster API on OpenShift has the potential to unlock new infrastructure providers and community engagement for our users.
During the technical preview we will gather feedback on its usefulness as well as evaluate the feasibility of using Cluster API as a primary infrastructure resource API for OpenShift.

### User Stories

#### Story 1

As an OpenShift developer, I would like to leverage the upstream community Cluster API infrastructure providers and reduce the barrier to OpenShift of supporting new platforms.

#### Story 2

As an OpenShift developer, I would like to collaborate with third parties who already have vested interests in maintaining Machine providers for various platforms and environments and benefit from their provider specific expertise as I try to add new features.

#### Story 3

As a cloud developer, I would like to easily onboard new platforms as this process is well documented by CAPI community and any implementation will be able to be leveraged by both Kubernetes and OpenShift customers, increasing the value of implementing a new provider.

#### Story 4

As a developer, I would like to be able to use the same set of tools for infrastructure management in OpenShift as I can for vanilla Kubernetes.

#### Story 5

As a cloud operator, I would like to be able to use infrastructure resource API for managing mixed platform clusters.

#### Story 6

As a developer, I would like to have support for hub-spoke openshift clusters. Where management cluster can manage workload clusters that are running on different platforms.

#### Story 7

As a user, I would like to create new MachineSets using CAPI and be able to explore feature that are not available in MAPI.

### Implementation Details

First, we need to establish Cluster API resource management by ensuring all required components are successfully installed and running within the OpenShift cluster.

Cluster API will only be present in the cluster (installed by a new operator) if and when a user installs a feature gate.
We will introduce a new, OpenShift specific, feature gate `ClusterAPIEnabled` and include it within the `TechPreviewNoUpgrade` FeatureSet.

Once installed, this preview will allow the user to create new MachineSets using CAPI and explore the
features available within CAPI, for comparison with MAPI. For example, availability set support in Azure that
was already in CAPI, but only being introduced to MAPI as of 4.10.

During this timeframe, any user wanting to migrate or try out the preview will be left to manually migrate the MachineSet
or create a new one, using either the upstream documentation or documentation provided by OpenShift.

This preview will be intended to be used to create day 2 worker MachineSets and is not expected to be integrated into the install process in any way.

It's important to note that the preview will focus on supporting CRDs that our users are familiar with, that includes:
Machines, MachineSets, MachineHealthChecks. We are not planning to document support for other CRDs like MachineDeployment,
however they will be installed.

### Supported platforms

The technical preview aims to support: AWS, Azure, GCP, Baremetal, Openstack.

#### Cluster API resource management

In order to maintain the lifecycle of Cluster API related resources, we will create a new operator `cluster-capi-operator`, this name was chosen for avoiding confusion with upstream Cluster API operator.
This operator will be responsible for all administrative tasks related to the deployment of the Cluster API project within the cluster.
During tech preview phase, the new operator will also manage all Cluster API related CRDs. All CRD manifests will placed in
openshift forks of CAPI and will take from there with no changes.

`cluster-capi-operator` and it's operands will be provisioned in a new `openshift-cluster-api` namespace.

The operator will perform the following tasks:

##### Reconcile FeatureGate object

While Cluster API intergration is in tech preview, the operator will reconcile the cluster [`FeatureGate`](https://docs.openshift.com/container-platform/4.8/nodes/clusters/nodes-cluster-enabling-features.html) object and check for `ClusterAPIEnabled` feature gate presence.
The operator will procceed with Cluster API installation if and only if the required feature gate is present.

##### Deploy Cluster Machine Approver

In order for Cluster API machines to succefully join the cluster, the Kubelet CSRs need to be approved.
The operator will deploy a separate instance of the cluster-machine-approver, which will be configured to be used with Cluster API machines by providing [`--apigroup`](https://github.com/openshift/cluster-machine-approver/blob/master/main.go#L54) flag that was recently introduced.

##### Install upstream CAPI operator

We will use the upstream [Cluster API operator](https://github.com/kubernetes-sigs/cluster-api/blob/master/docs/proposals/20201020-capi-provider-operator.md) for managing CRDs and deploying cloud providers.
`cluster-capi-operator` should first install the [upstream operator CRDs](https://github.com/kubernetes-sigs/cluster-api/blob/master/docs/proposals/20201020-capi-provider-operator.md#new-api-types) and then run the upstream operator itself using a `Deployment`.

##### Deploy core Cluster API

Once the upstream Cluster API Operator is installed, the next step is to create a `CoreProvider` CR along with a configmap that contains upstream Cluster API CRDs, Deployment, Webhooks and RBAC resources.
The example usage is described [here](https://github.com/kubernetes-sigs/cluster-api/blob/master/docs/proposals/20201020-capi-provider-operator.md#air-gapped-environment).

##### Deploy Cluster API infrastructure provider

The `cluster-capi-operator` operator will create an appropriate `InfrastructureProvider` CR (based on the cluster platform) and a configmap that contains upstream Cluster API cloud provider CRDs, Deployment, Webhooks and RBAC resources.

##### Reconcile Cluster object

Cluster API's main entity is a `Cluster`, it represents the cluster which is managed by Cluster API and the cluster's infrastructure. More details about cluster object are [here](https://cluster-api.sigs.k8s.io/user/concepts.html).
The `cluster-capi-operator` will need to create the `Cluster` and a proper `InfrastructureCluster` resource for the OpenShift cluster.
Because we have our own infrastructure management strategy in OpenShift, we should leverage the [externally managed cluster infrastructure](https://github.com/kubernetes-sigs/cluster-api/blob/master/docs/proposals/20210203-externally-managed-cluster-infrastructure.md) feature.
This means that the created `InfrastructureCluster` should have `cluster.x-k8s.io/managed-by:` annotation set.

Cluster object reconciler can be done as a separate controller. The controller should:
- Wait before Cluster and InfrastructureCluster CRD is present
- Create both Cluster and InfrastructureCluster objects with externally managed cluster infrastructure annotation.
- Ensure spec/status of InfrastructureCluster are configured for the OpenShift cluster (infrastructure information can be sourced from resources within the OpenShift Cluster).
- Patch `Cluster` status to `Ready=true`.

##### Create user data secret

Cluster API Machines will need a user data secret, similar to the one that Machine API uses.
This secret is created by installer for Machine API.
While Cluster API components are in technical preview, and therefore not integrated into the OpenShift Installer, the operator can copy the worker user data secret from `openshift-machine-api` namespace to `openshift-cluster-api`.

At this point all Cluster API components should be installed and ready to use.

#### CVO management

A new `cluster-capi-operator` image will be built and included in every release payload.

#### Credentials management

The `cluster-capi-operator`'s manifests should contain an appropriate `CredentialsRequest` for each supported cloud provider.
This is similiar to [machine-api-operator](https://github.com/openshift/machine-api-operator/blob/6f629682b791a6f4992b78218bfc6e41a32abbe9/install/0000_30_machine-api-operator_00_credentials-request.yaml)

#### Cluster API cloud providers

Cluster API cloud providers will live in forks, similar to what is now done for Machine API. We now evaluating moving
current providers implementation to new repos that will be called `machine-api-provider-*` and reseting current
`cluster-api-provider-*` to latest upstream.

#### Example usage

Usage is similar to Machine API with small differences, MachineSets reference the `Cluster` object and infrastructure machine template.

```yaml
---
apiVersion: cluster.x-k8s.io/v1alpha4
kind: MachineSet
metadata:
  name: capi-ms
  namespace: openshift-cluster-api
spec:
  clusterName: cluster-name
  replicas: 1
  selector:
    matchLabels: 
      test: example
  template:
    metadata:
      labels:
        test: example
    spec:
      bootstrap:
         dataSecretName: worker-user-data
      clusterName: cluster-name
      infrastructureRef:
        apiVersion: infrastructure.cluster.x-k8s.io/v1alpha4
        kind: AWSMachineTemplate
        name: cluster-name

---
apiVersion: infrastructure.cluster.x-k8s.io/v1alpha4
kind: AWSMachineTemplate
metadata:
  name: capi-machine-template
  namespace: openshift-cluster-api
spec:
  template:
    spec:
      uncompressedUserData: true
      iamInstanceProfile: ....
      instanceType: m5.large
      cloudInit:
        secureSecretsBackend: secrets-manager
        insecureSkipSecretsManager: true
      ami:
        id: ....
      subnet:
        filters:
        - name: tag:Name
          values:
          - ...
      additionalSecurityGroups:
      - filters:
        - name: tag:Name
          values:
          - ...
```

### Risks and Mitigations

- During tech preview `cluster-capi-operator` will have permissions to manage CRDs, this might be a not secure permission for an operator.
- Note, this permission should be restricted to creating CRDs only, as once installed, the technical preview cannot be uninstalled.
- CLI usage, once Cluster API is installed command like `oc get machine` will return Cluster API machines, in order to use Machine API users will have to use fully qualified name `oc get machines.machine.openshift.io`.
If we want to not introduce this breaking change then we have to set prefered API group in our [kubernetes fork](https://github.com/openshift/kubernetes/blob/master/pkg/controlplane/controller/crdregistration/patch.go).
- Feature parity, for last year we've been trying to upstream all features introduced to Machine API but we can't be sure all of them work in upstream. We need to have a good set of regression tests running periodically.

### API Extensions

With `ClusterAPIEnabled` feature enabled, the following API extensions will be added:

- Core Cluster API resources and webhooks, they can be found [here](https://github.com/kubernetes-sigs/cluster-api/tree/main/api/v1beta1)
- Depending on a platform where cluster is running, infrastructure provider CRD and webhooks will be added, see
[AWS](https://github.com/kubernetes-sigs/cluster-api-provider-aws/tree/main/api/v1beta1), [Azure](https://github.com/kubernetes-sigs/cluster-api-provider-azure/tree/main/api/v1beta1), [GCP](https://github.com/kubernetes-sigs/cluster-api-provider-gcp/tree/main/api/v1alpha4).
- Cluster API Operator CRDs will be added, see [here](https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20201020-capi-provider-operator.md#new-api-types)

## Design Details

### Test Plan

- Cluster API providers should already come with a set of e2e tests, we will run these on each PR.
- `cluster-capi-operator` will include it's own e2e suite ensuring that all Cluter API components are successfully installed.

### Operational Aspects of API Extensions
#### Failure Modes

If Cluster API starts failing, it will affect worker machine management, which is a critical
component of the OCP system. In case of Cluster API failures, users will be able to use the Machine API.

#### Support Procedures

The process of troubleshooting failure is similar to the process of troubleshooting Machine API failures.
We will be working on making sure that similar or equivalent events, metrics and alerts are present.

### Graduation Criteria

#### Dev Preview -> Tech Preview

- Write symptoms-based alerts for the component(s)
- Ability to have Cluster API installed using a feature gate
- Ability to use Cluster API for machine management
- End user documentation
- Running upstream e2e workflow on openshift

#### Tech Preview -> GA (Future Work)

- Cluster API will be installed in all OpenShift clusters by default.
- Bidirectional migration for MAPI and CAPI.
- New cloud providers implemented as CAPI.

#### Removing a deprecated feature

### Upgrade / Downgrade Strategy

### Version Skew Strategy

## Implementation History

## Drawbacks

## Alternatives

## Infrastructure Needed
