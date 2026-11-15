//go:build linux

package httpapi

import "syscall"

// diskUsage is the total, free and available bytes of the file system of dir.
func diskUsage(dir string) (total, free, avail uint64) {
	var st syscall.Statfs_t
	if syscall.Statfs(dir, &st) != nil && syscall.Statfs(".", &st) != nil {
		return 0, 0, 0
	}
	bs := uint64(st.Bsize)
	return st.Blocks * bs, st.Bfree * bs, st.Bavail * bs
}
