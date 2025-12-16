---
title: user-managed-kms-plugins
authors:
  - "@ardaguclu"
  - "@flavianmissi"
reviewers:
  - "@ardaguclu"
  - "@flavianmissi"
  - "@ibihim"
  - "@sjenning"
approvers:
  - "@benluddy"
api-approvers:
  - "@JoelSpeed"
creation-date: 2025-12-11
last-updated: 2025-12-11
tracking-link:
  - "https://issues.redhat.com/browse/OCPSTRAT-108"  # TP feature
  - "https://issues.redhat.com/browse/OCPSTRAT-1638" # GA feature
see-also:
  - "enhancements/kube-apiserver/encrypting-data-at-datastore-layer.md"
  - "enhancements/etcd/storage-migration-for-etcd-encryption.md"
replaces:
  - ""
superseded-by:
  - ""
---

# User-Managed KMS Plugins

## Summary

Enable users to deploy their own KMS (Key Management Service) plugins as static pods and configure OpenShift to use them for etcd encryption. Users manage the entire plugin lifecycle (deployment, configuration, updates, credentials) while OpenShift handles encryption operations and data migration.

## Motivation

Customers require KMS encryption for compliance and security, and Red Hat prefers not to manage the lifecycle of multiple external KMS provider plugins.
By allowing customers to manage their own kms plugin infrastructure, we establish a clear support boundary: Red Hat supports OpenShift's encryption controllers and APIs, users manage KMS plugin deployment and configuration.

### User Stories

* As a cluster admin, I want to deploy a standard upstream KMS plugin without modification, so that I can use any KMS provider that implements the Kubernetes KMS v2 API
* As a cluster admin, I want to configure OpenShift to use my KMS plugin by providing a socket path, so that OpenShift automatically handles encryption and migration
* As a cluster admin, I want to update my KMS plugin independently of OpenShift releases, so that I can patch CVEs or bugs immediately
* As a Red Hat support engineer, I want a clear support boundary where I support OpenShift components and customers manage their KMS plugins

### Goals

* Support any KMS provider implementing Kubernetes KMS v2 API
* Users deploy KMS plugins as static pods on control plane nodes
* Clear documentation with complete examples for common providers
* Minimal operational burden on Red Hat (no plugin lifecycle management)

### Non-Goals

* Red Hat managing KMS plugin deployment or lifecycle
* Providing KMS plugin container images
* Automatic credential provisioning for KMS authentication
* Support for KMS plugins deployed as regular workloads (Deployments/DaemonSets)
* Direct hardware security module (HSM) integration (only supported via KMS plugins)
* Support for KMS v1 API (only KMS v2 is supported)

## Proposal

Users deploy KMS plugins as static pods on each control plane node and configure the socket path in the `APIServer` CR. OpenShift API server operators validate connectivity to the user-deployed plugin and integrate it with the cluster's encryption infrastructure.

### Workflow Description

#### Initial KMS Configuration (AWS KMS Example)

1. User creates encryption key (KEK) in AWS KMS
2. User configures control plane node IAM roles with KMS permissions (`kms:Encrypt`, `kms:Decrypt`, `kms:DescribeKey`)
3. User creates socket directory on each control plane node with appropriate permissions:
   ```bash
   mkdir -p /var/run/kmsplugin
   chown 65532:65532 /var/run/kmsplugin  # nobody:nogroup
   chmod 0750 /var/run/kmsplugin
   ```
