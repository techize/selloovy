package catalog

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/techize/selloovy/internal/auth"
	"github.com/techize/selloovy/internal/catalogdb"
	"github.com/techize/selloovy/internal/shopdb"
)

type Publication struct {
	Revision          int64      `json:"revision"`
	PublishedRevision int64      `json:"publishedRevision"`
	ShopRevision      int64      `json:"shopRevision"`
	Shop              PublicShop `json:"shop"`
	PublicPath        string     `json:"publicPath"`
}
type PublicationInput struct {
	Revision     int64 `json:"revision"`
	ShopRevision int64 `json:"shopRevision"`
	Publish      bool  `json:"publish"`
}

// Public projections deliberately omit contact details, sessions, creation keys and stock counts.
type PublicVariant struct {
	ID                               int64
	Label, SizeLabel, ColourPair     string
	PricePence                       int64
	Availability                     string
	DispatchDaysMin, DispatchDaysMax int
}
type PublicProduct struct {
	Photo                              *Photo
	ID                                 int64
	Name, Description, CertificateName string
	Variants                           []PublicVariant
}
type PublicShop struct {
	Name        string `json:"name"`
	Tagline     string `json:"tagline"`
	Description string `json:"description"`
}
type PublicPage struct {
	Shop      PublicShop
	Products  []PublicProduct
	NextAfter int64
}

var publicKeyPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)

func publicPath(key string, id int64) string { return fmt.Sprintf("/shop/%s/products/%d", key, id) }
func readPublication(ctx context.Context, q *catalogdb.Queries, d []byte, id int64) (Publication, error) {
	row, e := q.ReadPublication(ctx, catalogdb.ReadPublicationParams{Digest: d, ID: id})
	if errors.Is(e, pgx.ErrNoRows) {
		return Publication{}, ErrNotFound
	}
	if e != nil {
		return Publication{}, ErrStorage
	}
	p := Publication{Revision: row.Revision, PublishedRevision: row.PublishedRevision, ShopRevision: row.ShopRevision, Shop: PublicShop{Name: row.ShopName, Tagline: row.ShopTagline, Description: row.ShopDescription}}
	if row.PublishedRevision > 0 {
		p.PublicPath = publicPath(row.PublicKey, id)
	}
	return p, nil
}
func (s *Store) ReadPublication(ctx context.Context, token string, id int64) (Publication, error) {
	d, e := auth.SessionDigest(token)
	if e != nil {
		return Publication{}, e
	}
	return readPublication(ctx, catalogdb.New(s.pool), d, id)
}

