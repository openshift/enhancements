# VEP-1827: Metrics Collection Sidecars for KubeVirt Components

## Summary

This VEP proposes implementing dedicated metrics collection sidecars for
KubeVirt components to improve observability, reduce resource overhead on
core components, and enable more flexible metrics collection and export
strategies.

## Motivation

### Current State

KubeVirt components currently expose Prometheus metrics directly through
their HTTP endpoints:
- `virt-handler`: Exposes 60+ VMI runtime metrics via local libvirt domain
  statistics
- `virt-controller`: Exposes VM lifecycle and cluster-wide metrics
- `virt-api`: Exposes API request and connection metrics
- `virt-operator`: Exposes operator health and configuration metrics

### Problems with Current Approach

1. **Resource Overhead**: Metrics collection adds CPU/memory overhead to
   critical components
2. **Coupling**: Metrics collection logic is tightly coupled with core
   component functionality
3. **Limited Flexibility**: Difficult to customize metrics export formats,
   destinations, or processing
4. **Scalability**: High-frequency metrics collection can impact component
   performance
5. **Maintenance**: Metrics changes require core component updates and releases
6. **Limited Backports**: New metrics, recording rules and Alerts can't be
   backported to older versions, based on KubeVirt policy.

### Goals

- **Decouple** metrics collection from core KubeVirt component logic
- **Improve Performance** by offloading metrics collection overhead
- **Enable Flexibility** in metrics export formats (Prometheus,
  OpenTelemetry, custom)
- **Support External Management** through separate repository and release
  cycle
- **Maintain Compatibility** with existing Prometheus scraping
  infrastructure
- **Backports Flexibility** in cases when specific metrics, recording rules
  and Alerts are needed in older versions.

### Non-Goals

- Changing existing metrics schemas or breaking Prometheus compatibility
- Implementing new metrics not currently exposed by KubeVirt
- Replacing KubeVirt's internal monitoring for health checks

## Proposal

### Architecture Overview

```yaml
# Metrics Sidecar Architecture
virt-handler-pod:
  containers:
  - name: virt-handler          # Core functionality
  - name: metrics-collector     # Sidecar for metrics
    ports:
    - containerPort: 9090       # Prometheus endpoint
    volumeMounts:
    - name: virt-share-dir      # Access to VMI sockets
      mountPath: /var/run/kubevirt-private
```

### Component-Specific Implementation

#### 1. virt-handler Metrics Sidecar (Priority 1)

**Responsibilities:**
- Collect VMI runtime metrics via Unix domain sockets
- Export 60+ `kubevirt_vmi_*` metrics (CPU, memory, network, storage)
- Handle guest agent information collection
- Process migration statistics

**Technical Requirements:**
- Access to `virtShareDir` for VMI socket communication
- Node-specific deployment (DaemonSet integration)
- High-frequency collection capability

#### 2. virt-controller Metrics Sidecar (Priority 2)

**Responsibilities:**
- Collect VM lifecycle metrics from Kubernetes API
- Export cluster-wide statistics and migration tracking
- Monitor controller health and leadership status

**Technical Requirements:**
- Kubernetes API access via service account
- Cluster-wide deployment (Deployment integration)

#### 3. virt-api Metrics Sidecar (Priority 3)

**Responsibilities:**
- Collect API request metrics and connection statistics
- Monitor request patterns and performance

#### 4. virt-operator Metrics Sidecar (Priority 4)

**Responsibilities:**
- Collect operator health and configuration metrics
- Monitor deployment status and lifecycle events

### Integration Strategies

#### Option A: KubeVirt CR Customization (Recommended)

```yaml
apiVersion: kubevirt.io/v1
kind: KubeVirt
metadata:
  name: kubevirt
spec:
  customizeComponents:
    patches:
    - resourceType: DaemonSet
      resourceName: virt-handler
      patch: |
        spec:
          template:
            spec:
              containers:
              - name: metrics-collector
                image: quay.io/kubevirt/virt-handler-metrics:v1.0.0
                ports:
                - containerPort: 9090
                  name: metrics
                  protocol: TCP
                volumeMounts:
                - name: virt-share-dir
                  mountPath: /var/run/kubevirt-private
                env:
                - name: METRICS_PORT
                  value: "9090"
                - name: NODE_NAME
                  valueFrom:
                    fieldRef:
                      fieldPath: spec.nodeName
```

#### Option B: Environment Variable Configuration

