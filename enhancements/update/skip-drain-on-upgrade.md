---
title: skip-drain-on-upgrade
authors:
  - "@michaelgugino"
reviewers:
  -
approvers:
  -
creation-date: 2021-05-20
last-updated: 2021-05-20
status: WIP
---

# Skip drain on upgrade for certain pods

Define an API that allows users/OpenShift developers/administrators to opt-out
pods from being drained on upgrade.

This is particularly relevant for stateful workloads.  Today, an upgrade
triggers a drain of each node, thereby evicting pods running on that node,
except for static pods and daemonsets.  When pods are evicted successfully,
they are deleted, therefor all storage needs to be cleaned up.

For some pods, it might be desirable to skip the drain step, so we don't need
to wait for storage to be cleaned up and the pod to be recreated elsewhere.
Since the node is coming back in a few minutes, the pod should be relatively
unaffected, provide it tolerates unreachable NoExecute taints for a sufficient
period (say, 15 minutes).

# Avoid application outages when not draining pods

The point of draining is two fold.  First, it's to cleanly shutdown workloads
so they can be rescheduled elsewhere.  Second, it's to validate PDBs for each
pod and ensure we don't cause an application to lose too many replicas at once.

If we're not evicting a pod, we need to come up with some other alternative
to prevent a total application outage.

## Validate PDBs on applications

Even though we're skipping a particular pod, we should enforce some logic that
still checks that PDBs have some allowed disruptions left.  Since we're not
actually evicting the PDBs, we might race with other nodes being drained, so
this by itself will not be sufficient.

## Option 1: Canary/Guard pods, separate PDBs

Consider application, MyAPP.  MyAPP has 3 replicas with hard pod anti-affinity.
Each replica is running on a separate node due to anti-affinity.  MyAPP is
protected by PDB-a, which has allowed disruptions==1.

MyAPP has an assoiated deployment, MyCanary.  MyCanary has pod-affinity with
MyAPP, required during scheduling, and anti-affinity to other MyCanary pods.
MyCanary has a separate PDB-b, which has allowed disruptions==1.

Draining 2 nodes at once, one node will successfully evict the canary, and the
other will not, due to PDBs.

This scenario breaks down when 2 nodes are being drained and there are 2 or
more applications deployed in such a manner.  MyAPP might win the race on node-1
whereas YourAPP might win the race on node-b.  node-1 will be unable to progress
pass YourAPP, and node-2 will be unable to progress past MyAPP.  In normal drain
scenarios, since we're actually evicting pods, provided there is enough capacity
in the cluster for the drained pods to land elsewhere, eventually this PDB
contention will be resolved as replacement pods for MyAPP and YourAPP will come
up elsewhere.

Another problem is if the canary pods get evicted by the scheduler or
otherwise aren't running on the nodes.

## Option 2: Dear Old Friend, Mutex

In order to avoid the wedging described in Option 1, we need a way to coordinate
actions between drainers.  Short of centralizing all drain operations and only
processing drains serially, there needs to be a mechanism to allow all
applications on a single node to be drained/locked prior to another node/actor
performing a drain.

MCD-1 wants to drain node-1.  It first obtains the global 'drain-lock-mutex'.
This lock is held indefinitely.  No other drain actors can obtain this resource
while held (ergo, mutex).  Once the lock global mutex is held, MCD-1 then locks
each PDB for each un-drained pod, if needed/possible.  The PDB lock could be
a new CRD, or it could be a new field on PDBs, or just an annotation.  Ideally,
we need to teach the eviction API about this new 'lock' and reject any eviction
attempts when the PDB is locked.  They may or may not go for this now or in the
near future, so webhooks could enable this functionality today.  For lantency
purposes, it makes the most sense to me to place the PDB specific lock on the
PDB itself in some fashion.

Once MCD-1 has locked all the necessary PDBs, it will release the global
'drain-lock-mutex' in hopes that any other MCDs can drain their respective nodes
so long as they don't have conflicting lock needs.  Perhaps prior to obtaining
the global lock we should attempt to calculate what locks are needed and then
attempt to take the lock or not based on that state.  If no PDB-specific locks
are needed, drain as normal.

Now, this should prevent race conditions from different MCDs/actors from
taking multiple replicas of the same application down at once, but it still
might leave us in a scenario where drain won't complete in a timely manner for
after we've obtained the global lock.  We might be unable to acquire all
PDB-specific locks due to new disruption after acquiring the global lock.  We
should give up the PDB specific locks after some reasonable timeout and fall
back to the normal drain (eg, deleting the pods) so we don't get wedged
indefinitely.

Finally, we need a place to keep records for each node so our drain code can
be re-entrant.  A small CRD to keep track of what PDBs were locked, and so we
can compare if we need to requeue and ensure we don't need any new ones
(definitely this one) or can release existing ones because all the pods are
gone (maybe this one).

# How else might a drain mutex and/or PDB lock be utilized?

I see the global drain mutex mostly being something that the MCD consumes as
it's a pretty specific use case.

The PDB lock could be used by application owners to signal maintenance.  For
instance, they might be defragging or rebalacing storage for some application
for one reason or another, and while all the pods are 'ready' they'd really
prefer no disruptions during this time period to avoid prolonging the operation
or other reasons.  This could also alleviate the need for canary pods or
otherwise having to have really tight loops controlling pod readiness to prevent
eviction.

While it seems like PDB locks could be a subject to abuse by users (eg, they
just lock all the time to prevent drain from ever occurring), such can be
obtained today by simply specifying more than 1 PDB per pod or otherwise
configuring a PDB to never allow evictions, such as setting allowedDisruptions
to zero or including a 'never-ready' pod in the PDB to skirt any
allowedDisruptions guidance by administrators.

# Graceful Node Shutdown

https://kubernetes.io/blog/2021/04/21/graceful-node-shutdown-beta/

This feature could really improve the overall usefulness of this feature by
allowing (I believe) pods to go unready and signal, for a short time, that they
are unready to the API.

I think adding some new functionality to the kubelet would make the shutdown
process even smoother, particularly if we add the ability to respect each
pod's grace period timeout, possibly with a new API resource, such as a pod
subresource like 'stop/start'.  pod/stop could be wired into the eviction API,
or use very similar semantics.  We would still need the locking semantics to
avoid wedges, but there might be other use cases for this API other than drain.

# POC

I think we have enough features existing in upstream k8s to implement something
that more or less works today.
