package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/egkike/mcp-appointments-crm/internal/domain/entity"
)

const maxSetupFileBytes = 1 << 20 // 1 MiB

var (
	dayNameToNumber = map[string]int{
		"monday":    1,
		"tuesday":   2,
		"wednesday": 3,
		"thursday":  4,
		"friday":    5,
		"saturday":  6,
		"sunday":    7,
	}
	hhmmRegex = regexp.MustCompile(`^([01]\d|2[0-3]):[0-5]\d$`)
)

// LoadSetup reads the three wizard JSON files from dir and decodes them into
// typed structs. It is strictly read-only: it never creates, writes, chmods,
// or removes files. Errors are semantic Spanish strings that name the offending
// file but never include the directory path.
func LoadSetup(dir string) (*SetupData, error) {
	var out SetupData

	files := []fileLoader{
		{name: "setup_business.json", load: func(b []byte) error { return json.Unmarshal(b, &out.Business) }},
		{name: "setup_staff.json", load: func(b []byte) error { return json.Unmarshal(b, &out.Staff) }},
		{name: "setup_services.json", load: func(b []byte) error { return json.Unmarshal(b, &out.Services) }},
	}

	for _, f := range files {
		path := filepath.Join(dir, f.name)
		data, err := readFileCapped(path, maxSetupFileBytes)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil, fmt.Errorf("el archivo de configuración %s no existe", f.name)
			}
			if errors.Is(err, errFileTooLarge) {
				return nil, fmt.Errorf("el archivo %s supera el tamaño máximo permitido (1 MiB)", f.name)
			}
			return nil, fmt.Errorf("no se pudo leer el archivo de configuración %s", f.name)
		}
		if err := f.load(data); err != nil {
			return nil, fmt.Errorf("el archivo %s tiene un formato inválido: %w", f.name, err)
		}
	}

	return &out, nil
}

// fileLoader pairs a setup filename with its typed decoding closure, so each
// file unmarshals directly into its concrete struct without reflection targets.
type fileLoader struct {
	name string
	load func([]byte) error
}

var errFileTooLarge = errors.New("el archivo supera el tamaño máximo permitido")

