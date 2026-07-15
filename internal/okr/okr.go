package okr

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const DefaultCSRFURL = "https://www.xfchat.iflytek.com/lgw/csrf_token"

var errStaleDraftVersion = errors.New("stale OKR draft version")

type ReadConfig struct {
	Source      string
	CookiesPath string
	CSRFURL     string
}

type WriteConfig struct {
	URL            string
	InputPath      string
	CookiesPath    string
	CSRFURL        string
	ObjectiveIndex int
	Apply          bool
}

type cookieObject struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Domain string `json:"domain"`
	Path   string `json:"path"`
}

type ObjectiveSpec struct {
	Objective string
	KRs       []string
}

func Read(config ReadConfig) (string, error) {
	if !DetectURL(config.Source) {
		return "", fmt.Errorf("source is not an OKR page URL")
	}
	okrID, err := IDFromURL(config.Source)
	if err != nil {
		return "", err
	}
	origin, err := OriginFor(config.Source)
	if err != nil {
		return "", err
	}
	cookies, err := loadCookieObjects(config.CookiesPath)
	if err != nil {
		return "", err
	}
	csrfURL := config.CSRFURL
	if csrfURL == "" {
		csrfURL = DefaultCSRFURL
	}
	client := &http.Client{Timeout: 30 * time.Second}
	lgwToken, cookies, err := ensureLGWCSRFToken(client, csrfURL, cookies)
	if err != nil {
		return "", err
	}
	detail, err := getDetail(client, origin, config.Source, okrID, lgwToken, cookies)
	if err != nil {
		return "", err
	}
	return RenderMarkdown(detail, okrID), nil
}

func WriteDryRun(config WriteConfig) (map[string]any, error) {
	if !DetectURL(config.URL) {
		return nil, fmt.Errorf("source is not an OKR page URL")
	}
	okrID, err := IDFromURL(config.URL)
	if err != nil {
		return nil, err
	}
	specs, err := ParseSpecs(config.InputPath)
	if err != nil {
		return nil, err
	}
	if config.Apply {
		return WriteObjectiveIndex(config, okrID, specs)
	}
	items := make([]map[string]any, 0, len(specs))
	for index, spec := range specs {
		items = append(items, map[string]any{
			"index":     index + 1,
			"objective": spec.Objective,
			"krCount":   len(spec.KRs),
			"action":    "plan",
		})
	}
	return map[string]any{
		"ok":                   true,
		"dryRun":               true,
		"okrId":                okrID,
		"targetObjectiveIndex": config.ObjectiveIndex,
		"objectives":           items,
		"applySupported":       true,
	}, nil
}

