# Go lesson 09: a product save that survives a lost response

The catalogue has three layers: catalog contains rules and transactions,
cataloghttp translates HTTP, and catalogdb is generated from reviewed SQL. Vue
sends a deliberate Input struct and receives Product, rather than database models.

Prices use int64 pence. £19.99 is 1999. The browser splits the decimal text into
whole pounds and fractional pence before sending an integer; Go validates its range.
Avoid storing currency in float64, where decimal amounts may not be exact.

Creating something differs from editing it. If an edit response is lost, reload
its revision. If a create response is lost, simply creating again could duplicate
the product. The browser assigns one UUID to the operation. Go normalizes the input,
serializes the struct and hashes those bytes with SHA-256. A database uniqueness
constraint scopes that key to the shop. Repeating the same key and hash returns the
saved product; different content conflicts. A transaction and owner lock serialize
racing creates and logout. This is idempotency applied to a concrete workflow.

An existing product uses optimistic concurrency: both clients send revision 1;
one transaction writes revision 2, and the next returns conflict. The owner lock
also establishes authorization while the product query limits access to that shop.
SQL checks credential version and expiry again instead of trusting only middleware.

Go converts generated query rows into a small public Product struct with explicit
JSON tags. Errors are checked with errors.Is/errors.As and translated to safe HTTP
statuses; raw PostgreSQL errors never reach the browser. defer rolls back unfinished
transactions using a separate bounded cleanup context.

Pagination asks for 51 rows but returns 50: the extra row tells us whether another
page exists. The last returned ID becomes a cursor, avoiding an unbounded catalogue
response. It is not a database snapshot across pages.

Optional exercise: trace the concurrent-create test. Explain why two callers return
one ID, then change only the second price and predict the conflict. Next explain
why another shop cannot read that ID even when it knows its value.
