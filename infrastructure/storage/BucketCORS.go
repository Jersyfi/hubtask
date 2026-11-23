// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package storage

import (
	"bytes"
	"context"
	"crypto/md5" //nolint:gosec // G501: Content-MD5 is the integrity header S3 names for this call (RFC 1864), not a security primitive
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	env "github.com/Jersyfi/hubtask/core/port/environment"
	"github.com/Jersyfi/hubtask/infrastructure/awssig"
)

// The browser's upload is a PUT of the presigned URL with a Content-Type and no credentials, so
// that is exactly what the preflight asks and what the rule allows (deployment.md §6.1); its
// read of a presigned download is a GET, which the rule allows too.
const (
	uploadMethod = http.MethodPut
	readMethod   = http.MethodGet
	uploadHeader = "Content-Type"

	// HubtaskRuleID marks the one CORS rule Hubtask writes, so that a later apply can tell it
	// from the rules somebody else set.
	HubtaskRuleID = "hubtask-upload"

	// corsProbeKey is the object the preflight names. Any key does: a bucket answers a preflight
	// from its rules, not from the object, and none needs to exist.
	corsProbeKey = "hubtask-cors-preflight"

	// corsProbeLabel is the workspace label the check asks with in subdomain mode. Any label
	// under the base host proves the wildcard, and a fixed one needs no query per run.
	corsProbeLabel = "hubtask-cors-check"

	// corsTimeout bounds one preflight or one call on the bucket's configuration (rule 7).
	corsTimeout = 10 * time.Second

	// corsBodyLimit bounds the configuration read back; S3 caps the document at 64 KB.
	corsBodyLimit = 64 << 10

	// corsRuleLimit is how many rules one configuration may hold (S3's documented limit).
	corsRuleLimit = 100

	s3Namespace = "http://s3.amazonaws.com/doc/2006-03-01/"
)

// PreflightOutcome is what one preflight found.
type PreflightOutcome int

const (
	// PreflightAllowed is a bucket that lets the browser's upload through from the origin.
	PreflightAllowed PreflightOutcome = iota + 1
	// PreflightRefused is a bucket that answered and did not allow it: the CORS finding.
	PreflightRefused
	// PreflightUnreachable is no answer to judge - the wire's problem, not a CORS finding.
	PreflightUnreachable
)

// Preflight is one origin's answer.
type Preflight struct {
	Origin  string
	Outcome PreflightOutcome
	// Status is the HTTP status the bucket answered; zero when it did not answer.
	Status int
}

// Preflight asks the bucket what a browser at origin asks before the upload: an OPTIONS without
// any credential, since a browser sends none, under its own deadline and with redirects refused
// by the adapter's client. Called on the adapter itself, never through the resilient wrapper: a
// failed preflight is a finding about the rule, and must not open the breaker media health reads.
func (s *S3Storage) Preflight(ctx context.Context, origin string) Preflight {
	ctx, cancel := context.WithTimeout(ctx, corsTimeout)
	defer cancel()

	target, err := s.objectURL(corsProbeKey)
	if err != nil {
		return Preflight{Origin: origin, Outcome: PreflightUnreachable}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodOptions, target, nil)
	if err != nil {
		return Preflight{Origin: origin, Outcome: PreflightUnreachable}
	}
	req.Header.Set("Origin", origin)
	req.Header.Set("Access-Control-Request-Method", uploadMethod)
	req.Header.Set("Access-Control-Request-Headers", strings.ToLower(uploadHeader))

	resp, err := s.client.Do(req)
	if err != nil {
		return Preflight{Origin: origin, Outcome: PreflightUnreachable}
	}
	defer drain(resp)

	if resp.StatusCode >= 500 {
		return Preflight{Origin: origin, Outcome: PreflightUnreachable, Status: resp.StatusCode}
	}
	outcome := PreflightRefused
	if allowsUpload(resp.Header, origin) {
		outcome = PreflightAllowed
	}
	return Preflight{Origin: origin, Outcome: outcome, Status: resp.StatusCode}
}

