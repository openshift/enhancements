# [5.1] Fast Out-of-Band Inspection

## Test Plan Identifier
`OCPSTRAT-3566-test-plan`

---

## 1. Introduction

### Purpose
This test plan validates the implementation of fast out-of-band inspection for bare metal hosts in the Metal Platform. The feature introduces a new inspection mode in openshift-5.1 that uses Redfish API calls instead of booting Ironic Python Agent (ramdisk) on the target machine, providing a faster and less invasive method of hardware inventory collection.

### Scope

**In Scope:**
- Fast inspection via Redfish API for BareMetalHost resources
- Hardware inventory collection without booting any payload on the target machine
- Performance validation (completion within 1 minute)
- Inventory field parity with agent-based inspection (where Redfish data is available)
- Re-inspection annotation behavior with fast inspection configuration
- Interoperability with existing BareMetalHost workflows
- Use as an alternative to agent-based inspection
- Integration with auto-discovery workflows

**Out of Scope:**
- Non-Redfish implementation methods
- OEM-specific Redfish fields or vendor-proprietary extensions
- Complete feature parity with agent-based inspection
- Modifications to the IPI installation flow
- Fields requiring vendor-specific OEM resources

---

## 2. Background / References

- **OCPSTRAT Issue:** OCPSTRAT-3566 - Fast out-of-band inspection in Metal platform
- **Component:** openshift/baremetal-operator (BareMetalHost API)
- **Related Documentation:** OpenShift Bare Metal IPI Documentation
- **Upstream Projects:** Metal3 Baremetal Operator

**Context:**
The Metal Platform currently supports hardware inspection by booting Ironic Python Agent (IPA) on target machines. This approach is comprehensive but time-consuming and requires booting a payload onto the machine. Fast inspection addresses use cases where hardware verification is needed before booting anything (e.g., auto-discovery, enrollment verification) by using out-of-band Redfish API calls.

---

## 3. Test Items

- **BareMetalHost API** - New inspection mode configuration
- **Baremetal Operator** - Fast inspection controller logic
- **Redfish Client** - Hardware inventory collection via Redfish
- **Hardware Data Schema** - Inventory field mapping and population
- **Re-inspection Annotation** - Configuration persistence

---

## 4. Features to Be Tested

### Functional Requirements (from Acceptance Criteria)
1. **FR-01:** Fast inspection completes within 1 minute with nominal BMC response times (< 0.1 sec/response)
2. **FR-02:** Fast inspection does not boot any payload on the target machine
3. **FR-03:** Inventory fields available from agent-based inspection are also available from fast inspection when Redfish exposes them in a vendor-neutral way
4. **FR-04:** Re-inspection annotation respects the inspection mode configuration (fast vs agent)

### User Goals
5. **UG-01:** User can request that a new BareMetalHost uses fast inspection instead of agent-based inspection
6. **UG-02:** User can verify machine hardware before booting any payload (auto-discovery use case)
7. **UG-03:** User can make provisioning decisions based on fast inspection results

### Inventory Coverage
8. **INV-01:** CPU information (architecture, model, count, clock speed)
9. **INV-02:** Memory capacity
10. **INV-03:** Network interfaces (MAC addresses, names, speed)
11. **INV-04:** Storage devices (model, size, vendor, serial number, type)
12. **INV-05:** System vendor information (manufacturer, product name, serial number)
13. **INV-06:** Firmware information

---

## 5. Features Not to Be Tested

The following are explicitly excluded from the scope of this test plan:

1. **Non-Redfish Implementations:** IPMI, custom vendor protocols, or other out-of-band management protocols
2. **OEM-Specific Fields:** Vendor-proprietary Redfish extensions or OEM resource fields
3. **Complete Feature Parity:** Fields that are only available through in-band inspection (e.g., kernel-level hardware detection)
4. **IPI Flow Modifications:** Changes to the installer-provisioned infrastructure installation process
5. **Agent-Based Inspection:** Existing ramdisk-based inspection (regression testing only)

---

## 6. Approach

### Testing Strategy

