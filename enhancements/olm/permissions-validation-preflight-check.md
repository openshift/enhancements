---
title: permissions-validation-preflight-check
authors:
  - "@brett"
  - "@Tayler Geiger"
  - "@jkeister"
reviewers: # Include a comment about what domain expertise a reviewer is expected to bring and what area of the enhancement you expect them to focus on. For example: - "@networkguru, for networking aspects, please look at IP bootstrapping aspect"
  - "@ilias, for k8s auth interactions"
  - "@krzys, for k8s auth interactions"
approvers: # A single approver is preferred, the role of the approver is to raise important questions, help ensure the enhancement receives reviews from all applicable areas/SMEs, and determine when consensus is achieved such that the EP can move forward to implementation.  Having multiple approvers makes it difficult to determine who is responsible for the actual approval.
  - "@joelanford"
api-approvers: # In case of new or modified APIs or API extensions (CRDs, aggregated apiservers, webhooks, finalizers). If there is no API change, use "None"
  - "None"
creation-date: 2025-03-17
last-updated: 2025-04-04
tracking-link: # link to the tracking ticket (for example: Jira Feature or Epic ticket) that corresponds to this enhancement
  - https://issues.redhat.com/browse/OPRUN-3781
---

<!-- To get started with this template:
1. **Pick a domain.** Find the appropriate domain to discuss your enhancement.
1. **Make a copy of this template.** Copy this template into the directory for
   the domain.
1. **Fill out the metadata at the top.** The embedded YAML document is
   checked by the linter.
1. **Fill out the "overview" sections.** This includes the Summary and
   Motivation sections. These should be easy and explain why the community
   should desire this enhancement.
1. **Create a PR.** Assign it to folks with expertise in that domain to help
   sponsor the process.
1. **Merge after reaching consensus.** Merge when there is consensus
   that the design is complete and all reviewer questions have been
   answered so that work can begin.  Come back and update the document
   if important details (API field names, workflow, etc.) change
   during code review.
1. **Keep all required headers.** If a section does not apply to an
   enhancement, explain why but do not remove the section. This part
   of the process is enforced by the linter CI job.

See ../README.md for background behind these instructions.

Start by filling out the header with the metadata for this enhancement. -->

# Permissions Validation Preflight Check

<!-- 
This is the title of the enhancement. Keep it simple and descriptive. A good
title can help communicate what the enhancement is and should be considered as
part of any review.

The YAML `title` should be lowercased and spaces/punctuation should be
replaced with `-`.

The `Metadata` section above is intended to support the creation of tooling
around the enhancement process. -->

## Summary

Validating of ServiceAccount permissions before attempting to install/manage extension content.  

Instead of hoping users are able to patch together a functioning ServiceAccount, OLMv1 should perform a preflight check that tells users about any required permissions that are missing from the provided ServiceAccount. The output of this preflight check could then be utilized in future improvements to the user experience, such as:
- Surfacing all the required permissions through a CLI tool or in GUIs
- Automating the creation of a suggested minimally functional provided ServiceAccount


## Motivation

Installing a ClusterExtension in OLMv1 requires users to provide a ServiceAccount for installing and managing that extension. Determining the required permissions for that ServiceAccount is currently a non-trivial, repetitive, lengthy process.

### User Stories

- As a cluster user, I wish to be able to easily assess sufficiency of the required RBAC necessary for a `ServiceAccount` associated with a `ClusterExtension` installation attempt.
- As a cluster user, I wish to be able to clearly assess permissions gaps for such an installation attempt so I may easily adjust them.
- As a gitops-style actor on the cluster, I wish to be able to interpret permissions gaps programmatically so that I can handle remediation/alerts appropriately.


### Goals

Introduce a preflight check that evaluates the permissions granted to the provided ServiceAccount against the permissions required by the ClusterExtension bundle. If any permissions are missing, the system will:
- Provide a detailed, user-friendly error message listing missing permissions.
- Prevent installation or upgrades from proceeding until the issue is resolved.
- The pre-flight check implemented as part of this work should include more than just surfacing the RBAC in the bundle. Generally, it should surface any permissions that are needed to stamp out all resources in the bundle on the cluster.  It should include any additional rules requirements for a service account that OLM might exert.
- Any bundle introspection is about what needs to be applied to the cluster by OLM. OLM doesn't, and shouldn't, care about what permissions the cluster extension needs to operate as expected in the cluster. 


