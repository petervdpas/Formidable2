package connection

import "encoding/json"

// InvokeErrorCode is the stable key for a remote call failure. The frontend
// maps these to messages through an explicit literal table; never build a
// translation key by interpolating a code into a template string.
type InvokeErrorCode string

const (
	// Configuration: the call never left the machine.
	CodeConnectionNotFound InvokeErrorCode = "connection_not_found"
	CodeResourceNotFound   InvokeErrorCode = "resource_not_found"
	CodeBindingInvalid     InvokeErrorCode = "binding_invalid"
	CodeUnknownField       InvokeErrorCode = "unknown_field"
	CodeNotConfigured      InvokeErrorCode = "not_configured"

	// Transport: the call went out and did not come back usable.
	CodeUnreachable InvokeErrorCode = "unreachable"
	CodeTimeout     InvokeErrorCode = "timeout"
	CodeCanceled    InvokeErrorCode = "canceled"

	// The remote answered, unhappily.
	CodeUnauthorized   InvokeErrorCode = "unauthorized"
	CodeForbidden      InvokeErrorCode = "forbidden"
	CodeRemoteNotFound InvokeErrorCode = "remote_not_found"
	CodeRateLimited    InvokeErrorCode = "rate_limited"
	CodeRemoteError    InvokeErrorCode = "remote_error"

	// The remote answered with something this interpreter cannot read.
	CodeBadResponse   InvokeErrorCode = "bad_response"
	CodeShapeMismatch InvokeErrorCode = "shape_mismatch"
	CodeTooLarge      InvokeErrorCode = "too_large"
)

// InvokeError is the typed envelope every invoker failure carries. Error()
// returns JSON so {code, message, status} survives the Wails boundary intact.
type InvokeError struct {
	Code    InvokeErrorCode `json:"code"`
	Message string          `json:"message"`
	Status  int             `json:"status,omitempty"`
	Cause   error           `json:"-"`
}

func (e *InvokeError) Error() string {
	b, err := json.Marshal(e)
	if err != nil {
		return string(e.Code) + ": " + e.Message
	}
	return string(b)
}

func (e *InvokeError) Unwrap() error { return e.Cause }

func invokeErr(code InvokeErrorCode, msg string, cause error) *InvokeError {
	return &InvokeError{Code: code, Message: msg, Cause: cause}
}

// statusCode maps an HTTP status onto the taxonomy. Anything below 400 is not
// a failure and never reaches here.
func statusCode(status int) InvokeErrorCode {
	switch status {
	case 401:
		return CodeUnauthorized
	case 403:
		return CodeForbidden
	case 404:
		return CodeRemoteNotFound
	case 429:
		return CodeRateLimited
	default:
		return CodeRemoteError
	}
}
