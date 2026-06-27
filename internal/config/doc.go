// Package config loads and validates the JSON configuration files produced by the
// config-wizard TUI (setup_business.json, setup_staff.json, setup_services.json).
//
// The config layer loads the initial setup data exported by the TUI wizard.
// Runtime state (business profile, professionals, services) lives in the
// repository/DB, not in these structs. The repository's GetBusinessProfile()
// performs lazy-init via INSERT OR IGNORE to ensure the singleton row exists
// on first access, so the config files are the bootstrap input, not the
// runtime source of truth.
package config
