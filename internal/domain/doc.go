// Package domain holds the rocket model and the rules for applying messages.
//
// It is pure: no I/O, no HTTP, no storage, no clocks. Everything here is
// deterministic so it can be tested exhaustively.
//
// Contents (see PLAN.md §2):
//   - Rocket state and the five message types.
//   - Apply: folds one message into a rocket's state.
//   - The reorder buffer: drops duplicates and holds early messages until
//     the gap before them is filled.
package domain
