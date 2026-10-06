package contact

import (
	"context"
	"errors"
	"net/mail"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/wikiccu/biznes/internal/organization"
)

type Input struct {
	Name, Kind, Email, Phone, Notes string
}

type Contact struct {
	ID, OrganizationID, Name, Kind, Email, Phone, Notes string
	CreatedAt, UpdatedAt                                time.Time
}

type FieldError struct{ Field, Code string }
type ValidationError struct{ Fields []FieldError }

func (e *ValidationError) Error() string { return "contact validation failed" }

var ErrUnavailable = errors.New("contact service unavailable")

type Service struct{ pool *pgxpool.Pool }

func NewService(pool *pgxpool.Pool) *Service { return &Service{pool: pool} }

func validate(input Input) (Input, error) {
	var fields []FieldError
	for _, field := range []struct {
		name     string
		value    *string
		maximum  int
		required bool
	}{{"name", &input.Name, 120, true}, {"email", &input.Email, 254, false}, {"phone", &input.Phone, 64, false}, {"notes", &input.Notes, 2000, false}} {
		value := *field.value
		invalid := !utf8.ValidString(value) || strings.ContainsFunc(value, func(r rune) bool {
			return unicode.IsControl(r) && !(field.name == "notes" && (r == '\n' || r == '\r' || r == '\t'))
		})
		if field.name != "notes" {
			value = strings.TrimSpace(value)
		}
		*field.value = value
		length := utf8.RuneCountInString(value)
		if field.name == "email" {
			length = len(value)
		}
		switch {
		case invalid:
			fields = append(fields, FieldError{field.name, "invalid_format"})
		case field.required && value == "":
			fields = append(fields, FieldError{field.name, "required"})
		case length > field.maximum:
			fields = append(fields, FieldError{field.name, "out_of_range"})
		case field.name == "email" && value != "":
			address, err := mail.ParseAddress(value)
			if err != nil || address.Name != "" || address.Address != value {
				fields = append(fields, FieldError{field.name, "invalid_format"})
			}
		}
	}
	if input.Kind == "" {
		fields = append(fields, FieldError{"kind", "required"})
	} else if input.Kind != "customer" && input.Kind != "supplier" && input.Kind != "both" {
		fields = append(fields, FieldError{"kind", "invalid_format"})
	}
	if len(fields) != 0 {
		return Input{}, &ValidationError{fields}
	}
	return input, nil
}

func scanContact(row pgx.Row) (Contact, error) {
	var item Contact
	err := row.Scan(&item.ID, &item.OrganizationID, &item.Name, &item.Kind, &item.Email, &item.Phone, &item.Notes, &item.CreatedAt, &item.UpdatedAt)
	return item, err
}

func (s *Service) Create(ctx context.Context, userID, organizationID string, input Input) (Contact, error) {
	return s.write(ctx, userID, organizationID, "", input)
}

func (s *Service) Replace(ctx context.Context, userID, organizationID, contactID string, input Input) (Contact, error) {
	if contactID == "" {
		return Contact{}, organization.ErrNotFound
	}
	return s.write(ctx, userID, organizationID, contactID, input)
}

func (s *Service) write(ctx context.Context, userID, organizationID, contactID string, input Input) (Contact, error) {
	input, err := validate(input)
	if err != nil {
		return Contact{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Contact{}, ErrUnavailable
	}
	defer tx.Rollback(ctx)
	role, err := organization.LockMembership(ctx, tx, userID, organizationID)
	if err != nil {
		return Contact{}, err
	}
	if role != "owner" && role != "admin" {
		return Contact{}, organization.ErrForbidden
	}
	var row pgx.Row
	if contactID == "" {
		row = tx.QueryRow(ctx, `INSERT INTO biznes.contacts (organization_id, name, kind, email, phone, notes)
			VALUES ($1, $2, $3, $4, $5, $6)
			RETURNING id, organization_id, name, kind, email, phone, notes, created_at, updated_at`,
			organizationID, input.Name, input.Kind, input.Email, input.Phone, input.Notes)
	} else {
		row = tx.QueryRow(ctx, `UPDATE biznes.contacts
			SET name = $3, kind = $4, email = $5, phone = $6, notes = $7, updated_at = clock_timestamp()
			WHERE organization_id = $1 AND id = $2
			RETURNING id, organization_id, name, kind, email, phone, notes, created_at, updated_at`,
			organizationID, contactID, input.Name, input.Kind, input.Email, input.Phone, input.Notes)
	}
	item, err := scanContact(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Contact{}, organization.ErrNotFound
	}
	if err != nil || tx.Commit(ctx) != nil {
		return Contact{}, ErrUnavailable
	}
	return item, nil
}

func (s *Service) Get(ctx context.Context, userID, organizationID, contactID string) (Contact, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	item, err := scanContact(s.pool.QueryRow(ctx, `SELECT c.id, c.organization_id, c.name, c.kind, c.email, c.phone, c.notes, c.created_at, c.updated_at
		FROM biznes.contacts c JOIN biznes.memberships m ON m.organization_id = c.organization_id
		WHERE c.organization_id = $1 AND c.id = $2 AND m.user_id = $3`, organizationID, contactID, userID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Contact{}, organization.ErrNotFound
	}
	if err != nil {
		return Contact{}, ErrUnavailable
	}
	return item, nil
}

func (s *Service) List(ctx context.Context, userID, organizationID string, page, limit int) ([]Contact, error) {
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
	rows, err := tx.Query(ctx, `SELECT id, organization_id, name, kind, email, phone, notes, created_at, updated_at
		FROM biznes.contacts WHERE organization_id = $1 ORDER BY created_at, id LIMIT $2 OFFSET $3`, organizationID, limit, (page-1)*limit)
	if err != nil {
		return nil, ErrUnavailable
	}
	defer rows.Close()
	items := make([]Contact, 0)
	for rows.Next() {
		item, err := scanContact(rows)
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
