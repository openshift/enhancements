---
title: CoreOS Multi Stream
authors:
  - @sdodson
  - @jlebon
  - @yuqi-zhang
reviewers: 
  - @jlebon        # RHCOS build, shipping, testing
  - @patrickdillon # Installer boot image selection and container native image refs
  - @yuqi-zhang    # MCO items and Boot Image management items
  - @cewong        # NodePool stream selection, boot image selection, and container native image refs
  - @joepvd      # Delivering multiple RHCOS streams in a single release
  - @peterhunt     # Node team concerns about supporting two major OSes
approvers:
  - @mrunalp  
api-approvers: # In case of new or modified APIs or API extensions (CRDs, aggregated apiservers, webhooks, finalizers). If there is no API change, use "None"
  - TBD
creation-date: 2024-08-21
last-updated: 2024-08-21
tracking-link: 
  - https://issues.redhat.com/browse/OCPSTRAT-1150

# CoreOS Multi Stream

## Summary

Starting as early as OpenShift Container Platform (OCP) 4.19 we will ship
and concurrently support two CoreOS streams, each based on one of the two
most recent major RHEL versions. All components responsible for lifecycling
hosts and host configuration will be extended to select from multiple streams
and configure their resources appropriately.

Installer, Machine Config Operator, RHCOS builds, Hypershift, and at least ART
delivery pipelines will need to coordinate to deliver this work.


## Motivation

RHEL Lifecycles and the hardware certified or supported by a given RHEL version
are substantially longer than OCP lifecycles, therefore in order to balance
support of latest hardware and hardware still supported through end of RHEL
Maintenance Phases we must support at least two major OS versions.

### User Stories

* As an OpenShift admin in an environment where hardware certification is not
a concern, I don't want to think about OS major version selection so that I'm
using the OS stream that Red Hat chooses.

* As an OpenShift admin, with hardware certified or supported only by a
specific OS stream I want to specify the OS Stream for that hardware grouped
by installer pools 'ControlPlane' and 'Compute'.

* As an OpenShift admin adding newer hardware to an existing cluster I want the
new hardware to boot, run, and update from a specific OS stream.

* As an OpenShift admin wishing to migrate existing hosts to a newer stream I
have a clearly documented procedure to do that, consisting of creating a new
MachineConfigPool and MachineSets / CAPI equivalents. Then scaling up the new
resources while scaling down the old resources.  ** an MCP does not transition
from one stream to another -- stream is effectively immutable **

* As an OpenShift admin upgrading to the next minor version all of my hosts
continue using their existing streams. If that stream is no longer supported
by the next minor version the upgrade should be inhibited by an Operator
setting Upgradeable=False with an informative message.

* As an OpenShift admin considering moving existing hardware to a new stream, I
have a tool which can validate my existing hardware and configuration letting
me know of challenges I may face including lack of hardware certification or
support on the next major OS version.

### Goals

* Through a combination of OpenShift Lifecycle and support for multiple
streams offer support for RHEL certified / supported hardware through the end
of RHEL's Maintenance phase.

* Make it easier and less risky for admins to transition their clusters from
one major OS version to the next by overlapping support for a substantial set
of OCP releases and time measured in years.

### Non-Goals

* Continue support of RHEL certified / supported hardware during RHEL ELS

* OpenShift supporting more than two major OS versions at once

* OpenShift supporting RHEL Package Mode and RHCOS. Given the additional
complexity introduced here it's advantageous to shed complexity elsewhere.

* OpenShift supports multiple minor versions from a given major OS version,
because we still want to maintain effective control and limit testing scope.

## Proposal

openshift/api/machineconfiguration/v1/types.go adds MachineOSStream enum with
valid values `rhcos-9`, `rhcos-10` for OCP and `scos-9` and `scos-10` for OKD.

** Need to dig into OKD integration options **

MachineConfigPool adds MachineOSStream as a field defaults to `rhcos-9` or `scos-9`
to handle upgrades from releases without the field. Expectation is that
installer always sets the value at install time going forward.

data/data/rhcos.json is moved to data/data/coreos/rhcos-9.json
data/data/fcos.json is moved to data/data/coreos/scos-9.json
future rhcos-10.json and scos-10.json will be located in the same directory

