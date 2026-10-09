# Go lesson 15: basket transactions and money

A basket form supplies identifiers, quantity and a name, not a price. The Go
service resolves published catalogue values and multiplies integer pence by
quantity. For example, 3 × 1,999 pence is exactly 5,997 pence; no floating-point
rounding is involved.

BasketChange describes intent. BasketItem is the stored private choice plus the
last accepted product/price description. BasketLine adds current availability and
change notices for display. Separating these types keeps browser input from
becoming authoritative money or public catalogue data.

ChangeBasket begins a transaction, locks the basket row, checks the submitted
revision, validates the current catalogue and writes a new revision. Deferred
cleanup rolls back on errors; Commit makes the change durable. A stale form or
PostgreSQL serialization conflict requires a fresh read instead of overwriting
another request. Database queries and totals share one snapshot.

Optional exercise: follow a duplicate add through the revision check. Explain why
it cannot add the same quantity twice, and why changing a price in an unpublished
product draft does not change the basket subtotal.
