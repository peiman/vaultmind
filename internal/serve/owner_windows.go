//go:build windows

package serve

// checkPrivateDir: the server is not offered on Windows.
func checkPrivateDir(string) error { return nil }