4. User creates static pod manifest on each control plane node:
   ```yaml
   # /etc/kubernetes/manifests/aws-kms-plugin.yaml
   apiVersion: v1
   kind: Pod
   metadata:
     name: aws-kms-plugin
     namespace: kube-system
   spec:
     hostNetwork: true # needed to use control plane node's IAM credentials via IMDS
     priorityClassName: system-node-critical
     containers:
     - name: aws-kms-plugin
       image: registry.k8s.io/kms/aws-encryption-provider:v0.3.0
       command:
       - /aws-encryption-provider
       - --key=arn:aws:kms:us-east-1:123456789012:key/12345678-1234-1234-1234-123456789012
       - --region=us-east-1
       - --listen=/var/run/kmsplugin/socket.sock
       - --health-port=:8083  # avoid conflict with kube-apiserver on :8080
       ports:
       - containerPort: 8083
         protocol: TCP
         name: healthz
       livenessProbe:
         httpGet:
           path: /healthz
           port: 8083
         initialDelaySeconds: 10
         periodSeconds: 10
       readinessProbe:
         httpGet:
           path: /healthz
           port: 8083
         initialDelaySeconds: 5
         periodSeconds: 5
       securityContext:
         runAsUser: 65532   # nobody
         runAsGroup: 65532  # nogroup
         allowPrivilegeEscalation: false
         capabilities:
           drop:
           - ALL
       volumeMounts:
       - name: kms-socket
         mountPath: /var/run/kmsplugin
     volumes:
     - name: kms-socket
       hostPath:
         path: /var/run/kmsplugin
         type: Directory  # must exist with correct permissions
   ```

5. User updates APIServer configuration:
   ```yaml
   apiVersion: config.openshift.io/v1
   kind: APIServer
   metadata:
     name: cluster
   spec:
     encryption:
       type: KMS
       kms:
         endpoint: unix:///var/run/kmsplugin/socket.sock
   ```

6. API server operators detect configuration change
7. API server operators generate `EncryptionConfiguration` with specified endpoint
8. API server operators validate KMS plugin is reachable (health check + Status call)
9. API server encryption controllers begin encrypting resources
10. User observes progress via `clusteroperator/kube-apiserver` conditions


### API Extensions

Users configure KMS plugins via the `APIServer` CR:

```yaml
apiVersion: config.openshift.io/v1
kind: APIServer
metadata:
  name: cluster
spec:
  encryption:
    type: KMS
    kms:
      endpoint: unix:///var/run/kmsplugin/socket.sock
```

API type definitions are in [KMS Encryption Foundations](kms-encryption-foundations.md).

### Topology Considerations

#### Hypershift / Hosted Control Planes

In Hypershift, users must deploy KMS plugins in the management cluster where hosted control plane API servers run. The plugin must be network-accessible from the management cluster's control plane namespace.

#### Standalone Clusters

Primary deployment model. Users deploy static pods on all control plane nodes.

#### Single-node Deployments or MicroShift

Supported. Single static pod on the control plane node. MicroShift may adopt this with file-based configuration instead of APIServer CR.

### Implementation Details/Notes/Constraints

#### Operator Responsibilities

**API Server Operators:**
1. Detect `spec.encryption.type: KMS` in APIServer CR
2. Generate `EncryptionConfiguration` with user-specified endpoint
3. Apply EncryptionConfiguration to API server static pods
4. Monitor API server pod readiness (which reflects KMS plugin health)
5. Surface API server degraded status via operator conditions and metrics
6. Trigger encryption controllers when configuration changes

**What operators do NOT do:**
- Deploy or manage KMS plugin pods
- Provision credentials
- Directly validate KMS plugin connectivity (operators lack host filesystem access)
- Restart or recover failed plugins
- Manage plugin lifecycle (updates, patches, configuration changes)

#### Access to KMS Plugin Socket

All three API servers can access the KMS plugin Unix socket:

- **kube-apiserver** (static pod, `hostNetwork: true`): Direct access via hostPath volume
- **openshift-apiserver** (Deployment, privileged): Can access host filesystem via hostPath volume
- **oauth-apiserver** (Deployment, privileged): Can access host filesystem via hostPath volume

API server operators configure all three API servers with the same socket path specified in the APIServer CR.

#### Validation and Health Checking

**How validation works:**

API server operators do not have direct access to the host filesystem where KMS plugin sockets reside (operators run as non-privileged pods without hostPath volumes). Instead, validation happens through API server pod health and continuous monitoring:

**When user configures KMS encryption:**
1. User updates APIServer CR with KMS endpoint
2. Each API server operator's state controller detects APIServer CR change (via informer):
   - `cluster-kube-apiserver-operator` for kube-apiserver
   - `cluster-openshift-apiserver-operator` for openshift-apiserver
   - `cluster-authentication-operator` for oauth-apiserver
