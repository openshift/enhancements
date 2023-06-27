---
title: relocatable-single-node-openshift
authors:
  - "@eranco"
reviewers: # Include a comment about what domain expertise a reviewer is expected to bring and what area of the enhancement you expect them to focus on. For example: - "@networkguru, for networking aspects, please look at IP bootstrapping aspect"
  - "@knobunc, Networking"
  - "@mrunalp, Runtime"
approvers: # A single approver is preferred, the role of the approver is to raise important questions, help ensure the enhancement receives reviews from all applicable areas/SMEs, and determine when consensus is achieved such that the EP can move forward to implementation.  Having multiple approvers makes it difficult to determine who is responsible for the actual approval.
  - "@mrunalp"
api-approvers: # In case of new or modified APIs or API extensions (CRDs, aggregated apiservers, webhooks, finalizers). If there is no API change, use "None"
  - None
creation-date: 2023-05-23
last-updated: 2023-06-28
tracking-link: # link to the tracking ticket (for example: Jira Feature or Epic ticket) that corresponds to this enhancement
  - https://issues.redhat.com/browse/MGMT-14516
  - https://issues.redhat.com/browse/OCPBU-608
see-also:
  - "[Use OVN-Kubernetes external gateway bridge without a host interface for Microshift](https://github.com/openshift/enhancements/pull/1388)"
  - https://rh-ecosystem-edge.github.io/ztp-pipeline-relocatable/1.0/ZTP-for-factories.html
  - https://github.com/RHsyseng/cluster-relocation-operator 
replaces:
  - 
superseded-by:
  - 
  
---

To get started with this template:
1. **Pick a domain.** Find the appropriate domain to discuss your enhancement.
1. **Make a copy of this template.** Copy this template into the directory for
   the domain.
1. **Fill out the "overview" sections.** This includes the Summary and
   Motivation sections. These should be easy and explain why the community
   should desire this enhancement.
1. **Create a PR.** Assign it to folks with expertise in that domain to help
   sponsor the process.
1. **Merge after reaching consensus.** Merge when there is consensus
   that the design is complete and all reviewer questions have been
   answered so that work can begin.  Come back and update the document
   if important details (API field names, workflow, etc.) change
   during code review.
1. **Keep all required headers.** If a section does not apply to an
   enhancement, explain why but do not remove the section. This part
   of the process is enforced by the linter CI job.

See ../README.md for background behind these instructions.

Start by filling out the header with the metadata for this enhancement.

# Relocatable single node openshift

## Summary

This enhancement proposes the ability to relocate a single node OpenShift (SNO). This capability is critical for fast deployment at the edge and for validating a complete solution before shipping to the edge.

Upon deployment at the edge site, the SNO should allow reconfiguring specific cluster attributes for OCP to function correctly at the edge site.

____________


## Motivation

The primary motivation for relocatable SNO is the fast deployment of single-node OpenShift.

Telecommunications providers continue to deploy OpenShift at the Far Edge. The acceleration of this adoption and the nature of existing Telecommunication infrastructure and processes drive the need to improve OpenShift provisioning speed at the Far Edge site.
A typical OpenShift installation takes a long time due to many gradual processes. Low bandwidth and high packet latency between the hub/registry and the SNO further increase installation time, as is typical in scenarios such as telco.
Although some progress has been made in shortening the installation time, it is an inherently long process that cannot complete within the telco edge time constraints. 

A secondary, though important, motivation for relocatable SNO is that our partners must validate that each SNO is fully functional before shipping it to a remote site, as it is costly to encounter errors in the field.

Although the initial driver for this enhancement is telco, it applies to partners and customers in various industries that rely on field technicians to deploy SNO quickly and reliably.

____________


### User Stories

* As a system installer, I provision and test SNOs in a “factory” and prepare them for shipment.
* As a technician, I receive pre-installed SNOs from a factory and deploy them at far-edge sites. Once deployed, I reconfigure the SNO with site-specific configurations.



### Goals

