---
title: automate-image-pruning
authors:
  - "@dmage"
  - "@adambkaplan"
reviewers:
  - "@bparees"
  - "@coreydaley"
  - "@ricardomarascini"
  - "@gabemontero"
  - "@eparis"
approvers:
  - "@bparees"
  - "@derekwaynecarr"
  - "@smarterclayton"
creation-date: 2019-09-26
last-updated: 2019-10-22
status: implementable
see-also:
replaces:
superseded-by:
---

# Automate Image Pruning

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift/docs]

## Summary

This enhancement will install a `CronJob` to automate the image pruning task
for the internal registry. An API will be exposed to allow cluster admins to
configure the pruning job to suit their needs, or disable pruning if that is
what is desired.

## Motivation

Regular image pruning is a vital maintenance task to ensure an OpenShift
cluster remains healthy. In certain use cases (such as CI/build farms), pruning
must be run frequently to prevent image resources from exhausting etcd capacity
as well as to reduce registry blob storage utilization. Today, pruning is
configured through a manual, multi-step process.

### Goals

- Provide a one-step process for cluster admins to configure recurring
  scheduled image pruning.
- Default pruning to be enabled on an OpenShift cluster at install time for new
  clusters.
- Provide alerting if image pruning fails, which may indicate a cluster is at
  risk of failure.

### Non-Goals

- Fix fundamental issues with the current pruning process (ex: pruning images
  that may be referenced in a CRD).
- Enable pruning on existing clusters during upgrade.
- Provide alerting if a cluster is at risk due to too many images.

## Proposal

### User Stories

#### Prune images in etcd

As an OpenShift cluster admin
I want the cluster to automatically prune ImageStreams stored in etcd
So that etcd storage is not overwhelmed by unused imagestreams
And the cluster remains healthy

#### Prune images on the registry

As an OpenShift cluster admin
I want the cluster to automatically prune unused images stored in the registry
So that registry storage is managed
And I don’t pay for storage consumed by images my organization doesn’t use

#### Orphaned blobs

As an OpenShift cluster admin
I want the cluster to automatically remove orphaned blobs stored in the registry
So that registry storage is managed
And I don’t pay for storage consumed by images my organization doesn’t use

### Implementation Details/Notes/Constraints

The implementation relies on the current image pruning process invoked by the
`oc adm prune` command. Running this in a container can be accomplished by
using the `cli` imagestream installed by the samples operator. The image must
be run using a service account with the `system:image-pruner` cluster role.

The image registry operator will be updated to configure and reconcile all the
components needed to run image pruning on an OpenShift cluster.
Specifically, this includes the following:

1. A `ServiceAccount` dedicated to the image pruner.
2. A `ClusterRoleBinding` granting the `system:image-pruner` role to the pruner
service account.
3. A `CronJob` which runs the image pruner with the following tunable
   parameters:
   1. Schedule for `CronJob`
   2. `--keep-tag-revision` command line argument for the pruner
   3. `--keep-younger-than` command line argument for the pruner
   4. Resources for the `CronJob` pod template

Installation and configuration of the pruner will be accomplished via a
separate custom resource (with correspoding CRD). These components should be
created/reconciled if the pruning custom resource is created or updated.

When the pruning custom resource is deleted, the pruning `CronJob` and its
related components should also be deleted.

The operator's behavior with respect to managing the pruner is orthogonal to
the `ManagementState` specified on the image registry operator's
`ClusterOperator` object. If the image registry operator is not in the
`Managed` state, the image pruner can still be configured and managed via the
pruning custom resource. However, the `ManagementState` of the image registry
operator will alter the behavior of the deployed image pruner job:

1. `Managed` - the `--prune-registry` flag for the image pruner will be set to
   `true`.
2. `Removed` - the `--prune-registry` flag for the image pruner will be set to
   `false` (only prune image metatdata in etcd).
3. `Unmanaged` - the `--prune-registry` flag for the image pruner will be set
   to `false`.

### Risks and Mitigations

#### Resource consumption

Risk: Image pruning requires significant memory to compute the blob layer graph
needed to identify "prunable" layers.

Mitigation: API will allow cluster admins to adjust the resources allocated to
the `CronJob`.

Risk: Image pruning requires too much memory to be scheduled to a node
Risk: Image pruning exhausts resources used for application workloads

Mitigation: API will allow admins to use standard tools to let pruning be run
on desired machine pools (`affinity`, `tolerations`, `nodeSelector`)

##### Concurrent pruning jobs

Risk: Pruning jobs could run concurrently if the job duration exceeds the
interval set by the `CronJob` schedule.

Mitigation: The `CronJob` for pruning must be configured with
`concurrencyPolicy: Forbid`

#### Removal of past pruning jobs

Risk: Runs of the pruning job create another set of objects in etcd that need
to be pruned.

Mitigation: `CronJob` specifies a history limit for successful and failed jobs.
This will be a tunable parameter via the API.

#### Pruning of in-use images

Risk: Pruner is not aware of a particular image, resulting in it being pruned
prior to use.

Mitigation: None at present - this is an inherent problem within the current
pruning process.

## Design Details

### API

A separate CRD will be created with the following `spec` and `status` fields:

```yaml
spec:
  schedule: "*/1 * * * *"
  suspend: false
  keepTagRevisions: 3
  keepYoungerThan: 60m
  resources: {}
  affinity: {}
  nodeSelector: {}
  tolerations: {}
  startingDeadlineSeconds: 60
  history:
    successfulJobsHistoryLimit: 3
    failedJobsHistoryLimit: 3
status:
  observedGeneration: 2
  conditions:
  - type: Available
    status: "True"
    lastTransitionTime: 2019-10-09T03:13:45
    reason: Ready
    message: "Periodic image pruner has been created."
  - type: Scheduled
    status: "True"
    lastTransitionTime: 2019-10-09T03:13:45
    reason: Scheduled
    message: "Image pruner job has been scheduled."
  - type: Failed
    staus: "False"
    lastTransitionTime: 2019-10-09T03:13:45
    reason: Succeeded
    message: "Most recent image pruning job succeeded."
```

