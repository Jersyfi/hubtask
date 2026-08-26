// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package backupstorage

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	port "github.com/Jersyfi/hubtask/core/port/backupstorage"
	httpport "github.com/Jersyfi/hubtask/core/port/httpclient"
	"github.com/Jersyfi/hubtask/infrastructure/awssig"
	"github.com/Jersyfi/hubtask/infrastructure/httpclient"
)

// targetClass is the label every backup call carries in the outbound metric. One value for all
// four adapters and every target: a label per host would grow a series per configured bucket,
// which is exactly the cardinality rule 10 bounds.
const targetClass = "backup_target"

// partSize is how much of an archive is held at once, and it is the whole memory story of this
// adapter.
//
// An archive has no known length before it is written - it is compressed and encrypted as it goes
// - and S3 needs a Content-Length per request. The two are reconciled with a multipart upload:
// each part is buffered, sent, and forgotten, so the process holds one part rather than an
// archive. Eight mebibytes because five is S3's floor for every part but the last, and a floor
// with no headroom is a protocol error waiting for a rounding difference.
//
// A spill to a temporary file would be the other answer, and is worse: a container's writable
// layer is small, and an archive that does not fit in it fails at the target rather than at the
// disk, which is a much later and much more confusing failure.
const PartSize = 8 << 20

// partSize is the internal spelling; PartSize is exported so that a test can produce an archive
// of exactly the size that makes the multipart path the one under test.
const partSize = PartSize

// maxKeysPerPage bounds a listing page. Small deliberately: the guarded client caps how much of a
// response it will read (HUBTASK_HTTP_MAX_RESPONSE_BYTES, one mebibyte by default), and a page
// that overran it would be truncated XML rather than an error.
const maxKeysPerPage = 200

// S3Store speaks the S3 API to AWS and to every S3-compatible service the roadmap names - MinIO,
// Ceph, Wasabi, Backblaze B2, Hetzner, IDrive e2 (backup-restore.md §2).
//
// Unlike the media object store, this one goes through GuardedClient, and the difference is who
// chose the endpoint. The media endpoint is operator configuration, the trust class of the
// database DSN; a backup target's endpoint arrives through the API, from an instance
// administrator or - where the operator has switched it on - from a tenant. That is a
// user-controlled outbound destination, which is precisely what T-07 is about, and backup-restore
// .md §2 says so in as many words: the probe runs through the same GuardedClient, with metadata
// endpoints and private ranges blocked unless explicitly released.
//
// The practical consequence is worth stating plainly, because somebody will hit it: a MinIO on
// the same private network is refused until HUBTASK_HTTP_ALLOW_PRIVATE_NETWORKS is set. That is
// the release the acceptance asks for, and it is a decision an operator makes once rather than a
// hole every target gets for free.
type S3Store struct {
	client *httpclient.GuardedClient
	// base is the endpoint origin, already parsed.
	base   *url.URL
	bucket string
	region string
	// prefix is the target's own directory inside the bucket, normalised to either empty or
	// something ending in a slash.
	prefix    string
	accessKey string
	secretKey string
	pathStyle bool
	now       func() time.Time
}

var _ port.Store = (*S3Store)(nil)

// NewS3Store builds the adapter from a target's configuration and credentials.
func NewS3Store(
	client *httpclient.GuardedClient, spec port.Spec, now func() time.Time,
) (*S3Store, error) {
	region := spec.Config.Get("region")
	if region == "" {
		region = "us-east-1"
	}

	endpoint := spec.Config.Get("endpoint")
	if endpoint == "" {
		// No endpoint means AWS itself; every compatible service names its own.
		endpoint = "https://s3." + region + ".amazonaws.com"
	}
	base, err := url.Parse(endpoint)
	if err != nil || base.Host == "" {
		return nil, configInvalid("endpoint", "backup.endpoint_invalid")
	}
	if base.Scheme != "https" && base.Scheme != "http" {
		return nil, configInvalid("endpoint", "backup.endpoint_invalid")
	}

	prefix := strings.Trim(spec.Config.Get("path"), "/")
	if prefix != "" {
		if err := CheckPrefix(prefix); err != nil {
			return nil, err
		}
		prefix += "/"
	}

	return &S3Store{
		client:    client,
		base:      base,
		bucket:    spec.Config.Get("bucket"),
		region:    region,
		prefix:    prefix,
		accessKey: spec.Credential("access_key").Reveal(),
		secretKey: spec.Credential("secret_key").Reveal(),
		pathStyle: spec.Config.Get("use_path_style") != "false",
		now:       now,
	}, nil
}

