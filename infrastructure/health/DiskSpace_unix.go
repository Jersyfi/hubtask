// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build linux || darwin

package health

import (
	"errors"
	"syscall"
)

// freeSpace asks the filesystem: Bavail rather than Bfree, because the reserve kept for root is
// not room this process can use.
func freeSpace(path string) (free, total uint64, err error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, 0, err
	}
	if stat.Bsize <= 0 {
		return 0, 0, errors.New("the filesystem reported no block size")
	}
	size := uint64(stat.Bsize) //nolint:gosec // G115: positive, checked on the line above
	return stat.Bavail * size, stat.Blocks * size, nil
}
