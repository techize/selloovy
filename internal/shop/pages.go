package shop

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/techize/selloovy/internal/auth"
	"github.com/techize/selloovy/internal/shopdb"
)

type FAQ struct {
	Question string `json:"question"`
	Answer   string `json:"answer"`
}
type Section struct {
	ID             string  `json:"id"`
	Kind           string  `json:"kind"`
	Heading        string  `json:"heading"`
	Text           string  `json:"text"`
	Theme          string  `json:"theme"`
	ImageProductID int64   `json:"imageProductId"`
	ProductIDs     []int64 `json:"productIds"`
	Questions      []FAQ   `json:"questions"`
	ButtonLabel    string  `json:"buttonLabel"`
	ButtonPath     string  `json:"buttonPath"`
}
type Page struct {
	Slug     string    `json:"slug"`
	Title    string    `json:"title"`
	Sections []Section `json:"sections"`
}
type PageProduct struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	PhotoID    string `json:"photoId"`
	PhotoAlt   string `json:"photoAlt"`
	PricePence int64  `json:"pricePence"`
}

// Pages keeps independently controlled drafts and live snapshots in a bounded shop configuration.
type Pages struct {
	Products     []PageProduct `json:"products"`
	MoreProducts bool          `json:"moreProducts"`
	Revision     int64         `json:"revision"`
	Drafts       []Page        `json:"drafts"`
	Published    []Page        `json:"published"`
	PublicKey    string        `json:"publicKey"`
}
type PageChange struct {
	Revision int64  `json:"revision"`
	Action   string `json:"action"`
	Slug     string `json:"slug"`
	Page     Page   `json:"page"`
}

var slugPattern = regexp.MustCompile(`^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$`)
var sectionIDPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)

func plain(v string, max int, multi bool) bool {
	if !utf8.ValidString(v) || utf8.RuneCountInString(v) > max {
		return false
	}
	for _, r := range v {
		if unicode.IsControl(r) && !(multi && r == '\n') {
			return false
		}
	}
	return true
}
func ValidatePage(p Page) error {
	fail := func() error {
		return &ValidationError{map[string]string{"page": "Check the page slug, title, sections and content limits."}}
	}
	if len(p.Slug) > 60 || !slugPattern.MatchString(p.Slug) || strings.TrimSpace(p.Title) == "" || !plain(p.Title, 120, false) || len(p.Sections) < 1 || len(p.Sections) > 20 {
		return fail()
	}
	ids := map[string]bool{}
	for _, s := range p.Sections {
		if strings.TrimSpace(s.Heading) == "" || !sectionIDPattern.MatchString(s.ID) || ids[s.ID] || !plain(s.Heading, 160, false) || !plain(s.Text, 4000, true) || !plain(s.ButtonLabel, 80, false) {
			return fail()
		}
		ids[s.ID] = true
		if s.Theme != "cream" && s.Theme != "sage" && s.Theme != "ink" {
			return fail()
		}
		if s.ImageProductID < 0 || s.ImageProductID > 9007199254740991 {
			return fail()
		}
		if s.ButtonPath != "" && s.ButtonPath != "/products" && !(strings.HasPrefix(s.ButtonPath, "/pages/") && len(s.ButtonPath) <= 67 && slugPattern.MatchString(strings.TrimPrefix(s.ButtonPath, "/pages/"))) {
			return fail()
		}
		if (s.ButtonLabel == "") != (s.ButtonPath == "") {
			return fail()
		}
		if (s.Kind != "collection" && len(s.ProductIDs) > 0) || (s.Kind != "faq" && len(s.Questions) > 0) || ((s.Kind == "collection" || s.Kind == "faq") && (s.Text != "" || s.ImageProductID != 0 || s.ButtonLabel != "" || s.ButtonPath != "")) {
			return fail()
		}
		switch s.Kind {
		case "hero", "text-image":
			if strings.TrimSpace(s.Heading) == "" {
				return fail()
			}
		case "collection":
			if len(s.ProductIDs) < 1 || len(s.ProductIDs) > 12 {
				return fail()
			}
		case "faq":
			if len(s.Questions) < 1 || len(s.Questions) > 12 {
				return fail()
			}
		default:
			return fail()
		}
		if len(s.ProductIDs) > 12 || len(s.Questions) > 12 {
			return fail()
		}
		seen := map[int64]bool{}
		for _, id := range s.ProductIDs {
			if id < 1 || id > 9007199254740991 || seen[id] {
				return fail()
			}
			seen[id] = true
		}
		for _, q := range s.Questions {
			if strings.TrimSpace(q.Question) == "" || strings.TrimSpace(q.Answer) == "" || !plain(q.Question, 200, false) || !plain(q.Answer, 2000, true) {
				return fail()
			}
		}
	}
	b, e := json.Marshal(p)
	if e != nil || len(b) > 128*1024 {
		return fail()
	}
	return nil
}
func decodePages(r shopdb.ReadPagesRow) (Pages, error) {
	p := Pages{Revision: r.Revision, PublicKey: r.PublicKey}
	if json.Unmarshal(r.PageDrafts, &p.Drafts) != nil || json.Unmarshal(r.PagePublications, &p.Published) != nil {
		return Pages{}, ErrStorage
	}
	return p, nil
}
func (s *Store) ReadPages(ctx context.Context, token string) (Pages, error) {
	d, e := auth.SessionDigest(token)
	if e != nil {
		return Pages{}, e
	}
	r, e := shopdb.New(s.pool).ReadPages(ctx, d)
	if errors.Is(e, pgx.ErrNoRows) {
		return Pages{}, auth.ErrCredential
	}
	if e != nil {
		return Pages{}, ErrStorage
	}
	p, e := decodePages(r)
	if e != nil {
		return Pages{}, e
	}
	if e = pageProducts(ctx, shopdb.New(s.pool), d, &p); e != nil {
		return Pages{}, e
	}
	return p, nil
}

