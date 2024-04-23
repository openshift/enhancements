---
title: audit-log-configuration-options
authors:
  - "@copejon"
reviewers:
  - "@dhellmann: MicroShift architect"
  - "@pacevedom: MicroShift team-lead"
  - "@jerpeter1, Edge Enablement Staff Engineer"
approvers:
  - "@dhellmann"
creation-date: 2023-01-26
last-updated: 2022-02-21
status: informational
tracking-link: 
- "https://issues.redhat.com/browse/USHIFT-2196"
api-approvers:
- "None"
---

# Configurable Audit Logging for MicroShift

## Summary

Add ability for MicroShift users to configure API server audit logging policies, log rotation and retention.

## Motivation

MicroShift currently uses a hardcoded audit logging policy. It should be configurable in a manner similar to OpenShift to meet customer needs.

### User Stories

* As a MicroShift administrator, I want to configure audit logging policies so that I can control what events are logged.

* As a MicroShift administrator, I want to configure the max file size and retention policy for audit logs so that I can better manage their disk usage.

### Goals

- Enable MicroShift administrators to manage logging policies as a set of hierarchical profiles.
- Provide flexibility to specify log file sizes and retention policies.

### Non-Goals

- Custom Rules: MicroShift is a single-user system without user groups, so custom audit log rules similar to OpenShift are not 
required and should be explicitly marked as out of scope.
- Support OVN-K audit log configuration. OVN-K audit log policies are managed via a configMap and do not have the same flexibility a OpenShift and Kubernetes API servers.

## Proposal

This proposes exposing a subset of kube-apiserver audit-log settings flags. The kube-apiserver settings will enable user control over log file rotation and retention. Users may set fields in combination to define a maximum storage limit (e.g. max num files * max single file size = total storage limit). This is a critical feature for far edge devices with limited storage capacities. On such devices, logging data accumulation risks starving the host system or cluster workloads, potentially bricking the device until human intervention can be applied.  Thus, it is necessary to provide users a means of enforcing such a limit and what actions to take at that limit. Users must also be able to select which events are logged. This will be exposed as a set of "profiles," with predefined behaviours and will be mutually exclusive. This should give users holistic control of their audit log rotation, retention, and max allowable storage allocation.

### Workflow Description

1. Administrator edits MicroShift config file to specify desired audit logging policy profile
2. Administrator edits MicroShift config file to specify max file size, number of files total, and max age of files for logs
3. Administrator restarts MicroShift service to apply changes

### API Extensions

**Audit Log Policy Configuration:**

- MicroShift config’s `apiServer` root-field must include a child node for storing log configurations.

```yaml
apiServer:
  auditLog: MAP(STRING)INTERFACE
```