// Put writes the archive, in one request when it fits in a part and as a multipart upload when it
// does not.
//
// The single-request path is not an optimisation for its own sake: a multipart upload leaves a
// half-finished upload behind if the process dies between the first part and the completion, and
// a bucket accumulating those costs money quietly. Most archives that are not a full backup fit
// in one part.
func (s *S3Store) Put(ctx context.Context, key string, content io.Reader) (int64, error) {
	if err := CheckKey(key); err != nil {
		return 0, err
	}

	// One part plus a byte: enough to tell "this fits in one request" from "this does not"
	// without a second read.
	first := make([]byte, partSize+1)
	read, err := io.ReadFull(content, first)
	switch {
	case errors.Is(err, io.EOF), errors.Is(err, io.ErrUnexpectedEOF):
		return s.putWhole(ctx, key, first[:read])
	case err != nil:
		return 0, failed("reading the archive", err)
	}
	return s.putInParts(ctx, key, first, content)
}

// putWhole is the one-request path.
func (s *S3Store) putWhole(ctx context.Context, key string, body []byte) (int64, error) {
	target, err := s.objectURL(key)
	if err != nil {
		return 0, err
	}

	resp, err := s.signed(ctx, http.MethodPut, target, body, awssig.UnsignedPayload)
	if err != nil {
		return 0, err
	}
	if err := s.expect(resp.Status, http.StatusOK); err != nil {
		return 0, err
	}
	return int64(len(body)), nil
}

// putInParts is the multipart path: begin, send parts of a bounded size, complete. A failure
// anywhere aborts the upload, so the bucket is not left holding parts nobody will finish - which
// is storage the operator pays for and never sees.
func (s *S3Store) putInParts(
	ctx context.Context, key string, first []byte, rest io.Reader,
) (int64, error) {
	target, err := s.objectURL(key)
	if err != nil {
		return 0, err
	}

	uploadID, err := s.beginUpload(ctx, target)
	if err != nil {
		return 0, err
	}
	completed := false
	defer func() {
		if completed {
			return
		}
		// Best effort, and on a context of its own: the caller's may well be the reason we are
		// here, and an upload nobody aborts is storage nobody meant to buy.
		abortCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
		defer cancel()
		_ = s.abortUpload(abortCtx, target, uploadID)
	}()

	var (
		written int64
		parts   []completedPart
		part    = first
		number  = 1
	)
	for {
		etag, err := s.putPart(ctx, target, uploadID, number, part)
		if err != nil {
			return 0, err
		}
		parts = append(parts, completedPart{PartNumber: number, ETag: etag})
		written += int64(len(part))
		number++

		// Every part but the last has to reach S3's floor, so a short read is filled rather than
		// sent: io.ReadFull returns what it got and says why it stopped, and only a zero-length
		// read ends the upload.
		next := make([]byte, partSize)
		read, err := io.ReadFull(rest, next)
		if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
			return 0, failed("reading the archive", err)
		}
		if read == 0 {
			break
		}
		part = next[:read]
	}

	if err := s.completeUpload(ctx, target, uploadID, parts); err != nil {
		return 0, err
	}
	completed = true
	return written, nil
}

