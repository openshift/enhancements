---
title: Installing and Scaling Clusters on Dedicated Hosts in Azure
authors:
  - "@rvanderp3"
reviewers:
  - "@nrb" # CAPZ/MAPI
  - "@patrickdillon" # installer 
  - "@makentenza" # Product Manager
approvers: 
  - "@patrickdillon"
creation-date: 2025-04-23
api-approvers: # In case of new or modified APIs or API extensions (CRDs, aggregated apiservers, webhooks, finalizers). If there is no API change, use "None"
  - "@JoelSpeed"
last-updated: 2025-04-23
tracking-link: 
  - https://issues.redhat.com/browse/SPLAT-1864
see-also: {}
replaces: {}
superseded-by: {}
---

# Installing and Scaling Clusters on Dedicated Hosts in Azure

## Summary

This enhancement proposal outlines the work required to enable OpenShift to deploy and scale nodes onto pre-created dedicated Azure hosts. This leverages existing Azure infrastructure, focusing on the automated deployment process and integrating OpenShift’s node management CAPZbilities with these hosts. The goal is to provide a simplified deployment and management experience for users who already have dedicated hosts set up.

## Motivation

### User Stories

1. As an administrator, I want to install OpenShift on a dedicated Azure host so my workloads are isolated from other tenants.

2. As an administrator, I need to relevant conditions, events, and/or alerts to ensure I can diagnose deployments on dedicated hosts.

3. As an administrator, I want to scale my OpenShift cluster by adding or removing nodes on dedicated hosts with machine API and/or CAPI.

### Goals

- Enable OpenShift to install on pre-existing, dedicated Azure hosts.
- Enable OpenShift to scale nodes on pre-existing, dedicated Azure hosts.
- Provide meaningful conditions, events, and/or alerts for deployments on dedicated hosts.

### Non-Goals

- Deploying or otherwise managing dedicated hosts on Azure.

## Proposal

