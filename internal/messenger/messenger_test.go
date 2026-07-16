package messenger

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSupportsMessengerDesktopPlatformsOnly(t *testing.T) {
	for _, goos := range []string{"darwin", "windows"} {
		if !SupportedOS(goos) {
			t.Fatalf("SupportedOS(%q) = false, want true", goos)
		}
	}
	if SupportedOS("linux") {
		t.Fatal("SupportedOS(linux) = true, want false")
	}
}

func TestDiscoverProfileSelectsNewestMacOSProfileExplorer(t *testing.T) {
	root := t.TempDir()
	oldProfile := filepath.Join(root, "aha", "users", "old", "profile_explorer")
	newProfile := filepath.Join(root, "aha", "users", "new", "profile_explorer")
	mustMkdir(t, oldProfile)
	mustMkdir(t, newProfile)
	oldTime := time.Unix(100, 0)
	newTime := time.Unix(200, 0)
	if err := os.Chtimes(oldProfile, oldTime, oldTime); err != nil {
		t.Fatalf("chtimes old profile: %v", err)
	}
	if err := os.Chtimes(newProfile, newTime, newTime); err != nil {
		t.Fatalf("chtimes new profile: %v", err)
	}

	result := DiscoverProfile(ProfileConfig{GOOS: "darwin", AppSupport: root})

	if !result.OK || result.Path != newProfile || result.Source != "macos-profile-explorer" {
		t.Fatalf("DiscoverProfile = %+v, want newest macOS profile_explorer", result)
	}
}

func TestDiscoverProfileSupportsExplicitProfileDir(t *testing.T) {
	profile := filepath.Join(t.TempDir(), "profile_explorer")
	mustMkdir(t, profile)

	result := DiscoverProfile(ProfileConfig{GOOS: "linux", ProfileDir: profile})

	if !result.OK || result.Path != profile || result.Source != "explicit" {
		t.Fatalf("DiscoverProfile explicit = %+v", result)
	}
}

func TestCloneProfileSkipsLocksAndCaches(t *testing.T) {
	source := filepath.Join(t.TempDir(), "profile_explorer")
	mustMkdir(t, filepath.Join(source, "Default"))
	mustWrite(t, filepath.Join(source, "Default", "Preferences"), []byte("prefs"))
	mustWrite(t, filepath.Join(source, "SingletonLock"), []byte("lock"))
	mustWrite(t, filepath.Join(source, "SingletonSocket"), []byte("socket"))
	mustMkdir(t, filepath.Join(source, "Cache"))
	mustWrite(t, filepath.Join(source, "Cache", "entry"), []byte("cache"))

	clone, err := CloneProfile(source, t.TempDir())
	if err != nil {
		t.Fatalf("CloneProfile returned error: %v", err)
	}

	if clone.Source != source || clone.Path == "" || !strings.Contains(clone.Path, "profile_explorer") {
		t.Fatalf("CloneProfile result = %+v", clone)
	}
	if string(mustRead(t, filepath.Join(clone.Path, "Default", "Preferences"))) != "prefs" {
		t.Fatalf("clone did not copy profile data")
	}
	for _, skipped := range []string{"SingletonLock", "SingletonSocket", filepath.Join("Cache", "entry")} {
		if _, err := os.Stat(filepath.Join(clone.Path, skipped)); !os.IsNotExist(err) {
			t.Fatalf("clone retained skipped path %s", skipped)
		}
	}
}

