// Package catalog owns product drafts, maker variants and explicit public projections.
package catalog

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/techize/selloovy/internal/auth"
	"github.com/techize/selloovy/internal/catalogdb"
	"github.com/techize/selloovy/internal/shopdb"
)

var ErrStorage = errors.New("products are unavailable")
var ErrNotFound = errors.New("product not found")
var ErrConflict = errors.New("product changed or creation key was reused")

type ValidationError struct{ Fields map[string]string }

func (e *ValidationError) Error() string { return "check product fields" }

type Product struct {
	ID              int64  `json:"id"`
	Name            string `json:"name"`
	Description     string `json:"description"`
	PricePence      int64  `json:"pricePence"`
	CertificateName string `json:"certificateName"`
	Revision        int64  `json:"revision"`
}
type Input struct {
	Name            string `json:"name"`
	Description     string `json:"description"`
	PricePence      int64  `json:"pricePence"`
	CertificateName string `json:"certificateName"`
	Revision        int64  `json:"revision"`
	CreationKey     string `json:"creationKey"`
}
type Page struct {
	Products  []Product `json:"products"`
	NextAfter int64     `json:"nextAfter"`
}

var creationKey = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func validate(in Input, create bool) (Input, error) {
	in.Name = strings.TrimSpace(in.Name)
	in.Description = strings.TrimSpace(in.Description)
	fields := map[string]string{}
	for _, f := range []struct {
		name, value string
		max         int
		multiline   bool
	}{{"name", in.Name, 160, false}, {"description", in.Description, 5000, true}} {
		invalid := !utf8.ValidString(f.value) || utf8.RuneCountInString(f.value) > f.max
		for _, r := range f.value {
			if unicode.IsControl(r) && !(f.multiline && r == '\n') {
				invalid = true
			}
		}
		if invalid {
			fields[f.name] = "Use plain text within the displayed character limit."
		}
	}
	if in.Name == "" {
		fields["name"] = "Enter a product name."
	}
	if in.PricePence < 1 || in.PricePence > 100000000 {
		fields["pricePence"] = "Enter a price from £0.01 to £1,000,000.00."
	}
	if in.CertificateName != "none" && in.CertificateName != "optional" && in.CertificateName != "required" {
		fields["certificateName"] = "Choose a certificate name setting."
	}
	if create {
		if !creationKey.MatchString(in.CreationKey) || in.Revision != 0 {
			fields["creationKey"] = "Start a new product draft."
		}
	} else if in.CreationKey != "" || in.Revision < 1 {
		fields["revision"] = "Reload the saved product."
	}
	if len(fields) > 0 {
		return Input{}, &ValidationError{fields}
	}
	return in, nil
}

type Store struct{ pool *pgxpool.Pool }

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool} }
func (s *Store) List(ctx context.Context, token string, after int64) (Page, error) {
	digest, e := auth.SessionDigest(token)
	if e != nil {
		return Page{}, e
	}
	// Distinguish an empty catalogue from an expired session, including direct store callers.
	if _, e = shopdb.New(s.pool).ReadShop(ctx, digest); e != nil {
		if errors.Is(e, pgx.ErrNoRows) {
			return Page{}, auth.ErrCredential
		}
		return Page{}, ErrStorage
	}
	rows, e := catalogdb.New(s.pool).ListProducts(ctx, catalogdb.ListProductsParams{Digest: digest, AfterID: after})
	if e != nil {
		return Page{}, ErrStorage
	}
	page := Page{Products: make([]Product, 0, len(rows))}
	if len(rows) > 50 {
		rows = rows[:50]
		page.NextAfter = rows[49].ID
	}
	for _, r := range rows {
		page.Products = append(page.Products, Product(r))
	}
	return page, nil
}
func read(ctx context.Context, q *catalogdb.Queries, digest []byte, id int64) (Product, error) {
	row, e := q.ReadProduct(ctx, catalogdb.ReadProductParams{Digest: digest, ID: id})
	if errors.Is(e, pgx.ErrNoRows) {
		return Product{}, ErrNotFound
	}
	if e != nil {
		return Product{}, ErrStorage
	}
	return Product(row), nil
}
func (s *Store) Read(ctx context.Context, token string, id int64) (Product, error) {
	d, e := auth.SessionDigest(token)
	if e != nil {
		return Product{}, e
	}
	return read(ctx, catalogdb.New(s.pool), d, id)
}
func (s *Store) Save(ctx context.Context, token string, id int64, in Input) (Product, error) {
	digest, e := auth.SessionDigest(token)
	if e != nil {
		return Product{}, e
	}
	in, e = validate(in, id == 0)
	if e != nil {
		return Product{}, e
	}
	tx, e := s.pool.Begin(ctx)
	if e != nil {
		return Product{}, ErrStorage
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = tx.Rollback(cleanup)
	}()
	owner, e := shopdb.New(tx).LockShopSession(ctx, digest)
	if errors.Is(e, pgx.ErrNoRows) {
		return Product{}, auth.ErrCredential
	}
	if e != nil {
		return Product{}, ErrStorage
	}
	q := catalogdb.New(tx)
	var product Product
	if id == 0 {
		encoded, _ := json.Marshal(in)
		hash := sha256.Sum256(encoded)
		prior, err := q.FindCreation(ctx, catalogdb.FindCreationParams{ID: owner, CreationKey: in.CreationKey})
		if err == nil {
			if !bytes.Equal(hash[:], prior.CreationHash) {
				return Product{}, ErrConflict
			}
			product, e = read(ctx, q, digest, prior.ID)
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return Product{}, ErrStorage
		} else {
			row, err := q.CreateProduct(ctx, catalogdb.CreateProductParams{Digest: digest, Name: in.Name, Description: in.Description, PricePence: in.PricePence, CertificateName: in.CertificateName, CreationKey: in.CreationKey, CreationHash: hash[:]})
			e = err
			product = Product(row)
		}
	} else {
		prior, err := read(ctx, q, digest, id)
		if err != nil {
			return Product{}, err
		}
		if prior.Revision != in.Revision {
			return Product{}, ErrConflict
		}
		row, err := q.UpdateProduct(ctx, catalogdb.UpdateProductParams{Digest: digest, ID: id, Name: in.Name, Description: in.Description, PricePence: in.PricePence, CertificateName: in.CertificateName, Revision: in.Revision})
		e = err
		product = Product(row)
	}
	if errors.Is(e, pgx.ErrNoRows) {
		return Product{}, auth.ErrCredential
	}
	if e != nil {
		return Product{}, ErrStorage
	}
	if e = tx.Commit(ctx); e != nil {
		return Product{}, ErrStorage
	}
	return product, nil
}
