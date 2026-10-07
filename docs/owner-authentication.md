# Owner authentication delivery

Required: owner email/password plus mandatory authenticator-app MFA. Recovery
codes require the password, replace one authenticator challenge and leave MFA
enabled. Password reset must retain MFA; losing all factors requires an
operator-verified, audited process. Those reset/re-enrollment flows remain open.

## Implemented

`internal/auth` persists credentials, factor consumption and sessions through
sqlc-generated pgx queries. `internal/authhttp` serves browser login, verification,
status and logout; Vue reflects these outcomes. No default credentials or public
registration exist. [Local setup](development.md#owner-setup-and-browser-sign-in)
uses an interactive operator command, private key file and confirmed authenticator.
An interrupted pending enrollment can resume with its password through that
local command; enrolled owners cannot use it to reset MFA.

- Argon2id: 64 MiB, three iterations, one lane, random 16-byte salt and 32-byte
  output. Strict PHC parsing accepts only the recorded parameters. A shared
  hasher permits two expensive operations per process, rejecting excess work.
  Passwords preserve 8–128 Unicode characters including spaces, prohibit control
  characters and are neither normalized nor truncated.
- Authenticator: random 160-bit seed, HMAC-SHA1, six digits and 30-second steps,
  with one step of clock tolerance. Successful counters cannot be reused.
  AES-256-GCM stores the seed encrypted with owner/purpose/version authenticated
  context. Its external 32-byte installation key is separate from the database.
- Ten random 128-bit recovery codes: only owner/purpose-bound SHA-256 digests are
  persisted. Setup displays them once. Explicit reveal methods are sensitive;
  formatting/JSON do not implicitly expose seed, recovery or token values.
- Version 2 stores owners, recovery digests, challenges and sessions. Pending
  MFA enrollment cannot log in. Confirmation consumes its authenticator step.
  Password success creates a five-minute challenge with at most five failed
  factors. A valid password alone grants no protected access.
- Owner row locks serialize MFA/recovery consumption across independent pools.
  Factor consumption, challenge consumption and session creation commit together;
  induced session-insert failure rolls all three back. A lost commit response
  may have an uncertain outcome: the browser starts again rather than retrying
  consumed factors automatically.
- Purpose-bound digests of random 256-bit session tokens: eight-hour absolute,
  thirty-minute idle expiry and credential-version invalidation. Protected
  requests check server-side validity. Starting a new login revokes the browser's
  old session; logout deletes its session and presented unfinished challenge.
- Cookies are HttpOnly, SameSite=Strict, Path=/, no Domain; HTTPS uses Secure and
  the `__Host-` prefix. HTTP is allowed only on an explicit loopback IP with a
  loopback listener. No tokens are returned in JSON, URLs or browser storage.
- Exact configured Host on auth requests; state changes also require exact Origin,
  a custom request header, JSON and non-cross-site Fetch Metadata. No CORS
  permission is emitted. Request bodies are limited to 4 KiB, reject unknown/
  trailing JSON, and handlers use five-second contexts. Responses are uncached
  with generic credential errors, redacted storage failures and unavailable/limit
  statuses. The admin document includes a same-origin CSP and blocks framing.
- Version 3 adds atomic PostgreSQL counters shared across replicas. Fifteen-minute
  windows permit ten password attempts per normalized email, ten factor attempts
  per owner and sixty login/verification requests per socket IP per endpoint.
  Successes count too. Enrollment resume has its own ten-attempt limit. Bucket
  identifiers are keyed HMAC digests, never plaintext IPs/emails. Forwarded
  headers cannot supply the limiting address. Multiple users behind an ingress
  therefore share its socket-IP budget until trusted proxy handling is delivered.

These limits are documented implementation defaults and need deployment testing.
The configured installation key must be the same across replicas. First-owner
creation locks the owner table and atomically creates its shop and identity;
concurrent setup cannot create a second owner or leave a stray shop.

## Verification and remaining gates

Unit and disposable PostgreSQL tests cover password/resource/redaction rules,
RFC vectors, ciphertext tampering/binding, migration preservation, concurrent
factor consumption, rollback, replay/expiry, shared logout, version revocation,
rate windows, bootstrap/resume and key-file boundaries. HTTP integration covers
password-only denial, MFA/recovery access, code/challenge reuse, session rotation,
pending logout and database outage. HTTP boundary tests cover CSRF/origin/header,
malformed bodies, duplicate cookies and generic errors. Browser QA uses disposable
synthetic credentials for wrong password/factor, recovery login and logout, with
desktop/mobile layouts. This is verification of the stated scope, not a security
certification or completed commerce POC.

Before live use: breached/common-password checking and cost calibration; password
reset delivery; audited full-factor-loss recovery and factor re-enrollment;
recovery-code regeneration; key rotation/independent restore; trusted ingress/IP
handling and edge abuse controls; cleanup of expired challenges/sessions/rate rows;
security audit and operational alerting. The worker does not yet run auth cleanup.
Stored expired digests currently remain until maintenance is implemented, though
expiry and one-use checks still prevent authentication. No insecure reset shortcut
or unauthenticated factor validator is exposed. Email delivery remains unconfigured.

Merchant editing APIs must authorize the session's owner/shop on every operation;
the preview and protected role endpoint contain no merchant data. Installation
operators already hold database/key access; the local setup command does not
establish authorization for multi-shop cloud provisioning. That future workflow
needs a separate verified design. Credentials never belong in logs, ordinary
exports, API errors, analytics or public Git history.

## Research basis

Checked 7 October 2026: [OWASP password storage](https://cheatsheetseries.owasp.org/cheatsheets/Password_Storage_Cheat_Sheet.html),
[Go Argon2](https://pkg.go.dev/golang.org/x/crypto/argon2),
[RFC 6238](https://www.rfc-editor.org/rfc/rfc6238.html),
[OWASP MFA](https://cheatsheetseries.owasp.org/cheatsheets/Multifactor_Authentication_Cheat_Sheet.html),
[password reset](https://cheatsheetseries.owasp.org/cheatsheets/Forgot_Password_Cheat_Sheet.html),
[authentication](https://cheatsheetseries.owasp.org/cheatsheets/Authentication_Cheat_Sheet.html),
[CSRF prevention](https://cheatsheetseries.owasp.org/cheatsheets/Cross-Site_Request_Forgery_Prevention_Cheat_Sheet.html),
[session management](https://cheatsheetseries.owasp.org/cheatsheets/Session_Management_Cheat_Sheet.html)
and [PostgreSQL row locks](https://www.postgresql.org/docs/17/explicit-locking.html).
Defaults are Selloovy choices informed by these sources, not vendor guarantees.

SQL source is db/queries/auth.sql; pinned sqlc v1.31.1 generates internal/authdb.
CI checks regeneration. Generated credential structs must never be logged or
returned directly; handlers use explicit public response shapes. The SHA1 OTP
implementation follows the algorithm, without copying the RFC Java reference code.