func TestDoctorIsSecretSafeAndReportsReadiness(t *testing.T) {
	home := t.TempDir()
	profile := filepath.Join(home, "profile_explorer")
	browser := filepath.Join(home, "chrome")
	cookies := filepath.Join(home, "cookies.json")
	mustMkdir(t, profile)
	mustWrite(t, browser, []byte("browser"))
	mustWrite(t, cookies, []byte(`[{"name":"_csrf_token","value":"secret-csrf"},{"name":"session","value":"secret-session"}]`))

	payload := Doctor(Config{
		GOOS:        "darwin",
		ProfileDir:  profile,
		BrowserPath: browser,
		CookiesPath: cookies,
	})
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal doctor payload: %v", err)
	}
	text := string(encoded)
	for _, secret := range []string{"secret-csrf", "secret-session"} {
		if strings.Contains(text, secret) {
			t.Fatalf("doctor payload leaked secret %q: %s", secret, text)
		}
	}
	if ok, _ := payload["ok"].(bool); !ok {
		t.Fatalf("doctor ok = false: %+v", payload)
	}
	if payload["runtime"] != "go" {
		t.Fatalf("runtime = %v, want go", payload["runtime"])
	}
}

func TestPlanOpenIsDryRunOnlyAndValidatesTargetMode(t *testing.T) {
	home := t.TempDir()
	profile := filepath.Join(home, "profile_explorer")
	browser := filepath.Join(home, "chrome")
	mustMkdir(t, profile)
	mustWrite(t, browser, []byte("browser"))

	if _, err := PlanOpen(OpenConfig{Target: "", Mode: "person", DryRun: true}); err == nil {
		t.Fatal("PlanOpen accepted empty target")
	}
	if _, err := PlanOpen(OpenConfig{Target: "示例群聊", Mode: "team", DryRun: true}); err == nil {
		t.Fatal("PlanOpen accepted invalid mode")
	}
	if _, err := PlanOpen(OpenConfig{Target: "示例群聊", Mode: "conversation", DryRun: false}); err == nil {
		t.Fatal("PlanOpen accepted non-dry-run open")
	}

	payload, err := PlanOpen(OpenConfig{
		Config: Config{
			GOOS:        "darwin",
			ProfileDir:  profile,
			BrowserPath: browser,
		},
		Target: "示例群聊",
		Mode:   "conversation",
		DryRun: true,
	})
	if err != nil {
		t.Fatalf("PlanOpen returned error: %v", err)
	}
	if payload["target"] != "示例群聊" || payload["mode"] != "conversation" || payload["dryRun"] != true {
		t.Fatalf("PlanOpen payload = %+v", payload)
	}
	if payload["willSend"] != false || payload["targetVerified"] != false {
		t.Fatalf("PlanOpen should not send or claim verification: %+v", payload)
	}
}

func TestOpenTargetDryRunDoesNotInvokeAutomator(t *testing.T) {
	automator := &recordingAutomator{}

	payload, err := OpenTarget(context.Background(), OpenConfig{
		Target: "示例群聊",
		Mode:   "conversation",
		DryRun: true,
	}, automator)

	if err != nil {
		t.Fatalf("OpenTarget dry-run returned error: %v", err)
	}
	if automator.called {
		t.Fatal("OpenTarget dry-run invoked the browser automator")
	}
	if payload["browserLaunch"] != false || payload["targetVerified"] != false {
		t.Fatalf("dry-run payload should not launch or verify target: %+v", payload)
	}
}