// SavePublication serializes with product/variant saves and publishes one reviewed revision.
func (s *Store) SavePublication(ctx context.Context, token string, id int64, in PublicationInput) (Publication, error) {
	d, e := auth.SessionDigest(token)
	if e != nil {
		return Publication{}, e
	}
	if in.Revision < 1 {
		return Publication{}, &ValidationError{map[string]string{"revision": "Reload the product before publishing."}}
	}
	tx, e := s.pool.Begin(ctx)
	if e != nil {
		return Publication{}, ErrStorage
	}
	defer cleanupTransaction(tx)
	owner, e := shopdb.New(tx).LockShopSession(ctx, d)
	if errors.Is(e, pgx.ErrNoRows) {
		return Publication{}, auth.ErrCredential
	}
	if e != nil {
		return Publication{}, ErrStorage
	}
	q := catalogdb.New(tx)
	prior, e := readPublication(ctx, q, d, id)
	if e != nil {
		return Publication{}, e
	}
	if prior.Revision != in.Revision || (in.Publish && prior.ShopRevision != in.ShopRevision) {
		return Publication{}, ErrConflict
	}

	revision, e := q.AdvancePublicationRevision(ctx, catalogdb.AdvancePublicationRevisionParams{ID: owner, ID_2: id, Revision: in.Revision})
	if errors.Is(e, pgx.ErrNoRows) {
		return Publication{}, ErrConflict
	}
	if e != nil {
		return Publication{}, ErrStorage
	}
	if in.Publish {
		p, e := read(ctx, q, d, id)
		if e != nil {
			return Publication{}, e
		}
		maker, e := readMaker(ctx, q, d, id)
		if e != nil {
			return Publication{}, e
		}
		if len(maker.Variants) == 0 {
			return Publication{}, &ValidationError{map[string]string{"variants": "Save at least one variant before publishing."}}
		}
		data := PublicProduct{ID: id, Name: p.Name, Description: p.Description, CertificateName: p.CertificateName, Variants: make([]PublicVariant, 0, len(maker.Variants))}
		for _, v := range maker.Variants {
			data.Variants = append(data.Variants, PublicVariant{ID: v.ID, Label: v.Label, SizeLabel: v.SizeLabel, ColourPair: v.ColourPair, PricePence: v.EffectivePricePence})
		}
		photo, e := readPhoto(ctx, q, d, id)
		if e != nil {
			return Publication{}, e
		}
		photoID := pgtype.Text{}
		if photo.Photo.ID != "" {
			data.Photo = &photo.Photo
			photoID = pgtype.Text{String: photo.Photo.ID, Valid: true}
		}
		snapshot, e := json.Marshal(data)
		if e != nil {
			return Publication{}, ErrStorage
		}
		var key [16]byte
		if _, e = rand.Read(key[:]); e != nil {
			return Publication{}, ErrStorage
		}
		if _, e = q.PublishShop(ctx, catalogdb.PublishShopParams{ID: owner, PublicKey: hex.EncodeToString(key[:])}); e != nil {
			return Publication{}, ErrStorage
		}
		e = q.PublishProduct(ctx, catalogdb.PublishProductParams{ProductID: id, Revision: revision, Snapshot: snapshot, PhotoID: photoID})
	} else {
		e = q.UnpublishProduct(ctx, id)
	}
	if e != nil {
		return Publication{}, ErrStorage
	}
	if e = q.DeleteUnusedPhotos(ctx, id); e != nil {
		return Publication{}, ErrStorage
	}
	data, e := readPublication(ctx, q, d, id)
	if e != nil {
		return Publication{}, e
	}
	if e = tx.Commit(ctx); e != nil {
		return Publication{}, ErrStorage
	}
	return data, nil
}
func readPublicShop(ctx context.Context, q *catalogdb.Queries, key string) (PublicShop, error) {
	row, e := q.PublicShop(ctx, key)
	if errors.Is(e, pgx.ErrNoRows) {
		return PublicShop{}, ErrNotFound
	}
	if e != nil {
		return PublicShop{}, ErrStorage
	}
	return PublicShop{Name: row.Name, Tagline: row.Tagline, Description: row.Description}, nil
}
func (s *Store) PublicList(ctx context.Context, key string, after int64) (PublicPage, error) {
	if !publicKeyPattern.MatchString(key) || after < 0 {
		return PublicPage{}, ErrNotFound
	}
	tx, e := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if e != nil {
		return PublicPage{}, ErrStorage
	}
	defer cleanupTransaction(tx)
	q := catalogdb.New(tx)
	sh, e := readPublicShop(ctx, q, key)
	if e != nil {
		return PublicPage{}, e
	}
	rows, e := q.PublicProducts(ctx, catalogdb.PublicProductsParams{PublicKey: key, AfterID: after})
	if e != nil {
		return PublicPage{}, ErrStorage
	}
	page := PublicPage{Shop: sh, Products: make([]PublicProduct, 0, len(rows))}
	if len(rows) > 50 {
		rows = rows[:50]
		page.NextAfter = rows[49].ID
	}
	for _, row := range rows {
		var p PublicProduct
		if json.Unmarshal(row.Snapshot, &p) != nil || p.ID != row.ID || len(p.Variants) == 0 {
			return PublicPage{}, ErrStorage
		}
		page.Products = append(page.Products, p)
	}
	if e = tx.Commit(ctx); e != nil {
		return PublicPage{}, ErrStorage
	}
	return page, nil
}
func (s *Store) PublicRead(ctx context.Context, key string, id int64) (PublicShop, PublicProduct, error) {
	if !publicKeyPattern.MatchString(key) || id < 1 {
		return PublicShop{}, PublicProduct{}, ErrNotFound
	}
	tx, e := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if e != nil {
		return PublicShop{}, PublicProduct{}, ErrStorage
	}
	defer cleanupTransaction(tx)
	sh, p, e := readPublicProduct(ctx, catalogdb.New(tx), key, id)
	if e != nil {
		return PublicShop{}, PublicProduct{}, e
	}
	if e = tx.Commit(ctx); e != nil {
		return PublicShop{}, PublicProduct{}, ErrStorage
	}
	return sh, p, nil
}

func readPublicProduct(ctx context.Context, q *catalogdb.Queries, key string, id int64) (PublicShop, PublicProduct, error) {
	sh, e := readPublicShop(ctx, q, key)
	if e != nil {
		return PublicShop{}, PublicProduct{}, e
	}
	row, e := q.PublicProduct(ctx, catalogdb.PublicProductParams{PublicKey: key, ID: id})
	if errors.Is(e, pgx.ErrNoRows) {
		return PublicShop{}, PublicProduct{}, ErrNotFound
	}
	if e != nil {
		return PublicShop{}, PublicProduct{}, ErrStorage
	}
	var p PublicProduct
	if json.Unmarshal(row.Snapshot, &p) != nil || p.ID != id || len(p.Variants) == 0 {
		return PublicShop{}, PublicProduct{}, ErrStorage
	}
	stock, e := q.PublicVariantStock(ctx, catalogdb.PublicVariantStockParams{PublicKey: key, ID: id})
	if e != nil {
		return PublicShop{}, PublicProduct{}, ErrStorage
	}
	// Only reviewed variants appear. New variants remain private until republishing.
	for i := range p.Variants {
		v := &p.Variants[i]
		v.Availability = "unavailable"
		for _, live := range stock {
			if live.ID == v.ID {
				v.Availability, v.DispatchDaysMin, v.DispatchDaysMax = availability(live.SupplyMode, live.StockQuantity, row.MadeToOrderFallback)
				break
			}
		}
	}
	return sh, p, nil
}
