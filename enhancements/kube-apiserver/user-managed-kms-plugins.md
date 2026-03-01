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

This feature lets users deploy their own KMS plugins. A KMS plugin is a tool that encrypts data using an external key management service.

Users deploy the plugin as a static pod (a pod that kubelet manages directly). Users configure OpenShift to use their plugin for encrypting etcd data.

Users manage the plugin's lifecycle: deployment, configuration, updates, and credentials. OpenShift handles encryption operations and data migration.

## Motivation

Customers need KMS encryption to meet compliance and security requirements. Red Hat does not want to manage the lifecycle of many different external KMS provider plugins.

When users manage their own KMS plugin infrastructure, the support boundary becomes clear:
- Red Hat supports OpenShift's encryption controllers and APIs
- Users manage KMS plugin deployment and configuration

### User Stories

* As a cluster admin, I want to deploy a standard upstream KMS plugin without changing it. This lets me use any KMS provider that follows the Kubernetes KMS v2 API standard.

* As a cluster admin, I want to configure OpenShift to use my KMS plugin by giving it a socket path. OpenShift should then handle encryption and migration automatically.

* As a cluster admin, I want to update my KMS plugin separately from OpenShift releases. This lets me patch security problems or bugs right away.

* As a Red Hat support engineer, I want a clear support boundary. I support OpenShift components. Customers manage their KMS plugins.

### Goals

* Support any KMS provider that implements the Kubernetes KMS v2 API
* Let users deploy KMS plugins as static pods on control plane nodes
* Provide clear documentation with complete examples for common providers
* Keep operational burden on Red Hat minimal (no plugin lifecycle management)

### Non-Goals

* Red Hat will not manage KMS plugin deployment or lifecycle
* Red Hat will not provide KMS plugin container images
* The system will not automatically provision credentials for KMS authentication
* We will not support KMS plugins deployed as regular workloads (Deployments or DaemonSets)
* We will not integrate directly with hardware security modules (HSMs). Users must use a KMS plugin to connect to an HSM.
* We will not support the KMS v1 API (only KMS v2)

## Proposal

Users deploy KMS plugins as static pods on each control plane node. They then configure the socket path in the `APIServer` custom resource.

OpenShift's API server operators validate that they can connect to the user's plugin. They integrate the plugin with the cluster's encryption system.

### Workflow Description

#### Initial KMS Configuration (AWS KMS Example)

1. Create an encryption key in AWS KMS. This is called the Key Encryption Key (KEK).

2. Configure control plane node IAM roles with KMS permissions:
   - `kms:Encrypt`
   - `kms:Decrypt`
   - `kms:DescribeKey`

3. Create a socket directory on each control plane node with the right permissions:
   ```bash
   mkdir -p /var/run/kmsplugin
   chown 65532:65532 /var/run/kmsplugin  # nobody:nogroup
   chmod 0750 /var/run/kmsplugin
   ```

4. Create a static pod manifest on each control plane node:
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

5. Update the APIServer configuration:
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

6. API server operators detect the configuration change.

7. API server operators generate an `EncryptionConfiguration` with the endpoint users specified.

8. API server operators validate that the KMS plugin is reachable. They do a health check and call the plugin's Status endpoint.

9. API server encryption controllers begin encrypting resources.

10. Users can watch progress through the `clusteroperator/kube-apiserver` conditions.


### API Extensions

Users configure KMS plugins through the `APIServer` custom resource:

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

The API type definitions are in [KMS Encryption Foundations](kms-encryption-foundations.md).

### Topology Considerations

#### Hypershift / Hosted Control Planes

In Hypershift, users must deploy KMS plugins in the management cluster. This is where the hosted control plane API servers run.

The plugin must be reachable over the network from the management cluster's control plane namespace.

#### Standalone Clusters

This is the primary deployment model. Users deploy static pods on all control plane nodes.

#### Single-node Deployments or MicroShift

These are supported. Users deploy a single static pod on the control plane node.

MicroShift may use file-based configuration instead of the APIServer custom resource.

### Implementation Details/Notes/Constraints

#### Operator Responsibilities

**API Server Operators do these things:**

1. Detect when users set `spec.encryption.type: KMS` in the APIServer custom resource

2. Generate an `EncryptionConfiguration` with the endpoint users specified

3. Apply the EncryptionConfiguration to API server static pods

4. Monitor API server pod readiness. When a pod is ready, the KMS plugin is healthy.

5. Report API server degraded status through operator conditions and metrics

