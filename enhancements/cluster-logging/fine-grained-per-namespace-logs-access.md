---
title: fine-grained-logs-access

authors:
  - "@aminesnow"

reviewers:
  - "@periklis"
  - "@shwetaap"
  - "@alanconway"
  - "@xperimental"
  - "@jcantrill"

approvers:
  - "@periklis"

creation-date: 2023-05-31
last-updated: 2023-05-31
tracking-link: # link to the tracking ticket (for example: Jira Feature or Epic ticket) that corresponds to this enhancement
  - https://issues.redhat.com/browse/LOG-4020
see-also: []
replaces: []
superseded-by: []
---

# Openshift Logging: Fine grained access to logs

## Summary

This enhancement proposal seeks to outline the solution for a more fine grained access to LokiStack logs on OpenShift Container Platform 4, ie, have the ability as the cluster administrator to grant access to logs on a namespace basis.

The proposal provide an overview of the changes TODO!!

## Motivation

In enterprise environments, where OpenShift is used across different legal entities, it's common to have central teams that support the application teams in the respective entities. But given that some application may log sensitive data, those centralized support teams are not granted access to logs but they can only view specific objects, such as pods in the namespace.

Currently, access to logs in LokiStack is granted when a user has access to the given namespace or when the user is part of a specific cluster-admin Group. And even though OpenShift Container Platform 4 does allow to configure RBAC to address this issue, LokiStack does not, and therefore grants access to logs to people that should not see them.

This enhancement proposal presents a solution that enables cluster admins to have a more fine grained control over who accesses what logs.

### User Stories

* Use Case number one, is where a user has access to a specific namespace to see the objects included but is denied to view logs from pods (for legal reason mostly).

* Use Case number two, is where a number of people have elevated permissions and therefore are able to access pretty much all namespaces on the OpenShift cluster. Usually those people are responsible for enabling and supporting applications when onboarding to OpenShift and therefore have permissions to see most of the objects in the given namespaces. However, due to legal and data protection constrains, those users can not have access to logs and therefore LokiStack should prevent them from seeing application specific logs.

### Goals

* Cluster admins can grant and revoke access of the workload logs of specific namespaces to users.
* Cluster admins can grant and revoke access of the workload logs of specific namespaces to users with elevated privileges.

### Non-Goals

* Prevent users with elevated privileges from escalating their privileges to access application logs that they are not supposed to have access to. 

## Proposal

Use the `ClusterLogging` custom resource definition managed by the Cluster Logging Operator to enable/disable the fine grained access to logs for the LokiStack logstore. This will be achieved by adding a new field to the CRD called `advancedLogsAccess`. This new field is a boolean with a default value of false. 

* For the first use case, where we have non-admin users, the proposed solution is to create a group for application log readers called `log-reader-group`, and grant this group the ability to read application logs by binding it to the existing ClusterRole `logging-application-logs-reader`. The cluster admin can then either add a user to the `log-reader-group` thus granting access to application logs on all namespace where they have access, or create a role binding to the user on each namespace they want to grant access to logs on.

* For the second use case, where we have users with admin like privileges, the proposed solution is to create a new group called `restricted-cluster-admin-group`. This group has all the permissions a cluster admin has, except for application logs. We can then grant this group access to application logs on specific namespace by binding it to `logging-application-logs-reader` on the desired namespace. 

### Workflow Description

1. The cluster administrator enables fine grained logs access in the `ClusterLogging` resource:

```yaml
apiVersion: logging.openshift.io/v1
kind: ClusterLogging
metadata:
  name: instance
  namespace: openshift-logging
spec:
  managementState: Managed
  logStore:
    type: lokistack
    lokistack:
      name: lokistack-dev
      advancedLogsAccess: true
  collection:
    ...
```

2. The Cluster Logging Operator (CLO) deletes the `logging-all-authenticated-application-logs-reader` ClusterRoleBinding. This binding allows all authenticated users to see application logs in namespace where they have access.

3. The CLO then creates a new empty group `log-reader-group` and binds it to `logging-application-logs-reader`:

```yaml
apiVersion: user.openshift.io/v1
kind: Group
metadata:
  name: log-reader-group
users:
```
The group contains no users, and is not managed by the operator once it's created. The cluster admin can then add users to the group by editing it manually.

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: logging-application-logs-reader-binding
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: logging-application-logs-reader
subjects:
- kind: Group
  name: log-reader-group
  apiGroup: rbac.authorization.k8s.io
