---
title: insights-gateway
authors:
  - "@iNecas"
reviewers:
  - "@jhjaggars"
  - "@chambridge"
  - "@martinkunc"
  - "@mfojtik"
approvers:
  - "@mfojtik"
  - "@smarterclayton"
creation-date: 2020-08-20
last-updated: 2020-08-20
status: implementable
see-also:
replaces:
superseded-by:
---

# Insights Gateway

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

Insights-operator provides a proxy/gateway to provide a secure yet simple
way for additional components to send payloads from the OCP cluster to
cloud.redhat.com. This reduces complexity on user as well as developer
side and adds additional control over the data sent to cloud.redhat.com.

## Motivation

In the last couple of releases, we've seen multiple new services build in 
cloud.redhat.com (c.r.c.) providing additional insights from various areas,
each needing additional data to be sent from the OCP clusters, such as:

- [cost management](https://github.com/project-koku/korekuta-operator)
- [subscription watch](https://github.com/chambridge/subscription-watch-operator)
- [marketplace](https://github.com/redhat-marketplace/redhat-marketplace-operator)

With more managed services, this list is expected to grow. Currently, every service
provides an OLM-managed operator that collects and sends the required data.
The data are eventually sent to `https://cloud.redhat.com/api/ingress/v1/upload`

One of the problems is that for each of the operators, one needs to configure
correct credentials to authenticate the cluster against the remote resource
(example in [cost management documentation](https://access.redhat.com/documentation
/en-us/openshift_container_platform/4.5/html/getting_started_with_cost_management/
assembly_adding_sources_cost#configuring_cost_mgmt-operator)). This leads to:

- increased complexity for the user to set up this components
- the need for each component to implement their own way to send the data 
- number of places the credentials to cloud.redhat.com are stored
- no single place to audit and opt-out form the payloads being sent to c.r.c.

Given insights-operator is already part of the OCP cluster and has proper credentials
configured already as part of the installation, we're proposing for it to expose
a proxy for other components to be able to send the payloads on their behalf.

The single place for uploading can also enable additional controls over the sending process
(e.g. throttling, limiting, upload queue). These enhancements however are not in scope
of this particular proposal.

### Goals

1. Streamline the process for additional components to send data from the OCP cluster.
2. No need for each component to keep their own credentials.
3. Basic centralized overview about payloads being sent to c.r.c.
4. Ability to opt-out form all the payloads being sent to c.r.c., leveraging the machanism
that's already in place in insights-operator.

### Non-Goals

1. Build and maintain a separate component to acts as a gateway: this can be
   subject of additional proposal.
2. Provide additional guaranties about the content of the payload beyond basic logging
   (e.g. time, source, size). The information about the data being collected and it's
   purpose needs to be documented by the operator collecting the data.
3. Granular control over allowed/denied payload types: for the sake of this proposal, it's 
   all or nothing.

## Proposal

1. insights-operator will expose a service available within the OCP cluster
   that would provide an endpoint to proxy the payloads to to c.r.c. using
   its pull-secret credentials. The endpoint would process POST requests
   at the following URL:
   
   ```
   https://insights-operator.openshift-insights.svc/rhinsights/v1/upload
   ```

2. insights-operator will define a `ClusterRole`` granting access to this API via 'nonResourceURLs':

```
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: rhinsights-upload
rules:
- nonResourceURLs:
  - /rhinsights/v1/upload
  verbs:
  - post
```
3. for the third-party component to use the endpoint, it will bind a service account to the cluster role:

```
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: olm-operator
  namespace: olm-operator
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: rhinsights-upload
subjects:
  - kind: ServiceAccount
    name: olm-operator
    namespace: olm-operator
```
4. before forwarding the request, the io will:
  - log the event of forwarding the request (service account info, content_type, payload size…)
  - add additional headers to the request about it's origin (service account
    etc… - TBD by the c.r.c. ingress about what headers would be useful)


### Implementation Details/Notes/Constraints [optional]

### Risks and Mitigations

An OLM operator relying only on this feature will not work on an older version
of the cluster, unless the operator has a fallback mechanism for this cases.
This needs to be considered by the OLM operator author.


## Design Details

### Open Questions [optional]

N/A

### Test Plan

The e2e testing should be performed from the perspective of an external
operator trying to use the upload API, mainly that:

- properly bound service account provides guarantees the upload to be accepted
- unauthenticated requests are denied

In order to make it easier to test the functionality, a support for a `noop`
parameter could be added to just report success without actually sending the
payload out.

### Graduation Criteria

Targeting for development in 4.7.

### Upgrade / Downgrade Strategy

### Version Skew Strategy

## Implementation History

- 2020-08-17: Initial draft

## Drawbacks

1. This proposal partly extends the responsibilities of insights-operator beyond
   the realm of support.
2. Problematic behaviour of an external operator (large payloads, too often) can negatively
   influence the whole insights operator.
3. There are other components in the OCP cluster (e.g. telemeter) that are sending
   data to managed services. This proposal is not covering these cases.

## Alternatives

For enhancements in further releases (4.8+), it should be considered building an
independent component dedicated for controlling and sending the data outside of
OCP cluster, with allowing further configuration granularity and considering
using it beyond just the use-cases of c.r.c. services.