```yaml
# virt-operator deployment
env:
- name: VIRT_HANDLER_METRICS_SIDECAR_IMAGE
  value: "quay.io/kubevirt/virt-handler-metrics:v1.0.0"
- name: VIRT_CONTROLLER_METRICS_SIDECAR_IMAGE
  value: "quay.io/kubevirt/virt-controller-metrics:v1.0.0"
```

#### Option C: External Operator (Advanced)

Develop a separate operator that watches KubeVirt deployments and
automatically injects sidecars.

### External Repository Structure

```
kubevirt-metrics-sidecars/
├── cmd/
│   ├── virt-handler-metrics/     # Node-level metrics collector
│   ├── virt-controller-metrics/  # Cluster-level metrics
│   ├── virt-api-metrics/         # API metrics collector
│   └── virt-operator-metrics/    # Operator metrics collector
├── pkg/
│   ├── collectors/               # Shared collection logic
│   ├── exporters/                # Prometheus/OTEL exporters
│   ├── config/                   # Configuration management
│   └── client/                   # KubeVirt API clients
├── build/
│   ├── Dockerfile.virt-handler
│   ├── Dockerfile.virt-controller
│   ├── Dockerfile.virt-api
│   └── Dockerfile.virt-operator
├── deploy/
│   ├── patches/                  # KubeVirt CR patches
│   ├── manifests/               # Kubernetes manifests
│   └── helm/                    # Helm charts
├── examples/
│   └── kubevirt-with-metrics-sidecars.yaml
└── docs/
    ├── design.md
    ├── deployment.md
    └── troubleshooting.md
```

## Implementation Plan

### Phase 1: virt-handler Sidecar (MVP)
- [ ] Implement virt-handler metrics collector
- [ ] Create container image and build pipeline
- [ ] Develop KubeVirt CR patch configuration
- [ ] Write documentation and examples
- [ ] Test with existing Prometheus setups

### Phase 2: Additional Components
- [ ] Implement virt-controller metrics collector
- [ ] Add virt-api metrics collector
- [ ] Implement virt-operator metrics collector
- [ ] Create unified deployment automation

### Phase 3: Advanced Features
- [ ] OpenTelemetry export support
- [ ] Custom metrics aggregation
- [ ] Multi-format export (JSON, InfluxDB, etc.)
- [ ] Performance optimization and caching

### Phase 4: Production Hardening
- [ ] Security review and hardening
- [ ] Resource optimization
- [ ] Monitoring and alerting for sidecars
- [ ] Documentation and best practices

## Risks and Mitigations

### Risk: Increased Resource Usage
**Mitigation**:
- Implement resource limits and requests
- Use efficient collection algorithms
- Provide configuration for collection intervals

### Risk: Metrics Compatibility
**Mitigation**:
- Maintain exact metric name and label compatibility
- Implement comprehensive testing against existing dashboards
- Provide migration guides

### Risk: Deployment Complexity
**Mitigation**:
- Provide simple KubeVirt CR patches
- Create automation tools and documentation
- Support gradual rollout

### Risk: Socket Access Security
**Mitigation**:
- Use minimal required permissions
- Implement proper volume mounting with security contexts
- Regular security audits

## Testing Strategy

### Unit Tests
- Individual collector logic
- Metric export functionality
- Configuration handling

### Integration Tests
- End-to-end metric collection
- Compatibility with existing Prometheus setups
- Performance impact measurement

### E2E Tests
- Full KubeVirt deployment with sidecars
- Metrics accuracy validation
- Upgrade/downgrade scenarios

## Graduation Criteria

### Alpha
- [ ] Working virt-handler sidecar implementation
- [ ] Basic KubeVirt CR integration
- [ ] Documentation and examples

### Beta
- [ ] All component sidecars implemented
- [ ] Performance testing completed
- [ ] Production deployment validation

### Stable
- [ ] Proven in production environments
- [ ] Complete test coverage
- [ ] Security review completed
- [ ] Community adoption

## Implementation History

- 2025-01-XX: VEP proposal created
- 2025-XX-XX: Alpha implementation started
- 2025-XX-XX: Beta release
- 2025-XX-XX: Stable release

## References

- [KubeVirt Metrics Documentation](
  https://github.com/kubevirt/kubevirt/blob/main/docs/observability/metrics.md)
- [Existing Sidecar Implementation](
  https://github.com/kubevirt/kubevirt/tree/main/cmd/sidecars)
- [KubeVirt Customization Components](
  https://kubevirt.io/user-guide/operations/customize_components/)
- [Prometheus Operator Integration](
  https://github.com/prometheus-operator/prometheus-operator)
- [KubeVirt Enhancement Process](
  https://github.com/kubevirt/enhancements#process)