### Non-Goals

- Replacing or superceding the cluster's authorization chain.
- Evaluating access to resources beyond what is necessary to install the ClusterExtension based on bundle contents. OLM will only have knowledge of Kubernetes and applying Kubernetes resources so we won't be able to verify something like access to an external secret store as part of an extension's installation. 
- Optionality of the preflight check.  
(**Note to auth reviewers:** it is (un)reasonable to have a mandatory check that assumes RBAC authorization for ClusterExtension service accounts?)

## Proposal

The preflight check will extend the existing preflight interface in the operator controller.   
Key components:
1. *Permission Analysis:*  
   Build a local cache of all RBAC and perform a local best-effort evaluation against required permissions declared in the ClusterExtension’s bundle.
2. *Error Reporting:*  
   Output detailed messages describing missing permissions in a user-friendly format.
3. *Integration:*  
   Hook into the ClusterExtension reconciliation workflow, ensuring preflight checks run before installation or upgrades.


Validation Workflow:
1. Extract RBAC information from the ClusterExtension bundle.
2. Use the preflight permissions validator to compare declared permissions against those granted to the ServiceAccount.
3. If discrepancies exist, output actionable error messages and halt the process.

<!-- 
This section should explain what the proposal actually is. Enumerate
*all* of the proposed changes at a *high level*, including all of the
components that need to be modified and how they will be
different. Include the reason for each choice in the design and
implementation that is proposed here.

To keep this section succinct, document the details like API field
changes, new images, and other implementation details in the
**Implementation Details** section and record the reasons for not
choosing alternatives in the **Alternatives** section at the end of
the document. -->

### Workflow Description

Our approach can be broken down into two parts.

Part 1: Validating permissions required for installing the Cluster Extension:
- Perform a client-only template rendering.
- Create a permissions validation preflight check which checks the local-only generated manifest(s) for all of the following permissions:
   - Necessary permissions checked by the Helm dry-run.
      - Currently, a Helm dry-run is performed inside a function called getReleaseState() which is called inside helm.Apply().
      - Errors from this Helm dry-run will be surfaced as a group and in a digestible manner.
         - **Parsing Multi-line Errors:** If the returned error message contains multiple permission issues (e.g., denied GET access on multiple resources), our preflight check will split these into structured components.
         - **Returning a List of Errors:** Instead of a single concatenated error string, the function will return an []error, where each element corresponds to a specific missing permission.
            - For the first iteration, this error array will be printed as a simple string in a status condition Message field
            - Keeping a structured array of the missing permissions will be utilized in future improvements to the provided SA workflow, i.e. connecting with a CLI tool and/or providing a structured output of all the missing permissions
       - Example of the existing Helm dry-run error output (via some debug prints added to show what is happening):

```log
21:51:49.390065 helm.go:170] "No existing release found, performing dry-run of Helm install" clusterExtension="argocd" namespace="argocd"

21:51:49.390116 helm.go:174] "Helm Install Dry-Run configured" clusterExtension="argocd" namespace="argocd"

21:51:49.856756 helm.go:179] "Error during Helm install dry-run" err="Unable to continue with install: could not get information about the resource ConfigMap \"argocd-operator-manager-config\" in namespace \"argocd\": configmaps \"argocd-operator-manager-config\" is forbidden: User \"system:serviceaccount:argocd:argocd-installer\" cannot get resource \"configmaps\" in API group \"\" in the namespace \
"argocd\"" clusterExtension="argocd" namespace="argocd"
```

   - For specific resources like (Cluster)Role(Binding)s we will need either:
      - All the same permissions as outlined in the (Cluster)Role that is being created OR referenced by the (Cluster)Role(Binding).
      - The ServiceAccount is granted the escalate and bind verbs. The escalate verb allows assigning permissions that exceed the ServiceAccount’s own privileges, and the bind verb enables the ServiceAccount to link roles or cluster roles to users, groups, or service accounts. These broader verbs remove the need to explicitly define and verify every individual permission required for the (Cluster)Role(Binding).

