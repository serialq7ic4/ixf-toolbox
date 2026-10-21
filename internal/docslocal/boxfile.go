package docslocal

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// boxFastPDFPreviewType is the preview the server builds for an uploaded file: page
// images plus a text layer. The frontend names it FAST_PDF.
const boxFastPDFPreviewType = "22"

// boxObjectType is the permission service's type code for an uploaded file.
const boxObjectType = 12

// boxPermissionAllowed is the permission service's value for an action the caller
// may perform. Everything else is treated as not allowed: a denial is reported as
// 2, and an owner-imposed restriction on copying as 102, but the set of codes is
// the server's to extend, so only the one known affirmative is trusted.
const boxPermissionAllowed = 1

// boxCopyRestriction explains why a copy-restricted file is refused by default.
//
// The file's owner disabled copying. Viewing is still allowed, so the caller can
// read the file on screen, but rendering its text into a local file is the copy
// the owner turned off. The default respects that; IgnoreCopyRestriction is the
// caller's explicit decision to read it anyway, and the result says so.
const boxCopyRestriction = "the owner has disabled copying for this file; it can be viewed but not copied. " +
	"Pass --ignore-copy-restriction to read its text anyway"

// readBoxFile reads an uploaded file through the server's text layer.
//
// An uploaded file is not built from document blocks, so there is nothing for the
// docx path to convert. The server does convert most uploads for preview, into
// page images and a text layer, and that layer is what is read here. The original
// file is never fetched.
func (session *remoteReadSession) readBoxFile(source string, origin string, path string) (Result, error) {
	token := tokenAfter(path, "/file/")
	if token == "" {
		return Result{}, fmt.Errorf("file URL is missing its file token")
	}

	actions, err := session.boxPermissions(origin, source, token)
	if err != nil {
		// Unknown permission is not permission: refuse rather than read.
		return Result{}, fmt.Errorf("could not confirm permission to read this file: %w", err)
	}
	if actions["view"] != boxPermissionAllowed || actions["preview"] != boxPermissionAllowed {
		return Result{}, fmt.Errorf("you do not have permission to view or preview this file")
	}
	copyRestricted := actions["copy"] != boxPermissionAllowed
	if copyRestricted && !session.ignoreCopyRestriction {
		return Result{}, fmt.Errorf("%s", boxCopyRestriction)
	}

	info, err := session.boxFileInfo(origin, source, token)
	if err != nil {
		return Result{}, err
	}
	dataVersion := strings.TrimSpace(stringValue(info["data_version"]))
	if dataVersion == "" {
		return Result{}, fmt.Errorf("file info did not report a data version")
	}
	if !boxHasTextLayer(info) {
		return Result{}, fmt.Errorf("this %s file has no server-generated text layer to read",
			strings.TrimSpace(stringValue(info["type"])))
	}

	layer, err := session.boxTextLayer(origin, source, token, dataVersion)
	if err != nil {
		return Result{}, err
	}
	body, pages := renderBoxTextLayer(layer)

	title := strings.TrimSpace(stringValue(info["name"]))
	if title == "" {
		title = token
	}
	warnings := []string{}
	lines := []string{"# " + title, ""}
	if copyRestricted {
		// Said in the content as well as in Warnings, so it travels with the text
		// wherever it is pasted rather than living only in a manifest field.
		notice := "Read with --ignore-copy-restriction: the owner disabled copying for this file."
		warnings = append(warnings, notice)
		lines = append(lines, "> "+notice, "")
	}
	lines = append(lines,
		fmt.Sprintf("[file token=%s type=%s pages=%d source=text-layer]",
			token, strings.TrimSpace(stringValue(info["type"])), pages),
		"")
	lines = append(lines, body...)

	counts := map[string]int{"file": 1, "file_pages": pages}
	return Result{
		Source:   source,
		Kind:     "file",
		Title:    title,
		Token:    token,
		Content:  strings.Join(lines, "\n"),
		Counts:   counts,
		Assets:   []map[string]any{},
		Warnings: warnings,
	}, nil
}

