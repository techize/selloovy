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

This library is not connected to checkout or database storage yet. A durable
attempt must persist the encrypted source and immutable authenticated context
before calling Square, enforce token retention/deletion and keep unknown financial
outcomes blocked for reconciliation. Terminal attempts must erase the source;
expiry must not be mistaken for proof that no earlier charge occurred.

Back up the installation key separately and restore the same key with the database.
Rotation/versioning and the GCM per-key message limit require an operating procedure
before live use; changing the existing key would also affect owner MFA.
Tests verify distinct nonce outputs, restoration, tampering, context/key isolation,
size/source rejection and implicit privacy. No new key file is generated here.

References: [Go HKDF](https://pkg.go.dev/crypto/hkdf) and
[Go random-nonce GCM](https://pkg.go.dev/crypto/cipher#NewGCMWithRandomNonce).