Part 2: Validating permissions required for runtime functionality:
   - A second check will need to validate any runtime permissions needed on the ClusterExtension ServiceAccount(s). Whereas, part 1 is checking “do we have GET permissions on necessary resources to do a helm dry-run,” this second part is checking do we have the permissions to do what OLM needs to do with what was returned from the helm dry-run.
   - How?: Utilize the SelfSubjectRulesReview Kubernetes API for asking the Kubernetes API server if $user can do $verb for $resource with the provided installer ServiceAccount against the list of permissions we require.
      - Obtain the resource list by unpacking the bundle and examining the CSV, for each resource. All checks will be performed against the final manifest of what will be applied after all rendering/templating has occurred.
      - SelfSubjectRulesReview
         - RulesReview returns all your permissions but it checks against a cache and therefore might not be representative of current state
         - Faster than alternative APIs, fewer requests, but potentially stale
      - There is prior art for using SelfSubjectRulesReview from when we were contributing to carvel. PoC PR using that library: https://github.com/operator-framework/operator-controller/pull/1282
         - We should use the work in this PoC as inspiration and as a usage model for the new preflight check.

OLMv1’s design principles stipulate that any preflight checks can be manually disabled. However, there does not appear to be much, if any, benefit to allowing this new check to be disabled.
- If a user were to disable this permissions check, then they would simply run into a permissions error the first time they attempt to apply anything to the cluster.
- We considered two API changes, to the spec and status sections, but eventually ruled them out:
   - Because we are not allowing this check to be disabled we do not need to include mention of it in the Cluster Extension spec.
   - Rather than make API changes under the status section as a means of reporting results, we will, for now, simply pass along a message in the form of pasteable RBAC corrections. These corrections will appear in the form of a string message on the Progressing status Condition. 
   - While the amount of feedback about needed permissions may be too extensive for a message field, we will take this route until we have more experience with how this feedback will be used in practice. Specifically, we will likely add two (2) more RFC under the same Brief as this RFC which will propose:
      - How to structure and pass the permission failure information, perhaps in a ConfigMap;
      - How to implement a CLI tool to read the structured data and make required permission fixes based on them. 
      - The message will list, with the same type of structure as rules for a (Cluster)Role resources. This output is geared towards making this output nearly copy/pasteable to a fixed ServiceAccount. Very large output may be truncated due to etcd limits: imagine a Cluster Extension with many resources and Service Account has no permissions. 
- Introduce a Feature Gate. We propose introducing a new downstream feature gate named `NewOLMPreflightPermissionChecks` under the `TechPreviewNoUpgrade` feature set. This gate will ensure that preflight checks for required permissions can be implemented and tested without impacting stable OLM functionality. Origin CI tests will be developed as part of this process to establish a solid foundation for both the gated and GA versions of the feature.



### API Extensions

N/A

### Topology Considerations

N/A

#### Hypershift / Hosted Control Planes

This feature does not introduce any new concerns/requirements w.r.t. hypershift.

#### Standalone Clusters

This feature does not introduce any new concerns/requirements w.r.t standalone clusters.

#### Single-node Deployments or MicroShift

Single-node deployments should expect this feature to generate slightly more network traffic to the apiserver in order to perform the access review.  CPU consumption would be slightly increased during the assessment period.

OLMv1 is not supported in the MicroShift platform.

### Implementation Details/Notes/Constraints

<!-- What are some important details that didn't come across above in the
**Proposal**? Go in to as much detail as necessary here. This might be
a good place to talk about core concepts and how they relate. While it is useful
to go into the details of the code changes required, it is not necessary to show
how the code will be rewritten in the enhancement. -->

### Risks and Mitigations

<!-- What are the risks of this proposal and how do we mitigate. Think broadly. For
example, consider both security and how this will impact the larger OKD
ecosystem.

How will security be reviewed and by whom?

How will UX be reviewed and by whom?

Consider including folks that also work outside your immediate sub-project. -->

### Drawbacks

<!-- The idea is to find the best form of an argument why this enhancement should
_not_ be implemented.

What trade-offs (technical/efficiency cost, user experience, flexibility,
supportability, etc) must be made in order to implement this? What are the reasons
we might not want to undertake this proposal, and how do we overcome them?

