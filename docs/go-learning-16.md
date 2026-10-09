# Go lesson 16: separate production, transit and money

A delivery service holds an integer-pence charge and a transit range. Charge uses
only the merchandise subtotal; adding shipping to that basis would make the £30
threshold incorrect. The service configuration and stock/preparation data are read
inside the basket transaction, so one calculation uses one consistent snapshot.

Preparation and transit are separate phases. Calendar.add advances local dates
with AddDate, skips excluded days and counts day one after the anchor date.
Calendar.Estimate first computes dispatch bounds, then advances each bound by its
matching transit bound. AddDate respects London daylight-saving boundaries; adding
24-hour durations would not reliably advance local calendar dates across a clock change.

Private fields on PublicVariant carry stock/preparation facts to basket logic.
They are unexported, so encoding/json excludes them from public projections.
Different certificate names still contribute to one variant's combined demand.

Optional exercise: explain why £29.99 plus £4.99 shipping still pays the shipping
charge, and why a 3–5-day made-to-order line determines dispatch when shipped with
a two-day stocked line. Trace a disabled service through a refreshed basket.
