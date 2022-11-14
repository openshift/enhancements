---
title: pluggable-konnectivity
authors:
  - 2uasimojo
reviewers: # Include a comment about what domain expertise a reviewer is expected to bring and what area of the enhancement you expect them to focus on. For example: - "@networkguru, for networking aspects, please look at IP bootstrapping aspect"
  - csrwng
  - zanetworker
  - sjenning
  - slaviered
  - xiangjingli
approvers:
  - csrwng
  - zanetworker
  - sjenning
api-approvers: # In case of new or modified APIs or API extensions (CRDs, aggregated apiservers, webhooks, finalizers). If there is no API change, use "None"
  - None
creation-date: yyyy-mm-dd
last-updated: yyyy-mm-dd
tracking-link: # link to the tracking ticket (for example: Jira Feature or Epic ticket) that corresponds to this enhancement
  - https://issues.redhat.com/browse/OCPPLAN-5771
see-also:
replaces:
superseded-by:
---

# Pluggable Konnectivity

## Summary

For worker-less deployments, hypershift deploys a konnectivity-server and -agent to facilitate seamless communication *from* the hosted control plane *to* the services running on its behalf in the hosting cluster.
At the time of this writing, the list of services thus supported is static and hardcoded.

This enhancement proposes a mechanism to facilitate adding any service to konnectivity.

## Motivation

One use case for hypershift -- particularly with zero-worker hosted control planes -- is to facilitate scaling of workloads, mitigating etcd limits by "offloading" kubernetes objects to etcds in one or more hosted control planes, while running the relevant workloads (e.g. controllers) on the hosting cluster.
In such a scenario, communication *from* the hosting cluster *to* the HCP is a simple matter of creating a client with a kubeconfig pointing to the HCP.
Communication in the other direction is almost never an issue, because any workload running "in" the HCP is actually running on the hosted cluster anyway.

A notable exception is admission webhooks: The configuration (Validating- or MutatingWebhookConfiguration) on the HCP is acted on by the kube-apiservice running *on behalf of* the HCP but *actually on* the hosting cluster, which must send the requests to a Service that is similarly running on the hosting cluster.
At the time of this writing, the only (known) way to plug an admission webhook into this configuration includes standing up a separate konnectivity-agent with the ClusterIP of the Service pointing to the webhook server.

It would be preferable to be able to plug such a Service into the existing konnectivity-agent already deployed by hypershift.
- Save duplication: In the workaround, the konnectivity-agent deployment is identical (except for IPs) to hypershift's.
- Save cluster resources: One konnectivity-agent workload instead of two.
- Save etcd space: Particularly in use cases where etcd slots are at a premium, we can save three objects (Deployment, ReplicaSet, Pod) by consolidating.
- Save devtest cycles and code: Developers don't need to figure out how to run a konnectivity-agent, or include one in their project.

### User Stories

1. As a developer, I would like to be able to run a Service on behalf of a zero-worker hosted control plane without creating a separate konnectivity-agent.
   As a specific example: I would like to be able to configure a validating webhook service to gate objects destined for the HCP's etcd.
   Since the service itself must run on the hosting cluster, network proxying is necessary to facilitate the communication between the HCP's kube-apiserver and the admission pod.

### Goals

See [User Stories](#user-stories)

### Non-Goals

The inverse of [User Stories](#user-stories)

## Proposal

The konnectivity-agent brokers network traffic for IP addresses provided to its executable via the `--agent-identifiers` argument, e.g. `ipv4=172.30.127.30&ipv4=172.30.78.219&ipv4=172.30.207.73`.
To achieve the desired function, we simply need to add more IP addresses to this list.
The IPs in question exist on the Service object that exposes the pod on the internal network.

### Workflow Description

The proposed mechanism is:
- The creator of the Service includes a label with a well-known key, such as `hypershift.openshift.io/konnectivity-service-for-hcp`, whose value is the name of the HostedControlPlane for which traffic is to be brokered.
- The hostedcontrolplane_controller already `Watch()`es Services owned by the HostedControlPlane; add a `Watch()` for Services with this label, enqueueing the corresponding HostedControlPlane.
- When reconciling a HostedControlPlane, the controller `List()`s Service objects in the HCP's namespace with the label key and value corresponding to the HCP's name.
  For any such Services found, their `spec.clusterIP` is added to those to be passed to `--agent-identifiers`.
  The konnectivity-agent is redeployed if necessary (if it has changed).

### API Extensions

This enhancement does not propose any API extensions.

### Risks and Mitigations

Q (@zanetworker): Does this mean I am able to inject any IPs? especially as a malicious actor for my malicious "webhook" and forward requests there? With static IPs we can police traffic to only known IPs for OAuth and aggregated API servers.

A (@2uasimojo): Good question. Someone more network/security-savvy would need to weigh in, but here's my take:
- The malicious actor would need to have CRUD RBAC to Services in the HostedControlPlane's namespace. Doesn't that theoretically allow them to hijack existing IPs in much the same (hypothetical) way? (Granted, they would either need to be able to pause HCP reconciliation as well, or jump in at exactly the right moment before the controller can undo their change.)
- The network proxy only forwards traffic. Auth(n|z) setup is still necessary on both ends. If the malicious user has enough access to configure CAs and certs on the hosting cluster and the HCP (or configure unauthenticated traffic?) then would this feature really be providing them anything additional?
- If the user had the ability to stand up a malicious pod on the hosting cluster... they would be able to stand up a konnectivity-agent pod to forward the traffic explicitly. This is how the current (non-malicious :) ) use cases intend to work around the limitation this RFE is proposing to mitigate.

### Drawbacks

There are currently no known drawbacks.

## Design Details

### Open Questions [optional]

- Is Service sufficient? (We can always add more types later.)
- Is it acceptable to restrict to Services in the HCP's namespace?
- IPv6?
 
### Test Plan

- Unit tests proving appropriately-configured Services get their IPs into the `--agent-identifiers` argument of the konnectivity-agent deployment.
- Integration testing proving that a Service works with the proper labeling (and not without it) without a separate konnectivity-agent.

### Graduation Criteria

Cap & gown must be navy blue. No restrictions on undergarments.

#### Dev Preview -> Tech Preview

Nothing here

#### Tech Preview -> GA

Nothing here

#### Removing a deprecated feature

Nothing here

### Upgrade / Downgrade Strategy

Nothing here

### Version Skew Strategy

Nothing here

### Operational Aspects of API Extensions

Nothing here

#### Failure Modes

1. A misconfigured Service should be ignored.
Misconfigurations could include:
- Service with no label
- Label value not matching the HCP

2. Your network doesn't work.

#### Support Procedures

Failure mode 1: Controller is reconciling a Service it should be ignoring.
- Engage engineering team. Watches should only be triggering on appropriate Services.

Failure mode 2:
- Does your Service have a `spec.clusterIP`?
- Does the konnectivity-agent's `--agent-identifiers` arg have your IP in it? Is the syntax correct?
- Can you make it work with a separate konnectivity-agent?

## Implementation History

- 20221114: Proposed

## Alternatives

- Stand up your own konnectivity-agent.

## Infrastructure Needed [optional]

None
