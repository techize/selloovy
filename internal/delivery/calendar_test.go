package delivery

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWorkingDayBoundariesAndCalendarMaintenance(t *testing.T) {
	c := DefaultCalendar()
	s := Service{DaysMin: 1, DaysMax: 2}
	cases := []struct {
		start             string
		lo, hi            int
		dispatch, arrival string
	}{{"2026-04-02T12:00:00Z", 1, 1, "7 Apr 2026", "9 Apr 2026"}, {"2026-12-24T12:00:00Z", 1, 1, "29 Dec 2026", "31 Dec 2026"}, {"2026-10-09T12:00:00Z", 2, 2, "13 Oct 2026", "15 Oct 2026"}, {"2026-03-29T23:30:00Z", 1, 1, "31 Mar 2026", "2 Apr 2026"}}
	for _, x := range cases {
		start, e := time.Parse(time.RFC3339, x.start)
		if e != nil {
			t.Fatal(e)
		}
		got := c.Estimate(start, x.lo, x.hi, s)
		if !got.Available || got.DispatchMin != x.dispatch || got.ArrivalMax != x.arrival {
			t.Fatalf("working-day estimate %s: %+v", x.start, got)
		}
	}
	if c.Estimate(time.Date(2028, 12, 29, 12, 0, 0, 0, time.UTC), 2, 2, s).Available {
		t.Fatal("unknown calendar range guessed")
	}
	path := filepath.Join(t.TempDir(), "calendar.json")
	if os.WriteFile(path, []byte(`{"england-and-wales":{"events":[{"date":"2029-01-01"},{"date":"2029-12-25"}]}}`), 0600) != nil {
		t.Fatal("fixture failed")
	}
	updated, e := LoadCalendar(path)
	if e != nil {
		t.Fatal(e)
	}
	if !updated.Estimate(time.Date(2029, 1, 1, 12, 0, 0, 0, time.UTC), 1, 1, s).Available {
		t.Fatal("calendar update not loaded")
	}
	if os.WriteFile(path, []byte(`{"scotland":{"events":[{"date":"2029-01-01"}]}}`), 0600) != nil {
		t.Fatal("fixture failed")
	}
	if _, e = LoadCalendar(path); e == nil {
		t.Fatal("wrong calendar accepted")
	}
}
func TestInclusiveFreeDeliveryAndPaidUpgrades(t *testing.T) {
	threshold := int64(3000)
	standard := Service{FeePence: 499, FreeFromPence: &threshold}
	for _, x := range []struct{ n, w int64 }{{2999, 499}, {3000, 0}, {3001, 0}} {
		if standard.Charge(x.n) != x.w {
			t.Fatal("inclusive product-only threshold")
		}
	}
	for _, fee := range []int64{599, 899} {
		if (Service{FeePence: fee}).Charge(3000) != fee {
			t.Fatal("upgrade discounted")
		}
	}
	if _, e := Validate(Settings{Revision: 1, Services: []Service{{ID: "00000000000000000000000000000001", Name: "Standard", DaysMin: 5, DaysMax: 3}}}); e == nil {
		t.Fatal("inverted transit estimate")
	}
}