// allowsUpload is the browser's own reading of the answer: the origin echoed back or `*`, and
// PUT among the methods. A bucket never answers with a pattern, so none is looked for.
func allowsUpload(header http.Header, origin string) bool {
	allowed := strings.TrimSpace(header.Get("Access-Control-Allow-Origin"))
	if allowed != origin && allowed != "*" {
		return false
	}
	for _, method := range strings.Split(header.Get("Access-Control-Allow-Methods"), ",") {
		if strings.EqualFold(strings.TrimSpace(method), uploadMethod) {
			return true
		}
	}
	return false
}

// UploadOrigins are the origins the browser uploads from, derived from the configuration: Check
// the ones a preflight asks with, Rule the ones the rule names. The base URL's origin always; in
// multi mode with subdomain hosts also a workspace's - one label under the base host, which the
// rule covers with a wildcard. With assigned hosts each workspace's host is its
// own origin and none can be derived here; the documentation names that.
type UploadOrigins struct {
	Check []string
	Rule  []string
}

// OriginsFor derives the upload origins. An unset or unreadable base URL answers none: the
// health report already warns about it, and there is nothing to ask with.
func OriginsFor(baseURL string, tenancy env.TenancyMode, hosts env.WorkspaceHosts) UploadOrigins {
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return UploadOrigins{}
	}
	host := parsed.Hostname()
	if port := parsed.Port(); port != "" && !isDefaultPort(parsed.Scheme, port) {
		host += ":" + port
	}
	base := parsed.Scheme + "://" + host
	origins := UploadOrigins{Check: []string{base}, Rule: []string{base}}
	if tenancy == env.TenancyMulti && hosts == env.HostsSubdomain {
		origins.Check = append(origins.Check, parsed.Scheme+"://"+corsProbeLabel+"."+host)
		origins.Rule = append(origins.Rule, parsed.Scheme+"://*."+host)
	}
	return origins
}

// isDefaultPort is the port a browser leaves out of the Origin it sends.
func isDefaultPort(scheme, port string) bool {
	return (scheme == "https" && port == "443") || (scheme == "http" && port == "80")
}

// CORSRule is one rule of a bucket's CORS configuration, as S3 writes it. Elements this type
// does not name are carried through unchanged, so that keeping somebody else's rule keeps all of
// it. The JSON form is the one a provider's console takes.
type CORSRule struct {
	ID             string           `xml:"ID,omitempty" json:"-"`
	AllowedHeaders []string         `xml:"AllowedHeader" json:"AllowedHeaders,omitempty"`
	AllowedMethods []string         `xml:"AllowedMethod" json:"AllowedMethods"`
	AllowedOrigins []string         `xml:"AllowedOrigin" json:"AllowedOrigins"`
	ExposeHeaders  []string         `xml:"ExposeHeader" json:"ExposeHeaders,omitempty"`
	MaxAgeSeconds  *int             `xml:"MaxAgeSeconds,omitempty" json:"MaxAgeSeconds,omitempty"`
	Other          []unknownElement `xml:",any" json:"-"`
}

type unknownElement struct {
	XMLName xml.Name
	Attrs   []xml.Attr `xml:",any,attr"`
	Inner   string     `xml:",innerxml"`
}

type corsConfiguration struct {
	XMLName xml.Name   `xml:"CORSConfiguration"`
	Xmlns   string     `xml:"xmlns,attr,omitempty"`
	Rules   []CORSRule `xml:"CORSRule"`
}

// HubtaskRule is the rule the browser needs from these origins: PUT with a Content-Type for an
// upload, GET for reading a download's bytes, and no credentials - the presigned URL is the
// credential.
func HubtaskRule(origins []string) CORSRule {
	return CORSRule{
		ID:             HubtaskRuleID,
		AllowedOrigins: append([]string(nil), origins...),
		AllowedMethods: []string{uploadMethod, readMethod},
		AllowedHeaders: []string{uploadHeader},
	}
}