// boxPermissions asks the permission service what the caller may do with a file.
func (session *remoteReadSession) boxPermissions(origin string, referer string, token string) (map[string]int, error) {
	payload, err := session.postJSON(
		session.apiHostFor(origin)+"/space/api/suite/permission/document/actions/state/",
		map[string]any{"token": token, "type": boxObjectType, "actions": []string{"view", "preview", "copy"}},
		origin, referer, "file permission")
	if err != nil {
		return nil, err
	}
	if codeNumber(payload["code"]) != 0 {
		return nil, fmt.Errorf("file permission request failed")
	}
	actions := map[string]int{}
	for name, value := range asMap(asMap(payload["data"])["actions"]) {
		actions[name] = intValue(value)
	}
	return actions, nil
}

// boxFileInfo reads a file's metadata, including which previews the server built.
func (session *remoteReadSession) boxFileInfo(origin string, referer string, token string) (map[string]any, error) {
	payload, err := session.postJSON(
		session.apiHostFor(origin)+"/space/api/box/file/info/",
		map[string]any{
			"caller":        "explorer",
			"file_token":    token,
			"mount_point":   "explorer",
			"option_params": []string{"preview_meta"},
		},
		origin, referer, "file info")
	if err != nil {
		return nil, err
	}
	if codeNumber(payload["code"]) != 0 {
		return nil, fmt.Errorf("file info request failed")
	}
	return asMap(payload["data"]), nil
}

// boxHasTextLayer reports whether the FAST_PDF preview finished and carries text.
//
// The preview's metadata is a JSON string inside a JSON field. A status of 0 means
// the conversion completed; has_txt_all is set only when a text layer was produced,
// which an image-only scan, for one, will not have.
func boxHasTextLayer(info map[string]any) bool {
	preview := asMap(asMap(asMap(info["preview_meta"])["data"])[boxFastPDFPreviewType])
	if len(preview) == 0 || intValue(preview["status"]) != 0 {
		return false
	}
	extra := map[string]any{}
	if raw := strings.TrimSpace(stringValue(preview["extra"])); raw != "" {
		if err := json.Unmarshal([]byte(raw), &extra); err != nil {
			return false
		}
	}
	return readBool(extra["has_txt_all"])
}

// boxTextLayer fetches the text layer the server extracted for preview.
//
// The response is the layer itself, {"page": ..., "link": ...}, with no code/data
// envelope.
func (session *remoteReadSession) boxTextLayer(origin string, referer string, token string, dataVersion string) (map[string]any, error) {
	query := url.Values{}
	query.Set("preview_type", boxFastPDFPreviewType)
	query.Set("sub_id", "txt_all.json")
	query.Set("version", dataVersion)
	requestURL := session.apiHostFor(origin) + "/space/api/box/stream/download/preview_sub/" +
		url.PathEscape(token) + "?" + query.Encode()
	payload, err := session.getJSON(requestURL, origin, referer)
	if err != nil {
		return nil, fmt.Errorf("file text layer: %w", err)
	}
	if _, ok := payload["page"]; !ok {
		return nil, fmt.Errorf("file text layer response carried no pages")
	}
	return payload, nil
}

// postJSON sends a JSON body and decodes a JSON reply.
//
// label names the request in errors, since a status code alone does not say which
// of several calls failed.
func (session *remoteReadSession) postJSON(requestURL string, body map[string]any, origin string, referer string, label string) (map[string]any, error) {
	encoded, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequest(http.MethodPost, requestURL, bytes.NewReader(encoded))
	if err != nil {
		return nil, err
	}
	session.addCommonHeaders(request, origin, referer)
	request.Header.Set("Content-Type", "application/json")
	response, err := session.client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("%s http status %d", label, response.StatusCode)
	}
	content, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}
	payload := map[string]any{}
	if err := json.Unmarshal(content, &payload); err != nil {
		return nil, fmt.Errorf("%s returned invalid JSON", label)
	}
	return payload, nil
}
