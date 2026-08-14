//go:build linux

package playback

import (
	"fmt"
	"os"
	"syscall"
)

func directPlayFilesystemRevision(_ *os.File, info os.FileInfo) (string, bool) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return "", false
	}
	// ctime changes for metadata-only operations such as chmod and chown even
	// though the bytes served by this endpoint are unchanged. Including it in
	// the validator makes an active range stream look like a replaced entity and
	// forces clients to discard an otherwise valid buffer. Device + inode still
	// detects a replacement at the same path, while mtime + size detect ordinary
	// in-place content updates.
	return fmt.Sprintf(
		"linux-content-v2:%x:%x:%x:%x",
		uint64(stat.Dev),
		stat.Ino,
		info.ModTime().UnixNano(),
		info.Size(),
	), true
}