**Manual Testing:**
- Initial functional validation
- Hardware compatibility verification across vendors
- Performance measurement under various BMC conditions
- User workflow validation

**Automated Testing:**
- CI/CD integration for regression detection
- Unit tests for Redfish client and inventory mapping
- Integration tests with mock Redfish endpoints
- End-to-end tests with virtual BMC (Sushy-tools)

**Environment Tiers:**
1. **Development:** Local clusters with virtual BMC (Sushy-tools)
2. **CI:** Automated test suites with mock/virtual infrastructure
3. **Staging:** Real bare metal hardware across multiple vendors (Dell, HPE, Supermicro, Lenovo)
4. **Production:** Customer validation and field testing

### Test Phases

1. **Phase 1 - Functional Validation:** Core feature functionality and acceptance criteria
2. **Phase 2 - Deployment/Topology:** Platform-specific variations and configurations
3. **Phase 3 - Interoperability:** Integration with existing workflows and auto-discovery
4. **Phase 4 - Non-Functional:** Performance, reliability, and vendor compatibility
5. **Phase 5 - Operational:** Day-2 operations, troubleshooting, and monitoring
6. **Phase 6 - Regression:** Verification that existing inspection still works

---

## 7. Item Pass/Fail Criteria

| Test Item | Pass Criteria | Fail Criteria |
|-----------|---------------|---------------|
| **Fast Inspection Performance** | Completes within 1 minute with BMC response time < 0.1 sec | Takes longer than 1 minute or times out |
| **No Payload Boot** | Machine power state remains off or unchanged during inspection | Machine boots any OS, ramdisk, or payload |
| **Inventory Field Parity** | All vendor-neutral Redfish fields populate matching agent-based fields | Missing fields that are available in both Redfish and agent inspection |
| **Re-inspection Annotation** | Triggered re-inspection uses configured mode (fast/agent) | Re-inspection ignores configuration or uses wrong mode |
| **Redfish API Calls** | Uses only standard Redfish endpoints (no OEM extensions) | Requires vendor-specific OEM resources |
| **Hardware Data Schema** | Populates HardwareData spec with correct field types and values | Schema validation errors or incorrect data types |
| **BMC Compatibility** | Works with DMTF-compliant Redfish implementations | Fails on standard-compliant BMC firmware |
| **Error Handling** | Gracefully handles BMC timeouts, incomplete data, unsupported fields | Crashes, hangs, or leaves BareMetalHost in error state |

---

## 8. Suspension Criteria and Resumption Requirements

### Suspension Criteria
Testing should be suspended if any of the following conditions occur:

1. **Critical Infrastructure Failure:** Test environment cluster is unavailable or unstable
2. **Blocking Defects:** 
   - Fast inspection causes machine boots (violates FR-02)
   - Fast inspection corrupts BareMetalHost state
   - BMO crashes or becomes unresponsive during fast inspection
3. **API Breaking Changes:** BareMetalHost API changes require test plan revision
4. **Dependency Failures:** Redfish endpoints unavailable across all test hardware

### Resumption Requirements
Testing may resume when:

1. Infrastructure is restored and validated with basic smoke tests
2. Blocking defects are fixed and verified in development environment
3. API changes are documented and test plan is updated accordingly
4. At least one functional Redfish endpoint is available for testing

---

## 9. Test Deliverables

1. **Test Plan Document:** This IEEE 829-style test plan (Markdown)
2. **Test Case Specifications:** Detailed test cases for each scenario category (see Appendix)
3. **Test Results Report:** Pass/fail status for each test case with evidence
4. **Defect Reports:** Jira issues for any failures or deviations from acceptance criteria
5. **Performance Metrics:** Inspection completion times across different hardware vendors
6. **Compatibility Matrix:** Hardware vendor/model compatibility results
7. **CI Automation Artifacts:** Automated test scripts and CI job configurations
8. **Regression Test Results:** Verification that agent-based inspection remains functional

---

## 10. Testing Tasks

