// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	env "github.com/Jersyfi/hubtask/core/port/environment"
	port "github.com/Jersyfi/hubtask/core/port/storage"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// S3Storage speaks the S3 API to AWS and to S3-compatible services (MinIO, Garage).
//
// It carries its own HTTP client rather than going through GuardedClient, and the exception is
// recorded where exceptions live (test/architecture/outbound_test.go): the endpoint is operator
// configuration - the same trust class as the database DSN, not a user-controlled target, so
// T-07's SSRF guard has nothing to guard - it streams objects of up to HUBTASK_MAX_UPLOAD_BYTES
// where the guarded port deliberately buffers small payloads, and a self-hosted MinIO lives on
// exactly the private network the guard exists to block. What rule 6 actually protects is kept:
// every call is bounded by a deadline, redirects are refused, and the resilient wrapper adds the
// breaker and the bulkhead (ADR-0016).
type S3Storage struct {
	client    *http.Client
	base      *url.URL
	bucket    string
	region    string
	accessKey string
	secretKey string
	pathStyle bool
	now       func() time.Time
}

var _ port.ObjectStore = (*S3Storage)(nil)

// s3Dependency is the name the breaker, the metrics and the health probe share.
const s3Dependency = "object_storage"

// NewS3Storage builds the adapter from the validated configuration (the surface has existed
// since A-02; with kind=s3 the bucket and both keys are mandatory at startup).
func NewS3Storage(cfg env.StorageConfig, timeout time.Duration) (*S3Storage, error) {
	endpoint := cfg.Endpoint
	if endpoint == "" {
		// No endpoint means AWS itself; everything else names its own.
		endpoint = "https://s3." + cfg.Region + ".amazonaws.com"
	}
	base, err := url.Parse(endpoint)
	if err != nil || base.Scheme == "" || base.Host == "" {
		return nil, shared.ErrInternal.
			WithDetail("config.s3_incomplete").
			WithCause(fmt.Errorf("the storage endpoint is not an origin"))
	}

	return &S3Storage{
		client: &http.Client{
			// No overall client timeout: a 64 MiB download through a slow link may legitimately
			// outlive any single number chosen here, and every call carries a context deadline
			// from the resilient wrapper (rule 7). The transport bounds the phases that hang.
			Transport: &http.Transport{
				Proxy:                 http.ProxyFromEnvironment,
				DialContext:           (&net.Dialer{Timeout: 5 * time.Second}).DialContext,
				TLSHandshakeTimeout:   5 * time.Second,
				ResponseHeaderTimeout: timeout,
				ExpectContinueTimeout: time.Second,
				MaxIdleConnsPerHost:   4,
				IdleConnTimeout:       90 * time.Second,
			},
			// A redirect is not something a bucket does. Following one would re-send the signed
			// request - credentials included - to wherever the answer pointed.
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return errors.New("the storage endpoint answered with a redirect")
			},
		},
		base:      base,
		bucket:    cfg.Bucket,
		region:    cfg.Region,
		accessKey: cfg.AccessKey.Reveal(),
		secretKey: cfg.SecretKey.Reveal(),
		pathStyle: cfg.UsePathStyle,
		now:       time.Now,
	}, nil
}

// Put writes one object whole, streamed: the body is never buffered here, and the payload
// travels unsigned-content because hashing it would mean reading it twice (the transport is TLS
// in production, which is the integrity layer).
func (s *S3Storage) Put(ctx context.Context, upload port.Upload) error {
	target, err := s.objectURL(upload.Key)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, target, io.NopCloser(upload.Content))
	if err != nil {
		return ioFailed("building the request", err)
	}
	req.ContentLength = upload.Size
	req.Header.Set("Content-Type", upload.ContentType)
	signV4(req, s.accessKey, s.secretKey, s.region, "s3", unsignedPayload, s.now())

	resp, err := s.client.Do(req)
	if err != nil {
		// The guard's refusal travels as itself when the stream refused mid-flight; anything
		// else is the wire's problem.
		if coded := refusalIn(err); coded != nil {
			return coded
		}
		return unreachable(err)
	}
	defer drain(resp)

	if resp.StatusCode == http.StatusOK {
		return nil
	}
	return s.unexpected(resp)
}

