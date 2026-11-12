package messenger

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

// echoPage serves a chat with two own messages sharing the same opening lines: an
// older one whose id is in the pre-submit snapshot, and optionally a new one.
const echoPageOld = `<div id="root-messenger-chat">
  <div id="m-old" class="js-message-item message-self">【周报】第45周<br>本周进展：<br>旧内容 A</div>
</div>`

const echoPageNew = `<div id="root-messenger-chat">
  <div id="m-old" class="js-message-item message-self">【周报】第45周<br>本周进展：<br>旧内容 A</div>
  <div id="m-new" class="js-message-item message-self">【周报】第45周<br><br>本周进展：<br>新内容 B</div>
</div>`

func runEchoMatch(t *testing.T, page string, exclude []string) bool {
	t.Helper()
	browser := DiscoverBrowser(Config{})
	if !browser.OK {
		t.Skip("no Chrome/Chromium available for a headless DOM test")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(page))
	}))
	defer server.Close()
	dir, err := os.MkdirTemp("", "ixf-echo-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	ctx, cancel, _ := newMessengerBrowserContext(context.Background(), browser.Path, dir, 30*time.Second, true)
	defer cancel()
	if err := chromedp.Run(ctx, chromedp.Navigate(server.URL)); err != nil {
		t.Fatalf("navigate: %v", err)
	}
	matched := false
	// A short timeout: a non-match polls until it expires, and that is the answer.
	_ = runWithTimeout(ctx, 3*time.Second, waitForSelfMessageAction("【周报】第45周\n本周进展：\n新内容 B", exclude, 3*time.Second, &matched))
	return matched
}

// An older message sharing the opening lines must not confirm a send. Only that
// older message is on the page, so a send that never landed would otherwise be
// reported as delivered. This is the false confirmation the prefix match opened.
func TestWaitForSelfMessageIgnoresAnOlderMessageWithTheSameOpening(t *testing.T) {
	if runEchoMatch(t, echoPageOld, []string{"m-old"}) {
		t.Fatal("matched the pre-existing message: a send that never landed would read as delivered")
	}
}

// The new message confirms, despite sharing its opening with the older one and
// despite the blank line the page renders differently.
func TestWaitForSelfMessageMatchesTheNewMessage(t *testing.T) {
	if !runEchoMatch(t, echoPageNew, []string{"m-old"}) {
		t.Fatal("did not match the new message")
	}
}
