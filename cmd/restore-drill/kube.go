// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// kubeClient is the four requests this program makes of the API server, and nothing else.
//
// It carries its own HTTP client rather than going through GuardedClient, and the exception is
// recorded where exceptions live (test/architecture/outbound_test.go): the API server is the
// pod's own control plane, reached through the address and the token the platform injected -
// the same trust class as the database DSN, and an address T-07's guard would refuse on sight,
// because it is a private one by definition. What rule 6 protects is kept: every request carries
// a deadline, redirects are refused, and the body read is bounded.
type kubeClient struct {
	base      string
	token     string
	namespace string
	client    *http.Client
}

// maxResponseBytes bounds what one API answer may be. A Cluster object is a few kilobytes; the
// bound exists so that a misdirected request cannot become a memory problem.
const maxResponseBytes = 4 << 20

func newKubeClient(cfg kubeConfig, namespace string) (*kubeClient, error) {
	base := cfg.APIURL
	if base == "" {
		host, port := os.Getenv("KUBERNETES_SERVICE_HOST"), os.Getenv("KUBERNETES_SERVICE_PORT")
		if host == "" || port == "" {
			return nil, errors.New("KUBERNETES_SERVICE_HOST and _PORT are not set - is this a pod?")
		}
		// Assembled rather than written out: the address is configuration the platform injects,
		// and only the scheme is fixed. A literal here would be an address in the source, which is
		// what PG-6 is about even when the address is the pod's own control plane.
		base = (&url.URL{Scheme: "https", Host: net.JoinHostPort(host, port)}).String()
	}
	parsed, err := url.Parse(base)
	if err != nil || parsed.Host == "" {
		return nil, fmt.Errorf("the API address is not a URL: %w", err)
	}

	token, err := os.ReadFile(cfg.TokenFile)
	if err != nil {
		return nil, fmt.Errorf("the service account token is not readable: %w", err)
	}

	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12}
	if ca, err := os.ReadFile(cfg.CAFile); err == nil {
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(ca) {
			return nil, errors.New("the cluster CA is not a PEM certificate")
		}
		tlsConfig.RootCAs = pool
	} else if parsed.Scheme == "https" {
		// Over TLS the CA is not optional: without it this client would trust whatever answered.
		return nil, fmt.Errorf("the cluster CA is not readable: %w", err)
	}

	return &kubeClient{
		base:      strings.TrimRight(base, "/"),
		token:     strings.TrimSpace(string(token)),
		namespace: namespace,
		client: &http.Client{
			Transport: &http.Transport{
				TLSClientConfig:       tlsConfig,
				DialContext:           (&net.Dialer{Timeout: 5 * time.Second}).DialContext,
				TLSHandshakeTimeout:   5 * time.Second,
				ResponseHeaderTimeout: 30 * time.Second,
				MaxIdleConnsPerHost:   2,
				IdleConnTimeout:       90 * time.Second,
			},
			// An API server does not redirect. Following one would carry the token elsewhere.
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return errors.New("the API server answered with a redirect")
			},
		},
	}, nil
}

// request performs one call and hands back the status and the (bounded) body. A non-2xx status
// is not an error here: the callers decide what a 404 or a 409 means for them.
func (c *kubeClient) request(ctx context.Context, method, path string, body []byte) (int, []byte, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, reader)
	if err != nil {
		return 0, nil, fmt.Errorf("building the request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("%s %s: %w", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	answer, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return resp.StatusCode, nil, fmt.Errorf("reading the answer to %s %s: %w", method, path, err)
	}
	return resp.StatusCode, answer, nil
}

func (c *kubeClient) clustersPath() string {
	return "/apis/postgresql.cnpg.io/v1/namespaces/" + c.namespace + "/clusters"
}

func (c *kubeClient) configMapsPath() string {
	return "/api/v1/namespaces/" + c.namespace + "/configmaps"
}

