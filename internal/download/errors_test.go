package download

import (
	"errors"
	"testing"
)

func TestDownloadErrorError(t *testing.T) {
	err := NewDownloadError(ErrNetwork, "connection timeout", nil)
	want := "network: connection timeout"
	if err.Error() != want {
		t.Errorf("got %q, want %q", err.Error(), want)
	}
}

func TestDownloadErrorWithWrapped(t *testing.T) {
	inner := errors.New("dial tcp: timeout")
	err := NewDownloadError(ErrNetwork, "failed to connect", inner)
	want := "network: failed to connect: dial tcp: timeout"
	if err.Error() != want {
		t.Errorf("got %q, want %q", err.Error(), want)
	}
}

func TestDownloadErrorUnwrap(t *testing.T) {
	inner := errors.New("underlying error")
	err := NewDownloadError(ErrFileSystem, "write failed", inner)
	if !errors.Is(err, inner) {
		t.Error("errors.Is should match the wrapped error")
	}
}

func TestDownloadErrorAs(t *testing.T) {
	err := NewDownloadError(ErrValidation, "invalid URL", nil)
	var dlErr *DownloadError
	if !errors.As(err, &dlErr) {
		t.Fatal("errors.As should match *DownloadError")
	}
	if dlErr.Kind != ErrValidation {
		t.Errorf("kind = %v, want ErrValidation", dlErr.Kind)
	}
}

func TestErrorKindString(t *testing.T) {
	tests := []struct {
		kind ErrorKind
		want string
	}{
		{ErrValidation, "validation"},
		{ErrNetwork, "network"},
		{ErrFileSystem, "filesystem"},
		{ErrExternal, "external"},
		{ErrCancelled, "cancelled"},
		{ErrorKind(99), "unknown"},
	}
	for _, tt := range tests {
		if got := tt.kind.String(); got != tt.want {
			t.Errorf("ErrorKind(%d).String() = %q, want %q", tt.kind, got, tt.want)
		}
	}
}
