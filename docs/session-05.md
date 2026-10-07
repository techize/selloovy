# Persistent authentication increment 05

Implemented: schema version 2 for owners, encrypted MFA enrollment, recovery
digests, expiring password-proven challenges and digest-only sessions. sqlc v1.31.1
generates pgx query methods; CI verifies regeneration. Store routines confirm
MFA enrollment, start/finish login, validate/touch sessions and log out.

Integration evidence: independent stores/pools cannot concurrently reuse a TOTP
step or recovery code; failed session insertion rolls factor/challenge changes
back; password alone/pending enrollment cannot create a session. Checks cover
five-attempt challenges, expiry, token purpose separation, reconnect persistence,
shared logout, idle/absolute limits, credential-version invalidation and redacted
storage failure. These use disposable databases and synthetic credentials.

HTTP sign-in, enrollment UI/CLI, session cookies, CSRF/origin checks, shared
account/IP throttling, credential recovery/reset delivery and cleanup remain open.
No real owner or operational encryption key is created. Admin stays a preview.
Next wire the operator-controlled setup and browser flow with those HTTP controls.
Go lesson 05 explains generated queries, row locking and transaction rollback.
