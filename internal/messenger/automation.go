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
	return chromedp.Evaluate(`(async function(cardSelector, titleSelector, previewSelector, timeSelector, scrollSelector, maxScrolls) {
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
		const collect = () => {
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
		};
		let stagnant = 0;
		for (let index = 0; index < maxScrolls; index += 1) {
			const before = cards.length;
			collect();
			stagnant = cards.length === before ? stagnant + 1 : 0;
			if (stagnant >= 3) break;
			const scroll = document.querySelector(scrollSelector);
			if (!scroll) break;
			scroll.scrollBy(0, Math.max(scroll.clientHeight * 0.9, 480));
			await new Promise((resolve) => setTimeout(resolve, 400));
		}
		return cards;
	})(`+jsArgs(feedCardSelector, feedTitleSelector, feedPreviewSelector, feedTimeSelector, recentScrollSelector)+`, `+fmt.Sprint(maxScrolls)+`)`, result)
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