6. Trigger encryption controllers when the configuration changes

**API Server Operators do NOT do these things:**

- Deploy or manage KMS plugin pods
- Provision credentials
- Directly validate KMS plugin connectivity. The operators run as non-privileged pods. They cannot access the host filesystem.
- Restart or recover failed plugins
- Manage plugin lifecycle (updates, patches, configuration changes)

#### Access to KMS Plugin Socket

All three API servers can access the KMS plugin Unix socket:

- **kube-apiserver**: This is a static pod with `hostNetwork: true`. It gets direct access through a hostPath volume.

- **openshift-apiserver**: This is a Deployment with privileged access. It can access the host filesystem through a hostPath volume.

- **oauth-apiserver**: This is a Deployment with privileged access. It can access the host filesystem through a hostPath volume.

API server operators configure all three API servers. They all use the same socket path users specified in the APIServer custom resource.

#### Validation and Health Checking

**How validation works:**

API server operators cannot access the host filesystem directly. They run as non-privileged pods without hostPath volumes.

Instead, validation happens through API server pod health and continuous monitoring:

**When users configure KMS encryption:**

1. Users update the APIServer custom resource with a KMS endpoint.

2. Each API server operator's state controller detects the change. The operators watch the APIServer custom resource through an informer (a Kubernetes watch mechanism):
   - `cluster-kube-apiserver-operator` handles kube-apiserver
   - `cluster-openshift-apiserver-operator` handles openshift-apiserver
   - `cluster-authentication-operator` handles oauth-apiserver

3. Each API server operator generates an EncryptionConfiguration with the endpoint users specified.

4. Each API server operator applies the EncryptionConfiguration secret to the openshift-config-managed namespace.

5. API server pods restart with the new configuration. They restart one pod at a time for each API server type.

6. Each API server pod reads the EncryptionConfiguration from a mounted secret.

7. The API server tries to connect to the KMS plugin. It checks health through a Status gRPC call.

8. If the plugin is unavailable:
   - The API server waits and retries
   - The readiness probe fails

9. If the plugin is healthy:
   - The API server becomes ready

**Continuous monitoring (after users apply the configuration):**

1. The API server continuously polls the KMS plugin Status endpoint. This verifies the plugin is healthy.

2. The API server exposes a health endpoint: `/healthz/kms-providers`. KMS v2 uses a single endpoint for all providers.

3. The API server readiness probe checks the `/readyz` endpoint. This includes KMS health checks.

4. If the plugin becomes unavailable during runtime:
   - The health endpoint returns an unhealthy status
   - Encrypt and decrypt operations fail with errors
   - The API server logs errors but may stay "ready". It can still serve requests that don't involve secrets.

**Operator monitoring:**

- The operators watch the APIServer custom resource for configuration changes. They use an informer.
- They monitor API server pod readiness. The readiness probe includes KMS health through `/readyz`.
- They report degraded conditions when API server pods fail to become ready or report errors.

**User debugging:**

- Check API server pod status: `oc get pods -n openshift-kube-apiserver`
- Check pod events: `oc describe pod -n openshift-kube-apiserver <pod-name>`
- Check pod logs for KMS errors: `oc logs -n openshift-kube-apiserver <pod-name> | grep -i kms`
- Check KMS health endpoint: `oc exec -n openshift-kube-apiserver <pod-name> -- curl localhost:6443/healthz/kms-providers`

#### Static Pod Deployment Requirements

**Why use static pods:**

- This avoids a circular dependency. The kube-apiserver needs the plugin to start. But regular pods need the kube-apiserver to run.
- The plugin is available before kube-apiserver starts.
- Users don't depend on kube-scheduler or other control plane components.
- This matches community best practice. AWS and Vault plugins both recommend static pods.

**Users must:**

- Create a manifest in `/etc/kubernetes/manifests/` on each control plane node
- Use `hostNetwork: true` for network access
- Set `priorityClassName: system-node-critical`
- Mount the socket directory through hostPath
- Make sure the plugin creates a socket at the configured path

#### Key Rotation (Tech Preview)

In Tech Preview, key rotation is a manual operation:

1. Users discover that the external KMS has rotated the key.

2. Users deploy a new KMS plugin static pod. Use a different socket path (for example, `/var/run/kmsplugin/socket-new.sock`).

3. Users update the APIServer custom resource with the new endpoint.

4. OpenShift treats this as a provider migration:
   - Add the new provider
   - Migrate data
   - Remove the old provider

