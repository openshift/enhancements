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
* As an administrator, I want only certain workloads to be allowed to schedule pods on tainted nodes.
* As an administrator, I want to restrict workloads from being scheduled on special node groups (even those using `.spec.nodeName` in their pods), so that those node groups can only run specialized workloads without obstruction.
* As a cluster administrator, I want to have a clear and manageable way to update and maintain who or what will be allowed to schedule workloads on control plane nodes, so that I can efficiently manage permissions as the cluster evolves or as new teams/services are on-boarded.
* As an OpenShift developer, I want to understand the impact of the new scheduling restrictions on existing and future workloads, so that I can design applications that comply with cluster policies and make informed decisions about resource requests and deployment strategies. 
* As an end user, I want to receive informative feedback when my workloads are rejected due to being repelled from control plane nodes or special node groups, so that I can make the necessary adjustments without needing extensive support from cluster administrators.
* As a security professional, I want to ensure that this pod rejection mechanism is enforceable and auditable, so that I can verify compliance with internal and external regulations regarding resource access and control plane integrity.
* As an administrator, I want the ability to temporarily override scheduling restrictions for emergency or maintenance tasks without compromising the overall security posture of the cluster, ensuring that critical operations can be performed when necessary.

### Goals

<!-- *Summarize the specific goals of the proposal. How will we know that
this has succeeded?  A good goal describes something a user wants from
their perspective, and does not include the implementation details
from the proposal.* -->

- Enhance Cluster Security and Stability: Prevent non-control plane workloads from being scheduled on control plane nodes to avoid resource competition and potential out-of-memory (OOM) issues that could affect critical cluster operations.

- Flexible and Manageable Workload Scheduling: Enable cluster administrators to specify and manage exceptions based on namespace labels, allowing certain workloads to be scheduled on control plane nodes when necessary for operational requirements.

- Protect Specialized Resources: Restrict scheduling of regular user workloads on nodes with specialized resources (e.g., GPU-enabled nodes) to ensure these costly resources are reserved for appropriate workloads.

- Emergency Override Capability: Allow administrators to temporarily override scheduling restrictions for emergency or maintenance tasks, ensuring critical operations can be performed when necessary without compromising the cluster's overall security posture.

### Non-Goals

<!-- What is out of scope for this proposal? Listing non-goals helps to
focus discussion and make progress. Highlight anything that is being
deferred to a later phase of implementation that may call for its own
enhancement. -->

- Granular Pod-Level Scheduling Controls: The enhancement will not introduce fine-grained controls for individual pod scheduling decisions beyond the existing Kubernetes mechanisms (e.g., taints, tolerations, and affinity rules).

- Automated User or Service Account Management: Automatically managing or updating the list of service accounts or namespaces authorized to schedule workloads on control plane nodes is out of scope. This process remains a manual administrative responsibility.

- Real-Time Resource Allocation Optimization: The proposal does not aim to dynamically optimize resource allocation or scheduling decisions based on real-time cluster utilization or performance metrics.

## Proposal

<!-- This is where we get down to the nitty gritty of what the proposal
actually is. Describe clearly what will be changed, including all of
the components that need to be modified and how they will be
different. Include the reason for each choice in the design and
implementation that is proposed here, and expand on reasons for not
choosing alternatives in the Alternatives section at the end of the
document. -->

### Overview
This enhancement proposes the implementation of a more robust and flexible mechanism for enforcing pod placement rejections on OpenShift clusters. It will focus on preventing user workloads from being scheduled on control plane nodes and allowing for exceptions based on administrative configurations.

### Implementation Strategies

Update OpenShift Components for Taint Tolerance: Modify OpenShift operators and control plane components to include tolerations for the NoExecute taint on control plane nodes that admins can apply to achieve the goals described here. This will ensure that essential services and components are not evicted or prevented from scheduling on these nodes due to the taint. The update process should involve a thorough review of all default and critical components to add the necessary toleration, ensuring they continue to operate as expected in environments where the NoExecute taint is applied. This step is crucial for maintaining cluster stability and ensuring that core functionalities are not disrupted by the enforcement of the new scheduling policies. 

