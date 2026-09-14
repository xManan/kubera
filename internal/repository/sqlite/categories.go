package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"kubera/internal/domain"
	"kubera/internal/duplicate"
	"kubera/internal/repository"
)

type catRepo struct{}

// NewCategoryRepository returns the SQLite-backed category repository.
func NewCategoryRepository() repository.CategoryRepository { return catRepo{} }

func scanCat(row interface{ Scan(...any) error }) (domain.Category, error) {
	var (
		c          domain.Category
		archivedAt sql.NullString
		createdAt  string
		updatedAt  string
	)
	err := row.Scan(&c.ID, &c.Name, &c.Description, &archivedAt, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return c, domain.NewError(domain.CodeCategoryNotFound, "The requested category does not exist.")
	}
	if err != nil {
		return c, fmt.Errorf("scan category: %w", err)
	}
	if archivedAt.Valid {
		a, err := parseTime(archivedAt.String)
		if err != nil {
			return c, fmt.Errorf("scan category archived_at: %w", err)
		}
		c.ArchivedAt = &a
	}
	if c.CreatedAt, err = parseTime(createdAt); err != nil {
		return c, fmt.Errorf("scan category created_at: %w", err)
	}
	if c.UpdatedAt, err = parseTime(updatedAt); err != nil {
		return c, fmt.Errorf("scan category updated_at: %w", err)
	}
	return c, nil
}

func (catRepo) Create(ctx context.Context, tx *sql.Tx, c domain.Category) error {
	_, err := tx.ExecContext(ctx,
		"INSERT INTO categories (id, name, description, name_normalized, archived_at, created_at, updated_at) VALUES (?, ?, ?, ?, NULL, ?, ?)",
		string(c.ID), c.Name, c.Description, duplicate.NormalizeText(c.Name), timeString(c.CreatedAt), timeString(c.UpdatedAt))
	if isUniqueViolation(err) {
		return domain.NewError(domain.CodeCategoryAlreadyExists, "An active category with this name already exists.")
	}
	if err != nil {
		return fmt.Errorf("insert category: %w", err)
	}
	return nil
}

func (catRepo) Get(ctx context.Context, q repository.Queryer, id domain.CategoryID) (domain.Category, error) {
	row := q.QueryRowContext(ctx,
		"SELECT id, name, description, archived_at, created_at, updated_at FROM categories WHERE id = ?", string(id))
	return scanCat(row)
}

func (catRepo) FindActiveByName(ctx context.Context, q repository.Queryer, nameNormalized string) (domain.Category, bool, error) {
	row := q.QueryRowContext(ctx,
		"SELECT id, name, description, archived_at, created_at, updated_at FROM categories WHERE name_normalized = ? AND archived_at IS NULL",
		nameNormalized)
	c, err := scanCat(row)
	if domain.Is(err, domain.CodeCategoryNotFound) {
		return domain.Category{}, false, nil
	}
	if err != nil {
		return domain.Category{}, false, err
	}
	return c, true, nil
}

func (catRepo) List(ctx context.Context, q repository.Queryer, includeArchived bool) ([]domain.Category, error) {
	query := "SELECT id, name, description, archived_at, created_at, updated_at FROM categories"
	if !includeArchived {
		query += " WHERE archived_at IS NULL"
	}
	query += " ORDER BY name_normalized ASC, id ASC"
	rows, err := q.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list categories: %w", err)
	}
	defer rows.Close()
	var out []domain.Category
	for rows.Next() {
		c, err := scanCat(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (catRepo) Update(ctx context.Context, tx *sql.Tx, c domain.Category) error {
	_, err := tx.ExecContext(ctx,
		"UPDATE categories SET name = ?, description = ?, name_normalized = ?, updated_at = ? WHERE id = ?",
		c.Name, c.Description, duplicate.NormalizeText(c.Name), timeString(c.UpdatedAt), string(c.ID))
	if isUniqueViolation(err) {
		return domain.NewError(domain.CodeCategoryAlreadyExists, "An active category with this name already exists.")
	}
	if err != nil {
		return fmt.Errorf("update category: %w", err)
	}
	return nil
}

func (catRepo) Archive(ctx context.Context, tx *sql.Tx, id domain.CategoryID, at time.Time) error {
	_, err := tx.ExecContext(ctx,
		"UPDATE categories SET archived_at = ?, updated_at = ? WHERE id = ? AND archived_at IS NULL",
		timeString(at), timeString(at), string(id))
	if err != nil {
		return fmt.Errorf("archive category: %w", err)
	}
	return nil
}

// isUniqueViolation detects SQLite UNIQUE constraint failures from the
// driver error text. ponytail: string match instead of error-code unwrap;
// swap to errors.As on the driver's sqlite3.Error if the driver exposes one.
func isUniqueViolation(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}
