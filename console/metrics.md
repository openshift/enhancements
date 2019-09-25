---
title: Console Metrics for Telemetry
authors:
  - "@benjaminapetersen"
reviewers:
  - "@spadgett"
  - "@bparees"
approvers:
  - "@spadgett"
  - "@bparees"
creation-date: 2019-09-20
last-updated: 2019-09-20
status: implementable
---

# Console Metrics for Telemetry

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift/docs]

## Summary

The purpose of this enhancement is to enable a set of telemetry metrics 
for the console in order to improve the support of our customer environments.  

## Motivation

### Goals

- Tracking metrics via telemetry would allow us to gather feature usage data for the 
console so that we can focus development efforts on the features customers find most 
valuable.

### Non-Goals

- Analytics will not be handled in this proposal.  Previously, the console supported an optional 
Google Analytics identifier. No other analytics platform is or has been supported.
- Error tracking will not be handled in this proposal.  We have discussed the use of tools like 
Sentry for collecting errors and stack traces to improve our ability to understand console 
error frequency and severity within the browser.

## Proposal

- Track the console URL as a metadata metric like `console_url` to support UHC to provide backlinking 
to the console.
- Track API requests from the console to the API server as a metric like `console_api_requests_total` 
with labels such as `referer` and `path` to clearly understand how the console consumes the API and 
make the correct design and optimizations for it.
- Track console pageviews as a metric like `console_page_view_total` to understand what pages and features
customers find most useful.
- Track user agent as a metric like `console_user_agent_total` to understand which browsers are most used 
by customers.  This metric may be collapsed into `console_page_view_total` with the `user agent` tracked 
as an additional label. 
- Track console login as a metric like `console_login_total` to understand average console use.
- Track enabled console extensions as a metric like `console_extension_enabled_total` to understand
what extensions customers find most valuable.  


### User Stories [optional]

#### Story 1

Instrument the console to collect the other metrics such as `console_page_view_total` and expose a 
`/metrics` endpoint. 

Prototype work has begun here https://github.com/openshift/console/pull/2821
    - expose a `/metrics` endpoint
    - collect `console_api_requests_total` with labels `"code", "method", "path", "referer"`

Additional work to do:

- `console_page_view_total{page="<page>" user-agent="<agent>"} <count>`
- `console_login_total <count>`
- `console_extension_enabled_total{extension="<extension>" enabled="<true|false"} <count>`

Note that we will not track personally identifiable information. The metric `console_login_total` 
should not track `user_id`, for example.  The `console_page_view_total` may pose a problem in that 
resource names (such as namespaces) are included in URLs. 

#### Story 2

Protect the `/metrics` endpoint of the `console` behind RBAC.

Use [kube-rbac-proxy](https://github.com/openshift/kube-rbac-proxy) as a sidecar container for the 
console to protect the `/metrics` endpoint specifically.  This seems to be precisely the reason that 
this component was developed and may fit our use case nicely.  There will be very little code changes 
to the console itself. 

### Story 3

Decide if the other metrics besides `console_url` should be reported back to Telemeter.  Open the 
appropriate PRs to the various repositories to make this happen.


### Implementation Details/Notes/Constraints [optional]

None.

### Risks and Mitigations

As we gather more metrics we need to be mindful of privacy and GDPR concerns. 

## Design Details

### Test Plan

**Note:** *Section not required until targeted at a release.*

An e2e test to make a request against the `/metrics` endpoint and verify 
that the expected fields exist should be sufficient.

### Graduation Criteria

**Note:** *Section not required until targeted at a release.*

Not relevant.

#### Examples

Not relevant.

##### Dev Preview -> Tech Preview

Not relevant.

##### Tech Preview -> GA 

Not relevant.

##### Removing a deprecated feature

Not relevant.

### Upgrade / Downgrade Strategy

Not relevant.

### Version Skew Strategy

Not relevant.

## Implementation History

We have already enabled the `/metrics` endpoint for the console operator.  In 
addition, the operator tracks `console_url`, which has been whitelisted for 
telemeter. The work for this has been completed in:

- `console_url` instrumented in the `console-operator` https://github.com/openshift/console-operator/pull/270
- `console_url` approved for telemetry: https://github.com/openshift/telemeter/pull/239
- `console_url` in cluster-monitoring-operator: https://github.com/openshift/cluster-monitoring-operator/pull/486
- `console_url` to observatorium: https://github.com/observatorium/configuration/pull/71/files
- `console_url` to [sass-telemeter](https://gitlab.cee.redhat.com/service/saas-telemeter/merge_requests/54)

## Drawbacks

As stated in "risks and mitigations" above, as we gather more metrics we need 
to be mindful of privacy and GDPR concerns.  

## Alternatives

To protect the `/metrics` endpoint of the console we could import [library-go](https://github.com/openshift/library-go) 
to use within the console backend server binary instead of using [kube-rbac-proxy](https://github.com/openshift/kube-rbac-proxy).  
This would allow us to make `tokenreview` and `subjectaccessreview` for the `/metrics` endpoint. 
The console backend server is not an API server, so this would require more significant code changes.  
The side-car container pattern seems a better solution for augmenting existing functionality.

## Infrastructure Needed [optional]

None.
