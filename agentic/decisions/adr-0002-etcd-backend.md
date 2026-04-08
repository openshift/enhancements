---
title: Use etcd for Cluster State
status: Accepted
date: 2026-04-08
affected_components:
  - kube-apiserver
  - openshift-apiserver
  - All operators
---

# ADR 0002: Use etcd for Cluster State

## Status

**Accepted**

## Context

OpenShift needs persistent, consistent, distributed storage for cluster state (API objects).

## Decision

Use etcd as the backend for Kubernetes and OpenShift API servers.

## Rationale

- ✅ **Upstream alignment**: Kubernetes uses etcd
- ✅ **Strong consistency**: Raft consensus protocol
- ✅ **Performance**: Handles OpenShift scale (1000s of nodes, 100k+ objects)
- ✅ **Mature tooling**: Battle-tested in production (Google, Red Hat, etc.)
- ✅ **HA support**: Built-in quorum and replication

## Alternatives Considered

### PostgreSQL

- **Pro**: Relational database, SQL queries, familiar to many
- **Con**: Not K8s standard, would diverge from upstream
- **Con**: Watch implementation complex

### Consul

- **Pro**: Similar to etcd, supports K/V store
- **Con**: Less K8s tooling, not upstream choice
- **Con**: Different consistency model

### Custom Distributed DB

- **Pro**: Could optimize for OpenShift use cases
- **Con**: High development/maintenance cost
- **Con**: Unproven at scale

## Consequences

**Positive**:
- Standard K8s patterns work
- Upstream knowledge applies
- Existing tools (etcdctl, backup/restore) work
- Strong consistency guarantees

**Negative**:
- etcd quorum loss = cluster down (CAP theorem - chose CP)
- Must backup etcd for disaster recovery
- Upgrade procedures must handle etcd version compatibility
- Performance tuning required for large clusters

## Mitigation Strategies

### Quorum Loss

Run etcd with 3+ members (5 for production):

```yaml
# etcd topology
master-0: etcd member
master-1: etcd member
master-2: etcd member
# Can tolerate 1 failure
```

### Backups

Automated etcd backup via cluster-etcd-operator:

```yaml
apiVersion: operator.openshift.io/v1
kind: Etcd
spec:
  backup:
    schedule: "0 */6 * * *"  # Every 6 hours
    retentionPolicy:
      maxNumberOfBackups: 30
```

### Upgrades

Coordinated etcd version upgrades via CVO:

```
4.15: etcd 3.5.x
4.16: etcd 3.5.y (minor version bump)
# Never skip etcd minor versions
```

### Performance

- **Defragmentation**: Automatic compaction
- **Monitoring**: etcd_disk_wal_fsync_duration_seconds
- **Resource limits**: Dedicated disks for etcd

## Affected Components

| Component | Impact |
|-----------|--------|
| **kube-apiserver** | Stores K8s resources in etcd |
| **openshift-apiserver** | Stores OpenShift resources in etcd |
| **All operators** | Read state from API server (backed by etcd) |
| **cluster-etcd-operator** | Manages etcd lifecycle |

## Operational Requirements

### Backup Schedule

- **Frequency**: Every 6 hours
- **Retention**: 30 backups (7.5 days)
- **Location**: S3-compatible storage

### Monitoring

- **Latency**: etcd_disk_wal_fsync_duration_seconds <10ms
- **Leader changes**: <1 per day
- **DB size**: <8GB (compact if larger)

### Disaster Recovery

**RTO** (Recovery Time Objective): 1 hour  
**RPO** (Recovery Point Objective): 6 hours (backup frequency)

## References

- **Kubernetes etcd**: https://kubernetes.io/docs/tasks/administer-cluster/configure-upgrade-etcd/
- **cluster-etcd-operator**: https://github.com/openshift/cluster-etcd-operator
- **etcd docs**: https://etcd.io/docs/
