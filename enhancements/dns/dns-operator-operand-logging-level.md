---
title: dns-operator-operand-logging-level
authors:
  - "@miheer"
reviewers:
  - "@alebedev87"
  - "@candita"
  - "@frobware"
  - "@knobunc"
  - "@Miciah"
  - "@rfredette"
approvers:
  - "@frobware"
  - "@knobunc"
  - "@Miciah"
  - "@alebedev87"
  - "@rfredette"
  - "@candita"
creation-date: 2021-10-14
last-updated: 2021-10-14
status: implementable
---

# DNS Log Level API

## Release Signoff Checklist

- [X] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

This enhancement describes the API and code changes necessary to expose
a means to change the DNS Operator and CoreDNS Logging Levels to
cluster administrators.

## Motivation

* As an OpenShift Cluster Administrator, I want to be able to raise the logging level of the DNS Operator and CoreDNS so that I can more quickly
  track down OpenShift DNS issues.

* Supporting a trivial way to raise the verbosity of the DNS Operator and its Operands (CoreDNS) would make debugging
the Operator and CoreDNS issues easier for cluster administrators and OpenShift developers.

For logging purposes, CoreDNS defines several classes of responses, such as error, denial and all.
* denial: either NXDOMAIN or nodata responses (Name exists, type does not). A nodata response sets the return code to NOERROR.
* error: SERVFAIL, NOTIMP, REFUSED, etc. Anything that indicates the remote server is not willing to resolve the request.
* all: all responses, including successful responses, errors, and denials.
A logging level API for CoreDNS logs would assist cluster administrators who wish to have more control
over CoreDNS logs.

Also, a logging level API for the DNS Operator would assist OpenShift developers working on the DNS Operator who may
desire more in-depth logging statements when working on the operator's controllers.

* Some users want to add new prometheus alert 'CoreDNS is returning SERVFAIL for X% of requests alert' to the recent updates of OCP.
  Adding this Prometheus alert is useful, but it would be more useful if we could see which requests were getting SERVFAIL responses.
  So we would like to configure the log plugin for CoreDNS to log queries.

* Some user want to avoid use of tcpdump to see the queries and want log plugin to be enabled to log queries in coredns.

### Goals