// ChangePage serializes against other owner/settings writes and publishes only saved content.
func (s *Store) ChangePage(ctx context.Context, token string, in PageChange) (Pages, error) {
	d, e := auth.SessionDigest(token)
	if e != nil {
		return Pages{}, e
	}
	if in.Revision < 1 || len(in.Slug) > 60 || !slugPattern.MatchString(in.Slug) {
		return Pages{}, &ValidationError{map[string]string{"page": "Reload the page and use a valid slug."}}
	}
	if in.Action == "save" {
		if in.Page.Slug != in.Slug {
			return Pages{}, &ValidationError{map[string]string{"page": "The page slug must match."}}
		}
		in.Page.Sections = append([]Section{}, in.Page.Sections...)
		for i := range in.Page.Sections {
			section := &in.Page.Sections[i]
			if section.ProductIDs == nil {
				section.ProductIDs = []int64{}
			}
			if section.Questions == nil {
				section.Questions = []FAQ{}
			}
		}
		if e = ValidatePage(in.Page); e != nil {
			return Pages{}, e
		}
	} else if in.Action != "publish" && in.Action != "unpublish" && in.Action != "delete" {
		return Pages{}, &ValidationError{map[string]string{"page": "Unknown page action."}}
	}
	tx, e := s.pool.Begin(ctx)
	if e != nil {
		return Pages{}, ErrStorage
	}
	defer func() {
		c, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = tx.Rollback(c)
	}()
	q := shopdb.New(tx)
	owner, e := q.LockShopSession(ctx, d)
	if errors.Is(e, pgx.ErrNoRows) {
		return Pages{}, auth.ErrCredential
	}
	if e != nil {
		return Pages{}, ErrStorage
	}
	rev, e := q.LockShopRevision(ctx, owner)
	if e != nil {
		return Pages{}, ErrStorage
	}
	if rev != in.Revision {
		return Pages{}, ErrConflict
	}
	r, e := q.ReadPages(ctx, d)
	if e != nil {
		return Pages{}, ErrStorage
	}
	p, e := decodePages(r)
	if e != nil {
		return Pages{}, e
	}
	find := func(a []Page) int {
		for i, v := range a {
			if v.Slug == in.Slug {
				return i
			}
		}
		return -1
	}
	di, pi := find(p.Drafts), find(p.Published)
	invalid := func(message string) (Pages, error) {
		return Pages{}, &ValidationError{map[string]string{"page": message}}
	}
	switch in.Action {
	case "save":
		if di < 0 {
			if len(p.Drafts) >= 20 {
				return invalid("A shop can have up to 20 pages.")
			}
			p.Drafts = append(p.Drafts, in.Page)
		} else {
			p.Drafts[di] = in.Page
		}
	case "publish":
		if di < 0 || p.PublicKey == "" {
			return invalid("Save this page and publish a product before publishing the page.")
		}
		draft := p.Drafts[di]
		if e = ValidatePage(draft); e != nil {
			return Pages{}, e
		}
		for _, section := range draft.Sections {
			if strings.HasPrefix(section.ButtonPath, "/pages/") {
				target := strings.TrimPrefix(section.ButtonPath, "/pages/")
				found := target == draft.Slug && target != "home"
				for _, page := range p.Published {
					if page.Slug == target && target != "home" {
						found = true
					}
				}
				if !found {
					return invalid("Publish the button's destination page first, or choose All creations.")
				}
			}

			ids := append([]int64{}, section.ProductIDs...)
			if section.ImageProductID > 0 {
				ids = append(ids, section.ImageProductID)
			}
			for _, id := range ids {
				_, e = q.PublishedPageProduct(ctx, shopdb.PublishedPageProductParams{ID: owner, ID_2: id})
				if errors.Is(e, pgx.ErrNoRows) {
					return invalid("Every selected product must be published in your shop.")
				}
				if e != nil {
					return Pages{}, ErrStorage
				}
			}
		}
		if pi < 0 {
			if len(p.Published) >= 20 {
				return invalid("A shop can publish up to 20 pages.")
			}
			p.Published = append(p.Published, draft)
		} else {
			p.Published[pi] = draft
		}
	case "unpublish":
		if pi < 0 {
			return invalid("This page is not published.")
		}
		p.Published = append(p.Published[:pi], p.Published[pi+1:]...)
	case "delete":
		if pi >= 0 {
			return invalid("Unpublish the page before deleting its draft.")
		}
		if di < 0 {
			return invalid("This draft does not exist.")
		}
		p.Drafts = append(p.Drafts[:di], p.Drafts[di+1:]...)
	}
	drafts, _ := json.Marshal(p.Drafts)
	published, _ := json.Marshal(p.Published)
	if len(drafts) > 220*1024 || len(published) > 220*1024 {
		return invalid("Page content exceeds this shop's storage limit.")
	}
	p.Revision, e = q.SavePages(ctx, shopdb.SavePagesParams{ID: owner, PageDrafts: drafts, PagePublications: published})
	if e != nil {
		return Pages{}, ErrStorage
	}
	if e = pageProducts(ctx, q, d, &p); e != nil {
		return Pages{}, e
	}
	if tx.Commit(ctx) != nil {
		return Pages{}, ErrStorage
	}
	return p, nil
}
func (s *Store) PublicPages(ctx context.Context, key string) ([]Page, error) {
	raw, e := shopdb.New(s.pool).PublicPages(ctx, key)
	if errors.Is(e, pgx.ErrNoRows) {
		return []Page{}, nil
	}
	if e != nil {
		return nil, ErrStorage
	}
	var p []Page
	if json.Unmarshal(raw, &p) != nil {
		return nil, ErrStorage
	}
	return p, nil
}

func pageProducts(ctx context.Context, q *shopdb.Queries, d []byte, p *Pages) error {
	rows, e := q.PageProducts(ctx, d)
	if e != nil {
		return ErrStorage
	}
	p.Products = []PageProduct{}
	if len(rows) > 500 {
		p.MoreProducts = true
		rows = rows[:500]
	}
	for _, r := range rows {
		var snapshot struct {
			Name     string
			Photo    *struct{ ID, Alt string }
			Variants []struct{ PricePence int64 }
		}
		if json.Unmarshal(r.Snapshot, &snapshot) != nil {
			return ErrStorage
		}
		item := PageProduct{ID: r.ProductID, Name: snapshot.Name}
		if snapshot.Photo != nil && r.PhotoID.Valid {
			item.PhotoID = r.PhotoID.String
			item.PhotoAlt = snapshot.Photo.Alt
		}
		for _, v := range snapshot.Variants {
			if item.PricePence == 0 || v.PricePence < item.PricePence {
				item.PricePence = v.PricePence
			}
		}
		p.Products = append(p.Products, item)
	}
	return nil
}
