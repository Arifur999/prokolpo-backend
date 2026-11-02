package staff

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
)

// sealer encrypts TOTP secrets at rest with AES-GCM; the key is derived from the server's
// signing secret, so a database dump alone can't produce codes.
type sealer struct{ aead cipher.AEAD }

func newSealer(serverSecret []byte) (sealer, error) {
	key := sha256.Sum256(append([]byte("prokolpo/totp/v1:"), serverSecret...))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return sealer{}, fmt.Errorf("totp cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return sealer{}, fmt.Errorf("totp cipher: %w", err)
	}
	return sealer{aead: aead}, nil
}

func (s sealer) seal(plain []byte) ([]byte, error) {
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("totp nonce: %w", err)
	}
	return s.aead.Seal(nonce, nonce, plain, nil), nil
}

func (s sealer) open(sealed []byte) ([]byte, error) {
	n := s.aead.NonceSize()
	if len(sealed) < n {
		return nil, errors.New("totp secret too short")
	}
	plain, err := s.aead.Open(nil, sealed[:n], sealed[n:], nil)
	if err != nil {
		return nil, fmt.Errorf("open totp secret: %w", err)
	}
	return plain, nil
}

// Backup codes: eight characters from a set without look-alikes (no 0, O, 1, I), shown as XXXX-XXXX.
const backupAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

func newBackupCode() (string, error) {
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("generate backup code: %w", err)
	}
	var b strings.Builder
	for i, v := range raw {
		if i == 4 {
			b.WriteByte('-')
		}
		b.WriteByte(backupAlphabet[int(v)%len(backupAlphabet)])
	}
	return b.String(), nil
}

// normalBackup is a typed code as it is hashed: upper case, no dash or spaces.
func normalBackup(code string) string {
	return strings.NewReplacer("-", "", " ", "").Replace(strings.ToUpper(strings.TrimSpace(code)))
}

func hashBackup(code string) []byte {
	sum := sha256.Sum256([]byte("prokolpo/backup:" + normalBackup(code)))
	return sum[:]
}
