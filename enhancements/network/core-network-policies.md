---
title: core-nework-policies
authors:
  - "@bbennett"
reviewers:
  - TBD
approvers: 
  - TBD
api-approvers:
  - No new API
tracking-link:
  - "https://issues.redhat.com/browse/OCPSTRAT-819"
creation-date: 2024-08-13
last-updated: 2024-11-14
---

# Network Policies for OpenShift Components


## Summary

Kubernetes NetworkPolicy objects allow control over what traffic can
enter and leave a pod.  Currently, most OpenShift operators do not
install policies for their namespaces.  This is a vital tool for
network security that we should use.

## Motivation

OpenShift should use the OpenShift platform capabilities to secure the
OpenShift platform workloads (including layered products that are
considered part of the platform).  It should both restrict what can
talk to our pods, but also what our pods can talk to, so that a
compromised pod can not be used as a springboard for other attacks.


### User Stories

- As an administrator, I want to have confidence that the cluster
  components are as secure as possible, and are not likely to be an
  attack vector so that I can trust the platform.

- As an administrator, I need a way to be able to disable the
  OpenShift policies in case something goes wrong so that I can
  operate my cluster as it was before while the policies are being
  fixed

- As an administrator, I need to be able to override specific
  OpenShift policies to be more restrictive so that I can satisfy my
  security requirements.  For instance there may be cases where a core
  component may be able to call webhooks, but I do not want to use
  them on the cluster, or have a specific list of endpoints.  I may
  want to override our permissive list to make it more restrictive.

- As an OpenShift release manager, I want to know that all cluster
  namespaces have policies applied so that I can determine if we are
  conforming with the policies outlined in this document.

- As an OpenShift support engineer, I need to know if network policy
  is breaking a service (perhaps not even the service that surfaces
  the error) before I spend a lot of time debugging things so that I
  can efficiently diagnose the issue.


### Goals

- Make it easy for operators to add network policy support

- Document how to override OpenShift policies

- Audit the traffic to ensure that a policy is not breaking a service
  and report it in a must-gather

- Audit the policies for compliance

- Ensure all OpenShift namespaces have a default deny-all policy for
  both egress and ingress defined, and identify those that do not
  (including `openshift-` namespaces managed by a non Red Hat
  operator)


### Non-Goals

- We are not considering requiring the definition of Admin Network
  Policy for components.  But the documentation of the network
  connections that a namespace uses should make future development of
  Admin Network Policy easier should we desire it.  We will be using
  Admin Network Policy to allow cluster admins to override our
  namespace Network Policies if needed.  One of the reasons for not
  requiring core components to have Admin Network Policy is that then
  the operational complexity of managing them across upgrades would
  need to move to a third-party operator.  Similarly, it is better to
  have the network policies near the code that uses them so that the
  team responsible for the objects develops and maintains the network
  policies.

- Adding policies for operators not created by Red Hat


## Proposal

1. Change the namespace admission controller so all namespaces with an
   openshift- prefix (since those are special anyway) are labeled with
   the `security.openshift.io/openshift-namespace` label so **that network policy can address
   them**

2. Change the cluster-network-operator to apply the same label to all
   openshift- prefix namespaces to handle the upgrade case (since the
   webhook may not fire)

3. Work out how we can identify connections blocked by policy in
   OpenShift namespaces (either egress or ingress) and work out how to
   include it in a must-gather

4. Work with a team to audit their traffic and document the expected
   flows. Use ACS where helpful

5. Develop the appropriate network policy for that team’s
   namespace. Use ACS network policy generation tools where helpful.

6. **Work out how to deploy it with their operator**

7. **Write up a guidelines and FAQ doc**

8. Iterate over all of the OpenShift namespaces and repeat the process

9. Consider extending the operator in step 2 to check for conformance
   with network policy and flag non-compliant namespaces (taint them
   somehow?)  They should at least have a wildcard match rule to cause a
   default deny

10. Eventually consider adding a default-deny rule to admin network
    policy for OpenShift namespaces that runs after network policy has a
    chance to run (but that may break things installed from the operator
    catalog into the openshift namespace)