The release payload adds additional images for new streams and their
extensions. Probably a good idea to tag osStream images uniformly and include
stream name in the off chance that we ever grow to more than two streams.
Whatever magic that plumbs in those image refs to MCPs needs to get updated.
TODO: This is part that I (Scott) don't understand very well so need help.

CoreOS stream files metadata section adds a `labels` array with at least
`node.openshift.io/ID` and `node.openshift.io/version_ID` where the values
match those fields in os-release metadata. Whenever nodes and machines are
derived from these streams the manager for those resources should apply all
labels from the stream metadata.

** Note, it doesn't actually look like any of the current values in /etc/os-release
are valid for to name specifically a major OS, do we need to add any? **

```
sh-5.1# cat /etc/os-release 
NAME="Red Hat Enterprise Linux CoreOS"
ID="rhcos"
ID_LIKE="rhel fedora"
VERSION="418.94.202409201939-0"
VERSION_ID="4.18"
VARIANT="CoreOS"
VARIANT_ID=coreos
PLATFORM_ID="platform:el9"
PRETTY_NAME="Red Hat Enterprise Linux CoreOS 418.94.202409201939-0"
ANSI_COLOR="0;31"
CPE_NAME="cpe:/o:redhat:enterprise_linux:9::baseos::coreos"
HOME_URL="https://www.redhat.com/"
DOCUMENTATION_URL="https://docs.okd.io/latest/welcome/index.html"
BUG_REPORT_URL="https://access.redhat.com/labs/rhir/"
REDHAT_BUGZILLA_PRODUCT="OpenShift Container Platform"
REDHAT_BUGZILLA_PRODUCT_VERSION="4.18"
REDHAT_SUPPORT_PRODUCT="OpenShift Container Platform"
REDHAT_SUPPORT_PRODUCT_VERSION="4.18"
OPENSHIFT_VERSION="4.18"
RHEL_VERSION=9.4
OSTREE_VERSION="418.94.202409201939-0"
```


CoreOS stream files metadata section adds an 'eol' boolean field. Whenever the
MCO encounters a MachineConfigPool which is configured with an osStream which
is marked as `eol` the MCO should mark itself as Upgradeable=False with an
appropriate message inhibiting upgrades to the next OCP minor version. In the
unlikely event that plans change a subsequent z-stream update could be applied
altering the `eol` value and MCO should then set Upgradeable=True whenever all
currently in use streams are eol false.

When the installer is built for OCP valid `osStream` must start with "rhcos"
and match the name of a file in data/data/coreos/

Somewhere in MCO's templates/ add streams/{rhcos-9,rhcos-10} anything outside
of streams/ goes to MCPs of all streams. Any time there's a file in both
non-streams and streams the more specific streams version takes precidence. And
any file only in streams/ goes only to MCPs of the relevant stream. Whenever
possible, prefer to implement limited conditional logic outside of streams/.

Alternatively, all files deployed via MachineConfig should have internal
conditional logic which applies to the relevant ID and VERSION_ID fields
recorded in /etc/os-release or other available facilities. There's probably
some systemd magic here or something. This probably also pushes more static
files to be generated on host via scripts which seems like a decent amount of added complexity.


### Workflow Description

At install time the **cluster creator** will either specify the desired os
for `ControlPlane` and `Compute` or not, if they provide no value the installer
is to render the current default stream into relevant resources.

The Installer will create control plane hosts using the correct boot images,
and Ignition which uses the correct container native OS to pivot into.

When updating MachineConfigPools or NodePools the stream for those pools
should be used to determine boot image and container native OS update sources.

In HCP, in order to upgrade to the new OS you should create new NodePools then
scale up resources into those pools and scale down resources from the old
pools.

In OCP 4.19 in order to upgrade to the new major OS in standalone clusters you
will similarly create new MachineSets and MachineConfigPools which reference
the desired stream then scale those resources up and old resources down.

In future releases we will explore to what extent we will offer in-place host
migration from one major version to the next and what state will be maintained
during that transition.

### API Extensions

API Extensions are CRDs, admission and conversion webhooks, aggregated API servers,
and finalizers, i.e. those mechanisms that change the OCP API surface and behaviour.

- Name the API extensions this enhancement adds or modifies.
- Does this enhancement modify the behaviour of existing resources, especially those owned
  by other parties than the authoring team (including upstream resources), and, if yes, how?
  Please add those other parties as reviewers to the enhancement.

  Examples:
  - Adds a finalizer to namespaces. Namespace cannot be deleted without our controller running.
  - Restricts the label format for objects to X.
  - Defaults field Y on object kind Z.

