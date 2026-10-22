package admin

import (
	"bytes"
	"errors"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/egkike/mcp-appointments-crm/internal/domain"
	"github.com/egkike/mcp-appointments-crm/internal/loopback"
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

	// hermesEnvKey is the entry key holding the environment block the Telegram
	// owner-bot values live in (feat-whatsapp-bot, D1/ADR-0018 Decision 3b).
	hermesEnvKey = "env"

	// Telegram owner-bot env keys (D1). The values are consumed by the Hermes
	// Telegram gateway; the exact consumer spelling is reconciled by the
	// gateway-contract docs (Phase 6), so these constants are the single place
	// to adjust if it differs.
	hermesTelegramTokenKey  = "MCP_TELEGRAM_BOT_TOKEN" // #nosec G101 -- env-var KEY name, never a credential value
	hermesTelegramChatIDKey = "MCP_TELEGRAM_ALLOWED_CHAT_ID"
	hermesTelegramPhoneKey  = "MCP_TELEGRAM_OWNER_PHONE"

	// Length caps for the Telegram inputs (input-validation "length" dimension):
	// generous ceilings far above real values, so hostile or pasted-garbage
	// input cannot bloat the persisted Hermes config.
	hermesTelegramChatIDMaxDigits = 20
	hermesTelegramTokenMaxLen     = 128

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

// HermesDocument is the exported handle for a Hermes config.yaml. It wraps a
// typed yaml.Node tree (ADR-0017 amended 2026-10-16, GGA round-12): the
// document keeps schema-flexible foreign keys without a single `any` value,
// the only leaves this flow writes stay concrete string values, and nodes this
// flow does not own round-trip VERBATIM — the lexical deviation of the previous
// map[string]any model is gone: comments, key order and scalar styles of
// non-owned keys survive load/write.
//
// Construct it with LoadHermesConfig or the zero value and mutate it through
// the typed methods below.
type HermesDocument struct {
	// fields is the typed node tree backing the document. Its root is nil for
	// the zero value, which the mutating methods lazily initialize.
	fields hermesDocument
}

// hermesDocument is the unexported storage of a Hermes config.yaml: a YAML
// mapping node. It stays unexported so the tree cannot be aliased outside this
// package while this flow mutates it.
type hermesDocument struct {
	root *yaml.Node
}

// storage returns the mapping node backing the document, initializing it in
// place for the zero value. A decoded empty document marshals from a
// zero-child mapping exactly as the empty map used to.
func (d *HermesDocument) storage() hermesDocument {
	if d.fields.root == nil {
		d.fields.root = &yaml.Node{Kind: yaml.MappingNode}
	}
	return d.fields
}

// putChild replaces or appends a child entry under parent, keeping the decoded
// key order untouched for everything else.
func putChild(parent *yaml.Node, key string, child *yaml.Node) {
	for i := 0; i+1 < len(parent.Content); i += 2 {
		if parent.Content[i].Value == key {
			parent.Content[i+1] = child
			return
		}
	}
	parent.Content = append(parent.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Value: key}, child)
}

// childMapping returns the mapping node stored at key under parent, creating
// an empty one when the key is absent. A present non-mapping value is a
// failure: overwriting it would silently drop data we do not own.
func childMapping(parent *yaml.Node, key string) (*yaml.Node, error) {
	if parent == nil {
		return nil, &domain.SemanticError{
			Code:    domain.ErrCodeInternal,
			Message: "el documento de Hermes no tiene una sección raíz",
		}
	}
	for i := 0; i+1 < len(parent.Content); i += 2 {
		if parent.Content[i].Value == key {
			child := parent.Content[i+1]
			if child.Kind == yaml.MappingNode {
				return child, nil
			}
			return nil, &domain.SemanticError{
				Code:    domain.ErrCodeInvalidInput,
				Message: "la sección " + key + " de la configuración de Hermes no es un mapa; no se puede combinar sin perder datos",
			}
		}
	}
	child := &yaml.Node{Kind: yaml.MappingNode}
	parent.Content = append(parent.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Value: key}, child)
	return child, nil
}

