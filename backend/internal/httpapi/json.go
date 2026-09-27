package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
)

type ErrorDetail struct {
	Code    string `json:"code" example:"INTERNAL_ERROR" binding:"required"`
	Message string `json:"message" example:"An unexpected error occurred." binding:"required"`
}

type ErrorResponse struct {
	Error ErrorDetail `json:"error" binding:"required"`
}

func JSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		slog.Warn("write JSON response failed", "error", err)
	}
}

func Error(w http.ResponseWriter, status int, code, message string) {
	JSON(w, status, ErrorResponse{Error: ErrorDetail{Code: code, Message: message}})
}

// Decode requires one JSON value and limits the entire body, including trailing data.
func Decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		Error(w, 415, "UNSUPPORTED_MEDIA_TYPE", "Use Content-Type: application/json.")
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	err = decoder.Decode(dst)
	if err == nil {
		var extra any
		err = decoder.Decode(&extra)
		if errors.Is(err, io.EOF) {
			return true
		}
	}
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		Error(w, 413, "PAYLOAD_TOO_LARGE", "Request body exceeds 16 KiB.")
	} else {
		Error(w, 400, "VALIDATION_ERROR", "Provide one valid JSON object with the expected fields.")
	}
	return false
}
