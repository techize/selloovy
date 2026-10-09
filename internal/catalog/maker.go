package catalog

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/techize/selloovy/internal/auth"
	"github.com/techize/selloovy/internal/catalogdb"
	"github.com/techize/selloovy/internal/shopdb"
)

// VariantInput records stock on hand, not checkout holds or manufacturing costs.
type VariantInput struct {
	ID            int64  `json:"id"`
	Label         string `json:"label"`
	SizeLabel     string `json:"sizeLabel"`
	ColourPair    string `json:"colourPair"`
	PricePence    *int64 `json:"pricePence"`
	SupplyMode    string `json:"supplyMode"`
	StockQuantity int64  `json:"stockQuantity"`
}
type MakerInput struct {
	Revision            int64          `json:"revision"`
	MadeToOrderFallback bool           `json:"madeToOrderFallback"`
	Variants            []VariantInput `json:"variants"`
}
type Variant struct {
	VariantInput
	EffectivePricePence int64  `json:"effectivePricePence"`
	Availability        string `json:"availability"`
	DispatchDaysMin     int    `json:"dispatchDaysMin"`
	DispatchDaysMax     int    `json:"dispatchDaysMax"`
}
type MakerSettings struct {
	ProductID           int64     `json:"productId"`
	ProductName         string    `json:"productName"`
	BasePricePence      int64     `json:"basePricePence"`
	Revision            int64     `json:"revision"`
	MadeToOrderFallback bool      `json:"madeToOrderFallback"`
	Variants            []Variant `json:"variants"`
	ShippingDaysMin     int       `json:"shippingDaysMin"`
	ShippingDaysMax     int       `json:"shippingDaysMax"`
}

func validateMaker(in MakerInput) (MakerInput, error) {
	fields := map[string]string{}
	if in.Revision < 1 {
		fields["revision"] = "Reload the saved product before saving."
	}
	if len(in.Variants) < 1 || len(in.Variants) > 50 {
		fields["variants"] = "Use between 1 and 50 variants."
	}
	labels := map[string]bool{}
	ids := map[int64]bool{}
	// Copy the slice so normalization never mutates a caller's draft.
	in.Variants = append([]VariantInput(nil), in.Variants...)
	for i := range in.Variants {
		v := &in.Variants[i]
		prefix := fmt.Sprintf("variants.%d.", i)
		v.Label = strings.TrimSpace(v.Label)
		v.SizeLabel = strings.TrimSpace(v.SizeLabel)
		v.ColourPair = strings.TrimSpace(v.ColourPair)
		for _, f := range []struct {
			name, value string
			max         int
		}{{"label", v.Label, 120}, {"sizeLabel", v.SizeLabel, 80}, {"colourPair", v.ColourPair, 120}} {
			invalid := !utf8.ValidString(f.value) || utf8.RuneCountInString(f.value) > f.max
			for _, r := range f.value {
				if unicode.IsControl(r) {
					invalid = true
				}
			}
			if invalid {
				fields[prefix+f.name] = "Use plain text within the displayed limit."
			}
		}
		labelKey := strings.ToLower(v.Label)
		if v.Label == "" || labels[labelKey] {
			fields[prefix+"label"] = "Use a unique variant name."
		}
		labels[labelKey] = true
		if v.ID < 0 || v.ID > 9007199254740991 || (v.ID > 0 && ids[v.ID]) {
			fields[prefix+"id"] = "Reload the saved variants."
		}
		ids[v.ID] = true
		if v.PricePence != nil && (*v.PricePence < 1 || *v.PricePence > 100000000) {
			fields[prefix+"pricePence"] = "Enter £0.01–£1,000,000.00, or leave blank for the product price."
		}
		if v.SupplyMode != "stocked" && v.SupplyMode != "made_to_order" {
			fields[prefix+"supplyMode"] = "Choose stocked or made to order."
		}
		if v.StockQuantity < 0 || v.StockQuantity > 1000000 {
			fields[prefix+"stockQuantity"] = "Enter a whole stock count from 0 to 1,000,000."
		}
		if v.SupplyMode == "made_to_order" && v.StockQuantity != 0 {
			fields[prefix+"stockQuantity"] = "Use zero stock for a made-to-order variant."
		}
	}
	if len(fields) > 0 {
		return MakerInput{}, &ValidationError{fields}
	}
	return in, nil
}
func availability(mode string, stock int64, fallback bool) (string, int, int) {
	if mode == "stocked" && stock > 0 {
		return "in_stock", 2, 2
	}
	if mode == "made_to_order" || (mode == "stocked" && fallback) {
		return "made_to_order", 5, 7
	}
	return "unavailable", 0, 0
}
func readMaker(ctx context.Context, q *catalogdb.Queries, digest []byte, id int64) (MakerSettings, error) {
	p, e := q.ReadMakerProduct(ctx, catalogdb.ReadMakerProductParams{Digest: digest, ID: id})
	if errors.Is(e, pgx.ErrNoRows) {
		return MakerSettings{}, ErrNotFound
	}
	if e != nil {
		return MakerSettings{}, ErrStorage
	}
	rows, e := q.ListMakerVariants(ctx, catalogdb.ListMakerVariantsParams{Digest: digest, ID: id})
	if e != nil || len(rows) > 50 {
		return MakerSettings{}, ErrStorage
	}
	data := MakerSettings{ProductID: p.ID, ProductName: p.Name, BasePricePence: p.PricePence, Revision: p.Revision, MadeToOrderFallback: p.MadeToOrderFallback, Variants: make([]Variant, 0, len(rows)), ShippingDaysMin: 3, ShippingDaysMax: 4}
	for _, r := range rows {
		v := Variant{VariantInput: VariantInput{ID: r.ID, Label: r.Label, SizeLabel: r.SizeLabel, ColourPair: r.ColourPair, SupplyMode: r.SupplyMode, StockQuantity: r.StockQuantity}, EffectivePricePence: p.PricePence}
		if r.PricePence.Valid {
			price := r.PricePence.Int64
			v.PricePence = &price
			v.EffectivePricePence = price
		}
		v.Availability, v.DispatchDaysMin, v.DispatchDaysMax = availability(v.SupplyMode, v.StockQuantity, p.MadeToOrderFallback)
		data.Variants = append(data.Variants, v)
	}
	return data, nil
}
func cleanupTransaction(tx pgx.Tx) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = tx.Rollback(ctx)
}

