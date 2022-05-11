---
title: pod-security-admission-autolabeling
authors:
  - "@stlaz"
reviewers:
  - "@deads2k"
  - "@s-urbaniak"
  - "@soltysh"
approvers:
  - "@sttts"
  - "@mfojtik"
api-approvers: # in case of new or modified APIs or API extensions (CRDs, aggregated apiservers, webhooks, finalizers)
  - "@sttts"
creation-date: 2022-02-04
last-updated: 2022-02-04
tracking-link: # link to the tracking ticket (for example: Jira Feature or Epic ticket) that corresponds to this enhancement
  - https://issues.redhat.com/browse/AUTH-133
see-also:
  - "/enhancements/authentication/pod-security-admission.md"
replaces: []
superseded-by: []
---

# Pod Security Admission Autolabeling

## Summary

This enhancement expands the "PodSecurity admission in OpenShift". It introduces
an opt-in mechanism that allows users to to keep their workloads running when
Pod Security Admission plugin gets turned on.

## Motivation

We want to adhere with the upstream pod security standards for our workloads but
we also want to provide our users access to the Security Context Constraints
(hereinafter SCCs) API that they are already used to. However, each of these
admission plugins works a bit differently and so there must be a middle-man
that synchronizes the privileges SCCs provide into privileges that Pod Security
admission (hereinafter PSa) understands.

### Goals

1. OpenShift clusters can run with the restricted Pod Security admission level by default
without breaking pod controller workloads
2. Privileged users can opt-in/opt-out a namespace to and from the Pod Security admission

### Non-Goals

1. Make sure that bare pods (pods w/o pod controllers) continue working
    - Both the admission systems are triggered on pod creation, possibly preventing the
      API server from persisting the pod on admission denial. It is therefore impossible
      to take the Pod as an input without touching code of both the admission controllers.
      Such a code change would also very likely end up being rather unreasonable.
2. Add the ability to tune the cluster-wide PSa configuration

## Proposal

### User Stories

1. I would like my workloads to be admitted based on the current pod security standards
   with minimal effort or no effort at all

### Pod Security Admission Label Synchronization

This section explains how pod security admission works and later elaborates on how to use
the pod security labeling in order to synchronize SCC permissions into pod security admission
restriction levels.

The SCC permissions to PSa levels transformation gets explained at the end of this section.

#### Pod Security Admission Introduction

