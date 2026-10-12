# Unresolved payment-token envelope

internal/paymentvault derives a separate AES-256 key from the existing 32-byte
installation key using HKDF-SHA256 and a payment-specific version/purpose label.
It uses AES-GCM with a random nonce. Associated data binds GBP, shop, amount,
attempt ID, stable idempotency key, Square location and saved quote hash.
Wrong installation/context or altered ciphertext cannot decrypt the source.

Only fresh card-nonce text is accepted, within bounded size. Context and vault
implicit formatting/JSON omit private fields. Temporary byte buffers are cleared;
Go strings/key schedules are not claimed to have guaranteed memory erasure.
Callers must not log individual token/context values or ordinary-export ciphertext.

The server-only financial service persists the encrypted source and immutable
authenticated context before calling Square. It erases terminal sources and
provides bounded expiry cleanup. Unknown financial outcomes remain blocked for
reconciliation; expiry is never proof that no earlier charge occurred. Sandbox customer HTTP integration and the recurring cleanup worker are connected;
see square-checkout.md. The worker must run for timely expiry cleanup.

Back up the installation key separately and restore the same key with the database.
Rotation/versioning and the GCM per-key message limit require an operating procedure
before live use; changing the existing key would also affect owner MFA.
Tests verify distinct nonce outputs, restoration, tampering, context/key isolation,
size/source rejection and implicit privacy. No new key file is generated here.

References: [Go HKDF](https://pkg.go.dev/crypto/hkdf) and
[Go random-nonce GCM](https://pkg.go.dev/crypto/cipher#NewGCMWithRandomNonce).

Financial storage now uses this vault in the server-only service described in
[financial-lifecycle.md](financial-lifecycle.md). Sources are cleared on terminal
outcomes and by bounded expiry cleanup after one hour. Sandbox checkout and the cleanup worker are connected; live readiness remains.
