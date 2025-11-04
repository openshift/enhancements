---
description: Create a new OpenShift Enhancement Proposal
args:
  - name: area
    description: Enhancement area (subdirectory under enhancements/)
  - name: name
    description: One-line title describing the enhancement
  - name: description
    description: Detailed description (what, why, who)
  - name: jira
    description: JIRA ticket URL for tracking
---

You are tasked with creating a new OpenShift Enhancement Proposal based on the template at `guidelines/enhancement_template.md`. You must mirror all required headings from guidelines/enhancement_template.md exactly, even if there is nothing to be added, and in this case the section should be empty.

## Inputs Provided

- **Area**: {{area}}
- **Name**: {{name}}
- **Description**: {{description}}
- **JIRA Ticket**: {{jira}}

## Instructions

Act as an experienced software architect to create a comprehensive enhancement proposal. Follow these steps:

**Important**: Reference the guidance in `dev-guide/feature-zero-to-hero.md`, particularly the section "Writing an OpenShift Enhancement", when creating enhancement proposals. This guide provides essential context on the OpenShift Enhancement Proposal process, feature gates, API design conventions, testing requirements, and promotion criteria.

1. **Parse the Description**: Extract the following from the description:
   - **What**: What is this enhancement about
   - **Why**: Why this change is required (motivation)
   - **Who**: Which personas this applies to (use this to generate user stories)

2. **Ask Clarifying Questions** (if needed): Use the AskUserQuestion tool to gather:
   - Specific user stories or motivations if not clear from the description
   - Explicit Goals or Non-Goals the user wants included
   - Any specific technical constraints or requirements
   - Topology considerations (Hypershift, SNO, MicroShift, OKE relevance)
   - Whether this proposal adds/changes CRDs, admission and conversion webhooks, ValidatingAdmissionPlugin, MutatingAdmissionPlugin, aggregated API servers, or finalizers (needed for API Extensions section)
   - Feature gate information: According to dev-guide/feature-zero-to-hero.md, ALL new OpenShift features must start disabled by default using feature gates. Ask about the proposed feature gate name and initial feature set (DevPreviewNoUpgrade or TechPreviewNoUpgrade).
   - Ask clarifying questions about telemetry, security, upgrade and downgrade process, rollbacks, dependencies, in case it is not possible to assert these fields.

3. **Generate the Enhancement File**:
   - Create the file at `enhancements/{{area}}/{{filename}}.md` where filename is the kebab-case version of the name
   - Fill in the template with:
     - **Title**: Use the provided name
     - **Summary**: One paragraph describing what this enhancement is about
     - **Motivation**: Explain why this change is required based on the description
     - **User Stories**: Generate 2-4 user stories based on the "who" information using the format:
       > "As a _role_, I want to _take some action_ so that I can _accomplish a goal_."
       Include a story on how the proposal will be operationalized: life-cycled, monitored and remediated at scale.
     - **Goals**: List specific, measurable goals (3-5 items). Goals should describe what users want from their perspective, not implementation details.
     - **Non-Goals**: List what is explicitly out of scope (2-3 items)
     - **Proposal**: High-level description of the proposed solution
     - **Workflow Description**: Detailed workflow with actors and steps
     - **Mermaid Diagram**: Add a sequence diagram when the workflow involves multiple actors or complex interactions between components (e.g., user -> API server -> controller -> operator). Simple single-actor workflows may not need a diagram.
     - **API Extensions**: Only fill this section if the user confirms the proposal adds/changes CRDs, admission and conversion webhooks, ValidatingAdmissionPlugin, MutatingAdmissionPlugin, aggregated API servers, or finalizers. Per the template, name the API extensions and describe if this enhancement modifies the behaviour of existing resources. Otherwise, add a TODO comment asking the user to complete this section if applicable.
     - **Topology Considerations**: Include subsections for Hypershift/Hosted Control Planes, Standalone Clusters, Single-node Deployments or MicroShift, and OKE (OpenShift Kubernetes Engine). Address how the proposal affects each topology.
     - **Implementation Details/Notes/Constraints**: Provide a high-level overview of the code changes required. Follow the guidance from the template: "While it is useful to go into the details of the code changes required, it is not necessary to show how the code will be rewritten in the enhancement." Keep it as an overview; the developer should fill in the specific implementation details. Include a reminder about creating a feature gate: Per dev-guide/feature-zero-to-hero.md, all new features must be gated behind a feature gate in https://github.com/openshift/api/blob/master/features/features.go with the appropriate feature set (DevPreviewNoUpgrade or TechPreviewNoUpgrade initially).
     - **Test Plan**: Add a TODO comment with guidance on required test labels per dev-guide/feature-zero-to-hero.md: Tests must include `[OCPFeatureGate:FeatureName]` label for the feature gate, `[Jira:"Component Name"]` for the component, and appropriate test type labels like `[Suite:...]`, `[Serial]`, `[Slow]`, or `[Disruptive]` as needed. Reference dev-guide/test-conventions.md for details.
     - **Graduation Criteria**: Add a TODO comment referencing the specific promotion requirements from dev-guide/feature-zero-to-hero.md: minimum 5 tests, 7 runs per week, 14 runs per supported platform, 95% pass rate, and tests running on all supported platforms (AWS, Azure, GCP, vSphere, Baremetal with various network stacks).
     - **Metadata**: Fill in creation-date with today's date, tracking-link with the provided JIRA ticket URL, set other fields to TBD. For api-approvers: use "None" if there are no API changes (no new/modified CRDs, webhooks, aggregated API servers, or finalizers); otherwise use "TBD" as a placeholder (the enhancement author will request an API reviewer from the #forum-api-review Slack channel later).

4. **Handle Unfilled Sections**: For sections that cannot be filled based on the input:
   - Add a clear comment like `<!-- TODO: This section needs to be filled in -->`
   - Provide guidance on what should be included

5. **Writing Guidelines**:
   - Write in a clear, concise, professional manner
   - Focus on the essential information
   - Use bullet points and structured formatting
   - Avoid unnecessary verbosity
   - **Line Length**: Keep lines in the generated enhancement at a maximum of 80 characters, but prioritize validity over line length limits. Only break lines at 80 characters if doing so will NOT create:
     - Invalid or broken URLs (URLs themselves should never be split, but the line CAN and SHOULD be broken before or after the URL)
     - Invalid markdown syntax (e.g., breaking markdown links, code blocks, or formatting)
     - Invalid code examples (e.g., breaking code in the middle of statements)
     If breaking at 80 characters would split a URL, code, or markdown syntax, find the nearest valid break point such as: after a sentence, before a URL starts, after a URL ends, or at a natural paragraph break. For regular prose, it is acceptable to exceed 80 characters by 10-15 characters to avoid breaking words mid-word. Only allow lines >95 characters when the line contains a single unbreakable element (like a standalone URL with no surrounding text, or a single line of code).

6. **Validate**:
   - Ensure the area directory exists under `enhancements/`
   - Create a valid filename from the name (lowercase, replace spaces with dashes)
   - Verify all required YAML metadata is present
   - Verify the JIRA ticket URL is included in the tracking-link metadata field

## Output

After creating the enhancement file, provide:
- The full path to the created file
- A brief summary of what was included
- A list of sections that need further attention (marked with TODO comments)

Begin by analyzing the inputs and asking any necessary clarifying questions before generating the enhancement proposal.
