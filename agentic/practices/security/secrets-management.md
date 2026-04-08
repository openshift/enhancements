# Secrets Management

**Category**: Engineering Practice  
**Applies To**: All OpenShift components  
**Last Updated**: 2026-04-08  

## Overview

Secure handling of secrets (credentials, tokens, certificates) in OpenShift operators and controllers.

## Principle: Defense in Depth

Never rely on single security mechanism. Layer protections:

1. **Avoid secrets** (use RBAC, pod identity instead)
2. **External storage** (Vault, cloud KMS)
3. **Kubernetes Secrets** (encrypted at rest)
4. **Never log** secrets
5. **Rotate** credentials regularly
6. **Audit** secret access

## Secret Storage

### Kubernetes Secrets

```yaml
# Basic secret
apiVersion: v1
kind: Secret
metadata:
  name: api-credentials
  namespace: openshift-my-operator
type: Opaque
data:
  username: YWRtaW4=  # base64 encoded
  password: cGFzc3dvcmQ=
```

**Pros**:
- Native Kubernetes resource
- Encrypted at rest (etcd encryption)
- RBAC-controlled access

**Cons**:
- Base64 is not encryption
- Visible to cluster-admin
- No automatic rotation

### External Secret Stores

```yaml
# Reference secret from external store
apiVersion: external-secrets.io/v1beta1
kind: ExternalSecret
metadata:
  name: api-credentials
spec:
  refreshInterval: 1h
  secretStoreRef:
    name: vault-backend
    kind: SecretStore
  target:
    name: api-credentials
    creationPolicy: Owner
  data:
  - secretKey: password
    remoteRef:
      key: secret/data/api-credentials
      property: password
```

**Pros**:
- Centralized secret management
- Automatic rotation
- Audit trail
- Fine-grained access control

**Cons**:
- Additional infrastructure
- Dependency on external service

## Accessing Secrets

### Volume Mounts (Preferred)

```yaml
# Mount secret as volume
apiVersion: apps/v1
kind: Deployment
spec:
  template:
    spec:
      containers:
      - name: operator
        volumeMounts:
        - name: api-credentials
          mountPath: /etc/secrets
          readOnly: true
      volumes:
      - name: api-credentials
        secret:
          secretName: api-credentials
          defaultMode: 0400  # Read-only
```

```go
// Read from file
func loadAPIKey() (string, error) {
    data, err := os.ReadFile("/etc/secrets/api-key")
    if err != nil {
        return "", err
    }
    return string(data), nil
}
```

**Advantages**:
- Not visible in environment variables
- Can be updated without pod restart (with subPath)
- File permissions control

### Environment Variables (Use Sparingly)

```yaml
# Environment variable from secret
env:
- name: API_PASSWORD
  valueFrom:
    secretKeyRef:
      name: api-credentials
      key: password
```

**Disadvantages**:
- Visible in pod spec (`oc describe pod`)
- Visible in process listing
- Visible in crash dumps
- Cannot be updated without restart

**Use only when**:
- Application requires env vars
- Secret is low-sensitivity
- Convenience > security

### Direct API Access (Last Resort)

```go
// Read secret from Kubernetes API
func getSecret(ctx context.Context, client client.Client, name string) (*corev1.Secret, error) {
    secret := &corev1.Secret{}
    err := client.Get(ctx, types.NamespacedName{
        Name:      name,
        Namespace: "openshift-my-operator",
    }, secret)
    return secret, err
}
```

**Use only when**:
- Secret not known at pod start time
- Dynamic secret selection

## Logging Secrets

### NEVER Log Secrets

```go
// NEVER DO THIS
log.Info("Loaded config", "apiKey", config.APIKey)
log.Info("Secret", "data", secret.Data)
fmt.Printf("Password: %s\n", password)

// GOOD: Redacted logging
log.Info("Loaded config", "apiKeyPresent", config.APIKey != "")
log.Info("Secret loaded", "name", secret.Name)
fmt.Printf("Password: [REDACTED]\n")
```

