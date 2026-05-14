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
last-updated: 2026-05-07
tracking-link: # link to the tracking ticket (for example: Jira Feature or Epic ticket) that corresponds to this enhancement
  - https://issues.redhat.com/browse/HPCASE-153
---

# OpenShift API TLS Groups Configuration

## Summary

This enhancement adds the option to configure a list of supported TLS groups in
the OpenShift API config server. This configuration mirrors the existing
`ciphersuites` option in the OpenShift API config TLS settings.

## Motivation

As cryptographic standards evolve, there is a growing need to support
Post-Quantum Cryptography (PQC) to protect against future threats. This
enhancement contributes directly to the goal of enabling PQC support in
OpenShift. It provides the mechanism to configure specific TLS groups in the
OpenShift API, allowing administrators to explicitly enable PQC-ready groups
such as X25519MLKEM768. This ensures OpenShift clusters can be configured to
meet emerging security compliance requirements and future-proof communications.

### User Stories

As an administrator, I want to explicitly set the supported TLS groups to ensure
PQC readiness throughout OpenShift so that I can ensure the security of TLS
communication in the era of quantum computing.

As an administrator, I want to rely on default TLS groups and default TLS
security profiles without explicitly configuring supported groups, so that my
cluster continues to work securely out of the box without requiring manual TLS
configuration.

### Goals

To provide an interface that allows the setting of TLS groups to be used cluster
wide.

This goal is part of the larger goal to:

1. Provide the necessary knobs to specify a PQC ready TLS configuration in
   OpenShift.
2. Improve the adaptability of the cluster's TLS configuration to provide
   support for the constantly evolving TLS landscape.

### Non-Goals

1. Overhauling the current process of TLS configuration in OpenShift. This
   change merely extends the current TLS options.

## Proposal

