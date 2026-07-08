package model

import "github.com/google/uuid"

// NewUUID generates a new UUID v4 string. Used by repository constructors
// to assign IDs to entities that use TEXT PRIMARY KEY (Client, Service,
// Professional, Booking).
func NewUUID() string {
	return uuid.New().String()
}
