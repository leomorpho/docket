package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	temp := filepath.Join(os.TempDir(), fmt.Sprintf("docket-test-home-%d", time.Now().UnixNano()))
	if err := os.MkdirAll(temp, 0755); err != nil {
		panic(fmt.Sprintf("failed to create DOCKET_HOME test dir: %v", err))
	}
	if err := os.Setenv("DOCKET_HOME", temp); err != nil {
		panic(fmt.Sprintf("failed to set DOCKET_HOME: %v", err))
	}
	os.Exit(m.Run())
}
