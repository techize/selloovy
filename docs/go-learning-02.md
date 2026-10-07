# Go lesson 02: persistence and connection ownership

Follow cmd/selloovy/main.go into internal/database/database.go. The application opens one pool, passes a readiness function to the web package, and closes the pool after HTTP shutdown.

**Pointers and ownership.** `*pgxpool.Pool` is a pointer to a pool managed by the application. Multiple requests reuse it; none owns closing it. `defer pool.Close()` belongs in startup so resources stay available until the server has drained its requests.

**A closure connects modules.** `ready = func(ctx context.Context) error { return database.Ready(ctx, pool) }` captures the pool. The web package still knows only a function accepting a context and returning an error; it does not need to own a database connection.

**Context limits database work.** Readiness uses the request's bounded context. When its deadline expires, pgx can cancel database work. A connection timeout also prevents indefinite connection establishment. An outage therefore changes readiness instead of silently claiming the shop is healthy.

**Parameters keep data separate from SQL.** Integration tests insert a synthetic name with `VALUES ($1)` and pass the name as a separate argument. Do the same for merchant/customer values in later features. Do not build SQL by concatenating those values. The integration test quotes its generated database identifier with `pgx.Identifier`, because identifiers cannot use ordinary value parameters.

**Schema and data are different.** The embedded migration defines the shops table. The migration command applies numbered schema changes explicitly; starting the server does not apply them. Tern provides migration locking and transactions. Readiness checks the version the application expects.

Optional exercise: trace the difference between an absent database URL, a blank URL, an unreachable database and a schema newer than the application. Which cases stop startup, and which produce HTTP 503? Run `go test ./internal/database` without the test URL, then with the dedicated test environment and notice the integration test's skip/pass distinction.

Primary references: [pgx source](https://github.com/jackc/pgx/tree/v5.11.0), [Tern migrations](https://github.com/jackc/tern/tree/v2.4.3), and [Go contexts](https://pkg.go.dev/context).
