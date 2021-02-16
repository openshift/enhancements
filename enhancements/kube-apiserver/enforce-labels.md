---
title: enforce-labels
authors:
 - "@mmirecki"
reviewers:
 - "@fedepaol"
approvers:
 - TBD
creation-date: 2020-08-07
last-updated: 2020-08-07
status: implementable
---

# Enforced Labels and Annotations


## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [x] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

This enhancement intends to provide a way to define, per project, labels and annotations that should be enforced in all resources of a given type created in the particular project (v.s. having those possibly  pre-defined in helm chart). The labels could be specified per project, resource type, resource name.

## Motivation

With the deployment of complex applications in multi-cluster and multi-tenant configurations, organizations use labels and annotations to represent complex organizational dependencies, a state in an approval or validation process, or other metadata the organization uses in internal processes.

As telco platforms are still evolving, so are the labels requirements. Those are likely to increase in volume. Managing those through the helm chart is not practical for CNF vendors, as they would need to maintain a separate chart for each provider. Moreover, this would only apply to those vendors that deliver their CNF via helm charts.

As an example, some providers expect CaaS specific labels in order to apply the service proxy configurations. These labels are used in the Service Provider’s environment to inject sidecars for service mesh configuration. Other possible uses can be logging or monitoring.

### Goals

- enforce a per project (namespace) labeling scheme
- ability to configure a cluster-level labeling scheme to enforce on all or a subset of namespaces
- specify the labeling scheme on resource type and resource level
- the labeling should complement installation time (eg. Helm or Operator) labeling scheme
- ensure the labeling scheme is maintained over resource lifecycle (e.g. pod restart)
- allow customer organizations to update the labeling scheme at any time
- relabel existing resources upon labeling scheme update

### Non-goals

- the component is only required by specific customers. As such it is NOT to be shipped with every openshift installation, but will be provided as an optional OLM component.

## Proposed solutions

The prefered solution now is to use Gatekeeper:
https://github.com/open-policy-agent/gatekeeper/

Gatekeeper is the prefered solution. Currently it does not meet all our requirements, but appropriate features are either at design stage, or have been agreed to by Gatekeeper as feasible to add (as opt-in features). An additional bonus is that there are multiple teams interested in having Gatekeeper as part of openshift.
A custom solution was evaluated, but the idea was discarded.


### Gatekeeper

https://github.com/open-policy-agent/gatekeeper/

Gatekeeper is an opensource constraint policy enforcement tool.
So far gatekeeper only provides a validation webhook, but has advanced plans for a mutation webhook. We're in discussion with gatekeeper people (smythe@google.com) about extending this to provide updates to existing resources in reaction to MutationTemplate changes.

PROs:

