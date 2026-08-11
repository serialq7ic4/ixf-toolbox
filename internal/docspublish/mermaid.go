package docspublish

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	mermaidRendererName    = "mmdc"
	mermaidPreferredFormat = "svg"
	mermaidFallbackFormat  = "png"
)

type imageSource struct {
	Kind    string
	Text    string
	Ordinal int
}

type renderedImage struct {
	Path     string
	Name     string
	MimeType string
	Size     int64
}

type imageBinding struct {
	BlockID string
	Token   string
	Image   renderedImage
}

func isMermaidFence(info string, text string) bool {
	info = strings.ToLower(strings.TrimSpace(info))
	if info == "mermaid" {
		return true
	}
	if info != "" && info != "plain" && info != "plain text" && info != "plaintext" {
		return false
	}
	first := firstNonEmptyLine(text)
	return isMermaidStart(first)
}

func firstNonEmptyLine(text string) string {
	for _, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return line
		}
	}
	return ""
}

func isMermaidStart(line string) bool {
	line = strings.TrimSpace(line)
	keywords := []string{
		"flowchart", "sequenceDiagram", "erDiagram", "graph", "classDiagram",
		"stateDiagram", "stateDiagram-v2", "gantt", "pie", "journey",
		"gitGraph", "mindmap", "timeline", "quadrantChart", "requirementDiagram",
	}
	for _, keyword := range keywords {
		if line == keyword || strings.HasPrefix(line, keyword+" ") || strings.HasPrefix(line, keyword+"\t") {
			return true
		}
	}
	return false
}

func mermaidRendererAvailable() bool {
	_, err := exec.LookPath(mermaidRendererName)
	return err == nil
}

func requireMermaidRendererForApply(specs []Spec) error {
	if countSpecsBySourceKind(specs, "image", "mermaid") == 0 {
		return nil
	}
	if mermaidRendererAvailable() {
		return nil
	}
	return fmt.Errorf("mermaid renderer %q not found in PATH; install Mermaid CLI before applying Mermaid image blocks", mermaidRendererName)
}

func renderMermaid(text string, ordinal int, format string, dir string) (renderedImage, error) {
	rendererPath, err := exec.LookPath(mermaidRendererName)
	if err != nil {
		return renderedImage{}, fmt.Errorf("mermaid renderer %q not found in PATH", mermaidRendererName)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return renderedImage{}, err
	}
	inputPath := filepath.Join(dir, fmt.Sprintf("mermaid-%03d.mmd", ordinal))
	if err := os.WriteFile(inputPath, []byte(text), 0o600); err != nil {
		return renderedImage{}, err
	}
	name := fmt.Sprintf("mermaid-%03d.%s", ordinal, format)
	outputPath := filepath.Join(dir, name)
	command := exec.Command(rendererPath, "-i", inputPath, "-o", outputPath)
	output, err := command.CombinedOutput()
	if err != nil {
		return renderedImage{}, fmt.Errorf("mmdc %s render failed: %s", format, strings.TrimSpace(string(output)))
	}
	info, err := os.Stat(outputPath)
	if err != nil {
		return renderedImage{}, fmt.Errorf("mmdc %s output missing", format)
	}
	if info.Size() == 0 {
		return renderedImage{}, fmt.Errorf("mmdc %s output empty", format)
	}
	return renderedImage{
		Path:     outputPath,
		Name:     name,
		MimeType: mimeTypeForRenderedFormat(format),
		Size:     info.Size(),
	}, nil
}

func mimeTypeForRenderedFormat(format string) string {
	switch format {
	case "svg":
		return "image/svg+xml"
	case "png":
		return "image/png"
	default:
		return "application/octet-stream"
	}
}

func generatedImageEntries(entries []blockEntry) []blockEntry {
	images := []blockEntry{}
	for _, entry := range entries {
		if entry.Image != nil {
			images = append(images, entry)
		}
	}
	return images
}

func (session *publishSession) attachGeneratedImages(pageID string, memberID string, referer string, entries []blockEntry) (int, error) {
	imageEntries := generatedImageEntries(entries)
	if len(imageEntries) == 0 {
		return 0, nil
	}
	state, err := session.clientVarsContainingBlocks(pageID, referer, imageEntryIDs(imageEntries))
	if err != nil {
		return 0, err
	}
	blockMap := asMap(state["block_map"])
	tempDir, err := os.MkdirTemp("", "ixf-mermaid-*")
	if err != nil {
		return 0, err
	}
	defer os.RemoveAll(tempDir)

	bindings := []imageBinding{}
	for _, entry := range imageEntries {
		binding, err := session.renderUploadMermaidImage(pageID, entry, referer, tempDir)
		if err != nil {
			return len(bindings), err
		}
		bindings = append(bindings, binding)
	}
	changeMap := buildImageBindingChangeMap(blockMap, bindings)
	if err := session.writeBlocks(pageID, memberID, changeMap, referer); err != nil {
		return len(bindings), err
	}
	return len(bindings), nil
}

func (session *publishSession) prepareGeneratedImageBlocks(pageID string, referer string, entries []blockEntry) (int, error) {
	imageEntries := generatedImageEntries(entries)
	if len(imageEntries) == 0 {
		return 0, nil
	}
	tempDir, err := os.MkdirTemp("", "ixf-mermaid-*")
	if err != nil {
		return 0, err
	}
	defer os.RemoveAll(tempDir)

	prepared := 0
	for _, entry := range imageEntries {
		binding, err := session.renderUploadMermaidImage(pageID, entry, referer, tempDir)
		if err != nil {
			return prepared, err
		}
		entry.Data["image"] = imageDataForBinding(binding)
		prepared++
	}
	return prepared, nil
}