// ErrCORSForbidden is a key that may not read or set the bucket's CORS configuration.
var ErrCORSForbidden = errors.New("the storage key may not read or set the bucket's CORS configuration")

// ErrCORSNotImplemented is a provider that does not take the CORS configuration through the S3
// API; it is set in the provider's own way.
var ErrCORSNotImplemented = errors.New("the storage provider does not implement the bucket CORS call")

// CORSStatusError is any other answer to a CORS call, by status and S3 error code only: the
// body can name the bucket and the key, and neither belongs in a log (T-18).
type CORSStatusError struct {
	Status int
	Code   string
}

func (e *CORSStatusError) Error() string {
	return fmt.Sprintf("the storage endpoint answered %d %s to the CORS call", e.Status, e.Code)
}

// BucketCORS reads the bucket's CORS configuration; a bucket without one answers no rules.
func (s *S3Storage) BucketCORS(ctx context.Context) ([]CORSRule, error) {
	ctx, cancel := context.WithTimeout(ctx, corsTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.bucketCORSURL(), nil)
	if err != nil {
		return nil, ioFailed("building the request", err)
	}
	awssig.Sign(req, s.accessKey, s.secretKey, s.region, "s3", awssig.EmptyPayloadHash, s.now())

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, unreachable(err)
	}
	defer drain(resp)

	body, err := io.ReadAll(io.LimitReader(resp.Body, corsBodyLimit))
	if err != nil {
		return nil, unreachable(err)
	}
	if resp.StatusCode != http.StatusOK {
		code := errorCode(body)
		if resp.StatusCode == http.StatusNotFound && code == "NoSuchCORSConfiguration" {
			return nil, nil
		}
		return nil, corsRefusal(resp.StatusCode, code)
	}
	var config corsConfiguration
	if err := xml.Unmarshal(body, &config); err != nil {
		return nil, ioFailed("reading the CORS configuration", err)
	}
	return config.Rules, nil
}

// PutBucketCORS writes the whole configuration, which replaces what the bucket held. The body is
// small, so its real hash is signed, and Content-MD5 is sent because S3 names it for this call.
func (s *S3Storage) PutBucketCORS(ctx context.Context, rules []CORSRule) error {
	ctx, cancel := context.WithTimeout(ctx, corsTimeout)
	defer cancel()

	body, err := xml.Marshal(corsConfiguration{Xmlns: s3Namespace, Rules: rules})
	if err != nil {
		return ioFailed("writing the CORS configuration", err)
	}
	body = append([]byte(xml.Header), body...)

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, s.bucketCORSURL(), bytes.NewReader(body))
	if err != nil {
		return ioFailed("building the request", err)
	}
	digest := md5.Sum(body) //nolint:gosec // G401: the integrity header the call names, not a security primitive
	req.Header.Set("Content-MD5", base64.StdEncoding.EncodeToString(digest[:]))
	req.Header.Set("Content-Type", "application/xml")
	hash := sha256.Sum256(body)
	awssig.Sign(req, s.accessKey, s.secretKey, s.region, "s3", hex.EncodeToString(hash[:]), s.now())

	resp, err := s.client.Do(req)
	if err != nil {
		return unreachable(err)
	}
	defer drain(resp)

	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusNoContent {
		return nil
	}
	answer, _ := io.ReadAll(io.LimitReader(resp.Body, corsBodyLimit))
	return corsRefusal(resp.StatusCode, errorCode(answer))
}

// bucketCORSURL is the bucket's `?cors` subresource, in the configured addressing style.
func (s *S3Storage) bucketCORSURL() string {
	target := *s.base
	if s.pathStyle {
		target.Path = "/" + s.bucket
	} else {
		target.Host = s.bucket + "." + target.Host
		target.Path = "/"
	}
	target.RawQuery = "cors"
	return target.String()
}

