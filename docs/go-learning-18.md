# Go lesson 18: review a quote, then allocate stock

Customer and CheckoutSnapshot are Go structs describing validated input and a
server-calculated review. The form supplies a revision and quote hash, not prices.
Hashing JSON with SHA-256 gives a comparison fingerprint; it is not authorization.
The secret cookie and shop-scoped database lookup establish access.

SaveCheckout begins a PostgreSQL transaction and defers rollback. It validates the
current revision/quote, writes customer and snapshot together, then commits. Any
error before commit leaves the previous review and allocations intact. Commit is
the point at which the change becomes visible to other requests.

HoldCheckout uses Read Committed isolation and row locks. A request waiting for a
variant lock sees the prior holder's committed allocation when it resumes. Sorted
locks reduce deadlocks. A map[int64]int64 combines quantities for one variant even
when certificate owner names create separate basket lines. Physical stock and
made-to-order demand are different quantities.

The HTTP handler uses a small CheckoutBackend interface and errors.Is/errors.As
to distinguish conflicts, validation and storage failures. html/template escapes
customer text; successful POST redirects to GET. These choices keep business
rules in the store and browser feedback in the handler.

Optional exercise: trace two customers attempting to hold the last physical item.
Explain why saving details does not reserve it, why one hold fails, and why an
uncertain payment must retain its allocation beyond the normal deadline.
