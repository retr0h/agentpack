package cmd

import (
	"os"
	"testing"

	"github.com/retr0h/agentpack/pkg/drivers"
)

func TestMain(m *testing.M) {
	drivers.RegisterAll()
	os.Exit(m.Run())
}
