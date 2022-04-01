---
title: development-environment
authors:
  - "@lranjbar"
reviewers:
  - "@andfasano"
  - "@bfournie"
  - "@hardys"
  - "@pawanpinjarkar"
  - "@rwsu"
  - "@zaneb"
approvers:
  - "@dhellmann"
  - "@celebdor"
api-approvers: # in case of new or modified APIs or API extensions (CRD, aggregated api servers, webhooks, finalizers)
  - N/A
creation-date: 2022-02-10
last-updated: 2022-04-01
tracking-link: # link to the tracking ticket (for example: Jira Feature or Epic ticket) that corresponds to this enhancement
  - https://issues.redhat.com/browse/AGENT-71
---

# Ephemeral Agent Installer Development Infrastructure

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

Create a development environment for the ephemeral agent based installer image in the 
[dev-scripts](https://github.com/openshift-metal3/dev-scripts) repository and refactor 
components as needed for the new project. In addition we will add base case e2e test flows
integrated into Openshift CI to get the project started.

## Motivation

The new agent based installer project will need a standard development environment for
the team to use. We will need an e2e testing framework for validation in Openshift CI. 
In this enhancement we will explain our desire to reuse existing frameworks for the new 
project while limiting the scope of modifications to them to the new project's requirements.

### Goals

* Create a development environment for the agent installer project that can be used for local
development and for automating the flows in Openshift CI.
* Create predefined configurations to deploy Openshift in HA, compact and SNO topologies.
* Create predefined network configurations for Openshift including but not limited IP type 
(IPv4, IPv6, dual-stack).
* A reusable and reproducible environment configuration with predefined flows for development 
and testing teams to use.
* Convergence of the underlying scripts and environment setup code between Metal Platform (IPI) 
team and the new Agent Installer teams.

### Non-Goals

* Re-factoring dev-scripts and assisted-test-infra for reasons not related to the new 
ephemeral agent based installer image project.
* Overall convergence between all the installer development teams of scripts and e2e test flows.

## Proposal

Create an development environment for the ephemeral agent based installer image in the 
[dev-scripts](https://github.com/openshift-metal3/dev-scripts) repository and refactor 
components as needed for the new project.

### User Stories

* As a developer, I need to set up an environment so that I can install an Openshift
cluster locally using the agent-based installer for development and testing

* As a developer, I need to launch the built ephemeral agent based installer image so that 
I can test and validate this image during development.

* As a developer, I want my environment configuration to be reusable and reproducible so that 
I can share it with my colleagues so that we can easily troubleshoot the same problems.

* As a developer, I need to check cluster installation progress of the Openshift cluster being
installed by agent installer and when this installation is complete to validate the Openshift
cluster.

* As a developer, I want to extract logs from agent installer and the Openshift cluster under
install to troubleshoot and debug.

### API Extensions

Not applicable.

### Risks and Mitigations

** There will be now a third team using dev-scripts for slightly different purposes.
Which means the risk of breaking things in the central framework now is larger and
has a larger "splash zone."

We can mitigate this for the new ephemeral agent installer by running e2e tests using the 
agent flows for changes to the dev-scripts repository. The purpose of this would be to find 
breakages in the tests before they happen.

** The dev-scripts repository clones the [metal-dev-env](https://github.com/metal3-io/metal3-dev-env) 
repository for most of its Ansible roles. Which means there is a risk of changes in this 
repository can break things in our repository as well. We have less control over this repository 
than others inside Openshift.

This is partially mitigated already because the repository is cloned at a specific
Git SHA. Extra testing and sometimes changes are needed when this Git SHA pin for metal3-dev-env
moves.

**OPTIONAL SUGGESTION:** Add a test in dev-scripts that runs maybe once a week that clones 
the HEAD of  metal3-dev-env repository. We would be alerted for breakages more frequently 
and mitigate them more often.


## Design Details

### General Strategy

* Using the scope of the agent installer project we will identify possible refactors to 
existing scripts and new functionality required by this new project.
* New functionality we will write in Ansible, unless it is highly coupled with existing
shell scripts.
* We will create Ansible roles to wrap the existing code making it easier to use within
a pure Ansible solution
* The existing bash scripts will be left as is. Refactoring them will be done piece by 
piece as needed.

### Phase 1 (Crawl): Add development scripts into dev-scripts for Agent Installer

1) Add development scripts into dev-scripts for the new agent installer project using 
existing conventions. [1]
2) Add Ansible roles and tasks to wrap existing shell scripts to make them easier
to integrate with an Ansible solution.
3) Add base e2e test flow to run the ephemeral agent installer image in Openshift CI. [2]
4) Write documentation for new developers joining the agent installer team.

