// Package dto carries cross-layer data contracts between the transport
// adapters (internal/mcp), the use cases (internal/application/usecase) and
// the entities (internal/domain/entity).
//
// Wire-tag convention (close-out of the INPUT-tag duplication advisory in
// odd/tasks/review-findings-backlog.md, tracked by issue #126):
//
//   - '*Input' carriers are wire-free: the transport adapter owns the JSON
//     request shapes through its '*In' structs (e.g. internal/mcp
//     tools_booking.go createBookingIn) and maps them onto these carriers.
//     A data field on an '*Input' carrier therefore carries no wire tag; only
//     marker guards (`json:"-"`) are allowed (Caller on every carrier, plus
//     internal presence/clear markers such as ClearLatitude).
//   - Results and views are response bodies the transport returns directly,
//     so their JSON tags ARE the live wire contract and they are pinned by
//     the tag and round-trip tests in this package.
//   - The Caller identity field keeps the protective `json:"-"` on every
//     'Input' carrier: auth context must never serialize into a transport
//     payload (see TestCallerFieldNotSerialized).
//
// Drift guards: TestCallerFieldNotSerialized, TestInputCarriersAreWireFree,
// TestDTOFieldTags, TestResultJSONRoundTrip.
package dto
