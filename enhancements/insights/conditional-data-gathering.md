---
title: conditional-data-gathering
authors:
  - "@Sergey1011010"
reviewers:
  - "@inecas"
  - "@tremes"
  - "@smarterclayton"
approvers:
  - "@smarterclayton"
creation-date: 2021-07-15
last-updated: 2021-07-15
status: implementable
---

# Conditional Data Gathering

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [x] Design details are appropriately documented from clear requirements
- [x] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

The conditional gatherer for Insights Operator collects data according to the defined gathering rules*.
Each rule contains one or more conditions such as "alert A is firing"
and one or more gatherers with parameters such as "collect X lines of logs from containers in namespace N".
Current version has these rules defined in the code and the proposal is to load them from an external source
to make collection of new conditional data faster. It's NOT about running brand new code,
but just calling the existing gatherers with different validated parameters, so the operator can't
collect anything we don't expect.

\* note that rule here and later has nothing to do with rules used to analyze data in archives written
by CCX presentation team

## Motivation

Collecting data on some condition such as an alert is quite common pattern
during the root-cause analysis and the ability to define the rules
what to collect on which condition externally will help to do that faster.

### Goals

- Implement validation of the rules for conditional gatherer
- Create a service taking gathering rules from the git repo and providing them through the API
- Fetch the rules definitions from the service's API and apply them to the conditional gatherer

### Non-Goals

Empty

## Proposal

Right now we have conditional gatherer which can collect data based on some conditions
which are defined in the code, for example the next config would collect 100 lines of logs
from each container in namespace `openshift-cluster-samples-operator`
and image streams of the same namespace when alert `SamplesImagestreamImportFailing` is firing
and add it to the next archive.

```json
[{
  "conditions": [{
    "type": "alert_is_firing",
    "params": {
      "name": "SamplesImagestreamImportFailing"
    }
  }],
  "gathering_functions": {
    "logs_of_namespace": {
      "namespace": "openshift-cluster-samples-operator",
      "tail_lines": 100
    },
    "image_streams_of_namespace": {
      "namespace": "openshift-cluster-samples-operator"
    }
  }
}]
```

Conditions can have type and parameters, implemented types are:

- `alert_is_firing` which is met when the alert with the name from parameter `name` is firing

A gathering function consists of its name and parameters, implemented functions are:

- `logs_of_namespace` which collects logs from each container in the namespace from parameter `namespace`
limiting it to only last N lines from parameter `tail_lines`
- `image_streams_of_namespace` which collects image streams of the namespace from parameter `namespace`

Conditions to be implemented:

- `cluster_version_matches` with parameter `version` makes the rule to be applied on specific cluster version.
It will use semantic versioning so you can wildcards and ranges like here https://github.com/blang/semver#features

The proposal is to implement the next process of adding new rules:

1. We'll have a repo with json configs defining the rules. The repo is going to have a simple CI with validation
against JSON schema and possibly some review process. The repo should live in https://github.com/RedHatInsights
We have created a PoC for the repo:

https://github.com/tremes/insights-operator-gathering-rules

The JSON schema can be found here:

https://raw.githubusercontent.com/openshift/insights-operator/f9b762149cd10ec98079e48b8a96fc02a2aca3c6/pkg/gatherers/conditional/gathering_rule.schema.json

2. There will be a service living in console.redhat.com which will have the rules baked into the container and will
provide all the rules through its API. The very first version of the service is going to be simple, but later we may
add some filtering on the API level (for example by cluster version).

3. Insights Operator fetches a config with the rules from the service and unmarshalls JSON to Go structs.

4. Insights Operator makes the most important validation against the next JSON schemas:

- https://raw.githubusercontent.com/openshift/insights-operator/f9b762149cd10ec98079e48b8a96fc02a2aca3c6/pkg/gatherers/conditional/gathering_rule.schema.json
- https://raw.githubusercontent.com/openshift/insights-operator/f9b762149cd10ec98079e48b8a96fc02a2aca3c6/pkg/gatherers/conditional/gathering_rules.schema.json

