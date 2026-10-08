// Package shop owns merchant shop identity settings.
package shop

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
	"github.com/techize/selloovy/internal/auth"
	"github.com/techize/selloovy/internal/shopdb"
)

var ErrStorage = errors.New("shop settings are unavailable")
var ErrConflict = errors.New("shop settings changed; reload the saved details")

type ValidationError struct{ Fields map[string]string }

func (e *ValidationError) Error() string { return "check the shop settings fields" }

type Settings struct {
	Name         string `json:"name"`
	Tagline      string `json:"tagline"`
	Description  string `json:"description"`
	ContactEmail string `json:"contactEmail"`
	CurrencyCode string `json:"currencyCode"`
	CountryCode  string `json:"countryCode"`
	Timezone     string `json:"timezone"`
	Revision     int64  `json:"revision"`
}
type Input struct {
	Name         string `json:"name"`
	Tagline      string `json:"tagline"`
	Description  string `json:"description"`
	ContactEmail string `json:"contactEmail"`
	Revision     int64  `json:"revision"`
}

func validate(in Input) (Input, error) {
	in.Name = strings.TrimSpace(in.Name)
	in.Tagline = strings.TrimSpace(in.Tagline)
	in.Description = strings.TrimSpace(in.Description)
	in.ContactEmail = strings.ToLower(strings.TrimSpace(in.ContactEmail))
	fields := map[string]string{}
	for _, field := range []struct {
		name, value string
		maximum     int
		multiline   bool
	}{{"name", in.Name, 120, false}, {"tagline", in.Tagline, 160, false}, {"description", in.Description, 2000, true}} {
		invalid := !utf8.ValidString(field.value) || utf8.RuneCountInString(field.value) > field.maximum
		for _, r := range field.value {
			if unicode.IsControl(r) && !(field.multiline && r == '\n') {
				invalid = true
			}
		}
		if invalid {
			fields[field.name] = "Use plain text within the displayed character limit."
		}
	}
	if in.Name == "" {
		fields["name"] = "Enter a shop name."
	}
	if in.ContactEmail != "" {
		parsed, err := mail.ParseAddress(in.ContactEmail)
		if err != nil || parsed.Address != in.ContactEmail || len(in.ContactEmail) > 254 {
			fields["contactEmail"] = "Enter a plain email address, or leave it blank."
		}
	}
	if in.Revision < 1 {
		fields["revision"] = "Reload the saved settings before saving."
	}
	if len(fields) > 0 {
		return Input{}, &ValidationError{Fields: fields}
	}
	return in, nil
}

type Store struct{ pool *pgxpool.Pool }

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }
func (s *Store) Read(ctx context.Context, token string) (Settings, error) {
	digest, err := auth.SessionDigest(token)
	if err != nil {
		return Settings{}, err
	}
	row, err := shopdb.New(s.pool).ReadShop(ctx, digest)
	if errors.Is(err, pgx.ErrNoRows) {
		return Settings{}, auth.ErrCredential
	}
	if err != nil {
		return Settings{}, ErrStorage
	}
	return Settings(row), nil
}
func (s *Store) Save(ctx context.Context, token string, in Input) (Settings, error) {
	digest, err := auth.SessionDigest(token)
	if err != nil {
		return Settings{}, err
	}
	in, err = validate(in)
	if err != nil {
		return Settings{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Settings{}, ErrStorage
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = tx.Rollback(cleanup)
	}()
	q := shopdb.New(tx)
	ownerID, err := q.LockShopSession(ctx, digest)
	if errors.Is(err, pgx.ErrNoRows) {
		return Settings{}, auth.ErrCredential
	}
	if err != nil {
		return Settings{}, ErrStorage
	}
	revision, err := q.LockShopRevision(ctx, ownerID)
	if err != nil {
		return Settings{}, ErrStorage
	}
	if revision != in.Revision {
		return Settings{}, ErrConflict
	}
	row, err := q.SaveShop(ctx, shopdb.SaveShopParams{Digest: digest, Name: in.Name, Tagline: in.Tagline, Description: in.Description, ContactEmail: in.ContactEmail})
	if errors.Is(err, pgx.ErrNoRows) {
		return Settings{}, auth.ErrCredential
	}
	if err != nil {
		return Settings{}, ErrStorage
	}
	if err = tx.Commit(ctx); err != nil {
		return Settings{}, ErrStorage
	}
	return Settings(row), nil
}
