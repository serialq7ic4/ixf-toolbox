package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	ixftoolbox "github.com/serialq7ic4/ixf-toolbox"
	ixfcookies "github.com/serialq7ic4/ixf-toolbox/internal/cookies"
	"github.com/serialq7ic4/ixf-toolbox/internal/docslocal"
	"github.com/serialq7ic4/ixf-toolbox/internal/docspublish"
	"github.com/serialq7ic4/ixf-toolbox/internal/markdown"
	ixfokr "github.com/serialq7ic4/ixf-toolbox/internal/okr"
	ixfupdate "github.com/serialq7ic4/ixf-toolbox/internal/update"
)

const defaultCookies = "/tmp/ixunfei_profile_explorer_cookies.json"

var version = "2.4.0"

var skillNames = []string{
	"using-ixf-toolbox",
	"ixf-docs-reader",
	"ixf-docs-writer",
	"ixf-okr-reader",
	"ixf-okr-writer",
}

type runtimeTarget struct {
	Key       string
	SkillsDir string
	SourceDir string
}

type skillResult struct {
	Runtime string `json:"runtime"`
	Skill   string `json:"skill"`
	Path    string `json:"path"`
	Reason  string `json:"reason,omitempty"`
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout io.Writer, stderr io.Writer) int {
	if len(args) == 0 {
		printRootHelp(stderr)
		return 2
	}
	if args[0] == "--version" || args[0] == "-version" {
		fmt.Fprintf(stdout, "ixf %s\n", version)
		return 0
	}
	if args[0] == "--help" || args[0] == "-h" || args[0] == "-help" || args[0] == "help" {
		printRootHelp(stdout)
		return 0
	}

	switch args[0] {
	case "docs":
		return runDocs(args[1:], stdout, stderr)
	case "okr":
		return runOKR(args[1:], stdout, stderr)
	case "doctor":
		return runDoctor(args[1:], stdout, stderr)
	case "setup":
		return runSetup(args[1:], stdout, stderr)
	case "cookies":
		return runCookies(args[1:], stdout, stderr)
	case "update":
		return runUpdate(args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "ERROR unsupported command: %s\n", args[0])
		printRootHelp(stderr)
		return 2
	}
}

func printRootHelp(w io.Writer) {
	rows := [][2]string{
		{"docs", "Read, inspect, chunk, clean up, or publish authorized documents."},
		{"okr", "Read or plan approved OKR changes."},
		{"doctor", "Inspect local Toolbox setup without printing secrets."},
		{"setup", "Install agent skill wrappers."},
		{"cookies", "Export local desktop session cookies."},
		{"update", "Check, apply, or refresh Toolbox updates."},
	}
	printCommandHelp(w, "ixf [--version]", rows)
}

func printCommandHelp(w io.Writer, prog string, rows [][2]string) {
	names := []string{}
	width := 0
	for _, row := range rows {
		names = append(names, row[0])
		if len(row[0]) > width {
			width = len(row[0])
		}
	}
	fmt.Fprintf(w, "usage: %s {%s} ...\n\n", prog, strings.Join(names, ","))
	fmt.Fprintln(w, "commands:")
	for _, row := range rows {
		fmt.Fprintf(w, "  %-*s  %s\n", width, row[0], row[1])
	}
}

func runSetup(args []string, stdout io.Writer, stderr io.Writer) int {
	if len(args) == 0 || args[0] != "skills" {
		fmt.Fprintln(stderr, "ERROR setup requires subcommand: skills")
		return 2
	}
	flags := flag.NewFlagSet("ixf setup skills", flag.ContinueOnError)
	flags.SetOutput(stderr)
	runtimesRaw := flags.String("runtimes", "auto", "")
	force := flags.Bool("force", false, "")
	asJSON := flags.Bool("json", false, "")
	if err := flags.Parse(args[1:]); err != nil {
		return 2
	}

	runtimes, err := normalizeRuntimes(strings.Split(*runtimesRaw, ","))
	if err != nil {
		fmt.Fprintf(stderr, "ERROR %s\n", err)
		return 2
	}
	payload, err := installSkills(runtimes, *force)
	if err != nil {
		fmt.Fprintf(stderr, "ERROR %s\n", err)
		return 1
	}
	if *asJSON {
		writeJSON(stdout, payload)
		return 0
	}
	fmt.Fprintf(stdout, "installed=%d skipped=%d\n", len(payload["installed"].([]skillResult)), len(payload["skipped"].([]skillResult)))
	return 0
}

func runDoctor(args []string, stdout io.Writer, stderr io.Writer) int {
	flags := flag.NewFlagSet("ixf doctor", flag.ContinueOnError)
	flags.SetOutput(stderr)
	cookiesPath := flags.String("cookies", defaultCookies, "")
	asJSON := flags.Bool("json", false, "")
	if err := flags.Parse(args); err != nil {
		return 2
	}

	payload := collectDiagnostics(*cookiesPath)
	if *asJSON {
		writeJSON(stdout, payload)
		if ok, _ := payload["ok"].(bool); ok {
			return 0
		}
		return 1
	}
	formatDiagnostics(stdout, payload)
	if ok, _ := payload["ok"].(bool); ok {
		return 0
	}
	return 1
}

