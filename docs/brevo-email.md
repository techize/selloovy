# Brevo transactional email boundary

Brevo is the selected service. This increment adds the provider adapter and a read-only account check; it does **not** connect the order-notification queue to external delivery. Paid orders still queue customer/owner jobs. No new delivery promises, recovery emails, dispatch emails or password-recovery flow are introduced here.

## Private setup

Use an existing Brevo **API key**, not an SMTP key. Save the key in an ignored owner-only regular file, for example `local/brevo.key` with mode 0600. Symlinks, public permissions, empty keys, control characters and oversized files are rejected. Keep credentials and real sender/customer details outside Git.

Set `SELLOOVY_BREVO_API_KEY_FILE` to the private file path and `SELLOOVY_BREVO_SENDER_EMAIL` to an existing verified sender. Run `go run ./cmd/brevo-check`. This performs one GET to the fixed Brevo `/v3/senders` endpoint and checks the selected address is active. It prints only success/failure, without sender lists, credentials or raw responses. It never creates or edits a sender and never sends email. This is account-access evidence, not proof of authenticated sending domains or inbox delivery. These settings are currently consumed only by this command; they do not enable the HTTP app or worker.

## API and delivery semantics

The adapter calls only `https://api.brevo.com/v3`, with API-key authentication, a 15-second timeout, bounded responses and redirects disabled. Each message has exactly one recipient, a subject, HTML and plain text, and a caller-supplied persisted UUIDv4 in the JSON `headers.IdempotencyKey` field. It performs exactly one POST and never automatically retries. Implicit request/result/client formatting and JSON do not expose keys, recipients, content or provider message IDs. Raw provider errors and bodies never become application errors.

Callers explicitly choose `drop` or `deliver`. Drop adds `headers.X-Sib-Sandbox=drop`; a valid 201/messageId becomes **validated**, never accepted/delivered. Deliver rejects reserved example/test recipients before network access. A valid 201/messageId becomes **accepted**, meaning provider acceptance only. A structured explicit 4xx rejection can become **rejected**; `duplicate_parameter`, transport interruption, malformed/oversized response, 408 and server errors remain **unknown**. Unknown does not authorize a fresh-key resend.

Brevo's current batch-idempotency guide specifies a 30-minute window, while its historical changelog specified 15 minutes. Neither is permanent duplicate protection. Selloovy must persist each job/key and outcome, enforce its own submission/lease state, and hold ambiguous submissions for reconciliation rather than repeatedly sending after the provider window. Sender/domain checks and actual delivery evidence remain necessary before activation.

## Next activation gate

After private account access and the chosen sender are available: verify the sender read-only, then prove an explicitly selected test message in drop mode. Implement durable job claiming, immutable rendered payloads/recipients, bounded handling of explicit rejections, crash/unknown outcomes and owner-visible delivery status. Keep payment/inventory transactions independent of email delivery. Test concurrency and failure recovery with fake transports and a disposable database. Sandbox-only orders need clear test copy and a separately approved real test recipient before any actual delivery check. Do not send existing queued jobs merely because a key file appears.

Abandoned-basket recovery emails and dispatch emails remain later work. Webhooks/delivery-event verification are required before claiming inbox delivery. Domain purchase and live Square activation remain separate gates.

## Evidence

Mock tests cover request encoding, the fixed destination, drop mode, sender matching, secret-file guards, privacy, input validation, ambiguous outcomes, reserved delivery recipients and redirect rejection. No real email was sent during implementation.

Primary sources reviewed 13 October 2026:

- [Send a transactional email](https://developers.brevo.com/reference/send-transac-email).
- [Get email senders](https://developers.brevo.com/reference/get-senders).
- [Using sandbox mode](https://developers.brevo.com/docs/using-sandbox-mode).
- [Current idempotency guide](https://developers.brevo.com/docs/heterogenous-versions-batch-emails).
- [Historical idempotency changelog](https://developers.brevo.com/changelog/2021/11/10).
