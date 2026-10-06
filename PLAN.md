# Lunar Rockets Challenge — Implementation Plan

**Status:** built. Tasks 0–7 are done and match the code; Task 8 (review and polish) remains.
**Main point:** build one Go service that applies each rocket's messages strictly in message-number order, with a small per-rocket reorder buffer that also drops duplicates. The design is event sourcing: each rocket's ordered message stream is the source of truth, and the rocket state is a projection of it. Prove it with a property test and an end-to-end run against the real test program at its default settings.

**Out of scope:** durable storage, horizontal scaling and a dashboard UI. The README describes how scaling would work; it isn't built.

Terms used throughout:
- **Rocket**: one entity, identified by its `channel` UUID. "Channel" refers only to the ID field.
- **Reorder buffer**: per-rocket logic that drops duplicates and holds early messages (§2).
- **Applied counter** (`lastAppliedMessage`): the highest message number applied with no gaps before it.

## 1. What the test program actually does

Measured on 2026-10-03 with `darwin_arm64/rockets`, seed 444, 2,000 messages, posting to a capture server that always returned 200.

| Finding | Consequence |
|---|---|
| Defaults are concurrency 3, no delay and 100,000 messages. The README example uses 500ms and concurrency 1, but Lunar says it will run the defaults. | The service must handle a fast concurrent flood. Concurrency correctness matters. |
| The only headers are standard ones (`Content-Type: application/json`). There is no idempotency key; the `X-Idempotency-Key` string in the binary is Go's stdlib. | Duplicates must be detected by (channel, message number). |
| 20 rockets, highest message number 125, 0 malformed bodies. | |
| 3 of 20 rockets received messages out of order, never displaced by more than 1. 0 duplicates. | Reordering is real but shallow in this sample. Duplicates are still guaranteed by the README, so they are tested via unit and property tests. |
| Full default run (100k), measured 2026-10-05: the test program opens a new TCP connection per message and never reuses it. With HTTP keep-alive on, ~42,000 idle connections piled up, the producer ran out of local ports (`connect: resource temporarily unavailable`, ~13,600 times), and 72 messages were lost when its redelivery gave up, leaving 1,710 pending. | The server disables keep-alive (`srv.SetKeepAlivesEnabled(false)`): 0 errors, 0 lost, 0 pending, and the run took 63s instead of 125s. `scripts/e2e.sh` fails without the fix. |
| In the 2,000-message capture, only launch, speed and mission messages were seen; no `RocketExploded`. Speeds never went below 500. | Unit tests cover explosions. The full 100k e2e run does include them (6 of 20 rockets exploded in one run) and passes. |

## 2. Approach

Write it in Go, because Lunar writes all its services in Go. Reviewers grade documentation, design (readable, scalable, maintainable) and verification. The core difficulty is correct state under out-of-order and duplicate delivery; the REST API is the easy part.

**Messages must be applied in order, not sorted by time.** Speed messages carry increments (`by: 3000`), so each must be applied exactly once and in sequence. Each rocket holds its applied counter plus a buffer of early messages keyed by number. For an incoming message *n*:

| Condition | Action | Response |
|---|---|---|
| *n* ≤ applied counter, or *n* already buffered | Duplicate: drop it | `200 {"status":"duplicate"}` |
| *n* = applied counter + 1 | Apply it, then apply buffered messages while the next number is present | `200 {"status":"applied"}` |
| *n* > applied counter + 1 | Buffer it until the gap fills | `200 {"status":"buffered"}` |
| Valid JSON, but `Apply` rejects it (e.g. a second launch) | Skip it: the counter moves past it, the state is unchanged, and a warning is logged | `200 {"status":"applied"}` |
| Malformed body | Reject | `400` |

**The producer redelivers any message that doesn't get a 2xx** (task README). That shapes the responses:
- Every valid message gets a 2xx, including duplicates and skipped ones. Otherwise it would come back again, pointlessly.
- A 2xx is a promise that the message is recorded, so the response is sent only after `Receive` has stored or applied it. Nothing is processed asynchronously after replying, and graceful shutdown finishes in-flight requests.
- Malformed bodies still get `400`, which is correct HTTP. The risk: a malformed message would be redelivered and rejected again indefinitely. Recon saw 0 malformed bodies, so this is accepted. Answering 2xx and dropping such messages is the alternative if that ever happens.
- Future work: a non-2xx is "retry later", not "lost", so `503` can safely shed load or cap the pending map without losing data. The producer's retry policy (which codes, how often, whether it gives up) hasn't been measured.

**State is built from the ordered message stream (event sourcing in shape).** The message number serves as the stream version, and the rocket state is what you get by applying the stream in order. Keeping the applied messages as an in-memory event log, with `domain.Replay(log)` to rebuild state from it, is deferred to Stretch (§4): the minimum doesn't need it, and in memory only tests would call it.

**The code is split so the ordering logic has no I/O.** The domain package knows nothing about HTTP or storage, which makes the core cheap to test. There is no repository interface: with a single in-memory implementation it would be abstraction without a second user. Adding Postgres later means introducing that interface then.

