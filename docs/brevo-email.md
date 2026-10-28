# Brevo transactional email boundary

Brevo is the selected service. The provider adapter, read-only account check and durable notification worker are available. External delivery requires explicit operator activation; it is disabled when email configuration is absent. Paid orders queue customer/owner jobs; without explicit Brevo worker configuration they remain queued. No new delivery promises, recovery emails, dispatch emails or password-recovery flow are introduced here.

## Private setup

Use an existing Brevo **API key**, not an SMTP key. Save the key in an ignored owner-only regular file, for example `local/brevo.key` with mode 0600. Symlinks, public permissions, empty keys, control characters and oversized files are rejected. Keep credentials and real sender/customer details outside Git.

Set `SELLOOVY_BREVO_API_KEY_FILE` to the private file path and `SELLOOVY_BREVO_SENDER_EMAIL` to an existing verified sender. Run `go run ./cmd/brevo-check`. This performs one GET to the fixed Brevo `/v3/senders` endpoint and checks the selected address is active. It prints success or a fixed diagnostic for blocked IP access, rejected API access, an unregistered sender or an inactive sender, without sender lists, credentials or raw responses. For blocked IP access, review the recent request in Brevo Settings > Security > Authorized IPs, then retry. It never creates or edits a sender and never sends email. This is account-access evidence, not proof of authenticated sending domains or inbox delivery. The checker uses these two settings. Worker activation additionally requires `SELLOOVY_BREVO_MODE=drop` or `deliver`; partial settings and unknown modes are rejected. Deliver also requires `SELLOOVY_BREVO_DELIVERY_AFTER`, an explicit RFC3339 timestamp saved once at activation. Both the paid capture and the queued job must be at or after this boundary. Keep the value stable across restarts; moving it backwards can expose historical jobs. Drop mode rejects an activation timestamp. Optional `SELLOOVY_BREVO_SENDER_NAME` sets the display name (default Selloovy). Leave all email settings unset to keep processing disabled.

## API and delivery semantics

The adapter calls only `https://api.brevo.com/v3`, with API-key authentication, a 15-second timeout, bounded responses and redirects disabled. Each message has exactly one recipient, a subject, HTML and plain text, and a caller-supplied persisted UUIDv4 in the JSON `headers.IdempotencyKey` field. It performs exactly one POST and never automatically retries. Implicit request/result/client formatting and JSON do not expose keys, recipients, content or provider message IDs. Raw provider errors and bodies never become application errors.

Callers explicitly choose `drop` or `deliver`. Drop adds `headers.X-Sib-Sandbox=drop`; a valid 201/messageId becomes **validated**, never accepted/delivered. Deliver rejects reserved example/test recipients before network access. A valid 201/messageId becomes **accepted**, meaning provider acceptance only. A structured explicit 4xx rejection can become **rejected**; `duplicate_parameter`, transport interruption, malformed/oversized response, 408 and server errors remain **unknown**. Unknown does not authorize a fresh-key resend.

Brevo's current batch-idempotency guide specifies a 30-minute window, while its historical changelog specified 15 minutes. Neither is permanent duplicate protection. Selloovy must persist each job/key and outcome, enforce its own submission/lease state, and hold ambiguous submissions for reconciliation rather than repeatedly sending after the provider window. Sender/domain checks and actual delivery evidence remain necessary before activation.

## Controlled activation

The worker supports drop validation and explicitly enabled delivery for this Square-sandbox POC. It verifies the selected active sender at startup before processing jobs. Never set credentials solely to probe an account on a worker: use the read-only checker first. An explicitly configured drop worker can validate existing queued jobs, but cannot deliver them. Validated jobs are terminal test evidence and will not later turn into real emails automatically.

Migration 19 adds a private immutable rendered payload, random UUIDv4 submission key, start/finish timestamps and provider message ID. At most five jobs are claimed per cycle, with row locks and `SKIP LOCKED`. Claims are committed before provider access. Customer emails use the immutable paid snapshot; owner notification addresses are selected and frozen when claimed. HTML escapes merchant/customer text; each message has a plain-text alternative, preserved totals, certificate names, shipping choice and conspicuous sandbox copy. No public receipt/session tokens appear in emails.

Valid drop responses become `validated`; explicit rejections become `failed`; ambiguous outcomes become `unknown`. A sending job older than two minutes becomes unknown without another POST. Terminal states and envelopes cannot be rewritten/requeued. Observations are saved using a short independent context even during shutdown. No automatic retry is performed for failed/unknown jobs. The owner orders list has a separate all-shop email-issues count and filter for failed/unknown requests and sending requests older than two minutes. Operator reconciliation/retry controls remain future work. Malformed historical snapshots stop processing with a generic warning and require private operator repair. Payment and inventory remain untouched. `sent` means API acceptance only. It is not a promise of inbox placement. Unknown or failed submissions remain held for private operator review in Brevo; there is no resend control or automatic replay.

Before delivery activation: verify the sender read-only, prove drop validation with disposable fixtures, authenticate the sending domain without weakening existing DMARC, and obtain explicit permission for a test recipient. Check a single test message in the provider events and receiving mailbox, including authentication results. Persist an activation timestamp before starting delivery; never auto-generate it at each restart and never clear or rewrite historical jobs. The worker keeps rejected/unknown submissions terminal and does not resend them. A provider outage can leave jobs unknown, requiring manual review; automated reconciliation and delivery-event ingestion remain future work. Do not silently send historical sandbox jobs.

The foreground development runner loads optional ignored `local/brevo.env` after the database and Square files. This is trusted operator shell configuration, with owner-only permissions; never source merchant uploads or third-party files. Store only file references and activation settings there, keeping the API key in its separate protected file. Stopping/restarting the runner retains the activation boundary and persisted job outcomes. No database migration is required for this activation change.

All outgoing confirmations still prominently identify Square sandbox orders and say that no real payment was taken. Brevo deliver mode sends real email; it does not enable live Square payments. Email status appears separately from payment/fulfilment status. Reserved example recipients are rejected by the deliver adapter before any network request. Recovery, dispatch and password-reset emails are not enabled by this setting.

Abandoned-basket recovery emails and dispatch emails remain later work. Webhooks/delivery-event verification are required before claiming inbox delivery. Domain purchase and live Square activation remain separate gates.

## Evidence

Mock tests cover request encoding, the fixed destination, drop mode, sender matching, secret-file guards, privacy, input validation, ambiguous outcomes, reserved delivery recipients and redirect rejection. Disposable PostgreSQL tests also cover concurrent claims, commit-before-submission, immutable envelopes, crash/uncertain no-resend, owner-visible status and financial isolation. Disposable worker tests cover activation boundaries, late-created jobs for historical captures, new paid orders and restart no-replay. A separately authorized synthetic adapter test reached a real test mailbox with SPF, aligned DKIM and DMARC passing. The durable worker was also proven against real Brevo drop validation using disposable data. These checks do not establish universal inbox placement or live payment readiness.

Primary sources reviewed 13 October 2026:

- [Send a transactional email](https://developers.brevo.com/reference/send-transac-email).
- [Get email senders](https://developers.brevo.com/reference/get-senders).
- [Using sandbox mode](https://developers.brevo.com/docs/using-sandbox-mode).
- [Current idempotency guide](https://developers.brevo.com/docs/heterogenous-versions-batch-emails).
- [Historical idempotency changelog](https://developers.brevo.com/changelog/2021/11/10).