### Sanitize Errors

```go
// Bad: Error exposes secret
if err := validateAPIKey(apiKey); err != nil {
    return fmt.Errorf("invalid API key %s: %w", apiKey, err)
}

// Good: Redacted error
if err := validateAPIKey(apiKey); err != nil {
    return fmt.Errorf("invalid API key: %w", err)
}
```

### Structured Logging

```go
// Use structured logging with field control
logger := log.WithValues("secretName", secret.Name)
// Don't add secret.Data to logger context

// When logging objects, implement Stringer
type Config struct {
    APIKey string
}

func (c Config) String() string {
    return fmt.Sprintf("Config{APIKey: [REDACTED]}")
}
```

## Secret Rotation

### Manual Rotation

```bash
# Update secret
oc create secret generic api-credentials \
  --from-literal=password=new-password \
  --dry-run=client -o yaml | oc apply -f -

# Restart pods to pick up new secret
oc rollout restart deployment/my-operator
```

### Automatic Rotation

```go
// Watch secret for changes
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
    return ctrl.NewControllerManagedBy(mgr).
        For(&corev1.Pod{}).
        Watches(&source.Kind{Type: &corev1.Secret{}}, 
            handler.EnqueueRequestsFromMapFunc(r.findPodsForSecret)).
        Complete(r)
}

func (r *Reconciler) findPodsForSecret(secret client.Object) []reconcile.Request {
    // Find pods using this secret
    // Return reconcile requests to restart pods
}
```

### Short-Lived Tokens

```go
// Use TokenRequest API for short-lived tokens
func (r *Reconciler) getToken(ctx context.Context) (string, error) {
    tokenRequest := &authenticationv1.TokenRequest{
        Spec: authenticationv1.TokenRequestSpec{
            ExpirationSeconds: int64Ptr(3600), // 1 hour
            Audiences:         []string{"my-api"},
        },
    }
    
    result, err := r.kubeClient.CoreV1().ServiceAccounts("openshift-my-operator").
        CreateToken(ctx, "my-operator", tokenRequest, metav1.CreateOptions{})
    if err != nil {
        return "", err
    }
    
    return result.Status.Token, nil
}
```

## Encryption at Rest

### etcd Encryption

```yaml
# Cluster-wide encryption config
apiVersion: config.openshift.io/v1
kind: APIServer
metadata:
  name: cluster
spec:
  encryption:
    type: aescbc  # or aesgcm
```

**Encrypts**:
- All Secrets
- All ConfigMaps (optional)

**Protects against**:
- etcd backup theft
- Disk theft

**Does NOT protect against**:
- cluster-admin access
- API server compromise

### Application-Level Encryption

```go
// Encrypt before storing in Secret
func encryptValue(plaintext string, key []byte) (string, error) {
    block, err := aes.NewCipher(key)
    if err != nil {
        return "", err
    }
    
    gcm, err := cipher.NewGCM(block)
    if err != nil {
        return "", err
    }
    
    nonce := make([]byte, gcm.NonceSize())
    if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
        return "", err
    }
    
    ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
    return base64.StdEncoding.EncodeToString(ciphertext), nil
}
```

**Use when**:
- Extra protection needed beyond etcd encryption
- Secrets shared across clusters
- Compliance requirements

## Secret Types

### Service Account Tokens

```yaml
# Automatic token (deprecated)
apiVersion: v1
kind: ServiceAccount
metadata:
  name: my-operator
# Kubernetes automatically creates token secret

# Better: Use TokenRequest API for short-lived tokens
```

### TLS Certificates

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: webhook-certs
type: kubernetes.io/tls
data:
  tls.crt: <base64-cert>
  tls.key: <base64-key>
```

### Docker Registry Credentials

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: registry-creds
type: kubernetes.io/dockerconfigjson
data:
  .dockerconfigjson: <base64-config>
```

### Cloud Provider Credentials

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: aws-credentials
  namespace: openshift-machine-api
