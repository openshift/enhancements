---
title: interactive-installation-using-ABI-for-disconnected-environments
authors:
  - "@rwsu"
reviewers: # Include a comment about what domain expertise a reviewer is expected to bring and what area of the enhancement you expect them to focus on. For example: - "@networkguru, for networking aspects, please look at IP bootstrapping aspect"
  - "@andfasano"
  - "@bfournie"
  - "@pawanpinjarkar"
  - "@danielerez"
  - "@rawagner"
approvers: # A single approver is preferred, the role of the approver is to raise important questions, help ensure the enhancement receives reviews from all applicable areas/SMEs, and determine when consensus is achieved such that the EP can move forward to implementation.  Having multiple approvers makes it difficult to determine who is responsible for the actual approval.
  - "@zaneb"
api-approvers: # In case of new or modified APIs or API extensions (CRDs, aggregated apiservers, webhooks, finalizers). If there is no API change, use "None"
  - None
creation-date: 2025-09-16
last-updated: 2025-09-16
tracking-link: # link to the tracking ticket (for example: Jira Feature or Epic ticket) that corresponds to this enhancement
  - https://issues.redhat.com/browse/VIRTSTRAT-60
  - https://issues.redhat.com/browse/OCPSTRAT-1874
  - https://issues.redhat.com/browse/OCPSTRAT-1985
see-also:
  - "https://github.com/openshift/enhancements/pull/1821"
replaces:
  - "/enhancements/that-less-than-great-idea.md"
superseded-by:
  - "/enhancements/our-past-effort.md"
---

# Interactive installation using the agent-based installer for disconnected environments

## Summary

This enhancement proposes combining the Agent-based Installer, the Assisted 
Installer UI, and the OpenShift Appliance technologies to provide a streamlined,
user-friendly installation workflow, simplifying configuration, deployment,
and monitoring.

## Motivation

The installation process for OpenShift Virtualization Engine (OVE) is complex
for customers, especially in disconnected environments, due to managing an 
external registry and difficulties with operator configuration. The complexity,
particularly the lack of UI-driven workflows and reliance on YAML files, creates
a significant barrier for VMware administrators looking to transition to OpenShift
Virtualization.

### User Stories

* As a VMware administrator transition to OpenShift Virtualization, I
  want a UI-driven installation experience so that I can easily set up
  a cluster without having to write YAML files, aligning with my 
  existing operational practices.
  
* As an enterprise customer in a disconnected environment, I want to 
  install OpenShift Virtualization with minimal prerequisites and 
  without relying on a pre-existing external image registry, so 
  that I can install a cluster quickly with the least amount of prep
  work.
  
* As an OpenShift administrator, I want the essential OpenShift 
  Virtualization operators to be pre-configured and selectable during
  installation via a UI, so that I can ensure a fully functional
  virtualization environment upon deployment reducing a significant
  amount of additional operator configuration and installation during
  Day-1.

### Goals

* Provide a user-friendly, UI-driven installation experience without 
  requiring manual YAML file creation.
  
* Pre-configure essential operators for OVE, allowing users to select
  operators from a predefined list in the UI.
  
* Eliminate the dependency on a pre-existing external image registry
  for disconnected installations.

### Non-Goals

