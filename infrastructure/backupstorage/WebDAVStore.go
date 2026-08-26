// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package backupstorage

import (
	"context"
	"encoding/base64"
	"encoding/xml"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	port "github.com/Jersyfi/hubtask/core/port/backupstorage"
	httpport "github.com/Jersyfi/hubtask/core/port/httpclient"
	"github.com/Jersyfi/hubtask/infrastructure/httpclient"
)

// WebDAVStore speaks WebDAV to Nextcloud, ownCloud and to a plain server with the module switched
// on (backup-restore.md §2).
//
// Through GuardedClient, for the reason the S3 adapter is: the address arrives through the API,
// which is what T-07 is about. A NAS on the same LAN therefore needs
// HUBTASK_HTTP_ALLOW_PRIVATE_NETWORKS, which is the same release and the same one-time decision.
//
// Two things about the protocol shape this file. A collection has to exist before something can
// be written into it, and no server creates one on the way - so a write walks the key's segments
// and issues MKCOL for each, treating "already there" as success. And a listing is PROPFIND, which
// this adapter runs at depth one and recurses itself rather than asking for infinite depth: Apache
// refuses infinite depth by default, and a listing that worked against Nextcloud and failed
// against Apache would be the worst kind of adapter.
type WebDAVStore struct {
	client *httpclient.GuardedClient
	// base is the collection this target owns, always ending in a slash.
	base     *url.URL
	username string
	password string
}

var _ port.Store = (*WebDAVStore)(nil)

// maxCollectionDepth bounds the recursion of a listing. A backup layout is a handful of levels
// deep; anything past this is a server answering with something that is not a tree, and a walk
// that followed it would never end.
const maxCollectionDepth = 12

// NewWebDAVStore builds the adapter from a target's configuration and credentials.
func NewWebDAVStore(
	client *httpclient.GuardedClient, spec port.Spec,
) (*WebDAVStore, error) {
	raw := spec.Config.Get("url")
	base, err := url.Parse(raw)
	if err != nil || base.Host == "" || (base.Scheme != "https" && base.Scheme != "http") {
		return nil, configInvalid("url", "backup.url_invalid")
	}
	if !strings.HasSuffix(base.Path, "/") {
		base.Path += "/"
	}
	// A URL carrying a credential is a credential in every log, every metric label and every
	// audit entry that ever names the target (rule 10). They belong in the credential map.
	base.User = nil
	base.RawQuery, base.Fragment = "", ""

	return &WebDAVStore{
		client:   client,
		base:     base,
		username: spec.Config.Get("username"),
		password: spec.Credential("password").Reveal(),
	}, nil
}

// Put writes the archive, creating the collections above it first.
func (s *WebDAVStore) Put(ctx context.Context, key string, content io.Reader) (int64, error) {
	if err := CheckKey(key); err != nil {
		return 0, err
	}
	if err := s.ensureCollections(ctx, key); err != nil {
		return 0, err
	}

	// The length is counted as it goes rather than known in advance, so the request is chunked
	// and the answer is what was actually sent.
	counter := &countingReader{from: content}
	resp, err := s.client.Upload(ctx, s.request(http.MethodPut, key), counter, -1)
	if err != nil {
		return 0, s.transport("writing the archive", err)
	}
	if err := s.expect(resp.Status, key,
		http.StatusOK, http.StatusCreated, http.StatusNoContent); err != nil {
		return 0, err
	}
	return counter.count, nil
}

// Get opens the object as a stream. The caller closes it.
func (s *WebDAVStore) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	if err := CheckKey(key); err != nil {
		return nil, err
	}

	resp, err := s.client.Stream(ctx, s.request(http.MethodGet, key))
	if err != nil {
		return nil, s.transport("reading the archive", err)
	}
	if resp.Status != http.StatusOK && resp.Status != http.StatusPartialContent {
		_ = resp.Body.Close()
		return nil, s.expect(resp.Status, key, http.StatusOK)
	}
	return resp.Body, nil
}

// List walks the collection at depth one, and recurses itself.
func (s *WebDAVStore) List(ctx context.Context, prefix string) ([]port.Entry, error) {
	if err := CheckPrefix(prefix); err != nil {
		return nil, err
	}
	return s.walk(ctx, strings.TrimSuffix(prefix, "/"), 0)
}

