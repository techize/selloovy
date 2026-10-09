package catalog

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/techize/selloovy/internal/auth"
	"github.com/techize/selloovy/internal/catalogdb"
	"github.com/techize/selloovy/internal/photoimage"
	"github.com/techize/selloovy/internal/shopdb"
)

type Photo struct {
	ID     string `json:"id"`
	Alt    string `json:"alt"`
	Width  int32  `json:"width"`
	Height int32  `json:"height"`
}
type PhotoSettings struct {
	Revision int64 `json:"revision"`
	Photo    Photo `json:"photo"`
}
type PhotoContent struct {
	Content   []byte
	MediaType string
}

func readPhoto(ctx context.Context, q *catalogdb.Queries, d []byte, id int64) (PhotoSettings, error) {
	row, e := q.ReadPhoto(ctx, catalogdb.ReadPhotoParams{Digest: d, ID: id})
	if errors.Is(e, pgx.ErrNoRows) {
		return PhotoSettings{}, ErrNotFound
	}
	if e != nil {
		return PhotoSettings{}, ErrStorage
	}
	return PhotoSettings{Revision: row.Revision, Photo: Photo{ID: row.PhotoID, Alt: row.Alt, Width: row.Width, Height: row.Height}}, nil
}
func (s *Store) ReadPhoto(ctx context.Context, token string, id int64) (PhotoSettings, error) {
	d, e := auth.SessionDigest(token)
	if e != nil {
		return PhotoSettings{}, e
	}
	return readPhoto(ctx, catalogdb.New(s.pool), d, id)
}
func (s *Store) PrivatePhoto(ctx context.Context, token string, id int64) (PhotoContent, error) {
	d, e := auth.SessionDigest(token)
	if e != nil {
		return PhotoContent{}, e
	}
	row, e := catalogdb.New(s.pool).ReadPrivatePhotoContent(ctx, catalogdb.ReadPrivatePhotoContentParams{Digest: d, ID: id})
	if errors.Is(e, pgx.ErrNoRows) {
		return PhotoContent{}, ErrNotFound
	}
	if e != nil {
		return PhotoContent{}, ErrStorage
	}
	return PhotoContent{row.Content, row.MediaType}, nil
}
func (s *Store) PublicPhoto(ctx context.Context, key string, id int64, photoID string) (PhotoContent, error) {
	if !publicKeyPattern.MatchString(key) || !publicKeyPattern.MatchString(photoID) || id < 1 {
		return PhotoContent{}, ErrNotFound
	}
	row, e := catalogdb.New(s.pool).PublicPhotoContent(ctx, catalogdb.PublicPhotoContentParams{PublicKey: key, ID: id, ID_2: photoID})
	if errors.Is(e, pgx.ErrNoRows) {
		return PhotoContent{}, ErrNotFound
	}
	if e != nil {
		return PhotoContent{}, ErrStorage
	}
	return PhotoContent{row.Content, row.MediaType}, nil
}

// SavePhoto creates immutable content so a draft replacement cannot change a published photo.
func (s *Store) SavePhoto(ctx context.Context, token string, id, revision int64, alt string, data []byte, remove bool) (PhotoSettings, error) {
	d, e := auth.SessionDigest(token)
	if e != nil {
		return PhotoSettings{}, e
	}
	prior, e := s.ReadPhoto(ctx, token, id)
	if e != nil {
		return PhotoSettings{}, e
	}
	if prior.Revision != revision {
		return PhotoSettings{}, ErrConflict
	}
	var img photoimage.Image
	if !remove {
		alt = strings.TrimSpace(alt)
		invalid := alt == "" || !utf8.ValidString(alt) || utf8.RuneCountInString(alt) > 160
		for _, r := range alt {
			if unicode.IsControl(r) {
				invalid = true
			}
		}
		if invalid {
			return PhotoSettings{}, &ValidationError{map[string]string{"alt": "Describe the photo in 1–160 plain-text characters."}}
		}
		if data == nil {
			if prior.Photo.ID == "" {
				return PhotoSettings{}, &ValidationError{map[string]string{"photo": "Choose a JPEG or PNG photo."}}
			}
			content, e := s.PrivatePhoto(ctx, token, id)
			if e != nil {
				return PhotoSettings{}, e
			}
			// Alt-only edits copy already canonical content without another lossy JPEG encode.
			img = photoimage.Image{Content: content.Content, MediaType: content.MediaType, Width: prior.Photo.Width, Height: prior.Photo.Height}
		} else {
			img, e = photoimage.Decode(data)
			if errors.Is(e, photoimage.ErrBusy) {
				return PhotoSettings{}, ErrStorage
			}
			if e != nil {
				return PhotoSettings{}, &ValidationError{map[string]string{"photo": photoimage.ErrInvalid.Error()}}
			}
		}
	}
	tx, e := s.pool.Begin(ctx)
	if e != nil {
		return PhotoSettings{}, ErrStorage
	}
	defer cleanupTransaction(tx)
	owner, e := shopdb.New(tx).LockShopSession(ctx, d)
	if errors.Is(e, pgx.ErrNoRows) {
		return PhotoSettings{}, auth.ErrCredential
	}
	if e != nil {
		return PhotoSettings{}, ErrStorage
	}
	q := catalogdb.New(tx)
	prior, e = readPhoto(ctx, q, d, id)
	if e != nil {
		return PhotoSettings{}, e
	}
	if prior.Revision != revision {
		return PhotoSettings{}, ErrConflict
	}
	photoID := pgtype.Text{}
	if !remove {
		var key [16]byte
		if _, e = rand.Read(key[:]); e != nil {
			return PhotoSettings{}, ErrStorage
		}
		photoID = pgtype.Text{String: hex.EncodeToString(key[:]), Valid: true}
		if e = q.AddPhoto(ctx, catalogdb.AddPhotoParams{ID: photoID.String, ProductID: id, Content: img.Content, MediaType: img.MediaType, Alt: alt, Width: img.Width, Height: img.Height}); e != nil {
			return PhotoSettings{}, ErrStorage
		}
	}
	if _, e = q.SelectPhoto(ctx, catalogdb.SelectPhotoParams{PhotoID: photoID, OwnerID: owner, ProductID: id, Revision: revision}); errors.Is(e, pgx.ErrNoRows) {
		return PhotoSettings{}, ErrConflict
	}
	if e != nil {
		return PhotoSettings{}, ErrStorage
	}
	if e = q.DeleteUnusedPhotos(ctx, id); e != nil {
		return PhotoSettings{}, ErrStorage
	}
	result, e := readPhoto(ctx, q, d, id)
	if e != nil {
		return PhotoSettings{}, e
	}
	if e = tx.Commit(ctx); e != nil {
		return PhotoSettings{}, ErrStorage
	}
	return result, nil
}
