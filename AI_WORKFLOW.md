# How I used AI on this challenge

I built this with Claude Code (Opus 5.5) in one long session. The rule I set at the start: **I own the architecture and the decisions; the agent explains, proposes, implements in small steps and checks its own work.** This file covers how that worked, what I changed or rejected, and where the agent got things wrong.

## Setup

- **`CLAUDE.md`** is the agent's rulebook for this repo: the package boundaries (`httpapi → store → domain`, no I/O in the domain), the domain rules (apply in message-number order, 2xx only after a message is recorded), Go guardrails.
- **`PLAN.md`** is the source of truth for tasks and decisions. Each decision was written there before or right after the code, so the agent and I always worked from the same document.

## How a task went

1. The agent explained the task and the concepts involved, and listed the decisions to make, with a recommendation for each.
2. I made the decisions, often against or beyond the recommendation (examples below).
3. We built it one step at a time. I wrote parts myself: the message types and most of `Apply`. The agent reviewed my code, filled in the rest, and explained what it wrote. I used this to learn idioms that were new to me: table-driven tests, property-based testing.
4. `go test -race ./...` after every change, then a PLAN.md update.

## Recon before code

Before writing any code, the agent ran the real test program against a small capture server and measured what it actually sends: the default settings (100k messages, concurrency 3, no delay), the headers (no idempotency key), how far out of order messages arrive, and whether duplicates appear. The design is based on those measurements rather than on assumptions from the brief.

## Decisions I made differently from the agent

| The agent proposed | What I decided | Why |
|---|---|---|
| A stored `Status` field on the rocket (launched, exploded…) | Dropped | The task doesn't ask for it, and it would duplicate `exploded` and `lastAppliedMessage`. |
| An in-memory event log and `Replay`, after I asked whether the plan used event sourcing | Deferred | Nice design, but not part of the minimum. It's described in the README's scaling section instead. |
| Mapping outcomes to status codes with a `switch` in `httpapi` | `Outcome.Recorded()` in the domain | I noticed that adding a new outcome would mean changing several places. Now it's one file. |
| Test cases for the reorder buffer that only covered the basics | More cases with duplicates, buffering and draining together | The simple cases didn't prove the hard part. |

Two decisions came from my own questions rather than the agent's proposals:
- **Negative speed.** My first instinct was to reject it as an error. Talking it through, I separated the input (an increase or decrease, always valid) from the result (a negative state), and decided messages are facts that should be recorded and flagged, not refused.
- **Redelivery.** I made the brief's "messages are redelivered on any non-2xx" explicit to the agent. That shaped the whole response policy: 2xx only after the message is stored, 2xx for duplicates, and 400 only for malformed bodies.

## Tests as the guardrail

every layer has a test that can fail:
- table-driven unit tests for `Apply` and the reorder buffer;
- a property test: any shuffled order with duplicates gives the same rocket as in-order delivery;
- a concurrency test under `-race`;
- HTTP tests that compare exact response JSON;
- an end-to-end run against the real test program at its default settings.


## The finding I'm happiest with

I asked for a periodic log line with message count, rocket count and goroutine count, just to watch a run. On the full default run it showed tens of thousands of goroutines. Following that up, the agent found that the test program opens a new TCP connection per message, our server kept every one alive, and the test program ran out of local ports, so some messages were lost after its retries gave up. Disabling keep-alive fixed it: 0 errors, 0 lost, and the run took about half as long. The e2e test was confirmed to fail without the fix. A spec-only reading wouldn't have found this; measuring did.
