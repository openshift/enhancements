# Webhooks

**Category**: Operator Pattern
**Last Updated**: 2026-10-23

---

## Overview

Kubernetes admission webhooks intercept API requests for validation and mutation. OpenShift uses webhooks for both platform components and user-facing APIs, with some notable differences from upstream Kubernetes.

## Webhook Types

| Type | Purpose | When |
|------|---------|------|
| **Validating** | Reject invalid requests | After authentication, authorization, and mutation |
| **Mutating** | Set defaults, inject fields | Before validation |
| **Conversion** | Convert between API versions | On storage/retrieval |

**Important**: Most OpenShift APIs handle version transitions via internal or aggregated-server conversion rather than CRD conversion webhooks. Exceptions exist — for example, `ConsoleDynamicPlugin` uses a CRD conversion webhook. Check the specific API before assuming no conversion webhook is in use.

## Registration

Webhooks are registered via `ValidatingWebhookConfiguration` or `MutatingWebhookConfiguration` resources. For operators using controller-runtime:

```go
mgr.GetWebhookServer().Register("/validate-myresource", &webhook.Admission{
    Handler: &MyValidator{},
})
```

## Key Conventions

1. **Choose failurePolicy carefully** — use `failurePolicy: Fail` when the webhook enforces invariants; use `Ignore` only when a safe fallback exists and the webhook is not required for correctness
2. **Namespace selectors** — scope webhooks to relevant namespaces to avoid performance impact
3. **Timeout** — set `timeoutSeconds` to 10 or less; default is 10s
4. **Side effects** — webhooks must be side-effect-free (`sideEffects: None`)
5. **Idempotency** — mutating webhooks must produce the same result when called multiple times

## CEL Validation

For new APIs, prefer **CEL (Common Expression Language)** validation rules over webhooks where possible. CEL rules are declared inline in the CRD schema and don't require running a webhook server.

## Platform Documentation

For API conventions around validation, see [dev-guide/api-conventions.md](../../../dev-guide/api-conventions.md). For API evolution patterns, see [practices/development/api-evolution.md](../../practices/development/api-evolution.md).
