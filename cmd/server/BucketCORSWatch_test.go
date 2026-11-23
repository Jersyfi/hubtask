// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"testing"

	envport "github.com/Jersyfi/hubtask/core/port/environment"
	"github.com/Jersyfi/hubtask/core/shared/secret"
	storageadapter "github.com/Jersyfi/hubtask/infrastructure/storage"
)

// deployment.md §6.1: the watch runs only with s3 storage and a base URL to ask with.
func TestTheBucketCORSWatchRunsOnlyWithObjectStorage(t *testing.T) {
	local := envport.Config{BaseURL: "https://tasks.example.org",
		Storage: envport.StorageConfig{Kind: envport.StorageLocal, LocalPath: t.TempDir()}}
	if watch := bucketCORSWatch(local, nil); watch != nil {
		t.Errorf("a watch with local storage")
	}

	s3cfg := envport.StorageConfig{Kind: envport.StorageS3, Endpoint: "http://127.0.0.1:1", Bucket: "media",
		Region: "us-east-1", AccessKey: secret.New("a"), SecretKey: secret.New("b"), UsePathStyle: true}
	store, err := storageadapter.NewS3Storage(s3cfg, 0)
	if err != nil {
		t.Fatal(err)
	}
	if watch := bucketCORSWatch(envport.Config{Storage: s3cfg}, store); watch != nil {
		t.Errorf("a watch without a base URL")
	}
	if watch := bucketCORSWatch(envport.Config{BaseURL: "https://tasks.example.org", Storage: s3cfg}, store); watch == nil {
		t.Errorf("no watch with s3 storage")
	}
}
