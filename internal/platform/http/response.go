package httpserver

import "github.com/gin-gonic/gin"

type ErrorDetail struct {
	Field string `json:"field"`
	Code  string `json:"code"`
}

type errorResponse struct {
	Error errorBody `json:"error"`
}

type errorBody struct {
	Code      string        `json:"code"`
	Message   string        `json:"message"`
	Details   []ErrorDetail `json:"details,omitempty"`
	RequestID string        `json:"request_id"`
}

// WriteError aborts the handler chain and writes a safe public error.
// Code, message, and details must be chosen by the handler, never copied from raw errors or input.
func WriteError(c *gin.Context, status int, code, message string, details ...ErrorDetail) {
	c.Abort()
	if c.Writer.Written() {
		return
	}
	c.Header("Content-Type", "application/json; charset=utf-8")
	c.Header("Cache-Control", "no-store")
	c.JSON(status, errorResponse{Error: errorBody{
		Code:      code,
		Message:   message,
		Details:   details,
		RequestID: c.GetString("request_id"),
	}})
}