// createCluster posts the manifest. An existing cluster of that name is an error rather than a
// reuse: the drill tears its own cluster down first, so one that is still there is somebody
// else's or a teardown that did not finish, and neither is a thing to restore into.
func (c *kubeClient) createCluster(ctx context.Context, manifest map[string]any) error {
	body, err := json.Marshal(manifest)
	if err != nil {
		return fmt.Errorf("encoding the cluster: %w", err)
	}
	status, answer, err := c.request(ctx, http.MethodPost, c.clustersPath(), body)
	if err != nil {
		return err
	}
	if status != http.StatusCreated && status != http.StatusOK {
		return apiError("creating the temporary cluster", status, answer)
	}
	return nil
}

// getCluster reads one cluster; found is false on a 404.
func (c *kubeClient) getCluster(ctx context.Context, name string) (bool, map[string]any, error) {
	status, answer, err := c.request(ctx, http.MethodGet, c.clustersPath()+"/"+name, nil)
	if err != nil {
		return false, nil, err
	}
	if status == http.StatusNotFound {
		return false, nil, nil
	}
	if status != http.StatusOK {
		return false, nil, apiError("reading the cluster "+name, status, answer)
	}
	var object map[string]any
	if err := json.Unmarshal(answer, &object); err != nil {
		return false, nil, fmt.Errorf("decoding the cluster %s: %w", name, err)
	}
	return true, object, nil
}

// deleteCluster asks for the deletion; a cluster that is already gone is a success.
func (c *kubeClient) deleteCluster(ctx context.Context, name string) error {
	status, answer, err := c.request(ctx, http.MethodDelete, c.clustersPath()+"/"+name, nil)
	if err != nil {
		return err
	}
	switch status {
	case http.StatusOK, http.StatusAccepted, http.StatusNotFound:
		return nil
	default:
		return apiError("deleting the cluster "+name, status, answer)
	}
}

// mergeConfigMap writes the given keys into the ConfigMap, creating it when it does not exist and
// keeping every other key it already holds. A replace rather than a patch, because a strategic
// merge patch is a content type this program would otherwise need for exactly one call.
func (c *kubeClient) mergeConfigMap(ctx context.Context, name string, data map[string]string) error {
	status, answer, err := c.request(ctx, http.MethodGet, c.configMapsPath()+"/"+name, nil)
	if err != nil {
		return err
	}
	switch status {
	case http.StatusNotFound:
		body, err := json.Marshal(map[string]any{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata":   map[string]any{"name": name, "namespace": c.namespace},
			"data":       data,
		})
		if err != nil {
			return fmt.Errorf("encoding the record: %w", err)
		}
		created, reply, err := c.request(ctx, http.MethodPost, c.configMapsPath(), body)
		if err != nil {
			return err
		}
		if created != http.StatusCreated {
			return apiError("creating the record "+name, created, reply)
		}
		return nil
	case http.StatusOK:
		var existing map[string]any
		if err := json.Unmarshal(answer, &existing); err != nil {
			return fmt.Errorf("decoding the record %s: %w", name, err)
		}
		merged := map[string]any{}
		if current, ok := existing["data"].(map[string]any); ok {
			for key, value := range current {
				merged[key] = value
			}
		}
		for key, value := range data {
			merged[key] = value
		}
		existing["data"] = merged
		body, err := json.Marshal(existing)
		if err != nil {
			return fmt.Errorf("encoding the record: %w", err)
		}
		updated, reply, err := c.request(ctx, http.MethodPut, c.configMapsPath()+"/"+name, body)
		if err != nil {
			return err
		}
		if updated != http.StatusOK {
			return apiError("updating the record "+name, updated, reply)
		}
		return nil
	default:
		return apiError("reading the record "+name, status, answer)
	}
}

// apiError carries the server's status message, which names the field it refused - and never
// the token, which travels in a header the server does not echo.
func apiError(doing string, status int, answer []byte) error {
	var reason struct {
		Message string `json:"message"`
	}
	_ = json.Unmarshal(answer, &reason)
	if reason.Message == "" {
		reason.Message = http.StatusText(status)
	}
	return fmt.Errorf("%s: the API server answered %d: %s", doing, status, reason.Message)
}
