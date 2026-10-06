package organization

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Organization struct {
	ID        string
	Name      string
	Role      string // The authenticated caller's membership, never an organization-wide role.
	CreatedAt time.Time
	UpdatedAt time.Time
}

type Service struct {
	pool *pgxpool.Pool
}

func NewService(pool *pgxpool.Pool) *Service { return &Service{pool: pool} }

var (
	ErrNameRequired = errors.New("organization name is required")
	ErrNameInvalid  = errors.New("organization name is invalid")
	ErrNameTooLong  = errors.New("organization name is too long")
	ErrNotFound     = errors.New("organization not found")
	ErrForbidden    = errors.New("organization operation forbidden")
	ErrUnavailable  = errors.New("organization service unavailable")
)

func validateName(name string) (string, error) {
	if !utf8.ValidString(name) || strings.ContainsFunc(name, unicode.IsControl) {
		return "", ErrNameInvalid
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return "", ErrNameRequired
	}
	if utf8.RuneCountInString(name) > 120 {
		return "", ErrNameTooLong
	}
	return name, nil
}

func (s *Service) Create(ctx context.Context, userID, name string) (Organization, error) {
	name, err := validateName(name)
	if err != nil {
		return Organization{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Organization{}, ErrUnavailable
	}
	defer tx.Rollback(ctx)
	var org Organization
	err = tx.QueryRow(ctx, `INSERT INTO biznes.organizations (name) VALUES ($1)
		RETURNING id, name, created_at, updated_at`, name).
		Scan(&org.ID, &org.Name, &org.CreatedAt, &org.UpdatedAt)
	if err != nil {
		return Organization{}, ErrUnavailable
	}
	_, err = tx.Exec(ctx, `INSERT INTO biznes.memberships (organization_id, user_id, role)
		VALUES ($1, $2, 'owner')`, org.ID, userID)
	if err != nil || tx.Commit(ctx) != nil {
		return Organization{}, ErrUnavailable
	}
	org.Role = "owner"
	return org, nil
}

func (s *Service) Get(ctx context.Context, userID, organizationID string) (Organization, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var org Organization
	err := s.pool.QueryRow(ctx, `SELECT o.id, o.name, m.role, o.created_at, o.updated_at
		FROM biznes.organizations o JOIN biznes.memberships m ON m.organization_id = o.id
		WHERE o.id = $1 AND m.user_id = $2`, organizationID, userID).
		Scan(&org.ID, &org.Name, &org.Role, &org.CreatedAt, &org.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Organization{}, ErrNotFound
	}
	if err != nil {
		return Organization{}, ErrUnavailable
	}
	return org, nil
}

func (s *Service) List(ctx context.Context, userID string, page, limit int) ([]Organization, error) {
	// Keep pagination bounded even when called outside HTTP.
	if page < 1 || page > 10000 || limit < 1 || limit > 100 {
		return nil, ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	rows, err := s.pool.Query(ctx, `SELECT o.id, o.name, m.role, o.created_at, o.updated_at
		FROM biznes.organizations o JOIN biznes.memberships m ON m.organization_id = o.id
		WHERE m.user_id = $1 ORDER BY o.created_at, o.id LIMIT $2 OFFSET $3`, userID, limit, (page-1)*limit)
	if err != nil {
		return nil, ErrUnavailable
	}
	defer rows.Close()
	orgs := make([]Organization, 0)
	for rows.Next() {
		var org Organization
		if err := rows.Scan(&org.ID, &org.Name, &org.Role, &org.CreatedAt, &org.UpdatedAt); err != nil {
			return nil, ErrUnavailable
		}
		orgs = append(orgs, org)
	}
	if rows.Err() != nil {
		return nil, ErrUnavailable
	}
	return orgs, nil
}

func (s *Service) Rename(ctx context.Context, userID, organizationID, name string) (Organization, error) {
	name, err := validateName(name)
	if err != nil {
		return Organization{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Organization{}, ErrUnavailable
	}
	defer tx.Rollback(ctx)
	var org Organization
	// Hold the membership through commit so concurrent revocation/demotion cannot race this write.
	err = tx.QueryRow(ctx, `SELECT role FROM biznes.memberships
		WHERE organization_id = $1 AND user_id = $2 FOR SHARE`, organizationID, userID).Scan(&org.Role)
	if errors.Is(err, pgx.ErrNoRows) {
		return Organization{}, ErrNotFound
	}
	if err != nil {
		return Organization{}, ErrUnavailable
	}
	if org.Role != "owner" && org.Role != "admin" {
		return Organization{}, ErrForbidden
	}
	err = tx.QueryRow(ctx, `UPDATE biznes.organizations SET name = $2, updated_at = clock_timestamp()
		WHERE id = $1 RETURNING id, name, created_at, updated_at`, organizationID, name).
		Scan(&org.ID, &org.Name, &org.CreatedAt, &org.UpdatedAt)
	if err != nil || tx.Commit(ctx) != nil {
		return Organization{}, ErrUnavailable
	}
	return org, nil
}
