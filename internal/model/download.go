package model

// DownloadRequest is the request body for POST /api/downloads.
type DownloadRequest struct {
	URL         string            `json:"url"`
	AudioURL    string            `json:"audio_url,omitempty"`
	FallbackURL string            `json:"fallback_url,omitempty"` // Used when primary method (e.g. yt-dlp) fails
	Headers     map[string]string `json:"headers,omitempty"`
	Filename    string            `json:"filename"`
	Directory   string            `json:"directory,omitempty"`
	Method      string            `json:"method,omitempty"`  // "ytdlp" to use yt-dlp
	Quality     string            `json:"quality,omitempty"` // yt-dlp format selector (e.g. "bestvideo+bestaudio/best")
	PageURL     string            `json:"page_url,omitempty"`
}

// DownloadState represents the current state of a download task.
type DownloadState string

const (
	StateQueued      DownloadState = "queued"
	StateDownloading DownloadState = "downloading"
	StateCompleted   DownloadState = "completed"
	StateFailed      DownloadState = "failed"
	StateCancelled   DownloadState = "cancelled"
)

// DownloadStatus is the response body for GET /api/downloads/:id.
type DownloadStatus struct {
	ID            string        `json:"id"`
	URL           string        `json:"url"`
	PageURL       string        `json:"page_url,omitempty"`
	State         DownloadState `json:"state"`
	BytesReceived int64         `json:"bytes_received"`
	TotalBytes    int64         `json:"total_bytes"`
	Filename      string        `json:"filename"`
	HasFile       bool          `json:"has_file,omitempty"`
	FilePath      string        `json:"-"`
	Error         *string       `json:"error"`
}
