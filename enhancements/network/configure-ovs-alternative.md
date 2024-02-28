---
title: configure-ovs-alternative
authors:
  - @cybertron
  - @cgoncalves
reviewers: # Include a comment about what domain expertise a reviewer is expected to bring and what area of the enhancement you expect them to focus on. For example: - "@networkguru, for networking aspects, please look at IP bootstrapping aspect"
  - @jcaamano
  - @trozet
  - TODO: Someone from MCO
approvers: # A single approver is preferred, the role of the approver is to raise important questions, help ensure the enhancement receives reviews from all applicable areas/SMEs, and determine when consensus is achieved such that the EP can move forward to implementation.  Having multiple approvers makes it difficult to determine who is responsible for the actual approval.
  - TBD
api-approvers: # In case of new or modified APIs or API extensions (CRDs, aggregated apiservers, webhooks, finalizers). If there is no API change, use "None"
  - None
creation-date: 2023-06-29
last-updated: 2023-09-27
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

* As an OpenShift developer, I want to avoid further complications in the
  configure-ovs.sh script by providing a better way to do advanced bridge
  configuration.

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

An example of a use case this is not intended to address would be a
single-nic deployment on a cloud platform with no unusual network settings.
In that case manual configuration of br-ex would be an unnecessary extra
burden on the user and the logic to handle it automatically would be fairly
simple so using configure-ovs is far less likely to cause problems.

However, it should be noted that this initial implementation does not preclude
future work to replace configure-ovs in all cases. The nmpolicy feature of
NMState may be able to provide a sufficiently generic configuration that no
user input would be required. This can be investigated as a followup once
we have a solution for the more complex use cases.

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
operator. When this mechanism is used to create br-ex, the configure-ovs
script will be skipped to avoid conflicts.

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
minimal networking is required in order to pull Ignition, and since this new
configuration will be provided through Ignition it will still be necessary
to have platform-specific methods of injecting network config before Ignition
runs. However, the platform-specific part can be much simpler because it
won't need to handle every possible network configuration, just enough to
reach the Ignition endpoint. In some use cases, platform-specific configuration
won't even be needed.

### Workflow Description

During initial deployment, the user will provide the installer with a
machine-config manifest containing base64-encoded NMState YAML for each node
in the cluster. This manifest may look something like:

```
apiVersion: machineconfiguration.openshift.io/v1
kind: MachineConfig
metadata:
  labels:
    machineconfiguration.openshift.io/role: worker
  name: 10-br-ex-worker
spec:
  config:
    ignition:
      version: 3.2.0
    storage:
      files:
      - contents:
          source: data:text/plain;charset=utf-8;base64,aW50ZXJmYWNlczoKLSBuYW1lOiBlbnAyczAKICB0eXBlOiBldGhlcm5ldAogIHN0YXRlOiB1cAogIGlwdjQ6CiAgICBlbmFibGVkOiBmYWxzZQogIGlwdjY6CiAgICBlbmFibGVkOiBmYWxzZQotIG5hbWU6IGJyLWV4CiAgdHlwZTogb3ZzLWJyaWRnZQogIHN0YXRlOiB1cAogIGNvcHktbWFjLWZyb206IGVucDJzMAogIGlwdjQ6CiAgICBlbmFibGVkOiBmYWxzZQogICAgZGhjcDogZmFsc2UKICBpcHY2OgogICAgZW5hYmxlZDogZmFsc2UKICAgIGRoY3A6IGZhbHNlCiAgYnJpZGdlOgogICAgcG9ydDoKICAgIC0gbmFtZTogZW5wMnMwCiAgICAtIG5hbWU6IGJyLWV4Ci0gbmFtZTogYnItZXgKICB0eXBlOiBvdnMtaW50ZXJmYWNlCiAgc3RhdGU6IHVwCiAgaXB2NDoKICAgIGVuYWJsZWQ6IHRydWUKICAgIGFkZHJlc3M6CiAgICAtIGlwOiAiMTkyLjE2OC4xMTEuMTEzIgogICAgICBwcmVmaXgtbGVuZ3RoOiAyNAogIGlwdjY6CiAgICBlbmFibGVkOiBmYWxzZQogICAgZGhjcDogZmFsc2UKZG5zLXJlc29sdmVyOgogIGNvbmZpZzoKICAgIHNlcnZlcjoKICAgIC0gMTkyLjE2OC4xMTEuMQpyb3V0ZXM6CiAgY29uZmlnOgogIC0gZGVzdGluYXRpb246IDAuMC4wLjAvMAogICAgbmV4dC1ob3AtYWRkcmVzczogMTkyLjE2OC4xMTEuMQogICAgbmV4dC1ob3AtaW50ZXJmYWNlOiBici1leA==
        mode: 0644
        overwrite: true
        path: /etc/nmstate/openshift/worker-0.yaml
      - contents:
          source: data:text/plain;charset=utf-8;base64,aW50ZXJmYWNlczoKLSBuYW1lOiBlbnAyczAKICB0eXBlOiBldGhlcm5ldAogIHN0YXRlOiB1cAogIGlwdjQ6CiAgICBlbmFibGVkOiBmYWxzZQogIGlwdjY6CiAgICBlbmFibGVkOiBmYWxzZQotIG5hbWU6IGJyLWV4CiAgdHlwZTogb3ZzLWJyaWRnZQogIHN0YXRlOiB1cAogIGNvcHktbWFjLWZyb206IGVucDJzMAogIGlwdjQ6CiAgICBlbmFibGVkOiBmYWxzZQogICAgZGhjcDogZmFsc2UKICBpcHY2OgogICAgZW5hYmxlZDogZmFsc2UKICAgIGRoY3A6IGZhbHNlCiAgYnJpZGdlOgogICAgcG9ydDoKICAgIC0gbmFtZTogZW5wMnMwCiAgICAtIG5hbWU6IGJyLWV4Ci0gbmFtZTogYnItZXgKICB0eXBlOiBvdnMtaW50ZXJmYWNlCiAgc3RhdGU6IHVwCiAgaXB2NDoKICAgIGVuYWJsZWQ6IHRydWUKICAgIGFkZHJlc3M6CiAgICAtIGlwOiAiMTkyLjE2OC4xMTEuMTE0IgogICAgICBwcmVmaXgtbGVuZ3RoOiAyNAogIGlwdjY6CiAgICBlbmFibGVkOiBmYWxzZQogICAgZGhjcDogZmFsc2UKZG5zLXJlc29sdmVyOgogIGNvbmZpZzoKICAgIHNlcnZlcjoKICAgIC0gMTkyLjE2OC4xMTEuMQpyb3V0ZXM6CiAgY29uZmlnOgogIC0gZGVzdGluYXRpb246IDAuMC4wLjAvMAogICAgbmV4dC1ob3AtYWRkcmVzczogMTkyLjE2OC4xMTEuMQogICAgbmV4dC1ob3AtaW50ZXJmYWNlOiBici1leA==
        mode: 0644
        overwrite: true
        path: /etc/nmstate/openshift/worker-1.yaml
```

