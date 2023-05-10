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
last-updated: 2023-05-09
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
As GA product, it is expected it can be updated to
apply security patches and bug fixes, and leverage features
in newer version while keeping and using existing data.

MicroShift is intended to be a part of Red Hat Device Edge (RHDE) which is based
on RHEL For Edge (R4E) which is immutable Linux distribution by leveraging
[ostree](https://ostreedev.github.io/ostree/) technology.
It allows changing root filesystem for next boot by staging new commits or
rolling back to previous one.
ostree is commonly paired with [greenboot](https://github.com/fedora-iot/greenboot)
which provides automated health assessment of the system and trigger a rollback
if rebooting the device doesn't result in device becoming healthy.

Even though, OpenShift does not support downgrade or rollback, MicroShift must
support it in some form to fit into RHDE.
Rollback (going back to older deployment) will be supported only if MicroShift
ran on that deployment and data compatible with that deployment was backed up.
Downgrade (migrating to older version of MicroShift) will not be supported.

In order to integrate into RHDE, MicroShift needs to be augmented with
functionality to back up and restore its data together with ostree deployments,
and refuse or perform data migration to newer storage schema version.
Integration with greenboot will allow system's health to depend on state of the
MicroShift and will provide necessary information to manage backups.

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
- Block data migration if MicroShift version skew is unsupported

Design aims to implement following principles:
- Keep it simple, optimize later
- MicroShift does not own the OS or host
- MicroShift and all its components are versioned, upgraded, and rolled back together
- Be defensive, fail fast
- Rely on outside intervention as a last resort

### Non-Goals

* Building MicroShift upgrade graph
* 3rd party applications' health checks and its data backup or rollback
  (although end user documentation should be provided)
* Defining procedures for backup and restore, and upgrading MicroShift on
  non-ostree systems is left to a future enhancement

## Proposal

### Integration with greenboot

greenboot integrates with systemd, ostree, and grub to provide auto-healing
capabilities of newly staged and booted deployment in form of reboot:
if system is still unhealthy after specified amount of reboots, it
will be rolled back to previous ostree deployment (commit).

Because greenboot already exists as an integral part of Red Hat Device Edge
systems, we will integrate with it, rather than creating a new system.

greenboot determines system health with health check scripts and MicroShift
already provides such script. For more information about greenboot and current
MicroShift integration see [Integrating MicroShift with Greenboot](./microshift-greenboot.md).

After health check, either "green" (system is healthy) or "red" (system is unhealthy)
scripts are executed. MicroShift will provide "green" and "red" scripts which
will persist the overall system's health for the current ostree commit.

### Triggers for greenboot failures

System images can introduce different types of changes, including:
1. New OS content unrelated to MicroShift
2. Different configuration settings for MicroShift
3. Different versions of MicroShift (higher or lower)
4. Different versions of applications running on MicroShift (higher or lower)
5. More, fewer, or different applications

Any of those transitions could result in a greenboot failure.

Because MicroShift cannot detect the cause of the failure, and cannot influence
how greenboot handles the failure, all failures will be handled by reverting
to a previous known-good state for MicroShift's data.

### Version change support

- Because we want to maintain upgrade expectations with Kubernetes and OpenShift,
  we will only support changing versions one Y version at a time (x.y to x.y+1).
- Because we cannot guarantee that the data formats between Y versions are
  compatible after an upgrade, we will only support rolling back to a previous
  Y version when restoring from a backup.
- Because we may need to support "manual" changes to correct for regressions
  within a Y version, and because we expect the storage format and other data
  to be forward and backward compatible within a Y version, we will support
  changing from any Z version to any other Z version for the same version of Y,
  including downgrading.
- Because we may need to block certain upgrade sequences, similar to OpenShift's
  upgrade graph, but we cannot ensure access to that upgrade graph from edge
  systems and we cannot prevent an attempted upgrade via a new ostree deployment,
  we will incorporate a mechanism to block specific upgrades by listing version
  numbers _from which_ a new version cannot be upgraded (X.Y+1.Z may include
  X.Y.Z in its "block" list). When a new version detects that the system is
  upgrading from a version in that block list, it will refuse to start and
  cause a greenboot failure.

### Backup retention

- Because a user may stage multiple ostree deployments on a host and boot them in
  any order, we will keep multiple backups to ensure that we can roll back to a
  state compatible with any ostree deployment### Integration with greenboot

greenboot integrates with systemd, ostree, and grub to provide auto-healing
capabilities of newly staged and booted deployment in form of reboot:
if system is still unhealthy after specified amount of reboots, it
will be rolled back to previous ostree deployment (commit).

Because greenboot already exists as an integral part of Red Hat Device Edge
systems, we will integrate with it, rather than creating a new system..
- Because we want to minimize the impact of backups on storage requirements,
  we will keep only 1 backup per ostree deployment.

### Backup format

- Because we want the backup process to be simple and we want to minimize the
  space used by each backup, we will use `cp` with reflinks to create copy-on-write
  versions of all of the content being backed up.

### Kubernetes storage format upgrades

- Because we need to support API version deprecation and removal in Kubernetes
  and OpenShift APIs, we will run the storage version migrator to update all
  data in the database when a Y version update is detected.

### Order of actions

- To ensure backup and restore process does not result in data corruption, it
  will be performed with MicroShift cluster not running.
- To allow database upgrades only etcd and kube-apiserver will be active during
  data migration.
- System start was chosen as point in time that above actions will happen,
  just before starting MicroShift cluster.

### Workflow Description

**MicroShift administrator** is a human responsible for preparing
ostree commits and scheduling devices to use these commits.

Upgrade:

1. MicroShift administrator prepares a new ostree commit
1. MicroShift administrator schedules device to reboot and use new ostree commit
1. Device boots new commit
1. Operating System, greenboot, and MicroShift take actions (migrating database
   content, causing a rollback, etc.) without any additional intervention

Manual rollback:

1. MicroShift administrator rollbacks to or stages an ostree commit with MicroShift
   that was already running on the device and performs a reboot
1. Staged ostree commit boots
1. MicroShift will restore the backup matching current ostree commit
1. MicroShift will run.

### API Extensions

Metadata persisted on filesystem related to the functionality described in this enhancement
is considered internal implementation detail and not an API intended for user.
Schemas and locations of these files are subject to change.

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

- **ostree commit**: image containing root filesystem
- **ostree deployment**: bootloader entries created from ostree commits
  (this document refers to "system image" very loosely as both "commit" and "deployment")
- **Rollback**: booting older (that already ran on the device) ostree commit -
  either due to greenboot or manual intervention
- **Backup**: backing up MicroShift's data
- **Restore**: restoring MicroShift's data from a backup
- **Data migration**: procedure of transitioning MicroShift's data to be compatible with newer binary
- **Version metadata**: file storing MicroShift's version and ostree commit ID
- **MicroShift greenboot healthcheck**: program verifying the status of MicroShift's cluster

### Phases of execution

1. Pre run phase
   - Failure blocks start of MicroShift's cluster
   - Backs up or restores data
   - Migrates data to newer schemas if needed
1. Run phase
   - Start of MicroShift's cluster

In parallel:
1. greenboot runs MicroShift health check
1. greenboot runs red or green scripts depending on system's health
   - MicroShift will plug into red/green scripts to persist system's health


### History of ostree commits (deployments)

As already mentioned, backing up and restoring data will happen on system boot.
It means that next boot makes backup compatible with previously booted
commit/MicroShift, but it restores data compatible with itself (currently running).

Decision whether to backup or restore will be primarily based on health of previous boot.
This means that MicroShift needs to keep history of running ostree commits
(featuring MicroShift) and their health. The software also needs access to the
history information to know when database format migrations are needed.
To support both decisions, a structured text file outside of the etcd database
will be used to persist the history information between various deployments.

Information about system's health will be obtained by greenboot integration
in form of "green" (system is healthy) and "red" (system is unhealthy) scripts
which will persist the overall system's health for the current ostree commit.

### Backing up and restoring MicroShift data

MicroShift needs to fit into workflow of ostree-based systems: device can be
upgraded by staging and booting new ostree commit, and rolled back if it's
unhealthy or admin wishes to do it.

To achieve that MicroShift needs to make backups and be able to restore them in
sync with ostree lifecycle.
As mentioned previously these actions will happen on boot, just before MicroShift cluster runs.
As a general rule: if previous boot was healthy, data will be backed up, if boot
was unhealthy, data will be restored.
In case of manual rollback data should be restored, even if previous boot was
healthy. The current database will be backed up before being replaced.

To provide ability to rollback and restore MicroShift data, backup for each
ostree commit/deployment will be kept.
For now, default behavior will be to keep backups for commits no longer existing on the system.
Reason for this is that reintroducing commit results in the same ID (it is a checksum)
and deleting backups would prevent restoring healthy data for the commit.
This could be a configurable option, but rules of pruning old backups are out
of scope of this enhancement.

It is worth noting that although MicroShift's data is focus of the enhancement,
backups will be tied to specific ostree commits.
Linking backups to ostree commits will ensure that staging and rolling back
is "all or nothing" and MicroShift does not accidentally run applications
belonging to another commits. Especially that difference between two commits
might not be MicroShift itself, but the applications that run on top of it.

#### Backup technique

MicroShift will perform backup and restore by leveraging functionality
Copy-on-Write (CoW). It is a feature of filesystem and is utilized by
providing a `--reflink=` param to `cp` program.
Because not all filesystems support CoW, we will provide `auto` argument
to `--reflink` so it gracefully falls back to regular copy.

This method was chosen because it's easy to use, doesn't require additional
tools (it's also not impacted by version changes), and should make backing up
fail rarely because by sharing filesystem blocks it's initially very small
(its size increases as original data changes).

End user documentation needs to include:
- guidance on picking and configuring filesystem to fullfil requirements
  for using copy-on-write,
- warn that in case of missing CoW support, full backup will be made.

#### Backup contents

Entire MicroShift data directory will be backed up, this includes etcd database,
and certificates and kubeconfigs.

- Copying entire etcd working directory will preserve history and other metadata
  that would have been lost when using etcd snapshots.
- Not regenerating certificates on each upgrade will keep them valid. It also
  means that kubeconfigs will continue to work as opposed to needing to obtain
  them again.

### Storing MicroShift version in data directory

MicroShift will persist into a file its X.Y.Z version and ID of ostree commit
that's currently active. The file will be created together with data directory
on first start of MicroShift and updated when data migration is performed.

It will be used as a source of truth for decisions regarding:
- blocking or allowing data migration,
- backing up and restoring data in more nuanced scenarios.

### Action log

MicroShift will keep a log of important action related to data management such
as reason and action taken like: backing up, restoring, migrating, starting,
checking health, etc. It will be used for support procedures.

### Data migration

Data migration is process of transforming data from one schema version to another.

Process needs to be aware of following data types:
- Kubernetes storage migration (e.g. from `v1beta1` to `v1`)
  - MicroShift will reuse
    [Cluster Kube Storage Version Migrator Operator](https://github.com/openshift/cluster-kube-storage-version-migrator-operator)
- etcd schema (although it's unlikely in near future)
- Internal MicroShift-specific data

Data migration will only be supported from Y to Y+1 version, although it might
change if upstream components will support greater version skews.
Going backwards, from Y to Y-1 will not be supported directly.
Migrating device to older MicroShift version will be only supported in form of
ostree rollbacks - it means that backup for older MicroShift must exist.

If MicroShift data directory does not contain version information, it will be
assumed that it was created by MicroShift 4.13 and tested with Y-stream skew
rule.

MicroShift is minimal version of OpenShift so risk of Z-stream incompatibilities
is greatly reduced therefore, at the time of writing the enhancement,
switching between different Z streams will be possible regardless of the
direction (older to newer, newer to older) and any divergence from this rule
should be documented.

Although it is not needed immediately, MicroShift binary will be fitted with
mechanism to embed list of prohibited version migrations.

If migration is blocked or fails, MicroShift cluster won't start.
This will render system unhealthy and, if rebooting the device does not affect
result of the migration, result in system rolling back to previous deployment.

Decision flow describing whether to block or attempt data migration can be summarized as:
- If version metadata is missing, assume 4.13
- Refuse to migrate if
  - Version of data is present on list of prohibited migrations
  - Version skew between data on disk and binary is bigger than X.Y+1
  - Y version of binary is older than Y version of data
- Skip migration if X.Y are the same
- Attempt data migration otherwise

#### Staging new ostree commits on top of unhealthy systems

Automated handling of MicroShift data in case of unhealthy system is
complicated due to ambiguity regarding admin's intention and each option has
different trade offs without clear winner.

For that reason, when needing to stage a new commit on top of unhealthy system
one of two actions must be performed:
- system must be brought to healthy state, or
- MicroShift data should be deleted completely resulting in clean start which
  is the same as first run.

Only exception is when rollback commit doesn't feature MicroShift.
This special case handles scenarios when device is preinstalled with system
without MicroShift, then commit with MicroShift runs, but is unhealthy so it
rolls back to factory system, later another commit with MicroShift is staged
and runs, but it should not be held back by stale data.

### Open Questions [optional]

### Workflows in detail

#### Decision tree

###### Backup management

- "Data" refers to MicroShift's data
- Terms such as _previous boot_, _previous boot's commit_, and
  _previous commit with MicroShift_ are related to MicroShift's "history of commits"
  (therefore only updated when commit runs MicroShift).
- _Current boot's commit_ is what currently runs on the device
- _Rollback_ refers to ostree's rollback, i.e. older commit that will be booted
  if newly staged commit is unhealthy.
- Not covered "else" conditions will result in error and not starting the cluster.

---

1. If data does not exist
   - MicroShift is running for the first time on the device.
   - There's nothing to backup or migrate, skip to running cluster.

1. If data exists but is missing version metadata:
   Assume it's 4.13, make a backup (use `4.13` as ID), proceed to [data migration](#data-migration-1).

1. If previous boot was healthy:
   **backup data for previous boot**, check if next point is applicable, proceed to [data migration](#data-migration-1).

1. (Regardless of previous boot health)<a name="current-commit-different-restore"></a>
   If current boot's commit is different from previous boot's commit and backup exist for current commit:
   **restore data for current commit, skip data migration, [start cluster](#starting-the-cluster)**.
   > Existence of backup for "current boot's commit" means commit already ran on the device and was healthy.

1. If health of previous boot is unknown (e.g. system was unexpectedly rebooted before health check)
   and commit of current and previous boot is the same:
   **skip to [data migration](#data-migration-1), i.e. retry start up, but check version skews just in case**.
   Otherwise assume it was unhealthy and proceed to the next point.

**(Rest of the checks assume that previous boot was unhealthy)**

1. [If commits of current and previous boots are different](#current-previous-different)
1. [If commits of current and previous boots are the same](#current-previous-same)

**If _current boot's commit_ and _previous boot's commit_ are different<a name="current-previous-different"></a>**

Backup of _current boot's commit_ does not exists, otherwise [it would be already restored](#current-commit-different-restore).

1. If _current boot's commit_ does not exist in "history of commits"
   and _previous boot's commit_ is not _rollback commit_:
   **delete data and [start cluster](#starting-the-cluster)**
   > It's first time MicroShift is running on this specific commit.
   >
   > We only expect to end up in this state if the previously boot commit didn't run MicroShift at all.
   > This fits the FIDO device onboarding (FDO) scenario. It is safe to delete
   > the database in this state because its contents cannot be used by the
   > current commit and rolling back to that other commit will not result in
   > a running MicroShift.

1. If _current boot's commit_ exists in "history of commits" and version metadata matches _current boot's commit_:
   **backup data for current commit and [start cluster](#starting-the-cluster)**
   > It means that current commit was already running on the device.
   > If the backup is missing, it means that either system was unhealthy or new commit failed to make a backup.
   > The former is unsupported scenario (system should be healthy or MicroShift data cleaned up),
   > so only the latter is considered.

**If _current boot's commit_ and _previous boot's commit_ are the same<a name="current-previous-same"></a>**

It means that current commit booted more than once in a row.

1. If backup for _current boot's commit_ exists: **restore**
   > This means that if backup exists it is always restored regardless of the
   > system's health (except when it was unknown)
   > ([see optional restore after backing up](#current-commit-different-restore)).
   >
   > - If this is greenboot's reboot, then backup was created before this commit was staged and deployed.
   >   So commit must've been reintroduced and [data would be restored early in the process](#current-commit-different-restore).
   >   - Being in this place in decision tree means that either restore failed or system was unhealthy.
   >     Restoring again seems to best bet.
   >
   > - If this is manual reboot, then system was healthy,
   >   then manually rebooted, backed data up and end up unhealthy,
   >   again rebooted resulting in being in this place in decision tree.
   >   - Admin should address problems before rebooting the system
   >     (and retrigger greenboot first, to refresh system's health)
   >   - Restoring data seems okay - going back to last healthy state.

1. If backup for _current boot's commit_ does not exist
   > There's no proof of the commit being ever healthy

   1. If "history of commits" only knows _current boot's commit_: **delete data and [start cluster](#starting-the-cluster)**
      > First commit with MicroShift running on the system, system is consistently unhealthy.

   1. If "history of commits" knows about _previous commit with MicroShift_
      > MicroShift was already running on the device.
      > But it doesn't mean system will rollback to that _previous commit with MicroShift_

      1. _Previous commit with MicroShift_ is the same as _rollback_
         1. If version metadata matches _previous commit with MicroShift_: **backup data and proceed to [data migration](#data-migration-1)**
            > Previous boot of current commit might have been unhealthy because it failed to make a backup.

         1. Otherwise: **restore backup of _previous commit with MicroShift_**
            > Give chance to migrate data and start cluster again.
            > Assumption that admin upgraded from healthy system is important here.

      1. _Previous commit with MicroShift_ is not the _rollback_ **delete data and [start cluster](#starting-the-cluster)**
         > Means that rollback does not feature MicroShift.
         > This is "retry boot" of FIDO scenario.

##### Data migration

1. Compare version persisted in metadata with MicroShift's binary
   - Binary's `Y` is smaller: **abort and block cluster start up**
   - Binary's `Y` is bigger by more than `1`: **abort and block cluster start up**
   - Version in metadata is present in "list of prohibited migrations:
     **abort and block cluster start up**
   - `Y`s are the same: **skip to cluster start up**
1. Perform data migration

##### Starting the cluster
1. If metadata exists and it doesn't match version of the binary: **abort**
   > Extra check to make sure that migration was performed
1. Create data dir if necessary
1. Create or update metadata (version and "history of commits")
1. Continue regular flow

##### Health check
1. Assess health of MicroShift and persist the result to "history of commits"

##### MicroShift's green and red scripts
1. Write system's health to "history of commits"

#### Manual interventions

##### Addressing MicroShift's health

Depending on MicroShift's health admin might:
- Unhealthy
  - Delete MicroShift's data to allow fresh start
  - Investigate and address problems with MicroShift cluster
- Healthy
  - Keep MicroShift's data
- Unhealthy application running on top of MicroShift
  - investigate and address problems with the app

After resolving the issues, admin should re-trigger greenboot healthcheck.
If admin wishes to migrate from unhealthy system, MicroShift's data should be cleaned up.

##### Backup exists, restore succeeds, but system is unhealthy

> Scenarios:
> - System was rolled back to 1st commit
>   (no more greenboot reboots, previous commit was `unhealthy` -> restore)
> - 1st and only ostree commit with MicroShift:
>   - 1st boot OK, **manual** reboot
>   - 2nd boot: data backed up, system NOK, **manual** reboot
>   - 3rd boot: data restored, system NOK

1. `microshift pre-run`
   - Restore from `backups/current-commit.id/`
     > `prev-boot-commit.system == unhealthy` &
     > `current-commit != prev-boot-commit` &
     > `current-commit exists in history.file and was healthy` &
     > `backups/current-commit.id/ exists`
1. `microshift run`
1. System is unhealthy (red)
1. Greenboot doesn't reboot device because `boot_counter` is only set when ostree commit is staged
1. System requires manual intervention

   - If the admin simply reboots the device
     1. 1st ostree commit boots
     1. `microshift pre-run`
        - Do nothing
          > `prev-boot-commit.system == unhealthy` &
          > `current-commit != prev-boot-commit` &
          > `backups/current-commit.id/ exists`
          >
          > Admin should address the issue and either retrigger greenboot or manually mark commit as healthy
          > as part of manual intervention procedure.
     1. `microshift run`

   - If the admin addresses the issue
     1. [MicroShift's health](#addressing-microshifts-health)
     1. Other components - admin's judgement
     1. Admin retriggers greenboot or manually marks commit as healthy
     1. Reboots the device
     1. `microshift pre-run`
        - Backup to `backups/prev-boot-commit.id/`
          > `prev-boot-commit.system == healthy`
     1. `microshift run`

##### 1st commit is unhealthy, admin wants to stage another one

> Scenario:
> - 1st commit on system is unhealthy beyond recovery
> - Admin decides to try another commit with different MicroShift build

1. 1st commit is unhealthy
1. Greenboot doesn't reboot because there's only one (no rollback, no `boot_counter`)
1. Admin stops MicroShift: `systemctl stop microshift`
1. Admin resets the system by removing `microshift/` and `microshift-backups/`
1. Admin stages 2nd commit
1. System is rebooted
1. 2nd commit starts
1. `microshift pre-run`
   - First boot
     > Neither `microshift/` nor `microshift-backups/` exist
1. `microshift run`

##### Rollback on demand

> Scenario:
> - System is running on 2nd commit
> - Both 1st and 2nd commit are healthy
>   (backup exists for 1st, backup for 2nd may or may not yet exist)
> - Admin wants to rollback to 1st commit

1. 2nd commit is running and healthy
1. Admin runs `rpm-ostree rollback` or equivalent
1. 2nd commit shuts down, 1st commit boots
1. `microshift pre-run`
   - Backup to `backups/prev-boot-commit.id/`
     > `prev-boot-commit.system == healthy`
   - Restore from `backups/current-commit.id/`
     > `current-commit != prev-boot-commit` &
     > `current-commit.system == healthy` &
     > `backups/current-commit.id/ exists` &
     > `backups/current-commit.id/version.file is older than microshift/version.file`
   - No need to migrate data
     > `version.file == microshift version`
1. `microshift run`

##### Addressing rollback after unsuccessful upgrade from 4.13

> Scenario: upgrading from 4.13 to 4.14 fails resulting in rollback.
> Workflow also describes how admin can attempt the upgrade again.

1. 0th commit (with MicroShift 4.13) is running
1. 1st commit (with MicroShift 4.14) is staged
1. 0th shuts down, 1st boots
1. `microshift pre-run`
   - Backup to `backups/4.13/`
     > `microshift/` exists, `version.file` does not
   - If upgrade from 4.13 supported: attempt storage migration from 4.13, otherwise block `microshift run`
1. `microshift run`
1. System is unhealthy due to different reasons
   - Upgrade was blocked or storage migration failed
   - MicroShift was unhealthy
   - Something else was unhealthy
1. System is rebooted, system is consistently unhealthy
1. Rollback to 0th commit (4.13)
1. `microshift run`
   - (Maybe) fails due to data inconsistency
1. MicroShift is healthy or unhealthy
   - Admin restores `backups/4.13/`
   - Even if healthy, admin should address the migration problem before attempting it again
1. 2nd commit (with MicroShift 4.14) is staged
1. 0th shuts down, 2nd boots
1. `microshift pre-run`
   - Backup to `backups/4.13/`
     > `microshift/` exists, `version.file` does not
   - If upgrade from 4.13 supported: attempt storage migration, otherwise block `microshift run`


#### First ostree commit

##### First commit, first boot

1. Device is freshly provisioned
1. 1st commit starts
1. `microshift pre-run`
   - First boot
     > Neither `microshift/` nor `microshift-backups/` exist
1. `microshift run`
1. Greenboot: health checks and green/red scripts
1. Alternative scenarios
   - System is healthy
     1. [First commit, second boot (reboot): backup fails](#first-commit-second-boot-reboot-backup-fails)
     1. [First commit, second boot (reboot): backup succeeds](#first-commit-second-boot-reboot-backup-succeeds)
   - System is unhealthy
     1. Greenboot doesn't reboot device because `boot_counter` is only set when ostree commit is staged
     1. System requires manual intervention.

##### First commit, second boot (reboot): backup fails

> First boot was healthy

1. Device is rebooted into the same (1st) commit
1. `microshift pre-run`
   - A backup is attempted but fails
1. Greenboot: health checks and red scripts
1. Greenboot doesn't reboot device because `boot_counter` is only set when ostree commit is staged
1. System requires manual intervention.

##### First commit, second boot (reboot): backup succeeds

> First boot was healthy

1. Device is rebooted into the same (1st) commit
1. `microshift pre-run`
   - Backup to `backups/prev-boot-commit.id/`
     > `prev-boot-commit.system == healthy`
   - No data migration
     > `version.file == microshift version`
1. `microshift run`
1. Greenboot: health checks and green/red scripts
1. Optional: System is unhealthy
   - Greenboot doesn't reboot device because `boot_counter` is only set when ostree commit is staged
   - System requires manual intervention.

#### Second ostree commit

Pre-steps:

1. **1st commit is healthy**
1. 2nd commit is staged (behind the scenes greenboot sets `boot_counter`)
1. 1st commit shuts down
1. 2nd commit boots

##### Backup succeeds, no MicroShift change, no cluster app change

> No changes are made to MicroShift version or apps running within the cluster,
> so new ostree commit might feature unrelated changes or RPMs

1. `microshift pre-run`
   - Backup to `backups/prev-boot-commit.id/`
     > `prev-boot-commit.system == healthy`
   - No data migration
     > `version.file == microshift version`
1. `microshift run`
1. Greenboot: health checks and green/red scripts

(Optional)
1. System is unhealthy, greenboot reboots the system
1. `microshift pre-run`
   - Delete data, try from clean
     > `prev-boot-commit.system == unhealthy` &
     > `current-commit == prev-boot-commit` &
     > `backups/current-commit.id/ doesn't exist` &
     > `history.file[earlier-commit] doesn't exist`
1. `microshift run`
1. Greenboot: health checks and green/red scripts
1. **System was unhealthy (red) each boot**
   - `boot_counter` reaches `-1`
   - **grub boots 1st commit (rollback)**
1. `microshift pre-run`
   - Restore from `backups/current-commit.id/`
     > `prev-boot-commit.system == unhealthy` &
     > `current-commit != prev-boot-commit` &
     > `history.file[current-commit] exists and was healthy` &
     > `backups/current-commit.id/ exists`
1. `microshift run`

##### First commit was active only for one boot, backup fails

1. `microshift pre-run`
   - Backup data to `backups/prev-boot-commit.id/`
     > `prev-boot-commit.system == healthy`
     - Fails
   - Exit 1 - blocks `microshift run`
1. MicroShift healthcheck fails, greenboot red script
1. System is rebooted
1. `microshift pre-run`
   - Backup data to `backups/prev-boot-commit.id/`
     > `prev-boot-commit.system == unhealthy` &
     > `current-commit == prev-boot-commit` &
     > `backups/current-commit/ doesn't exist` &
     > `history.file[earlier-commit] exists` &
     > `earlier-commit == ostree status' rollback` &
     > `earlier-commit.system == healthy` &
     > `version.file[commit] matches earlier-commit.id`
     - Fails again
   - Exit 1 - blocks `microshift run`
1. Greenboot reboots system multiple times
   (always red boot, `boot_counter` reaches `-1`, grub boots previous (1st) commit (rollback))
1. `microshift pre-run`
   - Backup data to `backups/current-commit.id/`
     > `prev-boot-commit.system == unhealthy` &&
     > `current-commit != prev-boot-commit` &&
     > `history.file[current-commit] exists` &&
     > `current-commit.system == healthy` &&
     > `backups/current-commit/ does not exist` &&
     > `version.file == current-commit.id`
     - Fails again
   - Exit 1 - blocks `microshift run`
1. Manual intervention needed

#### Staged commit is unhealthy and leads to rollback which fails to restore

1. 2nd commit boots
1. `microshift pre-run`
   - Backup
     > `prev-boot-commit.system == healthy`
1. `microshift run`
1. System is unhealthy
1. Greenboot reboots system multiple times (always red boot),
   `boot_counter` reaches `-1`,
    grub boots previous commit (rollback)
1. `microshift pre-run`
   - Restore from `backups/current-commit.id/`
     > `prev-boot-commit.system == unhealthy`
     > `current-commit != prev-boot-commit`
     > `history.file[current-commit] exists`
     > `current-commit.system == healthy`
     > `backups/current-commit.id/ exists`
     - Fails
   - Exit 1 (blocks `microshift run`)
1. System is unhealthy, `boot_counter` is unset *(it's already a rollback)*
1. Manual intervention is required.

#### System rolls back to commit without MicroShift leaving stale data (FIDO Device Onboard)

> Following workflow addresses scenario when device is preinstalled system without MicroShift and later commit with
> MicroShift is staged. commit happens to be unhealthy which leads to rollback. Then, admin stages another
> commit with MicroShift, which requires it to deal with stale data.

1. 1st commit (sans-MicroShift) is installed on the device at the factory
1. The device boots at a customer site
1. An agent in the ostree commit performs FIDO device onboarding or a similar process to determine the workload
1. 2nd commit (with-MicroShift) is staged
   - greenboot sets `boot_counter`
1. 1st commit (sans-MicroShift) shuts down
1. 2nd commit (with-MicroShift) starts
1. `microshift pre-run`
   - First boot
     > Neither `microshift/` nor `backups/` exist
1. `microshift run`
   - Create dir structure, `version.file`, `history.file`, etc.
1. System is unhealthy, red scripts
1. Greenboot reboots system multiple times (always red boot),
   `boot_counter` reaches `-1`,
    grub boots previous commit (rollback)
1. 1st commit (sans-MicroShift) starts
1. 3rd commit (with-MicroShift) is staged
   - greenboot sets `boot_counter`
1. 1st commit (sans-MicroShift) shuts down
1. 3rd commit (with-MicroShift) starts
1. `microshift pre-run`
   - Delete data and start clean
     > `prev-boot-commit.system == unhealthy` &&
     > `current-commit != prev-boot-commit` &&
     > `current-commit not in history.file` &&
     > `prev-boot-commit != ostree rollback commit`
1. `microshift run`
1. System is unhealthy, greenboot reboots the system (3rd commit)
1. `microshift pre-run`
   - Delete data and start clean
     > `prev-boot-commit.system == unhealthy` &&
     > `current-commit == prev-boot-commit` &&
     > `backups/current-commit.id/ does not exist` &&
     > `earlier-commit in history.file` &&
     > `earlier-commit.system != ostree rollback commit`
1. `microshift run`
1. System is unhealthy consistently, `boot_counter` falls to `-1`, grub boots 1st commit (sans-MicroShift)
1. 4th commit (with-MicroShift) is staged
   - greenboot sets `boot_counter`
1. 1st commit (sans-MicroShift) shuts down
1. 4th commit (with-MicroShift) starts
1. `microshift pre-run`
   - Delete data and start clean
     > `prev-boot-commit.system == unhealthy` &&
     > `current-commit != prev-boot-commit` &&
     > `current-commit not in history.file` &&
     > `prev-boot-commit != ostree rollback commit`
1. `microshift run`

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
- Sufficient test coverage - unit tests (where possible, virtualing/mocking filesystem encouraged),
  integration tests, e2e tests (CI, QE)
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
on a commit that was just staged and booted to or system waiting for manual
intervention if there's no rollback commit.

In such scenario, admin must perform manual steps to investigate and address root cause.
It is up to MicroShift team to document possible issues and how to resolve them.

Also refer to [manual interventions flows](#manual-interventions---1st-commit-or-rollback-no-more-greenboot-reboots).

#### Support Procedures

For now refer to [manual interventions flows](#manual-interventions---1st-commit-or-rollback-no-more-greenboot-reboots).

## Implementation History

- [MicroShift Upgrade and Rollback Enhancement](https://github.com/openshift/enhancements/pull/1312)

## Alternatives

### Using MicroShift greenboot healthcheck to decide whether to backup or restore

Although system might be unhealthy due to reasons unrelated to MicroShift, it cannot
make decision to backup or restore depending on the healthcheck rather than on green/red scripts.
This is because device as a whole must go forward or rollback.

In situation when MicroShift is healthy and system is not, MicroShift's healthcheck would persist
backup. This could result in a situation when system rollback to previous ostree commit,
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

### TODO: Symlinking live data to specific commit data

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