func runCookies(args []string, stdout io.Writer, stderr io.Writer) int {
	if len(args) == 0 || args[0] != "export" {
		fmt.Fprintln(stderr, "ERROR cookies requires subcommand: export")
		return 2
	}
	flags := flag.NewFlagSet("ixf cookies export", flag.ContinueOnError)
	flags.SetOutput(stderr)
	provider := flags.String("provider", "auto", "")
	output := flags.String("output", defaultCookies, "")
	appSupport := flags.String("app-support", ixfcookies.DefaultAppSupport, "")
	cookiesDB := flags.String("cookies-db", "", "")
	hostLike := flags.String("host-like", ixfcookies.DefaultHostLike, "")
	keychainService := flags.String("keychain-service", ixfcookies.DefaultKeychainService, "")
	keychainAccount := flags.String("keychain-account", "", "")
	appData := flags.String("app-data", "", "")
	localState := flags.String("local-state", "", "")
	asJSON := flags.Bool("json", false, "")
	for _, arg := range args[1:] {
		if arg == "-h" || arg == "--help" {
			flags.SetOutput(stdout)
			flags.Usage()
			return 0
		}
	}
	if err := flags.Parse(args[1:]); err != nil {
		return 2
	}
	payload, err := ixfcookies.Export(ixfcookies.ExportConfig{
		Provider:        *provider,
		Output:          *output,
		AppSupport:      *appSupport,
		CookiesDB:       *cookiesDB,
		HostLike:        *hostLike,
		KeychainService: *keychainService,
		KeychainAccount: *keychainAccount,
		AppData:         *appData,
		LocalState:      *localState,
	})
	if err != nil {
		errorPayload := map[string]any{
			"ok": false,
			"error": map[string]any{
				"type":      "cookie",
				"subtype":   "cookie_export_failed",
				"message":   err.Error(),
				"hint":      "Confirm the desktop client is logged in and retry `ixf cookies export`.",
				"retryable": false,
			},
		}
		if *asJSON {
			writeJSON(stdout, errorPayload)
		} else {
			fmt.Fprintf(stderr, "ERROR %s\n", err)
		}
		return 6
	}
	writeJSON(stdout, payload)
	return 0
}

func runDocs(args []string, stdout io.Writer, stderr io.Writer) int {
	rows := [][2]string{
		{"read", "Read authorized cloud document links or local Markdown files."},
		{"outline", "Print heading-aware chunk metadata for Markdown."},
		{"chunk", "Print one heading-aware Markdown chunk."},
		{"inspect", "Print a safe local/remote source routing summary."},
		{"cleanup", "Remove generated docs read artifacts."},
		{"publish", "Publish Markdown as an authorized cloud document."},
	}
	if len(args) == 0 {
		fmt.Fprintln(stderr, "ERROR docs requires a subcommand.")
		printCommandHelp(stderr, "ixf docs", rows)
		return 2
	}
	if args[0] == "-h" || args[0] == "--help" {
		printCommandHelp(stdout, "ixf docs", rows)
		return 0
	}
	switch args[0] {
	case "read":
		return runDocsRead(args[1:], stdout, stderr)
	case "outline":
		return runDocsOutline(args[1:], stdout, stderr)
	case "chunk":
		return runDocsChunk(args[1:], stdout, stderr)
	case "inspect":
		return runDocsInspect(args[1:], stdout, stderr)
	case "cleanup":
		return runDocsCleanup(args[1:], stderr)
	case "publish":
		return runDocsPublish(args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "ERROR unsupported docs subcommand: %s\n", args[0])
		printCommandHelp(stderr, "ixf docs", rows)
		return 2
	}
}

func runDocsRead(args []string, stdout io.Writer, stderr io.Writer) int {
	parsed, err := parseDocsReadArgs(args)
	if err != nil {
		fmt.Fprintf(stderr, "ERROR %s\n", err)
		return 2
	}
	results, err := docslocal.ReadSourcesWithOptions(parsed.sources, docslocal.ReadOptions{
		CookiesPath:    parsed.cookiesPath,
		SpaceAPI:       parsed.spaceAPI,
		DownloadImages: parsed.downloadImages,
		OutputRoot:     parsed.outDir,
		ExpandSheets:   parsed.expandSheets,
	})
	if err != nil {
		fmt.Fprintf(stderr, "ERROR %s\n", err)
		return 2
	}
	if parsed.outDir != "" {
		manifest, err := docslocal.WriteOutputs(results, parsed.outDir)
		if err != nil {
			fmt.Fprintf(stderr, "ERROR %s\n", err)
			return 1
		}
		if parsed.printManifest {
			writePrettyJSON(stdout, manifest)
		}
		if parsed.cleanup {
			if err := docslocal.CleanupOutputs(parsed.outDir); err != nil {
				fmt.Fprintf(stderr, "ERROR %s\n", err)
				return 1
			}
		}
		return 0
	}
	multiple := len(results) > 1
	for _, result := range results {
		if multiple {
			fmt.Fprintf(stdout, "=== %s (%s) ===\n", result.Source, result.Kind)
		}
		fmt.Fprint(stdout, result.Content)
		if !strings.HasSuffix(result.Content, "\n") {
			fmt.Fprintln(stdout)
		}
	}
	return 0
}

