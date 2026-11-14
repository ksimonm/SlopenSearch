package main

import (
	"os"
	"syscall"
)

// fdatasync flushes a file's data (and the metadata needed to read it back).
func fdatasync(f *os.File) error { return syscall.Fdatasync(int(f.Fd())) }
