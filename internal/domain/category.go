package domain

import "time"

// Category is a user-managed classification for transactions.
// Archived categories cannot be assigned to new or updated transactions but
// remain associated with historical ones.
type Category struct {
	ID         CategoryID
	Name       string
	ArchivedAt *time.Time
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

func (c Category) IsArchived() bool { return c.ArchivedAt != nil }