Fill in the operational impact of these API Extensions in the "Operational Aspects
of API Extensions" section.

### Topology Considerations

#### Hypershift / Hosted Control Planes

Are there any unique considerations for making this change work with
Hypershift?

Assuming NodePools map 1:1 with MachineConfigPools then NodePools would
similarly add a field for `osStream` to ensure that both proper MachineConfig
and boot images are used when creating new nodes.

#### Standalone Clusters

Is the change relevant for standalone clusters? Yes, but no special considerations here.
Need to consider how robust we feel like CPMS is at this point, can we rely on CPMS to
replace hosts?

Whenever compute resources aren't elastic should we support a special mode where the host
OS is reinstalled across versions specifically NOT preserving any data / config?

#### Single-node Deployments or MicroShift

Single Node - Reinstall and redeploy workloads seems like the best approach

Microshift - The OS is managed independently from OpenShift, host binaries
would be shipped for both OSes, and tightly coupled containers such as SDN
would use existing patterns to embed binaries for both OSes and execute the
desired version.

#### Compact Clusters

Compact clusters should not run mixed versions. Once we support in place
migration from one major to the next we should validate that process on
compact clusters.

### Implementation Details/Notes/Constraints

What are some important details that didn't come across above in the
**Proposal**? Go in to as much detail as necessary here. This might be
a good place to talk about core concepts and how they relate. While it is useful
to go into the details of the code changes required, it is not necessary to show
how the code will be rewritten in the enhancement.

### Risks and Mitigations

Core OpenShift Features delivered over the next three years have a hard
dependency on the newer major *host OS version*.
 -- Some OpenShift features may need to be limited to use on RHEL10 nodes.
 While the labeling of nodes with the ID and VERSION_ID is meant to help here
 this will often require a higher level "scheduler" that not only targets
 nodes labeled with the desired labels but tailors the workload, for instance
 CNV qemu pods may operate with different configuration or container image 
 based on host OS version.
 -- Or, some features may be deferred for significant period of time until
 RHEL10 is the older of the two supported OSes.
 -- Or, some features will need to be backported to RHEL9.
 
Significant OCP or layered product features delivered over the next three
years have a hard dependency on the next major *userspace OS version*. The
risk here is that as soon as the core platform depends on RHEL10 userspace we
will drop support for hosts which only offer x86-64-v2. While almost all
server grade hardware that may run OCP 4.19 and later offers x86-64-v3 or later
some edge deployments may not. We should collect telemetry data whever possible
to ensure that we have an understanding of the prevalence of x86-64-v2 limited
hardware in the fleet, ensuring that we account for disconnected edge devices.

Even after extending the effective life of RHEL9 constrained by another three
years we still have significant portion of the fleet running on hardware not
certified on RHEL10. We will collect telemetry data that helps us understand
the prevalence of both hardware not certified or not supported on RHEL10 to
inform future decisions here.

### Drawbacks

Supporting two major OS versions
- doubles our delivery pipeline requirements,
- effectively unions CVE exposure -- for instance a glibc vulnerability in both
versions is not addressed in a given OpenShift release until it's fixed in
both OS versions
- introduces potential for different workload behavior despite container
compatibility matrices
- introduces complexity for most privileged containers
- may defer the move to more modern host OSes

## Open Questions [optional]

To what extent do we return to the notion of an immutable host where at least
all OS and configuration data is reset at least upon upgrading across major OS
versions? Potentially only preserving data volumes or container storage.

Even container storage needs to be considered for clearing, has the underlying
filesystem tuning changed significantly between RHEL8 and RHEL10 warranting a
reformat or potentially conversion to a new filesystem?

What if anything does image mode change or complicate here? Can we assume by
the time we potentially migrate a host from RHEL9 to RHEL10 that can be done
via bootc?

Do OLM Operators get to choose which OSes they support or not?
I don't think they should be able to. Most unprivileged workloads will not
know or care due to the container compatibility matrix. Privileged workloads
that interact directly with the host OS should support both and part of the
goal here will be clearly articulating that going forward it's mandatory to
support the two latest major OS versions.

## Test Plan

