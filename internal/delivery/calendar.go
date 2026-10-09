package delivery

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"time"
	_ "time/tzdata" // Keep Europe/London available in minimal self-hosted containers.
)

var ErrCalendar = errors.New("working-day calendar unavailable for these dates")

type Calendar struct {
	dates       map[string]bool
	first, last int
	loc         *time.Location
}
type Estimate struct {
	From, DispatchMin, DispatchMax, ArrivalMin, ArrivalMax string
	Available                                              bool
}

func calendar(dates []string, first, last int) (*Calendar, error) {
	loc, e := time.LoadLocation("Europe/London")
	if e != nil {
		return nil, ErrCalendar
	}
	c := &Calendar{map[string]bool{}, first, last, loc}
	years := map[int]bool{}
	for _, s := range dates {
		d, e := time.Parse("2006-01-02", s)
		if e != nil {
			return nil, ErrCalendar
		}
		c.dates[s] = true
		years[d.Year()] = true
	}
	if first > last || last-first > 30 {
		return nil, ErrCalendar
	}
	for y := first; y <= last; y++ {
		if !years[y] {
			return nil, ErrCalendar
		}
	}
	return c, nil
}
func DefaultCalendar() *Calendar {
	c, e := calendar(defaultDates, 2026, 2028)
	if e != nil {
		panic("invalid built-in working-day calendar")
	}
	return c
}

// LoadCalendar accepts the GOV.UK feed shape, allowing an operator to refresh
// dates from a reviewed local file without fetching anything during checkout.
func LoadCalendar(path string) (*Calendar, error) {
	f, e := os.Open(path)
	if e != nil {
		return nil, ErrCalendar
	}
	defer f.Close()
	b, e := io.ReadAll(io.LimitReader(f, 262145))
	if e != nil || len(b) > 262144 {
		return nil, ErrCalendar
	}
	var feed map[string]struct {
		Events []struct {
			Date string `json:"date"`
		} `json:"events"`
	}
	if json.Unmarshal(b, &feed) != nil {
		return nil, ErrCalendar
	}
	dates := []string{}
	first, last := 9999, 0
	for _, v := range feed["england-and-wales"].Events {
		d, e := time.Parse("2006-01-02", v.Date)
		if e != nil {
			return nil, ErrCalendar
		}
		dates = append(dates, v.Date)
		if d.Year() < first {
			first = d.Year()
		}
		if d.Year() > last {
			last = d.Year()
		}
	}
	return calendar(dates, first, last)
}
func (c *Calendar) add(start time.Time, n int) (time.Time, error) {
	if c == nil || n < 0 || n > 120 {
		return time.Time{}, ErrCalendar
	}
	start = start.In(c.loc)
	d := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, c.loc)
	if d.Year() < c.first || d.Year() > c.last {
		return time.Time{}, ErrCalendar
	}
	for n > 0 {
		d = d.AddDate(0, 0, 1)
		if d.Year() > c.last {
			return time.Time{}, ErrCalendar
		}
		if d.Weekday() != time.Saturday && d.Weekday() != time.Sunday && !c.dates[d.Format("2006-01-02")] {
			n--
		}
	}
	return d, nil
}
func (c *Calendar) Estimate(start time.Time, prepMin, prepMax int, s Service) Estimate {
	if prepMin < 1 || prepMax < prepMin || s.DaysMin < 1 || s.DaysMax < s.DaysMin {
		return Estimate{}
	}
	lo, e := c.add(start, prepMin)
	if e != nil {
		return Estimate{}
	}
	hi, e := c.add(start, prepMax)
	if e != nil {
		return Estimate{}
	}
	arriveLo, e := c.add(lo, s.DaysMin)
	if e != nil {
		return Estimate{}
	}
	arriveHi, e := c.add(hi, s.DaysMax)
	if e != nil {
		return Estimate{}
	}
	return Estimate{From: start.In(c.loc).Format("2 Jan 2006"), DispatchMin: lo.Format("2 Jan 2006"), DispatchMax: hi.Format("2 Jan 2006"), ArrivalMin: arriveLo.Format("2 Jan 2006"), ArrivalMax: arriveHi.Format("2 Jan 2006"), Available: true}
}
