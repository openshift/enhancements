---
title: forward_to_cloudwatch
authors:
  - "@alanconway"
reviewers:
  - "@jcantrill"
  - "@jeremyeder"
approvers:
creation-date: 2020-12-17
last-updated: 2020-12-17
status: implementable
see-also:
superseded-by:
---

# Forward to CloudWatch

## Release Signoff Checklist

- [X] Enhancement is `implementable`
- [X] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

[Amazon CloudWatch][aws-cw] is a hosted monitoring and log storage service.
This proposal extends the `ClusterLogForwarder` API with an output type for CloudWatch.

## Motivation

Amazon CloudWatch is a popular log store.
We have requests from external and  Red Hat-internal customers to support it.

### Goals

Enable log forwarding to CloudWatch.

### Non-Goals

Enable CloudWatch metric collection.

## Proposal

### CloudWatch streams and groups

[CloudWatch][concepts] defines *log groups* and *log streams*. From the CloudWatch documentation:

> A log stream is a sequence of log events that share the same source ... For example, an Apache access log on a specific host.
>
> Log groups define groups of log streams that share the same retention, monitoring, and access control settings ... For example, if you have a separate log stream for the Apache access logs from each host, you could group those log streams into a single log group called MyWebsite.com/Apache/access_log.

In other words a *log stream* corresponds to the smallest distinct source of logs.
A *log group* is a collection of related *log streams*.

#### Log streams

The collector automatically creates a unique *log stream* for each log file it collects.

- Stream names are globally unique.
- Constructed without API calls
- Each stream corresponds to a single tailed log file.

**Note**: The log stream name is *opaque* to the end user for the first release.
It should *not* be used for indexing, searching or as a reliable source of meta-data.
The end user can retrieve all meta-data as JSON fields in the log record.
See "Open Questions" for more detail.

See "Implementation Details" for more.

#### Log groups

The initial implementation uses 3 fixed *log groups* (see Nice To Have for possible future options).
The log groups are composed of of <cluster-name>/<log-type>, for example:

- `mycluster.example.com:6443/application`
- `mycluster.example.com:6443/infrastructure`
- `mycluster.example.com:6443/audit`

The cluster name is the *DNS authority* (host and port) of the clusters API server.
It is guaranteed to be unique, and is more readable than the cluster's UUID.

The exact cluster name prefix for log groups is printed by:
```sh
oc config view -o jsonpath='{.clusters[].name}{"/"}'
```

### API fields

New API fields in the `output.cloudwatch` section:

- `region`: (string, required) AWS region name, required to connect.

Existing fields:

- `url`: Not used in production. Sets the `endpoint` parameter in fluentd for use in testing.
- `secret`: AWS credentials, the secret must contain keys `aws_access_key_id` and `aws_secret_access_key`.

**Note**: The installer UI (Addon or OLM) can get AWS credentials from a `cloudcredential.openshift.io/v1`.
The user only has to provide a `region` to enable CloudWatch forwarding for a cluster.
Details are out of scope for this proposal.

### User Stories

#### I want to forward logs to CloudWatch instead of a local store

```yaml
apiVersion: "logging.openshift.io/v1"
kind: "ClusterLogForwarder"
spec:
  outputs:
  - name: CloudWatchOut
    type: cloudwatch
    cloudwatch:
      region: myregion
    secret:
       name: mysecret
  pipelines:
  - inputRefs: [application, infrastructure, audit]
    outputRefs: [CloudWatchOut]
```

CloudWatch group names are: "*cluster-name*/application", "*cluster-name*/infrastructure", "*cluster-name*/audit"

### Implementation Details

Use the [fluentd CloudWatch plugin][plugin] to connect to CloudWatch.
Plugin configuration settings:

- `auto_create_stream`: true to create streams and groups on the fly.
- `log-stream-name`: set to `<hostname>.<routing-key>` for all log types. Guaranteed to be globally unique.
- `log_group_name`: Set to "cluster-name/log-type"
- `region`: Set from `cloudwatch.region`
- `aws_access_key_id`, `aws_secret_access_key`: Set from `secret`
- `endpoint`: set from optional `url`, for testing and debugging.

### Nice To Have: more options for log groups

_NOT REQUIRED for initial implementation, noted here for possible extensions._

- `groupPrefix`: (string, optional) Control prefix for  group names.
  - if `groupPrefix` is _missing_, the default prefix is cluster-name/log-type
  - if `groupPrefix` is present it is used verbatim as the prefix.\
    In particular `groupPrefix: ""` means *no prefix*.

- `groupBy`: (string, default "logType") Take group name from logging meta-data. Values:
  - `logType`: one of "application", "infrastructure", or "audit"\
    Note that *infrastructure* and  *audit* logs are always grouped by `logType`.
  - `namespaceName`: *application* logs are grouped by namespace name.
  - `namespaceUUID`: *application* logs are grouped by namespace UUID.

