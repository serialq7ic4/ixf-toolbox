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
	cdpinput "github.com/chromedp/cdproto/input"
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

func (automator ChromedpAutomator) Read(ctx context.Context, request BrowserReadRequest) (BrowserReadResult, error) {
	result, err := automator.readOnce(ctx, request, request.Headless)
	if err == nil {
		return result, nil
	}
	if request.Headless && request.AllowVisibleFallback {
		fallback, fallbackErr := automator.readOnce(ctx, request, false)
		if fallbackErr == nil {
			fallback.FallbackUsed = true
			return fallback, nil
		}
	}
	return BrowserReadResult{}, err
}

func (automator ChromedpAutomator) Send(ctx context.Context, request BrowserSendRequest) (BrowserSendResult, error) {
	result, err := automator.sendOnce(ctx, request, request.Headless)
	if err == nil {
		return result, nil
	}
	if request.Headless && request.AllowVisibleFallback {
		fallback, fallbackErr := automator.sendOnce(ctx, request, false)
		if fallbackErr == nil {
			fallback.FallbackUsed = true
			return fallback, nil
		}
	}
	return BrowserSendResult{}, err
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

func (automator ChromedpAutomator) readOnce(parent context.Context, request BrowserReadRequest, headless bool) (BrowserReadResult, error) {
	if strings.TrimSpace(request.BrowserPath) == "" {
		return BrowserReadResult{}, fmt.Errorf("browser path is required")
	}
	if strings.TrimSpace(request.UserDataDir) == "" {
		return BrowserReadResult{}, fmt.Errorf("cloned profile path is required")
	}
	timeout := time.Duration(request.TimeoutMS) * time.Millisecond
	if timeout <= 0 {
		timeout = 60 * time.Second
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
		return BrowserReadResult{}, err
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
	var cards []conversationCard
	actions = append(actions,
		chromedp.Navigate(valueOrDefault(request.URL, DefaultMessengerURL)),
		waitForMessengerAction(timeout),
		collectRecentCardsAction(request.MaxScrolls, &cards),
	)
	if err := chromedp.Run(browserCtx, actions); err != nil {
		return BrowserReadResult{}, err
	}
	candidates := filterConversationCards(cards, request.Scope, request.Limit)
	result := BrowserReadResult{
		Scope:      valueOrDefault(request.Scope, "unread"),
		RecentSeen: len(cards),
		UnreadSeen: countUnreadCards(cards),
		Headless:   headless,
	}
	for _, card := range candidates {
		verification := targetVerification{}
		if err := chromedp.Run(browserCtx,
			openTargetAction(card.Title, "conversation"),
			waitForTargetVerificationAction(card.Title, timeout, &verification),
		); err != nil {
			result.SkippedConversations = append(result.SkippedConversations, ConversationSkip{
				Title:   card.Title,
				Unread:  card.Unread,
				Time:    card.Time,
				Preview: card.Preview,
				Error:   safeAutomationError(err),
			})
			continue
		}
		openedTitle := valueOrDefault(verification.Title, card.Title)
		var messages []MessageRead
		if err := chromedp.Run(browserCtx, readMessageItemsAction(request.MessagesPerChat, request.IncludeSelfMessages, &messages)); err != nil {
			result.SkippedConversations = append(result.SkippedConversations, ConversationSkip{
				Title:   card.Title,
				Unread:  card.Unread,
				Time:    card.Time,
				Preview: card.Preview,
				Error:   safeAutomationError(err),
			})
			continue
		}
		if len(messages) == 0 {
			var panelText string
			if err := chromedp.Run(browserCtx, readPanelTextAction(1500, &panelText)); err == nil && panelText != "" {
				messages = []MessageRead{{Side: "panel", Text: panelText}}
			}
		}
		result.Conversations = append(result.Conversations, ConversationRead{
			Title:       card.Title,
			Unread:      card.Unread,
			Time:        card.Time,
			Preview:     card.Preview,
			OpenedTitle: openedTitle,
			Messages:    messages,
		})
	}
	result.Extracted = len(result.Conversations)
	result.Skipped = len(result.SkippedConversations)
	return result, nil
}

func (automator ChromedpAutomator) sendOnce(parent context.Context, request BrowserSendRequest, headless bool) (BrowserSendResult, error) {
	if strings.TrimSpace(request.BrowserPath) == "" {
		return BrowserSendResult{}, fmt.Errorf("browser path is required")
	}
	if strings.TrimSpace(request.UserDataDir) == "" || strings.TrimSpace(request.VerifyUserDataDir) == "" {
		return BrowserSendResult{}, fmt.Errorf("send and verify cloned profile paths are required")
	}
	timeout := time.Duration(request.TimeoutMS) * time.Millisecond
	if timeout <= 0 {
		timeout = 90 * time.Second
	}

	sendCtx, cancelSend, err := newMessengerBrowserContext(parent, request.BrowserPath, request.UserDataDir, timeout, headless)
	if err != nil {
		return BrowserSendResult{}, err
	}
	defer cancelSend()

	sendActions, err := messengerStartupActions(request.CookiePath, request.URL, timeout)
	if err != nil {
		return BrowserSendResult{}, err
	}
	sendVerification := targetVerification{}
	localEchoMatched := false
	if err := chromedp.Run(sendCtx, sendActions); err != nil {
		return BrowserSendResult{}, fmt.Errorf("send startup: %w", err)
	}
	if err := chromedp.Run(sendCtx, openTargetAction(request.Target, request.Mode)); err != nil {
		return BrowserSendResult{}, fmt.Errorf("send open target: %w", err)
	}
	if err := chromedp.Run(sendCtx, waitForTargetVerificationAction(request.Target, timeout, &sendVerification)); err != nil {
		return BrowserSendResult{}, fmt.Errorf("send verify target: %w", err)
	}
	if err := chromedp.Run(sendCtx, sendMessageAction(request.Message)); err != nil {
		return BrowserSendResult{}, fmt.Errorf("send message: %w", err)
	}
	if err := chromedp.Run(sendCtx, waitForSelfMessageAction(request.Message, timeout, &localEchoMatched)); err != nil {
		return BrowserSendResult{}, fmt.Errorf("send local echo: %w", err)
	}

	verifyCtx, cancelVerify, err := newMessengerBrowserContext(parent, request.BrowserPath, request.VerifyUserDataDir, timeout, headless)
	if err != nil {
		return BrowserSendResult{}, err
	}
	defer cancelVerify()

	verifyActions, err := messengerStartupActions(request.CookiePath, request.URL, timeout)
	if err != nil {
		return BrowserSendResult{}, err
	}
	verifyMatched := false
	if err := chromedp.Run(verifyCtx, verifyActions); err != nil {
		return BrowserSendResult{}, fmt.Errorf("verify startup: %w", err)
	}
	if err := chromedp.Run(verifyCtx, openVerificationTargetAction(request.Target, request.Mode)); err != nil {
		return BrowserSendResult{}, fmt.Errorf("verify open target: %w", err)
	}
	if err := chromedp.Run(verifyCtx, waitForTargetVerificationAction(request.Target, timeout, &targetVerification{})); err != nil {
		return BrowserSendResult{}, fmt.Errorf("verify target: %w", err)
	}
	if err := chromedp.Run(verifyCtx, waitForSelfMessageAction(request.Message, timeout, &verifyMatched)); err != nil {
		return BrowserSendResult{}, fmt.Errorf("verify message presence: %w", err)
	}

	opened := sendVerification.Title
	if opened == "" {
		opened = request.Target
	}
	return BrowserSendResult{
		OpenedTitle:      opened,
		TargetVerified:   sendVerification.Verified,
		Sent:             true,
		LocalEchoMatched: localEchoMatched,
		VerifiedPresent:  verifyMatched,
		Headless:         headless,
	}, nil
}

func newMessengerBrowserContext(parent context.Context, browserPath string, userDataDir string, timeout time.Duration, headless bool) (context.Context, func(), error) {
	runCtx, cancelTimeout := context.WithTimeout(parent, timeout)
	opts := []chromedp.ExecAllocatorOption{
		chromedp.ExecPath(browserPath),
		chromedp.UserDataDir(userDataDir),
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
	browserCtx, cancelBrowser := chromedp.NewContext(allocCtx)
	cleanup := func() {
		cancelBrowser()
		cancelAlloc()
		cancelTimeout()
	}
	return browserCtx, cleanup, nil
}

func messengerStartupActions(cookiePath string, targetURL string, timeout time.Duration) (chromedp.Tasks, error) {
	cookies, err := LoadBrowserCookies(cookiePath)
	if err != nil {
		return nil, err
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
		chromedp.Navigate(valueOrDefault(targetURL, DefaultMessengerURL)),
		waitForMessengerAction(timeout),
	)
	return actions, nil
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

func CollapseLines(text string) string {
	text = strings.ReplaceAll(text, "\u200b", "\n")
	text = strings.ReplaceAll(text, "\u00a0", " ")
	lines := strings.Split(text, "\n")
	cleaned := make([]string, 0, len(lines))
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		cleaned = append(cleaned, strings.Join(fields, " "))
	}
	return strings.Join(cleaned, "\n")
}

func ParseUnreadBadge(text string) int {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0
	}
	value := 0
	for _, r := range text {
		if unicode.IsDigit(r) {
			value = value*10 + int(r-'0')
		}
	}
	if value > 0 {
		return value
	}
	return 1
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
		if mode == "person" {
			return openPersonBySearch(ctx, target)
		}
		return openConversationByTitle(ctx, target)
	})
}

