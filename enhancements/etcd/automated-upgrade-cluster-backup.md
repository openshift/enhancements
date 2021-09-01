---
title: Automation of cluster backups before upgrade
authors:
  - "@wking"
  - "@jottofar"
  - "@marun"
  - "@hexfusion"
reviewers:
  - "@lilic"
  - "@sttts"
approvers:
  - "@lilic"
  - "@sttts"
creation-date: 2021-09-01
last-updated: 2021-09-01
status: implementable

see-also:
  - "https://docs.google.com/document/d/1UvGSFxlts0xPg_2kYUdWhkFoQSHJAdTh4iFtJX2HaFs/edit"
---
# Automation of cluster backups before upgrade

## Release Signoff Checklist

- [X] Enhancement is `implementable`
- [X] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

Enable automated cluster backup before upgrade to OCP 4.9.

## Motivation

There may, and probably will, be serious issues folks hit during 4.8 and 4.9 updates.  We are on the hook to keep
those clusters going for our supported customers.  Reliable 4.8 etcd snapshots give us a worst-case safety valve if 
we cannot find a way to roll forward.

### Goals

- Reliable backups that can get a customer back to 4.8 if 4.9 turns out to be a disaster for this cluster.
- Backup freshness is within minutes of the update request, to minimize the amount of customer work that is lost by  
restoring from a historical snapshot.

### Non-Goals

- Implement rollback functionality in OCP 4. ([RFE-1955](https://issues.redhat.com/browse/RFE-1955))
- Implement upgrade preflight checks for operators. ()
- Provide a long term backup feature. While this pattern may prove reusable it was not the goal and is instead 
considered a bug.

[1] [RFE-1955](https://issues.redhat.com/browse/RFE-1955)

[2] [enhancements/#363](https://github.com/openshift/enhancements/pull/363)
