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

The conditional gatherer for Insights Operator collects data according to the defined rules*. 
Each rule contains one or more conditions such as "alert A is firing" 
and one or more gatherers with parameters such as "collect N lines of logs from containers in namespace N".
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
- Create a service providing these rules
- Fetch them from there and apply to the conditional gatherer

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

The proposal is to implement the next process of adding new rules:

1. We have a repo with json configs defining the rules. The repo will have some simple CI with validation
against JSON schema and possibly some review process. 
We have created a PoC for the repo: 

https://github.com/tremes/insights-operator-gathering-rules

or the version with an example of JSON schema validation:

https://github.com/tremes/insights-operator-gathering-rules/tree/schema 

2. There's a service living in cloud.redhat.com fetching the rules from the repo and providing them through its API.
The very first version is going to provide just all the rules, but later we may consider splitting them by 
cluster version and introducing some more complicated logic around fetching the rules
3. Insights Operator fetches a config with the rules from the service and unmarshalls JSON to Go structs
4. Insights Operator makes the most important validation which checks the next things:

- The JSON version of the config matches the structs defined in the code 
- For each rule, there's at least one gathering function
- Only implemented conditions can be used
- Alert name from `alert_is_firing` condition should be a string of length between 1 and 128 
and consist of only alphanumeric characters
- Only implemented gathering functions can be used
- Namespace from `gather_logs_of_namespace` function should be a string of length between 1 and 128, 
match the next regular expression `^[a-zA-Z]([a-zA-Z0-9\-]+[\.]?)*[a-zA-Z0-9]$` and start with `openshift-`
- Tail lines from `gather_logs_of_namespace` function should be an integer between 1 and 4096
- Namespace from `gather_image_streams_of_namespace` function should be a string of length between 1 and 128, 
match the next regular expression `^[a-zA-Z]([a-zA-Z0-9\-]+[\.]?)*[a-zA-Z0-9]$` and start with `openshift-`

If anything fails to be validated, the config is rejected and an error is written to the logs.
The PR with validation on operator side - https://github.com/openshift/insights-operator/pull/470

5. Insights Operator goes rule by rule and collects the requested data if corresponding conditions are met

### User Stories

For OpenShift engineers it will be easier to get necessary information when some problem occurs (like alert is firing).
Getting such information now means changing operator's code which is especially painful with older versions of clusters.
When the data is not needed anymore, the rule can be simply removed from the repo.

### Implementation Details/Notes/Constraints [optional]

Empty

### Risks and Mitigations

The potential risks could come from an attacker spoofing the config somehow (if they got access to the repo),
all they could do is let the insights operator collect more data and send it to the c.r.c, but because of validation
on the operator side, the potential of collecting data is limited. For example, 
we check that namespaces start with `openshift-`, we're also limiting the amount of potentially collected data by,
for example, introducing the limit for amount of collected logs. 

Also in the regular workflow, changing the config involves going through the repo's CI (JSON schema validator) 
and probably a simple review process. 

Not really a risk, but in case the service is not available or provides invalid config for the operator,
we would just have less data in the archives (everything except conditional gatherer's data), 
an error would be written to the logs and to the metadata which would allow us to know 
that something is broken and we need to bring the service back.

## Design Details

Empty

### Open Questions [optional]

Empty

### Test Plan

The conditional gatherer is covered with unit tests considering many different scenarios. 
Later there will be integration tests as well.

### Graduation Criteria

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
- The service living in cloud.redhat.com
