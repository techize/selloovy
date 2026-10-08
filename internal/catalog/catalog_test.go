package catalog

import (
	"errors"
	"strings"
	"testing"
)

func TestValidationKeepsExactMoneyAndUnicode(t *testing.T) {
	in := Input{Name: "  " + strings.Repeat("界", 160) + "  ", Description: " <b>Plain text</b>\nSecond line ", PricePence: 1999, CertificateName: "required", CreationKey: "00000000-0000-4000-8000-000000000001"}
	v, e := validate(in, true)
	if e != nil || v.PricePence != 1999 || v.Name != strings.Repeat("界", 160) || v.Description != "<b>Plain text</b>\nSecond line" {
		t.Fatal("normalization changed valid product")
	}
	cases := []Input{in, in, in, in, in, in, in, in}
	cases[0].Name = ""
	cases[1].Name = strings.Repeat("a", 161)
	cases[2].Description = strings.Repeat("a", 5001)
	cases[3].Description = "bad\x00"
	cases[4].PricePence = 100000001
	cases[5].CertificateName = "other"
	cases[6].CreationKey = "not-a-key"
	cases[7].Revision = 1
	for _, v := range cases {
		var field *ValidationError
		if _, e := validate(v, true); !errors.As(e, &field) {
			t.Fatal("invalid product accepted")
		}
	}
	in.CreationKey = ""
	in.Revision = 1
	if _, e = validate(in, false); e != nil {
		t.Fatal("valid edit rejected")
	}
	in.Revision = 0
	if _, e = validate(in, false); e == nil {
		t.Fatal("unversioned edit accepted")
	}
}
