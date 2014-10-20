package httpbakery

import (
	"net/http"

	"github.com/juju/errgo"
	"github.com/juju/utils/jsonhttp"

	"github.com/rogpeppe/macaroon"
)

// ErrorCode holds an error code that classifies
// an error returned from a bakery HTTP handler.
type ErrorCode string

func (e ErrorCode) Error() string {
	return string(e)
}

func (e ErrorCode) ErrorCode() ErrorCode {
	return e
}

const (
	ErrBadRequest          = ErrorCode("bad request")
	ErrDischargeRequired   = ErrorCode("macaroon discharge required")
	ErrInteractionRequired = ErrorCode("interaction required")
)

var (
	handleJSON   = jsonhttp.HandleJSON(errorToResponse)
	handleErrors = jsonhttp.HandleErrors(errorToResponse)
	writeError   = jsonhttp.WriteError(errorToResponse)
)

// Error holds the type of a response from an httpbakery HTTP request,
// marshaled as JSON.
type Error struct {
	Code    ErrorCode  `json:",omitempty"`
	Message string     `json:",omitempty"`
	Info    *ErrorInfo `json:",omitempty"`
}

// ErrorInfo holds additional information provided
// by an error.
type ErrorInfo struct {
	// Macaroon may hold a macaroon that, when
	// discharged, may allow access to a service.
	// This field is associated with the ErrDischargeRequired
	// error code.
	Macaroon *macaroon.Macaroon `json:",omitempty"`

	// VisitURL and WaitURL are associated with the
	// ErrInteractionRequired error code.

	// VisitURL holds a URL that the client should visit
	// in a web browser to authenticate themselves.
	VisitURL string `json:",omitempty"`

	// WaitURL holds a URL that the client should visit
	// to acquire the discharge macaroon. A GET on
	// this URL will block until the client has authenticated,
	// and then it will return the discharge macaroon.
	WaitURL string `json:",omitempty"`
}

func (e *Error) Error() string {
	return e.Message
}

func (e *Error) ErrorCode() ErrorCode {
	return e.Code
}

// ErrorInfo returns additional information
// about the error.
// TODO return interface{} here?
func (e *Error) ErrorInfo() *ErrorInfo {
	return e.Info
}

func errorToResponse(err error) (int, interface{}) {
	errorBody := errorResponseBody(err)
	status := http.StatusInternalServerError
	switch errorBody.Code {
	case ErrBadRequest:
		status = http.StatusBadRequest
	case ErrDischargeRequired, ErrInteractionRequired:
		status = http.StatusProxyAuthRequired
	}
	return status, errorBody
}

type errorInfoer interface {
	ErrorInfo() *ErrorInfo
}

type errorCoder interface {
	ErrorCode() ErrorCode
}

// errorResponse returns an appropriate error
// response for the provided error.
func errorResponseBody(err error) *Error {
	errResp := &Error{
		Message: err.Error(),
	}
	cause := errgo.Cause(err)
	if coder, ok := cause.(errorCoder); ok {
		errResp.Code = coder.ErrorCode()
	}
	if infoer, ok := cause.(errorInfoer); ok {
		errResp.Info = infoer.ErrorInfo()
	}
	return errResp
}

func badRequestErrorf(f string, a ...interface{}) error {
	return errgo.WithCausef(nil, ErrBadRequest, f, a...)
}