func openVerificationTargetAction(target string, mode string) chromedp.Action {
	return chromedp.ActionFunc(func(ctx context.Context) error {
		if mode == "person" {
			if err := chromedp.Run(ctx, scrollRecentToTopAction()); err == nil {
				clicked := false
				if err := chromedp.Run(ctx, clickVisibleRecentCardLooseAction(target, &clicked)); err != nil {
					return err
				}
				if clicked {
					return nil
				}
			}
		}
		return chromedp.Run(ctx, openTargetAction(target, mode))
	})
}

func openConversationByTitle(ctx context.Context, target string) error {
	if err := chromedp.Run(ctx, scrollRecentToTopAction()); err != nil {
		return err
	}
	for index := 0; index < 18; index++ {
		clicked := false
		if err := chromedp.Run(ctx, clickVisibleRecentCardAction(target, &clicked)); err != nil {
			return err
		}
		if clicked {
			return nil
		}
		scrolled := false
		if err := chromedp.Run(ctx, scrollRecentCardsAction(&scrolled)); err != nil {
			return err
		}
		if !scrolled {
			break
		}
		if err := chromedp.Run(ctx, chromedp.Sleep(500*time.Millisecond)); err != nil {
			return err
		}
	}
	return openChatBySearch(ctx, target)
}

