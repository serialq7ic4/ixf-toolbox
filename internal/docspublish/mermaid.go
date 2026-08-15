package docspublish

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"image"
	_ "image/jpeg"
	"image/png"
	"io"
	"math"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	mermaidRendererName        = "mmdc"
	mermaidPreferredFormat     = "svg"
	mermaidFallbackFormat      = "png"
	mermaidRendererProbeSource = "flowchart LR\n  A --> B\n"
)

type imageSource struct {
	Kind    string
	Text    string
	Path    string
	Ordinal int
}

type renderedImage struct {
	Path     string
	Name     string
	MimeType string
	Size     int64
	Width    int
	Height   int
}

type imageBinding struct {
	BlockID string
	Token   string
	Image   renderedImage
}

type mermaidRendererReadiness struct {
	Path        string
	Available   bool
	Ready       bool
	Error       string
	Remediation string
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

func requireMermaidRendererForApply(specs []Spec) error {
	if countSpecsBySourceKind(specs, "image", "mermaid") == 0 {
		return nil
	}
	status := mermaidRendererStatus(true)
	if status.Ready {
		return nil
	}
	if !status.Available {
		return fmt.Errorf("%s; %s", status.Error, status.Remediation)
	}
	return fmt.Errorf("mermaid renderer %q is not ready: %s; %s", mermaidRendererName, status.Error, status.Remediation)
}

// MermaidDependencyStatus returns a secret-safe readiness summary for external
// Mermaid rendering dependencies used by docs publish/update/patch flows.
func MermaidDependencyStatus() map[string]any {
	status := mermaidRendererStatus(true)
	payload := map[string]any{
		"ok":              status.Ready,
		"renderer":        mermaidRendererName,
		"available":       status.Available,
		"ready":           status.Ready,
		"installable":     true,
		"requiredFor":     "docs mermaid image rendering",
		"preferredFormat": mermaidPreferredFormat,
		"fallbackFormat":  mermaidFallbackFormat,
	}
	if status.Error != "" {
		payload["error"] = status.Error
	}
	if status.Remediation != "" {
		payload["remediation"] = status.Remediation
	}
	return payload
}

func mermaidRendererStatus(probe bool) mermaidRendererReadiness {
	status := mermaidRendererReadiness{}
	rendererPath, err := exec.LookPath(mermaidRendererName)
	if err != nil {
		status.Error = fmt.Sprintf("mermaid renderer %q not found in PATH", mermaidRendererName)
		status.Remediation = mermaidRendererRemediation(status.Error)
		return status
	}
	status.Path = rendererPath
	status.Available = true
	if !probe {
		return status
	}
	tempDir, err := os.MkdirTemp("", "ixf-mermaid-probe-*")
	if err != nil {
		status.Error = fmt.Sprintf("mermaid renderer probe setup failed: %v", err)
		status.Remediation = "Check local temporary directory permissions, then retry the docs write."
		return status
	}
	defer os.RemoveAll(tempDir)
	if _, err := renderMermaid(mermaidRendererProbeSource, 0, mermaidPreferredFormat, tempDir); err != nil {
		status.Error = err.Error()
		status.Remediation = mermaidRendererRemediation(status.Error)
		return status
	}
	status.Ready = true
	return status
}

func mermaidRendererRemediation(message string) string {
	lower := strings.ToLower(message)
	if strings.Contains(lower, "chrome") || strings.Contains(lower, "chromium") || strings.Contains(lower, "puppeteer") || strings.Contains(lower, "headless") {
		return "Run `npx puppeteer browsers install chrome-headless-shell` for the Mermaid CLI installation, or set PUPPETEER_EXECUTABLE_PATH to an installed Chrome/Chromium binary."
	}
	return "Install Mermaid CLI `mmdc` and verify it can render locally before applying Mermaid image blocks."
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
	width, height, err := renderedImageDimensions(outputPath, format)
	if err != nil {
		return renderedImage{}, err
	}
	return renderedImage{
		Path:     outputPath,
		Name:     name,
		MimeType: mimeTypeForRenderedFormat(format),
		Size:     info.Size(),
		Width:    width,
		Height:   height,
	}, nil
}

func renderedImageDimensions(path string, format string) (int, int, error) {
	switch format {
	case "png":
		file, err := os.Open(path)
		if err != nil {
			return 0, 0, err
		}
		defer file.Close()
		config, err := png.DecodeConfig(file)
		if err != nil {
			return 0, 0, fmt.Errorf("mmdc png dimensions unavailable: %w", err)
		}
		if config.Width <= 0 || config.Height <= 0 {
			return 0, 0, fmt.Errorf("mmdc png dimensions invalid")
		}
		return config.Width, config.Height, nil
	case "svg":
		return svgDimensions(path)
	default:
		return 0, 0, fmt.Errorf("mmdc %s dimensions unsupported", format)
	}
}

func svgDimensions(path string) (int, int, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, 0, err
	}
	defer file.Close()
	decoder := xml.NewDecoder(file)
	for {
		token, err := decoder.Token()
		if err != nil {
			if err == io.EOF {
				break
			}
			return 0, 0, fmt.Errorf("mmdc svg dimensions unavailable: %w", err)
		}
		start, ok := token.(xml.StartElement)
		if !ok || start.Name.Local != "svg" {
			continue
		}
		attrs := map[string]string{}
		for _, attr := range start.Attr {
			attrs[attr.Name.Local] = attr.Value
		}
		if width, height := dimensionPair(attrs["width"], attrs["height"]); width > 0 && height > 0 {
			return width, height, nil
		}
		if width, height := viewBoxDimensions(attrs["viewBox"]); width > 0 && height > 0 {
			return width, height, nil
		}
		break
	}
	return 0, 0, fmt.Errorf("mmdc svg dimensions unavailable")
}

