---
title: microshift-router-configuration-errors-logging
authors:
  - "@pacevedom"
reviewers:
  - "@eslutsky"
  - "@copejon"
  - "@ggiguash"
  - "@pmtk"
  - "@pliurh"
  - "@Miciah"
approvers:
  - "@jerpeter1"
api-approvers:
  - None
creation-date: 2025-04-24
last-updated: 2025-04-24
tracking-link:
  - https://issues.redhat.com/browse/USHIFT-4092
---

# MicroShift router errors and logging configuration options

## Summary
MicroShift's default router is created as part of the platform, but does not
allow configuring some of its specific parameters. For example, you cannot
configure custom behavior with error pages, or whether headers and cookies are
captured in the access logs.

In order to allow these operations and more, a set of configuration options is
proposed.

## Motivation
Microshift Customers need a way to override the default Ingress Controller
logging configuration similar as OpenShift does.

### User Stories
* As a MicroShift admin, I want to configure custom error code pages in the
  router.
* As a MicroShift admin, I want to enable/disable access logging in the
  router.
* As a MicroShift admin, I want to configure which HTTP headers are captured
  in the access logs.
* As a MicroShift admin, I want to configure which HTTP cookies are captured
  in the access logs.

### Goals
Allow users to configure the additional Router customization parameters.

### Non-Goals
N/A

