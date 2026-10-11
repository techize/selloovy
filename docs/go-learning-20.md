# Go lesson 20: one transaction, one agreed intent

PreparePayment composes existing checkout locking and allocation with a new
immutable snapshot. The generated sqlc methods share the same pgx transaction.
If the final insert fails, deferred cleanup rolls everything back: no snapshot,
no deadline extension and no stranded physical allocation.

An idempotency key identifies one agreed operation. A retry after process restart
loads the stored key instead of generating another. Cancellation or an expired
pre-authorization window ends that intent; a fresh start creates a new key. This
rule must change once a financial request may have reached a provider: an unknown
outcome needs reconciliation with the original key, never a fresh charge.

The checkout row lock serializes duplicate starts. Ordered variant locks protect
competition for stock across different baskets. A partial unique index supplies
an additional database rule: one prepared intent per checkout. PostgreSQL checks
and a trigger protect snapshot immutability and terminal transitions.

Optional exercise: follow the forced-insert-failure test. Identify every write
that rolls back and explain why returning a conflict after a provider timeout
would be insufficient in the future financial lifecycle.
