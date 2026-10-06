// Package httpapi exposes the store over HTTP.
//
// Routes (see PLAN.md §3):
//
//	POST /messages
//	GET  /rockets/{channel}
//	GET  /rockets?sort=...&order=...
//
// It decodes the producer's JSON into domain messages, and it is the only
// package that knows about HTTP status codes; errors are mapped to them in
// one place.
package httpapi
