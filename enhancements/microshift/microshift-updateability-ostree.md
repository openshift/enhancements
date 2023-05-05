---
title: microshift-updateability-ostree
authors:
  - "@pmtk"
reviewers: # Include a comment about what domain expertise a reviewer is expected to bring and what area of the enhancement you expect them to focus on. For example: - "@networkguru, for networking aspects, please look at IP bootstrapping aspect"
  - "@pacevedom, MicroShift team"
  - "@ggiguash, MicroShift team"
  - "@copejon, MicroShift team"
  - "@dusk125, etcd team"
  - "@oglok, author of previous enhancement"
  - "RHEL expert"
approvers: # A single approver is preferred, the role of the approver is to raise important questions, help ensure the enhancement receives reviews from all applicable areas/SMEs, and determine when consensus is achieved such that the EP can move forward to implementation.  Having multiple approvers makes it difficult to determine who is responsible for the actual approval.
  - "@dhellmann"
api-approvers: # In case of new or modified APIs or API extensions (CRDs, aggregated apiservers, webhooks, finalizers). If there is no API change, use "None"
  - None
creation-date: 2023-04-14
last-updated: 2023-05-05
tracking-link: # link to the tracking ticket (for example: Jira Feature or Epic ticket) that corresponds to this enhancement
  - https://issues.redhat.com/browse/USHIFT-518
see-also:
  - "/enhancements/microshift/microshift-greenboot.md"
  - "/enhancements/microshift/etcd-supportability.md"
replaces:
  - https://github.com/openshift/enhancements/pull/1312
superseded-by:
  - None
---

# MicroShift updateability in ostree based systems

## Summary

This enhancement focuses on high level overview of updating
MicroShift running on ostree based systems such as RHEL 4 Edge.
Enhancement covers backup and restore of MicroShift data,
version migration (upgrade and downgrade) of MicroShift and 
its consequences (migration of data between schema versions),
and interactions with GreenBoot and operating system.

## Motivation

MicroShift team is working towards a general availability (GA) release.
As GA product, it is expected that it can be updated to
provide security patches, functional updates, and bug fixes
without needing to redeploy.

MicroShift is intended to be a part of Red Hat Device Edge
which is based on RHEL For Edge which features ostree and as such
provides rollbacks to go back to previous ostree deployment.
Even though, OpenShift does not support downgrade or rollback,
MicroShift must support it in some form.
Explicit downgrade for Y versions will not be supported, 
and rollback will be supported only when newer ostree commit 
is unhealthy and backup consistent with previously ran 
MicroShift is present.

To allow for such operations, we need to define how we'll
achieve that goal. We can define several areas we need to focus on:
backing up and restoring MicroShift's data, handling version changes
and its consequences such as migrating underlying data between schema
versions, defining a mechanism for allowing or blocking upgrades between
certain version of MicroShift.

### User Stories

* As a MicroShift administrator, I want to safely update MicroShift
  so that I can get bug fixes, new features, and security patches.
* As a MicroShift administrator, I want my system to roll back 
  to a previous good configuration when greenboot fails.

### Goals

Goal of the enhancement is to describe implementation roadmap for
integrating MicroShift with ostree and greenboot in order to provide
functionality to:
- Safely update MicroShift version (by backing up the data and
  restoring it in case of rollback)
- Migrating internal data (like Kubernetes storage or etcd schema) to
  newer version
- Block upgrades in case of version bump being too big

Design aims to implement following principles:
- Keep it simple, optimize later
- MicroShift does not own the OS or host
- MicroShift and all its components are versioned, upgraded, and rolled back together
- Be defensive, fail fast
- Rely on outside intervention as a last resort

### Non-Goals

* Building allowed/blocked version migration graph
* Handling readiness, and backup and rollback of 3rd party applications
  (although end user documentation should be provided)
* Defining updateability for non-ostree systems is left to a future enhancement
* Protecting against data corruption - we rely on file system 
  to maintain the backed up file integrity

## Proposal

### Workflow Description

**MicroShift administrator** is a human responsible for preparing
ostree commits and scheduling devices to use these commits.

Upgrade:

1. MicroShift administrator prepares a new ostree commit
1. MicroShift administrator schedules device to reboot and use new ostree commit
1. Device boots to new commit
1. Operating System, greenboot, and MicroShift take actions without any additional intervention

Manual rollback:

1. MicroShift administrator instructs MicroShift to do a restore on next boot
1. MicroShift administrator stages an ostree deployment with MicroShift
   that was already running and reboots the device
1. Staged ostree deployment boots
1. MicroShift will restore the backup matching current ostree deployment
1. MicroShift will run.

### API Extensions

Metadata persisted on filesystem related to the functionality described 
in this enhancement such as version metadata, next boot action, and any other,
is considered internal implementation detail and not an API to be consumed.
Content (schema) and location of these files are subject to change.

### Implementation Details/Notes/Constraints [optional]

### Risks and Mitigations

Being a GA feature from the beginning the risks are not foreseeing fail scenarios in advance and implementation bugs
that are not caught and fixed through graduation process.

To mitigate the risks, a thorough review of the enhancement must be done by MicroShift, OpenShift, and RHEL teams,
and making sure testing strategy is sound and prioritized equally with the feature development.

### Drawbacks

N/A

## Design Details

### Definitions

- **ostree deployment**: ostree commit present on disk and available as boot entry
- **Rollback**: booting older (that already ran on the device) ostree deployment - either due to greenboot or manual intervention
- **Backup**: backing up `/var/lib/microshift`
- **Restore**: restoring `/var/lib/microshift`
- **Data migration** - procedure of transitioning MicroShift's data to be compatible with newer binary
- **Version metadata**: File residing in MicroShift data dir containing version of MicroShift and ID of ostree deployment
- **MicroShift greenboot healthcheck**: Program verifying the status of MicroShift's cluster

### Preface

Every action related to procedure described in this enhancement is 
performed after system's boot rather than immediately before shutdown. 
Greenboot's healthchecks, green and red scripts are executed independent of MicroShift's processes.

Actions related to backup and restore will be performed with MicroShift components not
running to protect data integrity. Shortly after, etcd and kube-apiserver will be started
to perform a data migration, if needed.

Edge devices are usually resource constrained, however to provide ability of rolling back,
backup of MicroShift data will be kept per ostree deployment existing on the device.

### Phases of execution

1. `microshift admin pre-run`
   - Executed as separate systemd unit (`microshift-ostree-pre-run.service`)
   - If fails, it blocks start up of `microshift.service`
   - Performs
     - Backup or restore
     - Data migration (if needed)
1. `microshift run`
   - `microshift.service` systemd unit
   - Persists version right after starting

In parallel:
1. Greenboot healthcheck
   - Check health of MicroShift
1. Greenboot scripts (red or green) depending on healthcheck
   - Persist health of the system (used during next pre-run to determine actions)
   
### Deployments health history

MicroShift will keep history of deployments health in `/var/lib/microshift-backups/health.json`.
It fulfils two functions: persist data across boots and allow deciding on action to perform for
MicroShift data (backup, restore, delete or leave as is).

```json
{
  "deployments": [
    {
      "id": "rhel-d1",
      "microshift": "unknown | healthy | unhealthy",
      "system": "unknown | healthy | unhealthy",
      "last_boot": "yyyy-mm-dd HH:MM:SS"
    },
    {
      "id": "rhel-d0",
      "microshift": "unknown | healthy | unhealthy",
      "system": "unknown | healthy | unhealthy",
      "last_boot": "yyyy-mm-dd HH:MM:SS"
    }
  ]
}
```

- `last_deployment` is an ID of a deployment during which file was last updated
- `deployments` stores health during specific deployment

### MicroShift version persistence

When data migration is complete or MicroShift is about to start the cluster,
it will persist version of `microshift` executable and ID of current
ostree deployment to `/var/lib/microshift/version.json`.
Purpose of these value is to answer "with what MicroShift version the data is compatible with,
regardless whether it was healthy or unhealthy".
MicroShift version will be used to decide if data migration can be skipped, attempted, or should be blocked.
Deployment ID will be used during more complicated procedures related to restoring or backing up data.

Example of `/var/lib/microshift/version.json`:
```json
{
  "microshift": "4.14.0",
  "deployment": "rhel-d1"
}
```