OpenShift TRT team has recommended that we approach CI in a similar manner to
how it was performed when both OpenShiftSDN and OVNKubernetes were supported
SDN controllers. That means splitting jobs roughly evenly between the two OS
versions on each platform. Additionally, assuming we move forward with support
for mixed clusters we would need to add some number of mixed OS test jobs.

## Graduation Criteria

We may wish to introduce this capability as Dev Preview lowering initial support
commitments both in terms of the platform's capability to manage two OSes at once
and in terms of tolerances for RHEL10 upon first inclusion in OCP. We should however
have a goal to enable fully supported dual streams no less than one release prior
to the OCP version which requires RHCOS 10 for hardware enablement purposes,
likely OCP 4.22.

### Dev Preview -> Tech Preview

- Ability to utilize the enhancement end to end
- End user documentation, relative API stability
- Sufficient test coverage
- Gather feedback from users rather than just developers
- Enumerate service level indicators (SLIs), expose SLIs as metrics
- Write symptoms-based alerts for the component(s)

### Tech Preview -> GA

- More testing (upgrade, downgrade, scale)
- Sufficient time for feedback
- Available by default
- Backhaul SLI telemetry
- Document SLOs for the component
- Conduct load testing
- User facing documentation created in [openshift-docs](https://github.com/openshift/openshift-docs/)

**For non-optional features moving to GA, the graduation criteria must include
end to end tests.**

### Removing a deprecated feature

- Announce deprecation and support policy of the existing feature
- Deprecate the feature

## Upgrade / Downgrade Strategy

At least as written we are specifically precluding Upgrade and Downgrade.
We assume that customers have capacity to at least reprovision one node
at a time and migrate it between MachineConfigPools which target RHCOS9 
to RHCOS10 MCPs.

## Version Skew Strategy

At least initially, when considering RHCOS 9.6 and RHCOS 10.0 many components will be of the
same minor version. However after that point in time we expect RHEL 9 and RHEL 10 to continuously
drift apart as RHEL10 continues to innovate and RHEL9 approaches the maintenance phase of its lifecycle.

Certain features may not be possible on hosts running RHCOS 9, therefore components depending
on host capabilities will need to gracefully degrade. Components which run privileged will likely
need to ship binaries for both OSes like the SDN/OVN containers do today for RHEL8 workers and RHCOS9.

## Operational Aspects of API Extensions

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

- Describe the possible failure modes of the API extensions.
- Describe how a failure or behaviour of the extension will impact the overall cluster health
  (e.g. which kube-controller-manager functionality will stop working), especially regarding
  stability, availability, performance and security.
- Describe which OCP teams are likely to be called upon in case of escalation with one of the failure modes
  and add them as reviewers to this enhancement.

## Support Procedures

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

## Alternatives

### Lifecycle Extensions
Repeat the forced major OS version update as happened between 4.12 and 4.13
but couple that with extending the lifecycle of the last OCP version to
support RHEL9 (4.18 or 4.20) to match RHEL9 End Of Maintenance date. In
order to avoid asking RHEL for lifecycle extensions on 9.y versions this
version of OCP would be required to eventually rebase RHCOS and container
images to 9.10. The downside here is that we cut off RHEL9 constrained
hardware from OpenShift features which aren't dependent on host OS version.

### New OpenShift Major Version
4.y continues on with RHEL9 at least as long as proposed in the dual stream
proposal while 5.y introduces RHEL10. This would expand the delivery
requirements at least as much as dual stream, and carry most of the
implementation costs and risks. Would preclude mixed vintage hardware whenever
the old and new hardware have non overlapping certified OS versions. It would
however have a clearer story around feature enablement wherever host major OS
version is a factor.

### Blended/Abbreviated Dual Streams
Defer introduction of RHEL10 until necessary for hardware enablement, which is
assumed to be no later than 10.2. Limit the overlap in RHEL9 and RHEL10 support
to a period for which we're willing to accept lowest common denominator feature
enablement then extend the last of the dual stream versions to match RHEL9 life
cycle.  ie: 4.19-4.21 on 9.6, 4.22-4.24 on 9.8/10.2, 4.25-4.27 on 10.4, with
the life of 4.24 being extended to match RHEL9 EoM.

## Infrastructure Needed [optional]

We expect to expand testing infrastructure needed by approximately 10-20%.

