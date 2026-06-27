// Package model contains the domain structs for the project's entities.
//
// Entities include (8 total): BusinessProfile, Client, Service, Professional,
// Booking, Schedule, PendingAlert, and BusinessHoursException.
// The 8 domain structs (BusinessProfile, Client, Service, Professional, Booking, Schedule, BusinessHoursException, PendingAlert) map 1:1 to the 8 **relational** domain SQLite tables declared in internal/db/database.go. The 2 FTS5 virtual tables (`clients_fts`, `services_fts`) and the metadata table `schema_version` have no corresponding model structs (they live in the schema, not in the Go layer).
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
//
// All `*_datetime` fields in these structs hold ISO 8601 UTC strings
// (regex: `^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{3}Z$`, e.g.
// "2026-07-13T13:30:00.000Z"). The repository converts any input timezone
// to UTC at insert time. See design.md Decisión 2 and Decisión 11
// for the storage/comparison contract.
package model
