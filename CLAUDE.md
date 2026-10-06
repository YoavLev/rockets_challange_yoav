# CLAUDE.md

A Go service that consumes rocket messages (out of order, possibly duplicated) and serves each rocket's state over REST. The design and task list are in `PLAN.md`.

## Commands

- `make test`: `go test -race ./...`. Run it after every change.
- `make lint`: `go vet` plus golangci-lint.
- `make run`: start the service on :8088.
- `make e2e`: run against the real test program (`./rockets`).

## How we work

- I own the architecture. Don't add packages, dependencies, interfaces or abstractions without asking first.
- Work in small, reviewable steps, one task from `PLAN.md` at a time. Don't touch code outside the task.
- If the plan and the code disagree, stop and ask. Don't silently pick one.
- Never commit or push unless asked.

## Architecture rules

- `internal/domain` is pure: no I/O, no HTTP, no `time.Now()`, no logging.
- Packages: `httpapi → store → domain`. Dependencies point inward; nothing imports `httpapi`.
- No interfaces unless a second implementation or a test actually needs one. When one is needed, define it in the package that consumes it. Return concrete types.
- HTTP status codes appear only in `httpapi`.

## Domain rules

- Messages are applied strictly in `messageNumber` order per channel, never sorted by `messageTime`.
- Duplicates are detected by (channel, messageNumber) and dropped. They still get a 2xx response.
- The producer redelivers anything that doesn't get a 2xx. Reply 2xx only after the message is stored or applied, never before. Only malformed bodies get a 4xx.

## Go code guardrails

- Prioritize readability. Code is read far more often than written: a plain loop beats a clever one-liner.
- Keep functions short and flat. Use early returns instead of nested `if`/`else`.
- Use clear names: short names for small scopes (`r`, `msg`), descriptive names for exported identifiers. No stutter (`domain.Rocket`, not `domain.DomainRocket`).
- Wrap errors with context: `fmt.Errorf("decode payload: %w", err)`. Use sentinel errors (e.g. `ErrInvalidMessage`) and check them with `errors.Is`. For lookups, prefer comma-ok (`v, ok := ...`) over a not-found error.
- Never ignore errors silently. Never `panic` outside `main` for expected failures.
- Pass `context.Context` as the first argument wherever I/O happens. Don't store it in structs.
- Prefer the standard library. No frameworks, no ORMs, no generics unless they clearly reduce duplication.
- Avoid premature abstraction: no interface with a single implementation unless it's at a package boundary that needs it for testing.
- Keep zero values useful, and add constructors (`NewX`) only when setup is actually required.
- Reads from the store return copies; don't leak pointers to shared state.
- Format with `gofmt` and `goimports`; the code must pass `make lint` with no new warnings.

## Documentation

- Don't over-document. The code should explain itself through names and structure.
- Write a comment only for the *why*: a non-obvious decision, an edge case or a constraint. Never restate what the code does.
- Every package has one short package comment (`doc.go`). Exported identifiers get a one-line doc comment only when the name isn't self-explanatory.
- No commented-out code, no `// TODO` without a reason, no banner comments.
## Tests

- Domain and reorder-buffer code is written test-first with table-driven tests.
- Test behavior through exported APIs, not internals.
- Use the standard `testing` package. `pgregory.net/rapid` is used only for property tests.
- A test name says the scenario: `TestApply/speed_decrease_below_zero_is_allowed`.
- No sleeps in tests.
