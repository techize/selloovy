# Optional owner MFA increment 07

Supersedes the earlier mandatory-MFA default: MFA is optional and recommended.
CLI setup defaults to skipping it; MFA-off owners can sign in with their password.
Future public web signup should offer the recommended QR flow; public registration
is not implemented. Admin has a persistent recommendation until activation.

Implemented: current-password re-verification, locally encoded private QR/manual
key setup, session-bound ten-minute enrollment, code confirmation and one-time
recovery display. Activating MFA increments the credential version, invalidates
older sessions/challenges and enforces a factor at future sign-ins. Enabled factors
cannot be bypassed or replaced through the initial setup endpoints.

Schema version 4 adds nullable browser-enrollment metadata without changing
existing enabled flags. Existing unfinished/MFA-off owners can use password login;
existing enabled owners retain their factor requirement. Applied migrations are
preserved. go-qrcode is pinned with its MIT notice; QR content stays in the app.

Verification: local Go race tests, disposable PostgreSQL, module/vet/build and Vue
checks. HTTP integration covers optional login, private QR PNG delivery, fresh
password proof, wrong-session/expiry/code rejection, revocation and enforced MFA
with recovery after activation. Concurrent activation has exactly one winner.
Browser QA covers password-only login, persistent reminder, QR/manual-key view,
wrong-code feedback, cancel and enabled-account factor prompt/reminder removal;
desktop/mobile layout reviewed using disposable synthetic accounts. Activation
and recovery response checks are exercised by the HTTP/database tests.

Public scans and remote CI must pass before merge. No real owner credentials,
provider action, payment/email or live deployment is involved. Reset/factor-loss
recovery, factor/code regeneration, key lifecycle/restore, cleanup and ingress
controls remain live-use gates. Next: authenticated shop settings.
