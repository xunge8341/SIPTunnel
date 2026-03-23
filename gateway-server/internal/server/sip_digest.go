package server

import (
	"crypto/md5"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"

	"siptunnel/internal/nodeconfig"
)

type sipDigestChallenge struct {
	Realm     string
	Nonce     string
	Algorithm string
	QOP       string
	Opaque    string
	Stale     bool
}

type sipDigestAuthorization struct {
	Username  string
	Realm     string
	Nonce     string
	URI       string
	Response  string
	Algorithm string
	QOP       string
	NC        string
	CNonce    string
	Opaque    string
}

func effectiveRegisterAuthRealm(cfg TunnelConfigPayload, local nodeconfig.LocalNodeConfig) string {
	realm := strings.TrimSpace(cfg.RegisterAuthRealm)
	if realm != "" {
		return realm
	}
	if v := strings.TrimSpace(local.NodeID); v != "" {
		return v
	}
	return "siptunnel"
}

func buildRegisterDigestChallenge(cfg TunnelConfigPayload, local nodeconfig.LocalNodeConfig) sipDigestChallenge {
	algorithm := strings.ToUpper(strings.TrimSpace(cfg.RegisterAuthAlgorithm))
	if algorithm == "" {
		algorithm = "MD5"
	}
	return sipDigestChallenge{
		Realm:     effectiveRegisterAuthRealm(cfg, local),
		Nonce:     issueDigestNonce(effectiveRegisterAuthRealm(cfg, local), strings.TrimSpace(cfg.RegisterAuthPassword), strings.TrimSpace(local.NodeID)),
		Algorithm: algorithm,
		QOP:       "auth",
	}
}

func issueDigestNonce(realm, password, nodeID string) string {
	ts := time.Now().UTC().Unix()
	raw := fmt.Sprintf("%d:%s:%s:%s", ts, strings.TrimSpace(realm), strings.TrimSpace(password), strings.TrimSpace(nodeID))
	sig := fmt.Sprintf("%x", md5.Sum([]byte(raw)))
	return base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%d:%s", ts, sig)))
}

func verifyDigestNonce(nonce, realm, password, nodeID string, maxAge time.Duration) bool {
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(nonce))
	if err != nil {
		return false
	}
	parts := strings.SplitN(string(decoded), ":", 2)
	if len(parts) != 2 {
		return false
	}
	ts, err := strconv.ParseInt(strings.TrimSpace(parts[0]), 10, 64)
	if err != nil {
		return false
	}
	issuedAt := time.Unix(ts, 0).UTC()
	if time.Since(issuedAt) > maxAge || issuedAt.After(time.Now().UTC().Add(30*time.Second)) {
		return false
	}
	expected := fmt.Sprintf("%x", md5.Sum([]byte(fmt.Sprintf("%d:%s:%s:%s", ts, strings.TrimSpace(realm), strings.TrimSpace(password), strings.TrimSpace(nodeID)))))
	return strings.EqualFold(strings.TrimSpace(parts[1]), expected)
}

func formatSIPDigestChallenge(ch sipDigestChallenge) string {
	parts := []string{
		fmt.Sprintf("realm=%q", strings.TrimSpace(ch.Realm)),
		fmt.Sprintf("nonce=%q", strings.TrimSpace(ch.Nonce)),
	}
	if v := strings.ToUpper(strings.TrimSpace(ch.Algorithm)); v != "" {
		parts = append(parts, fmt.Sprintf("algorithm=%s", v))
	}
	if v := strings.TrimSpace(ch.QOP); v != "" {
		parts = append(parts, fmt.Sprintf("qop=%q", v))
	}
	if v := strings.TrimSpace(ch.Opaque); v != "" {
		parts = append(parts, fmt.Sprintf("opaque=%q", v))
	}
	if ch.Stale {
		parts = append(parts, "stale=true")
	}
	return "Digest " + strings.Join(parts, ", ")
}

func parseSIPDigestChallenge(header string) (sipDigestChallenge, bool) {
	params, ok := parseSIPDigestParams(header)
	if !ok {
		return sipDigestChallenge{}, false
	}
	return sipDigestChallenge{
		Realm:     params["realm"],
		Nonce:     params["nonce"],
		Algorithm: firstNonEmpty(params["algorithm"], "MD5"),
		QOP:       params["qop"],
		Opaque:    params["opaque"],
		Stale:     strings.EqualFold(params["stale"], "true"),
	}, true
}

func parseSIPDigestAuthorization(header string) (sipDigestAuthorization, bool) {
	params, ok := parseSIPDigestParams(header)
	if !ok {
		return sipDigestAuthorization{}, false
	}
	return sipDigestAuthorization{
		Username:  params["username"],
		Realm:     params["realm"],
		Nonce:     params["nonce"],
		URI:       params["uri"],
		Response:  params["response"],
		Algorithm: firstNonEmpty(params["algorithm"], "MD5"),
		QOP:       params["qop"],
		NC:        params["nc"],
		CNonce:    params["cnonce"],
		Opaque:    params["opaque"],
	}, true
}

