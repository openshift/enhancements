---
title: must-gather-operator-ftphost
authors:
  - "@shivprakahsmuley"
reviewers:
  - "@TrilokGeer"
  - "@Prashanth684"
approvers:
  - "@TrilokGeer"
  - "@Prashanth684"
api-approvers:
  - "@TrilokGeer"
  - "@Prashanth684"
creation-date: 2025-08-18
last-updated: 2025-08-18
tracking-link:
  - https://issues.redhat.com/browse/MG-53
status: implementable
see-also:

---

# Must Gather Operator Enhancement: FTPHost in MustGatherSpec

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

Add an optional field `ftpHost` to `MustGather.spec` that allows directing must-gather artifacts to a designated FTP/SFTP endpoint during the upload phase. This provides a supported, secure, and validated path for environments that require alternate upload targets beyond the default Red Hat case management destination.

Default: `access.redhat.com` (used when ftpHost is unset)

## Motivation

### User Stories

* As a cluster admin, I can set `spec.ftpHost: access.stage.redhat.com` to stage bundles to pre‑production.
* As a support engineer, I can ensure bundles always upload to the default `access.redhat.com` without specifying anything in the CR.
* As a CI/QA engineer, I want to direct must-gather uploads to a non-production staging environment to avoid polluting production case management data.

### Goals

* Add `spec.ftpHost` with a safe default.
* Ensure the upload container honors ftpHost and existing proxy settings.
* Maintain full backward compatibility.

### Non-Goals

* Introduce non-FTP providers (e.g., S3, HTTP PUT).


## Proposal

When `spec.ftpHost` is set, the upload process targets the indicated endpoint (over SFTP/FTP as implemented by the upload script). When omitted, the operator continues to use the existing Red Hat case upload pathway. Proxy handling remains unchanged.

### Workflow Description

**cluster administrator** is a human user responsible for configuring must-gather collection.

1. The cluster administrator creates a MustGather custom resource with an optional `ftpHost` field.
2. If `ftpHost` is specified (e.g., `access.stage.redhat.com`), the must-gather operator configures the upload job to target that endpoint.
3. If `ftpHost` is omitted, the operator defaults to `access.redhat.com` for backward compatibility.
4. The upload container uses the specified FTP host along with existing proxy and credential configurations.


### API Extensions

#### Types: must-gather-operator/api/v1alpha1/mustgather_types.go

```go
// MustGatherSpec defines the desired state of MustGather
type MustGatherSpec struct {
    // ...existing fields...
    
    // ftpHost is an optional FTP/SFTP host used to upload the bundle.
    // If unset, defaults to access.redhat.com.
    // +kubebuilder:validation:Optional
    // +kubebuilder:default:=access.redhat.com
    FTPHost string `json:"ftpHost,omitempty"`
}
```

#### CRD OpenAPI Schema

```yaml
ftpHost:
  type: string
  default: access.redhat.com
  description: "FTP/SFTP host used to upload the bundle. If unset, defaults to access.redhat.com."
```

### Topology Considerations

#### Hypershift / Hosted Control Planes


#### Standalone Clusters

This change is fully relevant for standalone clusters where must-gather operations are performed directly within the cluster.

#### Single-node Deployments or MicroShift


### Implementation Details/Notes/Constraints

* **API**: Add `FTPHost` to `MustGatherSpec` with kubebuilder markers (default).
* **Controller**: Update upload container to pass `FTP_HOST` environment variable (or argument) from `spec.ftpHost`.
* **Defaulting**: Respect defaulting when field is unset to maintain backward compatibility.
* **CRD Generation**: Regenerate deepcopy, OpenAPI, CRDs, and bundle files.

### Risks and Mitigations

* **Security (data exfiltration)**: Uploading to external endpoints introduces risk.
  * *Mitigation*: RBAC controls, documentation, and guidance to use trusted endpoints.
* **Reliability**: Proxies or firewalls may block FTP/SFTP.
  * *Mitigation*: Proxy env variables and observable errors/conditions remain in place.

## Drawbacks

* Adds another user‑visible configuration option to the API.
* Slight complexity increase in upload command.
* Risk of misconfiguration when arbitrary endpoints are used; guidance and observability mitigate this.

## Alternatives 

## Open Questions

## Test Plan

### Unit Tests

* API defaulting
* Job template generation includes FTP host when set; defaults when omitted
* Proxy configuration fallback/precedence works alongside ftpHost

### E2E Tests

* Happy path using `access.stage.redhat.com` in staging/dev environments
* Default path (unset → `access.redhat.com`)
* Upload success/failure with different FTP hosts

## Graduation Criteria

### Dev Preview -> Tech Preview

* Field added with defaulting
* Unit tests implemented
* Basic documentation available
* Ability to utilize the enhancement end to end

### Tech Preview -> GA

* E2E tests implemented
* Customer validation in staging environments
* User-facing documentation created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Upgrade / Downgrade Strategy

* **Backward compatible**: Field is optional with a safe default
* **Upgrade**: Older operators ignore the unknown field; no disruption to current flows
* **Downgrade**: If downgrading to an operator version that doesn't support ftpHost, the field is ignored and uploads continue to the default endpoint

## Version Skew Strategy

* Ensure operator CSV/CRD includes the new field before shipping the controller that uses it
* If controller is older than CRD, it safely ignores the field
* No coordination required between control plane and kubelet components

## Operational Aspects of API Extensions

This enhancement modifies the MustGather CRD by adding an optional field with a default value.

**Impact on existing SLIs**:
* No impact on API throughput or availability
* No impact on cluster performance as this is a configuration field only used during must-gather operations

**Failure modes**:
* Invalid or unreachable FTP host results in runtime upload errors surfaced via job logs and conditions
* Credential issues result in observable upload failures with clear error messages

## Support Procedures

### Detecting Issues

* **Symptoms**: Upload failures, timeout errors in must-gather job logs
* **Logs**: Check must-gather operator logs and must-gather job upload container logs for FTP-related errors


### Troubleshooting

* Verify FTP host is reachable from cluster network
* Check proxy configuration if upload fails
* Validate credentials in the referenced secret
* Ensure firewall rules allow FTP/SFTP traffic to the specified host

### Disabling the Feature

* Remove or omit the `ftpHost` field from MustGather resources to use default behavior
* No cluster-wide disable mechanism needed as this is a per-resource configuration

## Examples

### Default Configuration (ftpHost omitted)

```yaml
apiVersion: managed.openshift.io/v1alpha1
kind: MustGather
metadata:
  name: example-mustgather-default
spec:
  caseID: "01234567"
  caseManagementAccountSecretRef:
    name: case-management-creds
  serviceAccountRef:
    name: must-gather-admin
  # ftpHost omitted → defaults to access.redhat.com
```

### Staging Configuration

```yaml
apiVersion: managed.openshift.io/v1alpha1
kind: MustGather
metadata:
  name: example-mustgather-stage
spec:
  caseID: "01234567"
  caseManagementAccountSecretRef:
    name: case-management-creds
  serviceAccountRef:
    name: must-gather-admin
  ftpHost: access.stage.redhat.com
```

## Implementation History

* v0: Draft proposal created
* v1: API added with defaulting; unit tests; docs; CRDs regenerated
* v2: E2E tests and observability; promotion to tech preview
* v3: GA after stabilization

