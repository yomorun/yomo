// Package yerr describes yomo errors
package yerr

import (
	"fmt"

	"github.com/quic-go/quic-go"
)

// YomoError yomo error
type YomoError interface {
	error
	// ErrorCode getter method
	ErrorCode() ErrorCode
}

type yomoError struct {
	errorCode ErrorCode
	err       error
}

// New create yomo error
func New(code ErrorCode, err error) YomoError {
	return &yomoError{
		errorCode: code,
		err:       err,
	}
}

// Error is the built-in error interface
func (e *yomoError) Error() string {
	return fmt.Sprintf("%s error: message=%s", e.errorCode, e.err.Error())
}

// ErrorCode getter method
func (e *yomoError) ErrorCode() ErrorCode {
	return e.errorCode
}

// ErrorCode error code
type ErrorCode uint64

const (
	// ErrorCodeClientAbort client abort
	ErrorCodeClientAbort ErrorCode = 0xC7
	// ErrorCodeUnknown unknown error
	ErrorCodeUnknown ErrorCode = 0xC0
	// ErrorCodeClosed net closed
	ErrorCodeClosed ErrorCode = 0xC1
	// ErrorCodeBeforeHandler befor handler
	ErrorCodeBeforeHandler ErrorCode = 0xC2
	// ErrorCodeMainHandler main handler
	ErrorCodeMainHandler ErrorCode = 0xC3
	// ErrorCodeAfterHandler after handler
	ErrorCodeAfterHandler ErrorCode = 0xC4
	// ErrorCodeHandshake handshake frame
	ErrorCodeHandshake ErrorCode = 0xC5
	// ErrorCodeRejected server rejected
	ErrorCodeRejected ErrorCode = 0xCC
	// ErrorCodeGoaway goaway frame
	ErrorCodeGoaway ErrorCode = 0xCF
	// ErrorCodeData data frame
	ErrorCodeData ErrorCode = 0xCE
	// ErrorCodeUnknownClient unknown client error
	ErrorCodeUnknownClient ErrorCode = 0xCD
	// ErrorCodeDuplicateName unknown client error
	ErrorCodeDuplicateName ErrorCode = 0xC6
	// ErrorCodeStartHandler start handler
	ErrorCodeStartHandler ErrorCode = 0xC8
)

var errCodeStringMap = map[ErrorCode]string{
	ErrorCodeClientAbort:   "ClientAbort",
	ErrorCodeUnknown:       "UnknownError",
	ErrorCodeClosed:        "NetClosed",
	ErrorCodeBeforeHandler: "BeforeHandler",
	ErrorCodeMainHandler:   "MainHandler",
	ErrorCodeAfterHandler:  "AfterHandler",
	ErrorCodeHandshake:     "Handshake",
	ErrorCodeRejected:      "Rejected",
	ErrorCodeGoaway:        "Goaway",
	ErrorCodeData:          "DataFrame",
	ErrorCodeUnknownClient: "UnknownClient",
	ErrorCodeDuplicateName: "DuplicateName",
	ErrorCodeStartHandler:  "StartHandler",
}

func (e ErrorCode) String() string {
	msg, ok := errCodeStringMap[e]
	if !ok {
		return "XXX"
	}
	return msg
}

// Is parse quic ApplicationErrorCode to yomo ErrorCode
func Is(qerr quic.ApplicationErrorCode, yerr ErrorCode) bool {
	return uint64(qerr) == uint64(yerr)
}

// Parse parse quic ApplicationErrorCode
func Parse(qerr quic.ApplicationErrorCode) ErrorCode {
	return ErrorCode(qerr)
}

// To convert yomo ErrorCode to quic ApplicationErrorCode
func (e ErrorCode) To() quic.ApplicationErrorCode {
	return quic.ApplicationErrorCode(e)
}

// DuplicateNameError duplicate name(sfn)
type DuplicateNameError struct {
	streamID string
	err      error
}

// NewDuplicateNameError create a duplicate name error
func NewDuplicateNameError(streamID string, err error) DuplicateNameError {
	return DuplicateNameError{
		streamID: streamID,
		err:      err,
	}
}

// Error raw error
func (e DuplicateNameError) Error() string {
	return e.err.Error()
}

// ErrorCode getter method
func (e DuplicateNameError) ErrorCode() ErrorCode {
	return ErrorCodeDuplicateName
}

// StreamID duplicate stream ID
func (e DuplicateNameError) StreamID() string {
	return e.streamID
}