type: Opaque
data:
  aws_access_key_id: <base64>
  aws_secret_access_key: <base64>
```

**Better**: Use cloud IAM roles instead of static credentials

```yaml
# AWS IRSA (IAM Roles for Service Accounts)
apiVersion: v1
kind: ServiceAccount
metadata:
  name: my-operator
  annotations:
    eks.amazonaws.com/role-arn: arn:aws:iam::123456789:role/my-operator-role
# No static credentials needed
```

## Security Best Practices

### 1. Minimize Secret Count

```go
// Bad: Separate secret per credential
secrets:
  - api-key-1
  - api-key-2
  - api-key-3

// Good: Single secret with multiple keys
secret:
  name: api-credentials
  data:
    key-1: ...
    key-2: ...
    key-3: ...
```

### 2. Use RBAC to Protect Secrets

```yaml
# Least privilege: read specific secret only
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: secret-reader
rules:
- apiGroups: [""]
  resources: ["secrets"]
  resourceNames: ["api-credentials"]  # Specific secret only
  verbs: ["get"]
```

### 3. Audit Secret Access

```yaml
# Enable audit logging for secret access
apiVersion: audit.k8s.io/v1
kind: Policy
rules:
- level: RequestResponse
  resources:
  - group: ""
    resources: ["secrets"]
```

### 4. Validate Secret Format

```go
func validateSecret(secret *corev1.Secret) error {
    // Check required keys exist
    requiredKeys := []string{"username", "password"}
    for _, key := range requiredKeys {
        if _, ok := secret.Data[key]; !ok {
            return fmt.Errorf("missing required key: %s", key)
        }
    }
    
    // Validate format
    password := string(secret.Data["password"])
    if len(password) < 12 {
        return fmt.Errorf("password too short")
    }
    
    return nil
}
```

### 5. Handle Missing Secrets Gracefully

```go
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    secret := &corev1.Secret{}
    err := r.Get(ctx, types.NamespacedName{
        Name:      "api-credentials",
        Namespace: req.Namespace,
    }, secret)
    
    if err != nil {
        if errors.IsNotFound(err) {
            // Secret missing - report degraded but don't crash
            r.setCondition(DegradedCondition, True, "SecretMissing", 
                "Required secret 'api-credentials' not found")
            return ctrl.Result{RequeueAfter: 1 * time.Minute}, nil
        }
        return ctrl.Result{}, err
    }
    
    // Continue with secret
    return r.reconcileWithSecret(ctx, secret)
}
```

## Common Pitfalls

### ❌ Secrets in Git

```bash
# NEVER commit secrets to git
# Add to .gitignore:
*.key
*.pem
credentials.yaml
secrets/
```

### ❌ Secrets in Container Images

```dockerfile
# NEVER bake secrets into images
# Bad:
COPY api-key.txt /etc/secrets/

# Good: Mount at runtime
# (no secret in Dockerfile)
```

### ❌ Secrets in Logs

```go
// Bad
log.Error(err, "Failed to authenticate", "password", password)

// Good
log.Error(err, "Failed to authenticate")
```

### ❌ Secrets in Error Messages

```go
// Bad
return fmt.Errorf("authentication failed with key %s", apiKey)

// Good
return fmt.Errorf("authentication failed")
```

## Examples by Component

| Component | Secret Type | Storage | Rotation |
|-----------|------------|---------|----------|
| machine-api-operator | Cloud credentials | Kubernetes Secret | Manual |
| cluster-image-registry | Registry credentials | Kubernetes Secret | Manual |
| cert-manager | TLS certificates | Kubernetes Secret | Automatic |

## References

- **Kubernetes Secrets**: https://kubernetes.io/docs/concepts/configuration/secret/
- **etcd Encryption**: https://docs.openshift.com/container-platform/latest/security/encrypting-etcd.html
- **Threat Modeling**: [threat-modeling.md](./threat-modeling.md)
- **RBAC Guidelines**: [rbac-guidelines.md](./rbac-guidelines.md)
