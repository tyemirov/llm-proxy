package tests_test

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestOperationalInstalledGateway(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	application := filepath.Join(root, "application")
	if err := os.MkdirAll(application, 0o755); err != nil {
		t.Fatal(err)
	}
	makefile, err := os.ReadFile(filepath.Join(operationalRepositoryRoot(t), "Makefile"))
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(application, "Makefile"), makefile, 0o644); err != nil {
		t.Fatal(err)
	}
	initialize := exec.Command("git", "init", application)
	if output, err := initialize.CombinedOutput(); err != nil {
		t.Fatalf("initialize application: %v: %s", err, output)
	}
	source := filepath.Join(root, "gateway.go")
	executable := filepath.Join(root, "installed gateway")
	program := `package main
import("encoding/json";"fmt";"os";"strconv")
func main(){
 file,err:=os.OpenFile(os.Getenv("GATEWAY_FIXTURE_LOG"),os.O_CREATE|os.O_WRONLY|os.O_APPEND,0644)
 if err!=nil{panic(err)}
 if err=json.NewEncoder(file).Encode(map[string]any{"arguments":os.Args[1:],"operator_root":os.Getenv("MPRLAB_GATEWAY_OPERATOR_ROOT")});err!=nil{panic(err)}
 if err=file.Close();err!=nil{panic(err)}
 if status:=os.Getenv("GATEWAY_FIXTURE_EXIT");status!=""{code,_:=strconv.Atoi(status);fmt.Fprintln(os.Stderr,"native Gateway failure");os.Exit(code)}
}`
	if err = os.WriteFile(source, []byte(program), 0o644); err != nil {
		t.Fatal(err)
	}
	build := exec.Command("go", "build", "-o", executable, source)
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build host boundary fixture: %v: %s", err, output)
	}
	logfile := filepath.Join(root, "calls.jsonl")
	operatorRoot := filepath.Join(root, "operator")
	environment := append(os.Environ(), "MPRLAB_GATEWAY_EXECUTABLE="+executable, "MPRLAB_GATEWAY_OPERATOR_ROOT="+operatorRoot, "GATEWAY_FIXTURE_LOG="+logfile)
	chain := exec.Command("/bin/bash", "-c", "make release && make publish && make deploy")
	chain.Dir = application
	chain.Env = environment
	if output, err := chain.CombinedOutput(); err != nil {
		t.Fatalf("installed lifecycle without sibling source: %v: %s", err, output)
	}
	contents, err := os.ReadFile(logfile)
	if err != nil {
		t.Fatal(err)
	}
	var records []struct {
		Arguments    []string `json:"arguments"`
		OperatorRoot string   `json:"operator_root"`
	}
	for _, line := range bytes.Split(bytes.TrimSpace(contents), []byte("\n")) {
		var record struct {
			Arguments    []string `json:"arguments"`
			OperatorRoot string   `json:"operator_root"`
		}
		if err := json.Unmarshal(line, &record); err != nil {
			t.Fatal(err)
		}
		records = append(records, record)
	}
	if len(records) != 3 {
		t.Fatalf("expected three operations: %s", contents)
	}
	for index, operation := range []string{"app-release", "app-publish", "app-deploy"} {
		if !reflect.DeepEqual(records[index].Arguments, []string{operation, "--app-root", application}) || records[index].OperatorRoot != operatorRoot {
			t.Fatalf("incorrect installed call: %#v", records[index])
		}
	}
	failure := exec.Command("make", "publish")
	failure.Dir = application
	failure.Env = append(environment, "GATEWAY_FIXTURE_EXIT=37")
	output, err := failure.CombinedOutput()
	if err == nil || !strings.Contains(string(output), "native Gateway failure") || !strings.Contains(string(output), "Error 37") {
		t.Fatalf("native failure was not preserved: %v: %s", err, output)
	}
	missing := exec.Command("make", "release", "MPRLAB_GATEWAY_EXECUTABLE="+filepath.Join(root, "absent"))
	missing.Dir = application
	missing.Env = environment
	output, err = missing.CombinedOutput()
	if err == nil || !strings.Contains(string(output), "Install a released runtime") {
		t.Fatalf("missing runtime lacks actionable output: %v: %s", err, output)
	}
}
