package messenger

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTitlesMatchNormalizesWhitespaceCaseAndPunctuation(t *testing.T) {
	tests := []struct {
		actual   string
		expected string
		match    bool
	}{
		{actual: " 示例 群聊 ", expected: "示例群聊", match: true},
		{actual: "Alice Zhang azhang1", expected: "alice zhang", match: true},
		{actual: "产品-研发(周会)", expected: "产品研发周会", match: true},
		{actual: "另一个群", expected: "示例群聊", match: false},
		{actual: "", expected: "示例群聊", match: false},
	}
	for _, test := range tests {
		if got := TitlesMatch(test.actual, test.expected); got != test.match {
			t.Fatalf("TitlesMatch(%q, %q) = %t, want %t", test.actual, test.expected, got, test.match)
		}
	}
}

func TestPersonSearchQueriesIncludesFullQueryAndParts(t *testing.T) {
	got := personSearchQueries("张三 zhangsan1")
	want := []string{"张三 zhangsan1", "张三", "zhangsan1"}

	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("personSearchQueries = %#v, want %#v", got, want)
	}
}

func TestCardMatchesPersonRequiresAllQueryParts(t *testing.T) {
	if !cardMatchesPerson("张三\nzhangsan1\n用户", "张三 zhangsan1") {
		t.Fatal("cardMatchesPerson rejected matching person card")
	}
	if cardMatchesPerson("张三项目群\n群聊", "张三 zhangsan1") {
		t.Fatal("cardMatchesPerson accepted a card missing the account part")
	}
	if !cardMatchesPerson("jwwu14\n用户", "jwwu14") {
		t.Fatal("cardMatchesPerson rejected single account match")
	}
}

func TestLooksLikeAccountID(t *testing.T) {
	if !looksLikeAccountID("jwwu14") {
		t.Fatal("looksLikeAccountID rejected account-style target")
	}
	for _, target := range []string{"吴经纬", "吴经纬 jwwu14", "搞定存储"} {
		if looksLikeAccountID(target) {
			t.Fatalf("looksLikeAccountID accepted non-account target %q", target)
		}
	}
}

func TestSearchStateErrorDoesNotLeakQueryOrText(t *testing.T) {
	state := searchState{
		SearchTriggerCount: 1,
		SearchEditorCount:  1,
		UserResultCount:    2,
		ActiveElement:      "DIV.search-base-editor",
		TextLength:         128,
		QueryPresent:       false,
	}

	message := searchStateError("jwwu14", state).Error()

	if strings.Contains(message, "jwwu14") {
		t.Fatalf("searchStateError leaked query: %s", message)
	}
	for _, expected := range []string{"trigger=1", "editor=1", "userCards=2", "active=DIV.search-base-editor", "textLen=128", "queryPresent=false"} {
		if !strings.Contains(message, expected) {
			t.Fatalf("searchStateError missing %q in %s", expected, message)
		}
	}
}

func TestLoadBrowserCookiesParsesPlaywrightCookieJSONWithoutLeakingValues(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cookies.json")
	if err := os.WriteFile(path, []byte(`[
		{"name":"_csrf_token","value":"secret-csrf","domain":".example.test","path":"/","secure":true,"httpOnly":true,"expires":1893456000,"sameSite":"Lax"},
		{"name":"session","value":"secret-session","domain":".example.test","path":"/"}
	]`), 0o600); err != nil {
		t.Fatalf("write cookies: %v", err)
	}

	cookies, err := LoadBrowserCookies(path)

	if err != nil {
		t.Fatalf("LoadBrowserCookies returned error: %v", err)
	}
	if len(cookies) != 2 {
		t.Fatalf("len(cookies) = %d, want 2", len(cookies))
	}
	if cookies[0].Name != "_csrf_token" || cookies[0].Value != "secret-csrf" || cookies[0].SameSite != "Lax" {
		t.Fatalf("first cookie = %+v", cookies[0])
	}

	invalidPath := filepath.Join(t.TempDir(), "invalid.json")
	if err := os.WriteFile(invalidPath, []byte(`{"name":"_csrf_token","value":"secret-csrf"}`), 0o600); err != nil {
		t.Fatalf("write invalid cookies: %v", err)
	}
	_, err = LoadBrowserCookies(invalidPath)
	if err == nil {
		t.Fatal("LoadBrowserCookies accepted non-array JSON")
	}
	if strings.Contains(err.Error(), "secret-csrf") {
		t.Fatalf("LoadBrowserCookies leaked cookie value in error: %v", err)
	}
}

func TestCollapseLinesNormalizesWhitespaceAndRemovesEmptyLines(t *testing.T) {
	got := CollapseLines(" 第一行\u200b\n\n第二\t行 \u00a0 ")
	if got != "第一行\n第二 行" {
		t.Fatalf("CollapseLines = %q", got)
	}
}

func TestParseUnreadBadgeTreatsDotBadgeAsOne(t *testing.T) {
	tests := []struct {
		raw  string
		want int
	}{
		{raw: "", want: 0},
		{raw: "3", want: 3},
		{raw: "99+", want: 99},
		{raw: "•", want: 1},
	}
	for _, test := range tests {
		if got := ParseUnreadBadge(test.raw); got != test.want {
			t.Fatalf("ParseUnreadBadge(%q) = %d, want %d", test.raw, got, test.want)
		}
	}
}

func TestCollectRecentCardsExpressionReturnsArrayNotPromise(t *testing.T) {
	expression := collectRecentCardsExpression(3)

	if strings.Contains(expression, "async function") || strings.Contains(expression, "await ") {
		t.Fatalf("collectRecentCardsExpression must return an array value for chromedp.Evaluate, got Promise expression:\n%s", expression)
	}
	if !strings.Contains(expression, "return cards;") {
		t.Fatalf("collectRecentCardsExpression missing cards array return:\n%s", expression)
	}
}

func TestMessengerBrowserAutoCandidatesExcludeEdge(t *testing.T) {
	for _, candidate := range append(browserCandidates("darwin", Config{}), browserCandidates("windows", Config{})...) {
		lower := strings.ToLower(candidate)
		if strings.Contains(lower, "edge") || strings.Contains(lower, "msedge") {
			t.Fatalf("messenger auto browser candidate should not include Edge: %s", candidate)
		}
	}
	for _, name := range messengerPathBrowserNames() {
		if strings.Contains(strings.ToLower(name), "msedge") {
			t.Fatalf("messenger PATH browser fallback should not include Edge: %s", name)
		}
	}
}