func WriteObjectiveIndex(config WriteConfig, okrID string, specs []ObjectiveSpec) (map[string]any, error) {
	if config.ObjectiveIndex <= 0 {
		return nil, fmt.Errorf("--objective-index is required for Go OKR write --apply")
	}
	if len(specs) != 1 {
		return nil, fmt.Errorf("--objective-index requires exactly one Objective")
	}
	origin, err := OriginFor(config.URL)
	if err != nil {
		return nil, err
	}
	cookies, err := loadCookieObjects(config.CookiesPath)
	if err != nil {
		return nil, err
	}
	csrfURL := config.CSRFURL
	if csrfURL == "" {
		csrfURL = DefaultCSRFURL
	}
	client := &http.Client{Timeout: 30 * time.Second}
	lgwToken, cookies, err := ensureLGWCSRFToken(client, csrfURL, cookies)
	if err != nil {
		return nil, err
	}
	detail, err := getDetail(client, origin, config.URL, okrID, lgwToken, cookies)
	if err != nil {
		return nil, err
	}
	objectives := asSlice(firstValue(detail, "objective_list", "objectiveList"))
	createTarget := config.ObjectiveIndex == len(objectives)+1
	if config.ObjectiveIndex > len(objectives)+1 {
		return nil, fmt.Errorf("--objective-index %d cannot skip positions; current objective count is %d", config.ObjectiveIndex, len(objectives))
	}
	objectiveID := ""
	oldKRIDs := []string{}
	if !createTarget {
		target := asMap(objectives[config.ObjectiveIndex-1])
		objectiveID = asString(target["id"])
		if objectiveID == "" {
			return nil, fmt.Errorf("target Objective did not include an identifier")
		}
		for _, rawKR := range asSlice(firstValue(target, "kr_list", "krList")) {
			if id := asString(asMap(rawKR)["id"]); id != "" {
				oldKRIDs = append(oldKRIDs, id)
			}
		}
	}
	connID := fmt.Sprintf("%d", time.Now().UnixNano())
	versionCache := &draftVersionCache{}
	spec := specs[0]
	if createTarget {
		createPayload, err := okrAPIWithVersion(client, "POST", origin, config.URL, okrID, "/okrx/api/draft_v2/objective/", lgwToken, cookies, versionCache, connID, func(version string, conn string) map[string]any {
			return map[string]any{
				"draft_version": version,
				"conn_uuid":     conn,
				"token":         conn,
				"name":          deltaDocJSON(""),
				"changesets":    "[]",
				"okr_id":        okrID,
			}
		})
		if err != nil {
			return nil, err
		}
		objectiveID = firstString(asMap(createPayload["data"]), "objective_id", "objectiveId")
		if objectiveID == "" {
			return nil, fmt.Errorf("Objective creation did not return an identifier")
		}
	} else {
		if _, err := okrAPIWithVersion(client, "POST", origin, config.URL, okrID, "/okrx/api/draft_v2/enable/"+objectiveID+"/", lgwToken, cookies, versionCache, connID, func(version string, conn string) map[string]any {
			return draftBody(version, conn)
		}); err != nil {
			return nil, err
		}
	}
	if _, err := okrAPIWithVersion(client, "PUT", origin, config.URL, okrID, "/okrx/api/draft_v2/objective/"+objectiveID+"/", lgwToken, cookies, versionCache, connID, func(version string, conn string) map[string]any {
		return map[string]any{
			"draft_version": version,
			"conn_uuid":     conn,
			"name":          deltaDocJSON(spec.Objective),
			"changesets":    "[]",
		}
	}); err != nil {
		return nil, err
	}
	for _, krID := range oldKRIDs {
		if _, err := okrAPI(client, "DELETE", origin, config.URL, "/okrx/api/draft_v2/kr/"+krID+"/", lgwToken, cookies, nil); err != nil {
			return nil, err
		}
	}
	for _, text := range spec.KRs {
		createPayload, err := okrAPIWithVersion(client, "POST", origin, config.URL, okrID, "/okrx/api/draft_v2/kr/", lgwToken, cookies, versionCache, connID, func(version string, conn string) map[string]any {
			return map[string]any{
				"draft_version": version,
				"conn_uuid":     conn,
				"objective_id":  objectiveID,
				"content":       deltaDocJSON(""),
				"changesets":    "[]",
			}
		})
		if err != nil {
			return nil, err
		}
		krID := firstString(asMap(createPayload["data"]), "kr_id", "krId")
		if krID == "" {
			return nil, fmt.Errorf("KR creation did not return an identifier")
		}
		if _, err := okrAPIWithVersion(client, "PUT", origin, config.URL, okrID, "/okrx/api/draft_v2/kr/"+krID+"/", lgwToken, cookies, versionCache, connID, func(version string, conn string) map[string]any {
			return map[string]any{
				"draft_version": version,
				"conn_uuid":     conn,
				"content":       deltaDocJSON(text),
				"changesets":    "[]",
			}
		}); err != nil {
			return nil, err
		}
	}
	if _, err := okrAPIWithVersion(client, "POST", origin, config.URL, okrID, "/okrx/api/draft_v2/publish/"+objectiveID+"/", lgwToken, cookies, versionCache, connID, func(version string, conn string) map[string]any {
		return map[string]any{
			"draft_version":      version,
			"conn_uuid":          conn,
			"need_delete_kr_ids": oldKRIDs,
			"auto_notify":        false,
		}
	}); err != nil {
		return nil, err
	}
	finalDetail, err := getDetail(client, origin, config.URL, okrID, lgwToken, cookies)
	if err != nil {
		return nil, err
	}
	finalObjectives := asSlice(firstValue(finalDetail, "objective_list", "objectiveList"))
	if config.ObjectiveIndex > len(finalObjectives) {
		return nil, fmt.Errorf("O%d was not found after writing", config.ObjectiveIndex)
	}
	finalTarget := asMap(finalObjectives[config.ObjectiveIndex-1])
	actualObjective := itemText(finalTarget)
	actualKRs := []map[string]string{}
	for _, rawKR := range asSlice(firstValue(finalTarget, "kr_list", "krList")) {
		actualKRs = append(actualKRs, map[string]string{"text": itemText(asMap(rawKR))})
	}
	expectedKRs := make([]string, 0, len(spec.KRs))
	for _, item := range actualKRs {
		expectedKRs = append(expectedKRs, item["text"])
	}
	if actualObjective != spec.Objective || strings.Join(expectedKRs, "\n") != strings.Join(spec.KRs, "\n") {
		return nil, fmt.Errorf("O%d content did not match after writing", config.ObjectiveIndex)
	}
	return map[string]any{
		"ok":     true,
		"dryRun": false,
		"target": map[string]any{
			"objective": actualObjective,
			"krs":       actualKRs,
		},
	}, nil
}

