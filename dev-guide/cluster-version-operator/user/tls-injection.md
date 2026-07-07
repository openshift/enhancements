# TLS configuration injection for operators

This document describes how operators use the TLS injection feature to propagate cluster-wide [TLS](https://wikipedia.org/wiki/Transport_Layer_Security) settings.

## What is TLS injection?

The cluster-version operator (CVO) can inject a cluster-wide TLS configuration into the ConfigMap of operators that have opted in. The CVO gets the reference config, and merges it into the config of operators with the right annotation and kind. The updated operators then restart to apply the config change.

## What gets injected

The CVO reads `spec.tlsSecurityProfile` from the APIServer cluster resource `apiserver.config.openshift.io/cluster`.

The injected fields are
* `servingInfo.minTLSVersion`: e.g. `VersionTLS13` or `VersionTLS12`
* `servingInfo.cipherSuites`: ordered list of prefered cipher suites, as [supported by Go](https://pkg.go.dev/crypto/tls#pkg-constants)

If no TLS security profile is available, TLS injection is skipped and a diagnostic is printed to the CVO logs.

## Setting up TLS injection

### Cluster configuration

See [TLS security profile](https://docs.redhat.com/en/documentation/openshift_container_platform/latest/html/security_and_compliance/tls-security-profiles) to set the cluster-wide config.

For example, with `spec.tlsSecurityProfile.type: Modern` the resulting operator config would be

```yaml
data:
  config.yaml: |
    kind: GenericOperatorConfig
    apiVersion: operator.openshift.io/v1alpha1
    servingInfo:
      minTLSVersion: VersionTLS13
      cipherSuites:
        - TLS_AES_128_GCM_SHA256
        - TLS_AES_256_GCM_SHA384
        - TLS_CHACHA20_POLY1305_SHA256
```

### Operator configuration

For operator TLS injection to work, the operator must:

* Be annotated with `config.openshift.io/inject-tls: "true"` (any other value is ignored)
* Have one or more data structures with `kind: GenericOperatorConfig, APIVersion: operator.openshift.io/v1alpha1`

For example, with:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: my-operator-config
  namespace: openshift-my-operator
  annotations:
    config.openshift.io/inject-tls: "true"
data:
  config.yaml: |
    kind: GenericOperatorConfig
    apiVersion: operator.openshift.io/v1alpha1
    servingInfo:
      minTLSVersion: VersionTLS12
      cipherSuites:
        - TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256
  controller.yaml: |
    kind: GenericControllerConfig
    apiVersion: operator.openshift.io/v1
  
```

When the CVO updates this ConfigMap, it will
* Create or overwrite `config.yaml`'s `servingInfo.minTLSVersion` and `servingInfo.cipherSuites` fields.
* Ignore the `controller-config` entry (wrong kind/APIVersion).

### How the TLS stack uses this config

Although this config gives a lot of control, operators often ignore some of the nuance. In particular, operators using the Go TLS stack (all upstream OpenShift ones) ignore the ciphersuites preference order, and with TLS 1.3 ignore the ciphersuite list completely.

## When TLS injection happens

TLS injection occurs when applying or reconciling the configmap

1. During installation, apply the ConfigMap from the payload image
2. During normal operation, reconcile ConfigMap changes
3. During cluster upgrades, apply updated ConfigMaps

The CVO only updates the ConfigMap if the TLS settings differ from what's currently in the cluster. This prevents unnecessary updates and operator restarts.

## Operator Considerations

When using TLS injection, your operator should:

1. **Watch the ConfigMap**: Monitor for ConfigMap updates and reload configuration when TLS settings change
2. **Apply TLS settings**: Use the injected `minTLSVersion` and `cipherSuites` values when configuring your operator's serving endpoints
3. **Provide defaults**: Include reasonable default TLS settings in your release manifest in case TLS injection fails
4. **Test both modes**: Verify your operator works both with and without TLS injection

## Debugging

To verify TLS injection is working:

```console
# Check CVO logs for TLS injection messages
$ oc logs -n openshift-cluster-version deployment/cluster-version-operator | grep inject-tls

# Verify the ConfigMap has the expected TLS settings
$ oc get configmap -n openshift-my-operator my-operator-config -o yaml

# Check the APIServer TLS security profile
$ oc get apiserver cluster -o jsonpath='{.spec.tlsSecurityProfile}'
```

Look for:
* `ConfigMap ... has annotation config.openshift.io/inject-tls: true`
* `ConfigMap ... will apply observed minTLSVersion=... cipherSuites=...`
* `ConfigMap ... processing GenericOperatorConfig in key ...`
* `ConfigMap's ... entry ...`
* `ConfigMap ... updated TLS profile ...`
