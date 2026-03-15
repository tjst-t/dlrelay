package model

import "encoding/json"

// ConvertRequest is the request body for POST /api/convert.
type ConvertRequest struct {
	Args    []string          `json:"args"`
	Options map[string]string `json:"options,omitempty"`
}

// ConvertState represents the current state of a conversion task.
type ConvertState string

const (
	ConvertStateRunning   ConvertState = "running"
	ConvertStateCompleted ConvertState = "completed"
	ConvertStateFailed    ConvertState = "failed"
	ConvertStateCancelled ConvertState = "cancelled"
)

// ConvertStatus is the response body for GET /api/convert/:id.
type ConvertStatus struct {
	ID       string       `json:"id"`
	State    ConvertState `json:"state"`
	Progress float64      `json:"progress"`
	Error    *string      `json:"error"`
}

// ProbeRequest is the request body for POST /api/probe.
type ProbeRequest struct {
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers,omitempty"`
}

// ProbeResult is the response body for POST /api/probe.
type ProbeResult struct {
	Format  json.RawMessage `json:"format"`
	Streams json.RawMessage `json:"streams"`
}

// Codec represents an FFmpeg codec.
type Codec struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Type        string `json:"type"` // "video", "audio", "subtitle"
	CanDecode   bool   `json:"can_decode"`
	CanEncode   bool   `json:"can_encode"`
}

// Format represents an FFmpeg format.
type Format struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	CanDemux    bool   `json:"can_demux"`
	CanMux      bool   `json:"can_mux"`
}

// ErrorResponse is a standard error response.
type ErrorResponse struct {
	Error string `json:"error"`
}
