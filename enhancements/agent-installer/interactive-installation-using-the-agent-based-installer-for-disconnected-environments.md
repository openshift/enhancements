---
title: interactive-installation-using-the-agent-based-installer-for-disconnected-environments
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
  - 
superseded-by:
  -
---

# Interactive Installation using the Agent Based Installer for Disconnected Environments

## Summary

This enhancement proposes the creation of a streamlined, interactive, and
UI-driven installation workflow leveraging the Agent Based Installer and the
Assisted Installer service targeted for disconnected environments. The goal
is to address the complexity of the current installation process, which can
be challenging for users who are not accustomed to command-line tools and
manual configuration files. By providing an opinionated, user-friendly UI,
this initiative aims to simplify cluster setup and reduce the potential for
errors.

## Motivation

The installation process for a new cluster can be complex, involving multiple
prerequisites and manual configuration steps that are challenging and
time-consuming for some users. Customers, especially those accustomed to
graphical interfaces, often struggle with writing YAML files and ensuring they
are configured correctly. This proposal seeks to deliver an improved user
experience by abstracting away this complexity behind a simple, guided UI.

### User Stories

* As an administrator, I want a simple, UI-driven workflow to install a
cluster, so I can easily set up a new environment without needing to write
or manage complex YAML configuration files.
  
* As an administrator deploying a cluster, I want to configure all necessary
settings, such as networking and topology interactively through a UI, so that
I can avoid common configuration errors and reduce setup time.
  
* As an administrator, I want the installer to help me pre-configure and
install essential operators during cluster setup, so the cluster is ready for
my workloads immediately after installation.

### Goals

* Provide a user-friendly, UI-driven installation experience in a disconnected
environment that leverages the existing Agent Based Installer and Assisted
Installer technologies.
  
* Simplify the installation process to reduce complexity, especially for users
who prefer graphical interfaces.
  
* Allow for the selection and pre-configuration of essential operators as part
of the installation workflow.

### Non-Goals

* This proposal does not aim to replace existing installation methods but to
provide a simplified alternative for specific user personas.
  
* Post-installation cluster management and day-2 operations are out of scope
for this enhancement.

* Securing the communication between the Assisted Installer UI, Assisted
Service, and client browsers will be addressed in future updates.

## Proposal

This enhancement proposes extending the functionality of the Agent Based
Installer to support a fully interactive, UI-based installation flow. The
core of this proposal is to create a guided workflow within the UI that
captures all necessary configuration details from the user, which are then
used by Assisted Service to orchestrate the cluster deployment.

The UI will be deployed on the rendezvous host. The UI will inherit some of
the interface used by the cloud-based Assisted Installer. The UI will collect
crucial information to configure the cluster. The UI will provide host
verification to ensure the hardware inventory fulfill the cluster topology.
It will also check network connectivity between the hosts. The workflow will
also include a mechanism to select and bundle common operators, ensuring they
are installed and configured alongside the cluster itself.

### Workflow Description

**cluster creator** is a human user responsible for deploying a cluster.

**UI** is the assisted-installer UI used to configure, initiate, and monitor
the cluster installation. It is hosted on the rendezvous host.

**rendezvous host** is a host designated by the cluster creator using the
agent-tui during boot. The assisted-service and assisted-installer UI runs on
this host. It also serves as the bootstrap node.

**agent TUI** A terminal user interface that is displayed on all hosts during
boot that allows a user to set the rendezvous host IP. 

**console UI** is the installed cluster's UI.

1. The cluster creater begins by using the installer tooling to generate a
bootable ISO.

2. The cluster creater boots the ISO on target cluster hosts.

3. The cluster creator chooses one of the hosts to be the rendezvous host.

4. All hosts boot into the agent TUI. On the rendezvous host, the cluster creater
designates it as such by setting that host's IP address as the rendezvous IP.
On the remaining hosts, the cluster create enters the rendezvous host's IP so
that the assisted installer agents knows where to contact assisted-service to
register itself.
   
5. When the rendezvous host completes booting, the URL to the UI is displayed
on the console.
   
6. On a separate machine the cluster creater opens up the browser to the
assisted-installer UI running on the rendezvous host.
   
7. The cluster creator begins configuring the cluster through the UI. The
cluster creator is guided through a series of steps to configure the cluster
starting with cluster name, base domain, and pull secret. Optional operators
may be selected to be included in the installation.

8. The UI then displays all the hosts that have registered with assisted service
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
	
13. The UI shows the installation progress. When the installation reaches the
point when the rendezvous host is rebooted and will join the cluster as a node,
the UI will display the URL to the cluster's console UI.
	
14. The cluster creator then verifies the installation is complete by using the
console UI or the creator can use the kubeconfig that was downloaded and the
oc commands: "oc get clusterversion", "oc get co", and "oc get nodes"	to verify
the cluster and operators have finished installing.

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

No additional details.

### Risks and Mitigations

Communications between the agent running on all nodes and assisted-service
running on the redenzvous host are currently unsecured. Authentication and
authorization will need to be added in the future.

### Drawbacks

None.

## Alternatives (Not Implemented)

None.

## Test Plan

The entire installation flow from creation of the boot ISO to initiating the
install through the assisted-installer UI and verifying the cluster and
additional operators have been installed will be included in a e2e test in
OpenShift CI.

[dev-scripts](https://github.com/openshift-metal3/dev-scripts) will be
enhanced to test this scenario.

## Graduation Criteria

### Dev Preview -> Tech Preview

- Ability to utilize the enhancement end to end
- End user documentation
- Sufficient test coverage
- Gather feedback from users rather than just developers
- Integration with the official Red Hat build and release pipeline

### Tech Preview -> GA

- Complete e2e testing for installation.
- User facing documentation created in [openshift-docs](https://github.com/openshift/openshift-docs/)

### Removing a deprecated feature

Not applicable.

## Upgrade / Downgrade Strategy

Not applicable.

## Version Skew Strategy

Not applicable.

## Operational Aspects of API Extensions

Not applicable.

## Support Procedures

Not applicable.

