package usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/egkike/mcp-appointments-crm/internal/application/dto"
	"github.com/egkike/mcp-appointments-crm/internal/auth"
	"github.com/egkike/mcp-appointments-crm/internal/domain"
	domainrepo "github.com/egkike/mcp-appointments-crm/internal/domain/repository"
)

func TestGetLoyaltyReportUseCase(t *testing.T) {
	fixed := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)

	t.Run("invalid period rejected with valid values", func(t *testing.T) {
		uc := NewGetLoyaltyReportUseCase(&mockBookingsRepo{})
		_, err := uc.Execute(context.Background(), dto.GetLoyaltyReportInput{
			Caller: adminCaller(),
			Period: "yesterday",
		})
		if !errors.Is(err, domain.ErrInvalidInput) {
			t.Fatalf("got %v, want ErrInvalidInput", err)
		}
		wantSub := "last_week, last_month, last_quarter, last_year, all_time"
		if err == nil || !contains(err.Error(), wantSub) {
			t.Errorf("message %q does not list valid values", err)
		}
	})

	t.Run("omitted period defaults to last_month", func(t *testing.T) {
		var gotStart, gotEnd time.Time
		uc := NewGetLoyaltyReportUseCase(&mockBookingsRepo{
			AggregateByClientFn: func(ctx context.Context, start, end time.Time, limit int) ([]domainrepo.ClientBookingCount, error) {
				gotStart, gotEnd = start, end
				return nil, nil
			},
		})
		uc.SetClock(fixedClock{fixed})
		_, err := uc.Execute(context.Background(), dto.GetLoyaltyReportInput{Caller: adminCaller()})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !gotStart.Equal(fixed.AddDate(0, -1, 0)) || !gotEnd.Equal(fixed) {
			t.Errorf("window = [%s, %s), want [%s, %s)", gotStart, gotEnd, fixed.AddDate(0, -1, 0), fixed)
		}
	})

	t.Run("period windows", func(t *testing.T) {
		tests := []struct {
			period    string
			wantStart time.Time
		}{
			{"last_week", fixed.AddDate(0, 0, -7)},
			{"last_month", fixed.AddDate(0, -1, 0)},
			{"last_quarter", fixed.AddDate(0, -3, 0)},
			{"last_year", fixed.AddDate(-1, 0, 0)},
		}
		for _, tt := range tests {
			t.Run(tt.period, func(t *testing.T) {
				var gotStart time.Time
				uc := NewGetLoyaltyReportUseCase(&mockBookingsRepo{
					AggregateByClientFn: func(ctx context.Context, start, end time.Time, limit int) ([]domainrepo.ClientBookingCount, error) {
						gotStart = start
						return nil, nil
					},
				})
				uc.SetClock(fixedClock{fixed})
				_, err := uc.Execute(context.Background(), dto.GetLoyaltyReportInput{
					Caller: adminCaller(),
					Period: tt.period,
				})
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if !gotStart.Equal(tt.wantStart) {
					t.Errorf("window start = %s, want %s", gotStart, tt.wantStart)
				}
			})
		}
	})

	t.Run("all_time has no lower bound", func(t *testing.T) {
		var gotStart time.Time
		uc := NewGetLoyaltyReportUseCase(&mockBookingsRepo{
			AggregateByClientFn: func(ctx context.Context, start, end time.Time, limit int) ([]domainrepo.ClientBookingCount, error) {
				gotStart = start
				return nil, nil
			},
		})
		uc.SetClock(fixedClock{fixed})
		_, err := uc.Execute(context.Background(), dto.GetLoyaltyReportInput{
			Caller: adminCaller(),
			Period: "all_time",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !gotStart.IsZero() {
			t.Errorf("all_time start = %s, want zero", gotStart)
		}
	})

	t.Run("top_n within range honored", func(t *testing.T) {
		var gotLimit int
		topN := 15
		uc := NewGetLoyaltyReportUseCase(&mockBookingsRepo{
			AggregateByClientFn: func(ctx context.Context, start, end time.Time, limit int) ([]domainrepo.ClientBookingCount, error) {
				gotLimit = limit
				return nil, nil
			},
		})
		uc.SetClock(fixedClock{fixed})
		_, err := uc.Execute(context.Background(), dto.GetLoyaltyReportInput{
			Caller: adminCaller(),
			Period: "last_month",
			TopN:   &topN,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if gotLimit != 15 {
			t.Errorf("limit = %d, want 15", gotLimit)
		}
	})

	t.Run("top_n above 100 clamped", func(t *testing.T) {
		var gotLimit int
		topN := 1000000
		uc := NewGetLoyaltyReportUseCase(&mockBookingsRepo{
			AggregateByClientFn: func(ctx context.Context, start, end time.Time, limit int) ([]domainrepo.ClientBookingCount, error) {
				gotLimit = limit
				return nil, nil
			},
		})
		uc.SetClock(fixedClock{fixed})
		_, err := uc.Execute(context.Background(), dto.GetLoyaltyReportInput{
			Caller: adminCaller(),
			Period: "last_month",
			TopN:   &topN,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if gotLimit != 100 {
			t.Errorf("limit = %d, want 100", gotLimit)
		}
	})

	t.Run("top_n below 1 clamped", func(t *testing.T) {
		var gotLimit int
		topN := 0
		uc := NewGetLoyaltyReportUseCase(&mockBookingsRepo{
			AggregateByClientFn: func(ctx context.Context, start, end time.Time, limit int) ([]domainrepo.ClientBookingCount, error) {
				gotLimit = limit
				return nil, nil
			},
		})
		uc.SetClock(fixedClock{fixed})
		_, err := uc.Execute(context.Background(), dto.GetLoyaltyReportInput{
			Caller: adminCaller(),
			Period: "last_month",
			TopN:   &topN,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if gotLimit != 1 {
			t.Errorf("limit = %d, want 1", gotLimit)
		}
	})

	t.Run("omitted top_n defaults to 10", func(t *testing.T) {
		var gotLimit int
		uc := NewGetLoyaltyReportUseCase(&mockBookingsRepo{
			AggregateByClientFn: func(ctx context.Context, start, end time.Time, limit int) ([]domainrepo.ClientBookingCount, error) {
				gotLimit = limit
				return nil, nil
			},
		})
		uc.SetClock(fixedClock{fixed})
		_, err := uc.Execute(context.Background(), dto.GetLoyaltyReportInput{
			Caller: adminCaller(),
			Period: "last_month",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if gotLimit != 10 {
			t.Errorf("limit = %d, want 10", gotLimit)
		}
	})

	t.Run("owner allowed", func(t *testing.T) {
		uc := NewGetLoyaltyReportUseCase(&mockBookingsRepo{
			AggregateByClientFn: func(ctx context.Context, start, end time.Time, limit int) ([]domainrepo.ClientBookingCount, error) {
				return []domainrepo.ClientBookingCount{
					{ClientID: "c1", Name: "Ana", Phone: "+1", BookingCount: 3},
				}, nil
			},
		})
		uc.SetClock(fixedClock{fixed})
		result, err := uc.Execute(context.Background(), dto.GetLoyaltyReportInput{
			Caller: auth.Caller{ID: "owner-1", Role: auth.RoleOwner},
			Period: "last_month",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result.Results) != 1 {
			t.Fatalf("got %d results, want 1", len(result.Results))
		}
		if result.Results[0].BookingCount != 3 {
			t.Errorf("booking_count = %d, want 3", result.Results[0].BookingCount)
		}
	})

	t.Run("admin allowed", func(t *testing.T) {
		uc := NewGetLoyaltyReportUseCase(&mockBookingsRepo{
			AggregateByClientFn: func(ctx context.Context, start, end time.Time, limit int) ([]domainrepo.ClientBookingCount, error) {
				return []domainrepo.ClientBookingCount{}, nil
			},
		})
		uc.SetClock(fixedClock{fixed})
		_, err := uc.Execute(context.Background(), dto.GetLoyaltyReportInput{
			Caller: adminCaller(),
			Period: "last_month",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("staff rejected", func(t *testing.T) {
		uc := NewGetLoyaltyReportUseCase(&mockBookingsRepo{})
		_, err := uc.Execute(context.Background(), dto.GetLoyaltyReportInput{
			Caller: staffCaller("staff-1", "p1"),
			Period: "last_month",
		})
		if !errors.Is(err, domain.ErrForbidden) {
			t.Errorf("got %v, want ErrForbidden", err)
		}
	})

	t.Run("client rejected", func(t *testing.T) {
		uc := NewGetLoyaltyReportUseCase(&mockBookingsRepo{})
		_, err := uc.Execute(context.Background(), dto.GetLoyaltyReportInput{
			Caller: clientCaller("c1"),
			Period: "last_month",
		})
		if !errors.Is(err, domain.ErrForbidden) {
			t.Errorf("got %v, want ErrForbidden", err)
		}
	})

	t.Run("unauthenticated rejected", func(t *testing.T) {
		uc := NewGetLoyaltyReportUseCase(&mockBookingsRepo{})
		_, err := uc.Execute(context.Background(), dto.GetLoyaltyReportInput{
			Caller: emptyCaller(),
			Period: "last_month",
		})
		if !errors.Is(err, domain.ErrUnauthenticated) {
			t.Errorf("got %v, want ErrUnauthenticated", err)
		}
	})

	t.Run("empty result returns non-nil slice", func(t *testing.T) {
		uc := NewGetLoyaltyReportUseCase(&mockBookingsRepo{
			AggregateByClientFn: func(ctx context.Context, start, end time.Time, limit int) ([]domainrepo.ClientBookingCount, error) {
				return []domainrepo.ClientBookingCount{}, nil
			},
		})
		uc.SetClock(fixedClock{fixed})
		result, err := uc.Execute(context.Background(), dto.GetLoyaltyReportInput{
			Caller: adminCaller(),
			Period: "last_month",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Results == nil || len(result.Results) != 0 {
			t.Errorf("got %v, want empty non-nil slice", result.Results)
		}
	})

	t.Run("repo error propagates", func(t *testing.T) {
		uc := NewGetLoyaltyReportUseCase(&mockBookingsRepo{
			AggregateByClientFn: func(ctx context.Context, start, end time.Time, limit int) ([]domainrepo.ClientBookingCount, error) {
				return nil, domain.ErrInvalidInput
			},
		})
		uc.SetClock(fixedClock{fixed})
		_, err := uc.Execute(context.Background(), dto.GetLoyaltyReportInput{
			Caller: adminCaller(),
			Period: "last_month",
		})
		if !errors.Is(err, domain.ErrInvalidInput) {
			t.Errorf("got %v, want ErrInvalidInput", err)
		}
	})
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

type fixedClock struct {
	now time.Time
}

func (c fixedClock) Now() time.Time { return c.now }