func runDocsOutline(args []string, stdout io.Writer, stderr io.Writer) int {
	parsed, err := parseOutlineArgs(args)
	if err != nil {
		fmt.Fprintf(stderr, "ERROR %s\n", err)
		return 2
	}
	path := expandUser(parsed.source)
	content, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(stderr, "ERROR %s\n", err)
		return 2
	}
	outline, err := markdown.BuildOutline(string(content), parsed.targetChars)
	if err != nil {
		fmt.Fprintf(stderr, "ERROR %s\n", err)
		return 2
	}
	payload := map[string]any{
		"ok":                   true,
		"file":                 path,
		"selectedHeadingLevel": outline.SelectedHeadingLevel,
		"chunks":               outline.Chunks,
	}
	if parsed.asJSON {
		writeJSON(stdout, payload)
		return 0
	}
	for _, chunk := range outline.Chunks {
		fmt.Fprintf(stdout, "%d\t%d-%d\t%s\n", chunk.Index, chunk.StartLine, chunk.EndLine, chunk.Breadcrumb)
	}
	return 0
}

func runDocsChunk(args []string, stdout io.Writer, stderr io.Writer) int {
	parsed, err := parseChunkArgs(args)
	if err != nil {
		fmt.Fprintf(stderr, "ERROR %s\n", err)
		return 2
	}
	path := expandUser(parsed.source)
	content, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(stderr, "ERROR %s\n", err)
		return 2
	}
	text := string(content)
	outline, err := markdown.BuildOutline(text, parsed.targetChars)
	if err != nil {
		fmt.Fprintf(stderr, "ERROR %s\n", err)
		return 2
	}
	if parsed.index < 1 || parsed.index > len(outline.Chunks) {
		fmt.Fprintf(stderr, "ERROR chunk index out of range: %d\n", parsed.index)
		return 2
	}
	chunk := outline.Chunks[parsed.index-1]
	rendered, err := markdown.RenderChunk(text, outline, parsed.index)
	if err != nil {
		fmt.Fprintf(stderr, "ERROR %s\n", err)
		return 2
	}
	breadcrumb := strings.ReplaceAll(strings.ReplaceAll(chunk.Breadcrumb, `\`, `\\`), `"`, `\"`)
	fmt.Fprintf(stdout, "[chunk %d/%d breadcrumb=\"%s\"]\n\n", chunk.Index, len(outline.Chunks), breadcrumb)
	fmt.Fprint(stdout, rendered)
	return 0
}

func runDocsInspect(args []string, stdout io.Writer, stderr io.Writer) int {
	source := ""
	asJSON := false
	for _, arg := range args {
		if arg == "--json" {
			asJSON = true
			continue
		}
		if strings.HasPrefix(arg, "-") {
			fmt.Fprintf(stderr, "ERROR unsupported inspect flag: %s\n", arg)
			return 2
		}
		if source != "" {
			fmt.Fprintln(stderr, "ERROR inspect requires exactly one source")
			return 2
		}
		source = arg
	}
	if source == "" {
		fmt.Fprintln(stderr, "ERROR inspect requires one source")
		return 2
	}
	payload, err := docslocal.InspectSource(source)
	if err != nil {
		fmt.Fprintf(stderr, "ERROR %s\n", err)
		return 2
	}
	if asJSON {
		writeJSON(stdout, payload)
		return 0
	}
	if remote, _ := payload["remote"].(bool); remote {
		fmt.Fprintf(stdout, "source %s\n", payload["sourceRef"])
		fmt.Fprintln(stdout, "remote true")
		fmt.Fprintf(stdout, "kind %s\n", payload["kind"])
		fmt.Fprintf(stdout, "host %s\n", payload["host"])
		fmt.Fprintf(stdout, "route %s\n", payload["route"])
		return 0
	}
	fmt.Fprintf(stdout, "source %s\n", payload["source"])
	fmt.Fprintln(stdout, "remote false")
	fmt.Fprintf(stdout, "kind %s\n", payload["kind"])
	fmt.Fprintf(stdout, "path %s\n", payload["path"])
	fmt.Fprintf(stdout, "exists %t\n", payload["exists"])
	fmt.Fprintf(stdout, "readable %t\n", payload["readable"])
	return 0
}

func runDocsCleanup(args []string, stderr io.Writer) int {
	if len(args) != 1 {
		fmt.Fprintln(stderr, "ERROR cleanup requires one output directory")
		return 2
	}
	if err := docslocal.CleanupOutputs(args[0]); err != nil {
		fmt.Fprintf(stderr, "ERROR %s\n", err)
		return 2
	}
	return 0
}

func runDocsPublish(args []string, stdout io.Writer, stderr io.Writer) int {
	parsed, err := parseDocsPublishArgs(args)
	if err != nil {
		fmt.Fprintf(stderr, "ERROR %s\n", err)
		return 2
	}
	payload, err := docspublish.PublishMarkdown(docspublish.Config{
		MarkdownPath: parsed.markdown,
		BaseURL:      parsed.baseURL,
		CookiesPath:  parsed.cookiesPath,
		SpaceAPI:     parsed.spaceAPI,
		MemberID:     parsed.memberID,
		ParentToken:  parsed.parentToken,
		Title:        parsed.title,
		TitleSuffix:  parsed.titleSuffix,
		RequiredText: parsed.requiredText,
		Apply:        parsed.apply,
	})
	if err != nil {
		fmt.Fprintf(stderr, "ERROR %s\n", err)
		return 2
	}
	writeJSON(stdout, payload)
	return 0
}

func runOKR(args []string, stdout io.Writer, stderr io.Writer) int {
	rows := [][2]string{
		{"read", "Read an authorized OKR page as Markdown."},
		{"write", "Validate and plan confirmed Objective / KR content."},
	}
	if len(args) == 0 {
		fmt.Fprintln(stderr, "ERROR okr requires a subcommand.")
		printCommandHelp(stderr, "ixf okr", rows)
		return 2
	}
	if args[0] == "-h" || args[0] == "--help" {
		printCommandHelp(stdout, "ixf okr", rows)
		return 0
	}
	switch args[0] {
	case "read":
		return runOKRRead(args[1:], stdout, stderr)
	case "write":
		return runOKRWrite(args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "ERROR unsupported okr subcommand: %s\n", args[0])
		printCommandHelp(stderr, "ixf okr", rows)
		return 2
	}
}

func runOKRRead(args []string, stdout io.Writer, stderr io.Writer) int {
	source := ""
	cookiesPath := defaultCookies
	csrfURL := ixfokr.DefaultCSRFURL
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "--cookies":
			i++
			if i >= len(args) {
				fmt.Fprintln(stderr, "ERROR --cookies requires a value")
				return 2
			}
			cookiesPath = args[i]
		case "--csrf-url":
			i++
			if i >= len(args) {
				fmt.Fprintln(stderr, "ERROR --csrf-url requires a value")
				return 2
			}
			csrfURL = args[i]
		default:
			if strings.HasPrefix(arg, "-") {
				fmt.Fprintf(stderr, "ERROR unsupported okr read flag: %s\n", arg)
				return 2
			}
			if source != "" {
				fmt.Fprintln(stderr, "ERROR okr read requires exactly one OKR URL")
				return 2
			}
			source = arg
		}
	}
	if source == "" {
		fmt.Fprintln(stderr, "ERROR okr read requires one OKR URL")
		return 2
	}
	content, err := ixfokr.Read(ixfokr.ReadConfig{
		Source:      source,
		CookiesPath: cookiesPath,
		CSRFURL:     csrfURL,
	})
	if err != nil {
		fmt.Fprintf(stderr, "ERROR %s\n", err)
		return 2
	}
	fmt.Fprint(stdout, content)
	return 0
}

