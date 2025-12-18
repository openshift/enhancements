---
title: Enable Network Observability on Day 0
authors:
  - "@stleerh"
reviewers:
  - "@jotak"
  - "@jpinsonneau"
  - "@memodi"
  - "Mike Fiedler"
  - "@pavolloffay"
  - "@jan--f"
  - "@abhat"
approvers:
api-approvers:
creation-date: 2025-09-30
last-updated: 2025-12-09
tracking-link:
  - https://issues.redhat.com/browse/OCPSTRAT-2469
see-also:
  - N/A
replaces:
  - N/A
superseded-by:
  - N/A
---

# Enable Network Observability on Day 0

## Summary

This feature enhancement makes Network Observability available on day 0 by default.  That is, Network Observability is up and running after you create an OpenShift cluster using `openshift-install`.  It installs Network Observability Operator and creates a basic FlowCollector instance.  There is an option to turn this off.  It also makes it easy to have Network Observability available on day 1.

## Motivation

Network Observability is an optional OLM operator that collects and stores traffic flow information and provides insights into your network traffic, including troubleshooting features like packet drops, latencies, DNS tracking, and more.

### User Stories

* As a cluster admin or developer, I expect to be able to observe and manage my network traffic without having to install other components.  It should just be there.
* As a cluster admin, I should be able to see the networking health of my cluster after creating it.
* As a customer support engineer, I want the customer to be aware that Network Observability exists and can provide insights into their network traffic, including the ability to troubleshoot a number of networking issues.

These are the related issues for this feature enhancement.

