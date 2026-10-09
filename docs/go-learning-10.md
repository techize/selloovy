# Go lesson 10: save stock and its history together

VariantInput describes what the owner may change. Variant adds derived availability
and effective price for display. Embedding VariantInput lets Go reuse those fields
without returning a generated database model. MakerInput carries all variants plus
the product revision; MakerSettings is the read model.

A nullable price uses *int64: nil means inherit, while a pointer means an explicit
amount in pence. sqlc uses pgtype.Int8 with Valid for the PostgreSQL NULL boundary.
A zero price is invalid rather than another way to say inherit.

validateMaker copies the slice before trimming labels, so validation cannot mutate
another caller’s in-memory draft. A map checks repeated IDs and normalized labels.
Existing IDs must belong to this product; leaving an existing row out is rejected.
This prevents a form omission from becoming an accidental stock deletion.

The read transaction uses RepeatableRead. Parent revision, fallback flag and child
variants come from the same database snapshot. Otherwise a save between two SELECTs
could give the browser an old revision with new stock counts.

The write transaction first locks the verified session/owner, compares the product
revision and advances it. Every variant update and changed-stock history row happens
before commit. If history insertion fails, defer rolls the transaction back: the
count, fallback and revision all stay as they were. A lost response can still have
an uncertain result, so the browser reloads instead of repeating a stock snapshot.

Availability is a pure Go function. Stocked with positive stock gets the two-day
estimate. Zero stock uses the product fallback. Always made to order gets 5–7 days.
These are working-day durations; date arithmetic and shipping services come later.

Optional exercise: trace the induced history-insert failure test. List the changes
that must roll back, then explain why a stale product-description form also conflicts
after a variant save. Both screens protect the same product revision.