5. Users remove the old plugin static pod after migration completes.

**Note:** Automatic key rotation detection is deferred to GA. In GA, the system will monitor `key_id` changes automatically.

#### Provider Migration

When users switch KMS providers:

1. Deploy a second static pod for the new provider. Use a different socket path.

2. Update the APIServer custom resource with the new endpoint.

3. API server operators create a second `EncryptionConfiguration`. It includes both endpoints.

4. Migration proceeds automatically.

5. Remove the old static pod after migration completes.

### Risks and Mitigations

#### Risk: User Deployment Errors

**Risk:** Users might configure static pods incorrectly. For example, they might use the wrong socket path or miss a volume.

**Mitigation:**
- We provide comprehensive documentation with tested examples
- API server operators validate that they can access the socket. They provide specific error messages.
- We include a troubleshooting guide for common mistakes

#### Risk: Circular Dependency with Regular Pods

**Risk:** Users might deploy the KMS plugin as a Deployment or DaemonSet. This creates a bootstrap deadlock.

**Mitigation:**
- Documentation explicitly requires static pods
- We explain the circular dependency problem clearly
- Validation could warn users if the endpoint doesn't match the expected static pod socket pattern

#### Risk: Credentials Management

**Risk:** Static pods cannot use Secret volumes. This makes credential management more complex.

**Mitigation:**
- We document authentication options for each provider. For example: IAM roles, certificate files on the host.
- We provide examples for each supported provider
- For AWS: Use IMDS (IAM instance profile)
- For Vault: Use certificate-based authentication with certificate files on the host

#### Risk: Plugin Unavailability

**Risk:** The plugin might crash or stop responding. This blocks encryption and decryption.

**Impact:** Since Kubernetes 1.16+, kube-apiserver readiness fails. It doesn't serve requests until the plugin is available.

**Mitigation:**
- kube-apiserver has built-in KMS health checks. It won't serve traffic if the plugin is unhealthy.
- API server operator conditions report plugin status
- Users are responsible for plugin reliability. Users must monitor it and restart it if needed.

### Drawbacks

1. **User operational burden:** Users must manually deploy and manage plugins on each control plane node.

2. **No automatic updates:** Users are responsible for plugin updates and CVE patching.

3. **Limited troubleshooting support:** Red Hat can only help with OpenShift components. We cannot help with user-deployed plugins.

4. **Manual node access required:** Users must SSH to control plane nodes to create static pod manifests.

5. **Credential complexity:** Users cannot use Secrets. Users must use alternative authentication methods.

## Alternatives

### Alternative 1: Red Hat-Managed Plugins (Sidecar)

**Approach:** OpenShift API server operators automatically deploy KMS plugins as sidecar containers. They run in API server pods. The operators manage lifecycle, credentials, and updates.

**Pros:**
- Users have zero operational burden
- Upgrades happen without disruption
- Troubleshooting is unified. Red Hat owns all components.
- Credentials are automatically provisioned through the Cloud Credential Operator

**Cons:**
- Red Hat must deeply understand 5+ external KMS systems
- Plugin updates are tied to the OpenShift release cycle
- This creates a large support burden. We must understand IAM, Vault authentication, PKCS#11, and more.
- Users cannot update plugins independently

**Why we didn't choose this:** This is a business decision. Red Hat prefers not to carry the support burden for managing multiple external KMS provider plugins.

### Alternative 2: Shim/Proxy Architecture

**Approach:** OpenShift provides a shim (a sidecar in the API server) and a socket proxy (which users deploy). Users deploy plugins separately.

**Pros:**
- The support boundary is clear. Red Hat provides shim and proxy images. Users handle plugin deployment.
- Users can update plugins independently
- This solves SELinux MCS isolation issues

**Cons:**
- The three-layer architecture (shim → proxy → plugin) adds complexity
- Users get additional network latency
- Users still manually deploy and manage components
- There is more surface area for troubleshooting

**Why we didn't choose this:** This adds complexity without clear benefits. If users manage plugins anyway, it's simpler to access them directly.

## Open Questions

None. We finalized the design based on business direction.

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

**End-to-end Flow:**

1. Deploy a mock KMS plugin as a static pod

2. Configure the APIServer custom resource with an endpoint

3. Verify that EncryptionConfiguration was created

4. Encrypt resources

5. Simulate provider migration:
   - Deploy a second plugin
   - Update the custom resource
   - Verify migration

6. Remove the KMS plugin. Verify the degraded condition.

