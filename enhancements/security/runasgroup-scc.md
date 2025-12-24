---
title: runasgroup-support-in-scc
authors:
  - "@ropatil"
reviewers:
  - 
approvers:
  - 
api-approvers:
  - 
creation-date: 2024-11-21
last-updated: 2024-11-21
tracking-link:
  - https://issues.redhat.com/browse/CNTRLPLANE-1909
  - https://issues.redhat.com/browse/OCPBUGS-64663
  - https://issues.redhat.com/browse/OCPSTRAT-1735
status: Implementable, Ready for feature gate integration and enhancement proposal submission
---

## Summary

Add `runAsGroup` field support to OpenShift SecurityContextConstraints (SCC) to enable administrators to control the primary group ID (GID) that containers run with, providing complete security context control alongside the existing runAsUser, fsGroup, supplementalGroups fields.

## Motivation

OpenShift's SecurityContextConstraints currently lacks the `runAsGroup` field that is present in upstream Kubernetes' SecurityContext. 
Currently validates 3 out of 4 identity-related security fields from Kubernetes. The missing field, `runAsGroup`, this creates several critical issues:

1. **Incomplete Security Control**: Administrators can control UIDs via `runAsUser` but cannot enforce GID constraints, leaving a privilege escalation vector
2. **User Namespaces Feature Gap**: The User Namespaces feature (OCPSTRAT-1735) requires GID validation (0-65534 range), but without `runAsGroup` in SCC, validation fails at runtime instead of admission time
3. **Kubernetes Incompatibility**: Upstream Kubernetes validates `runAsGroup` in SecurityContext, but OpenShift SCCs cannot enforce policies on it
4. **Security Risk**: Containers can specify arbitrary GIDs, potentially accessing files/resources via group permissions they shouldn't have

