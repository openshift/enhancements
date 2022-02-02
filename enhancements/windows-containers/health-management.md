---
title: windows-node-health-management
authors:
  - "@sebsoto"
  - "@saifshaikh48"
reviewers:
  - "@openshift/openshift-team-windows-containers"
approvers:
  - "@aravindhp"
creation-date: 2021-12-16
last-updated: 2021-12-16
status: implementable
---

# Windows Node Health Management

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [x] Design details are appropriately documented from clear requirements
- [x] Test plan is defined
- [x] Operational readiness criteria is defined
- [x] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

The purpose of this enhancement is to ensure that all Windows nodes managed by the Windows Machine Config Operator 
(WMCO) maintain an expected state. With health management implemented, WMCO will automatically ensure that OpenShift 
related services running on the underlying Windows instances are configured, and have a state inline with the
expectations of the installed WMCO version. As the shift to [containerd](container-runtime-containerd.md) is imminent,
this enhancement is written considering containerd as the only supported container runtime.

## Motivation

Currently [WMCO](https://github.com/openshift/windows-machine-config-operator/) properly fulfills its goal of
configuring Windows instances into worker Nodes. Once the Node object exists, barring specific conditions, WMCO does
not have the functionality to detect and correct changes in Node state. Windows nodes should be resilient enough to not
require manual intervention when an issue occurs, whose cause is within the scope of WMCO's control. This work serves
as part of a larger initiative to minimize the downtime of customer workloads. There are other auxiliary benefits to
implementing this enhancement, for example, adding new Windows services becomes an easier process for WMCO developers,
and the node upgrade path from any WMCO version to another is codified through ConfigMap differences.

### Goals

* Maintain the `Ready` state of Windows Nodes to ensure Windows workloads can be run without interruption.
* Ensuring WMCO-managed Windows services on the instances associated with Windows Node objects are in the expected state
  and configured with the expected values:
  + Kubernetes components
  + Node-level networking components
  + Container runtime
  + Metrics collector/exporter
* Generate events to inform cluster administrators when Node health issues are not resolvable by WMCO.

### Non-Goals

* Diagnosing or remediating generic Windows issues, unrelated to changes made by WMCO.
* Cluster-level resource management, such as deleting or re-creating WMCO-configured Machines that enter an 
  unrecoverable state (this is a responsibility of [MachineHealthChecks](https://docs.openshift.com/container-platform/latest/machine_management/deploying-machine-health-checks.html)).


## Proposal

Currently, Windows Node configuration process is split between two components, WMCO and 
[WMCB](https://github.com/openshift/windows-machine-config-bootstrapper/). WMCB is a program which performs a one-shot 
configuration of a Windows instance, to ensure that it can become a worker Node.

To accomplish the goals listed in this enhancement, I am proposing that WMCB be converted into a daemon,
Windows Instance Config Daemon (WICD). WICD will have the following responsibilities:
* As part of the node `bootstrap` phase, WICD will configure the containerd runtime and start it as a Windows service
  based on its definition in the services ConfigMap.
* WICD will also maintain current responsibilities of starting kubelet as a service through the `bootstrap` command. 
  WICD will fully configure kubelet in one shot, as everything required will be available in the services ConfigMap.
* Using the configuration provided by a ConfigMap created by WMCO, WICD maintains the state of Windows Services on the 
  instance.
* WICD reverts all changes made to an instance when run with the `cleanup` command, and also deletes the Node
  object.

To enable this, WMCO will have these changes in responsibility:
* WMCO will no longer configure the Windows services on Windows instances, except for WICD.
* WMCO will take over the parsing of the worker ignition file. This is because, as the maintainer of the services
  ConfigMap, WMCO provide the expected state of the kubelet service to WICD. This requires information found within the
  ignition file. Note that WMCO will be able to provide all the information needed to fully configure kubelet right away
  since [CNI configuration is no longer provided to kubelet](https://github.com/kubernetes/kubernetes/pull/106907).
* WMCO will copy files obtained from parsing the worker ignition onto Windows instances as part of the payload.
  These files include the bootstrap kubeconfig and the client CA certificate needed to authenticate with the API server.
* WMCO will create and maintain a ConfigMap which provides WICD with the specifications for each Windows service that
  must be created on a Windows instance.
* WMCO will invoke the `bootstrap` command of WICD when initially adding a Windows instance to the cluster. 
  This will involve the services ConfigMap in order to start the containerd runtime and kubelet services.
* WMCO will invoke the `controller` command to run WICD as a Windows service on the instance.
* WMCO will invoke the `cleanup` command of WICD when removing an instance from the cluster. WMCO will no longer
  delete the Node object.

The Windows Node creation workflow for the end user will not be changed.

### User Stories

User stories can be found within the health management epic, [WINC-657](https://issues.redhat.com/browse/WINC-657).

### API Extensions

N/A

### Risks and Mitigations

* It is possible that issues with Windows instances will occur which WICD is unable to resolve, causing the Node to
  enter a `NotReady` state. For instances created through the machine-api, the user can set up MachineHealthChecks,
  which will correct the state by recreating the Machine. For BYOH instances, user intervention is required at this
  point. Events and alerts will ensure that a cluster administrator is made aware of the issue.
* There is a possibility that a cluster admin could mistakenly remove the services ConfigMap for an older WMCO version,
  before Node upgrades are completed. This would result in WICD not knowing what services it has configured on the
  instance. In order to prevent this, WICD can set the Windows service description to contain "OpenShift managed", so
  that it can search for services which were installed by WICD. As it is potentially possible for a user to remove this
  tag from the services, when the expected service ConfigMap is missing, both the tag and the latest service ConfigMap
  should be used when deconfiguring the node. Using a combination of these two will allow for all the installed
  services to be removed, in most cases. Additionally, in order to facilitate upgrades from a version without
  WICD, this tag should be added to all existing services in 4.10, one major version ahead of WICD's target release.
* If the ConfigMap controller of WMCO is busy configuring BYOH instances, the controller will not be able to react to
  events regarding the services ConfigMap, until it is no longer busy. There can be a period of time in which the
  services ConfigMap could have incorrect contents, which could then be picked up by WICD running on various instances.
  To reduce the risk of this, the `MaxConcurrentReconciles` option can be provided when initalizing the ConfigMap
  controller. This will enable the controller to process service ConfigMap events that occur while WMCO is handling
  its BYOH configuration responsibilities.
* WMCO will not default to containerd as the container runtime until the 6.0.0 timeframe. Therefore, we will be using a 
  runtime flag `dockerRuntime=false` introduced by the containerd enhancement to develop during the 5.y.z cycle.
  This flag will be dropped in WMCO 6.0.0.

## Design Details

### Services ConfigMap

WMCO will create and maintain an immutable ConfigMap with the naming scheme:
`services-<MajorVersion>-<MinorVersion>-<PatchVersion>-<Commit>`. For example, if WMCO was built from commit a7b5
of WMCO version 5.0.0, the created ConfigMap would be named `services-5-0-0-a7b5`.

The purpose of this ConfigMap is to provide WICD with the specifications required to configure a Node. The ConfigMap
will be watched via the existing ConfigMap controller. WMCO is the owner of the ConfigMap and the source of the data
present within it. If the ConfigMap is deleted, WMCO will re-create it.

If an entity attempts to create a services ConfigMap with incorrect values, WMCO will delete it.
This will resolve a scenerio in which the ConfigMap controller is busy configuring Windows instances, and a user
deletes and recreates the service ConfigMap with incorrect values, before the controller has a chance to re-create
the ConfigMap.

The services ConfigMap contains two keys: `services`, and `files`. The values for both keys are JSON objects.
The `services` key contains all data required to configure the required Windows services on any instance that is to be
added to the cluster as a Node. The proposed schema is as follows:
```json
[
  {
    "name": "name of the Windows service",
    "command": "command that will be executed. This could potentially include strings whose value will be derived from nodeVariablesinCommand and powershellVariablesinCommand.",
    "dependencies": [
      "name of a service that this service is dependent on"
    ],
    "nodeVariablesinCommand": {
      "name": "string within command field that will be substituted",
      "jsonPathNodeObject": "The jsonPath of a field within the instance's Node object"
    },
    "powershellVariablesinCommand": {
      "name": "string within command field that will be substituted",
      "path": "location of the PowerShell script to be run, in order to get its output"
    },
    "bootstrap": "boolean flag indicating whether this service should be handled as part of node bootstrapping",
    "priority": "non-negative integer that will be used to order the creation of the services, priority 0 is created first"
  }
]
```

The nodeVariablesinCommand and powershellVariablesinCommand fields will be used by WICD when processing the
ConfigMap. They are needed as some Windows services have arguments that require values sourced from either the
Node object associated with the instance, or data from the instance itself. These fields allow for a ConfigMap that is
generic enough to apply to all Windows instances being configured, by providing WICD instructions for how to retrieve
certain values. For these fields, a retry period will be necessary for when the information is not available yet. For
example, when waiting for an annotation to be applied to the Node object. This wait will be built into WICD, and should
be long enough to account for normal scenarios. If for some reason there is a delay in an annotation application, the
reconciliation would fail, the issue would be resolved when the next reconciliation occurs.

When making use of a PowerShell script, retries should be present within the script, if necessary. An example of this
is waiting for the HNS network to be created in order to create an HNS endpoint. In that specific case, the PowerShell
script which creates the endpoint should include a retry period which waits for the needed Network to exist.

Services marked with `bootstrap: true` will start before any others. Bootstrap services cannot depend on a non-bootstrap
service; WICD will throw an error if this is detected. Similarly, each service that has the bootstrap flag set as true
must have a higher priority than all non-bootstrap services. WICD will throw an error if this is not the case.
There should be no overlap in the priorities of bootstrap services and controller services.

For example, if the service ConfigMap had an entry with the following data:
```json
{
  "name": "new-service",
  "command": "C:\new-service --variable-arg1=NODE_NAME --variable-arg2=NETWORK_IP",
  "nodeVariablesinCommand": {
    "name": "NODE_NAME",
    "jsonPathNodeObject": "metadata.name"
  },
  "powershellVariablesinCommand": {
    "name": "NETWORK_IP",
    "path": "C:\k\scripts\get_net_ip.ps"
  },
  "dependencies": [],
  "bootstrap": false,
  "priority": 2
}
```

WICD would know to create the service named `new-service` with the value of the --variable-arg1 argument set to the
name of the instance's Node, and the output of the PowerShell script located at `C:\k\scripts\get_net_ip.ps`, as the
value of the argument --variable-arg2. WICD will be aware of the proper Node object, as it searching for a Node with an
internal IP equivalent to the IP of the instance.
This service is not marked as a bootstrap service, so it will be started by WICD when run with the `controller` command.
It should be created only after all bootstrap services and those with priorites 0 and 1 have been created.

The `files` key contains the path and checksum of files copied to the instance by WMCO. This will be used by WICD to
validate that essential files have not been tampered with.
```json
{
  "path": "filepath",
  "checksum": "checksum of file, generated at compile time"
}
```

### Archiving windows-machine-config-bootstrapper repository

The WICD source code will be added to the WMCO repo. Putting the WICD code into the WMCO repo has the major benefit
of allowing us to test and merge changes to WMCO and WICD within the same PR. Once all WMCO versions which make use of
WMCB go out of support, the WMCB repo will be archived.

### Node configuration: WMCO responsibilities

1) For a given Machine or BYOH instance, WMCO copies the payload to the instance.
2) WMCO runs WICD with the bootstrap command, this results in the node object being created.
3) WMCO annotates the Node object with the `desiredVersion` annotation, informing WICD of which ConfigMap to use to
   configure the instance. If this annotation is later changed to an incorrect value, WMCO's node controller will
   revert it.
4) The value of this is set to the current WMCO version.
5) WMCO starts WICD as a service.
6) Reconciliation can successfully return at this point.

