package identity

import "time"

// User is a global identity; organization access belongs to memberships.
// PasswordHash holds an encoded server-generated verifier, never plaintext.
// HTTP handlers should use explicit response types rather than expose this model.
type User struct {
	ID           string    `json:"id"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}
