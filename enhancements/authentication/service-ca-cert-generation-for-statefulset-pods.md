---
title: service-ca-cert-generation-for-statefulset-pods
authors:
  - "@mtrmac"
reviewers:
  - TBD
approvers:
  - TBD
creation-date: 2021-03-01
last-updated: 2021-03-01
status: implementable
see-also:
replaces:
superseded-by:
---

# Service CA Certificate Generation for StatefulSet Pods

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

The Service CA can be used to automatically generate certificates for StatefulSets,
to allow TLS-protected communication between StatefulSet pods.

The user adds a `service.beta.openshift.io/serving-cert-secret-name: $secretName` annotation,
and a `$secretName` secret is generated, which can be mounted in the StatefulSet’s pods.

Creating a StatefulSet triggers both a Secret creation and creating the StatefulSet pods,
which race against each other.
So, the pods **must** fail if a certificate for their identity is not found.

## Motivation

StatefulSet-managed pods often need to communicate with each other.
This traffic may need to be TLS-protected; because it is cluster-internal,
and necessary for deploying such a StatefulSet, letting the cluster automatically
manage the certificates is a natural extension to the same feature already
provided by the Service CA to allow TLS use with services.

### Goals

Allow deploying StatefulSets with pods that communicate with each other using TLS,
without having to manually generate certificates for these pods.

### Non-Goals

