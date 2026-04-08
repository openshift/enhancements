# Threat Modeling

**Category**: Engineering Practice  
**Applies To**: All OpenShift components  
**Last Updated**: 2026-04-08  

## Overview

Use the STRIDE framework to identify security threats during design and implementation.

## STRIDE Framework

| Threat | Question | Example |
|--------|----------|---------|
| **S**poofing | Can an attacker impersonate? | Fake ServiceAccount tokens |
| **T**ampering | Can data be modified? | ConfigMap injection |
| **R**epudiation | Can actions be denied? | Missing audit logs |
| **I**nformation Disclosure | Can data be leaked? | Secrets in logs |
| **D**enial of Service | Can availability be disrupted? | Resource exhaustion |
| **E**levation of Privilege | Can permissions be escalated? | RBAC bypass |

## When to Apply

- **Design phase**: New features, APIs, controllers
- **Code review**: Changes affecting security boundaries
- **Incident response**: Understanding attack vectors
- **Enhancement proposals**: Security impact analysis

## Process

### 1. Identify Assets

```markdown
## Assets
- Cluster credentials (etcd encryption keys)
- User workloads (containers, data)
- Control plane availability
- Node access
- Registry credentials
```

### 2. Map Data Flows

```
User → API Server → Operator → Managed Resource
  ↓         ↓           ↓            ↓
 RBAC    Webhook    Leader       Secrets
         Validation  Election
```

### 3. Apply STRIDE to Each Flow

**Example: Operator reads Secret**

| Threat | Risk | Mitigation |
|--------|------|-----------|
| Spoofing | Operator pod impersonated | Use ServiceAccount with limited RBAC |
| Tampering | Secret modified in transit | TLS between components |
| Information Disclosure | Secret logged | Sanitize logs, avoid printing secrets |
| Elevation of Privilege | Operator gains cluster-admin | Least privilege RBAC |

### 4. Document Mitigations

```go
// Mitigation: Information Disclosure
// Never log the entire secret
log.Info("Processing secret", "name", secret.Name) // OK
log.Info("Secret data", "data", secret.Data)       // NEVER

// Mitigation: Tampering
// Validate before processing
if err := validateSecret(secret); err != nil {
    return fmt.Errorf("invalid secret: %w", err)
}
```

## Common Threats in OpenShift

### Spoofing: ServiceAccount Token Theft

**Attack**: Attacker gains access to pod, steals `/var/run/secrets/kubernetes.io/serviceaccount/token`

**Mitigations**:
- Least privilege RBAC (minimum permissions)
- Short-lived tokens (TokenRequest API)
- Audit ServiceAccount usage
- Network policies (limit pod egress)

**Example**:
```yaml
# Bad: cluster-admin
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: my-operator
roleRef:
  kind: ClusterRole
  name: cluster-admin  # TOO BROAD!

# Good: Least privilege
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: my-operator
rules:
- apiGroups: [""]
  resources: ["configmaps"]
  verbs: ["get", "list", "watch"]
  # Only what's needed
```

### Tampering: Malicious Admission Webhooks

**Attack**: Attacker deploys webhook that modifies resources

**Mitigations**:
- Restrict webhook creation (RBAC)
- Validate webhook configurations
- Use `failurePolicy: Fail` for critical webhooks
- Monitor webhook mutations

**Example**:
```yaml
# Webhook protection
apiVersion: admissionregistration.k8s.io/v1
kind: ValidatingWebhookConfiguration
metadata:
  name: critical-validator
webhooks:
- name: validate.example.com
  failurePolicy: Fail  # Block on webhook failure
  admissionReviewVersions: ["v1"]
  clientConfig:
    service:
      name: validator
      namespace: openshift-validator
    caBundle: <CA_BUNDLE>
  rules:
  - operations: ["CREATE", "UPDATE"]
    apiGroups: ["security.openshift.io"]
    apiVersions: ["v1"]
    resources: ["securitycontextconstraints"]
```

### Information Disclosure: Secrets in Container Images

**Attack**: Credentials hardcoded in images or build logs

**Mitigations**:
- Mount secrets as volumes (not environment variables)
- Use external secret managers (Vault, AWS Secrets Manager)
- Scan images for secrets (pre-commit hooks)
- Never log secret values

**Example**:
```yaml
# Bad: Secret in environment variable (visible in pod spec)
env:
- name: API_KEY
  value: "sk-1234567890"  # NEVER!

# Good: Secret from volume
volumeMounts:
- name: api-key
  mountPath: /etc/secrets
  readOnly: true
volumes:
- name: api-key
  secret:
    secretName: api-credentials
```

```go
// Bad: Logging secrets
log.Info("Config loaded", "apiKey", config.APIKey)  // NEVER!

// Good: Redacted logging
log.Info("Config loaded", "apiKeyPresent", config.APIKey != "")  // OK
```

### Denial of Service: Resource Exhaustion

**Attack**: Create unlimited resources to exhaust cluster

**Mitigations**:
- Resource quotas per namespace
- Limit ranges for pods
- Admission webhooks to validate requests
- Rate limiting on APIs