NoExecute Taint Application: Admins seeking to implement this proposal will need to apply the NoExecute taints to control plane nodes (or specialized nodes) to automatically prevent pods without the specific toleration from being scheduled or remaining on these nodes. This approach leverages the kubelet's inherent behavior to ensure compliance with scheduling policies.

Validating Admission Policy and Binding: Admins seeking to implement this proposal will need to write and apply a new or extend an existing validating admission policy (and binding) to enforce scheduling policies based on namespace labels and pod tolerations. This policy will validate incoming pod creation and update requests to ensure they do not include tolerations for the node-role.kubernetes.io/control-plane:NoExecute taint (or any other taint/toleration the admin wishes to configure, for special node groups) unless the namespace is explicitly labeled to allow such tolerations.

Namespace Label Management: Admins seeking to implement this proposal need to introduce tools, scripts or organizational processes to assist administrators in managing labels on namespaces that should be exempt from the default scheduling restrictions. This could include automation for emergency situations where rapid response is necessary.

### Expected Outcomes

- Improved protection of critical cluster resources, ensuring that control plane nodes and nodes with 
 resources are shielded from inappropriate workloads.
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
   
This section outlines the workflow for enforcing the proposed mechanism for prevention of non-control plane workloads from being scheduled on control plane nodes and allow for exceptions based on administrative configurations.

#### Actors

Pre-Workflow:
- OpenShift Engineers: Ensure all control plane and essential workloads ship with the correct tolerations out of the box to seamlessly operate on NoExecute tainted nodes. This preparation is crucial for the uninterrupted functioning of OpenShift's core services.

Workflow:
- Cluster Administrator: Tasked with applying NoExecute taints to control plane nodes or nodes belonging to other specialized groups. For extending this approach to other special node groups, they must coordinate with relevant teams to ensure those workloads include necessary tolerations. They are also responsible for creating and applying ValidatingAdmissionPolicy and ValidatingAdmissionPolicyBinding resources to configure which taint tolerations are disallowed for pods, except in specific namespaces designated by a particular label (e.g., openshift.io/control-plane-namespace, or another custom label). This role involves a strategic overview of the cluster's security and workload management policies.
- Namespace Administrator: Manages their namespaces, including labeling them appropriately (e.g., with openshift.io/control-plane-namespace, or another custom label) when exemptions to the default scheduling policies are needed. This role requires understanding the impact of these labels on workload scheduling and compliance with the cluster's security policies.
- User/Developer: Those deploying workloads within the cluster must ensure their applications carry the correct tolerations as advised by Cluster Administrators, especially when targeting special node groups. They need to stay informed about the cluster's scheduling policies and adapt their workloads accordingly.

Additional Considerations:

- Security Auditors: Although not directly involved in the workflow, security auditors need to periodically review and audit the applied ValidatingAdmissionPolicies, ValidatingAdmissionPolicyBindings, and namespace labels to ensure compliance with the intended security and operational policies. They may also review logs and events related to admission policy rejections to ensure policies are correctly enforced.

#### Pre-Workflow Steps

1. Continuous Standardization of Shipped tolerations. 

- OpenShift Engineers need to ensure that all control plane and essential workloads ship with the correct tolerations for the NoExecute taints being applied to control plane nodes. This is critical for maintaining uninterrupted operations of OpenShift's core services on these nodes. This involves getting multiple teams on the same page and that new projects also incorporate the needed tolerations.

#### Workflow Steps

1. Taint Application:

- Cluster Administrator applies NoExecute taints to control plane nodes and any other special node groups deemed necessary to protect. This foundational step prevents unauthorized workloads from being scheduled (or directly placed with spec.nodeName field) on these critical nodes.

```
oc taint nodes ip-XX-XX-XX-XXX.ec2.internal node-role.kubernetes.io/control-plane:NoExecute-
```

2. Validating Admission Policy Creation:

- Cluster Administrator crafts and deploys a ValidatingAdmissionPolicy resource, incorporating CEL expressions to scrutinize pod tolerations. This policy is specifically designed to reject pods that unlawfully tolerate the NoExecute taints associated with the protected nodes.

```
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
    - expression: "object.spec.tolerations.all(toleration, !(toleration.key == 'node-role.kubernetes.io/master' && toleration.effect == 'NoExecute'))"
```

