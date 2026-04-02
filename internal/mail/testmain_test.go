package mail

import (
	"os"
	"testing"

	"github.com/colbymchenry/devpit/internal/testutil"
)

func TestMain(m *testing.M) {
	code := m.Run()
	testutil.TerminateDoltContainer()
	os.Exit(code)
}