```

4. The operator also creates a new ClusterRole called `restricted-cluster-admin` and binds it to a new group `restricted-cluster-admin-group`. `restricted-cluster-admin` grants access to all api objects except for logs:

```yaml
kind: ClusterRole
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: restricted-cluster-admin
rules:
- apiGroups:
  - ''
  - 'admissionregistration.k8s.io/v1'
  - 'apiextensions.k8s.io/v1'
  - 'apiregistration.k8s.io/v1'
  - 'apiserver.openshift.io/v1'
  - 'apps.openshift.io/v1'
  - 'authentication.k8s.io/v1'
  - 'authorization.k8s.io/v1'
  - ...
  resources:
  - '*'
  verbs:
  - '*'
```

```yaml
apiVersion: user.openshift.io/v1
kind: Group
metadata:
  name: restricted-cluster-admin-group
users:
```
The group contains no users, and is not managed by the operator once it's created. The cluster admin can then add users to the group by editing it manually.

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: restricted-cluster-admin-binding
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: restricted-cluster-admin
subjects:
- kind: Group
  name: restricted-cluster-admin-group
  apiGroup: rbac.authorization.k8s.io
```

Now the cluster admin can allow access to specific namespaces to the `restricted-cluster-admin-group` (or to a specific user in the group) by creating a RoleBinding with the `logging-application-logs-reader`, e.g.
```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: special-perm-restricted-cluster-admin-binding
  namespace: desired-namespace
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: logging-application-logs-reader
subjects:
- kind: Group # or User
  name: restricted-cluster-admin-group # or the User's name
  apiGroup: rbac.authorization.k8s.io
```


### API Extensions
TODO!!!!!!!!!

API Extensions are CRDs, admission and conversion webhooks, aggregated API servers,
and finalizers, i.e. those mechanisms that change the OCP API surface and behaviour.

- Name the API extensions this enhancement adds or modifies.
- Does this enhancement modify the behaviour of existing resources, especially those owned
  by other parties than the authoring team (including upstream resources), and, if yes, how?
  Please add those other parties as reviewers to the enhancement.

  Examples:
  - Adds a finalizer to namespaces. Namespace cannot be deleted without our controller running.
  - Restricts the label format for objects to X.
  - Defaults field Y on object kind Z.

Fill in the operational impact of these API Extensions in the "Operational Aspects
of API Extensions" section.

### Implementation Details/Notes/Constraints [optional]

What are the caveats to the implementation? What are some important details that
didn't come across above. Go in to as much detail as necessary here. This might
be a good place to talk about core concepts and how they relate.

### Risks and Mitigations

What are the risks of this proposal and how do we mitigate. Think broadly. For
example, consider both security and how this will impact the larger OKD
ecosystem.

How will security be reviewed and by whom?

How will UX be reviewed and by whom?

Consider including folks that also work outside your immediate sub-project.

### Drawbacks

The idea is to find the best form of an argument why this enhancement should
_not_ be implemented.  

What trade-offs (technical/efficiency cost, user experience, flexibility, 
supportability, etc) must be made in order to implement this? What are the reasons
we might not want to undertake this proposal, and how do we overcome them?  

Does this proposal implement a behavior that's new/unique/novel? Is it poorly
aligned with existing user expectations?  Will it be a significant maintenance
burden?  Is it likely to be superceded by something else in the near future?


## Design Details

### Open Questions [optional]

This is where to call out areas of the design that require closure before deciding
to implement the design.  For instance,
 > 1. This requires exposing previously private resources which contain sensitive
  information.  Can we do this?

### Test Plan

**Note:** *Section not required until targeted at a release.*

Consider the following in developing a test plan for this enhancement:
- Will there be e2e and integration tests, in addition to unit tests?
- How will it be tested in isolation vs with other components?
- What additional testing is necessary to support managed OpenShift service-based offerings?

No need to outline all of the test cases, just the general strategy. Anything
that would count as tricky in the implementation and anything particularly
challenging to test should be called out.

All code is expected to have adequate tests (eventually with coverage
expectations).

### Graduation Criteria

N/A

#### Dev Preview -> Tech Preview

N/A

#### Tech Preview -> GA

N/A

#### Removing a deprecated feature

N/A
### Upgrade / Downgrade Strategy

N/A

### Version Skew Strategy

N/A

### Operational Aspects of API Extensions

N/A

#### Failure Modes

N/A

#### Support Procedures

N/A

## Implementation History

Major milestones in the life cycle of a proposal should be tracked in `Implementation
History`.

## Alternatives

N/A