func runOKRWrite(args []string, stdout io.Writer, stderr io.Writer) int {
	flags := flag.NewFlagSet("ixf okr write", flag.ContinueOnError)
	flags.SetOutput(stderr)
	targetURL := flags.String("url", "", "")
	inputPath := flags.String("input", "", "")
	cookiesPath := flags.String("cookies", defaultCookies, "")
	csrfURL := flags.String("csrf-url", ixfokr.DefaultCSRFURL, "")
	objectiveIndex := flags.Int("objective-index", 0, "")
	prune := flags.Bool("prune", false, "")
	apply := flags.Bool("apply", false, "")
	dryRun := flags.Bool("dry-run", false, "")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *targetURL == "" {
		fmt.Fprintln(stderr, "ERROR --url is required")
		return 2
	}
	if *inputPath == "" {
		fmt.Fprintln(stderr, "ERROR --input is required")
		return 2
	}
	payload, err := ixfokr.WriteDryRun(ixfokr.WriteConfig{
		URL:            *targetURL,
		InputPath:      *inputPath,
		CookiesPath:    *cookiesPath,
		CSRFURL:        *csrfURL,
		ObjectiveIndex: *objectiveIndex,
		Prune:          *prune,
		Apply:          *apply && !*dryRun,
	})
	if err != nil {
		fmt.Fprintf(stderr, "ERROR %s\n", err)
		return 2
	}
	writeJSON(stdout, payload)
	return 0
}

