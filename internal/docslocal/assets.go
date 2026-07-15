package docslocal

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/serialq7ic4/ixf-toolbox/internal/docx"
)

var imageMimeExtensions = map[string]string{
	"image/png":     ".png",
	"image/jpeg":    ".jpg",
	"image/gif":     ".gif",
	"image/webp":    ".webp",
	"image/svg+xml": ".svg",
	"image/bmp":     ".bmp",
	"image/tiff":    ".tiff",
}

type cachedImage struct {
	markdownPath string
	failure      string
	ordinal      int
}

type imageAssetWriter struct {
	session               *remoteReadSession
	origin                string
	referer               string
	documentToken         string
	outputRoot            string
	assetGroup            string
	resolved              map[string]cachedImage
	nextOrdinal           int
	cleanedGeneratedFiles bool
}

func newImageAssetWriter(
	session *remoteReadSession,
	origin string,
	referer string,
	documentToken string,
	outputRoot string,
	assetGroup string,
) *imageAssetWriter {
	return &imageAssetWriter{
		session:       session,
		origin:        strings.TrimRight(origin, "/"),
		referer:       referer,
		documentToken: documentToken,
		outputRoot:    expandUser(outputRoot),
		assetGroup:    assetGroup,
		resolved:      map[string]cachedImage{},
		nextOrdinal:   1,
	}
}

func (writer *imageAssetWriter) resolve(reference docx.ImageReference) docx.ImageResolution {
	if cached, ok := writer.resolved[reference.Token]; ok {
		altText := imageAltText(reference, cached.ordinal)
		if cached.markdownPath != "" {
			return docx.ImageResolution{MarkdownPath: cached.markdownPath, AltText: altText}
		}
		return failedImageResolution(cached.ordinal, altText, cached.failure)
	}

	ordinal := writer.nextOrdinal
	writer.nextOrdinal++
	altText := imageAltText(reference, ordinal)
	if reference.Token == "" {
		return failedImageResolution(ordinal, altText, "missing_token")
	}
	assetDir := filepath.Join(writer.outputRoot, "assets", writer.assetGroup)
	if err := writer.removeStaleGeneratedFilesOnce(assetDir); err != nil {
		return writer.failure(reference, ordinal, altText, "io_error")
	}

	downloadURL := writer.origin + "/space/api/box/stream/download/all/" +
		url.PathEscape(reference.Token) + "/?mount_node_token=" +
		url.QueryEscape(writer.documentToken) + "&mount_point=docx_image"
	request, err := http.NewRequest(http.MethodGet, downloadURL, nil)
	if err != nil {
		return writer.failure(reference, ordinal, altText, "network_error")
	}
	request.Header.Set("User-Agent", "ixf-toolbox-go")
	request.Header.Set("Origin", writer.origin)
	request.Header.Set("Referer", writer.referer)
	request.Header.Set("X-CSRFToken", writer.session.csrfToken)
	for _, cookie := range writer.session.cookies {
		request.AddCookie(&cookie)
	}

	response, err := writer.session.client.Do(request)
	if err != nil {
		return writer.failure(reference, ordinal, altText, "network_error")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return writer.failure(reference, ordinal, altText, "http_error")
	}

	mimeType := normalizeMimeType(response.Header.Get("Content-Type"))
	extension := imageExtension(mimeType, reference.Name)
	if extension == "" {
		return writer.failure(reference, ordinal, altText, "mime_error")
	}
	filename := fmt.Sprintf("image-%03d%s", ordinal, extension)
	finalPath := filepath.Join(assetDir, filename)
	partialPath := finalPath + ".part"
	if err := os.MkdirAll(assetDir, 0o755); err != nil {
		return writer.failure(reference, ordinal, altText, "io_error")
	}

	content, err := io.ReadAll(response.Body)
	if err != nil {
		removeIfExists(partialPath)
		return writer.failure(reference, ordinal, altText, "network_error")
	}
	if !hasValidImageMagic(mimeType, extension, content) {
		removeIfExists(partialPath)
		return writer.failure(reference, ordinal, altText, "content_error")
	}
	if err := os.WriteFile(partialPath, content, 0o644); err != nil {
		removeIfExists(partialPath)
		return writer.failure(reference, ordinal, altText, "io_error")
	}
	if err := os.Rename(partialPath, finalPath); err != nil {
		removeIfExists(partialPath)
		return writer.failure(reference, ordinal, altText, "io_error")
	}

	markdownPath := filepath.ToSlash(filepath.Join("assets", writer.assetGroup, filename))
	writer.resolved[reference.Token] = cachedImage{markdownPath: markdownPath, ordinal: ordinal}
	return docx.ImageResolution{
		MarkdownPath: markdownPath,
		AltText:      altText,
		Asset: map[string]any{
			"path":      markdownPath,
			"mimeType":  mimeType,
			"width":     reference.Width,
			"height":    reference.Height,
			"sizeBytes": len(content),
			"status":    "downloaded",
			"ordinal":   ordinal,
		},
	}
}

