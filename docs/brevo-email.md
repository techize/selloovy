# Brevo transactional email boundary

Brevo is the selected service. The provider adapter, read-only account check and durable notification worker are available. External delivery is **not enabled**. Paid orders queue customer/owner jobs; without explicit Brevo worker configuration they remain queued. No new delivery promises, recovery emails, dispatch emails or password-recovery flow are introduced here.

## Private setup

Use an existing Brevo **API key**, not an SMTP key. Save the key in an ignored owner-only regular file, for example `local/brevo.key` with mode 0600. Symlinks, public permissions, empty keys, control characters and oversized files are rejected. Keep credentials and real sender/customer details outside Git.

Set `SELLOOVY_BREVO_API_KEY_FILE` to the private file path and `SELLOOVY_BREVO_SENDER_EMAIL` to an existing verified sender. Run `go run ./cmd/brevo-check`. This performs one GET to the fixed Brevo `/v3/senders` endpoint and checks the selected address is active. It prints success or a fixed diagnostic for blocked IP access, rejected API access, an unregistered sender or an inactive sender, without sender lists, credentials or raw responses. For blocked IP access, review the recent request in Brevo Settings > Security > Authorized IPs, then retry. It never creates or edits a sender and never sends email. This is account-access evidence, not proof of authenticated sending domains or inbox delivery. The checker uses these two settings. Worker activation additionally requires `SELLOOVY_BREVO_MODE=drop`; partial settings and actual delivery mode are rejected by runtime configuration. Leave all three unset to keep email processing disabled.

## API and delivery semantics

The adapter calls only `https://api.brevo.com/v3`, with API-key authentication, a 15-second timeout, bounded responses and redirects disabled. Each message has exactly one recipient, a subject, HTML and plain text, and a caller-supplied persisted UUIDv4 in the JSON `headers.IdempotencyKey` field. It performs exactly one POST and never automatically retries. Implicit request/result/client formatting and JSON do not expose keys, recipients, content or provider message IDs. Raw provider errors and bodies never become application errors.

Callers explicitly choose `drop` or `deliver`. Drop adds `headers.X-Sib-Sandbox=drop`; a valid 201/messageId becomes **validated**, never accepted/delivered. Deliver rejects reserved example/test recipients before network access. A valid 201/messageId becomes **accepted**, meaning provider acceptance only. A structured explicit 4xx rejection can become **rejected**; `duplicate_parameter`, transport interruption, malformed/oversized response, 408 and server errors remain **unknown**. Unknown does not authorize a fresh-key resend.

Brevo's current batch-idempotency guide specifies a 30-minute window, while its historical changelog specified 15 minutes. Neither is permanent duplicate protection. Selloovy must persist each job/key and outcome, enforce its own submission/lease state, and hold ambiguous submissions for reconciliation rather than repeatedly sending after the provider window. Sender/domain checks and actual delivery evidence remain necessary before activation.

## Next activation gate

The worker is deliberately limited to drop validation for this Square-sandbox POC. It verifies the selected active sender at startup before processing jobs. Never set credentials solely to probe an account on a worker: use the read-only checker first. An explicitly configured drop worker can validate existing queued jobs, but cannot deliver them. Validated jobs are terminal test evidence and will not later turn into real emails automatically.

Migration 19 adds a private immutable rendered payload, random UUIDv4 submission key, start/finish timestamps and provider message ID. At most five jobs are claimed per cycle, with row locks and `SKIP LOCKED`. Claims are committed before provider access. Customer emails use the immutable paid snapshot; owner notification addresses are selected and frozen when claimed. HTML escapes merchant/customer text; each message has a plain-text alternative, preserved totals, certificate names, shipping choice and conspicuous sandbox copy. No public receipt/session tokens appear in emails.

Valid drop responses become `validated`; explicit rejections become `failed`; ambiguous outcomes become `unknown`. A sending job older than two minutes becomes unknown without another POST. Terminal states and envelopes cannot be rewritten/requeued. Observations are saved using a short independent context even during shutdown. No automatic retry is performed for failed/unknown jobs. The owner orders list has a separate all-shop email-issues count and filter for failed/unknown requests and sending requests older than two minutes. Operator reconciliation/retry controls remain future work. Malformed historical snapshots stop processing with a generic warning and require private operator repair. Payment and inventory remain untouched. `sent` means API acceptance only and is currently exercised solely with fake providers, since the configured worker rejects actual delivery mode.

After account access is available: verify the sender read-only and prove drop validation. Actual transactional delivery requires a separate explicit activation change, a selected real test recipient, sender-domain authentication checks, delivery evidence, and reconciliation/retry tooling for ambiguous submissions. Do not silently send historical sandbox jobs.

Abandoned-basket recovery emails and dispatch emails remain later work. Webhooks/delivery-event verification are required before claiming inbox delivery. Domain purchase and live Square activation remain separate gates.

## Evidence

Mock tests cover request encoding, the fixed destination, drop mode, sender matching, secret-file guards, privacy, input validation, ambiguous outcomes, reserved delivery recipients and redirect rejection. Disposable PostgreSQL tests also cover concurrent claims, commit-before-submission, immutable envelopes, crash/uncertain no-resend, owner-visible status and financial isolation. No real email was sent during implementation.

Primary sources reviewed 13 October 2026:

- [Send a transactional email](https://developers.brevo.com/reference/send-transac-email).
- [Get email senders](https://developers.brevo.com/reference/get-senders).
- [Using sandbox mode](https://developers.brevo.com/docs/using-sandbox-mode).
- [Current idempotency guide](https://developers.brevo.com/docs/heterogenous-versions-batch-emails).
- [Historical idempotency changelog](https://developers.brevo.com/changelog/2021/11/10).
