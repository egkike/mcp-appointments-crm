package mcp

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/egkike/mcp-appointments-crm/internal/application/dto"
	"github.com/egkike/mcp-appointments-crm/internal/domain/entity"
)

// Location-contract tests for update_business_profile (feat-whatsapp-bot D4):
// `location_uri` is a transport-level RFC 5870 `geo:lat,long` alias that fills
// the numeric latitude/longitude fields before the use case runs. These tests
// pin the wire behavior; the messages below are the contract the LLM sees, so
// they are written out verbatim rather than referenced from the implementation.

const (
	wantGeoMalformed    = "Error: la ubicación no tiene un formato válido. Usá geo:lat,long (por ejemplo geo:-34.6037,-58.3816)."
	wantGeoOutOfRange   = "Error: las coordenadas de la ubicación están fuera de rango (latitud -90..90, longitud -180..180)."
	wantGeoMixedNumeric = "Error: no puedo combinar una ubicación geo: con latitude/longitude numéricas."
	wantGeoMixedClear   = "Error: no puedo combinar una ubicación geo: con el borrado de latitude/longitude."
)

// TestToolUpdateBusinessProfileAcceptsGeoLocationURI proves a valid `geo:` URI
// is equivalent to sending the numeric pair: it reaches the use case as two
// pointers and never as a clear signal.
func TestToolUpdateBusinessProfileAcceptsGeoLocationURI(t *testing.T) {
	tests := []struct {
		name     string
		uri      string
		wantLat  float64
		wantLong float64
	}{
		{name: "signed decimals", uri: "geo:-34.6037,-58.3816", wantLat: -34.6037, wantLong: -58.3816},
		{name: "leading plus signs", uri: "geo:+34.6037,+58.3816", wantLat: 34.6037, wantLong: 58.3816},
		{name: "asymmetric signs", uri: "geo:-34.6037,+58.3816", wantLat: -34.6037, wantLong: 58.3816},
		{name: "plain integers", uri: "geo:10,20", wantLat: 10, wantLong: 20},
		{name: "upper bounds", uri: "geo:90,180", wantLat: 90, wantLong: 180},
		{name: "lower bounds", uri: "geo:-90,-180", wantLat: -90, wantLong: -180},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv, ports := newToolServer(t)
			var got dto.UpdateBusinessProfileInput
			called := false
			ports.updateProfile.executeFn = func(_ context.Context, in dto.UpdateBusinessProfileInput) (*entity.BusinessProfile, error) {
				got, called = in, true
				return &entity.BusinessProfile{
					ID: "singleton", Name: "Mi Negocio",
					Latitude: in.Latitude, Longitude: in.Longitude,
				}, nil
			}

			resp := decodeToolResponse(t, callTool(srv.Handler(), ownerCallerPtr(), "update_business_profile",
				fmt.Sprintf(`{"location_uri":%q}`, tt.uri)))
			wantStructured(t, resp)
			if !called {
				t.Fatal("the use case port was never called: the geo: payload did not reach the handler")
			}
			if got.Latitude == nil || *got.Latitude != tt.wantLat {
				t.Errorf("input.Latitude = %v; want %v", got.Latitude, tt.wantLat)
			}
			if got.Longitude == nil || *got.Longitude != tt.wantLong {
				t.Errorf("input.Longitude = %v; want %v", got.Longitude, tt.wantLong)
			}
			if got.ClearLatitude || got.ClearLongitude {
				t.Errorf("geo: input must set the coordinates, not clear them: %+v", got)
			}
		})
	}
}

