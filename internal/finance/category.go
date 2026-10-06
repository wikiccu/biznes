package finance

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/wikiccu/biznes/internal/organization"
)

type Category struct {
	ID, OrganizationID, Name, Kind string
	CreatedAt, UpdatedAt           time.Time
}

type CategoryInput struct{ Name, Kind string }
type FieldError struct{ Field, Code string }
type ValidationError struct{ Fields []FieldError }

func (e *ValidationError) Error() string { return "finance validation failed" }

var (
	ErrCategoryConflict = errors.New("category name already exists for this kind")
	ErrUnavailable      = errors.New("finance service unavailable")
)

type Service struct{ pool *pgxpool.Pool }

func NewService(pool *pgxpool.Pool) *Service { return &Service{pool: pool} }

func validateName(name string) (string, string) {
	invalid := !utf8.ValidString(name) || strings.ContainsFunc(name, unicode.IsControl)
	name = strings.TrimSpace(name)
	switch {
	case invalid:
		return "", "invalid_format"
	case name == "":
		return "", "required"
	case utf8.RuneCountInString(name) > 120:
		return "", "out_of_range"
	default:
		return name, ""
	}
}

func validateCategory(input CategoryInput, create bool) (CategoryInput, error) {
	var fields []FieldError
	var code string
	input.Name, code = validateName(input.Name)
	if code != "" {
		fields = append(fields, FieldError{"name", code})
	}
	if create {
		if input.Kind == "" {
			fields = append(fields, FieldError{"kind", "required"})
		} else if input.Kind != "income" && input.Kind != "expense" {
			fields = append(fields, FieldError{"kind", "invalid_format"})
		}
	}
	if len(fields) != 0 {
		return CategoryInput{}, &ValidationError{fields}
	}
	return input, nil
}

func scanCategory(row pgx.Row) (Category, error) {
	var item Category
	err := row.Scan(&item.ID, &item.OrganizationID, &item.Name, &item.Kind, &item.CreatedAt, &item.UpdatedAt)
	return item, err
}

func (s *Service) CreateCategory(ctx context.Context, userID, organizationID string, input CategoryInput) (Category, error) {
	return s.writeCategory(ctx, userID, organizationID, "", input)
}

func (s *Service) RenameCategory(ctx context.Context, userID, organizationID, categoryID, name string) (Category, error) {
	if categoryID == "" {
		return Category{}, organization.ErrNotFound
	}
	return s.writeCategory(ctx, userID, organizationID, categoryID, CategoryInput{Name: name})
}

func (s *Service) writeCategory(ctx context.Context, userID, organizationID, categoryID string, input CategoryInput) (Category, error) {
	input, err := validateCategory(input, categoryID == "")
	if err != nil {
		return Category{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Category{}, ErrUnavailable
	}
	defer tx.Rollback(ctx)
	role, err := organization.LockMembership(ctx, tx, userID, organizationID)
	if err != nil {
		return Category{}, err
	}
	if role != "owner" && role != "admin" && role != "accountant" {
		return Category{}, organization.ErrForbidden
	}
	var row pgx.Row
	if categoryID == "" {
		row = tx.QueryRow(ctx, `INSERT INTO biznes.transaction_categories (organization_id, name, kind)
			VALUES ($1, $2, $3) RETURNING id, organization_id, name, kind, created_at, updated_at`,
			organizationID, input.Name, input.Kind)
	} else {
		row = tx.QueryRow(ctx, `UPDATE biznes.transaction_categories SET name = $3, updated_at = clock_timestamp()
			WHERE organization_id = $1 AND id = $2 RETURNING id, organization_id, name, kind, created_at, updated_at`,
			organizationID, categoryID, input.Name)
	}
	item, err := scanCategory(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Category{}, organization.ErrNotFound
	}
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "transaction_categories_name_unique" {
			return Category{}, ErrCategoryConflict
		}
		return Category{}, ErrUnavailable
	}
	if tx.Commit(ctx) != nil {
		return Category{}, ErrUnavailable
	}
	return item, nil
}

func (s *Service) GetCategory(ctx context.Context, userID, organizationID, categoryID string) (Category, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	item, err := scanCategory(s.pool.QueryRow(ctx, `SELECT c.id, c.organization_id, c.name, c.kind, c.created_at, c.updated_at
		FROM biznes.transaction_categories c JOIN biznes.memberships m ON m.organization_id = c.organization_id
		WHERE c.organization_id = $1 AND c.id = $2 AND m.user_id = $3`, organizationID, categoryID, userID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Category{}, organization.ErrNotFound
	}
	if err != nil {
		return Category{}, ErrUnavailable
	}
	return item, nil
}

func (s *Service) ListCategories(ctx context.Context, userID, organizationID string, page, limit int) ([]Category, error) {
	if page < 1 || page > 10000 || limit < 1 || limit > 100 {
		return nil, ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, ErrUnavailable
	}
	defer tx.Rollback(ctx)
	if _, err := organization.LockMembership(ctx, tx, userID, organizationID); err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `SELECT id, organization_id, name, kind, created_at, updated_at
		FROM biznes.transaction_categories WHERE organization_id = $1 ORDER BY created_at, id LIMIT $2 OFFSET $3`,
		organizationID, limit, (page-1)*limit)
	if err != nil {
		return nil, ErrUnavailable
	}
	defer rows.Close()
	items := make([]Category, 0)
	for rows.Next() {
		item, err := scanCategory(rows)
		if err != nil {
			return nil, ErrUnavailable
		}
		items = append(items, item)
	}
	if rows.Err() != nil || tx.Commit(ctx) != nil {
		return nil, ErrUnavailable
	}
	return items, nil
}
