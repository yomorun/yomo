package core

import "fmt"

// Version is the current yomo spec version.
// if the spec version is changed, the client maybe cannot work well with server.
const Version = "2024-01-03"

// DefaultVersionNegotiateFunc is default version negotiate function.
// if cVersion != sVersion, return error and respond RejectedFrame.
func DefaultVersionNegotiateFunc(cVersion, sVersion string) error {
	if cVersion != sVersion {
		return &ErrRejected{
			Message: fmt.Sprintf("version negotiation failed: client=%s, server=%s", cVersion, sVersion),
		}
	}
	return nil
}

// VersionNegotiateFunc is the version negotiate function.
// Use `ConfigVersionNegotiateFunc` to set it.
// If you want to connect to a new server, the function should return ErrConnectTo error.
// If you want reject the connection, the function should return ErrRejected error.
type VersionNegotiateFunc func(cVersion string, sVersion string) error

// ErrConnectTo is returned by VersionNegotiateFunc if you want to connect to a new server.
type ErrConnectTo struct {
	Endpoint string
}

// Error implements the error interface.
func (e *ErrConnectTo) Error() string {
	return fmt.Sprintf("connect to %s", e.Endpoint)
}

// ErrRejected is returned by VersionNegotiateFunc if you want to reject the connection.
type ErrRejected struct {
	Message string
}

// Error implements the error interface.
func (e *ErrRejected) Error() string {
	return e.Message
}
