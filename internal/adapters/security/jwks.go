package security

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"sync"
	"time"

	"github.com/code-corhuila/drp-notification-api/internal/app"
	"github.com/golang-jwt/jwt/v5"
)

const ClockSkew = 30 * time.Second

type KeySource interface {
	JWKS(ctx context.Context) (map[string]any, error)
}

type Verifier struct {
	mu     sync.RWMutex
	keys   map[string]*rsa.PublicKey
	source KeySource
	skew   time.Duration
}

func NewVerifier(source KeySource) *Verifier {
	return &Verifier{keys: map[string]*rsa.PublicKey{}, source: source, skew: ClockSkew}
}

type StaticJWKS map[string]any

func (s StaticJWKS) JWKS(context.Context) (map[string]any, error) {
	return s, nil
}

type HTTPJWKS struct {
	URL    string
	Client *http.Client
}

func (h HTTPJWKS) JWKS(ctx context.Context) (map[string]any, error) {
	if h.URL == "" {
		return nil, app.ErrJWKSUnavailable
	}
	client := h.Client
	if client == nil {
		client = http.DefaultClient
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, h.URL, nil)
	if err != nil {
		return nil, err
	}
	res, err := client.Do(req)
	if err != nil {
		return nil, app.ErrJWKSUnavailable
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, app.ErrJWKSUnavailable
	}
	var doc map[string]any
	if err := json.NewDecoder(res.Body).Decode(&doc); err != nil {
		return nil, app.ErrJWKSUnavailable
	}
	return doc, nil
}

func (v *Verifier) Parse(ctx context.Context, raw string) (app.Principal, error) {
	parser := jwt.NewParser(
		jwt.WithValidMethods([]string{jwt.SigningMethodRS256.Alg()}),
		jwt.WithLeeway(v.skew),
		jwt.WithExpirationRequired(),
	)
	tok, err := parser.Parse(raw, func(t *jwt.Token) (any, error) {
		if t.Method != jwt.SigningMethodRS256 {
			return nil, jwt.ErrTokenSignatureInvalid
		}
		kid, _ := t.Header["kid"].(string)
		key, err := v.keyFor(ctx, kid)
		if err != nil {
			return nil, err
		}
		return key, nil
	})
	if err != nil {
		if errors.Is(err, app.ErrJWKSUnavailable) {
			return app.Principal{}, app.ErrJWKSUnavailable
		}
		return app.Principal{}, app.ErrUnauthorized
	}
	if !tok.Valid {
		return app.Principal{}, app.ErrUnauthorized
	}
	claims, ok := tok.Claims.(jwt.MapClaims)
	if !ok {
		return app.Principal{}, app.ErrUnauthorized
	}
	sub, err := claims.GetSubject()
	if err != nil || sub == "" {
		return app.Principal{}, app.ErrUnauthorized
	}
	return app.Principal{Subject: sub, Role: claimRole(claims)}, nil
}

func claimRole(claims jwt.MapClaims) string {
	if r, ok := claims["role"].(string); ok && (r == "USER" || r == "ADMIN") {
		return r
	}
	switch roles := claims["roles"].(type) {
	case []any:
		if len(roles) > 0 {
			if s, ok := roles[0].(string); ok && (s == "USER" || s == "ADMIN") {
				return s
			}
		}
	case []string:
		if len(roles) > 0 && (roles[0] == "USER" || roles[0] == "ADMIN") {
			return roles[0]
		}
	}
	return "USER"
}

func (v *Verifier) keyFor(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	if k := v.cached(kid); k != nil {
		return k, nil
	}
	if err := v.refresh(ctx); err != nil {
		return nil, err
	}
	if k := v.cached(kid); k != nil {
		return k, nil
	}
	return nil, app.ErrUnauthorized
}

func (v *Verifier) cached(kid string) *rsa.PublicKey {
	v.mu.RLock()
	defer v.mu.RUnlock()
	if kid == "" && len(v.keys) == 1 {
		for _, k := range v.keys {
			return k
		}
	}
	return v.keys[kid]
}

func (v *Verifier) refresh(ctx context.Context) error {
	doc, err := v.source.JWKS(ctx)
	if err != nil {
		return app.ErrJWKSUnavailable
	}
	keys, err := parseJWKS(doc)
	if err != nil {
		return app.ErrJWKSUnavailable
	}
	v.mu.Lock()
	v.keys = keys
	v.mu.Unlock()
	return nil
}

func parseJWKS(doc map[string]any) (map[string]*rsa.PublicKey, error) {
	out := map[string]*rsa.PublicKey{}
	for _, m := range jwkMaps(doc["keys"]) {
		kty, _ := m["kty"].(string)
		alg, _ := m["alg"].(string)
		if kty != "RSA" || (alg != "" && alg != "RS256") {
			continue
		}
		n, _ := m["n"].(string)
		e, _ := m["e"].(string)
		kid, _ := m["kid"].(string)
		pub, err := rsaFromJWK(n, e)
		if err != nil || kid == "" {
			continue
		}
		out[kid] = pub
	}
	if len(out) == 0 {
		return nil, errors.New("empty jwks")
	}
	return out, nil
}

func jwkMaps(raw any) []map[string]any {
	switch keys := raw.(type) {
	case []any:
		out := make([]map[string]any, 0, len(keys))
		for _, item := range keys {
			if m, ok := item.(map[string]any); ok {
				out = append(out, m)
				continue
			}
			if m, ok := item.(map[string]string); ok {
				out = append(out, stringMap(m))
			}
		}
		return out
	case []map[string]any:
		return keys
	case []map[string]string:
		out := make([]map[string]any, 0, len(keys))
		for _, m := range keys {
			out = append(out, stringMap(m))
		}
		return out
	default:
		return nil
	}
}

func stringMap(in map[string]string) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func rsaFromJWK(nStr, eStr string) (*rsa.PublicKey, error) {
	nBytes, err := base64.RawURLEncoding.DecodeString(nStr)
	if err != nil || len(nBytes) == 0 {
		return nil, errors.New("bad n")
	}
	eBytes, err := base64.RawURLEncoding.DecodeString(eStr)
	if err != nil || len(eBytes) == 0 {
		return nil, errors.New("bad e")
	}
	e := 0
	for _, b := range eBytes {
		e = e<<8 + int(b)
	}
	if e <= 0 {
		return nil, errors.New("bad e")
	}
	return &rsa.PublicKey{N: new(big.Int).SetBytes(nBytes), E: e}, nil
}

func PublicJWKS(pub *rsa.PublicKey, kid string) map[string]any {
	return map[string]any{
		"keys": []any{
			map[string]any{
				"kty": "RSA",
				"use": "sig",
				"kid": kid,
				"alg": "RS256",
				"n":   base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
				"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes()),
			},
		},
	}
}
