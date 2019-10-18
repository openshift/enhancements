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
last-updated: 2019-10-18
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

The purpose of this enhancement is to enable a set of telemetry metrics for the console in order to improve 
the support of our customer environments.  

Initially we will track `console_url` to support UHC by providing a mechanism to link back to the console 
running on a cluster. 

In addition, we may track `console_login_total` as a count (no identifying information) in order to establish 
baseline console usage.   

We may be interested in additional metrics in the future. 


### Goals

- Tracking `console_url` allows us to support UHC by providing backlink functionality to a console
running on a cluster.
  

### Non-Goals

- Unfortunately, granular metrics will not be handled in this proposal.  There is a wealth of opportunity
in tracking metrics like `console_page_views` to help us understand what features of the console provide
the most value.  This would then help us focus development efforts on those high value features. Unfortunately, 
this would generate too many time-series and would exceed the given budget of `10`.  Therefore we will not 
be tracking the following metrics:
  - `console_page_views{page=<page>}`
  - `console_api_requests_total` 
  - `console_extension_total{extension=<extension>}`  
- Tracking console login via `console_login_total` could help us understand general console usage on a cluster.
The problem is that we will not know token lifetime.  The default is 1 day but it can be configured to weeks,
months, or years.  This metric would not consistently signal usage.
  - `console_login_total`
- We have evaluated a set of metrics around login that could eventually prove valuable but are not being 
pursued at this time.  Failed login situation, if attached to an alert, could be meaningful:
  - `console_failed_login_total`
  - `console_unique_login_total`
  - `console_login_oauth_error_total`
- We have also evaluated metrics around our extension points and may consider tracking in the future:
  - `console_extension_link_total`
  - `console_extension_cli_download_total`
  - `console_extension_external_log_link_total`
  - `console_extension_notification_total`
- We have also discussed several fundamental metrics.
  - `cpu`
  - `memory`
- Analytics will not be handled in this proposal.  Previously, the console supported an optional 
Google Analytics identifier. No other analytics platform is or has been supported.  Analytics tools would 
provide a more comprehensive structure for handling page views and usage.
- Error tracking will not be handled in this proposal.  We have discussed the use of tools like 
Sentry for collecting errors and stack traces to improve our ability to understand console 
error frequency and severity within the browser.

## Proposal

Tracking `console_url` and `console_login_total` will require the following:

- Instrumentation of the console operator to report back `console_url` via a `/metrics` endpoint.
- ServiceMonitor objects must be created for the console operator.
- Proposed metrics must be whitelisted for Telemeter.

### User Stories [optional]

#### Story 1

#### Story 2

## Risks and Mitigations

As with any data tracking, we need to be mindful of privacy and GDPR concerns. The proposal in 
current form will track no user specific identification data.

If we chose to track `console_login_total` via a `/metrics` endpoint in the console, we would need a 
mechanism of authentication for this endpoint.  The console is not built on top of library-go. Therefore 
we would choose to use [kube-rbac-proxy](https://github.com/openshift/kube-rbac-proxy) as a sidecar container 
and a front to the endpoint.

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

## Implementation History

Instrumentation of the console operator to track `console_url` has been completed in the following PR:

- `console_url` instrumentation in the [console operator](https://github.com/openshift/console-operator/pull/270)
- `console_url` approved for [telemetry](https://github.com/openshift/telemeter/pull/239)
- `console_url` in [cluster-monitoring-operator](https://github.com/openshift/cluster-monitoring-operator/pull/486)
- `console_url` to [observatorium](https://github.com/observatorium/configuration/pull/71/files)
- `console_url` to [sass-telemeter](https://gitlab.cee.redhat.com/service/saas-telemeter/merge_requests/54)