type outlineArgs struct {
	source      string
	targetChars int
	asJSON      bool
}

type docsReadArgs struct {
	sources        []string
	outDir         string
	printManifest  bool
	cleanup        bool
	cookiesPath    string
	spaceAPI       string
	downloadImages bool
	expandSheets   bool
}

type docsPublishArgs struct {
	markdown     string
	baseURL      string
	cookiesPath  string
	spaceAPI     string
	memberID     string
	parentToken  string
	title        string
	titleSuffix  string
	requiredText []string
	apply        bool
	dryRun       bool
}

func parseDocsReadArgs(args []string) (docsReadArgs, error) {
	parsed := docsReadArgs{
		cookiesPath: defaultCookies,
		spaceAPI:    docslocal.DefaultSpaceAPI,
	}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "--out-dir":
			i++
			if i >= len(args) {
				return parsed, fmt.Errorf("--out-dir requires a value")
			}
			parsed.outDir = args[i]
		case "--print-manifest":
			parsed.printManifest = true
		case "--cleanup":
			parsed.cleanup = true
		case "--expand-sheets":
			parsed.expandSheets = true
		case "--download-images":
			parsed.downloadImages = true
		case "--cookies":
			i++
			if i >= len(args) {
				return parsed, fmt.Errorf("%s requires a value", arg)
			}
			parsed.cookiesPath = args[i]
		case "--space-api":
			i++
			if i >= len(args) {
				return parsed, fmt.Errorf("%s requires a value", arg)
			}
			parsed.spaceAPI = args[i]
		default:
			if strings.HasPrefix(arg, "-") {
				return parsed, fmt.Errorf("unsupported docs read flag: %s", arg)
			}
			parsed.sources = append(parsed.sources, arg)
		}
	}
	if len(parsed.sources) == 0 {
		return parsed, fmt.Errorf("read requires at least one source")
	}
	if parsed.printManifest && parsed.outDir == "" {
		return parsed, fmt.Errorf("--print-manifest requires --out-dir")
	}
	if parsed.cleanup && parsed.outDir == "" {
		return parsed, fmt.Errorf("--cleanup requires --out-dir")
	}
	if parsed.downloadImages && parsed.outDir == "" {
		return parsed, fmt.Errorf("--download-images requires --out-dir")
	}
	return parsed, nil
}

func parseDocsPublishArgs(args []string) (docsPublishArgs, error) {
	parsed := docsPublishArgs{
		cookiesPath: defaultCookies,
		spaceAPI:    docslocal.DefaultSpaceAPI,
	}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "--base-url":
			i++
			if i >= len(args) {
				return parsed, fmt.Errorf("%s requires a value", arg)
			}
			parsed.baseURL = args[i]
		case "--space-api":
			i++
			if i >= len(args) {
				return parsed, fmt.Errorf("%s requires a value", arg)
			}
			parsed.spaceAPI = args[i]
		case "--cookies":
			i++
			if i >= len(args) {
				return parsed, fmt.Errorf("%s requires a value", arg)
			}
			parsed.cookiesPath = args[i]
		case "--member-id":
			i++
			if i >= len(args) {
				return parsed, fmt.Errorf("%s requires a value", arg)
			}
			parsed.memberID = args[i]
		case "--parent-token":
			i++
			if i >= len(args) {
				return parsed, fmt.Errorf("%s requires a value", arg)
			}
			parsed.parentToken = args[i]
		case "--title":
			i++
			if i >= len(args) {
				return parsed, fmt.Errorf("%s requires a value", arg)
			}
			parsed.title = args[i]
		case "--title-suffix":
			i++
			if i >= len(args) {
				return parsed, fmt.Errorf("%s requires a value", arg)
			}
			parsed.titleSuffix = args[i]
		case "--require":
			i++
			if i >= len(args) {
				return parsed, fmt.Errorf("%s requires a value", arg)
			}
			parsed.requiredText = append(parsed.requiredText, args[i])
		case "--apply":
			parsed.apply = true
		case "--dry-run":
			parsed.dryRun = true
		default:
			if strings.HasPrefix(arg, "-") {
				return parsed, fmt.Errorf("unsupported docs publish flag: %s", arg)
			}
			if parsed.markdown != "" {
				return parsed, fmt.Errorf("publish requires exactly one Markdown file")
			}
			parsed.markdown = arg
		}
	}
	if parsed.markdown == "" {
		return parsed, fmt.Errorf("publish requires one Markdown file")
	}
	if parsed.baseURL == "" {
		return parsed, fmt.Errorf("--base-url is required")
	}
	if parsed.dryRun {
		parsed.apply = false
	}
	return parsed, nil
}

