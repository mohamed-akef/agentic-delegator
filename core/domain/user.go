// core/domain/user.go
package domain

import "time"

// UserID is the opaque identifier of a user account. In SaaS, this is a UUID
// generated at signup. In selfhost, there is exactly one user with a fixed ID.
type UserID string

type User struct {
	ID          UserID
	DisplayName string
	CreatedAt   time.Time
}

func NewUser(id UserID, displayName string, now time.Time) *User {
	return &User{ID: id, DisplayName: displayName, CreatedAt: now}
}
