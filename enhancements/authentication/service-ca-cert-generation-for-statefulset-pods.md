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

Similarly to the existing annotation on services,
a `service.beta.openshift.io/serving-cert-secret-name` annotation on a StatefulSet
requests generating a single secret with a set of individual certificate+key pairs
named `{0,1,2,3,…}.{crt,key}`.
The subject of `$podID.crt` is the pod’s unique DNS identity,
`${statefulSet.name}-${podID}.${service.name}.${service.namespace}.svc` .
Each pod from a StatefulSet can then pick and use its own certificate.

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

- Provide each pod in a StatefulSet with a separate secret containing only the certificate+key
for that single pod:

  This is not currently possible,
  because the StatefulSet pod template does not have any templating mechanism
  that would allow a ${podID}-dependent value to be used for a secret name or a volume mount;
  there’s only StatefulSet.spec.volumeClaimTemplates , which is not practical to use.

  Upstream I could find https://github.com/kubernetes/kubernetes/issues/40651 , which only ends up
  discussing environment variables (which work via the downward API).

  The per-${podID} behavior can be simulated using an init container
  (e.g. https://itnext.io/kubernetes-statefulset-initialization-with-unique-configs-per-pod-7e02c01ada65 ),
  by making a full set of certificates available to all pods
  and relying on an init container to choose one certificate and make that one
  (and none of the others) available to the actual server container.
  
  That’s cumbersome, but ultimately probably secure enough — if it makes sense to protect
  the individual StatefulSet pods from each other at all.

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

- Generate a single certificate that signs multiple ~unrelated services as subjects
  (combining the StatefulSet per-pod subjects and ClusterIP services,
  or a ClusterIP service with NodePort service):

  In the most general case, this is not possible at all, because services choose pods by pod labels,
  i.e. the set of services served by a single pod at once (which require a shared certificate)
  can only be determined once such a pod is created —
  and at that point it’s too late to generate a certificate for that pod.

  If the different services are served on different ports,
  they can trivially use different certificates.
  Even for a single port exposed via the different services,
  the server should be able to use different certificates based on SNI nowadays
  (e.g. via https://golang.org/pkg/crypto/tls/#Config.Certificates ).

## Proposal

Similarly to the existing annotation on services,
a `service.beta.openshift.io/serving-cert-secret-name: $secretName` annotation on a `StatefulSet`
requests the `service-ca` controller to generate a single secret `$secretName`
with a set of individual certificate+key pairs named `{0,1,2,3,…}.{crt,key}`.
The subject of `$podID.crt` is the pod’s unique DNS identity,
`${statefulset.name}-${podID}.${service.name}.${service.namespace}.svc` .
Each pod from a `StatefulSet` can then pick and use its own certificate.

Creating a `StatefulSet` triggers both a `Secret` creation and creating the `StatefulSet` pods,
which race against each other.
So, each individual pod **must** fail if a certificate for its individual identity is not found.
We also generate a few more certificate+key pairs than necessary,
to allow scaling up without such pod failures.

### User Stories

#### Creating a TLS-protected StatefulSet

- Author of the application, or of the container image wrapping the application,
  ensures that the pod
  - Determines its identity $podID (already the case)
  - Loads `$podID.{crt.key}` on startup from a separate directory
  - Exits with a failing status if those files are missing
  - Uses the certificate+private key for accepting intra-StatefulSet connections
  - Trusts the Service CA for making outgoing intra-StatefulSet connections,
    using the signed `${statefulset.name}-${podID}.${service.name}.${service.namespace}.svc`
    (or the same with `….cluster.local`) host names.
- The StatefulSet for the application
  is updated to contain:
  - An annotation `service.beta.openshift.io/serving-cert-secret-name: $secretName`
  - In the pod template, a volume referencing secret `$secretName`
  - In the pod’s container template, a `volumeMount` mounting that volume
    in the application-expected directory
- The application administrator creates the StatefulSet object.
- The StatefulSet controller starts creating pods for the StatefulSet.
  They will initially fail because `$secretName` is missing or does not have the expected
  `$podID.{crt,key}` files, causing the StatefulSet controller to wait and retry.
- Concurrently, the Service CA controller observes the creation of the StatefulSet,
  and creates a secret `$secretName` in the same namespace, with `{0,1,2,3,…}.{crt,key}` keys
  matching the hostnames of the StatefulSet pods; the number of certificate/key pairs may be
  larger than the requested number of replicas of the StatefulSet.
- After the secret is created/updated, and the StatefulSet controller retries,
  the pods start up succesfully.

#### Scaling the StatefulSet up

- An admin increases the StatefulSet’s requested replica count.
- The StatefulSet controller starts creating the new pods.
  If the certificate/key pairs already exist in the secret (e.g. if just addding one pod)
  the pods start up succesfully;
  if not, the pods fail, and the StatefulSet controller will wait and retry.
- Concurrently, the Service CA controller observes the replica count increase;
  if the existing secret does not have enough certificate/key pairs (including a margin),
  it updates the secret so that it does.
  (It’s unspecified whether pre-existing certificate/key pairs are reused or replaced.)
- After the secret is created/updated, and the StatefulSet controller retries,
  the new pods start up succesfully.

#### Scaling the StatefulSet down

- An admin decreases the StatefulSet’s requested replica count.
- The StatefulSet controller starts tearing down the extra pods.
- Concurrently, the Service CA controller observes the replica count change;
  it’s unspecified whether that causes the now unnecessary certificates/keys to be removed
  from the secret, or perhaps even revoked by the CA.

#### Strictly isolating the StatefulSet pods’ certificates

- In addition to designing the StatefulSet as the basic case describes,
  the applicaton admin updates the StatefulSet to:
  - Add an `emptyDir` volume to the StatefulSet,
    and mount it in the application’s pods where the certificates are expected
  - Add an `initContainer` to the StatefulSet, which:
    - mounts that `emptyDir` volume above
    - mounts the generated secret
    - discovers the individual $podID identity of the pod within the StatefulSet,
      and copies that one certificate/key pair into the `emptyDir` volume.
- Deploying the StatefulSet otherwise works just like in the base case.

#### Certificate expiration, manual secret regeneration, CA rotations

Works substantially the same as for Service certificates generated by the Service CA
(notably currently requiring the admins to manually trigger a restart of affected pods
after certificates are regenerated).

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
but the author of the StatefulSet would have to manually refer to all of these Secrets
in the Pod template
(and to add more references, or remove some, on scaling the StatefulSet up, or down).

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

TBD?

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

## Implementation History

WIP PR: https://github.com/openshift/service-ca-operator/pull/144

## Drawbacks

- The pod vs. secret creation race is inelegeant, and causes expected pod initialization failures.

## Alternatives

- Manage the StatefulSet certificates manually, using some other CA.
- For the design that creates a single large secrets with multiple certificates/keys:
  Add a more general templating mechanism to StatefulSet , either as something specific to secrets
  or some very general text/YAML substitution mechanism.

## Infrastructure Needed [optional]

N/A
