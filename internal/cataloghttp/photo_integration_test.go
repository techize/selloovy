package cataloghttp_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/techize/selloovy/internal/auth"
	"github.com/techize/selloovy/internal/authhttp"
	"github.com/techize/selloovy/internal/catalog"
	"github.com/techize/selloovy/internal/cataloghttp"
	"github.com/techize/selloovy/internal/storefronthttp"
	"github.com/techize/selloovy/internal/web"
)

func photoFixture(t *testing.T, c color.NRGBA) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 12, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 12; x++ {
			img.Set(x, y, c)
		}
	}
	var b bytes.Buffer
	if png.Encode(&b, img) != nil {
		t.Fatal("fixture failed")
	}
	return b.Bytes()
}
func TestCoverPhotoOwnershipPublicationAndAtomicChanges(t *testing.T) {
	pool := fixtureDatabase(t)
	ctx := t.Context()
	vault, _ := auth.NewMFAVault(make([]byte, 32))
	owners, e := auth.NewStore(ctx, pool, vault, auth.NewPasswordHasher())
	if e != nil {
		t.Fatal(e)
	}
	if _, _, e = owners.CreateFirstOwner(ctx, "Photo maker", "first@example.com", "fixture8"); e != nil {
		t.Fatal(e)
	}
	var shopID int64
	if pool.QueryRow(ctx, "INSERT INTO shops(name) VALUES('Other maker') RETURNING id").Scan(&shopID) != nil {
		t.Fatal("fixture failed")
	}
	if _, _, e = owners.CreateOwner(ctx, shopID, "second@example.com", "fixture8"); e != nil {
		t.Fatal(e)
	}
	a, e := owners.Login(ctx, "first@example.com", "fixture8")
	if e != nil {
		t.Fatal(e)
	}
	b, e := owners.Login(ctx, "second@example.com", "fixture8")
	if e != nil {
		t.Fatal(e)
	}
	token, other := a.Token.Reveal(), b.Token.Reveal()
	service := catalog.NewStore(pool)
	create := func(token string) catalog.Product {
		t.Helper()
		p, e := service.Save(ctx, token, 0, catalog.Input{Name: "Photo creation", PricePence: 1999, CertificateName: "none", CreationKey: "00000000-0000-4000-8000-000000000001"})
		if e != nil {
			t.Fatal(e)
		}
		return p
	}
	p := create(token)
	foreign := create(other)
	guard, _ := authhttp.New(owners, "https://shop.example.com")
	routes := chi.NewRouter()
	routes.Mount("/api/admin/products", cataloghttp.New(service))
	admin := guard.ProtectAdmin(routes)
	path := fmt.Sprintf("/%d/photo", p.ID)
	send := func(method, path string, data []byte, ctype, token, origin string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "https://shop.example.com/api/admin/products"+path, bytes.NewReader(data))
		r.Header.Set("Content-Type", ctype)
		r.Header.Set("Origin", origin)
		r.Header.Set("X-Selloovy-Request", "owner-auth")
		if token != "" {
			r.AddCookie(&http.Cookie{Name: "__Host-selloovy_session", Value: token})
		}
		w := httptest.NewRecorder()
		admin.ServeHTTP(w, r)
		return w
	}
	upload := func(revision int64, alt string, photo []byte, extra bool) ([]byte, string) {
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		writer.WriteField("revision", fmt.Sprint(revision))
		writer.WriteField("alt", alt)
		if photo != nil {
			part, _ := writer.CreateFormFile("photo", "ignored-filename.png")
			part.Write(photo)
		}
		if extra {
			writer.WriteField("shopId", "2")
		}
		writer.Close()
		return body.Bytes(), writer.FormDataContentType()
	}
	origin := "https://shop.example.com"
	red := photoFixture(t, color.NRGBA{R: 255, A: 255})
	blue := photoFixture(t, color.NRGBA{B: 255, A: 255})
	body, ctype := upload(1, "Red creation", red, false)
	if send("GET", path, nil, "", "", "").Code != 401 || send("GET", path, nil, "", other, "").Code != 404 {
		t.Fatal("photo read boundary")
	}
	if send("PUT", path, body, ctype, token, "https://other.example.com").Code != 403 || send("PUT", path, body, ctype, other, origin).Code != 404 {
		t.Fatal("photo write boundary")
	}
	bad, ct := upload(1, "Red", red, true)
	if send("PUT", path, bad, ct, token, origin).Code != 400 {
		t.Fatal("selector accepted")
	}
	bad, ct = upload(1, "Red", []byte("<svg/>"), false)
	if send("PUT", path, bad, ct, token, origin).Code != 422 {
		t.Fatal("SVG accepted")
	}

	bad, ct = upload(1, "Red", []byte{}, false)
	if send("PUT", path, bad, ct, token, origin).Code != 422 {
		t.Fatal("empty file accepted")
	}
	bad, ct = upload(1, strings.Repeat("x", 161), red, false)
	if send("PUT", path, bad, ct, token, origin).Code != 422 {
		t.Fatal("long alt accepted")
	}
	bad, ct = upload(1, "Red", make([]byte, 5*1024*1024+1), false)
	if send("PUT", path, bad, ct, token, origin).Code != 400 {
		t.Fatal("oversized upload accepted")
	}
	w := send("PUT", path, body, ctype, token, origin)
	var current catalog.PhotoSettings
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &current) != nil || current.Revision != 2 || current.Photo.ID == "" || current.Photo.Width != 12 || current.Photo.Height != 8 {
		t.Fatal("photo upload failed")
	}
	old := current.Photo.ID
	if send("GET", path+"/image", nil, "", other, "").Code != 404 || send("GET", path+"/image", nil, "", "", "").Code != 401 {
		t.Fatal("private pixels exposed")
	}
	w = send("GET", path+"/image", nil, "", token, "")
	if w.Code != 200 || w.Header().Get("Content-Type") != "image/png" || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("private image headers")
	}
	maker, e := service.SaveMaker(ctx, token, p.ID, catalog.MakerInput{Revision: current.Revision, Variants: []catalog.VariantInput{{Label: "Standard", SupplyMode: "made_to_order"}}})
	if e != nil {
		t.Fatal(e)
	}
	pub, e := service.SavePublication(ctx, token, p.ID, catalog.PublicationInput{Revision: maker.Revision, ShopRevision: 1, Publish: true})
	if e != nil {
		t.Fatal(e)
	}
	key := strings.Split(pub.PublicPath, "/")[2]
	photoPath := pub.PublicPath + "/photos/" + old
	handler, e := web.NewHandler(nil, nil, nil, nil, storefronthttp.New(service))
	if e != nil {
		t.Fatal(e)
	}
	browse := func(path string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest("GET", "https://shop.example.com"+path, nil))
		return w
	}
	if browse(photoPath).Code != 200 || browse(fmt.Sprintf("/shop/%s/products/%d/photos/%s", key, foreign.ID, old)).Code != 404 || browse(photoPath+"?shopId=2").Code != 404 {
		t.Fatal("public photo scope")
	}
	w = browse(pub.PublicPath)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "alt=\"Red creation\"") || !strings.Contains(w.Body.String(), photoPath) {
		t.Fatal("photo not rendered")
	}
	current, e = service.SavePhoto(ctx, token, p.ID, pub.Revision, "Blue creation", blue, false)
	if e != nil {
		t.Fatal(e)
	}
	if browse(pub.PublicPath+"/photos/"+current.Photo.ID).Code != 404 || browse(photoPath).Code != 200 {
		t.Fatal("draft replacement changed public photo")
	}
	second := current.Photo.ID
	current, e = service.SavePhoto(ctx, token, p.ID, current.Revision, "Updated blue description", nil, false)
	if e != nil || current.Photo.ID == second || current.Photo.Alt != "Updated blue description" {
		t.Fatal("alt edit failed")
	}
	var count int
	if pool.QueryRow(ctx, "SELECT count(*) FROM product_photos WHERE id=$1", second).Scan(&count) != nil || count != 0 {
		t.Fatal("unused draft retained")
	}
	if _, e = service.SavePhoto(ctx, token, p.ID, pub.Revision, "Stale", red, false); !errors.Is(e, catalog.ErrConflict) {
		t.Fatal("stale upload accepted")
	}
	// A failed insert cannot advance revision or replace the selected photo.
	if _, e = pool.Exec(ctx, "ALTER TABLE product_photos ADD CONSTRAINT fixture_fail CHECK(width=0) NOT VALID"); e != nil {
		t.Fatal("fixture failure")
	}
	if _, e = service.SavePhoto(ctx, token, p.ID, current.Revision, "Failing upload", red, false); !errors.Is(e, catalog.ErrStorage) {
		t.Fatal("insert failure not returned")
	}
	after, e := service.ReadPhoto(ctx, token, p.ID)
	if e != nil || after != current {
		t.Fatal("partial photo write")
	}
	if _, e = pool.Exec(ctx, "ALTER TABLE product_photos DROP CONSTRAINT fixture_fail"); e != nil {
		t.Fatal("fixture reset")
	}
	// Two uploads from one revision have one winner and no orphaned image.
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Go(func() {
			_, e := service.SavePhoto(ctx, token, p.ID, current.Revision, "Racing photo", red, false)
			results <- e
		})
	}
	wg.Wait()
	close(results)
	success, conflict := 0, 0
	for e := range results {
		if e == nil {
			success++
		} else if errors.Is(e, catalog.ErrConflict) {
			conflict++
		} else {
			t.Fatal("unexpected race result")
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatal("photo race")
	}
	current, e = service.ReadPhoto(ctx, token, p.ID)
	if e != nil {
		t.Fatal(e)
	}
	if pool.QueryRow(ctx, "SELECT count(*) FROM product_photos WHERE product_id=$1", p.ID).Scan(&count) != nil || count != 2 {
		t.Fatal("orphaned photo")
	}
	current, e = service.SavePhoto(ctx, token, p.ID, current.Revision, "", nil, true)
	if e != nil || current.Photo.ID != "" || browse(photoPath).Code != 200 {
		t.Fatal("draft removal changed publication")
	}
	pub, e = service.SavePublication(ctx, token, p.ID, catalog.PublicationInput{Revision: current.Revision, ShopRevision: 1, Publish: true})
	if e != nil || browse(photoPath).Code != 404 {
		t.Fatal("republish did not remove photo")
	}
	if pool.QueryRow(ctx, "SELECT count(*) FROM product_photos WHERE product_id=$1", p.ID).Scan(&count) != nil || count != 0 {
		t.Fatal("unreferenced published bytes retained")
	}
	current, e = service.SavePhoto(ctx, token, p.ID, pub.Revision, "Final creation", blue, false)
	if e != nil {
		t.Fatal(e)
	}
	pub, e = service.SavePublication(ctx, token, p.ID, catalog.PublicationInput{Revision: current.Revision, ShopRevision: 1, Publish: true})
	if e != nil {
		t.Fatal(e)
	}
	finalPath := pub.PublicPath + "/photos/" + current.Photo.ID
	pub, e = service.SavePublication(ctx, token, p.ID, catalog.PublicationInput{Revision: pub.Revision, Publish: false})
	if e != nil || browse(finalPath).Code != 404 {
		t.Fatal("unpublish exposed photo")
	}
	if _, e = service.PrivatePhoto(ctx, token, p.ID); e != nil {
		t.Fatal("unpublish deleted draft photo")
	}

	decoded, _, e := image.Decode(bytes.NewReader(red))
	if e != nil {
		t.Fatal(e)
	}
	var jpg bytes.Buffer
	if jpeg.Encode(&jpg, decoded, nil) != nil {
		t.Fatal("JPEG fixture")
	}
	current, e = service.SavePhoto(ctx, token, p.ID, pub.Revision, "JPEG description", jpg.Bytes(), false)
	if e != nil {
		t.Fatal(e)
	}
	before, e := service.PrivatePhoto(ctx, token, p.ID)
	if e != nil {
		t.Fatal(e)
	}
	current, e = service.SavePhoto(ctx, token, p.ID, current.Revision, "New JPEG description", nil, false)
	if e != nil {
		t.Fatal(e)
	}
	afterContent, e := service.PrivatePhoto(ctx, token, p.ID)
	if e != nil || !bytes.Equal(before.Content, afterContent.Content) {
		t.Fatal("alt edit changed JPEG pixels")
	}

	unicodeBody, unicodeType := upload(current.Revision, strings.Repeat("🐉", 160), nil, false)
	if send("PUT", path, unicodeBody, unicodeType, token, origin).Code != 200 {
		t.Fatal("160 Unicode characters rejected")
	}
	if e = owners.Logout(ctx, token); e != nil {
		t.Fatal(e)
	}
	if send("GET", path+"/image", nil, "", token, "").Code != 401 {
		t.Fatal("logged out photo served")
	}
	pool.Close()
	if browse(finalPath).Code != 503 {
		t.Fatal("outage opened image")
	}
}
