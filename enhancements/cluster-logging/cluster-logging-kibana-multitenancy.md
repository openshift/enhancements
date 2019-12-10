---
title: cluster-logging-kibana-multitenancy
authors:
  - "@ewolinetz"
reviewers:
  - "@jcantrill"
  - "@bparees"
  - "@alanconway"
approvers:
  - "@jcantrill"
  - "@bparees"
  - "@alanconway"
creation-date: 2019-12-10
last-updated: 2020-01-07
status: implementable
see-also: []
replaces: []
superseded-by: []
---

# cluster-logging-kibana-multitenancy

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Migration plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

The purpose of updating the Kibana Multitenancy pattern is to no longer rely on the
[openshift-elasticsearch-plugin](https://github.com/fabric8io/openshift-elasticsearch-plugin/) to
provide multitenancy for Kibana. The [Kibana Multitenancy plugin](https://github.com/opendistro-for-elasticsearch/security-kibana-plugin) in conjunction with the [elasticsearch-proxy](https://github.com/openshift/enhancements/blob/master/enhancements/cluster-logging/cluster-logging-elasticsearch-proxy.md) would make the openshift-elasticsearch-plugin obsolete.

## Motivation

Currently the openshift-elasticsearch-plugin intercepts calls from Kibana, rewrites them
based on the username to match up with an user-specific index, then rewrites the response so that Kibana does not detect that things have been rewritten. This currently requires that we are able to
keep up with any changes to the Kibana -> Elasticsearch API for intercepting and updating
the calls, this instead would be handled by Kibana before sending a request to Elasticsearch.

Moving to use a technology not solely maintained by Red Hat also would increase our adoption time for future releases of Kibana and the plugin, and allows us to benefit from a larger community of contributors for features and fixes.

### Goals
The specific goals of this proposal are:

* Provide a means to link OCP users to Kibana tenants and provide Elasticsearch log access
based on their OCP access.

We will be successful when:

* An OCP user is able to log into Kibana and have a user specific tenant provided for them
* Their tenant has the appropriate index patterns assigned to them
* Their tenant can correctly access their data based on their OCP namespaces
* They should be able to share visualizations between accounts

### Non-Goals

* This is to configure Kibana such that users have their user tenants created for them and the appropriate
index patterns are made available. Role access is separate from this implementation and will only be tying
an index pattern to a specific user's role.

## Proposal

The OpenDistro Multitenancy plugin seeks to simplify our current process of catching requests from Kibana before they are processed by Elasticsearch where we then rewrite the index to uniquely match an user. The tenant patterns that are matched to
an user are defined within the user roles that are seeded into Elasticsearch. This functionality is to be provided by the Elasticsearch Proxy.

### User Stories

#### As an OKD admin, I want to be able to view all logs that I have access to

This is the current state, admin users should still be able to view all the cluster logs with the provided index patterns

#### As an OKD user, I want to be able to view all container logs form the namespaces to which I have access

This is current state, however instead of an index pattern per namespace the user has access to, it will be a single static index pattern with access controlled by DLS.

#### As an OKD user, I want to be able to create and share visualizations for logs

Currently users are unable to share their visualizations since Kibana did not have knowledge on how to access the specific user's Kibana index where the visualizations were stored. They would be able to share them now as Kibana specifies the tenant as part of the visualization path when sharing.

### Implementation Details

#### Assumptions

* The Elasticsearch Proxy will configure roles for users such that they have a tenant pattern defined that matches what Kibana would create for an user logging in.
* Each user would have only one tenant available to them: their user name based tenant

#### Security

No additional security concerns need to be addressed by this plugin.

### Risks and Mitigations

## Design Details

As part of the Kibana6 image building and distribution, we will provide the OpenDistro plugin for Kibana.
Installing and configuring that plugin will allow us to provide multitenancy from within Kibana.
We want to avoid using the `private` tenant as it would be a duplicate of the user tenant and may be confusing when trying to share a visualization.

### kibana.conf

As part of configuring multitenancy for Kibana, we would need to add the following to the Kibana config:

```
elasticsearch.requestHeadersWhitelist: ["securitytenant","Authorization"]
opendistro_security.multitenancy.enabled: true
opendistro_security.multitenancy.tenants.enable_global: false
opendistro_security.multitenancy.tenants.enable_private: false
opendistro_security.multitenancy.enable_filter: false
```

### roles.yml

In addition to needing to configure Kibana to allow multitenancy, we need to update the Elasticsearch `roles.yml` to specify `tenant_permissions` for each user's role.

```
example-role:
  reserved: false
  hidden: false
  cluster_permissions:
  - "read"
  - "cluster:monitor/nodes/stats"
  - "cluster:monitor/task/get"
  index_permissions:
  - index_patterns:
    - "app.logs"
    - "infra.container"
    - "infra.node"
    dls: ""
    fls: []
    masked_fields: []
    allowed_actions:
    - "read"
  tenant_permissions:
  - tenant_patterns:
    - "admin"
    allowed_actions:
    - "kibana_all_write"
  static: false
```

### tenants.yml

In order to create custom tenants to provide for users, we need to define them and then list them as a `tenant_patterns` entry.

```
---
_meta:
  type: "tenants"
  config_version: 2

## admin tenant
admin:
  reserved: true
  description: "The tenant for admin users"
```

### Test Plan

#### Unit Testing

* Elasticsearch-Proxy unit tests verify that we are able to create the appropriate `tenant_permissions`
* Elasticsearch-Operator has no actions to take for Kibana -- the config is built into the image itself

#### Integration and E2E tests

* Tests to verify users have index patterns defined for them upon logging in
* Tests to verify users cannot see other users' tenants
* Tests to verify users can share visualizations with other users
* Tests to verify users can view logs without regression

### Graduation Criteria

#### GA

* Gather feedback from users rather than just developers
* End user documentation
* Sufficient test coverage

### Version Skew Strategy

Version skew is not relevant because it is contained within the image for Kibana6.

## Implementation History

| release|Description|
|---|---|
|4.4| **GA** - Initial release

## Drawbacks

Currently there is a legal battle going on regarding Elasticsearch and Floragunn (including some aspects of OpenDistro), this could possibly compromise our ability to use OpenDistro.

## Alternatives

* Maintaining using our approach to rewrite Kibana index requests.

## Infrastructure Needed

* Elasticsearch-Proxy
