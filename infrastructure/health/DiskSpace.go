// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package health

import (
	"context"
	"log/slog"
	"strconv"

	port "github.com/Jersyfi/hubtask/core/port/health"
)

const (
	// DiskLowCode is the report's warning for a volume this process writes running out of room:
	// a full disk is how a self-hosted database most often goes down, and a Compose installation
	// has no Prometheus to say so before it happens.
	DiskLowCode = "disk.low"

	// diskWarnBelow and diskCriticalBelow are the free share, in percent, under which the warning
	// is warn and critical - the starting values of the volume alerts.
	diskWarnBelow     = 15
	diskCriticalBelow = 5
)

// Volume is a directory this process writes - the local media, the local backups - by the name the
// warning gives it.
type Volume struct {
	Name string
	Path string
}

// DiskSpace is a warning source: each volume's free share, measured live per report (a statfs is
// cheap). A volume that cannot be measured says nothing rather than something it did not check.
type DiskSpace struct {
	Volumes []Volume
	// measure answers the bytes free to an unprivileged writer and the filesystem's size.
	measure func(path string) (free, total uint64, err error)
}

// NewDiskSpace watches the volumes; one with an empty path is left out.
func NewDiskSpace(volumes ...Volume) DiskSpace {
	watched := make([]Volume, 0, len(volumes))
	for _, volume := range volumes {
		if volume.Path != "" {
			watched = append(watched, volume)
		}
	}
	return DiskSpace{Volumes: watched, measure: freeSpace}
}

// Warnings answers disk.low for every volume below diskWarnBelow percent free.
func (d DiskSpace) Warnings(ctx context.Context) []port.Warning {
	var warnings []port.Warning
	for _, volume := range d.Volumes {
		free, total, err := d.measure(volume.Path)
		if err != nil {
			slog.DebugContext(ctx, "a volume's free space could not be measured",
				slog.String("volume", volume.Name), slog.String("error", err.Error()))
			continue
		}
		if warning, low := diskWarning(volume.Name, free, total); low {
			warnings = append(warnings, warning)
		}
	}
	return warnings
}

// diskWarning is the threshold rule: warn below 15 % free, critical below 5 %.
func diskWarning(volume string, free, total uint64) (port.Warning, bool) {
	if total == 0 {
		return port.Warning{}, false
	}
	percent := float64(free) * 100 / float64(total)
	severity := ""
	switch {
	case percent < diskCriticalBelow:
		severity = "critical"
	case percent < diskWarnBelow:
		severity = "warn"
	default:
		return port.Warning{}, false
	}
	return port.Warning{Code: DiskLowCode, Severity: severity, Params: map[string]string{
		"volume":       volume,
		"free_percent": strconv.FormatFloat(percent, 'f', 1, 64),
		"free_mb":      strconv.FormatUint(free/(1<<20), 10),
	}}, true
}