func TestOpenTargetApplyClonesProfileInvokesAutomatorAndCleansClone(t *testing.T) {
	source := filepath.Join(t.TempDir(), "profile_explorer")
	browser := filepath.Join(t.TempDir(), "chrome")
	cookies := filepath.Join(t.TempDir(), "cookies.json")
	mustMkdir(t, filepath.Join(source, "Default"))
	mustWrite(t, filepath.Join(source, "Default", "Preferences"), []byte("prefs"))
	mustWrite(t, filepath.Join(source, "SingletonLock"), []byte("lock"))
	mustWrite(t, browser, []byte("browser"))
	mustWrite(t, cookies, []byte(`[{"name":"_csrf_token","value":"secret-csrf"}]`))
	automator := &recordingAutomator{result: BrowserOpenResult{
		OpenedTitle:    "示例群聊",
		TargetVerified: true,
		Headless:       true,
	}}

	payload, err := OpenTarget(context.Background(), OpenConfig{
		Config: Config{
			GOOS:        "darwin",
			ProfileDir:  source,
			BrowserPath: browser,
			CookiesPath: cookies,
		},
		Target:    "示例群聊",
		Mode:      "conversation",
		Apply:     true,
		TimeoutMS: 1500,
	}, automator)

	if err != nil {
		t.Fatalf("OpenTarget apply returned error: %v", err)
	}
	if !automator.called {
		t.Fatal("OpenTarget apply did not invoke the browser automator")
	}
	if automator.request.UserDataDir == source || automator.request.UserDataDir == "" {
		t.Fatalf("automator used live or empty profile path: %+v", automator.request)
	}
	if !automator.sawProfileData || !automator.sawNoSingleton {
		t.Fatalf("automator did not see the expected safe clone state: data=%t noSingleton=%t", automator.sawProfileData, automator.sawNoSingleton)
	}
	if automator.request.BrowserPath != browser || automator.request.CookiePath != cookies {
		t.Fatalf("automator request paths = %+v", automator.request)
	}
	if automator.request.Target != "示例群聊" || automator.request.Mode != "conversation" || !automator.request.Headless {
		t.Fatalf("automator request target/mode/headless = %+v", automator.request)
	}
	if automator.request.TimeoutMS != 1500 {
		t.Fatalf("automator timeout = %d, want 1500", automator.request.TimeoutMS)
	}
	if payload["browserLaunch"] != true || payload["targetVerified"] != true || payload["willSend"] != false {
		t.Fatalf("apply payload = %+v", payload)
	}
	if strings.Contains(fmtSprint(payload), "secret-csrf") {
		t.Fatalf("apply payload leaked cookie value: %+v", payload)
	}
	if _, err := os.Stat(automator.request.UserDataDir); !os.IsNotExist(err) {
		t.Fatalf("temporary cloned profile still exists after apply: %s", automator.request.UserDataDir)
	}
}

func TestOpenTargetApplyCanKeepProfileCloneForDebugging(t *testing.T) {
	source := filepath.Join(t.TempDir(), "profile_explorer")
	browser := filepath.Join(t.TempDir(), "chrome")
	mustMkdir(t, source)
	mustWrite(t, browser, []byte("browser"))
	automator := &recordingAutomator{result: BrowserOpenResult{OpenedTitle: "示例群聊", TargetVerified: true}}

	payload, err := OpenTarget(context.Background(), OpenConfig{
		Config: Config{
			GOOS:        "darwin",
			ProfileDir:  source,
			BrowserPath: browser,
		},
		Target:           "示例群聊",
		Mode:             "conversation",
		Apply:            true,
		KeepProfileClone: true,
	}, automator)

	if err != nil {
		t.Fatalf("OpenTarget keep clone returned error: %v", err)
	}
	if _, err := os.Stat(automator.request.UserDataDir); err != nil {
		t.Fatalf("kept clone missing: %v", err)
	}
	clone, _ := payload["clone"].(map[string]any)
	if clone["kept"] != true {
		t.Fatalf("payload clone metadata = %+v", payload["clone"])
	}
}

type recordingAutomator struct {
	called         bool
	sawProfileData bool
	sawNoSingleton bool
	request        BrowserOpenRequest
	result         BrowserOpenResult
	err            error
}

func (automator *recordingAutomator) Open(_ context.Context, request BrowserOpenRequest) (BrowserOpenResult, error) {
	automator.called = true
	automator.request = request
	if content, err := os.ReadFile(filepath.Join(request.UserDataDir, "Default", "Preferences")); err == nil && string(content) == "prefs" {
		automator.sawProfileData = true
	}
	if _, err := os.Stat(filepath.Join(request.UserDataDir, "SingletonLock")); os.IsNotExist(err) {
		automator.sawNoSingleton = true
	}
	return automator.result, automator.err
}

func fmtSprint(value any) string {
	return strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(jsonString(value)), "\\u003c", "<"), "\\u003e", ">")
}

func jsonString(value any) string {
	content, _ := json.Marshal(value)
	return string(content)
}

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}

func mustWrite(t *testing.T, path string, content []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir parent for %s: %v", path, err)
	}
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return content
}
