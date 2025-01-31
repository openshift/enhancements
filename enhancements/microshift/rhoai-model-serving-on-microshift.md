---
title: rhoai-model-serving-on-microshift
authors:
  - pmtk
reviewers: # Include a comment about what domain expertise a reviewer is expected to bring and what area of the enhancement you expect them to focus on. For example: - "@networkguru, for networking aspects, please look at IP bootstrapping aspect"
  - DanielFroehlich, MicroShift PM
  - jerpeter1, MicroShift Staff Eng, Architect
  - lburgazzoli, RHOAI Expert
approvers: # A single approver is preferred, the role of the approver is to raise important questions, help ensure the enhancement receives reviews from all applicable areas/SMEs, and determine when consensus is achieved such that the EP can move forward to implementation.  Having multiple approvers makes it difficult to determine who is responsible for the actual approval.
  - jerpeter1
api-approvers:
  - None
creation-date: 2025-01-17
last-updated: 2025-01-31
tracking-link:
  - https://issues.redhat.com/browse/OCPSTRAT-1721
# see-also:
#   - "/enhancements/this-other-neat-thing.md"
# replaces:
#   - "/enhancements/that-less-than-great-idea.md"
# superseded-by:
#   - "/enhancements/our-past-effort.md"
---

# RHOAI Model Serving on MicroShift

## Summary

Following enhancement describes process of enabling AI model
serving on MicroShift based on Red Hat OpenShift AI (RHOAI).

## Motivation

Enabling users to use MicroShift for AI model serving means they will be able to
train model in the cloud or datacenter on OpenShift, and serve models at the
edge using MicroShift.

### User Stories

* As a MicroShift user, I want to serve AI models on the edge in a lightweight manner.

### Goals

