---
title: prevent-user-workloads-from-being-scheduled-on-control-plane-nodes
authors:
  - knelasevero
  - ingvagabund
  - flavianmissi
reviewers: # Include a comment about what domain expertise a reviewer is expected to bring and what area of the enhancement you expect them to focus on. For example: - "@networkguru, for networking aspects, please look at IP bootstrapping aspect"
  - ingvagabund
  - deads2k
  - flavianmissi
approvers: # A single approver is preferred, the role of the approver is to raise important questions, help ensure the enhancement receives reviews from all applicable areas/SMEs, and determine when consensus is achieved such that the EP can move forward to implementation.  Having multiple approvers makes it difficult to determine who is responsible for the actual approval.
  - deads2k
api-approvers: # In case of new or modified APIs or API extensions (CRDs, aggregated apiservers, webhooks, finalizers). If there is no API change, use "None"
  - deads2k
creation-date: 2024-04-01
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
   answered so that work can begin. Come back and update the document
   if important details (API field names, workflow, etc.) change
   during code review.
1. **Keep all required headers.** If a section does not apply to an
   enhancement, explain why but do not remove the section. This part
   of the process is enforced by the linter CI job.

See ../README.md for background behind these instructions.

Start by filling out the header with the metadata for this enhancement.

# Prevent user workloads from being scheduled on control plane nodes

## Summary

