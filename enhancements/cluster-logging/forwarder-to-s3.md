---
title: forwarder-to-s3
authors:
  - "@jcantrill"
reviewers:
  - "@apahim"
  - "@alanconway"
  - "@cahartma"
  - "@cuppett"
  - "@xperimental"
approvers:
  - "@alanconway"
api-approvers:
  - "@alanconway"
creation-date: 2025-09-08
last-updated: 2025-09-08
tracking-link:
  - https://issues.redhat.com/browse/OBSDA-1099
  - https://issues.redhat.com/browse/LOG-7680
see-also: []
replaces: []
superseded-by: []
---

# Log Forward to S3 Endoint

## Summary

This feature adds support for collecting logs using the Red Hat Logging Operator and forwarding them
to an S3 configured endpoint.  The enhancements to **ClusterLogForwarder** include API changes to: allow 
administrators to utilize "assume role" authentication functionality that is provided by the underlying platform,
and rely upon "sane" defaults for organizing records in an S3 bucket.

## Motivation

The primary motivation for this proposal is to satisfy functionality requests from Red Hat managed services teams
which are providing managed clusters for customers. They have requirements to be able to collect, forward, and store logs
from both the hosted control plane and the management clusters utilizing credentials from multiple organizations in a 
cost efficient manner.

### User Stories

* As an administrator, I want to forward logs to an S3 endpoint
so that I can store low access logs (i.e. audit logs) and
retain them for longer periods with reduced costs when compared to Cloudwatch
* As an administrator, I want to forward logs to an S3 endpoint that might
otherwise exceed the size limits of Cloudwatch


### Goals

* A simple API for an specifying log forwarding to an S3 output
* A set of sane defaults for organizing log streams written to the specified S3 bucket
* The capability to define how log streams are organized when written to the specified S3 bucket
* Re-use existing AWS authentication features provided by the Cloudwatch output

### Non-Goals

* To provide an API the exposes all the configuration points of the underlying collector implementation

## Proposal

This enhancement proposes to:

* Enhance the **ClusterLogForwarder** API to add an S3 output
  * Define a default schema for writing log records to an S3 bucket that is based
upon the log type and source in order to be consistent with other output types
  * Allow the schema for writting log records to be modified by the administrator
  * Reuse the authorization mechanisms that are available with the Cloudwatch output
* Add a generator to support generating collector configuration based upon the spec defined by the **ClusterLogForwarder** API


### Workflow Description

**Cluster administrator** is a human responsible for administering the **cluster-logging-operator**
and **ClusterLogForwarders**

1. The cluster administrator creates an S3 bucket on their host platform (i.e. AWS)
1. The cluster administrator grants a platform role (i.e. IAM Role) the permissions to write to the S3 bucket 
1. The cluster administrator deployes the cluster-logging-operator if it is already not deployed
1. The cluster administrator edits or creates a **ClusterLogForwarder** and defines an S3 output
1. The cluster administrator references the S3 output in a pipeline
1. The cluster-logging-operator reconciles the **ClusterLogForwarder**, generates a new collector configuration,
and updates the collector deployment

### API Extensions

#### ClusterLogForwarder API

```yaml
apiVersion: "observability.openshift.io/v1"
kind: ClusterLogForwarder
spec:
  outputs:
  - name:
    type: s3                 # add s3 to the enum
    s3:
      url:                   # (optional) string is an alternate to the well-known AWS endpoints
      region:                # (optional) string that is different from the configured service default
      bucket:                # string for the S3 bucket absent leading 's3://' or trailing '/' and
                             #   truncated to 63 characters to meet length restrictions
      keyPrefix:             # (optional) templated string (see note 1)
      authentication:
        type:                # enum: awsAccessKey, iamRole
        awsAccessKey:
          assumeRole:        # (optional)
          roleARN:           # secret reference
          externalID:        # (optional) secret reference
        iamRole:
          roleARN:           # secret reference
          token:             # bearer token
          assumeRole:        # (optional)
            roleARN:         # secret reference
            externalID:      # (optional)string
      tuning:
        deliveryMode:        # (optional) enum: atLeastOnce, atMostOnce
        maxWrite:            # (optional) quantity (e.g. 500k)
        compression:         # (optional) none, gzip,zstd,snappy,zlib
        minRetryDuration:    # (optional) duration
        maxRetryDuration:    # (optional) duration
```

