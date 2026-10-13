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
	CodeOperationNotFound  InvokeErrorCode = "operation_not_found"
	CodeMissingParam       InvokeErrorCode = "missing_param"
	CodeMethodNotAllowed   InvokeErrorCode = "method_not_allowed"

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

	// The remote answered a write with a redirect, which is not followed: the
	// write would otherwise be sent a second time, to an address nobody chose.
	CodeRedirected InvokeErrorCode = "redirected"

	// The remote answered with something this interpreter cannot read.
	CodeBadResponse   InvokeErrorCode = "bad_response"
	CodeShapeMismatch InvokeErrorCode = "shape_mismatch"
	CodeTooLarge      InvokeErrorCode = "too_large"
)

// InvokeError is the typed envelope every invoker failure carries. Error()
// returns JSON so {code, message, status} survives the Wails boundary intact.
//
// Body is what the remote answered with a failing status, capped at
// maxErrorBody; a refusal's reason usually sits there. notDialed marks a
// transport failure that happened before a connection existed.
type InvokeError struct {
	Code    InvokeErrorCode `json:"code"`
	Message string          `json:"message"`
	Status  int             `json:"status,omitempty"`
	Body    string          `json:"body,omitempty"`
	Cause   error           `json:"-"`

	notDialed bool
}

// maxErrorBody caps how much of a refusal's body is kept.
const maxErrorBody = 64 * 1024

// Delivery says what a caller may conclude about its request from a failure.
// A write must not be repeated blindly on DeliveryUnknown: the remote may have
// acted on it.
const (
	DeliveryNotSent  = "not_sent" // nothing left this machine, or no connection was made
	DeliveryAnswered = "answered" // the remote received it and answered, unhappily
	DeliveryUnknown  = "unknown"  // it may or may not have arrived
)

// deliveryByCode is the outcome each code stands for. CodeUnreachable is the
// one code that depends on the moment of failure; see Delivery.
var deliveryByCode = map[InvokeErrorCode]string{
	CodeConnectionNotFound: DeliveryNotSent,
	CodeResourceNotFound:   DeliveryNotSent,
	CodeBindingInvalid:     DeliveryNotSent,
	CodeUnknownField:       DeliveryNotSent,
	CodeNotConfigured:      DeliveryNotSent,
	CodeOperationNotFound:  DeliveryNotSent,
	CodeMissingParam:       DeliveryNotSent,
	CodeMethodNotAllowed:   DeliveryNotSent,

	CodeUnauthorized:   DeliveryAnswered,
	CodeForbidden:      DeliveryAnswered,
	CodeRemoteNotFound: DeliveryAnswered,
	CodeRateLimited:    DeliveryAnswered,
	CodeRemoteError:    DeliveryAnswered,
	CodeRedirected:     DeliveryAnswered,
	CodeBadResponse:    DeliveryAnswered,
	CodeShapeMismatch:  DeliveryAnswered,
	CodeTooLarge:       DeliveryAnswered,

	CodeTimeout:  DeliveryUnknown,
	CodeCanceled: DeliveryUnknown,
}

// Delivery classifies the failure. An unreachable remote was never sent to
// when the connection itself could not be made; once a connection existed, a
// failure leaves it open whether the request arrived.
func (e *InvokeError) Delivery() string {
	if e == nil {
		return DeliveryUnknown
	}
	if e.Code == CodeUnreachable {
		if e.notDialed {
			return DeliveryNotSent
		}
		return DeliveryUnknown
	}
	if d, ok := deliveryByCode[e.Code]; ok {
		return d
	}
	return DeliveryUnknown
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
