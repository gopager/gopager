package processing

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
)

// Fingerprint returns a keyed digest of data for command fingerprints. The key derives from the
// master key, so the digest reveals nothing about a Secret value to anyone without the key and
// cannot be used to confirm a guess.
func (c *SecretCipher) Fingerprint(data []byte) string {
	mac := hmac.New(sha256.New, c.fingerprintKey)
	mac.Write(data)
	return hex.EncodeToString(mac.Sum(nil))
}

// deriveFingerprintKey separates the fingerprint key from the encryption key.
func deriveFingerprintKey(key []byte) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte("gopager/secret-fingerprint/v1"))
	return mac.Sum(nil)
}