// readFileCapped reads path returning at most maxBytes bytes.
// If the file is larger than maxBytes it returns errFileTooLarge.
func readFileCapped(path string, maxBytes int64) ([]byte, error) {
	path = filepath.Clean(path)
	// #nosec G304 -- path is constructed from ResolveSetupDir (filepath.Clean) and a constant filename.
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	limited := io.LimitReader(f, maxBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxBytes {
		return nil, errFileTooLarge
	}
	return data, nil
}

// mapBusinessHours transforms wizard day-name keys (monday..sunday) into the
// numeric-string keys consumed by BusinessProfile ("1".."7", 1=Monday).
// A nil entry means the day is closed and produces no key. The output is a
// JSON object string with deterministic sorted keys.
func mapBusinessHours(in map[string]*BusinessHoursEntry) (string, error) {
	out := make(map[string]BusinessHoursEntry)
	for dayName, entry := range in {
		if entry == nil {
			continue
		}
		n, ok := dayNameToNumber[dayName]
		if !ok {
			return "", fmt.Errorf("business_hours contiene un día desconocido: %q", dayName)
		}
		if !hhmmRegex.MatchString(entry.Open) {
			return "", fmt.Errorf("el horario de %s debe tener formato HH:MM", dayName)
		}
		if !hhmmRegex.MatchString(entry.Close) {
			return "", fmt.Errorf("el horario de %s debe tener formato HH:MM", dayName)
		}
		if entry.Open >= entry.Close {
			return "", fmt.Errorf("el horario de %s: la hora de apertura debe ser anterior al cierre", dayName)
		}
		key := strconv.Itoa(n)
		out[key] = *entry
	}
	b, err := json.Marshal(out)
	if err != nil {
		return "", fmt.Errorf("serializar business_hours: %w", err)
	}
	return string(b), nil
}

// validateForSeed performs pre-transaction semantic checks. Every failure is
// fatal with a Spanish message and the database remains untouched.
func validateForSeed(data *SetupData) error {
	if strings.TrimSpace(data.Business.Name) == "" {
		return fmt.Errorf("el nombre del negocio no puede estar vacío")
	}
	if strings.TrimSpace(data.Business.CurrencyCode) == "" {
		return fmt.Errorf("el campo currency_code del negocio no puede estar vacío")
	}
	if strings.TrimSpace(data.Business.CurrencySymbol) == "" {
		return fmt.Errorf("el campo currency_symbol del negocio no puede estar vacío")
	}
	if strings.TrimSpace(data.Business.Timezone) == "" {
		return fmt.Errorf("el campo timezone del negocio no puede estar vacío")
	}
	if data.Business.SlotIntervalMinutes <= 0 {
		return fmt.Errorf("el intervalo de turnos debe ser mayor a 0 minutos")
	}
	if data.Business.AcceptedPaymentMethods != nil {
		for i, m := range data.Business.AcceptedPaymentMethods {
			if strings.TrimSpace(m) == "" {
				return fmt.Errorf("el método de pago en la posición %d está vacío", i)
			}
		}
	}

	mappedHours, err := mapBusinessHours(data.Business.BusinessHours)
	if err != nil {
		return err
	}

	paymentMethodsJSON, err := stringSliceToJSON(data.Business.AcceptedPaymentMethods)
	if err != nil {
		return fmt.Errorf("serializar métodos de pago: %w", err)
	}

	bp := entity.BusinessProfile{
		Name:                   data.Business.Name,
		Industry:               data.Business.Industry,
		Country:                data.Business.Country,
		Address:                data.Business.Address,
		Latitude:               data.Business.Latitude,
		Longitude:              data.Business.Longitude,
		CoverPhotoURL:          data.Business.CoverPhotoURL,
		PublicPhone:            data.Business.PublicPhone,
		MessengerPlatform:      data.Business.MessengerPlatform,
		MessengerID:            data.Business.MessengerID,
		ContactEmail:           data.Business.ContactEmail,
		WebsiteURL:             data.Business.WebsiteURL,
		GeneralDescription:     data.Business.GeneralDescription,
		AcceptedPaymentMethods: paymentMethodsJSON,
		CurrencyCode:           data.Business.CurrencyCode,
		CurrencySymbol:         data.Business.CurrencySymbol,
		Timezone:               data.Business.Timezone,
		SlotIntervalMinutes:    data.Business.SlotIntervalMinutes,
		BusinessHours:          mappedHours,
	}
	if err := bp.Validate(); err != nil {
		return err
	}

	for _, m := range data.Staff {
		specialtiesJSON, err := stringSliceToJSON(m.Specialties)
		if err != nil {
			return fmt.Errorf("serializar especialidades de %q: %w", m.Name, err)
		}
		prof := entity.Professional{
			Name:          m.Name,
			RoleSpecialty: m.RoleSpecialty,
			Status:        statusOr(m.Status),
			Email:         m.Email,
			Phone:         m.Phone,
			Specialties:   specialtiesJSON,
		}
		if err := prof.Validate(); err != nil {
			return fmt.Errorf("profesional %q: %w", m.Name, err)
		}

		for _, s := range m.Schedule {
			if s.DayOfWeek < 0 || s.DayOfWeek > 6 {
				return fmt.Errorf("el horario del profesional %q para el día %d: el día debe estar entre 0 y 6", m.Name, s.DayOfWeek)
			}
			if !hhmmRegex.MatchString(s.StartTime) {
				return fmt.Errorf("el horario del profesional %q para el día %d: la hora de inicio debe tener formato HH:MM", m.Name, s.DayOfWeek)
			}
			if !hhmmRegex.MatchString(s.EndTime) {
				return fmt.Errorf("el horario del profesional %q para el día %d: la hora de fin debe tener formato HH:MM", m.Name, s.DayOfWeek)
			}
			if s.StartTime >= s.EndTime {
				return fmt.Errorf("el horario del profesional %q para el día %d: la hora de inicio debe ser anterior a la hora de fin", m.Name, s.DayOfWeek)
			}
		}
	}

	for _, svc := range data.Services {
		if strings.TrimSpace(svc.Name) == "" {
			return fmt.Errorf("el servicio %q: el nombre no puede estar vacío", svc.Name)
		}
		if svc.DurationMinutes <= 0 {
			return fmt.Errorf("el servicio %q: la duración debe ser mayor a 0 minutos", svc.Name)
		}
		if svc.Price < 0 {
			return fmt.Errorf("el servicio %q: el precio no puede ser negativo", svc.Name)
		}
		if svc.IsActive != 0 && svc.IsActive != 1 {
			return fmt.Errorf("el servicio %q: el campo is_active debe ser 0 o 1", svc.Name)
		}
	}

	return nil
}

// statusOr returns the given status if non-empty, otherwise "active".
func statusOr(status string) string {
	if strings.TrimSpace(status) == "" {
		return "active"
	}
	return status
}

// stringSliceToJSON returns nil for a nil slice (SQL NULL) or a pointer to a
// JSON-encoded array otherwise. An empty slice encodes as "[]".
func stringSliceToJSON(ss []string) (*string, error) {
	if ss == nil {
		return nil, nil
	}
	b, err := json.Marshal(ss)
	if err != nil {
		return nil, err
	}
	s := string(b)
	return &s, nil
}
