---
title: configure-ovs-alternative
authors:
  - @cybertron
  - @cgoncalves
reviewers: # Include a comment about what domain expertise a reviewer is expected to bring and what area of the enhancement you expect them to focus on. For example: - "@networkguru, for networking aspects, please look at IP bootstrapping aspect"
  - @jcaamano
  - @trozet
approvers: # A single approver is preferred, the role of the approver is to raise important questions, help ensure the enhancement receives reviews from all applicable areas/SMEs, and determine when consensus is achieved such that the EP can move forward to implementation.  Having multiple approvers makes it difficult to determine who is responsible for the actual approval.
  - TBD
api-approvers: # In case of new or modified APIs or API extensions (CRDs, aggregated apiservers, webhooks, finalizers). If there is no API change, use "None"
  - TBD
creation-date: 2023-06-29
last-updated: 2023-08-03
tracking-link: # link to the tracking ticket (for example: Jira Feature or Epic ticket) that corresponds to this enhancement
  - https://issues.redhat.com/browse/OPNET-265
see-also:
  - https://docs.google.com/document/d/1Zp1J2HbVvu-v4mHc9M594JLqt6UwdKoix8xkeSzVL98/edit?usp=sharing
  - enhancements/network/baremetal-ipi-network-configuration.md
replaces:
  - NA
superseded-by:
  - NA
---

# Configure-ovs Alternative

## Summary

There are some significant limitations to the existing configure-ovs.sh design
in OVNKubernetes. This enhancement is an alternative method of configuring the
br-ex bridge needed by OVNK using NMState that will address most, if not all,
of the problems with the existing implementation.

## Motivation

There are a few problems with configure-ovs.sh:

* It is implemented in Bash, which makes it fragile and difficult to test.
* It is guessing at what the deployer wants for their bridge configuration,
  and it is awkward to override when it guesses wrong.
* It has customer-specific logic because there is no alternative for us to
  support advanced use cases.
* It is incompatible with day 2 changes to the bridge using things like the
  Kubernetes-NMState operator because the operator and script configurations
  will overwrite each other.

### User Stories

* As an OpenShift administrator, I want the OVNKubernetes bridge on a different
  interface from the node IP.

* As an OpenShift administrator, I want to be able to make changes to the
  bridge interface using the same tools I do on the other interfaces.

* As an OpenShift developer, I want a mechanism for bridge configuration that
  is well-tested and less likely to break than a Bash script.

* As an OpenShift developer, I want to simplify the configure-ovs.sh script
  by providing a better way to do advanced bridge configuration.

### Goals

Once this design is implemented, deployers of OpenShift will be able to
explicitly configure br-ex to their exact specifications and will be able to
modify it after deployment using standard networking tools.

### Non-Goals

Complete replacement of configure-ovs. This mechanism is intended for more
advanced use cases, but it will require more complex configuration than
basic deployments need. A possible followup would be to reimplement
configure-ovs.sh in terms of this new mechanism so there is a simple path
here too, but that is not required for the initial implementation.

## Proposal

This mechanism will be similar to the day-1 network configuration described
in enhancements/network/baremetal-ipi-network-configuration.md. However,
there will be some important differences, which will be discussed below.

At a high level, we will provide an interface for the deployer to specify
per-host interface configuration. Configuration for an OpenVSwitch bridge
named br-ex will be mandatory. The format for the configuration will be
NMState YAML, which will be provided to each node via Ignition. Around the
same point in the boot process that configure-ovs would run, we will run
NMState to apply the provided configuration. This will only be done on first
boot because subsequent changes will be made using the Kubernetes-NMState
operator.

### Important differences from baremetal day-1

Instead of pre-processing the NMState config into NetworkManager
nmconnection files, we will simply write the NMState file directly to
the host disk. As of 4.14 we have NMState available at the host level
so it will no longer be necessary to run NMState in a container.