// Get opens the object as a stream. The caller closes it.
//
// Stream rather than Do, because an archive is unbounded by design and the guarded client's
// response cap exists for payloads that are not. Everything the guard actually protects still
// applies to a stream: the URL check, the resolve before the connect, the dial-time control and
// the redirect re-check.
func (s *S3Store) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	if err := CheckKey(key); err != nil {
		return nil, err
	}
	target, err := s.objectURL(key)
	if err != nil {
		return nil, err
	}

	req := httpport.Request{Method: http.MethodGet, URL: target, TargetClass: targetClass}
	s.sign(&req, awssig.EmptyPayloadHash)

	resp, err := s.client.Stream(ctx, req)
	if err != nil {
		return nil, s.transport("reading the archive", err)
	}
	switch resp.Status {
	case http.StatusOK, http.StatusPartialContent:
		return resp.Body, nil
	case http.StatusNotFound:
		_ = resp.Body.Close()
		return nil, notFound(key)
	case http.StatusForbidden, http.StatusUnauthorized:
		_ = resp.Body.Close()
		return nil, refused("reading the archive", statusError(resp.Status))
	default:
		_ = resp.Body.Close()
		return nil, failed("reading the archive", statusError(resp.Status))
	}
}

// List walks the prefix, page by page. The keys come back whole - the target's own prefix
// removed, everything else kept - so a caller can hand one straight back to Get.
func (s *S3Store) List(ctx context.Context, prefix string) ([]port.Entry, error) {
	if err := CheckPrefix(prefix); err != nil {
		return nil, err
	}

	var entries []port.Entry
	token := ""
	for {
		query := url.Values{
			"list-type": {"2"},
			"prefix":    {s.prefix + strings.TrimSuffix(prefix, "/")},
			"max-keys":  {strconv.Itoa(maxKeysPerPage)},
		}
		if token != "" {
			query.Set("continuation-token", token)
		}

		target, err := s.bucketURL(query)
		if err != nil {
			return nil, err
		}
		resp, err := s.signed(ctx, http.MethodGet, target, nil, awssig.EmptyPayloadHash)
		if err != nil {
			return nil, err
		}
		if err := s.expect(resp.Status, http.StatusOK); err != nil {
			return nil, err
		}

		var page listBucketResult
		if err := xml.Unmarshal(resp.Body, &page); err != nil {
			return nil, failed("reading the listing", err)
		}
		for _, object := range page.Contents {
			key := strings.TrimPrefix(object.Key, s.prefix)
			if key == "" || strings.HasSuffix(key, "/") {
				// A zero-length key ending in a slash is how several services represent a
				// directory. It is not an archive.
				continue
			}
			entries = append(entries, port.Entry{
				Key: key, Size: object.Size, ModifiedAt: object.LastModified.UTC(),
			})
		}

		if !page.IsTruncated || page.NextContinuationToken == "" {
			return entries, nil
		}
		token = page.NextContinuationToken
	}
}

// Stat answers the object's size and age without reading it.
func (s *S3Store) Stat(ctx context.Context, key string) (port.Entry, error) {
	if err := CheckKey(key); err != nil {
		return port.Entry{}, err
	}
	target, err := s.objectURL(key)
	if err != nil {
		return port.Entry{}, err
	}

	resp, err := s.signed(ctx, http.MethodHead, target, nil, awssig.EmptyPayloadHash)
	if err != nil {
		return port.Entry{}, err
	}
	switch resp.Status {
	case http.StatusOK:
	case http.StatusNotFound:
		return port.Entry{}, notFound(key)
	case http.StatusForbidden, http.StatusUnauthorized:
		return port.Entry{}, refused("measuring the archive", statusError(resp.Status))
	default:
		return port.Entry{}, failed("measuring the archive", statusError(resp.Status))
	}

	size, _ := strconv.ParseInt(header(resp.Header, "Content-Length"), 10, 64)
	modified, _ := http.ParseTime(header(resp.Header, "Last-Modified"))
	return port.Entry{Key: key, Size: size, ModifiedAt: modified.UTC()}, nil
}

