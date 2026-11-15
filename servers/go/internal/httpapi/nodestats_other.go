//go:build !linux

package httpapi

// diskUsage is unknown outside Linux.
func diskUsage(dir string) (total, free, avail uint64) { return 0, 0, 0 }
