---
title: service-catalog-and-broker-removal
authors:
  - "@jmrodri"
reviewers:
  - "@dmesser"
  - "@joelanford"
  - "@robszumski"
approvers:
  - TBD
creation-date: 2019-12-17
last-updated: 2019-12-17
status: implementable
see-also:
  - "/enhancements/service-catalog/prepare-service-catalog-and-brokers-for-removal.md"
replaces:
superseded-by:
---

# Service Catalog and Broker Removal

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Open Questions [optional]

This is where to call out areas of the design that require closure before deciding
to implement the design.  For instance, 
 > 1. This requires exposing previously private resources which contain sensitive
  information.  Can we do this? 

## Summary

In OpenShift 4.1, we announced the deprecation of the Service Catalog, the
Ansible Service Broker, and the Template Service Broker. In 4.4, we plan to
remove them from productization. They will no longer be installed in a new
cluster. They will also block cluster upgrades until they are disabled in older
clusters.

## Motivation

Service Catalog and Service Brokers need to be removed in OpenShift 4.4 because
it is an LTS release and it is our desire avoid the extended LTS support period
for these deprecated operators and services.

### Goals

1. Implement a pre-flight check for the CVO that ensures the Service Catalog and
   Service Brokers are not present after an upgrade to OpenShift 4.4. (ENG)
1. Alerts users during upgrade if Service Catalog and Service Brokers are in
   use. (ENG)
1. Prevent users from upgrading to OpenShift 4.4 if the Service Catalog or the
   Service Brokers are in use. (ENG)
1. Remove the Service Catalog from the OpenShift 4.4 release payload (ART)
1. Remove the Ansible Service Broker from the OpenShift 4.4 release payload (ART)
1. Remove the Template Service Broker from the OpenShift 4.4 release payload (ART)
1. Remove the Template Service Broker from productization (ART)
1. Remove the Ansible Service Broker from productization (ART)
1. Remove the Template Service Broker Operator from productization (ART)
1. Remove the Ansible Service Broker Operator from productization (ART)
1. Remove the Service Catalog from productization (ART)

### Non-Goals

N/A

## Proposal

### User Stories

#### Story 1

Create a pre-flight check in the CVO process to ensure the Service Catalog,
Ansible Service Broker, Template Service Broker, or any broker registered to
Service Catalog.

During the upgrade, alert users if the Service Catalog or any Service Brokers
are in use.

#### Story 2

Remove Service Catalog and the brokers from the release payload

1. Remove the Service Catalog from the OpenShift 4.4 release payload (ART)
1. Remove the Ansible Service Broker from the OpenShift 4.4 release payload (ART)
1. Remove the Template Service Broker from the OpenShift 4.4 release payload (ART)

#### Story 3

Remove the Brokers from productization

1. Remove the Template Service Broker from productization (ART)
1. Remove the Ansible Service Broker from productization (ART)
1. Remove the broker sections from the documentation (Docs)

#### Story 4

Remove the broker operators from productization

1. Remove the Template Service Broker Operator from productization (ART)
1. Remove the Ansible Service Broker Operator from productization (ART)
1. Remove the operator instructions from the documentation (Docs)

#### Story 5

1. Remove the Service Catalog from productization (ART)
1. Remove the installation instructions from the documentation (Docs)
1. Ensure you how to disable service catalog instructions are correct (Docs/Eng)

### Implementation Details/Notes/Constraints [optional]

What are the caveats to the implementation? What are some important details that
didn't come across above. Go in to as much detail as necessary here. This might
be a good place to talk about core concepts and how they releate.

### Risks and Mitigations

What are the risks of this proposal and how do we mitigate. Think broadly. For
example, consider both security and how this will impact the larger OKD
ecosystem.

How will security be reviewed and by whom? How will UX be reviewed and by whom?

Consider including folks that also work outside your immediate sub-project.

## Design Details

### Test Plan

**Note:** *Section not required until targeted at a release.*

Consider the following in developing a test plan for this enhancement:
- Will there be e2e and integration tests, in addition to unit tests?
- How will it be tested in isolation vs with other components?

No need to outline all of the test cases, just the general strategy. Anything
that would count as tricky in the implementation and anything particularly
challenging to test should be called out.

All code is expected to have adequate tests (eventually with coverage
expectations).

### Graduation Criteria

The Service Catalog and Brokers have reahed the removal stage of the graduation
criteria.

##### Removing a deprecated feature

- In 4.2, the Service Catalo gand Brokers APIs were marked as deprecated in the
  documentation:

- In 4.3, we alert users when the Service Catalog or its Brokers are in use.

- In 4.4, Service Catalog and Brokers will be removed from shipping.

### Upgrade / Downgrade Strategy

TODO: INSERT UPGRADE INFORMATION HERE

### Version Skew Strategy

N/A

## Implementation History

- 20190916 - Proposal for removal preparation was written
- 20191217 - Proposal to remove Service Catalog and Brokers

## Drawbacks

The idea is to find the best form of an argument why this enhancement should _not_ be implemented.

## Alternatives

Similar to the `Drawbacks` section the `Alternatives` section is used to
highlight and record other possible approaches to delivering the value proposed
by an enhancement.

## Infrastructure Needed [optional]

N/A
