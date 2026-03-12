package security

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// SignAlgorithm 预留算法升级位（如国密）。
type SignAlgorithm string

const (
	AlgHMACSHA256 SignAlgorithm = "HMAC_SHA256"
	AlgSM3HMAC    SignAlgorithm = "SM3_HMAC" // 预留
)

type Signer interface {
	Algorithm() SignAlgorithm
	Sign(payload []byte) (string, error)
	Verify(payload []byte, signature string) bool
}

type HMACSigner struct {
	secret []byte
}

func NewHMACSigner(secret string) *HMACSigner {
	return &HMACSigner{secret: []byte(secret)}
}

func (h *HMACSigner) Algorithm() SignAlgorithm {
	return AlgHMACSHA256
}

func (h *HMACSigner) Sign(payload []byte) (string, error) {
	if len(h.secret) == 0 {
		return "", fmt.Errorf("empty secret")
	}
	mac := hmac.New(sha256.New, h.secret)
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil)), nil
}

func (h *HMACSigner) Verify(payload []byte, signature string) bool {
	expected, err := h.Sign(payload)
	if err != nil {
		return false
	}
	expectedBytes := []byte(expected)
	realBytes := []byte(signature)
	return hmac.Equal(expectedBytes, realBytes)
}