### Node configuration: WICD responsibilities

For Node configuration WICD will have two separate responsibilities:

#### Bootstrap command

When run with the `bootstrap` command, WICD will do the steps required to ensure that Node is created for the instance. 
The `bootstrap` command will start all services that have a `bootstrap` value of true, and exclusively these services.

The `bootstrap` command will have the responsibility of starting the containerd service. This is becuase the kubelet
service has a dependency on the container runtime; the container runtime must have a reachable endpoint
(i.e. `\\.\pipe\containerd-containerd` is open and usable) when kubelet is initialized (with the `container-runtime`
and `container-runtime-endpoint` parameters supplied). The containerd service will be configured based on the
information in the services ConfigMap, a priority 0 service with no dependencies that points to the location of
CNI plug-ins on the instance. Although CNI config is populated later in the `bootstrap` phase after hybrid-overlay runs,
pointing the containerd config to this location is enough to ensure that the service picks up networking config changes
without erroring out on start-up or requiring a restart.

The rest of the functionality is reading the kubelet configuration from the service ConfigMap and starting the kubelet 
service. A key note is that, since networking configuration is now the container runtime's responsibility, kubelet can 
be fully configured upon service creation, no longer requiring a separate intitialization stage then a later restart.

WICD will wait for the Node to be created, and cordon it, as the Node is not ready for workloads at this point.

