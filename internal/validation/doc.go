// Package validation is an intentionally empty, doc-only package for now.
//
// Decision (D3, odd/tasks/code-hardening-followups.md): the data-access spec
// forbids internal/repository from importing this package, phone validation is
// single-sourced in internal/domain/entity (entity.Client.HasValidPhone, the
// E.164 subset rule), and no duplicated logic remains to promote — so forcing
// content here would be a premature abstraction (AGENTS.md). The package earns
// validators only when a genuinely cross-cutting rule appears that no single
// domain entity, use case, or adapter owns.
//
// When it earns content:
//   - Validators belong here only if they are shared across layers (the
//     "shared edges" row of the layer map). Single-use rules stay next to
//     their caller — see the header of internal/repository/validation.go for
//     the ratified rationale and its spec constraints.
//   - Error contract: business-domain semantic errors are
//     *domain.SemanticError{Code, Message, Cause} defined in
//     internal/domain/errors.go. This package does NOT own error types and
//     must stay a leaf package with no imports from other internal packages.
//   - Error messages are written in neutral Spanish, actionable for end
//     users. Stack traces are NEVER included in the message sent to the LLM;
//     server-side logs may include the full wrapped error via
//     fmt.Errorf("...: %w", err).
package validation
