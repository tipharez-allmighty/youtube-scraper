package youtube

import "fmt"

type Reason string

const (
	CommentsDisabled Reason = "commentsDisabled"
	QuotaExceeded    Reason = "quotaExceeded"
	VideoNotFound    Reason = "videoNotFound"
	UnknownReason    Reason = "unknown_reason"
)

type ErrorDetail struct {
	Reason Reason `json:"reason"`
}

type ErrorPayload struct {
	Code    int           `json:"code"`
	Message string        `json:"message"`
	Errors  []ErrorDetail `json:"errors"`
}

type APIError struct {
	ErrorData *ErrorPayload `json:"error"`
}

func (a APIError) Error() string {
	if a.ErrorData == nil {
		return "unknown youtube api error"
	}
	return fmt.Sprintf("api %d %s: %s", a.ErrorData.Code, a.Reason(), a.ErrorData.Message)
}

func (a APIError) Reason() Reason {
	reason := UnknownReason
	if len(a.ErrorData.Errors) > 0 {
		reason = a.ErrorData.Errors[0].Reason
	}
	return reason
}