Pod Security admission validates pods' security according to the upstream
[pod security standards](https://kubernetes.io/docs/concepts/security/pod-security-standards/)
and distinguishes three different security levels:
- privileged - most privileged mode, anything is allowed
- baseline   -  minimally restrictive policy which prevents known privilege escalations
- restricted - heavily restricted policy, following current Pod hardening best practices

By default, there is a cluster-global configuration which enforces the configured policies
on pods and known workloads.

It is possible to override the cluster-global policy enforce configuration on a per-namespace
basis by using the `pod-security.kubernetes.io/enforce` label on given namespaces. It is also
possible to exempt certain users, namespace and runtime classes from the admission completely.

#### PSa Label Synchronization Controller

This section describes a controller that is going to be a part of the cluster-policy-controller.

In order to keep SCCs working alongside restricted cluster-wide PSa level, there must be
a mechanism that would synchronize the PSa policy level for a namespace that workload runs
in, so that the workload is allowed by both SCC and PSa admissions. For that purpose, a PSa
Label Synchronization Controller (hereinafter the Controller) shall be built.

The Controller watches changes of:
- **namespaces**
  - manual changes to the interesting labels of the namespaces that were
    opted-in to label synchronization should be reverted
- **roles** and **rolebindings** (and their **cluster variants**)
  - to be able to properly assess SA capabilities in controlled namespaces
- **SCCs**
  - to properly assess restrictive PSa levels per affected namespaces
  - to be able to assess SA capabilities in controlled namespace along with the
    information retrieved from roles and rolebindings (using the SCC `users` and
    `groups` fields)

The generic control loop of the Controller performs the following actions:
1. list all controlled (opted-in by a label) namespaces
2. for each namespace:
    1. create SCC <-> []SA associations for all SAs present in the namespace
    1. order observed SCCs most-privileged first (according to PSa privilege levels
       they map to)
    1. evaluate the namespace's PSa privilege level based on the most privileged
       SCC that is allowed to be used by its SA
    1. modify the namespace's PSa labels based on the enforce level observed in the
       previous step

There is another control loop reacting only to changes of namespaces which performs
all the steps in 2.

#### Matching SCCs to Service Accounts

The Controller needs to be able to map SCCs to a given SA in order to be able to
later use this information to determine the restrictive level for a given SA.

To map SCCs to SAs, the Controller lists all roles and all SCCs, and locally attempts
to match the SCCs to the roles based on their `rules`. It does that using the [upstream
RBAC rules evaluation code](https://github.com/kubernetes/kubernetes/blob/c6153a93d0528a3dc6be9dedc2602f140081688d/plugin/pkg/auth/authorizer/rbac/rbac.go#L168-L193).

The local evaluation was chosen over sending SARs to the API server in order to
prevent periodic surge of requests in clusters with a larger amount of
namespaces + roles + SCCs.

For mapping the roles to SAs, an indexing should be introduced on top of the
rolebinding informer cache. The indexing will index rolebindings to SAs,
SA groups, and the `system:authenticated` group.

To get SCCs that match a given SA, the process is as follows:
1. list all SCCs
2. determine SA access to SCCs based on SCC `users` and `groups` fields
3. get all roles for an SA and its groups from an indexed informer cache
4. add SCCs for the roles found in 3. to the list of SCCs found in 2.

#### Label Synchronization Opting-in

The Controller described in [PSa Label Synchronization Controller](#psa-label-synchronization-controller)
works on an opt-in basis, meaning that users need to specifically sign themselves up
if they wanted to stick with just SCCs.

Namespaces that specifically want to have their PSa labels synchronized
should be labelled by the `security.openshift.io/scc.podSecurityLabelSync`
label with the label value set to `“true”`, on the other hand, to specifically
request no PSa label synchronization, the namespace should set the label's
value to "false".

By default, namespaces without the synchronization label will be still
considered for label synchronization. These namespaces are considered
"no-opinion" and the label synchronization behavior for these may change
in any future release.

Having the "no-opinion" path helps keeping the older workloads working
during upgrades. It also gives us space to decide whether we want the
label synchronization be done by default or whether the users should
specifically opt into it in the future.

As a consequence of the opt-in/opt-out being dependent on `Namespace`
labelling, only the privileged users can determine what kind of
admission mechanism may run on namespaces, non-privileged users
depend on the platform defaults.

#### SCC to PSa Level Transformation

The automatic PSa label synchronization requires us to map SCCs directly to a PSa
level.

In order to be able to map SCCs to PSa level automatically, it is necessary to
introspect each field of such an SCC.

SCC fields each tell the SCC authorizer how to either:
- default a value of a given pod's `securityContext` field if unspecified
- restrict the domain of values for a given pod's `securityContext` field

PSa is based on a number of checks, each of which validates a certain `securityContext`
field per given PSa restrictiveness level.

As per the above, it is possible to introspect the SCC type fields and based on
possible values and based on introspecting the PSa check functions for the given
pod's `securityContext` field that's affected by the SCC field values, it is possible
to create mappings to different PSa levels.

Given the need for human interaction with PSa check functions when creating the
SCC-\>PSa mapping, and the complexity of this task, each OpenShift version should
always ever consider the "latest" PSa checks.

#### Deriving PSa Level from SCC fields

The SCC fields perform different tasks for fields of a security context:

1. restrict the values to a certain value domain
2. modify the field value in case the field is unset
3. combination of the above
4. restrict the values of an already allowed set (e.g. `AllowedFlexVolumes` only
   further restricts flex volumes allowed by `Volumes` but upstream makes no
   difference there)

For the conversion between SCC and PSa levels, we are only interested in fields
that do 1. and 3. The conversion itself needs to make sure that the values allowed
by SCCs are a subset of the values allowed by the given PSa level in order for the
SCC to match the PSa level.

**Caveats and Gotchas Found During SCC Mapping Investigation**

1. Currently, there exists no default SCC supplied with the platform that would
match any PSa level more restricted than `baseline`
    - the `restricted` (and any dependent) SCC needs to be modified to match the
      pod security standards of today:
        - `RequiredDropCapabilities` should now contain just one value - `ALL`
        - `AllowPrivilegeEscalation` should now be `false`
        - `AllowedCapabilities` should include `NET_BIND_SERVICE` which allows
          the application to use domain privileged ports (numbers lower than 1024)
        - `SeccompProfiles` should equal `runtime/default` which only explicitly
          defaults this value for pods but it is already implicitly used
2. The SCC to PSa conversion depends on a namespace for the conversion
    - the `MustRunAs` strategy of `RunAsUser` allows setting an empty range and
      its evaluation for a given pod retrieves the range from the pod's namespace
      annotations
3. The upstream checks can be updated at any time and there is nothing that would
   allow a simple machine conversion between SCC and PSa levels, this is all human driven
4. PSa is designed in a way that allows improving security with time if such a need
   arises
    - this could mean that our SCCs might slip to less restrictive PSa levels in
      time
    - we will need to have a process to tighten SCC permissions that would match
      the PSa capabilities in this regard

From the above points, we can safely skip 2. as that is just an interesting observation.
We are evaluating the PSa levels on namespace level anyway when we are checking the
SCC-related permissions for each of the SA, which are namespaced by nature.

The above changes required **to make 1. work** might cause workloads to break
if applied directly to the `restricted` SCC. It is impossible to tell which
workload would break by this step because most of the platform users very likely just
went with the platform defaults and did not perform further steps in order to
tighten their workloads to their actual needs. To solve this issue, new
`*-v2` (e.g. `restricted-v2`, `nonroot-v2`) SCCs with tighter security policies
should be introduced for the `restricted` SCC and SCCs derived from it. The `restricted`
SCC will also drop `system:authenticated` from its `Groups` field in favour of adding
it to the `restricted-v2` SCC.  This effectively causes older clusters to keep the looser
permissions of `restricted` SCC for their workloads but tighten up the policies
for workloads of newly created clusters.

In case the changes above still break any of user workloads in upgraded clusters
(`restricted-v2` has better restrictive score during evaluation and so it's chosen
first for field defaulting), the users will need to fix their workloads' `securityContext`
accordingly but they should still be able to since they would still have access to
the `restricted` SCC. The **documentation should include information** how
to fix such a workload, and how to remove broad user audience access to the legacy
`restricted` SCC.

**For 3**, a process needs to be introduce that allows to simply check the changes to
the `k8s.io/pod-security-admission/policy` folder during k8s.io rebases. The
repository containing the SCC conversion should include the commit hash of the
latest checked version of the `k8s.io/pod-security-admission/policy` folder that
the current mapping corresponds to and developers will need to manually check that
no significant changes happened to the folder in between the two rebases that would
cause updates to checks. If there were, the conversion needs to be updated. In both
cases, the commit hash should be updated to match the latest checked version of the
folder.

The **process for 4.** should be drawn in a follow-up enhancement.

### API Extensions

A new `Namespace` label is being introduced - `security.openshift.io/scc.podSecurityLabelSync`.
When set to value `"true"`, the such-labeled `Namespace` gets picked by the
PSa label synchronization controller and this `Namespace` and its workloads
will be reconciled according to the [PSa Label Synchronization Controller](#psa-label-synchronization-controller)
section.

### Workflow Description

Described in the sections above.

### Risks and Mitigations

#### PSa Label Synchronization Risks
Turning on the PSa to restricted level enforcing even with the namespace label
synchronization described in this enforcement brings the risk of breaking direct
pod application for pods that don't rely on SA permissions, i.e. pods created
directly by users with SCC permissions higher than the ones of the SA for the
given pod.

The problem above could be mitigated by providing the SA running the pod the same
SCC-related permission as the users has, or at least permission to `use` the least
privileged SCC required to run the pod.

It is also likely that pod controllers might happen to fail pod creation before
the labels are properly synchronized on their namespace. There is no way to
mitigate that, the pods will eventually succeed to be created if the given
service account running them has the correct permissions.

#### Restricted SCC Change Risks

The `restricted` SCC and its variations (e.g. `nonroot`) will need a duplicit `<name>-v2`
variant that should match the updated pod security standards (see
[Deriving PSa Level from SCC fields](#deriving-psa-level-from-scc-fields) for
details of the proposed changes).

The `restricted` SCC changes may cause some workloads to break. Such failures
should be rather rare as modern restricted workloads should not be susceptible to
the *setuid* bits and should tolerate dropped capabilities. Nevertheless, the
documentation should state how to fix such workloads by modifying the `dropCapabilities`
to match the previous `restricted` SCC.

### Drawbacks

Everything was already described elsewhere.

## Design Details

### Open Questions

1. Should we have a way to completely disable SCC admission per namespace
   to allow only PSa?

### Test Plan

The tests shall include these scenarios:
1. a namespace labelled for label synchronization **gets** a more privileged
   label when it contains a workload controller whose SA **can** run the
   pods with matching SCC
2. a namespace labelled for label synchronization **does not get** a more privileged
   label when it contains a workload controller whose SA **cannot** run the
   pods with matching SCC
3. a namespace not labeled for label synchronization does not get its PSa
   labels synchronized

### Graduation Criteria

Irrelevant.

#### Dev Preview -> Tech Preview

Irrelevant.

#### Tech Preview -> GA

Irrelevant.

#### Removing a deprecated feature

Irrelevant.

### Upgrade / Downgrade Strategy

The ugprade strategy is discussed in the [Label Synchronization Opting-in](#label-synchronization-opting-in)
section.

### Version Skew Strategy

Irrelevant.

### Operational Aspects of API Extensions

One can derive all of this from the description in the [Proposal](#proposal) section.

#### Failure Modes

N/A

#### Support Procedures [TBD]

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

## Implementation History

Major milestones in the life cycle of a proposal should be tracked in `Implementation
History`.

## Alternatives

Empty.
