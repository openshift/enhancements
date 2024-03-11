---
title: prevent-user-workloads-from-being-scheduled-on-control-plane-nodes
authors:
  - knelasevero
  - ingvagabund 
  - flavianmissi
reviewers: # Include a comment about what domain expertise a reviewer is expected to bring and what area of the enhancement you expect them to focus on. For example: - "@networkguru, for networking aspects, please look at IP bootstrapping aspect"
  - TBD
approvers: # A single approver is preferred, the role of the approver is to raise important questions, help ensure the enhancement receives reviews from all applicable areas/SMEs, and determine when consensus is achieved such that the EP can move forward to implementation.  Having multiple approvers makes it difficult to determine who is responsible for the actual approval.
  - TBD
api-approvers: # In case of new or modified APIs or API extensions (CRDs, aggregated apiservers, webhooks, finalizers). If there is no API change, use "None"
  - TBD
creation-date: yyyy-mm-dd
last-updated: yyyy-mm-dd
tracking-link: # link to the tracking ticket (for example: Jira Feature or Epic ticket) that corresponds to this enhancement
  - https://issues.redhat.com/browse/OCPSTRAT-790
  - https://issues.redhat.com/browse/WRKLDS-1015
  - https://issues.redhat.com/browse/WRKLDS-1060
see-also:
  - .
---

To get started with this template:
1. **Pick a domain.** Find the appropriate domain to discuss your enhancement.
1. **Make a copy of this template.** Copy this template into the directory for
   the domain.
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

Start by filling out the header with the metadata for this enhancement.

# Prevent user workloads from being scheduled on control plane nodes

## Summary

