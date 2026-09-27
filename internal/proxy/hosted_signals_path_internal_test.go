//go:build darwin || linux

package proxy

import (
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestHostedSignalsRejectUnresolvableWorkingDirectory(t *testing.T) {
	fixture := newFundsStartupFixture(t, startupCompleteUsage)
	databasePath := hostedSignalsDatabasePath(t, fixture.database)
	before := fixture.state(t)
	baseline := readHostedSignalsFixture(t, fixture.database)
	workingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(t.TempDir())
	var absolute HostedFinancialSignals
	func() {
		var directories []string
		defer func() {
			for index := len(directories) - 1; index >= 0; index-- {
				if err := os.Chdir(".."); err != nil {
					t.Error(err)
					return
				}
				if err := os.Remove(directories[index]); err != nil {
					t.Error(err)
				}
			}
		}()
		// Relative operations can reach a directory beyond the operating
		// system path buffer and Go's bounded parent-directory walk.
		for range 400 {
			directory, err := os.MkdirTemp(".", "signal-path-")
			if err != nil {
				t.Fatal(err)
			}
			if err := os.Chdir(directory); err != nil {
				_ = os.Remove(directory)
				t.Fatal(err)
			}
			directories = append(directories, directory)
		}
		report, err := ReadHostedFinancialSignals(t.Context(), "relative.sqlite")
		if err == nil || !strings.Contains(err.Error(), "resolve financial database path") || !reflect.DeepEqual(report, HostedFinancialSignals{}) {
			t.Fatalf("unresolvable directory returned report=%+v error=%v", report, err)
		}
		absolute, err = ReadHostedFinancialSignals(t.Context(), databasePath)
		if err != nil {
			t.Fatal(err)
		}
	}()
	t.Chdir(workingDirectory)
	absolute.ObservedAt = baseline.ObservedAt
	if !reflect.DeepEqual(baseline, absolute) || !reflect.DeepEqual(before, fixture.state(t)) || fixture.calls.Load() != 1 {
		t.Fatal("path resolution failure changed the financial report or provider work")
	}
}