**Validation Tests:**
- Invalid socket path → API server operator becomes degraded
- Plugin not responding → API server operator becomes degraded
- Plugin returns errors → API server operator becomes degraded

### Manual Testing

**Per KMS Provider:**

- AWS KMS: Complete setup with IAM roles
- Vault: Complete setup with certificate authentication
- Verify all three API servers can encrypt and decrypt
- Test manual key rotation:
  - Deploy a new plugin
  - Update the custom resource
  - Verify migration
- Test provider migration (AWS → Vault)

## Graduation Criteria

### Tech Preview Acceptance Criteria

**Core Functionality:**
- ✅ Users can deploy standard upstream KMS plugins as static pods
- ✅ OpenShift encrypts resources using user plugins
- ✅ Manual provider migration works (for key rotation or provider changes)
- ✅ Basic validation and error reporting work

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
- ⚠️ Users must monitor the external KMS for key rotation events

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
- ✅ Automatic migration on key rotation (no manual steps required)
- ✅ Key rotation observability (metrics, conditions, events)
- ✅ Support for KMS encryption configuration changes (for example, provider migration, local encryption to KMS)

**Feature Gate:**
- Removed (enabled by default)

## Upgrade / Downgrade Strategy

### Upgrade

**From non-KMS to KMS encryption:**

1. Users deploy KMS plugin static pods

2. Users update the APIServer custom resource

3. OpenShift automatically migrates from `aescbc` or `aesgcm` to KMS

**During OpenShift upgrade:**

- User KMS plugin static pods are not affected (users manage them)
- API server operators are upgraded. They continue using the existing plugin.
- There is no disruption if the plugin stays available

### Downgrade

**From KMS to non-KMS encryption:**

1. Users update the APIServer custom resource to `type: aescbc`

2. OpenShift migrates data from KMS to local keys

3. Users remove KMS plugin static pods after migration

**OpenShift version downgrade:**

- If KMS is enabled, users must first disable it or migrate to non-KMS
- Users cannot downgrade with KMS encryption active

## Version Skew Strategy

**API server operator version skew:**

- API server operators at different versions can coexist
- All operators use the same `EncryptionConfiguration` format
- The KMS v2 API is stable across Kubernetes versions

**Plugin version skew:**

- The KMS v2 API is stable
- Plugins implement a standard gRPC interface
- OpenShift and plugin versions don't need to be coordinated

## Operational Aspects of API Extensions

**SLIs:**
- `KMSPluginDegraded` condition on API server operators
- Metrics: `kms_plugin_status_healthy{endpoint}`
- Metrics: `kms_plugin_request_duration_seconds{operation}`

**Impact on existing SLIs:**
- API throughput: KMS operations add latency (~5-20ms per encrypted resource operation)
- API availability: This depends on KMS plugin availability (user responsibility)

**Failure modes:**
- Plugin unavailable → kube-apiserver readiness fails, doesn't serve traffic
- Plugin slow → increased API latency, potential timeouts
- Plugin returns errors → encryption/decryption fails, errors are shown to users

## Support Procedures

### Support Boundary

**Red Hat supports:**
- ✅ OpenShift encryption controllers and APIs
- ✅ EncryptionConfiguration generation
- ✅ Validation and error reporting
- ✅ Migration orchestration

**Users are responsible for:**
- ❌ KMS plugin deployment and configuration
- ❌ KMS plugin health and monitoring
- ❌ KMS provider configuration and credentials
- ❌ Plugin bugs or errors

### Troubleshooting

**"KMS plugin not found":**

1. Check that the static pod manifest exists: `ls /etc/kubernetes/manifests/`
2. Check that the pod is running: `crictl pods | grep kms`
3. Check that the socket exists: `ls -la /var/run/kmsplugin/socket.sock`

**"Cannot connect to KMS plugin":**

1. Check socket permissions
2. Check plugin container logs: `crictl logs <container-id>`
3. Test the socket manually: `grpcurl -unix /var/run/kmsplugin/socket.sock kmsv2.KeyManagementService/Status`

**"KMS plugin returns errors":**

- This is user responsibility (plugin or KMS provider issue)
- Verify plugin configuration (key ARN, region, credentials)
- Check KMS provider status and permissions
- Contact the plugin vendor if needed

**"External KMS rotated keys, how do users migrate?":**

- In Tech Preview, this is a manual operation
- Follow the provider migration workflow:
  - Deploy a new plugin
  - Update the APIServer custom resource
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
