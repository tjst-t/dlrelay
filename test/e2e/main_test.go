package e2e

import (
	"os"
	"testing"

	"github.com/tjst-t/dlrelay/internal/download"
)

func TestMain(m *testing.M) {
	// Allow private IPs in tests (test servers run on localhost)
	download.AllowPrivateIPs = true
	os.Exit(m.Run())
}