### Workflow Description

**Cluster administrator** is a human user in charge of running a cluster.

**OpenShift developer** is a human responsible for creating and maintaining OpenShift.

**Third-Party developer** is a human who creates an operator that
  creates an `openshift-` namespace.  This is assumed to be for
  something extending OpenShift platform capabilities.

**Namespace Admission Controller** is a function in the OpenShift API
  server that runs when a namespace is created, changed, or deleted.

**Cluster Network Operator** is the OpenShift operator responsible for
  setting up the OpenShift networking software based on cluster
  configuration.


#### Applying the label at runtime

1. When a namespace is created or changed the Admission Controller
   will be called with the namespace object.

2. The Namespace Admission Controller will look at the namespace name

3. If it starts with `openshift-` it will be treated as an OpenShift
   namespace (since that namespace prefix has special meaning already
   elsewhere in the code)

  1. The namespace object will be changed to have a label applied with
     the name `security.openshift.io/openshift-namespace` and the value ''

4.  If it does not start with `openshift-` then any
    `security.openshift.io/openshift-namespace` label will be stripped out and can not be set


#### Applying the label at upgrade

1. When an upgrade happens, a namespace may not be changed, so the
   Admission Controller would not be able to apply the label

2. When the Cluster Network Operator starts it would:

   1. Loop over all namespaces

   2. If it sees a namespace starting with `openshift-`

       1. The namespace object will be changed to have a label applied
          with the name `security.openshift.io/openshift-namespace` and the value ""
       
   3. Otherwise it will strip the `security.openshift.io/openshift-namespace` label

   TODO: Decide if this is the correct behavior... do we want to allow
   namespaces to opt-in to being part of the platform.  I do not think
   there is a security issue since we are just adding restrictions,
   but that needs more consideration.  The advantage would be for
   things like ACS that do not install into an `openshift-` namespace.


#### Opting Into a Default Deny for OpenShift

1. The Cluster Administrator can deploy an Admin Network Policy we
   will document and test

2. The policy will change the default for all namespaces with the
   label described above so that ingress and egress traffic will fail if
   there is no explicit Network Policy defined for a pod in those
   namespaces

#### Setting a Global Default Deny

1. The Cluster Administrator can deploy an AdminNetworkPolicy we
   will document and test

2. The policy will change the default for all namespaces in the
   cluster so that ingress and egress traffic will fail if there is no
   explicit Network Policy defined for a pod in those namespaces


#### Removing Policy Restrictions for OpenShift

1. The Cluster Administrator can deploy an AdminNetworkPolicy we
   will document with a public KCS and test

2. The policy will bypass Network Policy for all namespaces with the
   label described above so that ingress and egress traffic will succeed
   without restriction (this is the current behavior)

3. This is only intended to be an escape hatch that support might need
   to deploy until an appropriate targeted policy can be applied


#### Developing and Shipping Network Policy

1. OpenShift developers and Third-Party developers will both need to
   create network policies for the pods running in their namespaces

2. Developing policies:

    1. Document what traffic flows (ingress and egress) need to be
       allowed for pods in the namespaces
    
    2. The ACS product can be used to analyze workloads in a cluster to
    
        1. understand which connections have live traffic (active connections)
        
        2. understand which connections have no traffic (hinting that
           these connections should be blocked by network policy)
        
        3. Suggest (generate) a network policy based on the observed traffic baseline
        
    3. ACS can also be used to analyze a folder with all of the
       namespace YAML resources to heuristically try to figure out who is
       trying to talk to whom and mint tight network policies to reflect
       that model.
    
    4. The Network Observability tool can be used to detect when a network policy blocked traffic
    
3. Auditing policies:

    5. Someone familiar with network policies should work with the
       team to ensure that the policies are as tight as is possible given
       the expected traffic
    
4. Deploying policies:

    6. The network policies need to be deployed by the operator in to the operand namespaces


### API Extensions