func DetectURL(source string) bool {
	parsed, err := url.Parse(source)
	return err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") && strings.Contains(parsed.Path, "/okr/user/")
}

func OriginFor(source string) (string, error) {
	parsed, err := url.Parse(source)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("OKR URL must be an absolute HTTP(S) URL")
	}
	return parsed.Scheme + "://" + parsed.Host, nil
}

func IDFromURL(source string) (string, error) {
	parsed, err := url.Parse(source)
	if err != nil {
		return "", err
	}
	query := parsed.Query()
	for _, key := range []string{"okrId", "okr_id"} {
		if value := query.Get(key); value != "" {
			return value, nil
		}
	}
	return "", fmt.Errorf("unable to locate okrId in OKR URL")
}

func ParseSpecs(path string) ([]ObjectiveSpec, error) {
	content, err := os.ReadFile(expandUser(path))
	if err != nil {
		return nil, err
	}
	raw := struct {
		Objectives []struct {
			Objective string   `json:"objective"`
			KRs       []string `json:"krs"`
		} `json:"objectives"`
	}{}
	if err := json.Unmarshal(content, &raw); err != nil {
		return nil, fmt.Errorf("OKR input must be valid JSON")
	}
	if len(raw.Objectives) == 0 {
		return nil, fmt.Errorf("OKR input contains no objectives")
	}
	specs := make([]ObjectiveSpec, 0, len(raw.Objectives))
	for index, item := range raw.Objectives {
		objective := strings.TrimSpace(item.Objective)
		if objective == "" {
			return nil, fmt.Errorf("objective %d is empty", index+1)
		}
		if len(item.KRs) > 4 {
			return nil, fmt.Errorf("objective %d has %d KRs; keep OKR scope realistic", index+1, len(item.KRs))
		}
		krs := []string{}
		for _, kr := range item.KRs {
			trimmed := strings.TrimSpace(kr)
			if trimmed != "" {
				krs = append(krs, trimmed)
			}
		}
		specs = append(specs, ObjectiveSpec{Objective: objective, KRs: krs})
	}
	return specs, nil
}

func getDetail(client *http.Client, origin string, source string, okrID string, lgwToken string, cookies []http.Cookie) (map[string]any, error) {
	query := url.Values{"okr_id": {okrID}, "withoutAddVisitLog": {"true"}}
	request, err := http.NewRequest("GET", origin+"/okrx/api/okr/owner/aggr_detail/?"+query.Encode(), nil)
	if err != nil {
		return nil, err
	}
	addLGWHeaders(request, origin, source, lgwToken, cookies)
	payload, err := doJSON(client, request, "OKR aggr_detail")
	if err != nil {
		return nil, err
	}
	if code, ok := payload["code"]; ok && asInt(code) != 0 {
		return nil, fmt.Errorf("OKR aggr_detail failed with code %d", asInt(code))
	}
	for _, key := range []string{"okr_detail_data", "okrDetailData", "data"} {
		if detail := asMap(payload[key]); len(detail) > 0 {
			return detail, nil
		}
	}
	return nil, fmt.Errorf("OKR aggr_detail returned an unexpected payload shape")
}

