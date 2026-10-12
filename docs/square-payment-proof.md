# Square payment policy investigation

Status: account access and selected sandbox authentication probes completed;
strict all-card enforcement and live compatibility remain unverified.
Updated 12 October 2026. See measured results below; no checkout adapter enabled.

## Required policy

For card payments, require successful 3DS authentication by default. Block failed, cancelled, unsupported, unavailable or unproven authentication before unauthenticated authorization. Successfully authenticated frictionless transactions are acceptable; a visible challenge is not mandatory. Wallets, exemptions and other payment methods require a separate approved policy. No silent fallback to another provider or refund-after-authorization substitute.

## Current documented integration

Square's [current card-payment guide](https://developer.squareup.com/docs/web-payments/take-card-payment) uses `Card.tokenize(verificationDetails)` and describes authentication based on regulatory requirements or seller Risk Manager rules. Its separate `verifyBuyer()` method is deprecated. Use the current flow for new investigation code.

The [SCA overview](https://developer.squareup.com/docs/sca-overview) describes provider determination of whether authentication is required and differences between sandbox and production. These documents do not by themselves establish enforcement of the required all-card blocking policy for a particular UK account. A successful tokenization or payment alone is insufficient evidence that successful 3DS was mandatory.

## Evidence matrix

Use the measured results below for completed probes; remaining scenarios stay unverified. Record SDK/API versions, scenario, environment, configured control, trusted evidence source, observed outcome and conclusion. Keep actual tokens, credentials, billing inputs, raw responses and account identifiers outside the public repository. Published examples must be synthetic and sanitized.

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

## Research update — 11 October 2026

Status: **strict-policy compatibility remains unverified**, not a provider-test
pass and not a confirmed claim that Square cannot support it. No account requests,
SDK tokenizations or payment submissions have been performed for this review.
The following are documentation findings, not measured provider outcomes.

| Source | Finding | Consequence for our proof |
| --- | --- | --- |
| [UK Risk Manager guidance](https://squareup.com/help/gb/en/article/6816-navigating-square-risk-manager) | Describes UK/EEA merchant-and-issuer coverage and says UK 3DS rules cannot be modified through Risk Manager. | Do not assume a UK seller can configure a blanket all-card rule. Obtain UK-account confirmation, including non-UK/EEA issuers and exemptions. |
| [Current SDK card flow](https://developer.squareup.com/docs/web-payments/take-card-payment) | Tokenization accepts amount, currency, billing contact and initiation flags; Square determines when authentication is required. | Tokenization success alone does not prove that every payment successfully completed 3DS. Use CHARGE, customerInitiated=true and sellerKeyedIn=false; amounts come from the saved server review. |
| [SCA overview](https://developer.squareup.com/docs/sca-overview) | Documents mandate/rule-dependent behavior, a private-beta custom-risk mechanism, and sandbox/production differences for stored cards. | Do not rely on beta eligibility, MOTO/seller initiation or stored-card defaults to satisfy the policy. Fresh customer-initiated card payments are the investigation scope. |
| [CardPaymentDetails reference](https://developer.squareup.com/reference/square/objects/CardPaymentDetails) | auth_result_code describes issuer authorization; verification_method/results describe EMV verification. | These fields are not documented as online 3DS success evidence. Neither CVV/AVS nor a captured-payment status establishes the pre-authorization gate. |
| [Sandbox payment scenarios](https://developer.squareup.com/docs/devtools/sandbox/payments) | Provides frictionless, challenge and failed-verification card scenarios. Some notes still describe verifyBuyer despite the current tokenize migration. | Test the current SDK path and record discrepancies. Generic successful API test tokens do not prove browser authentication; missing unsupported/unavailable coverage remains a gap. |

The reviewed Payments/Locations API references show version **2026-09-16**.
The hosted browser SDK version must also be recorded at the time of an actual
probe. No SDK/account enforcement version has been observed yet.

### Questions requiring provider/account evidence

Prepared questions for Square; no support message has been sent:

1. For a UK API seller using the current Web Payments SDK, can authorization be
   prevented unless 3DS succeeds for every fresh customer-initiated card payment,
   including non-UK/EEA issuers and payments eligible for exemptions?
2. What exact documented account/API control enforces this before authorization?
   How does it interact with the UK statement that 3DS rules cannot be modified?
3. Are unsuccessful, cancelled, unsupported, unavailable and attempted-only 3DS
   outcomes blocked? Can a direct CreatePayment request with a token obtained
   without verification bypass the control? What trusted evidence can our server
   use before authorization? A later refund/void does not meet the policy.
4. Can that control and its negative cases be exercised in a UK sandbox? Which
   cases differ in production? Identify account eligibility, cost and any way to
   detect a disabled or changed control.

Until there is a trustworthy enforceable mechanism and negative-case evidence,
Selloovy card-payment submission remains disabled. Square remains the preferred provider;
there is no policy relaxation or automatic provider switch.

### Read-only sandbox readiness command

`go run ./cmd/square-sandbox-check` makes only a GET to Square's fixed sandbox
[Locations endpoint](https://developer.squareup.com/reference/square/locations-api/list-locations),
using API version 2026-09-16. It checks for one selected ACTIVE GB/GBP location
with CREDIT_CARD_PROCESSING. It creates no payment, order or card and exposes no
HTTP probe route. It does not inspect Risk Manager settings or prove 3DS.

Configure a **sandbox** access token in an ignored private regular file, readable
only by its owner (mode 0600 or 0400); do not put it in chat, Git, command-line
arguments or a recorded/shared terminal. Select Sandbox in the Developer Console
and a UK test account. Use synthetic merchant/customer data. Square documents
sandbox separation and setup in its [sandbox overview](https://developer.squareup.com/docs/devtools/sandbox/overview).
The GET requires MERCHANT_PROFILE_READ. No payment-write permission is needed for
this check; account credentials may themselves have wider scopes.

```sh
export SELLOOVY_SQUARE_SANDBOX_ACCESS_TOKEN_FILE=local/square-sandbox.token
# Optional when exactly one eligible location exists; required if ambiguous.
export SELLOOVY_SQUARE_SANDBOX_LOCATION_ID=YOUR_SANDBOX_LOCATION_ID
go run ./cmd/square-sandbox-check
```

Create the token file privately using your chosen local secret-entry workflow;
the command never creates or changes credentials. It rejects symlinks,
group/world-accessible files and malformed tokens, refuses redirects, bounds
response size and time, and hides raw responses and transport errors. Output is
sanitized; a successful account check does not enable checkout. Tests use a fake
HTTP transport and do not contact Square. The selected UK sandbox account check has now passed; the full authentication
evidence matrix remains incomplete.

## Measured sandbox probes — 12 October 2026

Environment: selected active UK/GBP sandbox location, Web Payments SDK observed
iframe version **1.86.0**, Payments API **2026-09-16**. A disposable local probe
used official synthetic fixtures; no real cards, customer data or live endpoint.
Normal Selloovy checkout was not involved and remains unable to submit payments.

| Probe | Observed result | What it establishes |
| --- | --- | --- |
| Read-only account access | Selected active UK/GBP card-processing location passed. | Sandbox credentials/location access only. |
| Documented challenge fixture with verification details | Challenge displayed; published synthetic code completed; tokenization OK. | Browser challenge path works in this sandbox. No payment submitted for this case. |
| Documented frictionless fixture with verification details | Tokenization OK without visible challenge. | Fixture path completes; not proof of universal 3DS enforcement or trusted pre-authorization evidence. |
| Cancelled challenge | Tokenization failed; no replacement token saved and no payment submitted. | Cancelled browser path did not produce a token in this probe. |
| Documented failed-verification fixture | Tokenization OK, then Payments API HTTP 402 / CARD_DECLINED_VERIFICATION_REQUIRED with payment FAILED. | A browser OK result must not be treated as authenticated/paid. The required-verification failure was rejected by the provider. |
| Ordinary synthetic card tokenized without verification details | Tokenization OK; direct Payments API request with customer_initiated=true, seller_keyed_in=false and autocomplete=false returned HTTP 200 / APPROVED. | Current sandbox configuration accepts this unverified browser path; strict all-card enforcement is not demonstrated. |

The simulated APPROVED authorization was immediately cancelled and a provider GET
confirmed CANCELED. No completion/capture was requested. Both final provider records contain an authorization timestamp and no capture
timestamp, including the FAILED record. The failed record therefore does not
prove rejection before authorization; Square must clarify the timeline semantics.
Private records retain sanitized outcome metadata and stable probe keys; opaque
source tokens were removed.
No refund-after-authorization mechanism was added to Selloovy. Cancellation here
was cleanup of an isolated negative test, not a proposed fraud-control substitute.

The localhost origin initialized successfully; the 127.0.0.1 origin was rejected
by the SDK as requiring a secure context. This observation concerns this probe and
version; it is not evidence that ordinary HTTP deployment is acceptable.

Fixture definitions: [Square sandbox payments](https://developer.squareup.com/docs/devtools/sandbox/payments).
Provider request semantics: [CreatePayment](https://developer.squareup.com/reference/square/payments-api/create-payment).
These sandbox outcomes do not establish live behavior. Unsupported/unavailable
3DS, tampered/replayed evidence, changed amount, account-policy changes, durable
retries and complete live enforcement remain open. No claim that all Square
accounts cannot support the policy is made.

**Decision needed:** retain the strict policy and obtain a documented enforceable
UK account mechanism from Square; explicitly adopt Square's standard mandate/risk
policy with additional abuse controls; or evaluate another provider against the
strict policy. No policy change, support message or provider switch has occurred.
