---
title: infrastructure-external-platform-type
authors:
  - "@lobziik"
  - "@elmiko"
reviewers:
  - "@dhellmann"
  - "@mhrivnak"
  - "@rvanderp3"
  - "@mtulio"
  - "@deads2k, to review library-go, KCMO related parts andopenshift/api changes"
  - "@JoelSpeed, to review CCM, MAPO related parts and openshift/api changes"
  - "@sinnykumari, to review MCO related parts"
  - "@danwinship, to review CNO related parts"
  - "@Miciah, to review Ingress related parts"
  - "@openshift/openshift-team-windows-containers"
approvers:
  - "@dhellmann"
  - "@deads2k"
  - "@bparees"
api-approvers:
  - "@deads2k"
  - "@JoelSpeed"
creation-date: 2022-09-06
last-updated: 2022-10-25
tracking-link:
  - https://issues.redhat.com/browse/OCPPLAN-9429
  - https://issues.redhat.com/browse/OCPPLAN-8156
see-also:
  - "[KEP-2392 Cloud Controller Manager](https://github.com/kubernetes/enhancements/tree/master/keps/sig-cloud-provider/2392-cloud-controller-manager)"
  - "[OCP infrastructure provider onboarding guide](https://docs.providers.openshift.org/overview/)"
  - "[Out-of-tree cloud provider integration support](https://github.com/openshift/enhancements/blob/master/enhancements/cloud-integration/out-of-tree-provider-support.md)"
  - "[Platform Operators Proposal](https://github.com/openshift/enhancements/blob/master/enhancements/olm/platform-operators.md)"
  - "[Capabilites selection](https://github.com/openshift/enhancements/blob/master/enhancements/installer/component-selection.md)"
  - "[Bare metal in-cluster network infrastructure](https://github.com/openshift/enhancements/blob/ce4d303db807622687159eb9d3248285a003fabb/enhancements/network/baremetal-networking.md)"
---

# Introduce new platform type "External" in the OpenShift specific Infrastructure resource

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

In an effort to reduce the amount of time Red Hat engineers spend directly involved with third party engagements,
adding new platforms into the OpenShift product, this enhancement describes how we will add a new `External`
platform type that will allow third parties to self-serve and integrate with OpenShift without the need to modify
any core payload components and without the need for direct involvement of OpenShift engineers.

## Motivation