func openPersonBySearch(ctx context.Context, target string) error {
	queries := personSearchQueries(target)
	if len(queries) == 0 {
		return fmt.Errorf("empty personal recipient")
	}
	var lastError error
	for _, query := range queries {
		if err := openGlobalSearch(ctx, query); err != nil {
			lastError = err
			continue
		}
		deadline := time.Now().Add(15 * time.Second)
		for time.Now().Before(deadline) {
			clicked := false
			if err := chromedp.Run(ctx, clickMatchingUserCardAction(target, &clicked)); err != nil {
				lastError = err
				break
			}
			if clicked {
				if err := chromedp.Run(ctx, chromedp.Sleep(4*time.Second)); err != nil {
					return err
				}
				verification := targetVerification{}
				if err := chromedp.Run(ctx, waitForTargetVerificationAction(target, 8*time.Second, &verification)); err == nil && verification.Verified {
					return nil
				} else if err != nil {
					lastError = err
				}
				break
			}
			if err := chromedp.Run(ctx, chromedp.Sleep(500*time.Millisecond)); err != nil {
				return err
			}
		}
		_ = chromedp.Run(ctx, chromedp.KeyEvent(kb.Escape), chromedp.Sleep(500*time.Millisecond))
	}
	if looksLikeAccountID(target) {
		if err := chromedp.Run(ctx, scrollRecentToTopAction()); err == nil {
			clicked := false
			if err := chromedp.Run(ctx, clickVisibleRecentCardLooseAction(target, &clicked)); err != nil {
				return err
			}
			if clicked {
				verification := targetVerification{}
				if err := chromedp.Run(ctx, waitForTargetVerificationAction(target, 8*time.Second, &verification)); err == nil && verification.Verified {
					return nil
				} else if err != nil {
					lastError = err
				}
			}
		}
	}
	if lastError != nil {
		return lastError
	}
	return fmt.Errorf("unable to open personal recipient")
}

