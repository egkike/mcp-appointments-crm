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

func TestBusinessProfile_Validate(t *testing.T) {
	whatsapp := "whatsapp"
	telegram := "telegram"
	invalidPlatform := "signal"
	validPayments := `["cash","card"]`
	invalidPayments := `"not-an-array"`
	emptyStrPayment := `["cash",""]`
	validHours := `{"1":{"open":"09:00","close":"18:00"}}`
	invalidHours := `[1,2,3]`
	validTZ := "America/Argentina/Buenos_Aires"
	invalidTZ := "Mars/Unknown"

	tests := []struct {
		name    string
		bp      *BusinessProfile
		wantErr bool
	}{
		{
			name: "valid profile with all optional fields",
			bp: &BusinessProfile{
				MessengerPlatform:      &whatsapp,
				AcceptedPaymentMethods: &validPayments,
				BusinessHours:          validHours,
				Timezone:               validTZ,
			},
		},
		{
			name: "valid profile with nil optional fields",
			bp:   &BusinessProfile{},
		},
		{
			name: "valid profile with telegram platform",
			bp: &BusinessProfile{
				MessengerPlatform: &telegram,
			},
		},
		{
			name: "invalid messenger platform",
			bp: &BusinessProfile{
				MessengerPlatform: &invalidPlatform,
			},
			wantErr: true,
		},
		{
			name: "invalid payment methods (not an array)",
			bp: &BusinessProfile{
				AcceptedPaymentMethods: &invalidPayments,
			},
			wantErr: true,
		},
		{
			name: "invalid payment methods (empty string in array)",
			bp: &BusinessProfile{
				AcceptedPaymentMethods: &emptyStrPayment,
			},
			wantErr: true,
		},
		{
			name: "invalid business hours (array instead of object)",
			bp: &BusinessProfile{
				BusinessHours: invalidHours,
			},
			wantErr: true,
		},
		{
			name: "invalid timezone",
			bp: &BusinessProfile{
				Timezone: invalidTZ,
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.bp.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
