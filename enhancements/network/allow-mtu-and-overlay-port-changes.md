---
title: allow-mtu-and-overlay-port-changes
authors:
  - "@juanluisvaladas"
reviewers:
  - TBD
approvers:
  - TBD
creation-date: 2021-01-25
last-updated: 2021-01-25
status: provisional
---

# Allow MTU and overlay port changes

This covers adding the capability to the cluster network operator of changing
the MTU, and the vxlan and geneve ports post installation.

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

Customers may need to change the MTU, or the ports used for vxlan or geneve
tunnels post-installation. However these changes aren't trivial and may cause
downtime, hence the CNO forbids currently them.

We propose a brand new daemonset that will run on every node of the cluster and
will make the necessary changes in an ordered, and coordinated manner with a
service disruption within the TCP timeout.

## Motivation

While customers usually set the MTU, and the overlay ports some times they
need to change them afterwards for unpredictable reasons such as performance,
or changes in the underlay.

## Goals

* Allow to change MTU post install on both OpenShift SDN and OVN Kubernetes.

* Allow to change the overlay network ports on the underlay in both OpenShift
  SDN and OVN Kubernetes.

* Allow to change both vxlan and geneve ports in a single operation.

## Non goals

* Allow to change MTU configuration in te underlay interfaces.

* Allow to change MTU, and either port in a single operation.

* Other safe or unsafe configuration changes.

## Proposal

When the cluster network operator detects the change it will deploy a new
deaemonset with restartPolicy Mever which is responsible for doing the changes.

Even though both changing the MTU, and changing the port have a lot in common,
there are some key differences and therefore the daemonset will be slightly
different.

When decreasing the MTU each of the deamonset will:

1. Verify that the new MTU is valid for that node. If it is valid add to
   itself the annotation `openshift.io/cno-able-to-change-mtu`.
2. Wait until the timestamp defined in `CHANGE_MTU_INIT_TS`
3. Verify that every pod of that daemonset is running and has the annotation
   `openshift.io/cno-able-to-change-mtu` if any of the pods is not running, or
   is missing that annotation the pod will abort the procedure.
4. Cordon the node where its running.
5. Wait a grace period. This grace period is based on
   `CHANGE_MTU_INIT_TS` rather than the time of finalization of the
   previous step.
6. Proceed to change the MTU of `eth0` on each one of the pods netnamespace.
7. Wait a grace period. This grace period is based on
   `CHANGE_MTU_INIT_TS` rather than the time of finalization of the
   previous step.
8. Proceed to change the MTU of each pod in the virtual switch.
9. Wait a grace period. This grace period is based on
   `CHANGE_MTU_INIT_TS` rather than the time of finalization of the
   previous step.
10. In the case of OVN Kubernetes proceed to change the MTU of the interfaces
   ovn-k8s-mp0, br-local, and ovn-k8s-gw0 sequentially without any delay in
   between.

   In the case of OpenShiftSDN proceed to change the MTU of the interfaces
   vxlansys.

13. Uncordon the node

14. Annotate itself with `openshift.io/cno-succesuflly-changed-mtu` and exit
    with code 0.

When increasing the MTU the same process applies, except steps 10, 8, and 6 are done in reverse order.

Changing the overlay ports is a similar procedure, we will depoy a daemonset
where each pod will:

1. Verify that the port is not being used at the moment by anything else and
   add to itself the annotation `openshift.io/cno-able-to-change-port`
2. Wait until the timestamp defined in `CHANGE_PORT_INIT_TS`
3. Verify that every pod of that daemonset is running and has the annotation
   `openshift.io/cno-able-to-change-port` if any of the pods is not running, or
   is missing that annotation the pod will abort the procedure.
4. Cordon the node where its running.
5. Wait a grace period. This grace period is based on
   `CHANGE_PORT_INIT_TS` rather than the time of finalization of the
   previous step.
6. Actually change the port:
   For OpenShift SDN we'll delete the port:

   `ovs-vsctl --if-exists del-port br0 vxlan0`

   And create it again with no delay at all:

   ```
   ovs-vsctl add-port br0 vxlan0 \
     -- set Interface vxlan0 ofport_request=1 \
     type=vxlan options:remote_ip="flow" \
     options:key="flow" \
     options:dst_port=<port number>
   ```

   In the case of OVN Kubernetes we'll do the same procedure. If we need to
   change the vxlan port we'll gather its configuration from ovs-vsctl, delete
   it, and recreate it with the same parameters.

   And if we need to change the geneve port we'll do the same thing with it.

7. In the case of OpenShift SDN each node will uncordon iself.
8. EAch pod will annotate the node where it's running with
   `openshift.io/cno-succesuflly-changed-mtu` and exit

Once the daemonsets are finished, for OVN Kubernetes the CNO will verify that
the configmap ovnkube-config is synchronized, and will force a rollout of the
ovnkube-master daemonset. After that it will also force a rollout of the
ovnkube-node daemonset.

If any of the nodes fails during the procedure we expect the administrator to
manually reboot the nodes. We could reboot automatically, but given the risks
involved, and that this change is a very uncommon intervention, we consider
safer making the human responsible for it.

### Test Plan
For MTU changes start a TCP server and client in two different nodes
communicating continuosuly. Decrease the MTU and increase it once its finished.
The TCP connection must be alive after both changes.

For vxlan or geneve start a TCP server and client in two different nodes
communicating continuously. Change both ports in OVN kuberentes and vxlan
on OpenShift SDN. The TCP connection must be alive after the change.

## Alternatives

There aren't any supported alternatives to make this change post installation
with, or without downtime. The only option is deploying a new cluster and
migrating everything to the new one.
