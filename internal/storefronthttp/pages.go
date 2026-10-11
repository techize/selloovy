package storefronthttp

import (
	"context"
	"errors"

	"github.com/techize/selloovy/internal/catalog"
	"github.com/techize/selloovy/internal/shop"
)

type PagesBackend interface {
	PublicPages(context.Context, string) ([]shop.Page, error)
}
type renderedSection struct {
	shop.Section
	Image    *catalog.PublicProduct
	Products []catalog.PublicProduct
}

func loadPage(ctx context.Context, b Backend, pages PagesBackend, p *page, slug string) error {
	published, e := pages.PublicPages(ctx, p.Key)
	if e != nil {
		return e
	}
	for _, candidate := range published {
		if candidate.Slug != "home" {
			p.Links = append(p.Links, candidate)
		}
		if candidate.Slug == slug {
			copy := candidate
			p.Content = &copy
		}
	}
	if p.Content == nil {
		return nil
	}
	cache := map[int64]*catalog.PublicProduct{}
	resolve := func(id int64) (*catalog.PublicProduct, error) {
		if product, ok := cache[id]; ok {
			return product, nil
		}
		_, product, e := b.PublicRead(ctx, p.Key, id)
		if errors.Is(e, catalog.ErrNotFound) {
			cache[id] = nil
			return nil, nil
		}
		if e != nil {
			return nil, e
		}
		cache[id] = &product
		return &product, nil
	}
	for _, s := range p.Content.Sections {
		section := renderedSection{Section: s}
		if s.ImageProductID > 0 {
			product, e := resolve(s.ImageProductID)
			if e != nil {
				return e
			}
			section.Image = product
		}
		for _, id := range s.ProductIDs {
			product, e := resolve(id)
			if e != nil {
				return e
			}
			if product != nil {
				section.Products = append(section.Products, *product)
			}
		}
		p.Sections = append(p.Sections, section)
	}
	return nil
}
