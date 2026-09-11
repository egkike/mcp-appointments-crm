package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadSetup_HappyPath(t *testing.T) {
	dir := "testdata"
	data, err := LoadSetup(dir)
	if err != nil {
		t.Fatalf("LoadSetup failed: %v", err)
	}

	if data.Business.Name != "Estudio de Belleza Rosa" {
		t.Errorf("business name = %q, want %q", data.Business.Name, "Estudio de Belleza Rosa")
	}
	if data.Business.CurrencyCode != "ARS" {
		t.Errorf("currency code = %q, want %q", data.Business.CurrencyCode, "ARS")
	}
	if len(data.Business.AcceptedPaymentMethods) != 3 {
		t.Errorf("payment methods count = %d, want 3", len(data.Business.AcceptedPaymentMethods))
	}
	if data.Business.BusinessHours["monday"] == nil {
		t.Errorf("monday hours missing")
	}
	if data.Business.BusinessHours["sunday"] != nil {
		t.Errorf("sunday hours should be nil (closed)")
	}

	if len(data.Staff) != 1 {
		t.Fatalf("staff count = %d, want 1", len(data.Staff))
	}
	if data.Staff[0].Name != "María López" {
		t.Errorf("staff name = %q, want %q", data.Staff[0].Name, "María López")
	}
	if len(data.Staff[0].Schedule) != 2 {
		t.Errorf("schedule count = %d, want 2", len(data.Staff[0].Schedule))
	}

	if len(data.Services) != 2 {
		t.Fatalf("services count = %d, want 2", len(data.Services))
	}
	if data.Services[0].Name != "Corte de cabello" {
		t.Errorf("service[0] name = %q, want %q", data.Services[0].Name, "Corte de cabello")
	}
	if data.Services[1].Price != 0 {
		t.Errorf("service[1] price = %v, want 0", data.Services[1].Price)
	}
	if data.Services[1].IsActive != 1 {
		t.Errorf("service[1] is_active = %d, want 1", data.Services[1].IsActive)
	}
}

func TestLoadSetup_MissingFile(t *testing.T) {
	dir := t.TempDir()
	mustCopy(t, filepath.Join("testdata", "setup_business.json"), filepath.Join(dir, "setup_business.json"))
	mustCopy(t, filepath.Join("testdata", "setup_services.json"), filepath.Join(dir, "setup_services.json"))
	// setup_staff.json intentionally missing

	_, err := LoadSetup(dir)
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "setup_staff.json") {
		t.Errorf("error %q does not contain filename", msg)
	}
	if !strings.Contains(msg, "no existe") {
		t.Errorf("error %q does not indicate missing file", msg)
	}
}

func TestLoadSetup_MalformedFile(t *testing.T) {
	dir := t.TempDir()
	mustCopy(t, filepath.Join("testdata", "setup_business.json"), filepath.Join(dir, "setup_business.json"))
	mustCopy(t, filepath.Join("testdata", "setup_staff.json"), filepath.Join(dir, "setup_staff.json"))
	if err := os.WriteFile(filepath.Join(dir, "setup_services.json"), []byte("not json"), 0600); err != nil {
		t.Fatalf("write malformed file: %v", err)
	}

	_, err := LoadSetup(dir)
	if err == nil {
		t.Fatal("expected error for malformed file, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "setup_services.json") {
		t.Errorf("error %q does not contain filename", msg)
	}
	if !strings.Contains(msg, "formato inválido") {
		t.Errorf("error %q does not indicate invalid format", msg)
	}
}

func TestLoadSetup_OversizedFile(t *testing.T) {
	dir := t.TempDir()
	mustCopy(t, filepath.Join("testdata", "setup_staff.json"), filepath.Join(dir, "setup_staff.json"))
	mustCopy(t, filepath.Join("testdata", "setup_services.json"), filepath.Join(dir, "setup_services.json"))

	large := make([]byte, 1<<20+1)
	for i := range large {
		large[i] = 'a'
	}
	if err := os.WriteFile(filepath.Join(dir, "setup_business.json"), large, 0600); err != nil {
		t.Fatalf("write oversized file: %v", err)
	}

	_, err := LoadSetup(dir)
	if err == nil {
		t.Fatal("expected error for oversized file, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "setup_business.json") {
		t.Errorf("error %q does not contain filename", msg)
	}
	if !strings.Contains(msg, "supera el tamaño máximo permitido") {
		t.Errorf("error %q does not indicate size limit", msg)
	}
}

func TestLoadSetup_ReadOnly(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"setup_business.json", "setup_staff.json", "setup_services.json"} {
		mustCopy(t, filepath.Join("testdata", name), filepath.Join(dir, name))
	}

	before := map[string]struct {
		data []byte
		mod  os.FileInfo
	}{}
	for _, name := range []string{"setup_business.json", "setup_staff.json", "setup_services.json"} {
		path := filepath.Join(dir, name)
		// #nosec G304 -- path is inside t.TempDir() created by the test.
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat %s: %v", name, err)
		}
		before[name] = struct {
			data []byte
			mod  os.FileInfo
		}{data: data, mod: info}
	}

	if _, err := LoadSetup(dir); err != nil {
		t.Fatalf("LoadSetup failed: %v", err)
	}

	for _, name := range []string{"setup_business.json", "setup_staff.json", "setup_services.json"} {
		path := filepath.Join(dir, name)
		// #nosec G304 -- path is inside t.TempDir() created by the test.
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s after load: %v", name, err)
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat %s after load: %v", name, err)
		}
		if string(data) != string(before[name].data) {
			t.Errorf("%s content changed after load", name)
		}
		if !info.ModTime().Equal(before[name].mod.ModTime()) {
			t.Errorf("%s ModTime changed after load", name)
		}
	}
}