// Delete removes the object. S3 answers 204 whether or not the key was there, which is exactly
// the port's contract: deletion is the state the caller asked for.
func (s *S3Store) Delete(ctx context.Context, key string) error {
	if err := CheckKey(key); err != nil {
		return err
	}
	target, err := s.objectURL(key)
	if err != nil {
		return err
	}

	resp, err := s.signed(ctx, http.MethodDelete, target, nil, awssig.EmptyPayloadHash)
	if err != nil {
		return err
	}
	if resp.Status == http.StatusNotFound {
		return nil
	}
	return s.expect(resp.Status, http.StatusNoContent, http.StatusOK)
}

// The three multipart steps. Their answers are XML documents small enough for the guarded
// client's response cap by construction: an upload identifier, an entity tag, and a completion.

type initiateMultipartUploadResult struct {
	UploadID string `xml:"UploadId"`
}

type completedPart struct {
	PartNumber int    `xml:"PartNumber"`
	ETag       string `xml:"ETag"`
}

type completeMultipartUpload struct {
	XMLName xml.Name        `xml:"CompleteMultipartUpload"`
	Parts   []completedPart `xml:"Part"`
}

func (s *S3Store) beginUpload(ctx context.Context, target string) (string, error) {
	resp, err := s.signed(ctx, http.MethodPost, target+"?uploads=", nil, awssig.EmptyPayloadHash)
	if err != nil {
		return "", err
	}
	if err := s.expect(resp.Status, http.StatusOK); err != nil {
		return "", err
	}

	var started initiateMultipartUploadResult
	if err := xml.Unmarshal(resp.Body, &started); err != nil || started.UploadID == "" {
		return "", failed("beginning the upload", err)
	}
	return started.UploadID, nil
}

func (s *S3Store) putPart(
	ctx context.Context, target, uploadID string, number int, part []byte,
) (string, error) {
	query := "?partNumber=" + strconv.Itoa(number) + "&uploadId=" + url.QueryEscape(uploadID)

	resp, err := s.signed(ctx, http.MethodPut, target+query, part, awssig.UnsignedPayload)
	if err != nil {
		return "", err
	}
	if err := s.expect(resp.Status, http.StatusOK); err != nil {
		return "", err
	}
	// The entity tag is what the completion names each part by. Quoted in the header and quoted
	// again in the completion document, so it travels exactly as it came.
	return header(resp.Header, "ETag"), nil
}

func (s *S3Store) completeUpload(
	ctx context.Context, target, uploadID string, parts []completedPart,
) error {
	body, err := xml.Marshal(completeMultipartUpload{Parts: parts})
	if err != nil {
		return failed("completing the upload", err)
	}

	query := "?uploadId=" + url.QueryEscape(uploadID)
	resp, err := s.signed(ctx, http.MethodPost, target+query, body, awssig.UnsignedPayload)
	if err != nil {
		return err
	}
	if err := s.expect(resp.Status, http.StatusOK); err != nil {
		return err
	}
	// S3 answers 200 and then reports the failure inside the document, which is the one place a
	// status code is not the answer. An Error element means the object is not there.
	if strings.Contains(string(resp.Body), "<Error") {
		return failed("completing the upload", statusError(resp.Status))
	}
	return nil
}

func (s *S3Store) abortUpload(ctx context.Context, target, uploadID string) error {
	query := "?uploadId=" + url.QueryEscape(uploadID)
	_, err := s.signed(ctx, http.MethodDelete, target+query, nil, awssig.EmptyPayloadHash)
	return err
}

// listBucketResult is the subset of ListObjectsV2 this adapter reads.
type listBucketResult struct {
	XMLName               xml.Name `xml:"ListBucketResult"`
	IsTruncated           bool     `xml:"IsTruncated"`
	NextContinuationToken string   `xml:"NextContinuationToken"`
	Contents              []struct {
		Key          string    `xml:"Key"`
		Size         int64     `xml:"Size"`
		LastModified time.Time `xml:"LastModified"`
	} `xml:"Contents"`
}

// signed makes one signed request through the guarded client.
func (s *S3Store) signed(
	ctx context.Context, method, target string, body []byte, payloadHash string,
) (httpport.Response, error) {
	req := httpport.Request{
		Method: method, URL: target, Body: body, TargetClass: targetClass,
	}
	s.sign(&req, payloadHash)

	resp, err := s.client.Do(ctx, req)
	if err != nil {
		return httpport.Response{}, s.transport("reaching the target", err)
	}
	return resp, nil
}

