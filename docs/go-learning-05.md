# Go lesson 05: transactions and generated queries

SQL source in db/queries/auth.sql declares names and result modes. sqlc generates
Go parameter/result structs and methods in internal/authdb. Go compilation checks
how we use those methods; database integration tests still need to check real SQL
behavior. Generated code does not prove authorization or transaction correctness.

The store begins a pgx transaction and defers rollback with a short independent
cleanup context. A successful Commit makes the deferred rollback harmless. Early
returns discard all uncommitted changes, including a deleted recovery code.

FinishLogin locks the owner before the challenge. That consistent order serializes
factor use across replicas. The second request sees the updated consumed counter
or missing recovery digest and cannot create another session with it. Password
verification alone creates only an expiring challenge.

The rollback test deliberately makes session insertion fail after factor validation.
Removing that test-only constraint lets the same challenge/factor succeed, proving
the factor was restored by rollback. An uncertain network failure during Commit is
different: the database may have committed, so an error is not proof of rollback.

Optional exercise: trace FinishLogin's success and failure paths. Explain why the
owner lock must stay held through session creation and why querying database time
after locking matters when a request waits. Learning does not gate delivery.
