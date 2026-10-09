// Package delivery owns shipping prices and working-day estimates.
package delivery

import (
	"errors"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

type Service struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	FeePence      int64  `json:"feePence"`
	DaysMin       int    `json:"daysMin"`
	DaysMax       int    `json:"daysMax"`
	FreeFromPence *int64 `json:"freeFromPence"`
	Enabled       bool   `json:"enabled"`
}
type Settings struct {
	Revision int64     `json:"revision"`
	Services []Service `json:"services"`
}

var IDPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)

func Validate(in Settings) (Settings, error) {
	if in.Revision < 1 || len(in.Services) > 10 {
		return Settings{}, errors.New("Use up to ten services and reload before saving.")
	}
	in.Services = append([]Service{}, in.Services...)
	ids := map[string]bool{}
	names := map[string]bool{}
	for i := range in.Services {
		s := &in.Services[i]
		s.Name = strings.TrimSpace(s.Name)
		invalid := s.Name == "" || !utf8.ValidString(s.Name) || utf8.RuneCountInString(s.Name) > 120
		for _, r := range s.Name {
			if unicode.IsControl(r) {
				invalid = true
			}
		}
		key := strings.ToLower(s.Name)
		if invalid || !IDPattern.MatchString(s.ID) || ids[s.ID] || names[key] || s.FeePence < 0 || s.FeePence > 100000 || s.DaysMin < 1 || s.DaysMax < s.DaysMin || s.DaysMax > 30 || (s.FreeFromPence != nil && (*s.FreeFromPence < 1 || *s.FreeFromPence > 100000000)) {
			return Settings{}, errors.New("Use unique names (1–120 characters), charges £0–£1,000, transit 1–30 working days and an optional positive free-delivery threshold.")
		}
		ids[s.ID] = true
		names[key] = true
	}
	return in, nil
}
func (s Service) Charge(qualifyingPence int64) int64 {
	if s.FreeFromPence != nil && qualifyingPence >= *s.FreeFromPence {
		return 0
	}
	return s.FeePence
}
