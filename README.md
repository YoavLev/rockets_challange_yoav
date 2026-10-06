# Rockets

A Go service that consumes rocket messages from Lunar's test program and serves each rocket's current state over a REST API. The challenge brief is in [docs/CHALLENGE.md](docs/CHALLENGE.md).

Messages arrive out of order and may be duplicated. The core of the service is a small **per-rocket reorder buffer**: it applies each rocket's messages strictly in message-number order, exactly once, and holds early messages until the gap before them fills.

## Quick start

Requirements: Go 1.25+, and the test program from the challenge ZIP copied to `./rockets` (or set `ROCKETS=path/to/rockets`).

```bash
make run                                          # service on :8088
./rockets launch "http://localhost:8088/messages" # in another terminal; defaults: 100k messages, concurrency 3
```

Then query it:

```bash
curl localhost:8088/rockets                               # all rockets, sorted by channel
curl 'localhost:8088/rockets?sort=speed&order=desc'       # fastest first
curl localhost:8088/rockets/193270a9-c9cf-404a-8f83-838e71d9ae67
```

Tests:

```bash
make test                             # unit, property and concurrency tests, with -race (a few seconds)
make e2e                              # the real test program, default settings, against the built service (~45s)
./scripts/e2e.sh --max-messages=2000  # a quick e2e run (needs make build first)
```

`make e2e` runs the service on port 8089 so it doesn't clash with `make run`. It fails if the test program logged any delivery error or any rocket still has messages pending, and prints the final state as a table.

While running, the service logs a `stats` line every 2 seconds (messages received, rockets, goroutines).

## API

| Route | Description |
|---|---|
| `POST /messages` | Receives one message from the test program. |
| `GET /rockets/{channel}` | One rocket's state. `404` if the channel is unknown. |
| `GET /rockets?sort=&order=` | All rockets. `sort`: `speed`, `mission`, `type`, `launchTime` (default: channel). `order`: `asc` (default) or `desc`. Ties are broken by channel, so the order is stable. An unknown value gives `400`. |

A rocket looks like this:

```json
{
  "channel": "193270a9-c9cf-404a-8f83-838e71d9ae67",
  "type": "Falcon-9",
  "mission": "ARTEMIS",
  "speed": 600,
  "exploded": false,
  "explosionReason": "",
  "launchTime": "2022-02-02T19:39:05.86337+01:00",
  "lastUpdated": "2022-02-02T19:39:05.86337+01:00",
  "lastAppliedMessage": 2,
  "pendingMessages": 0
}
```

- `lastAppliedMessage`: the highest message number applied with no gaps before it.
- `pendingMessages`: messages received early, waiting for an earlier one. A value above 0 that never goes down means a message never arrived.
- `lastUpdated`: the `messageTime` of the last applied message, not the time it was received.
- `launchTime` and `lastUpdated` are left out until the launch message has been applied.

### Responses to `POST /messages`

