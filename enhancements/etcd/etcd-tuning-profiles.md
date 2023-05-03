---
title: etcd-tuning-profiles
authors:
  - "@dusk125"
reviewers: # Include a comment about what domain expertise a reviewer is expected to bring and what area of the enhancement you expect them to focus on. For example: - "@networkguru, for networking aspects, please look at IP bootstrapping aspect"
  - "@hasbro7, etcd team"
  - "@tjungblu, etcd team"
  - "@williamcaban, Openshift product manager"
  - "@deads2k, implemented a similar feature for API server"
approvers: # A single approver is preferred, the role of the approver is to raise important questions, help ensure the enhancement receives reviews from all applicable areas/SMEs, and determine when consensus is achieved such that the EP can move forward to implementation.  Having multiple approvers makes it difficult to determine who is responsible for the actual approval.
  - "@hasbro7, etcd team"
api-approvers: # In case of new or modified APIs or API extensions (CRDs, aggregated apiservers, webhooks, finalizers). If there is no API change, use "None"
  - None
creation-date: 2023-05-16
last-updated: 2023-09-21
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

* Add profiles that map to the existing values.
* Remove the parameters from the podspec rendering and replace with the Profile.
* Add an API to allow admins to change the profile.

### Non-Goals

* Adding more profiles beyond those listed above.
* Handle consuming profile changes without an etcd rollout.
* Allow users to set arbitrary values for the profile parameters.

## Proposal

The profiles are a layer of abstraction that allow a customer to tweak etcd to run more reliably on their system, while not being so open as to allow them to easily harm their cluster by (knowingly or not) setting bad values for them.
The default profile ("" or unset) will allow for upgrades to this feature as it tells the system to choose the values based on the platform: this is the current behavior.
The values for the two proposed profiles for the Tech Preview of this feature are the current default values (applied to all platform except for) and the values applied for Azure and IBMCloud VPC.
The latter values have been used successfully in the field for some time so the risk to future cluster is minimal.
Changing to a "slower" profile will likely incur a performance/latency penalty, but that is likely an acceptable trade for cluster stability.

In this iteration, for the Tech Preview, we will make it clear that changing the profile will require an etcd redeployment.
In the future enhancement, we can discuss a more seamless transition between profiles.
We will not allow the user to set arbitrary values for the parameters, they must conform to the profiles values (by way of the profile).

The active profile will be set via the API Server, then an etcd rollout will be triggered automatically by the Cluster Etcd Operator env var controller to consume the new profile.

The entry for the profile will be added to the operator/v1 etcd operator config crd in the API server, named ControlPlaneHardwareSpeed to allow for other, non-etcd, components to map their own configuration based on the set profile in the future.

The profiles that will be added are:
* Default (""):
  - HEARTBEAT_INTERVAL: Platform dependent
  - LEADER_ELECTION_TIMEOUT: Platform dependent
* Standard:
  - HEARTBEAT_INTERVAL: 100ms
  - LEADER_ELECTION_TIMEOUT: 1000ms
* "Slower" (or other name):
  - HEARTBEAT_INTERVAL: 500ms
  - LEADER_ELECTION_TIMEOUT: 2500ms

These profiles are based on the current platform independent, hard-coded values. All platforms are on 'Standard' except for Azure and IBMCloud VPC, which are on 'Slower'; these profiles will be set when the Default profile is set.

### Workflow Description

1. The cluster administrator decides to change the etcd profile from default to slower, or slower to default.
2. They set the new profile in the API server.
3. They force an etcd redeployment which restarts the etcd pods which consume the new profile value.

If the profile value is not valid, the API should fail to accept the value and return an error.

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

* There will be a required etcd rollout when changing profiles, this is to avoid the edge case where the etcd pods have different timeouts/heartbeats while we roll out the profile change.
* Because this is a compromise of configurability and testibility/supportibility, the customer won't get as much control as they may want, but it will greatly reduce the testing/support burden on the Openshift team.

## Design Details

### Open Questions [optional]

* Should we consider an additional profile that is between the proposed 'Default' and 'Slower' profiles?

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

- If the user attempts to set the profile to an invalid value (not one of the predefined profile names), the API will not accept the value and return an error.

## Implementation History

None

## Alternatives

### Arbitrary parameter values
Instead of restricting the customer to predefined profiles, this would allow them direct access to the parameters and allow them to set arbitrary values (within some bounds).

This would allow for more flexibility, but would also likely increase the number of support cases as it's very likely they will set values that are too slow, too fast, or exercise an edge case that cause more diruptions.
Another downside is that with the profiles, there is a discrete, small, number of permutations to test, giving more confidence of the exact effects a given profile has on performance/latency; allow arbitrary values, by definition, greatly increases the testing permutations, making it very difficult to catch bad values before they're allowed in production.

## Infrastructure Needed [optional]

None
