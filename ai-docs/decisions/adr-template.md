# ADR-NNNN: [Title]

**Status**: Proposed | Accepted | Deprecated | Superseded by [ADR-XXXX]  
**Date**: YYYY-MM-DD  
**Authors**: @username1, @username2  
**Scope**: Cross-repository  

## Context

What is the issue or situation that motivates this decision?

- Background information
- Current state
- Problem to solve
- Constraints (technical, organizational, timeline)

## Decision

What is the decision we're making?

State the decision clearly and concisely. Be specific about:
- What we will do
- What we won't do
- How it will work

### Example

```yaml
# If the decision involves API or code changes, include examples
apiVersion: config.openshift.io/v1
kind: ClusterOperator
status:
  conditions:
  - type: Available
    status: "True"
```

## Rationale

Why did we make this decision?

- Benefits of this approach
- How it solves the problem
- Why this is better than alternatives

## Consequences

What are the implications of this decision?

### Positive

- Benefit 1
- Benefit 2

### Negative

- Trade-off 1
- Trade-off 2

### Neutral

- Change 1 (neither good nor bad, just different)

## Alternatives Considered

What other options did we evaluate?

### Alternative 1: [Name]

**Description**: What this alternative would look like

**Pros**:
- Pro 1
- Pro 2

**Cons**:
- Con 1
- Con 2

**Rejected because**: Reason for rejection

### Alternative 2: [Name]

...

## Implementation

How will this decision be implemented?

- Changes required (API, operators, components)
- Migration path (if applicable)
- Timeline (if applicable)
- Affected components

## References

- Enhancement: [Link]
- Related ADRs: [Link]
- External docs: [Link]
- Discussions: [Link to GitHub issue/PR]
