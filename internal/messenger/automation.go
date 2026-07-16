package messenger

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
	"unicode"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
	"github.com/chromedp/chromedp/kb"
)

type BrowserCookie struct {
	Name     string `json:"name"`
	Value    string `json:"value"`
	Domain   string `json:"domain"`
	Path     string `json:"path"`
	Secure   bool   `json:"secure"`
	HTTPOnly bool   `json:"httpOnly"`
	Expires  int64  `json:"expires"`
	SameSite string `json:"sameSite"`
}

type ChromedpAutomator struct{}

func (automator ChromedpAutomator) Open(ctx context.Context, request BrowserOpenRequest) (BrowserOpenResult, error) {
	result, err := automator.openOnce(ctx, request, request.Headless)
	if err == nil {
		return result, nil
	}
	if request.Headless && request.AllowVisibleFallback {
		fallback, fallbackErr := automator.openOnce(ctx, request, false)
		if fallbackErr == nil {
			fallback.FallbackUsed = true
			return fallback, nil
		}
	}
	return BrowserOpenResult{}, err
}

func (automator ChromedpAutomator) openOnce(parent context.Context, request BrowserOpenRequest, headless bool) (BrowserOpenResult, error) {
	if strings.TrimSpace(request.BrowserPath) == "" {
		return BrowserOpenResult{}, fmt.Errorf("browser path is required")
	}
	if strings.TrimSpace(request.UserDataDir) == "" {
		return BrowserOpenResult{}, fmt.Errorf("cloned profile path is required")
	}
	timeout := time.Duration(request.TimeoutMS) * time.Millisecond
	if timeout <= 0 {
		timeout = 45 * time.Second
	}
	runCtx, cancelTimeout := context.WithTimeout(parent, timeout)
	defer cancelTimeout()

	opts := []chromedp.ExecAllocatorOption{
		chromedp.ExecPath(request.BrowserPath),
		chromedp.UserDataDir(request.UserDataDir),
		chromedp.NoFirstRun,
		chromedp.NoDefaultBrowserCheck,
		chromedp.WindowSize(1440, 960),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("disable-extensions", true),
	}
	if headless {
		opts = append(opts, chromedp.Headless)
	}
	allocCtx, cancelAlloc := chromedp.NewExecAllocator(runCtx, opts...)
	defer cancelAlloc()
	browserCtx, cancelBrowser := chromedp.NewContext(allocCtx)
	defer cancelBrowser()

	cookies, err := LoadBrowserCookies(request.CookiePath)
	if err != nil {
		return BrowserOpenResult{}, err
	}
	actions := chromedp.Tasks{network.Enable()}
	if len(cookies) > 0 {
		params := networkCookieParams(cookies)
		if len(params) > 0 {
			actions = append(actions, chromedp.ActionFunc(func(ctx context.Context) error {
				return network.SetCookies(params).Do(ctx)
			}))
		}
	}
	actions = append(actions,
		chromedp.Navigate(valueOrDefault(request.URL, DefaultMessengerURL)),
		waitForMessengerAction(timeout),
		openTargetAction(request.Target, request.Mode),
	)
	if err := chromedp.Run(browserCtx, actions); err != nil {
		return BrowserOpenResult{}, err
	}
	verification := targetVerification{}
	if err := chromedp.Run(browserCtx, waitForTargetVerificationAction(request.Target, timeout, &verification)); err != nil {
		return BrowserOpenResult{}, err
	}
	opened := verification.Title
	if opened == "" {
		opened = request.Target
	}
	return BrowserOpenResult{OpenedTitle: opened, TargetVerified: verification.Verified, Headless: headless}, nil
}

func LoadBrowserCookies(path string) ([]BrowserCookie, error) {
	path = expandUser(path)
	if strings.TrimSpace(path) == "" {
		return nil, nil
	}
	content, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("could not read cookie JSON")
	}
	var cookies []BrowserCookie
	if err := json.Unmarshal(content, &cookies); err != nil {
		return nil, fmt.Errorf("cookie JSON must be an array")
	}
	return cookies, nil
}

func TitlesMatch(actual string, expected string) bool {
	actualKey := matchKey(actual)
	expectedKey := matchKey(expected)
	if actualKey == "" || expectedKey == "" {
		return false
	}
	return actualKey == expectedKey || strings.Contains(actualKey, expectedKey) || strings.Contains(expectedKey, actualKey)
}

func matchKey(value string) string {
	var builder strings.Builder
	for _, r := range strings.ToLower(value) {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r), unicode.Is(unicode.Han, r):
			builder.WriteRune(r)
		}
	}
	return builder.String()
}

