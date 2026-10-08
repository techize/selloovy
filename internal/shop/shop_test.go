package shop

import (
	"errors"
	"strings"
	"testing"
)

func TestSettingsValidationBoundaries(t *testing.T) {
	for _, input := range []Input{{Name: "", Revision: 1}, {Name: strings.Repeat("界", 121), Revision: 1}, {Name: "Maker", Tagline: strings.Repeat("x", 161), Revision: 1}, {Name: "Maker", Description: strings.Repeat("x", 2001), Revision: 1}, {Name: "Maker\x00", Revision: 1}, {Name: "Maker", ContactEmail: "Display <owner@example.com>", Revision: 1}, {Name: "Maker", Revision: 0}} {
		var validation *ValidationError
		if _, err := validate(input); !errors.As(err, &validation) {
			t.Fatal("invalid fields accepted")
		}
	}
	input := Input{Name: strings.Repeat("界", 120), Tagline: strings.Repeat("🦕", 160), Description: "<script>plain text</script>\nSecond line", Revision: 1}
	out, err := validate(input)
	if err != nil || out.Description != input.Description {
		t.Fatal("Unicode/plain text boundaries rejected or transformed")
	}
}
