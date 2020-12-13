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
   a. Load balancer (only for bootstrap phase)
   b. Requires DNS (configured per installation)
3. Cannot use Bare Metal IPI:
   a. Adds irrelevant dependencies - vips, keepalived, mdns
   b. Requires same L2 between bootstrap and the Single Node OpenShift

### Goals

* Describe an approach for installing Single Node OpenShift in a BareMetal environment for production use.
* Minimal changes to OpenShift installer and the implementation shouldn't affect existing deployment flows.
* Installation should result a clean Single Node OpenShift without any bootstrap leftovers.


### Non-Goals

* Addressing a similar installation flow for multi-node clusters.
* High-availability for Single Node OpenShift.
* Single-node-developer (CRC) cluster-profile installation.
* Supporting cloud deployment for bootstrap in place (since livecd cannot be used). It will be addressed as part of future enhancement.
* Upgrading Single Node OpenShift will be addressed as part of a future enhancement.

## Proposal

The installer will be enhanced to provide a way to generate a single node ignition configuration. 
The user will be able to boot a RHCOS live CD with that ignition to initiate the installation.
The live CD will perform the cluster bootstrap flow.
A master ignition including the control plane static pods will be created as part of the bootstrap. The master ignition will then be used on reboot to complete the installation and bring up Single Node OpenShift.
Use of the liveCD helps to ensure that we have a clean bootstrap
flow with just the master ignition as the handoff point.


### User Stories

#### As an OpenShift user, I want to be able to deploy OpenShift on a supported single node configuration

A user will be able to run the OpenShift installer to create a single-node
deployment, with some limitations (see non-goals above). The user
will not require special support exceptions to receive technical assistance
for the features supported by the configuration.

### Implementation Details/Notes/Constraints

The OpenShift installer `create ignition-configs` command will generate a `bootstrap-in-place-for-live-iso.ign`
file, when the number of replicas for the control plane (in the `install-config.yaml`) is `1`.

The `bootstrap-in-place-for-live-iso.ign` will be embedded into an RHCOS liveCD by the user using the `coreos-install embed` command.

The user will boot a machine with this liveCD and the liveCD will start executing a similar flow to a bootstrap node in a regular installation.

`bootkube.sh` running on the live ISO will execute the rendering logic.
The live ISO environment provides a scratch place to write bootstrapping files so that they don't end up on the real node. This eliminates a potential source of errors and confusion when debugging problems.

The bootstrap static pods will be generated in a way that the control plane operators will be able to identify them and either continue in a controlled way for the next revision, or just keep them as the correct revision and reuse them.

`cluster-bootstrap` will apply all the required manifests (under ``/opt/openshift/manifests/``)

Bootkube will get the master Ignition from `machine-config-server` and
generate an updated master Ignition combining the original ignition with the control plane static pods manifests and all required resources including etcd data.
`bootkube.sh` will write the new master Ignition along with RHCOS to disk.
At this point `bootkube` will reboot the node and let it complete the cluster creation.

After the host reboots, the `kubelet` service will start the control plane static pods.
Kubelet will send a CSR (see below) and join the cluster.
CVO will deploy all cluster operators.
The control plane operators will rollout a new revision (if necessary).

#### OpenShift-installer

We will add logic to the installer to create `bootstrap-in-place-for-live-iso.ign` Ignition config.
This Ignition config will diverge from the default bootstrap Ignition:
bootkube.sh:
1. Start cluster-bootstrap without required pods (`--required-pods=''`)
2. Run cluster-bootstrap with `--bootstrap-in-place` entrypoint to enrich the master Ignition.
3. Write RHCOS image and the master Ignition to disk.
4. Reboot the node.

#### Cluster-bootstrap

By default, `cluster-bootstrap` starts the bootstrap control plane and creates all the manifests under ``/opt/openshift/manifests``.
`cluster-bootstrap` also waits for a list of required pods to be ready. These pods are expected to start running on the control plane nodes.
In case we are running the bootstrap in place, there is no control plane node that can run those pods. `cluster-bootstrap` should apply the manifest and tear down the control plane. If `cluster-bootstrap` fails to apply some of the manifests, it should return an error.


`cluster-bootstrap` will have a new entrypoint `--bootstrap-in-place` which will get the master Ignition as input and will enrich the master Ignition with control plane static pods manifests and all required resources including etcd data.

#### Bootstrap / Control plane static pods

The control plane components we will add to the master Ignition are (to be placed under `/etc/kubernetes/manifests`):

1. etcd-pod
2. kube-apiserver-pod
3. kube-controller-manager-pod
4. kube-scheduler-pod

Control plane required resources to be added to the Ignition:

1. `/var/lib/etcd`
2. `/etc/kubernetes/bootstrap-configs`
3. /opt/openshift/tls/* (`/etc/kubernetes/bootstrap-secrets`)
4. /opt/openshift/auth/kubeconfig-loopback (`/etc/kubernetes/bootstrap-secrets/kubeconfig`)

**Note**: `/etc/kubernetes/bootstrap-secrets` and `/etc/kubernetes/bootstrap-configs` will be deleted after the node reboots by the post-reboot service (see below), and the OCP control plane is ready.

The control plane operators (that will run on the node post reboot) will manage the rollout of a new revision of the control plane pods.

#### etcd data

In order to add a viable, working etcd post reboot, we will take a snapshot of etcd and add it to the master Ignition.
Post reboot we will use the restored etcd-member from the snapshot.

Another option is to stop the etcd pod (move the static pod manifest from `/etc/kubernetes/manifests`).
When stopped, etcd will save its state and exit. We can then add the `/var/lib/etcd` directory to the master Ignition config.
After the reboot, etcd should start with all the data it had prior to the reboot.

#### Post reboot

We will add a new `post-reboot` service for approving the kubelet and the node Certificate Signing Requests.
This service will also cleanup the bootstrap static pods resources once the OCP control plane is ready.
Since we start with a liveCD, the bootstrap services (`bootkube`, `approve-csr`, etc.), `/etc` and `/opt/openshift` "garbage" are written to the ephemeral filesystem of the liveCD, and not to the node's real filesystem.
The files that we need to delete are under:
`/etc/kubernetes/bootstrap-secrets` and `/etc/kubernetes/bootstrap-configs`
These files are required for the bootstrap control plane to start before it is replaced by the control plane operators.
Once the OCP control plane static pods are deployed we can delete the files as they are no longer required. 

### Initial Proof-of-Concept

User flow
1. Generate bootstrap ignition using the OpenShift installer.
2. Embed this Ignition to an RHCOS liveCD.
3. Boot a machine with this liveCD.

This POC uses the following services for mitigating some gaps:
- `patch.service` for allowing single node installation. it won't be required once [single-node production deployment](https://github.com/openshift/enhancements/pull/560/files) is implemented.
- `post_reboot.service` for approving the node CSR and bootstrap static pods resources cleanup post reboot.

Steps to try it out:
- Clone the installer branch: `iBIP_4_6` from https://github.com/eranco74/installer.git
- Build the installer (`TAGS=libvirt hack/build.sh`)
- Add your ssh key and pull secret to the `./install-config.yaml`
- Generate ignition - `make generate`
- Set up networking - `make network` (provides DNS for `Cluster name: test-cluster, Base DNS: redhat.com`)
- Download rhcos image - `make embed` (download RHCOS liveCD and embed the bootstrap Ignition)
- Spin up a VM with the the liveCD - `make start-iso`
- Monitor the progress using `make ssh` and `journalctl -f -u bootkube.service` or `kubectl --kubeconfig ./mydir/auth/kubeconfig get clusterversion`

### Risks and Mitigations

*What are the risks of this proposal and how do we mitigate. Think broadly. For
example, consider both security and how this will impact the larger OKD
ecosystem.*

*How will security be reviewed and by whom? How will UX be reviewed and by whom?*

## Design Details

### Open Questions

1. How will the user specify custom configuration, such as installation disk, static IPs?
2. Number of revisions for the control plane - do we want to make changes to the bootstrap static pods to make them closer to the final ones?
3. What do we do with the CRDs that are generated by operators instead of installed from manifests managed by the CVO?

### Bootable installation artifact (future work)

In order to embed the bootstrap-in-place-for-live-iso Ignition config to the liveCD the user need to get the liveCD and the coreos-installer binary.
We consider adding `openshift-install create single-node-iso` command that that result a liveCD with the bootstrap-in-place-for-live-iso.ign embeded.
It can also take things like additional manifests for setting the RT kernel (and kernel args) via MachineConfig as well
 as supporting injecting network configuration as files and choosing the target disk drive for coreos-installer.
Internally, create single-node-iso would compile a single-node-iso-target.yaml into Ignition (much like coreos/fcct)
 and include it along with the Ignition it generates and embed it into the ISO.

### Limitations

While most CRDs get created by CVO some CRDs are created by the operators, since during the bootstrap phase there is no schedulable node, 
 operators can't run, these CRDs won't be created until the node pivot to become the master node. 
This imposes a limitation on the user when specifying custom manifests prior to the installation. 

These are the CRDs that are not present during bootstrap:
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

### Test Plan

In order to claim full support for this configuration, we must have
CI coverage informing the release.
An end-to-end job using the bootstrap-in-place installation flow,
based on the [installer UPI CI](https://github.com/openshift/release/blob/master/ci-operator/templates/openshift/installer/cluster-launch-installer-metal-e2e.yaml#L507),
 running an appropriate subset of the standard OpenShift tests
will be created and configured to block accepting release images unless it passes.
This job is a different CI from the Single node production edge CI that will run with a bootstrap vm on cloud environment.

That end-to-end job should also be run against pull requests for
the  control plane repos, installer and cluster-bootstrap.

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

**For non-optional features moving to GA, the graduation criteria must include
end to end tests.**

## Implementation History

Major milestones in the life cycle of a proposal should be tracked in `Implementation
History`.

## Drawbacks

1. The API will be unavailable from time to time during the installation.
2. Coreos-installer cannot be used in the cloud environment.
      
## Alternatives

### Installing using remote bootstrap node

Run the bootstrap node in a HUB cluster as VM.
This approach is appealing because it keeps the current installation flow.
Requires external dependencies. 
However, there are drawbacks:
1. It will require Load balancer and DNS per installation.
2. Deployments run remotely via L3 connection (high latency (up to 150ms), low BW in some cases), this isn't optimal for etcd cluster (one member is running on the bootstrap during the installation) 
3. Running the bootstrap on the HUB cluster present a (resources) scale issue (~50*(8GB+4cores)), limits ACM capacity

### Installing without liveISO

Run the bootstrap flow on the node disk and clean up all the bootstrap residues once the node fully configured.
This is very similar to the current enhancement installation approach but without the requirement to start from liveCD. 
This approach advantage is that it will work on cloud environment. 
The disadvantage is that it's more prune to result a single node deployment with bootstrap leftovers.


### Installing using a baked Ignition file.

The installer will generate an ignition config.
This Ignition configuration includes all assets required for launching the single node cluster (including TLS certificates and keys).
When booting a machine with CoreOS and this Ignition configuration the Ignition config will lay down the control plane operator static pods.
The ignition config will also create a static pod that functions as cluster-bootstrap (this pod should delete itself once it’s done) and apply the OCP assets to the control plane.