- Prepare RHOAI-based kserve manifests that fit MicroShift's use cases and environments.
- Provide RHOAI supported ServingRuntimes CRs so that users can use them.
  - [List of supported model-serving runtimes](https://docs.redhat.com/en/documentation/red_hat_openshift_ai_self-managed/2.16/html/serving_models/serving-large-models_serving-large-models#supported-model-serving-runtimes_serving-large-models)
    (not all might be suitable for MicroShift - e.g. intended for multi model serving)
- Document how to use kserve on MicroShift.
  - Including reference to ["Tested and verified model-serving runtimes" that are not supported by Red Hat](https://docs.redhat.com/en/documentation/red_hat_openshift_ai_self-managed/2.16/html/serving_models/serving-large-models_serving-large-models#tested-verified-runtimes_serving-large-models)

### Non-Goals

- Deploying full RHOAI on MicroShift.
- Providing goodies such as RHOAI Dashboard. Using it will be more similar to upstream kserve.
- Securing serving endpoint - we'll defer this action to user if needed as inference might be consumed by other Pod.

## Proposal

Extract kserve manifests from RHOAI Operator image and adjust them for MicroShift:
- Make sure that cert-manager is not required, instead leverage OpenShift's service-ca.
  - This might require adding some extra annotations to resources so the service-ca injects the certs.
- Drop requirement for Istio as an Ingress Controller and use OpenShift Router instead.
  - Done by changing kserve's setting configmap to use another ingress controller.
  - RHOAI disables automatic Ingress creation by kserve and odh-model-controller takes over creation of appropriate CRs.
    We need to decide if we want to re-enable it or instruct user to create Route CR themselves.
- Use 'RawDeployment' mode, so that neither Service Mesh nor Serverless are required,
  to minimize to make the solution suitable for edge devices.
  - Also done in the configmap.
- Package the manifests as an RPM `microshift-kserve` for usage.

Provide users with ServingRuntime definitions derived from RHOAI, so they
are not forced to use upstream manifests.
Decision on how to do this is pending. See open questions.

### Rebase procedure in detail

1. `rebase_rhoai.sh` script shall take an argument pointing to the RHOAI Operator Bundle Image.
1. The script will:
   1. Extract ClusterServiceVersion (CSV) of the RHOAI Operator.
   1. Obtain RHOAI Operator image reference from the CSV.
   1. Extract /opt/manifests from the RHOAI Operator image.
   1. Copy contents of /opt/manifests/kserve to MicroShift repository.
   1. Copy contents of /opt/manifests/odh-model-controller/runtimes to MicroShift repository.
   1. Prepare top-level kustomization.yaml with MicroShift-specific tweaks.
      1. Create kserve settings ConfigMap to configure MicroShift-specific tweaks.
      1. Use contents of different files from RHOAI Operator's /opt/manifests
         to get downstream-rebuilt image references and apply them over the
         existing manifests.


### Workflow Description

**User** is a human administrating and using MicroShift cluster/device.

(RPM vs ostree vs bootc is skipped because it doesn't differ from any other MicroShift's RPM).

1. User installs `microshift-kserve` RPM and restarts MicroShift service.
1. Kserve manifest are deployed.
1. User configures the hardware, the OS, and additional Kubernetes components to
   make use of their accelerators.
1. ServingRuntimes are delivered with the kserve RPM or deployed by the user.
1. User creates InferenceService CR which references ServingRuntime of their choice
   and reference to the model.
1. Kserve creates Deployment, Ingress, and other.
1. Resources from previous step become ready and user can make HTTP/GRPC calls
   to the model server.

### API Extensions

`microshift-kserve` RPM will bring following CRDs, however they're not becoming
part of the core MicroShift deployment:
- InferenceServices
- TrainedModels
- ServingRuntimes
- InferenceGraphs
- ClusterStorageContainers
- ClusterLocalModels
- LocalModelNodeGroups

Contents of these CRDs can be viewed at https://github.com/red-hat-data-services/kserve/tree/master/config/crd/full.

### Topology Considerations

#### Hypershift / Hosted Control Planes

Enhancement is MicroShift specific.

#### Standalone Clusters

Enhancement is MicroShift specific.

#### Single-node Deployments or MicroShift

Enhancement is MicroShift specific.

### Implementation Details/Notes/Constraints

See "Proposal"

### Risks and Mitigations

At the time of writing this document, kserve's Raw Deployment mode is not fully
supported by the RHOAI. For for reason, this feature will start as Tech Preview
and only advance to GA when RHOAI starts supporting Raw Deployment mode.

### Drawbacks

At the time of writing this document, ARM architecture is not supported.
If that changes in the future, rebase process and RPM building will need to be
reworked (depending on how the RHOAI's manifests will look like).

## Open Questions [optional]

### Tweaking Kserve setting: ingress domain

**Update: If we disable creating Ingress and instruct user to create Route if**
**needed, there's no need to do following.**

Kserve settings are delivered in form of a ConfigMap:
- [Upstream example](https://github.com/red-hat-data-services/kserve/blob/master/config/configmap/inferenceservice.yaml)
- [RHOAI's overrides](https://github.com/red-hat-data-services/kserve/blob/master/config/overlays/odh/inferenceservice-config-patch.yaml)

While we can recommend users to create a manifest that will override the
ConfigMap to their liking, there's one setting that we could handle better:
`ingress.ingressDomain`.  In the example it has value of `example.com` and it
might be poor UX to require every customer to create a new ConfigMap just to change this.

Possible solution is to change our manifest handling logic, so that MicroShift 
uses kustomize's Go package to first render the manifest, then template it,
and finally apply. For this particular `ingress.ingressDomain` we could reuse value of
`dns.baseDomain` from MicroShift's config.yaml.


### How to deliver ServingRuntime CRs

From [kserve documentation](https://kserve.github.io/website/master/modelserving/servingruntimes/):

> KServe makes use of two CRDs for defining model serving environments:
> 
> ServingRuntimes and ClusterServingRuntimes
> 
> The only difference between the two is that one is namespace-scoped and the other is cluster-scoped.
> 
> A ServingRuntime defines the templates for Pods that can serve one or more 
> particular model formats. Each ServingRuntime defines key information such as
> the container image of the runtime and a list of the model formats that the
> runtime supports. Other configuration settings for the runtime can be conveyed
> through environment variables in the container specification.

RHOAI approach to ServingRuntimes:
- ClusterServingRuntimes are not supported (the CRD is not created and code that
  lists CSRs is commented out).
- Each usable ServingRuntime is wrapped in Template and resides in RHOAI's namespace.
- When user uses RHOAI Dashboard to serve a model, they must select a runtime
  from a list (which is constructed from Templates holding ServingRuntime)
  and provide information about the model.
- When the user sends the form, Dashboard creates a ServingRuntime from the
  Template in the user's Data Science Project (effectively Kubernetes namespace),
  and assembles the InferenceService CR.

Problem: how to provide user with ServingRuntimes without creating unnecessary obstacles.

In any of the following solutions, we need to drop the `Template` container to
get only the `ServingRuntime` CR part.

Potential solutions so far:
- Include ServingRuntimes in a RPM as a kustomization manifest (might be 
  `microshift-kserve`, `microshift-rhoai-runtimes`, or something else) using a
  specific predefined namespace.
  - This will force users to either use that namespace for serving, or they can
    copy the SRs to their namespace (either at runtime, or by including it in
    their manifests).
- Change ServingRuntimes to ClusterServingRuntimes and include them in an RPM,
  so they're accessible from any namespace (MicroShift is intended for
  single-user operation anyway).
  - This would also require change in `opendatahub-io/kserve` to start listing
    ClusterServingRuntimes as it's currently commented out (compared to upstream kserve).
    See GetSupportingRuntimes() function in [predictor_model.go](https://github.com/opendatahub-io/kserve/blob/master/pkg/apis/serving/v1beta1/predictor_model.go).
- Don't include SRs in any of the RPM. Instead include them in the documentation
  for users to copy and include in their manifests.
  - This might be prone to getting outdated very easily as documentation is not
    part of the MicroShift rebase procedure.


### No MicroShift support for GPU Operator, Node Feature Discovery, other hardware-enabling Operators, etc...

[RHOAI's how to on using Raw Deployment](https://docs.redhat.com/en/documentation/red_hat_openshift_ai_self-managed/2.16/html/serving_models/serving-large-models_serving-large-models#deploying-models-on-single-node-openshift-using-kserve-raw-deployment-mode_serving-large-models) lists some requirements such as:
> - If you want to use graphics processing units (GPUs) with your model server, you have enabled GPU support in OpenShift AI.
> - To use the vLLM runtime, you have enabled GPU support in OpenShift AI and have installed and configured the Node Feature Discovery operator on your cluster

~~Neither GPU Operator, NFD, or Intel Gaudi AI Accelerator Operator are supported on MicroShift.~~
[Accelerating workloads with NVIDIA GPUs with Red Hat Device Edge](https://docs.nvidia.com/datacenter/cloud-native/edge/latest/nvidia-gpu-with-device-edge.html)
is a procedure on setting up NVIDIA GPU for MicroShift. Users will be directed to
that website's relevant sections and also to [RHOAI's guide on adding NVIDIA Triton ServingRuntime](https://docs.redhat.com/en/documentation/red_hat_openshift_ai_self-managed/2.16/html/serving_models/serving-large-models_serving-large-models#adding-a-tested-and-verified-model-serving-runtime-for-the-single-model-serving-platform_serving-large-models).

~~Should users be instructed to use upstream releases of these components and configure them on their own?~~

If there's no existing guide on setting specific accelerator for usage with RHDE,
we shall work with partners to achieve that. We want to avoid directing users
to generic upstream information without any support.

### Do we need ODH Model Controller?

RHOAI Operator deploys both kserve and ODH Model Controller.
In the catalog image for Model Controller is described as:
> The controller removes the need for users to perform manual steps when deploying their models

Summary of ODH Model Controller's functionality in regards to Raw Deployment mode for InferenceService:
- Creates ClusterRoleBinding
  - Role: `system:auth-delegator`, Subject: InferenceService's Service Account
  - Seems not needed in MicroShift if we're delegating authentication to user
- Creates OpenShift Route CR
  - Route to expose the inference service.
  - We might delegate Route CR creation to the user, as not always inference service might require exposing outside.
- Creates Prometheus ServiceMonitor (it instructs in-cluster Prometheus' configuration to scrape the service)
  - MicroShift does not ship Prometheus - not needed
- Creates K8s Service for serving metrics
  - Probably for ServiceMonitor - not needed
- ConfigMap for a RHOAI Dashboard
  - Probably to configure Dashboard to present serving metrics - not needed for MicroShift

For the ServingRuntime CRs:
- Creates RoleBindings for monitoring - not needed as MicroShift doesn't ship Prometheus.

Model Controller also creates auth config (Authorino) for InferenceGraph.
MicroShift won't ship Authorino, so no need to include it. If user decides to use it,
they'll need to setup required elements.

Model Controller also watches Secrets to collect data connection secrets into
single Secret. Seems to be revelant to Model Mesh or using Dashboard, if that's the case
MicroShift users can use single secret for configuring access to models.

## Versioning

While it might be best to have RPM with RHOAI version, because RHOAI does not
follow OpenShift's release schedule and the RPM will live in the MicroShift
repository, it will be versioned together with MicroShift.

It means that certain version of RHOAI's kserve will be bound to MicroShift's
minor version.


## Test Plan

At the time of writing this enhancement, RHOAI is not yet supported on ARM
architecture, so testing on NVIDIA Jetson Orin devices will be postponed.

First, a smoke test in MicroShift's test harness:
- Stand up MicroShift device/cluster
- Install kserve
- Create an InferenceService using some basic model that can be easily handled by CPU
  - Model shall be delivered using ModelCar approach (i.e. model inside an OCI container)
- Make a HTTP call to the InferenceService to verify it is operational
- Make a HTTP call to the InferenceService's `/metrics` endpoint
  (possibly before and after an inference request)

This simple scenario will assert basic integration of the features:
that everything deploys and starts correctly - sort of quick and easy sanity test.
See [upstream kserve's "First InferenceService" guide](https://kserve.github.io/website/master/get_started/first_isvc/)
which represents similar verification.

Another test that should be implemented is using a hardware with an NVIDIA GPU,
for example AWS EC2 instance. Goal is to assert that all the elements in the
stack will work together on MicroShift. The test itself should not be much
different from the sanity test (setup everything, make a call to the
InferenceServing), but it should leverage a serving runtime and a model that
require a GPU.
Implementation of this test will reveal any additional dependencies such as
device plugin, drivers, etc.
This kind of test can run periodically - once or twice a week initially,
with frequency adjusted later if needed.

EC2 instance type candidates (ordered - chepeast first in us-west-2):
- g4dn.xlarge (4 vCores 2nd gen Intel Xeon, 16 GiB, NVIDIA T4 GPU 16GiB)
- g6.xlarge (4 vCores 3rd gen AMD EPYC, 16 GiB, NVIDIA L4 GPU 24GiB)
- g5.xlarge (4 vCores 2nd gen AMD EPYC, 16 GiB, NVIDIA A10G GPU 24GiB)
- g6e.xlarge (4 vCores 3rd gen AMD EPYC, 16 GiB, NVIDIA L40S GPU 48GiB)

## Graduation Criteria

### Dev Preview -> Tech Preview

RHOAI's kserve on MicroShift will begin Tech Preview.

### Tech Preview -> GA

Advancement to GA depends on RHOAI's support for Raw Deployments.

### Removing a deprecated feature

N/A

## Upgrade / Downgrade Strategy

By having RHOAI's kserve in MicroShift spec they will share version, so it's
expected that in case of MicroShift upgrade, the kserve also will be upgraded.

MicroShift team might need to monitor (or simply test upgrades) version changes
of the kserve CRDs, so there's always an upgrade path.

MicroShift team will need to monitor (or automate) RHOAI versions to make sure
each MicroShift release uses most recent stable RHOAI version (relates to the
MicroShift rebase procedure - RHOAI not being part of OCP payload is sort of
side loaded).

## Version Skew Strategy

MicroShift and kserve RPMs built from the same .spec file should not introduce
a version skew.

## Operational Aspects of API Extensions

N/A

## Support Procedures

For the most part, RHOAI's and/or kserve support procedures are to be followed.

Although there might some cases where debugging MicroShift might be required.
One example is Ingress and Routes, as this is the element that kserve will
integrate most with that is shipped as part of MicroShift.
So procedures for debugging OCP router are to be followed.

Other cases might involve hardware integration failures, those might depend
on individual components.

## Alternatives

N/A

## Infrastructure Needed [optional]

N/A
