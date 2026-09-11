package config

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/egkike/mcp-appointments-crm/internal/domain/entity"
)

func TestMapBusinessHours(t *testing.T) {
	tests := []struct {
		name    string
		in      map[string]*BusinessHoursEntry
		want    map[string]BusinessHoursEntry
		wantErr bool
		errSub  string
	}{
		{
			name: "monday maps to 1",
			in: map[string]*BusinessHoursEntry{
				"monday": {Open: "09:00", Close: "18:00"},
			},
			want: map[string]BusinessHoursEntry{
				"1": {Open: "09:00", Close: "18:00"},
			},
		},
		{
			name: "sunday maps to 7",
			in: map[string]*BusinessHoursEntry{
				"sunday": {Open: "10:00", Close: "14:00"},
			},
			want: map[string]BusinessHoursEntry{
				"7": {Open: "10:00", Close: "14:00"},
			},
		},
		{
			name: "null sunday becomes absent",
			in: map[string]*BusinessHoursEntry{
				"sunday": nil,
			},
			want: map[string]BusinessHoursEntry{},
		},
		{
			name: "full week 6 open plus 1 null",
			in: map[string]*BusinessHoursEntry{
				"monday":    {Open: "09:00", Close: "18:00"},
				"tuesday":   {Open: "09:00", Close: "18:00"},
				"wednesday": {Open: "09:00", Close: "18:00"},
				"thursday":  {Open: "09:00", Close: "18:00"},
				"friday":    {Open: "09:00", Close: "18:00"},
				"saturday":  {Open: "10:00", Close: "14:00"},
				"sunday":    nil,
			},
			want: map[string]BusinessHoursEntry{
				"1": {Open: "09:00", Close: "18:00"},
				"2": {Open: "09:00", Close: "18:00"},
				"3": {Open: "09:00", Close: "18:00"},
				"4": {Open: "09:00", Close: "18:00"},
				"5": {Open: "09:00", Close: "18:00"},
				"6": {Open: "10:00", Close: "14:00"},
			},
		},
		{
			name: "all closed",
			in: map[string]*BusinessHoursEntry{
				"monday":    nil,
				"tuesday":   nil,
				"wednesday": nil,
				"thursday":  nil,
				"friday":    nil,
				"saturday":  nil,
				"sunday":    nil,
			},
			want: map[string]BusinessHoursEntry{},
		},
		{
			name: "unknown day name",
			in: map[string]*BusinessHoursEntry{
				"lunes": {Open: "09:00", Close: "18:00"},
			},
			wantErr: true,
			errSub:  "día desconocido",
		},
		{
			name: "bad open time",
			in: map[string]*BusinessHoursEntry{
				"monday": {Open: "25:00", Close: "18:00"},
			},
			wantErr: true,
			errSub:  "HH:MM",
		},
		{
			name: "bad close time",
			in: map[string]*BusinessHoursEntry{
				"monday": {Open: "09:00", Close: "18:99"},
			},
			wantErr: true,
			errSub:  "HH:MM",
		},
		{
			name: "open after close",
			in: map[string]*BusinessHoursEntry{
				"monday": {Open: "18:00", Close: "09:00"},
			},
			wantErr: true,
			errSub:  "apertura debe ser anterior al cierre",
		},
		{
			name: "open equals close",
			in: map[string]*BusinessHoursEntry{
				"tuesday": {Open: "09:00", Close: "09:00"},
			},
			wantErr: true,
			errSub:  "apertura debe ser anterior al cierre",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotJSON, err := mapBusinessHours(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.errSub)
				}
				if tt.errSub != "" && !strings.Contains(err.Error(), tt.errSub) {
					t.Fatalf("error %q does not contain %q", err.Error(), tt.errSub)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			var got map[string]BusinessHoursEntry
			if err := json.Unmarshal([]byte(gotJSON), &got); err != nil {
				t.Fatalf("unmarshal mapped hours: %v", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("got %d keys, want %d", len(got), len(tt.want))
			}
			for k, v := range tt.want {
				g, ok := got[k]
				if !ok {
					t.Errorf("missing key %q", k)
					continue
				}
				if g.Open != v.Open || g.Close != v.Close {
					t.Errorf("key %q = %+v, want %+v", k, g, v)
				}
			}
			for k := range got {
				if _, ok := tt.want[k]; !ok {
					t.Errorf("unexpected key %q", k)
				}
			}
		})
	}
}

func TestMapBusinessHours_EntityRoundTrip(t *testing.T) {
	in := map[string]*BusinessHoursEntry{
		"monday": {Open: "09:00", Close: "18:00"},
		"sunday": nil,
	}
	mapped, err := mapBusinessHours(in)
	if err != nil {
		t.Fatalf("mapBusinessHours failed: %v", err)
	}

	bp := entity.BusinessProfile{BusinessHours: mapped}
	if !bp.IsOpenOn(1) {
		t.Errorf("Monday should be open")
	}
	open, close, ok := bp.GetOpenClose(1)
	if !ok {
		t.Fatal("GetOpenClose(1) should be ok")
	}
	if open != "09:00" || close != "18:00" {
		t.Errorf("GetOpenClose(1) = %s-%s, want 09:00-18:00", open, close)
	}
	if bp.IsOpenOn(7) {
		t.Errorf("Sunday should be closed")
	}
}

func TestMapBusinessHours_Deterministic(t *testing.T) {
	in := map[string]*BusinessHoursEntry{
		"friday":  {Open: "09:00", Close: "18:00"},
		"monday":  {Open: "09:00", Close: "18:00"},
		"tuesday": {Open: "09:00", Close: "18:00"},
	}
	first, err := mapBusinessHours(in)
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	second, err := mapBusinessHours(in)
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if first != second {
		t.Errorf("mapBusinessHours is not deterministic:\n%s\nvs\n%s", first, second)
	}
}
