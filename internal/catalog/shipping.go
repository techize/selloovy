package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"

	"github.com/jackc/pgx/v5"
	"github.com/techize/selloovy/internal/catalogdb"
	"github.com/techize/selloovy/internal/delivery"
)

type ShippingOption struct {
	delivery.Service
	ChargePence int64
}
type ShippingChoice struct {
	Service     delivery.Service
	ChargePence int64
}

func readShipping(ctx context.Context, q *catalogdb.Queries, key string) (delivery.Settings, error) {
	r, e := q.PublicShipping(ctx, key)
	if e != nil {
		return delivery.Settings{}, ErrStorage
	}
	var services []delivery.Service
	if json.Unmarshal(r.ShippingServices, &services) != nil {
		return delivery.Settings{}, ErrStorage
	}
	s, e := delivery.Validate(delivery.Settings{Revision: r.Revision, Services: services})
	if e != nil {
		return delivery.Settings{}, ErrStorage
	}
	return s, nil
}
func resolveShipping(ctx context.Context, q *catalogdb.Queries, key string, b *Basket, choiceData []byte) error {
	s, e := readShipping(ctx, q, key)
	if e != nil {
		return e
	}
	b.ShippingRevision = s.Revision
	b.ShippingOptions = []ShippingOption{}
	var choice ShippingChoice
	if len(choiceData) > 0 && json.Unmarshal(choiceData, &choice) != nil {
		return ErrStorage
	}
	b.SelectedShippingID = choice.Service.ID
	for _, service := range s.Services {
		if !service.Enabled {
			continue
		}
		fee := service.Charge(b.TotalPence)
		b.ShippingOptions = append(b.ShippingOptions, ShippingOption{service, fee})
		if service.ID == choice.Service.ID {
			b.ShippingSelected = true
			b.SelectedShipping = service
			b.ShippingPence = fee
			b.ShippingChanged = fee != choice.ChargePence || !reflect.DeepEqual(service, choice.Service)
		}
	}
	b.GrandTotalPence = b.TotalPence + b.ShippingPence
	return nil
}

// A retained, cookie-owned basket remains accessible when the last product is
// unpublished. This exposes only the already published shop identity; ordinary
// catalogue routes still require a visible product.
func readBasketShop(ctx context.Context, q *catalogdb.Queries, key string) (PublicShop, error) {
	r, e := q.BasketShop(ctx, key)
	if errors.Is(e, pgx.ErrNoRows) {
		return PublicShop{}, ErrNotFound
	}
	if e != nil {
		return PublicShop{}, ErrStorage
	}
	return PublicShop{Name: r.Name, Tagline: r.Tagline, Description: r.Description}, nil
}