func openChatBySearch(ctx context.Context, target string) error {
	attempts := [][]string{{}, {kb.ArrowDown}}
	var lastError error
	for _, keys := range attempts {
		if err := openGlobalSearch(ctx, target); err != nil {
			lastError = err
			continue
		}
		for _, key := range keys {
			if err := chromedp.Run(ctx, chromedp.KeyEvent(key), chromedp.Sleep(250*time.Millisecond)); err != nil {
				return err
			}
		}
		if err := chromedp.Run(ctx, chromedp.KeyEvent(kb.Enter), chromedp.Sleep(1500*time.Millisecond)); err != nil {
			return err
		}
		verification := targetVerification{}
		if err := chromedp.Run(ctx, waitForTargetVerificationAction(target, 8*time.Second, &verification)); err == nil && verification.Verified {
			return nil
		} else if err != nil {
			lastError = err
		}
		_ = chromedp.Run(ctx, chromedp.KeyEvent(kb.Escape), chromedp.Sleep(500*time.Millisecond))
	}
	if lastError != nil {
		return lastError
	}
	return fmt.Errorf("unable to open chat by search")
}

func openGlobalSearch(ctx context.Context, query string) error {
	if err := chromedp.Run(ctx,
		chromedp.KeyEvent(kb.Escape),
		chromedp.Sleep(300*time.Millisecond),
		clickSearchTriggerOrHotkeyAction(),
		chromedp.Sleep(1500*time.Millisecond),
	); err != nil {
		return err
	}
	state := searchState{}
	for attempt := 0; attempt < 4; attempt++ {
		if err := chromedp.Run(ctx,
			clickSearchEditorAction(),
			chromedp.Sleep(200*time.Millisecond),
			chromedp.KeyEvent(kb.Meta+"a"),
			chromedp.Sleep(100*time.Millisecond),
			chromedp.KeyEvent(kb.Backspace),
			chromedp.Sleep(100*time.Millisecond),
			insertTextAction(query),
			chromedp.Sleep(1500*time.Millisecond),
			searchStateAction(query, &state),
		); err != nil {
			return err
		}
		if state.QueryPresent {
			return chromedp.Run(ctx, chromedp.Sleep(3*time.Second))
		}
		if err := chromedp.Run(ctx, chromedp.KeyEvent(kb.Meta+"k"), chromedp.Sleep(1*time.Second)); err != nil {
			return err
		}
	}
	return searchStateError(query, state)
}

type searchState struct {
	SearchTriggerCount int    `json:"searchTriggerCount"`
	SearchEditorCount  int    `json:"searchEditorCount"`
	UserResultCount    int    `json:"userResultCount"`
	ActiveElement      string `json:"activeElement"`
	TextLength         int    `json:"textLength"`
	QueryPresent       bool   `json:"queryPresent"`
}