In this phase it should also become more clear what scripts and code are overlapping
with the existing code base.

[1]: The development scripts for agent installer will include the following scenarios:

* Building the agent installer from a configurable Git SHA.
* Setting up the environment for install and stopping. Allowing for a person to manually
test an install.
* Setting up the environment for install and automatically running the install using the
agent installer.

[2]: This will be run against the repository that the agent installer is being worked on.
The base e2e test flow for agent installer is the following scenario:

* Build agent installer
* Set up environment for Openshift installation
* Using the built agent installer start the Openshift installation
* Report when the Openshift installation is complete
* Run existing Openshift conformance tests and validations against the newly installed cluster

### Phase 2 (Walk): Define common environment default configurations for Agent Installer

1) Add basic cluster validations to be ran by a local developer after the Openshift cluster
is installed. [1]
2) Define an Ansible role that will pipe in values to existing shell scripts and manage
the environment variables needed for these scripts.
3) Add in more configuration examples like 
[config_example.sh](https://github.com/openshift-metal3/dev-scripts/blob/master/config_example.sh) 
for dev-scripts that define  common configurations for OCP installs. 
4) Use the above examples to define default configurations for VMs (HA, Compact, SNO) 
and networks (IPV4, IPV6, dualstack) in Ansible.
5) Identify minor refactors to existing scripts deemed helpful for the two projects and
move them into Ansible.

[1]: These local cluster validations include checks for the following: Kubeconfig is created and accessible, 
Openshift Control Plane came up successfully, Openshift cluster is in the "Installed" state and
finally a generic error capture for the errors that output when the Openshift cluster fails installation.

### Phase 3 (Run): Update existing e2e IPI flows to use the refactored Ansible in dev-scripts

1) Update Agent Installer flows to use the new Ansible roles to configure the flows instead
of using the environment variables directly.
2) Update the e2e IPI flows to use this new Ansible configuration roles as needed.


At the end of Phase 3 this enhancement should be considered complete. This enhancement is
defining the base set of things for our development environment. Phase 4 includes the next steps 
that we've already identified.

### Phase 4 (Fly): Continue adding features, flows and configurations as needed for Agent Installer

1) Add more advanced configurations for Agent Installer flows including: connected / disconnected,
network configurations using multiple L2 segments and DHCP configurations.
2) Add new scenarios and flows into the environment as new features require them.
3) Continue to evaluate the existing code base for improvements.

### Open Questions [optional]

### Test Plan

* Test locally while creating the development environment.
* On-board the base e2e flow into Openshift CI for agent installer.

### Graduation Criteria

Not applicable.

#### Dev Preview -> Tech Preview

Not applicable.

#### Tech Preview -> GA

Not applicable.

#### Removing a deprecated feature

Not applicable.

### Upgrade / Downgrade Strategy

Not applicable.

### Version Skew Strategy

* Currently the dev-scripts repository is not versioned. This will stay the same.

* For our test scenarios these will be versioned in openshift-tests following the existing
versioning scheme.

### Operational Aspects of API Extensions

Not applicable.

#### Failure Modes

Not applicable.

#### Support Procedures

Not applicable.

## Implementation History

TBD

## Drawbacks

The biggest argument against it is that by using dev-scripts as the base is we don't
inherit the work that was done previously in validating Assisted Installer in 
assisted-test-infra.

## Alternatives

The choices for making a new development environment for the agent installer project were:

1) Make an entirely new framework
2) Extend assisted-test-infra for agent-installer
3) Extend dev-scripts for agent-installer (Chosen)

Making a new framework (Option #1) was not chosen due to the amount of progress
that would be lost by starting over. In existing frameworks we have solved a lot
of problems for example: image registry mirroring, proxies, upgrades, etc. Redoing
this work is not trivial. The risks were deemed likely to happen with this approach.

As far as extending assisted-test-infra (Option #2) for agent installer was not pursued. 
This is because the flow of the e2e tests in this framework first setup a minikube cluster.
From the perspective of the agent-installer this is not needed. The beginning of the
e2e test flow used in dev-scripts is significantly closer to the flow we desire for the
agent installer.