1. **Environment Setup:**
   - Provision test clusters with Metal3 and BareMetalHost CRDs
   - Configure virtual BMC endpoints (Sushy-tools) for automated testing
   - Acquire access to physical bare metal hardware (Dell, HPE, Supermicro, Lenovo)
   - Set up Redfish credential management

2. **Test Case Development:**
   - Author detailed test cases for functional validation scenarios
   - Create deployment/topology test cases for supported configurations
   - Develop interoperability test cases for auto-discovery workflows
   - Write performance and reliability test scenarios

3. **Test Execution:**
   - Execute functional validation test suite
   - Run deployment/topology variation tests
   - Perform interoperability testing with existing workflows
   - Conduct performance testing and collect metrics
   - Execute regression test suite for agent-based inspection

4. **Results Analysis:**
   - Analyze test results and identify failures
   - Categorize defects by severity and impact
   - Compare performance across hardware vendors
   - Assess inventory field coverage

5. **Defect Management:**
   - File Jira issues for test failures
   - Track defect resolution and retest
   - Update test results based on fixes

6. **CI/CD Integration:**
   - Integrate automated tests into CI pipeline
   - Configure test job triggers and reporting
   - Establish regression test automation

---

## 11. Environmental Needs

### Infrastructure Requirements

**Cluster Configurations:**
- OpenShift clusters with Metal3 operator installed
- BareMetalHost CRD installed and functional
- Network connectivity to BMC endpoints

**Hardware Requirements:**

| Vendor | Model Examples | Redfish Version | Quantity |
|--------|----------------|-----------------|----------|
| Dell | PowerEdge (14th gen+) | 1.6+ | 2+ systems |
| HPE | ProLiant Gen10+ | 1.6+ | 2+ systems |
| Supermicro | X11/X12 series | 1.6+ | 2+ systems |
| Lenovo | ThinkSystem SR series | 1.6+ | 1+ system |

**Virtual BMC:**
- Sushy-tools for mock Redfish endpoints
- Virtual media capabilities for testing
- Configurable response latency simulation

**Network:**
- Isolated VLAN for BMC traffic
- DHCP server for PXE (regression testing only)
- Routing to BMC management network

---

## 12. Responsibilities

| Role | Testing Responsibilities |
|------|--------------------------|
| **QE Team** | Test plan development, test case authoring, test execution, results analysis, defect reporting |
| **Development Team** | Unit test implementation, feature implementation verification, defect fixes, code review |
| **SRE Team** | Test environment provisioning, infrastructure monitoring, production readiness review |
| **Release Engineering** | CI/CD pipeline integration, release criteria validation, version compatibility testing |
| **Product Management** | Acceptance criteria definition, feature scope approval, customer use case validation |
| **Technical Writers** | Documentation review, user guide validation, troubleshooting guide verification |

---

## 13. Staffing and Training Needs

### Skill Requirements
- **Redfish API Knowledge:** Understanding of DMTF Redfish standard, API endpoints, data models
- **Bare Metal Operations:** Experience with BMC management, hardware inspection, provisioning workflows
- **Kubernetes Operators:** Familiarity with operator patterns, CRD development, controller reconciliation
- **Metal3 Platform:** Knowledge of BareMetalHost API, Ironic integration, inspection workflows

### Training Needs
- **Redfish Training:** Team members unfamiliar with Redfish should complete DMTF documentation review
- **BMO Architecture:** Review BareMetalHost controller architecture and inspection state machine
- **Test Tooling:** Training on Sushy-tools, virtual BMC setup, and Redfish client libraries

---

## 14. Schedule

| Milestone | Target | Dependencies |
|-----------|--------|--------------|
| **Test Plan Approval** | TBD | Feature requirements finalized |
| **Test Environment Ready** | TBD | Cluster provisioned, hardware allocated |
| **Test Case Authoring Complete** | TBD | API design stable |
| **Functional Testing Complete** | TBD | Feature implementation in development branch |
| **Deployment/Topology Testing** | TBD | Functional tests passing |
| **Interoperability Testing** | TBD | Auto-discovery integration ready |
| **Performance Testing** | TBD | Real hardware access |
| **Regression Testing** | TBD | Feature merged to main branch |
| **CI Integration Complete** | TBD | Automated tests authored |
| **Final Test Report** | TBD | All test phases complete |