This extends the API server by changing the Admission Controller on
namespace objects so that the label `security.openshift.io/openshift-namespace` is applied
or removed depending on whether the namespace name starts with
`openshift-`.

Other than that, there is no other API change.


### Topology Considerations


#### Hypershift / Hosted Control Planes

Are there any unique considerations for making this change work with Hypershift?

No. 


#### Standalone Clusters

Is the change relevant for standalone clusters?

No.


#### Single-node Deployments or MicroShift

How does this proposal affect the resource consumption of a
single-node OpenShift deployment (SNO), CPU and memory?

No change, other than the creation of some additional objects in the API server.

How does this proposal affect MicroShift? For example, if the proposal
adds configuration options through API resources, should any of those
behaviors also be exposed to MicroShift admins through the
configuration file for MicroShift?

No impact.


### Implementation Details/Notes/Constraints

What are some important details that didn't come across above in the
**Proposal**? Go in to as much detail as necessary here. This might be
a good place to talk about core concepts and how they relate. While it
is useful to go into the details of the code changes required, it is
not necessary to show how the code will be rewritten in the
enhancement.


#### DNS

Almost every pod will need to have an egress policy to allow DNS.
Would it be better to have one policy that can be enabled by a label?
Or just have an AdminNetworkPolicy that just allows it.


#### Host Network Pods

