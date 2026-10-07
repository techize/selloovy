package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"strings"
)

// RecoveryCode is a randomly generated 128-bit bearer second factor. Store only
// its digest. A database transaction must consume it once, alongside session
// issuance; a digest comparison alone does not implement one-use recovery.
type RecoveryCode struct{ value string }

func (RecoveryCode) Format(s fmt.State, _ rune) { _, _ = io.WriteString(s, "[redacted recovery code]") }
func (c RecoveryCode) Reveal() string           { return c.value }

func NewRecoveryCodes(ownerID int64) ([]RecoveryCode, [][32]byte, error) {
	if ownerID < 1 {
		return nil, nil, ErrCredential
	}
	codes := make([]RecoveryCode, 10)
	digests := make([][32]byte, len(codes))
	for i := range codes {
		var entropy [16]byte
		if _, err := rand.Read(entropy[:]); err != nil {
			return nil, nil, ErrCredential
		}
		value := strings.ToUpper(hex.EncodeToString(entropy[:]))
		codes[i].value = value[:8] + "-" + value[8:16] + "-" + value[16:24] + "-" + value[24:]
		digests[i], _ = RecoveryDigest(ownerID, codes[i].value)
	}
	return codes, digests, nil
}

func RecoveryDigest(ownerID int64, code string) ([32]byte, error) {
	if ownerID < 1 || len(code) > 37 {
		return [32]byte{}, ErrCredential
	}
	canonical := strings.ReplaceAll(strings.TrimSpace(code), "-", "")
	if len(canonical) != 32 {
		return [32]byte{}, ErrCredential
	}
	decoded, err := hex.DecodeString(canonical)
	if err != nil {
		return [32]byte{}, ErrCredential
	}
	defer clear(decoded)
	var owner [8]byte
	binary.BigEndian.PutUint64(owner[:], uint64(ownerID))
	hash := sha256.New()
	_, _ = hash.Write([]byte("selloovy/recovery/v1/"))
	_, _ = hash.Write(owner[:])
	_, _ = hash.Write(decoded)
	var digest [32]byte
	copy(digest[:], hash.Sum(nil))
	return digest, nil
}