func TestLoadSetup_DecodeFixtures(t *testing.T) {
	for _, name := range []string{"setup_business.json", "setup_staff.json", "setup_services.json"} {
		path := filepath.Join("testdata", name)
		// #nosec G304 -- path is a constant testdata filename.
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		switch name {
		case "setup_business.json":
			var business SetupBusiness
			if err := json.Unmarshal(data, &business); err != nil {
				t.Errorf("decode %s: %v", name, err)
			}
		case "setup_staff.json":
			var staff []SetupStaffMember
			if err := json.Unmarshal(data, &staff); err != nil {
				t.Errorf("decode %s: %v", name, err)
			}
		case "setup_services.json":
			var services []SetupService
			if err := json.Unmarshal(data, &services); err != nil {
				t.Errorf("decode %s: %v", name, err)
			}
		}
	}
}

func TestValidateForSeed(t *testing.T) {
	validBusiness := func() SetupBusiness {
		return SetupBusiness{
			Name:                "Estudio Rosa",
			CurrencyCode:        "ARS",
			CurrencySymbol:      "$",
			Timezone:            "America/Argentina/Buenos_Aires",
			SlotIntervalMinutes: 30,
			BusinessHours: map[string]*BusinessHoursEntry{
				"monday": {Open: "09:00", Close: "18:00"},
			},
		}
	}
	validData := func() *SetupData {
		return &SetupData{
			Business: validBusiness(),
			Staff: []SetupStaffMember{
				{
					Name:   "María",
					Status: "active",
					Schedule: []SetupScheduleEntry{
						{DayOfWeek: 1, StartTime: "09:00", EndTime: "18:00"},
					},
				},
			},
			Services: []SetupService{
				{Name: "Corte", DurationMinutes: 60, Price: 100, IsActive: 1},
			},
		}
	}

	tests := []struct {
		name    string
		mutate  func(*SetupData)
		wantErr string
	}{
		{
			name:    "valid",
			mutate:  func(*SetupData) {},
			wantErr: "",
		},
		{
			name: "empty business name",
			mutate: func(d *SetupData) {
				d.Business.Name = ""
			},
			wantErr: "nombre del negocio no puede estar vacío",
		},
		{
			name: "negative price service",
			mutate: func(d *SetupData) {
				d.Services[0].Price = -1
			},
			wantErr: "precio no puede ser negativo",
		},
		{
			name: "is_active not 0 or 1",
			mutate: func(d *SetupData) {
				d.Services[0].IsActive = 2
			},
			wantErr: "is_active debe ser 0 o 1",
		},
		{
			name: "duplicate day in schedule",
			mutate: func(d *SetupData) {
				d.Staff[0].Schedule = append(d.Staff[0].Schedule, SetupScheduleEntry{DayOfWeek: 1, StartTime: "10:00", EndTime: "11:00"})
			},
			wantErr: "más de un horario para el día",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := validData()
			tt.mutate(d)
			err := validateForSeed(d)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("expected no error, got %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error %q does not contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func mustCopy(t *testing.T, src, dst string) {
	t.Helper()
	// #nosec G304 -- src is a constant testdata path; dst is inside t.TempDir().
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("read %s: %v", src, err)
	}
	// #nosec G703 -- dst is inside t.TempDir() created by the test.
	if err := os.WriteFile(dst, data, 0600); err != nil {
		t.Fatalf("write %s: %v", dst, err)
	}
}
