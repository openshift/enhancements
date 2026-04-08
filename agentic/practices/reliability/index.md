# Reliability Practices Index

**Last Updated**: 2026-04-08  

## Overview

Reliability practices for building resilient OpenShift components.

## Core Practices

| Practice | Purpose | File |
|----------|---------|------|
| SLO Framework | Define reliability targets | [slo-framework.md](./slo-framework.md) |
| Observability | Metrics, logging, tracing | [observability.md](./observability.md) |
| Degraded Mode | Graceful failure handling | [degraded-mode.md](./degraded-mode.md) |

## Quick Reference

### SLO Targets

- **Critical services**: 99.9% (43 min/month downtime)
- **Standard services**: 99% (7.2 hours/month)
- **Best-effort**: 95% (36 hours/month)

See [slo-framework.md](./slo-framework.md)

### Observability Pillars

- **Metrics**: Prometheus counters, gauges, histograms
- **Logging**: Structured logs with context
- **Tracing**: OpenTelemetry spans (optional)

See [observability.md](./observability.md)

### Degraded vs Failed

- **Degraded**: Partial functionality (Available=True, Degraded=True)
- **Failed**: No functionality (Available=False)

See [degraded-mode.md](./degraded-mode.md)

## Common Patterns

### Metric Instrumentation

```go
reconcileTotal.WithLabelValues("success").Inc()
reconcileDuration.WithLabelValues().Observe(time.Since(start).Seconds())
```

### Structured Logging

```go
log.Info("Reconciling", "namespace", req.Namespace, "name", req.Name)
```

### Degraded Handling

```go
if availableReplicas < desiredReplicas {
    setCondition(Degraded, True, "ReducedCapacity", "...")
    setCondition(Available, True, "AsExpected", "")  // Still available
}
```

## See Also

- [Testing Practices](../testing/) - Testing reliability
- [Security Practices](../security/) - Security monitoring
- [Operator Patterns](../../platform/operator-patterns/) - Status conditions
