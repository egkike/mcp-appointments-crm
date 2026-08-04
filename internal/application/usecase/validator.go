package usecase

import (
	"context"

	"github.com/egkike/mcp-appointments-crm/internal/domain"
	"github.com/egkike/mcp-appointments-crm/internal/domain/service"
)

// bookingValidator is the narrow contract the use case layer needs for
// datetime validation. The concrete *service.BookingValidator satisfies it
// structurally; tests inject a function-table mock (mockBookingValidator).
//
// Declared once in this shared file so both CreateBookingUseCase and
// RescheduleBookingUseCase depend on the same narrow contract without code
// duplication (Go-idiomatic "accept interfaces, return structs").
//
// The consumer-facing interface `domain.BookingValidator` is NOT declared
// here because `internal/domain/` has a zero-dependency rule that prevents
// importing `internal/domain/entity/` — and `service.ValidateBookingInput`
// references entity types. The full placement resolution is deferred to
// refactor-clean-architecture P4.1a, which establishes the full DI wiring
// (including cmd/mcp-server/main.go). Until then, the narrow contract lives
// in the consumer package (usecase) — same package as both use cases, no
// external consumer.
type bookingValidator interface {
	Validate(ctx context.Context, input service.ValidateBookingInput) *domain.SemanticError
}
