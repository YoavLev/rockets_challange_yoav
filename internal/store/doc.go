// Package store keeps every rocket's stream in memory.
//
// One sync.RWMutex guards the map of streams: writes take the write lock,
// reads take the read lock. At the test program's concurrency this is not a
// bottleneck, and it is much simpler than per-rocket locks. Reads return
// copies, never pointers to shared state.
package store