func imageEntryIDs(entries []blockEntry) []string {
	ids := make([]string, 0, len(entries))
	for _, entry := range entries {
		ids = append(ids, entry.ID)
	}
	return ids
}

func (session *publishSession) clientVarsContainingBlocks(pageID string, referer string, blockIDs []string) (map[string]any, error) {
	var last map[string]any
	for attempt := 0; attempt < 8; attempt++ {
		if attempt > 0 {
			time.Sleep(100 * time.Millisecond)
		}
		state, err := session.clientVars(pageID, referer)
		if err != nil {
			return nil, err
		}
		last = state
		blockMap := asMap(state["block_map"])
		missing := false
		for _, blockID := range blockIDs {
			if len(asMap(blockMap[blockID])) == 0 {
				missing = true
				break
			}
		}
		if !missing {
			return state, nil
		}
	}
	return last, fmt.Errorf("new image blocks were not visible after document write")
}

func (session *publishSession) renderUploadMermaidImage(pageID string, entry blockEntry, referer string, tempDir string) (imageBinding, error) {
	if entry.Image == nil || entry.Image.Kind != "mermaid" {
		return imageBinding{}, fmt.Errorf("unsupported generated image source")
	}
	svg, svgRenderErr := renderMermaid(entry.Image.Text, entry.Image.Ordinal, mermaidPreferredFormat, tempDir)
	if svgRenderErr == nil {
		token, uploadErr := session.uploadImage(pageID, entry.ID, svg, referer)
		if uploadErr == nil {
			return imageBinding{BlockID: entry.ID, Token: token, Image: svg}, nil
		}
		svgRenderErr = uploadErr
	}
	png, pngRenderErr := renderMermaid(entry.Image.Text, entry.Image.Ordinal, mermaidFallbackFormat, tempDir)
	if pngRenderErr != nil {
		return imageBinding{}, fmt.Errorf("mermaid render failed: svg=%v; png=%v", svgRenderErr, pngRenderErr)
	}
	token, uploadErr := session.uploadImage(pageID, entry.ID, png, referer)
	if uploadErr != nil {
		return imageBinding{}, fmt.Errorf("mermaid image upload failed: svg=%v; png=%v", svgRenderErr, uploadErr)
	}
	return imageBinding{BlockID: entry.ID, Token: token, Image: png}, nil
}

func (session *publishSession) uploadImage(pageID string, blockID string, image renderedImage, referer string) (string, error) {
	file, err := os.Open(image.Path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	fields := map[string]string{
		"file_name":        image.Name,
		"parent_type":      "docx_image",
		"parent_node":      blockID,
		"mount_point":      "docx_image",
		"mount_node_token": pageID,
		"size":             strconv.FormatInt(image.Size, 10),
	}
	for key, value := range fields {
		if err := writer.WriteField(key, value); err != nil {
			return "", err
		}
	}
	part, err := writer.CreateFormFile("file", image.Name)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(part, file); err != nil {
		return "", err
	}
	if err := writer.Close(); err != nil {
		return "", err
	}

	request, err := http.NewRequest(http.MethodPost, session.baseURL+"/space/api/box/stream/upload/all/", body)
	if err != nil {
		return "", err
	}
	request.Header.Set("Content-Type", writer.FormDataContentType())
	session.addCommonHeaders(request, referer)
	payload, err := session.doJSON(request, "image upload")
	if err != nil {
		return "", err
	}
	if code := asInt(payload["code"]); code != 0 {
		return "", fmt.Errorf("image upload failed: code=%d%s", code, serverMessageSuffix(payload))
	}
	token := imageTokenFromPayload(payload)
	if token == "" {
		return "", fmt.Errorf("image upload did not return a token")
	}
	return token, nil
}

func imageTokenFromPayload(payload map[string]any) string {
	for _, key := range []string{"token", "file_token", "fileToken"} {
		if token := asString(payload[key]); token != "" {
			return token
		}
	}
	data := asMap(payload["data"])
	for _, key := range []string{"token", "file_token", "fileToken"} {
		if token := asString(data[key]); token != "" {
			return token
		}
	}
	file := asMap(data["file"])
	for _, key := range []string{"token", "file_token", "fileToken"} {
		if token := asString(file[key]); token != "" {
			return token
		}
	}
	return ""
}

func buildImageBindingChangeMap(blockMap map[string]any, bindings []imageBinding) map[string]any {
	changeMap := map[string]any{}
	for _, binding := range bindings {
		entry := asMap(blockMap[binding.BlockID])
		changeMap[binding.BlockID] = map[string]any{
			"id":      binding.BlockID,
			"version": asInt(entry["version"]),
			"payload": map[string]any{
				"ops": []map[string]any{
					{
						"p":      []any{"image"},
						"action": map[string]any{"oi": imageDataForBinding(binding)},
					},
				},
			},
		}
	}
	return changeMap
}

func imageDataForBinding(binding imageBinding) map[string]any {
	return map[string]any{
		"token":    binding.Token,
		"name":     binding.Image.Name,
		"mimeType": binding.Image.MimeType,
		"size":     binding.Image.Size,
	}
}
