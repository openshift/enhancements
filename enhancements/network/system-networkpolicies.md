---
title: Network Policies for System Namespaces
authors:
  - "@danwinship"
reviewers:
  - TBD
approvers:
  - TBD
creation-date: 2021-01-18
last-updated: 2021-01-18
status: provisional
---

# Network Policies for System Namespaces

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

An OCP cluster contains various services that should not be accessible
to unprivileged users.

Currently, authorization for these services is handled via TLS
certificates; unauthorized users can _connect_ to the service, but
they cannot (usefully) _authenticate_ to it, and so the service is
protected.

Some customers, especially in the financial sector, have objected to
this on the grounds that attackers may be able to exploit bugs in the
TLS implementation to gain access to these services (or the services'
underlying credentials). They would prefer that untrusted users were
prevented from reaching the services entirely. (eg, see [RFE-701])

This enhancement proposes adding NetworkPolicies to an OCP cluster to
(optionally) ensure that restricted services are not reachable except
by the system services that are expected to access them. Each team
would responsible for defining the policies for their own operators.

[RFE-701]: https://issues.redhat.com/browse/RFE-701

## Motivation

### Goals

- Create a global configuration option indicating that OCP operators
  should restrict access to themselves.

- When the option is enabled, have various OCP operators create
  NetworkPolicies limiting access to their services to the appropriate
  other components. (Alternatively: have operators always create the
  restrictive policies but have something also create "allow from all"
  policies when the restricted mode is not configured.)

- Have various OCP operators add labels to their Namespaces as needed
  to allow them to be selected by other components' NetworkPolicies.

- Add appropriate e2e testing:

  - a release-blocking e2e job running in the "restricted"
    configuration

  - per-version upgrade jobs running in the "restricted"
    configuration

  - additions to the default e2e suite to ensure that all `openshift-*`
    namespaces have the expected labels and policies

- Figure out how to also leverage this work to automatically figure
  out when new namespaces need to be added to the "default" or
  "control-plane" VNID groups in openshift-sdn's Multitenant mode.

- Benefit from having more e2e testing of openshift-sdn/ovn-kubernetes
  NetworkPolicy code now since some clusters will now have lots of
  NetworkPolicies in them at all times, helping to avoid bugs like
  [rhbz 1914284] in the future.

[rhbz 1914284]: https://bugzilla.redhat.com/show_bug.cgi?id=1914284

### Non-Goals

- Using the restricted configuration by default for newly-installed or
  upgraded clusters (since that would potentially be breaking for some
  customers).

- Ending the use of TLS authentication for system services and relying
  solely on NetworkPolicy.

