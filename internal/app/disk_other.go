//go:build !unix

package app

// Unknown capacity pauses uploads rather than risking a full server disk.
func diskSpace(string) (int64, int64) { return 0, 0 }