- advanced plans to provide a mutation webhook implementing a MutationTemplate (design doc [here](https://docs.google.com/document/d/1MdchNFz9guycX__QMGxpJviPaT_MZs8iXaAFqCvoXYQ/edit?ts=5f73fb77#])
- seem to be interested in adding a updates to existing resources in reaction to MutationTemplate changes (but no hard commitment on this)
- a regularly release opensource project with  community backed by companies like google, ms, RH, ...
- multiple Red Hat teams want to have this as part of Openshift

CONs:
- don't provide any functionality we need yet
- reactive resource updates still under design
- alpha version of the mutation webhook only available by the end of the year
- an overkill for our immediate needs, provides a much broader functionality than we need

NOTE: We are only interested in the planned mutating feature of the project.

#### Gatekeeper features required

Two planned gatekeeper features will be used for this enhancement:
- label/annotation enforcement or incoming resources - incoming resources will be inspected and have labels/annotations created if missing. This feature has a [design document](https://docs.google.com/document/d/1MdchNFz9guycX__QMGxpJviPaT_MZs8iXaAFqCvoXYQ/edit?ts=5f73fb77#heading=h.ydl24d47rg98) and developement of the feature has started.
- drift correction - update of existing resources in reaction to policy changes or gatekeeper outages. This feature is planned, but its design has not started yet.


#### Gatekeeper operator

Gatekeeper would be installed using an operator. The operator would not only take care of the installation, but also perform some additional configuration to ensure gatekeeper does not interere with control-plane resources.

There is currently an effort to provide a gatekeeper operator [here](https://github.com/font/gatekeeper-operator). It is planned that the operator will be accepted as part of the open-policy-agent repo.

Gatekeeper inspects resources based on a validating webhook (we assume
the mutation feature will similarly use a mutating webhook). A sample
webhook definition can be found
[here](https://github.com/open-policy-agent/gatekeeper/blob/1de87b6d3c2ed3609e69b789d722e28285873861/charts/gatekeeper/templates/gatekeeper-validating-webhook-configuration-validatingwebhookconfiguration.yaml). The
operator would modify the webhook namespace selector to exclude all
control-plane components. The operator would keep track of the
namespaces and only allow the gatekeeper webhooks access to non
control-plane namespaces. The operator configuration could allow to
configure the namespaces accessible by gatekeeper.

#### Safeguaring control-plane components

Gatekeeper uses a single admission webhook to for constraints enforcement. We assume the same solution will be used for the mutation feature. The control-plane resources can be excluded from gatekeeper interference by specifying an appropriate namespace selector on the webhook.
Gatekeeper would be installed/configured by an operator, which would adjust the webhook namespace selector (based on some GatekeeperInstallConfig CRD).

The webhook namespaceselector allows to filter the included namespaces based on the namespace labels (details available: [here](https://kubernetes.io/docs/reference/access-authn-authz/extensible-admission-controllers/) [here](https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/)) .

Also the default gatekeeper webhook configuration is using
"failurePolicy​: ​Ignore", we assume this would be extended to the
mutation feature (more on this:
[here](https://github.com/open-policy-agent/gatekeeper#admission-webhook-fail-open-status)). Any
resources which would slip into the system unmodified would later be
corrected using the additional relabeling feature (implemented in the
second phase).


#### Performance

Multi-pod deployments of Gatekeeper is already implemented. This will allow to run multiple pods handling the mutating webhook to allow for better performance, better latency and resilience.
The relabeling feature would preferably be implemented to run on multiple pods as well (daemonset?). The relabeling feature will most probably be implemented as a controller watching for policy changes, and at system startup to fix any drift created during gatekeeper downtime. Note that the design of the relabeling feature has not started yet, so no details are available as of now.


#### Risk assesment

There are multiple risks involved with delivering the feature using gatekeeper:

- ETA: the time frame for the first part of the feature (mutating webhook) is quite tight, even though there are currently 3 people (different projects) commited to work on this feature
- We have no maintainers on the project, which could be a bottleneck with getting the feature code in, or getting some required features in.
- The requirements and design of the second part of the feature (drift correction) is nowhere near ready. There are still open questions on how do resolve issues like constraint conflicts or convergence.
- The upstream code might not be of the required quality by the required time.

#### API

The detailed API is still under discussion by the Gatekeeper community. A design proposal accepted in a general form by the gatekeeper community can be found [here](https://docs.google.com/document/d/1MdchNFz9guycX__QMGxpJviPaT_MZs8iXaAFqCvoXYQ/edit?ts=5f73fb77#).
Once finalized, there should be little risk of the API changing much after the alpha version.

NOTE: This feature will be kept in dev preview until the API is out of alpha. Moving to to tech preview or GA can be considered only when the API reaches at least beta.

### Other projects

Kyverno (https://github.com/nirmata/kyverno) was also looked at but was mostly rejected. It does not provide all the needed functionality, the community is mostly backed by one company and seems less lively than gatekeeper and it seems to be in bad technical shape (getting it to run was quite problematic as oposed to gatekeeper).

## Graduation Criteria

### Dev Preview -> Tech Preview

- Ability to utilize the enhancement end to end
- End user documentation, relative API stability
- Sufficient test coverage
- At least v1beta1 API level

### Tech Preview -> GA

- More testing (e2e)
- Sufficient time for feedback
- Available by default
- At least v1beta1 API level
