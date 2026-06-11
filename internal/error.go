package internal

import "fmt"

type ErrorDetail struct {
	Reason string `json:"reason"`
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
	return fmt.Sprintf("api %d %s: %s", a.ErrorData.Code, a.ErrorData.Errors[0].Reason, a.ErrorData.Message)
}