Historically, the k8s project contained plenty of code for handling integration with
various cloud providers (AWS, GCP, vSphere). These pieces were developed and released as part of Kubernetes core.
However, over time, the community concluded that this approach does not scale well and should be changed.
The community put a lot of energy into introducing the [mechanism](https://github.com/kubernetes/enhancements/tree/master/keps/sig-cloud-provider/2392-cloud-controller-manager) to allow cloud-providers and community members
to build, develop, test and release provider-specific components independently from the Kubernetes core.

With regard to OpenShift integrations with cloud providers, for the moment, a lot of things are tended to be encoded
in [the OpenShift codebase](https://docs.providers.openshift.org/cloud-controller-manager/#integrating-with-other-openshift-components)
within API definitions, operator logic, and installer program code.
This fact creates quite a lot of obstacles for RH partners and community members in their attempts to add new cloud providers to OpenShift,
as well as making RH engineering involvement quite necessary.

Lately, there have been several initiatives around making OpenShift more composable and flexible.
For example, [Capabilites selection](https://github.com/openshift/enhancements/blob/master/enhancements/installer/component-selection.md) and
[Platform Operators](https://github.com/openshift/enhancements/blob/master/enhancements/olm/platform-operators.md) are
significant steps in this direction. However, despite having these powerful instruments, it is still necessary
to land code into the OpenShift codebase for technical enablement of a new cloud provider, which might be hard or
nearly impossible for external contributors.

Imagine some regional or special-purpose cloud have created an infrastructure platform that resembles AWS but has its own API that is different than AWS.
They would like to give their users the best OpenShift experience possible, but integrating their code into a Red Hat release is not possible for them.
Using the "External" platform, [capabilities](https://github.com/openshift/enhancements/blob/master/enhancements/installer/component-selection.md),
and [platform operators](https://github.com/openshift/enhancements/blob/master/enhancements/olm/platform-operators.md),
they can still deliver this functionality by creating their own Cloud Controller Managers,
CSI drivers, network topology operators, Machine API controllers, and OpenShift configurations.
This allows these cloud providers to supply the best OpenShift experience while also developing their components
without the necessity of tying to Red Hat's internal processes or keeping a fork of the significant part of the OpenShift code base.

### User Stories

- As a cloud-provider affiliated engineer / platform integrator / RH partner
  I want to have a mechanism to signal OpenShift's built-in operators about additional
  cloud-provider specific components so that I can inject my own platform-specific controllers into OpenShift
  to improve the integration between OpenShift and my cloud provider.

  We are aware of examples of supplementing OKD installations with custom machine-api controllers,
  however, users are experiencing a lot of difficulties on this path due to the necessity of, literally, reverse engineering,
  manual management of generic MAPI controllers, and so on.

- As a cloud provider whose platform is not integrated into OpenShift,
  I want to have the Cloud Controller Manager for my infrastructure running in OpenShift from the initial install.
  Having a platform type that allows for the addition of operators or components which perform platform-specific functionality would help
  me to create better integrations between OpenShift and my infrastructure platform.

### Goals

- remove the necessity to make changes in "openshift/api", "openshift/library-go" and dependant infrastructure-related components during basic cloud-provider integration with OpenShift
- make a cloud provider integration process more accessible and simple to external contributors as well as for RH engineers
- provide an overview of projected changes to affected components that will be planned for a later phase of development
- introduce a somewhat "neutral" platform type, which would serve as a signal about an underlying generic cloud-provider presence

### Non-Goals

- describe concrete delivery mechanisms for cloud-provider specific components
- cover new infrastructure provider enablement from the RHCOS side
- describe specific changes for each affected component, aside no-op reaction to the new "External" platform type

## Proposal

Our main goal is to simplify the integration process for new cloud providers in OpenShift/OKD. To achieve this
we are proposing to add a new "External" [PlatformType](https://github.com/openshift/api/blob/51f399230d604fa013c2bb341040c4ad126e7309/config/v1/types_infrastructure.go#L128)
along with respective Spec and Status structures in openshift/api.

Such a generic platform type will serve as a signal for built-in OpenShift operators about an underlying cloud-provider presence.
Related PlatformSpec and PlatformStatus type structures will serve as a source of generic configuration information for
the OpenShift-specific operators.

Having that special platform type will allow infrastructure partners to clearly designate when their OpenShift
deployments contain components that replace and/or supplement the core Red Hat components.

### Implementation phases

Splitting the project into phases would be natural to make the implementation process smoother.
A reader can find the proposed phase breakdown in [OCPPLAN-9429](https://issues.redhat.com/browse/OCPPLAN-9429).

This document intends to describe the initial phases of this project. The proposed initial course of action:

1. Update "openshift/api" with adding "External" PlatformType
2. Ensure that all Red Hat operators tolerate the "External" platform and treat it the same as the "None" platform

Next phase which is out of the scope for this EP:

3. Modify operators for doing specific things when seeing the "External" platform.
   It will be briefly described in the [Affected Components](#affected-components) below.
   However, this should be addressed in separate EPs on a per-component basis.

#### Future work

There are several topics in this area that would be wise to defer for upcoming phases, namely:

1. Define missing capabilities and their concrete behaviour, for example, add a "capability" for machine-api
2. Precisely define the reaction of the operators listed below for the "External" platform type
3. Define and document concrete mechanisms for supplementing a cluster with provider-specific components at installation time (CCM, MAPI controller, DNS controller)
4. Research the necessity for engagement and api extension for ["on-prem"-like in-cluster network infrastructure](https://github.com/openshift/enhancements/blob/ce4d303db807622687159eb9d3248285a003fabb/enhancements/network/baremetal-networking.md) for the "External" platform.
   This will depend on demand from partner cloud providers and their cloud capabilities (the presence of a load-balancer-like concept, for example).

### Background

At the moment, the [Infrastructure](https://github.com/openshift/api/blob/51f399230d604fa013c2bb341040c4ad126e7309/config/v1/types_infrastructure.go#L14)
resource serves as a primary source of information about the underlying infrastructure provider and provider-specific parameters.
Specifically the [PlatformSpec](https://github.com/openshift/api/blob/51f399230d604fa013c2bb341040c4ad126e7309/config/v1/types_infrastructure.go#L47)
and [PlatformStatus](https://github.com/openshift/api/blob/51f399230d604fa013c2bb341040c4ad126e7309/config/v1/types_infrastructure.go#L65) parts
of the infrastructure resource.

Given that PlatformSpec and PlatformStatus are defined as "discriminated unions" and are required to have the platform type encoded within
"openshift/api" beforehand, it requires significant involvement and effort from Red Hat engineers to create the initial technical enablement of
a new cloud provider and is effectively impossible without Red Hat engineering engagement.

Since a lot of infrastructure related components (such as [CCCMO](https://github.com/openshift/cluster-cloud-controller-manager-operator), [Machine API operator](https://github.com/openshift/machine-api-operator), [Machine Config Operator](https://github.com/openshift/machine-config-operator), [Windows Machine Config Operator](https://github.com/openshift/windows-machine-config-operator) and so on)
require information about, at least, the presence of an underlying cloud provider, the "None" platform does not fit well as a signal in such a situation.

A special, built-in, and somewhat generic platform type that will signal about the presence of an underlying infrastructure platform without
platform-specific details will help to reduce the number of changes across OCP repositories
and simplify the initial integration work for non-Red Hat contributors.
Such a new ability will allow smaller / regional cloud providers to build and test their integration with OpenShift
with considerably less effort.

Additionally, there are difficulties that are present today due to having a predefined list of platforms.
A few examples:
- No defined mechanism to set `--cloud-provider=external` arg to kubelet/KCM/apiserver without merges and further revendoring of "openshift/api",
  at the moment decision-making here is tied to the PlatformType.
- No way to extend machine-api and deliver a new provider without merges to "openshift/machine-api-operator" and "openshift/api" repos.

#### Conjunction with Capabilities

In the future, to some degree, an approach based on [capabilites selection](https://github.com/openshift/enhancements/blob/master/enhancements/installer/component-selection.md)
might help to solve the issue and provide an option for supplementing *some* platform-dependent components.  However, some integral parts of
OpenShift cannot be disabled and still require a signal about the underlying platform, for example, KCM and MCO, with respect to enabling an
[external cloud controller manager](https://kubernetes.io/docs/concepts/architecture/cloud-controller/).

One of the possible examples of the interaction of the capabilities with the "External" platform type would be the MachineAPI and the Machine Api Operator.
At this point, the MachineAPI has no use in running if there is no machine controller (which is heavily cloud-provider dependent).
When the platform type is set to "External", and the machine-api capability is enabled,
that will cause Machine API operator to deploy only generic cloud-independent controllers (such as machine-healthcheck, machineset, and node-link controllers).
The platform-specific components would be deployed through a separate mechanism.
Such behaviour will simplify initial cloud-platform enablement and will reduce the necessity of reverse-engineering
and replicating work that was already done by Red Hat engineers.

### Affected components

This section enumerates OpenShift's components and briefly elaborates the future plans around this proposal.
During [initial implementation](#implementation-phases) we must ensure that all OpenShift's components treat "External" platform in the same way as
"None" in order to ensure a consistent baseline across OpenShift components.

In the future, we will need to change the behavior of OpenShift components on a case-by-case basis to be able to function harmoniously with supplemental provider-specific components from an infrastructure provider
or, if a component manages something else (e.g. kubelet, kcm), adjust its behaviour (set `--cloud-provider=external` arg to kubelet for example).

Specific component changes will be described in detail within separate enhancement documents on a per-component basis.

#### Library Go

Significant part of the code around PlatformType handling lives in the ["openshift/library-go"](https://github.com/openshift/library-go/blob/e1213f6ec5d10aa4aa8a4cac2780b1dc674c0396/pkg/operator/configobserver/cloudprovider/observe_cloudprovider.go).

Currently, this code is responsible for the decision around kubelet and kcm flags. Specifically, the [IsCloudProviderExternal](https://github.com/openshift/library-go/blob/e1213f6ec5d10aa4aa8a4cac2780b1dc674c0396/pkg/operator/configobserver/cloudprovider/observe_cloudprovider.go#L154) function
is used for decisions around kubelet and KCM flags (within MCO and KCMO respectively).
Also, this code is used for the decision-making about CCM operator engagement.

This piece should be changed to react appropriately to the "External" platform type. During the first phases, it will need
to behave the same as in the case of the "None" platform type. Then, in upcoming phases,
it will need to respect additional parameters from the "External" platform spec down the road.

#### Kube Controller Manager Operator

Same as the [MCO](#machine-config-operator) in regard to kubelet, Kube Controller Manager Operator manages KCM (kube-controller-manager) deployments.

Historically Kube Controller Manager was home for cloud-specific control loops. This logic is also engaged by setting up proper
flags on KCM executable like,
```shell
...
--cloud-provider=azure
--cloud-config=/etc/kubernetes/cloud.conf
...
```

For engaging an external Cloud Controller Manager no `cloud-provider` flag should be set for the KCM executable.

In the context of this EP, no particular changes will be needed in the operator itself,
changes made in [library-go](#library-go) with further dependency update should be suffice.

#### Machine Config Operator

Currently, MCO sets kubelet flags based on a [PlatformStatus and PlatformType](https://github.com/openshift/machine-config-operator/blob/7babc03f84088f49072c6f5b0d1769850d09a694/pkg/controller/template/render.go#L343).
This flag is [crucial](https://kubernetes.io/docs/tasks/administer-cluster/running-cloud-controller/#running-cloud-controller-manager) for Cloud Controller Manager engagement within the cluster.

Initially, the new "External" platform should be treated similarly to PlatformType "None" by the MCO, and do not set up any cloud-specific flags for the kubelet.
Then, down the road (during [phase 3](#implementation-phases)), it would be expected for the MCO to use the "External" platform type and its spec as a signal about the underlying platform and cloud controller manager presence and operate accordingly.
For an explicit signal about the necessity to set `--cloud-provider=external` flag to the kubelet, we will use the `CloudControllerManager` field of the `ExternalPlatformSpec`,
which is described in the [API Extensions](#api-extensions) section down below.

#### Windows Machine Config Operator

The Windows Machine Config Operator configures Windows instances into nodes, enabling Windows container workloads to run within OCP clusters.

Its behaviour relies on the [Machine Config Operator](#machine-config-operator) since Windows-related machinery uses MCO-rendered ignition files
(there are plans to switch this to use MachineConfigs) to extract and then use some kubelet flags, including the `--cloud-provider` one.

Initially, the new "External" platform should be treated similarly to PlatformType "None" by the WMCO.

Important to note that WMCO has specific behaviour for the `None` platform type.
With this platform type WMCO [will](https://github.com/openshift/windows-machine-config-operator/blob/95a2c5225a066f40fb4af44cbd5a4d4d763c149e/pkg/windows/windows.go#L654) [set](https://github.com/openshift/windows-machine-config-operator/blob/95a2c5225a066f40fb4af44cbd5a4d4d763c149e/pkg/windows/windows.go#L654)
`--node-ip` flag with the user-provided IP address, which requires additional configuration.
For other supported platform types, WMCO relies on MachineAPI to figure out IP addresses or does not set this flag at all.
This seems acceptable for the initial phase, but during [phase 3](#implementation-phases) this behaviour should be revised and changed
to provide users additional knobs to configure this behaviour or, perhaps, check MachineAPI engagement to make a decision.

#### Cluster Cloud Controller Manager Operator

Responsible for deploying platform-specific Cloud Controller Manager as well as for handling a number of OpenShift's
specific peculiarities (such as populating proxy settings for CCMs, sync credentials, and so on).

The code from the [library-go](#library-go) is used for decision-making about operator engagement.
In case library-go's [IsCloudProviderExternal](https://github.com/openshift/library-go/blob/e1213f6ec5d10aa4aa8a4cac2780b1dc674c0396/pkg/operator/configobserver/cloudprovider/observe_cloudprovider.go#L154) function
indicates that the cloud provider is external and the operator encounters a platform which it is not aware of, it will go into 'Degraded' state.

During the first phases of the "External" platform type enablement, this operator should be just disabled.
This might be done with changes within the [library-go](#library-go) and further dependency updates or, better,
by adding a respective check within the operator itself.

In the future, when the delivery mechanism for CCMs will be defined,
the operator might be engaged for deploying a user-provided cloud controller manager,
however this is a subject for the upcoming design work.

#### Machine Api Operator

Machine Api Operator is responsible for deploying and maintaining the set of machine-api related controllers, such as

- machineset controller
- nodelink controller
- machine health check controller
- machine controller

From the list above, only the "machine controller" is cloud-provider dependent, however, for now, Machine Api Operator
won't deploy anything if it encounters "None" or an unrecognized platform type.

In the future, "External" platform type, in conjunction with an enabled capability,
would serve as a signal for Machine Api Operator to deploy only provider-agnostic
controllers, which would leave room for the user to supplement only the machine controller and not to reverse engineer and
replicate everything that MAO does.

#### Cluster Storage Operator

Cluster Storage Operator will go to no-op state if it encounters PlatformType "None" or an unknown PlatformType.

At this point, nothing requires storage to be there during cluster installation, and storage (CSI) drivers might be supplemented
later via OLM or some other way as day two operation.

No particular changes in regards to the "External" platform type introduction are expected there.

#### Cloud Credential Operator

Cloud Credential Operator is responsible for handling `CredentialsRequest` custom resource.
`CredentialsRequest`s allow OpenShift components to request credentials for a particular cloud provider.

On unsupported platforms, the operator goes into no-op mode, which technically is mostly an equivalent
of the ["Manual"](https://docs.openshift.com/container-platform/4.11/authentication/managing_cloud_provider_credentials/cco-mode-manual.html)
mode.

Cloud Credential operator [uses](https://github.com/openshift/cloud-credential-operator/blob/master/pkg/operator/controller.go#L74) an "actuator pattern"
and, in theory, in the future, it might be extended in a way to react to the new "External" platform type and allow users to supplement their own
platform-specific credentials management logic.

During initial enablement phases of "External" platform type, no specific actions will be needed there, since
CCO [would](https://github.com/openshift/cloud-credential-operator/blob/58c41771a87e613415a1fa16d299601a1c2f48c2/pkg/operator/controller.go#L134) go
into no-op mode if it encounters an unrecognized platform.

#### Cluster Image Registry Operator

For image registry a [storage backend config decision](https://github.com/openshift/cluster-image-registry-operator/blob/99474318db709a6d17d06468b90cdf0dc0fd2b87/pkg/storage/storage.go#L157) is platform specific.
With the "None" platform type CIRO goes into no-op state, which means that no registry will be deploy in such case.
The image registry configures with EmptyDir storage for unknown platform type at the moment.

Image Registry storage options will be configured to use PVC-backed or external storage systems
(such as Ceph or S3 compatible object storage) as a day two operation.

For now, it seems that no particular action for the "External" platform type is needed within the Image Registry Operator,
since we're providing enough possibilities to customize Image Registry storage backend.

#### Cluster Ingress Operator

Within the ingress operator, a PlatformType affects two things:
1. Choosing EndpointPublishingStrategy, which is `HostNetworkStrategyType` for "None" and unknown PlatformType
2. Creating DNS provider on some platforms. This logic does not engage for "None" or unknown PlatformType. The DNS provider is used to create a wildcard DNS record for ingress when using `LoadBalancerServiceStrategyType`. It is not used for `HostNetworkStrategyType`.

With regard to the [EndpointPublishingStrategy](https://docs.openshift.com/container-platform/4.11/networking/ingress-operator.html#nw-ingress-controller-configuration-parameters_configuring-ingress),
the cluster admin can configure Cluster Ingress Operator to use `LoadBalancerServiceStrategyType` as a day two operation.
The operator itself creates `Service` objects with correct provider-specific annotations,
the actual handling of such objects happens in a provider-specific [cloud controller manager](https://kubernetes.io/docs/concepts/architecture/cloud-controller/).

#### Cluster Network Operator

Speaking about Cluster Network Operator, several things depends on PlatformType there.

- Component called CNCC ([cloud-network-config-controller](https://github.com/openshift/cloud-network-config-controller))
  contains the majority of platform-specific logic. CNO makes the decision about CNCC deployment based on
  the PlatformType value. Mainly CNCC itself is responsible for attaching/detaching private IP addresses to VMs associated with
  Kubernetes nodes, which require interaction with cloud-provider APIs.
  Currently, CNCC [deploys](https://github.com/openshift/cluster-network-operator/blob/33287d5d9580556ca1757fd4f20b1a6b99e68786/pkg/network/cloud_network.go#L29)
  on GCP, Azure, AWS, and OpenStack platforms, other platforms, such as IBM or Alibaba do not engage CNCC for the moment.

- There are also several platform specific hacks,
  like the [access restriction to a metadata service](https://github.com/openshift/ovn-kubernetes/blob/8d5db0bae387de423eb30db75142b0ad87df4f3f/go-controller/pkg/cni/OCP_HACKS.go#L18),
  but it is not entirely connected with the operator itself and is more CNI plugin specific.

For the [phase 1](#implementation-phases) of this project, there seems to be no particular action or API knobs needed regarding
the addition of the "External" platform type. Just ensure that CNO is non-reactive to the "External" platform type
and behaves the same as in the "None" platform case.
In the future, we possibly might want to make CNO more tolerant of partner CNCC implementations
and design a way for configuring platform-specific CNI behaviour.

### Workflow Description

During [phase 1](#implementation-phases), the proposed changes are intended to have no effect, and the "External"
platform type should be handled the same as the "None", so no specific user interaction is expected.

### API Extensions

A new optional constant of `PlatformType` type will be added to the "openshift/api".

```go
const (
    ...
    // ExternalPlatformType represent generic infrastructure provider. Provider-specific components should be supplemented separately.
    ExternalPlatformType PlatformType = "External"
    ...
)
```

Additionally, the respective external platform spec and status should be added to the infrastructure resource.

```go
// ExternalPlatformSpec holds the desired state for the generic External infrastructure provider.
type ExternalPlatformSpec struct{
    // PlatformName holds the arbitrary string represented cloud provider name, expected to be set at the installation time.
    // Intended to serve only for informational purposes and not expected to be used for decision-making.
    // +kubebuilder:default:="Unknown"
    // +optional
    PlatformName string `json:"platformName,omitempty"`
}

type PlatformSpec struct {
    ...
    // External contains settings specific to the generic External infrastructure provider.
    // +optional
    External *ExternalPlatformSpec `json:"external,omitempty"`
}
```

For the sake of consistency, status should be introduced as well, and will
define the settings set at installation time:

```go
type CloudControllerManagerMode string

const (
    // Cloud Controller Manager is enabled and expected to be supplied.
    // Signaling that kubelets and other CCM consumers should use --cloud-provider=external.
    CloudControllerManagerExternal CloudControllerManagerMode = "External"
    // Cloud Controller Manager is enabled and expected to be supplied.
    // Signaling that kubelets and other CCM consumers should not set --cloud-provider flag.
    CloudControllerManagerNone CloudControllerManagerMode = "None"
)

// CloudControllerManagerStatus holds the state of Cloud Controller Manager (a.k.a. CCM or CPI) related settings
type CloudControllerManagerStatus struct {
    // state determines whether or not an external Cloud Controller Manager is expected to
    // be installed within the cluster.
    // https://kubernetes.io/docs/tasks/administer-cluster/running-cloud-controller/#running-cloud-controller-manager
    //
    // When set to "External", new nodes will be tainted as uninitialized when created,
    // preventing them from running workloads until they are initialized by the cloud controller manager.
    // When omitted or set to "None", new nodes will be not tainted
    // and no extra initialization from the cloud controller manager is expected.
    // +kubebuilder:default:=""
    // +default=""
    // +kubebuilder:validation:Enum="";External;None
    // +optional
    State CloudControllerManagerState `json:"state"`
}

// ExternalPlatformStatus holds the current status of the generic External infrastructure provider.
type ExternalPlatformStatus struct {
    // CloudControllerManager contains settings specific to the external Cloud Controller Manager (a.k.a. CCM or CPI)
    // +optional
    CloudControllerManager CloudControllerManagerStatus `json:"cloudControllerManager"`
}

type PlatformStatus struct {
    ...
    // External contains settings specific to the generic External infrastructure provider.
    // +optional
    External *ExternalPlatformStatus `json:"external,omitempty"`
}
```

### Implementation Details/Notes/Constraints

### Risks and Mitigations

#### Support burden
There is concern that *random* customers will use this feature out of context
and create a support burden. However, using such platform type in conjunction with [capabilites selection](https://github.com/openshift/enhancements/blob/master/enhancements/installer/component-selection.md)
could give us a clear signal about how to properly triage third-party created replacements for Red Hat components.

### Drawbacks

Current approach with having statically defined platform list is already there, quite transparent and battle-hardened.
Changing it by adding the new, way less specific platform type would mean a significant step away from current design patterns in this area.

Also, future support strategy is not completely clear due to our plans around enablement of third-party platform-specific components, which
codebase would be mostly out of our control.

## Design Details

### Open Questions

- Should we explicitly communicate that the "External" platform is something that we do not support yet?

### Answered question

**Q**: Should we gate the "External" platform addition behind the feature gate by generating separate CRD for TPNU clusters?

**A**: There seems to be a soft consensus that we do not need to gate these changes behind Tech Preview if it is not necessary.
Because operators are intended to react on the "External" platform the same as for the "None" one during the first phases,
gating these API extensions does not seem needed.

Related discussion: [1](https://github.com/openshift/enhancements/pull/1234#discussion_r968935259)

---
**Q**: Should we invest in preparing CI workflows that will perform UPI with the "None"/"External" platform types installation
on the AWS or GCP, or existing vSphere-based workflows would be enough?

**A**: Adding a job on one additional cloud platform to ensure that the "External" platform type works as intended looks reasonable now,
but we should mainly rely on in-repo functional tests on a per-component basis and avoid creating new e2e workflows as much as possible.

Related discussions: [1](https://github.com/openshift/enhancements/pull/1234#discussion_r968934725), [2](https://github.com/openshift/enhancements/pull/1234#discussion_r968936500).

---
**Q**: Do we need an API for the MAPI components similar to the proposed [CCM one](#api-extensions)
to allow users to choose how the MAPI components are deployed?

**A**: We do not see an absolute necessity to add such API knobs right now. For the moment combination of the
"External" platform and an enabled MAPI capability look like a sufficient signal.
By the nature of the [capabilities mechanism](https://github.com/openshift/enhancements/blob/master/enhancements/installer/component-selection.md#summary) (i.e., no MAO deployment, no MAPI CRDs),
if Machine API operator is running and detects the "External" platform, that is the signal to it that *someone* is going to run a machine controller.

Given that capabilities "API" is already added and in use, introducing additional knobs which will interfere with that
would mean more code changes and the necessity of establishing a mechanism for communicating with users (i.e., what should handle such API if MAPI is disabled by the capabilities mechanism).

However, if during upcoming phases, we will discover a need to add an API field specifically to help a user install their machine API controller,
we could update this EP, or a new enhancement document should be created to provide a proper exploration of interaction with capabilities and other operation modes.

Related discussions: [1](https://github.com/openshift/enhancements/pull/1234#issuecomment-1271896532)

### Test Plan

During the first stages, we must ensure that OpenShift [built-in operators whose behaviour depends on platform type](#affected-components) treat the "External" platform type the same way as "None".
To achieve this - existing infrastructure and mechanisms employed for exercising topologies with the "None" platform type might be used.

At the time of writing, the only workflow that tests the "None" platform is [upi-conf-vsphere-platform-none-ovn](https://steps.ci.openshift.org/workflow/upi-vsphere-platform-none-ovn).
Based on this workflow, a new one with platform type "External" with a respective set of jobs should be added to ensure that we do not disrupt the current OpenShift operation.

However, given that vSphere is the only platform where we're exercising clusters installation with platform "None" specified,
it would be beneficial to develop additional workflows using a provider with better capacity and performance (AWS or GCP, perhaps).

### Graduation Criteria

Given that `infrastructure.config.openshift.io` has already been released and has to be supported during the whole major release lifecycle,
this change will be GAed from the beginning.

However, components behaviour changes might be gated with the feature gates.
Specific graduation criteria for each component should be defined separately in respective enhancement documents.

#### Dev Preview -> Tech Preview

N/A

#### Tech Preview -> GA

N/A

#### Removing a deprecated feature

N/A

### Upgrade / Downgrade Strategy

This enhancement does not introduce any additional components, it just describes changes in "openshift/api".

PlatformType is expected to be set once during cluster installation and does not expect to be changed, so adding new platform type
should not affect upgrade/downgrade process across existing clusters.

### Version Skew Strategy

Since PlatformType is set as a day zero operation during cluster installation and is not expected to be changed during the cluster lifecycle,
version skew should not be the case there.

### Operational Aspects of API Extensions

It should be no different from other PlatformTypes. Scalability, API throughput and availability should not be affected.
In the first phases, it is expected to work the same way as for PlatformType "None".

#### Failure Modes

- OpenShift built-in operators would not recognize the new PlatformType and would go into Degraded or crashed.
  This will break new clusters installations with that new "External" platform type.

#### Support Procedures

During the first phases of this effort, support procedures should be no different from clusters with PlatformType set to "None".

## Implementation History

- [`External` platform API](https://github.com/openshift/api/pull/1301)

### Remove CloudControllerManagerSpec from the infrastructure resource

Due to priority changes for the Installer Flexibility effort,
the decision about temporarily removing `CloudControllerManagerSpec` from the `Infrastructure` resource was made.
OCP 4.13 will not contain the `CloudControllerManagerSpec` part.

- [PR for removing CloudControllerManagerSpec](https://github.com/openshift/api/pull/1409)

### Add CloudControllerManagerStatus into the infrastructure resource

- Remove CloudControllerManagerSpec from the ExternalPlatformSpec because
  this setting is not meant to be updated after cluster installation
- Introduce CloudControllerManagerStatus to hold the setting which will be
  defined at installation time

This change will target OCP 4.14.

- [related PR](https://github.com/openshift/api/pull/1434)

## Alternatives

### Status Quo

Leave things as is, i.e., encode every new cloud platform statically into "openshift/api" from the beginning of a technical enablement process.

### Proceed to use "None" Platform type

We could proceed to leverage PlatformType "None"; however, there are some difficulties that need to be worked around somehow, some examples:

* No defined mechanism to set `--cloud-provider=external` arg to kubelet/KCM/apiserver without merges and further revendoring of "openshift/api",
  at the moment, decision-making here is tied to the PlatformType.
  - This might be solved by creating additional documentation and mechanisms for propagating and controlling additional flags on kubelet/KCM/apiserver.
  - Possible approach proposed in this EP, but an alternative API / mechanism is possible.

* No way to extend machine-api and deliver a new provider without merges to "openshift/machine-api-operator" and "openshift/api" repos.
  - This might be solved by teaching the MachineAPI operator to deploy platform-independent components despite a platform type.

## Infrastructure Needed

- Additional CI workflows and set of CI jobs that exercise OpenShift installation with the new "External" platform.