which check the next things:

- The JSON version of the config matches the structs defined in the code
- The maximum number of rules is 64
- The rules should not repeat
- There can be up to 8 conditions in each rule
- Only implemented conditions can be used
- Alert name from `alert_is_firing` condition should be a string of length between 1 and 128
and consist of only alphanumeric characters
- For each rule, there's at least one gathering function
- Only implemented gathering functions can be used
- There can be up to 8 gathering functions in each rule
- Namespace from `gather_logs_of_namespace` function should be a string of length between 1 and 128,
match the next regular expression `^[a-zA-Z]([a-zA-Z0-9\-]+[\.]?)*[a-zA-Z0-9]$` and start with `openshift-`
- Tail lines from `gather_logs_of_namespace` function should be an integer between 1 and 4096
- Namespace from `gather_image_streams_of_namespace` function should be a string of length between 1 and 128,
match the next regular expression `^[a-zA-Z]([a-zA-Z0-9\-]+[\.]?)*[a-zA-Z0-9]$` and start with `openshift-`

If anything fails to be validated, the config is rejected and an error is written to the logs.
The PR with validation on operator side - https://github.com/openshift/insights-operator/pull/483

5. Insights Operator goes rule by rule and collects the requested data if corresponding conditions are met

The new gathering functions and conditions are added the same way as any other code changes to insights-operator.
The JSON schema also lives in the operator repo and is changed through the regular process.

### User Stories

For OpenShift engineers it will be easier to get necessary information when some problem occurs (like alert is firing).
Getting such information now means changing operator's code which is especially painful with older versions of clusters.
When the data is not needed anymore, the rule can be simply removed from the repo.

### Implementation Details/Notes/Constraints [optional]

Empty

### Risks and Mitigations

The potential risks could come from an attacker spoofing the config somehow (if they got access to the repo),
all they could do is let the insights operator collect more data and send it to the c.r.c, but because of validation
on the operator side, the potential of collecting data is limited and there still would be
the same anonymization of potentially sensitive data as before.
For example, we check that namespaces start with `openshift-`, we're also limiting
the amount of potentially collected data by, for example, introducing the limit for amount of collected logs
(per container and the number of containers) and, in the worst case, if the conditional gathering takes too much time
it would be stopped by the timeout and we would just get less data.

Also in the regular workflow, changing the config involves going through the repo's CI (JSON schema validator)
and probably a simple review process.

Not really a risk, but in case the service is not available or provides invalid config for the operator,
we would just have less data in the archives (everything except conditional gatherer's data),
an error would be written to the logs and to the metadata which would allow us to know
that something is broken and we need to bring the service back.

In case GitHub is not available, we won't be able to add new rules, but the old ones would still be returned by
the service.

## Design Details

Empty

### Open Questions [optional]

Empty

### Test Plan

The conditional gatherer is covered with unit tests considering many different scenarios.
Later there will be integration tests as well.

### Graduation Criteria

Empty

#### Dev Preview -> Tech Preview

Empty

#### Tech Preview -> GA

Empty

#### Removing a deprecated feature

Empty

### Upgrade / Downgrade Strategy

The conditional gatherer itself (e.g. adding new/removing old conditions or gathering functions) is
modified through a standard process like all other operator's code. Only the rules to collect X with Y params on Z
are updated through the new process described above.

### Version Skew Strategy

Out of scope.

## Implementation History

Major milestones in the life cycle of a proposal should be tracked in `Implementation
History`.

## Drawbacks

None?

## Alternatives

Thin IO idea (https://github.com/openshift/enhancements/pull/465) could also solve this, but thin IO was about adding
new code bypassing the standard release process which could potentially be very dangerous.

## Infrastructure Needed [optional]

- The repo in RedHatInsights
- The service living in console.redhat.com
