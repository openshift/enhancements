---
title: kms-migration-recovery
authors:
  - "@ardaguclu"
  - "@dgrisonnet"
  - "@flavianmissi"
reviewers:
  - "@ibihim"
  - "@sjenning"
  - "@tkashem"
  - "@derekwaynecarr"
approvers:
  - "@sjenning"
api-approvers:
  - "None"
creation-date: 2025-01-28
last-updated: 2025-01-28
tracking-link:
  - "https://issues.redhat.com/browse/OCPSTRAT-1638" # GA feature only
see-also:
  - "enhancements/kube-apiserver/kms-encryption-foundations.md"
  - "enhancements/kube-apiserver/kms-plugin-management.md"
  - "enhancements/kube-apiserver/encrypting-data-at-datastore-layer.md"
  - "enhancements/etcd/storage-migration-for-etcd-encryption.md"
replaces:
  - ""
superseded-by:
  - ""
---

# KMS Migration and Disaster Recovery

## Summary

**This enhancement is targeted for GA only and is not part of Tech Preview.**

Provide comprehensive migration and disaster recovery capabilities for KMS encryption in OpenShift. This includes migrating between different KMS providers, recovering from KMS key loss scenarios, handling temporary KMS outages, and providing operational guidance for complex migration scenarios.

## Motivation

While basic KMS encryption and key rotation are covered in Tech Preview (see [KMS Encryption Foundations](kms-encryption-foundations.md) and [KMS Plugin Management](kms-plugin-management.md)), production deployments require robust migration and recovery capabilities. Cluster administrators need to:

- Migrate encrypted data between different KMS providers (e.g., AWS KMS → Vault, Vault → Thales)
- Recover from partial KMS failures (temporary outages, key deletion, credential expiration)
- Transition from local encryption (aescbc/aesgcm) to KMS and vice versa
- Handle cross-region or cross-account KMS migrations
- Understand backup and restore implications with KMS encryption

These scenarios are complex and require production validation before being supported. Tech Preview will gather operational experience with basic KMS functionality, and GA will build upon that foundation to provide advanced migration and recovery features.

### User Stories

* As a cluster admin, I want to migrate from AWS KMS to HashiCorp Vault without cluster downtime, so that I can change KMS providers based on my organization's policies
* As a cluster admin, I want automated recovery from temporary KMS outages, so that my cluster remains available during transient network or KMS service issues
* As a cluster admin, I want clear documentation on recovering from KMS key loss, so that I understand the risks and available options before they occur
* As a cluster admin, I want to migrate my cluster's encryption from local aescbc to KMS, so that I can improve security without disrupting workloads
* As a cluster admin, I want to migrate KMS encryption across AWS regions, so that I can handle disaster recovery scenarios

### Goals

* Support seamless migration between KMS providers
* Provide disaster recovery procedures for KMS key loss
* Handle temporary KMS outages gracefully (caching, degraded mode)
* Document and test all supported migration paths
* Provide monitoring and alerting for migration health
* Create runbooks for common recovery scenarios
* Define SLOs for migration completion time

### Non-Goals

* Automatic recovery from permanent KMS key deletion (data loss is expected)
* Migration between incompatible KMS versions (e.g., KMS v1 → KMS v2)
* Cross-cluster migration (backup/restore is separate feature)
* Zero-downtime guarantees for all migration scenarios (some may require brief unavailability)

## Proposal

**This section will be completed during GA planning, building on Tech Preview operational experience.**

Areas to be addressed:

1. **Migration Framework**
   - Automated migration between KMS providers
   - Pre-flight validation (target KMS reachable, credentials valid)
   - Progress tracking and rollback capabilities
   - Two-plugin parallel operation during migration

2. **Disaster Recovery**
   - KMS key loss detection mechanisms
   - Partial recovery strategies (cached DEKs, grace periods)
   - Complete data loss scenarios (when recovery impossible)
   - Key escrow considerations (future enhancement)

3. **Operational Procedures**
   - Migration runbooks per scenario
   - Health checks and validation scripts
   - Monitoring and alerting recommendations
   - Performance optimization for large-scale migrations

4. **Testing Strategy**
   - Migration matrix (all provider combinations)
   - Failure injection testing
   - Scale testing (time to migrate N resources)
   - Disaster recovery drills

## Scope of Work (GA Only)

### Supported Migration Paths

The following migration paths will be documented and tested:

**Between KMS Providers:**
- AWS KMS → HashiCorp Vault
- HashiCorp Vault → AWS KMS
- AWS KMS → Thales HSM
- Thales HSM → HashiCorp Vault
- (All bidirectional combinations)

**Between Encryption Types:**
- aescbc → AWS KMS
- aescbc → HashiCorp Vault
- aescbc → Thales HSM
- AWS KMS → aescbc (downgrade scenario)
- HashiCorp Vault → identity (disable encryption)

**Cross-Region/Account (AWS Specific):**
- AWS KMS key in us-east-1 → us-west-2
- AWS KMS key in account A → account B

### Disaster Recovery Scenarios

**Temporary KMS Outages:**
- Network partition (cluster cannot reach KMS)
- KMS service degradation (slow responses, timeouts)
- Credential expiration (temporary auth failure)
- **Mitigation**: In-memory DEK caching, degraded mode operation, automatic retry

**Permanent Key Loss:**
- KMS key accidentally deleted
- KMS account/subscription terminated
- Encryption key permanently corrupted
- **Impact**: Data encrypted with lost key is unrecoverable
- **Mitigation**: Key deletion grace periods, backup strategies, monitoring

**Partial Failures:**
- Some resources encrypted with lost key, others still accessible
- Mixed-version API servers during upgrade
- Plugin crashes during migration
- **Recovery**: Identify affected resources, manual intervention, partial restoration

### Implementation Details

**To be designed during GA phase. Key considerations:**

- How to run two KMS plugins simultaneously during migration (different socket paths)
- How to track migration progress per KMS provider
- When to clean up old KMS plugin after migration completes
- How to handle rollback if migration fails mid-way
- Performance optimizations (parallel migration, batching)

### Risks and Mitigations

**To be assessed during GA planning based on Tech Preview learnings.**

## Alternatives (Not Implemented)

**Alternative: Include in Tech Preview**

We could attempt to support migration in Tech Preview.

**Why deferred to GA:**
- Migration complexity requires production validation first
- Need operational experience with single-provider deployments
- Edge cases and failure modes not yet fully understood
- Risk of committing to unsupportable migration paths
- GA allows iteration based on real-world Tech Preview feedback

## Open Questions

1. Should we support "live" migration (both KMS active) or "offline" migration (brief unavailability)?
2. How do we handle very large clusters (millions of secrets)? Multi-day migrations acceptable?
3. Key escrow (storing encrypted DEKs for recovery) - security vs recoverability tradeoff?
4. Should we provide automated migration tooling or just documented procedures?
5. What SLOs should we commit to for migration completion time?

## Test Plan

**To be defined during GA planning.**

Areas requiring test coverage:
- All supported migration paths (automated testing)
- Failure injection at each migration phase
- Scale testing with realistic data volumes
- Disaster recovery drill procedures
- Performance benchmarking

## Graduation Criteria

This enhancement is **GA-only** and does not have a Tech Preview phase.

**Prerequisites for GA:**
- [KMS Encryption Foundations](kms-encryption-foundations.md) is GA
- [KMS Plugin Management](kms-plugin-management.md) is GA
- At least 2 KMS providers fully supported and production-validated
- Operational experience gathered from Tech Preview deployments

**GA Acceptance Criteria:**
- All documented migration paths tested and validated
- Disaster recovery runbooks created and tested
- Monitoring and alerting defined for migration health
- SLOs defined for migration completion time
- User documentation in openshift-docs
- Support team trained on recovery procedures

## Upgrade / Downgrade Strategy

This enhancement provides the upgrade/downgrade strategy for KMS encryption itself, so this section will be critical in the full proposal.

## Version Skew Strategy

To be defined during GA planning.

## Operational Aspects of API Extensions

No new API extensions - uses existing APIServer config from [KMS Plugin Management](kms-plugin-management.md).

## Support Procedures

This enhancement IS the support procedures for KMS migration and recovery. This section will be extensive in the full proposal.

## Infrastructure Needed

For testing migration and disaster recovery scenarios:
- Multiple KMS provider instances (AWS KMS, Vault, Thales HSM)
- Large-scale test clusters (simulate production data volumes)
- Chaos engineering infrastructure (failure injection)
- Automated testing framework for migration paths

---

## Note to Reviewers

This is a placeholder enhancement to document the scope of GA work. The full proposal will be developed after Tech Preview has been released and operational experience has been gathered.

**Do not block Tech Preview on this enhancement.** It exists to:
1. Clearly scope what is NOT in Tech Preview
2. Set expectations for GA requirements
3. Provide a tracking document for future work
