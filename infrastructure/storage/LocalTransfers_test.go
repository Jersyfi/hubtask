// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package storage

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/media"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	port "github.com/Jersyfi/hubtask/core/port/storage"
	"github.com/Jersyfi/hubtask/core/shared/secret"
	"github.com/Jersyfi/hubtask/infrastructure/security"
)

// A target is minted at the host the request arrived on - a workspace's own origin - so the page
// asking for it may fetch it under connect-src/img-src 'self' without CORS; the installation's
// address is the fallback for a call that came from no request.
func TestLocalTransfersMintAtTheRequestsHost(t *testing.T) {
	issuer := NewLocalTransfers(
		security.NewMediaTokenIssuer(secret.New("an installation secret of decent length")),
		"http://localhost:8087/",
	)
	object := media.Object{
		ID:       shared.MustParseID("01900000-0000-7000-8000-000000000001"),
		TenantID: shared.MustParseID("01900000-0000-7000-8000-000000000002"),
	}
	expires := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)

	cases := []struct {
		name string
		ctx  context.Context
		want string
	}{
		{"no request", context.Background(), "http://localhost:8087/api/v1/media/"},
		{
			"a workspace's own host",
			port.ContextWithRequestHost(context.Background(), "nordlicht.localhost:8087"),
			"http://nordlicht.localhost:8087/api/v1/media/",
		},
		{
			"a host that is no host keeps the installation's",
			port.ContextWithRequestHost(context.Background(), "evil.example/x?"),
			"http://localhost:8087/api/v1/media/",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			down, err := issuer.IssueDownload(tc.ctx, object, expires)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.HasPrefix(down.URL, tc.want) {
				t.Fatalf("download %q, want prefix %q", down.URL, tc.want)
			}
			up, err := issuer.IssueUpload(tc.ctx, object, expires)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.HasPrefix(up.URL, tc.want) {
				t.Fatalf("upload %q, want prefix %q", up.URL, tc.want)
			}
		})
	}
}