- Provide certificates for `127.0.0.1`, `localhost`, or NodeIP service endpoints:

  Giving out certificates from widely-accepted CAs (like the service CA)
  with these names makes no sense because they don’t specify any service identity,
  e.g. any compromised serviceA would get a “localhost” certificate
  that is acceptable for connections from a victim serviceB.

  (Reportedly some OSes do actually query `localhost` on DNS on the net:
  https://archive.cabforum.org/pipermail/public/2015-June/005673.html )

  If a client wants to connect to _only_ a “true localhost” service,
  a service CA-signed certificate does not provide that property.
  The best way to do that is to do it directly inside the process,
  without using TLS (and the associated cryptography overhead) at all.
  Second best is to rely on known properties of the environment,
  i.e. to assume that 127.0.0.1 is localhost-only
  and to use a non-TLS connection on that address.

  If a _pod_ had to use TLS for localhost for some reason, the closest to a right way to do that
  (relying on TLS to verify that only loopback connections happen) is to:
  - Create a temporary CA (specific to the individual pod for)
  - Generate, and have that CA sign a localhost certificate (specific to the individual pod)
  - Set up the CA certificate as a trusted root
  - _Irreversibly erase_ the CA private key
  - Set up the pod with the generated localhost certificate+private key.
  - (This doesn’t benefit from service-ca involvement at all.)
  
  Similarly single-purpose CA could be set up for NodeIP services.

## Proposal

Similarly to the existing annotation on services,
a `service.beta.openshift.io/serving-cert-secret-name: $secretName` annotation on a `StatefulSet`
requests the `service-ca` controller to generate a single secret `$secretName`
with a set of individual certificate+key pairs named `{0,1,2,3,…}.{crt,key}`.
Each pair is intended for one of the StatefulSet pods:
the subject of `$podID.crt` is the pod’s unique DNS identity,
`${statefulset.name}-${podID}.${service.name}.${service.namespace}.svc` .
Each pod from a `StatefulSet` can then pick and use its own certificate.

Creating a `StatefulSet` triggers both a `Secret` creation and creating the `StatefulSet` pods,
which race against each other.
So, each individual pod (either the primary container, or an init container)
**must** fail if a certificate for its individual identity is not found.
We also generate a few more certificate+key pairs than necessary,
to allow scaling up without such pod failures.

### User Stories

#### Creating a TLS-protected StatefulSet

- The StatefulSet for the application
  is updated to contain:
  - An annotation `service.beta.openshift.io/serving-cert-secret-name: $secretName`
  - In the pod template, a volume referencing secret `$secretName`
  - In the pod’s container template, a `volumeMount` mounting that volume
    in a application-expected directory
- Author of the application, or of the container image wrapping the application,
  ensures that the pod
  - Determines its identity `$podID` (already the case)
  - Loads `$podID.{crt,key}` on startup from the directory where `$secretName` is mounted.
  - Exits with a failing status if those files are missing
    (this could happen either in the main application pod or in an init container).
  - Uses the certificate+private key for accepting intra-StatefulSet connections
    (this may require changes to support diferent certificates for different services).
  - Trusts the Service CA for making outgoing intra-StatefulSet connections,
    using the signed `${statefulset.name}-${podID}.${service.name}.${service.namespace}.svc`
    (or the same with `….cluster.local`) host names.
- The application administrator creates the StatefulSet object.
- The StatefulSet controller starts creating pods for the StatefulSet.
  They will initially fail because `$secretName` is missing or does not have the expected
  `$podID.{crt,key}` files, causing the StatefulSet controller to wait and retry.
  (It doesn’t matter
  whether the `statefulSet.spec.podManagementPolicy` is `OrderedReady` or `Parallel`;
  either way the secret will be updated in a single step to contain all necessary certificates,
  and all pod creation attempts will fail until then.)
- Concurrently, the Service CA controller observes the creation of the StatefulSet,
  and creates a secret `$secretName` in the same namespace, with `{0,1,2,3,…}.{crt,key}` keys
  matching the hostnames of the StatefulSet pods; the number of certificate/key pairs may be
  larger than the requested number of replicas of the StatefulSet.
- After the secret is created/updated, and the StatefulSet controller retries,
  the pods start up succesfully.

#### Scaling the StatefulSet Up

- An admin increases the StatefulSet’s requested replica count.
- The StatefulSet controller starts creating the new pods.
  If the certificate/key pairs already exist in the secret (e.g. if just adding one pod)
  the pods start up succesfully;
  if not, the pods fail, and the StatefulSet controller will wait and retry.
- Concurrently, the Service CA controller observes the replica count increase;
  if the existing secret does not have enough certificate/key pairs (including a margin),
  it updates the secret so that it does.
  (It’s unspecified whether pre-existing certificate/key pairs are reused or replaced.)
- After the secret is created/updated, and the StatefulSet controller retries,
  the new pods start up succesfully.

#### Scaling the StatefulSet Down

- An admin decreases the StatefulSet’s requested replica count.
- The StatefulSet controller starts tearing down the extra pods.
- Concurrently, the Service CA controller observes the replica count change;
  it’s unspecified whether that causes the now unnecessary certificates/keys to be removed
  from the secret.

#### Changing the StatefulSet Pod Template

e.g. to update the deployed container image.

- An admin edits the StatefulSet.spec.template
- (Assuming no changes to the requested replica count or to the secret-name annotation,
  the Service CA does nothing:
  the already-generated secret and its certificates continue to be valid.)
- The StatefulSet controller performs the update (or not) as usual; values of `updateStrategy`
  don’t change how the Service CA operates.

#### Manually Isolating the StatefulSet Pods’ Certificates

To ensure that a compromised StatefulSet pod can’t impersonate other pods from the
same StatefulSet
(if this goal is achievable for the application,
i.e. if the pods treat the intra-StatefulSet communication as untrusted
and can plausibly protect against a rogue StatefulSet member):

- In addition to designing the StatefulSet as the basic case describes,
  the application admin updates the StatefulSet to:
  - Add an `emptyDir` volume to the StatefulSet,
    and mount it in the application’s pods where the certificates are expected
  - Add an `initContainer` to the StatefulSet, which:
    - mounts that `emptyDir` volume above
    - mounts the generated secret
    - discovers the individual $podID identity of the pod within the StatefulSet,
      and copies that one certificate/key pair into the `emptyDir` volume.
- Deploying the StatefulSet otherwise works just like in the base case.

#### Certificate Expiration, Manual Secret Regeneration, CA Rotations

Works substantially the same as for Service certificates generated by the Service CA
(notably currently requiring the admins to manually trigger a restart of affected pods
after certificates are regenerated).

If failures for Service certificates are reported by an annotation on a Service,
for StatefulSet certificates they are reported by an annotation on the StatefulSet.

### Implementation Details/Notes/Constraints

#### The Pod Creation vs. Secret Creation Race

As described above,
creating a StatefulSet triggers both pods and the secret to be created concurrently,
and the application must be designed to support that by triggering the pod to be re-created.

To avoid the race in predictable situations,
the controller creates some _extra_ certificates on top of the StatefulSet.spec.replicas request,
so that a future small scale-up can succeed at the first try.

The implementation currently hard-codes creating 5 extra certificates,
or 30% more than spec.replicas, whichever is larger.

If the StatefulSet is scaled down, we don’t immediately delete the extra certificates;
that would just be extra traffic for no benefit.
The secret is trimmed to a smaller size only when re-created
(on certificate expiry or a resync, e.g. if the user manually deletes or corrupts the secret).

#### Annotations

StatefulSets now use the same annotations as Services (`serving-sert-secret-name` as input, `serving-cert-{created-by,generation-error,generation-error-num}` as output.

Secrets can now have `originating-StatefulSet-{name,uid}` annotations
to link back to the StatefulSet;
this works just like the previous `originating-service-*` annotations.

##### Alpha Annotations Not Used

FIXME: Use Alpha Annotations Exclusively.

The StatefulSet controllers only use `service.beta.openshift.io` annotations,
not the `alpha` annotation which are supported for services.
This is new code, so supporting an older version seems unnecessary.
(Or should this feature start out as `alpha`?)

#### The OpenShift “Service UID” Certificate Extension Is Not Used
The service-ca certificates created for services
include an OpenShift-specific “Service UID” extension,
per https://github.com/openshift/origin/pull/12413 to identify intra-cluster ElasticSearch traffic.

That extension is specifically documented to contain a service UID,
so it can’t be directly used for StatefulSets,
and it’s not obvious that looking up the UID of a service
referenced by StatefulSet.spec.serviceName makes much sense —
that StatefulSet→Service reference is UID-agnostic,
and it’s a different set of certificates
that was not anticipated by the current consumers of the “service UID” certificate extension.

If necessary, we can allocate and define a new “StatefulSet UID” extension,
or look up a Service UID for StatefulSet.spec.serviceName,
or maybe even store a StatefulSet UID in the “Service UID”  extension, in the future;
it seems unnecessary for a first version of this feature.

#### Secret Size Grows with Replicas
Storing all certificates for a StatefulSet in a single Secret
could eventually make the Secret pretty large.
Hopefully that’s not a limit we are likely to hit?

It would be possible to limit the maximum Secret size
(e.g. to only store up to 100? certificates in a single Secret),
and create multiple secrets (using some naming convention) to hold all the certificates,
but the author of the StatefulSet would have to manually refer to all of these Secrets
in the Pod template
(and to add more references, or remove some, on scaling the StatefulSet up, or down).

If the secret grew so large that it would be rejected by the API server,
the Service CA controller would record that error in an annotation on the requesting StatefulSet,
and after a few attempts (10 in the current implementation)
stop trying to generate the secret
(TO FIX: until the built-in controller resync period,
at which point the current implementation tries again).

### Risks and Mitigations

The service creation vs. pod creation race
will introduce some ~unavoidable pod initialization failures,
perhaps causing alerts.
Those should be limited to initial deployments, or rapid scale-ups, of StatefulSets,
hopefully rare operations where the admins of the StatefulSet are aware of the
secret generation properties.

Otherwise,the design and implementation closely follows the existing certificate generation
code for services, so it should not introduce significant new issues.

## Design Details

### Open Questions

N/A

### Test Plan

Unit and e2e tests similar to the existing service certificate tests, both in structure and scope.

### Graduation Criteria

FIXME: TBD?

Given the close (but not 100%) correspondence
with the existing service certificate generation feature,
proposing to use `service.beta.openshift.io/serving-cert-secret-name` annotations
that are already used for that feature.

### Upgrade / Downgrade Strategy

This is a new feature.
Upgrading to a version that adds the feature should not affect any existing services or secrets.

Downgrades to a version that does not implement this feature would obviously cause
new secrets to not be generated, and existing secrets to no longer react to StatefulSet scaling;
the existing secrets would continue to work (as long as the Service CA, in general, is trusted),
probably at least enough to allow downgraded operators to revert to whatever they were doing before.

Otherwise, the already-generated secrets are not directly affected by upgrades/downgrades,
and should continue to work.

### Version Skew Strategy

N/A, there is currently a single version of the feature.

In general,
the generated certificates work independently of the version of the `service-ca` controller,
so they should not be immediately affected by upgrades of that controller.

A future version of the `service-ca` controller could, after an upgrade,
detect secrets created by an older version
(by lack of some future annotations on the generated secret,
or by inspecting the created certificates),
and act on that e.g. to regenerate new forms of the secret.

## Implementation History

WIP PR: https://github.com/openshift/service-ca-operator/pull/144

## Drawbacks

- The pod vs. secret creation race is inelegeant, and causes expected pod initialization failures.

## Alternatives

### Do Nothing
Manage the StatefulSet certificates manually, using some other CA.

### Modify StatefulSet to Support Individual Per-Pod Secrets
Instead of generating a single large secret with multiple certificates/keys,
provide each pod in a StatefulSet with a separate secret containing only the certificate+key
for that single pod.

That would automatically prevent a compromised StatefulSet pod from impersonating
any other pod in the StatefulSet.

The StatefulSet pod template does not have any templating mechanism
that would allow a ${podID}-dependent value to be used for a secret name or a volume mount;
there’s only StatefulSet.spec.volumeClaimTemplates , which is not practical to use.

(Upstream https://github.com/kubernetes/kubernetes/issues/40651 only ends up
discussing environment variables (which work via the downward API).)

So, we would need to add a more general templating mechanism to StatefulSet,
either as something specific to secrets
or some very general text/YAML substitution mechanism.

There would still be a pod vs. secret creation race.

Setting up a StatefulSet so that pods can’t impersonate each other
would no longer require admins to manually use init containers;
the init container approach is cumbersome, but ultimately probably secure enough.

### Generate a Single Wildcard Certificate for the StatefulSet
This would simplify the implementation, but the too broad subject introduces extra risk.

https://github.com/kubernetes/dns/blob/master/docs/specification.md
only specifies what DNS records must exist, not what must not exist,
so such a certificate could match unwanted DNS entries.

Notably once a wildcard certificate is created,
it’s no longer possible to rely on TLS to prevent a compromised StatefulSet pod
from impersonating a different pod from that StatefulSet.

(The simple variant of just generating a `*.${service.name}…` without including the
StatefulSet name at all also does not differentiate between different StatefulSets
using the same headless service for hostnames.
The more specific variant of `${statefulSet.name}-*.${service.name}…` only avoids this
to an extent, as long as StatefulSet names are not prefixes of other StatefulSet names
(`database` vs. `database-staging`),
and there’s still the general downside of inability prevent impersonation within
the StatefulSet.)

### Annotate a Service Instead of a StatefulSet
This would certainly be simpler
for generating the very generic wildcard certificates (`*.${service.name}…`):
they could be generated with the service, i.e. before creating the StatefulSet,
and the pod vs. secret creation race would not exist.
OTOH that only works well with such generic wildcards, which are undesirable.

For generating individual pod certificates
(or the more constrained `${statefulSet.name}-*.${service.name}…` wildcard variant),
where the certificate subject is
`${statefulSet.name}-${podID}.${service.name}.${service.namespace}.svc`,
it is both more correct and simpler to annotate StatefulSets:

- In the most general case,
  there could be two or more different StatefulSets using the same headless service,
  with one of the StatefulSets requesting a set of certificates,
  and the other one not requesting certificates
  (e.g. because they only communicate using shared storage, or some HW-specific mechanism).
  So, conceptually it’s a better fit to place the certificate request on the StatefulSet
  than on the service.
- Implementation-wise, to generate the
  `${statefulSet.name}-${podID}.${service.name}.${service.namespace}.svc` certificate subjects,
  the Service CA controller _must_ observe creation of StatefulSet objects.
  OTOH it does not actually have to observe Services, because the same value is available as
  `${statefulSet.name}-${podID}.${statefulSet.spec.serviceName}.${statefulSet.namespace}.svc`.

### Generate Multi-Service Certificates
So that applications can simply use a single certificate on all listening ports,
generate a single certificate that signs multiple ~unrelated services as subjects
(combining the StatefulSet per-pod subjects and ClusterIP services,
or a ClusterIP service with NodePort service),

In the most general case, this is not possible at all, because services choose pods by pod labels,
i.e. the set of services served by a single pod at once (which require a shared certificate)
can only be determined once such a pod is created —
and at that point it’s too late to generate a certificate for that pod.

Generating multi-subject certificates also makes it more difficult
to reason about security properties and to change service routing:
once a single certificate for (serviceA+serviceB) exists,
even if the implementation is later changed
by moving serviceB to a separate set of pods,
a compromised pod with access to the earlier combined certificate
would allow an attacker to continue to impersonate both serviceA and serviceB
(e.g. because the Service CA does not currently allow revoking certificates).

If the different services are served on different ports,
they can trivially use different certificates.

Even for a single port exposed via the different services,
the server should be able to use different certificates based on SNI nowadays
(e.g. via https://golang.org/pkg/crypto/tls/#Config.Certificates ).

## Infrastructure Needed [optional]

N/A
