// core/domain/user.go
package domain

import "time"

// UserID is the opaque identifier of a user account. It is a UUID
// generated at signup.
type UserID string

type User struct {
	ID          UserID
	DisplayName string
	CreatedAt   time.Time
}

func NewUser(id UserID, displayName string, now time.Time) *User {
	return &User{ID: id, DisplayName: displayName, CreatedAt: now}
}