### Action log

MicroShift will keep a log of important actions during execution in
`/var/lib/microshift-backups/actions.log`. It will store history log about actions related to
backing up, restoring, migrating, starting, checking health, and so one for support purposes.

Specifics such as log messages and form in codebase are an implementation detail and out of scope
of the enhancement. 

### Integration with greenboot

[greenboot](https://github.com/fedora-iot/greenboot) is "Generic Health Checking Framework for systemd".
It is used on ostree based systems (like CoreOS, RHEL For Edge, Fedora Silverblue) to assess system's
health and, if needed, rollback to previous ostree deployment.

For more information about greenboot and current MicroShift's integration with it see 
[Integrating MicroShift with Greenboot](https://github.com/openshift/enhancements/blob/master/enhancements/microshift/microshift-greenboot.md) 
enhancement.

In general, greenboot after boot runs scripts that are verifying if system is healthy
and, depending on result, runs either set of green (healthy) or red (unhealthy) scripts.
Healthy system can be also referred to as "green boot", whereas unhealthy as "red boot".

MicroShift will plug into greenboot green/red scripts integration to persist information about system's health. 
For reasons mentioned in section "Alternatives - Performing backup on shutdown" it was concluded that both 
backup and restore should happen on system start, rather than shutdown, therefore information persisted by green or red
script will be used on next boot of the system.

As a consequence, whether the next boot happens to be different or the same
ostree deployment, it will produce a backup compatible with previously booted deployment
and then attempt to perform a data migration if needed.
It also means that consecutive red boots of new ostree deployment will restore the data,
attempt to migrate it, and run MicroShift, i.e. each boot starts from the same place, just
like it would be a first boot of that ostree deployment.
This provides a safety net in case of invalid data migration - it will be attempted again,
on each boot following red boot.

Potential risk is possibility of losing data that might've been produced during window
of MicroShift start and system reboot. However, only applies to MicroShift's data,
because user's application data isn't persisted in etcd.

To integrate with greenboot, bash scripts will be placed in `/etc/greenboot/green.d` and `/etc/greenboot/red.d`.
These scripts will execute `microshift admin persist-system-health --healthy|--unhealthy` command which will persist
current boot's health into `/var/lib/microshift-backups/health.json`.
File is kept outside of `/var/lib/microshift/` to keep backup management data and runtime data separate.

### Backup and restore of MicroShift data

To integrate fully with greenboot and ostree deployments MicroShift needs to be able to
back up and restore its data.
If new ostree deployment fails to be healthy and system is rolled back to previous
deployment, MicroShift must also roll back in time to data compatible with 
the older deployment.
Because device administrator might want to manually go back to older deployment,
it means that backups for many deployments will be kept. 

OSTree usually keeps only two deployments (three if one is staged, or more if deployment is pinned).
However, deployment could disappear from the system (wasn't pinned, is too old to be kept) but later reintroduced
by admin (deployment's ID is based on checksum which would be the same), it should be configurable to only keep backups
of deployments present in local `rpm-ostree status` output (automatic pruning) or defer to manual cleanup.
In future, more options could be added to configure backup persistency policy.

Because difference between two deployments might not be MicroShift itself, but applications
that run on top of MicroShift from images and manifests embedded in the ostree deployment,
MicroShift's backups are tied to ostree deployments rather than MicroShift versions.

Decision whether to backup or restore is based on contents of `health.json` which contains health of past deployment
that ran MicroShift (see "Integration with greenboot").

#### Copy-on-Write

As a result of investigation and aiming for simplicity for initial implementation,
it was decided that backing up MicroShift's data will be done by leveraging using 
copy-on-write (CoW) functionality.

CoW is a feature of filesystem (supported by XFS and Btrfs) and it can be used by 
providing a `--reflink=` param to `cp` option.
`--reflink=auto` will be used over `--reflink=always` to gracefully fall back to regular
copying on filesystems not supporting CoW (ext4, ZFS).
Since CoW is backed by filesystem, it works only within that filesystem.

To keep track of which backup is intended for which deployment, backup will be placed in a
directory named after ostree deployment ID inside `/var/lib/microshift-backups/` dir, e.g.
`/var/lib/microshift-backups/rhel-8497faf62210000ffb5274c8fb159512fd6b9074857ad46820daa1980842d889.0`.

Restore operation works the same, just in the other direction - copying contents of 
`/var/lib/microshift.bak/ostree-deploy-id/` to `/var/lib/microshift/`.

End user documentation needs to include:
- guidance on setting up filesystem to fullfil requirements for using copy-on-write
  (e.g. making sure some filesystem options are not disabled).
- remark that in case of missing CoW support, full backup will be made.

#### Contents of MicroShift data backup

- etcd database will be backed up by copying whole etcd working directory to preserve
  history and other data that could be lost if snapshot would be performed and restored.
- kubeconfigs and certificates needs to be backed up and restored to keep communication working.
  MicroShift could regenerate them, but it would result in invalidating existing kubeconfigs and breaking communication.
  - Following approach may result in need to update certificates' Subject Alternative Names (SAN) list.
- Versions of binaries don't impact decision whether to perform snapshot or copy whole data
  directory because "newer" version will need to read the existing data anyway to
  perform the data migration.

Based on reasons above, it was decided that whole `/var/lib/microshift` will be backed up.

### Data migration ("upgrade" or "downgrade")

Data migration is process of transforming data from one schema version to another.
It includes following areas:
- Storage migration - upgrade of Kubernetes objects (e.g. from `v1beta1` to `v1`)
  - It's performed by reading Resource in older version and writing newer version
  - MicroShift will reuse existing 
    [Kube Storage Version Migrator](https://github.com/openshift/kubernetes-kube-storage-version-migrator)
    and [its Operator](https://github.com/openshift/cluster-kube-storage-version-migrator-operator).
- etcd schema - although it's not verify likely in near future
  - etcd project documents how to migrate from v2 to v3, 
    but we'll also ask OpenShift etcd team for guidance.

When new ostree deployment is staged and booted, it might or might not feature different
version of MicroShift. If it's different, it might be newer or older.
To keep process of data migration, its maintenance, testing matrix sane, we'll only allow
data migration in one direction: forward in regards to Y stream.
For now, maximum allowed version skew is Y+1, but it might change in the future depending
on upstream migration rules.

It means that it will be possible to use different Z versions of MicroShift with the same
data unless there's a breaking change making it impossible, in such case it should be documented.

Given above we can define:
- rollback as booting older deployment due to admin actions or unhealthy (red) boot
- upgrade as MicroShift version change from X.Y to X.Y+1 and resulting in data migration
- downgrade as MicroShift version change from X.Y to X.Y-1 (or older) which is unsupported

Both rollback and downgrade maybe look similar in terms of version change,
the difference is that for rollback a matching backups exists,
whereas for downgrade a data migration would have to be performed

Decision to perform or refuse a data migration to schema compatible with newly loaded 
MicroShift version will be based on following facts:
- version persisted in MicroShift's data dir (version that created/successfully ran using the data),
  also referred to as (version) metadata
- version of currently installed MicroShift binary
- embedded in MicroShift binary list of blocked "from" versions

A general flow will have following form:
1. If persisted version is missing, assume 4.13.
1. If version of `microshift` binary is older than version in metadata, **refuse to start MicroShift**.
1. If persisted version is on a list of blocked version migrations, **refuse to start MicroShift**.
1. If binary is the same version as persisted in metadata, **no need for a data migration**.
1. Otherwise attempt to migrate the data.

### Open Questions [optional]

#### Should 4.13 -> 4.14 be supported?
- Even if we handle "no metadata, no backup, existing data, no next-boot-action" and make
  a backup, upon rollback 4.13 won't be able to restore it, so it would try to use 4.14's
  data.
  - Is manual intervention acceptable (admin manually copying backup to data)?
  - We most likely don't want to implement special case for that (`pre-run` would save 
    that persist it was 4.13 previously and red script would restore before shutting down)

#### Should MicroShift healthcheck check and log version skew problems so it's easier to debug?
- Why not?

#### Should backups be kept only for deployments present in ostree command?
- Is it possible that deployment can be reintroduced?
  - I.e. it will have the same id and admin might want to rollback to it?

### Workflows in detail

##### Decision tree

Metadata schema reminder
```json
// /var/lib/microshift-backups/health.json
{
  "deployments": [
    {
      "deployment_id": "rhel-d1",
      "microshift": "unknown | healthy | unhealthy",
      "system": "unknown | healthy | unhealthy",
      "last_boot": "yyyy-mm-dd HH:MM:SS"
    },
    {
      "deployment_id": "rhel-d0",
      "microshift": "unknown | healthy | unhealthy",
      "system": "unknown | healthy | unhealthy",
      "last_boot": "yyyy-mm-dd HH:MM:SS"
    }
  ]
}

// /var/lib/microshift/version.json
{
  "microshift": "4.14.0",
  "deployment": "rhel-d1"
}
```

Abbreviations, glossary:
- `microshift/` -> `/var/lib/microshift/`
- `version.json` -> `/var/lib/microshift/version.json`
- `backups/` -> `/var/lib/microshift-backups/`
- `health.json` -> `/var/lib/microshift-backups/health.json`
- `prev-boot-deploy` - deployment in `health.json` with most recent `boot` timestamp in `health.json`
- `current-deploy` - currently running deployment
   - if absent in `health.json`, it means it's 1st boot of the deployment
   - if present, it's a subsequent boot and will be the same as `prev-boot-deploy`
- `earlier-deploy` - deployment that was before `prev-boot-deploy`
  - if `current-deploy` exists in `health.json`, then this is deployment that system is being
    upgraded from and will automatically roll back to
  - if `current-deploy` is not present in `health.json` (so ostree history looks like `current`, 
    `prev-boot`, and `earlier` deployments), it might no longer exist according to ostree
    (only `current` and `prev-boot` which is a rollback deployment)

`microshift pre-run`:
1. if neither `microshift/` nor `backups/` exist
   > *first boot*
   - **exit 0** :leftwards_arrow_with_hook:	

1. if `microshift/` exists, `version.json`, `health.json` does not
   > *assume it's 4.13*
   - create `version.json`: `{ "microshift": "4.13", "deployment": "4.13"}`
   - backup `microshift-backups/4.13`
   - proceed to data migration

1. load `version` and `health.json`

1. if `prev-boot-deploy.system` is `healthy`
   - backup to `microshift-backups/prev-boot-deploy.id`
   - special case - rollback on demand
     - `current-deploy.id` is different from `prev-boot-deploy.id`, and
       `current-deploy.system` was healthy, and
       `backups/current-deploy/` exists, and
       `microshift` in `backups/current-deploy/version.json` 
        is older than `microshift/version.json`, then
        restore `backups/current-deploy/`
   - proceed to data migration

1. else if `prev-boot-deploy.system` is `unknown`
   > MicroShift started, but system didn't get to a point when green/red script update `health.json`.
   > It might've been power loss or hard reboot.
   - `current-deploy.id` is the same as `prev-boot-deploy.id`
     * exit 0 and allow `microshift run`
   - `current-deploy.id` differs from `prev-boot-deploy.id`
     > Assuming it wasn't "quickly stage new deployment and do hard reboot" because that would be irresponsible.
     > Power loss might've happen on last boot before rollback (`boot_counter=0`) and now it's rollback deployment.
     * Assume `prev-boot-deploy.system` is `unhealthy`, go to next point

1. `prev-boot-deploy.system` is `unhealthy`
   > Ideally, we'd like to restore `backups/current-deploy/`.

   - `current-deploy` != `prev-boot-deploy`
     > Previous boot was unhealthy, but this is boot is different deployment.

     - `current-deploy` does not exist in `health.json`
       > First boot of new deployment, `prev-boot-deploy` was unhealthy == new deployment staged over (possibly) unhealthy.
       > Admin didn't address the issues or forgot to mark `prev-boot` as healthy - either way,
       > for MicroShift it's `unhealthy` and we don't want to upgrade from such data.
       * Backup `microshift/` as `backups/unhealthy__prev-boot-deploy/`.
       * Delete data, start clean.

     - `current-deploy` exists in `health.json`
       > Deployment already ran on the system.
       > System rolled back automatically or on demand (by admin).

       - `current-deploy` was `healthy`
         - `backups/current-deploy/` exists
           * Restore from `backups/current-deploy/`

         - `backups/current-deploy/` does not exist
           > Reminder: `prev-boot-deploy` was unhealthy, `current-deploy` was healthy and is missing backup
           - `version.json` matches `current-deploy.id`
             > Means that `prev-boot-deploy` possibly failed to make a backup. `version.json` is untouched so no migration attempt.
             * Backup `backups/current-deploy/`, continue running off `microshift/`
           - `version.json` does not match `current-deploy.id`
             > Migrated without backup? Bug or user interference.
             * Abort

       - `current-deploy` was `unhealthy`
         > `prev-boot-deploy` was staged over `unhealthy` `current-deploy` without metadata cleanup
         > and now system rolled back.
         > After addressing issues, admin should mark deployment as healthy, so **this shouldn't happen**.
         > Let's assume admin addressed the issues but forgot to mark as healthy.

         - `version.json` matches `current-deploy.id`
           * Move `backups/current-deploy/` to `backups/last_healthy__current-deploy/`
           * Backup `microshift/` to `backups/current-deploy/`
           * Proceed with `microshift run`

         - `version.json` does not match `current-deploy.id`
           > Other deployment attempted to migrate data
           - `backups/current-deploy/` exists
             > It was `healthy` at least once, but not on last run. Rolling back to last healthy state.
             * Restore from `backups/current-deploy/`

           - `backups/current-deploy/` does not exist
             * Delete data, start clean.

   - `current-deploy` == `prev-boot-deploy`
     > Because `current-deploy` is already present in `health.json`, it's 2nd, 3rd,... boot of the deployment (not 1st).

     - `backups/current-deploy/` exists
       > Deployment was healthy at least once already.
       > It means greenboot removed `boot_counter` and current boot isn't due to greenboot's automatic reboot.
       > After addressing issues, admin should mark deployment as healthy, so this shouldn't happen.
       > Let's assume admin addressed the issues but forgot to mark as healthy and just rebooted the device.
       * No backup, no restore, skip migration, allow `microshift run`
         > If admin addressed issues, it should be healthy again.
         > If issue persists, it'll end up unhealthy requiring manual intervention
         > (which is the same result if we'd block `microshift run`).

     - `backups/current-deploy/` doesn't exist
       > Deployment was always unhealthy.

       - `earlier-deploy` does not exist in `health.json`
         > For MicroShift it's first deployment it's running, but it doesn't mean it's the only deployment.
         > (there could be previous (rollback) deployment without MicroShift)
         >
         > ostree status:
         > 1. `current` == `prev-boot` - currently running, last boot unhealthy, greenboot didn't accept it yet
         > 1. (optionally) `earlier` - (auto) rollback - **without MicroShift**
         * delete data, try from clean state

       - `earlier-deploy` exists in `health.json`
         > For MicroShift it looks like it ran already on the device, but `earlier-deploy` might or might not be
         > the rollback deployment.
         >
         > ostree status:
         > 1. `current` == `prev-boot` - currently running, last boot unhealthy, greenboot didn't accept it yet
         > 1. (auto) rollback (might be `earlier` or not)

         - `earlier-deploy` == other deploy (not active) from `rpm-ostree status`
           > ostree status:
           > 1. `current` == `prev-boot` - currently running, last boot unhealthy, greenboot didn't accept it yet
           > 1. `earlier` - (auto) rollback
           >
           > Let's sum it up: it's 2nd or 3rd boot of `current-deploy` and it's `unhealthy` since 1st boot.
           > It was staged directly from `earlier` which also ran MicroShift.

           - `earlier-deploy` was healthy
             - `microshift/version.json` matches `earlier-deploy`
               > `current-deploy` didn't get to data migration, maybe backup failed.
               > `backups/earlier-deploy` may or may not be most recent backup of data.
               * backup `microshift/` as `backups/earlier-deploy/`
               * proceed to data migration

             - `version.json` doesn't match `earlier-deploy`
               - `backups/earlier-deploy/` exists
                 > Failed data migration or runtime problem. Retry upgrade like it's 1st boot.
                 * Restore data from `backups/earlier-deploy/` and proceed to data migration.
               - `backups/earlier-deploy/` doesn't exist
                 > This might mean that migration ran without prior backup - bug or user's interference.
                 * exit 1, block `microshift run`
           - `earlier-deploy` was unhealthy
             > Admin staged newer deployment over unhealthy one without prior cleanup.
             > This should be caught earlier by: `prev-boot.system == unhealthy` and `current` missing from `health.json`
             * just in case: exit 1, block `microshift run`

         - `earlier-deploy` != other deploy (not active) from `rpm-ostree status`
           > Matches "FIDO" scenario from workflows.
           > Admin didn't cleanup MicroShift's artifacts.
           > This should be caught by `current-deploy` != `prev-boot-deploy`
           >
           > ostree status:
           > 1. `current` == `prev-boot` - currently running
           > 1. (auto) rollback
           - delete `microshift/` and start clean

1. compare binary's `version` with value of `microshift` in `version.json`
   - binary's `Y` is smaller: **exit 1** :stop_sign:	
   - binary's `Y` is bigger by more than `1`: **exit 1** :stop_sign:	
   - `version.json` is present in binary's "list of blocked migrations": **exit 1** :stop_sign:	
   - `Y`s are the same: **exit 0** :leftwards_arrow_with_hook:
1. Perform data migration
   - Update `version.json`: `{ "microshift": "migrating-from-4.Y1.Z1-to-4.Y2.Z2", "deployment: "$current"}`
   - Success: **exit 0** :leftwards_arrow_with_hook:
     - Update `version.json`: `{ "microshift": "4.Y2.Z2", "deployment: "$current"}`
   - Failure: **exit 1** :stop_sign:	
     - Update `version.json`: `{ "microshift": "failed-migrating-from-4.Y1.Z1-to-4.Y2.Z2", "deployment: "$current"}`

`microshift run`
1. If data exists and `version.json` doesn't match binary's `version`
   > Means that data's version is different and migration is needed
   - **exit 1**
1. Create data dir if necessary
1. Create `version.json` if needed
1. Add new or update entry in `health.json`:
   ```json
    {
      "deployment_id": "$current",
      "microshift": "unknown",
      "system": "unknown",
      "last_boot": "yyyy-mm-dd HH:MM:SS"
    }
   ```
1. Continue regular flow

`microshift healthcheck`
1. Assess health of MicroShift
1. Update `health.json`: `deployments[$current].microshift` <- with `healthy` or `unhealthy`

`microshift persist-system-health`
1. Update `health.json`: `deployments[$current].system` <- with `healthy` or `unhealthy`


#### Manual interventions

##### Addressing MicroShift's health

Depending on MicroShift's health admin might:
- unhealthy
  - delete MicroShift's data, to allow fresh start
  - investigate and address problems with MicroShift cluster
- healthy
  - keep MicroShift's data
- unhealthy application running on top of MicroShift
  - investigate and address problems with the app

After admin addresses issues it should either:
- re-trigger greenboot healthcheck (`systemctl restart greenboot-healthcheck`), or
- mark deployment as `healthy` using `microshift admin mark-deploy-as-healthy`
  (so next boot has correct information about previous boot health)

If admin addressed issues, but forget to update deployment's health before booting into different deployment,
a backup of `unhealthy` deployment will be made with prefix `unhealthy__`.

##### Backup exists, restore succeeds, but system is unhealthy

> Scenarios:
> - System was rolled back to 1st deployment
>   (no more greenboot reboots, previous deployment was `unhealthy` -> restore) 
> - 1st and only ostree deployment with MicroShift:
>   - 1st boot OK, **manual** reboot
>   - 2nd boot: data backed up, system NOK, **manual** reboot
>   - 3rd boot: data restored, system NOK

1. `microshift pre-run`
   - `prev-boot-deploy.system` is unhealthy -> restore `current-deploy`
   - `backups/current-deploy` exists and is successfully restored
   - exit 0
1. `microshift run`
   - Updates `version.json`, `health.json`, etc.
1. System is unhealthy (red)
1. Greenboot doesn't reboot device because `boot_counter` is only set when ostree deployment is staged
1. Current state
   ```json
   // /var/lib/microshift/version.json
   {"microshift": "4.14.0", "deployment": "rhel-d1"}

   // ls /var/lib/microshift-backups/
   health.json
   rhel-d1/

   // /var/lib/microshift-backups/health.json
   { "deployments": [
       {
         "id": "rhel-d1",
         "microshift": "(un)healthy",
         "system": "unhealthy",
         "last_boot": "2023-07-01 02:00:00"
       },
       // optionally
       {
         "id": "rhel-d2",
         "microshift": "(un)healthy",
         "system": "unhealthy",
         "last_boot": "2023-07-01 01:00:00"
       }
       // end optionally
    ]}
   ```
1. System requires manual intervention

   - Admin simply reboots the device
     1. 1st ostree deployment boots
     1. `microshift pre-run`
        - `prev-boot-deploy.system` is `unhealthy` -> plan to restore
        - `backups/rhel-d1` exists, restore successful
     1. `microshift run`
     1. System might be healthy or unhealthy again, so back to the beginning of the flow

   - Admin addresses the issue
     1. [MicroShift's health](#addressing-microshifts-health)
     1. Other components - admin's judgement
     1. Reboots the device
     1. `microshift pre-run`
        - `prev-boot-deploy.system` is `unhealthy` -> plan to restore
        - `backups/rhel-d1` exists, restore successful
     1. `microshift run`
     1. System might be healthy or unhealthy again, so back to the beginning of the flow

##### Backup exists, restore fails, so MicroShift is unhealthy

> Scenarios:
> - System was rolled back to 1st deployment
>   (no more greenboot reboots, previous deployment was `unhealthy` -> restore) 
> - 1st and only ostree deployment with MicroShift:
>   - 1st boot OK, **manual** reboot
>   - 2nd boot: data backed up, system NOK, **manual** reboot
>   - 3rd boot: failed to restore data, system NOK

1. `microshift pre-run`
   - `prev-boot-deploy.system` is unhealthy -> restore `current-deploy`
   - `backups/current-deploy` exists but restoring fails
   - exit 0
1. `microshift run` is **not executed**
1. MicroShift is unhealthy, therefore system is also unhealthy
1. Greenboot doesn't reboot device because `boot_counter` is only set when ostree deployment is staged
1. System requires manual intervention
1. Current state
   ```json
   // /var/lib/microshift/version.json
   {"microshift": "4.14.0", "deployment": "rhel-d1"}

   // ls /var/lib/microshift-backups/
   health.json
   rhel-d1/

   // /var/lib/microshift-backups/health.json
   { "deployments": [
       {
         "id": "rhel-d1",
         "microshift": "unhealthy",
         "system": "unhealthy",
         "last_boot": "2023-07-01 02:00:00"
       },
       // optionally
       {
         "id": "rhel-d2",
         "microshift": "unhealthy",
         "system": "unhealthy",
         "last_boot": "2023-07-01 01:00:00"
       }
       // end optionally
    ]}
   ```
1. Admin addresses underlying issues: frees up disk space, fixes permissions, etc.
1. Reboots the device
1. `microshift pre-run`
   - `prev-boot-deploy.system` is unhealthy -> restore `current-deploy`
   - `backups/current-deploy` exists, restore successful
   - exit 0
1. `microshift run`

##### 1st deployment is unhealthy, admin wants to stage another one

> Scenario:
> - 1st deployment on system is unhealthy beyond recovery
> - Admin decides to try another deployment with different MicroShift build

1. 1st deployment is unhealthy
1. Greenboot doesn't reboot because there's only one (no rollback, no `boot_counter`)
1. Admin stops MicroShift: `systemctl stop microshift`
1. Admin removes `microshift/` and `microshift-backups/`
1. Admin stages 2nd deployment
1. System is rebooted
2. 2nd deployment starts
1. `microshift pre-run`
   - Neither `microshift/` nor `microshift-backups/` exist
   - Assume first boot
   - Exit 0, allow `microshift run`
1. `microshift run`

##### Rollback on demand

> Scenario:
> - System is running on 2nd deployment
> - Both 1st and 2nd deployment are healthy
>   (backup exists for 1st, backup for 2nd may or may not yet exist)
> - Admin wants to rollback to 1st deployment

1. 2nd deployment is running and healthy
1. Admin runs `rpm-ostree rollback` or equivalent
1. 2nd deployment shuts down, 1st deployment boots
1. `microshift pre-run`
   - `prev-boot-deploy.system` is `healthy`
     - backing up `microshift/` to `backups/prev-boot-deploy.id/`
   - `current-deploy.id` is different from `prev-boot-deploy.id`, and
     `current-deploy.system` was healthy, and
     `backups/current-deploy/` exists, and
     `microshift` in `backups/current-deploy/version.json` is older than `microshift/version.json`, then
     restore `backups/current-deploy/`
   - no need to migrate data because `version.json` matches executable's version
1. `microshift run`

#### First ostree deployment

##### First deployment, first boot

1. Device is freshly provisioned
1. Deployment `rhel-d1` starts
1. `microshift pre-run`
   - neither `microshift/` nor `microshift-backups/` exist
   - exit 0
1. MicroShift starts
   - Creates data dir structure
   - Creates `version.json`:
     ```json
     { "microshift": "4.14.0", "deployment": "rhel-d1"}
     ```
   - Creates `health.json` with:
     ```json
     {
       "deployments": [
         {
           "deployment_id": "rhel-d1",
           "microshift": "unknown",
           "system": "unknown",
           "last_boot": "2023-07-01 00:00:00"
         }
       ]
     }
     ```
   - Continue regular start up
1. MicroShift healthcheck for greenboot
   - `health.yaml`: `deployments[$current].microshift` <- `healthy|unhealthy`
1. Greenboot green/red scripts
   - `health.yaml`: `deployments[$current].system` <- `healthy|unhealthy`
1. Alternative scenarios
   - System and MicroShift are healthy
     1. [system is rebooted, backup fail](#reboot-second-boot-backup-fails)
     1. [system is rebooted, backup succeeds](#reboot-second-boot-backup-succeeds)

   - System or MicroShift are unhealthy
     1. Greenboot doesn't reboot device because `boot_counter` is only set when ostree deployment is staged
     1. System requires manual intervention.

##### First deployment, second boot (reboot): backup fails

> First boot was healthy

1. 1st ostree deployment shuts down
1. 1st ostree deployment boots
1. Current state
   ```json
   // /var/lib/microshift/version.json
   {"microshift": "4.14.0", "deployment": "rhel-d1"}

   // ls /var/lib/microshift-backups/
   health.json

   // /var/lib/microshift-backups/health.json
   { "deployments": [{
         "id": "rhel-d1",
         "microshift": "healthy",
         "system": "healthy",
         "last_boot": "2023-07-01 00:00:00"
       }]}
   ```
1. `microshift pre-run`
   - `prev-boot-deploy.system == healthy` -> backup for `prev-boot-deploy.id`
     - Copy `/var/lib/microshift` to `/var/lib/microshift-backups/rhel-d1`
     - Fails ^
   - exit 1
1. `microshift.service` doesn't run
1. MicroShift healthcheck for greenboot
   - `/var/lib/microshift-backups/health.yaml`: `deployments[$current].microshift` <- `unhealthy`
1. Greenboot green/red scripts
   - `/var/lib/microshift-backups/health.yaml`: `deployments[$current].system` <- `unhealthy`
1. Greenboot doesn't reboot device because `boot_counter` is only set when ostree deployment is staged
1. System requires manual intervention.

##### Reboot: second boot, backup succeeds

> First boot was healthy

1. 1st ostree deployment shuts down
1. 1st ostree deployment boots
1. Current state
   ```json
   // /var/lib/microshift/version.json
   {"microshift": "4.14.0", "deployment": "rhel-d1"}

   // ls /var/lib/microshift-backups/
   health.json

   // /var/lib/microshift-backups/health.json
   { "deployments": [{
         "id": "rhel-d1",
         "microshift": "healthy",
         "system": "healthy",
         "last_boot": "2023-07-01 00:00:00"
       }]}
   ```
1. `microshift pre-run`
   - `prev-boot-deploy.system == healthy` -> backup for `prev-boot-deploy.id`
     - Copy `/var/lib/microshift` to `/var/lib/microshift-backups/rhel-d1`
   - Compare `version.json.microshift` with `binary.version`
     - The same - no migration
   - exit 0
1. `microshift run`
   - Updates `version.json`
   - Updates `health.json`: 
     ```json
     {
      "id": "rhel-d1",
      "microshift": "unknown",
      "system": "unknown",
      "last_boot": "2023-07-01 01:00:00"
     }
     ```
1. MicroShift healthcheck for greenboot
   - `/var/lib/microshift-backups/health.yaml`: `deployments[$current].microshift` <- `healthy|unhealthy`
1. Greenboot green/red scripts
   - `/var/lib/microshift-backups/health.yaml`: `deployments[$current].system` <- `healthy|unhealthy`
1. Current state
   ```json
   // /var/lib/microshift/version.json
   {"microshift": "4.14.0", "deployment": "rhel-d1"}

   // ls /var/lib/microshift-backups/
   health.json
   rhel-d1/

   // /var/lib/microshift-backups/health.json
   { "deployments": [{
         "id": "rhel-d1",
         "microshift": "healthy | unhealthy",
         "system": "healthy | unhealthy",
         "last_boot": "2023-07-01 01:00:00"
       }]}
   ```
1. Optional: System is unhealthy
   - Greenboot doesn't reboot device because `boot_counter` is only set when ostree deployment is staged
   - System requires manual intervention.

#### Second ostree deployment is staged

Pre-steps:

1. 2nd deployment is staged
1. Greenboot sets `boot_counter`  
1. 1st deployment shuts down
1. 2nd deployment boots

##### Backup succeeds, no MicroShift change, no cluster app change

> No changes are made to MicroShift version or apps running within the cluster, 
> so new ostree deployment might feature unrelated changes or RPMs


1. Current state
   ```json
   // /var/lib/microshift/version.json
   {"microshift": "4.14.0", "deployment": "rhel-d1"}

   // ls /var/lib/microshift-backups/
   health.json

   // /var/lib/microshift-backups/health.json
   { "deployments": [{
         "id": "rhel-d1",
         "microshift": "healthy",
         "system": "healthy",
         "last_boot": "2023-07-01 00:00:00"
       }]}
   ```
1. `microshift pre-run`
   - `prev-boot-deploy.system == healthy` -> backup for `prev-boot-deploy.id`
     - Copy `/var/lib/microshift` to `/var/lib/microshift-backups/rhel-d1`
   - Compare `version.json` with `binary.version`
     - The same - no migration
   - exit 0
1. `microshift run`
   - Updates `version.json`: `{"microshift": "4.14.0", "deployment": "rhel-d2"}`
   - Adds to `health.json`: 
     ```json
     {
      "id": "rhel-d2",
      "microshift": "unknown",
      "system": "unknown",
      "last_boot": "2023-07-01 01:00:00"
     }
     ```
1. MicroShift healthcheck for greenboot
   - `health.yaml`: `deployments[$current].microshift` <- `healthy|unhealthy`
1. Greenboot green/red scripts
   - `health.yaml`: `deployments[$current].system` <- `healthy|unhealthy`
1. Current state
   ```json
   // /var/lib/microshift/version.json
   {"microshift": "4.14.0", "deployment": "rhel-d2"}

   // ls /var/lib/microshift-backups/
   health.json
   rhel-d1/

   // /var/lib/microshift-backups/health.json
   { "deployments": [
       {
        "id": "rhel-d2",
        "microshift": "(un)healthy",
        "system": "(un)healthy",
        "last_boot": "2023-07-01 02:00:00"
       },
       {
         "id": "rhel-d1",
         "microshift": "healthy",
         "system": "healthy",
         "last_boot": "2023-07-01 01:00:00"
       }
   ]}
   ```

(Optional)
1. System is unhealthy
1. Greenboot reboots system
1. Current state
   ```json
   // /var/lib/microshift/version.json
   {"microshift": "4.14.0", "deployment": "rhel-d2"}

   // ls /var/lib/microshift-backups/
   health.json
   rhel-d1/

   // /var/lib/microshift-backups/health.json
   { "deployments": [
       {
        "id": "rhel-d2",
        "microshift": "unhealthy",
        "system": "unhealthy",
        "last_boot": "2023-07-01 02:00:00"
       },
       {
         "id": "rhel-d1",
         "microshift": "healthy",
         "system": "healthy",
         "last_boot": "2023-07-01 01:00:00"
       }
   ]}
   ```
1. `microshift pre-run`
   - `prev-boot-deploy.system == unhealthy` -> restore `current-deploy`
   - `backups/rhel-d2/` does not exist - can't restore
   - Compare `most_recent(deployment).id` and `current-deploy` - the same, so it's Nth boot of red deployment
     try to restore backup for previous deployment
   - Get second to most recent deployment: `rhel-d1` 
   - It was `healthy`, `/var/lib/microshift-backups/rhel-d1/` exists, 
     `version.json` doesn't match `rhel-d1` -> restore previous
   - Restore `/var/lib/microshift-backups/rhel-d1` -> `/var/lib/microshift`
   - Compare `version.json` with `binary.version`
     - The same - no migration
   - exit 0
1. `microshift run`
   - Updates `version.json`: `{"microshift": "4.14.0", "deployment": "rhel-d2"}`
   - Adds to `health.json`: 
     ```json
     {
      "id": "rhel-d2",
      "microshift": "unknown",
      "system": "unknown",
      "last_boot": "2023-07-01 01:00:00"
     }
     ```
1. MicroShift healthcheck for greenboot
   - `/var/lib/microshift-backups/health.yaml`: `deployments[$current].microshift` <- `healthy|unhealthy`
1. Greenboot green/red scripts
   - `/var/lib/microshift-backups/health.yaml`: `deployments[$current].system` <- `healthy|unhealthy`
1. **System was unhealthy (red) each boot**
   - `boot_counter` reaches `-1`
   - **grub boots `rhel-d1` (rollback)**
1. Current state
   ```json
   // /var/lib/microshift/version.json
   {"microshift": "4.14.0", "deployment": "rhel-d2"}

   // ls /var/lib/microshift-backups/
   health.json
   rhel-d1/

   // /var/lib/microshift-backups/health.json
   { "deployments": [
       {
        "id": "rhel-d2",
        "microshift": "unhealthy",
        "system": "unhealthy",
        "last_boot": "2023-07-01 02:00:00"
       },
       {
         "id": "rhel-d1",
         "microshift": "healthy",
         "system": "healthy",
         "last_boot": "2023-07-01 01:00:00"
       }
   ]}
   ```
1. `microshift pre-run`
   - `prev-boot-deploy.system == unhealthy` -> restore `$current-deploy`
   - `/var/lib/microshift-backups/rhel-d1/` exists
   - Restore `/var/lib/microshift-backups/rhel-d1` -> `/var/lib/microshift`
   - exit 0
1. `microshift run`
   - Updates `version.json`: `{"microshift": "4.14.0", "deployment": "rhel-d1"}`
   - Updates `health.json`: 
     ```json
     {
      "id": "rhel-d1",
      "microshift": "unknown",
      "system": "unknown",
      "last_boot": "2023-07-01 02:00:00"
     }
     ```
1. Current state
   ```json
   // /var/lib/microshift/version.json
   {"microshift": "4.14.0", "deployment": "rhel-d1"}

   // ls /var/lib/microshift-backups/
   health.json
   rhel-d1/

   // /var/lib/microshift-backups/health.json
   { "deployments": [
       {
        "id": "rhel-d2",
        "microshift": "unhealthy",
        "system": "unhealthy",
        "last_boot": "2023-07-01 02:00:00"
       },
       {
         "id": "rhel-d1",
         "microshift": "unknown",
         "system": "unknown",
         "last_boot": "2023-07-01 03:00:00"
       }
   ]}
   ```

##### First deployment was active only for one boot, backup fails

1. Current state
   ```json
   // /var/lib/microshift/version.json
   {"microshift": "4.14.0", "deployment": "rhel-d1"}

   // ls /var/lib/microshift-backups/
   health.json

   // /var/lib/microshift-backups/health.json
   { "deployments": [
       {
         "id": "rhel-d1",
         "microshift": "healthy",
         "system": "healthy",
         "last_boot": "2023-07-01 00:00:00"
       }
   ]}
   ```
1. `microshift pre-run`
   - `prev-boot-deploy.system == healthy` -> backup for `prev-boot-deploy.id`
     - `mkdir -p /var/lib/microshift/backups/`
     - `cp -r --reflink=auto --preserve /var/lib/microshift/live /var/lib/microshift/backups/deploy1`
     - Fails ^
   - exit 1
1. `microshift run` doesn't start
1. MicroShift healthcheck fails, greenboot red script
1. System is rebooted
1. Current state
   ```json
   // /var/lib/microshift/version.json
   {"microshift": "4.14.0", "deployment": "rhel-d1"}

   // ls /var/lib/microshift-backups/
   health.json

   // /var/lib/microshift-backups/health.json
   { "deployments": [
       {
         "id": "rhel-d2",
         "microshift": "unhealthy",
         "system": "unhealthy",
         "last_boot": "2023-07-01 01:00:00"
       },
       {
         "id": "rhel-d1",
         "microshift": "healthy",
         "system": "healthy",
         "last_boot": "2023-07-01 00:00:00"
       }
   ]}
   ```
1. `microshift pre-run`
   - Most recent deploy is `unhealthy` - restore `$current-deploy-id`?
   - `/var/lib/microshift-backups/rhel-d2/` does not exist - can't restore
   - Compare `most_recent(deployment).id` and `$current-deployment-id` - the same,
     so let's try restore backup for previous deployment
   - Get second to most recent deployment: `rhel-d1` 
   - It was `healthy` but `/var/lib/microshift-backups/rhel-d1/` does not exist *(backup failed)*
   - Deployment ID from `health.json` matches what's in `version.json`
   - Assume `/var/lib/microshift/` is still healthy and try to back up
   - Backup fails again
1. Greenboot reboots system multiple times (always red boot)
1. `boot_counter` reaches `-1`
1. grub boots previous (1st) deployment (rollback)
1. Current state
   ```json
   // /var/lib/microshift/version.json
   {"microshift": "4.14.0", "deployment": "rhel-d1"}

   // ls /var/lib/microshift-backups/
   health.json

   // /var/lib/microshift-backups/health.json
   { "deployments": [
       {
         "id": "rhel-d2",
         "microshift": "unhealthy",
         "system": "unhealthy",
         "last_boot": "2023-07-01 04:00:00"
       },
       {
         "id": "rhel-d1",
         "microshift": "healthy",
         "system": "healthy",
         "last_boot": "2023-07-01 00:00:00"
       }
   ]}
   ```
1. `microshift pre-run`
   - Most recent deploy is `unhealthy` - try to restore `$current-deploy-id`
   - `/var/lib/microshift-backups/rhel-d1/` does not exist - can't restore
   - See if `deployments` contains `$current-deploy-id` - yes
   - Was it `healthy`? Yes
   - Check if `$current-deploy-id` is in `version.json` - yes
   - Assume `/var/lib/microshift/` is still healthy and try to back up
   - Backup fails again
   - exit 1
1. Manual intervention needed

#### Rollback to first deployment, failed restore

1. 2nd deployment boots
1. Current state
   ```json
   // /var/lib/microshift/version.json
   {"microshift": "4.14.0", "deployment": "rhel-d1"}

   // ls /var/lib/microshift-backups/
   health.json

   // /var/lib/microshift-backups/health.json
   { "deployments": [
       {
         "id": "rhel-d1",
         "microshift": "healthy",
         "system": "healthy",
         "last_boot": "2023-07-01 00:00:00"
       }
   ]}
   ```
1. `microshift pre-run`
   - `most_recent(deployment).system` is `healthy` -> backup
     - Copy `/var/lib/microshift` to `/var/lib/microshift-backups/rhel-d1`
     - No migration
   - Exit 0
1. `microshift run`
1. System is unhealthy
1. Greenboot reboots system multiple times (always red boot)
1. `boot_counter` reaches `-1`
1. grub boots previous deployment (rollback)
1. Current state
   ```json
   // /var/lib/microshift/version.json
   {"microshift": "4.14.0", "deployment": "rhel-d2"}

   // ls /var/lib/microshift-backups/
   health.json
   rhel-d1/

   // /var/lib/microshift-backups/health.json
   { "deployments": [
       {
         "id": "rhel-d2",
         "microshift": "(un)healthy",
         "system": "unhealthy",
         "last_boot": "2023-07-01 00:00:00"
       },
       {
         "id": "rhel-d1",
         "microshift": "healthy",
         "system": "healthy",
         "last_boot": "2023-07-01 00:00:00"
       }
   ]}
   ```
1. `microshift pre-run`
   - `most_recent(deployment).system` is `unhealthy` -> restore `$current_deployment` (`rhel-d1`)
     - Delete `/var/lib/microshift`
     - Copy `/var/lib/microshift-backups/rhel-d1` -> `/var/lib/microshift`
     - Failure
   - Exit 1
1. `microshift run` doesn't start
1. System is unhealthy (red)
1. `boot_counter` is unset *(it's already a rollback)*
1. Current state
   ```json
   // /var/lib/microshift/version.json
   {"microshift": "4.14.0", "deployment": "rhel-d2"}

   // ls /var/lib/microshift-backups/
   health.json
   rhel-d1/

   // /var/lib/microshift-backups/health.json
   { "deployments": [
       {
         "id": "rhel-d2",
         "microshift": "(un)healthy",
         "system": "unhealthy",
         "last_boot": "2023-07-01 01:00:00"
       },
       {
         "id": "rhel-d1",
         "microshift": "unhealthy",
         "system": "unhealthy",
         "last_boot": "2023-07-01 02:00:00"
       }
   ]}
   ```
1. Manual intervention is required.
   See [Backup exists, restore fails, so MicroShift is unhealthy](#backup-exists-restore-fails-so-microshift-is-unhealthy).


#### Fail first startup, FDO (FIDO Device Onboard) deployment

1. ostree deployment without MicroShift (`rhel-d0`) is installed on the device at the factory
1. The device boots at a customer site
1. An agent in the ostree commit performs FIDO device onboarding or a 
   similar process to determine the workload
1. ostree deployment with MicroShift installed (`rhel-d1`) is staged
   - greenboot sets `boot_counter`
1. The sans-MicroShift (`rhel-d0`) deployment shuts down
1. The with-MicroShift deployment (`rhel-d1`) starts up
1. `microshift pre-run`
   - First boot scenario
1. `microshift run`
   - Create dir structure, `version.json`, `health.json`, etc.
1. System is unhealthy, red scripts
1. Current state
   ```json
   // /var/lib/microshift/version.json
   {"microshift": "4.14.0", "deployment": "rhel-d1"}

   // ls /var/lib/microshift-backups/
   health.json

   // /var/lib/microshift-backups/health.json
   { "deployments": [
       {
         "id": "rhel-d1",
         "microshift": "unhealthy",
         "system": "unhealthy",
         "last_boot": "2023-07-01 00:00:00"
       }
   ]}
   ```
1. Greenboot reboots the system, but red boots continue to happen
1. `boot_counter` falls to `-1`
1. grub boots ostree deployment sans-MicroShift (`rhel-d0`)
1. The agent stages **2nd** ostree deployment with-MicroShift (`rhel-d2`)
   - greenboot sets `boot_counter`
1. The sans-MicroShift deployment (`rhel-d0`) shuts down
1. The **2nd** ostree deployment with-MicroShift (`rhel-d2`) starts up.
1. Current state
   ```json
   // /var/lib/microshift/version.json
   {"microshift": "4.14.0", "deployment": "rhel-d1"}

   // ls /var/lib/microshift-backups/
   health.json

   // /var/lib/microshift-backups/health.json
   { "deployments": [
       {
         "id": "rhel-d1",
         "microshift": "unhealthy",
         "system": "unhealthy",
         "last_boot": "2023-07-01 01:00:00"
       }
   ]}
   ```
1. `microshift pre-run`
   - Data exists (left over of 1st ostree deployment with-MicroShift)
   - `prev-boot-deploy.system` is `unhealthy` -> restore `$current_deployment` (`rhel-d2`)
     - `/var/lib/microshift-backups/rhel-d2` doesn't exist
     - `deployments[rhel-d2]` doesn't exist *(microshift didn't run on this deploy yet)*
     - Get second to most recent deployment - doesn't exist, there's only one and it's unhealthy
     - Delete `/var/lib/microshift`
   - exit 0
1. `microshift run`
   - Creates `version.json`, updates `health.json`, etc.
1. Current state
   ```json
   // /var/lib/microshift/version.json
   {"microshift": "4.14.0", "deployment": "rhel-d2"}

   // ls /var/lib/microshift-backups/
   health.json

   // /var/lib/microshift-backups/health.json
   { "deployments": [
       {
         "id": "rhel-d1",
         "microshift": "unhealthy",
         "system": "unhealthy",
         "last_boot": "2023-07-01 01:00:00"
       },
       {
         "id": "rhel-d2",
         "microshift": "unknown",
         "system": "unknown",
         "last_boot": "2023-07-01 03:00:00"
       }
   ]}
   ```
1. System is unhealthy, red scripts
1. Current state
   ```json
   // /var/lib/microshift/version.json
   {"microshift": "4.14.0", "deployment": "rhel-d2"}

   // ls /var/lib/microshift-backups/
   health.json

   // /var/lib/microshift-backups/health.json
   { "deployments": [
       {
         "id": "rhel-d1",
         "microshift": "unhealthy",
         "system": "unhealthy",
         "last_boot": "2023-07-01 01:00:00"
       },
       {
         "id": "rhel-d2",
         "microshift": "unhealthy",
         "system": "unhealthy",
         "last_boot": "2023-07-01 03:00:00"
       }
   ]}
   ```
1. Greenboot reboots the system, but red boots continue to happen
1. `boot_counter` falls to `-1`
1. grub boots ostree deployment sans-MicroShift (`rhel-d0`)
1. The agent stages **3rd** ostree deployment with-MicroShift (`rhel-d3`)
   - greenboot sets `boot_counter`
1. The sans-MicroShift deployment (`rhel-d0`) shuts down
1. The **3rd** ostree deployment with-MicroShift (`rhel-d3`) starts up.
1. Current state
   ```json
   // /var/lib/microshift/version.json
   {"microshift": "4.14.0", "deployment": "rhel-d2"}

   // ls /var/lib/microshift-backups/
   health.json

   // /var/lib/microshift-backups/health.json
   { "deployments": [
       {
         "id": "rhel-d1",
         "microshift": "unhealthy",
         "system": "unhealthy",
         "last_boot": "2023-07-01 01:00:00"
       },
       {
         "id": "rhel-d2",
         "microshift": "unhealthy",
         "system": "unhealthy",
         "last_boot": "2023-07-01 03:00:00"
       }
   ]}
   ```
1. `microshift pre-run`
   - Data exists (left over of 2nd ostree deployment with-MicroShift)
   - `prev-boot-deploy.system` is `unhealthy` -> restore `$current_deployment` (`rhel-d3`)
     - `/var/lib/microshift-backups/rhel-d3` doesn't exist
     - `deployments[rhel-d3]` doesn't exist *(microshift didn't run on this deploy yet)*
     - Get second to most recent deployment - `deployments[rhel-d2]` was also unhealthy
     - Delete `/var/lib/microshift`
   - exit 0
1. `microshift run`
   - Creates `version.json`, updates `health.json`, etc.

#### Flow 4.13 -> 4.14 -rollback-> 4.13 -> 4.14

1. Deployment with MicroShift 4.13 (`rhel-d0`) is running
1. Deployment with MicroShift 4.14 (`rhel-d1`) is staged
1. `rhel-d0` shuts down, `rhel-d1` boots
1. Current state
   ```plaintext
   /var/lib/microshift/              exists
   /var/lib/microshift/version.json  missing
   /var/lib/microshift-backups/      missing
   ```
1. `microshift pre-run`
   - `microshift/` exists, `version.json` and `health.json` do not
     - create `version.json` with `{"microshift": "4.13", "deployment": "4.13"}`
     - action: backup
   - Copy `/var/lib/microshift` to `/var/lib/microshift-backups/4.13`
   - Compare `version.json` with `microshift version`
     - Attempt storage migration
1. `microshift run`
1. System is unhealthy due to different reasons:
   - Upgrade was blocked or storage migration failed
   - MicroShift was unhealthy
   - Something else was unhealthy
1. Current state
   ```json
   // /var/lib/microshift/version.json
   {"microshift": "4.14.0", "deployment": "rhel-d1"}

   // ls /var/lib/microshift-backups/
   health.json
   4.13/

   // /var/lib/microshift-backups/health.json
   { "deployments": [
       {
         "id": "rhel-d1",
         "microshift": "(un)healthy",
         "system": "unhealthy",
         "last_boot": "2023-07-01 01:00:00"
       }
   ]}
   ```
1. System is rebooted, red boot continue
1. Rollback to `rhel-d0` deployment
1. `microshift run`
   - Fails due to data inconsistency
1. Manual intervention is needed
   - Admin overwrites `/var/lib/microshift` with `microshift-backups/4.13`
1. Deployment with MicroShift 4.14 (`rhel-d2`) is staged
1. `rhel-d0` shuts down, `rhel-d2` boots
1. Current state
   ```json
   // /var/lib/microshift/version.json
   {"microshift": "4.13", "deployment": "4.13"}

   // ls /var/lib/microshift-backups/
   health.json
   4.13/

   // /var/lib/microshift-backups/health.json
   { "deployments": [
       {
         "id": "rhel-d1",
         "microshift": "(un)healthy",
         "system": "unhealthy",
         "last_boot": "2023-07-01 01:00:00"
       }
   ]}
   ```
1. `microshift pre-run`
   - `microshift/` `version.json` and `health.json` exist
   - `prev-boot-deploy` is `unhealthy`, we want to restore `$current-deploy` (`rhel-d2`)
   - `microshift-backups/$current-deploy` doesn't exist
   - `$current-deploy` is not in `deployments`
   - check if `version` contains `4.13`
   - if `microshift version` supports upgrade from `4.13`, attempt to migrate data
   - depending on success of ^, cause a rollback by blocking `microshift run`,
     or allow it and start running

### Test Plan

#### Unit tests

Aiming to write as much as possible in Go, we should strive for maximum testability: 
- Separate code paths for planning (e.g. should it do a backup or restore?) 
  and acting (actually perform backup) - e.g. interface with two methods Plan(), Act()
  - This will allow testing decisions and actions separately
  - This will allow easy implementation of --dry-run describing what would happen
- Due to many interactions with filesystem, filesystem abstraction should be investigated
  so unit tests can use in-memory filesystem rather than host's to make testing easier
  and more robust.

#### Functional tests focused on each of the areas (backup, restore, migrate)

Functional tests need to be assessed in terms of effort and impact.

Given that implementation of the enhancement for ostree based systems has significantly
greater priority than implementation for regular RPM systems,
there's little incentive to work on exposing functionalities in form of `backup`, 
`restore`, and `migrate` commands.
By not providing a way to manually trigger these processes, ability to perform functional
tests in isolation might be severely limited or even impossible without entering territory 
of end to end tests.

#### End to end tests

Sequences from "Workflows in detail" should be implemented in CI.

### Graduation Criteria

Functionality will be GA from the beginning.

- All areas of functionality implemented and available for usage
- Sufficient test coverage - unit tests (where possible, virtualing/mocking filesystem encouraged), integration tests, e2e tests (CI, QE)
- End user documentation created

#### Dev Preview -> Tech Preview

N/A

#### Tech Preview -> GA

N/A

#### Removing a deprecated feature

N/A

### Upgrade / Downgrade Strategy

N/A

### Version Skew Strategy

See section "allowing and blocking upgrades".

### Operational Aspects of API Extensions

#### Failure Modes

Failure to perform backup, restore, or data migration will result in MicroShift 
not starting and failing greenboot check might result in rollback if problems happened
on a deployment that was just staged and booted to or system waiting for manual
intervention if there's no rollback deployment.

In such scenario, admin must perform manual steps to investigate and address root cause.
It is up to MicroShift team to document possible issues and how to resolve them.

Also refer to [manual interventions flows](#manual-interventions---1st-deployment-or-rollback-no-more-greenboot-reboots).

#### Support Procedures

For now refer to [manual interventions flows](#manual-interventions---1st-deployment-or-rollback-no-more-greenboot-reboots).

## Implementation History

- [MicroShift Upgrade and Rollback Enhancement](https://github.com/openshift/enhancements/pull/1312)

## Alternatives

### Using MicroShift greenboot healthcheck to decide whether to backup or restore

Although system might be unhealthy due to reasons unrelated to MicroShift, it cannot
make decision to backup or restore depending on the healthcheck rather than on green/red scripts.
This is because device as a whole must go forward or rollback.

In situation when MicroShift is healthy and system is not, MicroShift's healthcheck would persist
backup. This could result in a situation when system rollback to previous ostree deployment,
which might feature different set of Kubernetes applications running on top of MicroShift
resulting in running application that should not run.

### Performing backup on shutdown

Reasons for backing up MicroShift's data on boot rather on shutdown:
- Smaller risk of backup process being killed or shutdown not waiting for backup to finish,
   therefore greater confidence that backup will happen.
- Easier integration
  - As a part of MicroShift's pre-run procedure (executed just before MicroShift)
    result of backup will be more noticeable because MicroShift won't start
    (as opposed to it failing during shutdown).
  - Running backup on shutdown will require to setup new systemd units that will run before shutdown.
  - Running backup on boot (pre-run) means it could be contained within existing `microshift.service` (as `ExecStartPre`) - but it might make more sense to have separate service file.
- Copy-on-Write was chosen as backup strategy meaning that it won't perform any version specific procedures.
  - Even if such procedures would be executed, in case of MicroShift upgrade, new version must be able to read 
    data of older version in order to perform storage migration.

### Supporting downgrades (going from X.Y to X.Y-1...)

Decision to not support downgrades is based on following:
- Greatly increased effort of maintenance, testing, and more challenges to ensure quality
  with negligible gain
- Binaries cannot be amended after releases, so only way to specify allowed downgrades
  would be by documenting them and requiring administrator to consult the documentation.
- Process would be unsymmetrically more difficult than upgrade, consider:
  - Version A supports `v2`
  - Version B supports `v1` and `v2`
  - Version C supports `v1`
  - To downgrade from version A to C
    - Shutdown ostree commit A, boot commit B
    - Instruct MicroShift to just downgrade data from `v2` to `v1`, without running cluster (to not make migration too long)
    - Persist metadata that version C will accept
    - Shutdown ostree commit B, boot commit C
    - MicroShift C would validate metadata to make sure it's compatible
- Stemming from previous bullet - version metadata would need to go beyond simple MicroShift version of X.Y.Z
  to not only tracking versions of all resources, but perhaps versions of the embedded components as well.
  It could be a case of internal implementation details that would support newer and older behavior in newer version,
  but result in bugs when going back to older version.


### Alternative backup methods

#### Copy-on-write

Pros:
- Underlying blocks are shared, so initially backup takes very little to no additional space
Cons:
- Not supported by all filesystems - requirement needs documenting

#### etcdctl snapshot save/restore

Pros
- Database snapshot is much smaller than copy of database
Cons:
- Saved and restore etcd database doesn't contain whole history
- Would require to ship `etcdctl` increasing footprint of MicroShift
  which doesn't not happen at the moment

#### Creating a tar file with data dir

Pros:
- backup in form of a single file
Cons:
- Without compression is weights as much as data dir

### TODO: Symlinking live data to specific deployment data

### TODO: Executing pre-run as part of run (aka why is pre-run separate systemd unit)


<!-- #### How should `microshift pre-run` be executed? -->
- `microshift.service` - `ExecStartPre`
  - No need to add new systemd service files.
  - It will run on each `systemctl restart microshift` which is not desirable (will it run when systemd restarts MicroShift?)
- `microshift-pre-run.service`
  - Running on boot, just once, before `microshift.service`
  - Not repeated on MicroShift restart
  - New service file
## Infrastructure Needed [optional]

N/A

## Future Optimizations

- Use result of MicroShift's greenboot check to decide on backup/restore next boot.
  - Current implementation uses greenboot's green/red scripts and they have no knowledge what caused unhealthy boot

- Incorporate MicroShift's greenboot check into `microshift` binary as a separate command.
  - It'll get access to source of truth about "what MicroShift components" should run (e.g. optional TopoLVM)

- Supporting 4.y to 4.y+2 or 4.y+3 upgrades