package download

import "fmt"

// ErrorKind classifies download errors.
type ErrorKind int

const (
	ErrValidation ErrorKind = iota
	ErrNetwork
	ErrFileSystem
	ErrExternal // yt-dlp, ffmpeg
	ErrCancelled
)

func (k ErrorKind) String() string {
	switch k {
	case ErrValidation:
		return "validation"
	case ErrNetwork:
		return "network"
	case ErrFileSystem:
		return "filesystem"
	case ErrExternal:
		return "external"
	case ErrCancelled:
		return "cancelled"
	default:
		return "unknown"
	}
}

// DownloadError is a structured error type for download operations.
type DownloadError struct {
	Kind    ErrorKind
	Message string
	Err     error
}

func (e *DownloadError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %s: %v", e.Kind, e.Message, e.Err)
	}
	return fmt.Sprintf("%s: %s", e.Kind, e.Message)
}

func (e *DownloadError) Unwrap() error {
	return e.Err
}

// NewDownloadError creates a new DownloadError.
func NewDownloadError(kind ErrorKind, message string, err error) *DownloadError {
	return &DownloadError{Kind: kind, Message: message, Err: err}
}
