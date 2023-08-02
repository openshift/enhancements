# Upgrades and order

In the CVO, upgrades are performed in the order described in the payload (lexographic), with operators only running in parallel if they share the same file prefix `0000_NN_` and differ by the next chunk `OPERATOR`, i.e. `0000_70_authentication-operator_*` and `0000_70_image-registry-operator_*` are run in parallel.

Priorities during upgrade:

1. Simplify the problems that can occur
2. Ensure applications aren't disrupted during upgrade
3. Complete within a reasonable time period (30m-1h for control plane)

During upgrade we bias towards predictable ordering for operators that lack sophistication about detecting their prerequisites. Over time, operators should be better at detecting their prerequisites without overcomplicating or risking the predictability of upgrades.

Currently, upgrades proceed in operator order without distinguishing between node and control plane components. Future improvements may allow nodes to upgrade independently and at different schedules to reduce production impact. This in turn complicates the investment operator teams must make in testing and understanding how to version their control plane components independently of node infrastructure.

All components must be N-1 minor version (4.y and 4.y-1) compatible - a component must update its operator first, then its dependents.  All operators and control plane components MUST handle running with their dependents at a N-1 minor version for extended periods and test in that scenario.

## Generalized ordering

The following rough order is defined for how upgrades should proceed:

```linter
config-operator

kube-apiserver

kcm/ksched

important, internal apis, likely to break unless tested:
* cloud-credential-operator
* openshift-apiserver

non-disruptive:
* everything

olm (maybe later move to post disruptive)

maximum disruptive:
  node-specific daemonsets are on-disruptive:
  * network
  * dns

  eventually push button separate node upgrade:
  * mco, mao, cloud-operators
```

Which in practice can be described in runlevels:

```linter
0000_10_*: config-operator
0000_20_*: kube-apiserver
0000_25_*: kube scheduler and controller manager
0000_30_*: other apiservers: openshift and machine
0000_40_*: reserved
0000_50_*: all non-order specific components
0000_60_*: reserved
0000_70_*: disruptive node-level components: dns, sdn, multus
0000_80_*: machine operators
0000_90_*: reserved for any post-machine updates
```

**Note**: If the run level for a component is not defined i.e. the manifest names do not start with prefix `0000_`, it will be assigned to run level `0000_50_*` during the OCP release process. This is done to make sure if an component is agnostic about the run level it runs at, it is run as part of `0000_50_*` as it helps to update more components in parallel which helps in reducing the overall update time.

## Why does the OpenShift 4 upgrade process "restart" in the middle?

Since the release of OpenShift 4, a somewhat frequently asked question is: Why sometimes during an `oc adm upgrade` (cluster upgrade) does the process appear to re-start partway through?  [This bugzilla](https://bugzilla.redhat.com/show_bug.cgi?id=1690816) for example has a number of duplicates, and I've seen the question appear in chat and email forums.

The answer to this question is worth explaining in detail, because it illustrates some fundamentals of the [self-driving, operator-focused OpenShift 4](https://blog.openshift.com/openshift-4-a-noops-platform/).
During the initial development of OpenShift 4, the toplevel [cluster-version-operator](https://github.com/openshift/cluster-version-operator/) (CVO) and the [machine-config-operator](https://github.com/openshift/machine-config-operator/) (MCO) were developed concurrently (and still are).

The MCO is just one of a number of "second level" operators that the CVO manages.  However, the relationship between the CVO and MCO is somewhat special because the MCO [updates the operating system itself](https://github.com/openshift/machine-config-operator/blob/master/docs/OSUpgrades.md) for the control plane.

If the new release image has an updated operating system (`machine-os-content`), the CVO pulling down an update ends up causing it to (indirectly) restart itself.

This is because in order to apply the OS update (or any config changes) MCO will drain each node it is working on updating, then reboot.  The CVO is just a regular pod (driven by a `deployment`) running in the cluster (`oc -n openshift-cluster-version get pods`); it gets drained and rescheduled just like the rest of the platform it manages, as well as user applications.

Also, besides operating system updates, there's the case where an updated payload changes the CVO image itself.

Today, there's no special support in the CVO for passing "progress" between the previous and new pod; the new pod just looks at the current cluster state and attemps to reconcile between the observed and desired state.  This is generally true of the "second level" operators as well, from the MCO to the network operator, the router, etc.

Hence, the fact that the CVO is terminated and restarted is visible to components watching the `clusterversion` object as the status is recalculated.

I could imagine at some point adding clarification for this; perhaps a basic boolean flag state in e.g. a `ConfigMap` or so that denoted that the pod was drained due to an upgrade, and the new CVO pod would "consume" that flag and include "Resuming upgrade..." text in its status. But I think that's probably all we should do.

By not special casing upgrading itself, the CVO restart works the same way as it would if the kernel hit a panic and froze, or the hardware died, there was an unrecoverable network partition, etc.  By having the "normal" code path work in exactly the same way as the "exceptional" path, we ensure the upgrade process is robust and tested constantly.

In conclusion, OpenShift 4 installations by default have the cluster "self-manage", and the transient cosmetic upgrade status blip is a normal and expected consequence of this.
