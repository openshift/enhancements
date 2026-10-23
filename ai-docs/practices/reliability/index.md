# Reliability Practices

**Category**: Reliability
**Last Updated**: 2026-10-23

---

## Overview

OpenShift platform reliability is measured through SLIs (Service Level Indicators), evaluated via SLOs (Service Level Objectives), and communicated as SLAs. Component operators are expected to implement degraded-mode patterns and report health accurately.

## SLI/SLO Framework

| Concept | Definition | Example |
|---------|-----------|---------|
| **SLI** | Measurable signal of service health | API request latency p99 |
| **SLO** | Target threshold for an SLI | p99 latency < 1s for 99.9% of time |
| **SLA** | Contractual commitment based on SLOs | Platform availability > 99.95% |

## Degraded-Mode Patterns

Operators must handle partial failures gracefully:

1. **Set `Degraded=True`** with a clear reason when functionality is impaired but not lost
2. **Set `Available=False`** only when the component is completely non-functional
3. **Never block upgrades** unless there is a genuine data-loss or corruption risk
4. **Implement exponential backoff** for retries on transient failures
5. **Report `Progressing=True`** only during active state transitions, not during steady-state

## Availability During Upgrades

- Operators must remain `Available=True` during rolling upgrades
- Use `maxUnavailable` and `maxSurge` to control rollout pace
- Test N-1 → N version skew compatibility
- Report `Upgradeable=False` via ClusterOperator status when pre-conditions aren't met

## Disruption Budget

OpenShift tracks disruption budgets for API availability during upgrades. Components must not introduce regressions in:
- API availability (no new downtime windows)
- Console responsiveness
- Ingress connectivity
- DNS resolution

## Platform Documentation

For operator status condition conventions, see [platform/operator-patterns/status-conditions.md](../../platform/operator-patterns/status-conditions.md). For upgrade safety, see [platform/openshift-specifics/upgrade-strategies.md](../../platform/openshift-specifics/upgrade-strategies.md).
