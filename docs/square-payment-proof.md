# Square payment policy investigation

Status: research plan; no provider tests completed. Reviewed 7 October 2026.

## Required policy

For card payments, require successful 3DS authentication by default. Block failed, cancelled, unsupported, unavailable or unproven authentication before unauthenticated authorization. Successfully authenticated frictionless transactions are acceptable; a visible challenge is not mandatory. Wallets, exemptions and other payment methods require a separate approved policy. No silent fallback to another provider or refund-after-authorization substitute.

## Current documented integration

Square's [current card-payment guide](https://developer.squareup.com/docs/web-payments/take-card-payment) uses `Card.tokenize(verificationDetails)` and describes authentication based on regulatory requirements or seller Risk Manager rules. Its separate `verifyBuyer()` method is deprecated. Use the current flow for new investigation code.

The [SCA overview](https://developer.squareup.com/docs/sca-overview) describes provider determination of whether authentication is required and differences between sandbox and production. These documents do not by themselves establish enforcement of the required all-card blocking policy for a particular UK account. A successful tokenization or payment alone is insufficient evidence that successful 3DS was mandatory.

## Evidence matrix

Every row starts unverified. Record the SDK/API versions, scenario, environment, configured control, trusted evidence source, observed outcome and conclusion. Keep actual tokens, credentials, billing inputs, raw responses and account identifiers outside the public repository. Published examples must be synthetic and sanitized.

| Scenario | Required result and evidence |
| --- | --- |
| Successful frictionless authentication | Demonstrate successful 3DS without a challenge and a trustworthy enforcement path; only then allow authorization. |
| Successful challenge | Complete the challenge; prove failed outcomes would be blocked before authorization. |
| Failed authentication | Block; show that an unauthenticated authorization is not submitted or permitted by the provider control. |
| Cancelled challenge or browser closed | No unauthenticated authorization; preserve a safe retry state. |
| Unsupported card or authentication unavailable | Block; do not interpret a fallback payment token as successful authentication. |
| Missing, altered or replayed browser evidence | Reject untrusted assertions; demonstrate the server/provider enforcement boundary. |
| Direct server submission bypassing browser verification | Prove that the configured policy prevents unauthenticated authorization, including a caller supplying a separately obtained token. |
| Amount/currency changed after verification | Reject or reauthenticate against the authoritative amount and currency; do not trust browser totals. |
| Authentication succeeds but issuer declines | Report decline without creating a paid order or granting an exception. |
| Duplicate submission or retry | One financial effect using a stable server-owned attempt/idempotency key; do not generate a new key simply because a response timed out. |
| Network interruption after submission | Treat the outcome as unknown and reconcile before a new charge. |
| Policy/account configuration absent or changed | Fail closed or keep checkout disabled until enforcement can be established. |

Use Square's [sandbox payment guide](https://developer.squareup.com/docs/devtools/sandbox/payments) for official test inputs. Missing sandbox coverage is an evidence gap, not a simulated pass. Tests with a fake adapter can verify application logic but cannot prove Square's capabilities.

## Decision record

The investigation must answer: can the selected UK integration prevent authorization unless successful 3DS has been established, and what trustworthy mechanism enforces that? Identify any account eligibility, configuration, plan cost or runtime observability dependency. Record sandbox and live-account conclusions separately.

If this cannot be demonstrated, document the exact gap and present options to the maintainer. Continue foundation work. Do not enable live card payments, weaken policy, purchase an account feature or change providers automatically.