func searchStateError(_ string, state searchState) error {
	return fmt.Errorf(
		"search modal did not accept query (trigger=%d editor=%d userCards=%d active=%s textLen=%d queryPresent=%t)",
		state.SearchTriggerCount,
		state.SearchEditorCount,
		state.UserResultCount,
		state.ActiveElement,
		state.TextLength,
		state.QueryPresent,
	)
}

func personSearchQueries(query string) []string {
	parts := strings.Fields(CollapseLines(query))
	candidates := []string{strings.TrimSpace(query)}
	if len(parts) > 1 {
		candidates = append(candidates, parts...)
	}
	seen := map[string]bool{}
	result := []string{}
	for _, candidate := range candidates {
		key := matchKey(candidate)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, strings.TrimSpace(candidate))
	}
	return result
}

func cardMatchesPerson(cardText string, query string) bool {
	cardKey := matchKey(cardText)
	parts := strings.Fields(CollapseLines(query))
	if cardKey == "" || len(parts) == 0 {
		return false
	}
	if len(parts) == 1 {
		return strings.Contains(cardKey, matchKey(parts[0]))
	}
	for _, part := range parts {
		key := matchKey(part)
		if key == "" || !strings.Contains(cardKey, key) {
			return false
		}
	}
	return true
}

func looksLikeAccountID(target string) bool {
	target = strings.TrimSpace(target)
	if strings.ContainsAny(target, " \t\r\n") || target == "" {
		return false
	}
	hasLetter := false
	hasDigit := false
	for _, r := range target {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z':
			hasLetter = true
		case r >= '0' && r <= '9':
			hasDigit = true
		default:
			return false
		}
	}
	return hasLetter && hasDigit
}

func clickSearchTriggerOrHotkeyAction() chromedp.Action {
	return chromedp.ActionFunc(func(ctx context.Context) error {
		if err := chromedp.Run(ctx, chromedp.Click(searchTriggerSelector, chromedp.ByQuery)); err == nil {
			return nil
		}
		return chromedp.Run(ctx, chromedp.KeyEvent(kb.Meta+"k"))
	})
}

func clickSearchEditorAction() chromedp.Action {
	return chromedp.ActionFunc(func(ctx context.Context) error {
		clicked := false
		if err := chromedp.Run(ctx, chromedp.Evaluate(`(function(selector) {
			const el = document.querySelector(selector);
			if (!el) return false;
			el.click();
			return true;
		})(`+jsArgs(searchEditorSelector)+`)`, &clicked)); err != nil {
			return err
		}
		if clicked {
			return nil
		}
		return chromedp.Run(ctx, chromedp.MouseClickXY(520, 92))
	})
}

func searchStateAction(query string, result *searchState) chromedp.Action {
	return chromedp.Evaluate(`(function(query, searchTriggerSelector, searchEditorSelector, userResultSelector) {
		const pieces = [];
		for (const selector of ['.serp-nav-header', '.serp-search-bar', '.search-base-editor', '.zone-container.editor-kit-container', '.ace-line']) {
			for (const el of Array.from(document.querySelectorAll(selector))) {
				const text = (el.innerText || el.textContent || '').trim();
				if (text) pieces.push(text);
			}
		}
		const active = document.activeElement;
		const activeText = active ? (active.innerText || active.textContent || '').trim() : '';
		if (activeText) pieces.push(activeText);
		const activeClass = active ? String(active.className || '').slice(0, 120) : '';
		const activeName = active ? String(active.tagName || '') : '';
		const text = pieces.join('\n');
		return {
			searchTriggerCount: document.querySelectorAll(searchTriggerSelector).length,
			searchEditorCount: document.querySelectorAll(searchEditorSelector).length,
			userResultCount: document.querySelectorAll(userResultSelector).length,
			activeElement: activeName + (activeClass ? '.' + activeClass : ''),
			textLength: text.length,
			queryPresent: text.includes(query),
		};
	})(`+jsArgs(query, searchTriggerSelector, searchEditorSelector, userResultSelector)+`)`, result)
}

