---
title: etcd-tuning-profiles
authors:
  - alray
reviewers: # Include a comment about what domain expertise a reviewer is expected to bring and what area of the enhancement you expect them to focus on. For example: - "@networkguru, for networking aspects, please look at IP bootstrapping aspect"
  - @hasbro7, etcd team
  - @tjungblu, etcd team
  - @williamcaban, Openshift product manager
  - @deads2k, implemented a similar feature for API server
approvers: # A single approver is preferred, the role of the approver is to raise important questions, help ensure the enhancement receives reviews from all applicable areas/SMEs, and determine when consensus is achieved such that the EP can move forward to implementation.  Having multiple approvers makes it difficult to determine who is responsible for the actual approval.
  - @hasbro7, etcd team
api-approvers: # In case of new or modified APIs or API extensions (CRDs, aggregated apiservers, webhooks, finalizers). If there is no API change, use "None"
  - None
creation-date: 2023-05-16
last-updated: 2023-05-16
tracking-link: # link to the tracking ticket (for example: Jira Feature or Epic ticket) that corresponds to this enhancement
  - https://issues.redhat.com/browse/ETCD-425
---

# etcd Tuning Profiles

## Summary

This enhancement would replace the hardcoded values for the etcd parameters HEARTBEAT_INTERVAL and LEADER_ELECTION_TIMEOUT with predefined "profiles".
Each profile would map to predefined, and pretested, values for the internal etcd parameters.
This would allow for some user tweaking without giving them access to the full range of values.
This enhancement only covers the mvp for a tech preview release of this new feature; a future enhancement will be necessary.

## Motivation

Customers have asked for the ability to change the etcd heartbeat interval and leader election timeout to aid in the stability of their clusters.
We want to be able to allow for this while minimizing the risk of them setting values that cause issues.
This will also remove the hardcoded platform-dependent values; certain platforms could have different default profiles to maintain backwards compatibilty with the currently hardcoded values.

### User Stories

* As an adminstrator, I want to change etcd tuning profiles to help increase the stability of my cluster and understand the performance/latency cost of the profile change.
* As an Openshift support, I want to walk a customer through changing the active etcd profile in a minimal number of steps.
* As an etcd engineer, I want to easily add and test new profiles and internal profile parameters.

### Goals

* Add the currently known profiles:
  * Default:
    - HEARTBEAT_INTERVAL: 100ms
    - LEADER_ELECTION_TIMEOUT: 1000ms
  * "Slow" (or other name):
    - HEARTBEAT_INTERVAL: 500ms
    - LEADER_ELECTION_TIMEOUT: 2500ms
* Remove the parameters from the podspec rendering and replace with the Profile.
* Add a config map to allow an adminstrator to change the profile.

### Non-Goals

* Adding more profiles beyond those listed above.
* Add an API endpoint to change the profile.
* Handle changing profile without a workload disruption.
* Allow users to set arbitrary values for the profile parameters.

## Proposal

The profiles are a layer of abstraction that allow a customer to tweak etcd to run more reliably on their system, while not being so open as to allow them to easily harm their cluster by (knowingly or not) setting bad values for them.
The values for the two proposed profiles for the Tech Preview of this feature are the current default values (applied to all platform except for) and the values applied for Azure and IBMCloud VPC.
The latter values have been used successfully in the field for some time so the risk to future cluster is minimal.
Changing to a "slower" profile will likely incur a performance/latency penalty, but that is likely an acceptable trade for cluster stability.

In this iteration, for the Tech Preview, we will make it clear that changing the profile will require an etcd redeployment and thus a disruption on workloads. In the future enhancement, we can discuss a more seamless transition between profiles.
We will not allow the user to set arbitrary values for the parameters, they must conform to the profiles values (by way of the profile).

In the Tech Preview, the active profile will be set via a configmap that's applied before the etcd pod starts up. An administrator can update the profile by changing the configmap value and forcing the etcd pods to restart.

### Workflow Description

1. The cluster administrator decides to change the etcd profile from default to slow, or slow to default.
2. They change the configmap with the new profile they want to use.
3. They force an etcd redeployment which restarts the etcd pods which consume the new profile value.

If the profile value is not valid, the configmap will be reset to the platform default before the etcd pods are started.

#### Variation [optional]

None

### API Extensions

None

### Implementation Details/Notes/Constraints [optional]

There will still be hardcoded values for each of the parameters and their mappings.
We can reuse the functions in the Cluster Etcd Operator that retrieve the values, it would just be reading the environment variable for the profile, looking up the mapping, and returning the value.

### Risks and Mitigations

We will need to test the latency of the API server on platforms that currently use the default parameters to see if there are any issues with running slower values.
If there is an issue, we could use slightly different values than proposed above; both Azure and IBMCloud VPC were originally meant to be temporary values to compensate for lacking IOPs.

### Drawbacks

* There will be a required disruption when changing profiles currently, this is to avoid the edge case where the etcd pods have different timeouts/heartbeats while we roll out the profile change.
* Because this is a compromise of configurability and testibility/supportibility, the customer won't get as much control as they may want, but it will greatly reduce the testing/support burden on the Openshift team.

## Design Details

### Open Questions [optional]

* Do we want to attempt to add the API change for the Tech Preview, or continue to handle it in a future enhancement?
* Should we consider an additional profile that is between the proposed 'Default' and 'Slow' profiles?

### Test Plan

**Note:** *Section not required until targeted at a release.*

Given that the profiles map to existing values, it should be possible to update existing tests to run each profile to ensure compatibility and stability.

### Graduation Criteria

**Note:** *Section not required until targeted at a release.*

None

#### Dev Preview -> Tech Preview

N/A

#### Tech Preview -> GA

- Customer feedback
- API extension
- Profile change with minimal disruption
- Profile testing on all platforms
- Performance testing for each profile
- End user documentation
  - Description of different profiles and their effects
  - Steps to change profiles

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

- If the user attempts to set the profile to an invalid value (not one of the predefined profile names), the configmap will be reset to the last good value.

## Implementation History

None

## Alternatives

### Arbitrary parameter values
Instead of restricting the customer to predefined profiles, this would allow them direct access to the parameters and allow them to set arbitrary values (within some bounds).

This would allow for more flexibility, but would also likely increase the number of support cases as it's very likely they will set values that are too slow, too fast, or exercise an edge case that cause more diruptions.
Another downside is that with the profiles, there is a discrete, small, number of permutations to test, giving more confidence of the exact effects a given profile has on performance/latency; allow arbitrary values, by definition, greatly increases the testing permutations, making it very difficult to catch bad values before they're allowed in production.

## Infrastructure Needed [optional]

None