3. Each API server operator generates EncryptionConfiguration with user-specified endpoint
4. Each API server operator applies EncryptionConfiguration secret to openshift-config-managed namespace
5. API server pods restart with new configuration (one pod at a time, per API server type)
6. Each API server pod reads EncryptionConfiguration from mounted secret
7. API server attempts to connect to KMS plugin and checks health via Status gRPC call
8. If plugin unavailable: API server waits and retries, readiness probe fails
9. If plugin healthy: API server becomes ready

**Continuous monitoring (after configuration is applied):**
1. API server continuously polls KMS plugin Status endpoint to verify health
2. API server exposes a consolidated health endpoint: `/healthz/kms-providers` (KMS v2 uses a single endpoint for all providers)
3. API server readiness probe checks `/readyz` endpoint, which includes KMS health checks
4. If plugin becomes unavailable during runtime:
   - Health endpoint returns unhealthy status
   - Encrypt/decrypt operations fail with errors
   - API server logs errors but may remain "ready" (can still serve non-secret requests)

**Operator monitoring:**
- Watch APIServer CR for configuration changes (via informer)
- Monitor API server pod readiness (readiness probe includes KMS health via `/readyz`)
- Surface degraded conditions when API server pods fail to become ready or report errors

**User debugging:**
- Check API server pod status: `oc get pods -n openshift-kube-apiserver`
- Check pod events: `oc describe pod -n openshift-kube-apiserver <pod-name>`
- Check pod logs for KMS errors: `oc logs -n openshift-kube-apiserver <pod-name> | grep -i kms`
- Check KMS health endpoint: `oc exec -n openshift-kube-apiserver <pod-name> -- curl localhost:6443/healthz/kms-providers`

#### Static Pod Deployment Requirements

**Why static pods:**
- Avoid circular dependency (kube-apiserver needs plugin to start, but regular pods need kube-apiserver)
- Plugin available before kube-apiserver starts
- No dependency on kube-scheduler or other control plane components
- Matches community best practice (AWS, Vault plugins all recommend static pods)

**User must:**
- Create manifest in `/etc/kubernetes/manifests/` on each control plane node
- Use `hostNetwork: true` for network access
- Set `priorityClassName: system-node-critical`
- Mount socket directory via hostPath
- Ensure plugin creates socket at configured path

#### Key Rotation (Tech Preview)

For Tech Preview, key rotation is a **manual operation**:

1. User becomes aware that external KMS has rotated the key
2. User deploys new KMS plugin static pod with different socket path (e.g., `/var/run/kmsplugin/socket-new.sock`)
3. User updates APIServer CR with new endpoint
4. OpenShift treats this as a provider migration (add new provider, migrate data, remove old provider)
5. User removes old plugin static pod after migration completes

**Automatic key rotation detection (monitoring `key_id` changes) is deferred to GA.**

#### Provider Migration

When switching KMS providers:
1. User deploys second static pod (new provider) with different socket path
2. User updates APIServer CR with new endpoint
3. API server operators create second `EncryptionConfiguration` with both endpoints
4. Migration proceeds automatically
5. User removes old static pod after completion

### Risks and Mitigations

#### Risk: User Deployment Errors

**Risk:** Users incorrectly configure static pods (wrong socket path, missing volume, etc.)

**Mitigation:**
- Comprehensive documentation with tested examples
- API server operators validate socket accessibility and provide specific error messages
- Troubleshooting guide for common mistakes

#### Risk: Circular Dependency with Regular Pods

**Risk:** Users deploy KMS plugin as Deployment/DaemonSet, creating bootstrap deadlock

**Mitigation:**
- Documentation explicitly requires static pods
- Explain circular dependency problem clearly
- Validation could warn if endpoint doesn't match expected static pod socket pattern

#### Risk: Credentials Management

**Risk:** Static pods cannot use Secret volumes, complicating credential management

**Mitigation:**
- Document authentication options per provider (IAM roles, cert files on host, etc.)
- Provide examples for each supported provider
- For AWS: Use IMDS (IAM instance profile)
- For Vault: Use cert-based auth with cert files on host

#### Risk: Plugin Unavailability

**Risk:** Plugin crashes or stops responding, blocking encryption/decryption

**Impact:** kube-apiserver readiness fails (since Kubernetes 1.16+), doesn't serve requests until plugin available

