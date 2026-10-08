# Go lesson 07: optional MFA is a state transition

An owner can now use password login while MFA is off. Starting QR setup does not
change that. Only a confirmed authenticator code enables MFA; future sign-ins then
require a factor. The UI recommends that transition without granting permissions.

`LoginResult` carries a token and `MFARequired` flag. The HTTP handler sets either
a challenge cookie or a session cookie from that server-owned result. The browser
never supplies the flag. The challenge-only `StartLogin` helper remains separate
for internal factor-required flows; the browser calls `Login`.

The important check happens under the owner row lock. Password verification can
take time outside the transaction, so the store rechecks the password hash and
credential version before creating a session. A simultaneous MFA activation
cannot leave a newly issued password-only session valid after the transition.

Browser enrollment records the session digest and a ten-minute expiry alongside
the encrypted seed. Confirmation checks both after locking the owner. This is
similar to a Rails transaction using a locked model, but Go makes each query and
error return explicit. A QR scanned in one browser cannot be confirmed through a
different session. An expired setup must start again.

Activation stores recovery-code digests and increments `auth_version` together.
Existing sessions still carry the previous version; the session query rejects
them. Recovery codes are shown once, and the owner signs in again with the new
factor. Tests run simultaneous confirmation calls and require exactly one winner.

The Vue reminder is derived from the authenticated status response. It persists
across admin sections, returns when unconfirmed setup is cancelled, and disappears
when the server reports MFA enabled. Hiding the reminder manually would change
only presentation, not the backend's enrolled-factor enforcement.

Optional exercise: follow one MFA-off login and one MFA-enabled login through
`beginLogin`. Identify where the token's stored purpose changes, then explain why
changing a frontend boolean cannot bypass an enabled factor.