func dimensionPair(widthValue string, heightValue string) (int, int) {
	width := parseDimension(widthValue)
	height := parseDimension(heightValue)
	if width <= 0 || height <= 0 {
		return 0, 0
	}
	return width, height
}

func viewBoxDimensions(value string) (int, int) {
	fields := strings.Fields(strings.ReplaceAll(strings.TrimSpace(value), ",", " "))
	if len(fields) != 4 {
		return 0, 0
	}
	width, widthErr := strconv.ParseFloat(fields[2], 64)
	height, heightErr := strconv.ParseFloat(fields[3], 64)
	if widthErr != nil || heightErr != nil || width <= 0 || height <= 0 {
		return 0, 0
	}
	return int(math.Round(width)), int(math.Round(height))
}

func parseDimension(value string) int {
	value = strings.TrimSpace(value)
	if value == "" || strings.HasSuffix(value, "%") {
		return 0
	}
	value = strings.TrimSuffix(value, "px")
	parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil || parsed <= 0 {
		return 0
	}
	return int(math.Round(parsed))
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

func localRenderedImage(path string) (renderedImage, error) {
	expanded := expandUser(path)
	info, err := os.Stat(expanded)
	if err != nil {
		return renderedImage{}, err
	}
	if info.Size() == 0 {
		return renderedImage{}, fmt.Errorf("image file is empty: %s", expanded)
	}
	name := filepath.Base(expanded)
	ext := strings.ToLower(filepath.Ext(expanded))
	if ext == ".svg" {
		width, height, err := svgDimensions(expanded)
		if err != nil {
			return renderedImage{}, err
		}
		return renderedImage{
			Path:     expanded,
			Name:     name,
			MimeType: "image/svg+xml",
			Size:     info.Size(),
			Width:    width,
			Height:   height,
		}, nil
	}
	file, err := os.Open(expanded)
	if err != nil {
		return renderedImage{}, err
	}
	defer file.Close()
	config, format, err := image.DecodeConfig(file)
	if err != nil {
		return renderedImage{}, fmt.Errorf("image dimensions unavailable for %s: %w", expanded, err)
	}
	if config.Width <= 0 || config.Height <= 0 {
		return renderedImage{}, fmt.Errorf("image dimensions invalid for %s", expanded)
	}
	mimeType, err := mimeTypeForLocalImageFormat(format)
	if err != nil {
		return renderedImage{}, err
	}
	return renderedImage{
		Path:     expanded,
		Name:     name,
		MimeType: mimeType,
		Size:     info.Size(),
		Width:    config.Width,
		Height:   config.Height,
	}, nil
}

func mimeTypeForLocalImageFormat(format string) (string, error) {
	switch format {
	case "png":
		return "image/png", nil
	case "jpeg":
		return "image/jpeg", nil
	default:
		return "", fmt.Errorf("unsupported local image format %q; supported formats are png, jpeg, and svg", format)
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
		binding, err := session.renderUploadImageSource(pageID, entry, referer, tempDir)
		if err != nil {
			return len(bindings), err
		}
		bindings = append(bindings, binding)
	}
	changeMap := buildImageBindingChangeMap(blockMap, bindings)
	if err := session.writeBlocks(pageID, memberID, changeMap, referer); err != nil {
		return len(bindings), fmt.Errorf("generated image binding write failed: %w", err)
	}
	return len(bindings), nil
}

func prepareGeneratedImagePlaceholders(entries []blockEntry) (int, error) {
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
		image, err := placeholderImageForSource(entry, tempDir)
		if err != nil {
			return prepared, err
		}
		entry.Data["image"] = imagePlaceholderData(image)
		prepared++
	}
	return prepared, nil
}

