---
title: single-node-deployment-with-bootstrap-in-place
authors:
  - "@eranco"
  - "@mrunalp"
  - "@dhellmann"
  - "@romfreiman"
  - "@tsorya"
reviewers:
  - TBD
  - "@markmc"
  - "@deads2k"
  - "@wking"
  - "@eparis"
  - "@hexfusion"
approvers:
  - TBD
creation-date: 2020-12-13
last-updated: 2020-12-13
status: implementable
see-also:
  - https://github.com/openshift/enhancements/pull/560
  - https://github.com/openshift/enhancements/pull/302
---

# Single Node deployment with bootstrap-in-place

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

As we add support for new features such as [single-node production deployment](https://github.com/openshift/enhancements/pull/560/files),
we need a way to install such clusters without an extra node dependency for bootstrap.

This enhancement describes the flow for installing Single Node OpenShift using a liveCD that performs the bootstrap logic and reboots to become the single node.

## Motivation

Currently, all OpenShift installations use an auxiliary bootstrap node.
The bootstrap node creates a temporary control plane that is required for launching the actual cluster.

Single Node OpenShift installations will often be performed in environments where there are no extra nodes, so it is highly desirable to remove the need for a separate bootstrap machine to reduce the resources required to install the cluster.

The downsides of requiring a bootstrap node for Single Node OpenShift are:

1. The obvious additional node.
2. Requires external dependencies:
   1. Load balancer (only for bootstrap phase)
   2. Preconfigured DNS (for each deployment)

### Goals

* Describe an approach for installing Single Node OpenShift in a BareMetal environment for production use.
* The implementation should require minimal changes to the OpenShift installer,
it should strive to reuse existing code and should not affect existing deployment flows.
* Installation should result in a clean Single Node OpenShift without any bootstrap leftovers.
* Describe an approach that can be carried out by a user manually or automated by an orchestration tool.

### Non-Goals

* Addressing a similar installation flow for multi-node clusters.
* Single-node-developer (CRC) cluster-profile installation.
* Supporting cloud deployment for bootstrap in place. Using a live CD image is challenging in cloud environments,
 so this work is postponed to a future enhancement.

## Proposal

The OpenShift install process relies on an ephemeral bootstrap
environment so that none of the hosts in the running cluster end up
with unique configuration left over from computing how to create the
cluster. When the bootstrap virtual machine is removed from the
process, the temporary files, logs, etc. from that phase should still
be segregated from the "real" OpenShift files on the host. This means
it is useful to retain a "bootstrap environment", as long as we can
avoid requiring a separate host to run a virtual machine.

The focus for single-node deployments right now is edge use cases,
either for telco RAN deployments or other situations where a user may
have several instances being managed centrally. That means it is
important to make it possible to automate the workflow for deploying,
even if we also want to retain the option for users to deploy by hand.
In the telco RAN case, single-node deployments will be managed from a
central "hub" cluster using tools like RHACM, Hive, and metal3.

The baseboard management controller (BMC) in enterprise class hardware
can be given a URL to an ISO image and told to attach the image to the
host as though it was inserted into a CD-ROM or DVD drive. An image
booted from an ISO can use a ramdisk as a place to create temporary
files, without affecting the persistent storage in the host.  This
capability makes the existing live ISO for RHCOS a good foundation on
which to build this feature. A live ISO can serve as the "bootstrap
environment", separate from the real OpenShift system on persistent
storage in the host, with just the master Ignition as the handoff point.
The BMC in the host can be used to automate
deployment via a multi-cluster orchestration tool.

The RHCOS live ISO image uses Ignition to configure the host, just as
the RHCOS image used for running OpenShift does. This means Ignition
is the most effective way to turn an RHCOS live image into a
special-purpose image for performing the installation.

We propose the following steps for deploying single-node instances of
OpenShift:

1. Add a new `create single-node-ignition-config` command to `openshift-installer`
   which generates an Ignition config for a single-node deployment.
2. Combine that Ignition config with an RHCOS live ISO image to build
   an image for deploying OpenShift on a single node.
3. Boot the new image on the host.
4. Bootstrap the deployment, generating a new master Ignition config
   and the static pod definitions for OpenShift. Write them, along
   with an RHCOS image, to the disk in the host.
5. Reboot the host to the internal disk, discarding the ephemeral live
   image environment and allowing the previously generated artifacts
   to complete the installation and bring up OpenShift.

### User Stories

#### As an OpenShift user, I want to be able to deploy OpenShift on a supported single node configuration

A user will be able to run the OpenShift installer to create a single-node
deployment, with some limitations (see non-goals above). The user
will not require special support exceptions to receive technical assistance
for the features supported by the configuration.

### Implementation Details/Notes/Constraints

The installation images for single-node clusters will be unique for
each cluster. The user or orchestration tool will create an
installation image by combining the
`bootstrap-in-place-for-live-iso.ign` created by the installer with an
RHCOS live image using `coreos-install embed`. Making the image unique
allows us to build on the existing RHCOS live image, instead of
delivering a different base image, and means that the user does not
need any infrastructure to serve Ignition configs to hosts during
deployment.

In order to add a viable, working etcd post reboot, we will take a
snapshot of etcd and add it to the Ignition config for the host.
After rebooting, we will use the restored `etcd-member` from the
snapshot to rebuild the database. This allows etcd and the API service
to come up on the host without having to re-apply all of the
kubernetes operations run during bootstrapping.

#### OpenShift-installer

We will add a new `create single-node-ignition-config` command to the
installer to create the `bootstrap-in-place-for-live-iso.ign` Ignition
config.  This new target will not output `master.ign` and `worker.ign`
files.

Users will specify the target disk drive for `coreos-installer` using
the environment variable
`OPENSHIFT_INSTALL_EXPERIMENTAL_BOOTSTRAP_IN_PLACE_COREOS_INSTALLER_ARGS`.
Before the feature graduates from preview, the environment variable
will be replaced with a field in the `install-config.yaml` schema.

This Ignition config will have a different `bootkube.sh` from the
default bootstrap Ignition. In addition to the standard rendering
logic, the modified script will:

1. Start `cluster-bootstrap` without required pods by setting `--required-pods=''`
2. Run `cluster-bootstrap` with the `--bootstrap-in-place` option.
3. Fetch the master Ignition and combine it with the original Ignition
   config, the control plane static pod manifests, the required
   kubernetes resources, and the bootstrap etcd database snapshot to
   create a new Ignition config for the host.
3. Write the RHCOS image and the combined Ignition config to disk.
4. Reboot the node.

#### Cluster-bootstrap

`cluster-bootstrap` will have a new entrypoint `--bootstrap-in-place`
which will get the master Ignition as input and will enrich the master
Ignition with control plane static pods manifests and all required
resources, including the etcd database.

`cluster-bootstrap` normally waits for a list of required pods to be
ready. These pods are expected to start running on the control plane
nodes when the bootstrap and control plane run in parallel. That is
not possible when bootstrapping in place, so when `cluster-bootstrap`
runs with the `--bootstrap-in-place` option it should only apply the
manifests and then tear down the control plane.

If `cluster-bootstrap` fails to apply some of the manifests, it should
return an error.

#### Bootstrap / Control plane static pods

We will review the list of revisions for apiserver and etcd to see if
we can reduce them by eliminating changes caused by observations of
known conditions.  For example, in a single node we know what the etcd
endpoints will be in advance, so we can avoid a revision by observing
this after installation.  This work will go a long way to reducing
disruption during install and improve mean time to recovery for
upgrade re-deployments and failures.

While there is a goal to ensure that the final node state does not
include bootstrapping files, it is necessary to copy some of the files
into the host temporarily to allow bootstrapping to complete. These
files are copied by embedding them in the combined Ignition config,
and after OpenShift is running on the host the files are deleted by
the `post-reboot` service.

The control plane components we will copy from
`/etc/kubernetes/manifests` into the master Ignition are:

1. etcd-pod
2. kube-apiserver-pod
3. kube-controller-manager-pod
4. kube-scheduler-pod

These components also require other files generated during bootstrapping:

1. `/var/lib/etcd`
2. `/etc/kubernetes/bootstrap-configs`
3. `/opt/openshift/tls/*` (`/etc/kubernetes/bootstrap-secrets`)
4. `/opt/openshift/auth/kubeconfig-loopback` (`/etc/kubernetes/bootstrap-secrets/kubeconfig`)

The bootstrap logs are also copied from `/var/log` to aid in
debugging.

See [installer PR
#4482](https://github.com/openshift/installer/pull/4482/files#diff-d09d8f9e83a054002d5344223d496781ea603da7c52706dfcf33debf8ceb1df3)
for a detailed list of the files added to the Ignition config.

After the node reboots, the temporary copies of the bootstrapping
files are deleted by the `post-reboot` service, including:

1. `/etc/kubernetes/bootstrap-configs`
2. `/opt/openshift/tls/*` (`/etc/kubernetes/bootstrap-secrets`)
3. `/opt/openshift/auth/kubeconfig-loopback` (`/etc/kubernetes/bootstrap-secrets/kubeconfig`)
4. bootstrap logs

The bootstrap static pods will be generated in a way that the control
plane operators will be able to identify them and either continue in a
controlled way for the next revision, or just keep them as the correct
revision and reuse them.

#### Post-reboot service

We will add a new `post-reboot` service for approving the kubelet and
the node Certificate Signing Requests. This service will also cleanup
the bootstrap static pods resources when the OpenShift control plane
is ready.

Since we start with a liveCD, the bootstrap services (`bootkube`,
`approve-csr`, etc.), `/etc` and `/opt/openshift` temporary files are
written to the ephemeral filesystem of the live image, and not to the
node's real filesystem.

The files that we need to delete are under:

* `/etc/kubernetes/bootstrap-secrets`
* `/etc/kubernetes/bootstrap-configs`

These files are required for the bootstrap control plane to start
before it is replaced by the control plane operators.  Once the OCP
control plane static pods are deployed we can delete the files as they
are no longer required.

When the `post-reboot` service has completed its work, it removes
itself so it is not run the next time the host reboots.

#### Prerequisites for a Single Node deployment with bootstrap-in-place
The requirements are a subset of the requirements for user-provisioned infrastructure installation.
1. Configure DHCP or set static IP addresses for the node.
The node IP should be persistent, otherwise TLS SAN will be invalidated and will cause the communications between apiserver and etcd to fail.
2. DNS records:
* api.<cluster_name>.<base_domain>.
* api-int.<cluster_name>.<base_domain>.
* *.apps.<cluster_name>.<base_domain>.
* <hostname>.<cluster_name>.<base_domain>.
### Initial Proof-of-Concept

A proof-of-concept implementation is available for experimenting with
the design.

To try it out: [bootstrap-in-place-poc](https://github.com/eranco74/bootstrap-in-place-poc.git)

### Risks and Mitigations

*What are the risks of this proposal and how do we mitigate. Think broadly. For
example, consider both security and how this will impact the larger OKD
ecosystem.*

*How will security be reviewed and by whom? How will UX be reviewed and by whom?*

#### Custom Manifests for CRDs

One limitation of single-node deployments not present in multi-node
clusters is handling some custom resource definitions (CRDs). During
bootstrapping of a multi-node cluster, the bootstrap host and real
cluster hosts run in parallel for a time. This means that the
bootstrap host can iterate publishing manifests to the API server
until the operators running on the other hosts are up and define their
CRDs. If it takes a little while for those operators to install their
CRDs, the bootstrap host can wait and retry the operation. In a
single-node deployment, the bootstrap environment and real node are
not active at the same time. This means the bootstrap process may
block if it tries to create a custom resource using a CRD that is not
installed.

While most CRDs are created by the `cluster-version-operator`, some
CRDs are created later by the cluster operators. These CRDs from
cluster operators are not present during bootstrapping:

* clusternetworks.network.openshift.io
* controllerconfigs.machineconfiguration.openshift.io
* egressnetworkpolicies.network.openshift.io
* hostsubnets.network.openshift.io
* ippools.whereabouts.cni.cncf.io
* netnamespaces.network.openshift.io
* network-attachment-definitions.k8s.cni.cncf.io
* overlappingrangeipreservations.whereabouts.cni.cncf.io
* volumesnapshotclasses.snapshot.storage.k8s.io
* volumesnapshotcontents.snapshot.storage.k8s.io
* volumesnapshots.snapshot.storage.k8s.io

This limitation is unlikely to be triggered by manifests created by
the OpenShift installer, but we cannot control what extra manifests
users add to their deployment. Users need to be made aware of this
limitation and encouraged to avoid creating custom manifests using
CRDs installed by cluster operators instead of the
`cluster-version-operator`.

#### Post-reboot service makes single-node deployments different from multi-node clusters

The `post-reboot` service is only used with single-node
deployments. This makes those deployments look different in a way that
may lead to confusion when debugging issues on the cluster. To
mitigate this, we can add documentation to the troubleshooting guide
to explain the service and its role in the cluster.

#### Bootstrap logs retention

 Due to the bootstrap-in-place behavior all bootstrap artifacts
 will be lost once the bootstrap the node reboots.
In a regular installation flow the bootstrap node goes down only once
 the control plane is running, and the bootstrap node served its purpose.
In case of bootstrap in place things can go wrong after the reboot.
The bootstrap logs can aid in troubleshooting a subsequently failed install.

Mitigation by gathering the bootstrap logs before reboot.
 bootstrap will gather logs from itself using /usr/local/bin/installer-gather.sh.
 Once gathering is complete, the bundle will be added to the master ignition,
 thus making the bootstrap logs available from the master after reboot.
 The log bundle will be deleted once the installation completes.

The installer `gather` command works as usual before the reboot.
We will add a new script called installer-master-bootstrap-in-place-gather.sh.
 This script will be delivered to the master using via Ignition to the same
 location where the bootstrap node usually has installer-gather.sh.
The installer-master-bootstrap-in-place-gather.sh, will be called by the
 `openshift-install gather` command that believes its collecting logs from a bootstrap node.
The script however behaves slightly different, instead of collecting bootstrap logs
and then remotely running /usr/local/bin/installer-masters-gather.sh on all master nodes,
 the script will collect the bootstrap logs from the bundle copied to via the master-ignition,
 and collect master logs by running /usr/local/bin/installer-masters-gather.sh directly
 on itself. The final archiving of all the logs into the home directory,
 exactly the same as /usr/local/bin/installer-gather.sh.

## Design Details

### Open Questions

1. How will the user specify custom configurations, such as static IPs?
2. Number of revisions for the control plane - do we want to make
   changes to the bootstrap static pods to make them closer to the
   final ones? This would also benefit multi-node deployments, but is
   especially helpful for single-node deployments where updating those
   static pod definitions may be more disruptive.

### Test Plan

#### End-to-end testing

In order to claim full support for this configuration, we must have CI
coverage informing the release.  An end-to-end job using the
bootstrap-in-place installation flow, based on the [installer UPI
CI](https://github.com/openshift/release/blob/master/ci-operator/templates/openshift/installer/cluster-launch-installer-metal-e2e.yaml#L507)
and running an appropriate subset of the standard OpenShift tests will
be created. This job is a different CI from the Single node production
edge CI that will run with a bootstrap vm on cloud environment.

The new end-to-end job will be configured to block accepting release
images, and be run against pull requests for the control plane
repositories, installer and cluster-bootstrap.

Although the feature is primarily targeted at bare metal use cases, we
have to balance the desire to test in 100% accurate configurations
with the effort and cost of running CI on bare metal.

Our bare metal CI environment runs on Packet. The hosts are not
necessarily similar to those used by edge or telco customers, and the
API for managing the host is completely different than the APIs
supported by the provisioning tools that will be used to deploy
single-node instances in production environments. Given these
differences, we would derive little benefit from running CI jobs
directly on the hardware.

Each CI job will need to create the test ISO configured to create the
cluster, then boot it on a host. This cannot be done from within a
single host, because the ISO must be served up to the host during the
bootstrap process, while the installer is overwriting the internal
storage of the host. So either the code to create and serve the ISO
needs to run in the CI cluster, or on another host.

Both of these constraints make it simpler, more economical, and faster
to implement the CI job using VMs on a Packet host. Gaps introduced by
using VMs in CI will be covered through other testing performed by the
QE team using hardware more similar to what customers are expected to
have in their production environments. Over time, we may be able to
move some of those tests to more automated systems, including Packet,
if it makes sense.

#### Bootstrap cleanup tests

A goal of this enhancement is to ensure that the host does not contain
any of the temporary bootstrapping files after OpenShift is
running. During bootstrapping, it is necessary to copy some of the
unwanted files into the host temporarily. A test will be created to
verify that the host does not retain those files after the cluster is
successfully launched. The test will run either as part of the
end-to-end job described above, or as part of a separate job.

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

#### Examples

##### Dev Preview -> Tech Preview

- Ability to utilize the enhancement end to end
- End user documentation, relative API stability
- Sufficient test coverage
- Gather feedback from users rather than just developers

##### Tech Preview -> GA

- More testing (upgrade, downgrade, scale)
- Sufficient time for feedback
- Available by default
- Update the installer to replace
  `OPENSHIFT_INSTALL_EXPERIMENTAL_BOOTSTRAP_IN_PLACE_COREOS_INSTALLER_ARGS`
  with a field in the `install-config.yaml` schema.

**For non-optional features moving to GA, the graduation criteria must include
end to end tests.**

## Implementation History

Major milestones in the life cycle of a proposal should be tracked in `Implementation
History`.

## Drawbacks

1. The API will be unavailable from time to time during the installation.
2. Coreos-installer cannot be used in a cloud environment.
3. We need to build new integration with RHACM and Hive for orchestration.

## Alternatives

### Installing using remote bootstrap node

We could continue to run the bootstrap node in a HUB cluster as VM.

This approach is appealing because it keeps the current installation
flow.

However, there are drawbacks:

1. It will require configuring a Load balancer and DNS for each cluster.
2. In some cases, deployments run over L3 connection with high latency
   (up to 150ms) and low bandwidth to sites where there is no
   hypervisor. We would therefore need to run the bootstrap VM
   remotely, and form the etcd cluster with members on both sides of
   the poor connection. Since etcd has requirements for low latency,
   high bandwidth, connections between all nodes, this is not ideal.
3. The bootstrap VM requires 8GB of RAM and 4 CPU cores. Running the
   bootstrap VM on the hub cluster constrains the number of
   simultaneous deployments that can be run based on the CPU and RAM
   capacity of the hub cluster.

### Installing without a live image

We could run the bootstrap flow on the node's regular disk and clean
up all the bootstrap residue once the node is fully configured.  This
is very similar to the current enhancement installation approach but
without the requirement to start from a live image.  The advantage of
this approach is that it will work in a cloud environment as well as
on bare metal. The disadvantage is that it is more prone to result in
a single node deployment with bootstrapping leftovers in place,
potentially leading to confusion for users or support staff debugging
the instances.


### Installing using an Ignition config not built into the live image

We could have the installer generate an Ignition config that includes
all of the assets required for launching the single node cluster
(including TLS certificates and keys). When booting a machine with
CoreOS and this Ignition configuration, the Ignition config would lay
down the control plane operator static pods and create a static pod
that functions as `cluster-bootstrap` This pod should delete itself
after it is done applying the OCP assets to the control plane.
The disadvantage in this approach is that it's very different than
the regular installation flow which involve a bootstrap node.
It also adds more challenges such as:
1. The installer machine (usually the Admin laptop) will need to pull
20 container images in order to render the Ignition.
2. The installer will need to know the node IP in advance for rendering
etcd certificates

### Preserve etcd database instead of a snapshot

Another option for preserving the etcd database when pivoting from
bootstrap to production is to copy the entire database, instead of
using a snapshot operation.  When stopped, etcd will save its state
and exit. We can then add the `/var/lib/etcd` directory to the master
Ignition config.  After the reboot, etcd should start with all the
data it had prior to the reboot. By using a snapshot, instead of
saving the entire database, we will have more flexibility to change
the production etcd configuration before restoring the content of the
database.

### Creating a bootable installation artifact directly from the installer

In order to embed the bootstrap-in-place-for-live-iso Ignition config
to the liveCD the user needs to download the liveCD image and the
`coreos-installer` binary.  We considered adding an `openshift-install
create single-node-iso` command that that result a liveCD with the
`bootstrap-in-place-for-live-iso.ign` embeded.  The installer command
could also include custom manifests, especially `MachineConfig`
instances for setting the realtime kernel, setting kernel args,
injecting network configuration as files, and choosing the target disk
drive for `coreos-installer`.  Internally, `create single-node-iso`
would compile a single-node-iso-target.yaml into Ignition (much like
coreos/fcct) and include it along with the Ignition it generates and
embed it into the ISO.

This approach has not been rejected entirely, and may be handled with
a future enhancement.

### Allow bootstrap-in-place in cloud environment (future work)

For bootstrap-in-place model to work in cloud environemnt we need to mitigate the following gaps:
1. The bootstrap-in-place model relay on the live ISO environment as a place to write bootstrapping files so that they don't end up on the real node.
Optional mitigation: We can mimic this environment by mounting some directories as tmpfs during the bootstrap phase.
2. The bootstrap-in-place model uses coreos-installer to write the final Ignition to disk along with the RHCOS image.
Optional mitigation: We can boot the machine with the right RHCOS image for the release.
Instead of writing the Ignition to disk we will use the cloud credentials to update the node Ignition config in the cloud provider.

### Check control plane replica count in `create ignition-configs`

Instead of adding a new installer command, we could use the current command for generating Ignition configs
`create ignition-configs` to generate the `bootstrap-in-place-for-live-iso.ign` file,
by adding logic to the installer that check the number of replicas for the control
 plane (in the `install-config.yaml`) is `1`.
This approach might conflict with CRC/SNC which also run openshift-install with a 1-replica control plane.

### Use `create ignition-configs` with environment variable to generate the `bootstrap-in-place-for-live-iso.ign`.

We also considered adding a new environment variable `OPENSHIFT_INSTALL_EXPERIMENTAL_BOOTSTRAP_IN_PLACE`
for marking the new path under the `ignition-configs` target.
We decided to add `single-node-ignition-config` target to in order to gain:
1. Allow us to easily add different set of validations (e.g. ensure that the number of replicas for the control plane is 1).
2. We can avoid creating unnecessary assets (master.ign and worker.ign).
3. Less prune to user errors than environment variable.
