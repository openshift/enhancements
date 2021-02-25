---
title: Audit Policy Configuration
authors:
  - "@sttts"
reviewers:
  - "@mfojtik"
  - "@soltysh"
  - "@deads2k"
  - "@stlaz"
approvers:
  - "@mfojtik"
  - "@mcurry"
creation-date: 2020-03-24
last-updated: 2020-05-14
status: provisional
see-also:
replaces:
superseded-by:
---

# Audit Policy Configuration

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Open Questions

## Summary

This enhancement describes a high-level API in the `config.openshift.io/v1` API group to configure the audit policy for the
API servers in the system. The audit configuration will be part of the [`APIServers`](https://github.com/openshift/api/blob/master/config/v1/0000_10_config-operator_01_apiserver.crd.yaml) resource.
It applies to all API servers at once.

The API is meant to enable customers with stronger audit requirements than the average customer to increase the depth
(from _metadata_-only level, over _request payloads_ to _request and response payload_ level) of audit
logs, accepting the increased resource consumption of the API servers.

This API is intentionally **not about filtering events**. Filtering is to be done via an external mechanism (post-filtering).
It was proven through performance tests (compare alternatives section) that this trade-off is acceptable with small two-digit percent overhead.

The API is not meant to replace the [upstream dynamic audit](https://github.com/kubernetes/enhancements/blob/f1a799d5f4658ed29797c1fb9ceb7a4d0f538e93/keps/sig-auth/0014-dynamic-audit-configuration.md) API
now and in the future. I.e. the API of this enhancement is only about the master node audit files on disk, not about webhooks or
any other alternative audit log sink.

## Motivation

The [advanced audit mechanism](https://kubernetes.io/docs/tasks/debug-application-cluster/audit/) was added
to kube-apiserver (and other API servers) many versions ago. There are two configuration mechanisms for the
audit policy:

1. [**dynamic audit**](https://github.com/kubernetes/enhancements/blob/f1a799d5f4658ed29797c1fb9ceb7a4d0f538e93/keps/sig-auth/0014-dynamic-audit-configuration.md): an alpha API which lets the
user to create [`AuditSink`](https://github.com/kubernetes/api/blob/1fc28ea2498c5c1bc60693fab7a6741b0b4973bc/auditregistration/v1alpha1/types.go#L65) objects in the cluster with webhook backends which will receive the events.
2. [**static file-based policies**](https://kubernetes.io/docs/tasks/debug-application-cluster/audit/#audit-policy): policies in the master host file system passed via flags to the API servers.

The former is an alpha API supposed to replace the latter file-based policies. I.e. the latter is supposed
to go away eventually.

The former policies are very restricted in its current form by only allowing to set
- the [**audit level**](https://github.com/kubernetes/api/blob/1fc28ea2498c5c1bc60693fab7a6741b0b4973bc/auditregistration/v1alpha1/types.go#L102) of events (how much is logged, e.g. meta data only or complete requests or responses)
- the [**stages**](https://github.com/kubernetes/api/blob/1fc28ea2498c5c1bc60693fab7a6741b0b4973bc/auditregistration/v1alpha1/types.go#L106) audit events are created in (request received, response started, response finished).

The [latter policies](https://github.com/kubernetes/apiserver/blob/822585c65b3829cc20fbcbe76481f7413d5fd98e/pkg/apis/audit/v1/types.go#L153) are
- full-featured
- with an [advanced rule system](https://github.com/kubernetes/apiserver/blob/822585c65b3829cc20fbcbe76481f7413d5fd98e/pkg/apis/audit/v1/types.go#L163) with complex semantics.

In OpenShift 3.x the file-based policies were customizable by customers. In 4.x, with its unclear future and its complexity involved
we hesitate to go the same route: if the file-based API goes away upstream, it is infeasible to patch it back into our OpenShift API server
binaries without major effort.

From the requirements we get from customers, it is apparent that the file-based policies are far too low-level, and
too complex to get right (customer policies we see are insecure very often because users don't get the list of security-sensitive
resources right). Hence, this enhancement is about

- a high-level API
- which will satisfy the needs of most customers with strong regulatory or security requirement,
  i.e. it allows them to increase audit verbosity and provides the specific policy required by a  customer for a `UserRequests` policy for logging of the `system:authenticated:oauth` group.
- without risking to log security-sensitive resources
- and it is feasible to maintain the feature in the future when the dynamic audit API takes over.

With performance tests we have proven that there is no strong reason to pre-filter audit logs before
writing them to disk. Filtering can be done by custom solutions implemented by customers, or by
using external log processing systems. Hence, we strictly separate filtering from setting the audit log
depth using the new API.

### Goals

1. add an abstract audit policy configuration to
   `apiservers.config.openshift.io/v1` with few predefined policies
   - starting with different audit depths policies only,
   - but stay extensible in the future if necessary.
2. applied by kube-apiserver, openshift-apiserver (and soon oauth-apiserver).

### Non-Goals

- configuration of audit backends like webhooks
- configuration of file logger parameters
- configuration of filtering (like by resource, API group, namespaces, etc.)
- configuration of what we log in each event (other than the depth).

## Proposal

We propose to add an `audit.profile` field to `apiservers.config.openshift.io/v1` which selects the
audit policy to be deployed to all OpenShift-provided API servers in the cluster. From the beginning
we provide the following profiles:

- `Default`: this is the existing default policy. `Default` is also the default value in the `APIServer` custom resource:

  ```yaml
    apiVersion: audit.k8s.io/v1beta1
    kind: Policy
    rules:
    # Don't log requests for events
    - level: None
      resources:
      - group: ""
        resources: ["events"]
    # Don't log oauth tokens as metadata.name is the secret
    - level: None
      resources:
      - group: "oauth.openshift.io"
        resources: ["oauthaccesstokens", "oauthauthorizetokens"]
    # Don't log authenticated requests to certain non-resource URL paths.
    - level: None
      userGroups: ["system:authenticated", "system:unauthenticated"]
      nonResourceURLs:
      - "/api*" # Wildcard matching.
      - "/version"
      - "/healthz"
      - "/readyz"
    # Log the full Identity API resource object so that the audit trail
    # allows us to match the username with the IDP identity.
    - level: RequestResponse
      verbs: ["create", "update", "patch"]
      resources:
      - group: "user.openshift.io"
        resources: ["identities"]

    # Here the tail begins which differs per profile:

    # A catch-all rule to log all other requests at the Metadata level.
    - level: Metadata
      # Long-running requests like watches that fall under this rule will not
      # generate an audit event in RequestReceived.
      omitStages:
      - RequestReceived
   ```

- `WriteRequestBodies`: this is like `Default`, but it logs request and response HTTP payloads for write requests (create, update, patch).
- `AllRequestBodies`: this is like `WriteRequestBodies`, but also logs request and response HTTP payloads for read requests (get, list).
- `UserRequests`: this logs the requests from users in the `system:authenticated:oauth` user groups and logs requests for (create, delete).

All of the profiles have in common that security-sensitive resources, namely

- `secrets`
- `routes.route.openshift.io`
- `oauthclients.oauth.openshift.io`

are never logged beyond metadata level, and

- `oauthaccesstokens.oauth.openshift.io`
- `oauthauthorizetokens.oauth.openshift.io`.

are not logged at all (because even their names are security-sensitive).

Note: this is in line with [etcd-encryption enhancement](https://github.com/openshift/enhancements/blob/master/enhancements/kube-apiserver/encrypting-data-at-datastore-layer.md#proposal)
with the exception that payloads of configmaps are also audit logged in `WriteRequestBodies` and `AllRequestBodies` policies.

Any kind of further, deeper policy configuration, e.g. that filters by API groups, user agents or namespaces
is explicitly not part of this proposal and requires work on the [upstream dynamic audit](https://github.com/kubernetes/enhancements/blob/f1a799d5f4658ed29797c1fb9ceb7a4d0f538e93/keps/sig-auth/0014-dynamic-audit-configuration.md)
mechanism and its promotion to beta eventually, or a post-processing mechanism of the audit logs.

### Logging of Token with Secure OAuth Storage

Starting with OpenShift 4.6, new `oauthaccesstokens` and `oauthauthorizetokens` are not sensitive anymore because their names are hashed with sha256 (and prefixed with `sha256~`).
Hence, starting from 4.6, new clusters audit log `authaccesstokens` and `oauthauthorizetokens` via the additional rule for the default policy:

```yaml
# Log the full Identity API resource object so that the audit trail
# allows us to match the username with the IDP identity.
- level: RequestResponse
  verbs: ["create", "update", "patch", "delete"]
  resources:
  - group: "user.openshift.io"
    resources: ["identities"]
  - group: "oauth.openshift.io"
    resources: ["oauthaccesstokens", "oauthauthorizetokens"]
```

and by removing the exception for these resources in the `WriteRequestBodies` and `AllRequestBodies` policies.
This will extend the existing audit policies to log the creation and deletion of oauthaccesstokens which corresponds to successful logins and logouts.
New cluster deployed with 4.6 or later are identified through the `oauth-apiserver.openshift.io/secure-token-storage: true` annotation
on the `apiservers.config.openshift.io/v1` resource. Old cluster upgraded from 4.5 or older, don't have this annotation and hence do not
audit log `authaccesstokens` and `oauthauthorizetokens`, not even on metadata level as it is not known whether old, non-sha256 hashed tokens
are in the system. In 4.8 or later, it is planned to remove old, non-sha256 tokens, forbid creation of new non-sha256 tokens, and add
the annotation to switch over to the extended policies.  

### User Stories

#### Story 1

As an average customer, I want to have a default audit policy which does not cause
noticeable resource overhead (CPU, memory and IO).

#### Story 2

As any customer, I don't want to leak sensitive information of resources like secrets,
oauth tokens, etc. in audit logs.

#### Story 3

As a security and regulatory demanding customer, I want to log **every** non-sensitive resource
change up to request payload level detail, but accept increased resource usage.

#### Story 4

As an even more security and regulatory demanding customer, I want to log **every** non-sensitive request
both for read **and** for write operations, but also accept even more increased resource usage.

### Implementation Details/Notes/Constraints

The proposed API looks like this:

```yaml
kind: APIServer
apiVersion: config.openshift.io/v1
spec:
  ...
  audit:
    profile: Default | WriteRequestBodies | AllRequestBodies | UserRequests
```

In the future, we could add more, even more detailed profiles of logging responses, if customers need this:

```yaml
    profile: ... | Regulation12345
```

But we are not going to add these without a strong business case.

### Risks and Mitigations

- going beyond the proposed profiles, e.g. by filtering of API groups, user agents or namespaces, yields the risk that the
  defined semantics do not map to the future [upstream dynamic audit](https://github.com/kubernetes/enhancements/blob/f1a799d5f4658ed29797c1fb9ceb7a4d0f538e93/keps/sig-auth/0014-dynamic-audit-configuration.md)
  API types. In the file-based API of kube-apiserver, which implements powerful, but controversial rule behaviour, this kind
  filters are possible, but it also turned out as the biggest blocker for moving it towards the dynamic variant as part of
  the Kubernetes REST API. We cannot and must not follow this failure as it is big risk to the maintainability and future
  feasibility of this OpenShift API.

## Design Details

The profile defined in the `APIServer` singleton instance named `cluster` will be translated into
the respective file-based audit policy of

- kube-apiserver
- openshift-apiserver
- oauth-apiserver

by their respective operators.

The profile translate like this:

- `Default` translates to the policy given above.
- `WriteRequestBodies` translates to the following tail (with the head the same as the default policy):

  ```yaml
      ... # common head

      # exclude resources where the body is security-sensitive
      - level: Metadata
        resources:
        - group: "route.openshift.io"
          resources: ["routes"]
        - resources: ["secrets"]
      - level: Metadata
        resources:
        - group: "oauth.openshift.io"
          resources: ["oauthclients"]
      # log request and response payloads for all write requests
      - level: RequestResponse
        verbs:
        - update
        - patch
        - create
        - delete
        - deletecollection
      # catch-all rule to log all other requests at the Metadata level.
      - level: Metadata
        # Long-running requests like watches that fall under this rule will not
        # generate an audit event in RequestReceived.
        omitStages:
        - RequestReceived
  ```
- `AllRequestBodies`

    ```yaml
        ... # common head

        # exclude resources where the body is security-sensitive
        - level: Metadata
          resources:
          - group: "route.openshift.io"
            resources: ["routes"]
          - resources: ["secrets"]
        - level: Metadata
          resources:
          - group: "oauth.openshift.io"
            resources: ["oauthclients"]
        # catch-all rule to log all other requests with request and response payloads
        - level: RequestResponse
    ```
  
With secure OAuth storage in-place (see "Logging of Token with Secure OAuth Storage" section) and
therefore the `oauth-apiserver.openshift.io/secure-token-storage: true` annotation on the `apiservers.config.openshift.io/v1` resource,
these policies are extended by allowing logging of `oauthaccesstokens` and `oauthauthorizetokens`.

### Test Plan

**Note:** *Section not required until targeted at a release.*

Consider the following in developing a test plan for this enhancement:
- Will there be e2e and integration tests, in addition to unit tests?
- How will it be tested in isolation vs with other components?

No need to outline all of the test cases, just the general strategy. Anything
that would count as tricky in the implementation and anything particularly
challenging to test should be called out.

All code is expected to have adequate tests (eventually with coverage
expectations).

One test suite that will be added are unit tests to ensure that any existing policies are not capable of exposing any of the following in the Audit Logs:

- `secrets`
- `routes.route.openshift.io`
- `oauthclients.oauth.openshift.io`
- `oauthaccesstokens.oauth.openshift.io`
- `oauthauthorizetokens.oauth.openshift.io`.

These items are invariants that should not be logged in all responses above `Metadata` level.

### Graduation Criteria

None

### Upgrade / Downgrade Strategy

If applicable, how will the component be upgraded and downgraded? Make sure this
is in the test plan.

Consider the following in developing an upgrade/downgrade strategy for this
enhancement:
- What changes (in invocations, configurations, API use, etc.) is an existing
  cluster required to make on upgrade in order to keep previous behavior?
- What changes (in invocations, configurations, API use, etc.) is an existing
  cluster required to make on upgrade in order to make use of the enhancement?

### Version Skew Strategy

How will the component handle version skew with other components?
What are the guarantees? Make sure this is in the test plan.

Consider the following in developing a version skew strategy for this
enhancement:
- During an upgrade, we will always have skew among components, how will this impact your work?
- Does this enhancement involve coordinating behavior in the control plane and
  in the kubelet? How does an n-2 kubelet without this feature available behave
  when this feature is used?
- Will any other components on the node change? For example, changes to CSI, CRI
  or CNI may require updating that component before the kubelet.
- How can Audit Policies made to be made more dynamic in the future?

## Implementation History

Major milestones in the life cycle of a proposal should be tracked in `Implementation
History`.

## Drawbacks

The idea is to find the best form of an argument why this enhancement should _not_ be implemented.

## Alternatives

- we considered offering more fine-grained policies in order to reduce resource overhead. This turned out
  to be unnecessary as the overhead (CPU, memory, IO) is smaller than expected:

### Performance Test Results

- Platform: AWS
- OCP Version: 4.4.0-rc.2
- Kubernetes Version: v1.17.1
- Worker Node count: 250
- Master: 3
- Infrastructure: 3
- Masters Nodes: r5.4xlarge
- Infrastructure Nodes: m5.12xlarge
- Worker Nodes: m5.2xlarge

The load was running: clusterloader - master vertical testing with 100 projects.

The resource consumption was captured querying:

- `container_cpu_usage_seconds_total` looking at the `openshift-kube-apiserver` namespace
- `container_memory_rss looking` at the `openshift-kube-apiserver namespace`
- `node_disk_read_bytes_total`.

#### Results: 256 Nodes Idle

| Policy | CPU | Memory | Disk |
| --- | --- | --- | --- |
| Baseline | 11.93% | 1.66GiB | 617kBs |
| Write Request | 21.55% | 1.792GiB | 654kBs |
| Write RequestResponse | 15.34% | 1.607GiB | 570kBs |
| R+W Request Response | 20.39% | 2.035GiB | 880kBs |

#### Results: 256 Nodes Under load

| Policy | CPU | Memory | Disk |
| --- | --- | --- | --- |
| Baseline | 39% | 2.154GiB | 1.277MBs |
| Write Request | 45% | 2.397GiB | 1.448MBs |
| Write RequestResponse | 32% | 2.085GiB | 1.3MBs |
| R+W Request Response | 47% | 2.393GiB | 1.974MBs |

#### References

- [1] target policies: https://gist.github.com/sttts/a36ecba4eb112f605b53b3276524aad1
- [2] performance analysis doc: https://docs.google.com/document/d/1061FPmY90686zu2Fcs_ya2SRBCyP0F4rQm4KVrEr8oI/edit#heading=h.jib9yfrphvof
