package download

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	// Allow private IPs in tests (test servers run on localhost)
	AllowPrivateIPs = true
	os.Exit(m.Run())
}
