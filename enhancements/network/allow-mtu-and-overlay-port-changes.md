---
title: allow-mtu-and-overlay-port-changes
authors:
  - "@juanluisvaladas"
reviewers:
  - "@danwinship"
  - "@mccv1r0"
  - "@russellb"
approvers:
  - TBD
creation-date: 2021-01-25
last-updated: 2022-02-012
status: provisional
---

# Allow MTU and overlay port changes

This covers adding the capability to the cluster network operator of changing
the MTU, and the VXLAN and Geneve ports post installation.

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

Customers may need to change the MTU, or the ports used for VXLAN or Geneve
tunnels post-installation. However these changes aren't trivial and may cause
downtime, hence the CNO forbids currently them.

We propose a brand new daemonset that will be launched on demand and will
run on every node of the cluster. This daemonset will make the necessary
changes in an ordered, and coordinated manner with a service disruption
within the TCP timeout (best effort).

## Motivation

While cluster administrators usually set the MTU, and the overlay ports
correctly during the installation sometimes they need to change them
afterwards for reasons such as changes in the underlay, or because they were
set incorrectly at install time.

## Goals

* Allow to change MTU of the tunnels post install on both OpenShift SDN and
  OVN Kubernetes.

* Allow to change the overlay network ports on the underlay in both OpenShift
  SDN and OVN Kubernetes.

* Allow to change both VXLAN and Geneve ports porst install.

## Non goals

* Allow to change MTU configuration in the underlay interfaces.

* Allow to change MTU, and either port in a single operation.

* Other safe or unsafe configuration changes.

## Proposal

Even though both changing the MTU, and changing the port have a lot in common,
there are some key differences and therefore the daemonset will be slightly
different.

When the cluster network operator detects the change it will:
1. Set the `clusteroperator/network` conditions:
   - Progressing: true
   - Upgradeable: false
2. Deploy a deaemonset with `restartPolicy: Mever` which is responsible for
   validating the preconditions. If the preconditions are met the pod will
   exit with code 0. Some of the preconditions that we will check are:
   - Chrony is synchronized
   - For MTU changes, the new MTU is valid to apply on that node.
   - For Port changes, the new port is not being used by other process
3. Once all the pods in the daemonset are finished, if any of the pods has
   exited with a code different than 0, the CNO will set the
   `clusteroperator/network` conditions to:
   - Progressing: false
   - Degraded: true
   At this point we require manual intervention.

Once the preconditions are met the steps to change the MTU and the ports are
different, for MTU changes the CNO will:
1. Cordon every node, we don't want pods created during the process.
2. Deploy a new daemonset that will run on every node which will apply the
   changes that must be done manually in that node in particular.
3. So that we don't have nodes doing things a t different times and we have
   everything synchronized, the pods will wait until a timestamp defined in
   the environment variable `CHANGE_MTU_INIT_TS`, which is defined by the CNO.
4. The pod deployed by the daemonset, will enter the network namespace of
   every pod in the pod network and change the MTU of `eth0`.
5. The pod will wait a grace period. This grace period is based on
   `CHANGE_MTU_INIT_TS` rather than the time of finalization of the
   previous step.
6. The pod will change the MTU of the end of the veth pair in the virtual
   switch.
7. The pod will wait a grace period. This grace period is based on
   `CHANGE_MTU_INIT_TS` rather than the time of finalization of the
   previous step.
8. For OVN Kubernetes the pod will change the MTU of the interfaces
   ovn-k8s-mp0, br-local, and ovn-k8s-gw0 sequentially without any delay in
   between.
   For OpenShift SDN the pod will change the MTU of the interface tun0.
9. If all these steps failed, the pod will exit with code 1, if all were
   successful it will exit with code 0.

For port changes the CNO will:
1. Cordon every node, we don't want pods created during the process.
2. Deploy a new daemonset that will run on every node which will apply the
   changes that must be done manually in that node in particular.
3. So that we don't have nodes doing things a t different times and we have
   everything synchronized, the pods will wait until a timestamp defined in
   the environment variable `CHANGE_PORT_INIT_TS`, which is defined by the CNO.
4. Actually change the port:
   For OpenShift SDN we'll delete the port:

   `ovs-vsctl --if-exists del-port br0 vxlan0`

   And create it again with no delay at all:

   ```sh
   ovs-vsctl add-port br0 vxlan0 \
     -- set Interface vxlan0 ofport_request=1 \
     type=vxlan options:remote_ip="flow" \
     options:key="flow" \
     options:dst_port=<port number>
   ```

   In the case of OVN Kubernetes we'll do the same procedure. If we need to
   change the VXLAN port we'll gather its configuration from ovs-vsctl, delete
   it, and recreate it with the same parameters.
   And if we need to change the Geneve port we'll do the same thing with it.
5. If all these steps failed, the pod will exit with code 1, if all were
   successful it will exit with code 0.

Once the deamonset that actually makes the changes in the nodes is finished the
CNO will:
1. For OVN Kubernetes the CNO verify that the configmap ovnkube-config is
   synchronized, and will force a rollout of the ovnkube-master daemonset.
   Once the rollout is finished and all the pods are ready, it will also
   force a rollout of the ovnkube-node daemonset.

   For OpenShift SDN we'll make sure the ClusterNetwork object is synchronized
   and we'll force a rollout of the sdn daemonset.

2. Once this is finished, check the exit codes of the daemonset that actually
   makes the changes, if the exit code is 0 uncordon the node, otherwise
   reboot the node, wait for the kubelet to be reporting as Ready again
   and uncordon it.
3. Set the `clusteroperator/network` conditions:
   - Progressing: false
   - Upgradeable: true

### Test Plan
For both MTU, and VXLAN or Geneve port changes we will create the following
tests:
1. A TCP server, and a client in two different nodes communicating
   continuously. The acceptance criteria is the connection has to be kept
   alive.
2. We will also create in parallel another TCP server, and multiple clients
   in different nodes which will establish a lot of very short lived TCP
   connections. The acceptance criteria is that the connections must be
   established.
3. An HTTPS server with a very large certificate, and multiple clients
   doing a single HTTPS request. The acceptance criteria is TLS negotiation
   suceeds and HTTPS request returns 200.

The acceptance criteria for this test is that the long lived TCP connection is
kept alive, and the short lived connections get established.

Packet loss, TCP retransmissions, increased latency, and reduced bandwidth are
considered acceptable while the chane is happening.

For MTU changes, while previous test is running, we will decrease the MTU, and
once it's finished we'll increase it to it's previous value.

For port changes, while previous test is running, we will change both ports in
OVN Kuberentes, and VXLAN on OpenShift SDN. The TCP connection must be alive
after the change.

This test will be two new jobs in CI (one for OVN, and one for OpenShift
SDN), but given that this won't be changed often they will be launched on
demand.

## Alternatives

There aren't any supported alternatives to make this change post installation
with, or without downtime. The only option is deploying a new cluster and
migrating everything to the new one.