- The `apiServer.auditLog` field must include a child node for storing the audit log policy profile to use. See [Implementation Details](#implementation-detailsnotesconstraints) for more. These are the same policies as in [Openshift Audit log policies](https://docs.openshift.com/container-platform/4.14/security/audit-log-policy-config.html#about-audit-log-profiles_audit-log-policy-config).

```yaml
apiServer:
  auditLog:
    profile: STRING
```

**Audit Log File Rotation:**

MicroShift will expose 3 kube-apiserver fields in the MicroShift config file. Together, the values specified here enable the user to enforce certain size and age limits of audit log backups.
For all 3 fields, MicroShift will enforce a minimum value of >=0. See [Operational Aspects of API Extensions](#operational-aspects-of-api-extensions) for failure modes.

>NOTE: If users want to disable logging entirely, the "None" profile should be specified instead. This is _not_ suggested and is at their own peril. See warning highlights in [OCP audit log configuration docs.](https://docs.openshift.com/container-platform/4.14/security/audit-log-policy-config.html#audit-log-policy-config)

```yaml
apiServer:
  auditLog:
    maxFileSize: INT
    maxFiles: INT
    maxFileAge: INT
```

- `maxFileSize` specifies maximum audit log file size in megabytes. When the value is `0`, the limit is disabled.
- `maxFiles` specifies the maximum number of rotated audit log files to retain.  Once this limit is reached, the apiserver will delete log files in order from oldest to newest, until all specified limits are satisfied. When the value is `0`, the limit is disabled.
  - _For example_,`maxFiles: 1` will result in only 1 file of size `maxFileSize` being retained in addition to the current active log, provided it also is within the `maxFileAge` limit, if specified.
- `maxFileAge` specifies the maximum time in days to retain log files.  Files older than this limit will be deleted. When the value is `0`, the limit is disabled.

Field values are processed independently of one another, without prioritization.  Max size and max number of files may be used in conjunction to limit the storage footprint of retained logs (e.g. `maxFileSize` * `maxFiles` = log storage upper limit).  Setting `maxFileAge` will cause files older than the timestamp in the file name to be deleted, regardless of the `maxFiles` value.

For example: Given the below configuration, the system will keep at most 1 rotated log file. If that log file reaches >7 days old, it will be deleted, regardless of whether the live log has reached the max file size of 200Mb.  Otherwise, if the live log reaches the 200Mb limit, it will be rotated, causing the existing log backup (if any) to be deleted. 
```yaml
apiServer:
  auditLog:
    maxFileSize: 200
    maxFiles: 1
    maxFileAge: 7
  ```

### Topology Considerations

#### Hypershift / Hosted Control Planes

- N/A

#### Standalone Clusters

- N/A

#### Single-node Deployments or MicroShift

- This EP is specific to MicroShift only.

### Implementation Details/Notes/Constraints

**Passing User Provided Options to the API Server**

MicroShift's existing logic embeds a default Kube API server config, which is written to disk during boot at `/var/lib/microshift/resources/kube-apiserver-audit-policies/default.yaml`. This is not a user facing config and is used to pass configuration from MicroShift to the kube-apiserver. When the kube-apiserver service-manager is [created](https://github.com/openshift/microshift/blob/76f51316bb2b82dff876d89a36a17a3b12b444f6/pkg/controllers/kube-apiserver.go#L81-L87), MicroShift applies additional configuration values to the kube-apiserver config.  The `NewKubeAPIServer` function already accepts a MicroShift config parameter, which it passes to the `KubeAPIServerConfig.configure()` method. The implementation of this proposal is therefore greatly simplified since the path for passing data from the MicroShift config to the kube-apiserver is already defined.

**Setting kube-apiserver log values**
The `KubeAPIServerConfig.configure()` method will be altered to include logic for handling the MicroShift config's audit log fields (`apiServer.auditLog.{maxFileSize: INT,maxFiles: INT,maxFileAge: INT}`). The fields will be validated and appended to the `overrides` object, which is then merged with the default kube-apiserver config. When `Run()` is executed, the updated kube-apiserver config is written to disk.

This provides an elegant fallback for user-facing config values. If a user does not specify a value for a field, the default value will be used, just as it is now.  Should a user remove a field they previously set, then at next restart, the field will no longer be overridden and the default value will be restored. All audit logging configuration fields must be optional. If not specified, the existing default audit logging policy will be used.

**Setting kube-apiserver policies**

MicroShift currently hardcodes the kube-apiserver policy. This logic will be updated to map the MicroShift config `apiServer.auditLog.profile` value to the corresponding policy.  The policies and mapping logic already exist in the openshift/api and openshift/library-go modules, which are current dependencies of MicroShift. The necessary packages are public and currently in-use in OpenShift operators.

**Default Values**

MicroShift currently sets default values for `audit-log-maxbackup` to "10" and `audit-log-maxsize` to "200". Thus default log storage is 2,000 MB. These values are defined as part of the embedded [kube-apiserver config file](https://github.com/openshift/microshift/blob/66394512e6adac09f26e4cf049951d77a83a05a4/assets/controllers/kube-apiserver/defaultconfig.yaml#L44-L48). This design will not add a default value for `audit-log-maxage`.  It is sufficient to specify a storage limit on logs for default deployments. 

**Audit Log Rotation**

The kube-apiserver provides three CLI flags to dictate log rotation and retention policies which may be thinly exposed to the user via the MicroShift config API. These options will be set dynamically, depending on user configuration in the MicroShift config. Logic already exists to set these flags with hardcoded values, which will need to be updated to allow for dynamic arguments.  These options are:

* `--audit-log-maxage` defines the maximum number of days to retain old audit log files.  MicroShift defaults to 0, disabling the age limit.
* `--audit-log-maxbackup` defines the maximum number of audit log files to retain. MicroShift defaults to 10 files.
* `--audit-log-maxsize` defines the maximum size in megabytes the live log file before it is rotated.  Defaults to 200Mb.

Thus, the default maximum storage consumption of audit logs will be 2000Mb, provided all files are younger than 10 days.

**Policy Profiles**

OpenShift's policy profiles are defined as part of the `openshift-cluster-config-operator` API and are not recognized by the kube-apiserver. A profile can be considered a higher level implementation of the `policy.audit.k8s.io/v1` API, where each profile maps to a predefined `policy`. A `Policy.audit.k8s.io/v1` object consists of a set of rules, with each rule specifying the API resources, verbs, and level. Policy objects must be defined by the user and thus require a familiarity with Kubernetes APIs and auditing events, and the user's target security posture.  There are no safeguards that prevent users from inadvertently exposing sensitive information in logs.

| Kube API-Server Level |Description|
|-----------------------|---|
| None                  |don't log events that match this rule.|
| Metadata              |log request metadata (requesting user, timestamp, resource, verb, etc.) but not request or response body.|
| Request               |log event metadata and request body but not response body. This does not apply for non-resource requests.|
| RequestResponse       |log event metadata, request and response bodies. This does not apply for non-resource requests.|


OpenShift audit profiles provide an abstraction layer to the Kube `policy` API, where each profile corresponds to an entire predefined policy object. These policies are maintained by OpenShift and encapsulate various levels of logging coverage, while also ensuring rules for sensitive resources, such as Secret, Route, and OAuthClient objects, are only ever logged at the metadata level. OpenShift OAuth server events are only ever logged at the metadata level.

| OpenShift Audit Policy Profile | Description                                                                                                                                                                                                                           |
|--------------------------------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| None                           | No requests are logged; even OAuth access token requests and OAuth authorize token requests are not logged.                                                                                                                           |
| Default                        | Logs only metadata for read and write requests; does not log request bodies except for OAuth access token requests. This is the default policy.                                                                                       |
| WriteRequestBodies             | In addition to logging metadata for all requests, logs request bodies for every write request to the API servers (create, update, patch, delete, deletecollection). This profile has more resource overhead than the Default profile. |
| AllRequestBodies               | In addition to logging metadata for all requests, logs request bodies for every read and write request to the API servers (get, list, create, update, patch). This profile has the most resource overhead.                            |

MicroShift will reuse the existing OpenShift custom resource, `apiserver.config.openshift.io`, defined as part of the [OpenShift API](https://github.com/openshift/api/blob/master/config/v1/types_apiserver.go) as part of the translation process from "profile" to "policy". Logic for translating the OpenShift profiles to the `policy.audit.k8s.io` objects is provided by the [openshift/library-go](https://github.com/openshift/library-go/tree/release-4.14/pkg/operator/apiserver/audit) library and is intended for import into external code bases.

To enable OpenShift profiles, MicroShift will get the `apiserver.auditLog.profile` value from the MicroShift config and internally wrap this value into an `APIServer.config.openshift.io/v1` API object, which is an [OpenShift API type](https://github.com/openshift/api/blob/750a3e21ebaf57f97e022f2c7f5ed784322de844/config/v1/types_apiserver.go#L87). This instance will be passed to the [library-go GetAuditPolicy function](https://github.com/openshift/library-go/blob/release-4.14/pkg/operator/apiserver/audit/audit_policies.go#L93), which will return the `policy.audit.k8s.io/v1` equivalent.

**Included Dependencies**

- **openshift/library-go**: already imported by MicroShift.
- **openshift/api**: already imported by MicroShift.

### Risks and Mitigations

- Exceeding disk capacity: Microshift targets small form-factor devices with limit on-board storage. If the product of `maxFileSize` and `maxFiles` equals a size larger than the available storage, the apiserver risks destabilizing the system.  This can be mitigated via documentation which recommends the user understand their storage limitations when setting these values.
- Lost Log Data: The apiserver culls log files given a certain size or age. If users do not take care to back up logs at a rate greater than the rate at which the kube-apiserver culls the files, they risk losing log data. This can be mitigated with examples of log-forwarding provided in documentation, example manifests, or both. Alternatively, it may be necessary to consider deploying the openshift-cluster-logging-operator to provide supportable log forwarding features.
- Exposing the apiserver's audit-log-path, which allows users to set a custom log location, would hinder sos report gathering. Instead, users should replace `/var/log/kube-apiserver` with a symlink the desired path.  Sos will follow the symlink and collect logs by default.

### Drawbacks

- N/A

## Open Questions

- N/A

## Test Plan

* Unit tests to validate new config APIs
* Integration tests to verify configured audit logging policies are applied properly

## Graduation Criteria

* Upstream code, tests, docs merged
* Downstream builds, docs updated
* Automated CI coverage for new functionality
* QE test plans defined

### Dev Preview -> Tech Preview

- Ability to utilize the enhancement end to end
- End user documentation completed and published
- Sufficient test coverage
- Gather feedback from users
- Available by default

### Tech Preview -> GA

- Sufficient time for feedback
- End-to-end tests

### Removing a deprecated feature

- Announce deprecation and support policy of the existing feature
- Deprecate the feature

## Upgrade / Downgrade Strategy

- The API must be backwards compatible for y-stream version

## Operational Aspects of API Extensions

- **Disk capacity exceeded**: MicroShift and the apiserver do not assess existing storage capacity for any logs, including audit logs.  As audit logs can grow quite quickly, this creates the potential for maxing out a storage device and hindering system performance.  Users must therefore consider their total storage needs, in addition to how and how often to transfer logs off-device.

- **Invalid rotation values**: MicroShift will enforce a floor value of n>=0 for `maxFiles`, `maxFileAge`, and `maxFileSize` fields.  Values of n<0 will be considered a configuration error and MicroShift will fail during startup. The invalid config field will be clearly logged along with suggested remedial action (i.e. set a value >=0).

## Support Procedures 

- N/A

## Version Skew Strategy

- openshift/library-go and openshift/api are shared core libraries of OpenShift components and thus are included in each OpenShift release.  Version skew is not an issue.

## Alternatives

* Continue using hardcoded audit logging policy
* Disabling api-server log rotation behavior entirely and referring users to the `logrotate` system utility to manage logs.
* Custom log paths would be passed to the apiserver.  The MicroShift sos plugin would have to be made capable of finding logs at a user-defined path, which would create an undesirable coupling between the support tool and the MicroShift config (in which users would specify the log path).  Users who require a non-standard default path may replace `/var/log/kube-apiserver` directory with a symlink to their desired directory. This is supported by `sos`.

* Supporting a `minFreeStorage` field that would be used to determine whether an acceptable amount of space exists at the log file path. This is not a value recognized by the apiserver, but it could be used by Microshift to check if the system is in an acceptable state to boot into. However, providing additional fields and internal handling logic is outside the scope of this EP. If this is a useful feature, it should be documented in a bespoke EP.

## Infrastructure Needed

- N/A