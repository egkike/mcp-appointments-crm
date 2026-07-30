package entity

import "testing"

func TestBusinessProfile_IsOpenOn(t *testing.T) {
	tests := []struct {
		name          string
		businessHours string
		dayOfWeek     int
		want          bool
	}{
		{
			name:          "open on monday (day 1)",
			businessHours: `{"1":{"open":"09:00","close":"18:00"},"6":{"open":"10:00","close":"14:00"}}`,
			dayOfWeek:     1,
			want:          true,
		},
		{
			name:          "closed on wednesday (day 3)",
			businessHours: `{"1":{"open":"09:00","close":"18:00"},"6":{"open":"10:00","close":"14:00"}}`,
			dayOfWeek:     3,
			want:          false,
		},
		{
			name:          "open on saturday (day 6)",
			businessHours: `{"1":{"open":"09:00","close":"18:00"},"6":{"open":"10:00","close":"14:00"}}`,
			dayOfWeek:     6,
			want:          true,
		},
		{
			name:          "empty business hours",
			businessHours: `{}`,
			dayOfWeek:     1,
			want:          false,
		},
		{
			name:          "invalid JSON returns closed",
			businessHours: `{invalid`,
			dayOfWeek:     1,
			want:          false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bp := &BusinessProfile{BusinessHours: tt.businessHours}
			if got := bp.IsOpenOn(tt.dayOfWeek); got != tt.want {
				t.Errorf("IsOpenOn(%d) = %v, want %v", tt.dayOfWeek, got, tt.want)
			}
		})
	}
}

func TestBusinessProfile_GetOpenClose(t *testing.T) {
	bh := `{"1":{"open":"09:00","close":"18:00"},"6":{"open":"10:00","close":"14:00"}}`
	bp := &BusinessProfile{BusinessHours: bh}

	t.Run("existing day returns hours", func(t *testing.T) {
		open, close, ok := bp.GetOpenClose(1)
		if !ok {
			t.Fatal("GetOpenClose(1) returned ok=false, want true")
		}
		if open != "09:00" {
			t.Errorf("open = %q, want %q", open, "09:00")
		}
		if close != "18:00" {
			t.Errorf("close = %q, want %q", close, "18:00")
		}
	})

	t.Run("saturday hours", func(t *testing.T) {
		open, close, ok := bp.GetOpenClose(6)
		if !ok {
			t.Fatal("GetOpenClose(6) returned ok=false, want true")
		}
		if open != "10:00" {
			t.Errorf("open = %q, want %q", open, "10:00")
		}
		if close != "14:00" {
			t.Errorf("close = %q, want %q", close, "14:00")
		}
	})

	t.Run("non-existing day returns false", func(t *testing.T) {
		_, _, ok := bp.GetOpenClose(3)
		if ok {
			t.Error("GetOpenClose(3) returned ok=true, want false")
		}
	})

	t.Run("invalid JSON returns false", func(t *testing.T) {
		bpBad := &BusinessProfile{BusinessHours: `{broken`}
		_, _, ok := bpBad.GetOpenClose(1)
		if ok {
			t.Error("GetOpenClose with invalid JSON returned ok=true, want false")
		}
	})
}
