# Stability Fix Approved Workflow
The `stability-fix-approved` label exists to denote that a PR increases stability in a repository that would otherwise have changes locked down; allowing the PR to merge. 
Application of this label is restricted to [openshift-team-leads](https://github.com/orgs/openshift/teams/openshift-team-leads) and [openshift-staff-engineers](https://github.com/orgs/openshift/teams/openshift-staff-engineers).

## Locking Repos Down
When it is determined by the architects that a team's repo(s) should be locked down from additional changes due to ongoing regressions, the label must be added to the respective `tide` merge queries. 
This is a simple, manual process that involves creating a PR in `openshift/release`. This [example](https://github.com/openshift/release/pull/62947/files) shows how to require the label on **all** branches of the `openshift/cluster-api` repo. 
Once the PR is merged, no changes will be able to be merged in the respective repo(s) without the `stability-fix-approved`  label present.

## Unblocking a PR
[openshift-team-leads](https://github.com/orgs/openshift/teams/openshift-team-leads) and [openshift-staff-engineers](https://github.com/orgs/openshift/teams/openshift-staff-engineers) 
can add the `stability-fix-approved` label to **any** PR by simply commenting `/label stability-fix-approved`  on the PR. At that point, that PR will no longer be blocked and will follow the standard review and merge process.