**One lock for the whole store.** A single `sync.RWMutex` guards the map of rockets: writes take the write lock, reads take the read lock. At the test program's concurrency (3) it is nowhere near a bottleneck, and it is far easier to get right than per-rocket locks. Reads return copies. Per-rocket locks and partitioning by channel are the documented next steps (§3 Scaling).

**The correctness test checks shuffled input against in-order input.** Take a valid message stream, shuffle it and inject random duplicates. Applying that must give the same final state as applying the original stream in order. This test is the strongest evidence a reviewer can get.

### Decisions on edge cases

Each decision is in the README's "Edge cases" table.

| Case | Decision |
|---|---|
| Messages after `RocketExploded` | Still applied in order. `exploded` stays true, keeping a full history. |
| Speed would go negative | Allowed, not clamped or rejected: messages are facts, and the state must match them. The store logs a warning. Rejecting malformed amounts (≤ 0) is future work. |
| A message never arrives (permanent gap) | State stays at the applied counter. `pendingMessages > 0` exposes it in the API. A gap timeout is listed as future work. |
| Message 1 hasn't arrived yet | The rocket is listed with `lastAppliedMessage: 0` and empty type/mission until it does. "Not launched yet" is derived from that, not stored. |
| A second `RocketLaunched` (different message number) | Invalid: `Apply` returns an error. The reorder buffer skips it: the applied counter advances, the state doesn't change, and a warning is logged. This keeps the stream flowing. |
| What `lastUpdated` means | The `messageTime` of the last applied message, not the time it was received. This keeps the domain clock-free and deterministic. |

Reorder buffer choices (Task 2):
- A `Stream` holds the rocket, its own counter of handled messages and the pending map. `Receive(m)` returns `(Outcome, error)`.
- The counter is separate from `Rocket.LastAppliedMessage`, because a skipped invalid message moves the counter but not the rocket.
- Errors from messages skipped during a drain are combined with `errors.Join`.
- A duplicate with different content is still a duplicate: the first one wins.
- The pending map is unbounded. A size cap (answering `503` so the producer redelivers later) or a gap timeout is future work.

Store choices (Task 3):
- `Receive(m)` returns only an `Outcome`. The store logs skipped messages and speed crossing below zero itself.
- Whether a message is safely recorded is a domain fact: `Outcome.Recorded()`, next to the constants. `httpapi` only translates it: recorded → `200`, otherwise `503` so the producer resends. A future outcome (e.g. `OutcomeRetryLater` for a pending-map cap) is added in one file, `domain/stream.go`. No error return is kept "just in case".
- `Get(channel)` returns `(Snapshot, bool)`, the comma-ok idiom. There is no `ErrNotFound`.
- `Snapshot` embeds `domain.Rocket` and adds `Pending`.
- Sorting lives in the store, with typed `SortBy` keys. Ties are broken by channel.
- `store.New(logger)` takes a `*slog.Logger`.

