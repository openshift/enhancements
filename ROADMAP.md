---
title: roadmap
authors:
  - "@smarterclayton"
  - "@derekwaynecarr"
  - "@jwforres"
  - "@crawford"
  - "@eparis"
reviewers:
  - "@smarterclayton"
  - "@derekwaynecarr"
  - "@jwforres"
  - "@crawford"
  - "@eparis"
approvers:
  - "@smarterclayton"
  - "@derekwaynecarr"
  - "@jwforres"
  - "@crawford"
  - "@eparis"
creation-date: 2019-11-24
last-updated: 2019-11-24
status: provisional
see-also:
replaces:
superseded-by:
---

# OpenShift Roadmap

## Summary

This document identifies the top level initiatives driving the OpenShift project
as a whole and identifies key interlocking objectives that provide context for
individual enhancements. This document is not a replacement for the enhancements
it references - instead it identifies thematic goals across the entire project
and helps orient developers, users, and advocates in specific directions. This
roadmap is advisory and describes problems and constraints that span multiple
areas of a very large project.


## Motivation

The roadmap helps drive continuity across releases and coherence across many
individual areas of the project. This document is intended to remain relatively
up to date and describe in broad details the top-level objectives of the project.

As a platform, predictability of lifecycle and direction is critical for consumers
making multi-year bets, and the roadmap must provide sufficient clarity that a
new consumer can assess the difference between short, medium, and long term risks.


### Goals

OpenShift generally attempts to satisfy the following objectives:

#### Platform

1. Provide a predictable and reliable distribution of Kubernetes that remains close to the upstream project cadence
2. Provide long-term stability of features and APIs (over a 1-3 year timeframe), regardless of upstream project choices
3. Be "secure by default" in terms of all choices in lifecycle, features, and configuration within the project
4. Provide balanced support for self-service by users on the platform as well as platform as deployment target

#### Ecosystem

5. Identify, stabilize, and operationalize critical ecosystem components and provide them "out-of-the-box" with the distribution (e.g. ingress, networking)
6. Make extension of the core platform (including replacement out-of-the-box components) easy
7. Make platform and component lifecycle trivially easy to manage and low risk

#### Operational

8. Be easily installable in all major environments in an opinionated best-practices fashion, but be flexible to user-provided opinionation
9. Ensure configuration, rollback, and reconfiguration of the platform is broadly consistent and easily automatable
10. Perform automatic maintenance of all software components and infrastructure, detect and repair drift, and continuously monitor subsystem health
11. Provide clear guidance via alerting, user interface, and dashboards when manual intervention is necessary

#### Applications

12. Make developing and deploying a broad range of applications from a broad range of developer skill sets easy and/or possible
13. Provide tools for operational teams to monitor, strictly control or enable self-service, and securely subdivide the resources within a cluster
14. Identify and enable key application development technologies to integrate well with the platform, while preserving the other objectives
15. Progressively orient and educate developers across a broad skill range about patterns and tools that can improve their effectiveness


### Non-Goals

1. Build new components that could be better adapted from within the ecosystem (unless otherwise necessary)
2. Endorse one particular "right way" to build and develop containerized applications - instead enable specific patterns (GitOps, iterative appdev, team driven microservices, etc) that can match a broad range of organizational needs
3. Be a "kitchen-sink" distribution - it is better to have a small core with stable APIs and a big ecosystem at different lifecycles that can evolve without regressing
4. Allow deep customization within the platform - for the components we ship, we want to avoid complex configuration and expansive test matrices
5. Ship upstream components as fast as possible - we emphasize "don't worry" over "fear of missing out" with respect to new changes


## Proposal

OpenShift is a containerized application platform built on Kubernetes and its ecosystem of
tools focused on maximizing operational and developer effiency. Everyone - from a single
developer to the world's largest companies - should be able to develop, build, and run
mission-critical applications with OpenShift in any enviroment and see benefits over their
existing platforms and toolchains.

### User Stories

These stories define the core use cases OpenShift looks to address.

#### Stable Enterprise Kubernetes

As an enterprise IT organization deploying Kubernetes,
I should have a stable and reliable Kubernetes distribution that reduces my support and operational burden while allowing me to meet the organizational, legal, and functional requirements I must work within,
so that I can quickly evaluate, integrate, and deploy Kubernetest to production.

This includes:

* Corporate identity integration like LDAP, SSO, and large scale team hierarchies
* Resource usage reporting and chargeback, hard and soft resource limits, and configurable self-service for teams
* Security and audit compliance (with or without regulatory features), like FIPS, FedRamp, off-cluster audit, secure containers, role-based access control for operations and teams, least-privilege default configurations, and encryption at rest of high value secrets
* Private clusters in cloud environments, airgapped cluster deployments, delegated install with preconfigured VPC networking
* Ability to both integrate with existing data center tooling (load balancing, DNS, networking) as well as the ability to take ownership of those problems within a cluster to reduce organizational friction and improve operational velocity
* A reliable bare-metal and multi-environment block and object storage solution
* Tooling and practices around common problems such as multiple datacenter high-availability, migration of containerized applications across clusters, whole cluster backup and restore, and network tracing control


#### Programmable containerized application deployment environment

As an organization with an existing development pipeline, or one building a new enterprise application
platform, or as a small to medium sized team using Kubernetes as a deployment target,
I should expect Kubernetes and the necessary ecosystem components to remain stable over multiple-year timeframes,
so that I can delivery applications more rapidly, with better operational efficiency, at higher scales, and with better availability.

This includes:

* API stability and conformance within the Kubernetes project and other ecosystem projects
* Backwards and forwards compatibility for all APIs and extensions - all breaks are regressions
* A clear lifecycle that matches my organizational needs with safe upgrades and long term support
* Automation for common operational patterns like autoscaling, machine lifecycle, and load balancer integration
* Automatic hardware, infrastructure, and software monitoring and remediation to mitigate entropy
* Easy infrastructure and user workload monitoring and alerting that can help track and monitor health
* Easy access to both reliable application components on platform and cloud or organizational services off platform
* Access to virtualization tools to migrate existing applications and reduce the need for alternative platforms
* A command line and web console that provide simple operational troubleshooting
* A single-pane-of-glass management experience across one or more clusters that targets planning, capacity, operational monitoring, and policy enforcement


#### Self-service developer platform

As an organization looking to modernize, innovate, or standardize large portions of application development,
I should have tools and patterns that are easily accessible and consumable by a wide range of
developer skillsets and that allow organizational, operational, or security practices to easily integrate,
so that I can rapidly improve my development organization efficiency and react more quickly to business needs.

This includes:

* Simple out-of-the-box tooling and user experiences to iteratively develop and deploy containerized applications
* A range of available runtime frameworks that combine sufficient lifecycles and reasonably recent versions
* A command line and web console that provide simple self-service development workflows on top of the platform
* Easy access to function-as-a-service, service mesh, remote cloud services, and easy to consume automated components (like queues, databases, and caches)
* Deployment and iteration integration with common IDEs, and an on-demand zero-install IDE for quick iteration, prototyping, and troubleshooting
* User experiences that enable incremental learning about Kubernetes, containerized applications, and advanced concepts


#### Project reliability engineering

As an open-source community and product focused organization,
OKD and OpenShift should have a development lifecycle that leverages automation and data capture to rapidly test, release, and
validate the projects being developed within the product,
so that we can deliver higher quality software faster to more environments, with less regressions, and with a tighter feedback loop between developer and deployer.

This includes:

* Broad CI automation to integrate the work of hundreds of open source projects
* Extensive test-before-merge and test-before-release gating via end-to-end and project specific suites, along with manual testing on pull-requests, to catch regressions before they are merged
* Short, automated, and reliable processes for promoting projects to release candidates and publishing them for consumption
* Remote health monitoring of CI, evaluation, and production clusters to identify issues as upgrades roll out and to determine common failures
* Predictable and short release cadences that reduce slippage by derisking delaying individual features


## Initiatives

This lists the important initiatives across the project. These are the ones that span
multiple releases, require close coordination between teams, or have subtle implications
on a large number of areas.


### Automating management of the control plane

Our goal is to fully automate control plane node lifecycle, reduce operational complexity
during recovery of a master, simplify the install sequence and remove the need for a
unique bootstrap node, prepare for vertical autosizing of masters, and enable some form of
non-HA clusters. As of 4.1, a number of operational advantages provided to worker nodes
cannot be realized. A brief sketch of the approach is covered below (in rough order):

1. Automate the core etcd quorum and lifecycle of etcd members with the cluster-etcd-operator
2. Make the bootstrap node look more like a full master and have additional masters join
3. Front the API servers and other master services with service load balancers
4. Automatically recover when a master machine dies on cloud providers by creating a new machine (machine health check)
5. Add out-of-the-box metal load balancing support (with metallb project?).
6. Allow masters to be vertically scaled by changing a machine size property and replacing mismatched masters
7. Add a simple backup recovery experience to etcd operator instances that requires no additional scripting / commands (form new cluster with X after shutting down other workers)
8. Allow the bootstrap node to be easily transitioned to a worker node post boot (to reduce minimum cluster requirements)

Completing this change will simplify the operational experience for masters to only a
single recovery action (purge other masters, pick leader or restore from backup) on all
clouds.

### Allow cluster control planes to be hosted on another cluster

TODO

### Improve management experience of one or more clusters

TODO

### Improve OpenShift on bare metal

TODO


### Improve platform observability and reactivity

The introduction of remote health monitoring and deeper CI monitoring in 4.x is allowing
us to more quickly identify and triage issues impacting the fleet and deliver fixes and
improved monitoring and alerting. We must continue to improve and invest in this pattern by:

1. Identify and prioritize top failure modes in production environments
2. Ensure thorough alert and metrics coverage of those failure modes
3. Improve usage of alerting by making configuration and status more obvious to end users (have you configured alerting yet?)
4. Refine and improve failure monitoring in operators and on cluster (health detection) for key components like ingress, networking, and machines
5. Better correlate configuration failures (on upgrade or in normal operation) and safeguard those changes
6. Identify and implement e2e tests that better simulate top problems (machine failure, master recovery, network loss)
7. Automate detection and reporting of failures as upgrades are being rolled out
8. Reduce triage time of failures with better standard development tooling and dashboarding
9. Better understand which features are in common use to prioritize investment

Investment in this area allows us to more effectively fix the most impactful issues, which
has better user outcomes.


### Improve operator lifecycle manager end-user experience and operator-author lifecycle

TODO


### Improve the networking stack

openshift-sdn has succeeded at being a no-frills default networking plugin for OpenShift.
The introduction of multus in 4.1 opened significant flexibility for integrators to provide
multiple networks and specialized use cases.

As a long term direction we believe OVN has better abstractions in place to grow feature
capability and integrations. IPv6 support (single and dual-stack) is planned only for OVN.
We will continue to improve support for third party networking plugins at install and
update time.

We also wish to improve the integration of multus with the project, potentially by adding
service integration to secondary interfaces.

Finally, a key challenge with SDN is detecting subtle bugs and misconfigurations. We would
like to add network tracing and failure detection to each node to better diagnose and catch
those issues.
