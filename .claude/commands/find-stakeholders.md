---
description: Find potential stakeholder reviewers for an OpenShift enhancement proposal
---

You are helping to identify potential stakeholders who should review a new OpenShift enhancement proposal.

## Your Task

1. **Locate and read the enhancement proposal:**
   - Ask the user which enhancement file they want to find stakeholders for (if not obvious from context)
   - Read the entire enhancement proposal to understand:
     - What the enhancement does (Summary, Motivation sections)
     - Which domains/components it affects (look for references to operators, APIs, subsystems)
     - Whether it involves API changes (API Extensions section, api-approvers field)
     - Specific technologies mentioned (networking, storage, authentication, etcd, etc.)
     - Any existing reviewers/approvers already listed in the YAML frontmatter

2. **Identify the affected OpenShift components and repositories** based on what you learned from reading the enhancement:
   - Look for references to specific operators (e.g., cluster-authentication-operator, console-operator, machine-config-operator)
   - Look for references to core components (e.g., openshift/api, hypershift, installer, oauth-server)
   - Look for references to domain-specific components (e.g., cluster-monitoring-operator, cluster-network-operator, cluster-ingress-operator)
   - Create a list of potential repository names to search for

3. **Query the openshift GitHub organization** to find the actual repository names:
   - Use WebFetch to query the GitHub API for openshift repositories: `https://api.github.com/orgs/openshift/repos?per_page=100&page=1`
   - Note: The API returns paginated results (100 per page). You may need to fetch multiple pages if needed.
   - Search through the repository list to find repositories that match the components mentioned in the enhancement
   - Look for repositories containing relevant keywords (e.g., if enhancement mentions "authentication", look for repos with "authentication" or "oauth" in the name)
   - Common repository patterns (but don't rely solely on these):
     - `<component>-operator` (e.g., console-operator, machine-config-operator)
     - `cluster-<component>-operator` (e.g., cluster-authentication-operator, cluster-network-operator)
     - Component names without "-operator" suffix (e.g., installer, hypershift, api)

4. **Fetch OWNERS files from the identified repositories** to get current maintainers:
   - Use WebFetch to retrieve OWNERS files from the matched repositories
   - Try both `master` and `main` branches:
     - `https://raw.githubusercontent.com/openshift/<repo-name>/master/OWNERS`
     - `https://raw.githubusercontent.com/openshift/<repo-name>/main/OWNERS`
   - Fetch multiple repository OWNERS files in parallel when the enhancement affects multiple components
   - Extract the `approvers` and `reviewers` lists from each OWNERS file

5. **Use local enhancement OWNERS files as supplementary context** (lower priority):
   - Look in `enhancements/<domain>/OWNERS` files for related domains (if they exist)
   - Available domains include: network, ingress, storage, installer, authentication, machine-config, kube-apiserver, etcd, monitoring, observability, cluster-logging, dns, security, and many others
   - Check the root `OWNERS` file for staff engineers who handle broad-scope enhancements
   - **Important:** Local enhancement OWNERS files may be outdated or missing. Always prioritize repository OWNERS files from step 4 over enhancement OWNERS files

6. **Verify API reviewer assignment** (critical step):
   - Check if the enhancement involves API changes (new APIs, API extensions, CRDs, webhooks, etc.)
   - If it does, verify that the `api-approvers` field in the YAML frontmatter is populated
   - Use WebFetch to retrieve the openshift/api OWNERS file: https://github.com/openshift/api/blob/master/OWNERS
   - Verify that at least one person listed in the enhancement's `api-approvers` field appears in the openshift/api OWNERS file as an approver or reviewer
   - If there is NO api-approver listed OR the listed api-approver is not in the openshift/api OWNERS file, recommend that the author reach out to the #forum-api-review Slack channel to request an API reviewer be assigned
   - Slack channel link: https://redhat.enterprise.slack.com/archives/CE4L0F143

7. **Present your findings** in this format:

   ```
   ## Potential Stakeholder Reviewers for [Enhancement Name]

   ### Primary Repository: openshift/[repo-name]
   **Approvers:**
   - @username1
   - @username2

   **Reviewers:**
   - @username3
   - @username4

   ### Related Repository: openshift/[related-repo-name]
   **Approvers:**
   - @username5

   **Reviewers:**
   - @username6

   ### API Review Status

   [Either:]
   ✅ **Good news!** The enhancement has @username listed as api-approver, who is confirmed as an [approver/reviewer] in the openshift/api OWNERS file.

   [Or:]
   ⚠️ **Action needed:** This enhancement introduces API changes but [does not have an api-approver listed / has @username listed who is not in the openshift/api OWNERS file]. Please reach out to the #forum-api-review Slack channel (https://redhat.enterprise.slack.com/archives/CE4L0F143) to request an API reviewer be assigned.

   ### Recommended Next Steps:
   1. For the primary repository (openshift/[repo]), consider reaching out to [specific approver] as your enhancement approver
   2. Include reviewers from [list repositories] to ensure all affected areas are covered
   3. If this is a broad-scope enhancement affecting multiple pillars, consider consulting with staff engineers from the root OWNERS file
   ```

8. **Provide guidance** on:
   - Which approver would be most appropriate (remind them that a single approver is preferred)
   - Which reviewers to include and what domain expertise they should focus on
   - Whether they should reach out in `#forum-arch` on Slack if this is high-priority or broad-scope

## Important Context

- Enhancement reviewers should include representatives from any team that will need to do work for the enhancement
- The approver helps recognize when consensus is reached and doesn't need to be a subject-matter expert (but it helps)
- For broad-scope enhancements (changing OpenShift definition, adding required dependencies, changing customer support), a staff engineer approver is appropriate
- Clearly indicate what aspect of the enhancement each reviewer should focus on (see guidelines/README.md for examples)

## Tool Usage

- Use WebFetch to query the GitHub API for openshift organization repositories
- Use WebFetch to retrieve OWNERS files from identified openshift repositories (primary source of truth)
- Use WebFetch to retrieve the openshift/api OWNERS file for API reviewer verification
- Use the Glob tool to find local enhancement OWNERS files: `find enhancements -name "OWNERS"` (supplementary context only)
- Use the Read tool to read local enhancement OWNERS files if needed (supplementary context only)
- Be efficient: fetch multiple OWNERS files in parallel when possible
- Remember: Repository OWNERS files are the authoritative source and should take priority over local enhancement OWNERS files