Does this proposal implement a behavior that's new/unique/novel? Is it poorly
aligned with existing user expectations?  Will it be a significant maintenance
burden?  Is it likely to be superceded by something else in the near future? -->

## Open Questions [optional]

<!-- This is where to call out areas of the design that require closure before deciding
to implement the design.  For instance,
 > 1. This requires exposing previously private resources which contain sensitive
  information.  Can we do this? -->

## Test Plan

**Note:** *Section not required until targeted at a release.*

<!-- Consider the following in developing a test plan for this enhancement:
- Will there be e2e and integration tests, in addition to unit tests?
- How will it be tested in isolation vs with other components?
- What additional testing is necessary to support managed OpenShift service-based offerings?

No need to outline all of the test cases, just the general strategy. Anything
that would count as tricky in the implementation and anything particularly
challenging to test should be called out.

All code is expected to have adequate tests (eventually with coverage
expectations). -->

## Graduation Criteria

**Note:** *Section not required until targeted at a release.*

<!-- Define graduation milestones.

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
end to end tests.** -->

### Removing a deprecated feature

<!-- - Announce deprecation and support policy of the existing feature
- Deprecate the feature -->

## Upgrade / Downgrade Strategy

<!-- If applicable, how will the component be upgraded and downgraded? Make sure this
is in the test plan.

Consider the following in developing an upgrade/downgrade strategy for this
enhancement:
- What changes (in invocations, configurations, API use, etc.) is an existing
  cluster required to make on upgrade in order to keep previous behavior?
- What changes (in invocations, configurations, API use, etc.) is an existing
  cluster required to make on upgrade in order to make use of the enhancement?

Upgrade expectations:
- Each component should remain available for user requests and
  workloads during upgrades. Ensure the components leverage best practices in handling [voluntary
  disruption](https://kubernetes.io/docs/concepts/workloads/pods/disruptions/). Any exception to
  this should be identified and discussed here.
- Micro version upgrades - users should be able to skip forward versions within a
  minor release stream without being required to pass through intermediate
  versions - i.e. `x.y.N->x.y.N+2` should work without requiring `x.y.N->x.y.N+1`
  as an intermediate step.
- Minor version upgrades - you only need to support `x.N->x.N+1` upgrade
  steps. So, for example, it is acceptable to require a user running 4.3 to
  upgrade to 4.5 with a `4.3->4.4` step followed by a `4.4->4.5` step.
- While an upgrade is in progress, new component versions should
  continue to operate correctly in concert with older component
  versions (aka "version skew"). For example, if a node is down, and
  an operator is rolling out a daemonset, the old and new daemonset
  pods must continue to work correctly even while the cluster remains
  in this partially upgraded state for some time.

Downgrade expectations:
- If an `N->N+1` upgrade fails mid-way through, or if the `N+1` cluster is
  misbehaving, it should be possible for the user to rollback to `N`. It is
  acceptable to require some documented manual steps in order to fully restore
  the downgraded cluster to its previous state. Examples of acceptable steps
  include:
  - Deleting any CVO-managed resources added by the new version. The
    CVO does not currently delete resources that no longer exist in
    the target version. -->

## Version Skew Strategy

<!-- How will the component handle version skew with other components?
What are the guarantees? Make sure this is in the test plan.

Consider the following in developing a version skew strategy for this
enhancement:
- During an upgrade, we will always have skew among components, how will this impact your work?
- Does this enhancement involve coordinating behavior in the control plane and
  in the kubelet? How does an n-2 kubelet without this feature available behave
  when this feature is used?
- Will any other components on the node change? For example, changes to CSI, CRI
  or CNI may require updating that component before the kubelet. -->

## Operational Aspects of API Extensions

<!-- Describe the impact of API extensions (mentioned in the proposal section, i.e. CRDs,
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
  and add them as reviewers to this enhancement. -->

## Support Procedures

<!-- Describe how to
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
    objects when another namespace with the same name is created. -->

## Alternatives

<!-- Similar to the `Drawbacks` section the `Alternatives` section is used
to highlight and record other possible approaches to delivering the
value proposed by an enhancement, including especially information
about why the alternative was not selected. -->

## Infrastructure Needed [optional]

<!-- Use this section if you need things from the project. Examples include a new
subproject, repos requested, github details, and/or testing infrastructure. -->