3. Validating Admission Policy Binding:

- Concurrently, the Cluster Administrator establishes a ValidatingAdmissionPolicyBinding to dictate the realms of policy application. By default, it affects all namespaces, excluding those marked with a predefined exception label (e.g., openshift.io/control-plane-namespace, or another custom label), to manage exceptions systematically.

```
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

4. Namespace Labeling for Exceptions:

- Namespace Administrators are tasked with applying specific labels to their namespaces if exemptions to the default scheduling policies are warranted. This procedure enables selected workloads within these namespaces to bypass the standard restrictions by tolerating the designated NoExecute taints (should be performed on control plane nodes for control plane protection - should be performed on special node groups if that is what needs to be protected).

```
oc label <NAMESPACE_NAME> default openshift.io/control-plane-namespace=true
```

5. Pod Scheduling Attempt:

- Users/Developers proceed to deploy or update pods, embedding scheduling preferences, such as necessary tolerations. Example of adding a relevant toleration:

```
apiVersion: v1
kind: Pod
metadata:
  name: my-pod
  namespace: <NAMESPACE_NAME>
  labels:
    app: my-app
spec:
  containers:
  - name: my-container
    image: nginx
    ports:
    - containerPort: 80
  nodeName: ip-XX-X-XX-XXX.ec2.internal
  tolerations:
  - key: "node-role.kubernetes.io/master"
    operator: "Exists"
    effect: "NoExecute"