// TestToolUpdateBusinessProfileRejectsMalformedLocationURI pins the strict
// grammar: only `geo:` + two decimal numbers survives, and a rejected payload
// never reaches the use case (no partial write).
func TestToolUpdateBusinessProfileRejectsMalformedLocationURI(t *testing.T) {
	tests := []struct {
		name string
		uri  string
	}{
		{name: "wrong scheme", uri: "https://maps.google.com/?q=1,2"},
		{name: "missing scheme", uri: "-34.6037,-58.3816"},
		{name: "empty string", uri: ""},
		{name: "scheme only", uri: "geo:"},
		{name: "missing longitude", uri: "geo:-34.6037"},
		{name: "missing latitude", uri: "geo:,-58.3816"},
		{name: "non numeric", uri: "geo:abc,def"},
		{name: "separator whitespace", uri: "geo:-34.6037, -58.3816"},
		{name: "leading whitespace", uri: "geo: -34.6037,-58.3816"},
		{name: "trailing whitespace", uri: "geo:-34.6037,-58.3816 "},
		{name: "scientific notation", uri: "geo:1e-7,2"},
		{name: "hex float", uri: "geo:0x1p-2,2"},
		{name: "NaN", uri: "geo:NaN,2"},
		{name: "Infinity", uri: "geo:+Inf,2"},
		{name: "trailing dot", uri: "geo:1.,2"},
		{name: "three components", uri: "geo:1,2,3"},
		{name: "uncertainty parameter", uri: "geo:-34.6037,-58.3816;u=100"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv, ports := newToolServer(t)
			called := false
			ports.updateProfile.executeFn = func(_ context.Context, _ dto.UpdateBusinessProfileInput) (*entity.BusinessProfile, error) {
				called = true
				return nil, errors.New("port must not be called")
			}

			resp := decodeToolResponse(t, callTool(srv.Handler(), ownerCallerPtr(), "update_business_profile",
				fmt.Sprintf(`{"location_uri":%q}`, tt.uri)))
			wantErrorCode(t, resp, -32002)
			if resp.Error.Message != wantGeoMalformed {
				t.Errorf("error.message = %q; want %q", resp.Error.Message, wantGeoMalformed)
			}
			if called {
				t.Error("a rejected geo: URI must not reach the use case (no partial write)")
			}
		})
	}
}

// TestToolUpdateBusinessProfileRejectsOutOfRangeLocationURI pins the range
// guard: a syntactically valid URI whose coordinates fall outside the WGS-84
// bounds is a semantic error, not a stored value.
func TestToolUpdateBusinessProfileRejectsOutOfRangeLocationURI(t *testing.T) {
	tests := []struct {
		name string
		uri  string
	}{
		{name: "latitude above 90", uri: "geo:90.000001,0"},
		{name: "latitude below -90", uri: "geo:-90.000001,0"},
		{name: "longitude above 180", uri: "geo:0,180.000001"},
		{name: "longitude below -180", uri: "geo:0,-180.000001"},
		{name: "spec example out of range", uri: "geo:120.0,10.0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv, ports := newToolServer(t)
			called := false
			ports.updateProfile.executeFn = func(_ context.Context, _ dto.UpdateBusinessProfileInput) (*entity.BusinessProfile, error) {
				called = true
				return nil, errors.New("port must not be called")
			}

			resp := decodeToolResponse(t, callTool(srv.Handler(), ownerCallerPtr(), "update_business_profile",
				fmt.Sprintf(`{"location_uri":%q}`, tt.uri)))
			wantErrorCode(t, resp, -32002)
			if resp.Error.Message != wantGeoOutOfRange {
				t.Errorf("error.message = %q; want %q", resp.Error.Message, wantGeoOutOfRange)
			}
			if called {
				t.Error("an out-of-range geo: URI must not reach the use case (no partial write)")
			}
		})
	}
}

// TestToolUpdateBusinessProfileRejectsLocationURIConflicts pins the two
// exclusivity rules: `location_uri` cannot be combined with the numeric pair
// nor with the F-4 clear flags. Both are semantic errors and the payload never
// reaches the use case, so no partial write can happen.
func TestToolUpdateBusinessProfileRejectsLocationURIConflicts(t *testing.T) {
	const geo = `"location_uri":"geo:-34.6037,-58.3816"`
	tests := []struct {
		name    string
		args    string
		wantMsg string
	}{
		{
			name:    "with numeric latitude",
			args:    `{` + geo + `,"latitude":-34.6037}`,
			wantMsg: wantGeoMixedNumeric,
		},
		{
			name:    "with numeric longitude",
			args:    `{` + geo + `,"longitude":-58.3816}`,
			wantMsg: wantGeoMixedNumeric,
		},
		{
			name:    "with both numeric coordinates",
			args:    `{` + geo + `,"latitude":-34.6037,"longitude":-58.3816}`,
			wantMsg: wantGeoMixedNumeric,
		},
		{
			name:    "with latitude clear flag",
			args:    `{` + geo + `,"latitude":null}`,
			wantMsg: wantGeoMixedClear,
		},
		{
			name:    "with longitude clear flag",
			args:    `{` + geo + `,"longitude":null}`,
			wantMsg: wantGeoMixedClear,
		},
		{
			name:    "with both clear flags",
			args:    `{` + geo + `,"latitude":null,"longitude":null}`,
			wantMsg: wantGeoMixedClear,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv, ports := newToolServer(t)
			called := false
			ports.updateProfile.executeFn = func(_ context.Context, _ dto.UpdateBusinessProfileInput) (*entity.BusinessProfile, error) {
				called = true
				return nil, errors.New("port must not be called")
			}

			resp := decodeToolResponse(t, callTool(srv.Handler(), ownerCallerPtr(), "update_business_profile", tt.args))
			wantErrorCode(t, resp, -32002)
			if resp.Error.Message != tt.wantMsg {
				t.Errorf("error.message = %q; want %q", resp.Error.Message, tt.wantMsg)
			}
			if called {
				t.Error("a conflicting location payload must not reach the use case (no partial write)")
			}
		})
	}
}

