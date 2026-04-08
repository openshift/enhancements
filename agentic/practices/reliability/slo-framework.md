# SLO Framework

**Category**: Engineering Practice  
**Applies To**: All OpenShift ClusterOperators  
**Last Updated**: 2026-04-08  

## Overview

Service Level Objectives (SLOs) define reliability targets for OpenShift components.

## Definitions

| Term | Definition | Example |
|------|-----------|---------|
| **SLI** | Service Level Indicator (metric) | API request success rate |
| **SLO** | Service Level Objective (target) | 99.9% of API requests succeed |
| **SLA** | Service Level Agreement (contract) | 99.5% uptime or refund |
| **Error Budget** | Allowed failure rate | 0.1% = ~43 minutes/month downtime |

## OpenShift SLO Structure

### Availability SLO

```yaml
apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: my-operator-slos
  namespace: openshift-my-operator
spec:
  groups:
  - name: my-operator-availability
    interval: 30s
    rules:
    - record: my_operator:availability:ratio_rate5m
      expr: |
        sum(rate(my_operator_reconcile_success_total[5m]))
        /
        sum(rate(my_operator_reconcile_total[5m]))
    
    - alert: MyOperatorSLOAvailabilityBudgetBurn
      expr: my_operator:availability:ratio_rate5m < 0.999
      for: 5m
      labels:
        severity: warning
      annotations:
        summary: "My Operator availability below SLO"
        description: "Current: {{ $value | humanizePercentage }}, Target: 99.9%"
```

### Latency SLO

```yaml
- record: my_operator:latency:p99_rate5m
  expr: |
    histogram_quantile(0.99, 
      rate(my_operator_reconcile_duration_seconds_bucket[5m])
    )

- alert: MyOperatorSLOLatencyBudgetBurn
  expr: my_operator:latency:p99_rate5m > 5
  for: 5m
  labels:
    severity: warning
  annotations:
    summary: "My Operator P99 latency above SLO"
    description: "Current: {{ $value }}s, Target: <5s"
```

## Calculating Error Budget

**Formula**: `Error Budget = (1 - SLO) × Time Window`

**Example** (99.9% monthly SLO):
```
Error Budget = (1 - 0.999) × 30 days × 24 hours × 60 minutes
             = 0.001 × 43,200 minutes
             = 43.2 minutes per month
```

## Error Budget Policy

### Budget Remaining > 50%

- ✅ Ship new features
- ✅ Perform experiments
- ✅ Deploy during business hours

### Budget Remaining 10-50%

- ⚠️ Slow down feature velocity
- ⚠️ Focus on reliability fixes
- ⚠️ Require deployment approval

### Budget Exhausted (<10%)

- ❌ Feature freeze
- ❌ Focus ONLY on reliability
- ❌ Root cause analysis required

## Implementation

### 1. Define SLIs

```go
// Instrument code with metrics
var (
    reconcileTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "my_operator_reconcile_total",
            Help: "Total reconciliations",
        },
        []string{"result"}, // "success" or "error"
    )
    
    reconcileDuration = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "my_operator_reconcile_duration_seconds",
            Help:    "Reconciliation duration",
            Buckets: []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
        },
        []string{},
    )
)

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    start := time.Now()
    defer func() {
        reconcileDuration.WithLabelValues().Observe(time.Since(start).Seconds())
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

### 2. Define SLOs

```markdown
## My Operator SLOs

| SLO | Target | Measurement Window |
|-----|--------|-------------------|
| Availability | 99.9% | 30 days |
| P99 Latency | <5s | 5 minutes |
| Error Rate | <0.1% | 1 hour |
```

### 3. Monitor Error Budget

```promql
# Error budget consumption rate (30-day window)
(1 - my_operator:availability:ratio_rate30d) / (1 - 0.999)

# >1.0 means burning budget faster than sustainable
# 1.0 means exactly on target
# <1.0 means budget accumulating
```

### 4. Alert on Budget Burn

```yaml
- alert: MyOperatorErrorBudgetBurnRateFast
  expr: |
    (1 - my_operator:availability:ratio_rate1h) / (1 - 0.999) > 14.4
  for: 5m
  labels:
    severity: critical
  annotations:
    summary: "Error budget burning at 14.4x normal rate"
    description: "At this rate, monthly budget exhausted in 2 days"

- alert: MyOperatorErrorBudgetBurnRateSlow
  expr: |
    (1 - my_operator:availability:ratio_rate6h) / (1 - 0.999) > 6
  for: 30m
  labels:
    severity: warning
  annotations:
    summary: "Error budget burning at 6x normal rate"
    description: "At this rate, monthly budget exhausted in 5 days"
