package yerr

import (
	"fmt"

	"github.com/lucas-clemente/quic-go"
)

// YomoError yomo error
type YomoError struct {
	errorCode ErrorCode
	err       error
}

// New create yomo error
func New(code ErrorCode, err error) *YomoError {
	return &YomoError{
		errorCode: code,
		err:       err,
	}
}

func (e *YomoError) Error() string {
	return fmt.Sprintf("%s error: message=%s", e.errorCode, e.err.Error())
}

// ErrorCode error code
type ErrorCode uint64

const (
	// ErrorCodeClientAbort client abort
	ErrorCodeClientAbort ErrorCode = 0x00
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
)

func (e ErrorCode) String() string {
	switch e {
	case ErrorCodeClientAbort:
		return "ClientAbort"
	case ErrorCodeUnknown:
		return "UnknownError"
	case ErrorCodeClosed:
		return "NetClosed"
	case ErrorCodeBeforeHandler:
		return "BeforeHandler"
	case ErrorCodeMainHandler:
		return "MainHandler"
	case ErrorCodeAfterHandler:
		return "AfterHandler"
	case ErrorCodeHandshake:
		return "Handshake"
	case ErrorCodeRejected:
		return "Rejected"
	case ErrorCodeGoaway:
		return "Goaway"
	case ErrorCodeData:
		return "DataFrame"
	case ErrorCodeUnknownClient:
		return "UnknownClient"
	default:
		return "XXX"
	}
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
func To(code ErrorCode) quic.ApplicationErrorCode {
	return quic.ApplicationErrorCode(code)
}
