package cmd

import (
	"os"
	"testing"

	"github.com/retr0h/agentpack/internal/driver"
)

func TestMain(m *testing.M) {
	driver.RegisterAll()
	os.Exit(m.Run())
}