// TestToolUpdateBusinessProfileNullLocationURIIsIgnored is the triangulation
// case: an explicit JSON null means "no URI supplied" and must follow the
// ordinary partial-merge path instead of being treated as an empty string.
func TestToolUpdateBusinessProfileNullLocationURIIsIgnored(t *testing.T) {
	srv, ports := newToolServer(t)
	var got dto.UpdateBusinessProfileInput
	called := false
	ports.updateProfile.executeFn = func(_ context.Context, in dto.UpdateBusinessProfileInput) (*entity.BusinessProfile, error) {
		got, called = in, true
		return &entity.BusinessProfile{ID: "singleton", Name: "Mi Negocio"}, nil
	}

	resp := decodeToolResponse(t, callTool(srv.Handler(), ownerCallerPtr(), "update_business_profile",
		`{"name":"Mi Negocio","location_uri":null}`))
	wantStructured(t, resp)
	if !called {
		t.Fatal("the use case port was never called: a null location_uri must not be rejected")
	}
	if got.Latitude != nil || got.Longitude != nil || got.ClearLatitude || got.ClearLongitude {
		t.Errorf("null location_uri changed the location input: %+v", got)
	}
}

// TestIntegrationMaintenanceGeoURILocation proves the contract against real
// SQLite: a geo: URI stores the same values as the numeric pair (visible via
// get_business_profile), and a conflicting payload leaves the stored
// coordinates untouched.
func TestIntegrationMaintenanceGeoURILocation(t *testing.T) {
	mux := newIntegrationMux(t)

	result := mustCallTool(t, mux, "owner-1", "update_business_profile",
		`{"location_uri":"geo:-34.6037,-58.3816"}`)
	var written businessProfileOut
	decodeToolStructured(t, result, &written)
	if written.Latitude == nil || *written.Latitude != -34.6037 ||
		written.Longitude == nil || *written.Longitude != -58.3816 {
		t.Fatalf("stored coordinates = %v/%v; want -34.6037/-58.3816", written.Latitude, written.Longitude)
	}

	// The write is the same the numeric form would have produced.
	if read := getBusinessProfile(t, mux, "owner-1"); read.Latitude == nil || *read.Latitude != -34.6037 ||
		read.Longitude == nil || *read.Longitude != -58.3816 {
		t.Fatalf("read-back coordinates = %v/%v; want the geo: values", read.Latitude, read.Longitude)
	}

	// A conflicting payload is rejected and does not touch the stored value.
	_, code, msg := callMCPTool(t, mux, "owner-1", "update_business_profile",
		`{"location_uri":"geo:10,20","latitude":10,"longitude":20}`)
	if code != -32002 || msg != wantGeoMixedNumeric {
		t.Errorf("conflict: code=%d msg=%q; want -32002 %q", code, msg, wantGeoMixedNumeric)
	}
	read := getBusinessProfile(t, mux, "owner-1")
	if read.Latitude == nil || *read.Latitude != -34.6037 ||
		read.Longitude == nil || *read.Longitude != -58.3816 {
		t.Errorf("after a rejected conflict coordinates = %v/%v; want them unchanged", read.Latitude, read.Longitude)
	}
}