**Mitigation:**
- kube-apiserver has built-in KMS health checks (won't serve traffic if plugin unhealthy)
- API server operator conditions surface plugin status
- Users responsible for plugin reliability (monitor, restart, etc.)

### Drawbacks

1. **User operational burden:** Users must manually deploy and manage plugins on each control plane node
2. **No automatic updates:** Users responsible for plugin updates and CVE patching
3. **Limited troubleshooting support:** Red Hat can only help with OpenShift components, not user-deployed plugins
4. **Manual node access required:** Must SSH to control plane nodes to create static pod manifests
5. **Credential complexity:** Cannot use Secrets, must use alternative authentication methods

## Alternatives

### Alternative 1: Red Hat-Managed Plugins (Sidecar)

**Approach:** OpenShift API server operators automatically deploy KMS plugins as sidecar containers in API server pods, manage lifecycle, credentials, and updates.

**Pros:**
- Zero user operational burden
- Disruption-free upgrades guaranteed
- Unified troubleshooting (Red Hat owns all components)
- Automatic credential provisioning via Cloud Credential Operator

**Cons:**
- Red Hat must deeply understand 5+ external KMS systems
- Plugin updates tied to OpenShift release cycle
- Large support burden (IAM, Vault auth, PKCS#11, etc.)
- Users cannot update plugins independently

**Why not chosen:** Business decision - Red Hat prefers not to carry the support burden for managing multiple external KMS provider plugins.

### Alternative 2: Shim/Proxy Architecture

**Approach:** OpenShift provides shim (sidecar in API server) and socket proxy (user-deployed), users deploy plugins separately.

**Pros:**
- Clear support boundary (Red Hat: shim/proxy images, User: plugin deployment)
- Users update plugins independently
- Solves SELinux MCS isolation issues

**Cons:**
- Three-layer architecture (shim → proxy → plugin) adds complexity
- Additional network latency
- Users still manually deploy and manage components
- More troubleshooting surface area

**Why not chosen:** Adds complexity without clear benefits over direct user-managed approach. If users manage plugins anyway, simpler to access them directly.

## Open Questions

None - design finalized based on business direction.

## Test Plan

### Unit Tests

**Operator Validation:**
- Socket path validation (format, accessibility)
- gRPC connection handling
- Status call parsing
- Error message generation

**State Controller:**
- EncryptionConfiguration generation with KMS endpoint
- Multi-endpoint handling during migration

### Integration Tests

**E2E Flow:**
1. Deploy mock KMS plugin as static pod
2. Configure APIServer CR with endpoint
3. Verify EncryptionConfiguration created
4. Encrypt resources
5. Simulate provider migration (deploy second plugin, update CR, verify migration)
6. Remove KMS plugin, verify degraded condition

**Validation Tests:**
- Invalid socket path → API server operator degraded
- Plugin not responding → API server operator degraded
- Plugin returns errors → API server operator degraded

### Manual Testing

**Per KMS Provider:**
- AWS KMS: Complete setup with IAM roles
- Vault: Complete setup with cert authentication
- Verify all three API servers can encrypt/decrypt
- Test manual key rotation (deploy new plugin, update CR, migrate)
- Test provider migration (AWS → Vault)

## Graduation Criteria

### Tech Preview Acceptance Criteria

**Core Functionality:**
- ✅ Users can deploy standard upstream KMS plugins as static pods
- ✅ OpenShift encrypts resources using user-deployed plugins
- ✅ Manual provider migration works (for key rotation or provider changes)
- ✅ Basic validation and error reporting

**Documentation:**
- ✅ Complete setup guides for AWS KMS and Vault
- ✅ Static pod YAML examples (tested and working)
- ✅ Manual key rotation procedure documented
- ✅ Troubleshooting guide
- ✅ Clear support boundary documentation

**Feature Gate:**
- ✅ Behind `KMSEncryptionProvider` feature gate (disabled by default)

**Known Limitations:**
- ⚠️ Manual key rotation (automatic detection deferred to GA)
- ⚠️ User must monitor external KMS for key rotation events

### Tech Preview → GA

**Production Validation:**
- ✅ Validated with at least 2 KMS providers in production
- ✅ Load testing and performance benchmarks
- ✅ 6+ months of Tech Preview feedback incorporated

**Documentation:**
- ✅ Additional provider guides (Azure, GCP, Thales)
- ✅ Runbooks for common failure scenarios
- ✅ Migration guides (provider-to-provider, encryption type changes)

**Operational Readiness:**
- ✅ Metrics and alerts for KMS plugin health
- ✅ Dashboard integration
- ✅ SLI/SLO definitions

**New Features for GA:**
- ✅ Automatic key rotation detection (poll `key_id` from Status endpoint)
- ✅ Automatic migration on key rotation (no user intervention required)
- ✅ Key rotation observability (metrics, conditions, events)
- ✅ Support for KMS encryption configuration changes (e.g., provider migration, local encryption to KMS)

**Feature Gate:**
- Removed (enabled by default)

## Upgrade / Downgrade Strategy

### Upgrade

**From non-KMS to KMS encryption:**
1. User deploys KMS plugin static pods
2. User updates APIServer CR
3. OpenShift automatically migrates from `aescbc`/`aesgcm` to KMS

**During OpenShift upgrade:**
- KMS plugin static pods unaffected (user-managed)
- API server operators upgraded, continue using existing plugin
- No disruption if plugin remains available

### Downgrade

**From KMS to non-KMS encryption:**
1. User updates APIServer CR to `type: aescbc`
2. OpenShift migrates data from KMS to local keys
3. User removes KMS plugin static pods after migration

**OpenShift version downgrade:**
- If KMS enabled, must first disable or migrate to non-KMS
- Cannot downgrade with KMS encryption active

## Version Skew Strategy

**API server operator version skew:**
- API server operators at different versions can coexist
- All use same `EncryptionConfiguration` format
- KMS v2 API is stable across Kubernetes versions

**Plugin version skew:**
- KMS v2 API is stable
- Plugins implement standard gRPC interface
- No coordination required between OpenShift and plugin versions

## Operational Aspects of API Extensions

**SLIs:**
- `KMSPluginDegraded` condition on API server operators
- Metrics: `kms_plugin_status_healthy{endpoint}`
- Metrics: `kms_plugin_request_duration_seconds{operation}`

**Impact on existing SLIs:**
- API throughput: KMS operations add latency (~5-20ms per encrypted resource operation)
- API availability: Dependent on KMS plugin availability (user responsibility)

**Failure modes:**
- Plugin unavailable → kube-apiserver readiness fails, doesn't serve traffic
- Plugin slow → increased API latency, potential timeouts
- Plugin returns errors → encryption/decryption fails, surfaced to users

## Support Procedures

### Support Boundary

**Red Hat supports:**
- ✅ OpenShift encryption controllers and APIs
- ✅ EncryptionConfiguration generation
- ✅ Validation and error reporting
- ✅ Migration orchestration

**User responsible for:**
- ❌ KMS plugin deployment and configuration
- ❌ KMS plugin health and monitoring
- ❌ KMS provider configuration and credentials
- ❌ Plugin bugs or errors

### Troubleshooting

**"KMS plugin not found":**
1. Check static pod manifest exists: `ls /etc/kubernetes/manifests/`
2. Check pod running: `crictl pods | grep kms`
3. Check socket exists: `ls -la /var/run/kmsplugin/socket.sock`

**"Cannot connect to KMS plugin":**
1. Check socket permissions
2. Check plugin container logs: `crictl logs <container-id>`
3. Test socket manually: `grpcurl -unix /var/run/kmsplugin/socket.sock kmsv2.KeyManagementService/Status`

**"KMS plugin returns errors":**
- This is user responsibility (plugin or KMS provider issue)
- Verify plugin configuration (key ARN, region, credentials)
- Check KMS provider status and permissions
- Contact plugin vendor if needed

**"External KMS rotated keys, how do I migrate?":**
- For Tech Preview, this is a manual operation
- Follow the provider migration workflow (deploy new plugin, update APIServer CR)
- Automatic rotation detection will be available in GA

## Infrastructure Needed

**For Testing:**
- AWS account with KMS access
- Vault instance for testing
- Test clusters with control plane node access
- CI infrastructure to deploy static pods

**For Documentation:**
- Example static pod manifests for each provider
- Tested end-to-end setup procedures
- Video walkthrough (optional but helpful)
