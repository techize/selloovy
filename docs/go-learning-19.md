# Go lesson 19: a small, read-only HTTP client

The square-sandbox-check command uses net/http and encoding/json from Go's standard
library. A context carries a deadline; the client also has a timeout. The URL and
GET method are fixed, so configuration cannot redirect the probe to a live payment
endpoint. CheckRedirect prevents an HTTP redirect from moving the request elsewhere.

A private token file supplies the Authorization header. File metadata is checked
before and after opening; the command limits file/response reads and returns fixed
errors rather than printing credentials, account data or raw network responses.
Unknown response fields are ignored because Square locations include much more
information than this check needs. Only country, currency, status and processing
capability decide whether account readiness passes.

http.RoundTripper is the interface that actually executes an HTTP request. Tests
supply a function implementing it to inspect the exact destination/method/headers
and return synthetic responses. They cover wrong-region/currency locations,
ambiguous selection, API failure, response bounds, secret-file permissions and
redirect rejection without contacting Square. They prove our request boundary,
not Square's 3DS behavior.

Optional exercise: trace what happens if Square sends a redirect or a response
containing private error details. Explain why neither its URL nor those details
appear in the command's error output, and why a successful location check cannot
justify enabling card checkout.