Starting OCP 4.1 Kubernetes Scheduler Operator’s `config.openshift.io/v1/scheduler` type was extended with `.spec.mastersSchedulable` field [[1]](#ref-1) set to `false` by default. Its purpose is to protect control plane nodes from receiving a user workload. When the field is set to `false` each control plane node is tainted with `node-role.kubernetes.io/master:NoSchedule`. If set to `true` the taint is removed from each control plane node. No user workload is expected to tolerate the taint. Unfortunately, there’s currently no protection from users (with pod’s create/update RBAC permissions) explicitly tolerating `node-role.kubernetes.io/master:NoSchedule` taint or setting `.spec.nodeName` field directly (thus by-passing the kube-scheduler).

<a id="ref-1"></a>[1] https://docs.openshift.com/container-platform/latest/nodes/nodes/nodes-nodes-managing.html#nodes-nodes-working-master-schedulable_nodes-nodes-managing

## Motivation

<!-- This section is for explicitly listing the motivation, goals and non-goals of
this proposal. Describe why the change is important and the benefits to users. -->

Allowing arbitrary users to bypass the `spec.mastersSchedulable` field and schedule their workloads on control plane nodes poses a security risk of scheduling too many pods on each control-plane node while oversaturating resources and increasing the chance of e.g. memory pressure. Resulting in e.g. control plane pods getting OOM killed. Even when all the control plane pods have (are expected to have) the highest priority classes and thus pre-emption is not expected it’s safer to have abundance of resources than deficit to accommodate for various disruptions.

Also, secondary schedulers might not take taints and tolerations into account when selecting a node. Thus, another layer of protection is needed to avoid prohibited node assignments for components that are unaware of the `mastersSchedulable` functionality.

### User Stories

* As an administrator, I want to restrict non control plane workloads from being scheduled on control plane nodes (even those using `.spec.nodeName` in their pods), so that the control plane components are not at risk of running out of resources.
* As an administrator, I want only workloads created by certain service accounts or users to be allowed to schedule pods on nodes with a given label (i.e `node-role.kubernetes.io/control-plane`)
<!-- * As an administrator, I want to restrict regular users from scheduling their workloads on GPU-enabled nodes, so that I can enforce protection of costly resources from running regular workloads 

TODO: get feedback on namespace allow - in a call we decided internally SA control is suficient but that might get some other discussions going with broader group

-->
* As a cluster administrator, I want to have a clear and manageable way to update and maintain the list of service accounts and users allowed to schedule workloads on control plane nodes, so that I can efficiently manage permissions as the cluster evolves or as new teams/services are onboarded.
* As an OpenShift developer, I want to understand the impact of the new scheduling restrictions on existing and future workloads, so that I can design applications that comply with cluster policies and make informed decisions about resource requests and deployment strategies. <!-- this should be a given with the list of SAs but something to have explicitly here if we think of something else -->
* As an end user, I want to receive informative feedback when my workloads are rejected due to the new scheduling policies, so that I can make the necessary adjustments without needing extensive support from cluster administrators. <!-- firing logs, events, etc -->
* As a security professional, I want to ensure that the new scheduling policies are enforceable and auditable, so that I can verify compliance with internal and external regulations regarding resource access and control plane integrity. <!-- from my time in consulting, should we have an audit log of some sort? -->
* As an administrator, I want the ability to temporarily override scheduling restrictions for emergency or maintenance tasks without compromising the overall security posture of the cluster, ensuring that critical operations can be performed when necessary.

### Goals

<!-- *Summarize the specific goals of the proposal. How will we know that
this has succeeded?  A good goal describes something a user wants from
their perspective, and does not include the implementation details
from the proposal.* -->

- Enhance Cluster Security and Stability: Prevent non-control plane workloads from being scheduled on control plane nodes to avoid resource competition and potential out-of-memory (OOM) issues that could affect critical cluster operations.

- Flexible and Manageable Workload Scheduling: Enable cluster administrators to specify and manage exceptions based on service accounts, allowing certain workloads to be scheduled on control plane nodes when necessary for operational requirements.

<!-- - Protect Specialized Resources: Restrict scheduling of regular user workloads on nodes with specialized resources (e.g., GPU-enabled nodes) to ensure these costly resources are reserved for appropriate workloads. -->

<!-- Going to the alternative route of giving more flexibility from the start, but as discussed in some meetings, might not be the route we should go initially -->

- Improve Feedback Mechanisms: Provide clear and informative feedback to users when their workloads are rejected due to scheduling policies, enabling them to adjust their deployment strategies without extensive administrative intervention.

- Emergency Override Capability: Allow administrators to temporarily override scheduling restrictions for emergency or maintenance tasks, ensuring critical operations can be performed when necessary without compromising the cluster's overall security posture.

### Non-Goals

<!-- What is out of scope for this proposal? Listing non-goals helps to
focus discussion and make progress. Highlight anything that is being
deferred to a later phase of implementation that may call for its own
enhancement. -->

- Granular Pod-Level Scheduling Controls: The enhancement will not introduce fine-grained controls for individual pod scheduling decisions beyond the existing Kubernetes mechanisms (e.g., taints, tolerations, and affinity rules).

- Automated User or Service Account Management: Automatically managing or updating the list of service accounts authorized to schedule workloads on control plane nodes is out of scope. This process remains a manual administrative responsibility. <!-- this is the general decision, right? -->

- Real-Time Resource Allocation Optimization: The proposal does not aim to dynamically optimize resource allocation or scheduling decisions based on real-time cluster utilization or performance metrics.

- Protection of other type of nodes: Only control plane nodes are taken into account. Protection of other type of nodes such as gpu nodes or edge nodes connected through mobile network is not included.

## Proposal

<!-- This is where we get down to the nitty gritty of what the proposal
actually is. Describe clearly what will be changed, including all of
the components that need to be modified and how they will be
different. Include the reason for each choice in the design and
implementation that is proposed here, and expand on reasons for not
choosing alternatives in the Alternatives section at the end of the
document. -->

### Overview
This enhancement proposes the implementation of a more robust and flexible mechanism for enforcing scheduling policies on OpenShift clusters. It will focus on preventing user workloads from being scheduled on control plane nodes and allowing for exceptions based on administrative configurations.

### Implementation Strategies

- Admission Controller Enhancements: Develop or extend an existing admission controller/plugin to enforce the new scheduling policies. This controller will reject pods that attempt to schedule on protected nodes unless they meet the criteria defined by administrators (e.g. coming from selected service accounts).

- Configurable Policy Management: Introduce a new configuration resource or extend an existing one within the OpenShift API to allow administrators to define and manage scheduling policies, including protected node selectors, service account allowlists, and other exemptions.

- User Feedback Mechanisms: Enhance the admission controller to provide meaningful logs and/or events when rejecting pod scheduling attempts, helping users understand policy violations and encouraging self-resolution of deployment issues.

- Emergency Override Mechanism: Implement a mechanism for administrators to temporarily bypass scheduling restrictions, ensuring that critical maintenance and emergency operations can be executed without delay. <!-- even if it is just enabling/disabling the plugin -->

- Documentation and User Guides: Provide documentation and best practice guides to assist administrators in configuring scheduling policies and to help users understand how to comply with these policies.

### Expected Outcomes

- Improved protection of critical cluster resources, ensuring that control plane nodes <!--and nodes with 
 resources --> are shielded from inappropriate workloads.
- Increased flexibility for administrators to tailor scheduling policies to the specific needs of their organization and operational environment.



### Workflow Description

<!-- Explain how the user will use the feature. Be detailed and explicit.
Describe all of the actors, their roles, and the APIs or interfaces
involved. Define a starting state and then list the steps that the
user would need to go through to trigger the feature described in the
enhancement. Optionally add a
[mermaid](https://github.com/mermaid-js/mermaid#readme) sequence
diagram.

Use sub-sections to explain variations, such as for error handling,
failure recovery, or alternative outcomes.

For example:

**cluster creator** is a human user responsible for deploying a
cluster.

**application administrator** is a human user responsible for
deploying an application in a cluster.

1. The cluster creator sits down at their keyboard...
2. ...
3. The cluster creator sees that their cluster is ready to receive
   applications, and gives the application administrator their
   credentials. -->
   
This section outlines the workflow for enforcing the proposed scheduling policies that prevent non-control plane workloads from being scheduled on control plane nodes and allow for exceptions based on administrative configurations.

#### Actors

- Cluster Administrator: Responsible for configuring scheduling policies, managing exceptions, and overseeing cluster operations.
- User: Individuals or services attempting to deploy workloads within the cluster.
- Admission Controller: The mechanism that intercepts pod scheduling requests to enforce scheduling policies.

#### Workflow Steps

1. Policy Configuration:

- The cluster administrator defines and configures the scheduling policies using a new or extended configuration resource within the OpenShift API. 
- The admission plugin configuration will be stored under KAO [configuration](https://github.com/openshift/cluster-kube-apiserver-operator/blob/189ce0e7d47864366c741d3d5b9bd9c820421c14/bindata/assets/config/defaultconfig.yaml#L4C3-L4C15). The configuration will not be writable in the first iteration. There's going to be a predefined list of authorized SAs/users for all openshift components allowed to run on control plane nodes. Admins will be allowed to extend the authorized list of SAs/users only through config.openshift.io/v1/scheduler singleton object.
- The policy configuration will be TechPreviewNoUpgrade feature gated. The feature gate name needs to be `OpenShift` prefixed.

2. Pod Scheduling Request:

- A user creates a new pod or updates an existing pod specification, including scheduling preferences such as node selectors or tolerations.
- The request is submitted to the Kubernetes API server.

3. Admission Control:

- The admission controller intercepts the pod scheduling request before it is written to the etcd database.
    - The controller evaluates the request against the configured scheduling policies. This includes checking:
      - if the pod targets any control plane node and
      - whether the pod's service account and requestor are on the allowlist.

4. Policy Enforcement:

- If the request violates the scheduling policies:
  - The admission controller rejects the request.
  - A meaningful error message is returned to the user, explaining the policy violation and suggesting corrective actions.
- If the request complies with the scheduling policies or is exempt:
  - The request is approved.
  - The pod is scheduled according to its specified preferences and Kubernetes scheduling algorithms.

5. Emergency Overrides (Optional):

- In cases where temporary overrides of scheduling policies are necessary, the cluster administrator can apply a temporary configuration change.
- The admission controller processes requests based on the updated policies until the override is removed.
- Any actor with pre-granted permissions to perform an emergency override will be part of the list of authorized service accounts/users.

<!-- #### Variation and form factor considerations [optional]

How does this proposal intersect with Standalone OCP, Microshift and Hypershift?
- Hypershift does not need this protection as all the user control planes are separated from application workloads.
- MicroShift does not deploy any operators or openshift CRDs (there's no scheduler config CRD). MicroShift will not be able to exercise the functionality in the first iteration. A different API needs to be suggested to allow admins to extend the list of authorized service accouts and users.

If the cluster creator uses a standing desk, in step 1 above they can
stand instead of sitting down.

See
https://github.com/openshift/enhancements/blob/master/enhancements/workload-partitioning/management-workload-partitioning.md#high-level-end-to-end-workflow
and https://github.com/openshift/enhancements/blob/master/enhancements/agent-installer/automated-workflow-for-agent-based-installer.md for more detailed examples. -->

### API Extensions

API Extensions are CRDs, admission and conversion webhooks, aggregated API servers,
and finalizers, i.e. those mechanisms that change the OCP API surface and behaviour.

- API extensions this enhancement adds or modifies.
  - introducing a new field under `config.openshift.io/v1/scheduler` allowing admins to extend the list of authorized service accounts and users
  - introducing a new `NodeSchedulingPolicyConfig` type extending the list of configurable plugins in `KubeAPIServerConfig` CRD
- Modification of behavior and restrictions.
  - More strict about who can assign pod's `.spec.nodeName` to a control plane node. Control plane or critical components might fail to be assigned to control plane nodes if the default list of authorized service accounts and users is not properly populated.
  - When upgrading to newer OCP versions new control plane and critical components can be introduced that are required to be assigned to control plane nodes. Thus, creating a dependency on newer Kube API server Operator version to be upgraded first to refresh the list of authorized service accounts and users.

<!--  Examples:
  - Adds a finalizer to namespaces. Namespace cannot be deleted without our controller running.
  - Restricts the label format for objects to X.
  - Defaults field Y on object kind Z.
-->


<!-- ### Implementation Details/Notes/Constraints [optional]

What are the caveats to the implementation? What are some important details that
didn't come across above. Go in to as much detail as necessary here. This might
be a good place to talk about core concepts and how they relate. -->



<!-- #### Hypershift [optional]

Does the design and implementation require specific details to account for the Hypershift use case?
See https://github.com/openshift/enhancements/blob/e044f84e9b2bafa600e6c24e35d226463c2308a5/enhancements/multi-arch/heterogeneous-architecture-clusters.md?plain=1#L282 -->


### Risks and Mitigations

- Unawareness of workloads getting scheduled to control-plane nodes: Users might not be aware of workloads getting accidently scheduled to control-plane nodes. E.g. copy-pasted and altered workloads from available examples.
  - A KCS article informing about the right mitigation can be composed to inform users.
- Third party workloads: Some workloads can be generated through third party solutions. E.g. Tekton. As such users are unable to edit pod specs to keep tolerating `node-role.kubernetes.io/master:NoSchedule` taint.
  - Usual recommendation is to implement a custom admission webhook to drop the toleration.
- Extra authorization: Any additional components scheduling to control plane nodes that are not in the default list of authorized service accounts and users can extend the list through `config.openshift.io/v1/scheduler` singleton object.

### Drawbacks

- Any control-plane or critical component owner needs to be aware of this functionality. Forgetting to extend the list of authorized service accouts and users might cause these components to fail to start properly.
  - **TODO: exercise every control-plane and critical component while the admission plugin is enabled and enforcing to get a list of must-have service accounts and users required to run the components correctly.**
- Admins need to be allowed to run `oc debug`, `oc adm must-gather` and similar commands to perform debugging operations while the admission plugin is in enforcing mode. Thus, all such admins need to be part of the default list of authorized service accounts and users. Failing to do so will reduce debugging capability.
- There could be non-admin users scheduling "special" pods to control plane nodes. E.g. to perform auditing, data collection or maintenance operations over control-plane nodes. All such non-admin users need to be educated to be part of either of the lists of authorized service accounts and users (either admission plugin configuration or config.openshift.io/v1/scheduler).
- Some users might have their own admission plugin mechanism to protect control-plane nodes from user application workloads. Any such case will be analysed per case.

## Design Details

### Open Questions [optional]

1. the default list of authorized service accounts and users needs to be kept in sync for each OCP version. A missing item in the list should be easily noticeable as affected components fail to start. Nevertheless, we need to make an e2e test that will flake for each component under control plane and critical namespaces until each "important" pod is marked as "reviewed".

2. When a service account or a user is no longer needed in the authorized list we need to keep at least one release for deprecation. The same holds for a service account or user renaming. Both the old and the new name must be kept in the list until the deprecation period is over.

3. What happens when a service account or a user is missing in the default authorized list? A new component will need to wait until a new OCP version is released. If the new component is critical any admin can update `config.openshift.io/v1/scheduler` object and extend the list. Nevertheless, this needs to be done in every cluster which makes the extension impractical.

4. Currently, `config.openshift.io/v1/scheduler` type exposes `.spec.mastersSchedulable` field. The new field is defined as:
   ```go
   type SchedulerSpec struct {
       MastersSchedulable bool `json:"mastersSchedulable"`
       
       SchedulingGroups []SchedulingGroup `json:"schedulingGroups"`
   }

   type SchedulingGroup struct {
       // name corresponds to a group name
       Name GroupName `json:"name"`
       
       // mode determines how a policy gets enforced
       Mode GroupMode `json:"mode"`
       
       // authorizedUsers extend the list of default authorized users for a group
       AuthorizedUsers []string `json:"authorizedUsers"`
       
   }

   type GroupName string

   var (
      // ControlPlane represents control plane nodes
      ControlPlane GroupName = "ControlPlane"
   )

   type GroupMode string

   var (
      // Disable means no policy is enforced
      Disable GroupMode = "Disable"
      // Enable means policy is enforced
      Enable GroupMode = "Enable"
      // Inform means inform about the policy
      Inform GroupMode = "Inform"
   )
   ```
   Example:
   ```yaml
   apiVersion: config.openshift.io/v1
   kind: Scheduler
   metadata:
      name: cluster
   spec:
      mastersSchedulable: false
      schedulingGroups:
      - name: ControlPlane
        mode: Inform
        authorizedUsers:
        - "system:vendor:xxx-serviceaccount"
        - "system:vendor:yyy-serviceaccount"  
   ```
   The existing `.spec.mastersSchedulable` field is to be deprecated in favor of the new scheduling groups once the functionality is promoted to GA. Meantime, `.spec.mastersSchedulable` needs to be kept in sync with `ControlPlane` group:
   - when `.spec.mastersSchedulable` is `false` then `.spec.schedulingGroups` for `ControPlane` group needs to be absent or the group mode set to `Disabled` or `Inform`.
   - when `.spec.mastersSchedulable` is `true` then `.spec.schedulingGroups` for `ControPlane` group needs to be present and the group mode set to 'Enabled'
   
5. The admission plugin configuration is defined as:
   ```go
   type NodeSchedulingPolicyConfig struct {
       Groups []SchedulingPolicyGroup `json:"groups"`
   }


   type SchedulingPolicyGroup struct {
       // name corresponds to a group name
       // +kubebuilder:validation:Required
	   // +required
       Name GroupName `json:"name"`
       
       // labelSelector matching a group of nodes
       // +kubebuilder:validation:Required
	   // +required
       LabelSelector metav1.LabelSelector `json:"labelSelector"`
       
       // mode determines how a policy gets enforced.
       // When omitted, this means no opinion and the platform is left to
       // choose a reasonable default, which is subject to change over time.
       // The current default is "Disabled".
	   // +optional
       Mode GroupMode `json:"mode"`
       
       // authorizedUsers is a list of authorized users for a group
       // +kubebuilder:validation:Required
	   // +required
       AuthorizedUsers []string `json:"authorizedUsers"`
       
   }

   // +kubebuilder:validation:Enum=ControlPlane
   type GroupName string

   var (
      // ControlPlane represents control plane nodes
      ControlPlane GroupName = "ControlPlane"
   )

   // +kubebuilder:validation:Enum="";Disable;Enable;Inform
   type GroupMode string

   var (
      // Disable means no policy is enforced
      Disable GroupMode = "Disable"
      // Enable means policy is enforced
      Enable GroupMode = "Enable"
      // Inform means inform about the policy
      Inform GroupMode = "Inform"
   )
   ```

   Example:
   ```json
   apiVersion: kubecontrolplane.config.openshift.io/v1
   kind: KubeAPIServerConfig
   admission:
     pluginConfig:
       scheduling.openshift.io/NodeSchedulingPolicy:
         configuration:
           apiVersion: scheduling.openshift.io/alphav1
           kind: NodeSchedulingPolicyConfig
           groups:
           - name: ControlPlane
             labelSelector:
               matchLabels:
                 node-role.kubernetes.io/control-plane: ""
             mode: Inform
             authorizedUsers:
             - openshift-kube-scheduler/openshift-kube-scheduler-sa
             - ...
   ```
   The admission plugin configuration is not exposed to admins. The list of authorized users will be hard-coded and updated for each OCP release. Admins can extend this default list through `config.openshift.io/v1/scheduler` singleton object for service accounts and users which create additional components running on control plane nodes. E.g. additional component layers that are not known in advance but are required to run on control plane nodes.
   A group name can be an arbitrary name. Nevertheless, group names in `config.openshift.io/v1/scheduler` will be pre-defined and map to their admission plugin group names equivalents. Currently, only `ControlPlane` group name is considered.

6. A scheduler is expected to set pod's target node through a pods/bind subresource request. The admission plugin intercepts this request with the scheduler's service account. Thus, each scheduler service account needs to be in the authorized list of service accounts and users. Any secondary scheduler is expected to extend the authorized list through `config.openshift.io/v1/scheduler` object.

7. Question: "A service account `SA_C` has an rbac rule to create a pod, a group of service accounts `SA_U1` - `SA_Un` have rbac rules to update a pod and a scheduler service account `SA_bind` has an rbac rule to bind a pod. Which of the service accounts is considered the authoritative service account? Should all be considered as such?"
    - The general question here is how to determine whether a pod can be scheduled to a control plane node.
    - Creating an allow list of pods is too granular, also a pod name is usually not known in advance. Namespaces and service accounts are longer lasting.
    - Creating an allow list of namespaces is equivalent to creating a list of service accouts. I.e.
        - if a pod in a given namespace is allowed to be scheduled to a control plane node, there's a corresponding service account in that namespace (e.g. default) that can be used as a discriminator.
        - if a pod with a given service account is allowed to be scheduled to a control plan node, there's a corresponding namespace (SA's namespace) that can be used as a discriminator.
    - Presence of any scheduler service account in the authorized list is not a sufficient condition for authorizing an assignment to a control plane node. Both scheduler and corresponding pod service accounts need to be checked to confirm:
        - Actor performing node assignement is authorized to assign to a specific node
        - Pod getting a node assigned is authorized to be assigned to a specific node
    - In case there are two or more service accounts in a namespace it's sufficient to list only one of them in the authorized list. E.g. the default one. Example:
      ```yaml
       - name: ControlPlane
         labelSelector:
           matchLabels:
             node-role.kubernetes.io/control-plane: ""
         authorizedUsers:
         # authorized to assign control plane nodes
         - openshift-kube-scheduler/openshift-kube-scheduler-sa 
         # authorized to be assigned control plane nodes
         - openshift-console/default
         - openshift-multus/multus
      ```
    - Meaning, node assignment authorization is namespaced scoped. E.g. a pod can be assigned to a control plane node when it lives under an authorized namespace. Authorization is carried out through a service account attached to the namespace.

### Test Plan

**Note:** *Section not required until targeted at a release.*

TODO

<!-- Consider the following in developing a test plan for this enhancement:
- Will there be e2e and integration tests, in addition to unit tests?
- How will it be tested in isolation vs with other components?
- What additional testing is necessary to support managed OpenShift service-based offerings?

No need to outline all of the test cases, just the general strategy. Anything
that would count as tricky in the implementation and anything particularly
challenging to test should be called out.

All code is expected to have adequate tests (eventually with coverage
expectations). -->

### Graduation Criteria

**Note:** *Section not required until targeted at a release.*

TODO

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
to the aforementioned [maturity levels][maturity-levels]. -->

#### Dev Preview -> Tech Preview

TODO

<!-- - Ability to utilize the enhancement end to end
- End user documentation, relative API stability
- Sufficient test coverage
- Gather feedback from users rather than just developers
- Enumerate service level indicators (SLIs), expose SLIs as metrics
- Write symptoms-based alerts for the component(s) -->

#### Tech Preview -> GA

TODO

<!-- - More testing (upgrade, downgrade, scale)
- Sufficient time for feedback
- Available by default
- Backhaul SLI telemetry
- Document SLOs for the component
- Conduct load testing
- User facing documentation created in [openshift-docs](https://github.com/openshift/openshift-docs/)

**For non-optional features moving to GA, the graduation criteria must include
end to end tests.** -->


### Upgrade / Downgrade Strategy

TODO

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

### Version Skew Strategy

TODO

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

### Operational Aspects of API Extensions

TODO

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
  this enhancement) -->

#### Failure Modes

TODO

<!-- - Describe the possible failure modes of the API extensions.
- Describe how a failure or behaviour of the extension will impact the overall cluster health
  (e.g. which kube-controller-manager functionality will stop working), especially regarding
  stability, availability, performance and security.
- Describe which OCP teams are likely to be called upon in case of escalation with one of the failure modes
  and add them as reviewers to this enhancement. -->

#### Support Procedures

TODO

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

## Implementation History

TODO

<!-- Major milestones in the life cycle of a proposal should be tracked in `Implementation
History`. -->

## Alternatives

### Have control-plane nodes tainted with NoExecute taint

- Kubelet recognizes any NoExecute taint and rejects any pod that does not tolerate the taint. E.g. `node-role.kubernetes.io/master:NoExecute`.
- The taint is a scheduler-free taint, i.e. no scheduler needs to understand taints and tolerations.
- Any pod expected to be scheduled to a control plane node needs to tolerate the taint.
- The admission plugin needs to reject any pod that is not allowed to tolerate the NoExecute taint.
- When `masterSchedulable` is set to `true`, the admission plugin is disabled.
- A scheduling group will no longer need a label selector. Instead, a list of taints will be required.
- Each group will still need to keep a list of authorized service accouts and users
- There's probably no guarantee for all control plane pods getting updated first (with the NoExecute toleration) before control plane nodes get tainted during an OCP upgrade. For that, all control plane pods need to tolerate the taint first in OCP X so nodes can be tainted in OCP X+1 without any unnecessary rejections/disruptions.

### Validating Admission Policy

A validation admission policy can be defined to enforce a rule stating that pods are not allowed to specify toleration for the `node-role.kubernetes.io/master:NoExecute` taint:

```yaml
apiVersion: admissionregistration.k8s.io/v1beta1
kind: ValidatingAdmissionPolicy
metadata:
  name: "control-plane-scheduling-policy"
spec:
  failurePolicy: Fail
  matchConstraints:
    resourceRules:
    - apiGroups:   [""]
      apiVersions: ["v1"]
      operations:  ["CREATE", "UPDATE"]
      resources:   ["pods"]
  validations:
    - expression: "spec.tolerations.all(toleration, !(toleration.key == 'node-role.kubernetes.io/master' && toleration.effect == 'NoExecute'))"
```

Afterward, installing a binding that enforces the validation specifically in namespaces that do not have the `openshift.io/control-plane-namespace` label key set:

```yaml
apiVersion: admissionregistration.k8s.io/v1beta1
kind: ValidatingAdmissionPolicyBinding
metadata:
  name: "control-plane-scheduling-policy-binding"
spec:
  policyName: "control-plane-scheduling-policy"
  validationActions: [Deny]
  matchResources:
    namespaceSelector:
      matchExpressions:
      - key: openshift.io/control-plane-namespace
        operator: DoesNotExist
```

**Limitation**: users with namespace create/update RBAC rule can label their namespaces accordingally to bypass the validation.

**Advantage**: no need for the authorized list of service accounts and users.

To allow pod scheduling during emergencies any affected namespace is expected to be labeled with the mentioned label.

