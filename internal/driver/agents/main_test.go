package agents_test

import (
	"os"
	"testing"

	"github.com/retr0h/agentpack/internal/driver/agents"
)

func TestMain(m *testing.M) {
	agents.RegisterAll()
	os.Exit(m.Run())
}