#### Running as a Windows service

WICD will fullfil its responsibilities as a Windows service controller when run with the `controller` command.
When this command is run with the flag `--windows-service`, WICD will be managed by the Windows Service Manager.

WICD will have an informer for both ConfigMap and Node events. The desired version annotation set on the Node object
by WMCO directs WICD to the proper services ConfigMap to use to configure the instance. When it is detected that either
the desired version annotation has been set for the Node, or the ConfigMap specified by the desired version annotation
has been created, WICD will configure the Windows services to the specifications listed in the existing ConfigMap. If
an error occurs, an event will be generated against the Node object. When reconciliation completes sucessfully, the
`windowsmachineconfig.openshift.io/version` annotation will be set to the value given by the desired version
annotation. This indicates that the Node was configured according to the specifications given by that version's WMCO.

On a separate thread, WICD will be periodically polling the state of Windows services. If there is any change in the
expected state, the services will be created or modified in order to restore expected state. Since the Node/ConfigMap
controller could be reading from and modifying Windows service state, while these periodic state checks are made,
appropriate thread safety procedures must be followed.


### BYOH Node de-configuration: WMCO responsibilies

When a user indicates to WMCO that it wishes to remove a BYOH node from the cluster, the following procedure will be
kicked off.

1) WMCO stops the WICD service on the instance.
2) WMCO runs the cleanup command of WICD.
3) WMCO removes the files it copied to the instance.