func (s *WebDAVStore) walk(ctx context.Context, prefix string, depth int) ([]port.Entry, error) {
	if depth > maxCollectionDepth {
		return nil, nil
	}

	req := s.request("PROPFIND", asCollection(prefix))
	req.Header["Depth"] = []string{"1"}
	req.Header["Content-Type"] = []string{"application/xml; charset=utf-8"}
	req.Body = []byte(propfindBody)

	resp, err := s.client.Do(ctx, req)
	if err != nil {
		return nil, s.transport("listing the target", err)
	}
	switch resp.Status {
	case http.StatusMultiStatus, http.StatusOK:
	case http.StatusNotFound:
		// A collection nothing has been written into yet is an empty listing, not a failure:
		// it is the answer a fresh target gives, and a restore has to be able to ask.
		return nil, nil
	default:
		return nil, s.expect(resp.Status, prefix, http.StatusMultiStatus)
	}

	var answer multistatus
	if err := xml.Unmarshal(resp.Body, &answer); err != nil {
		return nil, failed("reading the listing", err)
	}

	var entries []port.Entry
	for _, response := range answer.Responses {
		key, ok := s.keyOf(response.Href)
		if !ok || key == prefix || key == "" {
			// The collection itself, which PROPFIND always includes.
			continue
		}
		if response.isCollection() {
			below, err := s.walk(ctx, key, depth+1)
			if err != nil {
				return nil, err
			}
			entries = append(entries, below...)
			continue
		}
		entries = append(entries, port.Entry{
			Key: key, Size: response.size(), ModifiedAt: response.modified(),
		})
	}
	return entries, nil
}

// Stat answers the object's size and age without reading it. HEAD rather than PROPFIND: every
// server that serves a file answers it, and the two headers are all that is wanted.
func (s *WebDAVStore) Stat(ctx context.Context, key string) (port.Entry, error) {
	if err := CheckKey(key); err != nil {
		return port.Entry{}, err
	}

	resp, err := s.client.Do(ctx, s.request(http.MethodHead, key))
	if err != nil {
		return port.Entry{}, s.transport("measuring the archive", err)
	}
	if err := s.expect(resp.Status, key, http.StatusOK); err != nil {
		return port.Entry{}, err
	}

	size, _ := strconv.ParseInt(header(resp.Header, "Content-Length"), 10, 64)
	modified, _ := http.ParseTime(header(resp.Header, "Last-Modified"))
	return port.Entry{Key: key, Size: size, ModifiedAt: modified.UTC()}, nil
}

// Delete removes the object. A key that is not there is the state the caller asked for.
func (s *WebDAVStore) Delete(ctx context.Context, key string) error {
	if err := CheckKey(key); err != nil {
		return err
	}

	resp, err := s.client.Do(ctx, s.request(http.MethodDelete, key))
	if err != nil {
		return s.transport("removing the archive", err)
	}
	if resp.Status == http.StatusNotFound {
		return nil
	}
	return s.expect(resp.Status, key, http.StatusOK, http.StatusNoContent, http.StatusAccepted)
}

// ensureCollections creates the directories above a key. MKCOL on one that exists answers 405,
// which is success here: what was asked for is that the collection be there.
func (s *WebDAVStore) ensureCollections(ctx context.Context, key string) error {
	segments := strings.Split(key, "/")
	for index := range len(segments) - 1 {
		collection := strings.Join(segments[:index+1], "/")

		resp, err := s.client.Do(ctx, s.request("MKCOL", asCollection(collection)))
		if err != nil {
			return s.transport("preparing the collection", err)
		}
		switch resp.Status {
		case http.StatusCreated, http.StatusMethodNotAllowed, http.StatusOK:
		case http.StatusConflict:
			// A server that will not create a collection under one that does not exist yet -
			// which cannot happen here, because this walks downwards. Reported rather than
			// swallowed, so a target with an unwritable root says so at the first write.
			return refused("preparing the collection", statusError(resp.Status))
		default:
			return s.expect(resp.Status, collection, http.StatusCreated)
		}
	}
	return nil
}