func placeholderImageForSource(entry blockEntry, tempDir string) (renderedImage, error) {
	if entry.Image == nil {
		return renderedImage{}, fmt.Errorf("unsupported generated image source")
	}
	if entry.Image.Kind == "file" {
		return localRenderedImage(entry.Image.Path)
	}
	if entry.Image.Kind != "mermaid" {
		return renderedImage{}, fmt.Errorf("unsupported generated image source kind %q", entry.Image.Kind)
	}
	svg, svgErr := renderMermaid(entry.Image.Text, entry.Image.Ordinal, mermaidPreferredFormat, tempDir)
	if svgErr == nil {
		return svg, nil
	}
	png, pngErr := renderMermaid(entry.Image.Text, entry.Image.Ordinal, mermaidFallbackFormat, tempDir)
	if pngErr != nil {
		return renderedImage{}, fmt.Errorf("mermaid render failed: svg=%v; png=%v", svgErr, pngErr)
	}
	return png, nil
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
		binding, err := session.renderUploadImageSource(pageID, entry, referer, tempDir)
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

func (session *publishSession) renderUploadImageSource(pageID string, entry blockEntry, referer string, tempDir string) (imageBinding, error) {
	if entry.Image == nil {
		return imageBinding{}, fmt.Errorf("unsupported generated image source")
	}
	if entry.Image.Kind == "file" {
		image, err := localRenderedImage(entry.Image.Path)
		if err != nil {
			return imageBinding{}, err
		}
		token, uploadErr := session.uploadImage(pageID, entry.ID, image, referer)
		if uploadErr != nil {
			return imageBinding{}, fmt.Errorf("local image upload failed: %w", uploadErr)
		}
		return imageBinding{BlockID: entry.ID, Token: token, Image: image}, nil
	}
	if entry.Image.Kind != "mermaid" {
		return imageBinding{}, fmt.Errorf("unsupported generated image source kind %q", entry.Image.Kind)
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

func (session *publishSession) renderUploadMermaidImage(pageID string, entry blockEntry, referer string, tempDir string) (imageBinding, error) {
	return session.renderUploadImageSource(pageID, entry, referer, tempDir)
}

func (session *publishSession) uploadImage(pageID string, blockID string, image renderedImage, referer string) (string, error) {
	file, err := os.Open(image.Path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
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

	query := url.Values{
		"name":                     {image.Name},
		"size":                     {strconv.FormatInt(image.Size, 10)},
		"mount_node_token":         {blockID},
		"mount_point":              {"docx_image"},
		"push_open_history_record": {"0"},
	}
	requestURL := session.baseURL + "/space/api/box/stream/upload/all/?" + query.Encode()
	request, err := http.NewRequest(http.MethodPost, requestURL, body)
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
		data := dataForBlock(entry)
		action := map[string]any{"oi": imageDataForBinding(binding)}
		if oldImage := asMap(data["image"]); len(oldImage) > 0 {
			action["od"] = oldImage
		}
		changeMap[binding.BlockID] = map[string]any{
			"id":      binding.BlockID,
			"version": asInt(entry["version"]),
			"payload": map[string]any{
				"ops": []map[string]any{
					{
						"p":      []any{"image"},
						"action": action,
					},
				},
			},
		}
	}
	return changeMap
}

func imageDataForBinding(binding imageBinding) map[string]any {
	data := imagePlaceholderData(binding.Image)
	data["token"] = binding.Token
	return data
}

func imagePlaceholderData(image renderedImage) map[string]any {
	return map[string]any{
		"name":     image.Name,
		"mimeType": image.MimeType,
		"size":     image.Size,
		"width":    image.Width,
		"height":   image.Height,
		"src":      "",
		"scale":    1,
		"align":    "center",
	}
}
