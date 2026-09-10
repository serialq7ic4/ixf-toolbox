package dependencies

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	ixfupdate "github.com/serialq7ic4/ixf-toolbox/internal/update"
)

type fakeRunner struct {
	calls    *[]string
	runCalls *int
	missing  map[string]error
	failAt   string
}

func (r fakeRunner) LookPath(name string) (string, error) {
	if err := r.missing[name]; err != nil {
		return "", err
	}
	return "/fixture/bin/" + name, nil
}

func (r fakeRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if r.runCalls != nil {
		(*r.runCalls)++
	}
	call := strings.Join(append([]string{name}, args...), " ")
	if r.calls != nil {
		*r.calls = append(*r.calls, call)
	}
	if call == r.failAt {
		return []byte("private fixture output"), errors.New("fixture command failed")
	}
	return nil, nil
}

func TestDiagnosePreservesDependencyShape(t *testing.T) {
	releaseCalls := 0
	result := Diagnose(Options{
		CurrentVersion: "1.2.3",
		ReleaseLoader: func(string, string) (ixfupdate.Release, error) {
			releaseCalls++
			return ixfupdate.Release{TagName: "v1.2.4", HTMLURL: "https://example.test/v1.2.4"}, nil
		},
		MermaidStatus:   func() map[string]any { return map[string]any{"ok": true, "available": true, "ready": true} },
		MessengerStatus: func(string) map[string]any { return map[string]any{"ok": false} },
	})
	if releaseCalls != 1 || result["ok"] != false {
		t.Fatalf("diagnose result=%#v releaseCalls=%d", result, releaseCalls)
	}
	for _, key := range []string{"mermaid", "messenger", "update"} {
		if _, ok := result[key].(map[string]any); !ok {
			t.Fatalf("missing dependency key %q: %#v", key, result)
		}
	}
	update := result["update"].(map[string]any)
	if update["ok"] != true || update["currentVersion"] != "1.2.3" || update["latestVersion"] != "1.2.4" {
		t.Fatalf("update status = %#v", update)
	}
}

func TestInstallDryRunPlansWithoutRunningCommandsOrLoadingRelease(t *testing.T) {
	var runCalls int
	var releaseCalls int
	status := func() map[string]any {
		return map[string]any{"ok": false, "available": false, "ready": false}
	}
	result, err := Install(InstallOptions{Options: Options{
		CurrentVersion: "1.2.3",
		ReleaseLoader: func(string, string) (ixfupdate.Release, error) {
			releaseCalls++
			return ixfupdate.Release{}, nil
		},
		MermaidStatus: status,
		MessengerStatus: func(string) map[string]any {
			return map[string]any{"ok": false}
		},
		Runner: fakeRunner{runCalls: &runCalls},
	}, Apply: false})
	if err != nil {
		t.Fatal(err)
	}
	if result["dryRun"] != true || result["apply"] != false || result["applied"] != false {
		t.Fatalf("result = %#v", result)
	}
	if runCalls != 0 || releaseCalls != 0 {
		t.Fatalf("dry-run side effects: run=%d release=%d", runCalls, releaseCalls)
	}
	dependencies := result["dependencies"].(map[string]any)
	update := dependencies["update"].(map[string]any)
	if update["checked"] != false || update["reason"] != "not-used-by-installer" {
		t.Fatalf("update was checked: %#v", update)
	}
	want := []string{
		"npm install -g @mermaid-js/mermaid-cli",
		"npx puppeteer browsers install chrome-headless-shell",
	}
	if !reflect.DeepEqual(result["commands"], want) {
		t.Fatalf("commands = %#v, want %#v", result["commands"], want)
	}
}

func TestInstallApplyRunsOnlyAllowlistedCommandsInOrder(t *testing.T) {
	var calls []string
	statuses := 0
	runner := fakeRunner{calls: &calls}
	result, err := Install(InstallOptions{Options: Options{
		MermaidStatus: func() map[string]any {
			statuses++
			ready := statuses >= 2
			return map[string]any{"ok": ready, "available": ready, "ready": ready}
		},
		MessengerStatus: func(string) map[string]any { return map[string]any{"ok": true} },
		Runner:          runner,
	}, Apply: true})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"npm install -g @mermaid-js/mermaid-cli",
		"npx puppeteer browsers install chrome-headless-shell",
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %#v, want %#v", calls, want)
	}
	if result["applied"] != true {
		t.Fatalf("result = %#v", result)
	}
	if statuses != 2 {
		t.Fatalf("Mermaid status calls = %d, want one preflight and one post-install probe", statuses)
	}
}

func TestInstallPreflightsEveryExecutableBeforeRunning(t *testing.T) {
	var calls []string
	result, err := Install(InstallOptions{Options: Options{
		MermaidStatus:   func() map[string]any { return map[string]any{"ok": false, "available": false, "ready": false} },
		MessengerStatus: func(string) map[string]any { return map[string]any{"ok": true} },
		Runner: fakeRunner{
			calls:   &calls,
			missing: map[string]error{"npx": errors.New("not found")},
		},
	}, Apply: true})
	if err == nil || len(calls) != 0 {
		t.Fatalf("missing preflight err=%v calls=%#v result=%#v", err, calls, result)
	}
	if result["failedCommand"] != "npx puppeteer browsers install chrome-headless-shell" || !strings.Contains(result["remediation"].(string), "Node.js") {
		t.Fatalf("missing preflight result = %#v", result)
	}
}

func TestInstallFailureStopsSequenceAndDoesNotLeakOutput(t *testing.T) {
	var calls []string
	result, err := Install(InstallOptions{Options: Options{
		MermaidStatus:   func() map[string]any { return map[string]any{"ok": false, "available": false, "ready": false} },
		MessengerStatus: func(string) map[string]any { return map[string]any{"ok": true} },
		Runner: fakeRunner{
			calls:  &calls,
			failAt: "npm install -g @mermaid-js/mermaid-cli",
		},
	}, Apply: true})
	if err == nil || !reflect.DeepEqual(calls, []string{"npm install -g @mermaid-js/mermaid-cli"}) {
		t.Fatalf("failure err=%v calls=%#v result=%#v", err, calls, result)
	}
	if result["failedCommand"] != "npm install -g @mermaid-js/mermaid-cli" {
		t.Fatalf("failure result = %#v", result)
	}
	encoded, marshalErr := json.Marshal(result)
	if marshalErr != nil {
		t.Fatal(marshalErr)
	}
	if strings.Contains(string(encoded), "private fixture output") || strings.Contains(err.Error(), "private fixture output") {
		t.Fatalf("private command output leaked: result=%s err=%v", encoded, err)
	}
}

func TestInstallEmptyPlanDoesNotReportApplied(t *testing.T) {
	var runCalls int
	result, err := Install(InstallOptions{Options: Options{
		MermaidStatus:   func() map[string]any { return map[string]any{"ok": true, "available": true, "ready": true} },
		MessengerStatus: func(string) map[string]any { return map[string]any{"ok": true} },
		Runner:          fakeRunner{runCalls: &runCalls},
	}, Apply: true})
	if err != nil || runCalls != 0 || result["applied"] != false {
		t.Fatalf("empty plan result=%#v err=%v runCalls=%d", result, err, runCalls)
	}
}
