# Go lesson 01: from startup to an HTTP response

Read these files in order: cmd/selloovy/main.go, internal/config/config.go, internal/web/web.go, internal/server/server.go. The entry point wires components together; the packages each own one responsibility.

## Four ideas in this increment

**Explicit errors.** `cfg, err := config.Load(os.LookupEnv)` receives two values. An error is handled at the call site rather than automatically raising an exception. `:=` declares variables while assigning their first values. The configuration loader returns a `Config` struct only after validation succeeds.

**Functions as dependencies.** `Load` accepts a lookup function. Production supplies `os.LookupEnv`; tests supply a tiny function returning a synthetic value. The readiness check is also a function. This avoids changing process-wide environment state or connecting a real database to test routing. A readiness test is not a provider or database capability test.

**Handlers and interfaces.** Chi routes HTTP requests to handlers. A handler receives a response writer and a request. Go's `http.Handler` interface describes the behaviour the server needs; the server need not know which router implements it. This supports the selected modular architecture without introducing separate services.

**Contexts, goroutines and channels.** `signal.NotifyContext` cancels a context when the process receives a stop signal. `server.Run` starts HTTP serving in a goroutine and uses a channel to receive its result. `select` waits for either that result or cancellation. Shutdown uses a fresh bounded context so cancellation of the serving context does not immediately abort draining requests.

`defer` schedules cleanup for when the current function returns. That is why cancellation functions and signal registrations are cleaned up even when a function exits early.

## Optional exercise

Add a test in internal/config/config_test.go for a port containing letters. Predict the result before running `go test ./internal/config`. Then follow a request to `/health/ready` and explain why a missing dependency check must return 503 while liveness still returns 200. Learning is optional; delivery does not wait for the exercise.

Primary references: [Go language specification](https://go.dev/ref/spec), [HTTP server shutdown](https://pkg.go.dev/net/http#Server.Shutdown), [signal contexts](https://pkg.go.dev/os/signal#NotifyContext), and [Chi source and releases](https://github.com/go-chi/chi).
