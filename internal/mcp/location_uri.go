package mcp

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/egkike/mcp-appointments-crm/internal/domain"
)

// Location-contract semantic messages (feat-whatsapp-bot D4). The malformed and
// mixed-numeric strings are pinned verbatim by design.md §3; the range and
// clear-flag rows have no pinned wording, so they follow the same "Error: "
// LLM-facing convention as the other semantic errors of this change.
const (
	msgLocationURIMalformed = "Error: la ubicación no tiene un formato válido. Usá geo:lat,long (por ejemplo geo:-34.6037,-58.3816)."
	msgLocationURIRange     = "Error: las coordenadas de la ubicación están fuera de rango (latitud -90..90, longitud -180..180)."
	msgLocationURIMixed     = "Error: no puedo combinar una ubicación geo: con latitude/longitude numéricas."
	msgLocationURIClear     = "Error: no puedo combinar una ubicación geo: con el borrado de latitude/longitude."
)

// geoCoordinatePattern is the strict numeric shape accepted inside a `geo:`
// URI: an optional sign, at least one integer digit and an optional single
// fractional part. It deliberately rejects the shapes strconv.ParseFloat would
// otherwise accept — scientific notation ("1e-7"), hexadecimal floats
// ("0x1p-2"), Inf/NaN and any surrounding whitespace — because RFC 5870
// coordinates are plain decimals and rejecting an unexpected shape is safer
// than silently reinterpreting it (design §3).
var geoCoordinatePattern = regexp.MustCompile(`^[+-]?[0-9]+(\.[0-9]+)?$`)

// parseGeoURI parses the strict RFC 5870 subset `geo:lat,long` pinned by the
// business-profile delta: no whitespace, no `;u=` parameters, exactly two
// decimal coordinates. The returned error is always a *domain.SemanticError, so
// the handler maps it to -32002 through the errors.go boundary without leaking
// parsing internals. The range guard below is defense-in-depth for the URI path
// only: the authoritative invariant lives in entity.BusinessProfile.Validate,
// which also covers raw numeric latitude/longitude.
func parseGeoURI(uri string) (latitude, longitude float64, err error) {
	body, ok := strings.CutPrefix(uri, "geo:")
	if !ok {
		return 0, 0, malformedGeoURI()
	}
	latToken, longToken, ok := strings.Cut(body, ",")
	if !ok || !geoCoordinatePattern.MatchString(latToken) || !geoCoordinatePattern.MatchString(longToken) {
		return 0, 0, malformedGeoURI()
	}
	if latitude, err = strconv.ParseFloat(latToken, 64); err != nil {
		return 0, 0, malformedGeoURI()
	}
	if longitude, err = strconv.ParseFloat(longToken, 64); err != nil {
		return 0, 0, malformedGeoURI()
	}
	if latitude < -90 || latitude > 90 || longitude < -180 || longitude > 180 {
		return 0, 0, &domain.SemanticError{Code: domain.ErrCodeInvalidInput, Message: msgLocationURIRange}
	}
	return latitude, longitude, nil
}

// malformedGeoURI returns the semantic error for every shape violation of the
// `geo:lat,long` grammar. Keeping it in one place means the message cannot
// drift between the parser's early exits.
func malformedGeoURI() error {
	return &domain.SemanticError{Code: domain.ErrCodeInvalidInput, Message: msgLocationURIMalformed}
}
