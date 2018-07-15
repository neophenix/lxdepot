// Package utils is meant to be a collection of functions that could be useful elsewhere
package utils

import "fmt"

// MakeBytesMoreHuman takes in a uint64 value that is meant to be something in bytes, like
// memory usage, disk usage, etc.  It returns a string converted to and having the appropriate
// suffix: GB, MB, KB, B
func MakeBytesMoreHuman(bytes uint64) string {
	switch {
	case bytes >= 1000000000:
		return fmt.Sprintf("%v GB", bytes/1073741824)
	case bytes >= 1000000:
		return fmt.Sprintf("%v MB", bytes/1048576)
	case bytes >= 1000:
		return fmt.Sprintf("%v KB", bytes/1024)
	}

	return fmt.Sprintf("%v B", bytes)
}

// MakeIntBytesMoreHuman same as MakeBytesMoreHuman but takes in an int64, mostly because
// the lxd client returns byte values in differenting bit sizes /shrug
func MakeIntBytesMoreHuman(bytes int64) string {
	return MakeBytesMoreHuman(uint64(bytes))
}