func networkCookieParams(cookies []BrowserCookie) []*network.CookieParam {
	params := make([]*network.CookieParam, 0, len(cookies))
	for _, cookie := range cookies {
		if cookie.Name == "" || cookie.Value == "" {
			continue
		}
		param := &network.CookieParam{
			Name:     cookie.Name,
			Value:    cookie.Value,
			Domain:   cookie.Domain,
			Path:     valueOrDefault(cookie.Path, "/"),
			Secure:   cookie.Secure,
			HTTPOnly: cookie.HTTPOnly,
		}
		switch cookie.SameSite {
		case "Strict":
			param.SameSite = network.CookieSameSiteStrict
		case "Lax":
			param.SameSite = network.CookieSameSiteLax
		case "None":
			param.SameSite = network.CookieSameSiteNone
		}
		if cookie.Expires > 0 {
			expires := cdp.TimeSinceEpoch(time.Unix(cookie.Expires, 0))
			param.Expires = &expires
		}
		params = append(params, param)
	}
	return params
}

type targetVerification struct {
	Verified bool   `json:"verified"`
	Title    string `json:"title"`
	Panel    string `json:"panel"`
}

func waitForMessengerAction(timeout time.Duration) chromedp.Action {
	return chromedp.PollFunction(`(cardSelector, searchSelector) => {
		const hasFeed = document.querySelectorAll(cardSelector).length > 0;
		const hasSearch = !!document.querySelector(searchSelector);
		const body = document.body ? (document.body.innerText || '') : '';
		if (body.includes('登录') && !body.includes('消息')) throw new Error('Messenger session appears logged out');
		return hasFeed && hasSearch;
	}`, nil,
		chromedp.WithPollingArgs(feedCardSelector, searchTriggerSelector),
		chromedp.WithPollingTimeout(timeout),
	)
}

func openTargetAction(target string, mode string) chromedp.Action {
	return chromedp.ActionFunc(func(ctx context.Context) error {
		clicked := false
		if mode == "conversation" {
			if err := chromedp.Run(ctx, clickVisibleRecentCardAction(target, &clicked)); err != nil {
				return err
			}
			if clicked {
				return nil
			}
		}
		return chromedp.Run(ctx,
			chromedp.Click(searchTriggerSelector, chromedp.ByQuery),
			chromedp.Sleep(500*time.Millisecond),
			chromedp.KeyEvent(target),
			chromedp.Sleep(1500*time.Millisecond),
			chromedp.KeyEvent(kb.Enter),
			chromedp.Sleep(1500*time.Millisecond),
		)
	})
}

func clickVisibleRecentCardAction(target string, clicked *bool) chromedp.Action {
	return chromedp.Evaluate(`(function(target, cardSelector, titleSelector, scrollSelector) {
		const normalize = (value) => String(value || '').toLowerCase().replace(/[^0-9a-zA-Z\u4e00-\u9fff]+/g, '');
		const expected = normalize(target);
		const scroll = document.querySelector(scrollSelector);
		if (scroll) scroll.scrollTop = 0;
		for (const card of Array.from(document.querySelectorAll(cardSelector))) {
			const title = (card.querySelector(titleSelector)?.innerText || '').trim();
			if (normalize(title) === expected) {
				card.click();
				return true;
			}
		}
		return false;
	})(`+jsArgs(target, feedCardSelector, feedTitleSelector, recentScrollSelector)+`)`, clicked)
}

func waitForTargetVerificationAction(target string, timeout time.Duration, result *targetVerification) chromedp.Action {
	return chromedp.PollFunction(`(target, titleSelector, panelSelector) => {
		const normalize = (value) => String(value || '').toLowerCase().replace(/[^0-9a-zA-Z\u4e00-\u9fff]+/g, '');
		const expected = normalize(target);
		const title = (document.querySelector(titleSelector)?.innerText || '').trim();
		const panel = (document.querySelector(panelSelector)?.innerText || '').trim();
		const matches = (value) => {
			const actual = normalize(value);
			return actual && expected && (actual === expected || actual.includes(expected) || expected.includes(actual));
		};
		if (matches(title) || matches(panel)) return {verified: true, title, panel: panel.slice(0, 512)};
		return false;
	}`, result,
		chromedp.WithPollingArgs(target, chatTitleSelector, rightPanelSelector),
		chromedp.WithPollingTimeout(timeout),
	)
}

func jsArgs(values ...string) string {
	encoded := make([]string, 0, len(values))
	for _, value := range values {
		content, _ := json.Marshal(value)
		encoded = append(encoded, string(content))
	}
	return strings.Join(encoded, ",")
}