func parseSIPDigestParams(header string) (map[string]string, bool) {
	trimmed := strings.TrimSpace(header)
	if trimmed == "" {
		return nil, false
	}
	if idx := strings.Index(trimmed, " "); idx >= 0 {
		if !strings.EqualFold(strings.TrimSpace(trimmed[:idx]), "Digest") {
			return nil, false
		}
		trimmed = strings.TrimSpace(trimmed[idx+1:])
	}
	if trimmed == "" {
		return nil, false
	}
	parts := splitCommaAware(trimmed)
	out := make(map[string]string, len(parts))
	for _, part := range parts {
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(kv[0]))
		val := strings.Trim(strings.TrimSpace(kv[1]), "\"")
		if key != "" {
			out[key] = val
		}
	}
	if out["realm"] == "" && out["nonce"] == "" {
		return nil, false
	}
	return out, true
}

func splitCommaAware(raw string) []string {
	var (
		parts   []string
		current strings.Builder
		quoted  bool
	)
	for _, r := range raw {
		switch r {
		case '"':
			quoted = !quoted
			current.WriteRune(r)
		case ',':
			if quoted {
				current.WriteRune(r)
				continue
			}
			parts = append(parts, strings.TrimSpace(current.String()))
			current.Reset()
		default:
			current.WriteRune(r)
		}
	}
	if current.Len() > 0 {
		parts = append(parts, strings.TrimSpace(current.String()))
	}
	return parts
}

func buildSIPDigestAuthorization(method, uri, username, password string, ch sipDigestChallenge) string {
	algorithm := strings.ToUpper(strings.TrimSpace(ch.Algorithm))
	if algorithm == "" {
		algorithm = "MD5"
	}
	realm := strings.TrimSpace(ch.Realm)
	nonce := strings.TrimSpace(ch.Nonce)
	qop := strings.TrimSpace(ch.QOP)
	nc := "00000001"
	cnonce := fmt.Sprintf("%x", md5.Sum([]byte(fmt.Sprintf("%s:%s:%s", username, nonce, method))))[:16]
	ha1 := digestHex(algorithm, fmt.Sprintf("%s:%s:%s", username, realm, password))
	ha2 := digestHex(algorithm, fmt.Sprintf("%s:%s", strings.ToUpper(strings.TrimSpace(method)), strings.TrimSpace(uri)))
	responseInput := fmt.Sprintf("%s:%s:%s", ha1, nonce, ha2)
	if qop != "" {
		responseInput = fmt.Sprintf("%s:%s:%s:%s:%s:%s", ha1, nonce, nc, cnonce, qop, ha2)
	}
	response := digestHex(algorithm, responseInput)
	parts := []string{
		fmt.Sprintf("username=%q", strings.TrimSpace(username)),
		fmt.Sprintf("realm=%q", realm),
		fmt.Sprintf("nonce=%q", nonce),
		fmt.Sprintf("uri=%q", strings.TrimSpace(uri)),
		fmt.Sprintf("response=%q", response),
		fmt.Sprintf("algorithm=%s", algorithm),
	}
	if qop != "" {
		parts = append(parts, fmt.Sprintf("qop=%q", qop), fmt.Sprintf("nc=%s", nc), fmt.Sprintf("cnonce=%q", cnonce))
	}
	if v := strings.TrimSpace(ch.Opaque); v != "" {
		parts = append(parts, fmt.Sprintf("opaque=%q", v))
	}
	return "Digest " + strings.Join(parts, ", ")
}

func verifySIPDigestAuthorization(header, method, uri, expectedUsername, password, realm, nodeID string) bool {
	auth, ok := parseSIPDigestAuthorization(header)
	if !ok {
		return false
	}
	if expectedUsername != "" && !strings.EqualFold(strings.TrimSpace(auth.Username), strings.TrimSpace(expectedUsername)) {
		return false
	}
	if realm != "" && auth.Realm != realm {
		return false
	}
	if !verifyDigestNonce(auth.Nonce, realm, password, nodeID, 15*time.Minute) {
		return false
	}
	algorithm := strings.ToUpper(strings.TrimSpace(auth.Algorithm))
	if algorithm == "" {
		algorithm = "MD5"
	}
	ha1 := digestHex(algorithm, fmt.Sprintf("%s:%s:%s", strings.TrimSpace(auth.Username), realm, password))
	ha2 := digestHex(algorithm, fmt.Sprintf("%s:%s", strings.ToUpper(strings.TrimSpace(method)), strings.TrimSpace(uri)))
	expected := fmt.Sprintf("%s:%s:%s", ha1, strings.TrimSpace(auth.Nonce), ha2)
	if strings.TrimSpace(auth.QOP) != "" {
		expected = fmt.Sprintf("%s:%s:%s:%s:%s:%s", ha1, strings.TrimSpace(auth.Nonce), strings.TrimSpace(auth.NC), strings.TrimSpace(auth.CNonce), strings.TrimSpace(auth.QOP), ha2)
	}
	return strings.EqualFold(strings.TrimSpace(auth.Response), digestHex(algorithm, expected))
}

func digestHex(algorithm, value string) string {
	switch strings.ToUpper(strings.TrimSpace(algorithm)) {
	case "", "MD5":
		return fmt.Sprintf("%x", md5.Sum([]byte(value)))
	default:
		return fmt.Sprintf("%x", md5.Sum([]byte(value)))
	}
}
