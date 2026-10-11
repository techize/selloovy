package shop

import (
	"strings"
	"testing"
)

func TestPageValidationRejectsUnsafeLinksAndMalformedSections(t *testing.T) {
	base := Page{Slug: "home", Title: "Welcome", Sections: []Section{{ID: "00000000000000000000000000000001", Kind: "hero", Heading: "Hello", Theme: "cream", ProductIDs: []int64{}, Questions: []FAQ{}}}}
	if ValidatePage(base) != nil {
		t.Fatal("valid page rejected")
	}
	for _, path := range []string{"javascript:alert(1)", "https://example.com", "//example.com", "/pages/../admin", "/pages/home?x=1", "/products/1", "/pages/"} {
		p := base
		p.Sections = append([]Section{}, base.Sections...)
		p.Sections[0].ButtonLabel = "Visit"
		p.Sections[0].ButtonPath = path
		if ValidatePage(p) == nil {
			t.Fatal("unsafe link accepted")
		}
	}
	for _, mutate := range []func(*Page){func(p *Page) { p.Slug = "../admin" }, func(p *Page) { p.Title = "" }, func(p *Page) { p.Sections[0].Text = "bad\x00text" }, func(p *Page) { p.Sections[0].Theme = "red;display:none" }, func(p *Page) { p.Sections[0].Kind = "html" }, func(p *Page) { p.Sections[0].Heading = strings.Repeat("x", 161) }, func(p *Page) { p.Sections = append(p.Sections, p.Sections[0]) }, func(p *Page) { p.Sections[0].ProductIDs = []int64{1} }, func(p *Page) { p.Sections[0].Kind = "collection"; p.Sections[0].ProductIDs = []int64{1, 1} }, func(p *Page) {
		p.Sections[0].Kind = "faq"
		p.Sections[0].Questions = []FAQ{{Question: "", Answer: "No"}}
	}} {
		p := base
		p.Sections = append([]Section{}, base.Sections...)
		mutate(&p)
		if ValidatePage(p) == nil {
			t.Fatal("malformed page accepted")
		}
	}
}
