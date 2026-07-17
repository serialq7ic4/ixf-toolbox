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
	messengerPayload, ok := payload["messenger"].(map[string]any)
	if !ok {
		t.Fatalf("messenger diagnostics missing: %+v", payload)
	}
	if messengerPayload["realSendAvailable"] != true {
		t.Fatalf("realSendAvailable = %v, want true after send automation release", messengerPayload["realSendAvailable"])
	}
}

func TestDoctorReportsSecretSafeRemediation(t *testing.T) {
	home := t.TempDir()
	profile := filepath.Join(home, "profile_explorer")
	browser := filepath.Join(home, "chrome")
	cookies := filepath.Join(home, "cookies.json")
	mustMkdir(t, profile)
	mustWrite(t, browser, []byte("browser"))
	mustWrite(t, cookies, []byte(`[{"name":"_csrf_token","value":"secret-csrf"},{"name":"session","value":"secret-session"}]`))

	missingBrowserPayload := Doctor(Config{
		GOOS:        "darwin",
		ProfileDir:  profile,
		BrowserPath: filepath.Join(home, "missing-chrome"),
		CookiesPath: cookies,
	})
	remediation, ok := missingBrowserPayload["remediation"].([]string)
	if !ok || len(remediation) == 0 {
		t.Fatalf("doctor missing remediation for browser failure: %+v", missingBrowserPayload)
	}
	text := strings.Join(remediation, "\n")
	for _, expected := range []string{"Chrome or Chromium", "--browser-path", "IXF_MESSENGER_BROWSER_PATH"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("browser remediation missing %q: %s", expected, text)
		}
	}
	for _, secret := range []string{"secret-csrf", "secret-session"} {
		if strings.Contains(fmtSprint(missingBrowserPayload), secret) {
			t.Fatalf("doctor remediation leaked secret %q: %+v", secret, missingBrowserPayload)
		}
	}

	missingCookiesPayload := Doctor(Config{
		GOOS:        "darwin",
		ProfileDir:  profile,
		BrowserPath: browser,
		CookiesPath: filepath.Join(home, "missing-cookies.json"),
	})
	remediation, ok = missingCookiesPayload["remediation"].([]string)
	if !ok || !strings.Contains(strings.Join(remediation, "\n"), "ixf cookies export") {
		t.Fatalf("doctor missing cookie export remediation: %+v", missingCookiesPayload)
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

func TestPlanReadValidatesScopeAndRequiresDryRunOrApply(t *testing.T) {
	if _, err := PlanRead(ReadConfig{Scope: "unknown", DryRun: true}); err == nil {
		t.Fatal("PlanRead accepted unsupported scope")
	}
	if _, err := PlanRead(ReadConfig{Scope: "unread"}); err == nil {
		t.Fatal("PlanRead accepted missing --dry-run/--apply")
	}
	if _, err := PlanRead(ReadConfig{Scope: "unread", DryRun: true, Apply: true}); err == nil {
		t.Fatal("PlanRead accepted mutually exclusive dry-run/apply")
	}

	payload, err := PlanRead(ReadConfig{Scope: "unread", DryRun: true, Limit: 2, MessagesPerChat: 3})
	if err != nil {
		t.Fatalf("PlanRead returned error: %v", err)
	}
	if payload["action"] != "read" || payload["scope"] != "unread" || payload["dryRun"] != true {
		t.Fatalf("PlanRead payload = %+v", payload)
	}
	if payload["willSend"] != false || payload["browserLaunch"] != false {
		t.Fatalf("PlanRead should not send or launch in dry-run: %+v", payload)
	}
}

func TestReadMessagesDryRunDoesNotInvokeAutomator(t *testing.T) {
	automator := &recordingAutomator{}

	payload, err := ReadMessages(context.Background(), ReadConfig{
		Scope:  "recent",
		Limit:  2,
		DryRun: true,
	}, automator)

	if err != nil {
		t.Fatalf("ReadMessages dry-run returned error: %v", err)
	}
	if automator.readCalled {
		t.Fatal("ReadMessages dry-run invoked the browser automator")
	}
	if payload["browserLaunch"] != false || payload["willSend"] != false {
		t.Fatalf("dry-run payload should not launch or send: %+v", payload)
	}
}

func TestReadMessagesApplyClonesProfileInvokesAutomatorAndCleansClone(t *testing.T) {
	source := filepath.Join(t.TempDir(), "profile_explorer")
	browser := filepath.Join(t.TempDir(), "chrome")
	cookies := filepath.Join(t.TempDir(), "cookies.json")
	mustMkdir(t, filepath.Join(source, "Default"))
	mustWrite(t, filepath.Join(source, "Default", "Preferences"), []byte("prefs"))
	mustWrite(t, filepath.Join(source, "SingletonLock"), []byte("lock"))
	mustWrite(t, browser, []byte("browser"))
	mustWrite(t, cookies, []byte(`[{"name":"_csrf_token","value":"secret-csrf"}]`))
	automator := &recordingAutomator{readResult: BrowserReadResult{
		Scope: "unread",
		Conversations: []ConversationRead{
			{
				Title:       "示例群聊",
				Unread:      2,
				Time:        "09:30",
				Preview:     "最新预览",
				OpenedTitle: "示例群聊",
				Messages: []MessageRead{
					{Side: "other", Text: "需要处理的问题"},
				},
			},
		},
		RecentSeen:   4,
		UnreadSeen:   1,
		Extracted:    1,
		Headless:     true,
		FallbackUsed: false,
	}}

	payload, err := ReadMessages(context.Background(), ReadConfig{
		Config: Config{
			GOOS:        "darwin",
			ProfileDir:  source,
			BrowserPath: browser,
			CookiesPath: cookies,
		},
		Scope:           "unread",
		Limit:           5,
		MessagesPerChat: 3,
		Apply:           true,
		TimeoutMS:       1500,
	}, automator)

	if err != nil {
		t.Fatalf("ReadMessages apply returned error: %v", err)
	}
	if !automator.readCalled {
		t.Fatal("ReadMessages apply did not invoke the browser automator")
	}
	if automator.readRequest.UserDataDir == source || automator.readRequest.UserDataDir == "" {
		t.Fatalf("automator used live or empty profile path: %+v", automator.readRequest)
	}
	if !automator.sawProfileData || !automator.sawNoSingleton {
		t.Fatalf("automator did not see the expected safe clone state: data=%t noSingleton=%t", automator.sawProfileData, automator.sawNoSingleton)
	}
	if automator.readRequest.Scope != "unread" || automator.readRequest.Limit != 5 || automator.readRequest.MessagesPerChat != 3 {
		t.Fatalf("automator read request options = %+v", automator.readRequest)
	}
	if automator.readRequest.TimeoutMS != 1500 || !automator.readRequest.Headless {
		t.Fatalf("automator read request timeout/headless = %+v", automator.readRequest)
	}
	if payload["browserLaunch"] != true || payload["willSend"] != false || payload["ok"] != true {
		t.Fatalf("read apply payload = %+v", payload)
	}
	if payload["totalExtractedConversations"] != 1 || payload["totalUnreadConversations"] != 1 {
		t.Fatalf("read totals = %+v", payload)
	}
	if !strings.Contains(fmtSprint(payload), "需要处理的问题") {
		t.Fatalf("read payload missing message text: %+v", payload)
	}
	if strings.Contains(fmtSprint(payload), "secret-csrf") {
		t.Fatalf("read payload leaked cookie value: %+v", payload)
	}
	if _, err := os.Stat(automator.readRequest.UserDataDir); !os.IsNotExist(err) {
		t.Fatalf("temporary cloned profile still exists after read: %s", automator.readRequest.UserDataDir)
	}
}

func TestPlanSendValidatesTargetModeMessageAndDoesNotEchoMessage(t *testing.T) {
	if _, err := PlanSend(SendConfig{Target: "", Mode: "person", Message: "hello", DryRun: true}); err == nil {
		t.Fatal("PlanSend accepted empty target")
	}
	if _, err := PlanSend(SendConfig{Target: "示例群聊", Mode: "team", Message: "hello", DryRun: true}); err == nil {
		t.Fatal("PlanSend accepted invalid mode")
	}
	if _, err := PlanSend(SendConfig{Target: "示例群聊", Mode: "conversation", Message: "", DryRun: true}); err == nil {
		t.Fatal("PlanSend accepted empty message")
	}
	if _, err := PlanSend(SendConfig{Target: "示例群聊", Mode: "conversation", Message: "hello"}); err == nil {
		t.Fatal("PlanSend accepted missing dry-run/apply")
	}
	if _, err := PlanSend(SendConfig{Target: "示例群聊", Mode: "conversation", Message: "hello", DryRun: true, Apply: true}); err == nil {
		t.Fatal("PlanSend accepted mutually exclusive dry-run/apply")
	}

	payload, err := PlanSend(SendConfig{Target: "示例群聊", Mode: "conversation", Message: "secret message", DryRun: true})
	if err != nil {
		t.Fatalf("PlanSend returned error: %v", err)
	}
	if payload["action"] != "send" || payload["willSend"] != true || payload["sent"] != false {
		t.Fatalf("PlanSend payload = %+v", payload)
	}
	if payload["messageLength"] != len("secret message") {
		t.Fatalf("PlanSend message length = %+v", payload["messageLength"])
	}
	if strings.Contains(fmtSprint(payload), "secret message") {
		t.Fatalf("PlanSend echoed message body: %+v", payload)
	}
}

func TestSendMessageDryRunDoesNotInvokeAutomator(t *testing.T) {
	automator := &recordingAutomator{}

	payload, err := SendMessage(context.Background(), SendConfig{
		Target:  "示例群聊",
		Mode:    "conversation",
		Message: "secret message",
		DryRun:  true,
	}, automator)

	if err != nil {
		t.Fatalf("SendMessage dry-run returned error: %v", err)
	}
	if automator.sendCalled {
		t.Fatal("SendMessage dry-run invoked the browser automator")
	}
	if payload["browserLaunch"] != false || payload["sent"] != false || payload["verifiedPresent"] != false {
		t.Fatalf("dry-run payload should not launch, send, or verify: %+v", payload)
	}
	if strings.Contains(fmtSprint(payload), "secret message") {
		t.Fatalf("dry-run payload echoed message body: %+v", payload)
	}
}

func TestSendMessageApplyUsesTwoClonesAndRequiresVerification(t *testing.T) {
	source := filepath.Join(t.TempDir(), "profile_explorer")
	browser := filepath.Join(t.TempDir(), "chrome")
	cookies := filepath.Join(t.TempDir(), "cookies.json")
	mustMkdir(t, filepath.Join(source, "Default"))
	mustWrite(t, filepath.Join(source, "Default", "Preferences"), []byte("prefs"))
	mustWrite(t, filepath.Join(source, "SingletonLock"), []byte("lock"))
	mustWrite(t, browser, []byte("browser"))
	mustWrite(t, cookies, []byte(`[{"name":"_csrf_token","value":"secret-csrf"}]`))
	automator := &recordingAutomator{sendResult: BrowserSendResult{
		OpenedTitle:      "示例群聊",
		TargetVerified:   true,
		Sent:             true,
		LocalEchoMatched: true,
		VerifiedPresent:  true,
		Headless:         true,
	}}

	payload, err := SendMessage(context.Background(), SendConfig{
		Config: Config{
			GOOS:        "darwin",
			ProfileDir:  source,
			BrowserPath: browser,
			CookiesPath: cookies,
		},
		Target:    "示例群聊",
		Mode:      "conversation",
		Message:   "secret message",
		Apply:     true,
		TimeoutMS: 1500,
	}, automator)

	if err != nil {
		t.Fatalf("SendMessage apply returned error: %v", err)
	}
	if !automator.sendCalled {
		t.Fatal("SendMessage apply did not invoke the browser automator")
	}
	if automator.sendRequest.UserDataDir == source || automator.sendRequest.VerifyUserDataDir == source {
		t.Fatalf("automator used live profile path: %+v", automator.sendRequest)
	}
	if automator.sendRequest.UserDataDir == "" || automator.sendRequest.VerifyUserDataDir == "" || automator.sendRequest.UserDataDir == automator.sendRequest.VerifyUserDataDir {
		t.Fatalf("automator did not receive two distinct clones: %+v", automator.sendRequest)
	}
	if !automator.sawProfileData || !automator.sawNoSingleton || !automator.sawVerifyProfileData || !automator.sawVerifyNoSingleton {
		t.Fatalf("automator did not see expected clone state: %+v", automator)
	}
	if automator.sendRequest.Target != "示例群聊" || automator.sendRequest.Mode != "conversation" || automator.sendRequest.Message != "secret message" {
		t.Fatalf("automator send request = %+v", automator.sendRequest)
	}
	if payload["sent"] != true || payload["targetVerified"] != true || payload["verifiedPresent"] != true || payload["localEchoMatched"] != true {
		t.Fatalf("send apply payload = %+v", payload)
	}
	if strings.Contains(fmtSprint(payload), "secret message") || strings.Contains(fmtSprint(payload), "secret-csrf") {
		t.Fatalf("send apply payload leaked secret data: %+v", payload)
	}
	if _, err := os.Stat(automator.sendRequest.UserDataDir); !os.IsNotExist(err) {
		t.Fatalf("temporary send clone still exists: %s", automator.sendRequest.UserDataDir)
	}
	if _, err := os.Stat(automator.sendRequest.VerifyUserDataDir); !os.IsNotExist(err) {
		t.Fatalf("temporary verify clone still exists: %s", automator.sendRequest.VerifyUserDataDir)
	}
}

type recordingAutomator struct {
	called               bool
	readCalled           bool
	sendCalled           bool
	sawProfileData       bool
	sawNoSingleton       bool
	sawVerifyProfileData bool
	sawVerifyNoSingleton bool
	request              BrowserOpenRequest
	readRequest          BrowserReadRequest
	sendRequest          BrowserSendRequest
	result               BrowserOpenResult
	readResult           BrowserReadResult
	sendResult           BrowserSendResult
	err                  error
	readErr              error
	sendErr              error
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

func (automator *recordingAutomator) Read(_ context.Context, request BrowserReadRequest) (BrowserReadResult, error) {
	automator.readCalled = true
	automator.readRequest = request
	if content, err := os.ReadFile(filepath.Join(request.UserDataDir, "Default", "Preferences")); err == nil && string(content) == "prefs" {
		automator.sawProfileData = true
	}
	if _, err := os.Stat(filepath.Join(request.UserDataDir, "SingletonLock")); os.IsNotExist(err) {
		automator.sawNoSingleton = true
	}
	return automator.readResult, automator.readErr
}

func (automator *recordingAutomator) Send(_ context.Context, request BrowserSendRequest) (BrowserSendResult, error) {
	automator.sendCalled = true
	automator.sendRequest = request
	if content, err := os.ReadFile(filepath.Join(request.UserDataDir, "Default", "Preferences")); err == nil && string(content) == "prefs" {
		automator.sawProfileData = true
	}
	if _, err := os.Stat(filepath.Join(request.UserDataDir, "SingletonLock")); os.IsNotExist(err) {
		automator.sawNoSingleton = true
	}
	if content, err := os.ReadFile(filepath.Join(request.VerifyUserDataDir, "Default", "Preferences")); err == nil && string(content) == "prefs" {
		automator.sawVerifyProfileData = true
	}
	if _, err := os.Stat(filepath.Join(request.VerifyUserDataDir, "SingletonLock")); os.IsNotExist(err) {
		automator.sawVerifyNoSingleton = true
	}
	return automator.sendResult, automator.sendErr
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