// childValue returns the value node stored at key under parent; ok is false
// when absent.
func childValue(parent *yaml.Node, key string) (*yaml.Node, bool) {
	if parent == nil {
		return nil, false
	}
	for i := 0; i+1 < len(parent.Content); i += 2 {
		if parent.Content[i].Value == key {
			return parent.Content[i+1], true
		}
	}
	return nil, false
}

// dropChild removes the key/value pair stored at key under parent, reporting
// whether the key was present. Siblings keep their order, so a removal never
// reorders the bytes it does not touch.
func dropChild(parent *yaml.Node, key string) bool {
	if parent == nil {
		return false
	}
	for i := 0; i+1 < len(parent.Content); i += 2 {
		if parent.Content[i].Value == key {
			parent.Content = append(parent.Content[:i], parent.Content[i+2:]...)
			return true
		}
	}
	return false
}

// HermesDir resolves the Hermes DIRECTORY: HERMES_HOME when set, which — per
// the install.sh contract scripts/tests/install_skill_test.sh pins — IS the
// Hermes directory itself (never a parent that contains .hermes). Without the
// override it is ~/.hermes. Fail secure: an unresolvable home is an error, not
// a guess.
func HermesDir() (string, error) {
	if dir := strings.TrimSpace(os.Getenv("HERMES_HOME")); dir != "" {
		return filepath.Clean(dir), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, hermesDirName), nil
}

// HermesConfigPath returns the default Hermes configuration path: the Hermes
// directory resolved by HermesDir plus config.yaml. The caller resolves it once
// and passes the path explicitly into the load/write functions so tests never
// touch a real home.
func HermesConfigPath() (string, error) {
	dir, err := HermesDir()
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
	return filepath.Join(dir, HermesConfigFileName), nil
}

// ValidateHermesURL validates the MCP endpoint URL that Hermes will call.
// Amended 2026-10-16 (GGA finding during feat-hermes-receptionist-skill, owner
// decision "el hardening real"): in addition to the format checks it now
// HARD-rejects non-loopback hosts, because SetHermesServer stores the owner's
// phone as the X-Caller-Id header for that endpoint — a remote host could
// receive PII. It reuses the shared loopback.Classify predicate, so this gate
// and the server's startup bind gate can never drift.
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
	// The endpoint contract is exactly one path (GGA round 10): queries,
	// fragments and a non-canonical path would send the X-Caller-Id headers to
	// an unintended local endpoint.
	if parsed.Path != HermesMCPPath {
		return &domain.SemanticError{
			Code:    domain.ErrCodeInvalidInput,
			Message: "la URL del endpoint MCP debe apuntar al path " + HermesMCPPath,
		}
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return &domain.SemanticError{
			Code:    domain.ErrCodeInvalidInput,
			Message: "la URL del endpoint MCP no admite query ni fragmento",
		}
	}

	// A numeric port must be a real TCP port (GGA rounds 11/16/17: the builder
	// validates its own compose; a URL pasted straight into the config must not
	// smuggle an unusable 0/70000/overflowing port either).
	if endpointPortInvalid(parsed.Port()) {
		return &domain.SemanticError{
			Code:    domain.ErrCodeInvalidInput,
			Message: "no se pudo armar la URL del endpoint MCP: el puerto debe estar entre 1 y 65535",
		}
	}

	// Userinfo can never be part of the endpoint: ansible://user:pass@host
	// would silently carry credentials (GGA round 9). Nothing in the Hermes
	// bootstrap contract has a legitimate userinfo segment.
	if parsed.User != nil {
		return &domain.SemanticError{
			Code:    domain.ErrCodeInvalidInput,
			Message: "la URL del endpoint MCP no admite usuario ni contraseña (userinfo)",
		}
	}

	// Loopback-only hard gate (owner decision 2026-10-16): a hostname or a
	// non-loopback IP could otherwise carry the owner phone to a remote server.
	// Bracket-stripping for IPv6 literals happens inside Classify via
	// net.ParseIP(Hostname()); Hostname() already drops the brackets.
	switch reason := loopback.Classify(parsed.Hostname()); reason {
	case loopback.OK:
		return nil
	case loopback.NotAnIP:
		return &domain.SemanticError{
			Code:    domain.ErrCodeInvalidInput,
			Message: "la URL del endpoint MCP no es alcanzable: el host " + parsed.Hostname() + " no es una IP loopback literal",
		}
	case loopback.Unspecified:
		return &domain.SemanticError{
			Code:    domain.ErrCodeInvalidInput,
			Message: "la URL del endpoint MCP no es alcanzable: el host es la dirección no especificada " + parsed.Hostname(),
		}
	default:
		return &domain.SemanticError{
			Code:    domain.ErrCodeInvalidInput,
			Message: "la URL del endpoint MCP no es alcanzable: el host " + parsed.Hostname() + " no es loopback",
		}
	}
}

