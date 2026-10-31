// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package chatbot

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	httpport "github.com/Jersyfi/hubtask/core/port/httpclient"
)

// Keys answers the Bot Framework's signing key by its identifier.
type Keys interface {
	Key(ctx context.Context, id string, now time.Time) (*rsa.PublicKey, error)
}

// StaticKeys are keys known beforehand, for tests.
type StaticKeys map[string]*rsa.PublicKey

// Key answers a known key.
func (k StaticKeys) Key(_ context.Context, id string, _ time.Time) (*rsa.PublicKey, error) {
	if key, ok := k[id]; ok {
		return key, nil
	}
	return nil, errors.New("chat bot: an unknown signing key")
}

// keyRefresh bounds how often an unknown key fetches the published set again, so a stream of
// forged tokens naming invented keys is not a stream of calls.
const keyRefresh = 5 * time.Minute

// RemoteKeys reads the Bot Framework's published keys through the guarded client, again when a
// token names one it does not know and the last read is older than keyRefresh.
type RemoteKeys struct {
	Client  httpport.Port
	URL     string
	mu      sync.Mutex
	keys    map[string]*rsa.PublicKey
	fetched time.Time
}

// Key answers a published key, reading the set when it is not known yet.
func (r *RemoteKeys) Key(ctx context.Context, id string, now time.Time) (*rsa.PublicKey, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if key, ok := r.keys[id]; ok {
		return key, nil
	}
	if !r.fetched.IsZero() && now.Sub(r.fetched) < keyRefresh {
		return nil, errors.New("chat bot: an unknown signing key")
	}
	r.fetched = now
	response, err := r.Client.Do(ctx, httpport.Request{
		Method: http.MethodGet, URL: r.URL, Header: map[string][]string{"Accept": {"application/json"}}, TargetClass: TargetClass,
	})
	if err != nil {
		return nil, err
	}
	if response.Status/100 != 2 {
		return nil, errors.New("chat bot: the key set answered " + http.StatusText(response.Status))
	}
	keys, err := parseKeys(response.Body)
	if err != nil {
		return nil, err
	}
	r.keys = keys
	if key, ok := keys[id]; ok {
		return key, nil
	}
	return nil, errors.New("chat bot: an unknown signing key")
}

// parseKeys reads a JSON Web Key Set's RSA keys.
func parseKeys(body []byte) (map[string]*rsa.PublicKey, error) {
	var set struct {
		Keys []struct {
			Kty string `json:"kty"`
			Kid string `json:"kid"`
			N   string `json:"n"`
			E   string `json:"e"`
		} `json:"keys"`
	}
	if err := json.Unmarshal(body, &set); err != nil {
		return nil, errors.New("chat bot: the key set is no JSON")
	}
	out := map[string]*rsa.PublicKey{}
	for _, k := range set.Keys {
		n, errN := base64.RawURLEncoding.DecodeString(k.N)
		e, errE := base64.RawURLEncoding.DecodeString(k.E)
		if k.Kty != "RSA" || k.Kid == "" || errN != nil || errE != nil || len(e) == 0 || len(e) > 4 {
			continue
		}
		exponent := int(new(big.Int).SetBytes(e).Int64())
		key := &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: exponent}
		if key.N.BitLen() < 2048 || exponent < 3 {
			continue
		}
		out[k.Kid] = key
	}
	return out, nil
}

// tokenSkew is how far the token's moments may be off the arrival.
const tokenSkew = 5 * time.Minute

type botFrameworkClaims struct {
	Issuer     string          `json:"iss"`
	Audience   json.RawMessage `json:"aud"`
	Expires    int64           `json:"exp"`
	NotBefore  int64           `json:"nbf"`
	ServiceURL string          `json:"serviceurl"`
}

func (c botFrameworkClaims) names(audience string) bool {
	var one string
	if json.Unmarshal(c.Audience, &one) == nil {
		return one == audience
	}
	var many []string
	if json.Unmarshal(c.Audience, &many) == nil {
		for _, a := range many {
			if a == audience {
				return true
			}
		}
	}
	return false
}

// verifyToken holds a token to RS256 by a published key, the Bot Framework as its issuer, the
// bot's application as its audience and its moments; it answers the service address it names.
func verifyToken(ctx context.Context, keys Keys, raw, audience string, now time.Time) (string, error) {
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		return "", errors.New("chat bot: no token")
	}
	var head struct {
		Alg string `json:"alg"`
		Kid string `json:"kid"`
	}
	headRaw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil || json.Unmarshal(headRaw, &head) != nil || head.Alg != "RS256" || head.Kid == "" {
		return "", errors.New("chat bot: a token not signed with RS256")
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return "", errors.New("chat bot: an unreadable signature")
	}
	key, err := keys.Key(ctx, head.Kid, now)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if rsa.VerifyPKCS1v15(key, crypto.SHA256, digest[:], sig) != nil {
		return "", errors.New("chat bot: a signature that does not hold")
	}
	var claims botFrameworkClaims
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || json.Unmarshal(payload, &claims) != nil {
		return "", errors.New("chat bot: unreadable claims")
	}
	switch {
	case claims.Issuer != BotFrameworkIssuer:
		return "", errors.New("chat bot: another issuer")
	case !claims.names(audience):
		return "", errors.New("chat bot: another audience")
	case claims.Expires == 0 || now.After(time.Unix(claims.Expires, 0).Add(tokenSkew)):
		return "", errors.New("chat bot: an expired token")
	case claims.NotBefore != 0 && now.Add(tokenSkew).Before(time.Unix(claims.NotBefore, 0)):
		return "", errors.New("chat bot: a token not yet valid")
	}
	return claims.ServiceURL, nil
}
