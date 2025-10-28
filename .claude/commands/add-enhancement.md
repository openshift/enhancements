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

You are tasked with creating a new OpenShift Enhancement Proposal based on the template at `guidelines/enhancement_template.md`.

## Inputs Provided

- **Area**: {{area}}
- **Name**: {{name}}
- **Description**: {{description}}
- **JIRA Ticket**: {{jira}}

## Instructions

Act as an experienced software architect to create a comprehensive enhancement proposal. Follow these steps:

1. **Parse the Description**: Extract the following from the description:
   - **What**: What is this enhancement about
   - **Why**: Why this change is required (motivation)
   - **Who**: Which personas this applies to (use this to generate user stories)

2. **Ask Clarifying Questions** (if needed): Use the AskUserQuestion tool to gather:
   - Specific user stories or motivations if not clear from the description
   - Explicit Goals or Non-Goals the user wants included
   - Any specific technical constraints or requirements
   - Topology considerations (Hypershift, SNO, MicroShift relevance)
   - Whether this proposal adds/changes CRDs, admission and conversion webhooks, aggregated API servers, or finalizers (needed for API Extensions section)

3. **Generate the Enhancement File**:
   - Create the file at `enhancements/{{area}}/{{filename}}.md` where filename is the kebab-case version of the name
   - Fill in the template with:
     - **Title**: Use the provided name
     - **Summary**: One paragraph describing what this enhancement is about
     - **Motivation**: Explain why this change is required based on the description
     - **User Stories**: Generate 2-4 user stories based on the "who" information using the format:
       > "As a _role_, I want to _take some action_ so that I can _accomplish a goal_."
     - **Goals**: List specific, measurable goals (3-5 items)
     - **Non-Goals**: List what is explicitly out of scope (2-3 items)
     - **Proposal**: High-level description of the proposed solution
     - **Workflow Description**: Detailed workflow with actors and steps
     - **Mermaid Diagram**: Add a sequence diagram when applicable to visualize the workflow
     - **API Extensions**: Only fill this section if the user confirms the proposal adds/changes CRDs, admission and conversion webhooks, aggregated API servers, or finalizers. Otherwise, add a TODO comment asking the user to complete this section if applicable.
     - **Implementation Details/Notes/Constraints**: Provide a high-level overview of the code changes required. Follow the guidance from the template: "While it is useful to go into the details of the code changes required, it is not necessary to show how the code will be rewritten in the enhancement." Keep it as an overview; the developer should fill in the specific implementation details.
     - **Metadata**: Fill in creation-date with today's date (2025-10-28), tracking-link with the provided JIRA ticket URL, set other fields to TBD

4. **Handle Unfilled Sections**: For sections that cannot be filled based on the input:
   - Add a clear comment like `<!-- TODO: This section needs to be filled in -->`
   - Provide guidance on what should be included

5. **Writing Guidelines**:
   - Write in a clear, concise, professional manner
   - Focus on the essential information
   - Use bullet points and structured formatting
   - Avoid unnecessary verbosity
   - **Line Length**: Keep lines in the generated enhancement at a maximum of 80 characters. It is acceptable to exceed by 10-15 characters when necessary (e.g., for URLs or code examples), but not more than that.

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