Starting OCP 4.1 Kubernetes Scheduler Operator’s `config.openshift.io/v1/scheduler` type was extended with `.spec.mastersSchedulable` field [[1]](#ref-1) set to `false` by default.
Its purpose is to protect control plane nodes from receiving a user workload. When the field is set to `false` each control plane node is tainted with `node-role.kubernetes.io/control-plane:NoSchedule`.
If set to `true` the taint is removed from each control plane node.
No user workload is expected to tolerate the taint.
Unfortunately, there’s currently no protection from users (with pod’s create/update RBAC permissions) explicitly tolerating `node-role.kubernetes.io/control-plane:NoSchedule` taint or setting `.spec.nodeName` field directly (thus by-passing the kube-scheduler).

<a id="ref-1"></a>[1] https://docs.openshift.com/container-platform/latest/nodes/nodes/nodes-nodes-managing.html#nodes-nodes-working-master-schedulable_nodes-nodes-managing

## Motivation

<!-- This section is for explicitly listing the motivation, goals and non-goals of
this proposal. Describe why the change is important and the benefits to users. -->

Allowing arbitrary users to bypass the `spec.mastersSchedulable` field and schedule their workloads on control plane nodes poses a security risk of scheduling too many pods on each control-plane node while overloading resources and increasing the chance of e.g. memory pressure.
Resulting in e.g. control plane pods getting OOM killed.
Even when all the control plane pods have (are expected to have) the highest priority classes and thus pre-emption is not expected it’s safer to have abundance of resources than deficit to accommodate for various disruptions.

Also, secondary schedulers might not take taints and tolerations into account when selecting a node.
Thus, another layer of protection is needed to avoid prohibited node assignments for components that are unaware of the `mastersSchedulable` functionality.

### User Stories

- As an administrator, I want to restrict non control plane workloads from being scheduled on control plane nodes (even those using `.spec.nodeName` in their pods), so that the control plane components are not at risk of running out of resources.
- As an administrator, I want only certain workloads to be allowed to schedule pods on tainted nodes.
- As an administrator, I want to restrict workloads from being scheduled on special node groups (even those using `.spec.nodeName` in their pods), so that those node groups can only run specialized workloads (e.g., GPU-enabled nodes) without obstruction.
- As a cluster administrator, I want to have a clear and manageable way to update and maintain who or what will be allowed to schedule workloads on control plane nodes, so that I can efficiently manage permissions as the cluster evolves or as new teams/services are on-boarded, per namespace or cluster-wide.
- As an OpenShift developer, I want to understand the impact of the new scheduling restrictions on existing and future workloads, so that I can design applications that comply with cluster policies and make informed decisions about resource requests and deployment strategies.
- As an end user, I want to receive informative feedback when my workloads are rejected due to being repelled from control plane nodes or special node groups, so that I can make the necessary adjustments without needing extensive support from cluster administrators.
- As a security professional, I want to ensure that this pod rejection mechanism is enforceable and auditable, so that I can verify compliance with internal and external regulations regarding resource access and control plane integrity.
- As an administrator, I want the ability to temporarily override scheduling restrictions for emergency or maintenance tasks without compromising the overall security posture of the cluster, ensuring that critical operations can be performed when necessary.

### Goals

<!-- *Summarize the specific goals of the proposal. How will we know that
this has succeeded?  A good goal describes something a user wants from
their perspective, and does not include the implementation details
from the proposal.* -->

- Enhance Cluster Security and Stability: Prevent non-control plane workloads from being scheduled on control plane nodes to avoid resource competition and potential out-of-memory (OOM) issues that could affect critical cluster operations.

- Flexible and Manageable Workload Scheduling: Enable cluster administrators to specify and manage exceptions, allowing certain workloads to be scheduled on control plane nodes when necessary for operational requirements.

- Protect Specialized Resources: Restrict scheduling of regular user workloads on nodes with specialized resources (e.g., GPU-enabled nodes) to ensure these costly resources are reserved for appropriate workloads.

- Emergency Override Capability: Allow administrators to temporarily override scheduling restrictions for emergency or maintenance tasks, ensuring critical operations can be performed when necessary without compromising the cluster's overall security posture.

### Non-Goals

<!-- What is out of scope for this proposal? Listing non-goals helps to
focus discussion and make progress. Highlight anything that is being
deferred to a later phase of implementation that may call for its own
enhancement. -->

- Granular Pod-Level Scheduling Controls: The enhancement will not introduce fine-grained controls for individual pod scheduling decisions beyond the existing Kubernetes mechanisms (e.g., taints, tolerations, and affinity rules).

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

This enhancement proposes the implementation of a more robust and flexible mechanism for enforcing pod placement rejections on OpenShift clusters.
It will focus on preventing user workloads from being scheduled on control plane nodes and allowing for exceptions based on administrative configurations.

### Implementation Strategies

Update OpenShift Components for Taint Tolerance: Modify OpenShift operators and control plane components to include tolerations for the NoExecute taint on control plane nodes that admins can apply to achieve the goals described here.
This will ensure that essential services and components are not evicted or prevented from running on these nodes due to the taint.
The update process should involve a thorough review of all default and critical components to add the necessary toleration, ensuring they continue to operate as expected in environments where the NoExecute taint is applied.
This step is crucial for maintaining cluster stability and ensuring that core functionalities are not disrupted by the enforcement of the new scheduling policies.

NoExecute Taint Application: Admins seeking to implement this proposal will need to apply the NoExecute taints to control plane nodes (or specialized nodes) to automatically prevent pods without the specific toleration from being scheduled or remaining on these nodes. This approach leverages the kubelet's inherent behavior to ensure compliance with scheduling policies.

Validating Admission Policy and Binding: Admins seeking to implement this proposal will need to write and apply a new or extend an existing validating admission policy (and binding) to enforce scheduling policies based on namespace labels and pod tolerations. This policy will validate incoming pod creation and update requests to ensure they do not include tolerations for the node-role.kubernetes.io/control-plane:NoExecute taint (or any other taint/toleration the admin wishes to configure, for special node groups) unless the namespace is explicitly labeled to allow such tolerations.

Namespace Label Management: Admins seeking to implement this proposal need to introduce tools, scripts or organizational processes to assist administrators in managing labels on namespaces that should be exempt from the default scheduling restrictions. This could include automation for emergency situations where rapid response is necessary.

### Expected Outcomes

- Improved protection of critical cluster resources, ensuring that control plane nodes and nodes with
  specialized resources are shielded from inappropriate workloads.
- Increased flexibility for administrators to tailor the enforcement scheduling policies to the specific needs of their organization and operational environment.

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

- OpenShift Engineers: Ensure all control plane and essential workloads ship with the correct tolerations and permissions out of the box to seamlessly operate on NoExecute tainted nodes. This preparation is crucial for the uninterrupted functioning of OpenShift's core services.

Workflow:

- Cluster Administrator: Tasked with applying NoExecute taints to control plane nodes or nodes belonging to other specialized groups. For extending this approach to other special node groups, they must coordinate with relevant teams to ensure those workloads include necessary tolerations and permissions. They are also responsible for creating and applying the ValidatingAdmissionPolicyBinding (that will enable the enforcement of the provided ValidatingAdmissionPolicy we ship) resources to configure which taint tolerations are disallowed for pods. This role involves a strategic overview of the cluster's security and workload management policies.
- Namespace Administrator: Manages their namespaces, including adding new RBAC rules to allow certain workloads to tolerate the applied NoExecute taint.
- User/Developer: Those deploying workloads within the cluster must ensure their applications carry the correct tolerations as advised by Cluster Administrators, especially when targeting special node groups. They need to stay informed about the cluster's scheduling policies and adapt their workloads accordingly.

Additional Considerations:

- Security Auditors: Although not directly involved in the workflow, security auditors need to periodically review and audit the applied ValidatingAdmissionPolicies, ValidatingAdmissionPolicyBindings, and relevant RBAC rules to ensure compliance with the intended security and operational policies. They may also review logs and events related to admission policy rejections to ensure policies are correctly enforced.

#### Pre-Workflow Steps

1. Continuous Standardization of Shipped tolerations.

- OpenShift Engineers need to ensure that all control plane and essential workloads ship with the correct tolerations for the NoExecute taints being applied to control plane nodes. This is critical for maintaining uninterrupted operations of OpenShift's core services on these nodes. This involves getting multiple teams on the same page and that new projects also incorporate the needed tolerations.
- OpenShift Engineers also need to guarantee that those workloads have the right permissions to tolerate the NoExecute taint, as the ValidatingAdmissionPolicy checks for custom taint/toleration key and verbs.
- If custom special node groups are being considered for protection, Cluster Administrators need to coordinate with Users/Developers to implement new tolerations for workloads intended for those specific node groups (e.g., GPU-enabled nodes). This coordination ensures that the designated workloads are appropriately scheduled on the protected nodes.

#### Workflow Steps

1. Node tainting:

- Cluster Administrator applies NoExecute taints to control plane nodes and any other special node groups deemed necessary to protect. This foundational step prevents unauthorized workloads from being scheduled (or directly placed with spec.nodeName field) on these critical nodes.

```
oc taint nodes ip-XX-XX-XX-XXX.ec2.internal node-role.kubernetes.io/control-plane:NoExecute-
```

2. Validating Admission Policy provided:

- As part of this proposal a `ValidatingAdmissionPolicy` will be provided as part of the static manifests applied by Kube Scheduler Operator ([staticresourcecontroller](https://github.com/openshift/cluster-kube-scheduler-operator/blob/67304344622e50cbcd95e7a05056294e9d96df99/pkg/operator/starter.go#L108)).

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
    - expression: "object.spec.tolerations.all(toleration, (
      toleration.effect != 'NoExecute' ||
      (toleration.effect == 'NoExecute' &&
      (auhorizer.serviceAccount(object.metadata.namespace, object.spec.serviceAccountName).group('').resource(toleration.key).namespace(object.metadata.namespace).check(toleration.effect).allowed()))))"
      messageExpression: >
        "Pod toleration for 'NoExecute' is not authorized for service account '" + object.spec.serviceAccountName + "' in namespace '" + object.metadata.namespace + "'."

```

3. Validating Admission Policy Binding:

- The Cluster Administrator creates a ValidatingAdmissionPolicyBinding to dictate the realms of policy application. Binding shown here affects all namespaces.

```
apiVersion: admissionregistration.k8s.io/v1beta1
kind: ValidatingAdmissionPolicyBinding
metadata:
  name: enforce-tolerations-policy-binding
spec:
  policyName: enforce-tolerations-policy
  validationActions: [Deny]

```

4. Service Account to be used by the workload:

- Namespace Administrators or Application Developers need to have a valid Service Account to be used by the workload before the workload is created so the ValidatingAdmisisonPolicy can use the secondary authorization approach above checking if this Service Account can tolerate the relevant taints.

```
apiVersion: v1
kind: ServiceAccount
metadata:
  name: special-service-account
  namespace: default
```

5. Role and RoleBinding with custom verbs

- Namespace Administrators or application developers must grant the necessary permissions to this special Service Account to tolerate the relevant taints required. We are using custom verbs and keys, and those are never used by kubernetes itself. We can use the authorizer in the ValidatingAdmissionPolicy to check this custom verbs specific to this workflow.

```
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: tolerate-noexecute-role
  namespace: default
# rules: []
rules:
- apiGroups: [""]
  resources: ["node-role.kubernetes.io/control-plane"]
  verbs: ["NoExecute"]
- apiGroups: [""]
  resources: ["node.kubernetes.io/not-ready"]
  verbs: ["NoExecute"]
- apiGroups: [""]
  resources: ["node.kubernetes.io/unreachable"]
  verbs: ["NoExecute"]
- apiGroups: [""]
  resources: ["node.kubernetes.io/disk-pressure"]
  verbs: ["NoExecute"]
- apiGroups: [""]
  resources: ["node.kubernetes.io/memory-pressure"]
  verbs: ["NoExecute"]
- apiGroups: [""]
  resources: ["node.kubernetes.io/pid-pressure"]
  verbs: ["NoExecute"]
- apiGroups: [""]
  resources: ["node.kubernetes.io/unschedulable"]
  verbs: ["NoExecute"]
- apiGroups: [""]
  resources: ["node.kubernetes.io/network-unavailable"]
  verbs: ["NoExecute"]
- apiGroups: [""]
  resources: ["node.kubernetes.io/kubelet-unreachable"]
  verbs: ["NoExecute"]
```

- Note we are adding some other NoExecute taints on top of `node-role.kubernetes.io/control-plane:NoExecute` as some other ones might be needed (case-by-case).

- Next the RoleBinding uses this role to give the necessary permissions to the Service Account that later will be used by the workload.

```
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: tolerate-noexecute-rolebinding
  namespace: default
subjects:
- kind: ServiceAccount
  name: special-service-account
  namespace: default
roleRef:
  kind: Role
  name: tolerate-noexecute-role
  apiGroup: rbac.authorization.k8s.io
```

6. Pod Scheduling Attempt:

- Users/Developers proceed to deploy or update pods, embedding scheduling preferences, such as necessary tolerations. Example of adding a relevant tolerations:

```
apiVersion: v1
kind: Pod
metadata:
  name: example-pod
  namespace: default
spec:
  containers:
    - name: example-container
      image: nginx
  serviceAccountName: special-service-account
  tolerations:
    # - key: "node-role.kubernetes.io/control-planexxx"
    - key: "node.kubernetes.io/kubelet-unreachable"
      operator: "Exists"
      effect: "NoExecute"
    - key: "node-role.kubernetes.io/control-plane"
      operator: "Exists"
      effect: "NoExecute"
    - key: "node.kubernetes.io/network-unavailable"
      operator: "Exists"
      effect: "NoExecute"
    - key: "node.kubernetes.io/kubelet-unreachable"
      operator: "Exists"
      effect: "NoExecute"
```

- This pod needs to use the Service Account created before.

- These requests are then submitted to the Kubernetes API server for processing.

7. Admission Policy Enforcement:

- Upon receiving pod deployment or update requests, the Kubernetes API server invokes the ValidatingAdmissionPolicy. It evaluates the pod's tolerations against the policy's defined CEL expressions, determining the request's compatibility with established configured guidelines.

8. Policy Decision Feedback:

- Should a pod's request contradict the policy, it is promptly rejected. The User/Developer is informed of this decision through an error message that highlights the specific policy violation, akin to:

```
The pods "my-pod" is invalid: : "Pod toleration for 'NoExecute' is not authorized for service account 'special-service-account' in namespace 'default'."
```

This message clearly indicates the failed policy expression, aiding Users/Developers in understanding the reason behind the rejection and guiding them towards necessary adjustments.

- Conversely, if the request aligns with the policy or falls within an exception due RBAC permission grating, the process advances eventually adhering to Kubernetes' standard scheduling steps (or direct pod placement with spec.nodeName).

9. Emergency Overrides and Adjustments:

- For emergency scenarios necessitating temporary deviations from the norm, Cluster Administrators have the option to swiftly allow new tolerations through the RBAC mechanism or adjust the ValidatingAdmissionPolicyBinding resource to unbind the ValidatingAdmissioPolicy (or change the binding rule). This flexibility ensures that critical operations can proceed unhindered, even under exceptional circumstances.

**Note:** While the ValidatingAdmissionPolicy feature is in Tech Preview within OpenShift, an initial step requires Cluster Administrators to enable the TechPreview feature gate on their cluster and subsequently restart the API Servers to facilitate the creation of policies and bindings. It's important to note that activating this feature marks the cluster as non-upgradable. This significant consideration should be carefully weighed when deciding to implement this enforcement approach.

### API Extensions

No new API or fields added. Solution works out of the box, given we ship necessary tolerations for control plane workloads managed by us.

### Topology Considerations

#### Hypershift / Hosted Control Planes

Cluster admins won´t be able to taint control-plane nodes in this scenario. It would not be possible to apply this workflow.

#### Standalone Clusters

This workflow shines here, where cluster admins have full control over their clusters. This workflow is targeting this topology.

#### Single-node Deployments or MicroShift

Tainting a single-node cluster would not make sense, then this workflow is not aimed at those types of topologies.

### Implementation Details/Notes/Constraints

See Alternatives section. The previous discarded approach required actual implementation of a admission plugin/controller or some other way to avoid tolerations with new code provided by us, but the accepted workflow is now available out-of-the-box with ValidatingAdmissionPolicies with CEL (still in tech preview, but available if enabled) combined with the NoExecute taint enforcement. So no real implementation from our side is needed, besides providing the default ValidatingAdmissionPolicy and writing documentation/guidance on how to apply this workflow.

### Risks and Mitigations

- Unintended RBAC permission granting: Administrators or namespace owners might inadvertently give permissions in a way that bypasses the scheduling restrictions, potentially exposing control plane or special nodes to unauthorized workloads.
  - Mitigation: Implement clear policies and documentation around the RBAC that needs to be applied. Standardize a simple workflow, similar to the one mentioned above, so users don't deviate too much, while still allowing them to if they want.
- Complex Policy Management: The creation and maintenance of Validating Admission Policies and Bindings could become complex, especially in large clusters with diverse workloads and multiple special node groups.
  - Mitigation: Similar to above mitigation, we could have KCS articles and documentation with step by step examples for the main control plane protection use-case and possibly an additional one for GPU enabled nodes being protected so users follow those without too much deviation.
- Accidental Eviction of Critical Workloads: The application of NoExecute taints and subsequent enforcement might lead to the accidental eviction of workloads that are critical but were not correctly configured with the necessary tolerations.
  - Mitigation: Prior to applying NoExecute taints, perform a comprehensive review of all workloads running on the nodes to be protected. Ensure that all essential services and components include the correct tolerations. Utilize dry-run or audit modes of the Validating Admission Policy, if available, to assess the impact before advised enforcement.

### Drawbacks

- Increased Complexity for Cluster Administrators: The introduction of NoExecute taints and the requirement to manage Validating Admission Policies and Bindings may increase the complexity of cluster administration. Administrators now need a deeper understanding of how taints, tolerations, and admission policies interact to enforce these constraints.

- Potential for Misconfiguration: The reliance RBAC rules to exempt certain workloads from scheduling restrictions introduces a risk of misconfiguration, either by applying incorrect rules or failing to update rules as policies evolve.

- Risk of Disruption to Existing Workloads: Applying NoExecute taints to nodes could lead to the eviction of existing workloads that do not have the necessary tolerations, potentially disrupting services.

- Limitation on Feedback Detail from Admission Policies: The feedback provided to users when their pods are rejected due to policy violations may not always be detailed or user-friendly, potentially leading to confusion and delays in troubleshooting.

- Challenges with Third-Party Workloads: Ensuring that third-party operators or helm charts comply with the new scheduling restrictions could be challenging, especially if those workloads require updates to include the necessary tolerations.

## Test Plan

The workflow was manually tested as described in the `Workflow Steps` section. Since these will be the manual steps customers/cluster admins would have to follow, it is validated.

For this to work out we need to be sure that all workloads running in control-plane nodes have the relevant tolerations, so a new e2e tests enforcing that will be added to our test suites.

## Graduation Criteria

### Dev Preview -> Tech Preview

The necessary feature for this solution (ValidatingAdmissionPolicy) is already available, even though it is in Tech Preview (it is in beta but disabled by default upstream). The evolution of this solution ties with the evolution of [ValidatingAdmissionPolicy](https://github.com/kubernetes/enhancements/blob/master/keps/sig-api-machinery/3488-cel-admission-control/README.md) and the decisions to graduate it downstream on Openshift.

### Tech Preview -> GA

As soon as ValidatingAdmissionPolicy is graduated to GA we can consider this workflow to be graduated as well.

### Removing a deprecated feature

N/A

## Upgrade / Downgrade Strategy

N/A

## Version Skew Strategy

N/A

## Operational Aspects of API Extensions

N/A

## Support Procedures

### Failure Modes

N/A

## Implementation History

N/A

<!-- Major milestones in the life cycle of a proposal should be tracked in `Implementation
History`. -->

## Alternatives

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
