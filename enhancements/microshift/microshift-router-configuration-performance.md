---
title: microshift-router-configuration-performance
authors:
  - "@eslutsky"
reviewers:
  - "@pacevedom"
  - "@copejon"
  - "@ggiguash"
  - "@pmtk"
  - "@pliurh"
  - "@Miciah"
approvers:
  - "@jerpeter1"
api-approvers:
  - None
creation-date: 2024-09-23
last-updated: 2024-10-07
tracking-link:
  - https://issues.redhat.com/browse/USHIFT-4091
---
z
# MicroShift router Operations & performance configuration options

## Summary
MicroShift's default router is created as part of the platform, but does not
allow configuring any of its specific parameters. For example, you cannot
specify the policy for HTTP traffic compression or enable HTTP2 protocol.

In order to allow these operations and many more, a set of configuration options
is proposed.

## Motivation

Microshift Customers need a way to override the default Ingress Controller  configuration parameters similar as OpenShift does.


### User Stories

#### User Story 1

My application starts processing requests from clients, but the connection is
getting closed before it can respond.

I set `ingress.tuningOptions.serverTimeout` in the configuration file to a
higher value to accommodate the slow response from the server.

#### User Story 2

The router has many connections open because an application running on my
cluster doesn't close connections properly.

I set `ingress.tuningOptions.serverTimeout` and
`spec.tuningOptions.serverFinTimeout` in the ingresscontroller API to a lower
value, forcing those connections to close sooner if my application stops
responding to them.

### Goals
Allow users to configure the additional HAProxy/Router performance customization parameters, see Proposal table for details.


### Non-Goals
N/A

## Proposal

Microshift don't use ingress [operator](https://github.com/openshift/cluster-ingress-operator) , all the customization performed through  configuration file.
the configuration will propagate to the router deployment [manifest](https://github.com/openshift/microshift/blob/aea40ae1ee66dc697996c309268be1939b018f56/assets/components/openshift-router/deployment.yaml) Environment variables.

see the table below for the proposed configuration changes:

| new configuration                                 | description                                                                                                                                                                                     | default |
| ------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------- |
| ingress.httpCompressionMimeTypes                  | list of MIME types that should have compression applied. <br>At least one MIME type must be specified.<br>                                                                                      | none    |
| ingress.forwardedHeaderPolicy                     | specify when and how the Ingress Controller sets the `Forwarded`, `X-Forwarded-For`, `X-Forwarded-Host`, `X-Forwarded-Port`, `X-Forwarded-Proto`, and `X-Forwarded-Proto-Version` HTTP headers. | Append  |
| ingress.tuningOptions.headerBufferBytes           | describes how much memory should be reserved (in bytes) for IngressController connection sessions.                                                                                              | 32768   |
| ingress.tuningOptions.headerBufferMaxRewriteBytes | describes how much memory should be reserved  from headerBufferBytes for HTTP header rewriting and appending for IngressController connection sessions.                                         | 8192    |
| ingress.tuningOptions.healthCheckInterval         | defines how long the router waits between two consecutive health checks on its configured backends.                                                                                             | 5000ms  |
| ingress.tuningOptions.clientTimeout               | defines the maximum time to wait for a connection attempt to a server/backend to succeed.                                                                                                       | 30s     |
| ingress.tuningOptions.clientFinTimeout            | defines how long a connection will be held open while waiting for the client response to the server/backend closing the connection.                                                             | 1s      |
| ingress.tuningOptions.serverTimeout               | defines how long a connection will be held open while waiting for a server/backend response.                                                                                                    | 30s     |
| ingress.tuningOptions.serverFinTimeout            | defines how long a connection will be held open while waiting for the server/backend response to the client closing the connection.                                                             | 1s      |
| ingress.tuningOptions.tunnelTimeout               | defines how long a tunnel connection (including websockets) will be held open while the tunnel is idle.                                                                                         | 1h      |
| ingress.tuningOptions.tlsInspectDelay             | defines how long the router can hold data to find a matching route.                                                                                                                             | 5s      |
| ingress.tuningOptions.threadCount                 | defines the number of threads created per HAProxy process..                                                                                                                                     | 4       |
| ingress.tuningOptions.maxConnections              | defines the maximum number of simultaneous connections that can be established per HAProxy process.                                                                                             | 50000   |
| ingress.logEmptyRequests                          | specifies how connections on which no request is received should be logged.                                                                                                                     | Log     |
| ingress.httpEmptyRequestsPolicy                   | indicates how HTTP connections for which no request is received should be handled.                                                                                                              | Respond |
| ingress.defaultHTTPVersion                        | Determines default http version should be used for the ingress backends.                                                                                                                        | 1       |

see full [ocp](https://docs.openshift.com/container-platform/4.17/networking/ingress-operator.html) configuration reference.


### Workflow Description
**cluster admin** is a human user responsible for configuring a MicroShift
cluster.

1. The cluster admin adds specific configuration for the router prior to
   MicroShift's start.
2. After MicroShift started, the system will ingest the configuration and setup
   everything according to it.


### API Extensions
As described in the proposal, there is an entire new section in the configuration:
```yaml
ingress:
    tuningOptions:
        headerBufferBytes: 32768
        headerBufferMaxRewriteBytes: 8192
        healthCheckInterval: 5000m
        clientTimeout: 30s
        clientFinTimeout: 1s
        serverTimeout: 30s 
        serverFinTimeout: 1s
        tunnelTimeout: 1h
        tlsInspectDelay: 5s
        threadCount: 4
        maxConnections: 50000
    httpCompressionMimeTypes: none
    LogEmptyRequests: Log
    forwardedHeaderPolicy: Append
    HTTPEmptyRequestsPolicy: Respond
    HTTP2IsEnabled: false
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

#### How config options change manifests
Each of the configuration options described above has a direct effect on the
manifests that MicroShift will apply after starting.  
see the full Implementation details in the [router-configuration](microshift-router-configuration.md) enhancement.

see the [table](#proposal) in the proposal above for all the new configuration options.



### Risks and Mitigations
-  Incorrect ingress configuration can cause network  disruption and unpredictable behavior , the documentation should warn about dangers of changing the defaults. 

### Drawbacks

- Setting the timeout higher may cause some dead connections to be kept open
  for longer, and would add to the memory footprint of the router
- Setting the timeout too low can cause connection closure before the server or
  client has enough time to respond


## Open Questions
N/A

## Test Plan
all of of the configuration changes listed here will be included in the current e2e scenario
testing harness in Microshift, verifying that its applied to the ingress deployment pods.
testing ingress functionality is out of scope .

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

When upgrading from 4.17 or earlier to 4.18, the new configuration fields will remain
unset, causing the existing defaults to be used.

When downgrading from 4.18 to 4.17 or earlier, the specified timeout values will
be discarded, and the previous defaults will be used.


## Version Skew Strategy
N/A

## Operational Aspects of API Extensions

### Failure Modes
N/A

## Support Procedures
N/A

## Implementation History
Implementation [PR](https://github.com/openshift/microshift/pull/4000) for Micorshift
## Alternatives (Not Implemented)
N/A
