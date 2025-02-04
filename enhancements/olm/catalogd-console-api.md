---
title: catalogd-console-api
authors:
  - "@grokspawn"
  - "@anik120"
reviewers: # Include a comment about what domain expertise a reviewer is expected to bring and what area of the enhancement you expect them to focus on. For example: - "@networkguru, for networking aspects, please look at IP bootstrapping aspect"
  - "@spadgett" # for OCP console
  - "@jhadvig" # for OCP console
  - "@TheRealJon" # for OCP console
  - "@joelanford" # for OLM
  - "@eggfoobar" # for SNO
  - "@csrwng" # for Hypershift
approvers: # A single approver is preferred, the role of the approver is to raise important questions, help ensure the enhancement receives reviews from all applicable areas/SMEs, and determine when consensus is achieved such that the EP can move forward to implementation.  Having multiple approvers makes it difficult to determine who is responsible for the actual approval.
  - "@spadgett"
api-approvers: # In case of new or modified APIs or API extensions (CRDs, aggregated apiservers, webhooks, finalizers). If there is no API change, use "None"
  - None
creation-date: 2025-01-30
last-updated: 2025-02-11
tracking-link: # link to the tracking ticket (for example: Jira Feature or Epic ticket) that corresponds to this enhancement
  - https://issues.redhat.com/browse/OPRUN-3688
see-also:
  - n/a
replaces:
  - n/a
superseded-by:
  - n/a
---
<!--
To get started with this template:
1. **Pick a domain.** Find the appropriate domain to discuss your enhancement.
1. **Make a copy of this template.** Copy this template into the directory for
   the domain.
1. **Fill out the metadata at the top.** The embedded YAML document is
   checked by the linter.
1. **Fill out the "overview" sections.** This includes the Summary and
   Motivation sections. These should be easy and explain why the community
   should desire this enhancement.
1. **Create a PR.** Assign it to folks with expertise in that domain to help
   sponsor the process.
1. **Merge after reaching consensus.** Merge when there is consensus
   that the design is complete and all reviewer questions have been
   answered so that work can begin.  Come back and update the document
   if important details (API field names, workflow, etc.) change
   during code review.
1. **Keep all required headers.** If a section does not apply to an
   enhancement, explain why but do not remove the section. This part
   of the process is enforced by the linter CI job.

See ../README.md for background behind these instructions.

Start by filling out the header with the metadata for this enhancement.
-->

# catalogd-console-api

## Summary

Catalogd currently supports an HTTPS endpoint to access file-based catalog (FBC) contents
but the endpoint provides complete catalog content in a single transaction.  In performance testing with 
proof of concept OCP console work, it proves insufficiently responsive and requires user-agent-side results
 caching of large amounts of catalog data.   
We would like to introduce a new HTTPS endpoint to provide more fine-grained and lower-latency access to 
catalog contents. 

<!--
The `Summary` section is important for producing high quality
user-focused documentation such as release notes or a development roadmap. It
should be possible to collect this information before implementation begins in
order to avoid requiring implementors to split their attention between writing
release notes and implementing the feature itself.

Your summary should be one paragraph long. More detail
should go into the following sections.
--> 

## Motivation
<!--
This section is for explicitly listing the motivation, goals and non-goals of
this proposal. Describe why the change is important and the benefits to users.
-->

The existing `/api/v1/all` endpoint returns the entire FBC, which console or other clients then need 
to process in order to retrieve relevant information. This is very inefficient for 
accesses which can be satisfied by a much smaller record set. 

<!--
redhat-operator-index:    55MB
certified-operator-ind:   32MB
community-operator-index: 29MB
operatorhub.io:           21MB
redhat-marketplace-index:  9MB
-->

Requiring OCP console to make the common RH catalog contents available to users incurs a 4-10 second delay even 
under optimal network conditions, plus large local cache coherency challenges to avoid additional retrievals to 
fulfill further use-cases as a user makes progress towards selecting content to install.


### User Stories

* As a console user, I would like to be able to discover available packages for installation.
* As a console user, I would like to be able to request only catalog context relevant to a specific package.
* As a console user, I would like to be able to discover available package updates.
* As a console user, I would like to be able to view details for a specific channel for a package.
* As a console user, I would like to be able to discover all bundle versions associated with a package 
(and optionally a specific channel).
* As a console user, I would like to be able to view details for a specific bundle version.


### Goals

* Provide an HTTPS catalog content discovery endpoint which fulfills targeted catalog queries with a minimum record set.

