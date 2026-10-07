# Go lesson 04: credentials, values and ownership

`internal/auth` is a small package with no HTTP or database imports. That keeps
credential verification separate from the decision to issue a session. A valid
password hash comparison does not mean an owner is signed in; the later service
must also verify MFA and commit its one-use state.

**Structs can hide sensitive fields.** `MFASecret` and `RecoveryCode` keep their
fields unexported and implement `fmt.Formatter` to redact common debug printing.
Explicit `EnrollmentKey`/`Reveal` calls expose sensitive values for enrollment.
This reduces accidental leakage; it cannot stop a caller logging revealed text.

**A channel can bound expensive work.** One shared `PasswordHasher` uses a channel
with capacity two. An operation puts a value in the channel to take a slot and
uses `defer` to release it on every return. A nonblocking `select` returns busy
when both slots are occupied. Context cancellation is checked before and after
Argon2; it cannot interrupt the library's computation mid-call.

**Return evidence for the transaction.** `MatchTOTP` returns the accepted time
counter. The future database service must update the last-used counter atomically
with session issuance. Two requests reading the same old value can both pass the
helper, so the SQL transaction must decide which one succeeds. This resembles a
Rails transaction with a row lock rather than a model validation alone.

Optional exercise: trace the `defer` in `Hash`. Then explain why storing a recovery
digest and comparing it is insufficient for one-use recovery. No exercise gates
the next feature.