HTTP choices (Task 4):
- Every recorded message gets `200`, not `202`: the reply is sent only after the message is recorded, so processing is finished. The producer only checks for 2xx.
- Unknown JSON fields are ignored (Go's default). Rejecting them would turn a new producer field into 400s, and then an endless redelivery loop.
- A zero `launchTime`/`lastUpdated` is left out of the response (`omitzero`) rather than shown as year 1.
- No access log: at 100k messages it would flood the output. Only warnings (from the store) and server errors are logged.

Domain modelling choices (Task 1): payloads are plain structs handled by one type `switch` in `Apply`. `Apply` takes a rocket by value and returns a new rocket. Speeds are `int`, the channel is a `string` and times are `time.Time`.

## 3. Architecture

```
rockets binary ──POST /messages──▶ ┌──────── rockets-service ────────┐
                                   │ internal/httpapi                │
                                   │   decode JSON, routes, errors   │
                                   │        │                        │
                                   │ internal/store                  │
                                   │   map[channel]*Stream + RWMutex │
                                   │        │                        │
                                   │ internal/domain                 │
                                   │   Rocket, Apply, Stream         │
                                   └─────────────────────────────────┘
```

| Package | Responsibility |
|---|---|
| `internal/domain` | The rocket state, the five message types, `Apply` and the reorder buffer (`Stream`), plus `Outcome.Recorded()`. No I/O. |
| `internal/store` | In-memory map of channel → `*domain.Stream` behind one `sync.RWMutex`. `Receive(m)`, `Get(channel)`, `List(sortBy, desc)` and `Stats()` (messages received, rocket count). Returns copies. Logs a warning when a message is skipped or speed crosses below zero. |
| `internal/httpapi` | JSON → `domain.Message` (envelope first, then the payload chosen by `messageType`), routes on Go 1.22+ `net/http`, and the mapping from errors to HTTP status codes, in one place. Basic validation only: valid JSON, a known `messageType`, `messageNumber` ≥ 1, a channel present. |
| `cmd/rockets-service` | `-addr` flag (default `:8088`), `slog` JSON logging, keep-alive disabled (§1), a `ReadHeaderTimeout`, graceful shutdown on SIGINT/SIGTERM, and a `stats` log line every 2s (messages, rockets, goroutines). |

### API

- `POST /messages`: see the table in §2.
- `GET /rockets/{channel}`: returns `{channel, type, mission, speed, exploded, explosionReason, launchTime, lastUpdated, lastAppliedMessage, pendingMessages}`. There is no stored status: the task doesn't require one, and it would duplicate `exploded` and `lastAppliedMessage`. An unknown channel returns 404.
- `GET /rockets?sort=speed|mission|type|launchTime&order=asc|desc`: the default is by channel; ties are broken by channel, so ordering is stable. An unknown `sort` value returns 400.
- Stretch (§4), with the event log: `GET /rockets/{channel}/events`, for debugging and audit.
- Stretch (§4): `/metrics` (Prometheus counters per outcome).

### Scaling (README only, not built)

The pattern is event sourcing with CQRS: the write side is the per-rocket event stream, and the read side is the `rockets` projection the API serves. All work is per rocket, so the system partitions by channel:
- Ingest writes to Kafka keyed by channel. Each consumer then owns its rockets' state, with no cross-node locking.
- The event store is an `events` table with `UNIQUE(channel, message_number)`. Duplicates are then rejected by the database, and the `rockets` read table is updated in the same transaction.
- Projections can be rebuilt by replaying `events`. For long streams, store a snapshot every N events and replay only what comes after it.

## 4. Tasks (6h)

The README asks for at most 6 hours, so the scope is cut to fit. Items under "Stretch" are done only if time remains.

Revised on 2026-10-05 after Task 2, keeping only what the minimum needs: the `ingest` and `service` packages and the repository interface were merged away, per-rocket locks became one lock, and packaging and extra ADRs moved to Stretch.

| # | Task | Description | Est. |
|---|---|---|---|
| 0 | Bootstrap ✅ | `go mod init`, Makefile, golangci-lint config, `CLAUDE.md`. | 0.25h |
| 1 | Domain model ✅ | Rocket state, the five message types, `Apply`. Table-driven tests. | 0.75h |
| 2 | Reorder buffer ✅ | `Stream.Receive`: duplicates, buffering, drain, skip. Table tests, skip test, property test (`rapid`). | 1.25h |
| 3 | Store ✅ | `internal/store`: `Receive`, `Get`, `List` with sorting, one `RWMutex`, warnings logged. A test sends concurrent messages to many rockets under `-race`. | 0.75h |
| 4 | HTTP API ✅ | `internal/httpapi`: decode and validate JSON, `POST /messages`, `GET /rockets/{channel}`, `GET /rockets`, 1 MB body limit. One `httptest` test per route, against the real store. | 1.25h |
| 5 | Main ✅ | Port flag, `slog`, graceful shutdown on SIGTERM. | 0.25h |
| 6 | End-to-end test ✅ | `scripts/e2e.sh` starts the service on port 8089, runs `rockets launch` with default flags (the setup Lunar will use), and fails if the test program logged any delivery error or any rocket has `pendingMessages > 0`. Full run: 42s, pass. Confirmed to fail without the keep-alive fix. | 0.5h |
| 7 | Documentation ✅ | README: how to run and test, curl examples, the API and response codes, design decisions, edge cases, verification, scaling. `AI_WORKFLOW.md`: how AI was used, what was rejected or corrected. The challenge brief moved to `docs/CHALLENGE.md`. | 0.5h |
| 8 | Review and polish | Self review plus an AI review pass | 0.25h |
| | Buffer | For unseen test-program behaviour, e.g. explosions or malformed messages in a 100k run. | 0.25h |

**Stretch, only if time remains:**
- Event log and `Replay` (decide after the minimum is done): keep each rocket's applied messages in memory, add `domain.Replay(log)`, assert live state equals `Replay` in the property test and e2e, and expose `GET /rockets/{channel}/events`.
- `/metrics`.
- A 20k-message capture to look for duplicates and explosions.
- Stricter validation: channel is a UUID, speed amounts > 0, launch speed ≥ 0.
- Per-rocket locks, so rockets never block each other.
- More ADRs (in-memory vs database, in-order application vs time sorting), if the README's design section isn't enough.
- In the e2e test, a comparison of speeds against an in-order replay, if the same seed produces the same messages every run.

Anything not built goes in the README's "what I'd do with more time".

## 5. Coding conventions

- Write tests first for the domain and the reorder buffer, using table-driven tests.
- Use the standard library (`net/http`, `encoding/json`, `log/slog`, `testing`). The only planned dependency is `rapid`, plus `prometheus/client_golang` if `/metrics` is built.
- No interfaces unless a second implementation or a test needs one; when one is needed, define it in the package that consumes it.
- Run `go test -race ./...` on every change.
- Make one commit per task. Reviewers read history, and it shows how AI was used.