Since this is intended to be a replacement for configure-ovs, it must
work in a cross-platform manner. The baremetal feature is implemented in the
baremetal-operator, which means non-baremetal platforms cannot use it.

However, this is not a replacement for the baremetal feature (and any other
platform-specific deploy-time network configuration tools). This is because
minimal networking is required in order to pull Ignition, and since this
configuration will be provided through Ignition it will still be necessary
to have platform-specific methods of injecting network config before Ignition
runs. However, the platform-specific part can be much simpler because it
won't need to handle every possible network configuration, just enough to
reach the Ignition endpoint. In some use cases, platform-specific configuration
won't even be needed.

### Workflow Description

When the user is populating install-config they will provide the necessary
configuration in the networking section (TODO: Is this the right place?).
It will look something like this:

networking:
  networkType: OVNKubernetes
  machineNetwork:
  - cidr: 192.0.2.0/24
  [...]
  hostConfig:
  - name: master-0
    networkConfig:
      interfaces:
      - name: enp2s0
        type: ethernet
        state: up
        ipv4:
          enabled: false
        ipv6:
          enabled: false
      - name: br-ex
        type: ovs-bridge
        state: up
        copy-mac-from: enp2s0
        ipv4:
          enabled: false
          dhcp: false
        ipv6:
          enabled: false
          dhcp: false
        bridge:
          port:
          - name: enp2s0
          - name: br-ex
      - name: br-ex
        type: ovs-interface
        state: up
        ipv4:
          enabled: true
          dhcp: true
        ipv6:
          enabled: false
          dhcp: false

The name field will need to be something that uniquely identifies a given host.
I'm unsure if name is necessarily the best way to do that, but it is one valid
option. I reused the networkConfig name from the baremetal feature for
consistency. Perhaps something different would be preferable to avoid confusion
though?

Note that although the configuration can be provided on a per-host basis, it is
not mandatory to do so. YAML anchors can be used to duplicate a single
configuration across multiple nodes. For example:

networking:
  networkType: OVNKubernetes
  machineNetwork:
  - cidr: 192.0.2.0/24
  [...]
  hostConfig:
  - name: master-0
    networkConfig: &BOND
      interfaces:
      - name: br-ex
        ...etc...
  - name: master-1
    networkConfig: *BOND
  - name: master-2
    networkConfig: *BOND

This reduces duplication in install-config and the chance of errors due to
unintended differences in node config.

When this mechanism is in use, a flag will be set to indicate to configure-ovs
that it should not attempt to configure br-ex.

If changes to the bridge configuration are desired on day 2 (e.g. adding a VLAN
or changing MTU), the user will need to install Kubernetes-NMState and use it
to apply those changes directly to br-ex.

#### Variation [optional]