## Proposal
Microshift doesnt use [ingress operator](https://github.com/openshift/cluster-ingress-operator),
which means all the customization is performed through the configuration file.
The configuration will propagate to the router deployment [manifest](https://github.com/openshift/microshift/blob/aea40ae1ee66dc697996c309268be1939b018f56/assets/components/openshift-router/deployment.yaml) through environment variables, just like what the ingress operator does.

See the API Extensions section to check the details.

See full [OpenShift Router](https://docs.openshift.com/container-platform/4.18/networking/ingress-operator.html)
configuration reference for more information.

### Workflow Description
***configuring errors and logging options***
1. The cluster admin adds specific configuration for the router prior to
   MicroShift's start.
2. After MicroShift starts, the system will read the configuration and setup
   the router according to it.

### API Extensions
As mentioned in the proposal, there is an entire new section in the configuration:
```yaml
ingress:
    httpErrorCodePages:
      name: <string>
    accessLogging:
      status: <Enabled|Disabled>
      format: <string>
      httpCaptureHeaders:
        request:
          - maxLength: <integer>
            name: <string>
        response:
          - maxLength: <integer>
            name: <string>
      httpCaptureCookies:
        - matchType: <Exact|Prefix>
          maxLength: <integer>
          name: <string>
          namePrefix: <string>
```

For more information check each individual section.

#### Hypershift / Hosted Control Planes
N/A
### Topology Considerations
N/A

#### Standalone Clusters
N/A

#### Single-node Deployments or MicroShift
Enhancement is solely intended for MicroShift.

### Implementation Details/Notes/Constraints
The default router is composed of a bunch of assets that are embedded as part
of the MicroShift binary. These assets come from the rebase, copied from the
original router in [cluster-ingress-operator](https://github.com/openshift/cluster-ingress-operator).

Based on the configuration parameters, the manifest for the router pod will
mutate to translate all the new options.

#### Enabling access logging
HAProxy allows configuration for access logging. This happens through rsyslog,
which requires another container in the router. This is the equivalent to the
`Container` approach in OpenShift Router access logging.

The second container (named `access-log`) will print through stdout all the
logs from the router.

This approach does not require configuring rsyslogd in the host and is self
contained, not dedicating any resources in case it is not enabled.

Configuring either of `ingress.accessLogging.httpCaptureHeaders` or
`ingress.accessLogging.httpCaptureCookies` will also enable `ingress.accessLogging.status`.

`ingress.accessLogging.status` defaults to `Disabled`.

#### Configuring access log format
`ingress.accessLogging.format` specifies the format of the log message for an
HTTP request. If this field is empty, log messages use the implementation's
default HTTP log format, which is described [here](http://cbonte.github.io/haproxy-dconv/2.0/configuration.html#8.2.3).

Note that this format only applies to cleartext and encryption terminated
requests.

#### Configuring custom error code pages
To configure custom error code pages the user needs to specify a configmap name
in `ingress.httpErrorCodePages.name`. This configmap must be in the
`openshift-config` namespace and should have keys in the format of
`error-page-<error code>.http` where `<error code>` is an HTTP status code.

Each value in the configmap should be the full response, including HTTP
headers.

As of today, only errors for 503 and 404 can be customized.

`ingress.httpErrorCodePages.name` defaults to empty.

#### Capturing headers
To configure specific HTTP header capture so they are included in the access
logs the user needs to create entries in `ingress.accessLogging.httpCaptureHeaders`.
This field is a list and allows capturing request and response headers
independently. Each of the entries in the list has different parameters that
follow. If the list is empty (which is the default value) no headers will be
captured.

This option only applies to cleartext HTTP or reencrypt connections. Headers
can not be captured for TLS passthrough connections.

Each element of the list includes:
* `request`. Specifies which HTTP request headers to capture. If this field is
  empty, no request headers are captured.
* `response`. Specifies which HTTP response headers to capture. If this field
  is empty, no request headers are captured.

Both elements have the same fields:
* `maxLength`. Specifies a maximum length for the header value. If a header
  value exceeds this length, the value will be truncated in the log message. Minimum value 1.
* `name`. Specifies a header name.  Its value must be a valid HTTP header name
  as defined in RFC 2616 section 4.2. String regex ```^[-!#$%&'*+.0-9A-Z^_`a-z|~]+$```.

If configured, it is mandatory to include at least `maxLength` and `name`.

`ingress.accessLogging.httpCaptureHeaders` defaults to an empty list.

#### Capturing cookies
To configure specific HTTP cookie capture so they are included in the access
logs the user needs to create an entry in `ingress.accessLogging.httpCaptureCookies`.
This field is a list (limited to 1 element) which includes information on which
cookie to capture. If the list is empty (which is the default value) no cookies
will be captured.

In each element of the list we find:
* `matchType`. Specifies the type of match to perform against the cookie name.
  Allowed values are `Exact` and `Prefix`.
* `maxLength`. Specifies a maximum length of the string that will be logged,
  which includes the cookie name, cookie value, and one-character delimiter.
  If the log entry exceeds this length, the value will be truncated in the log
  message. Minimum value 1, maximum value 1024.
* `name`. Specifies a cookie name. It must be a valid HTTP cookie name as
  defined in RFC 6265 section 4.1. String regex ```^[-!#$%&'*+.0-9A-Z^_`a-z|~]*$```.
  Minimum length 0, maximum length 1024.
* `namePrefix`. Specifies a cookie name prefix. It must be a valid HTTP cookie
  name as defined in RFC 6265 section 4.1. String regex ```^[-!#$%&'*+.0-9A-Z^_`a-z|~]*$```.
  Minimum length 0, maximum length 1024.

If configured, it is mandatory to include at least `matchType` and `maxLength`.

`ingress.accessLogging.httpCaptureCookies` defaults to an empty list.

#### How config options change manifests
Each of the configuration options described above has a direct effect on the
manifests that MicroShift will apply after starting.  
See the full Implementation details in the [router-configuration](microshift-router-configuration.md)
enhancement.

### Risks and Mitigations
* Not configuring custom error pages will return the default ones, which are
  usually empty and only return the http status code.
* Not configuring capture of http headers and/or cookies will not include them
  in the access logs of the router.

### Drawbacks
N/A

## Open Questions
N/A

## Test Plan
All configuration changes will be included in already existing e2e router
tests. Testing router functionality is out of scope of this enhancement.


## Graduation Criteria
Not applicable

### Dev Preview -> Tech Preview
- Ability to utilize the enhancement end to end
- End user documentation, relative API stability
- Sufficient test coverage

### Tech Preview -> GA
- More testing (upgrade, downgrade)
- Sufficient time for feedback
- Available by default
- User facing documentation created in [openshift-docs](https://github.com/openshift/openshift-docs/)

### Removing a deprecated feature
N/A

## Upgrade / Downgrade Strategy
When upgrading from 4.19 or earlier the new configuration fields will remain
unset, causing the existing defaults to be used.

When downgrading from 4.20 to earlier versions the new parameters will be
ignored.

## Version Skew Strategy
N/A

## Operational Aspects of API Extensions

### Failure Modes
N/A

## Support Procedures
Access logging, if enabled, will be part of the logs of the openshift-router
logs. Logs from this pod are captured in the already existing sos report
procedure available for MicroShift.

## Implementation History
Implementation [PR](https://github.com/openshift/microshift/pull/4474/) for Micorshift
## Alternatives (Not Implemented)
N/A