func currentDraftVersion(client *http.Client, origin string, source string, okrID string, lgwToken string, cookies []http.Cookie) (string, error) {
	request, err := http.NewRequest("GET", origin+"/okrx/api/okr/"+okrID+"/version/", nil)
	if err != nil {
		return "", err
	}
	addLGWHeaders(request, origin, source, lgwToken, cookies)
	payload, err := doJSON(client, request, "OKR draft version")
	if err != nil {
		return "", err
	}
	data := asMap(payload["data"])
	version := firstString(data, "okr_draft_version", "okrDraftVersion", "draft_version", "draftVersion", "version")
	if version == "" {
		version = firstString(payload, "okr_draft_version", "okrDraftVersion", "draft_version", "draftVersion", "version")
	}
	if version == "" {
		return "", fmt.Errorf("unable to determine the OKR draft version")
	}
	return version, nil
}

type draftVersionCache struct {
	value string
}

func (cache *draftVersionCache) get(client *http.Client, origin string, source string, okrID string, lgwToken string, cookies []http.Cookie) (string, error) {
	if cache.value != "" {
		return cache.value, nil
	}
	version, err := currentDraftVersion(client, origin, source, okrID, lgwToken, cookies)
	if err != nil {
		return "", err
	}
	cache.value = version
	return version, nil
}

func (cache *draftVersionCache) setFromPayload(payload map[string]any) {
	if version := draftVersionFromPayload(payload); version != "" {
		cache.value = version
	}
}

func (cache *draftVersionCache) clear() {
	cache.value = ""
}

func draftVersionFromPayload(payload map[string]any) string {
	data := asMap(payload["data"])
	if version := firstString(data, "draft_version", "draftVersion", "okr_draft_version", "okrDraftVersion", "version"); version != "" {
		return version
	}
	return firstString(payload, "draft_version", "draftVersion", "okr_draft_version", "okrDraftVersion", "version")
}

func okrAPIWithVersion(
	client *http.Client,
	method string,
	origin string,
	source string,
	okrID string,
	path string,
	lgwToken string,
	cookies []http.Cookie,
	cache *draftVersionCache,
	connID string,
	makeBody func(version string, connID string) map[string]any,
) (map[string]any, error) {
	for attempt := 0; attempt < 3; attempt++ {
		version, err := cache.get(client, origin, source, okrID, lgwToken, cookies)
		if err != nil {
			return nil, err
		}
		payload, err := okrAPI(client, method, origin, source, path, lgwToken, cookies, makeBody(version, connID))
		if errors.Is(err, errStaleDraftVersion) {
			cache.clear()
			continue
		}
		if err != nil {
			return nil, err
		}
		cache.setFromPayload(payload)
		return payload, nil
	}
	version, err := cache.get(client, origin, source, okrID, lgwToken, cookies)
	if err != nil {
		return nil, err
	}
	payload, err := okrAPI(client, method, origin, source, path, lgwToken, cookies, makeBody(version, connID))
	if err != nil {
		return nil, err
	}
	cache.setFromPayload(payload)
	return payload, nil
}

func okrAPI(client *http.Client, method string, origin string, source string, path string, lgwToken string, cookies []http.Cookie, body map[string]any) (map[string]any, error) {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(raw)
	}
	request, err := http.NewRequest(method, origin+path, reader)
	if err != nil {
		return nil, err
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json;charset=UTF-8")
	}
	addLGWHeaders(request, origin, source, lgwToken, cookies)
	payload, err := doJSON(client, request, method+" "+path)
	if err != nil {
		return nil, err
	}
	if code, ok := payload["code"]; ok && asInt(code) != 0 {
		if asInt(code) == 100001 {
			return nil, errStaleDraftVersion
		}
		return nil, fmt.Errorf("%s %s failed with code %d", method, path, asInt(code))
	}
	if success, ok := payload["success"].(bool); ok && !success {
		return nil, fmt.Errorf("%s %s failed", method, path)
	}
	return payload, nil
}

func draftBody(version string, connID string) map[string]any {
	return map[string]any{
		"draft_version": version,
		"conn_uuid":     connID,
		"token":         connID,
	}
}