func parseOutlineArgs(args []string) (outlineArgs, error) {
	parsed := outlineArgs{targetChars: 12000}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "--json":
			parsed.asJSON = true
		case "--target-chars":
			i++
			if i >= len(args) {
				return parsed, fmt.Errorf("--target-chars requires a value")
			}
			value, err := strconv.Atoi(args[i])
			if err != nil {
				return parsed, err
			}
			parsed.targetChars = value
		default:
			if strings.HasPrefix(arg, "-") {
				return parsed, fmt.Errorf("unsupported outline flag: %s", arg)
			}
			if parsed.source != "" {
				return parsed, fmt.Errorf("outline requires exactly one source")
			}
			parsed.source = arg
		}
	}
	if parsed.source == "" {
		return parsed, fmt.Errorf("outline requires one source")
	}
	return parsed, nil
}

type chunkArgs struct {
	source      string
	index       int
	targetChars int
}

func parseChunkArgs(args []string) (chunkArgs, error) {
	parsed := chunkArgs{targetChars: 12000}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "--index":
			i++
			if i >= len(args) {
				return parsed, fmt.Errorf("--index requires a value")
			}
			value, err := strconv.Atoi(args[i])
			if err != nil {
				return parsed, err
			}
			parsed.index = value
		case "--target-chars":
			i++
			if i >= len(args) {
				return parsed, fmt.Errorf("--target-chars requires a value")
			}
			value, err := strconv.Atoi(args[i])
			if err != nil {
				return parsed, err
			}
			parsed.targetChars = value
		default:
			if strings.HasPrefix(arg, "-") {
				return parsed, fmt.Errorf("unsupported chunk flag: %s", arg)
			}
			if parsed.source != "" {
				return parsed, fmt.Errorf("chunk requires exactly one source")
			}
			parsed.source = arg
		}
	}
	if parsed.source == "" {
		return parsed, fmt.Errorf("chunk requires one source")
	}
	if parsed.index == 0 {
		return parsed, fmt.Errorf("chunk requires --index")
	}
	return parsed, nil
}

func runUpdate(args []string, stdout io.Writer, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "ERROR update requires a subcommand")
		return 2
	}
	switch args[0] {
	case "check":
		return runUpdateCheck(args[1:], stdout, stderr)
	case "self":
		return runUpdateSelf(args[1:], stdout, stderr)
	case "skills":
		return runUpdateSkills(args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "ERROR unsupported update subcommand: %s\n", args[0])
		return 2
	}
}

func runUpdateCheck(args []string, stdout io.Writer, stderr io.Writer) int {
	repo, releaseFile, asJSON, err := parseUpdateArgs(args, false)
	if err != nil {
		fmt.Fprintf(stderr, "ERROR %s\n", err)
		return 2
	}
	release, err := ixfupdate.LoadRelease(repo, releaseFile)
	if err != nil {
		fmt.Fprintf(stderr, "ERROR update check failed: %s\n", err)
		return 10
	}
	payload, err := ixfupdate.CheckLatestRelease(repo, version, release)
	if err != nil {
		fmt.Fprintf(stderr, "ERROR update check failed: %s\n", err)
		return 10
	}
	printUpdatePayload(stdout, payload, asJSON)
	return 0
}

func runUpdateSelf(args []string, stdout io.Writer, stderr io.Writer) int {
	repo, releaseFile, asJSON, apply, targetPath, err := parseUpdateSelfArgs(args)
	if err != nil {
		fmt.Fprintf(stderr, "ERROR %s\n", err)
		return 2
	}
	release, err := ixfupdate.LoadRelease(repo, releaseFile)
	if err != nil {
		fmt.Fprintf(stderr, "ERROR update self failed: %s\n", err)
		return 10
	}
	payload, err := ixfupdate.SelfUpdateWithOptions(ixfupdate.SelfUpdateOptions{
		Repo:           repo,
		CurrentVersion: version,
		Release:        release,
		Apply:          apply,
		TargetPath:     targetPath,
	})
	if err != nil {
		fmt.Fprintf(stderr, "ERROR update self failed: %s\n", err)
		return 10
	}
	printUpdatePayload(stdout, payload, asJSON)
	return 0
}

func runUpdateSkills(args []string, stdout io.Writer, stderr io.Writer) int {
	flags := flag.NewFlagSet("ixf update skills", flag.ContinueOnError)
	flags.SetOutput(stderr)
	runtimesRaw := flags.String("runtimes", "auto", "")
	asJSON := flags.Bool("json", false, "")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	runtimes, err := normalizeRuntimes(strings.Split(*runtimesRaw, ","))
	if err != nil {
		fmt.Fprintf(stderr, "ERROR %s\n", err)
		return 2
	}
	payload, err := installSkills(runtimes, true)
	if err != nil {
		fmt.Fprintf(stderr, "ERROR %s\n", err)
		return 1
	}
	if *asJSON {
		writeJSON(stdout, payload)
		return 0
	}
	fmt.Fprintf(stdout, "installed=%d skipped=%d\n", len(payload["installed"].([]skillResult)), len(payload["skipped"].([]skillResult)))
	return 0
}

