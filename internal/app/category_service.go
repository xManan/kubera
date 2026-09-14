package app

import (
	"context"
	"database/sql"

	"kubera/internal/domain"
	"kubera/internal/duplicate"
)

// CreateCategory creates a category; names are trimmed and unique among
// active categories case-insensitively. Description is optional free text
// that AI clients use to classify transactions.
func (s *Services) CreateCategory(ctx context.Context, name, description string) (domain.Category, error) {
	name = trim(name)
	if err := domain.ValidateLength(name, domain.MaxCategoryNameLength,
		domain.CodeInvalidRequest, "Category name"); err != nil {
		return domain.Category{}, err
	}
	description = trim(description)
	if err := domain.ValidateMaxLength(description, domain.MaxCategoryDescriptionLength,
		domain.CodeInvalidRequest, "Category description"); err != nil {
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
		c := domain.Category{ID: domain.CategoryID(s.id("cat")), Name: name, Description: description, CreatedAt: now, UpdatedAt: now}
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

// UpdateCategory patches a category's name and/or description (nil = leave
// unchanged, empty string = clear), rejecting name collisions with active names.
func (s *Services) UpdateCategory(ctx context.Context, id domain.CategoryID, name, description *string) (domain.Category, error) {
	if name == nil && description == nil {
		return domain.Category{}, domain.NewError(domain.CodeEmptyUpdate,
			"Provide a name and/or description to update.")
	}
	newName := ""
	if name != nil {
		newName = trim(*name)
		if err := domain.ValidateLength(newName, domain.MaxCategoryNameLength,
			domain.CodeInvalidRequest, "Category name"); err != nil {
			return domain.Category{}, err
		}
	}
	newDescription := ""
	if description != nil {
		newDescription = trim(*description)
		if err := domain.ValidateMaxLength(newDescription, domain.MaxCategoryDescriptionLength,
			domain.CodeInvalidRequest, "Category description"); err != nil {
			return domain.Category{}, err
		}
	}
	var updated domain.Category
	err := s.withWriteTx(ctx, func(tx *sql.Tx) error {
		cur, err := s.Categories.Get(ctx, tx, id)
		if err != nil {
			return err
		}
		if name != nil {
			if dup, exists, err := s.Categories.FindActiveByName(ctx, tx, duplicate.NormalizeText(newName)); err != nil {
				return err
			} else if exists && dup.ID != cur.ID {
				return domain.NewError(domain.CodeCategoryAlreadyExists,
					"An active category with this name already exists.")
			}
			cur.Name = newName
		}
		if description != nil {
			cur.Description = newDescription
		}
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