// HermesEndpointURL builds the MCP endpoint URL from the server bind/port, with
// the path pinned to HermesMCPPath. The URL is derived from the running server
// configuration (MCP_BIND/MCP_PORT) and is never hardcoded by callers.
//
// The bind is validated here as a literal loopback IP through the shared
// loopback.Classify predicate, the same one the server uses at startup
// (mcp.ValidateLoopback, ADR-0007 §D4); this mirrors that semantic instead of
// importing internal/mcp, because the failure is about the endpoint URL rather
// than about opening a socket, and admin→mcp imports stay at zero by design.
func HermesEndpointURL(bind, port string) (string, error) {
	bind = strings.TrimSpace(bind)
	port = strings.TrimSpace(port)
	if bind == "" || port == "" {
		return "", &domain.SemanticError{
			Code:    domain.ErrCodeInvalidInput,
			Message: "no se pudo armar la URL del endpoint MCP: bind y puerto son obligatorios",
		}
	}
	// A numeric port must be a real TCP port (GGA round-4: "0"/"70000" produce
	// an endpoint nothing can ever listen on; GGA rounds 16/17 tightened the
	// parse). A non-numeric port already fails downstream at net/url parse and
	// keeps its own message.
	if endpointPortInvalid(port) {
		return "", &domain.SemanticError{
			Code:    domain.ErrCodeInvalidInput,
			Message: "no se pudo armar la URL del endpoint MCP: el puerto debe estar entre 1 y 65535",
		}
	}

	// A hostname is not a loopback literal; serve mode rejects it too, so a URL
	// built from it would advertise an endpoint that never comes up.
	reason := loopback.Classify(bind)
	if reason != loopback.OK {
		var message string
		switch reason {
		case loopback.NotAnIP:
			message = "la URL del endpoint MCP no es alcanzable: el bind " + bind + " no es una IP loopback literal"
		case loopback.Unspecified:
			message = "la URL del endpoint MCP no es alcanzable: el bind es la dirección no especificada " + bind
		default:
			message = "la URL del endpoint MCP no es alcanzable: el bind " + bind + " no es loopback"
		}
		return "", &domain.SemanticError{
			Code:    domain.ErrCodeInvalidInput,
			Message: message,
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

// endpointPortInvalid rejects a port that is not a real TCP port. Non-empty
// numeric or out-of-range values fail here (GGA rounds 11/16/17: ParseUint on
// 16 bits rejects read-ahead overflow the int-based check missed); a
// non-numeric port keeps its own downstream message.
func endpointPortInvalid(port string) bool {
	if port == "" {
		return false
	}
	n, err := strconv.ParseUint(port, 10, 16)
	return err != nil || n < 1
}

// LoadHermesConfig reads path into a typed yaml.Node document (HermesDocument).
// A missing file is NOT an error: it returns a usable empty document so the
// flow can create the file (ADR-0017 Decision 1). An unreadable or malformed
// file is reported as a *domain.SemanticError; the operator message never
// contains a filesystem path.
//
// Decoding preserves the node tree verbatim for keys this flow does not own —
// comments, key order and scalar styles all survive the load/write round trip
// (ADR-0017 amended 2026-10-16); owned leaves are concrete string scalars.
func LoadHermesConfig(path string) (HermesDocument, error) {
	// Fail symmetric with WriteHermesConfig: a blank path must not degrade into
	// "missing file" (os.ReadFile("") returns ENOENT and would look like an
	// empty document, silently dropping real keys on the next write).
	if strings.TrimSpace(path) == "" {
		return HermesDocument{}, &domain.SemanticError{
			Code:    domain.ErrCodeInvalidInput,
			Message: "no se indicó de dónde leer la configuración de Hermes",
		}
	}

	// #nosec G304 -- path is the resolved Hermes config path (constant filename
	// under ~/.hermes, or an explicit caller path), not attacker-controlled input.
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return HermesDocument{}, nil
		}
		return HermesDocument{}, &domain.SemanticError{
			Code:    domain.ErrCodeInternal,
			Message: "no se pudo leer la configuración de Hermes",
			Cause:   err,
		}
	}

	if len(bytes.TrimSpace(data)) == 0 {
		return HermesDocument{}, nil
	}

	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return HermesDocument{}, &domain.SemanticError{
			Code:    domain.ErrCodeInvalidInput,
			Message: "la configuración de Hermes no es YAML válido",
			Cause:   err,
		}
	}
	if root.Kind == 0 {
		return HermesDocument{}, nil
	}
	// Only a top-level mapping is a config document; scalars or sequences fail
	// with the same message the previous unmarshal-into-map decode produced
	// (a kind mismatch inside yaml.v3).
	if root.Kind == yaml.DocumentNode && len(root.Content) == 1 && root.Content[0].Kind == yaml.MappingNode {
		return HermesDocument{fields: hermesDocument{root: root.Content[0]}}, nil
	}
	if root.Kind == yaml.MappingNode {
		return HermesDocument{fields: hermesDocument{root: &root}}, nil
	}
	return HermesDocument{}, &domain.SemanticError{
		Code:    domain.ErrCodeInvalidInput,
		Message: "la configuración de Hermes no es YAML válido",
	}
}

