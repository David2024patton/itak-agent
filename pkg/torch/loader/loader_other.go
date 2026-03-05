//go:build !windows

package loader

// setDllSearchPath is a no-op on non-Windows platforms.
// On Linux/macOS, dlopen resolves dependencies from RPATH, LD_LIBRARY_PATH,
// or already-loaded shared objects in the process.
func setDllSearchPath(dir string) {
	// No-op on Linux/macOS.
}