* Minimize deployment time: The deployment time at the far-edge site should be in the order of minutes, ideally less than 20 minutes.
* Validation before shipment: The solution should allow partners and customers to validate each installed product before shipping it to the far edge, where it is costly to experience errors.
* Simplify SNO deployment at the far edge: Non-technical operators should be able to reconfigure SNO at deployment time.

____________


### Non-Goals

* Relocatable multi-node clusters.
* 

## Proposal

### Configuring single node OpenShift to be reloactable

A few configurations will be applied before the node is relocated to allow the networking changes and reconfiguration required at the edge deployment.
These changes will be applied using a MachineConfig that will:
1. Create an additional known IP on the br-ex bridge. 
2. Disable kubelet service (to prevent kubelet from starting upon deployment at the edge).
3. Create systemd service to re-configure the SNO. 

#### IP address modification

Currently, when a single node OpenShift boots, ovs-configuration.service creates the br-ex bridge with the node’s default interface
and sets the node’s IP as the environment variable for kubelet.service and crio.service.
A relocatable SNO will likely use different network settings at the factory where it is first installed and at the edge site where it is deployed.
To accommodate these network changes, we will decouple the host’s physical interface IP address from the internal node IP used by crio and kubelet.
During the initial installation at the factory, a MachineConfig will be applied that adds a NetworkManager configuration script that adds an IP
to the br-ex bridge when created by ovs-configuration.service.
This IP will be the same IP as ovs-configuration.service configured on the br-ex bridge during the installation. 
The MachineConfig will also add an env override to kubelet.service and crio.service that sets the kubelet NODE_IP to the internal node IP 
added by the NetworkManager pre-up script.
See example MachineConfig [here](https://github.com/eranco74/image-based-installation-poc/blob/master/bake/node-ip.yaml).
This decoupling should allow OCP to keep running as if the hostIP didn't change.

[//]: # (check the implication of this change on diffrent traffic cases, what IP will be the endpoint IP services will get?, what about ingress traffic and nodePort?)
#### Certificate expiry

When deploying SNO, the kubelet and node-bootstrapper certificate signing requests need to get approved and issued for kubelet to authenticate with the kube-apiserver and for the node to register.
The initial certificate is currently valid for 24 hours. Extending this certificate’s validity to 30 days will reduce the time needed to complete the reconfiguration process at the edge.

#### SNO reconfiguration
We will add a new systemd service to reconfigure the node upon deployment at the edge site (see [Alternatives for SNO reconfiguration](#Alternatives for SNO reconfiguration)).
This new service will read the site-specific configuration (relocation-config) and reconfigure the following:

1. IP address and DNS server
The relocation config may contain the desired network configuration for the edge site.
The re-configuration service will expect network overrides as [nmstate](https://github.com/nmstate/nmstate) config yamls.
The reconfiguration service will apply the nmstate configs via nmstatectl as the first reconfiguration step.
Once the networking is configured, the reconfiguration service will start kubelet and wait for the kube-apiserver to be available.

2. cluster Domain
The re-configuration service will retrieve the domain for the cluster from the relocation-config. Although changing the default domain of OCP is not currently supported, we can [modify the domain](https://docs.openshift.com/container-platform/4.12/networking/ingress-operator.html) for all related components. To achieve this, several tasks are performed by the re-configuration service:
a. Create new certificates for the console and authentication with the new domain suffix.
b. Add componentRoutes for the console and authentication.
c. Add namedCert into the apiserver
d. Add appsDomain to use the new domain suffix.
e. Replace all existing routes, ensuring that the components of the cluster utilize the updated domain suffix. 

3. Reconfigure the pull secret
Changing the pull secret is supported by OCP. In case the relocation-config contains pull secret, the re-configuration service will execute `oc set data secret/pull-secret -n openshift-config` with the new pull secret as described [here](https://access.redhat.com/solutions/4902871).
4. Reconfigure the ImageContentSourcePolicy
In case the relocation-config contains ImageContentSourcePolicy the re-configuration service will create a new ImageContentSourcePolicy resource with the updated policy.

5. Hostname
Changing the node hostname is not currently supported by OpenShift.
We need to investigate the impact of this change on OpenShift.

6. Cluster name
Changing the cluster name is not currently supported by OpenShift.
We need to investigate the impact of this change on OpenShift.

### Workflow Description

Telecommunication providers have existing Service Depots where they currently prepare SW/HW prior to shipping servers to Far Edge sites.
pre-installing SNO onto servers in these facilities enable them to validate and update servers in these pre-installed server pools, as needed.

Telecommunications Service Provider Technicians will be rolling out single node Openshift with a vDU configuration to new Far Edge sites.
They will be working from a service depot where they will pre-install a set of Far Edge servers to be deployed at a later date. When ready for deployment,
a technician will take one of these generic-OCP servers to a Far Edge site, enter the site specific information, wait for confirmation that the vDU is in-service/online, and then move on to deploy another server to a different Far Edge site.

Retail employees in brick-and-mortar stores will install SNO servers and it needs to be as simple as possible.
The servers will likely be shipped to the retail store, cabled and powered by a retail employee and the site-specific information needs to be provided to the system in the simplest way possible,
ideally without any action from the retail employee.

#### Variation [optional]

Creating a relocatable single node OpenShift:
The user can configure the SNO to be relocatable prior to the installation.
Alternatively the user can configure a regular functional SNO (installed with supported installation method) to be relocatable 
after the SNO was successfully installed and already running.

Reconfiguration at the Edge site:
The relocation-config for the edge location can be delivered by placing the config file at /opt/openshift/site-config.yaml
The relocation-config can also b delivered using an attached ISO, the reconfiguration service will mount that ISO (identified by known label)
and copy the relocation config to /opt/openshift/site-config.yaml

### API Extensions

TBD, This enhancement might require changes in the behaviour of existing resources in order to allow all required reconfiguration options, e.g. cluster name or hostname. 

### Implementation Details/Notes/Constraints [optional]

There are few caveats of SNO relocation:
1. Changing some cluster attributes (namely hostname and cluster name) isn't supported by OCP, and it's hard to tell what is the amount of work required to allow these re-reconfiguring these fields after the initial installation.  
2. Due to the network configuration changes which set the node IP to an internal IP, we should check the implication of using the internal IP network on the behavior of OCP when exposing a service or adding a worker node.

A proof-of-concept implementation is available for experimenting with the design.
To try it out: [sno-relocation-poc](https://github.com/eranco74/sno-relocation-poc)

### Risks and Mitigations

Some day-2 operators (e.g. sriov-network-operator, performance-addon-operator) might not function properly if they are installed at the factory 
since they require the SNO to be fully configured with the edge site configuration before they are applied.
We can mitigate it by testing the deployment profile and in case the day-2 operator requires the edge site configuration it will get applied
at the edge site instead of at the factory.

#### Security & cryptography

##### Secrets

### Drawbacks

This proposal suggest an approach for adding a new capability that is limited to a specific cluster type (single node), this approach will not work for multi-node clusters.

In order to allow our customers to deploy edge clusters at scale we will need to create a Zero Touch Configuring service (similar to ZTP that only reconfigure the cluster instaed of provisionig it) to enable reconfiguring relocatable SNOs at scale.

## Design Details

### Open Questions [optional]

1. How will the end user get the cluster credentials?
The kubeconfig and kubeadmin password for the cluster will be generated during the initial instalaltion at the factory.
Also, upon chainging 
 we will need a way to communicate thie information to the end customer.

2. Do we need to replace all existing routes, will we need new certs for them?  

3. [How ovn-kbue gets the node IP](https://github.com/ovn-org/ovn-kubernetes/blob/master/go-controller/pkg/node/gateway_init.go#L97)? is it taking it from the from the nodeip hint or does it pick the first one?

### Test Plan

#### Network tests

* Run the network conformance tests on a relocated SNO
* Run [ovn-kubernetes downstream test](https://github.com/ovn-org/ovn-kubernetes/tree/master/test) suite on a relocated SNO

#### General tests
Add unit tests to the Cluster Ingress Operator to make sure the IngressController resource and corresponding Deployment is generated as exepected.

Add tests that check that a relocated single-node cluster is fully functional by running conformance tests post-relocation.

Add tests that check that a relocated single-node cluster is fully functional after an upgrade by running conformance tests post-upgrade.

### Graduation Criteria

**Note:** *Section not required until targeted at a release.*

#### Dev Preview -> Tech Preview

- Ability to utilize the enhancement end to end
- End user documentation, relative API stability
- Sufficient test coverage
- Gather feedback from users rather than just developers
- Enumerate service level indicators (SLIs), expose SLIs as metrics
- Write symptoms-based alerts for the component(s)

#### Tech Preview -> GA

- More testing (upgrade, downgrade, scale)
- Sufficient time for feedback
- Available by default
- Backhaul SLI telemetry
- Document SLOs for the component
- Conduct load testing
- User facing documentation created in [openshift-docs](https://github.com/openshift/openshift-docs/)

**For non-optional features moving to GA, the graduation criteria must include
end to end tests.**

#### Removing a deprecated feature
N/A

### Upgrade / Downgrade Strategy

N/A

### Version Skew Strategy
N/A

### Operational Aspects of API Extensions

Describe the impact of API extensions (mentioned in the proposal section, i.e. CRDs,
admission and conversion webhooks, aggregated API servers, finalizers) here in detail,
especially how they impact the OCP system architecture and operational aspects.

- For conversion/admission webhooks and aggregated apiservers: what are the SLIs (Service Level
  Indicators) an administrator or support can use to determine the health of the API extensions

  Examples (metrics, alerts, operator conditions)
  - authentication-operator condition `APIServerDegraded=False`
  - authentication-operator condition `APIServerAvailable=True`
  - openshift-authentication/oauth-apiserver deployment and pods health

- What impact do these API extensions have on existing SLIs (e.g. scalability, API throughput,
  API availability)

  Examples:
  - Adds 1s to every pod update in the system, slowing down pod scheduling by 5s on average.
  - Fails creation of ConfigMap in the system when the webhook is not available.
  - Adds a dependency on the SDN service network for all resources, risking API availability in case
    of SDN issues.
  - Expected use-cases require less than 1000 instances of the CRD, not impacting
    general API throughput.

- How is the impact on existing SLIs to be measured and when (e.g. every release by QE, or
  automatically in CI) and by whom (e.g. perf team; name the responsible person and let them review
  this enhancement)

#### Failure Modes

- Describe the possible failure modes of the API extensions.
- Describe how a failure or behaviour of the extension will impact the overall cluster health
  (e.g. which kube-controller-manager functionality will stop working), especially regarding
  stability, availability, performance and security.
- Describe which OCP teams are likely to be called upon in case of escalation with one of the failure modes
  and add them as reviewers to this enhancement.

#### Support Procedures

Describe how to
- detect the failure modes in a support situation, describe possible symptoms (events, metrics,
  alerts, which log output in which component)

  Examples:
  - If the webhook is not running, kube-apiserver logs will show errors like "failed to call admission webhook xyz".
  - Operator X will degrade with message "Failed to launch webhook server" and reason "WehhookServerFailed".
  - The metric `webhook_admission_duration_seconds("openpolicyagent-admission", "mutating", "put", "false")`
    will show >1s latency and alert `WebhookAdmissionLatencyHigh` will fire.

- disable the API extension (e.g. remove MutatingWebhookConfiguration `xyz`, remove APIService `foo`)

  - What consequences does it have on the cluster health?

    Examples:
    - Garbage collection in kube-controller-manager will stop working.
    - Quota will be wrongly computed.
    - Disabling/removing the CRD is not possible without removing the CR instances. Customer will lose data.
      Disabling the conversion webhook will break garbage collection.

  - What consequences does it have on existing, running workloads?

    Examples:
    - New namespaces won't get the finalizer "xyz" and hence might leak resource X
      when deleted.
    - SDN pod-to-pod routing will stop updating, potentially breaking pod-to-pod
      communication after some minutes.

  - What consequences does it have for newly created workloads?

    Examples:
    - New pods in namespace with Istio support will not get sidecars injected, breaking
      their networking.

- Does functionality fail gracefully and will work resume when re-enabled without risking
  consistency?

  Examples:
  - The mutating admission webhook "xyz" has FailPolicy=Ignore and hence
    will not block the creation or updates on objects when it fails. When the
    webhook comes back online, there is a controller reconciling all objects, applying
    labels that were not applied during admission webhook downtime.
  - Namespaces deletion will not delete all objects in etcd, leading to zombie
    objects when another namespace with the same name is created.

## Implementation History

Major milestones in the life cycle of a proposal should be tracked in `Implementation
History`.

## Alternatives

An alternative approach would be to use Zero touch provisioning for factory workflows (ZTPFW), it's an existing (unsupported) solution for
relocatable edge clusters allowing both single node and multi (4) node deployments.
The downside of ZTPFW is that it requires 2 additional interface for the internal IP, also it has an overhead of metalLB for API and ingress IPs.

### Alternatives for IP address modification

Instead of adding an extra IP on the br-ex bridge, an alternative method described in
Use [OVN-Kubernetes external gateway bridge without a host interface for Microshift](https://github.com/openshift/enhancements/pull/1388/files)
can be utilized to facilitate network relocation and changes to the host IP address. 
However, it is important to note that employing an external gateway bridge 
without a host interface is more intricate and considerably distinct from the conventional approach of setting up br-ex for OVN-Kubernetes in OCP.

Instead of adding an extra IP we can use ovn local IP as the internal IP, this approach will save the trouble of configuring the internal IP
but will definitely not allow adding workers since both the master (SNO) and the worker will have the same internal IP.

Another approach is to override the default node IP selection by creating a MachineConfig that modifies /etc/default/nodeip-configuration.
This is supported and [documented](https://docs.openshift.com/container-platform/4.12/support/troubleshooting/troubleshooting-network-issues.html#overriding-default-node-ip-selection-logic_troubleshooting-network-issues) and seems preferable vs modifying
/etc/systemd/system/kubelet.service.d/30-nodenet.conf and /etc/systemd/system/crio.service.d/30-nodenet.conf.
since it would ensure that any other services that may need to know the node IP can get it from /etc/default/nodeip-configuration.
However, for this override to work the internal IP should be already configured on the right interface.
Note that this IP needs to be configured both during the bootstrap and post reboot in a way that doesn't interfere with other networking configurations.
While testing these approach it seems that configuring the internal IP with nmstate config (the supported way to configure it by the Agent Based Installer)
the factory installation worked as expected (without having to override the kubelet and crio env), but upon relocation the static configuration prevents the node from getting DHCP.
We think that the current approach avoid this issue since the internal IP is added to the br-ex in a later stage without any dependency on the node networking.
Another advantage of the current approach is that it can be applied on any SNO that was already installed.


### Alternatives for SNO reconfiguration

Since We could use an operator to reconfigure the SNO upon deployment at the edge site instead of a systemd service.
Most of the reconfiguration is reconfiguring API resources like APIServer, Ingress, ICSPs, etc..
This can be performed by applying a `ClusterRelocation` CR and having an Operator update the various objects and reporting the reconfiguration status on the CR.
This approach would require a new operator or adding a controller that reconciles the `ClusterRelocation` CR to an existing cluster operator.
The systemd service will be responsible for loading the configuration, configuring the networking (if provided with static network config), starting kubelet and applying the `ClusterRelocation` CR.
See the POC for [cluster-relocation-operator](https://github.com/RHsyseng/cluster-relocation-operator)

## Infrastructure Needed [optional]
None