---

## 15. Risks and Contingencies

| Risk | Impact | Probability | Mitigation Strategy |
|------|--------|-------------|---------------------|
| **Hardware Availability** | Cannot test vendor compatibility | Medium | Prioritize virtual BMC testing; schedule hardware windows early |
| **Redfish Implementation Variance** | Feature fails on certain vendors | High | Test across multiple vendors early; document vendor-specific issues |
| **BMC Firmware Bugs** | Inconsistent Redfish responses | Medium | Maintain compatibility matrix; report firmware issues to vendors |
| **Incomplete Redfish Data** | Inventory fields missing | High | Document per-vendor field availability; accept partial parity |
| **Performance Variability** | BMC response times exceed 0.1 sec | Medium | Use configurable timeout; test with simulated latency |
| **API Changes During Development** | Test cases become outdated | Low | Track API changes via PRs; update test plan as needed |
| **Dependency Delays** | Feature implementation delayed | Medium | Begin test case authoring early; use mock implementations |
| **Insufficient CI Resources** | Cannot run automated tests frequently | Low | Optimize test suite for speed; use virtual BMC for most tests |

---

## 16. Approvals

This test plan requires approval from the following stakeholders:

- **QE Lead:** _[Name/Role]_ - Test plan completeness and quality standards
- **Feature Owner:** Dmitry Tantsur - Acceptance criteria alignment and scope
- **Development Lead:** _[Name/Role]_ - Technical feasibility and implementation approach
- **Product Management:** _[Name/Role]_ - Business requirements and customer value
- **Release Engineering:** _[Name/Role]_ - Release readiness and CI/CD integration

**Approval Date:** TBD

---

## 17. Detailed Test Cases (Appendix)

### Coverage Summary

This section provides a summary of test coverage across all scenario categories. Each test case traces back to specific requirements from the OCPSTRAT feature. Full step-by-step test case details can be provided on request for any category.

---

#### 17.1 Functional Validation

**Source:** Functional Requirements (Acceptance Criteria)

| Test Case ID | Title | Requirement Traceability | Coverage |
|--------------|-------|--------------------------|----------|
| TC-FUNC-01 | Fast inspection completes within 1 minute | FR-01 | Complete |
| TC-FUNC-02 | Fast inspection does not boot any payload | FR-02 | Complete |
| TC-FUNC-03 | CPU information collected via Redfish | FR-03, INV-01 | Complete |
| TC-FUNC-04 | Memory capacity collected via Redfish | FR-03, INV-02 | Complete |
| TC-FUNC-05 | Network interfaces collected via Redfish | FR-03, INV-03 | Complete |
| TC-FUNC-06 | Storage devices collected via Redfish | FR-03, INV-04 | Complete |
| TC-FUNC-07 | System vendor information collected via Redfish | FR-03, INV-05 | Complete |
| TC-FUNC-08 | Firmware information collected via Redfish | FR-03, INV-06 | Complete |
| TC-FUNC-09 | Re-inspection respects fast mode configuration | FR-04 | Complete |
| TC-FUNC-10 | User configures BareMetalHost for fast inspection | UG-01 | Complete |
| TC-FUNC-11 | Fast inspection HardwareData schema validation | FR-03 | Complete |
| TC-FUNC-12 | Fast inspection state transitions in BMH status | UG-01 | Complete |

**Coverage Assessment:** All functional requirements from acceptance criteria are covered by dedicated test cases.

---

#### 17.2 Testing and Validation

**Note:** The OCPSTRAT feature does not include a separate "Testing and Validation Requirements" section. This category is covered by the Functional Validation test cases above.

---

#### 17.3 Deployment/Topology Variations

**Source:** Inferred from Metal Platform deployment patterns (no explicit deployment matrix provided in the feature)

