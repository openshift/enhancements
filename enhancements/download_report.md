---
title: Download must-gather diagnostics - server side
authors:
  - "@masayag"
  - "@nmagnezi"
  - "@pkliczewski"
reviewers:
  - "@deads2k"
  - "@derekwaynecarr"
  - "@soltysh"
  - "@mfojtik"
  - "@sanchezl"
approvers:
  - "@deads2k"
  - "@derekwaynecarr"
creation-date: 2019-09-18
last-updated: 2019-09-20
status: implementable
see-also:
replaces:
superseded-by:
---

# Download must-gather diagnostics - server side

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift/docs]

## Summary

Once cluster start to fail a customer needs to figure out how to collect diagnostic information to understand what is the problem. At the moment we provide the ability to get the data by running `oc adm must-gather` command line. In order to ease collection process there are UI changes planned in `4.3 Brief: Serviceability: download diagnostics` which provide a way to get the data as designed [here](https://github.com/bmignano/openshift-origin-design/blob/65359e2ac5e751020c1d061c87f3ba1efee6fabd/web-console/future-openshift/diagnostics/diagnostics.md).

## Motivation

As part of proposed changes we need to provide a mechanism to request generation of must-gather report on demand and expose generated files via http. Having this changes would allow console developers to implement download diagnostics dialog. With this work we are not going to provide ability to filter data by applicable component but we are aiming to reuse existing functionality based on must-gather collection images for specific layered products (i.e. [kubevirt/must-gather](https://github.com/kubevirt/must-gather), [OCS must-gather](https://github.com/openshift/ocs-operator/tree/master/must-gather).

### Goals

As part of this work we want to provide api for the UI to call to trigger must-gather report generation on demand. We want to reuse existing must-gather functionality to generate the report and store it on one of the nodes in the cluster. We want to run http server with a service and route on the same node and expose generated file so the console can provide the file to the user. We need to make sure that only cluster admin is able to access the file so we will use sidecar container oauth-proxy.

### Non-Goals

Out of the scope for this work is filtering collected data by the component or object status (like operator degraded status).

## Proposal

In order to provide required interface we need to create new operator and introduce new custom resource definition which would provide a one or more collection containers to run. Here is the example of custom resource:

```yaml
apiVersion: apiextensions.k8s.io/v1alpha1
kind: MustGatherReport
metadata:
  name: example-mustgatherreport
  namespace: example-ns
spec:
  images:
  - quay.io/kubevirt/must-gather
  - quay.io/ocs-dev/ocs-must-gather
  - quay.io/openshift/origin-must-gather
  deleteAfter: 30m
# once report available route is added
status:
  report_url: https://example-mustgatherreport-openshift-must-gather-tdv78.router.default.svc.cluster.local/must-gather.tar.gz
conditions:
  - status: "True"
    type: Initialized
  - status: "True"
    lastTransitionTime: "2019-09-16T12:02:39Z"
    type: Ready
```
images - describes which images to use for running must-gather

deleteAfter - the duration to keep the generated reports

Conditions:
 * Initialized - indicates diagnostic information collection started
 * Ready - indicates diagnostic information collection completed, file compressed and ready to be downloaded


New controller watches for events on MustGatherReport custom resource. Once a resource is created the controller creates new must-gather namespace and a pod which uses list of container images to gather diagnostic data. We have 3 options to persist the files:
* must-gather and http server are part of a single pod, the files are persisted to a single RW PV. With this approach we are unable to scale and provide HA
* must-gather and http server are running in separate pods that use a hostpath on a pet node and can do many RW. This is similar to current must-gather flow triggered by oc command. As in bullet one we are unable to scale and HA
* must-gather and http server are part of separate pods that use a RW many storage. In this case we have no issues with scaling scale, no need for a pet nodes and parallel collection possible. With this approach we are dependent on OCS.

At this time we have decided to go with #1 and for now we won’t be able to scale nor to provide HA.

We need to add post-generation step to compress the files. Report is generated and compressed we create a service and a route pointing to http server. The controller updates custom resource status with URL to access generated file.

### Implementation Details

The controller can reuse most of the [logic](https://github.com/openshift/oc/blob/9ff96feb1aea1217938e2f1aeaf0be091cc59728/pkg/cli/admin/mustgather/mustgather.go#L223) run by oc command and persist the files in one of the ways described above. We can allow UI to cleanup the space be removing custom resource or controller can wait for specified amount of time before cleanup.

If we decide to choose option #2 we would need to make sure that http container is running on the same node. Current implementation of must-gather depends on a pet node.

The flow needs to be divided into 3 steps. We need to reuse existing code base to collect and persist the files. Next as post-generation step we need to compress the files. Finally once the  generated file is available we need to create service, route and update custom resource for UI to consume.

Running must-gather from command line requires cluster admin permission and we need to make sure that only this role can trigger report generation and fetch the file. In order to protect the http endpoint we need to use [oauth-proxy](https://github.com/openshift/oauth-proxy) which allow us to chech identity and permissions of the user.

### Risks and Mitigations

There is a risk of consuming a lot of disk space if many reports are pending deletion.

## Design Details

### Test Plan

We plan to have the repositories associated with this effort fully integrated with CI and the server side functionality will be tested together with the UI dialog with end to end tests.

### Graduation Criteria

This enhancement will start as GA

### Upgrade / Downgrade Strategy

We will support upgrades managed by CVO of the operator by publishing a new release. An older release can be used to downgrade.

## Implementation History

Version 4.3