This proposal is to expose the ability to specify the TLS groups used in
OpenShift components to the OpenShift administrator. Currently, administrators
can specify a custom TLS profile where they can specifically set which TLS
ciphersuites and the minimum TLS version as opposed to using one of the
preconfigured TLS profiles. Specifying the set of supported TLS groups will
mirror this process of setting [supported ciphers and the minimum TLS version](https://github.com/openshift/api/blob/138912d4ee9944c989f593c51f15c41908155856/config/v1/types_tlssecurityprofile.go#L206).

The current state of the OpenShift TLS stack uses a default set of groups with
no way to specify them. This eases the burden on administrators, however new
quantum secure algorithms rely on a set of groups outside of the conventional
default groups. For example, [X25519MLKEM768](https://www.ietf.org/archive/id/draft-connolly-tls-mlkem-key-agreement-05.html)
provides a quantum safe mechanism for sharing secrets necessary for the TLS
handshake, whereas conventional groups like [X25519](https://datatracker.ietf.org/doc/html/rfc7748)
are [weak against quantum computing](https://crypto.stackexchange.com/questions/59770/how-effective-is-quantum-computing-against-elliptic-curve-cryptography).

The ability to set groups explicitly will also make it possible to align our
OpenShift TLS profiles to match the groups present in the [Mozilla TLS Profiles](https://wiki.mozilla.org/Security/Server_Side_TLS).

This change will require working with OpenShift component owners to use this new
field. The scope of this feature includes ensuring that appropriate components
respect the new groups field when it is set in custom profiles. Default groups
are being added to the non-custom profiles (Old, Intermediate, Modern) as part
of this implementation; see the [Default group configuration](#default-group-configuration)
section for details.

### Workflow Description

Administrators configure TLS groups cluster-wide via the `apiserver.config.openshift.io/cluster`
object, which serves as the global default for all cluster components (subject to
the `tlsAdherence` setting described in the
[centralized TLS config enhancement](centralized-tls-config.md)):

```bash
oc edit apiserver cluster
```

```yaml
apiVersion: config.openshift.io/v1
kind: APIServer
metadata:
  name: cluster
spec:
  tlsSecurityProfile:
    type: Custom
    custom:
      ciphers:
      - ECDHE-ECDSA-AES128-GCM-SHA256
      minTLSVersion: VersionTLS12
      groups:
      - X25519MLKEM768
      - X25519
```

In the example above, the APIServer configures the `tlsSecurityProfile` to use
TLS v1.2 or higher, with one cipher (`ECDHE-ECDSA-AES128-GCM-SHA256`), and two
supported groups (`X25519MLKEM768` and `X25519`) for key exchange.
`X25519MLKEM768` is preferred over `X25519` due to the ordering.

Component-specific objects can be used to override the cluster-wide default
for individual components. For example, to override the groups on the ingress
controller only:

```bash
oc edit IngressController default -n openshift-ingress-operator
```

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
      ciphers:
      - ECDHE-RSA-CHACHA20-POLY1305
      minTLSVersion: VersionTLS13
      groups:
      - X25519MLKEM768
```

In the example above, the Ingress Controller can restrict traffic to use TLS
v1.3 only, with one cipher (`ECDHE-RSA-CHACHA20-POLY1305`), and one supported
group (`X25519MLKEM768`) for key exchange.

See the [Component Configuration Consumption](#component-configuration-consumption)
section for the full list of configuration sources and their precedence.

### API Extensions

- Adds a `groups` field to the `spec.tlsSecurityProfile`
  (https://github.com/openshift/api/pull/2583/files#diff-2101eac4196d9b14cf061c8a6a4d40f9d8e5a77fc2690f969e7293294218afe3R267)
- The field is gated behind the `TLSCurvePreferences` feature gate, enabled in
  `DevPreviewNoUpgrade` and `TechPreviewNoUpgrade` tiers
- The addition of this field should not affect existing API behaviour
- **Component implementors do not need to check for the feature gate.** Because
  the field is optional (`+optional`, `omitempty`), components need only inspect
  the field's value when unmarshaling the TLS security profile. When the feature
  gate is disabled the field will never be set, so components will continue
  using the TLS implementation's defaults transparently. When the field is
  present, components use the specified groups; when absent, they fall back to
  defaults. This mirrors the approach used for `tlsAdherence` in
  [openshift/api#2583](https://github.com/openshift/api/pull/2583).

### Topology Considerations

#### Hypershift / Hosted Control Planes

For HyperShift deployments, the TLS security profile for hosted control plane
components is determined by the **management cluster**, not the hosted cluster.
Hosted cluster administrators should not control TLS groups for components
running in the provider's domain.

HyperShift's control-plane-operator (CPO) directly manages a set of control
plane components that, in standalone clusters, are managed through the normal
SLO/operand chain. These components fall into three categories, each requiring
a different mechanism for consuming TLS group configuration:

**Category 1 — HyperShift-aware SLOs:**

In standalone clusters, these operators watch `apiserver.config.openshift.io/cluster`
and configure their operands. In HyperShift, these SLOs are run directly by the
CPO and have access to the management cluster's KAS. They should read the TLS
profile (including `groups`) from the `HostedCluster` CR spec in the management
KAS rather than from `apiserver.config.openshift.io/cluster` in the hosted
cluster's KAS.

**Category 2 — Operands of SLOs (most components):**

In standalone clusters, these components are configured by their SLO via
command-line flags or environment variables. In HyperShift, the CPO replaces
the SLO's role and must directly set the TLS-related flags and environment
variables on their pod specs, including any group configuration derived from
the `HostedCluster` CR.

**Category 3 — Self-configuring components:**

In standalone clusters, these components watch `apiserver.config.openshift.io/cluster`
and configure their own TLS servers directly. In HyperShift, such components
may be watching the wrong config object (the hosted cluster's rather than the
management cluster's). These components should accept CLI flags for TLS
settings that take precedence over the watched config object. The CPO sets
these flags when deploying the component.

This categorization mirrors the approach described in the
[centralized TLS config enhancement](centralized-tls-config.md#hypershift--hosted-control-planes).

#### Standalone Clusters

This is the primary target for this enhancement. Standalone clusters benefit
directly from the ability to configure TLS groups cluster-wide via
`apiserver.config.openshift.io/cluster`. Operators that already watch this
object for TLS profile changes will be updated to read the new `groups` field
and pass it through to their respective components.

#### Single-node Deployments or MicroShift

This change will affect the TLS profile of both single node and microshift
deployments.

#### OpenShift Kubernetes Engine

N/A

### Implementation Details/Notes/Constraints

#### Component Configuration Consumption

Different OpenShift components consume TLS configuration from different sources
based on their operational context:

**1. API Server Components** (kube-apiserver, openshift-apiserver, oauth-server, etc.)

- Read TLS configuration from `apiserver.config.openshift.io/cluster`
- Component operators watch this object and regenerate configuration when it
  changes
- When `tlsAdherence: StrictAllComponents` is set, all components must honor
  the `groups` field from this object. When `tlsAdherence` is unset or
  `LegacyAdheringComponentsOnly`, only components that already honor the
  cluster-wide TLS profile are required to do so
- Component implementors should use the `ShouldHonorClusterTLSProfile` helper
  function from library-go (rather than checking `tlsAdherence` values
  directly) to determine whether to apply the configured groups
- Example: The kube-apiserver operator reads the `tlsSecurityProfile` field
  (including `groups`) and passes the configuration to the kube-apiserver via
  command-line flags or configuration files

**2. Kubelet Configuration**

- Kubelet TLS configuration is managed through
  `kubeletconfig.machineconfiguration.openshift.io`
- Administrators can set a TLS profile (including groups) at this level:

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
        groups:
        - X25519MLKEM768
        - X25519
  ```

- The Machine Config Operator (MCO) watches `KubeletConfig` objects
- MCO renders this configuration into kubelet configuration files on nodes via
  MachineConfigs
- Kubelet reads the configuration from `/etc/kubernetes/kubelet.conf` or similar

**3. Ingress Controller**

- Ingress configuration is managed through
  `ingresscontroller.operator.openshift.io`
- Administrators configure TLS profiles (including groups) on the
  IngressController object:

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
        groups:
        - X25519MLKEM768
  ```

- The Ingress Operator watches IngressController objects
- The operator configures the ingress router pods with the specified TLS
  settings
- Router pods (typically HAProxy or similar) apply these settings to their TLS
  listeners

**4. General Pattern for Operators**

For operators managing components that need to respect TLS configuration:

1. **Watch** the appropriate configuration source:
   - `apiserver.config.openshift.io/cluster` — the cluster-wide default for all
     components when `tlsAdherence: StrictAllComponents` is set, or for
     legacy-adhering components when `tlsAdherence` is unset or
     `LegacyAdheringComponentsOnly`
   - Component-specific operator CRs (IngressController, KubeletConfig, etc.)
     when a per-component override is needed
   - `HostedCluster` CR in the management KAS — for HyperShift-aware SLOs
     running in HyperShift (Category 1 components)

2. **Extract** the `tlsSecurityProfile` including the `groups` field

3. **Translate** to the component's native configuration format:
   - For Go components: Set `tls.Config.CurvePreferences`
   - For OpenSSL-based components: Use `SSL_CTX_set1_groups_list()` or
     configuration directives
   - For HAProxy: Use `curves` directive in configuration

4. **Apply** configuration by:
   - Regenerating configuration files
   - Restarting components (if hot-reload not supported)
   - Or triggering configuration reload (if supported)

5. **Report** status via operator conditions if configuration cannot be applied

**Configuration Precedence**

When multiple TLS configuration sources exist, components follow this
precedence:

1. Component-specific configuration (e.g.,
   `IngressController.spec.tlsSecurityProfile`)
2. Category-level configuration (e.g.,
   `KubeletConfig.spec.tlsSecurityProfile` for node components)
3. Cluster-wide default (e.g., `apiserver.config.openshift.io/cluster` for API
   server components)

This precedence model allows for centralized defaults with selective overrides
where needed.

#### Default group configuration

The named TLS profiles (Old, Intermediate, Modern) are updated as part of this
implementation to include default groups based on Go's `crypto/tls` default
group preferences: `X25519`, `SecP256r1`, `SecP384r1`, and `X25519MLKEM768`.
Note that the [Mozilla TLS Profiles](https://wiki.mozilla.org/Security/Server_Side_TLS)
(version 5.7) do not include `X25519MLKEM768`; Go's defaults are used here to
align with the runtime behavior of Go-based OpenShift components and to enable
post-quantum hybrid key exchange where supported.

**FIPS mode constraint:** `X25519` and `X25519MLKEM768` are not FIPS-approved.
Components running in FIPS mode must omit these groups. The FIPS-approved
post-quantum alternatives (`SecP256r1MLKEM768`, `SecP384r1MLKEM1024`) require
Go 1.26+ and are not currently supported.

**Library-go update:** To support these defaults, the `TLSProfiles` mapping in
[library-go](https://github.com/openshift/library-go) will be updated to include
the `groups` field for each named profile. Operators and components that
currently use the library-go `TLSProfiles` mapping to build their `tls.Config`
will automatically pick up the new group defaults after updating their
library-go dependency. Component owners must ensure their library-go version
includes this update as part of the GA promotion work.

#### Mismatching groups and ciphersuites

There is a case where the administrator could incorrectly specify a set of
ciphersuites that do not work with each other. For example, using an RSA
ciphersuite with an ECDHE group (such as `TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256`
and `P-256`). The default behavior of OpenSSL as well as Go's crypto/tls (both
used extensively in OpenShift) is to fail at **TLS handshake time**. The TLS
server instance will start normally, but when TLS clients attempt to handshake
with the TLS server, the handshake will fail with a `handshake failure`.

To avoid this scenario, OpenShift should implement validation to prevent known
invalid combinations. A validation layer will be added to check for compatible
combinations of groups and ciphersuites. If a known invalid combination is
detected, the configuration will be rejected, informing the user of the
incompatibility immediately rather than failing at runtime. This validation
will be implemented as CEL validation expressions on the CRD, consistent with
the approach used for TLS 1.3 cipher validation described in the
[centralized TLS config enhancement](centralized-tls-config.md).

#### Handling unsupported groups in custom profiles

Custom TLS profiles follow a "use at your own risk" model that allows
administrators with advanced cryptographic knowledge to configure specific
parameters. This same model applies to groups as it does to existing cipher
suite configuration.

**Configuration-time behavior:**
TLS implementations (OpenSSL, Go crypto/tls) do not fail when configured with
unsupported groups or ciphers. Instead, they silently filter out unsupported
items and proceed with the valid ones.

**Runtime behavior:**
If no mutually supported groups (or ciphers) remain after filtering, TLS
handshakes will fail with errors like "handshake failure" (for cipher suites)
or "no shared group" (for groups). This is the expected and desired
behavior — it ensures only supported cryptographic parameters are used.

**API-level validation against the IANA registry:**
Rather than foregoing all API-level validation, group names will be validated
against the [IANA TLS Supported Groups registry](https://www.iana.org/assignments/tls-parameters/tls-parameters.xhtml#tls-parameters-8).
This approach provides a meaningful improvement over completely unvalidated
strings while avoiding the infeasible task of tracking per-library support
matrices:

- Group names not present in the IANA registry are rejected at the API level,
  providing immediate feedback for clearly invalid values
- Group names that are valid per the IANA registry but unsupported by a
  specific component version still produce runtime errors — this is the
  expected "use at your own risk" behavior for custom profiles
- Automation (a periodic job or CI check) queries the IANA registry and updates
  the CEL allowlist in the CRD to keep it current as new groups are
  standardized

This is strictly an improvement over completely unvalidated strings and avoids
the maintenance burden of a per-library, version-specific support matrix.

**Recommended approach:**

- **Most users**: Use the predefined profiles (Old, Intermediate, Modern), which
  are tested and guaranteed to work across all OpenShift components. These
  profiles include secure default group configurations.
- **Advanced users**: Custom profiles are available for specific requirements
  (e.g., early PQC adoption, compliance mandates). Administrators using custom
  profiles should:
  - Understand the cryptographic implications of their configuration
  - Test connectivity to critical services after applying changes
  - Use the tls-scanner tool to verify actual negotiated parameters
  - Monitor component logs for TLS handshake failures

This approach is consistent with the existing [custom TLS profile documentation](https://docs.redhat.com/en/documentation/openshift_container_platform/4.20/html/security_and_compliance/tls-security-profiles),
which warns: "Use caution when using a Custom profile, because invalid
configurations can cause problems."

### Risks and Mitigations

OpenShift components could forego utilizing the groups set in the API config.
However, this is a risk that exists in the current TLS config flow. This change
will require coordination with component owners to ensure compliance with the
new TLS config field, particularly for custom profiles where administrators
explicitly set groups. For the initial scope of this enhancement, this may only
apply when a custom profile is used, but backing implementation for core
components is considered a requirement for GA promotion.

### Drawbacks

N/A

## Alternatives (Not Implemented)

N/A

## Open Questions [optional]

N/A

## Test Plan

Utilize the `oc edit` and `oc describe` commands to verify that the API config
server is exposing the correct list of groups.

Once components are onboarded to utilize these groups, the cluster will be
scanned with the [tls-scanner tool](github.com/openshift/tls-scanner) to verify
that TLS implementations within OpenShift expose these groups as supported. It
should also be verified that the TLS implementations will fallback to a default
group set when not specified.

## Graduation Criteria

### Dev Preview -> Tech Preview

- Ability to specify supported groups via the `TLSCurvePreferences` feature
  gate.

### Tech Preview -> GA

- **Backing implementation for core components to respect the groups field when
  set in custom profiles.** This is a GA blocker.
- Verify the general support for these groups using the
  [tls-scanner](github.com/openshift/tls-scanner)
- Ensure that key OpenShift components (ingress controller, API server, etc.)
  properly consume and apply the configured groups from custom TLS profiles

### Removing a deprecated feature

N/A

## Upgrade / Downgrade Strategy

In OpenShift versions where the TLS groups are not specified, components will
not specify the set of groups to be used to their underlying TLS
implementations. The TLS implementation should fallback to a sensible default
set of groups when not set. This should be verified during the component
onboarding work as outlined in the test plan.

## Version Skew Strategy

By default, TLS implementations (openssl, golang, etc.) fallback to a sensible
default when groups are not set. Currently, OpenShift components that do not set
groups exhibit this behavior. This should be verified during component
onboarding.

## Operational Aspects of API Extensions

N/A

## Support Procedures

### Verifying Configuration

**Check configured groups:**

```bash
# For IngressController
oc get ingresscontroller default -n openshift-ingress-operator -o yaml | grep -A 10 tlsSecurityProfile

# For APIServer
oc get apiserver cluster -o yaml | grep -A 10 tlsSecurityProfile
```

**Test connectivity:**
After applying a custom group configuration, test connectivity to critical
services:

- OpenShift console access
- API server connectivity (`oc` commands)
- Application routes through ingress
- Internal service-to-service communication

### Troubleshooting

**Symptoms of group misconfiguration:**

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

2. **Verify component is using groups:**
Use [tls-scanner](https://github.com/openshift/tls-scanner) to confirm which
components are respecting the group configuration and which may not have
implemented support yet.

3. **Check for unsupported groups:**
If components are using older TLS library versions, they may not support newer
groups (e.g., post-quantum groups like X25519MLKEM768). Review component
documentation for supported group lists.

### Recovery Procedures

**Quick recovery - revert to predefined profile:**
If a custom group configuration is causing issues, immediately revert to a
predefined profile:

```bash
oc edit ingresscontroller default -n openshift-ingress-operator
```

Change from:

```yaml
spec:
  tlsSecurityProfile:
    type: Custom
    custom:
      groups:
      - X25519MLKEM768
      - X25519
```

To:

```yaml
spec:
  tlsSecurityProfile:
    type: Intermediate  # or Modern/Old depending on requirements
```

This will restore known-good group defaults.

**Gradual recovery - adjust group list:**
If only specific groups are causing problems:

1. Keep the Custom profile
2. Remove problematic groups from the list
3. Ensure at least one widely-supported group remains (e.g., X25519, P-256)
4. Monitor logs and connectivity

**Full rollback:**
If needed, restore the previous configuration:

```bash
oc rollout undo ingresscontroller/default -n openshift-ingress-operator
```

### Prevention

- **Always include fallback groups:** When configuring custom groups (especially
  experimental ones like PQC groups), always include widely-supported groups in
  the list as fallbacks
- **Test in non-production first:** Apply custom group configurations to
  development/staging clusters before production
- **Use predefined profiles when possible:** Most users should use
  Old/Intermediate/Modern profiles, which are tested across all components
- **Monitor after changes:** Watch component logs for 15-30 minutes after
  applying group configuration changes
