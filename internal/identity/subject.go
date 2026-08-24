package identity

import "time"

type Subject struct {
	Issuer    string
	ID        string
	Audience  []string
	ExpiresAt time.Time
	IssuedAt  time.Time
	Claims    map[string]any
}