// sign puts the three headers on the request. It builds a throwaway http.Request because that is
// what the signer canonicalises, and the port's Request is a map of headers rather than one.
func (s *S3Store) sign(req *httpport.Request, payloadHash string) {
	//nolint:noctx // not a request that is sent: it exists so the signer can canonicalise it
	signing, err := http.NewRequest(req.Method, req.URL, nil)
	if err != nil {
		return
	}
	awssig.Sign(signing, s.accessKey, s.secretKey, s.region, "s3", payloadHash, s.now())

	req.Header = map[string][]string{
		"x-amz-date":           {signing.Header.Get("x-amz-date")},
		"x-amz-content-sha256": {signing.Header.Get("x-amz-content-sha256")},
		"Authorization":        {signing.Header.Get("Authorization")},
	}
}

// objectURL is where one key lives.
func (s *S3Store) objectURL(key string) (string, error) {
	return s.address(s.prefix+key, nil)
}

// bucketURL is the bucket itself, for a listing.
func (s *S3Store) bucketURL(query url.Values) (string, error) {
	return s.address("", query)
}

// address assembles the endpoint, the bucket and the key in whichever of the two styles the
// target uses. Path style is the default here, because every self-hosted S3 service speaks it and
// several speak nothing else.
func (s *S3Store) address(key string, query url.Values) (string, error) {
	if s.bucket == "" {
		return "", configInvalid("bucket", "backup.config_required")
	}

	target := *s.base
	if s.pathStyle {
		target.Path = "/" + s.bucket + "/" + key
	} else {
		target.Host = s.bucket + "." + target.Host
		target.Path = "/" + key
	}
	// The path is set rather than escaped here, and the signer canonicalises it: two escapings
	// would produce a signature over a path the server never sees.
	target.Path = strings.TrimSuffix(target.Path, "/")
	if key == "" {
		target.Path = "/" + s.bucket
		if !s.pathStyle {
			target.Path = "/"
		}
	}
	if query != nil {
		target.RawQuery = query.Encode()
	}
	return target.String(), nil
}

// expect turns a status into the port's vocabulary.
func (s *S3Store) expect(status int, accepted ...int) error {
	for _, want := range accepted {
		if status == want {
			return nil
		}
	}
	switch status {
	case http.StatusNotFound:
		return shared.ErrNotFound.WithDetail(port.CodeObjectNotFound)
	case http.StatusForbidden, http.StatusUnauthorized:
		return refused("the target answered", statusError(status))
	default:
		return failed("the target answered", statusError(status))
	}
}

// transport separates "the guard said no" from "the target did not answer". A blocked address is
// a configuration mistake the caller can fix; a timeout is not.
func (s *S3Store) transport(doing string, err error) error {
	if httpclient.IsBlocked(err) {
		return err
	}
	return unreachable(doing, err)
}

// statusError is a status as an error, with no body in it. An S3 error document repeats the
// bucket and the key, and a key names a tenant and a moment (rule 10).
func statusError(status int) error {
	return fmt.Errorf("the target answered %d", status)
}

// header reads one header case-insensitively, which the port's map is not.
func header(headers map[string][]string, name string) string {
	for key, values := range headers {
		if strings.EqualFold(key, name) && len(values) > 0 {
			return values[0]
		}
	}
	return ""
}

// configInvalid is a target whose configuration cannot be turned into a connection.
func configInvalid(setting, code string) error {
	return shared.ErrValidation.
		WithDetail("backup.target_invalid").
		WithFields(shared.FieldError{
			Path: "/config/" + setting, Code: code,
			Params: map[string]string{"setting": setting},
		})
}

// unreachable is a target that did not answer at all.
func unreachable(doing string, err error) error {
	return shared.ErrUnavailable.
		WithDetail(port.CodeTargetUnreachable).
		WithCause(fmt.Errorf("%s: %w", doing, err))
}