// Get returns one object for streaming; the caller closes it.
func (s *S3Storage) Get(ctx context.Context, key string) (port.Object, error) {
	target, err := s.objectURL(key)
	if err != nil {
		return port.Object{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return port.Object{}, ioFailed("building the request", err)
	}
	signV4(req, s.accessKey, s.secretKey, s.region, "s3", emptyPayloadHash, s.now())

	resp, err := s.client.Do(req)
	if err != nil {
		return port.Object{}, unreachable(err)
	}

	switch resp.StatusCode {
	case http.StatusOK:
		size := int64(-1)
		if header := resp.Header.Get("Content-Length"); header != "" {
			if parsed, err := strconv.ParseInt(header, 10, 64); err == nil {
				size = parsed
			}
		}
		return port.Object{
			Content:     resp.Body,
			Size:        size,
			ContentType: resp.Header.Get("Content-Type"),
		}, nil
	case http.StatusNotFound:
		drain(resp)
		return port.Object{}, shared.ErrNotFound
	default:
		defer drain(resp)
		return port.Object{}, s.unexpected(resp)
	}
}

// Delete removes one object. S3 answers 204 for present and absent alike, which is exactly the
// idempotence the port asks for.
func (s *S3Storage) Delete(ctx context.Context, key string) error {
	target, err := s.objectURL(key)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, target, nil)
	if err != nil {
		return ioFailed("building the request", err)
	}
	signV4(req, s.accessKey, s.secretKey, s.region, "s3", emptyPayloadHash, s.now())

	resp, err := s.client.Do(req)
	if err != nil {
		return unreachable(err)
	}
	defer drain(resp)

	switch resp.StatusCode {
	case http.StatusNoContent, http.StatusOK, http.StatusNotFound:
		return nil
	default:
		return s.unexpected(resp)
	}
}

// CreateBucket makes the configured bucket exist, treating "it already does" as the success it
// is. For an operator's first run against a fresh MinIO, and for the conformance suite; AWS
// itself is not the audience - a bucket there is infrastructure somebody provisions, and a name
// taken by another account answers 409 with a different meaning, which still reads as "not
// yours to create" and fails the first upload honestly.
//
// No location constraint body: the S3-compatible services this serves ignore the region, and
// the empty body keeps the request signable with the empty payload hash.
func (s *S3Storage) CreateBucket(ctx context.Context) error {
	target := *s.base
	if s.pathStyle {
		target.Path = "/" + s.bucket
	} else {
		target.Host = s.bucket + "." + target.Host
		target.Path = "/"
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, target.String(), nil)
	if err != nil {
		return ioFailed("building the request", err)
	}
	signV4(req, s.accessKey, s.secretKey, s.region, "s3", emptyPayloadHash, s.now())

	resp, err := s.client.Do(req)
	if err != nil {
		return unreachable(err)
	}
	defer drain(resp)

	switch resp.StatusCode {
	case http.StatusOK, http.StatusConflict:
		return nil
	default:
		return s.unexpected(resp)
	}
}

// objectURL is where the object lives: path-style (the self-hosting default - MinIO without
// wildcard DNS) or virtual-host style.
func (s *S3Storage) objectURL(key string) (string, error) {
	if key == "" || strings.HasPrefix(key, "/") {
		return "", keyInvalid(key)
	}
	for _, segment := range strings.Split(key, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return "", keyInvalid(key)
		}
	}

	target := *s.base
	if s.pathStyle {
		target.Path = "/" + s.bucket + "/" + key
	} else {
		target.Host = s.bucket + "." + target.Host
		target.Path = "/" + key
	}
	return target.String(), nil
}

// unexpected is every answer the protocol did not promise, coded rather than quoted: an S3 error
// body is XML that can carry the key and the bucket, and neither belongs in a log (T-18).
func (s *S3Storage) unexpected(resp *http.Response) error {
	if resp.StatusCode >= 500 {
		return unreachable(fmt.Errorf("the storage endpoint answered %d", resp.StatusCode))
	}
	return shared.ErrUnavailable.
		WithDetail("storage.io_failed").
		WithCause(fmt.Errorf("the storage endpoint answered %d", resp.StatusCode))
}

func unreachable(err error) error {
	return shared.ErrUnavailable.
		WithDetail("dependency.unavailable").
		WithParams(map[string]string{"dependency": s3Dependency}).
		WithCause(err)
}

// refusalIn digs a coded refusal out of the transport's wrapping: the HTTP client wraps a body
// read error in a *url.Error, and the guard's size refusal must come back out as itself.
func refusalIn(err error) error {
	var domainErr *shared.Error
	if errors.As(err, &domainErr) && domainErr.DetailCode != "" {
		return domainErr
	}
	return nil
}

// drain reads the rest of an answer and closes it, so the connection returns to the pool. The
// read is bounded: an error body is small, and one that is not is not worth the connection.
func drain(resp *http.Response) {
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	_ = resp.Body.Close()
}
