---
title: align-etcd-directories-in-microshift
authors:
  - dusk125
reviewers:
  - "@hasbro17, etcd Team"
  - "@tjungblu, etcd Team"
  - "@Elbehery, etcd Team"
  - "@fzdarsky"
  - "@derekwaynecarr"
  - "@mangelajo"
  - "@pmtk"
approvers:
  - "@dhellmann"
  - "@deads2k"
api-approvers:
  - None
creation-date: 2023-02-20
last-updated: 2023-02-21
tracking-link:
  - https://issues.redhat.com/browse/ETCD-356
---

# Align etcd directories in Microshift

## Summary

In Openshift, etcd and related tools (like etcdctl and sos) rely on the directories `/var/lib/etcd`, `/etc/kubernetes/static-pod-certs`, `/etc/kubenetes/static-pod-resources` to exist and to contain relevant files by default. In Microshift, all of the Microshift related information (including etcd) is located under `/var/lib/microshift` (with the etcd data directory `/var/lib/microshift/etcd`).

## Motivation

This enhancement opens a discussion for deciding how best to align, if at all, the above directories. 
The three proposed options are:

1. To do no alignment and keep all etcd directories under `/var/lib/microshift`.
2. Symlink the etcd directories under `/var/lib/microshift` to their Openshift counterparts (listed above).
3. Update Microshift to use the Openshift-expected locations.

### User Stories

TODO

### Goals

- Select one of the approaches to implement.

### Non-Goals

TODO

## Proposal

### No Alignment
With this option, Microshift is not updated and everything (etcd related) remains under the `/var/lib/microshift` base directory; this includes the etcd data directory, certs directory, and the scripts directory.
Since Microshift's design is substaintely different from Openshift, only some of the high-level and very little of the low-level Openshift documentation would directly apply to Microshift.
Processes like running a script on Openshift first require to `oc exec` into the cluster to either start a container to run the script, or exec into an already running container to run the script; however, in Microshift, the same script would require logging into the box (via something like ssh) and running the script directly on the machine.

Openshift documentation, etc could be used as a starting point/guide for that of Microshift, but it's very unlikely that it would be directly usable given the architectural differences between the two.

### Workflow Description

TODO

#### Variation [optional]

TODO

### API Extensions

N/A

### Implementation Details/Notes/Constraints [optional]

TODO

### Risks and Mitigations

TODO

### Drawbacks

TODO

## Design Details

### Open Questions [optional]

TODO

### Test Plan

**Note:** *Section not required until targeted at a release.*

TODO

### Graduation Criteria

TODO

#### Dev Preview -> Tech Preview

TODO

#### Tech Preview -> GA

TODO

#### Removing a deprecated feature

TODO

### Upgrade / Downgrade Strategy

TODO

### Version Skew Strategy

TODO

### Operational Aspects of API Extensions

TODO

#### Failure Modes

TODO

#### Support Procedures

TODO

## Implementation History

TODO

## Alternatives

### Symlink Alignment
This is the middle ground between No Alignment and Full Alignment; the current proposal is to keep everything stored under `/var/lib/microshift` and symlink its contents to the Openshift directories.
This would allow some of the Openshift documentation, etc to still apply to Microshift; however, there would need to validation done to ensure that scripts and workflows will work appropiately through a symlink.

### Full Alignment
This approach would include an update to Microshift that would make the etcd data, scripts, and certs directories match that of Openshift so that all documentation, scripts, and knowledge would match one-for-one in Microshift and Openshift.

## Infrastructure Needed [optional]

TODO
