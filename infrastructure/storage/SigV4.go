// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package storage

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

// AWS Signature Version 4, over the standard library.
//
// Hand-written rather than imported, and that is a decision with a reason: an S3 SDK is a new
// third-party dependency, and a dependency is a supply-chain decision that is not taken inside a
// pull request (CLAUDE.md). The protocol below is HMAC-SHA256 chained four times over public,
// stable inputs - the standard library's primitives, no cryptography of our own (security.md §8)
// - and the three requests this adapter makes carry no query strings and three headers, which is
// the corner of SigV4 that fits on a page. MinIO validates every signature strictly, so the
// conformance suite is what proves this file rather than trust.
//
// Swapping to an SDK stays open as an ADR; this file is what it would replace.

// unsignedPayload is the marker for a streamed body: the content hash is not computed, because
// hashing would mean reading the stream twice or buffering it, and the connection is TLS in
// production (T-17 wins over a second integrity layer the transport already provides).
const unsignedPayload = "UNSIGNED-PAYLOAD"

// emptyPayloadHash is sha256 of nothing: what GET and DELETE carry.
const emptyPayloadHash = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

// signV4 signs the request in place: x-amz-date, x-amz-content-sha256 and Authorization.
func signV4(req *http.Request, accessKey, secretKey, region, service, payloadHash string, now time.Time) {
	stamp := now.UTC().Format("20060102T150405Z")
	date := stamp[:8]

	req.Header.Set("x-amz-date", stamp)
	req.Header.Set("x-amz-content-sha256", payloadHash)

	host := req.Host
	if host == "" {
		host = req.URL.Host
	}
	// The three headers every request here carries, already sorted. A fourth header would need
	// to join this list to be part of the signature; S3 only demands host and the two x-amz ones.
	canonicalHeaders := "host:" + host + "\n" +
		"x-amz-content-sha256:" + payloadHash + "\n" +
		"x-amz-date:" + stamp + "\n"
	signedHeaders := "host;x-amz-content-sha256;x-amz-date"

	canonicalRequest := strings.Join([]string{
		req.Method,
		canonicalURI(req.URL.Path),
		canonicalQuery(req.URL.RawQuery),
		canonicalHeaders,
		signedHeaders,
		payloadHash,
	}, "\n")

	scope := date + "/" + region + "/" + service + "/aws4_request"
	stringToSign := strings.Join([]string{
		"AWS4-HMAC-SHA256",
		stamp,
		scope,
		hexSHA256([]byte(canonicalRequest)),
	}, "\n")

	signature := hex.EncodeToString(
		hmacSHA256(deriveKey(secretKey, date, region, service), []byte(stringToSign)))

	req.Header.Set("Authorization",
		"AWS4-HMAC-SHA256 Credential="+accessKey+"/"+scope+
			", SignedHeaders="+signedHeaders+
			", Signature="+signature)
}

// deriveKey is the four-step HMAC chain of the specification.
func deriveKey(secretKey, date, region, service string) []byte {
	key := hmacSHA256([]byte("AWS4"+secretKey), []byte(date))
	key = hmacSHA256(key, []byte(region))
	key = hmacSHA256(key, []byte(service))
	return hmacSHA256(key, []byte("aws4_request"))
}

func hmacSHA256(key, data []byte) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write(data)
	return mac.Sum(nil)
}

func hexSHA256(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// canonicalURI is the path with every segment URI-encoded the way SigV4 wants it: RFC 3986
// unreserved characters bare, everything else percent-encoded upper-case, the slashes kept.
// Go's URL escaping makes different choices (it leaves '$' and '&' alone, among others), so the
// encoding is written out against the specification's own table.
func canonicalURI(path string) string {
	if path == "" {
		return "/"
	}
	segments := strings.Split(path, "/")
	for i, segment := range segments {
		segments[i] = uriEncode(segment)
	}
	return strings.Join(segments, "/")
}

// canonicalQuery sorts and encodes the query. The three requests this adapter makes carry none,
// but a signer that silently mis-signed the first future query parameter would be a debugging
// afternoon somebody else pays for.
func canonicalQuery(rawQuery string) string {
	if rawQuery == "" {
		return ""
	}

	var pairs []string
	for _, pair := range strings.Split(rawQuery, "&") {
		key, value, _ := strings.Cut(pair, "=")
		pairs = append(pairs, uriEncode(key)+"="+uriEncode(value))
	}
	sort.Strings(pairs)
	return strings.Join(pairs, "&")
}

// presignV4 signs a URL instead of a request: the query form of the same signature, which is
// what makes the URL itself the capability - whoever holds it may perform exactly this method on
// exactly this object until the expiry, and nothing else (arc42 §8.4).
//
// `extra` carries response-override parameters (response-content-disposition and friends); they
// are signed like everything else, so a holder cannot strip the attachment disposition off a
// download URL without invalidating it (T-11).
func presignV4(method string, target string, accessKey, secretKey, region, service string,
	expires time.Duration, now time.Time, extra map[string]string,
) string {
	stamp := now.UTC().Format("20060102T150405Z")
	date := stamp[:8]
	scope := date + "/" + region + "/" + service + "/aws4_request"

	host, path := splitOrigin(target)
	query := map[string]string{
		"X-Amz-Algorithm":     "AWS4-HMAC-SHA256",
		"X-Amz-Credential":    accessKey + "/" + scope,
		"X-Amz-Date":          stamp,
		"X-Amz-Expires":       strconv.Itoa(int(expires.Seconds())),
		"X-Amz-SignedHeaders": "host",
	}
	for key, value := range extra {
		query[key] = value
	}

	pairs := make([]string, 0, len(query))
	for key, value := range query {
		pairs = append(pairs, uriEncode(key)+"="+uriEncode(value))
	}
	sort.Strings(pairs)
	canonicalQueryString := strings.Join(pairs, "&")

	canonicalRequest := strings.Join([]string{
		method,
		canonicalURI(path),
		canonicalQueryString,
		"host:" + host + "\n",
		"host",
		unsignedPayload,
	}, "\n")

	stringToSign := strings.Join([]string{
		"AWS4-HMAC-SHA256", stamp, scope, hexSHA256([]byte(canonicalRequest)),
	}, "\n")
	signature := hex.EncodeToString(
		hmacSHA256(deriveKey(secretKey, date, region, service), []byte(stringToSign)))

	return target + "?" + canonicalQueryString + "&X-Amz-Signature=" + signature
}

// splitOrigin separates scheme://host from the path of an already-built object URL.
func splitOrigin(target string) (host, path string) {
	rest := target
	if at := strings.Index(rest, "://"); at >= 0 {
		rest = rest[at+3:]
	}
	if at := strings.Index(rest, "/"); at >= 0 {
		return rest[:at], rest[at:]
	}
	return rest, "/"
}

const upperhex = "0123456789ABCDEF"

func uriEncode(value string) string {
	var out strings.Builder
	for i := 0; i < len(value); i++ {
		c := value[i]
		switch {
		case c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z', c >= '0' && c <= '9',
			c == '-', c == '.', c == '_', c == '~':
			out.WriteByte(c)
		default:
			out.WriteByte('%')
			out.WriteByte(upperhex[c>>4])
			out.WriteByte(upperhex[c&0xF])
		}
	}
	return out.String()
}