```

- These requests are then submitted to the Kubernetes API server for processing.

6. Admission Policy Enforcement:

- Upon receiving pod deployment or update requests, the Kubernetes API server invokes the ValidatingAdmissionPolicy. It evaluates the pod's tolerations against the policy's defined CEL expressions, determining the request's compatibility with established configured guidelines.

7. Policy Decision Feedback:

- Should a pod's request contradict the policy, it is promptly rejected. The User/Developer is informed of this decision through an error message that highlights the specific policy violation, akin to:

```
The pods "my-pod" is invalid: : ValidatingAdmissionPolicy 'control-plane-scheduling-policy' with binding 'control-plane-scheduling-policy-binding' denied request: failed expression: object.spec.tolerations.all(toleration, !(toleration.key == 'node-role.kubernetes.io/master' && toleration.effect == 'NoExecute'))
```

This message clearly indicates the failed policy expression, aiding Users/Developers in understanding the reason behind the rejection and guiding them towards necessary adjustments.

- Conversely, if the request aligns with the policy or falls within an exception due to namespace labeling, the process advances eventually adhering to Kubernetes' standard scheduling steps (or direct pod placement with spec.nodeName).

8. Emergency Overrides and Adjustments:

- For emergency scenarios necessitating temporary deviations from the norm, Cluster Administrators have the option to swiftly modify namespace labels or adjust the ValidatingAdmissionPolicy and ValidatingAdmissionPolicyBinding resources. This flexibility ensures that critical operations can proceed unhindered, even under exceptional circumstances.


**Note:** While the ValidatingAdmissionPolicy feature is in Tech Preview within OpenShift, an initial step requires Cluster Administrators to enable the TechPreview feature gate on their cluster and subsequently restart the API Servers to facilitate the creation of policies and bindings. It's important to note that activating this feature marks the cluster as non-upgradable. This significant consideration should be carefully weighed when deciding to implement this enforcement approach.


### API Extensions

No new API or fields added. Solution works out of the box, given we ship necessary tolerations for control plane workloads managed by us.


### Risks and Mitigations

- Unintended Namespace Labeling: Administrators or namespace owners might inadvertently label namespaces in a way that bypasses the scheduling restrictions, potentially exposing control plane or special nodes to unauthorized workloads.
    - Mitigation: Implement clear policies and documentation around the labeling of namespaces. Standardize a simple workflow, similar to the one mentioned above, so users don't deviate too much, while still allowing them to if they want.
- Complex Policy Management: The creation and maintenance of Validating Admission Policies and Bindings could become complex, especially in large clusters with diverse workloads and multiple special node groups.
    - Mitigation: Similar to above mitigation, we could have KCS articles and documentation with step by step examples for the main control plane protection use-case and possibly an additional one for GPU enabled nodes being protected so users follow those wihout too much deviation.
- Accidental Eviction of Critical Workloads: The application of NoExecute taints and subsequent enforcement might lead to the accidental eviction of workloads that are critical but were not correctly configured with the necessary tolerations.
    - Mitigation: Prior to applying NoExecute taints, perform a comprehensive review of all workloads running on the nodes to be protected. Ensure that all essential services and components include the correct tolerations. Utilize dry-run or audit modes of the Validating Admission Policy, if available, to assess the impact before advised enforcement.

### Drawbacks

- Increased Complexity for Cluster Administrators: The introduction of NoExecute taints and the requirement to manage Validating Admission Policies and Bindings may increase the complexity of cluster administration. Administrators now need a deeper understanding of how taints, tolerations, and admission policies interact to enforce these constraints.

- Potential for Misconfiguration: The reliance on namespace labels to exempt certain workloads from scheduling restrictions introduces a risk of misconfiguration, either by applying incorrect labels or failing to update labels as policies evolve.

- Risk of Disruption to Existing Workloads: Applying NoExecute taints to nodes could lead to the eviction of existing workloads that do not have the necessary tolerations, potentially disrupting services.

- Limitation on Feedback Detail from Admission Policies: The feedback provided to users when their pods are rejected due to policy violations may not always be detailed or user-friendly, potentially leading to confusion and delays in troubleshooting.

- Difficulty in Handling Emergency Situations: In emergencies requiring rapid adjustments to scheduling policies or taints, the process to modify or temporarily disable these restrictions must be swift and fail-safe to avoid adding delays to critical operations. 

- Challenges with Third-Party Workloads: Ensuring that third-party operators or helm charts comply with the new scheduling restrictions could be challenging, especially if those workloads require updates to include the necessary tolerations.


#### Dev Preview -> Tech Preview -> GA

The necessary feature for this solution (ValidatingAdmissionPolicy) is already available, even though it is in Tech Preview (it is in beta but disabled by default upstream). The evolution of this solution ties with the evolution of [ValidatingAdmissionPolicy](https://github.com/kubernetes/enhancements/blob/master/keps/sig-api-machinery/3488-cel-admission-control/README.md) and the decisions to graduate it downstream on Openshift. 



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

### Admission Plugin Approach (Discarded)


Initially, the proposal considered using an admission plugin to enforce scheduling policies on control plane nodes. This approach involved developing or extending an existing admission controller/plugin that would reject pods attempting to schedule on protected nodes unless they met specific criteria defined by administrators, such as coming from selected service accounts or users.

#### Key Points of the Admission Plugin Approach:
- Required the creation of a new or modified admission controller within the OpenShift ecosystem to intercept and evaluate pod scheduling requests based on a predefined list of authorized service accounts and users and labeled nodes.
- Administrators would have to manage and update this list as part of the cluster configuration, potentially complicating the administration and leading to scalability issues in larger or more dynamic environments.
- It relied on manual configuration and updates to keep the authorized list relevant, which could increase the risk of human error and oversight.
- Offered less flexibility in terms of policy enforcement compared to dynamic evaluation with CEL expressions.

#### Reasons for Discarding:

- Complexity and Maintenance Overhead: Managing a static list of authorized service accounts and users introduced significant overhead for cluster administrators, especially in dynamic and large-scale environments.
- Lack of Flexibility: The approach was less adaptable to complex or changing requirements, as updating the policy required manual intervention and could not easily accommodate context-aware decisions.
- Potential for Misconfiguration: The reliance on manual updates increased the risk of misconfiguration, potentially leading to security vulnerabilities or disruptions in service.
- Advancements in Kubernetes Ecosystem: The introduction and maturation of features like the Validating Admission Policy with CEL integration offered more robust, flexible, and Kubernetes-native mechanisms for enforcing scheduling policies together with NoExecute taints.
- Given these considerations, the proposal pivoted towards leveraging NoExecute taints combined with Validating Admission Policies as the main solution. This approach aligns better with Kubernetes' declarative and extensible design principles, providing a more scalable, flexible, and maintainable method for enforcing pod scheduling policies on control plane nodes, also giving more power to customers to shape their solution.