**Note:** The feature does not specify deployment topology restrictions. Fast inspection is a BareMetalHost-level feature that should work consistently across OpenShift deployment types. Testing will focus on BMC/hardware variations rather than cluster topology.

| Deployment Type | Platform | Topology | Test Case ID | Coverage |
|-----------------|----------|----------|--------------|----------|
| Self-Managed | Bare Metal | Multi-node | TC-DEPLOY-01 | Complete |
| Self-Managed | Bare Metal | Compact (3-node) | TC-DEPLOY-02 | Complete |
| Self-Managed | Bare Metal | SNO | TC-DEPLOY-03 | Complete |

| Test Case ID | Title | Coverage |
|--------------|-------|----------|
| TC-DEPLOY-01 | Fast inspection on multi-node bare metal cluster | Complete |
| TC-DEPLOY-02 | Fast inspection on compact 3-node cluster | Complete |
| TC-DEPLOY-03 | Fast inspection on Single Node OpenShift (SNO) | Complete |

**Coverage Assessment:** Basic cluster topology coverage is complete. The feature is topology-agnostic and primarily depends on BMC/Redfish availability.

---

#### 17.4 Interoperability

**Source:** Interoperability Considerations section

The feature identifies two key interoperability scenarios:
1. **Faster alternative to agent-based inspection** - Use fast inspection when speed is prioritized over comprehensive inventory
2. **Integration with auto-discovery workflows** - Use fast inspection as part of machine enrollment and auto-discovery processes

| Test Case ID | Title | Requirement Traceability | Coverage |
|--------------|-------|--------------------------|----------|
| TC-INTEROP-01 | Fast inspection as alternative to agent-based inspection | Interop Use Case 1 | Complete |
| TC-INTEROP-02 | Fast inspection in auto-discovery enrollment workflow | Interop Use Case 2 | Complete |
| TC-INTEROP-03 | Switching from agent to fast inspection mode | Interop Use Case 1 | Complete |
| TC-INTEROP-04 | Fast inspection with existing provisioning workflows | General interop | Complete |
| TC-INTEROP-05 | Fast inspection results used for provisioning decisions | UG-03 | Complete |

**Coverage Assessment:** Both identified interoperability use cases are covered. Additional general interoperability tests ensure integration with existing BareMetalHost workflows.

---

#### 17.5 Upgrade/Rollback

**Status:** N/A

**Rationale:** The OCPSTRAT feature does not document upgrade or rollback requirements. The feature is additive (new inspection mode) and does not require migration of existing resources. Agent-based inspection continues to work as the default mode. Regression testing (see Section 17.10) verifies that existing inspection is not affected.

---

#### 17.6 Success Criteria

**Note:** The OCPSTRAT feature does not include a formal "Success Criteria" section with adoption or outcome metrics. The acceptance criteria (Requirements section) serve as the primary success criteria and are covered in Functional Validation (Section 17.1).

**Implied Success Criteria:**

| Success Metric | Test Case ID | Pass Criteria | Coverage |
|----------------|--------------|---------------|----------|
| Fast inspection completes successfully | TC-FUNC-01 | Inspection finishes within 1 minute | Complete |
| No machine boots during inspection | TC-FUNC-02 | Machine power state unchanged | Complete |
| Inventory data available for decision-making | TC-INTEROP-05 | HardwareData populated with sufficient fields | Complete |
| Feature usable in auto-discovery workflows | TC-INTEROP-02 | Auto-discovery can trigger fast inspection | Complete |

**Coverage Assessment:** Implied success criteria are covered by functional and interoperability test cases.

---

#### 17.7 Non-Functional Requirements

**Source:** Inferred from feature goals and acceptance criteria (no explicit NFR section provided)

The feature acceptance criteria imply several non-functional requirements:

| NFR Category | Test Case ID | Title | Requirement Traceability | Coverage |
|--------------|--------------|-------|--------------------------|----------|
| **Performance** | TC-NFR-PERF-01 | Inspection completes within 1 minute | FR-01 | Complete |
| **Performance** | TC-NFR-PERF-02 | Performance with slow BMC responses (0.5 sec) | FR-01 (boundary) | Complete |
| **Performance** | TC-NFR-PERF-03 | Performance degradation measurement vs agent inspection | Implied by "fast" | Complete |
| **Reliability** | TC-NFR-REL-01 | Graceful handling of BMC timeout | Error handling | Complete |
| **Reliability** | TC-NFR-REL-02 | Retry logic for transient BMC failures | Error handling | Complete |
| **Reliability** | TC-NFR-REL-03 | Recovery from incomplete Redfish data | Error handling | Complete |
| **Compatibility** | TC-NFR-COMPAT-01 | Dell hardware compatibility (Redfish 1.6+) | FR-03 (vendor-neutral) | Complete |
| **Compatibility** | TC-NFR-COMPAT-02 | HPE hardware compatibility (Redfish 1.6+) | FR-03 (vendor-neutral) | Complete |
| **Compatibility** | TC-NFR-COMPAT-03 | Supermicro hardware compatibility (Redfish 1.6+) | FR-03 (vendor-neutral) | Complete |
| **Compatibility** | TC-NFR-COMPAT-04 | Lenovo hardware compatibility (Redfish 1.6+) | FR-03 (vendor-neutral) | Complete |
| **Compatibility** | TC-NFR-COMPAT-05 | Redfish version compatibility (1.6, 1.8, 1.10) | DMTF standards | Complete |
| **Security** | TC-NFR-SEC-01 | BMC credentials secure storage and handling | Security best practices | Complete |
| **Security** | TC-NFR-SEC-02 | TLS/HTTPS for Redfish communication | Security best practices | Complete |
| **Usability** | TC-NFR-USE-01 | Clear error messages for unsupported hardware | User experience | Complete |
| **Usability** | TC-NFR-USE-02 | Status reporting during fast inspection | User experience | Complete |

**Pass/Fail Criteria:**

- **Performance:** Inspection completes within documented time limits; performance degradation is measurable and acceptable
- **Reliability:** System handles errors gracefully without data corruption or resource leaks
- **Compatibility:** Feature works on major hardware vendors with DMTF-compliant Redfish implementations
- **Security:** Credentials protected; all BMC communication encrypted
- **Usability:** Error messages actionable; status clearly communicated to users

**Coverage Assessment:** Complete coverage for performance, reliability, and compatibility. Security and usability covered at basic level.

---

#### 17.8 Operational / Day-2

**Source:** Inferred operational needs (no explicit Operational Requirements section provided)

| Test Case ID | Title | Operational Need | Coverage |
|--------------|-------|------------------|----------|
| TC-OPS-01 | Inspection failure troubleshooting workflow | Support runbook | Partial |
| TC-OPS-02 | BMC connectivity diagnostics | Troubleshooting | Partial |
| TC-OPS-03 | Inspection metrics exposure (if implemented) | Observability | Gap |
| TC-OPS-04 | Logging for inspection failures | Troubleshooting | Partial |
| TC-OPS-05 | Documentation accuracy verification | Support enablement | Partial |

**Coverage Assessment:** Partial. Operational requirements are not explicitly defined in the feature. Testing focuses on basic troubleshooting and observability. Full operational validation requires documentation and runbook development (noted in Documentation Considerations).

**Recommendation:** Expand operational testing once inspection documentation is developed.

---

#### 17.9 Negative Tests

**Source:** Out of Scope section (explicit boundaries)

The Out of Scope section defines clear boundaries that can be validated through negative testing:

| Test Case ID | Title | Out of Scope Item | Coverage |
|--------------|-------|-------------------|----------|
| TC-NEG-01 | OEM-specific fields are not required for fast inspection | OEM fields not required | Complete |
| TC-NEG-02 | Non-Redfish protocols are not supported | Non-Redfish implementations | Complete |
| TC-NEG-03 | IPI flow remains unchanged | IPI modifications | Complete |
| TC-NEG-04 | Missing Redfish fields do not block inspection | Partial data acceptable | Complete |

