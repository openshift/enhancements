# Observability

**Category**: Engineering Practice  
**Applies To**: All OpenShift components  
**Last Updated**: 2026-04-08  

## Overview

Observability through metrics, logging, and tracing enables debugging, monitoring, and understanding system behavior.

## Three Pillars

| Pillar | Purpose | OpenShift Tool |
|--------|---------|----------------|
| **Metrics** | Quantitative measurements over time | Prometheus |
| **Logging** | Event records | OpenShift Logging (Loki/EFK) |
| **Tracing** | Request flow across services | Jaeger (optional) |

## Metrics

### Instrumentation

```go
import (
    "github.com/prometheus/client_golang/prometheus"
    "sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
    // Counter: monotonically increasing value
    reconcileTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "machineconfig_reconcile_total",
            Help: "Total reconciliations",
        },
        []string{"result"}, // Labels
    )
    
    // Gauge: value that can go up or down
    nodeCount = prometheus.NewGauge(
        prometheus.GaugeOpts{
            Name: "machineconfig_nodes_total",
            Help: "Total managed nodes",
        },
    )
    
    // Histogram: distribution of values
    reconcileDuration = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "machineconfig_reconcile_duration_seconds",
            Help:    "Reconciliation duration",
            Buckets: prometheus.DefBuckets,
        },
        []string{},
    )
)

func init() {
    metrics.Registry.MustRegister(
        reconcileTotal,
        nodeCount,
        reconcileDuration,
    )
}

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    start := time.Now()
    defer func() {
        duration := time.Since(start).Seconds()
        reconcileDuration.WithLabelValues().Observe(duration)
    }()
    
    err := r.reconcile(ctx, req)
    if err != nil {
        reconcileTotal.WithLabelValues("error").Inc()
        return ctrl.Result{}, err
    }
    
    reconcileTotal.WithLabelValues("success").Inc()
    return ctrl.Result{}, nil
}
```

### Metric Naming

Follow Prometheus conventions:

```
<namespace>_<subsystem>_<name>_<unit>

Examples:
- machineconfig_reconcile_total
- machineconfig_node_update_duration_seconds
- cluster_version_available_updates
```

### Metric Types

**Counter** - Cumulative, only increases:
```go
reconcile_total
reconcile_errors_total
```

**Gauge** - Point-in-time value:
```go
nodes_total
available_replicas
memory_bytes
```

**Histogram** - Distribution:
```go
reconcile_duration_seconds
request_size_bytes
```

### ServiceMonitor

```yaml
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: my-operator
  namespace: openshift-my-operator
spec:
  selector:
    matchLabels:
      app: my-operator
  endpoints:
  - port: metrics
    interval: 30s
    path: /metrics
```

## Logging

### Structured Logging

```go
import (
    "github.com/go-logr/logr"
    ctrl "sigs.k8s.io/controller-runtime"
)

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    log := ctrl.LoggerFrom(ctx)
    
    // Structured logging with key-value pairs
    log.Info("Reconciling resource",
        "namespace", req.Namespace,
        "name", req.Name)
    
    mc := &MachineConfig{}
    if err := r.Get(ctx, req.NamespacedName, mc); err != nil {
        log.Error(err, "Failed to get MachineConfig",
            "namespace", req.Namespace,
            "name", req.Name)
        return ctrl.Result{}, err
    }
    
    log.V(1).Info("Processing MachineConfig",
        "generation", mc.Generation,
        "resourceVersion", mc.ResourceVersion)
    
    return ctrl.Result{}, nil
}
```

### Log Levels

| Level | Purpose | Example |
|-------|---------|---------|
| Error | Errors requiring attention | `log.Error(err, "Failed to update")` |
| Info (0) | Important events | `log.Info("Reconciliation complete")` |
| Debug (1) | Detailed flow | `log.V(1).Info("Processing item")` |
| Trace (2+) | Very detailed | `log.V(2).Info("Cache hit", "key", key)` |

### Best Practices

**DO**:
```go
// Include context
log.Info("Node updated", "node", node.Name, "version", version)

// Use Error for failures
log.Error(err, "Failed to create deployment", "name", name)

// Use V() for debug
log.V(1).Info("Cache size", "size", len(cache))
```

**DON'T**:
```go
// Don't log secrets
log.Info("Config", "apiKey", config.APIKey)  // NEVER!

// Don't use fmt.Printf
fmt.Printf("Node: %s\n", node.Name)  // Use log.Info instead

// Don't log in tight loops
for _, item := range items {
    log.Info("Processing", "item", item)  // Too noisy
}
```

## Tracing

### OpenTelemetry