func (writer *imageAssetWriter) failure(reference docx.ImageReference, ordinal int, altText string, reason string) docx.ImageResolution {
	if reference.Token != "" {
		writer.resolved[reference.Token] = cachedImage{failure: reason, ordinal: ordinal}
	}
	return failedImageResolution(ordinal, altText, reason)
}

func failedImageResolution(ordinal int, altText string, reason string) docx.ImageResolution {
	return docx.ImageResolution{
		AltText: altText,
		Warning: fmt.Sprintf(
			"image %d download failed: %s",
			ordinal,
			reason,
		),
	}
}

func (writer *imageAssetWriter) removeStaleGeneratedFilesOnce(assetDir string) error {
	if writer.cleanedGeneratedFiles {
		return nil
	}
	writer.cleanedGeneratedFiles = true
	entries, err := os.ReadDir(assetDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || !isGeneratedImageFilename(entry.Name()) {
			continue
		}
		if err := os.Remove(filepath.Join(assetDir, entry.Name())); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func isGeneratedImageFilename(name string) bool {
	if !strings.HasPrefix(name, "image-") {
		return false
	}
	rest := strings.TrimPrefix(name, "image-")
	digitCount := 0
	for digitCount < len(rest) && rest[digitCount] >= '0' && rest[digitCount] <= '9' {
		digitCount++
	}
	return digitCount >= 3 && digitCount < len(rest) && rest[digitCount] == '.'
}

func imageAltText(reference docx.ImageReference, ordinal int) string {
	if strings.TrimSpace(reference.Caption) != "" {
		return strings.TrimSpace(reference.Caption)
	}
	stem := strings.TrimSpace(strings.TrimSuffix(reference.Name, filepath.Ext(reference.Name)))
	if stem != "" {
		return stem
	}
	return fmt.Sprintf("image %d", ordinal)
}

func normalizeMimeType(value string) string {
	return strings.ToLower(strings.TrimSpace(strings.Split(value, ";")[0]))
}

func imageExtension(mimeType string, name string) string {
	if extension := imageMimeExtensions[mimeType]; extension != "" {
		return extension
	}
	extension := strings.ToLower(filepath.Ext(name))
	if strings.HasPrefix(mimeType, "image/") && extension == ".png" {
		return extension
	}
	return ""
}

func hasValidImageMagic(mimeType string, extension string, content []byte) bool {
	if mimeType == "image/png" || extension == ".png" {
		return len(content) >= 8 && string(content[:8]) == "\x89PNG\r\n\x1a\n"
	}
	if mimeType == "image/jpeg" || extension == ".jpg" || extension == ".jpeg" {
		return len(content) >= 3 && content[0] == 0xff && content[1] == 0xd8 && content[2] == 0xff
	}
	if mimeType == "image/gif" || extension == ".gif" {
		return len(content) >= 6 && (string(content[:6]) == "GIF87a" || string(content[:6]) == "GIF89a")
	}
	if mimeType == "image/webp" || extension == ".webp" {
		return len(content) >= 12 && string(content[:4]) == "RIFF" && string(content[8:12]) == "WEBP"
	}
	if mimeType == "image/svg+xml" || extension == ".svg" {
		trimmed := bytes.ToLower(bytes.TrimSpace(content))
		return bytes.HasPrefix(trimmed, []byte("<svg")) ||
			(bytes.HasPrefix(trimmed, []byte("<?xml")) && bytes.Contains(trimmed, []byte("<svg")))
	}
	if mimeType == "image/bmp" || extension == ".bmp" {
		return len(content) >= 2 && string(content[:2]) == "BM"
	}
	if mimeType == "image/tiff" || extension == ".tif" || extension == ".tiff" {
		return len(content) >= 4 &&
			(string(content[:4]) == "II*\x00" || string(content[:4]) == "MM\x00*")
	}
	return false
}

func removeIfExists(path string) {
	_ = os.Remove(path)
}
