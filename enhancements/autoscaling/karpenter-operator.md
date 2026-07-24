---
title: karpenter-operator
authors:
  - "@maxcao13"
reviewers:
  - "@elmiko" ## reviewer for autoscaling component
  - "@enxebre" ## reviewer for architecture
  - "@jkyros" ## reviewer for autoscaling component
  - "@joshbranham" ## reviewer for ROSA
  - "@muraee" ## reviewer for hypershift
approvers:
  - "@csrwng" ## approver for hypershift
api-approvers:
  - "@JoelSpeed" ## approver for api
creation-date: 2026-05-07
last-updated: 2026-07-07
status: provisional
tracking-link:
  - https://redhat.atlassian.net/browse/OCPSTRAT-3109
see-also:
  - "/enhancements/machine-api/cluster-autoscaler-operator.md"
  - "Karpenter CAPI standalone enhancement (TBD)"

---

# Karpenter Operator

## Summary

This enhancement proposes `karpenter-operator`, a Cluster Version
Operator (CVO)-managed component in the OpenShift release payload
that deploys and manages [Karpenter](https://karpenter.sh/) across
standalone OpenShift and Hosted Control Planes (HCP). On HCP,
Karpenter is already shipped as AutoNode, Red Hat's managed node
autoprovisioning offering. This enhancement covers refactoring
the existing HCP implementation into the new operator binary, and
establishing the generic operator framework for all OpenShift platforms.

## Motivation

[Karpenter](https://karpenter.sh/) is an open-source Kubernetes
node autoscaler hosted under the Cloud Native Computing
Foundation (CNCF). It watches for pods that the Kubernetes
scheduler cannot place, evaluates their scheduling constraints
against the full set of available instance types, and launches
best-fit instances directly without an intermediate node group
abstraction. Three custom resources (CRs) define the system: a
**NodePool** declares scheduling constraints, instance
requirements, and limits; a **NodeClass** (provider-specific,
e.g. `EC2NodeClass` on AWS) configures cloud settings like
AMI selection, subnets, and security groups; and a
**NodeClaim** represents a single node request created by
Karpenter at runtime. Karpenter also handles node lifecycle
through disruption: consolidating underutilized nodes,
replacing drifted nodes, and terminating expired ones. For a
full overview, see the upstream
[Karpenter Concepts](https://karpenter.sh/docs/concepts/)
documentation.

Karpenter adoption is growing across the Kubernetes ecosystem,
both in managed offerings (AWS
[EKS Auto Mode](https://docs.aws.amazon.com/eks/latest/userguide/automode.html),
Azure
[AKS Node Autoprovision](https://learn.microsoft.com/en-us/azure/aks/node-autoprovision))
and in self-managed clusters where teams deploy Karpenter
directly. OpenShift should support this capability natively
as a platform component, and customers migrating from other
Kubernetes distributions expect it to be available.

OpenShift today provides
[Cluster Autoscaler (CAS)](/enhancements/machine-api/cluster-autoscaler-operator.md)
paired with Machine API for automatic node scaling. That model
gives administrators explicit control: each MachineSet defines
a specific instance type and zone, and CAS scales those
MachineSets in response to pending pods. Karpenter serves a
different use case where administrators prefer to declare
high-level intent and let the autoscaler choose instances
dynamically. The two models are complementary; this
enhancement adds Karpenter as an opt-in alternative, not a
replacement for CAS.

HCP already ships Karpenter on AWS (AutoNode), but the
deployment/operation is embedded in the HyperShift repository.
Karpenter Go dependencies are coupled to HyperShift's go.mod,
and upstream rebases require cross-repo coordination and
dependency conflict resolution. This enhancement also covers
extracting that logic into a standalone karpenter-operator
binary so the team can develop and iterate on Karpenter
independently of HyperShift's codebase, enabling code reuse
across standalone and HCP topologies.

### User Stories

- As a platform engineer on another Kubernetes offering that
  uses Karpenter, I want to migrate to OpenShift without
  rewriting my autoscaling configuration.

- As a site reliability engineer (SRE) running both Cluster
  Autoscaler and Karpenter, I
  want OpenShift to let me run both so I can migrate workloads
  from one to the other without downtime.

- As a cluster admin running self-managed OpenShift, I want
  Karpenter available as a platform component so I can use its
  scheduling and instance selection capabilities without
  deploying and maintaining it outside of the platform.

- As a cluster admin, I want Karpenter installed, upgraded, and
  monitored as part of the platform so I do not have to manage
  its lifecycle separately.

- As a cluster admin on a managed HCP cluster, I want the operator to provide a
  default NodeClass that mirrors my cluster's infrastructure
  settings so nodes can be allowed to auto-provision "out of the box"
  without manual cloud configuration.

- As an SRE managing HCP clusters, I expect the
  karpenter-operator migration to be transparent. Existing
  AutoNode behavior should not change.

### Goals

- Establish `karpenter-operator` as an OpenShift payload
  component that works across standalone (CVO-managed) and
  HCP (HO-managed) topologies, gated by a
  `KarpenterOperator` feature gate in each topology's
  respective feature gate registry.

- Refactor the [existing karpenter-operator logic][hcp-karpenter-operator] out of the
  HyperShift repository into
  [openshift/karpenter-operator](https://github.com/openshift/karpenter-operator).

- Implement the AWS cloud provider in the operator (required
  for HCP integration).

- Define the `Karpenter` custom resource
  (`autoscaling.openshift.io`) lifecycle object, auto-created
  on HCP from `HostedCluster` spec and user-created on standalone,
  following the same pattern as the
  [`ClusterAutoscaler`](/enhancements/machine-api/cluster-autoscaler-operator.md)
  custom resource.

- Define the build, test, and release process for delivering
  karpenter-operator to both OCP and Managed Services (i.e. ROSA/ARO HCP).

### Non-Goals

- Replacing Cluster Autoscaler.

- Standalone provider-specific design. This enhancement covers
  the generic operator framework (CR lifecycle, operand
  deployment, CRD management, ClusterOperator reporting).
  Provider-specific standalone details (NodeClass fields,
  credentials, bootstrap pipeline) are out of scope.
  Karpenter Cluster API (CAPI) is the planned standalone
  provider and will be covered in a separate enhancement
  (TBD), including CAPI integration and APIs.

- Implementation details of individual Karpenter cloud provider
  controllers beyond what is needed for HCP integration (AWS, Azure).

- Guaranteeing safe simultaneous autoscaling by Karpenter and
  Cluster Autoscaler over the same nodes. Coexistence is
  supported for migration, but administrators must ensure they
  do not target the same nodes.

## Proposal

### Overview

Karpenter Operator on startup will auto-discover the cluster's cloud infrastructure,
and deploy/configure the corresponding Karpenter operand depending on the platform.
Additionally, the operator will reconcile CRDs and maintain the lifecycle of the
Karpenter CR and related resources. On HCP, it also creates a default NodeClass
and runs a custom machine approver for CSR approval.
The same operator binary will run on both standalone and HCP.
On standalone, CVO will deploy the operator
into the `openshift-karpenter` namespace. On HCP, the HyperShift Operator (HO) will
deploy it into the hosted control plane namespace on the management cluster. For more
information on the autoscaling capabilities of Karpenter itself,
refer to [Karpenter Concepts](https://karpenter.sh/docs/concepts/).

#### Karpenter CR lifecycle

The `Karpenter` CR is namespace-scoped (to support HCP where
multiple hosted clusters share a management cluster). On
standalone, the operator will only watch its own namespace and
the cluster administrator will create the CR directly in order to create
a Karpenter operand. On HCP, the CR is
auto-created on the management cluster in the hosted control
plane namespace. The user configures Karpenter settings in
[`HostedCluster.spec`](https://github.com/openshift/hypershift/blob/main/api/hypershift/v1beta1/hostedcluster_types.go)
(e.g. [`spec.autoNode`](https://github.com/openshift/hypershift/blob/main/api/hypershift/v1beta1/hostedcluster_types.go)).
The HyperShift operator will automatically create the `Karpenter` CR based on the existence of the `autoNode` field and
and reconcile it. HCP will scale down Karpenter
by removing the Karpenter CR if the `autoNode` field is removed.

#### Provider strategy

On HCP, the operator will deploy the cloud-native [AWS][karpenter-aws] provider
for ROSA HCP and self-managed HCP on AWS, and the [Azure][karpenter-azure] provider
for ARO HCP and self-managed HCP on Azure. They are mature upstream projects with vendor backing (AWS maintains the AWS provider,
Microsoft maintains the Azure provider).
Karpenter [CAPI][karpenter-capi] is the planned provider for standalone OpenShift, covered in a separate enhancement.
GCP on both standalone and HCP is expected to be served by the CAPI provider.

Regardless of provider or platform/topology, the operator deploys the Karpenter
operand (Deployment, RBAC) and the provider-specific CRDs. On HCP it additionally manages a default NodeClass and
a CSR approver for node identity. These responsibilities are
detailed in [Topology Considerations](#topology-considerations).

#### NodeClass customization

On HCP, the operator creates and fully manages a default
NodeClass (e.g. `OpenshiftEC2NodeClass` on ROSA HCP) pre-configured
with the cluster's infrastructure settings. Users can create
additional NodeClass resources for specialized workloads
(e.g., nodes in a different subnet or with different security
group rules). Users can mutate non-protected fields on the
default NodeClass. Protected fields (`amiFamily`,
`amiSelectorTerms`, `userData`) are enforced by the operator
via [ValidatingAdmissionPolicies](#validatingadmissionpolicies)
and continuous reconciliation.

On standalone, the operator does not create or manage a default
NodeClass. Users will be expected to create their own sets of `ClusterAPINodeClass`.

### Workflow Description

**cluster administrator** is a human user responsible for
managing the OpenShift workload cluster.

#### Standalone workflow

1. The `KarpenterOperator` feature gate must be enabled on
   the cluster (initially part of the `DevPreviewNoUpgrade`
   feature set).

2. The CVO will apply the operator manifests from the payload:
   namespace, CRDs, RBAC, operator Deployment, and
   `ClusterOperator` CR.

3. The operator will start, read the `Infrastructure` CR to detect topology, and
   wait for a `Karpenter` CR.

4. The cluster administrator will create a `Karpenter` CR:

   ```yaml
   apiVersion: autoscaling.openshift.io/v1alpha1
   kind: Karpenter
   metadata:
     name: cluster
     namespace: openshift-karpenter
   ```

5. The operator will reconcile and only look for a Karpenter CR in its own namespace, and with the name `default`. It will then deploy the operand and related objects.

6. The cluster administrator will create and configure a `ClusterAPINodeClass` and `NodePool`.

7. Karpenter will observe unschedulable pods and scale up a
   matching MachineDeployment. The Cluster API infrastructure
   provider provisions the node through its normal flow.

#### HCP workflow

1. The HO deploys karpenter-operator into the hosted control
   plane namespace on the management cluster.

2. The HyperShift operator auto-creates a `Karpenter` CR from
   [`HostedCluster.spec`](https://github.com/openshift/hypershift/blob/main/api/hypershift/v1beta1/hostedcluster_types.go)
   configuration (i.e. [`spec.autoNode`](https://github.com/openshift/hypershift/blob/main/api/hypershift/v1beta1/hostedcluster_types.go)).

3. The operator reconciles the CR, deploys the Karpenter
   operand on the management cluster (with a guest-cluster kubeconfig),
   manages CRDs and NodeClass on the guest cluster, and runs a special
   Karpenter-specific machine approver controller on the management cluster.

4. A KarpenterIgnition controller generates
   userData secrets. The operator's NodeClass controller reads
   those secrets and writes userData into the NodeClass.

5. Customers create `NodePool` resources in the guest cluster
   and later creates unschedulable workloads for Karpenter.
   Karpenter provisions nodes through the cloud provider API
   and the machine approver verifies identity before approving CSRs.

On HCP and OCP, if the `Karpenter` CR is deleted, a
ValidatingAdmissionPolicy will reject the deletion while any
`NodeClaim` resources still exist, forcing the user to remove
all NodePools and wait for Karpenter nodes to drain first. Once all
NodeClaims are gone, the CR deletion proceeds and the operator
can tear down the operand.

### API Extensions

**Deployed by CVO (standalone) / HO (HCP):**

- **`Karpenter`** (`autoscaling.openshift.io/v1alpha1`):
  namespace-scoped lifecycle trigger. Creating it deploys the
  operand; deleting it tears down after draining nodes. On HCP
  it is auto-created from
  [`HostedCluster.spec`](https://github.com/openshift/hypershift/blob/main/api/hypershift/v1beta1/hostedcluster_types.go);
  on standalone the administrator creates it directly.

  **spec:**
  - `provider`: provider-specific configuration using a
    discriminated union.
    The `type` field acts as the discriminator.

    ```yaml
    spec:
      provider:
        type: AWS
        aws:
          logLevel: info  # info | debug | error
    ```

    ```yaml
    spec:
      provider:
        type: Azure
        azure:
          logLevel: info  # info | debug | warn | error
    ```

    ```yaml
    spec:
      provider:
        type: ClusterAPI
        clusterAPI:
          logLevel: 1  # integer 0-9
    ```

    Each provider supports different log verbosity values
    because the underlying operand binaries accept different
    formats. AWS and Azure use Karpenter core's `--log-level`
    flag (named levels). CAPI uses klog-style numeric
    verbosity.

    The operator installs a ValidatingAdmissionPolicy that
    constrains which `spec.provider.type` and fields can be set based on the
    detected topology and infrastructure platform:

    - HCP + AWS: only `spec.provider.type` can be `AWS` and `spec.provider.aws` is allowed
    - HCP + Azure: only `spec.provider.type` can be `Azure` and `spec.provider.azure` is allowed
    - Standalone: only `spec.provider.type` can be `ClusterAPI` and `spec.provider.clusterAPI` is allowed

    Setting a provider field that does not match the cluster's
    topology/platform is rejected at admission time. The
    rejection message states that the specified provider is not
    supported on this topology and platform, and indicates which
    provider is supported.
  - `deployment.requests` / `deployment.limits`: resource
    requests and limits for the Karpenter operand Deployment.

  **status:**
  - `conditions`. Standard operator conditions: `Available`,
    `Progressing`, `Degraded`. On standalone these mirror the
    `ClusterOperator` conditions. On HCP there is no
    `ClusterOperator`, so the `Karpenter` CR conditions are the
    primary operator health signal.
  - `enabledFeatureGates`. List of upstream Karpenter feature
    gates currently enabled on the operand.

  Future fields: additional provider-specific fields in the
  provider union as needed.

**Deployed programmatically by the operator at runtime:**

- **`NodePool`** (`karpenter.sh/v1`): upstream CRD defining
  scheduling constraints, instance requirements, and limits.
- **`NodeClaim`** (`karpenter.sh/v1`): upstream CRD
  representing a Karpenter node lifecycle. These custom resources are created by Karpenter.
- **Provider-specific NodeClass CRDs**:

  | Topology | User-facing CRD | Underlying CRD |
  | -------- | --------------- | -------------- |
  | AWS HCP | `OpenshiftEC2NodeClass` (`karpenter.hypershift.openshift.io/v1`) | `EC2NodeClass` (`karpenter.k8s.aws/v1`) |
  | Azure HCP | `AzureNodeClass` (`karpenter.azure.com/v1alpha2`) | n/a (no wrapper, VAPs enforce protected fields) |
  | Standalone | `ClusterAPINodeClass` (`karpenter.cluster.x-k8s.io/v1alpha1`) | n/a (no wrapper) |

The operator deploys these CRDs at runtime.

On HCP, some NodeClass fields are operator-managed (e.g.
`amiFamily`, `userData`) and protected from user modification
via [ValidatingAdmissionPolicies](#validatingadmissionpolicies)
and the [`OpenshiftEC2NodeClass`](#openshiftec2nodeclass)
facade API (AWS HCP). On standalone, users have full control
over `ClusterAPINodeClass` resources with no operator-managed
fields or restrictions. See
[Topology Considerations](#topology-considerations) for
details. Write permissions for each actor are listed in
[RBAC and write contract](#rbac-and-write-contract).

This enhancement does not modify existing OpenShift resources.
Karpenter-provisioned nodes are standard Kubernetes nodes with
Karpenter-specific labels and annotations.

### Topology Considerations

#### Hypershift / Hosted Control Planes

In HCP, the control plane (API server, etcd, controllers)
runs in a management cluster, while customer workloads run in
a separate guest cluster. Operators like karpenter-operator
run on the management cluster and interact with the guest
cluster via a kubeconfig.

HyperShift already deploys Karpenter on AWS HCP clusters
(ROSA HCP and self-managed HCP on AWS) today under
AutoNode. This section describes the current state,
the refactoring, and the target architecture.

##### Today vs target

- **Today:** The HO deploys a karpenter Deployment from the
  `aws-karpenter-provider-aws` payload image and a
  karpenter-operator Deployment from the HyperShift image
  (different binary entrypoint). There is no separate operator
  binary. HyperShift-specific controllers (KarpenterIgnition,
  `OpenshiftEC2NodeClass` reconciler) are embedded in
  HyperShift.
- **Target:** The HO will deploy karpenter-operator from an image
  referenced in the image overrides file (see
  [Build, Release, and Delivery to HCP](#build-release-and-delivery-to-hcp)).
  Everything currently in HyperShift's
  [`karpenter-operator` directory][hcp-karpenter-operator]
  will move to the karpenter-operator repository.
  The operator will contain both common and HCP-specific code
  paths, toggled by topology detection from the
  `Infrastructure` CR.

##### Current state

In HCP the HyperShift Operator (HO) directly manages all
Deployments in the hosted control plane namespace. For
Karpenter this means the HO deploys two Deployments:

- A **karpenter Deployment** using the
  `aws-karpenter-provider-aws` payload image. This is the
  Karpenter operand. It targets the guest cluster via a
  kubeconfig mounted by the HO.
- A **karpenter-operator Deployment** using the Hypershift
  image itself, running a different binary entrypoint. This
  controller handles EC2NodeClass reconciliation, Ignition
  configuration, and machine approval, but it does not deploy
  the Karpenter operand; the HO handles that directly.

Karpenter Go dependencies are coupled to the HyperShift
go.mod. Rebasing the OpenShift fork of karpenter-provider-aws
requires coordinating with the HyperShift release cycle.

Both Deployments run on the management cluster. Customers
never see or manage these processes.

![Current HCP Karpenter architecture](karpenter-hcp-current.png)

##### HCP Karpenter Operator refactoring

All HCP karpenter-operator controllers and logic will be moved to
[openshift/karpenter-operator](https://github.com/openshift/karpenter-operator).
Topology-specific controllers are disabled when running on
the other topologies. The operator will ship as its own payload
image, deployed by the HO via CPOv2 controlPlaneComponents. HyperShift
will carry zero upstream Karpenter Go dependencies after the
refactor and be able to ship independently of any Karpenter changes made
by the team maintaining Karpenter for OpenShift. `OpenshiftEC2NodeClass`
API types will move to the `karpenter-operator/api` sub-module.

![Target HCP Karpenter architecture](karpenter-hcp-target.png)

##### Migration period

The refactoring cannot happen atomically. Existing HCP
clusters will continue running the embedded code path until
they upgrade to a version with `KarpenterOperator`
enabled by default. During this transition period, both code paths must
be maintained, but features and bug fixes will no longer be merged into the
old karpenter-operator code path in HyperShift unless absolutely necessary.

- The embedded Karpenter logic in HyperShift remains the
  production path for clusters that have not yet upgraded.
  Critical fixes will be cherry-picked into the embedded
  code in hypershift/karpenter-operator when necessary to support those clusters.
- New clusters created after the feature gate is enabled will
  use the standalone karpenter-operator image from the start.
- Existing clusters will switch to karpenter-operator when
  they upgrade to a release where the gate is enabled. The
  HO handles this transition by deploying karpenter-operator
  and scaling down the embedded controllers.

The old embedded code will be removed from HyperShift when the
refactor reaches GA. Since HO does not backport and ships from
main, there is no version-dependent migration window. Once
the feature gate graduates, the embedded path is dead code and
will be removed completely. New development will land in the
karpenter-operator repository.

##### Topology detection

The same binary will run on both topologies. The operator will
detect the topology at startup (e.g. from the `Infrastructure` CR
or by a flag/env var) and enable the appropriate controller
set:

- **Standalone-only:** ClusterOperator status reporting.
- **HCP-only:** `OpenshiftEC2NodeClass` reconciliation, HCP
  lifecycle management, machine approver.
- **Both:** `Karpenter` lifecycle CRD (auto-created on HCP,
  user-created on standalone), operand deployment and RBAC,
  CRD management.

##### HCP deployment model (target)

After the refactor:

- `v2/karpenter/` (operand controlPlaneComponent) will be
  removed. karpenter-operator will manage the operand Deployment
  in both topologies.
- `v2/karpenteroperator/` (the HO's controlPlaneComponent for
  karpenter-operator) will deploy the `karpenter-operator`
  image instead of the HyperShift image.
- On HCP, the karpenter-operator image will not be sourced from the
  hosted cluster's OCP payload. It will be pinned as a digest in
  hypershift-operator source and delivered through the
  HCP/managed-services release stream (see
  [Build, Release, and Delivery to HCP](#build-release-and-delivery-to-hcp)).
  The OCP payload carries the image for standalone only or as a fallback for HCP.
- Karpenter configuration on HCP will flow through the
  [`HostedCluster` API](https://github.com/openshift/hypershift/blob/main/api/hypershift/v1beta1/hostedcluster_types.go)
  (e.g. [`spec.autoNode`](https://github.com/openshift/hypershift/blob/main/api/hypershift/v1beta1/hostedcluster_types.go)),
  which feeds into creating a `Karpenter` CR that the operator
  reconciles.
- On HCP, the operator will mount a guest-cluster kubeconfig and
  pass it to the Karpenter operand Deployment so the operand
  targets the guest cluster. This plumbing will be disabled on
  standalone where there is no management/guest distinction.

The karpenter-operator controlPlaneComponent will use
`.MonitorOperandsRolloutStatus()` to track the karpenter
operand Deployment. The operand Deployment will be labeled
with `hypershift.openshift.io/managed-by: karpenter-operator`
and annotated with `release.openshift.io/version`. The
karpenter-operator component will not report rollout-complete
until its operand is ready. No separate controlPlaneComponent
is needed for the operand.

The HCP refactor resolves tech debt and is not a new user-facing
feature. It is gated by a `KarpenterOperator`
feature gate registered in the HyperShift Operator's
[internal feature gate framework](https://github.com/openshift/hypershift/blob/main/hypershift-operator/featuregate/feature.go),
controlled by the `HYPERSHIFT_FEATURESET` environment
variable on the HO deployment. This is independent of the
`openshift/api` `KarpenterOperator` gate used on standalone
so the HO gate can be toggled without depending on the
management cluster's OCP version. On HCP, this gate
controls the rollout of the standalone karpenter-operator
binary on existing AWS HCP clusters.

##### OpenshiftEC2NodeClass

`OpenshiftEC2NodeClass`
(`karpenter.hypershift.openshift.io/v1`) is an OpenShift-owned
API modeled after the upstream `EC2NodeClass` but adapted to
fit OpenShift's API conventions. It emulates most upstream
`EC2NodeClass` fields while removing fields that users should
not touch in an OpenShift context (`amiFamily`, `userData`,
`amiSelectorTerms`) and adding OpenShift-specific fields
(e.g. `version` for release-pinned node configuration). The
karpenter-operator in HCP reconciles from
`OpenshiftEC2NodeClass` to the upstream `EC2NodeClass`,
filling in the removed fields (AMI, userData) from
platform-managed sources automatically. This wrapper exists because HCP customers interact with NodeClasses directly in the guest cluster and need a surface that is both simplified and compatible for RHCOS-based nodes.

> **Note:** The Autoscale team is exploring deprecating
> `OpenshiftEC2NodeClass` in favour of using the upstream
> `EC2NodeClass` directly with
> [ValidatingAdmissionPolicies](#validatingadmissionpolicies)
> controlling the same behaviour. A foreign API that diverges
> from upstream is confusing for users and requires ongoing
> toil (syncing new fields, reconciling drift, carrying an
> additional CRD lifecycle). The `version` field would move to
> another API surface. This is tracked in
> [AUTOSCALE-526](https://redhat.atlassian.net/browse/AUTOSCALE-526).
> For the same reasons, we are not planning to introduce an
> `OpenshiftAzureNodeClass` wrapper API for Azure HCP. The Azure provider
> will use the upstream `AzureNodeClass` directly with VAPs
> from the start (see
> [ValidatingAdmissionPolicies](#validatingadmissionpolicies)).

On standalone, the operator will deploy the ClusterAPINodeClass CRD.
Users will create and manage their own ClusterAPINodeClass resources, and the operator
will not interfere with any changes because provisioning will be handled by ClusterAPI.

#### Standalone Clusters

Standalone self-managed OpenShift is supported behind the
`KarpenterOperator` feature gate. See
[Provider strategy](#provider-strategy) for the standalone
provider path.

The operator detects the cloud provider from the
`Infrastructure` CR and deploys the corresponding operand
image. It manages the `Karpenter` CR lifecycle, deploys
provider-specific CRDs, and maintains a `ClusterOperator` CR
with standard conditions, version reporting, and related-object
references for `oc adm must-gather`. The `ClusterOperator` is
standalone-only; on HCP, status is reported through the hosted
control plane infrastructure.

#### Single-node Deployments or MicroShift

Not applicable.

#### OpenShift Kubernetes Engine

Same deployment model as standalone OpenShift. No differences
in karpenter-operator behavior.

### Implementation Details/Notes/Constraints

#### Payload Images

The following images are used by the operator:

- **`karpenter-operator`**: the operator binary, built from
  [openshift/karpenter-operator](https://github.com/openshift/karpenter-operator).
- **`aws-karpenter-provider-aws`**: the Karpenter operand for
  AWS, built from
  [openshift/aws-karpenter-provider-aws](https://github.com/openshift/aws-karpenter-provider-aws)
  (OpenShift's fork of upstream
  [aws/karpenter-provider-aws](https://github.com/aws/karpenter-provider-aws)).
  Used on AWS HCP (ROSA HCP and self-managed HCP on AWS).
- **`azure-karpenter-provider-azure`**: the Karpenter operand
  for Azure, built from
  [openshift/azure-karpenter-provider-azure](https://github.com/openshift/azure-karpenter-provider-azure)
  (OpenShift's fork of upstream
  [Azure/karpenter-provider-azure](https://github.com/Azure/karpenter-provider-azure)).
  Used on Azure HCP (ARO HCP). This image is not yet in the payload.

The operator selects the correct image at runtime based on the `Infrastructure` CR and topology.

`aws-karpenter-provider-aws` is already part of the OpenShift
release payload today. HyperShift uses it as the Karpenter
operand deployed into the hosted control plane namespace for
AutoNode on AWS HCP clusters. `azure-karpenter-provider-azure`
is not yet in the payload.

The standalone CAPI provider image will be covered in the
CAPI enhancement.

#### Delivery Model and Provider Interface

Internally the operator uses a provider interface.
Cloud-specific logic lives in per-provider packages. Adding a
provider means implementing the interface and supplying
provider-specific CRDs. At startup the operator reads the
`Infrastructure` CR's `status.platformStatus.type` to
determine the cloud provider and selects the corresponding
operand image (e.g. `aws-karpenter-provider-aws` for AWS)
and enables/disables topology specfific controllers.

On HCP, cloud credentials are provided through the HO's
credential management. On standalone with CAPI, the operand
does not call cloud APIs directly (CAPI infrastructure
providers handle that), so no cloud credentials are needed
for the Karpenter operator or operand.

#### Upstream Karpenter feature gates

Upstream Karpenter has its own set of
[feature gates](https://karpenter.sh/docs/reference/settings/#feature-gates)
independent of OpenShift feature gates. The operator controls
which upstream gates are enabled on the operand Deployment.
All providers share the same Karpenter core feature gate set
since they all import the same core library.

OpenShift follows the upstream Karpenter feature gate lifecycle
but applies its own graduation criteria:

- **Upstream GA and default-on features** are considered GA in
  OpenShift from the start.
- **Upstream beta features** can be considered for GA in
  OpenShift. The upstream project tends to keep features in
  beta for a long time, and beta features are relatively
  stable (e.g. ReservedCapacity is default-on but still beta
  upstream). The Autoscale team will go through QA and documentation
  processes for beta features before GA-ing them in OpenShift.
- **Upstream alpha features** can be considered for TechPreview
  in OpenShift but cannot be GA until the upstream project
  graduates them to at least beta. Alpha APIs can change
  upstream at any time, making them unsuitable for a GA
  supported product.

At GA of karpenter-operator for OCP, the following upstream
features will be considered GA (on by default and does not require
an openshift/api feature gate):

- **Drift**
- **ReservedCapacity**

All other upstream feature gates will be registered in
`openshift/api`, started at DevPreview/TechPreview, and off by default.
They will graduate through the standard OpenShift feature gate
lifecycle as validation is completed.

Users control which Karpenter feature gates are active through
the OpenShift feature gate mechanism, not through fields on the
`Karpenter` CR or direct operand configuration. On standalone,
this is the cluster-scoped `FeatureGate` CR. On HCP, this is
`HostedCluster.spec.configuration.featureGate` (same
`configv1.FeatureGateSpec`). The operator reads the enabled
gates and translates them to the appropriate operand arguments.

OpenShift official documentation will list which upstream Karpenter
feature gates are available, enabled by default, and at what
OpenShift graduation level.

#### RBAC and write contract

| Actor | Resources | Verbs / fields |
| ----- | --------- | -------------- |
| karpenter-operator | `Infrastructure` | Read |
| karpenter-operator | `Karpenter` status, default NodeClass (HCP), operand `Deployment`, VAPs (HCP), `ClusterOperator` (standalone only) | Write / reconcile |
| Karpenter operand | `NodePool` status, `NodeClaim` | Read / write |
| Karpenter operand | `[...]NodeClass` | Read |
| Karpenter operand (HCP) | Cloud API (e.g. EC2) | Per upstream [AWS IAM reference][karpenter-aws-iam] |
| Machine approver (HCP) | CSR | Read / approve / deny |
| Machine approver (HCP) | Cloud API (e.g. `ec2:DescribeInstances`) | Read |

On HCP, the operator and operand use a guest-cluster kubeconfig
for guest-cluster resources. RBAC for HCP KO is reused from
existing manifests in `v2/assets/karpenter-operator/`. The guest-cluster kubeconfig follows
the standard HO-minted kubeconfig secret pattern. HCP-specific
code paths in karpenter-operator are disabled on standalone.

#### RHCOS Bootstrap and Ignition userData

OpenShift nodes use RHCOS and require Ignition-based bootstrap.

**Standalone:**

On standalone, Karpenter CAPI scales MachineDeployment replicas
rather than launching instances directly, so RHCOS bootstrapping
is handled by the Cluster API infrastructure provider and the
OpenShift platform. Neither the operator nor the operand
has any role in userData generation or image selection.

**HCP:**

On HCP, Karpenter launches instances directly via the cloud
provider, which passes userData from the NodeClass to the
instance at boot. For RHCOS nodes, the userData is a small Ignition "pointer
config" that tells the node where to fetch its full
configuration. There is no MCO or MCS on the guest cluster.
The KarpenterIgnition controller creates userData secrets via
the existing HyperShift NodePool ignition pipeline. The
resulting pointer config authenticates to the HyperShift
ignition-server with a bearer token to bootstrap nodes with
the correct RHCOS configuration. Node customization on HCP
goes through `spec.kubelet` on `OpenshiftEC2NodeClass`.
KarpenterIgnition reads the kubelet settings, generates a
KubeletConfig manifest stored in a ConfigMap on the
management cluster, and incorporates it into the rendered
Ignition. The `spec.kubelet` field supports both explicitly
typed fields (with CEL validation) and an overflow mechanism
for arbitrary kubelet settings that pass through to the node
(see [PR #8192](https://github.com/openshift/hypershift/pull/8192)).

Karpenter compares the configuration of
running nodes (AMI, userData, etc.) against the current
NodeClass spec. When they diverge, Karpenter marks affected
nodes as "Drifted" and replaces them. On HCP, the
ignition-server rotates the bearer token approximately every
5.5 hours, which changes the userData JSON. A naive hash of
the full userData would trigger false drift on every rotation.
To avoid this, the operator configures a
`TargetConfigVersionHash` header in the Ignition merge source
URL. When a node fetches its config, this value is sent as an
HTTP request header to the ignition-server. The
ignition-server derives the value from
`ConfigGenerator.Hash()`, which covers MachineConfig inputs,
release version, and cluster configuration, but not the
rotating bearer token. The OpenShift fork of
karpenter-provider-aws carries a patch that uses this header
value as the drift hash instead of hashing the full userData.
Token rotation does not trigger drift, but a `NodeClass` upgrade
or `NodePool/NodeClass` field change does.

#### ValidatingAdmissionPolicies

The operator reconciles the following categories of VAPs:

**1. NodeClass field protection (HCP only)**

The upstream provider NodeClass (`EC2NodeClass`,
`AzureNodeClass`) lives in the guest cluster because
Karpenter runs against the guest API server. Customers have
full API access to the guest cluster, so VAPs are needed to
prevent tampering with operator-managed fields.

On AWS HCP, the `OpenshiftEC2NodeClass` wrapper exists as the
user-facing API and the operator reconciles from it to the
underlying `EC2NodeClass`. VAPs on `EC2NodeClass` prevent
customers from bypassing the wrapper and modifying the
underlying resource directly.

On Azure HCP, there is no wrapper and instead we provide `AzureNodeClass` as the
user-facing API directly. VAPs protect specific
operator-managed fields (image reference, custom data) while
leaving other fields (subnets, instance types, tags) open for
user customization.

**2. Karpenter CR deletion guard (all topologies)**

A VAP rejects deletion of the `Karpenter` CR while any
`NodeClaim` resources still exist, forcing the user to drain
nodes first (described in [Workflow](#workflow-description)).

**3. Karpenter CR provider constraint (all topologies)**

A VAP constrains `spec.provider.type` to match the detected
topology and platform, as described in
[API Extensions](#api-extensions).

**4. Default NodeClass deletion protection (HCP only)**

A VAP prevents customers from deleting/modifying the operator-managed
default NodeClass.

---

Not all VAPs apply to all topologies. On standalone,
`ClusterAPINodeClass` is fully user-managed with no field
protection. Only the Karpenter CR deletion guard and provider
constraint VAPs apply.

Note that VAPs which exist on the guest cluster can be subject to user 
deletion/modification. This can techincally result in users deleting
the VAPs and modifying the protected resources in an unsafe manner.
However, the operator mitigates this race condition by continually reconciling
the VAPS and the protected resources upon a deletion/update event.

To fully prevent this any tampering, we are exploring Kubernetes 1.36
[manifest-based admission control](https://kubernetes.io/blog/2026/05/04/kubernetes-v1-36-manifest-based-admission-control/)
to bake policies into the API server configuration on the
management cluster. This would make VAPs a hard security
boundary on HCP. Customers cannot delete or modify them
because the policies are enforced by the API server itself,
outside customer access.

#### Build, Release, and Delivery to HCP

Today, Karpenter Go dependencies are embedded in the
HyperShift repository (see [Current state](#current-state)),
and upstream Karpenter AWS + Karpenter core rebases frequently
conflict with HyperShift's large dependency tree. After the refactoring (see
[above](#hcp-karpenter-operator-refactoring)), Karpenter
dependencies move to karpenter-operator's own go.mod which will be much
easier to manage. A hypershift-operator release is still required to ship to HCP
but for any karpenter specific functionality, the only HyperShift-side change
will be a single image digest bump, ignoring any changes that may need to happen
to HostedControlPlane status fields, or other HCP specific logic we can't port.

##### Development and release workflow

After the refactor, development happens in
[openshift/karpenter-operator](https://github.com/openshift/karpenter-operator).
Builds go through ART's Konflux pipeline with two release
streams from the same source repo:

1. **OCP stream** - standard OCP release cadence (standalone).
2. **HCP stream** - tied to main, shipped at the Autoscale
   team's own cadence (ROSA/ARO).

Development on main targets HCP; changes also flow into the
OCP stream. OCP backports do not affect the HCP stream.

##### HCP release process

The ART pipeline auto-builds every commit on main to a
staging registry. To cut a release the team raises an ART
JIRA ticket (5 working days lead time), identifies and tests
a staged build, and ART promotes it to production.

The karpenter-operator image is pinned as a digest in an
overrides file in hypershift-operator source. On promotion,
renovate opens a PR bumping the digest. AutoNode presubmits
validate the build; the Autoscale team manually approves the
merge and notifies Managed Services of the new version.

##### Image stream overlap risk

Both streams publish to the same Red Hat Registry image
stream. The most recently published image is not necessarily
the most recent HCP build. Digest pinning mitigates this: the
HO always deploys the exact tested build. The risk is that an
automated PR could propose a "stale" OCP stream digest instead
of the intended HCP build since the bot would only create a PR
based on a new "latest" build in that image stream.
This can potentially be fragile.

This is an evolving workflow and will require feedback over time.

##### Impact on Managed Services

The refactor will be transparent to Managed Services
(ROSA/ARO). Managed Services will continue consuming
hypershift-operator releases as today. Each HyperShift
operator version will pin the karpenter-operator version it
was tested against. Upgrading the HO will continue to redeploy
karpenter-operator across all hosted clusters on that
management cluster.

Maintaining two release streams means changes to the karpenter-operator image flow differently depending on which topology they affect. The following scenarios illustrate common situations.

##### HCP-only change (e.g. OpenshiftEC2NodeClass API field update)

The fix goes to main branch only. The team cuts an
HCP stream release via ART, and renovate bumps the digest in
hypershift-operator.

The change will still appear in the OCP
payload changelog since both streams build from the same repo,
but it is transparent to standalone clusters and will not be
included in OCP release notes or documentation.

##### Shared bug fix

The fix lands on main first. If applicable, the team
backports it to the relevant OCP release branch(es). HCP
consumes the fix from main automatically on the next release.
Backports to previous OCP release branches (4.x, 5.x, etc.)
are invisible to HCP and managed services. Each stream ships
through its own mechanism (OCP z-stream release for standalone,
hypershift-operator digest bump for HCP).

##### Standalone-only fix
The fix goes to the relevant OCP release branch and can potentially
be backported.

If needed in the next OCP release, bug-fixes will be merged to main.
However, any standalone-specific changes are transparent to HCP and no HCP Karpenter Operator release is needed.
Regression periodic e2es targetting HCP platform will still be run to confirm no unintended side effects from the merge to main.

##### QE and testing

Autoscale QE tests bugs and features pre-merge on standalone
and self-managed HCP, using regression e2es and automated
suites. Bug cards assigned to the Autoscale team targeting the
`autoscaling / karpenter` component should and
will be closed after standalone and self-managed
HCP QE validation and signoff, irrespective of ROSA/ARO.

ROSA/ARO validation is handled separately by Managed
Services QE in their own staging environments, post-merge.
Managed Services should create their own tracking cards linked
to the original OCPBUGS card for their post-merge testing.

##### Communication

The dual-stream model requires tighter coordination with
Managed Services writers and stakeholders. New features and
bug fixes will be communicated through the team's forum
channel and the `wg-rosa-hcp-karpenter` + `wg-aro-hcp-karpenter` Slack channels. Each
karpenter-operator release will include a summary of changes for Managed
Services consumers, and the next hypershift-operator release they will be available in.

### Risks and Mitigations

On HCP, Karpenter's cloud-native providers launch instances
directly via the cloud API, bypassing Cluster/Machine API.
The operator controls which node image and bootstrap
configuration is used by making AMI/image selection and
userData operator-managed fields, protected via
[ValidatingAdmissionPolicies](#validatingadmissionpolicies)
and facade APIs such as `OpenshiftEC2NodeClass`.
Customers cannot substitute an unsupported image or inject
arbitrary bootstrap scripts.

On HCP, the operator's machine approver auto-approves CSRs,
which is a security-sensitive operation. A bug or bypass in the
identity check could allow a rogue node to join the cluster.
The approver mitigates this by requiring a matching cloud
instance (e.g. via `ec2:DescribeInstances` on AWS) and a
corresponding `NodeClaim` before approving any CSR. CSRs with
no `NodeClaim` are ignored.

On HCP, the operand requires broad cloud permissions (e.g.
EC2 and IAM on AWS). Credentials are provisioned by HyperShift
at cluster install time using STS web identity. Both the
operand and the operator's machine approver share the same
credentials.

On standalone with CAPI, the operand does not call cloud APIs
directly because CAPI infrastructure providers handle that.
Neither the operator nor the operand needs cloud credentials.

Running Karpenter and Cluster Autoscaler concurrently is
supported for migration but carries operational risk. Both
autoscalers respond to pending pods independently. If a
`NodePool` and a MachineSet-backed node group cover the same
schedulable capacity, both may scale up for the same workload.
Downscaling is the harder problem. Both autoscalers will try
to remove nodes they consider underutilized, and without
disjoint scopes they will conflict. Administrators need to
partition workloads and nodes between the two (labels, taints,
topology constraints) so each autoscaler only operates on its
own set. Running both is possible but adds operational
complexity. We will document that running both Karpenter and Cluster Autoscaler for non-migration purposes is unsupported for OpenShift.

### Drawbacks

Maintaining cloud-native provider forks (AWS, Azure, and
potentially others) adds ongoing rebase and release overhead
per cloud, separate from the CAPI path planned for standalone.

The HCP refactor touches a shipped
AutoNode product. A regression during migration would affect
production ROSA HCP and self-managed HCP clusters on AWS.

On HCP, RHCOS bootstrap requires Ignition userData pipelines,
drift-detection patches, and topology-specific controllers
(KarpenterIgnition). This diverges from upstream Karpenter's
simpler cloud-init model and adds complexity the operator must
carry for HCP.

On AWS HCP, `OpenshiftEC2NodeClass` is a facade API over
upstream `EC2NodeClass`. It simplifies the user surface but
requires schema sync as upstream evolves and may confuse users
who expect standard Karpenter documentation.

## Alternatives (Not Implemented)

### Do nothing

Without this feature OpenShift lacks support for mixed-instance
autoprovisioning. Managed Kubernetes services from AWS and
Microsoft already offer this via Karpenter (EKS Auto Mode, AKS
Node Auto Provisioning).

### Enhance Cluster Autoscaler and MachineSets

We explored replicating Karpenter's multi-instance-type
scheduling in Cluster Autoscaler. The problem is that CAS
requires one MachineSet per instance type per zone. A cluster
with broad instance flexibility needs 100+ MachineSets, and at
that scale the autoscaler and CAPI controllers degrade.
Karpenter avoids this with a single NodePool that maps to a
fleet API call (e.g. EC2 CreateFleet) spanning many instance
types and zones at once.

### Operator Lifecycle Manager (OLM)-managed layered operator

Users see Karpenter as a peer to Cluster Autoscaler, which is
CVO-managed, and expect parity in lifecycle management. CVO
provides tighter integration with upgrades, rollbacks, and
ClusterOperator status reporting.

## Open Questions [optional]

1. What is the cleanup procedure when the `Karpenter` CR is
   deleted on a cluster with active Karpenter nodes?

2. What is the node upgrade model for Karpenter-provisioned
   nodes? Covered in topology-specific enhancements (HCP
   Karpenter, standalone CAPI Karpenter (TBD)).

3. ~~What is the gating mechanism for the HCP refactor
   transition?~~ Resolved: A `KarpenterOperator` feature gate
   in each topology's registry: `openshift/api` for standalone
   (`DevPreviewNoUpgrade`), HO's internal feature gate framework
   for HCP (`TechPreviewNoUpgrade`, controlled by
   `HYPERSHIFT_FEATURESET`).

4. ~~How is the bidirectional API dependency between
   `karpenter-operator/api` and `hypershift/api` managed?~~
   Resolved: The `Karpenter` CR exists on HCP.
   `karpenter-operator` imports `hypershift/api` types (for
   `HostedControlPlane`, `AutoNodeStatus`, etc.). The
   `OpenshiftEC2NodeClass` and `Karpenter` API types live in a
   `karpenter-operator/api` Go sub-module so that HyperShift can
   import them without pulling in all of karpenter-operator's
   dependencies. There is no circular dependency because
   HyperShift only imports the lightweight `api` sub-module, not
   the full operator.

5. ~~Should upstream Karpenter CRDs (`NodePool`, `NodeClaim`,
   provider-specific NodeClass) be deployed by the CVO from
   payload manifests, or remain operator-managed at runtime?~~
   Resolved: the operator manages CRDs at runtime. It applies
   the correct CRD version before starting the operand on both
   upgrade and downgrade, providing the same ordering CVO would.
   Upstream Karpenter CRDs are at `v1` with no planned API
   version bumps, so storage version migration is not a near-term
   concern.

6. ~~Should upstream CRD schemas be modified to remove fields that
   are not applicable on OpenShift (e.g., restricting `amiFamily`
   enum to only `Custom`), or should the upstream schemas be kept
   as-is with VAPs enforcing constraints?~~ Resolved: Keep
   upstream CRD schemas as-is. On HCP, use
   ValidatingAdmissionPolicies to enforce constraints on fields
   that users must not change. This avoids confusion from schema
   drift and keeps the upstream CRDs canonical.

## Test Plan

Testing follows a three-tier strategy:

1. **HCP e2e tests**: `TestKarpenter` and
   `TestKarpenterUpgradeControlPlane` in the HyperShift test
   suite serve as regression coverage for the
   karpenter-operator refactoring on AWS HCP clusters.

2. **Standalone OCP e2e tests**: presubmits in the
   karpenter-operator repository cover provisioning,
   scale-down/consolidation, drift, disruption budgets,
   ClusterOperator status, and upgrade rollout.

3. **autoscale-tests common suite**: runs upstream Karpenter
   core library tests shared across provider implementations.

Pre-merge (karpenter-operator repo): `e2e-aws-hypershift`
presubmit runs `TestKarpenter` and
`TestKarpenterUpgradeControlPlane` against the PR's image
and latest HyperShift operator. Pre-merge (hypershift repo):
renovate digest-bump PR runs standard HyperShift operator
presubmits including Karpenter e2e. Post-merge periodics:
daily AutoNode jobs run the full Karpenter test suite on
current OCP and n-1.

## Graduation Criteria

Both topologies are gated by a `KarpenterOperator` feature
gate, but the gate lives in different registries:

- **Standalone:** Defined in `openshift/api` and controlled
  via the cluster's "cluster" `FeatureGate` CR. The CVO deploys the
  operator when the gate is enabled.
- **HCP:** Defined in the HyperShift Operator's
  [internal feature gate framework](https://github.com/openshift/hypershift/blob/main/hypershift-operator/featuregate/feature.go)
  and controlled via the `HYPERSHIFT_FEATURESET` environment
  variable. This makes the HCP gate independent of the
  management cluster's OCP version. HO itself will deploy different versions of KO
  depending on the feature gate state.

### Dev Preview -> Tech Preview

**Standalone (OCP):**

- karpenter-operator payload component deployed on standalone
  via the `KarpenterOperator` feature gate.
- Karpenter CAPI provider image added to the payload and deployable by the operator.
- `Karpenter` CR lifecycle working (user-created).
- Karpenter CAPI enhancement submitted for review.
- ClusterOperator conditions reliable.
- Operand deployment and CRD management functional
  (ClusterAPINodeClass CRD deployed, CAPI operand running).
- Sufficient e2e coverage validating CAPI-based provisioning.
- End user documentation published.
- Feedback gathered from users and field teams.

**HCP (refactor rollout, feature-gated):**

- Refactoring complete: operator logic
  extracted from HyperShift, image deployed by HO via CPOv2,
  zero Karpenter Go deps in HyperShift.
- `KarpenterOperator` HO feature gate enabled,
  karpenter-operator binary deployed
  instead of embedded HyperShift image controllers.
- AWS cloud-native provider functional on AWS HCP clusters
  (ROSA HCP and self-managed HCP on AWS).
- `Karpenter` CR lifecycle working (auto-created from
  `HostedCluster` spec).
- No regression in existing AutoNode behavior.
- E2e coverage validates no behavioral drift between the
  embedded code path and the standalone karpenter-operator
  during migration.
- Sufficient e2e coverage (HCP test tier).

Azure HCP (ARO HCP) Karpenter provider integration will be
worked through karpenter-operator once the AWS HCP refactoring
is complete. The operator refactor is a prerequisite. Azure
graduation criteria will be defined in a follow-up enhancement.

### Tech Preview -> GA

- Documentation published explaining how Karpenter
  and Cluster Autoscaler interact when both are enabled,
  including guidance on partitioning node groups to avoid
  conflicting scale decisions.
- Sufficient feedback across multiple releases.
- `KarpenterOperator` feature gates graduate to GA in both
  registries (operator is present on all standalone clusters,
  idle until a `Karpenter` CR is created; HO deploys
  karpenter-operator unconditionally on AWS HCP clusters).
- Load testing (large NodePool counts, high churn).
- User-facing documentation in
  [openshift-docs](https://github.com/openshift/openshift-docs/).

### Removing a deprecated feature

Not applicable.

## Upgrade / Downgrade Strategy

### Operator and Operand Upgrade

During a cluster upgrade the CVO updates the operator manifests
as part of the normal payload rollout. The operator performs a
rolling update of the Karpenter Deployment. No administrator
action is required.

In Dev Preview the standalone `KarpenterOperator` feature gate
is part of the `DevPreviewNoUpgrade` feature set, which
prevents upgrades and downgrades. Downgrade is not applicable
at this stage. On HCP, the gate follows the same progression
in the HO's feature gate framework (`TechPreviewNoUpgrade`
initially).

## Version Skew Strategy

The operator and operand are part of the same payload and
upgraded together by the CVO. Karpenter nodes may run a
previous kubelet during upgrades, covered by standard
Kubernetes version skew guarantees (control plane N,
kubelet N-2).

## Operational Aspects of API Extensions

`NodePool` and `NodeClaim` instances are expected to stay in
the low hundreds per cluster; NodeClass resources under 10;
one `Karpenter` CR per namespace. None of these CRDs use
webhooks.

**Failure modes:**

- **Unreachable cloud API (HCP):** The machine approver
  cannot verify node identity. CSRs from
  Karpenter-provisioned nodes remain pending until the cloud
  API is reachable.
- **VAPs deleted (HCP):** The operator recreates VAPs on the
  next reconcile and logs a warning. Nodes provisioned while
  VAPs are absent may use incorrect AMI or userData.
- **NodeClaim stuck (no node appears):** The cloud instance
  may have failed to launch (capacity, quota, permissions) or
  the instance launched but never registered with the API
  server (network, bootstrap failure). Karpenter's garbage
  collection will eventually terminate the NodeClaim. The
  operator surfaces this via NodeClaim conditions.
- **Node not becoming Ready:** The instance booted and
  registered but kubelet reports NotReady (CSR not approved,
  misconfigured bootstrap, missing CNI). On HCP, a pending
  CSR indicates the machine approver has not yet validated
  the node. Standard node troubleshooting applies (kubelet
  logs, node conditions).
- **NodeClaim stuck on deletion:** A deleted NodeClaim that
  won't go away typically means its finalizer is not being
  removed. Karpenter removes the finalizer after the cloud
  instance is terminated. If the cloud API call to terminate
  fails (permissions, API outage) or Karpenter itself is
  down, the NodeClaim will remain with a deletion timestamp
  indefinitely. The cloud instance may still be running and
  billing. As a last resort, manually removing the
  finalizer will delete the NodeClaim but orphan the
  instance, which may requires manual cleanup outside of OpenShift.

## Support Procedures

**Version compatibility:**

- OCP documentation will list which upstream Karpenter version
  is packaged with each OCP release. HCP/ROSA/ARO
  documentation will do the same for each managed services
  version. Some upstream Karpenter features may not exist in
  the version shipped with a given release.
- Upstream Karpenter feature gates available and enabled by
  default will also be documented per release (see
  [Upstream Karpenter feature gates](#upstream-karpenter-feature-gates)).
  Admins can check which gates are active via
  `status.enabledFeatureGates` on the `Karpenter` CR, or by
  inspecting the args on the Karpenter operand Deployment.

**Detecting failure:**

- On standalone, `oc get clusteroperator karpenter` shows
  `Degraded=True` or `Available=False` when the operator
  cannot reconcile. The `Karpenter` CR
  (`oc get karpenter -n openshift-karpenter -o yaml`) has
  more granular conditions.
- On HCP, there is no `ClusterOperator`. Check the
  `Karpenter` CR in the hosted control plane namespace on the
  management cluster
  (`oc get karpenter -n <hcp-namespace> -o yaml`).
- On HCP, nodes stuck in `NotReady` with pending CSRs indicate
  the machine approver cannot verify node identity. Check
  karpenter-operator logs for approver errors and verify
  cloud API connectivity.
- On HCP, a missing or incomplete NodeClass (no subnets, no
  AMI, no userData) usually points to the operator failing an
  intermediate reconciliation step. Check karpenter-operator
  logs for errors.
- Operator and operand logs:
  - Standalone:
    `oc logs -n openshift-karpenter -l app=karpenter-operator`
    and `oc logs -n openshift-karpenter -l app=karpenter`
  - HCP (management cluster):
    `oc logs -n <hcp-namespace> -l app=karpenter-operator`
    and `oc logs -n <hcp-namespace> -l app=karpenter`
- Karpenter resources to inspect:
  `oc get nodepools,nodeclaims,ec2nodeclasses` (HCP guest
  cluster) or `oc get nodepools,nodeclaims,clusterapinodeclasses`
  (standalone).
- On HCP, AutoNode issues may originate in the HyperShift
  operator. Check HyperShift operator logs for
  `HostedCluster` reconciliation errors related to
  [`spec.autoNode`](https://github.com/openshift/hypershift/blob/main/api/hypershift/v1beta1/hostedcluster_types.go).

**Disabling the feature:**

- Delete all `NodePool` resources and wait for Karpenter to
  drain and terminate the associated nodes. Then delete the
  `Karpenter` CR. A ValidatingAdmissionPolicy blocks CR
  deletion while `NodeClaim` resources still exist.
- On HCP, disable AutoNode through the `HostedCluster` spec.
  The HyperShift operator will remove the `Karpenter` CR subject
  to `NodeClaim` resources still existing in the guest cluster.
  After a graceful timeout period, the Karpenter Operator will begin
  to forcefully terminate `NodeClaim` resources to unblock the deletion of the `Karpenter` CR.

**Consequences of disabling:**

- Existing Karpenter-provisioned nodes are drained and
  terminated. Workloads running on those nodes are
  rescheduled onto other nodes (MachineSet-backed or
  otherwise).
- The operator and operand Deployments remain but are idle
  until a new `Karpenter` CR is created.

**Recovery:**

- Create a new `Karpenter` CR (standalone) or re-enable
  AutoNode on the `HostedCluster` (HCP). The operator
  reconciles and redeploys the operand. No manual
  intervention beyond CR creation is needed.

## Infrastructure Needed [optional]

- The source repository already exists at
  [openshift/karpenter-operator](https://github.com/openshift/karpenter-operator).
- ART team adds the `karpenter-operator` image to the OCP release payload.
- ART team maintains two release streams for `karpenter-operator`:
  the regular OCP stream and an HCP/Managed Services stream
  tied to main (see
  [Build, Release, and Delivery to HCP](#build-release-and-delivery-to-hcp)).

[karpenter-aws]: https://github.com/aws/karpenter-provider-aws
[karpenter-azure]: https://github.com/Azure/karpenter-provider-azure
[karpenter-capi]: https://github.com/kubernetes-sigs/karpenter-provider-cluster-api
[karpenter-aws-iam]: https://karpenter.sh/docs/reference/cloudformation/
[hcp-karpenter-operator]: https://github.com/openshift/hypershift/tree/main/karpenter-operator