**Note 1:** A combination of date formatters, static or dynamic values consisting of field paths followed by "||" followed by another field path or a static value (e.g `foo.{"%Y-%m-%d"}/{.bar.baz||.qux.quux.corge||.grault||"nil"}-waldo.fred{.plugh||"none"}`)

Date formatters are specified using one or more of the following subset of [chrono](https://docs.rs/chrono/latest/chrono/format/strftime/index.html#specifiers)
specifiers to format the `.timestamp` field value:

| Spec | Example | Description |
|------|---------|-------------|
| %F | 2001-07-08| Year-month-day format (ISO 8601). Same as %Y-%m-%d.|
| %Y | 2001 |The full proleptic Gregorian year, zero-padded to 4 digits
| %m | 07 | Month number (01–12), zero-padded to 2 digits.|
| %d |08|Day number (01–31), zero-padded to 2 digits.|
| %H |00|Hour number (00–23), zero-padded to 2 digits.|
| %M |34|Minute number (00–59), zero-padded to 2 digits.|
| %S |60|Second number (00–60), zero-padded to 2 digits.|

The collector will write logs to the s3 bucket defaulting the key prefix that is constructed using attributes of the log entries when not defined by the **ClusterLogForwarder** spec as follows:

| log type| log source | key prefix |
| --- | --- | --- | 
| Application | container |`<cluster_id>/<yyyy-mm-dd>/<log_type>/<log_source>/<namespace_name>/<pod_name>/<container_name>/`|
| Infrastructure | container|`<cluster_id>/<yyyy-mm-dd>/<log_type>/<log_source>/<namespace_name>/<pod_name>/<container_name>/`|
| Infrastructure | node (Journal)|`<cluster_id>/<yyyy-mm-dd>/<log_type>/<log_source>/<host_name>/`|
| Audit | auditd|`<cluster_id>/<yyyy-mm-dd>/<log_type>/<log_source>/<host_name>/`|
| Audit | kubeAPI|`<cluster_id>/<yyyy-mm-dd>/<log_type>/<log_source>/`|
| Audit | openshiftAPI|`<cluster_id>/<yyyy-mm-dd>/<log_type>/<log_source>/`|
| Audit | ovn|`<cluster_id>/<yyyy-mm-dd>/<log_type>/<log_source>/`|

**Note 2:** The collector will encode events as [JSON](https://www.rfc-editor.org/rfc/rfc8259)

### Topology Considerations

#### Hypershift / Hosted Control Planes


#### Standalone Clusters


#### Single-node Deployments or MicroShift

### Implementation Details/Notes/Constraints

Implementation includes:

* `ClusterLogForwarder` API updates
* Log collector config generator updates with S3 code config template additions

### Risks and Mitigations

This feature is being requested by HCP with a very short deadline for providing a deliverable.  This change
is dependent upon another change that introduces "assumeRole" functionality which has not been completed.  The
risk to the Logging team is HCP may choose to utilize an alternate product if these changes can not be realized
within their time constraints.

### Drawbacks

The drawbacks to this change is we may be providing users with an alternative to the product's LokiStack
offereing which may delay its adoption.  The feature set of the receivers addresses separate usecases but
this choice may be construed as a "cheap" or "simple" alternative.

Additionally, this change may be interpreted as a "reliable" delivery mechanism for forwarding logs which
is still misleading. The OpenShift logging product is not a guaranted log collection and storage system and this
output will remain subject to the same set of limitations as all other outputs.

Lastly, using this output provides no mechanism to query log records in a useful manner that is offered by other outputs (i.e. LokiStack).  The available "metadata" is dependent upon the definition of the "keyPrefix" when the logs are written to S3.  If the "keyPrefix" does not provide useful way to organize the data then retrieval of that data will be challenging.

## Alternatives (Not Implemented)


## Open Questions [optional]

1. Do we need to support `filename_time_format` to address the key prefix functionality proposed by the draft [PR](https://github.com/openshift/cluster-logging-operator/pull/3096)
* All indicators are that we need some way to provide a way for users to inject a formatted date into the "keyPrefix" field in order to provide logical organization of the records when written to the bucket
2. Is there a need to introduce this feature as tech-preview with a `v2beta1` API to allow the "soak" time for the API and additional testing?

## Test Plan

Aside from the usual testing by logging QE, the intent is to deploy, potentially early candidate releases, to the HCP environment in order to exercise their S3 lambda design

## Graduation Criteria


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

## Upgrade / Downgrade Strategy


## Version Skew Strategy

## Operational Aspects of API Extensions

## Support Procedures


## Infrastructure Needed [optional]

HCP deployment
