---
title: api-tls-curves-config
authors:
  - richardsonnick
  - davidesalerno
reviewers:
  - dsalerno # OpenShift networking stack knowledge
approvers: 
  - joelanford
api-approvers:
  - everettraven
creation-date: 2025-11-19
last-updated: 2025-11-20
tracking-link: # link to the tracking ticket (for example: Jira Feature or Epic ticket) that corresponds to this enhancement
  - https://issues.redhat.com/browse/HPCASE-153
---

# OpenShift API TLS Curves Configuration 

## Summary

This enhancement adds the option to configure a list of supported TLS curves in the OpenShift API config server. This configuration mirrors the existing `ciphersuites` option in the OpenShift API config TLS settings.

## Motivation

As cryptographic standards evolve, there is a growing need to support Post-Quantum Cryptography (PQC) to protect against future threats. This enhancement contributes directly to the goal of enabling PQC support in OpenShift. It provides the mechanism to configure specific TLS curves in the OpenShift API, allowing administrators to explicitly enable PQC-ready curves such as ML-KEM. This ensures OpenShift clusters can be configured to meet emerging security compliance requirements and future-proof communications.

### User Stories

As an administrator, I want to explicitely set the supported TLS curves to ensure PQC readiness throughout OpenShift so that I can ensure the security of TLS communication in the era of quantum computing.

### Goals

To provide an interface that allows the setting of TLS curves to be used cluser wide.

This goal is part of the larger goal to:
 1. Provide the necessary knobs to specify a PQC ready TLS configuration in OpenShift.
 2. Improve the adaptability of the cluster's TLS configuration to provide support for the constantly evolving TLS landscape.

### Non-Goals

1. Overhauling the current process of TLS configuration in OpenShift. This change merely extends the current TLS options.

## Proposal

