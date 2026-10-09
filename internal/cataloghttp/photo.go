package cataloghttp

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/techize/selloovy/internal/authhttp"
	"github.com/techize/selloovy/internal/catalog"
	"github.com/techize/selloovy/internal/photoimage"
)

type PhotoBackend interface {
	ReadPhoto(context.Context, string, int64) (catalog.PhotoSettings, error)
	SavePhoto(context.Context, string, int64, int64, string, []byte, bool) (catalog.PhotoSettings, error)
	PrivatePhoto(context.Context, string, int64) (catalog.PhotoContent, error)
}

func photoRoutes(r chi.Router, b PhotoBackend) {
	r.Get("/{id}/photo", func(w http.ResponseWriter, r *http.Request) {
		id, ok := number(chi.URLParam(r, "id"))
		if !ok {
			http.NotFound(w, r)
			return
		}
		data, e := b.ReadPhoto(r.Context(), authhttp.SessionToken(r.Context()), id)
		if e != nil {
			failure(w, e)
			return
		}
		reply(w, 200, data)
	})
	r.Get("/{id}/photo/image", func(w http.ResponseWriter, r *http.Request) {
		id, ok := number(chi.URLParam(r, "id"))
		if !ok {
			http.NotFound(w, r)
			return
		}
		data, e := b.PrivatePhoto(r.Context(), authhttp.SessionToken(r.Context()), id)
		if e != nil {
			failure(w, e)
			return
		}
		w.Header().Set("Content-Type", data.MediaType)
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
		_, _ = w.Write(data.Content)
	})
	r.Put("/{id}/photo", func(w http.ResponseWriter, r *http.Request) {
		id, ok := number(chi.URLParam(r, "id"))
		if !ok {
			http.NotFound(w, r)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, photoimage.MaxBytes+64*1024)
		reader, e := r.MultipartReader()
		if e != nil {
			reply(w, 400, map[string]string{"error": "Use a photo upload form."})
			return
		}
		fields := map[string][]byte{}
		for {
			part, e := reader.NextPart()
			if e == io.EOF {
				break
			}
			if e != nil {
				reply(w, 400, map[string]string{"error": "Invalid photo upload."})
				return
			}
			name := part.FormName()
			limit := int64(1024)
			if name == "photo" {
				limit = photoimage.MaxBytes
			} else if name == "revision" {
				limit = 32
			} else if name != "alt" {
				reply(w, 400, map[string]string{"error": "Unknown photo field."})
				return
			}
			if _, exists := fields[name]; exists {
				reply(w, 400, map[string]string{"error": "Duplicate photo field."})
				return
			}
			data, e := io.ReadAll(io.LimitReader(part, limit+1))
			_ = part.Close()
			if e != nil || int64(len(data)) > limit {
				reply(w, 400, map[string]string{"error": "Photo upload exceeds the displayed limits."})
				return
			}
			fields[name] = data
		}
		if data, present := fields["photo"]; present && len(data) == 0 {
			reply(w, 422, map[string]any{"error": "Check the photo.", "fields": map[string]string{"photo": "Choose a non-empty JPEG or PNG photo."}})
			return
		}
		revision, e := strconv.ParseInt(string(fields["revision"]), 10, 64)
		if e != nil || revision < 1 {
			reply(w, 400, map[string]string{"error": "Reload the photo before saving."})
			return
		}
		data, e := b.SavePhoto(r.Context(), authhttp.SessionToken(r.Context()), id, revision, string(fields["alt"]), fields["photo"], false)
		if e != nil {
			failure(w, e)
			return
		}
		reply(w, 200, data)
	})
	r.Delete("/{id}/photo", func(w http.ResponseWriter, r *http.Request) {
		id, ok := number(chi.URLParam(r, "id"))
		if !ok {
			http.NotFound(w, r)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 4096)
		d := json.NewDecoder(r.Body)
		d.DisallowUnknownFields()
		var in struct{ Revision int64 }
		if d.Decode(&in) != nil || d.Decode(new(any)) != io.EOF || in.Revision < 1 {
			reply(w, 400, map[string]string{"error": "Invalid photo removal request."})
			return
		}
		data, e := b.SavePhoto(r.Context(), authhttp.SessionToken(r.Context()), id, in.Revision, "", nil, true)
		if e != nil {
			failure(w, e)
			return
		}
		reply(w, 200, data)
	})
}
