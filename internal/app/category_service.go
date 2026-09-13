package app

import (
	"context"
	"database/sql"

	"kubera/internal/domain"
	"kubera/internal/duplicate"
)

// CreateCategory creates a category; names are trimmed and unique among
// active categories case-insensitively.
func (s *Services) CreateCategory(ctx context.Context, name string) (domain.Category, error) {
	name = trim(name)
	if err := domain.ValidateLength(name, domain.MaxCategoryNameLength,
		domain.CodeInvalidRequest, "Category name"); err != nil {
		return domain.Category{}, err
	}
	var created domain.Category
	err := s.withWriteTx(ctx, func(tx *sql.Tx) error {
		if _, exists, err := s.Categories.FindActiveByName(ctx, tx, duplicate.NormalizeText(name)); err != nil {
			return err
		} else if exists {
			return domain.NewError(domain.CodeCategoryAlreadyExists,
				"An active category with this name already exists.")
		}
		now := s.now()
		c := domain.Category{ID: domain.CategoryID(s.id("cat")), Name: name, CreatedAt: now, UpdatedAt: now}
		if err := s.Categories.Create(ctx, tx, c); err != nil {
			return err
		}
		created = c
		return nil
	})
	if err != nil {
		return domain.Category{}, err
	}
	return created, nil
}

// ListCategories returns active (or all) categories ordered by normalized name.
func (s *Services) ListCategories(ctx context.Context, includeArchived bool) ([]domain.Category, error) {
	return s.Categories.List(ctx, s.DB, includeArchived)
}

// GetCategory returns one category.
func (s *Services) GetCategory(ctx context.Context, id domain.CategoryID) (domain.Category, error) {
	return s.Categories.Get(ctx, s.DB, id)
}

// UpdateCategory renames a category, rejecting collisions with active names.
func (s *Services) UpdateCategory(ctx context.Context, id domain.CategoryID, name string) (domain.Category, error) {
	name = trim(name)
	if err := domain.ValidateLength(name, domain.MaxCategoryNameLength,
		domain.CodeInvalidRequest, "Category name"); err != nil {
		return domain.Category{}, err
	}
	var updated domain.Category
	err := s.withWriteTx(ctx, func(tx *sql.Tx) error {
		cur, err := s.Categories.Get(ctx, tx, id)
		if err != nil {
			return err
		}
		if dup, exists, err := s.Categories.FindActiveByName(ctx, tx, duplicate.NormalizeText(name)); err != nil {
			return err
		} else if exists && dup.ID != cur.ID {
			return domain.NewError(domain.CodeCategoryAlreadyExists,
				"An active category with this name already exists.")
		}
		cur.Name = name
		cur.UpdatedAt = s.now()
		if err := s.Categories.Update(ctx, tx, cur); err != nil {
			return err
		}
		updated = cur
		return nil
	})
	if err != nil {
		return domain.Category{}, err
	}
	return updated, nil
}

// ArchiveCategory archives a category idempotently, preserving historical
// transaction associations.
func (s *Services) ArchiveCategory(ctx context.Context, id domain.CategoryID) (domain.Category, error) {
	var archived domain.Category
	err := s.withWriteTx(ctx, func(tx *sql.Tx) error {
		cur, err := s.Categories.Get(ctx, tx, id)
		if err != nil {
			return err
		}
		if !cur.IsArchived() {
			now := s.now()
			cur.ArchivedAt = &now
			cur.UpdatedAt = now
			if err := s.Categories.Archive(ctx, tx, id, now); err != nil {
				return err
			}
		}
		archived = cur
		return nil
	})
	if err != nil {
		return domain.Category{}, err
	}
	return archived, nil
}