This proposal is to expose the ability to specify the TLS curves used in OpenShift components to the OpenShift administrator.
Currently, administrators can specify a custom TLS profile where they can specifically set which TLS ciphersuites and the minimum TLS version as opposed to using one of the preconfigured TLS profiles. Specifying the set of supported TLS curves will mirror this process of setting [supported ciphers and the minimum TLS version](https://github.com/openshift/api/blob/138912d4ee9944c989f593c51f15c41908155856/config/v1/types_tlssecurityprofile.go#L206). 

The current state of the OpenShift TLS stack uses a default set of curves with no way to specify them. This eases the burden on administators, however new quantum secure algorithms rely on a set of curves outside of the conventional default curves. For example, curves like [ML-KEM](https://www.ietf.org/archive/id/draft-connolly-tls-mlkem-key-agreement-05.html) provide a quantum safe mechanism for sharing secrets necessary for the TLS handshake, whereas curves like [X22519](https://datatracker.ietf.org/doc/html/rfc7748) (a commonly used conventional curve) are [weak against quantum computing](https://crypto.stackexchange.com/questions/59770/how-effective-is-quantum-computing-against-elliptic-curve-cryptography).

The ability to set curves explicitely will also make it possible to align our 
OpenShift TLS profiles to match the curves present in the [Mozilla TLS Profiles](https://wiki.mozilla.org/Security/Server_Side_TLS). 

This change will require working with OpenShift component owners to use this new field. The scope of this feature includes ensuring that appropriate components respect the new curves field when it is set in custom profiles. Adding default curves to the non-custom profiles (Old, Intermediate, Modern) is a separately scoped action and will be addressed in future work.

### Workflow Description

Administrators will use the [existing custom TLS security profile flow](https://docs.redhat.com/en/documentation/openshift_container_platform/4.20/html/security_and_compliance/tls-security-profiles#tls-profiles-ingress-configuring_tls-security-profiles) for setting the supported curves. 

Specifically administrators will use 

`oc edit IngressController default -n openshift-ingress-operator`

and edit the spec.tlsSecurityProfile field:

```
apiVersion: operator.openshift.io/v1
kind: IngressController
 ...
spec:
  tlsSecurityProfile:
    type: Custom 
    custom: 
      ciphers: 
      - ECDHE-RSA-CHACHA20-POLY1305
      minTLSVersion: VersionTLS13
      curves:
      - X25519MLKEM512
 ...
```

### API Extensions

- Adds a `curves` field to the `spec.tlsSecurityProfile` (https://github.com/openshift/api/pull/2583/files#diff-2101eac4196d9b14cf061c8a6a4d40f9d8e5a77fc2690f969e7293294218afe3R267)
- The addition of this field should not affect existing API behaviour

### Topology Considerations

#### Hypershift / Hosted Control Planes

Hypershift [does not currently consume custom TLS supported groups](https://github.com/openshift/hypershift/blob/6b0338c192c966a9c072bfc6af45202739e9e553/support/config/cipher.go#L30). However, this is planned in the future.

#### Standalone Clusters

N/A


#### Single-node Deployments or MicroShift

This change will effect the TLS profile of both single node and microshift deployments.

### Implementation Details/Notes/Constraints

#### Upstream Component TLS Curve Support

The following Kubernetes components lack explicit configuration support for TLS elliptic curves:

| Component | Status | Reference |
|-----------|--------|-----------|
| Kubelet | No Support | [types.go#L182](https://github.com/kubernetes/kubelet/blob/ce0febd5f9e2c0e97ef3b3161a9098ef3f34afcb/config/v1beta1/types.go#L182) |
| Etcd | No Support | [config.go#L250](https://github.com/etcd-io/etcd/blob/ed430d025f5d1ff33d997e42569073c55f3ef513/server/embed/config.go#L250) |
| Controller-manager | No Support | [kube-controller-manager.md](https://github.com/kubernetes/website/blob/8d4885bbb055ec4558520a021ab1ac65064cd896/content/en/docs/reference/command-line-tools-reference/kube-controller-manager.md#L972-L998) |
| Kube-scheduler | No Support | [serving.go#L66](https://github.com/kubernetes/kubernetes/blob/master/staging/src/k8s.io/apiserver/pkg/server/options/serving.go#L66) |

This limitation means that TLS curve configuration will primarily benefit components that use their own TLS implementations (such as Ingress controllers using HAProxy/OpenSSL) rather than components that rely on upstream Kubernetes code. Upstream support would need to be added for these components to honor TLS curve configuration.

#### Component Configuration Consumption

Different OpenShift components consume TLS configuration from different sources based on their operational context:

**1. API Server Components** (kube-apiserver, openshift-apiserver, oauth-server, etc.)
- Read TLS configuration from `apiserver.config.openshift.io/cluster`
- Component operators watch this object and regenerate configuration when it changes
- Example: The kube-apiserver operator reads the `tlsSecurityProfile` field and passes the curves to the kube-apiserver via command-line flags or configuration files

**2. Kubelet Configuration**
- Kubelet TLS configuration is managed through `kubeletconfig.machineconfiguration.openshift.io`
- Administrators can set a TLS profile (including curves) at this level:
  ```yaml
  apiVersion: machineconfiguration.openshift.io/v1
  kind: KubeletConfig
  metadata:
    name: custom-config
  spec:
    tlsSecurityProfile:
      type: Custom
      custom:
        minTLSVersion: VersionTLS13
        curves:
        - X25519MLKEM768
        - X25519
  ```
- The Machine Config Operator (MCO) watches `KubeletConfig` objects
- MCO renders this configuration into kubelet configuration files on nodes via MachineConfigs
- Kubelet reads the configuration from `/etc/kubernetes/kubelet.conf` or similar

**3. Ingress Controller**
- Ingress configuration is managed through `ingresscontroller.operator.openshift.io`
- Administrators configure TLS profiles (including curves) on the IngressController object:
  ```yaml
  apiVersion: operator.openshift.io/v1
  kind: IngressController
  metadata:
    name: default
    namespace: openshift-ingress-operator
  spec:
    tlsSecurityProfile:
      type: Custom
      custom:
        curves:
        - X25519MLKEM768
  ```
- The Ingress Operator watches IngressController objects
- The operator configures the ingress router pods with the specified TLS settings
- Router pods (typically HAProxy or similar) apply these settings to their TLS listeners

**4. General Pattern for Operators**

For operators managing components that need to respect TLS configuration:

1. **Watch** the appropriate configuration source:
   - `apiserver.config.openshift.io/cluster` for control plane components
   - Component-specific operator CRs (IngressController, KubeletConfig, etc.)

2. **Extract** the `tlsSecurityProfile` including the `curves` field

3. **Translate** to the component's native configuration format:
   - For Go components: Set `tls.Config.CurvePreferences`
   - For OpenSSL-based components: Use `SSL_CTX_set1_groups_list()` or configuration directives
   - For HAProxy: Use `curves` directive in configuration

4. **Apply** configuration by:
   - Regenerating configuration files
   - Restarting components (if hot-reload not supported)
   - Or triggering configuration reload (if supported)

5. **Report** status via operator conditions if configuration cannot be applied

**Configuration Precedence**

When multiple TLS configuration sources exist, components follow this precedence (highest to lowest priority):
1. Component-specific configuration (e.g., `IngressController.spec.tlsSecurityProfile`)
2. Category-level configuration (e.g., `KubeletConfig.spec.tlsSecurityProfile` for node components)
3. Cluster-wide default (e.g., `apiserver.config.openshift.io/cluster` for API server components)

This precedence model allows for centralized defaults with selective overrides where needed.

#### Default curve configuration
The [default openshift TLS profiles](https://docs.redhat.com/en/documentation/openshift_container_platform/4.20/html/security_and_compliance/tls-security-profiles#tls-profiles-understanding_tls-security-profiles) (Old, Intermediate, Modern) do not currently specify any curves, instead relying on the underlying TLS implementation to select a sensible default group. However, the default Mozilla TLS profiles (which OpenShift TLS profiles are based on) *do* specify curves. We are planning on specifically adding these curves to OpenShift's non-custom profiles in the future as a separately scoped action. This API change should expose the curves field first to allow components time to implement the consumption of these curves when set in custom profiles.

#### Go crypto/tls Implementation Limitations

Components using Go's `crypto/tls` library face specific limitations that affect curve and cipher suite configuration:

**TLS 1.3 Cipher Suite Configuration**

In TLS 1.3, Go's `crypto/tls` does not allow cipher suite configuration ([golang/go#29349](https://github.com/golang/go/issues/29349)). The `Config.CipherSuites` field is ignored for TLS 1.3 connections, and Go uses a hardcoded set of cipher suites. This means:
- Components using Go cannot honor custom cipher suite configurations when TLS 1.3 is used
- Administrators configuring `minTLSVersion: VersionTLS13` with custom cipher suites will find the cipher suite configuration is not applied by Go-based components
- This is a known limitation of the Go standard library and cannot be worked around by OpenShift components

**Curve Preferences Ordering**

Starting in Go 1.24, the semantics of `CurvePreferences` are changing ([golang/go#69393](https://github.com/golang/go/issues/69393)):
- `CurvePreferences` will no longer specify preference ordering
- Instead, it will be a list of enabled key exchanges, with `crypto/tls` automatically determining priority and key share selection
- This change is driven by Post-Quantum Cryptography requirements where the library needs to intelligently manage curve selection (e.g., sending both ML-KEM768X25519 and X25519 key shares)

**Implications for OpenShift Components**

Go-based components (which represent a significant portion of OpenShift) will have these constraints:
- When using TLS 1.3, configured cipher suites cannot be enforced
- Curve preference ordering may not be honored as specified by administrators
- Components should document these limitations in their operator conditions or status messages
- Administrators should be aware that Go-based components have reduced configurability compared to OpenSSL-based components

These limitations should be considered when evaluating component compliance with TLS configuration requirements.

#### Mismatching curves and ciphersuites
There is a case where the administrator could incorrectly specify a set of ciphersuites
that do not work with the configured curves. For example, using an RSA ciphersuite with an ECDHE curve (such as TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256 with P-256). The default behavior of OpenSSL and Go crypto/tls (both used extensively in OpenShift) is to fail at **TLS handshake time**. The TLS server instance will start normally, but when TLS clients attempt to handshake with the TLS server, the handshake will fail with a `handshake failure`.

To avoid this scenario, OpenShift should implement validation to prevent **known incompatible cipher-curve combinations**. A validation layer will be added to check for compatible combinations of curves and ciphersuites. If a known invalid combination is detected, the configuration will be rejected, informing the user of the incompatibility immediately rather than failing at runtime.

**Note**: This validation only covers known incompatible cipher-curve combinations, not validation of curve names themselves. Curve names (valid, invalid, or malformed) are accepted and passed to the underlying TLS implementation, which filters them as described in the "Handling unsupported curves in custom profiles" section below.

#### Handling unsupported curves in custom profiles

Custom TLS profiles follow a "use at your own risk" model that allows administrators 
with advanced cryptographic knowledge to configure specific parameters. This same 
model applies to curves as it does to existing cipher suite configuration.

**Configuration-time behavior:**
TLS implementations (OpenSSL, Go crypto/tls, HAProxy) accept arbitrary curve names and do not fail when configured with invalid, malformed, or unsupported curves. Instead, they silently filter out:
- **Invalid curve names**: Curves that are not recognized (e.g., typos like "X225519" instead of "X25519")
- **Malformed identifiers**: Curve strings that don't match expected naming patterns
- **Unsupported curves**: Valid curve names that the specific TLS library version doesn't support (e.g., PQC curves in older library versions)

The TLS implementation will proceed with only the valid and supported curves from the configured list.

**Important**: This behavior means administrators can configure curve lists that result in **no valid curves** being available, which will cause TLS handshake failures and render components inoperable. Manually setting curves in custom TLS profiles incurs significant risk and requires careful testing. See the [Support Procedures](#support-procedures) section for troubleshooting guidance.

**Runtime behavior:**
If no mutually supported curves remain after filtering, TLS handshakes will fail with errors like "no shared group". This is the expected and desired behavior—it ensures only supported cryptographic parameters are used.

**Why not validate at API level:**
Validating curve support at the API level would require maintaining a comprehensive 
registry of:
- All TLS implementation libraries used across OpenShift components
- Version-specific support matrices for each library
- Continuous updates as libraries evolve

This approach is infeasible and would create a maintenance burden that outweighs 
the benefit. Runtime failures provide clear, immediate feedback about incompatibilities.

**Recommended approach:**
- **Most users**: Use the predefined profiles (Old, Intermediate, Modern), which are 
  tested and guaranteed to work across all OpenShift components. These profiles will 
  be enhanced to include secure curve configurations in future work.
- **Advanced users**: Custom profiles are available for specific requirements (e.g., 
  early PQC adoption, compliance mandates). Administrators using custom profiles should:
  - Understand the cryptographic implications of their configuration
  - Test connectivity to critical services after applying changes
  - Use the tls-scanner tool to verify actual negotiated parameters
  - Monitor component logs for TLS handshake failures

This approach is consistent with the existing [custom TLS profile documentation](https://docs.redhat.com/en/documentation/openshift_container_platform/4.20/html/security_and_compliance/tls-security-profiles), 
which warns: "Use caution when using a Custom profile, because invalid 
configurations can cause problems."

### Risks and Mitigations

OpenShift components could forego utilizing the curves set in the API config. However, this is a risk
that exists in the current TLS config flow. This change will require coordination with component owners
to ensure compliance with the new TLS config field, particularly for custom profiles where administrators
explicitly set curves. For the initial scope of this enhancement, this may only apply when a custom profile
is used, but backing implementation for core components is considered a requirement for GA promotion.

### Drawbacks

N/A

## Alternatives (Not Implemented)

N/A

## Open Questions [optional]

N/A

## Test Plan

Utilize the `oc edit` and `oc describe` commands to verify that the API config server is exposing the correct list of curves.

Once components are onboarded to utilize these curves, the cluster will be scanned with the [tls-scanner tool](github.com/openshift/tls-scanner) to verify that TLS implemenations within OpenShift expose these curves as supported. It should also be verified that the TLS implementations will fallback to a default curve set when not specified.

### Dev Preview -> Tech Preview

- Ability to specify supported curves.

### Tech Preview -> GA

- **Backing implementation for core components to respect the curves field when set in custom profiles.** This is a GA blocker.
- Verify the general support for these curves using the [tls-scanner](github.com/openshift/tls-scanner)
- Ensure that key OpenShift components (ingress controller, API server, etc.) properly consume and apply the configured curves from custom TLS profiles

### Removing a deprecated feature

N/A


## Upgrade / Downgrade Strategy

In openshift versions where the TLS curves are not specified, components will not specify the set of curves to be used to their underlying TLS implementations. The TLS implementation should fallback to a sensible default set of curves when not set. This should be verified during the component onboarding work as outlined in the test plan.


## Version Skew Strategy

By default, TLS implementations (openssl, golang, etc...) fallback to a sensible default when curves are not set. Currently, openshift components that do not set curves exhibit this behavior. This should be verified during component onboarding.

## Operational Aspects of API Extensions

N/A

## Support Procedures

### Verifying Configuration

**Check configured curves:**
```bash
# For IngressController
oc get ingresscontroller default -n openshift-ingress-operator -o yaml | grep -A 10 tlsSecurityProfile

# For APIServer
oc get apiserver cluster -o yaml | grep -A 10 tlsSecurityProfile
```

**Test connectivity:**
After applying a custom curve configuration, test connectivity to critical services:
- OpenShift console access
- API server connectivity (`oc` commands)
- Application routes through ingress
- Internal service-to-service communication

### Troubleshooting

**Symptoms of curve misconfiguration:**
- TLS handshake failures in component logs
- "no shared group" errors
- "handshake failure" errors
- Inability to connect to services that were previously working

**Identifying the problem:**

1. **Check component logs for TLS errors:**
```bash
# Ingress router logs
oc logs -n openshift-ingress -l ingresscontroller.operator.openshift.io/deployment-ingresscontroller=default

# API server logs
oc logs -n openshift-kube-apiserver -l app=openshift-kube-apiserver
```

Look for errors containing:
- "tls: no supported group"
- "tls: handshake failure"
- "no shared group"

2. **Verify component is using curves:**
Use [tls-scanner](https://github.com/openshift/tls-scanner) to confirm which components are respecting the curve configuration and which may not have implemented support yet.

3. **Check for unsupported curves:**
If components are using older TLS library versions, they may not support newer curves (e.g., post-quantum curves like ML-KEM). Review component documentation for supported curve lists.

### Recovery Procedures

**Quick recovery - revert to predefined profile:**
If a custom curve configuration is causing issues, immediately revert to a predefined profile:

```bash
oc edit ingresscontroller default -n openshift-ingress-operator
```

Change from:
```yaml
spec:
  tlsSecurityProfile:
    type: Custom
    custom:
      curves:
      - X25519MLKEM768
      - X25519
```

To:
```yaml
spec:
  tlsSecurityProfile:
    type: Intermediate  # or Modern/Old depending on requirements
```

This will restore known-good curve defaults.

**Gradual recovery - adjust curve list:**
If only specific curves are causing problems:
1. Keep the Custom profile
2. Remove problematic curves from the list
3. Ensure at least one widely-supported curve remains (e.g., X25519, P-256)
4. Monitor logs and connectivity

**Full rollback:**
If needed, restore the previous configuration:
```bash
oc rollout undo ingresscontroller/default -n openshift-ingress-operator
```

### Prevention

- **Always include fallback curves:** When configuring custom curves (especially experimental ones like PQC curves), always include widely-supported curves in the list as fallbacks
- **Test in non-production first:** Apply custom curve configurations to development/staging clusters before production
- **Use predefined profiles when possible:** Most users should use Old/Intermediate/Modern profiles, which are tested across all components
- **Monitor after changes:** Watch component logs for 15-30 minutes after applying curve configuration changes