func deltaDocJSON(text string) string {
	raw, _ := json.Marshal(map[string]any{
		"0": map[string]any{
			"ops":      []map[string]string{{"insert": text + "\n"}},
			"zoneId":   "0",
			"zoneType": "Z",
		},
	})
	return string(raw)
}

func ensureLGWCSRFToken(client *http.Client, csrfURL string, cookies []http.Cookie) (string, []http.Cookie, error) {
	request, err := http.NewRequest("GET", csrfURL, nil)
	if err != nil {
		return "", nil, err
	}
	for _, cookie := range cookies {
		request.AddCookie(&cookie)
	}
	response, err := client.Do(request)
	if err != nil {
		return "", nil, fmt.Errorf("unable to obtain lgw_csrf_token from local session cookies")
	}
	defer response.Body.Close()
	io.Copy(io.Discard, response.Body)
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", nil, fmt.Errorf("unable to obtain lgw_csrf_token from local session cookies")
	}
	for _, cookie := range response.Cookies() {
		replaced := false
		for index := range cookies {
			if cookies[index].Name == cookie.Name {
				cookies[index] = *cookie
				replaced = true
				break
			}
		}
		if !replaced {
			cookies = append(cookies, *cookie)
		}
	}
	for _, cookie := range cookies {
		if cookie.Name == "lgw_csrf_token" && cookie.Value != "" {
			return cookie.Value, cookies, nil
		}
	}
	return "", nil, fmt.Errorf("unable to obtain lgw_csrf_token from local session cookies")
}

func addLGWHeaders(request *http.Request, origin string, referer string, token string, cookies []http.Cookie) {
	request.Header.Set("User-Agent", "ixf-toolbox-go")
	request.Header.Set("Origin", origin)
	request.Header.Set("Referer", referer)
	request.Header.Set("accept", "application/json,text/plain,*/*")
	request.Header.Set("x-lgw-csrf-token", token)
	request.Header.Set("x-requested-with", "XMLHttpRequest")
	request.Header.Set("okr-language", "zh-CN")
	request.Header.Set("okr-timezone", "Asia/Shanghai")
	for _, cookie := range cookies {
		request.AddCookie(&cookie)
	}
}

func doJSON(client *http.Client, request *http.Request, label string) (map[string]any, error) {
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("%s request failed", label)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("%s http status %d", label, response.StatusCode)
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}
	payload := map[string]any{}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("%s returned invalid JSON", label)
	}
	return payload, nil
}

func RenderMarkdown(detail map[string]any, okrID string) string {
	period := strings.TrimSpace(firstString(detail, "name", "period_name", "periodName"))
	owner := ownerName(detail)
	titleParts := []string{"OKR"}
	if owner != "" {
		titleParts = append(titleParts, owner)
	}
	if period != "" {
		titleParts = append(titleParts, period)
	}
	title := strings.Join(titleParts, " - ")
	objectives := asSlice(firstValue(detail, "objective_list", "objectiveList"))
	lines := []string{
		"# " + title,
		"",
		fmt.Sprintf("[okr id=%s objectives=%d]", okrID, len(objectives)),
		"",
	}
	for objectiveIndex, rawObjective := range objectives {
		objective := asMap(rawObjective)
		objectiveText := itemText(objective)
		if objectiveText == "" {
			objectiveText = asString(objective["id"])
		}
		lines = append(lines, fmt.Sprintf("## O%d %s", objectiveIndex+1, objectiveText), "")
		krs := asSlice(firstValue(objective, "kr_list", "krList"))
		for krIndex, rawKR := range krs {
			kr := asMap(rawKR)
			krText := itemText(kr)
			if krText == "" {
				krText = asString(kr["id"])
			}
			progress := progressText(firstValue(kr, "progress_rate", "progressRate"))
			suffix := ""
			if progress != "" {
				suffix = " _(progress: " + progress + ")_"
			}
			lines = append(lines, fmt.Sprintf("- KR%d: %s%s", krIndex+1, krText, suffix))
		}
		lines = append(lines, "")
	}
	return strings.Join(normalizeLines(lines), "\n") + "\n"
}