```

## SLO Types

### Availability SLO

**Measures**: Percentage of successful operations

```promql
sum(rate(operation_success_total[30d]))
/
sum(rate(operation_total[30d]))
```

**Example targets**:
- Critical services: 99.9% (43 min/month downtime)
- Standard services: 99% (7.2 hours/month downtime)
- Best-effort: 95% (36 hours/month downtime)

### Latency SLO

**Measures**: Response time percentiles

```promql
histogram_quantile(0.99, 
  rate(operation_duration_seconds_bucket[5m])
)
```

**Example targets**:
- Interactive: P99 <1s
- Batch: P99 <10s
- Background: P99 <60s

### Throughput SLO

**Measures**: Operations per second

```promql
sum(rate(operation_total[5m]))
```

**Example targets**:
- Handle 1000 req/s
- Process 100 reconciles/s

## Multi-Window Multi-Burn-Rate Alerts

Fast burn detection with different time windows:

```yaml
# Fast burn (2% budget in 1 hour)
- alert: ErrorBudgetBurnFast
  expr: |
    (1 - availability:ratio_rate1h) > (14.4 * (1 - 0.999))
    and
    (1 - availability:ratio_rate5m) > (14.4 * (1 - 0.999))
  for: 2m
  labels:
    severity: critical

# Medium burn (5% budget in 6 hours)
- alert: ErrorBudgetBurnMedium
  expr: |
    (1 - availability:ratio_rate6h) > (6 * (1 - 0.999))
    and
    (1 - availability:ratio_rate30m) > (6 * (1 - 0.999))
  for: 15m
  labels:
    severity: warning

# Slow burn (10% budget in 3 days)
- alert: ErrorBudgetBurnSlow
  expr: |
    (1 - availability:ratio_rate3d) > (1 * (1 - 0.999))
    and
    (1 - availability:ratio_rate6h) > (1 * (1 - 0.999))
  for: 1h
  labels:
    severity: info
```

## Reporting

### Dashboard

```json
{
  "panels": [
    {
      "title": "Availability SLO",
      "targets": [
        {
          "expr": "my_operator:availability:ratio_rate30d * 100"
        }
      ],
      "thresholds": [
        {
          "value": 99.9,
          "color": "green"
        },
        {
          "value": 99.5,
          "color": "yellow"
        },
        {
          "value": 99,
          "color": "red"
        }
      ]
    },
    {
      "title": "Error Budget Remaining",
      "targets": [
        {
          "expr": "(0.001 - (1 - my_operator:availability:ratio_rate30d)) / 0.001 * 100"
        }
      ]
    }
  ]
}
```

### Monthly Report

```markdown
# My Operator SLO Report - January 2024

## Availability SLO (99.9% target)

- **Actual**: 99.95%
- **Error Budget**: 43.2 minutes
- **Consumed**: 21.6 minutes (50%)
- **Status**: ✅ Within budget

## Incidents

| Date | Duration | Impact | Cause |
|------|----------|--------|-------|
| Jan 15 | 10 min | 100% outage | Deployment rollout issue |
| Jan 22 | 5 min | 50% degraded | Node failure |

## Action Items

- [ ] Improve rollout strategy (prevent total outage)
- [ ] Add node failure resilience
```

## Examples

| Component | SLO | Error Budget Policy |
|-----------|-----|---------------------|
| kube-apiserver | 99.9% availability | Feature freeze if budget exhausted |
| machine-config-operator | 99% node configuration success | Require approval for risky changes |
| cluster-network-operator | P95 < 10s network configuration | Alert if P95 > 10s for >5min |

## Best Practices

### 1. Start Simple

Begin with availability SLO only. Add latency/throughput later.

### 2. User-Centric SLIs

Measure what users care about, not internal metrics.

**Bad**: `operator_pod_restarts_total`  
**Good**: `machineconfig_node_update_success_rate`

### 3. Achievable Targets

Set realistic targets based on current performance.

**Bad**: 99.99% (53 seconds/year) for non-critical service  
**Good**: 99% (7 hours/month) for best-effort service

### 4. Iterate

Review and adjust SLOs quarterly based on:
- Actual performance
- User feedback
- Business needs

## References

- **SRE Book**: https://sre.google/sre-book/service-level-objectives/
- **Error Budgets**: https://sre.google/workbook/error-budget-policy/
- **Observability**: [observability.md](./observability.md)
- **Degraded Mode**: [degraded-mode.md](./degraded-mode.md)