The `groupBy` value translates to a meta-data key in the message.
There is no implementation cost to allowing arbitrary meta-data to be used as a group name.
However, the choices should be restricted for safety and simplicity.

A "safe" key must have values that:

1. are valid CloudWatch group name strings.
2. will not generate an excessive number of groups.
3. are constant for messages in the same *log stream* (streams belong only one group)

The following keys are safe and would be useful:

- kubernetes.labels.`<key>`: Use pod label value with key `<key>`
- openshift.labels.`<key>`: Use label added by the openshift log forwarder

Other keys should be considered case-by case, for example:

- `message` is definitely *not* safe, fails all safety requirements.
- `ip_addr` is safe (node cardinality), but debatable if it would ever be useful.
- `hostname` is safe (node cardinality), and probably more useful than ip_addr but still debatable.
- etc.

Custom log groups can be created using `openshift.labels`.
To support custom logs we add:

- `groupByOptional`: (list of string) List of optional metadata keys to use for `groupBy`.
  The first key that is present and non-empty is instead of `groupBy`.
  If none found, use the value of `groupBy`.

For example, I want to group most logs by log type, except for logs from
namespaces [magic1, magic2] which should be in log group "magic".

```yaml
apiVersion: "logging.openshift.io/v1"
kind: "ClusterLogForwarder"
spec:
  intputs:
  - name: MagicApp
    application:
      namespaces: [ magic1, magic2 ]
  outputs:
  - name: CloudWatchOut
    type: cloudwatch
    cloudwatch:
      region: myregion
      groupBy: logType
      groupByOptional: [ openshift.labels.logGroup ]
    secret:
       name: mysecret
  pipelines:
  - inputRefs: [application, infrastructure, audit]
    outputRefs: [CloudWatchOut]
  - inputRefs: [MagicApp]
    outputRefs: [CloudWatchOut]
    labels: { logGroup: magic }
```

### Open Questions

#### Log stream names and static meta-data

Initial log stream names will use our current fluent tags for uniqueness,
which includes some static meta-data.

We *may* want to advertise this stream name format as a way to access static meta-data,
and reduce the repetition of static data in log records.
It is too early to decide now because:

- We need to clean up the format before making it public
- We need to solve the static meta-data problem consistently for other output types as well.
- There may be other solutions e.g. using [cloudwatch group tags][groups-and-streams]

For now the name will be documented as *opaque* to the user, so we can make changes in future without breaking user assumptions.

#### EKS authentication

Is this a requirement? If so need to define appropriate `secret` keys.

#### Additional API fields

- `retentionDays`: (number) Number of days to keep logs.
- [cloudwatch tags][groups-and-streams]

### Risks and Mitigations

[CloudWatch quota][quota] can be exceeded if insufficiently granular streams are configured.
We configure a stream-per-container which is the finest granularity we have for logging.

- 5 requests per second per log stream. Additional requests are throttled. This quota can't be changed.
- The maximum batch size of a PutLogEvents request is 1MB.
- 800 transactions per second per account per Region, except for the following Regions where the quota is 1500 transactions per second per account per Region: US East (N. Virginia), US West (Oregon), and Europe (Ireland). You can request a quota increase.

## Design Details

### Test Plan

- E2E tests: Need access to AWS logging accounts.
- Functional tests: can we use [fluentd] `in_cloudwatch_logs` as a dummy cloudwatch server?

### Graduation Criteria

- Initially release as [beta][maturity-levels] tech-preview to internal customers.
- GA when internal customers are satisfied.

### Version Skew Strategy

Not coupled to other components.

## References

- [Amazon CloudWatch][aws-cw]
- [Amazon CloudWatch Logs Concepts][concepts]
- [CloudWatch Logs Plugin for Fluentd][plugin]
- [Maturity Levels][maturity-levels]
- [CloudWatch Logs quotas][quota]
- [CloudWatch Log Groups and Streams][groups-and-streams]

[aws-cw]: https://docs.aws.amazon.com/cloudwatch/index.html "[Amazon CloudWatch]"
[concepts]: https://docs.aws.amazon.com/AmazonCloudWatch/latest/logs/CloudWatchLogsConcepts.html "[Amazon CloudWatch Logs Concepts]"
[plugin]: https://github.com/fluent-plugins-nursery/fluent-plugin-cloudwatch-logs "[CloudWatch Logs Plugin for Fluentd]"
[maturity-levels]: https://git.k8s.io/community/contributors/devel/sig-architecture/api_changes.md#alpha-beta-and-stable-versions "[Maturity Levels]"
[quota]: https://docs.aws.amazon.com/AmazonCloudWatch/latest/logs/cloudwatch_limits_cwl.html "[CloudWatch Logs quotas - Amazon CloudWatch Logs]"
[groups-and-streams]: https://docs.aws.amazon.com/AmazonCloudWatch/latest/logs/Working-with-log-groups-and-streams.html "Log streams and groups"
[put-logs]: https://docs.aws.amazon.com/cli/latest/reference/logs/put-log-events.html "Put log events API"