The filenames must correspond to the hostname of each node as that will be
used to map the configuration to the node. A separate manifest will need
to be provided for masters and workers, which is typical when working with
machine-configs.

When the new service runs, it will copy the appropriate YAML file (based on
the hostname of the node) to /etc/nmstate so it will be applied by the normal
system NMState service. If there is already a file in /etc/nmstate with the
same name, the service will fail in order to avoid overwriting important
system configuration.

Alternatively, there will be a mechanism to deploy a single cluster-wide
configuration. That might look like:

```
apiVersion: machineconfiguration.openshift.io/v1
kind: MachineConfig
metadata:
  labels:
    machineconfiguration.openshift.io/role: master
  name: 10-br-ex-master
spec:
  config:
    ignition:
      version: 3.2.0
    storage:
      files:
      - contents:
          source: data:text/plain;charset=utf-8;base64,aW50ZXJmYWNlczoKLSBuYW1lOiBlbnAyczAKICB0eXBlOiBldGhlcm5ldAogIHN0YXRlOiB1cAogIGlwdjQ6CiAgICBlbmFibGVkOiBmYWxzZQogIGlwdjY6CiAgICBlbmFibGVkOiBmYWxzZQotIG5hbWU6IGJyLWV4CiAgdHlwZTogb3ZzLWJyaWRnZQogIHN0YXRlOiB1cAogIGNvcHktbWFjLWZyb206IGVucDJzMAogIGlwdjQ6CiAgICBlbmFibGVkOiBmYWxzZQogICAgZGhjcDogZmFsc2UKICBpcHY2OgogICAgZW5hYmxlZDogZmFsc2UKICAgIGRoY3A6IGZhbHNlCiAgYnJpZGdlOgogICAgcG9ydDoKICAgIC0gbmFtZTogZW5wMnMwCiAgICAtIG5hbWU6IGJyLWV4Ci0gbmFtZTogYnItZXgKICB0eXBlOiBvdnMtaW50ZXJmYWNlCiAgc3RhdGU6IHVwCiAgaXB2NDoKICAgIGVuYWJsZWQ6IHRydWUKICAgIGRoY3A6IHRydWUKICBpcHY2OgogICAgZW5hYmxlZDogZmFsc2UKICAgIGRoY3A6IGZhbHNl
        mode: 0644
        overwrite: true
        path: /etc/nmstate/openshift/cluster.yaml
```

The only difference is there will be a single file applied to each node of the
specified role. The name of the file must be `cluster.yaml`.
If there are both node-specific and cluster-wide configuration
files present, the node-specific one will take precedence. Again, separate
manifests will be needed for each role because of how MCO works.

When this mechanism is in use, a flag will be set to indicate to configure-ovs
that it should not attempt to configure br-ex.

Day 2 changes will be handled by Kubernetes-NMState. For the initial
implementation, a limited set of changes will be supported. This is because
some operations will be disruptive and we don't currently have any way to
orchestrate such a change. To begin, we intend to support the following day 2
changes:

