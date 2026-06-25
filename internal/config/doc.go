// Package config loads and validates the JSON configuration files produced by the
// config-wizard TUI (setup_business.json, setup_staff.json, setup_services.json).
//
// The config layer is the single source of truth for business profile, professionals,
// and services at startup time. The repository layer reads from these structs (or
// from a fresh DB query) at runtime; the MCP handlers never touch the config files
// directly.
package config
