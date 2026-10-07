# Go lesson 06: a browser request earns a session

Follow a sign-in through `internal/authhttp/http.go`: the guard validates the
request origin and bounds its lifetime, the handler decodes a small JSON object,
and the authentication store decides whether the credentials prove anything.
Password success grants only a challenge cookie. Factor success commits a session;
only then can a protected handler run.

The `Backend` interface describes the few operations HTTP needs. Go types satisfy
interfaces by implementing their methods; there is no `implements` declaration.
The PostgreSQL store and test fake both fit. Unlike a Rails controller with
implicit model access, this boundary makes the dependency explicit in `New`.

Middleware takes an `http.Handler` and returns another handler. Its closure keeps
the configured host/backend and runs checks before calling `next.ServeHTTP`.
`requireOwner` queries the database on every protected request, so a frontend
boolean or stale cookie cannot become authorization. Session validation will also
need owner/shop scoping when we add actual merchant APIs.

`context.WithTimeout` derives a request context; `defer cancel()` releases its
resources when the handler returns. Database calls inherit that deadline. Errors
are mapped with `errors.Is` into safe HTTP responses; driver errors and credentials
stay out of JSON. Context cancellation can leave a commit outcome uncertain, so
we avoid automatic factor retries.

The cookie holds a random token; PostgreSQL stores its purpose-bound digest.
HttpOnly prevents ordinary JavaScript access, while Secure and SameSite constrain
transport and cross-site use. These flags work alongside explicit origin checks;
none replaces server-side session lookup or one-use transaction rules.

The setup command uses `golang.org/x/term` to read a password without echoing it.
It accepts no command-line credentials and requires a terminal. File creation
uses `O_EXCL`: an existing installation key cannot be overwritten accidentally.
The loader checks permissions, file type, identity and length before using a key.

Optional exercise: trace the `TestBrowserOwnerAuthentication` request that supplies
only a challenge cookie to `/workspace`. Identify the check that prevents access,
then explain why hiding a Vue navigation button would not enforce that rule.
Delivery continues without requiring the exercise.
