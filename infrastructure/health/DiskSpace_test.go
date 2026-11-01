// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package health

import (
	"context"
	"errors"
	"testing"
)

func TestTheDiskWarningThresholds(t *testing.T) {
	for _, c := range []struct {
		name        string
		free, total uint64
		severity    string
	}{
		{"plenty of room", 50, 100, ""},
		{"exactly fifteen percent", 15, 100, ""},
		{"under fifteen percent warns", 14, 100, "warn"},
		{"exactly five percent warns", 5, 100, "warn"},
		{"under five percent is critical", 4, 100, "critical"},
		{"full is critical", 0, 100, "critical"},
		{"a size of zero says nothing", 0, 0, ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			warning, low := diskWarning("media", c.free, c.total)
			if c.severity == "" {
				if low {
					t.Errorf("warned %+v", warning)
				}
				return
			}
			if !low || warning.Code != DiskLowCode || warning.Severity != c.severity || warning.Params["volume"] != "media" {
				t.Errorf("got %+v (%v), want %s at %s", warning, low, DiskLowCode, c.severity)
			}
		})
	}
}

// A volume that cannot be measured is not reported as full or as fine; the others still are.
func TestAVolumeThatCannotBeMeasuredSaysNothing(t *testing.T) {
	disks := DiskSpace{
		Volumes: []Volume{{Name: "media", Path: "/m"}, {Name: "backups", Path: "/b"}},
		measure: func(path string) (uint64, uint64, error) {
			if path == "/m" {
				return 0, 0, errors.New("no such directory")
			}
			return 3, 100, nil
		},
	}
	warnings := disks.Warnings(context.Background())
	if len(warnings) != 1 || warnings[0].Params["volume"] != "backups" || warnings[0].Severity != "critical" {
		t.Errorf("warnings = %+v, want backups critical only", warnings)
	}
}

// The real filesystem answers: the test's own temporary directory is measured without an error.
func TestTheFreeSpaceOfARealDirectoryIsMeasured(t *testing.T) {
	free, total, err := freeSpace(t.TempDir())
	if err != nil {
		t.Skipf("not measured on this platform: %v", err)
	}
	if total == 0 || free > total {
		t.Errorf("free %d of %d", free, total)
	}
}