```go
import (
    "go.opentelemetry.io/otel"
    "go.opentelemetry.io/otel/trace"
)

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    tracer := otel.Tracer("my-operator")
    ctx, span := tracer.Start(ctx, "Reconcile")
    defer span.End()
    
    span.SetAttributes(
        attribute.String("namespace", req.Namespace),
        attribute.String("name", req.Name),
    )
    
    // Nested span
    ctx, childSpan := tracer.Start(ctx, "FetchResource")
    mc := &MachineConfig{}
    err := r.Get(ctx, req.NamespacedName, mc)
    childSpan.End()
    
    if err != nil {
        span.RecordError(err)
        return ctrl.Result{}, err
    }
    
    return ctrl.Result{}, nil
}
```

## Debugging

### Viewing Metrics

```bash
# Port-forward to metrics endpoint
oc port-forward -n openshift-my-operator pod/my-operator-xxx 8080:8080

# Query metrics
curl http://localhost:8080/metrics

# Query Prometheus
oc port-forward -n openshift-monitoring prometheus-k8s-0 9090:9090
# Open http://localhost:9090
```

### Viewing Logs

```bash
# View operator logs
oc logs -n openshift-my-operator deployment/my-operator

# Follow logs
oc logs -n openshift-my-operator deployment/my-operator -f

# Previous container
oc logs -n openshift-my-operator pod/my-operator-xxx -p

# All pods
oc logs -n openshift-my-operator -l app=my-operator --tail=100
```

### Log Aggregation

```bash
# Query logs in OpenShift Console
# Observe → Logs → Filter by namespace/pod

# CLI with oc-observability plugin
oc observability logs -n openshift-my-operator
```

## Alerts

```yaml
apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: my-operator-alerts
  namespace: openshift-my-operator
spec:
  groups:
  - name: my-operator
    interval: 30s
    rules:
    - alert: MyOperatorDown
      expr: up{job="my-operator"} == 0
      for: 5m
      labels:
        severity: critical
      annotations:
        summary: "My Operator is down"
        description: "My Operator has been down for 5 minutes"
        
    - alert: MyOperatorHighErrorRate
      expr: |
        rate(machineconfig_reconcile_errors_total[5m]) 
        / 
        rate(machineconfig_reconcile_total[5m]) > 0.1
      for: 10m
      labels:
        severity: warning
      annotations:
        summary: "High error rate in My Operator"
        description: "Error rate: {{ $value | humanizePercentage }}"
```

## Dashboards

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: my-operator-dashboard
  namespace: openshift-my-operator
  labels:
    grafana_dashboard: "1"
data:
  my-operator.json: |
    {
      "dashboard": {
        "title": "My Operator",
        "panels": [
          {
            "title": "Reconcile Rate",
            "targets": [{
              "expr": "rate(machineconfig_reconcile_total[5m])"
            }]
          },
          {
            "title": "Error Rate",
            "targets": [{
              "expr": "rate(machineconfig_reconcile_errors_total[5m])"
            }]
          },
          {
            "title": "P99 Latency",
            "targets": [{
              "expr": "histogram_quantile(0.99, rate(machineconfig_reconcile_duration_seconds_bucket[5m]))"
            }]
          }
        ]
      }
    }
```

## Health Endpoints

```go
import (
    "net/http"
    "sigs.k8s.io/controller-runtime/pkg/healthz"
)

func main() {
    mgr, err := ctrl.NewManager(cfg, ctrl.Options{
        HealthProbeBindAddress: ":8081",
    })
    
    // Liveness probe
    mgr.AddHealthzCheck("healthz", healthz.Ping)
    
    // Readiness probe
    mgr.AddReadyzCheck("readyz", func(req *http.Request) error {
        // Check if operator is ready to serve
        if !operatorReady {
            return fmt.Errorf("operator not ready")
        }
        return nil
    })
}
```

```yaml
# Pod with health checks
spec:
  containers:
  - name: operator
    livenessProbe:
      httpGet:
        path: /healthz
        port: 8081
      initialDelaySeconds: 15
      periodSeconds: 20
    readinessProbe:
      httpGet:
        path: /readyz
        port: 8081
      initialDelaySeconds: 5
      periodSeconds: 10
```

## Examples

| Component | Metrics | Alerts | Dashboards |
|-----------|---------|--------|------------|
| machine-config-operator | reconcile rate, node count | HighErrorRate, NodeUpdateStuck | MachineConfig Overview |
| cluster-version-operator | upgrade progress, operator status | UpgradeFailed | Cluster Version |
| cluster-network-operator | network config rate | NetworkDegraded | Network Overview |

## References

- **Prometheus**: https://prometheus.io/docs/
- **Controller-runtime Metrics**: https://book.kubebuilder.io/reference/metrics.html
- **SLO Framework**: [slo-framework.md](./slo-framework.md)
- **Degraded Mode**: [degraded-mode.md](./degraded-mode.md)