// corsRefusal sorts an answer into the two a caller acts on and the rest.
func corsRefusal(status int, code string) error {
	switch {
	case status == http.StatusNotImplemented || code == "NotImplemented":
		return ErrCORSNotImplemented
	case status == http.StatusForbidden || code == "AccessDenied":
		return ErrCORSForbidden
	default:
		return &CORSStatusError{Status: status, Code: code}
	}
}

// errorCode is the S3 error document's Code, and nothing else of it.
func errorCode(body []byte) string {
	var answer struct {
		Code string `xml:"Code"`
	}
	if xml.Unmarshal(body, &answer) != nil {
		return ""
	}
	return answer.Code
}

// CORSPlan is what an apply would write, worked out from what the bucket holds.
type CORSPlan struct {
	// Rules is the configuration to write; nil when nothing needs writing.
	Rules []CORSRule
	// Kept are the rules of others that stay.
	Kept []CORSRule
	// Removed are the rules of others the write removes; only ever with replace.
	Removed []CORSRule
	// Contradicting are the rules of others that would have to go, found without replace:
	// nothing is written then.
	Contradicting []CORSRule
	// Current is a bucket that already holds Hubtask's rule as it should be.
	Current bool
	// TooMany is a configuration that has no room for one more rule.
	TooMany bool
}

// PlanCORS merges Hubtask's rule into what the bucket holds. Rules of others are kept where they
// answer other requests; one that names an origin of Hubtask's rule and allows PUT answers the
// upload on its own terms, and it goes only with replace. Hubtask's rule goes first, because a
// bucket uses the first rule that matches.
func PlanCORS(existing []CORSRule, ours CORSRule, replace bool) CORSPlan {
	var plan CORSPlan
	mine := 0
	for _, rule := range existing {
		switch {
		case rule.ID == HubtaskRuleID || sameRule(rule, ours):
			mine++
		case contradicts(rule, ours):
			if replace {
				plan.Removed = append(plan.Removed, rule)
			} else {
				plan.Contradicting = append(plan.Contradicting, rule)
			}
		default:
			plan.Kept = append(plan.Kept, rule)
		}
	}
	if len(plan.Contradicting) > 0 {
		plan.Kept = nil
		return plan
	}
	if mine == 1 && len(plan.Removed) == 0 && sameRule(existing[0], ours) {
		plan.Current = true
		return plan
	}
	if len(plan.Kept)+1 > corsRuleLimit {
		plan.TooMany = true
		return plan
	}
	plan.Rules = append([]CORSRule{ours}, plan.Kept...)
	return plan
}

// sameRule compares what a rule allows, not its ID: a rule set by hand from the printed JSON
// carries none and is still Hubtask's.
func sameRule(a, b CORSRule) bool {
	return sameSet(a.AllowedOrigins, b.AllowedOrigins, false) &&
		sameSet(a.AllowedMethods, b.AllowedMethods, true) &&
		sameSet(a.AllowedHeaders, b.AllowedHeaders, true) &&
		len(a.ExposeHeaders) == 0 && a.MaxAgeSeconds == nil && len(a.Other) == 0
}

// contradicts is a rule of others that answers the upload's own requests.
func contradicts(rule, ours CORSRule) bool {
	putAllowed := false
	for _, method := range rule.AllowedMethods {
		if strings.EqualFold(method, uploadMethod) {
			putAllowed = true
		}
	}
	if !putAllowed {
		return false
	}
	for _, origin := range rule.AllowedOrigins {
		for _, mine := range ours.AllowedOrigins {
			if origin == "*" || strings.EqualFold(origin, mine) {
				return true
			}
		}
	}
	return false
}

func sameSet(a, b []string, fold bool) bool {
	if len(a) != len(b) {
		return false
	}
	seen := make(map[string]int, len(a))
	norm := func(v string) string {
		if fold {
			return strings.ToLower(v)
		}
		return v
	}
	for _, v := range a {
		seen[norm(v)]++
	}
	for _, v := range b {
		seen[norm(v)]--
	}
	for _, n := range seen {
		if n != 0 {
			return false
		}
	}
	return true
}
