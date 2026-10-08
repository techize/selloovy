package auth

import (
	"context"
	"errors"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/techize/selloovy/internal/authdb"
)

type SessionState struct {
	OwnerID    int64
	MFAEnabled bool
}

func (s *Store) SessionState(ctx context.Context, token string) (SessionState, error) {
	digest, err := tokenDigest(token, "session")
	if err != nil {
		return SessionState{}, err
	}
	row, err := authdb.New(s.pool).TouchSession(ctx, digest)
	if errors.Is(err, pgx.ErrNoRows) {
		return SessionState{}, ErrCredential
	}
	if err != nil {
		return SessionState{}, ErrStorage
	}
	return SessionState{OwnerID: row.OwnerID, MFAEnabled: row.MfaEnabled}, nil
}

// PrepareMFA requires the current password and a valid session. A fresh seed
// replaces abandoned setup; confirmation is bound to this session for ten minutes.
func (s *Store) PrepareMFA(ctx context.Context, session, password string) (MFASecret, error) {
	state, err := s.SessionState(ctx, session)
	if err != nil {
		return MFASecret{}, err
	}
	if state.MFAEnabled {
		return MFASecret{}, ErrCredential
	}
	if err = s.AllowAttempt(ctx, "enroll-start", strconv.FormatInt(state.OwnerID, 10), 10); err != nil {
		return MFASecret{}, err
	}
	row, err := authdb.New(s.pool).PasswordByID(ctx, state.OwnerID)
	if err != nil {
		return MFASecret{}, ErrStorage
	}
	if err = s.hasher.Verify(ctx, password, row.PasswordHash); err != nil {
		return MFASecret{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return MFASecret{}, ErrStorage
	}
	defer rollback(tx)
	q := authdb.New(tx)
	current, err := q.LockPassword(ctx, state.OwnerID)
	if err != nil {
		return MFASecret{}, ErrStorage
	}
	if current.MfaEnabled || current.AuthVersion != row.AuthVersion || current.PasswordHash != row.PasswordHash {
		return MFASecret{}, ErrCredential
	}
	digest, _ := tokenDigest(session, "session")
	if _, err = q.TouchSession(ctx, digest); errors.Is(err, pgx.ErrNoRows) {
		return MFASecret{}, ErrCredential
	} else if err != nil {
		return MFASecret{}, ErrStorage
	}
	secret, err := NewMFASecret()
	if err != nil {
		return MFASecret{}, err
	}
	ciphertext, err := s.vault.Seal(state.OwnerID, secret)
	if err != nil {
		return MFASecret{}, ErrStorage
	}
	if err = q.SaveBrowserEnrollment(ctx, authdb.SaveBrowserEnrollmentParams{ID: state.OwnerID, MfaCiphertext: ciphertext, MfaEnrollmentSessionDigest: digest}); err != nil {
		return MFASecret{}, ErrStorage
	}
	if err = tx.Commit(ctx); err != nil {
		return MFASecret{}, ErrStorage
	}
	return secret, nil
}
func (s *Store) ConfirmSessionEnrollment(ctx context.Context, session, code string) ([]RecoveryCode, error) {
	state, err := s.SessionState(ctx, session)
	if err != nil {
		return nil, err
	}
	if err = s.AllowAttempt(ctx, "enroll-confirm", strconv.FormatInt(state.OwnerID, 10), 10); err != nil {
		return nil, err
	}
	return s.confirmEnrollment(ctx, state.OwnerID, code, session)
}
