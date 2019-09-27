---
title: inspect
authors:

- "@sanchezl"
- "@deads2k"
  reviewers:
- "@deads2k"
- "@derekwaynecarr"
- "@soltysh"
- "@mfojtik"
  approvers:
- "@deads2k"
  creation-date: 2019-09-21
  last-updated: 2019-09-25
  status: provisional
  see-also:
  replaces:
  superseded-by:

---

# inspect

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift/docs]

## Summary

When examining a cluster resource for debugging purposes, there are usually many other resources that need to also be gathered to provide context. The `inspect` command has semantic understand of some resources and traverses links to get interesting information beyond the original resource. 

## Motivation

When gathering information for debugging purposes, you issue `get` commands to retrieve resources, but then you must also know what other resources are needed for context. The `inspect` command encapsulats the knowlege of what other resources provide context for the interested resource and enable a "super" `get` ability to easily gather them all at once.

### Goals

* A single command should gather a specified resource and all the other resources needed to inspect that resource.

### Non-Goals

### Proposal

`oc adm inspect` is a noteworthy command because of the way that it traverses and gathers information.  Intead of being 
truly generic, it has a generic fallback, but it understands many resources so that you can express an intent like, 
"look at this cluster operator".  

`oc adm inspect clusteroperator/kube-apiserver` does...

1. Queue the resource
2. Get and dump the resource (clusteroperator)
3. Check against a well-known list of resources to do custom logic for
4. If custom logic is found, queue more resources to iterate through
5. Perform the custom logic.

There are several special cases today.

1. clusteroperators 
   1. get all config.openshift.io resources
   2. queue all clusteroperator's related resources under `.status.relatedObjects` 
2. namespaces
   1. queue everything in the `all` API category
   2. queue secrets, configmaps, events, and PVCs (these are standard, core kube resources)
3. routes
   1. elide secret content from routes
4. secrets
   1. elide secret content from secrets.  Some keys are known to be non-secret though (ca.crt or tls.crt, for instance)
5. pods
   1. gets all current and previous container logs
   2. take a best guess to find a metrics endpoint
   3. take a best guess to find a healthz endpoint and all sub-healthz endpoints 

#### --size

`oc adm inspect --size=[small,medium,large,<number>Mi` The precise size matches the resourcequotas parsing rules.
The sizes are subject to change, but initially
1. small=10Mi
2. medium=100Mi
3. large=1Gi
The default is no size restriction.

The size of inspected assets can get quite large on systems that have been running for a while.
Because the content is varied (logs, resources, metrics, etc), trying to tune each type of collection individually doesn't make sense.
Instead we will support an intent based size that inspect tries to stay under the size limit.

Exactly how we restrict this size is based on an unconfigurable heuristic that is subject to change.
This doc will try to stay up to date with how the heuristic works, with least important information listed first.

1. Logs past the most recent 10000 lines.
2. ServiceAccount secret CAs past the default SA (to ensure we always get one).
3. healthz
4. per-binary metrics
5. Healthy logs past the most recent 1000 lines.
6. events more than two hours old.
7. All logs past the most recent 1000 lines.

If we have extra space, keep more of the available logs.
When trimming logs, prefer to trim logs for healthy pods more than unhealthy pods.  
*Happy pods are all alike, but every unhappy pod is unhappy in its own way.*

### User Stories [optional]

#### Story 1

#### Story 2

### Implementation Details/Notes

### Constraints

Attempt to gather the `/healthz`, `/version`, and `/metrics` endpoints from pod containers will only try to use HTTPS on the first container port defined.

### Risks and Mitigations

## Design Details

### Output Format

```
├── cluster-scoped-resources
│   ├── <API_GROUP_NAME>
│   │   ├── <API_RESOURCE_PLURAL>.yaml
│   │   └── <API_RESOURCE_PLURAL>
│   │       └── individually referenced resources here
│   ├── config.openshift.io
│   │   ├── authentications.yaml
│   │   ├── apiservers.yaml
│   │   ├── builds.yaml
│   │   ├── clusteroperators.yaml
│   │   ├── clusterversions.yaml
│   │   ├── consoles.yaml
│   │   ├── dnses.yaml
│   │   ├── featuregates.yaml
│   │   ├── images.yaml
│   │   ├── infrastructures.yaml
│   │   ├── ingresses.yaml
│   │   ├── networks.yaml
│   │   ├── oauths.yaml
│   │   ├── projects.yaml
│   │   ├── schedulers.yaml
│   │   └── support.yaml
│   ├── core
│   │   └── nodes
│   ├── machineconfiguration.openshift.io
│   │   ├── machineconfigpools
│   │   └── machineconfigs
│   ├── network.openshift.io
│   │   ├── clusternetworks
│   │   └── hostsubnets
│   ├── oauth.openshift.io
│   │   └── oauthclients
│   ├── operator.openshift.io
│   │   ├── authentications
│   │   ├── consoles
│   │   ├── kubeapiservers
│   │   ├── kubecontrollermanagers
│   │   ├── kubeschedulers
│   │   ├── openshiftapiservers
│   │   ├── openshiftcontrollermanagers
│   │   ├── servicecas
│   │   └── servicecatalogcontrollermanagers
│   ├── rbac.authorization.k8s.io
│   │   ├── clusterrolebindings
│   │   └── clusterroles
│   ├── samples.operator.openshift.io
│   │   └── configs
│   └── storage.k8s.io
│       └── storageclasses
├── host_service_logs
│   └── masters
│       ├── crio_service.log
│       └── kubelet_service.log
└── namespaces
    ├── <NAMESPACE>
    │   ├── <API_GROUP_NAME>
    │   |   ├── <API_RESOURCE_PLURAL>.yaml
    │   |   └── <API_RESOURCE_PLURAL>
    │   |       └── individually referenced resources here
    │   └── pods
    │       └── <POD_NAME>
    │           ├── <POD_NAME>.yaml
    │           └── <CONTAINER_NAME>
    │               └── <CONTAINER_NAME>
    │                   ├── healthz
    │                   |   └── <SUB_HEALTH>
    │                   ├── logs
    │                   |   ├── current.log
    │                   |   └── previous.log
    │                   └── metrics.json
    ├── default
    │   ├── apps
    │   │   ├── daemonsets.yaml
    │   │   ├── deployments.yaml
    │   │   ├── replicasets.yaml
    │   │   └── statefulsets.yaml
    │   ├── apps.openshift.io
    │   │   └── deploymentconfigs.yaml
    │   ├── autoscaling
    │   │   └── horizontalpodautoscalers.yaml
    │   ├── batch
    │   │   ├── cronjobs.yaml
    │   │   └── jobs.yaml
    │   ├── build.openshift.io
    │   │   ├── buildconfigs.yaml
    │   │   └── builds.yaml
    │   ├── core
    │   │   ├── configmaps.yaml
    │   │   ├── events.yaml
    │   │   ├── pods.yaml
    │   │   ├── replicationcontrollers.yaml
    │   │   ├── secrets.yaml
    │   │   └── services.yaml
    │   ├── default.yaml
    │   ├── image.openshift.io
    │   │   └── imagestreams.yaml
    │   └── route.openshift.io
    │       └── routes.yaml
...
`
### Test Plan

There is an e2e test that makes sure the command always exits successfully and that certain apsects of the content
are always present.

### Graduation Criteria

### Upgrade / Downgrade Strategy

N/A

### Version Skew Strategy

The `inspect` command must skew +/- one like normal commands.

## Implementation History

## Drawbacks

## Alternatives

## Infrastructure Needed [optional]

[1]: https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.10/#container-v1-core