func ownerName(detail map[string]any) string {
	owner := asMap(firstValue(detail, "owner_info", "ownerInfo"))
	user := asMap(firstValue(owner, "user_info", "userInfo"))
	locale := asMap(firstValue(user, "locale_names", "localeNames"))
	for _, key := range []string{"zh", "en", "ja"} {
		if value := strings.TrimSpace(asString(locale[key])); value != "" {
			return value
		}
	}
	return strings.TrimSpace(firstString(user, "name", "displayName", "display_name"))
}

func itemText(item map[string]any) string {
	for _, key := range []string{"content_v2", "contentV2", "content", "name"} {
		if text := textFromRichValue(item[key]); text != "" {
			return text
		}
	}
	return ""
}

func textFromRichValue(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(strings.ReplaceAll(typed, "\u200b", ""))
	case map[string]any:
		if blocks := asSlice(typed["blocks"]); len(blocks) > 0 {
			parts := []string{}
			for _, raw := range blocks {
				text := strings.TrimSpace(asString(asMap(raw)["text"]))
				if text != "" {
					parts = append(parts, text)
				}
			}
			return strings.TrimSpace(strings.Join(parts, "\n"))
		}
		if zone := asMap(typed["0"]); len(zone) > 0 {
			parts := []string{}
			for _, raw := range asSlice(zone["ops"]) {
				parts = append(parts, asString(asMap(raw)["insert"]))
			}
			return strings.TrimSpace(strings.Join(parts, ""))
		}
		if nested, ok := typed["text"]; ok {
			return textFromRichValue(nested)
		}
	}
	return ""
}

func progressText(value any) string {
	progress := asMap(value)
	if len(progress) == 0 {
		return ""
	}
	raw, ok := progress["percent"]
	if !ok {
		return ""
	}
	switch typed := raw.(type) {
	case float64:
		if typed == float64(int64(typed)) {
			return fmt.Sprintf("%d%%", int64(typed))
		}
		return strconv.FormatFloat(typed, 'f', -1, 64) + "%"
	case string:
		if typed == "" {
			return ""
		}
		return typed + "%"
	default:
		return ""
	}
}

func normalizeLines(lines []string) []string {
	out := []string{}
	blankRun := 0
	for _, line := range lines {
		clean := strings.TrimRight(line, " \t")
		if strings.TrimSpace(clean) == "" {
			blankRun++
			if blankRun <= 1 {
				out = append(out, "")
			}
			continue
		}
		blankRun = 0
		out = append(out, clean)
	}
	for len(out) > 0 && strings.TrimSpace(out[len(out)-1]) == "" {
		out = out[:len(out)-1]
	}
	return out
}

func loadCookieObjects(path string) ([]http.Cookie, error) {
	content, err := os.ReadFile(expandUser(path))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("cookie file not found: %s", expandUser(path))
		}
		return nil, err
	}
	objects := []cookieObject{}
	if err := json.Unmarshal(content, &objects); err != nil {
		return nil, fmt.Errorf("cookie file invalid: %s", expandUser(path))
	}
	cookies := make([]http.Cookie, 0, len(objects))
	for _, object := range objects {
		if object.Name == "" {
			continue
		}
		path := object.Path
		if path == "" {
			path = "/"
		}
		cookies = append(cookies, http.Cookie{
			Name:   object.Name,
			Value:  object.Value,
			Domain: object.Domain,
			Path:   path,
		})
	}
	return cookies, nil
}

func firstValue(source map[string]any, keys ...string) any {
	for _, key := range keys {
		if value, ok := source[key]; ok {
			return value
		}
	}
	return nil
}

func firstString(source map[string]any, keys ...string) string {
	return asString(firstValue(source, keys...))
}

func asMap(value any) map[string]any {
	if typed, ok := value.(map[string]any); ok {
		return typed
	}
	return map[string]any{}
}

func asSlice(value any) []any {
	if typed, ok := value.([]any); ok {
		return typed
	}
	return []any{}
}

func asString(value any) string {
	if typed, ok := value.(string); ok {
		return typed
	}
	return ""
}

func asInt(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	default:
		return 0
	}
}

func expandUser(path string) string {
	if path == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			return home
		}
	}
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return home + path[1:]
		}
	}
	return path
}
