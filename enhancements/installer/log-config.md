---
title: log-config
authors:
  - @patrickdillon
reviewers: # Include a comment about what domain expertise a reviewer is expected to bring and what area of the enhancement you expect them to focus on. For example: - "@networkguru, for networking aspects, please look at IP bootstrapping aspect"
  - @rna-afk
  - @2uasimojo
  - @jstuever
  - @jhixson74
approvers:
  - @sdodson
  - @zaneb
api-approvers: # In case of new or modified APIs or API extensions (CRDs, aggregated apiservers, webhooks, finalizers). If there is no API change, use "None"
  - n/a
creation-date: 2022-07-25
last-updated: 2022-07-25
tracking-link: # link to the tracking ticket (for example: Jira Feature or Epic ticket) that corresponds to this enhancement
  - https://issues.redhat.com/browse/CORS-2068
see-also:
  - https://docs.google.com/document/d/16_tccZjADKJMd2txQ7nmz8olZ5iUrw81XxGTRmqMXoU/edit#
---

# Installer Log Configuration

## Summary

This enhancement proposes the introduction of a log-config.yaml configuration
file which allows users to pass input that will be used specifically for
configuring logging behavior in the Installer.

## Motivation

* Decorating installer logs with k/v pairs provided by the ARO resource provider is the
top priority ask for the installer in the 4.12 release. The ARO team wants this feature
in order to filter logs when logs from multiple installs are pooled. For example, all installer
logs could be decorated with a k/v pair denoting an aroClusterID:

```shell
INFO OpenShift Installer unreleased-master-6230-gf46ac2f0d1a3628a51d3437bd79e4221da9aff23-dirty  aroClusterID=123
INFO Built from commit f46ac2f0d1a3628a51d3437bd79e4221da9aff23  aroClusterID=123
INFO Fetching Master Machines...                   aroClusterID=123
INFO Loading Master Machines...                    aroClusterID=123
```

* We can generalize the ask from ARO so that it may be useful to other users
* We should keep future logging needs in mind when designing this solution 
so that we do not complicate or preclude future development work. Future
considerations could be:
  * Configuring any hooks within `logrus`
([list of service hooks](https://github.com/sirupsen/logrus/wiki/Hooks))
  * Allowing formatting of logs as JSON (cf. `logrus`
[config docs](https://pkg.go.dev/github.com/sirupsen/logrus?utm_source=godoc#JSONFormatter))
  * Changing output path for logs
  * Configuring any future logging solution other than `logrus`


### User Stories

* As an installer user, I want to be able to provide a set of key-value pairs that
will decorate all Installer log lines
* As an installer developer, I want the design for this user input to be extensible

### Goals

The goal of this enhancement is to determine the proper way of passing user input
for the need of configuring logging.

### Non-Goals

The need for decorating log lines is just an example of the kind of configuration
that is needed. The implementation for decorating log lines is not relevant
to this discussion.

## Proposal

This design proposes we add support for a log-config.yaml file to
allow users to pass in logging configuration, for example:

```go
type LogConfig struct {
  // Fields contains user-provided fields to be added to all log entries.
  // +optional
  Fields map[string]string `json:"fields,omitempty"`
}
```

The install-config is the typical vehicle for installer configuration,
but it is not acceptable in this use case because the install-config
is an Installer Asset, and logging must be configured:

* before assets are initialized
* despite asset failures

In the example use case of decorating logs with fields, we need
to decorate lines that are output before the install config is loaded
and we need to decorate output that would occur if there are
errors in the install config and it cannot be loaded.

The log-config.yaml file would be loaded if present in the install directory,
as well as adding a command line flag  --log-config  to accept a path to the config file.

Users should be able to provide their config in a file that looks, for example, like:

```yaml
fields:
  key1: value1
  key2: value2
  key3: value3
```

### Workflow Description

1. Installer user specifies configuration in `log-config.yaml`
2. `log-config.yaml` is placed in install dir or path is specified with a flag
3. Installer uses `log-config.yaml` for any Installer commands


### API Extensions

n/a

### Risks and Mitigations

Adding the ability to configure logging comes with a risk that the
logging could have errors or misconfiguration. There is an Open
Question about whether logging errors should fail the install or
just log a warning message:

If users include a `log-config.yaml` file, they intend to configure
logging. If there is an error in that file, should we prevent installs
until that configuration is properly handled? Or should we make the
best effort to complete the install although the logging is not
properly configured? 

My weakly held opinion is that we should prevent attempting installs if
`log-config.yaml` is supplied but misconfigured. This would be in line
with existing installer behavior for the `--log-level` flag which
errors if an invalid value is passed. Any additions to the config
should be backwards compatible and the default path for the config
file would not change.

### Drawbacks

This enhancement proposes a new way to introduce logging configuration.
The existing methods we have are: install config, CLI flags, and 
environment variables (not supported for customer user). By introducing
another method of configuration, we risk confusion. I do not think
this is a serious drawback or perhaps even a valid one.

A more serious consideration is whether this should be implemented by the installer
at all; or just handled by the client. The main drawbacks to implementing this in the
installer are opportunity cost and maintenance. The implementation is not highly complex
so from a technical perspective, the main drawback would be a potential slippery slope
of introducing a second API for logging configuration.

## Design Details

### Open Questions [optional]

1. Should this logging functionality be implemented in the installer or be
the responsibility of the client?
3. Should logging misconfiguration prevent installs? (see Risks & Mitigations)
4. Should log-config.yaml be versioned according to k8s API versioning?

### Test Plan

For e2e-tests, it would be possible to drop a log-config.yaml
file in the configuration steps and ensure that logs are
properly configured.

### Graduation Criteria

The intended audience for this feature is ARO, so it would be possible
to have this used by our managed services providers before releasing
to customers, but this level of precaution may not be needed for this 
feature.

### Upgrade / Downgrade Strategy

n/a

### Version Skew Strategy

n/a 

### Operational Aspects of API Extensions

n/a

#### Failure Modes

As discussed above, it is an open question about whether logging
misconfiguration should prevent an install from launching.

## Alternatives

* install config has been ruled out for reasons discussed above
* env vars are not supported for customer use
* passing logging configuration directly through the CLI has poor UX
* implementing this as an "undocumented" feature that could be used
purely by managed services or internal use cases