func parseUpdateArgs(args []string, allowApply bool) (string, string, bool, error) {
	repo := ixfupdate.DefaultReleaseRepo
	releaseFile := ""
	asJSON := false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--repo":
			i++
			if i >= len(args) {
				return repo, releaseFile, asJSON, fmt.Errorf("--repo requires a value")
			}
			repo = args[i]
		case "--release-file":
			i++
			if i >= len(args) {
				return repo, releaseFile, asJSON, fmt.Errorf("--release-file requires a value")
			}
			releaseFile = args[i]
		case "--json":
			asJSON = true
		case "--apply":
			if !allowApply {
				return repo, releaseFile, asJSON, fmt.Errorf("--apply is only supported by update self")
			}
		default:
			return repo, releaseFile, asJSON, fmt.Errorf("unsupported update flag: %s", args[i])
		}
	}
	return repo, releaseFile, asJSON, nil
}

func parseUpdateSelfArgs(args []string) (string, string, bool, bool, string, error) {
	repo := ixfupdate.DefaultReleaseRepo
	releaseFile := ""
	asJSON := false
	apply := false
	targetPath := ""
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--repo":
			i++
			if i >= len(args) {
				return repo, releaseFile, asJSON, apply, targetPath, fmt.Errorf("--repo requires a value")
			}
			repo = args[i]
		case "--release-file":
			i++
			if i >= len(args) {
				return repo, releaseFile, asJSON, apply, targetPath, fmt.Errorf("--release-file requires a value")
			}
			releaseFile = args[i]
		case "--target-path":
			i++
			if i >= len(args) {
				return repo, releaseFile, asJSON, apply, targetPath, fmt.Errorf("--target-path requires a value")
			}
			targetPath = args[i]
		case "--json":
			asJSON = true
		case "--apply":
			apply = true
		default:
			return repo, releaseFile, asJSON, apply, targetPath, fmt.Errorf("unsupported update self flag: %s", args[i])
		}
	}
	return repo, releaseFile, asJSON, apply, targetPath, nil
}

func printUpdatePayload(stdout io.Writer, payload map[string]any, asJSON bool) {
	if asJSON {
		writeJSON(stdout, payload)
		return
	}
	fmt.Fprintf(stdout, "current %s\n", payload["currentVersion"])
	fmt.Fprintf(stdout, "latest %s\n", payload["latestVersion"])
	fmt.Fprintf(stdout, "updateAvailable %t\n", payload["updateAvailable"])
	if applied, ok := payload["applied"].(bool); ok {
		fmt.Fprintf(stdout, "applied %t\n", applied)
	}
	if command, _ := payload["installCommand"].(string); command != "" {
		fmt.Fprintln(stdout, command)
	}
}

func goCommandUnavailable(stderr io.Writer, command string, hint string) int {
	fmt.Fprintf(stderr, "ERROR Go runtime does not support `%s` yet.\n", command)
	fmt.Fprintf(stderr, "HINT %s\n", hint)
	return 9
}

func installSkills(runtimes []string, force bool) (map[string]any, error) {
	installed := []skillResult{}
	skipped := []skillResult{}
	targets := detectRuntimeTargets()
	selected := map[string]bool{}
	for _, runtime := range runtimes {
		selected[runtime] = true
	}

	for _, target := range targets {
		if !selected[target.Key] {
			continue
		}
		for _, skillName := range skillNames {
			source := filepath.ToSlash(filepath.Join(target.SourceDir, skillName, "SKILL.md"))
			content, err := fs.ReadFile(ixftoolbox.SkillFS, source)
			if err != nil {
				return nil, err
			}
			destination := filepath.Join(target.SkillsDir, skillName)
			skillPath := filepath.Join(destination, "SKILL.md")
			if pathExists(destination) && !force {
				skipped = append(skipped, skillResult{Runtime: target.Key, Skill: skillName, Path: destination, Reason: "exists"})
				continue
			}
			if force {
				if err := os.RemoveAll(destination); err != nil {
					return nil, err
				}
			}
			if err := os.MkdirAll(destination, 0o755); err != nil {
				return nil, err
			}
			if err := os.WriteFile(skillPath, content, 0o644); err != nil {
				return nil, err
			}
			installed = append(installed, skillResult{Runtime: target.Key, Skill: skillName, Path: destination})
		}
	}
	return map[string]any{"ok": true, "installed": installed, "skipped": skipped}, nil
}

func detectRuntimeTargets() []runtimeTarget {
	home := homeDir()
	codexDir := getenvDefault("IXF_TOOLBOX_CODEX_SKILLS_DIR", filepath.Join(home, ".codex", "skills"))
	claudeDir := getenvDefault("IXF_TOOLBOX_CLAUDE_CODE_SKILLS_DIR", filepath.Join(home, ".claude", "skills"))
	return []runtimeTarget{
		{Key: "codex", SkillsDir: codexDir, SourceDir: filepath.FromSlash("skills/codex")},
		{Key: "claude-code", SkillsDir: claudeDir, SourceDir: filepath.FromSlash("skills/claude-code")},
	}
}

