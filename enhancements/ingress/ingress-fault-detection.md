---
title: ingress-fault-detection
authors:
  - "@sgreene570"
reviewers:
  - "@danehans"
  - "@frobware"
  - "@knobunc"
  - "@miciah"
  - "@candita"
  - "@rfredette"
approvers:
  - "@knobunc"
  - "@smarterclayton"
creation-date: 2020-08-17
last-updated: 2020-08-17
status: partially implemented
see-also:
  - "https://github.com/openshift/enhancements/pull/274"
replaces:
superseded-by:
---

# Ingress Fault Detection and Pinpointing

## Release Signoff Checklist

- [X] Enhancement is `implementable`
- [X] Design details are appropriately documented from clear requirements
- [X] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

This enhancement enriches the [Ingress Operator](https://github.com/openshift/cluster-ingress-opreator)
by granting the operator the ability to automatically detect when ingress-related issues occur within a cluster.
This enhancement also gives Cluster Admins the ability to pinpoint which specific part of the cluster Ingress "chain" is at fault when
external traffic bound for application pods is unable to reach (or experiences difficulties en-route to) the correct application destination.

## Motivation

Cluster Ingress issues can greatly and rapidly impact the health of an OpenShift cluster. Cluster Administrators should be alerted to
any potential ingress complications via the Ingress Operator as soon as they occur, as to help minimize unplanned application downtime.
OpenShift cluster administrators should not first find out about ingress breakages when they inspect the status of a different critical
cluster component, such as the console. A Cluster Administrator should also have tooling available to them to thoroughly diagnose and
understand why routes on their cluster are not working.

This enhancement also has benefits for internal OpenShift development.
Often times when networking-related bugs are identified within a cluster, determining which networking sub-component
(router, SDN, DNS, etc.) is responsible for the observed bug is difficult. Bug reports tend to spend lots of time bouncing between different teams
as engineers are not sure which component(s) is legitimately at fault. Being able to quickly pinpoint Ingress related issues will expedite the bugfix process.

### Goals

1. Teach the [Ingress Operator](https://github.com/openshift/cluster-ingress-operator) to automatically detect cluster Ingress issues
and alert cluster administrators when relevant problems are discovered.
2. (Post 4.7) Provide a secondary tool that can be triggered at will by a Cluster Administrator or an OpenShift support engineer that spawns an Ingress "debug"
container that quickly identifies and triages potential Ingress issues within the cluster.

### Non-Goals

1. Teach the Ingress Operator to "self-heal" when cluster Ingress errors or issues are observed.
2. Modify the [Ingress Controller API](https://github.com/openshift/api/blob/master/operator/v1/types_ingress.go)
to allow Cluster Administrators to disable the periodic Ingress Operator fault detector at will for a given Ingress Controller.

## Proposal

Once the Ingress Operator has successfully deployed the default [Ingress Controller](https://github.com/openshift/cluster-ingress-operator/blob/master/README.md)
within a cluster, the Ingress Operator will spawn a simple HTTP echo server via a canary daemonset. Currently in OCP 4.7, this echo server is the trivial
[Hello OpenShift](https://hub.docker.com/r/openshift/hello-openshift/) image. In future releases, the ingress operator binary will include an echo server sub-command
(ie `ingress-operator serve-healthcheck`) so that the ingress operator no longer relies on Hello OpenShift.
The Ingress Operator will also create the necessary Service and Route resources to provide Ingress to the echo container within the `openshift-ingress-canary`
namespace over HTTP and HTTP/2 connections. Note that a custom certificate will be needed for any HTTP/2 enabled Routes.
Then, the Ingress Operator will periodically attempt to send a request to the echo container(s), and await a proper response. These requests will be sent
such that they are routed outside of the cluster to the cloud load balancer first (when applicable), in order to ensure that each part of the Ingress "chain" is being tested.
If the Ingress Operator is unable to send requests to the echo container(s), the Ingress Operator fails to receive any responses
from the echo container(s), or the entire request and response flow takes an unusual amount of time, the default Ingress Controller will become degraded as long as sustained failures are observed.
The ingress canary container(s) will be reachable via a route resource that periodically rotates its target port. This will cycle the canary endpoints used by the
[OpenShift Router](https://github.com/openshift/router") so that the ingress operator can continuously ensure that the router has not become wedged or otherwise broken.
To accommodate this, the canary echo server (Hello OpenShift currently) will be enhanced to return a `x-request-port` response header.
This will allow the canary controller to verify that the test request was received on the correct container port via the router.

Note that this route-rotation behavior is [currently opt-in](https://github.com/openshift/cluster-ingress-operator/pull/525) for OCP 4.7 due to
the potential performance impact of frequent router reloads.

In addition to the automatic Ingress fault detection created by the Ingress Operator, a separate Ingress debugging tool will be available.
This tool will be invoked via the `oc` CLI tool, either as an `oc debug`, `oc adm debug`, or `oc adm diagnose` type command. Once invoked,
this tool will spawn a container that will assess and diagnose failures within each step of the Ingress "chain". This tool will attempt
to communicate with the cloud provider's Load Balancer (if applicable), the OpenShift Router, individual application pods, the Ingress Operator,
cluster DNS instances, and the OpenShift SDN components to try and pinpoint precisely where and why Ingress is failing. To accomplish this, the Ingress debug container
will include tools such as `curl`, `dig`, `ping`, `oc`, `nslookup`, etc. along with a series of scripts to run these tools and gather useful diagnosis data.

### User Stories

#### As an OpenShift cluster administrator or support engineer, I want the Ingress operator to tell me when Routes seem to be broken, so that I don't have to diagnose problems indirectly through failing downstream consumers (e.g. console, auth server)

To satisfy this story, automatic fault detection functionality will be added to the Ingress Operator. The Ingress Operator will alert cluster administrators and support engineers
via the Prometheus alerting UI.

#### As an OpenShift cluster administrator or support engineer, I want to be able to run a tool on-demand that tells me specifically why Routes seem to be broken, so that I do not have to spend time pinpointing network and Ingress failures manually

To satisfy this story, an Ingress debug container that diagnoses and triages Ingress and other network issues will be available to cluster admins and support engineers.

### Implementation Details/Notes/Constraints

Implementing this enhancement requires changes in the following places:

* `openshift/cluster-ingress-operator`
* `openshift/oc`
* `hello-openshift`
* `ART`

Proper Prometheus Alert rules will need to be created once the Ingress Operator exposes fault check metrics.
These rules should live in the Ingress Operator's repository, but will most likely need to be reviewed by a member of the OpenShift monitoring or OpenShift service delivery team
for transparency. Note that the current Prometheus Alert that fires when the Ingress Operator becomes degraded may be sufficient, since sustained failing canary checks would fire this alert
on behalf of the default ingress controller.

The Ingress Operator automatic fault detection code will always be enabled. Cluster administrators will not be able to turn off the Ingress background checks.
The thinking here was that the "knob" to disable this feature could be provided after this enhancement is released should it be deemed necessary based on customer input.

The `hello-openshift` container image has been added to the OCP core images referenced from the release image, so that it gets mirrored into disconnected environments.
In the future, `hello-openshift` may be removed from the release image and a sub-command of the ingress operator's binary may be used in its place.

The Ingress debug container may also need to be included in the OCP release image, similar to how the [must-gather](https://github.com/openshift/must-gather) tool is provided.
See https://issues.redhat.com/browse/NE-413 for more details about the debug container specifically (and how the container may or may not be pulled in a cluster).

### Risks and Mitigations

The Ingress debug container should only be accessible to cluster administrators or OpenShift support engineers, as to prevent unintended privilege escalations for normal cluster users.
The Ingress debug container should also not leak any cluster report data to unprivileged cluster users or unknown users external to the cluster.

## Design Details

### Open Questions

* How should the Ingress debug tool be invoked by cluster administrators? Current thinking is through some sub-command of `oc`. Is there a better mechanism for this?

### Test Plan

The Ingress canary controller will be tested via new e2e tests as a part of the ingress operator's e2e test suite. An e2e test will do the following:

1) Ensure that the canary route exists in the proper canary namespace.
2) Create a simple curl execpod to curl the canary route.
3) Curl the canary route and ensure that a 200 response is received from the echo server application.
4) Inspect the HTTP response body for any appropriate headers and body contents.
5) Ensure that successfully canary checks set a positive canary check status condition on the default ingress controller.


### Graduation Criteria

N/A

### Upgrade / Downgrade Strategy

On upgrade, the Ingress Operator will create the additional resources needed to perform the
periodic Ingress fault detection. The Ingress Operator will have the ability to update these resources
should they change between release versions.

On downgrade, the Ingress canary namespace created by the operator can be deleted. This will clean up all canary
resources created by the operator quickly.

The Ingress debug container is launched on-demand, so there are no upgrade concerns with that component.

### Version Skew Strategy

The Ingress debug container should be included in OCP release images.
The `oc` cli command to launch the Ingress debug container will use the debug container image included in the current release version.
If no debug container is available, then the `oc` invocation will gracefully exit.

## Implementation History

The monitoring team has attempted to create a [route monitoring tool](https://github.com/openshift/route-monitor-operator), that will be replaced by
native [Prometheus synethic probes](https://github.com/prometheus/blackbox_exporter) in a later version of OCP.

## Drawbacks

