package admin

import (
	"bytes"
	"errors"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/egkike/mcp-appointments-crm/internal/domain"
	"gopkg.in/yaml.v3"
)

// This file is the framework-free core of the "configurar Hermes" capability
// (ADR-0017): it loads the existing ~/.hermes/config.yaml into a generic
// representation, sets only the mcp_servers.mcp-appointments entry (preserving
// every other key and server), atomically writes it back, and renders the
// exact YAML snippet used as on-screen fallback. No Bubble Tea and no transport
// imports live here: the TUI screens (T2) call these functions.
//
// Quoting contract (ADR-0017 Decision 2): the X-Caller-Id value is ALWAYS
// emitted double-quoted, e.g. `X-Caller-Id: "+5491100000000"`. YAML 1.1 parses
// an unquoted `+54...` as an integer, so the quotes are load-bearing for any
// consumer parser, not cosmetic. The style is forced with a *yaml.Node scalar
// instead of trusting emitter heuristics.

const (
	// HermesServerName is the only mcp_servers entry this flow writes; it is
	// the single name used across the docs (ADR-0017 Decision 3).
	HermesServerName = "mcp-appointments"

	// HermesCallerIDHeader is the header Hermes sends to identify the owner.
	HermesCallerIDHeader = "X-Caller-Id"

	// HermesMCPPath is the fixed MCP endpoint path (ADR-0017 Decision 3).
	HermesMCPPath = "/mcp"

	// HermesConfigFileName is the file inside ~/.hermes this flow edits.
	HermesConfigFileName = "config.yaml"

	// hermesDirName is the standard Hermes directory under the user home.
	hermesDirName = ".hermes"

	// hermesServersSection is the top-level YAML mapping that owns the server
	// entries. It is a constant so the merge never rewrites the section name.
	hermesServersSection = "mcp_servers"

	// hermesURLKey and hermesHeadersKey are the fixed keys of the entry this
	// flow owns; other keys of the entry are intentionally dropped on replace.
	hermesURLKey     = "url"
	hermesHeadersKey = "headers"

	// hermesDirMode and hermesFileMode restrict the Hermes config we create to
	// the OS user that owns the installation: the file carries identity
	// material (the owner phone), not shared configuration. They are applied
	// ONLY when this flow creates the directory/file; a pre-existing Hermes
	// directory is never chmod-ed (ADR-0017 Decision 1).
	hermesDirMode  = 0o700
	hermesFileMode = 0o600

	// hermesYAMLIndent matches the 2-space layout documented in ADR-0017. The
	// same indent is used by the writer and the snippet renderer so the snippet
	// is byte-identical to the block the writer emits.
	hermesYAMLIndent = 2
)

// HermesDocument is a generic representation of a Hermes config.yaml. It is
// intentionally a named map rather than a fixed struct: the merge must
// preserve every key this flow does not own (other servers, other top-level
// settings) and we do not control Hermes's schema. Only the leaf values that
// this flow sets are concrete (strings plus a *yaml.Node for the quoted phone).
type HermesDocument map[string]any

// HermesConfigPath returns the default Hermes configuration path:
// ~/.hermes/config.yaml. The caller resolves it once and passes the path
// explicitly into the load/write functions so tests never touch a real home.
func HermesConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		// Fail secure: without a home directory there is no safe place to read
		// or write the Hermes config, and guessing one could edit an unrelated
		// file. The message never leaks a path.
		return "", &domain.SemanticError{
			Code:    domain.ErrCodeInternal,
			Message: "no se pudo resolver la configuración de Hermes: no hay directorio home",
			Cause:   err,
		}
	}
	return filepath.Join(home, hermesDirName, HermesConfigFileName), nil
}