func normalizeRuntimes(raw []string) ([]string, error) {
	values := []string{}
	for _, value := range raw {
		value = strings.ToLower(strings.TrimSpace(value))
		if value != "" {
			values = append(values, value)
		}
	}
	if len(values) == 0 || contains(values, "auto") || contains(values, "all") {
		return []string{"codex", "claude-code"}, nil
	}
	if contains(values, "none") {
		return []string{}, nil
	}
	result := []string{}
	seen := map[string]bool{}
	for _, value := range values {
		normalized := value
		if value == "claude" || value == "claude_code" {
			normalized = "claude-code"
		}
		if normalized != "codex" && normalized != "claude-code" {
			return nil, fmt.Errorf("unsupported runtime: %s", value)
		}
		if !seen[normalized] {
			result = append(result, normalized)
			seen[normalized] = true
		}
	}
	return result, nil
}

func collectDiagnostics(cookiesPath string) map[string]any {
	skills := skillsStatus()
	cookies := cookieDiagnostics(cookiesPath)
	skillsOK := false
	for _, raw := range skills {
		if status, ok := raw.(map[string]any); ok {
			if value, _ := status["ok"].(bool); value {
				skillsOK = true
			}
		}
	}
	cookiesOK, _ := cookies["ok"].(bool)
	return map[string]any{
		"ok":      skillsOK && cookiesOK,
		"version": version,
		"runtime": "go",
		"capabilities": map[string]bool{
			"docsRead":      true,
			"docsPublish":   true,
			"okrRead":       true,
			"okrWrite":      true,
			"cookiesExport": true,
		},
		"skills":  skills,
		"cookies": cookies,
	}
}

func skillsStatus() map[string]any {
	result := map[string]any{}
	for _, target := range detectRuntimeTargets() {
		installed := map[string]bool{}
		ok := true
		for _, skillName := range skillNames {
			exists := pathExists(filepath.Join(target.SkillsDir, skillName, "SKILL.md"))
			installed[skillName] = exists
			if !exists {
				ok = false
			}
		}
		result[target.Key] = map[string]any{"ok": ok, "dir": target.SkillsDir, "installed": installed}
	}
	return result
}

func cookieDiagnostics(cookiePath string) map[string]any {
	path := expandUser(cookiePath)
	payload := map[string]any{
		"ok":          false,
		"exists":      false,
		"path":        path,
		"cookieCount": 0,
		"cookieNames": []string{},
		"hasCsrf":     false,
		"hasLgwCsrf":  false,
	}
	content, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return payload
	}
	if err != nil {
		payload["exists"] = true
		payload["error"] = fmt.Sprintf("%T: %v", err, err)
		return payload
	}
	var cookies []map[string]any
	if err := json.Unmarshal(content, &cookies); err != nil {
		payload["exists"] = true
		payload["error"] = fmt.Sprintf("%T: %v", err, err)
		return payload
	}
	names := map[string]bool{}
	hasCsrf := false
	hasLgwCsrf := false
	for _, cookie := range cookies {
		name, _ := cookie["name"].(string)
		value, _ := cookie["value"].(string)
		if name == "" {
			continue
		}
		names[name] = true
		if name == "_csrf_token" && value != "" {
			hasCsrf = true
		}
		if name == "lgw_csrf_token" && value != "" {
			hasLgwCsrf = true
		}
	}
	nameList := make([]string, 0, len(names))
	for name := range names {
		nameList = append(nameList, name)
	}
	sort.Strings(nameList)
	payload["ok"] = true
	payload["exists"] = true
	payload["cookieCount"] = len(cookies)
	payload["cookieNames"] = nameList
	payload["hasCsrf"] = hasCsrf
	payload["hasLgwCsrf"] = hasLgwCsrf
	return payload
}

func formatDiagnostics(w io.Writer, payload map[string]any) {
	fmt.Fprintf(w, "ixf-toolbox %s\n", payload["version"])
	if ok, _ := payload["ok"].(bool); ok {
		fmt.Fprintln(w, "overall ok")
	} else {
		fmt.Fprintln(w, "overall fail")
	}
}

func writeJSON(w io.Writer, payload any) {
	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(false)
	_ = encoder.Encode(payload)
}

func writePrettyJSON(w io.Writer, payload any) {
	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	_ = encoder.Encode(payload)
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func getenvDefault(name string, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return expandUser(value)
	}
	return fallback
}

func homeDir() string {
	if home := os.Getenv("HOME"); home != "" {
		return home
	}
	if home, err := os.UserHomeDir(); err == nil {
		return home
	}
	return "."
}

func expandUser(path string) string {
	if path == "~" {
		return homeDir()
	}
	if strings.HasPrefix(path, "~/") || strings.HasPrefix(path, `~\`) {
		return filepath.Join(homeDir(), path[2:])
	}
	return path
}
