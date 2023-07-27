---
title: fine-grained-per-namespace-logs-access

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

api-approvers:
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

This enhancement proposal seeks to outline the solution for more fine-grained access to LokiStack logs on OpenShift Container Platform 4, ie, have the ability as the cluster administrator to grant access to logs on a namespace basis.

The proposal provides an overview of the changes required to achieve this goal as well as some implementation details.

## Motivation

In enterprise environments, where OpenShift is used across different legal entities, it's common to have central teams that support the application teams in the respective entities. Support teams are granted access to view Kubernetes resources, however, as some applications may log sensitive data, support teams should not have access to logs by default.

Currently, access to logs in LokiStack is granted when a user has access to the given namespace or when the user is part of a certain group that we (`logging-team`) define as a cluster-admin group. And even though OpenShift Container Platform 4 does allow configuring RBAC to address this issue, LokiStack does not, and therefore grants access to logs to people that should not see them.

This enhancement proposal presents a solution that enables cluster admins to have a more fine grained control over who accesses what logs.

### User Stories

* As a cluster admin, I want to deny a user from viewing logs from pods in a namespace in which they have access. This is mostly for legal reason.

* As a cluster admin, I want to deny a user with cluster admin like privileges access to application logs, on a namespace basis or cluster wide. This is done due to legal and data protection constrains.

### Goals

* Cluster admins can grant and revoke access of the workload logs of specific namespaces to users.
* Cluster admins can grant and revoke access of the workload logs of specific namespaces to users with elevated privileges.
* Access should be managed using OpenShift RBAC in order to be consistent with how permissions are managed elsewhere.

### Non-Goals

* Prevent users with elevated privileges from escalating their privileges to access application logs that they are not supposed to have access to. 

## Assumptions

This proposal assumes that the existing virtual resource `logs` (API group `loki.grafana.com`) is extended to be used on a namespace basis. This would be achieved by implementing a new authorization workflow for requests in `opa-openshift`, by using namespaced `SubjectAccessReview` (SAR) API calls.

Here's what the new authorization workflow would look like:

![Authz workflow](images/authz-flow.png)


## Proposal

Make the `logging-all-authenticated-application-logs-reader` ClusterRoleBinding unmanaged by the operator, and create three ClusterRoles, one for each of the three tenants (`application`, `infrastructure` and `audit`):

* `cluster-logging-application-view`: give application logs read access.
* `cluster-logging-infrastructure-view`: give infrastructure logs read access.
* `cluster-logging-audit-view`: give audit logs read access.

These roles can then be bound to users/groups on a namespace basis or cluster wide. 
Using the new SAR implementation along with these roles, we can simply use RBAC configurations to achieve our goals:

  1. For the first use case, where we have non-admin users, the cluster admin can create the necessary RoleBindings for users to have access to each log type on the target namespaces.

  2. For the second use case, where we have users with admin like privileges, the cluster admin can for instance create ClusterRoleBindings for users to have access to `infrastructure` and `audit` logs, and then create a RoleBinding on namespaces where they wish to grant access to `application` logs.

### Workflow Description

1. The Cluster Logging Operator (CLO) creates `cluster-logging-application-view`, `cluster-logging-infrastructure-view` and `cluster-logging-audit-view` ClusterRoles:

```yaml
kind: ClusterRole
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: cluster-logging-application-view
rules:
- apiGroups:
  - loki.grafana.com
  resources:
  - application
  resourceNames:
  - logs
  verbs:
  - get
```

```yaml
kind: ClusterRole
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: cluster-logging-infrastructure-view
rules:
- apiGroups:
  - loki.grafana.com
  resources:
  - infrastructure
  resourceNames:
  - logs
  verbs:
  - get
```

```yaml
kind: ClusterRole
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: cluster-logging-audit-view
rules:
- apiGroups:
  - loki.grafana.com
  resources:
  - audit
  resourceNames:
  - logs
  verbs:
  - get
```

2. The cluster admin deletes the `logging-all-authenticated-application-logs-reader` ClusterBinding.

3. The cluster admin create the necessary binding to grant access to users, e.g.:

Granting `simple-user` access to `application` logs on `desired-namespace`.
```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: simple-user-application-logs
  namespace: desired-namespace
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: cluster-logging-application-view
subjects:
- kind: User
  name: simple-user
  apiGroup: rbac.authorization.k8s.io
```

Granting `admin-user` access to `infrastructure` logs.
```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: admin-user-infrastructure-logs
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: cluster-logging-infrastructure-view
subjects:
- kind: User
  name: admin-user
  apiGroup: rbac.authorization.k8s.io
```

### API Extensions
N/A

### Implementation Details/Notes/Constraints

In the CLO, we need to disable the reconciliation of `logging-all-authenticated-application-logs-reader` ClusterRoleBinding. This reconciliation happens here:

`internal/logstore/lokistack/logstore_lokistack.go:64`
```go
func ReconcileLokiStackLogStore(k8sClient client.Client, deletionTimestamp *v1.Time, appendFinalizer func(identifier string) error) error {
  
  ...
  if err := reconcile.ClusterRoleBinding(k8sClient, lokiStackAppReaderClusterRoleBindingName, newLokiStackAppReaderClusterRoleBinding); err != nil {
    return kverrors.Wrap(err, "Failed to create or update ClusterRoleBinding for reading application logs.")
  }

  return nil
}
```


### Risks and Mitigations

* Privilege escalation, ie, a restricted cluster admin being able to grant themselves the ability to access logs they should not be able to access via an RBAC modification, is something cluster admins should be aware of and manage themselves.

* This proposition assumes and is dependent on the fact that the SAR is implemented and works correctly.

### Drawbacks

* We don't automate much as this is an RBAC configuration focused proposal, so in order for cluster admins to properly configure access to logs, a comprehensive documentation is required.


## Design Details

### Open Questions

1. What happens if the SAR solution is not feasible?

### Test Plan
TBD

## Alternatives
TBD

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
N/A

