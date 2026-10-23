# Reliability Practices

Reliability patterns and SLO frameworks for OpenShift components.

## Key Areas

### SLI/SLO/SLA Framework

| Term | Definition | Example |
|------|-----------|---------|
| **SLI** (Indicator) | Measurable metric of service quality | API request latency p99 |
| **SLO** (Objective) | Target value for an SLI | p99 latency < 200ms |
| **SLA** (Agreement) | Contractual commitment with consequences | 99.9% availability / month |

### Degraded-Mode Patterns
- Operators should continue functioning with reduced capability when dependencies fail
- Report `Degraded=True` condition with specific reason and message
- See [status-conditions.md](../../platform/operator-patterns/status-conditions.md) for reporting

### Availability Patterns

| Pattern | Use Case |
|---------|----------|
| **PodDisruptionBudget** | Maintain minimum replicas during disruption |
| **Health Probes** | Liveness, readiness, and startup probes |
| **Graceful Shutdown** | Handle SIGTERM with connection draining |
| **Circuit Breaker** | Prevent cascade failures from dependency outages |
| **Rate Limiting** | Protect APIs from excessive load |

### Error Budget
- Track error budget consumption
- When budget is exhausted, prioritize reliability over features
- Use Prometheus alerts on burn rate

## Monitoring

```promql
# Availability SLI
sum(rate(http_requests_total{code!~"5.."}[5m])) /
sum(rate(http_requests_total[5m]))

# Latency SLI
histogram_quantile(0.99, rate(http_request_duration_seconds_bucket[5m]))
```

## References

- **Pattern**: [status-conditions.md](../../platform/operator-patterns/status-conditions.md) - Health reporting
- **Pattern**: [upgrade-strategies.md](../../platform/openshift-specifics/upgrade-strategies.md) - PDB and rolling updates
- **Upstream**: [Kubernetes Probes](https://kubernetes.io/docs/tasks/configure-pod-container/configure-liveness-readiness-startup-probes/)