Add a user-facing API for controlling the run-time verbosity of the [OpenShift DNS Operator and CoreDNS](https://github.com/openshift/cluster-dns-operator).
### Non-Goals

* Change the default logging verbosity of the DNS Operator or CoreDNS in production OCP clusters.

## Proposal

### DNS Operator Log Level API
We will be defining a new API field `operatorLogLevel` in `DNSSpec` with newly defined type `DNSLogLevel`.
This type is similar to the existing `LogLevel` type except that the values of `DNSLogLevel` are a subset of the values of `LogLevel`.
Valid values will be the following: "Normal", "Debug", "Trace".
We will use [logrus](https://github.com/sirupsen/logrus#level-logging) to set the log level for the operator-level logging.
Logrus has seven logging levels: Trace, Debug, Info, Warning, Error, Fatal and Panic.
```go
log.Trace("Something very low level.")

log.Debug("Useful debugging information.")

log.Info("Something noteworthy happened!")

log.Warn("You should probably take a look at this.")

log.Error("Something failed but I'm not quitting.")

// Calls os.Exit(1) after logging:
log.Fatal("Bye.")

// Calls panic() after logging:
log.Panic("I'm bailing.")
```
After the logging level on a Logger is set, log entries with that severity or anything above it will be logged.
For example, `log.SetLevel(log.InfoLevel)` will log anything that is info or above (warn, error, fatal, panic).  This is the default log level.  
So, we will be reading `operatorLogLevel` in a separate controller to watch dnses and setting log level.

`operatorLogLevel: "Normal"` will set `logrus.SetLogLevel("Info")`.

`operatorLogLevel: "Debug"` will set `logrus.SetLogLevel("Debug")`.

`operatorLogLevel: "Trace"` will set  `logrus.SetLogLevel("Trace")`.


### CoreDNS Log Level API

Valid values for coredns logLevel are: "Normal", "Debug", "Trace" as per `DNSLogLevel` type.

We will enable logging of CoreDNS's [classes of responses](https://github.com/coredns/coredns/tree/master/plugin/log#syntax) that correspond to the log level specified in the API.
So,

`logLevel: "Normal"`  will enable the "errors" class: `log . { class error }`.

`logLevel: "Debug"` will enable the "denial" class: `log . { class denial error }`.

`logLevel: "Trace"` will enable the "all" class: `log . { class all }`.

Note that the `errors` plugin is always enabled.  The `errors` plugin logs TCP/UDP connection errors whereas `log . { class error }` logs DNS error responses (such as SERVFAIL).
The CoreDNS reloads its configuration without requiring a restart, so the operator can adjust CoreDNS's log level just by updating the Corefile configmap without need to restart the pod.


We will be adding an API field `operatorlogLevel` in `DNSSpec` with the type `DNSLogLevel`:
```go
// operatorLogLevel controls the logging level of the DNS Operator.
// Valid values are: "Normal", "Debug", "Trace".
// Defaults to "Normal".
// setting operatorLogLevel: Trace will produce extremely verbose logs.
// +optional
// +kubebuilder:default=Normal
OperatorLogLevel DNSLogLevel `json:"operatorLogLevel,omitempty"`
```
This new field would allow a cluster administrator to specify the desired logging level specifically for the DNS Operator.

Additionally, a new API field `LogLevel` of type `DNSLogLevel` will be added to specify the log level for CoreDNS:
```go
// logLevel describes the desired logging verbosity for CoreDNS.
// Any one of the following values may be specified:
// * Normal logs errors from upstream resolvers.
// * Debug logs errors, NXDOMAIN responses, and NODATA responses.
// * Trace logs errors and all responses.
//  Setting logLevel: Trace will produce extremely verbose logs.
// Valid values are: "Normal", "Debug", "Trace".
// Defaults to "Normal".
// +optional
// +kubebuilder:default=Normal
LogLevel DNSLogLevel `json:"logLevel,omitempty"`
```

Both of these new API fields use the aforementioned `DNSLogLevel` type, which is defined as follows:
```go

// +kubebuilder:validation:Enum:=Normal;Debug;Trace
type DNSLogLevel string

var (
// Normal is the default.  Normal, working log information, everything is fine, but helpful notices for auditing or common operations.  In kube, this is probably glog=2.
DNSLogLevelNormal DNSLogLevel = "Normal"

// Debug is used when something went wrong.  Even common operations may be logged, and less helpful but more quantity of notices.  In kube, this is probably glog=4.
DNSLogLevelDebug DNSLogLevel = "Debug"

// Trace is used when something went really badly and even more verbose logs are needed.  Logging every function call as part of a common operation, to tracing execution of a query.  In kube, this is probably glog=6.
DNSLogLevelTrace DNSLogLevel = "Trace"
)

```

### User Stories

Some users actually want logs of every DNS query for auditing purposes, similar to access logs for ingress or audit logs for the API.


### Implementation Details/Notes/Constraints [optional]

### Risks and Mitigations

Raising the logging verbosity for any component typically results in larger log files that grow quickly.
To mitigate this we will document that logLevel: Trace will produce extremely verbose logs.

## Design Details

### Open Questions [optional]
N/A

### Test Plan

Unit tests will be written to test if setting LogLevel sets the respective logging in CoreDNS.
Unit tests will be written to test if setting operatorLogLevel sets the respective logging in DNS Operator.

### Graduation Criteria

N/A

#### Dev Preview -> Tech Preview

N/A

#### Tech Preview -> GA

N/A

#### Removing a deprecated feature

N/A

### Upgrade / Downgrade Strategy

On downgrade, any logging options are ignored by the DNS Operator and CoreDNS.
The downgraded operator will update the configmap and delete the log stanzas.


### Version Skew Strategy

N/A

## Implementation History

* API Implementation for logging can be found [here](https://github.com/openshift/api/pull/1031/).
* Cluster DNS Operator implementation for logging can be found [here](https://github.com/openshift/cluster-dns-operator/pull/307/).

## Drawbacks


## Alternatives

* Don't provide any DNS logging level APIs for the operator and coredns (current behavior)
* Raise current verbosity of the DNS Operator and coredns (not desirable)
* Use tcpdump to analyze queries.

### API Extensions
Please refer sections `DNS Operator Log Level API` and `CoreDNS Log Level API` under section `Proposal`.

### Operational Aspects of API Extensions

* To set log level for CoreDNS please run the following with the log level you want to set:
```shell
$ oc patch dnses.operator.openshift.io/default -p '{"spec":{"logLevel":"Debug"}}' --type=merge
```

* To set log level for Cluster DNS Operator please run the following with the log level you want to set:
```shell
$ oc patch dnses.operator.openshift.io/default -p '{"spec":{"operatorLogLevel":"Debug"}}' --type=merge
```

#### Failure Modes

There are no known failure modes.

#### Support Procedures

* To check the contents of configmap if the desired log level was set:
```shell
$ oc get configmap/dns-default -n openshift-dns -o yaml
```