Network Policy does not apply to pods that are running in the host
network.  However, other pods that expect to receive connections from
the host network on nodes will need
[policy](https://docs.openshift.com/container-platform/4.17/networking/network_security/network_policy/about-network-policy.html)
to allow it.


#### API Server

The API servers have to be specified by IP address, labels do not
work.  We need a policy that is maintained by an operator that has a
well-known label that pods can have applied to them.


#### Monitoring


#### Logging


#### Operators

Open issue: Who manages the operator namespaces?  They obviously need
API server access to make the other objects, but who defines them.


### Risks and Mitigations


#### Policies Too Restrictive

If we define policies that are too restrictive, valid flows can be
blocked.  In some cases, the product allows for user-configured
endpoint addresses, so we could have missed that in product testing.

When this issue is identified, it can quickly be tested by using an
Admin Network Policy on the namespace (or globally) to temporarily
allow all traffic.  If that mitigates the issue, then a more targeted
Admin Network Policy can be used to override our policies until a fix
can be delivered in a z-stream.


#### Challenges Debugging and Supporting

When a network policy blocks traffic, it may be blocking traffic of an
internal service communication causing a mysterious failure message to
the user.

You can use the Network Observability tool to audit network policy
blocks.  There should not be any in the OpenShift namespaces.

We also need to provide this information to the support organization
in a must-gather.  We will augment the must-gather tool to include
information about network policy blocks in OpenShift namespaces.


### Drawbacks

None foreseen.


## Test Plan

- The existing e2e and integration tests should catch network policy errors where they are too restrictive

- We will need to add new tests to ensure that all `openshift-` namespaces have the default deny policy defined.

- We should add a test that runs alongside the existing e2e tests to
  monitor for connections being blocked by network policy in openshift
  namespaces.  There should be no such failures in normal operation.

- We need to have tests for the new functionality in the admission controller and for the cluster network operator


## Graduation Criteria

### Dev Preview -> Tech Preview

We do not anticipate a dev preview or tech preview.

### Tech Preview -> GA

- Cluster Network Operator changes complete:
- Namespace Admission controller change complete
- Must-gather able to get all recent network policy connection blocks in `openshift-` namespaces
- Customers must be able to see network policy connection blocks using the observability tools
- The test changes as described above must be complete
- Documentation will be updated to describe how to:
    - Override our network policies using admin network policy
    - Apply a default-deny rule to all OpenShift namespaces
    - Apply a default-deny rule to all cluster namespaces
    - Disable our policies in a KCS

### Removing a deprecated feature

If the feature is deprecated, then we will need to strip the labels we
added and remove the code.  The policies developed for the namespaces
should remain.

## Upgrade / Downgrade Strategy

#### Upgrades:

1. The new API server will roll out and will start labeling namespaces

2. The Cluster Network Operator will be upgraded and upon restart it
will scan all namespaces and fix the labels to handle namespaces that
have not changed

Since nothing needs to use the labels immediately, there is no concern about ordering and racing, as long as the change happens during the upgrade.

At which point, the cluster administrator can use the new labels to easily apply default policies to all of the OpenShift namespaces.	


#### Downgrades:



1. The cluster administrator must remove any deny policies that they
   created using the default labels because…

2. The labels will remain present upon downgrade… nothing will remove them

3. But the operators may remove the network policies they manage from
   the namespaces and that may cause traffic to be blocked if the
   administrator changed the default policy to deny for an OpenShift
   namespace


## Version Skew Strategy

- Version skew is managed by having the operator responsible for each
  namespace deploy the policies needed for that software

- If the traffic flows change substantially in a namespace, it may
  have to define policies for the old flow and the new, but that would
  be atypical

## Operational Aspects of API Extensions

Not applicable.  There will be no API change.


## Support Procedures

Describe how to:

* detect the failure modes in a support situation, describe possible symptoms (events, metrics, alerts, which log output in which component) \
 \
 Examples: \

    * If the webhook is not running, kube-apiserver logs will show errors like "failed to call admission webhook xyz".
    * Operator X will degrade with message "Failed to launch webhook server" and reason "WehhookServerFailed".
    * The metric `webhook_admission_duration_seconds("openpolicyagent-admission", "mutating", "put", "false")` will show >1s latency and alert `WebhookAdmissionLatencyHigh` will fire.
* disable the API extension (e.g. remove MutatingWebhookConfiguration `xyz`, remove APIService `foo`) \

    * What consequences does it have on the cluster health? \
 \
 Examples: \

        * Garbage collection in kube-controller-manager will stop working.
        * Quota will be wrongly computed.
        * Disabling/removing the CRD is not possible without removing the CR instances. Customer will lose data. Disabling the conversion webhook will break garbage collection.
    * What consequences does it have on existing, running workloads? \
 \
 Examples: \

        * New namespaces won't get the finalizer "xyz" and hence might leak resource X when deleted.
        * SDN pod-to-pod routing will stop updating, potentially breaking pod-to-pod communication after some minutes.
    * What consequences does it have for newly created workloads? \
 \
 Examples: \

        * New pods in namespace with Istio support will not get sidecars injected, breaking their networking.
* Does functionality fail gracefully and will work resume when re-enabled without risking consistency? \
 \
 Examples: \

    * The mutating admission webhook "xyz" has FailPolicy=Ignore and hence will not block the creation or updates on objects when it fails. When the webhook comes back online, there is a controller reconciling all objects, applying labels that were not applied during admission webhook downtime.
    * Namespaces deletion will not delete all objects in etcd, leading to zombie objects when another namespace with the same name is created.


## Alternatives

Similar to the `Drawbacks` section the `Alternatives` section is used
to highlight and record other possible approaches to delivering the
value proposed by an enhancement, including especially information
about why the alternative was not selected.

Ideas:

- Log messages from ANP (go into ovn pod logs, see if better way)

- Admin policies for allowing access to DNS from everything

- Admin policies for allowing access to api server with label (so openshift && apiserver)

- May cause issues for SD, make sure we talk to them


## Open Issues

- What should we enforce on the labels?  If there is an openshift-
  namespace, it will get the label.  But should we strip it otherwise?
  Or should we allow namespaces to self identify as a cluster
  component?

- How do the Operators themselves get policies?  Do they just get a
  stock one to allow DNS and API access and nothing else?

- Should we allow everything to get DNS access?  Is there ever a need
  not to?

- Need to work out a good way to say “can not talk to pods, only to
  external”.  Right now it is ugly, it needs an ipblock rule with an
  ‘except’ clause for all of the pod CIDRs… so an operator that needs
  that rule needs to read the config and maintain the policy.

  Or we may define a global ANP that allows namespaces labelled with
  `security.openshift.io/openshift-namespace` to label pods that need
  this rule applied.

