package server

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

type secureJSONEnvelope struct {
	Encrypted bool   `json:"encrypted"`
	Algorithm string `json:"algorithm"`
	Nonce     string `json:"nonce"`
	Payload   string `json:"payload"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

func secureConfigKey() []byte {
	secret := strings.TrimSpace(os.Getenv("GATEWAY_CONFIG_KEY"))
	if secret == "" {
		return nil
	}
	sum := sha256.Sum256([]byte(secret))
	key := make([]byte, len(sum))
	copy(key, sum[:])
	return key
}

func configEncryptionEnabled() bool {
	return len(secureConfigKey()) > 0
}

func tunnelSignerSecretConfigured() bool {
	if strings.TrimSpace(os.Getenv("GATEWAY_TUNNEL_SIGNER_SECRET")) != "" {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(os.Getenv("GATEWAY_ALLOW_INSECURE_DEFAULT_SIGNER")), "true")
}

func marshalSecureJSON(payload any) ([]byte, error) {
	plain, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return nil, err
	}
	key := secureConfigKey()
	if len(key) == 0 {
		return plain, nil
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	sealed := gcm.Seal(nil, nonce, plain, nil)
	env := secureJSONEnvelope{
		Encrypted: true,
		Algorithm: "AES-256-GCM",
		Nonce:     base64.StdEncoding.EncodeToString(nonce),
		Payload:   base64.StdEncoding.EncodeToString(sealed),
		UpdatedAt: formatTimestamp(time.Now().UTC()),
	}
	return json.MarshalIndent(env, "", "  ")
}

func unmarshalSecureJSON(buf []byte, out any) error {
	var env secureJSONEnvelope
	if err := json.Unmarshal(buf, &env); err == nil && env.Encrypted {
		key := secureConfigKey()
		if len(key) == 0 {
			return fmt.Errorf("encrypted config requires GATEWAY_CONFIG_KEY")
		}
		nonce, err := base64.StdEncoding.DecodeString(strings.TrimSpace(env.Nonce))
		if err != nil {
			return err
		}
		payload, err := base64.StdEncoding.DecodeString(strings.TrimSpace(env.Payload))
		if err != nil {
			return err
		}
		block, err := aes.NewCipher(key)
		if err != nil {
			return err
		}
		gcm, err := cipher.NewGCM(block)
		if err != nil {
			return err
		}
		plain, err := gcm.Open(nil, nonce, payload, nil)
		if err != nil {
			return err
		}
		return json.Unmarshal(plain, out)
	}
	return json.Unmarshal(buf, out)
}