Upstream support for dedicated hosts is being added [upstream](https://github.com/kubernetes-sigs/cluster-api-provider-Azure/pull/5398). This PR will introduce support for dedicated hosts in the Azure provider. The OpenShift installer and machine management components([machine|cluster] API) will be updated to support dedicated hosts based on the upstream changes.

Changes to the OpenShift installer will include:
- Introduce fields for dedicated hosts in the [installer machinePool for Azure](https://github.com/openshift/installer/blob/main/pkg/types/Azure/machinepool.go).
- Update the [manifest generation logic](https://github.com/openshift/installer/blob/main/pkg/infrastructure/Azure/clusterapi/Azure.go) to include dedicated hosts when generating manifests for CAPI.

Changes to the machine management components will include:
- Introduce fields for dedicated hosts in the [machine API](https://github.com/openshift/api/blob/master/machine/v1beta1/types_Azureprovider.go#L12).
- Update the [machine reconciliation logic](https://github.com/openshift/machine-api-provider-Azure/blob/main/pkg/actuators/machine/reconciler.go) to handle dedicated hosts when reconciling machines.

### Workflow Description

**Azure administrator** is a human user responsible for deploying dedicated hosts.
1. The Azure administrator logs into the Azure Management Console...
2. The Azure administrator creates a dedicated host
3. The Azure administrator provides the dedicated host ID to the cluster creator.

**cluster creator** is a human user responsible for deploying a
cluster.

1. The cluster creator creates a new install-config.yaml file.
2. The cluster creator specifies the dedicated host ID in the install-config.yaml file in the machine pool.
3. The cluster creator runs the `openshift-install create cluster` command to deploy the cluster.

If any errors are encountered, the installer should log a descriptive error message and exit. The cluster creator should be able to troubleshoot the issue by reviewing the logs and retrying the deployment.

**cluster maintainer** is a human user responsible for day 2 operations on the cluster.
1. The cluster maintainer logs into the OpenShift web console to scale/up or down nodes in a machine set.

If any errors are encountered, the machine API controller should log a descriptive error message and set a condition on the impacted machine(s). The cluster creator should be able to troubleshoot the issue by reviewing the logs and conditions.

### API Extensions

TBD


### Topology Considerations

In general, this feature should be transparent to OpenShift components aside from honoring and/or propogating the dedicated host configuration
to [cluster|machine] API.

#### Hypershift / Hosted Control Planes

#### Standalone Clusters

#### Single-node Deployments or MicroShift

### Implementation Details/Notes/Constraints

What are some important details that didn't come across above in the
**Proposal**? Go in to as much detail as necessary here. This might be
a good place to talk about core concepts and how they relate. While it is useful
to go into the details of the code changes required, it is not necessary to show
how the code will be rewritten in the enhancement.

### Risks and Mitigations

What are the risks of this proposal and how do we mitigate. Think broadly. For
example, consider both security and how this will impact the larger OKD
ecosystem.

How will security be reviewed and by whom?

How will UX be reviewed and by whom?

Consider including folks that also work outside your immediate sub-project.

### Drawbacks

N/A

## Alternatives (Not Implemented)

N/A

## Open Questions [optional]

## Test Plan

**Note:** *Section not required until targeted at a release.*

- Atleast 5 new e2e tests will be created and run during the tech preview and feature gate promotion period.

## Graduation Criteria

**Note:** *Section not required until targeted at a release.*

Define graduation milestones.

These may be defined in terms of API maturity, or as something else. Initial proposal
should keep this high-level with a focus on what signals will be looked at to
determine graduation.

Consider the following in developing the graduation criteria for this
enhancement:

- Maturity levels
  - [`alpha`, `beta`, `stable` in upstream Kubernetes][maturity-levels]
  - `Dev Preview`, `Tech Preview`, `GA` in OpenShift
- [Deprecation policy][deprecation-policy]

Clearly define what graduation means by either linking to the [API doc definition](https://kubernetes.io/docs/concepts/overview/kubernetes-api/#api-versioning),
or by redefining what graduation means.

In general, we try to use the same stages (alpha, beta, GA), regardless how the functionality is accessed.

[maturity-levels]: https://git.k8s.io/community/contributors/devel/sig-architecture/api_changes.md#alpha-beta-and-stable-versions
[deprecation-policy]: https://kubernetes.io/docs/reference/using-api/deprecation-policy/

**If this is a user facing change requiring new or updated documentation in [openshift-docs](https://github.com/openshift/openshift-docs/),
please be sure to include in the graduation criteria.**

**Examples**: These are generalized examples to consider, in addition
to the aforementioned [maturity levels][maturity-levels].

### Dev Preview -> Tech Preview

- Ability to utilize the enhancement end to end
- End user documentation, relative API stability
- Sufficient test coverage
- Gather feedback from users rather than just developers
- Enumerate service level indicators (SLIs), expose SLIs as metrics
- Write symptoms-based alerts for the component(s)

### Tech Preview -> GA

- More testing (upgrade, downgrade, scale)
- Sufficient time for feedback
- Available by default
- Backhaul SLI telemetry
- Document SLOs for the component
- Conduct load testing
- User facing documentation created in [openshift-docs](https://github.com/openshift/openshift-docs/)

**For non-optional features moving to GA, the graduation criteria must include
end to end tests.**

### Removing a deprecated feature

- Announce deprecation and support policy of the existing feature
- Deprecate the feature

## Upgrade / Downgrade Strategy

N/A

## Version Skew Strategy

N/A


********** WIP: Below this line needs to be completed **********

## Operational Aspects of API Extensions

Describe the impact of API extensions (mentioned in the proposal section, i.e. CRDs,
admission and conversion webhooks, aggregated API servers, finalizers) here in detail,
especially how they impact the OCP system architecture and operational aspects.

- For conversion/admission webhooks and aggregated apiservers: what are the SLIs (Service Level
  Indicators) an administrator or support can use to determine the health of the API extensions

  Examples (metrics, alerts, operator conditions)
  - authentication-operator condition `APIServerDegraded=False`
  - authentication-operator condition `APIServerAvailable=True`
  - openshift-authentication/oauth-apiserver deployment and pods health

- What impact do these API extensions have on existing SLIs (e.g. scalability, API throughput,
  API availability)

  Examples:
  - Adds 1s to every pod update in the system, slowing down pod scheduling by 5s on average.
  - Fails creation of ConfigMap in the system when the webhook is not available.
  - Adds a dependency on the SDN service network for all resources, risking API availability in case
    of SDN issues.
  - Expected use-cases require less than 1000 instances of the CRD, not impacting
    general API throughput.

- How is the impact on existing SLIs to be measured and when (e.g. every release by QE, or
  automatically in CI) and by whom (e.g. perf team; name the responsible person and let them review
  this enhancement)

- Describe the possible failure modes of the API extensions.
- Describe how a failure or behaviour of the extension will impact the overall cluster health
  (e.g. which kube-controller-manager functionality will stop working), especially regarding
  stability, availability, performance and security.
- Describe which OCP teams are likely to be called upon in case of escalation with one of the failure modes
  and add them as reviewers to this enhancement.

## Support Procedures

Describe how to
- detect the failure modes in a support situation, describe possible symptoms (events, metrics,
  alerts, which log output in which component)

  Examples:
  - If the webhook is not running, kube-apiserver logs will show errors like "failed to call admission webhook xyz".
  - Operator X will degrade with message "Failed to launch webhook server" and reason "WehhookServerFailed".
  - The metric `webhook_admission_duration_seconds("openpolicyagent-admission", "mutating", "put", "false")`
    will show >1s latency and alert `WebhookAdmissionLatencyHigh` will fire.

- disable the API extension (e.g. remove MutatingWebhookConfiguration `xyz`, remove APIService `foo`)

  - What consequences does it have on the cluster health?

    Examples:
    - Garbage collection in kube-controller-manager will stop working.
    - Quota will be wrongly computed.
    - Disabling/removing the CRD is not possible without removing the CR instances. Customer will lose data.
      Disabling the conversion webhook will break garbage collection.

  - What consequences does it have on existing, running workloads?

    Examples:
    - New namespaces won't get the finalizer "xyz" and hence might leak resource X
      when deleted.
    - SDN pod-to-pod routing will stop updating, potentially breaking pod-to-pod
      communication after some minutes.

  - What consequences does it have for newly created workloads?

    Examples:
    - New pods in namespace with Istio support will not get sidecars injected, breaking
      their networking.

- Does functionality fail gracefully and will work resume when re-enabled without risking
  consistency?

  Examples:
  - The mutating admission webhook "xyz" has FailPolicy=Ignore and hence
    will not block the creation or updates on objects when it fails. When the
    webhook comes back online, there is a controller reconciling all objects, applying
    labels that were not applied during admission webhook downtime.
  - Namespaces deletion will not delete all objects in etcd, leading to zombie
    objects when another namespace with the same name is created.

## Infrastructure Needed

- Dedicated host(s) for unit testing, e2e, and development purposes.