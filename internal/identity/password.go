package identity

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"strings"

	"golang.org/x/crypto/argon2"
)

const passwordHashPrefix = "$argon2id$v=19$m=19456,t=2,p=1$"

func derivePassword(password string, salt []byte) []byte {
	// Argon2id v19: OWASP's 19 MiB / two passes / one lane minimum.
	return argon2.IDKey([]byte(password), salt, 2, 19*1024, 1, 32)
}

func hashPassword(password string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", errors.New("generate password salt")
	}
	key := derivePassword(password, salt)
	return passwordHashPrefix + base64.RawStdEncoding.EncodeToString(salt) + "$" + base64.RawStdEncoding.EncodeToString(key), nil
}

func passwordMatches(password, encoded string) bool {
	salt, expected := make([]byte, 16), make([]byte, 32)
	valid := false
	parts := strings.Split(encoded, "$")
	// Only the implemented policy is accepted; stored values cannot select unbounded hashing costs.
	if len(parts) == 6 && strings.HasPrefix(encoded, passwordHashPrefix) {
		decodedSalt, saltErr := base64.RawStdEncoding.Strict().DecodeString(parts[4])
		decodedKey, keyErr := base64.RawStdEncoding.Strict().DecodeString(parts[5])
		if saltErr == nil && keyErr == nil && len(decodedSalt) == 16 && len(decodedKey) == 32 {
			salt, expected, valid = decodedSalt, decodedKey, true
		}
	}
	// Unknown accounts and unsupported verifiers still perform the same fixed-cost derivation.
	key := derivePassword(password, salt)
	matched := subtle.ConstantTimeCompare(key, expected) == 1
	return valid && matched
}