* The proposal does not describe how to upgrade the cluster or how
  additional nodes can be added to the cluster. These features
  are being developed in another enhancement proposal [Simplified cluster
  operations for disconnected environments](https://github.com/openshift/enhancements/pull/1821/)
  
* This proposal does not aim to included every possible operator in 
  the initial release. The full list of operators for OVE will be
  integrated iteratively.

* Securing the communication between the assisted-installer UI,
  assisted-service, and client browsers will be addressed in
  future updates.

## Proposal

The proposal is the build and distribute a generic live ISO
containing the [agent-based installer services](https://github.com/openshift/installer/blob/main/docs/user/agent/agent-services.md#interactive-workflow-unconfigured-ignition---interactive), the [assisted-installer
UI](https://github.com/openshift-assisted/assisted-installer-ui) 
and the images from the release payload that are required to 
install a cluster. The ISO is targetd for use in a disconnected 
environment. The OpenShift Appliance's [build live-iso](https://github.com/openshift/appliance/blob/master/docs/user-guide.md#live-iso) command 
is used to generate the live ISO. 

### Workflow Description

**cluster creator** is a human user responsible for deploying a cluster.

**OVE ISO** is an agent-based installer based live ISO containing all the images from
the release payload that are necessary to install the cluster and a set of preselected
operators.

**UI** is the assisted-installer UI used to configure, initiate, and monitor the cluster
installation. It is hosted on the rendezvous host.

**rendezvous host** is a host designated by the cluster creator using the agent-tui during
boot. The assisted-service and assisted-installer UI runs on this host. It also serves
as the bootstrap node.

**agent TUI** A terminal user interface that is displayed on all hosts during boot that allows
a user to set the rendezvous host IP. 

**console UI** is the installed cluster's UI.

1. The cluster creater downloads the OVE ISO from redhat.com.

2. The cluster creater mounts the ISO as virtual media on all hosts that will form the cluster.

3. One of the host is selected as the rendezvous node. The rendezvous node hosts
   assisted-service and the assisted-installer UI.

4. All nodes boot into the agent TUI. On the rendezvous host, the cluster creater
   designates it as such by setting that host's IP address as the rendezvous IP.
   On the remaining hosts, the cluster create enters the rendezvous host's IP so 
   that the assisted installer agents knows where to contact assisted-service to
   register itself.
   
5. When the rendezvous node completes booting, the URL to the UI is displayed on the console.
   
6. On a separate machine the cluster creater opens up the browser to the assisted-installer
   UI running on the rendezvous host.
   
7. The cluster creator begins configuring the cluster through the UI. In the cluster
   details screen the cluster creater enters the cluster name, base domain, pull secret,
   and topology.
   
8. On the operators selection screen, the cluster create checks the box to include
   virtualization operators.

9. The UI then displays all the hosts that have registered with assisted service 
   detailing their hardware inventory and status. A role may be selected for each host
   with the exception of the rendezvous host. Or it may be left as auto-assign.
   When the required number of roles have registered and validated to have the 
   required resources and network connectivity, the cluster creator can navigate
   to the next screen, networking.
   
10. On the networking screen, the cluster creator selects the desired method of 
    network management (Cluster-Managed or User-Managed) and enters the machine
	network, API IP, and ingress IP. A SSH public key may be entered to allow
	troubleshooting after installation. Assisted-service validates the inputs and
	displays warnings if there are any configuration issues.
	
11. If there are no configuration issues, the cluster creator moves to the download
    credentials screen and downloads files containing the kubeconfig and kubeadmin 
	password. 
	
12. Then the UI shows a confirmation screen showing the configuration that was
    entered and a list of operators that will be enabled. The cluster creator
	clicks on a button to begin the installation.
	
13. The UI shows the installation progress. When the installation reaches the point
    when the rendezvous host is rebooted and will join the cluster as a node, the
	UI will display the URL to the cluster's console UI.
	
14. The cluster creator then verifies the installation is complete by using the 
    console UI or the creator can use the kubeconfig that was downloaded and 
	the oc commands: "oc get clusterversion", "oc get co", and "oc get nodes"
	to verify the cluster and operators have finished installing.

### API Extensions

There are no changes to APIs.

### Topology Considerations

Will function identically in all topologies.

#### Hypershift / Hosted Control Planes

There are no specific considerations for this feature regarding
Hypershift / Hosted Control Planes.

#### Standalone Clusters

There are no specific considerations for this feature regarding 
Standalone Clusters.

#### Single-node Deployments or MicroShift

The same workflow applies to SNO. 

There is no specific considerations for this feature regarding
MicroShift.

### Implementation Details/Notes/Constraints

#### Live ISO

On the live ISO, all images required to install the cluster including the selected 
operators are baked into the ISO and are served by an internal registry on each host.
After the first reboot, the images are copied to the node's local container 
storage and a registry to serve the images from localhost is started.

Images used by the agent-based installer's services and for the assisted-installer
UI are also included in the live ISO. The images are fetched from the release payload
and embedded into the live ISO by the OpenShift Appliance "build live-iso" command.

### Risks and Mitigations

Communications between the agent running on all nodes and assisted-service running on
the redenzvous host are currently unsecured. Authentication and authorization will need
to be added in the future.

### Drawbacks

An ISO containing all images needed to install the cluster and the OVE operators 
is more than 40GB in size. Currently a limited number of operators are 
included in the ISO. If more operators are to be supported, the size of the ISO may
increase significantly. 

## Alternatives (Not Implemented)

Alternative approaches are discussed in the [Simplied cluster operations for
disconnected environments](https://github.com/openshift/enhancements/pull/1821) enhancement proposal.

## Test Plan

The entire installation flow from initial creation of the 
live ISO to initiating the install through the assisted-installer UI
and verifying the cluster and additional operators have been
installed will be included in an e2e test in OpenShift CI. 

[dev-scripts](https://github.com/openshift-metal3/dev-scripts) will
be enhanced to test this scenario.

## Graduation Criteria

### Dev Preview -> Tech Preview

- Ability to utilize the enhancement end to end
- End user documentation
- Sufficient test coverage (using a disconnected environment)
- Gather feedback from users rather than just developers
- Integration with the official Red Hat build and release pipeline

### Tech Preview -> GA

- Complete e2e testing for installation, upgrade, and node expansion
- User facing documentation created in [openshift-docs](https://github.com/openshift/openshift-docs/)

### Removing a deprecated feature

Not applicable.

## Upgrade / Downgrade Strategy

Support for upgrades is described in the [Simplified cluster operations for disconnected environments](https://github.com/openshift/enhancements/pull/1821) enhancement proposal. 

## Version Skew Strategy

Not applicable.

## Operational Aspects of API Extensions

Not applicable.

## Support Procedures

Not applicable.