func clickMatchingUserCardAction(target string, clicked *bool) chromedp.Action {
	return chromedp.Evaluate(`(function(cardSelector, target) {
		const normalize = (value) => String(value || '').toLowerCase().replace(/[^0-9a-zA-Z\u4e00-\u9fff]+/g, '');
		const parts = String(target || '').split(/\s+/).filter(Boolean);
		const matches = (text) => {
			const cardKey = normalize(text);
			if (!cardKey || parts.length === 0) return false;
			if (parts.length === 1) return cardKey.includes(normalize(parts[0]));
			return parts.every((part) => {
				const key = normalize(part);
				return key && cardKey.includes(key);
			});
		};
		for (const card of Array.from(document.querySelectorAll(cardSelector))) {
			const text = (card.innerText || card.textContent || '').trim();
			if (matches(text)) {
				card.click();
				return true;
			}
		}
		return false;
	})(`+jsArgs(userResultSelector, target)+`)`, clicked)
}

func scrollRecentToTopAction() chromedp.Action {
	return chromedp.Evaluate(`(function(scrollSelector) {
		const scroll = document.querySelector(scrollSelector);
		if (scroll) scroll.scrollTop = 0;
	})(`+jsArgs(recentScrollSelector)+`)`, nil)
}

func clickVisibleRecentCardAction(target string, clicked *bool) chromedp.Action {
	return chromedp.Evaluate(`(function(target, cardSelector, titleSelector) {
		const normalize = (value) => String(value || '').toLowerCase().replace(/[^0-9a-zA-Z\u4e00-\u9fff]+/g, '');
		const expected = normalize(target);
		for (const card of Array.from(document.querySelectorAll(cardSelector))) {
			const title = (card.querySelector(titleSelector)?.innerText || '').trim();
			if (normalize(title) === expected) {
				card.click();
				return true;
			}
		}
		return false;
	})(`+jsArgs(target, feedCardSelector, feedTitleSelector)+`)`, clicked)
}