// asCollection is the trailing slash a collection is addressed by.
//
// Not decoration: Apache answers a request for a collection without one with a 301 to the same
// path with one, and this client follows no redirects - a redirect on a signed or authenticated
// request is how a credential ends up somewhere it was not addressed to (T-07). Asking for the
// form the server would have redirected to is the fix; refusing the redirect stays right.
func asCollection(prefix string) string {
	if prefix == "" || strings.HasSuffix(prefix, "/") {
		return prefix
	}
	return prefix + "/"
}

// request builds one request against a key, with the credential in a header and never in the URL.
func (s *WebDAVStore) request(method, key string) httpport.Request {
	target := *s.base
	target.Path += key

	req := httpport.Request{
		Method: method, URL: target.String(), TargetClass: targetClass,
		Header: map[string][]string{},
	}
	if s.username != "" || s.password != "" {
		credential := base64.StdEncoding.EncodeToString([]byte(s.username + ":" + s.password))
		req.Header["Authorization"] = []string{"Basic " + credential}
	}
	return req
}

// keyOf turns an href from the answer back into a key of ours. A server may answer an absolute
// URL or an absolute path, escaped or not, so this is the one place that is decided.
func (s *WebDAVStore) keyOf(href string) (string, bool) {
	parsed, err := url.Parse(href)
	if err != nil {
		return "", false
	}
	path := parsed.Path
	if path == "" {
		return "", false
	}
	if !strings.HasPrefix(path, s.base.Path) {
		return "", false
	}
	return strings.Trim(strings.TrimPrefix(path, s.base.Path), "/"), true
}

// expect turns a status into the port's vocabulary.
func (s *WebDAVStore) expect(status int, key string, accepted ...int) error {
	for _, want := range accepted {
		if status == want {
			return nil
		}
	}
	switch status {
	case http.StatusNotFound, http.StatusGone:
		return notFound(key)
	case http.StatusUnauthorized, http.StatusForbidden:
		return refused("the target answered", statusError(status))
	default:
		return failed("the target answered", statusError(status))
	}
}

func (s *WebDAVStore) transport(doing string, err error) error {
	if httpclient.IsBlocked(err) {
		return err
	}
	return unreachable(doing, err)
}

// propfindBody asks for the two properties this adapter reads and nothing else. A server that is
// asked for everything answers with everything, and several of them include the file's contents.
const propfindBody = `<?xml version="1.0" encoding="utf-8"?>` +
	`<D:propfind xmlns:D="DAV:"><D:prop>` +
	`<D:getcontentlength/><D:getlastmodified/><D:resourcetype/>` +
	`</D:prop></D:propfind>`

// multistatus is the subset of a PROPFIND answer this adapter reads.
type multistatus struct {
	XMLName   xml.Name      `xml:"multistatus"`
	Responses []davResponse `xml:"response"`
}

type davResponse struct {
	Href     string `xml:"href"`
	Propstat []struct {
		Prop struct {
			ContentLength string `xml:"getcontentlength"`
			LastModified  string `xml:"getlastmodified"`
			ResourceType  struct {
				Collection *struct{} `xml:"collection"`
			} `xml:"resourcetype"`
		} `xml:"prop"`
		Status string `xml:"status"`
	} `xml:"propstat"`
}

func (r davResponse) isCollection() bool {
	for _, stat := range r.Propstat {
		if stat.Prop.ResourceType.Collection != nil {
			return true
		}
	}
	return false
}

func (r davResponse) size() int64 {
	for _, stat := range r.Propstat {
		if value, err := strconv.ParseInt(stat.Prop.ContentLength, 10, 64); err == nil {
			return value
		}
	}
	return 0
}

func (r davResponse) modified() time.Time {
	for _, stat := range r.Propstat {
		if at, err := http.ParseTime(stat.Prop.LastModified); err == nil {
			return at.UTC()
		}
	}
	return time.Time{}
}

// countingReader counts what went past. The archive's length is not known before it is written,
// and the answer to Put is what was actually sent rather than what somebody expected.
type countingReader struct {
	from  io.Reader
	count int64
}

func (r *countingReader) Read(p []byte) (int, error) {
	read, err := r.from.Read(p)
	r.count += int64(read)
	return read, err
}