// ValidateHermesURL validates the MCP endpoint URL that Hermes will call. It is
// format-only by design (ADR-0017): net/url.Parse plus an http/https scheme and
// a non-empty host. Loopback checks are warn-only and live outside this core.
func ValidateHermesURL(rawURL string) error {
	trimmed := strings.TrimSpace(rawURL)
	if trimmed == "" {
		return &domain.SemanticError{
			Code:    domain.ErrCodeInvalidInput,
			Message: "la URL del endpoint MCP no puede estar vacía",
		}
	}

	parsed, err := url.Parse(trimmed)
	if err != nil {
		return &domain.SemanticError{
			Code:    domain.ErrCodeInvalidInput,
			Message: "la URL del endpoint MCP no es válida",
			Cause:   err,
		}
	}

	switch parsed.Scheme {
	case "http", "https":
	default:
		return &domain.SemanticError{
			Code:    domain.ErrCodeInvalidInput,
			Message: "la URL del endpoint MCP debe usar http o https",
		}
	}

	if parsed.Host == "" {
		return &domain.SemanticError{
			Code:    domain.ErrCodeInvalidInput,
			Message: "la URL del endpoint MCP debe incluir un host",
		}
	}
	return nil
}

// HermesEndpointURL builds the MCP endpoint URL from the server bind/port, with
// the path pinned to HermesMCPPath. The URL is derived from the running server
// configuration (MCP_BIND/MCP_PORT) and is never hardcoded by callers.
func HermesEndpointURL(bind, port string) (string, error) {
	bind = strings.TrimSpace(bind)
	port = strings.TrimSpace(port)
	if bind == "" || port == "" {
		return "", &domain.SemanticError{
			Code:    domain.ErrCodeInvalidInput,
			Message: "no se pudo armar la URL del endpoint MCP: bind y puerto son obligatorios",
		}
	}

	// net.JoinHostPort brackets IPv6 literals (::1) correctly.
	endpoint := url.URL{
		Scheme: "http",
		Host:   net.JoinHostPort(bind, port),
		Path:   HermesMCPPath,
	}
	rawURL := endpoint.String()
	if err := ValidateHermesURL(rawURL); err != nil {
		return "", err
	}
	return rawURL, nil
}

// LoadHermesConfig reads path into a generic document. A missing file is NOT an
// error: it returns a non-nil empty document so the flow can create the file
// (ADR-0017 Decision 1). An unreadable or malformed file is reported as a
// *domain.SemanticError; the operator message never contains a filesystem path.
func LoadHermesConfig(path string) (HermesDocument, error) {
	// #nosec G304 -- path is the resolved Hermes config path (constant filename
	// under ~/.hermes, or an explicit caller path), not attacker-controlled input.
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return HermesDocument{}, nil
		}
		return nil, &domain.SemanticError{
			Code:    domain.ErrCodeInternal,
			Message: "no se pudo leer la configuración de Hermes",
			Cause:   err,
		}
	}

	if len(bytes.TrimSpace(data)) == 0 {
		return HermesDocument{}, nil
	}

	var raw map[string]any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, &domain.SemanticError{
			Code:    domain.ErrCodeInvalidInput,
			Message: "la configuración de Hermes no es YAML válido",
			Cause:   err,
		}
	}
	if raw == nil {
		return HermesDocument{}, nil
	}
	return HermesDocument(raw), nil
}

// SetHermesServer sets (or replaces) the mcp_servers.<HermesServerName> entry
// to {url, headers{X-Caller-Id}} while preserving every other key and server.
//
// It validates the URL and the phone before touching the document. The phone is
// validated through ValidatePhone — the single phone validation point of the
// codebase — never a copy.
//
// The entry is replaced wholesale, so re-running the flow is idempotent and
// stale keys inside the previous entry cannot survive.
func (d HermesDocument) SetHermesServer(rawURL, phone string) error {
	if err := ValidateHermesURL(rawURL); err != nil {
		return err
	}
	if err := ValidatePhone(phone); err != nil {
		return err
	}
	if d == nil {
		return &domain.SemanticError{
			Code:    domain.ErrCodeInternal,
			Message: "el documento de configuración de Hermes no está inicializado",
		}
	}

	servers, err := hermesMapping(d, hermesServersSection)
	if err != nil {
		return err
	}
	servers[HermesServerName] = hermesServerEntry(strings.TrimSpace(rawURL), phone)
	return nil
}