This could be implemented almost entirely with machine-configs, with only a simple
service on the host to apply the provided NMState files. This is the approach
the [prototype implementation](https://docs.google.com/document/d/1Zp1J2HbVvu-v4mHc9M594JLqt6UwdKoix8xkeSzVL98/edit#heading=h.fba4j9nvp0nl)
used since per-host configuration was not available any other way. However,
this feels messy as it requires every node to have the configuration for every
other node in the cluster and select the correct one for itself. It does have
the benefit of requiring significantly less work to implement. If users are
writing their NMState configs in machine-configs instead of via a top-level
OpenShift API perhaps this also side-steps the concerns about API style
compatibility between OpenShift and NMState?

One concern with this is that changes to machine-configs require a reboot of
all affected nodes. We could eliminate that by making the machine-configs for
this feature part of the [rebootless updates](https://github.com/openshift/machine-config-operator/blob/94fc0354f154e2daa923dd96defdf86448b2b327/docs/MachineConfigDaemon.md?plain=1#L147)
list in MCO. That way if you wanted to, for example, scale out a new machine
you would just add its config to the existing machine-config, roll that out,
then deploy the new node. These configs will only be applied at deploy time
anyway since Kubernetes-NMState will be used for day 2 changes, so it shouldn't
ever be necessary to reboot a node for changes to these configs.

I believe we would need to add logic for these new config to
[this code](https://github.com/openshift/machine-config-operator/blob/a41f5af837d95e0fc4f59e4497447a26acaf5bc2/pkg/daemon/update.go#L328)
in MCO to make changes without rebooting.

### API Extensions

This will not directly modify the API of OpenShift. The API for this feature is
implemented in Kubernetes-NMState, and this does not require any specific
modifications in that project at this time.

### Implementation Details/Notes/Constraints [optional]

What are the caveats to the implementation? What are some important details that
didn't come across above. Go in to as much detail as necessary here. This might
be a good place to talk about core concepts and how they relate.

### Risks and Mitigations

Because this gives the deployer a great deal of control over the bridge
configuration for OVNK, they will have the ability to configure it incorrectly,
which may cause problems either on initial deployment or at any point after.
We will want to ensure that must-gather is collecting enough information about
the bridge configuration for us to determine whether it is compliant with our
guidelines, and we must have clear instructions on what is required for br-ex.

Note, however, that if an incorrect bridge configuration is applied NMState
will usually catch that and roll back the changes.

### Drawbacks

OpenShift has historically avoided per-host configuration for cluster nodes.
This is a large departure from that philosophy, but that is unavoidable.
Certain aspects of networking (notably static IPs) are by nature host-specific
and cannot be handled any other way.


## Design Details

### Open Questions [optional]

* There has been resistance to using the NMState API in OpenShift in the past
  because it is not compliant with the OpenShift API Guidelines. There is work
  underway in NMState to address this, but is that mandatory for this to be
  implemented?

  Previous discussions:
  https://github.com/openshift/enhancements/pull/1267#discussion_r1013320148

  Some initial work to address this concern:
  https://github.com/nmstate/nmstate/pull/2338

* Which operator is going to be responsible for deploying the NMState configuration?
  MCO will need to be involved since it needs to be included in Ignition, but
  do we want to use machine-configs? The variation discussion above covers this
  option in more detail.

* If changes are made to the bridge configuration on day 2, will OVNKubernetes
  handle those correctly without a reboot? If not, how do we orchestrate a
  reboot?

  Reply from @cgoncalves:
  > OVN-Kubernetes will not handle the replacement of the gateway interface (enp2s0 in the example above). This is an existing limitation where OVN-Kubernetes installs an OpenFlow rule in br-ex that states the gateway interface is the OVS port ID of enp2s0 (port 1) and where attaching a new interface, say enp3s0, its OVS port ID will be different hence no egress traffic forwarding.

  That doesn't have to block this feature being implemented, but it's something
  we should look into as a followup with the OVNK team.

* Do all platforms have a concept of individual hosts? I know baremetal does and
  I believe VSphere as well, but I'm not sure about cloud platforms.

* Depending on the answer to the previous question, can we make this only for
  on-prem platforms?


**** End of current document. Everything below here is unedited template sections ****


### Test Plan

**Note:** *Section not required until targeted at a release.*

Consider the following in developing a test plan for this enhancement:
- Will there be e2e and integration tests, in addition to unit tests?
- How will it be tested in isolation vs with other components?
- What additional testing is necessary to support managed OpenShift service-based offerings?

No need to outline all of the test cases, just the general strategy. Anything
that would count as tricky in the implementation and anything particularly
challenging to test should be called out.

All code is expected to have adequate tests (eventually with coverage
expectations).

### Graduation Criteria

**Note:** *Section not required until targeted at a release.*

Define graduation milestones.

These may be defined in terms of API maturity, or as something else. Initial proposal
should keep this high-level with a focus on what signals will be looked at to
determine graduation.

Consider the following in developing the graduation criteria for this
enhancement:

- Maturity levels
  - [`alpha`, `beta`, `stable` in upstream Kubernetes][maturity-levels]
  - `Dev Preview`, `Tech Preview`, `GA` in OpenShift
- [Deprecation policy][deprecation-policy]

Clearly define what graduation means by either linking to the [API doc definition](https://kubernetes.io/docs/concepts/overview/kubernetes-api/#api-versioning),
or by redefining what graduation means.

In general, we try to use the same stages (alpha, beta, GA), regardless how the functionality is accessed.

[maturity-levels]: https://git.k8s.io/community/contributors/devel/sig-architecture/api_changes.md#alpha-beta-and-stable-versions
[deprecation-policy]: https://kubernetes.io/docs/reference/using-api/deprecation-policy/

**If this is a user facing change requiring new or updated documentation in [openshift-docs](https://github.com/openshift/openshift-docs/),
please be sure to include in the graduation criteria.**

**Examples**: These are generalized examples to consider, in addition
to the aforementioned [maturity levels][maturity-levels].

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

- Announce deprecation and support policy of the existing feature
- Deprecate the feature

### Upgrade / Downgrade Strategy

If applicable, how will the component be upgraded and downgraded? Make sure this
is in the test plan.

Consider the following in developing an upgrade/downgrade strategy for this
enhancement:
- What changes (in invocations, configurations, API use, etc.) is an existing
  cluster required to make on upgrade in order to keep previous behavior?
- What changes (in invocations, configurations, API use, etc.) is an existing
  cluster required to make on upgrade in order to make use of the enhancement?

Upgrade expectations:
- Each component should remain available for user requests and
  workloads during upgrades. Ensure the components leverage best practices in handling [voluntary
  disruption](https://kubernetes.io/docs/concepts/workloads/pods/disruptions/). Any exception to
  this should be identified and discussed here.
- Micro version upgrades - users should be able to skip forward versions within a
  minor release stream without being required to pass through intermediate
  versions - i.e. `x.y.N->x.y.N+2` should work without requiring `x.y.N->x.y.N+1`
  as an intermediate step.
- Minor version upgrades - you only need to support `x.N->x.N+1` upgrade
  steps. So, for example, it is acceptable to require a user running 4.3 to
  upgrade to 4.5 with a `4.3->4.4` step followed by a `4.4->4.5` step.
- While an upgrade is in progress, new component versions should
  continue to operate correctly in concert with older component
  versions (aka "version skew"). For example, if a node is down, and
  an operator is rolling out a daemonset, the old and new daemonset
  pods must continue to work correctly even while the cluster remains
  in this partially upgraded state for some time.

Downgrade expectations:
- If an `N->N+1` upgrade fails mid-way through, or if the `N+1` cluster is
  misbehaving, it should be possible for the user to rollback to `N`. It is
  acceptable to require some documented manual steps in order to fully restore
  the downgraded cluster to its previous state. Examples of acceptable steps
  include:
  - Deleting any CVO-managed resources added by the new version. The
    CVO does not currently delete resources that no longer exist in
    the target version.

### Version Skew Strategy

How will the component handle version skew with other components?
What are the guarantees? Make sure this is in the test plan.

Consider the following in developing a version skew strategy for this
enhancement:
- During an upgrade, we will always have skew among components, how will this impact your work?
- Does this enhancement involve coordinating behavior in the control plane and
  in the kubelet? How does an n-2 kubelet without this feature available behave
  when this feature is used?
- Will any other components on the node change? For example, changes to CSI, CRI
  or CNI may require updating that component before the kubelet.

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

Similar to the `Drawbacks` section the `Alternatives` section is used to
highlight and record other possible approaches to delivering the value proposed
by an enhancement.

## Infrastructure Needed [optional]

Use this section if you need things from the project. Examples include a new
subproject, repos requested, github details, and/or testing infrastructure.

Listing these here allows the community to get the process for these resources
started right away.