Machine Nodes do not go through this process, as the Machine can be deleted and the Machine controller will drain
and remove the Node.

### BYOH Node de-configuration: WICD responsibilities

When the WICD cleanup command is executed by WMCO, all services listed in the ConfigMap will be stopped and removed.
It will then delete the instance's Node object.

### Test Plan

Most functionality described within this enhancement is already tested as part of the extensive end-to-end
tests in the WMCO repo. As the different commands of WICD are developed, they can be integrated into WMCO, and
tested through the existing tests. New tests will be added to test functionality that wasn't already present, such
as validating the state of the services ConfigMap, and ensuring that Windows service state is reverted after it
is tampered with.

### Graduation Criteria

This enhancement is targeted for OpenShift 4.11/ WMCO version 6.0.0.

#### Dev Preview -> Tech Preview

The features added here can be previewed in the Community 5.y.z release.

#### Tech Preview -> GA

As the functionality described in this enhancement will be integrated into WMCO, the normal release process
of WMCO will be followed. That being a community operator release for early preview, followed by the official Red Hat
operator release.

#### Removing a deprecated feature

N/A, as the changes made in the enhancement should be invisible to the user.

### Upgrade / Downgrade Strategy

When WMCO is upgraded, it will create the new Windows service ConfigMap, and then for each existing Windows Node:
1) Stop the WICD service
2) Run the WICD `cleanup` command with the `--upgrade` flag to stop and remove all services without removing the Node
   object.
2) Copy over all new and modified files in the payload, including WICD.
3) Update the desired version annotation on the Node, to the current WMCO version. This should be done one Node at a
   time, in order to ensure that workloads can continue to run with minimal interruption. WMCO is okay to change a
   Node's desired version annotation if: there are zero Nodes with a mismatched desired and current version; there
   are zero NotReady Nodes which have a current version matching WMCO's version.
4) Start the WICD service.

WICD, triggered by a mismatch between desired node version and current node version labels will correct the state
by using the ConfigMap of the version it is upgrading to, in order to configure the instance normally. As part of this
process, the Node version label is patched from the previous version to the desired version.

There should be no difference in the upgrade process between Machine and BYOH instances.

Downgrades are not supported through OLM, and thus are not supported by WMCO.

### Version Skew Strategy

This enhancement has the potential to reduce issues related to having Windows Nodes which were configured using an
older, now uninstalled WMCO version, on a more recent OpenShift cluster. When a compatible WMCO version is installed,
the services ConfigMap provides a way for WICD to upgrade a Node cleanly from one version to another. This is true when
upgrading only from WMCO 5.0.0 onwards, as WICD will have no way to cleanly upgrade without either a service ConfigMap
from the previous version, or the `OpenShift managed` label on the Windows services themselves. There should be no
Windows Nodes configured with WMCO versions below 5.0.0, as WMCO does not support skipping a major version when
upgrading.

### Operational Aspects of API Extensions

N/A

#### Failure Modes

This remains the same, on failure, the state of Windows nodes will not be managed properly. With improved
event generation problems will be easier to debug.

#### Support Procedures

In general the support procedures for WMCO will remain the same. Support will be further enabled by having
the additional information given by the Node events generated by WICD.
The WICD log collection will be added to the must-gather script.

## Implementation History

v1: Initial Proposal

## Drawbacks

The only drawback to this is that complexity increases by distributing responsibilities between WMCO and WICD.
This is not enough to outweigh the benefits of the proposal.

## Alternatives

Instead of using a ConfigMap, a CRD can be added. It did not feel like there was enough reasoning to require a CRD,
but that remains an alternative in the future if desired.