<!--
Summarize the specific goals of the proposal. How will we know that
this has succeeded?  A good goal describes something a user wants from
their perspective, and does not include the implementation details
from the proposal.
-->

### Non-Goals

* Redesigning FBC schema to facilitate additional efficiencies.
* Implementing adoption by existing tools (for e.g. kubectl plugins).
* Response scaling beyond ~1024 simultaneous request.
* Implementing API discovery mechanisms.


<!--
What is out of scope for this proposal? Listing non-goals helps to
focus discussion and make progress. Highlight anything that is being
deferred to a later phase of implementation that may call for its own
enhancement.
-->

## Proposal

This proposal introduces an additional HTTPS endpoint to an existing catalogd API.  
The existing HTTPS "all" endpoint will remain as a default option; the user will be 
able to enable this new capability via a feature gate.

<!--
This section should explain what the proposal actually is. Enumerate
*all* of the proposed changes at a *high level*, including all of the
components that need to be modified and how they will be
different. Include the reason for each choice in the design and
implementation that is proposed here.

To keep this section succinct, document the details like API field
changes, new images, and other implementation details in the
**Implementation Details** section and record the reasons for not
choosing alternatives in the **Alternatives** section at the end of
the document.
-->

### Workflow Description

To serve curated data from the FBC, a new HTTPS endpoint will be exposed by 
the existing service under the base URL as detailed in `API Specification`. 
The new endpoint will be derived from `.status.urls.base` following the pattern 
`/api/v1/metas?[...parameters]` and will accept parameters which correspond to
any of the fields of the `declcfg.Meta` catalog 
[atomic type](https://github.com/operator-framework/operator-registry/blob/e15668c933c03e229b6c80025fdadb040ab834e0/alpha/declcfg/declcfg.go#L111):

```golang
 type Meta struct {
	Schema  string
	Package string
	Name	  string
}
```
Query parameters will be logically ANDed and used to restrict response scope.   
This API will be conditionally enabled by an upstream `APIV1MetasEndpoint`
feature gate as part of a downstream `NewOLM{suffix}` style feature gate.

<!--
Explain how the user will use the feature. Be detailed and explicit.
Describe all of the actors, their roles, and the APIs or interfaces
involved. Define a starting state and then list the steps that the
user would need to go through to trigger the feature described in the
enhancement. Optionally add a
[mermaid](https://github.com/mermaid-js/mermaid#readme) sequence
diagram.

Use sub-sections to explain variations, such as for error handling,
failure recovery, or alternative outcomes.

For example:

**cluster creator** is a human user responsible for deploying a
cluster.

**application administrator** is a human user responsible for
deploying an application in a cluster.

1. The cluster creator sits down at their keyboard...
2. ...
3. The cluster creator sees that their cluster is ready to receive
   applications, and gives the application administrator their
   credentials.

See
https://github.com/openshift/enhancements/blob/master/enhancements/workload-partitioning/management-workload-partitioning.md#high-level-end-to-end-workflow
and https://github.com/openshift/enhancements/blob/master/enhancements/agent-installer/automated-workflow-for-agent-based-installer.md for more detailed examples.
-->

### API Extensions

This proposal has no effect on OCP APIs.

### Topology Considerations

#### Hypershift / Hosted Control Planes

We project no impacts, since the catalog service endpoints and data are cluster-scoped. 
Hypershift presently lacks support for OLMv1, but that is likely to come soon.

#### Standalone Clusters

This proposal provides an additional HTTPS endpoint within an existing component service.

#### Single-node Deployments or MicroShift

This proposal provides an additional HTTPS endpoint within an existing component service.

### Implementation Details/Notes/Constraints

#### API Specification

```yaml
openapi: 3.0.2
info:
  title: catalogd-web
  version: 1.0.0
  description: ""
paths:
  /{baseURL}/api/v1/metas:
    get:
      parameters:
        - name: name
          description: query by declcfg.Meta.name
          schema:
            type: string
          in: metas
        - name: package
          description: query by declcfg.Meta.package
          schema:
            type: string
          in: metas
        - name: schema
          description: query by declcfg.Meta.schema
          schema:
            type: string
          in: metas
      responses:
        "200":
          headers:
            Last-Modified:
              schema:
                format: date-time
                type: string
          content:
            application/jsonl:
              schema:
                type: string
          description: JSONL-formatted schemas stream, or empty if no match
      operationId: queryCatalog
      description: Retrieve catalog content corresponding to a catalog content query by name, schema, or package name
    head:
      responses:
        "200":
          headers:
            Last-Modified:
              schema:
                format: date-time
                type: string
          description: Response headers as if the request were performed
    parameters:
      - name: baseURL
        description: The base API URL from the object's status.urls.base
        schema:
          type: string
        in: path
        required: true
      - examples:
          all:
            value: 'Accept: */*'
        name: Accept
        description: Communicates the response encoding(s) this client handles
        schema:
          type: string
        in: header
        required: false
  /{baseURL}/api/v1/all:
    get:
      responses:
        "200":
          headers:
            Last-Modified:
              schema:
                format: date-time
                type: string
          content:
            application/jsonl:
              schema:
                type: string
          description: JSONL-formatted catalog FBC
        "404":
          content:
            text/plain:
              schema:
                type: string
          description: ""
      operationId: getCatalogJSONL
      description: Retrieve all catalog content
    parameters:
      - name: baseURL
        description: The base API URL from the object's status.urls.base
        schema:
          type: string
        in: path
        required: true
```
<!--
What are some important details that didn't come across above in the
**Proposal**? Go in to as much detail as necessary here. This might be
a good place to talk about core concepts and how they relate. While it is useful
to go into the details of the code changes required, it is not necessary to show
how the code will be rewritten in the enhancement.
-->

### Caching Considerations

This proposal intends to provide [RFC7234, Section 2](https://www.rfc-editor.org/rfc/rfc7234#section-2) caching compliance through support for `Last-Modified` and 
`If-Modified-Since` directives. Clients can use these headers to avoid re-downloading unchanged data. 
The server will respond with `304 Not Modified` if the catalog metadata is unchanged.  
                  
Clients are also encouraged to implement local caching for frequently queried metadata.

The existing `all` endpoint also incentivizes clients to conserve resources via local cache to avoid making 
many (potentially duplicate) requests.  However, the OCP console proof of concept 
required what was deemed an unsupportable amount of code, complexity, and duration to cache, decompose, and render the 
complete FBC.

### Resource Considerations

This proposal should have no impact on 
SNO [resource minimums](https://docs.openshift.com/container-platform/4.17/installing/installing_sno/install-sno-preparing-to-install-sno.html#install-sno-requirements-for-installing-on-a-single-node_install-sno-preparing)
- vCPU: 8
- RAM: 16GiB
- storage: 120GiB

or Hypershift [SLOs](https://hypershift-docs.netlify.app/reference/slos/) where the primary consideration is memory uses in the 10s of MiB, not 100s.

### Risks and Mitigations

Depending on the implementation of this proposal, exercise of this endpoint could function as an 
I/O multiplier which could starve other clients which are dependent on the same API. 
The first iteration of this endpoint will not attempt to mitigate denial-of-service attempts via 
rate-limiting or other resource consumption constraints.

The proposal does not include any provision for authentication/authorization, since there are no 
current use-cases which require it. 

<!--
What are the risks of this proposal and how do we mitigate. Think broadly. For
example, consider both security and how this will impact the larger OKD
ecosystem.

How will security be reviewed and by whom?

How will UX be reviewed and by whom?

Consider including folks that also work outside your immediate sub-project.
-->

### Drawbacks

#### Caching 
Initial implementation will not implement server-side request/response caching, and will instead assess requests' `If-Modified-Since` 
headers against the catalog unpack timestamp and compose responses from indices generated during catalog data unpacking.

#### Complexity
Server implementation is much more complex and resource intensive due to the need to index catalog 
content and serve variable requests.

#### Completeness
The previous `all` endpoint always returns valid FBC.  The new service cannot make that promise, 
so clients could make incorrect assumptions about the suitability of results.  See Open Questions.


<!--
The idea is to find the best form of an argument why this enhancement should
_not_ be implemented.

What trade-offs (technical/efficiency cost, user experience, flexibility,
supportability, etc) must be made in order to implement this? What are the reasons
we might not want to undertake this proposal, and how do we overcome them?

Does this proposal implement a behavior that's new/unique/novel? Is it poorly
aligned with existing user expectations?  Will it be a significant maintenance
burden?  Is it likely to be superceded by something else in the near future?
-->

## Open Questions [optional]
<!--
This is where to call out areas of the design that require closure before deciding
to implement the design.  For instance,
 > 1. This requires exposing previously private resources which contain sensitive
  information.  Can we do this?
-->
 > 1. If a query comes in with `/api/v1/metas?package=foo`, should we include the blob with schema: `olm.package` and name: `foo`?

We feel that it is incorrect for the metas service endpoint to mutate the data model (specifically, to create a synthetic package attribute for the `olm.package` schema).  To access all the data modeled for an installable package, separate queries need to be made for the package-level metadata (`schema=olm.package&name=foo`) versus the channel/bundle-level metadata (`package=foo`).

  > 2. What guarantees do we make about the response bodies of `all` and `metas`?
  >    - `all` and `metas` return a stream of valid FBC blobs
  >    - Does catalogd make any guarantee that all passes opm validate-style validation?
  >    - We definitely can't guarantee that `metas` responses pass opm validate-style validation.
  >    - Do we need to clarify that it is up to clients to verify semantic validity if they need it? Are we comfortable putting that burden on clients?

`all` responses will preserve existing validity of the catalog data both from an installably-complete perspective (can be used as an installation reference) as well as syntactically valid perspective (opm validate).  
`metas` responses will be valid `declcfg.Meta` elements and make no promise that the response may be installably-complete (in the sense that the response itself could be used as a fully-intact catalog) or syntactically valid.

## Test Plan

**Note:** *Section not required until targeted at a release.*
<!--
Consider the following in developing a test plan for this enhancement:
- Will there be e2e and integration tests, in addition to unit tests?
- How will it be tested in isolation vs with other components?
- What additional testing is necessary to support managed OpenShift service-based offerings?

No need to outline all of the test cases, just the general strategy. Anything
that would count as tricky in the implementation and anything particularly
challenging to test should be called out.

All code is expected to have adequate tests (eventually with coverage
expectations).
-->

## Graduation Criteria

**Note:** *Section not required until targeted at a release.*

### Tech Preview
- Initial implementation is protected by the default-disabled feature gate `APIV1MetasEndpoint`.
- Sufficient test coverage
- Feedback from OCP Console team. 
- e2e feature tests are enabled for TPNU clusters.
- origin tests demonstrating endpoint inaccessibility in non-TPNU clusters
- resource benchmarking

### Tech Preview --> GA
- announce deprecation schedule for `all` endpoint.
- feature gate moves to default-enabled
- collect test data for coverage / reliability

### GA --> Maturity
- remove feature gate 

<!--
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

**If this is a user facing change requiring new or updated documentation in [openshift-docs](https://github.com/openshift/openshift-docs/),
please be sure to include in the graduation criteria.**

**Examples**: These are generalized examples to consider, in addition
to the aforementioned [maturity levels][maturity-levels].

### Dev Preview -> Tech Preview

- Ability to utilize the enhancement end to end
- End user documentation, relative API stability
- Sufficient test coverage
- Gather feedback from users rather than just developers
- Enumerate service level indicators (SLIs), expose SLIs as metrics
- Write symptoms-based alerts for the component(s)

### Tech Preview -> GA

- More testing (upgrade, downgrade, scale)
- Sufficient time for feedback
- Available by default
- Backhaul SLI telemetry
- Document SLOs for the component
- Conduct load testing
- User facing documentation created in [openshift-docs](https://github.com/openshift/openshift-docs/)

**For non-optional features moving to GA, the graduation criteria must include
end to end tests.**

### Removing a deprecated feature

- Announce deprecation and support policy of the existing feature
- Deprecate the feature
-->

## Upgrade / Downgrade Strategy

<!--
If applicable, how will the component be upgraded and downgraded? Make sure this
is in the test plan.

Consider the following in developing an upgrade/downgrade strategy for this
enhancement:
- What changes (in invocations, configurations, API use, etc.) is an existing
  cluster required to make on upgrade in order to keep previous behavior?
- What changes (in invocations, configurations, API use, etc.) is an existing
  cluster required to make on upgrade in order to make use of the enhancement?

Upgrade expectations:
- Each component should remain available for user requests and
  workloads during upgrades. Ensure the components leverage best practices in handling [voluntary
  disruption](https://kubernetes.io/docs/concepts/workloads/pods/disruptions/). Any exception to
  this should be identified and discussed here.
- Micro version upgrades - users should be able to skip forward versions within a
  minor release stream without being required to pass through intermediate
  versions - i.e. `x.y.N->x.y.N+2` should work without requiring `x.y.N->x.y.N+1`
  as an intermediate step.
- Minor version upgrades - you only need to support `x.N->x.N+1` upgrade
  steps. So, for example, it is acceptable to require a user running 4.3 to
  upgrade to 4.5 with a `4.3->4.4` step followed by a `4.4->4.5` step.
- While an upgrade is in progress, new component versions should
  continue to operate correctly in concert with older component
  versions (aka "version skew"). For example, if a node is down, and
  an operator is rolling out a daemonset, the old and new daemonset
  pods must continue to work correctly even while the cluster remains
  in this partially upgraded state for some time.

Downgrade expectations:
- If an `N->N+1` upgrade fails mid-way through, or if the `N+1` cluster is
  misbehaving, it should be possible for the user to rollback to `N`. It is
  acceptable to require some documented manual steps in order to fully restore
  the downgraded cluster to its previous state. Examples of acceptable steps
  include:
  - Deleting any CVO-managed resources added by the new version. The
    CVO does not currently delete resources that no longer exist in
    the target version.
-->

## Version Skew Strategy

<!--
How will the component handle version skew with other components?
What are the guarantees? Make sure this is in the test plan.

Consider the following in developing a version skew strategy for this
enhancement:
- During an upgrade, we will always have skew among components, how will this impact your work?
- Does this enhancement involve coordinating behavior in the control plane and
  in the kubelet? How does an n-2 kubelet without this feature available behave
  when this feature is used?
- Will any other components on the node change? For example, changes to CSI, CRI
  or CNI may require updating that component before the kubelet.
-->

## Operational Aspects of API Extensions

N/A

## Support Procedures

<!--
Describe how to
- detect the failure modes in a support situation, describe possible symptoms (events, metrics,
  alerts, which log output in which component)

  Examples:
  - If the webhook is not running, kube-apiserver logs will show errors like "failed to call admission webhook xyz".
  - Operator X will degrade with message "Failed to launch webhook server" and reason "WehhookServerFailed".
  - The metric `webhook_admission_duration_seconds("openpolicyagent-admission", "mutating", "put", "false")`
    will show >1s latency and alert `WebhookAdmissionLatencyHigh` will fire.

- disable the API extension (e.g. remove MutatingWebhookConfiguration `xyz`, remove APIService `foo`)

  - What consequences does it have on the cluster health?

    Examples:
    - Garbage collection in kube-controller-manager will stop working.
    - Quota will be wrongly computed.
    - Disabling/removing the CRD is not possible without removing the CR instances. Customer will lose data.
      Disabling the conversion webhook will break garbage collection.

  - What consequences does it have on existing, running workloads?

    Examples:
    - New namespaces won't get the finalizer "xyz" and hence might leak resource X
      when deleted.
    - SDN pod-to-pod routing will stop updating, potentially breaking pod-to-pod
      communication after some minutes.

  - What consequences does it have for newly created workloads?

    Examples:
    - New pods in namespace with Istio support will not get sidecars injected, breaking
      their networking.

- Does functionality fail gracefully and will work resume when re-enabled without risking
  consistency?

  Examples:
  - The mutating admission webhook "xyz" has FailPolicy=Ignore and hence
    will not block the creation or updates on objects when it fails. When the
    webhook comes back online, there is a controller reconciling all objects, applying
    labels that were not applied during admission webhook downtime.
  - Namespaces deletion will not delete all objects in etcd, leading to zombie
    objects when another namespace with the same name is created.
-->

## Alternatives

- Do nothing 

This option would require clients to query the entirety of the data (~21 MB for operatorhubio 
catalog) and parse the response to retrieve relevant information every time the client 
needs the data. Even if clients’ implement some form of caching, the first query the client 
does to catalogd server is still the dealbreaker. In a highly resource constrained environment 
(e.g. clusters in Edge devices), this basically translates to a chokepoint for the clients to get started.

- A “path hierarchy” based construction of API endpoints to expose filtered FBC metadata

The alternative to exposing a single, parameterized query endpoint is exposing many, “path 
hierarchy” based API endpoints. Eg: 

/api/v1/catalogs/operatorhubio/packages/, /api/v1/catalogs/operatorhubio/packages/<package-name>/, /api/v1/catalogs/operatorhubio/packages/<package-name>/bundles/
etc.

This interface creates a new API on top of the existing FBC structure. It also increases 
the number of discoverable endpoints by clients, with the scope for an unsustainable 
expansion of the API surface area in the future.

The main approach proposed in this document instead uses the already existing “FBC API”, with 
a clean API surface area.


<!--
Similar to the `Drawbacks` section the `Alternatives` section is used
to highlight and record other possible approaches to delivering the
value proposed by an enhancement, including especially information
about why the alternative was not selected.
-->
