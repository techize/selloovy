# Browser owner authentication increment 06

Implemented: explicit interactive first-owner setup and MFA confirmation; private
installation-key generation/loading; browser password then authenticator/recovery
login and logout; protected owner-role endpoint; origin/CSRF/cookie/request controls;
shared keyed-digest attempt counters in schema version 3. Vue includes responsive
login/factor forms and signed-in/logout states; the workspace remains a preview.

Verification: full local Go race suite with disposable PostgreSQL, vet/build/module
checks, sqlc regeneration, frontend type/build checks and browser QA using disposable
synthetic credentials. Browser checks cover wrong password/factor, recovery login,
logout and mobile/desktop layout. HTTP/database tests prove password-only denial,
one-use factors/challenges, session rotation and cancellation of pending login,
closed access during database outage, origin/header/body boundaries and cookie flags.
Publication scans and remote CI must pass before merge.

No real owner, provider account, payment, email or live deployment is configured.
Local browser fixtures stay ignored and are removed after QA. Operators create their
own credentials privately in a terminal. Password reset, full-factor-loss recovery,
key rotation/restore, cleanup and trusted ingress controls remain live-use gates.

Next: authenticated shop settings, with server-side owner/shop scoping, validation
and persistence. Then catalogue and section builder toward the maker/test-order POC.
Square strict 3DS proof remains open. Go lesson 06 explains interfaces, middleware,
contexts, cookies and exclusive key-file creation through this feature.