**Pass Criteria:**
- TC-NEG-01: Fast inspection completes successfully even when OEM resources are unavailable
- TC-NEG-02: Attempting fast inspection on non-Redfish BMC results in clear error (not crash)
- TC-NEG-03: IPI installation workflow is not affected by fast inspection feature
- TC-NEG-04: Inspection completes with partial inventory when some Redfish endpoints return no data

**Coverage Assessment:** All explicit out-of-scope boundaries are covered with negative test validation.

---

#### 17.10 Regression Scenarios

**Purpose:** Verify that the introduction of fast inspection does not break existing agent-based inspection or BareMetalHost functionality.

| Test Case ID | Title | Regression Target | Coverage |
|--------------|-------|-------------------|----------|
| TC-REG-01 | Agent-based inspection still works (default behavior) | Existing inspection | Complete |
| TC-REG-02 | Switching from fast to agent mode works | Mode switching | Complete |
| TC-REG-03 | BareMetalHost provisioning after fast inspection | Provisioning workflow | Complete |
| TC-REG-04 | BareMetalHost provisioning after agent inspection (baseline) | Provisioning workflow | Complete |
| TC-REG-05 | Re-inspection annotation with agent mode | Re-inspection | Complete |
| TC-REG-06 | BareMetalHost state machine transitions unchanged | Controller logic | Complete |
| TC-REG-07 | Existing HardwareData schema compatibility | API schema | Complete |

**Coverage Assessment:** Complete. All existing inspection and provisioning workflows are covered by regression tests.

---

## 18. Readiness Integration

### Linking from the Feature
Add this test plan as a link or attachment on OCPSTRAT-3566 so it is discoverable during feature readiness reviews and planning discussions.

### Storage in Git Repository
Commit this Markdown test plan to the appropriate repository:
- **Option 1:** `openshift/baremetal-operator` repository under `docs/test-plans/`
- **Option 2:** Component-specific quality repository (if one exists for Metal3/BMO)
- **Option 3:** OpenShift enhancements repository under `enhancements/baremetal/test-plans/`

This ensures version control, review process integration, and long-term discoverability.

### Multiple Plans for Complex Work
This feature is self-contained within the BareMetalHost API and baremetal-operator. If future work expands fast inspection to other components (e.g., cluster-baremetal-operator integration, IPI flow changes), create separate component-specific test plans that cross-reference this plan.

### Derive Work Items
Use the Testing Tasks (Section 10) and Detailed Test Cases (Section 17) to create specific QE work items:

**Example Work Item Structure:**
- **Epic:** OCPSTRAT-3566 - Fast Out-of-Band Inspection QE
- **Stories/Tasks:**
  - Story: Author functional validation test cases (TC-FUNC-01 through TC-FUNC-12)
  - Story: Set up virtual BMC test environment (Sushy-tools)
  - Story: Execute hardware compatibility testing (TC-NFR-COMPAT-01 through TC-NFR-COMPAT-05)
  - Story: Develop performance benchmarking suite (TC-NFR-PERF-01 through TC-NFR-PERF-03)
  - Story: Integrate automated tests into CI pipeline
  - Task: Acquire Dell PowerEdge hardware access
  - Task: Acquire HPE ProLiant hardware access
  - Task: Document fast inspection troubleshooting workflow

Each work item should trace back to specific test case IDs from this plan for traceability.

---

## 19. Expansion Requests

This test plan provides a comprehensive coverage summary with test case IDs, titles, and requirement traceability. For detailed test case specifications with full step-by-step instructions, preconditions, expected results, and verification commands, request expansion of specific categories:

**Example Requests:**
- "Expand the Functional Validation test cases (TC-FUNC-01 through TC-FUNC-12)"
- "Show full details for TC-NFR-COMPAT-01 (Dell hardware compatibility)"
- "Provide step-by-step instructions for TC-INTEROP-02 (auto-discovery workflow)"
- "Detail all Performance test cases with measurement methodology"

---

**Document Version:** 1.0  
**Last Updated:** 2026-09-15  
**Status:** Draft - Pending Approval
