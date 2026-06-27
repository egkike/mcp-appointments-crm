// Package model contains the domain structs for the project's entities.
//
// Entities include: BusinessProfile, Client, Service, Professional, Booking, Schedule,
// and PendingAlert. All structs map 1:1 to the SQLite tables declared
// in internal/db/database.go.
//
// Computed/DTO types (e.g., a future LoyaltyReport DTO built from aggregation
// queries) do NOT map 1:1 to a table and are not declared here unless they
// represent a persistent entity.
//
// Canonical naming (per docs/PRD.md §3.5):
//   - The reservations table is "bookings" (not "appointments").
//   - Duration field is "duration_minutes" (not "duration_mins").
//   - Messenger fields live in BusinessProfile, not Client.
//   - Go repos are plural for collections (BookingsRepo) and singular for aggregates
//     (Booking); models are always singular (Booking, Client).
package model
