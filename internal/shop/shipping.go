package shop

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/techize/selloovy/internal/auth"
	"github.com/techize/selloovy/internal/delivery"
	"github.com/techize/selloovy/internal/shopdb"
)

func (s *Store) ReadShipping(ctx context.Context, token string) (delivery.Settings, error) {
	d, e := auth.SessionDigest(token)
	if e != nil {
		return delivery.Settings{}, e
	}
	r, e := shopdb.New(s.pool).ReadShipping(ctx, d)
	if errors.Is(e, pgx.ErrNoRows) {
		return delivery.Settings{}, auth.ErrCredential
	}
	if e != nil {
		return delivery.Settings{}, ErrStorage
	}
	var services []delivery.Service
	if json.Unmarshal(r.ShippingServices, &services) != nil {
		return delivery.Settings{}, ErrStorage
	}
	return delivery.Settings{Revision: r.Revision, Services: services}, nil
}
func (s *Store) SaveShipping(ctx context.Context, token string, in delivery.Settings) (delivery.Settings, error) {
	d, e := auth.SessionDigest(token)
	if e != nil {
		return delivery.Settings{}, e
	}
	in, e = delivery.Validate(in)
	if e != nil {
		return delivery.Settings{}, &ValidationError{map[string]string{"services": e.Error()}}
	}
	tx, e := s.pool.Begin(ctx)
	if e != nil {
		return delivery.Settings{}, ErrStorage
	}
	defer func() {
		c, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = tx.Rollback(c)
	}()
	q := shopdb.New(tx)
	owner, e := q.LockShopSession(ctx, d)
	if errors.Is(e, pgx.ErrNoRows) {
		return delivery.Settings{}, auth.ErrCredential
	}
	if e != nil {
		return delivery.Settings{}, ErrStorage
	}
	rev, e := q.LockShopRevision(ctx, owner)
	if e != nil {
		return delivery.Settings{}, ErrStorage
	}
	if rev != in.Revision {
		return delivery.Settings{}, ErrConflict
	}
	data, e := json.Marshal(in.Services)
	if e != nil {
		return delivery.Settings{}, ErrStorage
	}
	r, e := q.SaveShipping(ctx, shopdb.SaveShippingParams{ID: owner, ShippingServices: data})
	if e != nil {
		return delivery.Settings{}, ErrStorage
	}
	if tx.Commit(ctx) != nil {
		return delivery.Settings{}, ErrStorage
	}
	in.Revision = r.Revision
	return in, nil
}
