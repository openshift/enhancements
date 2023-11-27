---
Title: Policy for Breaking Changes
authors:
  - "@dhellmann"
  - "@deads2k"
  - "@derekwaynecarr"
creation-date: 2023-11-16
last-updated: 2023-11-16
status: informational
---

# Policy for Breaking Changes

## Upstream Kubernetes APIs

Breaking changes cannot be completely avoided. API version deprecation
is a part of the process of evolving Kubernetes APIs to reach stable
versions (v1, etc.). Beta APIs are automatically removed from upstream
Kubernetes after 6 releases. Each deprecation of a Kubernetes beta API
is announced at least 3 releases before the API is removed, and
usually (but not always) after a new version of the API (either
another beta or a stable version) is available, as described in the
[deprecation
policy](https://kubernetes.io/docs/reference/using-api/deprecation-policy/). We
recommend that users move to new APIs as soon as they are available to
allow for smooth transition time across multiple OpenShift releases
before older versions of the APIs are removed.

Each OpenShift release is updated to be based on a new version of
Kubernetes. Red Hat has some influence over Kubernetes API deprecation
and removal, but we do not have (or want) complete control of the
community schedule or processes. We therefore do not commit to
Kubernetes API (resource types in a k8s.io group) removals happening
only in specific kinds of OCP releases (EUS or non-EUS). We work
within the community to ensure the declared policies, especially about
advanced notice of removals, are followed.

One way we minimize the impact of the evolution of upstream APIs is to
not enable new beta APIs by default. This policy was adopted in
[Kubernetes
1.24](https://github.com/kubernetes/enhancements/tree/master/keps/sig-architecture/3136-beta-apis-off-by-default)
and OCP 4.11. All k8s.io beta APIs that have already been enabled will
be removed in [Kubernetes
1.29](https://kubernetes.io/docs/reference/using-api/deprecation-guide/#v1-29)
and OCP 4.16. In limited cases, future k8s.io beta APIs can be made
available in OpenShift for technical preview and testing. APIs only
available through the tech preview flag should not be used in
production clusters.

## OpenShift APIs

For openshift.io APIs, we consider changes within the same non-beta
version (v1, v2, etc.) of API definitions that prevent them from being
backwards compatible to be *bugs*. When breaking changes in stable
APIs are identified, we must fix them.

We intend to avoid producing new openshift.io beta APIs, and rely
instead on differentiating between tech preview features and GA
features. Preview versions of APIs, enabled in a cluster by turning on
the tech preview feature flag, are not guaranteed to be stable.

We also, as part of transitioning to entirely new openshift.io APIs,
will deprecate an API without removing it. The deprecated API is
supported across upgrades, but after a few releases in newly created
clusters only the new type of API resources will be present. We
communicate deprecations of this sort through release notes on the
release where the API is deprecated.

Some APIs behave differently on different deployment topologies. For
example, in hosted control plane or managed services clusters some
APIs are read-only or removed entirely if they are not relevant. The
"API availability per product installation topology" section of
[Understanding Compatibility
Guidelines](https://docs.openshift.com/container-platform/4.14/rest_api/understanding-compatibility-guidelines.html)
covers this in more detail.

## Command Line Tools

All CLI elements default to API tier 1 unless otherwise noted or the
CLI depends on a lower tier API. Refer to the [product
documentation](https://docs.openshift.com/container-platform/4.14/rest_api/understanding-api-support-tiers.html#deprecating-cli-elements_understanding-api-tiers)
for more details.

## Documented Policies

We have more information about the API change policy in the knowledge
base article [Navigating Kubernetes API deprecations and
removals](https://access.redhat.com/articles/6955985) and product
documentation in [Understanding API
tiers](https://docs.openshift.com/container-platform/4.14/rest_api/understanding-api-support-tiers.html)
and [Understanding API compatibility
guidelines](https://docs.openshift.com/container-platform/4.14/rest_api/understanding-compatibility-guidelines.html).

## Add-ons

Note that life-cycle and API deprecation policies do not necessarily
extend beyond the core of OpenShift. Operators and other products that
define their own APIs make their own decisions about support tiers.

## Other Behavior Changes

Not every potentially breaking change is an "API change". We also make
security and stability changes in OpenShift, such as the more
restrictive default pod security admission enforcement and the
migration from OpenShiftSDN to OVN-K. Those changes can have the
effect of breaking some workloads that rely on aspects of the old
default behavior. We provide advance notice of those changes through
blog posts, release notes, knowledge base articles, alerts within the
cluster, and other communications, with guidance for updating
workloads to make the transition cleanly.

## Recommendations For Mitigating the Impact of Changes

### Stay Up To Date

We recommend that users move to new APIs as soon as they are available
to allow for smooth transition time across multiple OpenShift releases
before older versions of the APIs are removed.

### Do Not Use Tech Preview In Production

APIs only available through the tech preview flag should not be used
in production clusters.

### Test With Early Releases

We provide engineering candidates of pre-release versions of OpenShift
roughly every 3 weeks as the output of each development sprint and
release candidates are published more often starting a few weeks
before the announced GA date for each release. The best way to be
confident that workloads are compatible with upcoming releases of
OpenShift is to test with these pre-release versions.

### Monitor Alerts

Another way to catch potential issues in advance is to watch the
alerts in OpenShift that are triggered by the use of deprecated APIs.
