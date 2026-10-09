package catalog

import (
	"errors"
	"strings"
	"testing"
)

func TestMakerAvailabilityAndValidation(t *testing.T) {
	for _, c := range []struct {
		mode     string
		stock    int64
		fallback bool
		state    string
		min, max int
	}{{"stocked", 3, false, "in_stock", 2, 2}, {"stocked", 3, true, "in_stock", 2, 2}, {"stocked", 0, false, "unavailable", 0, 0}, {"stocked", 0, true, "made_to_order", 5, 7}, {"made_to_order", 0, false, "made_to_order", 5, 7}} {
		state, min, max := availability(c.mode, c.stock, c.fallback)
		if state != c.state || min != c.min || max != c.max {
			t.Fatal("availability rule mismatch")
		}
	}
	valid := MakerInput{Revision: 1, Variants: []VariantInput{{Label: " Standard ", SupplyMode: "stocked", StockQuantity: 3}}}
	got, e := validateMaker(valid)
	if e != nil || got.Variants[0].Label != "Standard" || valid.Variants[0].Label != " Standard " {
		t.Fatal("validation mutated draft")
	}
	cases := []MakerInput{{Revision: 0, Variants: valid.Variants}, {Revision: 1}, {Revision: 1, Variants: []VariantInput{{Label: "Same", SupplyMode: "stocked"}, {Label: "same", SupplyMode: "stocked"}}}, {Revision: 1, Variants: []VariantInput{{ID: 2, Label: "A", SupplyMode: "stocked"}, {ID: 2, Label: "B", SupplyMode: "stocked"}}}}
	for _, v := range []VariantInput{{Label: "Bad\x00", SupplyMode: "stocked"}, {Label: strings.Repeat("a", 121), SupplyMode: "stocked"}, {Label: "A", SupplyMode: "other"}, {Label: "A", SupplyMode: "made_to_order", StockQuantity: 1}, {Label: "A", SupplyMode: "stocked", StockQuantity: -1}, {Label: "A", SupplyMode: "stocked", StockQuantity: 1000001}} {
		cases = append(cases, MakerInput{Revision: 1, Variants: []VariantInput{v}})
	}
	tooMany := MakerInput{Revision: 1, Variants: make([]VariantInput, 51)}
	var bounded *ValidationError
	if _, err := validateMaker(tooMany); !errors.As(err, &bounded) || bounded.Fields["variants"] == "" {
		t.Fatal("variant limit not enforced")
	}
	for _, c := range cases {
		var v *ValidationError
		if _, e := validateMaker(c); !errors.As(e, &v) {
			t.Fatal("invalid maker input accepted")
		}
	}
}