- DNS updates
- MTU updates
- Add more subordinate interfaces (such as VLANs) to the interface underlying br-ex
  - Currently known to be blocked by https://issues.redhat.com/browse/OCPBUGS-14107
    but a fix is in progress.
- Change bond miimon, mode, and QoS configuration
  - This has been tested and is already working with configure-ovs, so in
    theory it should require little to no effort in this new model.
- Rollback of any failed changes
  - There are known issues with this that may require changes on the OVNK side.

Details on writing NodeNetworkConfigurationPolicies for each of these cases
will be provided in the product documentation.

#### Scaleout

When scaling out new nodes on day 2, it will be necessary to update the
appropriate machine-configs with NMState YAML for the new node(s). If a
cluster-wide configuration is in use, no changes will be necessary. This
must happen before the scaleout occurs.

Since we don't want to trigger a reboot of the entire cluster before each
scaleout operation, we will add the NMState configuration directory to the
list of paths for which the machine-config-operator will not trigger a reboot.
Files in this path will only be applied on initial deployment anyway so it
will never be necessary to reboot a node when updates are made. Note that,
as mentioned above, if changes are desired on day 2 they should be made
with Kubernetes-NMState, not by updating the machine-configs used here.

#### Variation [optional]

We have two paths forward for applying the configuration. One is to write a
custom service that will select only the host-specific NMState file and
apply it. This is what my [current prototype](https://docs.google.com/document/d/1Zp1J2HbVvu-v4mHc9M594JLqt6UwdKoix8xkeSzVL98/edit#heading=h.fba4j9nvp0nl) does.

The other is to use the NMState service already included in the OS image to
apply the configurations. One issue with this is that it will apply everything
in the /etc/nmstate directory rather than just the host-specific file.
To get around that, we could still implement a custom service, but instead of
applying the configurations, it would only be responsible for copying the
correct file to /etc/nmstate so the regular service can apply it. This has
the benefit of not reinventing the NMState service and still gives us full
control over what gets applied to the host, and also leaves us a way to signal
to ovs-configuration that we've already handled the bridge setup.

I'm currently leaning toward the second option just so we avoid having two
NMState services that may conflict with each other.

### API Extensions

This will not directly modify the API of OpenShift. The API for this feature is
implemented in Kubernetes-NMState, and this does not require any specific
modifications in that project at this time.

### Implementation Details/Notes/Constraints [optional]

Scaleout in this implementation (using static IPs on baremetal to demonstrate
as many parts of the process as possible) would look something like this:

* Add the new node's configuration to the machine-config for its role.
  * `oc edit mc 10-br-ex-worker`
* Create and apply a secret containing the minimal static IP configuration
  needed for the node to pull ignition.
  * `oc apply -f ./extraworker-secret.yaml`
* Create and apply a BareMetalHost resource with the network secret attached
  via the `preprovisioningNetworkDataName` field.
```
apiVersion: metal3.io/v1alpha1
kind: BareMetalHost
spec:
...
  preprovisioningNetworkDataName: ostest-extraworker-0-network-config-secret
```
* Scale out the machineset to deploy the new node.
  * `oc project openshift-machine-api`
    `oc get machinesets`
    `oc scale machineset [name of machineset] --replicas=[number of nodes desired after scaling]`

The node will initially boot with the network configuration specified in
`preprovisioningNetworkDataName` and then have the configuration in the
machine-config applied.

Only the first step of the process is new for this feature. The rest is
standard baremetal scaleout when using static IPs.

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

Using a machine-config to do initial deployment and a
NodeNetworkConfigurationPolicy to make day 2 changes will require a manual sync
of the NMState configuration. Unfortunately this is unavoidable because the
Kubernetes-NMState operator is not part of the core payload so it (and its
CRDs) cannot be used for initial deployment. A possible future improvement
in this area would be to have Kubernetes-NMState "adopt" the configuration
applied by this process and generate an NNCP automatically when it is
installed.


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

* If changes are made to the bridge configuration on day 2, will OVNKubernetes
  handle those correctly without a reboot? If not, how do we orchestrate a
  reboot?

  Reply from @cgoncalves:
  > OVN-Kubernetes will not handle the replacement of the gateway interface (enp2s0 in the example above). This is an existing limitation where OVN-Kubernetes installs an OpenFlow rule in br-ex that states the gateway interface is the OVS port ID of enp2s0 (port 1) and where attaching a new interface, say enp3s0, its OVS port ID will be different hence no egress traffic forwarding.

  That doesn't have to block this feature being implemented, but it's something
  we should look into as a followup with the OVNK team.

* Do all platforms have a concept of individual hosts? I know baremetal does and
  I believe VSphere as well, but I'm not sure about cloud platforms.

* Depending on the answer to the previous question, can we require such platforms
  to use only the cluster-wide configuration? I think this should be fine since
  those platforms tend not to use things like static IPs that may require per-host
  configuration.

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
