// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build linux || darwin

package backupstorage

import (
	"context"
	"errors"
	"math"
	"syscall"
)

// FreeBytes asks the filesystem how much room is left for an unprivileged writer.
//
// Bavail rather than Bfree, deliberately: the difference between the two is the reserve the
// filesystem keeps for root, and this process is not root. Reporting space it cannot use would
// make "there is room" and "the backup fits" two different statements.
//
// Two platforms rather than every unix, because `Statfs_t` is not the same struct on all of them
// and a build that compiles against a guess is worse than one that says it cannot answer. Linux
// is what the released images run (support-matrix.md); darwin is where this is written.
func (s LocalStore) FreeBytes(_ context.Context) (int64, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(s.root, &stat); err != nil {
		return 0, failed("measuring the free space", err)
	}

	blockSize := blockSizeOf(stat)
	if blockSize <= 0 {
		return 0, failed("measuring the free space",
			errors.New("the filesystem reported no block size"))
	}
	// A filesystem that would overflow the answer is not a thing anybody is backing up to, and a
	// negative number would be worse than a saturated one.
	if stat.Bavail > uint64(math.MaxInt64)/uint64(blockSize) {
		return math.MaxInt64, nil
	}
	// The branch above proves the product fits, and therefore that each operand does.
	return int64(stat.Bavail) * blockSize, nil //nolint:gosec // G115: bounded on the line above
}
