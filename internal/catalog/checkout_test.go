package catalog

import (
	"errors"
	"strings"
	"testing"
)

func TestCustomerValidation(t *testing.T) {
	valid := Customer{Email: "customer@example.com", Name: " Test 名 ", Address1: " 1 Example Lane ", Town: "Exampletown", Postcode: "bt1  1aa", Country: "GB"}
	c, e := validateCustomer(valid)
	if e != nil || c.Name != "Test 名" || c.Postcode != "BT1 1AA" {
		t.Fatal("valid UK details not normalized")
	}
	for _, tc := range []struct {
		name   string
		change func(*Customer)
	}{
		{"email", func(c *Customer) { c.Email = "Test <customer@example.com>" }},
		{"name", func(c *Customer) { c.Name = strings.Repeat("名", 121) }},
		{"address1", func(c *Customer) { c.Address1 = "" }},
		{"address2", func(c *Customer) { c.Address2 = "hidden\x00text" }},
		{"postcode", func(c *Customer) { c.Postcode = "<script>" }},
		{"country", func(c *Customer) { c.Country = "US" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := valid
			tc.change(&c)
			_, e := validateCustomer(c)
			var v *ValidationError
			if !errors.As(e, &v) || v.Fields[tc.name] == "" {
				t.Fatal("invalid customer field accepted")
			}
		})
	}
}
