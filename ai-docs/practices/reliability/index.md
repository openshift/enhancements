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
2. **Set `Available=False`** when pods aren't ready, the API is not serving, or the component cannot fulfill its function — not only for complete failure
3. **Block upgrades when preconditions are unmet** — set `Upgradeable=False` when any documented upgrade precondition is not satisfied, not only for data-loss risk
4. **Implement exponential backoff** for retries on transient failures
5. **Report `Progressing=True`** only during active state transitions, not during steady-state

## Availability During Upgrades

- Operators must remain `Available=True` during rolling upgrades
- Use `maxUnavailable` and `maxSurge` to control rollout pace
- Test N-1 → N version skew compatibility
- Report `Upgradeable=False` via ClusterOperator status when pre-conditions aren't met

## Disruption Budget

OpenShift uses PodDisruptionBudgets (PDBs) to limit voluntary disruption of selected pods during upgrades and maintenance. PDBs do not guarantee API, console, ingress, or DNS availability — they only constrain voluntary eviction of pods matching the PDB's selector. PDBs also do not cover involuntary failures (node crashes, OOM kills). Components should not introduce regressions in disruption testing.

## Platform Documentation

For operator status condition conventions, see [platform/operator-patterns/status-conditions.md](../../platform/operator-patterns/status-conditions.md). For upgrade safety, see [platform/openshift-specifics/upgrade-strategies.md](../../platform/openshift-specifics/upgrade-strategies.md).
