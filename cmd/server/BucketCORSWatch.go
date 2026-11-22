// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	envport "github.com/Jersyfi/hubtask/core/port/environment"
	storageport "github.com/Jersyfi/hubtask/core/port/storage"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	storageadapter "github.com/Jersyfi/hubtask/infrastructure/storage"
)

// bucketCORSWatch is the health report's watch on the bucket's CORS rule: only with s3 storage
// and a base URL to ask with.
func bucketCORSWatch(cfg envport.Config, transfers storageport.TransferIssuer) *storageadapter.CORSWatch {
	if cfg.Storage.Kind != envport.StorageS3 {
		return nil
	}
	store, ok := transfers.(*storageadapter.S3Storage)
	if !ok {
		return nil
	}
	origins := storageadapter.OriginsFor(cfg.BaseURL, cfg.Tenancy, cfg.WorkspaceHosts)
	if len(origins.Check) == 0 {
		return nil
	}
	return storageadapter.NewCORSWatch(store, origins.Check, clockadapter.System{})
}