**Affected Bug**: [OCPBUGS-64663](https://issues.redhat.com/browse/OCPBUGS-64663)

### Goals

- Add `runAsGroup` field to SecurityContextConstraints API v1
- Implement three validation strategies matching runAsUser patterns:
  - `MustRunAs`: Enforce specific GID
  - `MustRunAsRange`: Allow GID range(s)
  - `RunAsAny`: No restrictions
- Maintain 100% backward compatibility with existing SCCs and workloads
- Enable User Namespaces feature (OCPSTRAT-1735) to work correctly
- Achieve parity with upstream Kubernetes SecurityContext validation

### Non-Goals

- Changing behavior of existing SCC fields (runAsUser, fsGroup, supplementalGroups)
- Retrofitting runAsGroup validation to existing clusters before feature gate is enabled
- Creating new default SCCs beyond restricted-v3 (if needed)
- Validating group membership or group name resolution (only GID numeric values)

## Proposed Solution

Add `runAsGroup` field to SecurityContextConstraints with three validation strategies:
- **MustRunAs**: Enforce specific GID (e.g., exactly 1000)
- **MustRunAsRange**: Allow GID range (e.g., 1000-65534)
- **RunAsAny**: No restrictions (privileged workloads)

### Workflow Description

**Cluster Administrator** is a human responsible for configuring SecurityContextConstraints.

**Developer** is a human creating pod specifications.

**SCC Admission Controller** is the OpenShift component that validates pods against SCCs.

#### Workflow: Creating a Pod with runAsGroup

1. **Developer** writes a Pod spec with `securityContext.runAsGroup: 3000`
2. **Developer** submits pod: `oc create -f pod.yaml`
3. **SCC Admission Controller** receives admission request
4. **SCC Admission Controller** iterates through applicable SCCs by priority
5. For each SCC, **SCC Admission Controller** validates:
   - runAsUser (existing)
   - **runAsGroup (NEW)** ← This enhancement
   - fsGroup (existing)
   - supplementalGroups (existing)
   - capabilities (existing)
   - volumes (existing)
   - etc.
6. If **runAsGroup** validation passes for an SCC:
   - Continue validating other fields
   - If all fields pass, accept pod with that SCC
7. If **runAsGroup** validation fails:
   - Try next SCC
   - If no SCC accepts, reject pod with detailed error

#### Workflow: Validation Strategies

**MustRunAs Strategy**:
- Cluster Admin configures SCC with exact GID:
  ```yaml
  runAsGroup:
    type: MustRunAs
    ranges:
    - min: 1000
      max: 1000
  ```
- Pod must specify `runAsGroup: 1000` or omit it (will be defaulted to 1000)
- Any other value is rejected

**MustRunAsRange Strategy**:
- Cluster Admin configures SCC with GID range(s):
  ```yaml
  runAsGroup:
    type: MustRunAsRange
    ranges:
    - min: 1000
      max: 65534
  ```
- Pod can specify any GID in the range, e.g., `runAsGroup: 20000`
- GID outside range is rejected

**RunAsAny Strategy**:
- Cluster Admin configures SCC:
  ```yaml
  runAsGroup:
    type: RunAsAny
  ```
- Pod can specify any GID including 0 (root group)
- Used for privileged workloads

### Topology Considerations

#### Hypershift / Hosted Control Planes

No special considerations. The `runAsGroup` validation occurs in the API server admission phase, which exists in both:
- Traditional clusters (control plane on cluster)
- HyperShift clusters (control plane hosted separately)

The implementation is purely in the kube-apiserver admission plugin, with no node-level or cluster-topology-specific logic.

#### Standalone Clusters

Standard implementation applies. No topology-specific behavior.

#### Single-node Deployments

No special considerations. Admission validation is independent of node count.

### Implementation Details/Notes/Constraints

#### Implementation Structure

The implementation follows the same pattern as existing SCC strategies:

**Pattern**: `pkg/securitycontextconstraints/<field>/<strategy>.go`

**Existing Examples**:
- `user/mustrunas.go`, `user/mustrunasrange.go`, `user/runasany.go`
- `group/mustrunas.go`, `group/runasany.go`

**New Implementation**:
- `runasgroup/mustrunas.go` (MustRunAs strategy)
- `runasgroup/mustrunasrange.go` (MustRunAsRange strategy)
- `runasgroup/runasany.go` (RunAsAny strategy)

## Technical Architecture

```
Developer creates Pod
    ↓
API Server receives request
    ↓
SCC Admission Controller validates:
    - runAsUser ✅ (existing)
    - runAsGroup ✅ (NEW)
    - fsGroup ✅ (existing)
    - supplementalGroups ✅ (existing)
    ↓
If valid → Pod created
If invalid → Rejected with clear error

### Risks and Mitigations

User Namespaces Integration

**Risk**: runAsGroup range (0-65534) for User Namespaces might conflict with existing namespace UID/GID ranges.

**Likelihood**: Low - User Namespaces is a new feature, minimal existing usage.

**Mitigation**:
1. Coordinate releases: both features in same OCP version
2. Clear documentation on range selection
3. Validation tests ensuring ranges don't overlap incorrectly
4. `restricted-v3` SCC specifically designed for User Namespaces workloads

### Drawbacks

1. **API Complexity**: Adds another field to SCC API, increasing configuration surface area
- *Counterpoint*: Matches existing patterns (runAsUser, fsGroup), low learning curve

2. **Testing Burden**: Need to test combinations of runAsUser + runAsGroup + fsGroup
   - *Counterpoint*: Comprehensive unit tests already implemented (1,402 lines)

## Open Questions [optional]

1. **Default SCC Updates**: Should we update existing SCCs immediately or wait for 4.22?

2. **Backporting**: Should we backport to OCP 4.21.z for OCPBUGS-64663?


### Example shows deployment fails to validate runAsGroup field range values:
oc create -f 'dep_uid_gid.yaml'
deployment.apps/deployment-invalid-user-test-65535 created     pass 
deployment.apps/deployment-invalid-fsgroup-test-65535 created  pass 
deployment.apps/deployment-invalid-group-test-65535 created    fail 
deployment.apps/deployment-invalid-user-test-999 created       pass 
deployment.apps/deployment-invalid-group-test-999 created      fail 
deployment.apps/deployment-invalid-fsgroup-test-999 created    pass  
deployment.apps/deployment-valid-user-test-65534 created       pass 
deployment.apps/deployment-valid-fsgroup-test-65534 created    pass 
deployment.apps/deployment-valid-group-test-65534 created      pass 
deployment.apps/deployment-valid-user-test-1000 created        pass  
deployment.apps/deployment-valid-fsgroup-test-1000 created     pass  
deployment.apps/deployment-valid-group-test-1000 created       pass   

oc get deploy
NAME                                    READY   UP-TO-DATE   AVAILABLE   AGE
deployment-invalid-fsgroup-test-65535   0/1     0            0           34s
deployment-invalid-fsgroup-test-999     0/1     0            0           33s
deployment-invalid-group-test-65535     1/1     1            1           34s
deployment-invalid-group-test-999       1/1     1            1           33s
deployment-invalid-user-test-65535      0/1     0            0           34s
deployment-invalid-user-test-999        0/1     0            0           33s
deployment-valid-fsgroup-test-1000      1/1     1            1           30s
deployment-valid-fsgroup-test-65534     1/1     1            1           32s
deployment-valid-group-test-1000        1/1     1            1           30s
deployment-valid-group-test-65534       1/1     1            1           31s
deployment-valid-user-test-1000         1/1     1            1           31s
deployment-valid-user-test-65534        1/1     1            1           32s

oc get deploy/deployment-invalid-fsgroup-test-65535 -n testropatil -o yaml | yq ".status.conditions[1].message"
pods "deployment-invalid-fsgroup-test-65535-55bb879c68-" is forbidden: unable to validate against any security context constraint: provider restricted-v3: .spec.securityContext.fsGroup: Invalid value: [65535]: 65535 is not an allowed group

oc get deploy/deployment-invalid-fsgroup-test-999 -n testropatil -o yaml | yq ".status.conditions[1].message"
pods "deployment-invalid-fsgroup-test-999-69785c7477-" is forbidden: unable to validate against any security context constraint: provider restricted-v3: .spec.securityContext.fsGroup: Invalid value: [999]: 999 is not an allowed group

oc get deploy/deployment-invalid-group-test-65535 -n testropatil -o yaml | yq ".status.conditions[1].message"
ReplicaSet "deployment-invalid-group-test-65535-7c9d4594bd" has successfully progressed

oc get deploy/deployment-invalid-group-test-999 -n testropatil -o yaml | yq ".status.conditions[1].message"
ReplicaSet "deployment-invalid-group-test-999-68c45ffb69" has successfully progressed.

oc get deploy/deployment-invalid-user-test-65535 -n testropatil -o yaml | yq ".status.conditions[1].message"
pods "deployment-invalid-user-test-65535-76d99c87f6-" is forbidden: unable to validate against any security context constraint: provider restricted-v3: .containers[0].runAsUser: Invalid value: 65535: must be in the ranges: [1000, 65534]

oc get deploy/deployment-invalid-user-test-999 -n testropatil -o yaml | yq ".status.conditions[1].message"
pods "deployment-invalid-user-test-999-5597b946ff-" is forbidden: unable to validate against any security context constraint: provider restricted-v3: .containers[0].runAsUser: Invalid value: 999: must be in the ranges: [1000, 65534]

### With proper implementation it works as
oc create -f dep_uid_gid.yaml
deployment.apps/deployment-invalid-user-test-999 created
deployment.apps/deployment-invalid-group-test-999 created
deployment.apps/deployment-invalid-fsgroup-test-999 created
deployment.apps/deployment-valid-user-test-65534 created
deployment.apps/deployment-valid-group-test-65534 created
deployment.apps/deployment-valid-user-test-1000 created
deployment.apps/deployment-valid-group-test-1000 created
Error from server (Invalid): error when creating "dep_uid_gid.yaml": Deployment.apps "deployment-invalid-user-test-65535" is invalid: spec.template.spec.securityContext.runAsUser: Invalid value: 65535: must be between 0 and 65535 when user namespaces are enabled (hostUsers=false)
Error from server (Invalid): error when creating "dep_uid_gid.yaml": Deployment.apps "deployment-invalid-group-test-65535" is invalid: spec.template.spec.securityContext.runAsGroup: Invalid value: 65535: must be between 0 and 65535 when user namespaces are enabled (hostUsers=false)

NAME                                  READY   UP-TO-DATE   AVAILABLE   AGE
deployment-invalid-fsgroup-test-999   0/1     0            0           16s
deployment-invalid-group-test-999     0/1     0            0           17s
deployment-invalid-user-test-999      0/1     0            0           17s
deployment-valid-group-test-1000      1/1     1            1           15s
deployment-valid-group-test-65534     1/1     1            1           15s
deployment-valid-user-test-1000       1/1     1            1           15s
deployment-valid-user-test-65534      1/1     1            1           16s

oc get deploy/deployment-invalid-fsgroup-test-999 -n testropatil -o yaml | yq '.status.conditions[2].message'
pods "deployment-invalid-fsgroup-test-999-69785c7477-" is forbidden: unable to validate against any security context constraint: provider restricted-v3: .spec.securityContext.fsGroup: Invalid value: [999]: 999 is not an allowed group

oc get deploy/deployment-invalid-group-test-999 -n testropatil -o yaml | yq '.status.conditions[2].message'
pods "deployment-invalid-group-test-999-68c45ffb69-" is forbidden: unable to validate against any security context constraint: [provider restricted-v3: .spec.securityContext.runAsGroup: Invalid value: [999]: 999 is not an allowed group, provider restricted-v3: .containers[0].runAsGroup: Invalid value: [999]: 999 is not an allowed group]

oc get deploy/deployment-invalid-user-test-999 -n testropatil -o yaml | yq '.status.conditions[2].message'
pods "deployment-invalid-user-test-999-5597b946ff-" is forbidden: unable to validate against any security context constraint: provider restricted-v3: .containers[0].runAsUser: Invalid value: 999: must be in the ranges: [1000, 65534]