| Case | Response |
|---|---|
| Next message in order: applied, along with any buffered messages that now follow it | `200 {"status":"applied"}` |
| Early message: held until the gap fills | `200 {"status":"buffered"}` |
| Already applied or already buffered | `200 {"status":"duplicate"}`. Answering anything else would only make it come back. |
| Valid JSON, but breaks the protocol (a second `RocketLaunched`) | `200 {"status":"applied"}`: skipped, see [Edge cases](#edge-cases) |
| Malformed body (bad JSON, unknown `messageType`, missing channel/time, `messageNumber` < 1) | `400` |

## How it works

```
rockets ──POST /messages──▶ internal/httpapi   decode JSON, routes, status codes
                                   │
                            internal/store     map[channel]*Stream behind one RWMutex, sorting, warnings
                                   │
                            internal/domain    Rocket, Apply, Stream (the reorder buffer); no I/O
```

Dependencies point inward: `domain` knows nothing about HTTP or storage, which keeps the hard part cheap to test.

**Order by message number, not by time.** Each rocket's `Stream` keeps a counter of handled messages and a map of early ones. For incoming message *n*:

1. *n* ≤ counter, or *n* already in the map: duplicate, drop it.
2. *n* > counter + 1: put it in the map and wait.
3. *n* = counter + 1: apply it, then keep applying from the map while the next number is there.



## Design decisions

This is event sourcing in shape: applied as an ordered stream. the message number is the stream version, and a rocket's state is `Apply` folded over its stream. Copies are safe because `Rocket` holds only values and Stream.Rocket() returns it by value, so the store can hand out copies without sharing state.

### Other decisions

| Decision | Why |
|---|---|
| Duplicates detected by (channel, message number) | The test program sends no idempotency key (no header or message ID; checked in a captured run), but each message number is unique within its channel, so the pair identifies a message.|
| `Outcome.Recorded()` lives in the domain | Whether a message is safe is a domain fact; `httpapi` only maps it to `200` (recorded) or `503` (resend). A future outcome, such as "buffer full, retry later", is added in one place, `domain/stream.go`. Today every outcome is recorded, so `503` is never sent. |
| One `sync.RWMutex` for the whole store | nowhere near a bottleneck at concurrency 3. Per-rocket locks are the next step if it becomes one. |
| In-memory state | Durable storage is out of scope for the challenge; a restart loses state.|
| No stored rocket status | The task doesn't ask for one, and it would duplicate `exploded` and `lastAppliedMessage`. |
| Unknown JSON fields are ignored | Rejecting them would turn a new producer field into `400`s, and then an endless redelivery loop. |
| Only the standard library | Plus [`rapid`](https://pkg.go.dev/pgregory.net/rapid) for the property test. |

### Edge cases

| Case | Decision |
|---|---|
| Speed goes below zero | Allowed, with a warning logged when it crosses zero. Messages are facts; clamping or rejecting would make every later speed wrong.|
| A second `RocketLaunched` | Invalid: it breaks "sent out once". It is skipped (the counter moves past it, the state is unchanged) and a warning is logged, so later messages keep flowing. |
| Messages after `RocketExploded` | Still applied in order; `exploded` stays true. |
| Message 1 hasn't arrived yet | The rocket is listed with `lastAppliedMessage: 0`, an empty type and mission, and `pendingMessages > 0`. |
| A message never arrives | The rocket stays at the last applied message, and `pendingMessages` shows the stuck messages. |
| A malformed body | `400`. Risk: the test program would redeliver it forever. No malformed body was seen in any run, so this is accepted; answering `2xx` and dropping it is the fallback. |

## Verification

| Layer | What it proves |
|---|---|
| Table-driven tests for `Apply` and `Stream` (`internal/domain`) | Every message type; duplicates, buffering, draining, skipping an invalid message. |
| Property test (`TestReceiveAnyDeliveryOrder`, with `rapid`) | For random streams, any shuffled order with random duplicates gives the same rocket as applying the stream in order. This is the main correctness argument. |
| Concurrency test (`TestConcurrentReceive`, under `-race`) | 8 writers and a reader on 20 rockets, every message sent twice in random order: correct final state, no data races. |
| HTTP tests (`internal/httpapi`, `httptest`) | Decoding and validation, status codes, and the exact response JSON, against the real store. |
| End-to-end (`make e2e`) | The real test program at its default settings|

The tests were also checked against deliberate bugs:
- Removing the `delete` from the drain loop: the property test fails.
- Removing the store lock: the race detector reports races, and the concurrency test crashes with Go's fatal concurrent map access error.

## Project layout

```
cmd/rockets-service/   main: flags, logging, graceful shutdown, stats
internal/domain/       Rocket, the five message types, Apply, Stream (reorder buffer)
internal/store/        in-memory store: Receive, Get, List with sorting
internal/httpapi/      JSON decoding, routes, status codes
scripts/e2e.sh         end-to-end test against the real test program
PLAN.md                the plan and measurements this was built from
CLAUDE.md              conventions for AI coding agents working in this repo
```

## Scaling and production

All the work is per rocket, so the system scales by partitioning on channel. This isn't built; it's how I'd take it further:

- **Ingest:** the HTTP endpoint writes to Kafka, keyed by channel. Each consumer owns a set of rockets, so there's no cross-node locking and order within a partition is kept.
- **Event store:** an `events` table with `UNIQUE(channel, message_number)`. The `rockets` read table is updated in the same transaction (event sourcing with CQRS).
- **Rebuilds:** a projection can be rebuilt by replaying `events`; for long streams, snapshot every N events and replay only what follows.
- **Backpressure:** cap each rocket's pending map and answer `503` beyond it. The producer redelivers, so nothing is lost.
- **Operations:** health and readiness endpoints, Prometheus metrics per outcome, a container image.
