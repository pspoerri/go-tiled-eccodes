//go:build unix

package mmap

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

// Open mmaps the entire file read-only and returns the byte slice. The caller
// must call Close(data) to release the mapping. The returned slice is backed
// by an OS-managed mapping and must not be appended to or modified.
func Open(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	size := st.Size()
	if size == 0 {
		return nil, fmt.Errorf("mmap: %s is empty", path)
	}
	if size != int64(int(size)) {
		return nil, fmt.Errorf("mmap: file too large for address space")
	}
	data, err := unix.Mmap(int(f.Fd()), 0, int(size), unix.PROT_READ, unix.MAP_SHARED)
	if err != nil {
		return nil, fmt.Errorf("mmap: %w", err)
	}
	// Hint sequential read pattern for the indexer pass; the OS may upgrade
	// to random later when the renderer touches arbitrary pages.
	_ = unix.Madvise(data, unix.MADV_SEQUENTIAL)
	return data, nil
}

func Close(data []byte) error {
	if data == nil {
		return nil
	}
	return unix.Munmap(data)
}

// AdviseRandom tells the kernel that subsequent accesses to this slice will
// be non-sequential — useful once the indexer is finished and the renderer
// starts hitting arbitrary tiles.
func AdviseRandom(data []byte) {
	if data != nil {
		_ = unix.Madvise(data, unix.MADV_RANDOM)
	}
}