func clickVisibleRecentCardLooseAction(target string, clicked *bool) chromedp.Action {
	return chromedp.Evaluate(`(function(target, cardSelector, titleSelector) {
		const normalize = (value) => String(value || '').toLowerCase().replace(/[^0-9a-zA-Z\u4e00-\u9fff]+/g, '');
		const expected = normalize(target);
		for (const card of Array.from(document.querySelectorAll(cardSelector))) {
			const title = (card.querySelector(titleSelector)?.innerText || '').trim();
			const actual = normalize(title);
			if (actual && expected && (actual === expected || actual.includes(expected) || expected.includes(actual))) {
				card.click();
				return true;
			}
		}
		return false;
	})(`+jsArgs(target, feedCardSelector, feedTitleSelector)+`)`, clicked)
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

type conversationCard struct {
	Title   string `json:"title"`
	Preview string `json:"preview,omitempty"`
	Time    string `json:"time,omitempty"`
	Unread  int    `json:"unread"`
}

func collectRecentCardsAction(maxScrolls int, result *[]conversationCard) chromedp.Action {
	if maxScrolls <= 0 {
		maxScrolls = 18
	}
	return chromedp.ActionFunc(func(ctx context.Context) error {
		seen := map[string]bool{}
		cards := []conversationCard{}
		stagnant := 0
		for index := 0; index < maxScrolls; index++ {
			before := len(cards)
			var visible []conversationCard
			if err := chromedp.Run(ctx, chromedp.Evaluate(collectRecentCardsExpression(maxScrolls), &visible)); err != nil {
				return err
			}
			for _, card := range visible {
				key := matchKey(card.Title)
				if key == "" || seen[key] {
					continue
				}
				seen[key] = true
				cards = append(cards, card)
			}
			if len(cards) == before {
				stagnant++
			} else {
				stagnant = 0
			}
			if stagnant >= 3 {
				break
			}
			var scrolled bool
			if err := chromedp.Run(ctx, scrollRecentCardsAction(&scrolled)); err != nil {
				return err
			}
			if !scrolled {
				break
			}
			if err := chromedp.Run(ctx, chromedp.Sleep(400*time.Millisecond)); err != nil {
				return err
			}
		}
		*result = cards
		return nil
	})
}

func collectRecentCardsExpression(maxScrolls int) string {
	return `(function(cardSelector, titleSelector, previewSelector, timeSelector) {
		const normalize = (value) => String(value || '').toLowerCase().replace(/[^0-9a-zA-Z\u4e00-\u9fff]+/g, '');
		const parseUnread = (card) => {
			const badge = card.querySelector('sup');
			if (!badge) return 0;
			const text = (badge.innerText || '').trim();
			const digits = text.match(/\d+/);
			return digits ? Number(digits[0]) : 1;
		};
		const seen = new Set();
		const cards = [];
		for (const card of Array.from(document.querySelectorAll(cardSelector))) {
			const title = (card.querySelector(titleSelector)?.innerText || '').trim();
			const key = normalize(title);
			if (!key || seen.has(key)) continue;
			seen.add(key);
			cards.push({
				title,
				preview: (card.querySelector(previewSelector)?.innerText || '').trim(),
				time: (card.querySelector(timeSelector)?.innerText || '').trim(),
				unread: parseUnread(card),
			});
		}
		return cards;
	})(` + jsArgs(feedCardSelector, feedTitleSelector, feedPreviewSelector, feedTimeSelector) + `)`
}

func scrollRecentCardsAction(scrolled *bool) chromedp.Action {
	return chromedp.Evaluate(`(function(scrollSelector) {
		const scroll = document.querySelector(scrollSelector);
		if (!scroll) return false;
		const before = scroll.scrollTop;
		scroll.scrollBy(0, Math.max(scroll.clientHeight * 0.9, 480));
		return scroll.scrollTop !== before;
	})(`+jsArgs(recentScrollSelector)+`)`, scrolled)
}

func readMessageItemsAction(limit int, includeSelf bool, result *[]MessageRead) chromedp.Action {
	if limit <= 0 {
		limit = 5
	}
	scanLimit := limit * 3
	if scanLimit < 8 {
		scanLimit = 8
	}
	return chromedp.Evaluate(`(function(messageSelector, scanLimit, keepLimit, includeSelf) {
		const collapseLines = (value) => String(value || '')
			.replace(/\u200b/g, '\n')
			.replace(/\u00a0/g, ' ')
			.split(/\n+/)
			.map((line) => line.replace(/[ \t\r\f\v]+/g, ' ').trim())
			.filter(Boolean)
			.join('\n');
		const items = Array.from(document.querySelectorAll(messageSelector));
		const selected = items.slice(Math.max(0, items.length - scanLimit));
		const extracted = [];
		for (const item of selected) {
			const className = item.getAttribute('class') || '';
			const text = collapseLines(item.innerText || item.textContent || '');
			if (!text || /^\d+$/.test(text)) continue;
			let side = 'other';
			if (className.includes('message-self')) side = 'self';
			if (className.includes('system-text-background')) side = 'system';
			if (side === 'system') continue;
			if (side === 'self' && !includeSelf) continue;
			extracted.push({
				side,
				text,
			});
		}
		return extracted.slice(Math.max(0, extracted.length - keepLimit));
	})(`+jsArgs(messageItemSelector)+`, `+fmt.Sprint(scanLimit)+`, `+fmt.Sprint(limit)+`, `+fmt.Sprint(includeSelf)+`)`, result)
}

func readPanelTextAction(maxChars int, result *string) chromedp.Action {
	if maxChars <= 0 {
		maxChars = 1500
	}
	return chromedp.Evaluate(`(function(panelSelector, maxChars) {
		const panel = document.querySelector(panelSelector);
		if (!panel) return '';
		return String(panel.innerText || panel.textContent || '')
			.replace(/\u200b/g, '\n')
			.replace(/\u00a0/g, ' ')
			.split(/\n+/)
			.map((line) => line.replace(/[ \t\r\f\v]+/g, ' ').trim())
			.filter(Boolean)
			.join('\n')
			.slice(0, maxChars);
	})(`+jsArgs(rightPanelSelector)+`, `+fmt.Sprint(maxChars)+`)`, result)
}

func sendMessageAction(message string) chromedp.Action {
	return chromedp.ActionFunc(func(ctx context.Context) error {
		clicked := false
		if err := chromedp.Run(ctx,
			chromedp.Click(chatEditorSelector, chromedp.ByQuery),
			chromedp.Sleep(200*time.Millisecond),
			chromedp.KeyEvent(kb.Meta+"a"),
			chromedp.Sleep(100*time.Millisecond),
			chromedp.KeyEvent(kb.Backspace),
			chromedp.Sleep(200*time.Millisecond),
			insertTextAction(message),
			chromedp.Sleep(500*time.Millisecond),
			clickSendButtonAction(&clicked),
		); err != nil {
			return err
		}
		if clicked {
			return nil
		}
		return chromedp.Run(ctx, chromedp.KeyEvent(kb.Enter))
	})
}

func insertTextAction(text string) chromedp.Action {
	return chromedp.ActionFunc(func(ctx context.Context) error {
		return cdpinput.InsertText(text).Do(ctx)
	})
}

func clickSendButtonAction(clicked *bool) chromedp.Action {
	return chromedp.Evaluate(`(function(wrapperSelector, buttonSelector) {
		const wrapper = document.querySelector(wrapperSelector);
		if (!wrapper) return false;
		const className = wrapper.getAttribute('class') || '';
		if (className.includes('toolbar-item--disabled')) return false;
		const button = document.querySelector(buttonSelector) || wrapper;
		button.click();
		return true;
	})(`+jsArgs(sendWrapperSelector, sendButtonSelector)+`)`, clicked)
}

func waitForSelfMessageAction(message string, timeout time.Duration, matched *bool) chromedp.Action {
	return chromedp.PollFunction(`(messageSelector, expected) => {
		const collapseLines = (value) => String(value || '')
			.replace(/\u200b/g, '\n')
			.replace(/\u00a0/g, ' ')
			.split(/\n+/)
			.map((line) => line.replace(/[ \t\r\f\v]+/g, ' ').trim())
			.filter(Boolean)
			.join('\n');
		const items = Array.from(document.querySelectorAll(messageSelector));
		for (const item of items.slice(Math.max(0, items.length - 30)).reverse()) {
			const className = item.getAttribute('class') || '';
			if (!className.includes('message-self')) continue;
			const text = collapseLines(item.innerText || item.textContent || '');
			if (expected && text.includes(expected)) return true;
		}
		return false;
	}`, matched,
		chromedp.WithPollingArgs(messageItemSelector, message),
		chromedp.WithPollingTimeout(timeout),
	)
}

func filterConversationCards(cards []conversationCard, scope string, limit int) []conversationCard {
	if limit <= 0 {
		limit = 20
	}
	filtered := make([]conversationCard, 0, len(cards))
	for _, card := range cards {
		if scope == "unread" && card.Unread <= 0 {
			continue
		}
		filtered = append(filtered, card)
		if len(filtered) >= limit {
			break
		}
	}
	return filtered
}

func countUnreadCards(cards []conversationCard) int {
	count := 0
	for _, card := range cards {
		if card.Unread > 0 {
			count++
		}
	}
	return count
}

func safeAutomationError(err error) string {
	if err == nil {
		return ""
	}
	text := err.Error()
	if len(text) > 240 {
		text = text[:240]
	}
	return text
}

func jsArgs(values ...string) string {
	encoded := make([]string, 0, len(values))
	for _, value := range values {
		content, _ := json.Marshal(value)
		encoded = append(encoded, string(content))
	}
	return strings.Join(encoded, ",")
}