// ReadMaker uses one snapshot for the parent revision and its variants.
func (s *Store) ReadMaker(ctx context.Context, token string, id int64) (MakerSettings, error) {
	digest, e := auth.SessionDigest(token)
	if e != nil {
		return MakerSettings{}, e
	}
	tx, e := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if e != nil {
		return MakerSettings{}, ErrStorage
	}
	defer cleanupTransaction(tx)
	data, e := readMaker(ctx, catalogdb.New(tx), digest, id)
	if e != nil {
		return MakerSettings{}, e
	}
	if e = tx.Commit(ctx); e != nil {
		return MakerSettings{}, ErrStorage
	}
	return data, nil
}

// SaveMaker requires every existing variant. Omission cannot silently delete stock.
func (s *Store) SaveMaker(ctx context.Context, token string, id int64, in MakerInput) (MakerSettings, error) {
	digest, e := auth.SessionDigest(token)
	if e != nil {
		return MakerSettings{}, e
	}
	in, e = validateMaker(in)
	if e != nil {
		return MakerSettings{}, e
	}
	tx, e := s.pool.Begin(ctx)
	if e != nil {
		return MakerSettings{}, ErrStorage
	}
	defer cleanupTransaction(tx)
	ownerID, e := shopdb.New(tx).LockShopSession(ctx, digest)
	if errors.Is(e, pgx.ErrNoRows) {
		return MakerSettings{}, auth.ErrCredential
	} else if e != nil {
		return MakerSettings{}, ErrStorage
	}
	q := catalogdb.New(tx)
	prior, e := readMaker(ctx, q, digest, id)
	if e != nil {
		return MakerSettings{}, e
	}
	if prior.Revision != in.Revision {
		return MakerSettings{}, ErrConflict
	}
	previousStock := map[int64]int64{}
	existing := map[int64]bool{}
	for _, v := range prior.Variants {
		existing[v.ID] = true
		previousStock[v.ID] = v.StockQuantity
	}
	for _, v := range in.Variants {
		if v.ID != 0 {
			if !existing[v.ID] {
				return MakerSettings{}, ErrNotFound
			}
			delete(existing, v.ID)
		}
	}
	if len(existing) > 0 {
		return MakerSettings{}, &ValidationError{map[string]string{"variants": "Keep all saved variants. Removing saved stock is not available yet."}}
	}
	if _, e = q.SaveMakerPolicy(ctx, catalogdb.SaveMakerPolicyParams{Digest: digest, ID: id, MadeToOrderFallback: in.MadeToOrderFallback, Revision: in.Revision}); errors.Is(e, pgx.ErrNoRows) {
		return MakerSettings{}, auth.ErrCredential
	} else if e != nil {
		return MakerSettings{}, ErrStorage
	}
	for _, v := range in.Variants {
		price := pgtype.Int8{}
		if v.PricePence != nil {
			price = pgtype.Int8{Int64: *v.PricePence, Valid: true}
		}
		variantID := v.ID
		reason := "admin_count"
		if v.ID == 0 {
			reason = "initial_stock"
			variantID, e = q.AddMakerVariant(ctx, catalogdb.AddMakerVariantParams{ProductID: id, Label: v.Label, LabelKey: strings.ToLower(v.Label), SizeLabel: v.SizeLabel, ColourPair: v.ColourPair, PricePence: price, SupplyMode: v.SupplyMode, StockQuantity: v.StockQuantity})
		} else {
			var count int64
			count, e = q.UpdateMakerVariant(ctx, catalogdb.UpdateMakerVariantParams{ProductID: id, ID: v.ID, Label: v.Label, LabelKey: strings.ToLower(v.Label), SizeLabel: v.SizeLabel, ColourPair: v.ColourPair, PricePence: price, SupplyMode: v.SupplyMode, StockQuantity: v.StockQuantity})
			if e == nil && count != 1 {
				return MakerSettings{}, ErrNotFound
			}
		}
		if e != nil {
			return MakerSettings{}, ErrStorage
		}
		if previousStock[v.ID] != v.StockQuantity {
			if e = q.RecordStockAdjustment(ctx, catalogdb.RecordStockAdjustmentParams{VariantID: variantID, OwnerID: ownerID, PreviousQuantity: previousStock[v.ID], NewQuantity: v.StockQuantity, Reason: reason}); e != nil {
				return MakerSettings{}, ErrStorage
			}
		}
	}
	data, e := readMaker(ctx, q, digest, id)
	if e != nil {
		return MakerSettings{}, e
	}
	if e = tx.Commit(ctx); e != nil {
		return MakerSettings{}, ErrStorage
	}
	return data, nil
}
