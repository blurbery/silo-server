//go:build linux

package playback

import (
	"io/fs"
	"os"
	"syscall"
	"testing"
	"time"
)

func TestDirectPlayFilesystemRevisionIgnoresMetadataOnlyChanges(t *testing.T) {
	base := linuxRevisionFileInfo{
		stat: syscall.Stat_t{
			Dev:  11,
			Ino:  42,
			Ctim: syscall.Timespec{Sec: 100, Nsec: 200},
			Mode: 0o100600,
			Uid:  1000,
			Gid:  1000,
		},
		modTime: time.Unix(300, 400),
		size:    4096,
	}
	metadataOnly := base
	metadataOnly.stat.Ctim = syscall.Timespec{Sec: 500, Nsec: 600}
	metadataOnly.stat.Mode = 0o100640
	metadataOnly.stat.Uid = 2000
	metadataOnly.stat.Gid = 2000

	want, ok := directPlayFilesystemRevision(nil, base)
	if !ok {
		t.Fatal("base revision unavailable")
	}
	got, ok := directPlayFilesystemRevision(nil, metadataOnly)
	if !ok {
		t.Fatal("metadata-only revision unavailable")
	}
	if got != want {
		t.Fatalf("metadata-only update changed revision: got %q, want %q", got, want)
	}
}

func TestDirectPlayFilesystemRevisionTracksContentIdentity(t *testing.T) {
	base := linuxRevisionFileInfo{
		stat:    syscall.Stat_t{Dev: 11, Ino: 42},
		modTime: time.Unix(300, 400),
		size:    4096,
	}
	want, ok := directPlayFilesystemRevision(nil, base)
	if !ok {
		t.Fatal("base revision unavailable")
	}

	tests := []struct {
		name string
		info linuxRevisionFileInfo
	}{
		{name: "device", info: func() linuxRevisionFileInfo { changed := base; changed.stat.Dev++; return changed }()},
		{name: "inode", info: func() linuxRevisionFileInfo { changed := base; changed.stat.Ino++; return changed }()},
		{name: "mtime", info: func() linuxRevisionFileInfo {
			changed := base
			changed.modTime = changed.modTime.Add(time.Nanosecond)
			return changed
		}()},
		{name: "size", info: func() linuxRevisionFileInfo { changed := base; changed.size++; return changed }()},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := directPlayFilesystemRevision(nil, test.info)
			if !ok {
				t.Fatal("changed revision unavailable")
			}
			if got == want {
				t.Fatalf("%s change did not invalidate revision %q", test.name, got)
			}
		})
	}
}

type linuxRevisionFileInfo struct {
	stat    syscall.Stat_t
	modTime time.Time
	size    int64
}

func (info linuxRevisionFileInfo) Name() string       { return "fixture.mkv" }
func (info linuxRevisionFileInfo) Size() int64        { return info.size }
func (info linuxRevisionFileInfo) Mode() fs.FileMode  { return os.FileMode(info.stat.Mode) }
func (info linuxRevisionFileInfo) ModTime() time.Time { return info.modTime }
func (info linuxRevisionFileInfo) IsDir() bool        { return false }
func (info linuxRevisionFileInfo) Sys() any           { return &info.stat }