#### Spec

1. `schedule:` Cron formatted schedule, defaults to daily at midnight for new
clusters. Required field.
2. `suspend:` if `true`, CronJob running pruning is suspended. Optional,
   default false.
3. `keepTagRevisions:` number of revisions per tag to keep. Optional, default 3
   if not set.
4. `keepYoungerThan:` retain images younger than this duration. Optional,
   default 60m if not set.
5. `resources:` standard Pod resource requests and limits. Optional.
6. `affinity:` standard Pod affinity. Optional.
7. `nodeSelector:` standard Pod node selector. Optional
8. `tolerations:` standard Pod tolerations. Optional
9. `startingDeadlineSeconds:` start deadline for `CronJob`. Optional.
10. `history:` sets the job history to be retained on the cluster. Object is
    optional.
    1. `successfulJobsHistoryLimit:` maximum number of successful jobs to
       retain. Must be >= 1 to ensure metrics are reported. Default 3 if not
       set.
    2. `failedJobsHistoryLimit:` maximum number of failed jobs to retain. Must
       be >= 1 to ensure metrics are reported. Default 3 if not set.

#### Status

1. `observedGeneration:`: generation observed by the operator
2. `conditions:` standard condition objects with the following types:
   1. `Available` - indicates if the pruning job has been created.
   Reasons can be `Ready` or `Error`.
   2. `Scheduled` - indicates if the next pruning job has been scheduled.
   Reasons can be `Scheduled`, `Suspended`, or `Error`.
   3. `Failed` - indicates if the most recent pruning job failed.

### Fixed Configuration

The following settings for the pruning `CronJob` cannot be configured:

1. `concurrencyPolicy: Forbid` - this prevents concurrent cron jobs from
running.
2. Pruning CLI:
   1. `--prune-registry` - `true` if the image registry is `Managed`, `false`
      otherwise.
   2. `--all` - always `true` (default)
   3. `--prune-over-size-limit` - not set
   4. `--confirm` - always `true` so pruning runs

### Telemetry and Alerting

1. Send to Telemeter a metric indicating if pruning has been scheduled.
2. Fire a warning alert if pruning is suspended or could not be scheduled.
3. Fire a warning alert if the active pruning job exceeds 24 hours in duration.
4. Fire a warning alert if the most recent pruning job failed.

### Test Plan

- `e2e-*-operator` suite can verify that the `CronJob` is configured properly
  on the cluster.
- A separate suite can also verify that the pruning works end to end. Ex:
  - Create and run two builds with different source that push to the same
    imagestream tag.
  - Run pruning with aggressive settings (`keepTagRevisions: 0`,
    `keepYoungerThan: 0s`).
  - Verify the first image is pruned.

### Graduation Criteria

#### Tech Preview

Not applicable - this feature is indended to be GA upon release.

#### Generally Available

Requirements for reaching GA (in addition to tech preview criteria):

1. Pruner CRD definition is installed via the CVO. Replaces item 1 for tech
   preview.
2. Image registry operator watches the pruner CRD instance with canonical name
   `cluster`.
3. Operator installs the `CronJob` and associated RBAC if the pruner CRD
   instance is created. Operator removes the `CronJob` and associated RBAC if
   the pruner CRD instance is deleted.
4. Tests to ensure that the `CronJob` is installed and configured properly.
5. Telemetry which indicates the pruner has been scheduled.
6. New installs: pruner CRD instance is created, with job scheduled to run
   daily at midnight.
7. Upgrades: pruner CRD is created, but underlying job is suspended.
8. E2e testing of pruning via CI (this may require a separate test suite),
   voting for `image-registry`, `cluster-image-registry-operator`, and `oc`
   changes related to `oc adm prune images`.
9. Alerting requirements:
   1. Alert if the image pruner is not scheduled.
   2. Alert if the most recent pruning run failed.
   3. Alert if the most recent pruning run exceeded 24 hours in duration.
10. Documentation requirements:
    1. How to create the pruner CRD and enable pruning.
    2. How to suspend/lift suspension of the pruning job.
    3. Describe pruner's behavior if the registry operator is `Managed` vs.
       `Unmanaged` or `Removed`.

### Upgrade / Downgrade Strategy

On new installs the pruner will be enabled on the cluster.
For GA the pruner will be enabled and run daily at midnight.

On upgrade the cluster admin will be responsible for setting `suspend: false`
on the pruning CRD in order to enable pruning. If pruning is suspended, an
alert is fired.

On downgrade to 4.3 the pruner CRD, `CronJob`, and associated RBAC will remain
intact. The pruner will not be managed by the image registry operator - cluster
admins can opt to keep the pruner or delete the components. On subsequent
upgrade the image registry operator should respect any existing CRD instances
and reconcile accordingly.

### Version Skew Strategy

The image pruner should be compatible with the 4.2 and 4.3 image registry.

## Implementation History

2019-09-27: Initial draft proposal.
2019-10-22: Expanded details for API, alerting, and graduation requirements.

## Drawbacks

Pruning carries the risk that an image that could be run on the cluster is
deleted, leading to `ImagePullBackoff` conditions in deployments.
For this reason we have previously never made explicit recommendations on
when/how admins should run pruning.

## Alternatives

In 4.2 we documented how to deploy pruning as a scheduled `CronJob`.
Cluster admins can use the published YAML template to install the pruning job
via `oc apply -f`.
