//go:build linux || darwin

package releasekey

// The containing directory and key file use 0700/0600. Maintainer deployments
// requiring hardware-backed keys should replace this offline signing workflow.
func protect(b []byte) ([]byte, error)   { return append([]byte{}, b...), nil }
func unprotect(b []byte) ([]byte, error) { return append([]byte{}, b...), nil }
