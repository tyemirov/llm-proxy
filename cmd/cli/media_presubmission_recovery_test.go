//go:build pilot_presubmission_recovery

package main

import (
	"bytes"
	"testing"
)

func TestMediaPresubmissionRecoveryCommandExists(t *testing.T) {
	var output bytes.Buffer
	rootCmd.SetOut(&output)
	rootCmd.SetErr(&output)
	t.Cleanup(func() { rootCmd.SetOut(nil); rootCmd.SetErr(nil) })
	if err := executeRootCommand(t, "recover-media-presubmission", "--help"); err != nil {
		t.Fatalf("recovery command absent: %v", err)
	}
	if !bytes.Contains(output.Bytes(), []byte("finalize")) {
		t.Fatalf("missing recovery phases: %s", &output)
	}
}
