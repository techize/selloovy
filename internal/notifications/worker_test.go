package notifications

import (
	"encoding/json"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/techize/selloovy/internal/catalog"
	"github.com/techize/selloovy/internal/notificationdb"
	"strings"
	"testing"
	"time"
)

func TestRenderEscapesPersonalisationAndKeepsHistoricalTotals(t *testing.T) {
	customer, _ := json.Marshal(catalog.Customer{Email: "customer@example.com", Name: "Buyer", Address1: "1 Example Road", Country: "GB"})
	snapshot, _ := json.Marshal(catalog.CheckoutSnapshot{Shop: catalog.PublicShop{Name: "Fixture shop"}, Currency: "GBP", SubtotalPence: 1999, ShippingPence: 499, TotalPence: 2498, Lines: []catalog.BasketLine{{BasketItem: catalog.BasketItem{ProductName: "Dragon", VariantLabel: "Amber", Quantity: 1, UnitPricePence: 1999, OwnerName: "<script>alert('certificate')</script>"}, TotalPence: 1999}}})
	row := notificationdb.LockQueuedNotificationRow{AttemptID: strings.Repeat("0", 32), Kind: "customer_receipt", Customer: customer, Snapshot: snapshot, PaidAt: pgtype.Timestamptz{Time: time.Now(), Valid: true}, OwnerEmail: "owner@example.com"}
	p, e := render(row, "00000000-0000-4000-8000-000000000001")
	if e != nil || strings.Contains(p.HTML, "<script>") || !strings.Contains(p.HTML, "&lt;script&gt;") || !strings.Contains(p.Text, "<script>") || !strings.Contains(p.Text, "£24.98") || p.To != "customer@example.com" {
		t.Fatal("receipt content unsafe or changed")
	}
	row.Kind = "owner_notification"
	p, e = render(row, "key")
	if e != nil || p.To != "owner@example.com" || !strings.Contains(p.Text, "New order to fulfil") {
		t.Fatal("owner recipient wrong")
	}
	row.Kind = "unsupported"
	if _, e = render(row, "key"); e != ErrContent {
		t.Fatal("unsupported notification")
	}
	row.Kind = "customer_receipt"
	row.PaidAt.Valid = false
	if _, e = render(row, "key"); e != ErrContent {
		t.Fatal("unpaid snapshot rendered")
	}
}
