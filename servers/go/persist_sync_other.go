//go:build !linux

package main

import "os"

func fdatasync(f *os.File) error { return f.Sync() }