// SetHermesServer sets (or replaces) the mcp_servers.<HermesServerName> entry
// to {url, headers{X-Caller-Id}} while preserving every other key and server.
//
// It validates the URL and the phone before touching the document. The phone is
// validated through ValidatePhone — the single phone validation point of the
// codebase — never a copy. A pre-existing Telegram env block owned by
// SetHermesTelegramOwnerBot is preserved, so the two flows compose in either
// order.
//
// The Telegram owner-bot channel is OPTIONAL (S11): when it was never configured
// — and the flow skips it — the entry renders as {url, headers{X-Caller-Id}}
// alone, with no env block. That is the shape Hermes boots with, so the server
// entry is never conditional on the channel and no write path requires it.
//
// The entry is replaced wholesale, so re-running the flow is idempotent and
// stale keys inside the previous entry cannot survive. The pointer receiver is
// required because a zero-value HermesDocument lazily allocates its backing
// mapping through storage() in place.
func (d *HermesDocument) SetHermesServer(rawURL, phone string) error {
	if err := ValidateHermesURL(rawURL); err != nil {
		return err
	}
	if err := ValidatePhone(phone); err != nil {
		return err
	}

	root := d.storage().root
	servers, err := childMapping(root, hermesServersSection)
	if err != nil {
		return err
	}
	entry := &yaml.Node{Kind: yaml.MappingNode}
	putChild(entry, hermesURLKey, plainScalar(strings.TrimSpace(rawURL)))
	headers := &yaml.Node{Kind: yaml.MappingNode}
	putChild(headers, HermesCallerIDHeader, hermesQuotedScalar(phone))
	putChild(entry, hermesHeadersKey, headers)
	// The Telegram owner-bot env block (SetHermesTelegramOwnerBot) is owned by
	// a different flow of the same entry: replacing the whole mapping here
	// would silently drop it when the operator re-runs the server flow after
	// the Telegram flow. Preserve IT verbatim; everything else in the entry is
	// replaced (wholesale idempotency applies to the keys this flow owns).
	if prev, ok := childValue(servers, HermesServerName); ok {
		// GGA round 26: a pre-existing non-mapping entry is a semantic conflict
		// for BOTH flows — SetHermesServer used to silently replace it and the
		// Telegram merge already rejects the same shape. Fail with the same
		// wording, leaving the stored value untouched.
		if prev.Kind != yaml.MappingNode {
			return &domain.SemanticError{
				Code:    domain.ErrCodeInvalidInput,
				Message: "la entrada " + HermesServerName + " de la configuración de Hermes no es un mapa; no se puede combinar sin perder datos",
			}
		}
		if env, ok := childValue(prev, hermesEnvKey); ok {
			putChild(entry, hermesEnvKey, env)
		}
	}
	putChild(servers, HermesServerName, entry)
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

	doc := &HermesDocument{}
	servers, err := childMapping(doc.storage().root, hermesServersSection)
	if err != nil {
		return "", err
	}
	entry := &yaml.Node{Kind: yaml.MappingNode}
	putChild(entry, hermesURLKey, plainScalar(strings.TrimSpace(rawURL)))
	headers := &yaml.Node{Kind: yaml.MappingNode}
	putChild(headers, HermesCallerIDHeader, hermesQuotedScalar(phone))
	putChild(entry, hermesHeadersKey, headers)
	putChild(servers, HermesServerName, entry)
	data, err := marshalHermesDocument(*doc)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// SetHermesTelegramOwnerBot merges the private Telegram owner-bot values
// (ADR-0018 Decision 3b) into the mcp_servers.<HermesServerName> entry env
// block WITHOUT touching url/headers or any foreign key: the two flows
// compose in either order — SetHermesServer preserves this env block when it
// replaces the entry (see there), and this merge leaves url/headers alone.
//
// All three values are validated before the document is touched — a validation
// failure leaves the document byte-identical. The phone goes through
// ValidatePhone (the single phone validation point) plus the 15-digit
// registration-cap convention of this change. Every env value is written as a
// double-quoted *yaml.Node: the phone for the YAML 1.1 +54 integer trap, and
// the token because its "123456:hash" shape would otherwise parse as a mapping.
func (d *HermesDocument) SetHermesTelegramOwnerBot(token, chatID, phone string) error {
	if err := ValidateTelegramBotToken(token); err != nil {
		return err
	}
	if err := ValidateTelegramChatID(chatID); err != nil {
		return err
	}
	if err := ValidatePhone(phone); err != nil {
		return err
	}
	// The 15-digit E.164 maximum is enforced by entity.HasValidPhone itself
	// (the domain owns the invariant); ValidatePhone inherits it, so every
	// admin flow applies the same bound with no adapter-side compensation.

	root := d.storage().root
	servers, err := childMapping(root, hermesServersSection)
	if err != nil {
		return err
	}
	entry, ok := childValue(servers, HermesServerName)
	if !ok {
		entry = &yaml.Node{Kind: yaml.MappingNode}
		putChild(servers, HermesServerName, entry)
	}
	// R3-001 (review-f32febf9c0f9d664): a SCALAR pre-existing entry can never
	// hold the env block — appending children to it would be dropped by the
	// marshaller while the flow reports success, silently losing the token.
	// Fail explicitly, same contract the map-based model already had.
	if entry.Kind != yaml.MappingNode {
		return &domain.SemanticError{
			Code:    domain.ErrCodeInvalidInput,
			Message: "la entrada " + HermesServerName + " de la configuración de Hermes no es un mapa; no se puede combinar sin perder datos",
		}
	}
	env, err := childMapping(entry, hermesEnvKey)
	if err != nil {
		return err
	}
	putChild(env, hermesTelegramTokenKey, hermesQuotedScalar(strings.TrimSpace(token)))
	putChild(env, hermesTelegramChatIDKey, hermesQuotedScalar(strings.TrimSpace(chatID)))
	putChild(env, hermesTelegramPhoneKey, hermesQuotedScalar(phone))
	return nil
}

// RemoveHermesTelegramOwnerBot deletes the Telegram owner-bot env block of the
// mcp_servers.<HermesServerName> entry: the explicit "sin Telegram" path of
// option 7 (S11, TASK-2.4 step 2). Skipping the optional group PRESERVES a
// configured block (SetHermesServer carries it over by contract, see there), so
// a business that configured the owner bot and retires it needs this separate,
// explicit removal instead of a hand-edited YAML.
//
// Contract:
//   - Byte-clean. Only the three MCP_TELEGRAM_* keys this repo writes are
//     dropped; url, headers (including the quoted X-Caller-Id), any foreign env
//     key (pollers, other integrations) and every other byte of the document —
//     key order, comments, scalar styles — survive verbatim. A removal never
//     reorders or rewrites what it does not own; the merge-without-clobber
//     robustness of ApplyHermesConfig applies to deletion as well.
//   - The env mapping itself is dropped only when it becomes EMPTY, so the entry
//     ends up in exactly the shape a never-configured channel renders
//     ({url, headers}), never an empty `env: {}` shell.
//   - Idempotent. A document without the block (no env, no entry, no servers
//     section, empty document) marshals back byte-identical: the flow can run on
//     every host without producing a diff. The same holds for a malformed entry
//     or env (a scalar), which can never carry the block: it is left untouched
//     rather than rewritten into a shape this flow does not own.
//
// It cannot fail, so it returns no error: every path either edits the owned keys
// or leaves the document alone. The zero value of the document is not
// materialized either — removal never creates a document.
func (d *HermesDocument) RemoveHermesTelegramOwnerBot() {
	servers, ok := childValue(d.fields.root, hermesServersSection)
	if !ok || servers.Kind != yaml.MappingNode {
		return
	}
	entry, ok := childValue(servers, HermesServerName)
	if !ok || entry.Kind != yaml.MappingNode {
		return
	}
	env, ok := childValue(entry, hermesEnvKey)
	if !ok || env.Kind != yaml.MappingNode {
		return
	}

	removed := false
	for _, key := range []string{
		hermesTelegramTokenKey,
		hermesTelegramChatIDKey,
		hermesTelegramPhoneKey,
	} {
		if dropChild(env, key) {
			removed = true
		}
	}
	if removed && len(env.Content) == 0 {
		dropChild(entry, hermesEnvKey)
	}
}

// ValidateTelegramBotToken validates the Telegram bot token shape
// "<bot-id digits>:<hash>". It is deliberately a shape check, not a liveness
// check: whether the token is accepted by Telegram is only knowable at gateway
// runtime, outside this process.
func ValidateTelegramBotToken(token string) error {
	if len(strings.TrimSpace(token)) > hermesTelegramTokenMaxLen {
		return &domain.SemanticError{
			Code:    domain.ErrCodeInvalidInput,
			Message: "el token del bot de Telegram excede la longitud máxima permitida",
			Cause:   domain.ErrInvalidInput,
		}
	}
	botID, hash, ok := splitBotToken(token)
	if !ok {
		return &domain.SemanticError{
			Code:    domain.ErrCodeInvalidInput,
			Message: "el token del bot de Telegram debe tener la forma <id>:<hash> (por ejemplo 123456789:AAEh...)",
			Cause:   domain.ErrInvalidInput,
		}
	}
	for _, r := range botID {
		if r < '0' || r > '9' {
			return &domain.SemanticError{
				Code:    domain.ErrCodeInvalidInput,
				Message: "el token del bot de Telegram debe empezar con el id numérico del bot antes de ':'",
				Cause:   domain.ErrInvalidInput,
			}
		}
	}
	for _, r := range hash {
		valid := (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-'
		if !valid {
			return &domain.SemanticError{
				Code:    domain.ErrCodeInvalidInput,
				Message: "el hash del token del bot de Telegram solo acepta letras, dígitos, '_' y '-'",
				Cause:   domain.ErrInvalidInput,
			}
		}
	}
	return nil
}

// ValidateTelegramChatID validates the owner-chat allowlist id: an optionally
// negative integer string (Telegram private chats are positive; groups are
// negative, so the shape accepts both and the allowlist semantics decide).
// The value is length-capped (Telegram ids are far shorter) so hostile or
// pasted-garbage input cannot bloat the persisted Hermes config.
func ValidateTelegramChatID(chatID string) error {
	trimmed := strings.TrimSpace(chatID)
	if trimmed == "" {
		return &domain.SemanticError{
			Code:    domain.ErrCodeInvalidInput,
			Message: "el chat id de la allowlist de Telegram no puede estar vacío",
		}
	}
	digits := strings.TrimPrefix(trimmed, "-")
	if digits == "" || len(digits) > hermesTelegramChatIDMaxDigits {
		return &domain.SemanticError{
			Code:    domain.ErrCodeInvalidInput,
			Message: "el chat id de la allowlist de Telegram debe ser numérico y de longitud razonable",
		}
	}
	for _, r := range digits {
		if r < '0' || r > '9' {
			return &domain.SemanticError{
				Code:    domain.ErrCodeInvalidInput,
				Message: "el chat id de la allowlist de Telegram debe ser numérico",
			}
		}
	}
	return nil
}

// MaskTelegramToken renders the operator-safe summary form of a bot token: the
// bot id stays visible (it is not the secret), the hash collapses to "***". A
// malformed token collapses entirely — the mask must never echo an input it
// could not parse.
func MaskTelegramToken(token string) string {
	botID, _, ok := splitBotToken(token)
	if !ok {
		return "***"
	}
	return botID + ":***"
}

// splitBotToken cuts a bot token into its numeric bot id and hash, trimming
// surrounding whitespace first. ok is false when the shape does not hold.
func splitBotToken(token string) (botID, hash string, ok bool) {
	botID, hash, ok = strings.Cut(strings.TrimSpace(token), ":")
	if !ok || botID == "" || hash == "" {
		return "", "", false
	}
	for _, r := range botID {
		if r < '0' || r > '9' {
			return "", "", false
		}
	}
	return botID, hash, true
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
// ensureHermesDir creates the Hermes directory atomically with its final
// mode (hermesDirMode): os.Mkdir applies the umask, which can only REMOVE
// permission bits, so the created directory can never be more permissive
// than 0700 and no follow-up chmod is needed — any post-Mkdir chmod, path-
// or fd-based, races with a swap of the newly created path (GGA rounds 4/5).
// A pre-existing path must be a directory and is never chmod-ed.
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
	if err := os.MkdirAll(filepath.Dir(dir), hermesDirMode); err != nil {
		return &domain.SemanticError{
			Code:    domain.ErrCodeInternal,
			Message: "no se pudo crear el directorio de la configuración de Hermes",
			Cause:   err,
		}
	}
	if err := os.Mkdir(dir, hermesDirMode); err != nil {
		// EEXIST: another creator won the race — validate it and never chmod.
		info, statErr := os.Stat(dir)
		if statErr != nil {
			return &domain.SemanticError{
				Code:    domain.ErrCodeInternal,
				Message: "no se pudo verificar el directorio de la configuración de Hermes",
				Cause:   statErr,
			}
		}
		if !info.IsDir() {
			return &domain.SemanticError{
				Code:    domain.ErrCodeInternal,
				Message: "la ruta de la configuración de Hermes no es un directorio",
			}
		}
		return nil
	}
	return nil
}

// plainScalar returns a plain-style string scalar node.
func plainScalar(value string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Value: value}
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
	if err := enc.Encode(doc.storage().root); err != nil {
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