* (Feature) [OCPSTRAT-2469](https://issues.redhat.com/browse/OCPSTRAT-2469) Provide a default OpenShift install experience for Network Observability
* (Epic) [NETOBSERV-2454](https://issues.redhat.com/browse/NETOBSERV-2454) Install Network Observability operator by default on OpenShift clusters
* (Spike) [NETOBSERV-2236](https://issues.redhat.com/browse/NETOBSERV-2236) What it would take to enable Network Observability by default in the console
* (PoC) [NETOBSERV-2247](https://issues.redhat.com/browse/NETOBSERV-2247) Have network observability be available and enabled on day 0

### Goals

Being able to manage and observe the network in an OpenShift cluster is critical in maintaining the health and integrity of the network.  Without it, there’s no way to verify whether your changes are working as expected or whether your network is experiencing issues.

Currently, Network Observability is an optional operator that many customers are not aware of.  A majority of customers using OpenShift Networking do not have Network Observability installed.  Customers are missing out on features that they should have and have already paid for.

By enabling Network Observability at install time, customers don’t need to know about installing a separate operator.  Network observability should just be a part of networking and not thought of as a separate item.  However, there are a few scenarios where you don’t want Network Observability, so there is a way to opt out.

There is no one size fits all solution in terms of configuring Network Observability, but the goal is to keep this part simple, while still providing as much value as possible given the constraints, and make it an easy way to change parameters on day 2.

### Non-Goals

There are other proposals to make Network Observability more visible and prominent, such as displaying a panel that would describe the features of Network Observability and provide a button to install it.  However, this feature enhancement addresses [OCPSTRAT-2469](https://issues.redhat.com/browse/OCPSTRAT-2469) that explicitly calls for Network Observability to be up and running after install.

Network Observability Operator manages the components, such as flowlog pipelines.  Therefore, there is no need to consider the lifecycle management, since that will not change.

## Proposal

There are three OpenShift repositories that this proposal changes.  They are [openshift/api](https://github.com/openshift/api), [openshift/cluster-network-operator](https://github.com/openshift/cluster-network-operator), and [openshift/install](https://github.com/openshift/installer).

### Repository: openshift/api

The openshift/api repository is a shared repository for defining the API.  This adds the `installNetworkObservability` field in the Network Custom Resource Definition (CRD) under the spec section.

```yaml
apiVersion: config.openshift.io/v1
kind: Network
metadata:
  name: cluster
spec:
  installNetworkObservability: true
```

Listing 1: Network manifest

If the value is true or doesn't exist, Network Observability is enabled.  If it is set to false, Network Observability is not enabled or to be precise, *nothing is done*.  It doesn’t remove Network Observability if it is set to false, hence the reason *not* to call it `enableNetworkObservability`.  To be clear, `installNetworkObservability` not only installs Network Observability, but it also creates the FlowCollector custom resource.

### Repository: openshift/cluster-network-operator

The actual enabling of Network Observability is done in the Cluster Network Operator (CNO).  The rationale is that we want the network observability feature to be part of networking.  This is as opposed to being part of the general observability or as a standalone entity.  Yet, there is still a separation at the lower level so that the two can be independently developed and released at different times, particularly for bug fixes.

In CNO, it adds a new controller for observability and adds it to the manager.  The controller is a single Go file where the Reconciler is called initially and reads the state of the installNetworkObservability field.  If true, it does the following:

1. Check if Network Observability Operator (NOO) is installed. If yes, exit.
2. Create "openshift-netobserv-operator" namespace if it doesn't exist.
3. Install NOO using OLM's OperatorGroup and Subscription.
4. Wait for NOO to be ready and the OpenShift web console to be available.
5. Create the "netobserv" namespace if it doesn't exist.
6. Check if a FlowCollector instance exists. If yes, exit.
7. Create a FlowCollector instance.

The Reconciler leverages the existing framework and reuses the concept of client, scheme, and manager.  It provides a clear ownership by having a separate controller for it.  If the Network CR changes, the Reconciler will repeat the above steps.  Note it doesn’t monitor NOO or any of NOO's components for changes, and it doesn’t do any upgrades.  That is still the responsibility of NOO.

### Repository: openshift/install

The openshift/install repository contains the source code for the **openshift-install** binary.  This adds an `installNetworkObservability` field under the existing networking section in **install-config.yaml** to install and enable Network Observability or do nothing.

```yaml
apiVersion: v1
baseDomain: devcluster.openshift.com
networking:
  installNetworkObservability: false
```

Listing 2: install-config.yaml

The `installNetworkObservability` field is passed on to CNO to set the field of the samename in the Network CRD.  If this field is set to true or doesn’t exist, it sets the Network CR’s `installNetworkObservability` field to true.  To *not* enable Network Observability, set it to false as shown above.  This then sets the Network CR’s `installNetworkObservability` field to false.

### FlowCollector Custom Resource (CR)

Here is the FlowCollector Custom Resource (CR) that is instantiated.

```yaml
apiVersion: flows.netobserv.io/v1beta2
kind: FlowCollector
metadata:
  name: cluster
spec:
  agent:
    ebpf:
      features:
        - DNSTracking
      sampling: 400
    type: eBPF
  deploymentModel: Service
  loki:
    enable: false
  namespace: netobserv
```

Listing 3: FlowCollector configuration

Summary:

* Sampling at 400
* No Loki
* No Kafka
* DNSTracking feature enabled, possibly others

### Workflow Description

Network Observability is enabled by default on day 0 (planning stage).  You don’t have to configure anything when using `openshift-install`, and Network Observability Operator will be installed and a FlowCollector custom resource (CR) will be created (Listing 3).

If you don’t want Network Observability enabled, create the **install-config.yaml** file with the command below, and then add the `installNetworkObservability: false` statement under the `networking` section as shown in Listing 2.

`$ openshift-install create install-config`

Alternatively, using your regular **install-config.yaml** file, you can create manifests from it and add the change there instead.  To create the manifests, enter:

`$ openshift-install create manifests`

This creates a **manifests** directory.  Of particular relevance in this directory is a file named **cluster-network-02-config.yml**, which is the Network CR.  Under the spec section, the `installNetworkObservability` field will be set to true or false (Listing 1), depending on your setting in **install-config.yaml**.  If you don’t have the field in **install-config.yaml**, it will set `installNetworkObservability: true` in this file.  If you remove the field in **cluster-network-02-config.yml** or set `installNetworkObservability: false`, Network Observability will not be enabled.

Finally, to create the cluster, enter:

`$ openshift-install create cluster`

When you bring up the OpenShift web console, you should see that NOO is installed just like it would be, had you gone to **Ecosystem > Software Catalog** (formerly **Operators > OperatorHub** in 4.19 or earlier) to install **Network Observability** from Red Hat (not the Community version).  In **Installed Operators**, there should be a row for **Network Observability**.  And in the **Observe** menu, there should be a panel named **Network Traffic**.

### API Extensions

This adds the installNetworkObservability field in the Network CRD under the spec section.  See Listing 2 above.

### Topology Considerations

All topologies are supported where CNO is supported, so this excludes MicroShift.

### Implementation Details/Notes/Constraints

See PoC at [https://github.com/stleerh/cno-observability](https://github.com/stleerh/cno-observability).  It implements what is described here.

### Risks and Mitigations

* Network Observability requires CPU, memory, and storage that the customer might not be aware of.
  Mitigation: The default setting stores only metrics at a high sampling interval to minimize the use of resources. If this isn’t sufficient, more fine-tuning and filtering can be done in the provided default configuration (e.g. filtering on specific interfaces only).
* Some of the Network Observability features aren’t enabled in order to use minimal resources.  Therefore, users might not know about these features.
  Mitigation: Determine what features, particularly related to troubleshooting, can be enabled with minimal CPU and memory impact. Mention other features in the panels.

### Drawbacks

Rather than actually installing NOO and creating the FlowCollector instance, it is less risky and simpler to just display a panel or a button to let the user install and enable Network Observability.  This resolves the awareness issue.  However, by doing this, it will get much less installs compared to making it enabled by default.  It goes against the principle that networking and network observability should always go hand in hand and be there from the start.

## Alternatives (Not Implemented)

### Alternative #1: Make NOO a core component of OpenShift

Rather than have CNO enable Network Observability, take the existing Network Observability Operator (NOO) and have it be installed by default in the cluster.  There needs to be some logic to accept a boolean field in openshift-install, such as **installNetworkObservability** field, to decide whether NOO should be installed or not.  A variation could be that NOO is always installed, but it just determines whether the FlowCollector CR should be created.

| Pros | Cons |
| :---- | :---- |
| Continue independence of NOO and CNO | Goes against principle that networking and network observability should go together |
| Avoids bloating CNO | Only adds a controller that runs on startup |
| NOO is responsible for creating FlowCollector instance | Doesn’t leverage existing framework in CNO to enable Network Observability |
| It moves future observability components under the same section. | In install-config.yaml, it leverages the existing networking section for the installNetworkObservability field.  Where would this be defined?  Perhaps a new observability section needs to be added. |
| none | There needs to be code somewhere that decides whether NOO should be installed or not.  This feature enhancement leverages CNO for this. |

One of the central questions boils down to, "Do we want to position Network Observability as part of OpenShift Networking or part of Cluster Observability?"  This feature enhancement favors the former.

### Alternative #2: Have COO enable Network Observability

Instead of CNO enabling Network Observability, the Cluster Observability Operator (COO) can take this responsibility.  COO is becoming the operator and the central place for core observability components to be installed.  In addition, it provides services like metrics, Perses for dashboards, and troubleshooting via Korrel8r (Observability Signal Correlation).

One major issue is that COO is itself an optional operator, so it can’t enable Network Observability on day 0, because it has to be installed first.  While the author is advocating for COO to be enabled by default for other reasons, it might take longer to get there if that happens.

Architecturally, COO provides common observability services and functions.  Component-based observability, such as Network Observability, should be a layer on top of COO rather than a part of COO, much like observability is for service mesh and virtualization.

### Alternative #3: Have CVO enable Network Observability

Similar to alternative #1, this explicitly suggests having the Cluster Version Operator (CVO) enable Network Observability.  CVO currently manages larger scope operators that represent core cluster functions, such as CNO or Cluster Storage Operator (CSO), rather than specific operators like Network Observability.

## Test Plan

Consider the following in developing a test plan for this enhancement:

- Different architectures
- Different size clusters
- Does it need to integrate with OpenShift’s CI/CD?

Performance testing will be done to optimize the use of resources and to determine the specific FlowCollector settings, with the goal of using less than 5% resources (CPU and memory) and an ideal target of less than 3%.

## Graduation Criteria

Network Observability reached GA back in January 2023.  Because the feature is to simply enable Network Observability, which has already existed for three years, the plan is to forego the Tech Preview and provide GA requirements.

### GA Requirements

* [NETOBSERV-2533](https://issues.redhat.com/browse/NETOBSERV-2533) Performance testing in Loki-less mode with default settings
    - Provide guidance on CPU, memory, and storage resources
    - Measure the impact on Prometheus in the In-Cluster Monitoring
    - Optimize the default FlowCollector configuration
* [NETOBSERV-2534](https://issues.redhat.com/browse/NETOBSERV-2534) Have a way to pause Network Observability functions
* [NETOBSERV-2535](https://issues.redhat.com/browse/NETOBSERV-2535) Security audit on Network Observability code
* [NETOBSERV-2428](https://issues.redhat.com/browse/NETOBSERV-2428) New Service deployment model
* User facing documentation created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Upgrade / Downgrade Strategy

On an upgrade, it will enable Network Observability if it doesn't already exist.  If it does exist, this feature will do nothing.  If you don't want Network Observability to be enabled, edit the Network CR and add the `installNetworkObservablity: false` line.

On a downgrade, the enabling of Network Observability is removed.  Network Observability will remain if it was installed and the FlowCollector will remain if the resource was created.

## Version Skew Strategy

There are no issues with version skew, since the logic to enable Network Observability only resides in CNO.

## Operational Aspects of API Extensions

N/A

## Support Procedures

Check the CNO logs and search for "observability\_controller.go" to determine whether Network Observability did or did not get enabled.