// RenderHermesSnippet returns the exact YAML block the writer would emit for
// the entry, used as the on-screen fallback when the write is impossible
// (ADR-0017 Decision 1). It validates the URL and the phone with the same
// functions SetHermesServer uses, so a snippet can never advertise a value the
// writer would reject.
func RenderHermesSnippet(rawURL, phone string) (string, error) {
	if err := ValidateHermesURL(rawURL); err != nil {
		return "", err
	}
	if err := ValidatePhone(phone); err != nil {
		return "", err
	}

	doc := HermesDocument{
		hermesServersSection: map[string]any{
			HermesServerName: hermesServerEntry(strings.TrimSpace(rawURL), phone),
		},
	}
	data, err := marshalHermesDocument(doc)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// WriteHermesConfig writes doc to path atomically: a temp file in the same
// directory followed by a rename, so a crash or a full disk never leaves a
// half-written Hermes config.
//
// Mode discipline (ADR-0017 Decision 1): the directory is created 0700 and the
// file 0600 ONLY when this flow creates them. A pre-existing Hermes directory
// is never chmod-ed. The final path is replaced by the rename; no explicit
// chmod touches a file we did not create in this call.
func WriteHermesConfig(path string, doc HermesDocument) error {
	if strings.TrimSpace(path) == "" {
		return &domain.SemanticError{
			Code:    domain.ErrCodeInvalidInput,
			Message: "no se indicó dónde escribir la configuración de Hermes",
		}
	}

	data, err := marshalHermesDocument(doc)
	if err != nil {
		return err
	}

	dir := filepath.Dir(path)
	if err := ensureHermesDir(dir); err != nil {
		return err
	}

	tmp, err := os.CreateTemp(dir, ".hermes-config-*.tmp")
	if err != nil {
		return &domain.SemanticError{
			Code:    domain.ErrCodeInternal,
			Message: "no se pudo crear el archivo temporal de la configuración de Hermes",
			Cause:   err,
		}
	}
	tmpPath := tmp.Name()

	// Any failure before the rename must remove the temp file: leaving it behind
	// would both litter the Hermes directory and keep a copy of the owner phone.
	written := false
	defer func() {
		if !written {
			_ = tmp.Close()
			_ = os.Remove(tmpPath)
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		return &domain.SemanticError{
			Code:    domain.ErrCodeInternal,
			Message: "no se pudo escribir la configuración de Hermes",
			Cause:   err,
		}
	}
	// os.CreateTemp already requests 0600, but an inherited umask could loosen
	// it; this file is ours, so tightening it here is safe and deterministic.
	if err := tmp.Chmod(hermesFileMode); err != nil {
		return &domain.SemanticError{
			Code:    domain.ErrCodeInternal,
			Message: "no se pudieron restringir los permisos de la configuración de Hermes",
			Cause:   err,
		}
	}
	if err := tmp.Sync(); err != nil {
		return &domain.SemanticError{
			Code:    domain.ErrCodeInternal,
			Message: "no se pudo sincronizar la configuración de Hermes",
			Cause:   err,
		}
	}
	if err := tmp.Close(); err != nil {
		return &domain.SemanticError{
			Code:    domain.ErrCodeInternal,
			Message: "no se pudo cerrar la configuración de Hermes",
			Cause:   err,
		}
	}

	if err := os.Rename(tmpPath, path); err != nil {
		return &domain.SemanticError{
			Code:    domain.ErrCodeInternal,
			Message: "no se pudo reemplazar la configuración de Hermes",
			Cause:   err,
		}
	}
	written = true
	return nil
}

// ensureHermesDir guarantees dir exists, creating it 0700 only when this call
// is the one that creates it. A pre-existing directory — typically owned by
// Hermes — is left with its current mode, and a non-directory path is a
// semantic failure rather than a confusing write error.
func ensureHermesDir(dir string) error {
	info, err := os.Stat(dir)
	switch {
	case err == nil:
		if !info.IsDir() {
			return &domain.SemanticError{
				Code:    domain.ErrCodeInternal,
				Message: "la ruta de la configuración de Hermes no es un directorio",
			}
		}
		return nil
	case !errors.Is(err, os.ErrNotExist):
		return &domain.SemanticError{
			Code:    domain.ErrCodeInternal,
			Message: "no se pudo preparar el directorio de la configuración de Hermes",
			Cause:   err,
		}
	}

	if err := os.MkdirAll(dir, hermesDirMode); err != nil {
		return &domain.SemanticError{
			Code:    domain.ErrCodeInternal,
			Message: "no se pudo crear el directorio de la configuración de Hermes",
			Cause:   err,
		}
	}
	// MkdirAll passes the mode through the umask; enforce 0700 on the directory
	// this call created so identity material never lands in a group-readable
	// directory.
	if err := os.Chmod(dir, hermesDirMode); err != nil {
		return &domain.SemanticError{
			Code:    domain.ErrCodeInternal,
			Message: "no se pudieron restringir los permisos del directorio de Hermes",
			Cause:   err,
		}
	}
	return nil
}

// hermesMapping returns the mapping stored at key, creating an empty one when
// the key is absent. A present non-mapping value is a failure: overwriting it
// would silently drop data we do not own.
func hermesMapping(doc HermesDocument, key string) (map[string]any, error) {
	existing, ok := doc[key]
	if !ok || existing == nil {
		section := map[string]any{}
		doc[key] = section
		return section, nil
	}

	section, ok := existing.(map[string]any)
	if !ok {
		return nil, &domain.SemanticError{
			Code:    domain.ErrCodeInvalidInput,
			Message: "la sección " + key + " de la configuración de Hermes no es un mapa; no se puede combinar sin perder datos",
		}
	}
	return section, nil
}

// hermesServerEntry builds the single entry this flow owns. Only the two keys
// of the contract are present; a previous entry is replaced entirely.
func hermesServerEntry(rawURL, phone string) map[string]any {
	return map[string]any{
		hermesURLKey: rawURL,
		hermesHeadersKey: map[string]any{
			HermesCallerIDHeader: hermesQuotedScalar(phone),
		},
	}
}

// hermesQuotedScalar returns a string node forced to the double-quoted style.
// yaml.v3 honors the Style of a *yaml.Node value even when it is nested inside
// a generic map, which is what makes the quoting contract robust instead of
// emitter-dependent.
func hermesQuotedScalar(value string) *yaml.Node {
	return &yaml.Node{
		Kind:  yaml.ScalarNode,
		Tag:   "!!str",
		Style: yaml.DoubleQuotedStyle,
		Value: value,
	}
}

// marshalHermesDocument serializes doc with the shared 2-space indent. The
// writer and the snippet renderer both go through here so they cannot drift.
func marshalHermesDocument(doc HermesDocument) ([]byte, error) {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(hermesYAMLIndent)
	if err := enc.Encode(map[string]any(doc)); err != nil {
		_ = enc.Close()
		return nil, &domain.SemanticError{
			Code:    domain.ErrCodeInternal,
			Message: "no se pudo serializar la configuración de Hermes",
			Cause:   err,
		}
	}
	if err := enc.Close(); err != nil {
		return nil, &domain.SemanticError{
			Code:    domain.ErrCodeInternal,
			Message: "no se pudo serializar la configuración de Hermes",
			Cause:   err,
		}
	}
	return buf.Bytes(), nil
}