- Using NetworkPolicy to restrict access to the apiserver (or any
  other host-network service). (This would be more complicated to do,
  but is also something customers have requested, so we may at least
  want to think about this when designing the configuration option
  [ie, do it in such a way that we could add a "restrict apiserver
  access" option along with it later].)

- Restricting `default` or any `kube-*` namespaces by default.

## Proposal

### Implementation Details/Notes/Constraints

#### "Legacy" vs "New" Namespaces

There is an argument for not retroactively blocking access to existing
namespaces (eg `openshift-apiserver`), but there's no good argument
for leaving _future_ system namespaces open by default in the same
way, since there are no existing users depending on having access to
them. (Of course, if some users in some clusters need access to one of
these new system services, the cluster administrators could create
NetworkPolicies of their own to allow that access.)

#### `default` and `kube-*` Namespaces

The `default` namespace contains the `kubernetes` Service (and the
`openshift` service which is an alias to it). The `kube-system`
namespace contains the `kubelet` Service, which points to all of the
kubelet metrics ports. Both of these services point to host IPs not
pod IPs, so they are out of scope for this enhancement. The
`kube-node-lease` and `kube-public` namespaces do not contain any pods
or services.

Thus, we do not have to consider these namespaces in this proposal.

#### Server-Maintained NetworkPolicies vs Client-Maintained NetworkPolicies

If a client Pod C in Namespace C owned by Operator C needs to talk to
a Service S in Namespace S owned by Operator S, then who should create
the NetworkPolicy, Operator C or Operator S?

Since the policy is going into a Namespace owned by Operator S, it
seems generally more in line with common practice to have Operator S
create the policy. This is also simpler, since Operator S knows the
Namespace will exist when it creates the policy, while Operator C
might need to wait for it to be created.

This also seems better since if the owner of the service creates the
policy, that means they definitely are aware that the other operator
is depending on having access to them. Whereas if the client operator
created the policy itself, it's possible the owner of the service
might never realize (or might eventually forget) that this particular
client was depending on it, and might change things in a way that
would break that client.

#### Pod-Level NetworkPolicies vs Namespace-Level NetworkPolicies

Using the same example of a client Pod C in Namespace C owned by
Operator C that needs to talk to a Service S in Namespace S owned by
Operator S, what exactly should the NetworkPolicy say?

The most-restrictive possibility would be to say "Pod C in Namespace C
is allowed to connect to Service S in Namespace S". But it would be
simpler and possibly more-future-proof to just say "All Pods in
Namespace C can connect to all Pods in Namespace S".

(In terms of efficiency, namespace-only rules are more efficient in
openshift-sdn (since they result in a single match-on-VNID rule), but
per-pod rules are generally more efficient in ovn-kubernetes (since
namespace-based rules have to be rewritten in terms of individual pod
IPs there).)

Assuming the policy is going to be created by the operator managing
the service, not by the operator managing the client, it may be best
to be more restrictive on the service side, and less restrictive on
the client side ("All Pods in Namespace C can connect to Service S in
Namespace S"); that way if the client operator makes minor changes to
its pods, it's less likely to require changes to a NetworkPolicy in
another component.

#### "Restrict when Restricted" vs "Allow when Allowed"

Broadly speaking, there are two ways the policies could be written:

  1. When the restricted mode is disabled, operators do nothing
     special, just like now. When restricted mode is enabled,
     operators create restrictive NetworkPolicies.

  2. Operators always create restrictive NetworkPolicies, but when the
     restricted mode is disabled, each system namespace also has a
     permissive NetworkPolicy allowing all inbound access. (Because of
     how NetworkPolicy works, the permissive policies would override
     the restrictive ones).

       a. The permissive NetworkPolicies could be created by the
          individual operators along with the restrictive policies.

       b. Alternatively, all of the permissive NetworkPolicies could
          be created by the Cluster Network Operator.

Option 2 is preferable to Option 1 because in general, NetworkPolicy
is easier to work with when namespaces always have either a "default
deny" or "allow to all" baseline policy. (See Appendix for gory
details.)

Of the two sub-options, 2b ("CNO creates permissive policies") seems
nice because it means that individual operators wouldn't need to track
the restricted-vs-open option; they'd just always configure themselves
as restricted, and CNO would fix things up if they were supposed to be
open.

However, 2b would be harder to transition to from the current
always-open state, since we have to ensure that no component ever
creates a restrictive NetworkPolicy unless CNO has already created a
permissive NetworkPolicy (if one is needed). The easiest way to do
that would be to add the "permissive NetworkPolicy" feature to CNO one
full Y-stream release before adding the restrictive NetworkPolicies to
other operators...

#### OpenShift SDN Multitenant Mode

OCP 4.x still supports the old pre-NetworkPolicy "Multitenant" mode
where all namespaces are isolated from each other by default. This
doesn't really work as well in OCP 4.x as it did in OCP 3.x, since
there are now many more system namespaces that need to talk to each
other.

In Multitenant mode, it is not possible to fine-tune permissions; any
namespace which _either_ needs to be accessible from arbitrary other
namespaces, _or_ needs to be able access arbitrary other namespaces,
must be joined to the "default" namespace. In 4.7.0, this set of
namespaces is:

- `openshift-dns`
- `openshift-image-registry`
- `openshift-ingress`
- `openshift-kube-apiserver`
- `openshift-monitoring`
- `openshift-operator-lifecycle-manager`
- `openshift-user-workload-monitoring`

We also create a second "control-plane" set of joined namespaces,
which are all able to access each other, but which remain isolated
from other (non-default) namespaces:

- `kube-system`
- `openshift-ansible-service-broker`
- `openshift-apiserver`
- `openshift-authentication`
- `openshift-authentication-operator`
- `openshift-etcd`
- `openshift-etcd-operator`
- `openshift-service-catalog-apiserver`
- `openshift-service-catalog-controller-manager`
- `openshift-template-service-broker`

(The presence of `kube-system` there is probably a mistake, since
there are no pods in `kube-system`...)

These lists occasionally need to be adjusted. It would be good if we
could find a way to figure these out automatically, either based on
analyzing the NetworkPolicies that the operators create, or by having
the operators annotate their namespaces with hints to openshift-sdn.

### Risks and Mitigations

1. As with any security-related improvement, there is the possibility
that we will do something wrong and not actually improve security.

2. It is possible that flaws in the NetworkPolicy implementation of
OpenShift SDN, OVN Kubernetes, or a third-party network plugin might
allow access to services that were supposed to have been blocked off.
However, we will still also be using TLS certificates to authenticate,
so this would not result in an immediate compromise of the system. The
NetworkPolicies protect against bugs in the TLS implementation, and
the TLS certificates protect against bugs in the NetworkPolicy
implementation. If either one works correctly, then the system is
secure.

3. If we fail in the opposite direction and make the system "too
secure" (eg, breaking some uncommon use case that doesn't get caught
by e2e testing, or breaking OLM operators), then the mitigation is
just that customers using that feature/operator will not be able to
use the restrictive-NetworkPolicies feature until we fix the
permissions.

## Design Details

### Test Plan

**Note:** *Section not required until targeted at a release.*

### Graduation Criteria

#### Dev Preview

- CNO begins creating permissive NetworkPolicies in all `openshift-*`
  namespaces that existed up until this point. A config option is
  added to `network.operator.openshift.io` to enable restrictive mode,
  with the documentation clarifying that the feature is in Dev Preview
  and will break your cluster.

- `openshift-*` namespaces added after this point are required to have
  restrictive NetworkPolicies. This is enforced by e2e tests.

- There is a periodic e2e job that runs in fully-restrictive mode, but
  it is not intially expected to pass.

- Other operators begin testing with restrictive NetworkPolicies.
  (We'll have to make it possible for an operator to disable its own
  permissive NetworkPolicy without also disabling permissive
  NetworkPolicies cluster-wide, for testing purposes.)

#### Dev Preview -> Tech Preview

- The periodic e2e test that runs in fully-restrictive mode can now
  pass. We add some presubmit jobs.

- Namespaces that are "obviously" not end-user-facing (eg, that
  contain no Services) could perhaps be made restricted.

- We update the documentation on the "restrictive mode" config option
  to note that the feature is in Tech Preview and is expected to work,
  but is not yet fully supported.

#### Tech Preview -> GA

- More testing
- Sufficient time for feedback

### Upgrade / Downgrade / Version Skew Strategy

As discussed earlier, for the initial upgrade from a release that does
not have this feature, to a release that does have this feature, we
must ensure that the feature is designed in a way that we do not
temporarily restrict access accidentally during the upgrade,
regardless of the order that components are upgraded in.

In the future, when operators want to change their NetworkPolicies, or
the Namespace labels that are used by them, they will need to ensure
that they change them in ways that will not create problems on
upgrade. (eg, if Service A needs to allow access from Namespace B, but
B's Operator wants to change the labels on Namespace B in the next
release, this would need to be done in a way that would not break the
`namespaceSelectors` in A's NetworkPolicies, regardless of whether A
is updated before B or B before A.) This should be ensured by e2e
testing.

When downgrading from a version that supports this feature to a
version that does not support this feature, the simplest way to ensure
the downgrade goes smoothly would be to require that the cluster is in
the "unrestricted" mode before downgrading. (If CNO is not working
correctly at the time of the downgrade, the permissive NetworkPolicies
could be created by hand instead.) After downgrading, the
administrator could delete all NetworkPolicies in the `openshift-*`
namespaces.

## Implementation History

- Initial proposal: 2020-01-18

## Appendix: Why Explicit "Allow All" Policies Are Good, and Implicitly Allowing All is Bad

NetworkPolicy has unfortunate semantics:

- If a Pod is not targeted by any NetworkPolicies, then it accepts
  all connections.

- If a Pod is targeted by at least one NetworkPolicy, then it
  accepts only the connections that are allowed by any of the
  NetworkPolicies that target it.

This means that the first NetworkPolicy that targets a pod flips the
pod from "accept all connections" to "accept only connections that are
explicitly listed as being accepted".

So, imagine you have a Namespace `implicitly-open` with no
NetworkPolicies, a Namespace `explicitly-open` containing a
NetworkPolicy that says "all pods accept all connections", and a
Namespace `closed` containing a NetworkPolicy that says "all pods
accept connections from Namespace `X`":

- `implicitly-open`: all pods accept all connections
- `explicitly-open`: all pods accept all connections
- `closed`: all pods accept connections only from Namespace `X`

Now imagine that a new external component needs to ensure that it is
able to access each of these namespaces. So, it adds a new policy
saying "all pods accept connections from Namespace `Y`" to each of the
three namespaces. The result is now:

- `implicitly-open`: all pods accept connections only from Namespace `Y` and nowhere else
- `explicitly-open`: all pods accept all connections
- `closed`: all pods accept connections from either Namespace `X` or Namespace `Y`

The `explicitly-open` and `closed` namespaces work as expected; they
allow all the connections they used to allow, plus the new connections
from Namespace `Y`. But the `implicitly-open` namespace is now broken,
because the addition of the first NetworkPolicy to the namespace
removed the implicit "allow all" that was there before.

For this reason, it's better to add explicit "allow all" policies to
namespaces to implement the "unrestricted" case, rather than falling
back on the default implicit "allow all" behavior.
