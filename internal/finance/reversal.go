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
	"github.com/wikiccu/biznes/internal/organization"
)

type Reversal struct {
	ID, OrganizationID, TransactionID, IdempotencyKey, Reason, CreatedBy string
	CreatedAt                                                            time.Time
}

type ReversalInput struct{ IdempotencyKey, Reason string }

var ErrReversalConflict = errors.New("transaction reversal conflicts with existing record")

func validateReversal(input ReversalInput) (ReversalInput, error) {
	var fields []FieldError
	if input.IdempotencyKey == "" {
		fields = append(fields, FieldError{"idempotency_key", "required"})
	} else if !transactionUUID.MatchString(input.IdempotencyKey) {
		fields = append(fields, FieldError{"idempotency_key", "invalid_format"})
	}
	invalid := !utf8.ValidString(input.Reason) || strings.ContainsFunc(input.Reason, unicode.IsControl)
	input.Reason = strings.TrimSpace(input.Reason)
	switch {
	case invalid:
		fields = append(fields, FieldError{"reason", "invalid_format"})
	case input.Reason == "":
		fields = append(fields, FieldError{"reason", "required"})
	case utf8.RuneCountInString(input.Reason) > 2000:
		fields = append(fields, FieldError{"reason", "out_of_range"})
	}
	if len(fields) != 0 {
		return ReversalInput{}, &ValidationError{fields}
	}
	return input, nil
}

func scanReversal(row pgx.Row) (Reversal, error) {
	var item Reversal
	err := row.Scan(&item.ID, &item.OrganizationID, &item.TransactionID, &item.IdempotencyKey,
		&item.Reason, &item.CreatedBy, &item.CreatedAt)
	return item, err
}

func (s *Service) ReverseTransaction(ctx context.Context, userID, organizationID, transactionID string, input ReversalInput) (Reversal, bool, error) {
	input, err := validateReversal(input)
	if err != nil {
		return Reversal{}, false, err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Reversal{}, false, ErrUnavailable
	}
	defer tx.Rollback(ctx)
	role, err := organization.LockMembership(ctx, tx, userID, organizationID)
	if err != nil {
		return Reversal{}, false, err
	}
	if role != "owner" && role != "admin" && role != "accountant" {
		return Reversal{}, false, organization.ErrForbidden
	}
	if lockTransactionRecognition(ctx, tx, organizationID, transactionID) != nil {
		return Reversal{}, false, ErrUnavailable
	}
	item, err := scanReversal(tx.QueryRow(ctx, `INSERT INTO biznes.transaction_reversals
		(organization_id, transaction_id, idempotency_key, reason, created_by)
		SELECT organization_id, id, $3, $4, $5 FROM biznes.transactions WHERE organization_id = $1 AND id = $2
		ON CONFLICT DO NOTHING
		RETURNING id, organization_id, transaction_id, idempotency_key, reason, created_by, created_at`,
		organizationID, transactionID, input.IdempotencyKey, input.Reason, userID))
	created := err == nil
	if errors.Is(err, pgx.ErrNoRows) {
		// Read the committed winner after either unique constraint serialized concurrent requests.
		item, err = scanReversal(tx.QueryRow(ctx, `SELECT id, organization_id, transaction_id, idempotency_key, reason, created_by, created_at
			FROM biznes.transaction_reversals WHERE organization_id = $1 AND (idempotency_key = $3 OR transaction_id = $2)
			ORDER BY (idempotency_key = $3) DESC LIMIT 1`, organizationID, transactionID, input.IdempotencyKey))
		if errors.Is(err, pgx.ErrNoRows) {
			return Reversal{}, false, organization.ErrNotFound
		}
		if err == nil && (item.TransactionID != transactionID || item.IdempotencyKey != input.IdempotencyKey || item.Reason != input.Reason) {
			return Reversal{}, false, ErrReversalConflict
		}
	}
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			return Reversal{}, false, organization.ErrNotFound
		}
		return Reversal{}, false, ErrUnavailable
	}
	if tx.Commit(ctx) != nil {
		return Reversal{}, false, ErrUnavailable
	}
	return item, created, nil
}

func (s *Service) GetReversal(ctx context.Context, userID, organizationID, transactionID string) (Reversal, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	item, err := scanReversal(s.pool.QueryRow(ctx, `SELECT r.id, r.organization_id, r.transaction_id, r.idempotency_key, r.reason, r.created_by, r.created_at
		FROM biznes.transaction_reversals r JOIN biznes.memberships m ON m.organization_id = r.organization_id
		WHERE r.organization_id = $1 AND r.transaction_id = $2 AND m.user_id = $3`, organizationID, transactionID, userID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Reversal{}, organization.ErrNotFound
	}
	if err != nil {
		return Reversal{}, ErrUnavailable
	}
	return item, nil
}
