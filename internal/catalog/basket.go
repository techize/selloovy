package catalog

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/techize/selloovy/internal/catalogdb"
)

type BasketItem struct {
	ID                        string
	ProductID, VariantID      int64
	OwnerName                 string
	Quantity                  int64
	ProductName, VariantLabel string
	UnitPricePence            int64
}
type BasketLine struct {
	BasketItem
	TotalPence         int64
	Available, Changed bool
	Availability       string
	CertificatePolicy  string
}
type Basket struct {
	Revision   int64
	Shop       PublicShop
	Lines      []BasketLine
	TotalPence int64
	Complete   bool
}
type BasketChange struct {
	Revision                       int64
	LineID                         string // Empty for an add; populated for an update/removal.
	ProductID, VariantID, Quantity int64
	OwnerName                      string
}

func basketDigest(token string) ([]byte, error) {
	if len(token) != 64 {
		return nil, ErrNotFound
	}
	b, e := hex.DecodeString(token)
	if e != nil || len(b) != 32 {
		return nil, ErrNotFound
	}
	d := sha256.Sum256(b)
	return d[:], nil
}
func basketWriteError(e error) error {
	var pg *pgconn.PgError
	if errors.As(e, &pg) && (pg.Code == "40001" || pg.Code == "40P01") {
		return ErrConflict
	}
	return ErrStorage
}
func basketValidation(message string) error {
	return &ValidationError{map[string]string{"basket": message}}
}
func certificateName(policy, name string) (string, error) {
	name = strings.TrimSpace(name)
	invalid := !utf8.ValidString(name) || utf8.RuneCountInString(name) > 80
	for _, r := range name {
		if unicode.IsControl(r) {
			invalid = true
		}
	}
	if invalid {
		return "", basketValidation("Enter an owner name of up to 80 plain-text characters.")
	}
	if policy == "required" && name == "" {
		return "", basketValidation("Enter the owner’s name for this certificate.")
	}
	if policy == "none" && name != "" {
		return "", basketValidation("This product does not offer a personalised certificate.")
	}
	return name, nil
}
func decodeBasket(data []byte) ([]BasketItem, error) {
	var items []BasketItem
	if json.Unmarshal(data, &items) != nil || len(items) > 40 {
		return nil, ErrStorage
	}
	return items, nil
}
func resolveBasket(ctx context.Context, q *catalogdb.Queries, key string, rev int64, items []BasketItem) (Basket, error) {
	shop, e := readPublicShop(ctx, q, key)
	if e != nil {
		return Basket{}, e
	}
	result := Basket{Revision: rev, Shop: shop, Complete: true, Lines: make([]BasketLine, 0, len(items))}
	for _, item := range items {
		if !publicKeyPattern.MatchString(item.ID) || item.Quantity < 1 || item.Quantity > 99 || item.UnitPricePence < 1 || item.UnitPricePence > 100000000 {
			return Basket{}, ErrStorage
		}
		line := BasketLine{BasketItem: item}
		_, p, e := readPublicProduct(ctx, q, key, item.ProductID)
		if e != nil && !errors.Is(e, ErrNotFound) {
			return Basket{}, e
		}
		if e == nil {
			for _, v := range p.Variants {
				if v.ID == item.VariantID {
					line.Changed = v.PricePence != item.UnitPricePence || p.Name != item.ProductName || v.Label != item.VariantLabel
					line.CertificatePolicy = p.CertificateName
					line.ProductName = p.Name
					line.VariantLabel = v.Label
					line.UnitPricePence = v.PricePence
					line.Availability = v.Availability
					_, nameErr := certificateName(p.CertificateName, item.OwnerName)
					line.Available = v.Availability != "unavailable" && nameErr == nil
					break
				}
			}
		}
		line.TotalPence = line.UnitPricePence * line.Quantity
		result.TotalPence += line.TotalPence
		if !line.Available {
			result.Complete = false
		}
		result.Lines = append(result.Lines, line)
	}
	return result, nil
}
func (s *Store) ReadBasket(ctx context.Context, key, token string) (Basket, error) {
	if !publicKeyPattern.MatchString(key) {
		return Basket{}, ErrNotFound
	}
	d, e := basketDigest(token)
	if e != nil {
		return Basket{}, e
	}
	tx, e := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if e != nil {
		return Basket{}, ErrStorage
	}
	defer cleanupTransaction(tx)
	q := catalogdb.New(tx)
	row, e := q.ReadBasket(ctx, catalogdb.ReadBasketParams{PublicKey: key, Digest: d})
	var items []BasketItem
	if errors.Is(e, pgx.ErrNoRows) {
		e = nil
	} else if e == nil {
		items, e = decodeBasket(row.Lines)
	}
	if e != nil {
		return Basket{}, ErrStorage
	}
	result, e := resolveBasket(ctx, q, key, row.Revision, items)
	if e != nil {
		return Basket{}, e
	}
	if tx.Commit(ctx) != nil {
		return Basket{}, ErrStorage
	}
	return result, nil
}
func (s *Store) ChangeBasket(ctx context.Context, key, token string, in BasketChange) (Basket, error) {
	if !publicKeyPattern.MatchString(key) {
		return Basket{}, ErrNotFound
	}
	d, e := basketDigest(token)
	if e != nil {
		return Basket{}, e
	}
	if in.Revision < 0 || in.Quantity < 0 || in.Quantity > 99 || (in.LineID == "" && in.Quantity == 0) {
		return Basket{}, basketValidation("Choose a quantity from 1 to 99.")
	}
	tx, e := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if e != nil {
		return Basket{}, ErrStorage
	}
	defer cleanupTransaction(tx)
	q := catalogdb.New(tx)
	if _, e = readPublicShop(ctx, q, key); e != nil {
		return Basket{}, e
	}
	if e = q.PurgeExpiredBaskets(ctx, key); e != nil {
		return Basket{}, basketWriteError(e)
	}
	if e = q.CreateBasket(ctx, catalogdb.CreateBasketParams{PublicKey: key, Digest: d}); e != nil {
		return Basket{}, basketWriteError(e)
	}
	row, e := q.LockBasket(ctx, catalogdb.LockBasketParams{PublicKey: key, Digest: d})
	if errors.Is(e, pgx.ErrNoRows) {
		return Basket{}, ErrConflict
	}
	if e != nil {
		return Basket{}, basketWriteError(e)
	}
	if row.Revision != in.Revision {
		return Basket{}, ErrConflict
	}
	items, e := decodeBasket(row.Lines)
	if e != nil {
		return Basket{}, e
	}
	index := -1
	if in.LineID != "" {
		for i, item := range items {
			if item.ID == in.LineID {
				index = i
				in.ProductID = item.ProductID
				in.VariantID = item.VariantID
				break
			}
		}
		if index < 0 {
			return Basket{}, ErrNotFound
		}
	}
	if in.Quantity == 0 {
		items = append(items[:index], items[index+1:]...)
	} else {
		_, p, e := readPublicProduct(ctx, q, key, in.ProductID)
		if e != nil {
			return Basket{}, e
		}
		var variant PublicVariant
		for _, v := range p.Variants {
			if v.ID == in.VariantID {
				variant = v
				break
			}
		}
		if variant.ID == 0 {
			return Basket{}, ErrNotFound
		}
		if variant.Availability == "unavailable" {
			return Basket{}, basketValidation("This option is currently unavailable. Choose another option.")
		}
		name, e := certificateName(p.CertificateName, in.OwnerName)
		if e != nil {
			return Basket{}, e
		}
		quantity := in.Quantity
		if in.LineID == "" {
			for i, item := range items {
				if item.ProductID == p.ID && item.VariantID == variant.ID && item.OwnerName == name {
					index = i
					quantity += item.Quantity
					break
				}
			}
		}
		if quantity > 99 {
			return Basket{}, basketValidation("A basket line can contain at most 99 items.")
		}
		item := BasketItem{ProductID: p.ID, VariantID: variant.ID, OwnerName: name, Quantity: quantity, ProductName: p.Name, VariantLabel: variant.Label, UnitPricePence: variant.PricePence}
		if index >= 0 {
			item.ID = items[index].ID
			items[index] = item
		} else {
			if len(items) >= 40 {
				return Basket{}, basketValidation("Your basket can contain at most 40 different lines.")
			}
			var id [16]byte
			if _, e = rand.Read(id[:]); e != nil {
				return Basket{}, ErrStorage
			}
			item.ID = hex.EncodeToString(id[:])
			items = append(items, item)
		}
	}
	data, e := json.Marshal(items)
	if e != nil {
		return Basket{}, ErrStorage
	}
	n, e := q.SaveBasket(ctx, catalogdb.SaveBasketParams{PublicKey: key, Digest: d, Lines: data, Revision: row.Revision})
	if e != nil {
		return Basket{}, basketWriteError(e)
	}
	if n != 1 {
		return Basket{}, ErrConflict
	}
	result, e := resolveBasket(ctx, q, key, row.Revision+1, items)
	if e != nil {
		return Basket{}, e
	}
	if e = tx.Commit(ctx); e != nil {
		return Basket{}, basketWriteError(e)
	}
	return result, nil
}