**Example**:
```yaml
# Resource quota
apiVersion: v1
kind: ResourceQuota
metadata:
  name: namespace-quota
  namespace: tenant-workload
spec:
  hard:
    requests.cpu: "100"
    requests.memory: 200Gi
    persistentvolumeclaims: "10"
    
# Limit range
apiVersion: v1
kind: LimitRange
metadata:
  name: pod-limits
  namespace: tenant-workload
spec:
  limits:
  - max:
      cpu: "2"
      memory: 4Gi
    min:
      cpu: 100m
      memory: 128Mi
    type: Pod
```

### Elevation of Privilege: RBAC Escalation

**Attack**: User with limited permissions escalates to admin

**Mitigations**:
- Never grant `escalate` verb on Roles
- Audit RoleBinding changes
- Use separate ServiceAccounts per component
- Minimize use of ClusterRole

**Example**:
```yaml
# Bad: Allows privilege escalation
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: dangerous-role
rules:
- apiGroups: ["rbac.authorization.k8s.io"]
  resources: ["clusterroles", "clusterrolebindings"]
  verbs: ["*"]  # Can grant themselves cluster-admin!

# Good: Limited scope
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: safe-role
  namespace: my-namespace
rules:
- apiGroups: [""]
  resources: ["configmaps"]
  verbs: ["get", "list"]
  # Cannot escalate privileges
```

## OpenShift-Specific Considerations

### Multi-Tenancy

**Threat**: Tenant A accesses Tenant B's resources

**Mitigations**:
- Network policies (isolate namespaces)
- RBAC (namespace-scoped roles)
- SCCs (prevent privileged containers)
- Resource quotas (prevent resource starvation)

**Example**:
```yaml
# Network policy: deny all ingress by default
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: deny-all
  namespace: tenant-a
spec:
  podSelector: {}
  policyTypes:
  - Ingress
  - Egress
  # Empty ingress/egress = deny all
```

### Node Access

**Threat**: Container escapes to host node

**Mitigations**:
- SCCs (restrict privileged, hostPath, hostNetwork)
- SELinux enforcement (confine processes)
- Read-only root filesystems
- No host PID/IPC namespace sharing

**Example**:
```yaml
# SCC: Restricted
apiVersion: security.openshift.io/v1
kind: SecurityContextConstraints
metadata:
  name: restricted-custom
allowHostDirVolumePlugin: false
allowHostIPC: false
allowHostNetwork: false
allowHostPID: false
allowHostPorts: false
allowPrivilegedContainer: false
readOnlyRootFilesystem: true
runAsUser:
  type: MustRunAsRange
seLinuxContext:
  type: MustRunAs
```

### Webhook Vulnerabilities

**Threat**: Webhook allows malicious resource creation

**Mitigations**:
- Validate all inputs in webhooks
- Use `failurePolicy: Fail` for security webhooks
- Webhook timeout protection
- Monitor webhook performance

```go
// Webhook validation example
func (v *Validator) ValidatePod(pod *corev1.Pod) error {
    // Mitigation: Prevent privileged escalation
    if pod.Spec.SecurityContext != nil && 
       pod.Spec.SecurityContext.Privileged != nil && 
       *pod.Spec.SecurityContext.Privileged {
        return fmt.Errorf("privileged pods not allowed")
    }
    
    // Mitigation: Prevent host namespace access
    if pod.Spec.HostNetwork || pod.Spec.HostPID || pod.Spec.HostIPC {
        return fmt.Errorf("host namespace access not allowed")
    }
    
    return nil
}
```

## Documentation Template

```markdown
# Security Analysis: [Feature Name]

## Assets
- Cluster credentials
- User data
- Control plane availability

## Data Flows
```
User → API → Controller → Resource
```

## Threats (STRIDE)

### Spoofing
| Threat | Likelihood | Impact | Mitigation |
|--------|-----------|--------|-----------|
| ServiceAccount impersonation | Medium | High | Least privilege RBAC |

### Tampering
...

### Information Disclosure
...

## Mitigations Implemented
- [ ] RBAC follows least privilege
- [ ] Secrets never logged
- [ ] TLS for all communications
- [ ] Input validation on all webhooks

## Open Risks
- None

## Sign-off
Security review completed: YYYY-MM-DD
```

## Examples by Component

| Component | Threat Identified | Mitigation |
|-----------|------------------|-----------|
| console-operator | XSS in console UI | CSP headers, input sanitization |
| machine-api-operator | Cloud credentials exposure | Short-lived credentials, rotation |
| cluster-network-operator | Network policy bypass | Validation webhooks, default-deny |

## References

- **STRIDE**: https://learn.microsoft.com/en-us/azure/security/develop/threat-modeling-tool-threats
- **RBAC Guidelines**: [rbac-guidelines.md](./rbac-guidelines.md)
- **Secrets Management**: [secrets-management.md](./secrets-management.md)
- **OpenShift Security**: https://docs.openshift.com/container-platform/latest/security/
