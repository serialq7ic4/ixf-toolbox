package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	ixftoolbox "github.com/serialq7ic4/ixf-toolbox"
	ixfbitable "github.com/serialq7ic4/ixf-toolbox/internal/bitable"
	ixfcookies "github.com/serialq7ic4/ixf-toolbox/internal/cookies"
	"github.com/serialq7ic4/ixf-toolbox/internal/docslocal"
	"github.com/serialq7ic4/ixf-toolbox/internal/docspublish"
	"github.com/serialq7ic4/ixf-toolbox/internal/markdown"
	"github.com/serialq7ic4/ixf-toolbox/internal/messenger"
	ixfokr "github.com/serialq7ic4/ixf-toolbox/internal/okr"
	ixfsheets "github.com/serialq7ic4/ixf-toolbox/internal/sheets"
	ixfupdate "github.com/serialq7ic4/ixf-toolbox/internal/update"
)

const defaultCookies = "/tmp/ixunfei_profile_explorer_cookies.json"
const docsDefaultBaseURLEnv = "IXF_DOCS_DEFAULT_BASE_URL"
const globalDefaultBaseURLEnv = "IXF_DEFAULT_BASE_URL"

var version = ixftoolbox.DefaultVersion

var dependencyReleaseLoader = ixfupdate.LoadRelease
var bitableInspect = ixfbitable.Inspect
var bitableAttach = ixfbitable.Attach
var bitableRecordCreate = ixfbitable.RecordCreate
var docsTableAppendRow = docspublish.AppendTableRow

var skillNames = []string{
	"using-ixf-toolbox",
	"ixf-docs-reader",
	"ixf-docs-writer",
	"ixf-okr-reader",
	"ixf-okr-writer",
	"ixf-messenger-reader",
	"ixf-messenger-writer",
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
	case "sheets":
		return runSheets(args[1:], stdout, stderr)
	case "bitable":
		return runBitable(args[1:], stdout, stderr)
	case "okr":
		return runOKR(args[1:], stdout, stderr)
	case "messenger":
		return runMessenger(args[1:], stdout, stderr)
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
		{"sheets", "Read direct sheets links or plan approved sheet cell updates."},
		{"bitable", "Inspect or plan approved bitable attachment changes."},
		{"okr", "Read or plan approved OKR changes."},
		{"messenger", "Inspect and plan safe i讯飞 Messenger automation."},
		{"doctor", "Inspect local Toolbox setup without printing secrets."},
		{"setup", "Install agent skill wrappers or optional dependencies."},
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

func isHelpArg(arg string) bool {
	return arg == "-h" || arg == "--help" || arg == "-help" || arg == "help"
}

func hasHelpArg(args []string) bool {
	for _, arg := range args {
		if isHelpArg(arg) {
			return true
		}
	}
	return false
}

func printUsageHelp(w io.Writer, usage string, options [][2]string) {
	fmt.Fprintf(w, "usage: %s\n\n", usage)
	if len(options) == 0 {
		return
	}
	width := 0
	for _, option := range options {
		if len(option[0]) > width {
			width = len(option[0])
		}
	}
	fmt.Fprintln(w, "options:")
	for _, option := range options {
		fmt.Fprintf(w, "  %-*s  %s\n", width, option[0], option[1])
	}
}

func runSetup(args []string, stdout io.Writer, stderr io.Writer) int {
	rows := [][2]string{
		{"skills", "Install agent skill wrappers."},
		{"deps", "Inspect or install optional local dependencies."},
	}
	if len(args) == 0 {
		fmt.Fprintln(stderr, "ERROR setup requires subcommand: skills or deps")
		printCommandHelp(stderr, "ixf setup", rows)
		return 2
	}
	if isHelpArg(args[0]) {
		printCommandHelp(stdout, "ixf setup", rows)
		return 0
	}
	switch args[0] {
	case "skills":
		return runSetupSkills(args[1:], stdout, stderr)
	case "deps":
		return runSetupDeps(args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "ERROR unsupported setup subcommand: %s\n", args[0])
		printCommandHelp(stderr, "ixf setup", rows)
		return 2
	}
}

func runSetupSkills(args []string, stdout io.Writer, stderr io.Writer) int {
	flags := flag.NewFlagSet("ixf setup skills", flag.ContinueOnError)
	flags.SetOutput(stderr)
	runtimesRaw := flags.String("runtimes", "auto", "")
	force := flags.Bool("force", false, "")
	asJSON := flags.Bool("json", false, "")
	if hasHelpArg(args) {
		flags.SetOutput(stdout)
		flags.Usage()
		return 0
	}
	if err := flags.Parse(args); err != nil {
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

func runSetupDeps(args []string, stdout io.Writer, stderr io.Writer) int {
	flags := flag.NewFlagSet("ixf setup deps", flag.ContinueOnError)
	flags.SetOutput(stderr)
	cookiesPath := flags.String("cookies", defaultCookies, "")
	apply := flags.Bool("apply", false, "")
	dryRun := flags.Bool("dry-run", false, "")
	asJSON := flags.Bool("json", false, "")
	if hasHelpArg(args) {
		flags.SetOutput(stdout)
		flags.Usage()
		return 0
	}
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *apply && *dryRun {
		fmt.Fprintln(stderr, "ERROR setup deps accepts only one of --dry-run or --apply")
		return 2
	}
	payload, err := setupDependencies(*cookiesPath, *apply)
	if err != nil {
		fmt.Fprintf(stderr, "ERROR %s\n", err)
		return 1
	}
	if *asJSON {
		writeJSON(stdout, payload)
		return 0
	}
	formatSetupDeps(stdout, payload)
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

func runSheets(args []string, stdout io.Writer, stderr io.Writer) int {
	rows := [][2]string{
		{"read", "Read a direct authorized sheets link as Markdown/TSV."},
		{"update", "Dry-run or apply an approved TSV cell update."},
	}
	if len(args) == 0 {
		fmt.Fprintln(stderr, "ERROR sheets requires a subcommand.")
		printCommandHelp(stderr, "ixf sheets", rows)
		return 2
	}
	if isHelpArg(args[0]) {
		printCommandHelp(stdout, "ixf sheets", rows)
		return 0
	}
	switch args[0] {
	case "read":
		return runSheetsRead(args[1:], stdout, stderr)
	case "update":
		return runSheetsUpdate(args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "ERROR unsupported sheets subcommand: %s\n", args[0])
		printCommandHelp(stderr, "ixf sheets", rows)
		return 2
	}
}

func runSheetsRead(args []string, stdout io.Writer, stderr io.Writer) int {
	if hasHelpArg(args) {
		printUsageHelp(stdout, "ixf sheets read <sheets-url> [--cookies PATH] [--space-api URL]", [][2]string{
			{"--cookies PATH", "Read exported desktop session cookies from PATH."},
			{"--space-api URL", "Override the i讯飞 Space API base URL."},
		})
		return 0
	}
	source := ""
	cookiesPath := defaultCookies
	spaceAPI := docslocal.DefaultSpaceAPI
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
		case "--space-api":
			i++
			if i >= len(args) {
				fmt.Fprintln(stderr, "ERROR --space-api requires a value")
				return 2
			}
			spaceAPI = args[i]
		default:
			if strings.HasPrefix(arg, "-") {
				fmt.Fprintf(stderr, "ERROR unsupported sheets read flag: %s\n", arg)
				return 2
			}
			if source != "" {
				fmt.Fprintln(stderr, "ERROR sheets read requires exactly one direct sheets URL")
				return 2
			}
			source = arg
		}
	}
	if source == "" {
		fmt.Fprintln(stderr, "ERROR sheets read requires exactly one direct sheets URL")
		return 2
	}
	content, err := ixfsheets.Read(ixfsheets.ReadConfig{
		Source:      source,
		CookiesPath: cookiesPath,
		SpaceAPI:    spaceAPI,
	})
	if err != nil {
		fmt.Fprintf(stderr, "ERROR %s\n", err)
		return 2
	}
	fmt.Fprint(stdout, content)
	if !strings.HasSuffix(content, "\n") {
		fmt.Fprintln(stdout)
	}
	return 0
}

func runSheetsUpdate(args []string, stdout io.Writer, stderr io.Writer) int {
	flags := flag.NewFlagSet("ixf sheets update", flag.ContinueOnError)
	flags.SetOutput(stderr)
	targetURL := flags.String("url", "", "")
	hostURL := flags.String("host-url", "", "")
	rangeStart := flags.String("range", "", "")
	inputPath := flags.String("input", "", "")
	cookiesPath := flags.String("cookies", defaultCookies, "")
	spaceAPI := flags.String("space-api", "", "")
	dryRun := flags.Bool("dry-run", false, "")
	apply := flags.Bool("apply", false, "")
	if hasHelpArg(args) {
		flags.SetOutput(stdout)
		flags.Usage()
		return 0
	}
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *targetURL == "" {
		fmt.Fprintln(stderr, "ERROR --url is required")
		return 2
	}
	if *rangeStart == "" {
		fmt.Fprintln(stderr, "ERROR --range is required")
		return 2
	}
	if *inputPath == "" {
		fmt.Fprintln(stderr, "ERROR --input is required")
		return 2
	}
	if *dryRun && *apply {
		fmt.Fprintln(stderr, "ERROR --dry-run and --apply are mutually exclusive")
		return 2
	}
	payload, err := ixfsheets.Update(ixfsheets.UpdateConfig{
		URL:         *targetURL,
		HostURL:     *hostURL,
		Range:       *rangeStart,
		InputPath:   *inputPath,
		DryRun:      *dryRun,
		Apply:       *apply,
		CookiesPath: *cookiesPath,
		SpaceAPI:    *spaceAPI,
	})
	if err != nil {
		fmt.Fprintf(stderr, "ERROR %s\n", err)
		return 2
	}
	writeJSON(stdout, payload)
	return 0
}

func runBitable(args []string, stdout io.Writer, stderr io.Writer) int {
	rows := [][2]string{
		{"inspect", "Inspect a bitable source and report safe metadata."},
		{"read", "Read bitable metadata as a safe JSON summary."},
		{"attach", "Plan an existing-record attachment upload; dry-run only."},
		{"record", "Inspect or plan approved bitable record changes."},
	}
	if len(args) == 0 {
		fmt.Fprintln(stderr, "ERROR bitable requires a subcommand.")
		printCommandHelp(stderr, "ixf bitable", rows)
		return 2
	}
	if isHelpArg(args[0]) {
		printCommandHelp(stdout, "ixf bitable", rows)
		return 0
	}
	switch args[0] {
	case "inspect":
		return runBitableInspect(args[1:], stdout, stderr)
	case "read":
		return runBitableRead(args[1:], stdout, stderr)
	case "attach":
		return runBitableAttach(args[1:], stdout, stderr)
	case "record":
		return runBitableRecord(args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "ERROR unsupported bitable subcommand: %s\n", args[0])
		printCommandHelp(stderr, "ixf bitable", rows)
		return 2
	}
}

func runBitableRecord(args []string, stdout io.Writer, stderr io.Writer) int {
	rows := [][2]string{
		{"create", "Dry-run or apply an approved bitable record creation."},
	}
	if len(args) == 0 {
		fmt.Fprintln(stderr, "ERROR bitable record requires a subcommand.")
		printCommandHelp(stderr, "ixf bitable record", rows)
		return 2
	}
	if isHelpArg(args[0]) {
		printCommandHelp(stdout, "ixf bitable record", rows)
		return 0
	}
	switch args[0] {
	case "create":
		return runBitableRecordCreate(args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "ERROR unsupported bitable record subcommand: %s\n", args[0])
		printCommandHelp(stderr, "ixf bitable record", rows)
		return 2
	}
}

func runBitableRecordCreate(args []string, stdout io.Writer, stderr io.Writer) int {
	flags := flag.NewFlagSet("ixf bitable record create", flag.ContinueOnError)
	flags.SetOutput(stderr)
	targetURL := flags.String("url", "", "")
	inputPath := flags.String("input", "", "")
	insertPosition := flags.String("insert-position", "", "top|bottom, default bottom")
	cookiesPath := flags.String("cookies", defaultCookies, "")
	spaceAPI := flags.String("space-api", "", "")
	dryRun := flags.Bool("dry-run", false, "")
	apply := flags.Bool("apply", false, "")
	asJSON := flags.Bool("json", false, "")
	if hasHelpArg(args) {
		flags.SetOutput(stdout)
		flags.Usage()
		return 0
	}
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
	if *dryRun && *apply {
		fmt.Fprintln(stderr, "ERROR --dry-run and --apply are mutually exclusive")
		return 2
	}
	payload, err := bitableRecordCreate(ixfbitable.RecordCreateConfig{
		URL:            *targetURL,
		InputPath:      *inputPath,
		InsertPosition: *insertPosition,
		DryRun:         *dryRun,
		Apply:          *apply,
		CookiesPath:    *cookiesPath,
		SpaceAPI:       *spaceAPI,
	})
	if err != nil {
		fmt.Fprintf(stderr, "ERROR %s\n", err)
		return 2
	}
	if *asJSON {
		writeJSON(stdout, payload)
		return 0
	}
	formatBitableSummary(stdout, payload)
	return 0
}

func runBitableInspect(args []string, stdout io.Writer, stderr io.Writer) int {
	flags := flag.NewFlagSet("ixf bitable inspect", flag.ContinueOnError)
	flags.SetOutput(stderr)
	targetURL := flags.String("url", "", "")
	cookiesPath := flags.String("cookies", defaultCookies, "")
	spaceAPI := flags.String("space-api", "", "")
	asJSON := flags.Bool("json", false, "")
	if hasHelpArg(args) {
		flags.SetOutput(stdout)
		flags.Usage()
		return 0
	}
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *targetURL == "" {
		fmt.Fprintln(stderr, "ERROR --url is required")
		return 2
	}
	payload, err := bitableInspect(ixfbitable.InspectConfig{
		URL:         *targetURL,
		CookiesPath: *cookiesPath,
		SpaceAPI:    *spaceAPI,
	})
	if err != nil {
		fmt.Fprintf(stderr, "ERROR %s\n", err)
		return 2
	}
	if *asJSON {
		writeJSON(stdout, payload)
		return 0
	}
	formatBitableSummary(stdout, payload)
	return 0
}

func runBitableRead(args []string, stdout io.Writer, stderr io.Writer) int {
	return runBitableInspect(args, stdout, stderr)
}

func runBitableAttach(args []string, stdout io.Writer, stderr io.Writer) int {
	flags := flag.NewFlagSet("ixf bitable attach", flag.ContinueOnError)
	flags.SetOutput(stderr)
	targetURL := flags.String("url", "", "")
	field := flags.String("field", "", "")
	recordID := flags.String("record-id", "", "")
	recordMatch := flags.String("record-match", "", "")
	filePath := flags.String("file", "", "")
	cookiesPath := flags.String("cookies", defaultCookies, "")
	spaceAPI := flags.String("space-api", "", "")
	dryRun := flags.Bool("dry-run", false, "")
	apply := flags.Bool("apply", false, "currently unavailable for bitable attach; use --dry-run")
	asJSON := flags.Bool("json", false, "")
	if hasHelpArg(args) {
		flags.SetOutput(stdout)
		flags.Usage()
		return 0
	}
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *targetURL == "" {
		fmt.Fprintln(stderr, "ERROR --url is required")
		return 2
	}
	if *field == "" {
		fmt.Fprintln(stderr, "ERROR --field is required")
		return 2
	}
	if *filePath == "" {
		fmt.Fprintln(stderr, "ERROR --file is required")
		return 2
	}
	if *dryRun && *apply {
		fmt.Fprintln(stderr, "ERROR --dry-run and --apply are mutually exclusive")
		return 2
	}
	payload, err := bitableAttach(ixfbitable.AttachConfig{
		URL:         *targetURL,
		Field:       *field,
		RecordID:    *recordID,
		RecordMatch: *recordMatch,
		FilePath:    *filePath,
		DryRun:      *dryRun,
		Apply:       *apply,
		CookiesPath: *cookiesPath,
		SpaceAPI:    *spaceAPI,
	})
	if err != nil {
		fmt.Fprintf(stderr, "ERROR %s\n", err)
		return 2
	}
	if *asJSON {
		writeJSON(stdout, payload)
		return 0
	}
	formatBitableSummary(stdout, payload)
	return 0
}

func formatBitableSummary(w io.Writer, payload map[string]any) {
	fmt.Fprintf(w, "ok %t\n", boolFromMap(payload, "ok"))
	if operation, _ := payload["operation"].(string); operation != "" {
		fmt.Fprintf(w, "operation %s\n", operation)
	}
	if sourceKind, _ := payload["sourceKind"].(string); sourceKind != "" {
		fmt.Fprintf(w, "source_kind %s\n", sourceKind)
	}
	if dryRun, ok := payload["dryRun"].(bool); ok {
		fmt.Fprintf(w, "dry_run %t\n", dryRun)
	}
}

func runDocs(args []string, stdout io.Writer, stderr io.Writer) int {
	rows := [][2]string{
		{"read", "Read authorized cloud document links or local Markdown files."},
		{"outline", "Print heading-aware chunk metadata for Markdown."},
		{"chunk", "Print one heading-aware Markdown chunk."},
		{"inspect", "Print a safe local/remote source routing summary."},
		{"cleanup", "Remove generated docs read artifacts."},
		{"publish", "Create a new authorized docx document from Markdown."},
		{"update", "Update an existing docx body from Markdown."},
		{"patch", "Plan or apply localized docx/wiki block patches."},
		{"table", "Plan or apply native docx table row changes."},
		{"structure", "Print safe docx/wiki structure preflight metadata."},
	}
	if len(args) == 0 {
		fmt.Fprintln(stderr, "ERROR docs requires a subcommand.")
		printCommandHelp(stderr, "ixf docs", rows)
		return 2
	}
	if isHelpArg(args[0]) {
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
	case "update":
		return runDocsUpdate(args[1:], stdout, stderr)
	case "patch":
		return runDocsPatch(args[1:], stdout, stderr)
	case "table":
		return runDocsTable(args[1:], stdout, stderr)
	case "structure":
		return runDocsStructure(args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "ERROR unsupported docs subcommand: %s\n", args[0])
		printCommandHelp(stderr, "ixf docs", rows)
		return 2
	}
}

func runDocsTable(args []string, stdout io.Writer, stderr io.Writer) int {
	rows := [][2]string{
		{"append-row", "Append one row to a native docx table, including image cells."},
	}
	if len(args) == 0 {
		fmt.Fprintln(stderr, "ERROR docs table requires a subcommand.")
		printCommandHelp(stderr, "ixf docs table", rows)
		return 2
	}
	if isHelpArg(args[0]) {
		printCommandHelp(stdout, "ixf docs table", rows)
		return 0
	}
	switch args[0] {
	case "append-row":
		return runDocsTableAppendRow(args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "ERROR unsupported docs table subcommand: %s\n", args[0])
		printCommandHelp(stderr, "ixf docs table", rows)
		return 2
	}
}

func runDocsTableAppendRow(args []string, stdout io.Writer, stderr io.Writer) int {
	flags := flag.NewFlagSet("ixf docs table append-row", flag.ContinueOnError)
	flags.SetOutput(stderr)
	targetURL := flags.String("url", "", "")
	inputPath := flags.String("input", "", "")
	tableIndex := flags.Int("table-index", 0, "")
	cookiesPath := flags.String("cookies", defaultCookies, "")
	spaceAPI := flags.String("space-api", docslocal.DefaultSpaceAPI, "")
	dryRun := flags.Bool("dry-run", false, "")
	apply := flags.Bool("apply", false, "")
	asJSON := flags.Bool("json", false, "")
	var required repeatedStringFlag
	flags.Var(&required, "require", "")
	if hasHelpArg(args) {
		flags.SetOutput(stdout)
		flags.Usage()
		return 0
	}
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
	if *dryRun && *apply {
		fmt.Fprintln(stderr, "ERROR --dry-run and --apply are mutually exclusive")
		return 2
	}
	payload, err := docsTableAppendRow(docspublish.TableAppendRowConfig{
		URL:          *targetURL,
		InputPath:    *inputPath,
		CookiesPath:  *cookiesPath,
		SpaceAPI:     *spaceAPI,
		TableIndex:   *tableIndex,
		RequiredText: []string(required),
		DryRun:       *dryRun,
		Apply:        *apply,
	})
	if err != nil {
		fmt.Fprintf(stderr, "ERROR %s\n", err)
		return 2
	}
	if *asJSON {
		writeJSON(stdout, payload)
		return 0
	}
	writeJSON(stdout, payload)
	return 0
}

func runDocsRead(args []string, stdout io.Writer, stderr io.Writer) int {
	if hasHelpArg(args) {
		printDocsReadHelp(stdout)
		return 0
	}
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

func runDocsStructure(args []string, stdout io.Writer, stderr io.Writer) int {
	if hasHelpArg(args) {
		printDocsStructureHelp(stdout)
		return 0
	}
	parsed, err := parseDocsStructureArgs(args)
	if err != nil {
		fmt.Fprintf(stderr, "ERROR %s\n", err)
		return 2
	}
	payload, err := docspublish.Structure(parsed.source, parsed.cookiesPath, parsed.spaceAPI)
	if err != nil {
		fmt.Fprintf(stderr, "ERROR %s\n", err)
		return 2
	}
	writeJSON(stdout, payload)
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
	if hasHelpArg(args) {
		printDocsPublishHelp(stdout)
		return 0
	}
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
	payload["baseURLSource"] = parsed.baseURLSource
	if host := hostFromURL(parsed.baseURL); host != "" {
		payload["targetHost"] = host
	}
	writeJSON(stdout, payload)
	return 0
}

func runDocsUpdate(args []string, stdout io.Writer, stderr io.Writer) int {
	if hasHelpArg(args) {
		printDocsUpdateHelp(stdout)
		return 0
	}
	parsed, err := parseDocsUpdateArgs(args)
	if err != nil {
		fmt.Fprintf(stderr, "ERROR %s\n", err)
		return 2
	}
	payload, err := docspublish.UpdateMarkdown(docspublish.UpdateConfig{
		MarkdownPath: parsed.markdown,
		URL:          parsed.url,
		CookiesPath:  parsed.cookiesPath,
		SpaceAPI:     parsed.spaceAPI,
		RequiredText: parsed.requiredText,
		AllowComplex: parsed.allowComplex,
		Apply:        parsed.apply,
	})
	if err != nil {
		fmt.Fprintf(stderr, "ERROR %s\n", err)
		return 2
	}
	writeJSON(stdout, payload)
	return 0
}

func runDocsPatch(args []string, stdout io.Writer, stderr io.Writer) int {
	rows := [][2]string{
		{"insert", "Insert Markdown blocks under a heading without replacing the body."},
		{"replace-section", "Replace one heading section with Markdown blocks."},
		{"delete-section", "Delete one heading section."},
	}
	if len(args) == 0 {
		fmt.Fprintln(stderr, "ERROR docs patch requires a subcommand.")
		printCommandHelp(stderr, "ixf docs patch", rows)
		return 2
	}
	if isHelpArg(args[0]) {
		printCommandHelp(stdout, "ixf docs patch", rows)
		return 0
	}
	switch args[0] {
	case "insert":
		return runDocsPatchInsert(args[1:], stdout, stderr)
	case "replace-section":
		return runDocsPatchReplaceSection(args[1:], stdout, stderr)
	case "delete-section":
		return runDocsPatchDeleteSection(args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "ERROR unsupported docs patch subcommand: %s\n", args[0])
		printCommandHelp(stderr, "ixf docs patch", rows)
		return 2
	}
}

func runDocsPatchInsert(args []string, stdout io.Writer, stderr io.Writer) int {
	if hasHelpArg(args) {
		printDocsPatchInsertHelp(stdout)
		return 0
	}
	parsed, err := parseDocsPatchInsertArgs(args)
	if err != nil {
		fmt.Fprintf(stderr, "ERROR %s\n", err)
		return 2
	}
	payload, err := docspublish.PatchInsertMarkdown(docspublish.PatchInsertConfig{
		MarkdownPath: parsed.markdown,
		URL:          parsed.url,
		CookiesPath:  parsed.cookiesPath,
		SpaceAPI:     parsed.spaceAPI,
		UnderHeading: parsed.underHeading,
		Position:     parsed.position,
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

func runDocsPatchReplaceSection(args []string, stdout io.Writer, stderr io.Writer) int {
	if hasHelpArg(args) {
		printDocsPatchReplaceSectionHelp(stdout)
		return 0
	}
	parsed, err := parseDocsPatchSectionArgs(args, false)
	if err != nil {
		fmt.Fprintf(stderr, "ERROR %s\n", err)
		return 2
	}
	payload, err := docspublish.PatchSectionMarkdown(docspublish.PatchSectionConfig{
		MarkdownPath: parsed.markdown,
		URL:          parsed.url,
		CookiesPath:  parsed.cookiesPath,
		SpaceAPI:     parsed.spaceAPI,
		UnderHeading: parsed.underHeading,
		RequiredText: parsed.requiredText,
		AllowComplex: parsed.allowComplex,
		Apply:        parsed.apply,
		DeleteOnly:   false,
	})
	if err != nil {
		fmt.Fprintf(stderr, "ERROR %s\n", err)
		return 2
	}
	writeJSON(stdout, payload)
	return 0
}

func runDocsPatchDeleteSection(args []string, stdout io.Writer, stderr io.Writer) int {
	if hasHelpArg(args) {
		printDocsPatchDeleteSectionHelp(stdout)
		return 0
	}
	parsed, err := parseDocsPatchSectionArgs(args, true)
	if err != nil {
		fmt.Fprintf(stderr, "ERROR %s\n", err)
		return 2
	}
	payload, err := docspublish.PatchSectionMarkdown(docspublish.PatchSectionConfig{
		URL:          parsed.url,
		CookiesPath:  parsed.cookiesPath,
		SpaceAPI:     parsed.spaceAPI,
		UnderHeading: parsed.underHeading,
		RequiredText: parsed.requiredText,
		AllowComplex: parsed.allowComplex,
		Apply:        parsed.apply,
		DeleteOnly:   true,
	})
	if err != nil {
		fmt.Fprintf(stderr, "ERROR %s\n", err)
		return 2
	}
	writeJSON(stdout, payload)
	return 0
}

func printDocsReadHelp(w io.Writer) {
	printUsageHelp(w, "ixf docs read <source>... [--out-dir DIR] [--print-manifest] [--expand-sheets]", [][2]string{
		{"--out-dir DIR", "Write read artifacts under DIR instead of printing content."},
		{"--print-manifest", "Print the generated artifact manifest; requires --out-dir."},
		{"--cleanup", "Remove generated artifacts after reading; requires --out-dir."},
		{"--expand-sheets", "Expand embedded sheet blocks into local TSV/Markdown artifacts when available."},
		{"--download-images", "Download referenced images into the output directory."},
		{"--cookies PATH", "Read exported desktop session cookies from PATH."},
		{"--space-api URL", "Override the i讯飞 Space API base URL."},
	})
}

func printDocsStructureHelp(w io.Writer) {
	printUsageHelp(w, "ixf docs structure <doc-or-wiki-url> [--json]", [][2]string{
		{"--json", "Print safe structure preflight metadata as JSON."},
		{"--cookies PATH", "Read exported desktop session cookies from PATH."},
		{"--space-api URL", "Override the i讯飞 Space API base URL."},
	})
}

func printDocsPublishHelp(w io.Writer) {
	printUsageHelp(w, "ixf docs publish <markdown.md> [--base-url URL] [--dry-run|--apply]", [][2]string{
		{"--base-url URL", "Target tenant base URL; optional when IXF_DOCS_DEFAULT_BASE_URL or docs.defaultBaseURL is configured."},
		{"--space-api URL", "Override the i讯飞 Space API base URL."},
		{"--cookies PATH", "Read exported desktop session cookies from PATH."},
		{"--member-id ID", "Override the authenticated document member ID."},
		{"--parent-token TOKEN", "Create the document under a specific parent token."},
		{"--title TITLE", "Override the Markdown H1 title."},
		{"--title-suffix TEXT", "Append text to the Markdown H1 title."},
		{"--require TEXT", "Require TEXT to appear during post-write verification."},
		{"--dry-run", "Plan the publish without writing remote content."},
		{"--apply", "Create the remote document."},
	})
}

func printDocsUpdateHelp(w io.Writer) {
	printUsageHelp(w, "ixf docs update <markdown.md> --url DOCX_URL [--dry-run|--apply]", [][2]string{
		{"--url DOCX_URL", "Direct existing docx URL to update."},
		{"--space-api URL", "Override the i讯飞 Space API base URL."},
		{"--cookies PATH", "Read exported desktop session cookies from PATH."},
		{"--require TEXT", "Require TEXT to appear during post-write verification."},
		{"--allow-complex-replace", "Permit replacing body content when unsupported existing blocks are present."},
		{"--dry-run", "Plan the update without writing remote content."},
		{"--apply", "Replace the remote docx body."},
	})
}

func printDocsPatchInsertHelp(w io.Writer) {
	printUsageHelp(w, "ixf docs patch insert <fragment.md> --url URL --under-heading TEXT [--position section-end|after-heading] [--dry-run|--apply]", [][2]string{
		{"--url URL", "Existing docx URL or wiki-backed docx URL to patch."},
		{"--space-api URL", "Override the i讯飞 Space API base URL."},
		{"--cookies PATH", "Read exported desktop session cookies from PATH."},
		{"--under-heading TEXT", "Insert under the unique heading matching TEXT."},
		{"--position VALUE", "Insert at section-end (default) or after-heading."},
		{"--require TEXT", "Require TEXT to appear during post-write verification."},
		{"--dry-run", "Plan the localized insert without writing remote content."},
		{"--apply", "Apply the localized insert."},
	})
}

func printDocsPatchReplaceSectionHelp(w io.Writer) {
	printUsageHelp(w, "ixf docs patch replace-section <fragment.md> --url URL --under-heading TEXT [--dry-run|--apply]", [][2]string{
		{"--url URL", "Existing docx URL or wiki-backed docx URL to patch."},
		{"--space-api URL", "Override the i讯飞 Space API base URL."},
		{"--cookies PATH", "Read exported desktop session cookies from PATH."},
		{"--under-heading TEXT", "Replace the unique section heading matching TEXT."},
		{"--require TEXT", "Require TEXT to appear during post-write verification."},
		{"--allow-complex-section-replace", "Permit replacing a section containing unsupported rich blocks."},
		{"--dry-run", "Plan the bounded section replace without writing remote content."},
		{"--apply", "Apply the bounded section replace."},
	})
}

func printDocsPatchDeleteSectionHelp(w io.Writer) {
	printUsageHelp(w, "ixf docs patch delete-section --url URL --under-heading TEXT [--dry-run|--apply]", [][2]string{
		{"--url URL", "Existing docx URL or wiki-backed docx URL to patch."},
		{"--space-api URL", "Override the i讯飞 Space API base URL."},
		{"--cookies PATH", "Read exported desktop session cookies from PATH."},
		{"--under-heading TEXT", "Delete the unique section heading matching TEXT."},
		{"--require TEXT", "Require TEXT to appear during post-write verification."},
		{"--allow-complex-section-replace", "Permit deleting a section containing unsupported rich blocks."},
		{"--dry-run", "Plan the bounded section delete without writing remote content."},
		{"--apply", "Apply the bounded section delete."},
	})
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
	if isHelpArg(args[0]) {
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
	if hasHelpArg(args) {
		printOKRReadHelp(stdout)
		return 0
	}
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

func printOKRReadHelp(w io.Writer) {
	printUsageHelp(w, "ixf okr read <okr-url> [--cookies PATH] [--csrf-url URL]", [][2]string{
		{"--cookies PATH", "Read exported desktop session cookies from PATH."},
		{"--csrf-url URL", "Override the URL used to establish CSRF/session readiness."},
	})
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
	if hasHelpArg(args) {
		flags.SetOutput(stdout)
		flags.Usage()
		return 0
	}
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

func runMessenger(args []string, stdout io.Writer, stderr io.Writer) int {
	rows := [][2]string{
		{"doctor", "Inspect local Messenger automation readiness."},
		{"open", "Plan opening a person or conversation without sending."},
		{"read", "Read recent or unread conversations without sending."},
		{"send", "Plan or apply a verified message send."},
	}
	if len(args) == 0 {
		fmt.Fprintln(stderr, "ERROR messenger requires a subcommand.")
		printCommandHelp(stderr, "ixf messenger", rows)
		return 2
	}
	if isHelpArg(args[0]) {
		printCommandHelp(stdout, "ixf messenger", rows)
		return 0
	}
	switch args[0] {
	case "doctor":
		return runMessengerDoctor(args[1:], stdout, stderr)
	case "open":
		return runMessengerOpen(args[1:], stdout, stderr)
	case "read":
		return runMessengerRead(args[1:], stdout, stderr)
	case "send":
		return runMessengerSend(args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "ERROR unsupported messenger subcommand: %s\n", args[0])
		printCommandHelp(stderr, "ixf messenger", rows)
		return 2
	}
}

func runMessengerDoctor(args []string, stdout io.Writer, stderr io.Writer) int {
	flags := flag.NewFlagSet("ixf messenger doctor", flag.ContinueOnError)
	flags.SetOutput(stderr)
	config, asJSON := messengerFlags(flags)
	if hasHelpArg(args) {
		flags.SetOutput(stdout)
		flags.Usage()
		return 0
	}
	if err := flags.Parse(args); err != nil {
		return 2
	}
	payload := messenger.Doctor(config())
	if *asJSON {
		writeJSON(stdout, payload)
	} else {
		formatMessengerDiagnostics(stdout, payload)
	}
	if ok, _ := payload["ok"].(bool); ok {
		return 0
	}
	return 1
}

func runMessengerOpen(args []string, stdout io.Writer, stderr io.Writer) int {
	flags := flag.NewFlagSet("ixf messenger open", flag.ContinueOnError)
	flags.SetOutput(stderr)
	config, asJSON := messengerFlags(flags)
	target := flags.String("to", "", "")
	mode := flags.String("mode", "", "")
	dryRun := flags.Bool("dry-run", false, "")
	apply := flags.Bool("apply", false, "")
	allowVisibleFallback := flags.Bool("allow-visible-fallback", false, "")
	keepProfileClone := flags.Bool("keep-profile-clone", false, "")
	timeoutMS := flags.Int("timeout-ms", 45000, "")
	if hasHelpArg(args) {
		flags.SetOutput(stdout)
		flags.Usage()
		return 0
	}
	if err := flags.Parse(args); err != nil {
		return 2
	}
	payload, err := messenger.OpenTarget(context.Background(), messenger.OpenConfig{
		Config:               config(),
		Target:               *target,
		Mode:                 *mode,
		DryRun:               *dryRun,
		Apply:                *apply,
		AllowVisibleFallback: *allowVisibleFallback,
		KeepProfileClone:     *keepProfileClone,
		TimeoutMS:            *timeoutMS,
	}, messenger.ChromedpAutomator{})
	if err != nil {
		fmt.Fprintf(stderr, "ERROR %s\n", err)
		return 2
	}
	if *asJSON {
		writeJSON(stdout, payload)
		return 0
	}
	fmt.Fprintf(stdout, "target %s\n", payload["target"])
	fmt.Fprintf(stdout, "mode %s\n", payload["mode"])
	fmt.Fprintf(stdout, "dry_run %t\n", payload["dryRun"])
	fmt.Fprintf(stdout, "apply %t\n", payload["apply"])
	fmt.Fprintf(stdout, "will_send %t\n", payload["willSend"])
	fmt.Fprintf(stdout, "target_verified %t\n", payload["targetVerified"])
	return 0
}

func runMessengerRead(args []string, stdout io.Writer, stderr io.Writer) int {
	flags := flag.NewFlagSet("ixf messenger read", flag.ContinueOnError)
	flags.SetOutput(stderr)
	config, asJSON := messengerFlags(flags)
	scope := flags.String("scope", "unread", "")
	limit := flags.Int("limit", 20, "")
	messagesPerChat := flags.Int("messages-per-chat", 5, "")
	maxScrolls := flags.Int("max-scrolls", 18, "")
	includeSelfMessages := flags.Bool("include-self-messages", false, "")
	dryRun := flags.Bool("dry-run", false, "")
	apply := flags.Bool("apply", false, "")
	allowVisibleFallback := flags.Bool("allow-visible-fallback", false, "")
	keepProfileClone := flags.Bool("keep-profile-clone", false, "")
	timeoutMS := flags.Int("timeout-ms", 60000, "")
	if hasHelpArg(args) {
		flags.SetOutput(stdout)
		flags.Usage()
		return 0
	}
	if err := flags.Parse(args); err != nil {
		return 2
	}
	payload, err := messenger.ReadMessages(context.Background(), messenger.ReadConfig{
		Config:               config(),
		Scope:                *scope,
		Limit:                *limit,
		MessagesPerChat:      *messagesPerChat,
		MaxScrolls:           *maxScrolls,
		IncludeSelfMessages:  *includeSelfMessages,
		DryRun:               *dryRun,
		Apply:                *apply,
		AllowVisibleFallback: *allowVisibleFallback,
		KeepProfileClone:     *keepProfileClone,
		TimeoutMS:            *timeoutMS,
	}, messenger.ChromedpAutomator{})
	if err != nil {
		fmt.Fprintf(stderr, "ERROR %s\n", err)
		return 2
	}
	if *asJSON {
		writeJSON(stdout, payload)
		return 0
	}
	fmt.Fprintf(stdout, "scope %s\n", payload["scope"])
	fmt.Fprintf(stdout, "dry_run %t\n", payload["dryRun"])
	fmt.Fprintf(stdout, "apply %t\n", payload["apply"])
	fmt.Fprintf(stdout, "will_send %t\n", payload["willSend"])
	fmt.Fprintf(stdout, "recent_seen %v\n", payload["totalRecentConversationsSeen"])
	fmt.Fprintf(stdout, "unread_conversations %v\n", payload["totalUnreadConversations"])
	fmt.Fprintf(stdout, "extracted_conversations %v\n", payload["totalExtractedConversations"])
	return 0
}

func runMessengerSend(args []string, stdout io.Writer, stderr io.Writer) int {
	flags := flag.NewFlagSet("ixf messenger send", flag.ContinueOnError)
	flags.SetOutput(stderr)
	config, asJSON := messengerFlags(flags)
	target := flags.String("to", "", "")
	mode := flags.String("mode", "", "")
	message := flags.String("message", "", "")
	dryRun := flags.Bool("dry-run", false, "")
	apply := flags.Bool("apply", false, "")
	allowVisibleFallback := flags.Bool("allow-visible-fallback", false, "")
	keepProfileClone := flags.Bool("keep-profile-clone", false, "")
	timeoutMS := flags.Int("timeout-ms", 90000, "")
	if hasHelpArg(args) {
		flags.SetOutput(stdout)
		flags.Usage()
		return 0
	}
	if err := flags.Parse(args); err != nil {
		return 2
	}
	payload, err := messenger.SendMessage(context.Background(), messenger.SendConfig{
		Config:               config(),
		Target:               *target,
		Mode:                 *mode,
		Message:              *message,
		DryRun:               *dryRun,
		Apply:                *apply,
		AllowVisibleFallback: *allowVisibleFallback,
		KeepProfileClone:     *keepProfileClone,
		TimeoutMS:            *timeoutMS,
	}, messenger.ChromedpAutomator{})
	if err != nil {
		fmt.Fprintf(stderr, "ERROR %s\n", err)
		return 2
	}
	if *asJSON {
		writeJSON(stdout, payload)
		return 0
	}
	fmt.Fprintf(stdout, "target %s\n", payload["target"])
	fmt.Fprintf(stdout, "mode %s\n", payload["mode"])
	fmt.Fprintf(stdout, "dry_run %t\n", payload["dryRun"])
	fmt.Fprintf(stdout, "apply %t\n", payload["apply"])
	fmt.Fprintf(stdout, "will_send %t\n", payload["willSend"])
	fmt.Fprintf(stdout, "sent %t\n", payload["sent"])
	fmt.Fprintf(stdout, "verified_present %t\n", payload["verifiedPresent"])
	return 0
}

func messengerFlags(flags *flag.FlagSet) (func() messenger.Config, *bool) {
	profileDir := flags.String("profile-dir", "", "")
	appSupport := flags.String("app-support", "", "")
	appData := flags.String("app-data", "", "")
	browserPath := flags.String("browser-path", "", "")
	cookiesPath := flags.String("cookies", messenger.DefaultCookieJSON, "")
	goos := flags.String("goos", "", "")
	asJSON := flags.Bool("json", false, "")
	return func() messenger.Config {
		return messenger.Config{
			GOOS:        *goos,
			AppSupport:  *appSupport,
			AppData:     *appData,
			ProfileDir:  *profileDir,
			BrowserPath: *browserPath,
			CookiesPath: *cookiesPath,
		}
	}, asJSON
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

type docsStructureArgs struct {
	source      string
	cookiesPath string
	spaceAPI    string
	asJSON      bool
}

type docsPublishArgs struct {
	markdown      string
	baseURL       string
	baseURLSource string
	cookiesPath   string
	spaceAPI      string
	memberID      string
	parentToken   string
	title         string
	titleSuffix   string
	requiredText  []string
	apply         bool
	dryRun        bool
}

type toolboxConfig struct {
	Docs struct {
		DefaultBaseURL string `json:"defaultBaseURL"`
	} `json:"docs"`
}

type docsUpdateArgs struct {
	markdown     string
	url          string
	cookiesPath  string
	spaceAPI     string
	requiredText []string
	allowComplex bool
	apply        bool
	dryRun       bool
}

type docsPatchInsertArgs struct {
	markdown     string
	url          string
	cookiesPath  string
	spaceAPI     string
	underHeading string
	position     string
	requiredText []string
	apply        bool
	dryRun       bool
}

type docsPatchSectionArgs struct {
	markdown     string
	url          string
	cookiesPath  string
	spaceAPI     string
	underHeading string
	requiredText []string
	allowComplex bool
	apply        bool
	dryRun       bool
	deleteOnly   bool
}

type repeatedStringFlag []string

func (flag *repeatedStringFlag) String() string {
	return strings.Join(*flag, ",")
}

func (flag *repeatedStringFlag) Set(value string) error {
	*flag = append(*flag, value)
	return nil
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

func parseDocsStructureArgs(args []string) (docsStructureArgs, error) {
	parsed := docsStructureArgs{
		cookiesPath: defaultCookies,
		spaceAPI:    docslocal.DefaultSpaceAPI,
	}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "--json":
			parsed.asJSON = true
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
				return parsed, fmt.Errorf("unsupported docs structure flag: %s", arg)
			}
			if parsed.source != "" {
				return parsed, fmt.Errorf("structure requires exactly one source URL")
			}
			parsed.source = arg
		}
	}
	if parsed.source == "" {
		return parsed, fmt.Errorf("structure requires one source URL")
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
			parsed.baseURLSource = "flag"
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
		value, source, err := resolveDocsDefaultBaseURL()
		if err != nil {
			return parsed, err
		}
		parsed.baseURL = value
		parsed.baseURLSource = source
	}
	if parsed.baseURL == "" {
		return parsed, fmt.Errorf("--base-url is required; set %s or docs.defaultBaseURL in %s to use a default", docsDefaultBaseURLEnv, docsConfigPath())
	}
	if parsed.dryRun {
		parsed.apply = false
	}
	return parsed, nil
}

func parseDocsUpdateArgs(args []string) (docsUpdateArgs, error) {
	parsed := docsUpdateArgs{
		cookiesPath: defaultCookies,
		spaceAPI:    docslocal.DefaultSpaceAPI,
	}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "--url":
			i++
			if i >= len(args) {
				return parsed, fmt.Errorf("%s requires a value", arg)
			}
			parsed.url = args[i]
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
		case "--require":
			i++
			if i >= len(args) {
				return parsed, fmt.Errorf("%s requires a value", arg)
			}
			parsed.requiredText = append(parsed.requiredText, args[i])
		case "--allow-complex-replace":
			parsed.allowComplex = true
		case "--apply":
			parsed.apply = true
		case "--dry-run":
			parsed.dryRun = true
		default:
			if strings.HasPrefix(arg, "-") {
				return parsed, fmt.Errorf("unsupported docs update flag: %s", arg)
			}
			if parsed.markdown != "" {
				return parsed, fmt.Errorf("update requires exactly one Markdown file")
			}
			parsed.markdown = arg
		}
	}
	if parsed.markdown == "" {
		return parsed, fmt.Errorf("update requires one Markdown file")
	}
	if parsed.url == "" {
		return parsed, fmt.Errorf("--url is required")
	}
	if parsed.dryRun && parsed.apply {
		return parsed, fmt.Errorf("--dry-run and --apply are mutually exclusive")
	}
	return parsed, nil
}

func parseDocsPatchInsertArgs(args []string) (docsPatchInsertArgs, error) {
	parsed := docsPatchInsertArgs{
		cookiesPath: defaultCookies,
		spaceAPI:    docslocal.DefaultSpaceAPI,
		position:    "section-end",
	}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "--url":
			i++
			if i >= len(args) {
				return parsed, fmt.Errorf("%s requires a value", arg)
			}
			parsed.url = args[i]
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
		case "--under-heading":
			i++
			if i >= len(args) {
				return parsed, fmt.Errorf("%s requires a value", arg)
			}
			parsed.underHeading = args[i]
		case "--position":
			i++
			if i >= len(args) {
				return parsed, fmt.Errorf("%s requires a value", arg)
			}
			parsed.position = args[i]
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
				return parsed, fmt.Errorf("unsupported docs patch insert flag: %s", arg)
			}
			if parsed.markdown != "" {
				return parsed, fmt.Errorf("patch insert requires exactly one Markdown file")
			}
			parsed.markdown = arg
		}
	}
	if parsed.markdown == "" {
		return parsed, fmt.Errorf("patch insert requires one Markdown file")
	}
	if parsed.url == "" {
		return parsed, fmt.Errorf("--url is required")
	}
	if parsed.underHeading == "" {
		return parsed, fmt.Errorf("--under-heading is required")
	}
	if parsed.dryRun && parsed.apply {
		return parsed, fmt.Errorf("--dry-run and --apply are mutually exclusive")
	}
	return parsed, nil
}

func parseDocsPatchSectionArgs(args []string, deleteOnly bool) (docsPatchSectionArgs, error) {
	parsed := docsPatchSectionArgs{
		cookiesPath: defaultCookies,
		spaceAPI:    docslocal.DefaultSpaceAPI,
		deleteOnly:  deleteOnly,
	}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "--url":
			i++
			if i >= len(args) {
				return parsed, fmt.Errorf("%s requires a value", arg)
			}
			parsed.url = args[i]
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
		case "--under-heading":
			i++
			if i >= len(args) {
				return parsed, fmt.Errorf("%s requires a value", arg)
			}
			parsed.underHeading = args[i]
		case "--require":
			i++
			if i >= len(args) {
				return parsed, fmt.Errorf("%s requires a value", arg)
			}
			parsed.requiredText = append(parsed.requiredText, args[i])
		case "--allow-complex-section-replace":
			parsed.allowComplex = true
		case "--apply":
			parsed.apply = true
		case "--dry-run":
			parsed.dryRun = true
		default:
			if strings.HasPrefix(arg, "-") {
				if deleteOnly {
					return parsed, fmt.Errorf("unsupported docs patch delete-section flag: %s", arg)
				}
				return parsed, fmt.Errorf("unsupported docs patch replace-section flag: %s", arg)
			}
			if deleteOnly {
				return parsed, fmt.Errorf("patch delete-section does not accept a Markdown file")
			}
			if parsed.markdown != "" {
				return parsed, fmt.Errorf("patch replace-section requires exactly one Markdown file")
			}
			parsed.markdown = arg
		}
	}
	if !deleteOnly && parsed.markdown == "" {
		return parsed, fmt.Errorf("patch replace-section requires one Markdown file")
	}
	if parsed.url == "" {
		return parsed, fmt.Errorf("--url is required")
	}
	if parsed.underHeading == "" {
		return parsed, fmt.Errorf("--under-heading is required")
	}
	if parsed.dryRun && parsed.apply {
		return parsed, fmt.Errorf("--dry-run and --apply are mutually exclusive")
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
	rows := [][2]string{
		{"check", "Check the latest ixf Toolbox release."},
		{"self", "Download and optionally apply a CLI self-update."},
		{"skills", "Refresh installed agent skill wrappers."},
	}
	if len(args) == 0 {
		fmt.Fprintln(stderr, "ERROR update requires a subcommand")
		printCommandHelp(stderr, "ixf update", rows)
		return 2
	}
	if isHelpArg(args[0]) {
		printCommandHelp(stdout, "ixf update", rows)
		return 0
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
	if hasHelpArg(args) {
		printUpdateSelfHelp(stdout)
		return 0
	}
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

func printUpdateSelfHelp(w io.Writer) {
	printUsageHelp(w, "ixf update self [--target-path PATH] [--apply] [--json]", [][2]string{
		{"--repo OWNER/REPO", "GitHub repository to inspect for releases."},
		{"--release-file PATH", "Use a local release JSON fixture."},
		{"--target-path PATH", "Install the downloaded binary at PATH."},
		{"--apply", "Replace the target binary; omitted means dry-run/check only."},
		{"--json", "Print machine-readable JSON output."},
	})
}

func runUpdateSkills(args []string, stdout io.Writer, stderr io.Writer) int {
	flags := flag.NewFlagSet("ixf update skills", flag.ContinueOnError)
	flags.SetOutput(stderr)
	runtimesRaw := flags.String("runtimes", "auto", "")
	asJSON := flags.Bool("json", false, "")
	if hasHelpArg(args) {
		flags.SetOutput(stdout)
		flags.Usage()
		return 0
	}
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
	legacyCommands := legacyCommandsStatus()
	dependencies := dependencyDiagnostics(cookiesPath)
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
			"docsRead":           true,
			"docsPublish":        true,
			"sheetsRead":         true,
			"sheetsUpdateDryRun": true,
			"sheetsUpdateApply":  true,
			"okrRead":            true,
			"okrWrite":           true,
			"cookiesExport":      true,
			"messengerDoctor":    true,
			"messengerOpenPlan":  true,
			"messengerOpenApply": true,
			"messengerReadPlan":  true,
			"messengerReadApply": true,
			"messengerSendPlan":  true,
			"messengerSendApply": true,
		},
		"skills":         skills,
		"cookies":        cookies,
		"docs":           docsDiagnostics(),
		"dependencies":   dependencies,
		"legacyCommands": legacyCommands,
		"agentRouting":   agentRoutingStatus(),
	}
}

func dependencyDiagnostics(cookiesPath string) map[string]any {
	mermaid := docspublish.MermaidDependencyStatus()
	messengerStatus := messengerDependencyStatus(cookiesPath)
	updateStatus := updateDependencyStatus()
	return map[string]any{
		"ok":        boolFromMap(mermaid, "ok") && boolFromMap(messengerStatus, "ok") && boolFromMap(updateStatus, "ok"),
		"mermaid":   mermaid,
		"messenger": messengerStatus,
		"update":    updateStatus,
	}
}

func messengerDependencyStatus(cookiesPath string) map[string]any {
	payload := messenger.Doctor(messenger.Config{CookiesPath: cookiesPath})
	profile := map[string]any{"ok": false}
	if value, ok := payload["profile"].(messenger.ProfileDiscovery); ok {
		profile["ok"] = value.OK
		if value.Source != "" {
			profile["source"] = value.Source
		}
		if value.Error != "" {
			profile["error"] = value.Error
		}
	}
	browser := map[string]any{"ok": false}
	if value, ok := payload["browser"].(messenger.BrowserDiscovery); ok {
		browser["ok"] = value.OK
		if value.Source != "" {
			browser["source"] = value.Source
		}
		if value.Error != "" {
			browser["error"] = value.Error
		}
	}
	cookies := map[string]any{
		"ok":          boolFromMap(payload["cookies"], "ok"),
		"exists":      boolFromMap(payload["cookies"], "exists"),
		"cookieCount": intFromMap(payload["cookies"], "cookieCount"),
		"hasCsrf":     boolFromMap(payload["cookies"], "hasCsrf"),
		"hasLgwCsrf":  boolFromMap(payload["cookies"], "hasLgwCsrf"),
	}
	result := map[string]any{
		"ok":          boolFromMap(payload, "ok"),
		"installable": false,
		"requiredFor": "messenger browser automation",
		"profile":     profile,
		"browser":     browser,
		"cookies":     cookies,
	}
	if messengerInfo, ok := payload["messenger"].(map[string]any); ok {
		result["supportedPlatform"] = boolFromMap(messengerInfo, "supportedPlatform")
		if goosValue, _ := messengerInfo["goos"].(string); goosValue != "" {
			result["goos"] = goosValue
		}
	}
	if remediation, ok := payload["remediation"].([]string); ok && len(remediation) > 0 {
		result["remediation"] = remediation
	}
	return result
}

func updateDependencyStatus() map[string]any {
	result := map[string]any{
		"ok":          false,
		"repo":        ixfupdate.DefaultReleaseRepo,
		"requiredFor": "update check and self-update",
		"installable": false,
	}
	release, err := dependencyReleaseLoader(ixfupdate.DefaultReleaseRepo, "")
	if err != nil {
		result["error"] = err.Error()
		result["remediation"] = "Ensure GitHub Releases are reachable. If a proxy is required, set HTTPS_PROXY, HTTP_PROXY, and ALL_PROXY before running update commands."
		return result
	}
	check, err := ixfupdate.CheckLatestRelease(ixfupdate.DefaultReleaseRepo, version, release)
	if err != nil {
		result["error"] = err.Error()
		result["remediation"] = "Check the release metadata returned by GitHub and retry `ixf update check --json`."
		return result
	}
	for key, value := range check {
		result[key] = value
	}
	result["ok"] = true
	if boolFromMap(result, "updateAvailable") {
		result["remediation"] = "Run `ixf update self --apply --json`, then `ixf update skills --runtimes auto --json`."
	}
	return result
}

type setupDependencyCommand struct {
	Name    string
	Args    []string
	Display string
}

func setupDependencies(cookiesPath string, apply bool) (map[string]any, error) {
	before := dependencyDiagnostics(cookiesPath)
	commands := plannedSetupDependencyCommands(before)
	displays := setupDependencyCommandDisplays(commands)
	payload := map[string]any{
		"ok":           true,
		"dryRun":       !apply,
		"apply":        apply,
		"applied":      false,
		"commands":     displays,
		"dependencies": before,
	}
	if !apply {
		return payload, nil
	}
	for _, command := range commands {
		path, err := exec.LookPath(command.Name)
		if err != nil {
			return nil, fmt.Errorf("setup dependency command %q not found; install Node.js/npm first or run manually: %s", command.Name, command.Display)
		}
		output, err := exec.Command(path, command.Args...).CombinedOutput()
		if err != nil {
			return nil, fmt.Errorf("setup dependency command failed: %s: %s", command.Display, strings.TrimSpace(string(output)))
		}
	}
	payload["applied"] = len(commands) > 0
	payload["dependencies"] = dependencyDiagnostics(cookiesPath)
	return payload, nil
}

func plannedSetupDependencyCommands(dependencies map[string]any) []setupDependencyCommand {
	mermaid := mapFromAny(dependencies["mermaid"])
	commands := []setupDependencyCommand{}
	if !boolFromMap(mermaid, "available") {
		commands = append(commands, setupDependencyCommand{
			Name:    "npm",
			Args:    []string{"install", "-g", "@mermaid-js/mermaid-cli"},
			Display: "npm install -g @mermaid-js/mermaid-cli",
		})
	}
	if !boolFromMap(mermaid, "ready") {
		commands = append(commands, setupDependencyCommand{
			Name:    "npx",
			Args:    []string{"puppeteer", "browsers", "install", "chrome-headless-shell"},
			Display: "npx puppeteer browsers install chrome-headless-shell",
		})
	}
	return commands
}

func setupDependencyCommandDisplays(commands []setupDependencyCommand) []string {
	result := make([]string, 0, len(commands))
	for _, command := range commands {
		result = append(result, command.Display)
	}
	return result
}

func formatSetupDeps(w io.Writer, payload map[string]any) {
	fmt.Fprintf(w, "dry_run %t\n", boolFromMap(payload, "dryRun"))
	fmt.Fprintf(w, "apply %t\n", boolFromMap(payload, "apply"))
	fmt.Fprintf(w, "applied %t\n", boolFromMap(payload, "applied"))
	if dependencies, ok := payload["dependencies"].(map[string]any); ok {
		fmt.Fprintf(
			w,
			"dependencies ok=%t mermaid=%t messenger=%t update=%t\n",
			boolFromMap(dependencies, "ok"),
			boolFromMap(dependencies["mermaid"], "ok"),
			boolFromMap(dependencies["messenger"], "ok"),
			boolFromMap(dependencies["update"], "ok"),
		)
	}
	if commands, ok := payload["commands"].([]string); ok {
		for _, command := range commands {
			fmt.Fprintf(w, "command %s\n", command)
		}
	}
}

func agentRoutingStatus() map[string]any {
	return map[string]any{
		"goOnly":                 true,
		"backgroundRouting":      true,
		"defaultAmbiguousIntent": "read-only",
		"currentGuidance": []string{
			"AGENTS.md",
			"docs/agent-routing.md",
			"skills/*/*/SKILL.md",
		},
		"historicalGuidanceIgnored": []string{
			"CHANGELOG.md",
			"docs/superpowers/",
		},
		"routingSkill": "using-ixf-toolbox",
		"note":         "Users describe the task naturally; installed skills route docs, OKR, sheets, and Messenger requests in the background.",
	}
}

func legacyCommandsStatus() []map[string]string {
	var result []map[string]string
	for _, name := range []string{"ixfdoc", "ixfwrite"} {
		path, err := exec.LookPath(name)
		if err != nil {
			continue
		}
		result = append(result, map[string]string{
			"name":    name,
			"path":    path,
			"runtime": "go-only",
			"status":  "ignored",
			"note":    "legacy command present; Toolbox skills use ixf only",
		})
	}
	return result
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

	capabilities := payload["capabilities"]
	fmt.Fprintf(
		w,
		"native docsRead=%t docsPublish=%t sheetsRead=%t sheetsUpdateDryRun=%t sheetsUpdateApply=%t okrRead=%t okrWrite=%t cookiesExport=%t messengerDoctor=%t messengerOpenPlan=%t messengerOpenApply=%t messengerReadPlan=%t messengerReadApply=%t messengerSendPlan=%t messengerSendApply=%t\n",
		boolFromMap(capabilities, "docsRead"),
		boolFromMap(capabilities, "docsPublish"),
		boolFromMap(capabilities, "sheetsRead"),
		boolFromMap(capabilities, "sheetsUpdateDryRun"),
		boolFromMap(capabilities, "sheetsUpdateApply"),
		boolFromMap(capabilities, "okrRead"),
		boolFromMap(capabilities, "okrWrite"),
		boolFromMap(capabilities, "cookiesExport"),
		boolFromMap(capabilities, "messengerDoctor"),
		boolFromMap(capabilities, "messengerOpenPlan"),
		boolFromMap(capabilities, "messengerOpenApply"),
		boolFromMap(capabilities, "messengerReadPlan"),
		boolFromMap(capabilities, "messengerReadApply"),
		boolFromMap(capabilities, "messengerSendPlan"),
		boolFromMap(capabilities, "messengerSendApply"),
	)

	if skills, ok := payload["skills"].(map[string]any); ok {
		runtimes := make([]string, 0, len(skills))
		for runtime := range skills {
			runtimes = append(runtimes, runtime)
		}
		sort.Strings(runtimes)
		for _, runtime := range runtimes {
			status, _ := skills[runtime].(map[string]any)
			fmt.Fprintf(w, "skill %s ok=%t", runtime, boolFromMap(status, "ok"))
			if dir, _ := status["dir"].(string); dir != "" {
				fmt.Fprintf(w, " dir=%s", dir)
			}
			fmt.Fprintln(w)
		}
	}

	if cookies, ok := payload["cookies"].(map[string]any); ok {
		fmt.Fprintf(
			w,
			"cookies %s count=%d csrf=%t lgw_csrf=%t\n",
			okWord(boolFromMap(cookies, "ok")),
			intFromMap(cookies, "cookieCount"),
			boolFromMap(cookies, "hasCsrf"),
			boolFromMap(cookies, "hasLgwCsrf"),
		)
	}
	if docs, ok := payload["docs"].(map[string]any); ok {
		if target, ok := docs["defaultBaseURL"].(map[string]any); ok {
			fmt.Fprintf(
				w,
				"docs_default_base_url configured=%t source=%s host=%s config=%s\n",
				boolFromMap(target, "configured"),
				target["source"],
				target["host"],
				target["configPath"],
			)
		}
	}
	if dependencies, ok := payload["dependencies"].(map[string]any); ok {
		fmt.Fprintf(
			w,
			"dependencies ok=%t mermaid=%t messenger=%t update=%t\n",
			boolFromMap(dependencies, "ok"),
			boolFromMap(dependencies["mermaid"], "ok"),
			boolFromMap(dependencies["messenger"], "ok"),
			boolFromMap(dependencies["update"], "ok"),
		)
	}
	if routing, ok := payload["agentRouting"].(map[string]any); ok {
		fmt.Fprintf(
			w,
			"agent_routing go_only=%t background=%t default=%s\n",
			boolFromMap(routing, "goOnly"),
			boolFromMap(routing, "backgroundRouting"),
			routing["defaultAmbiguousIntent"],
		)
	}
	if legacyCommands, ok := payload["legacyCommands"].([]map[string]string); ok {
		for _, item := range legacyCommands {
			fmt.Fprintf(w, "legacy %s %s path=%s note=%s\n", item["name"], item["status"], item["path"], item["note"])
		}
	}
	if remediation, ok := payload["remediation"].([]string); ok {
		for _, item := range remediation {
			fmt.Fprintf(w, "remediation %s\n", item)
		}
	}
}

func formatMessengerDiagnostics(w io.Writer, payload map[string]any) {
	fmt.Fprintln(w, "ixf messenger")
	if ok, _ := payload["ok"].(bool); ok {
		fmt.Fprintln(w, "overall ok")
	} else {
		fmt.Fprintln(w, "overall fail")
	}
	if messengerPayload, ok := payload["messenger"].(map[string]any); ok {
		fmt.Fprintf(w, "platform supported=%t goos=%s\n", boolFromMap(messengerPayload, "supportedPlatform"), messengerPayload["goos"])
		if stability, ok := messengerPayload["stability"].(map[string]any); ok {
			fmt.Fprintf(
				w,
				"stability operating_model=%s macos=%s windows=%s\n",
				stability["operatingModel"],
				stability["macOS"],
				stability["windows"],
			)
		}
	}
	if profile, ok := payload["profile"].(messenger.ProfileDiscovery); ok {
		fmt.Fprintf(w, "profile %s", okWord(profile.OK))
		if profile.Path != "" {
			fmt.Fprintf(w, " path=%s", profile.Path)
		}
		fmt.Fprintln(w)
	}
	if browser, ok := payload["browser"].(messenger.BrowserDiscovery); ok {
		fmt.Fprintf(w, "browser %s", okWord(browser.OK))
		if browser.Path != "" {
			fmt.Fprintf(w, " path=%s", browser.Path)
		}
		fmt.Fprintln(w)
	}
	if cookies, ok := payload["cookies"].(map[string]any); ok {
		fmt.Fprintf(
			w,
			"cookies %s count=%d csrf=%t lgw_csrf=%t\n",
			okWord(boolFromMap(cookies, "ok")),
			intFromMap(cookies, "cookieCount"),
			boolFromMap(cookies, "hasCsrf"),
			boolFromMap(cookies, "hasLgwCsrf"),
		)
	}
	if remediation, ok := payload["remediation"].([]string); ok {
		for _, item := range remediation {
			fmt.Fprintf(w, "remediation %s\n", item)
		}
	}
}

func okWord(ok bool) string {
	if ok {
		return "ok"
	}
	return "fail"
}

func boolFromMap(raw any, key string) bool {
	switch value := raw.(type) {
	case map[string]bool:
		return value[key]
	case map[string]any:
		result, _ := value[key].(bool)
		return result
	default:
		return false
	}
}

func mapFromAny(raw any) map[string]any {
	if values, ok := raw.(map[string]any); ok {
		return values
	}
	return map[string]any{}
}

func intFromMap(raw any, key string) int {
	values, ok := raw.(map[string]any)
	if !ok {
		return 0
	}
	switch value := values[key].(type) {
	case int:
		return value
	case int64:
		return int(value)
	case float64:
		return int(value)
	default:
		return 0
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

func resolveDocsDefaultBaseURL() (string, string, error) {
	if value := strings.TrimSpace(os.Getenv(docsDefaultBaseURLEnv)); value != "" {
		return value, "env:" + docsDefaultBaseURLEnv, nil
	}
	if value := strings.TrimSpace(os.Getenv(globalDefaultBaseURLEnv)); value != "" {
		return value, "env:" + globalDefaultBaseURLEnv, nil
	}
	configPath := docsConfigPath()
	content, err := os.ReadFile(configPath)
	if errors.Is(err, os.ErrNotExist) {
		return "", "", nil
	}
	if err != nil {
		return "", "", fmt.Errorf("read docs default base URL config: %w", err)
	}
	var config toolboxConfig
	if err := json.Unmarshal(content, &config); err != nil {
		return "", "", fmt.Errorf("parse docs default base URL config: %w", err)
	}
	if value := strings.TrimSpace(config.Docs.DefaultBaseURL); value != "" {
		return value, "config:docs.defaultBaseURL", nil
	}
	return "", "", nil
}

func docsDiagnostics() map[string]any {
	value, source, err := resolveDocsDefaultBaseURL()
	defaultBaseURL := map[string]any{
		"configured": false,
		"source":     "",
		"host":       "",
		"configPath": docsConfigPath(),
	}
	if err != nil {
		defaultBaseURL["error"] = fmt.Sprintf("%T: %v", err, err)
		return map[string]any{"defaultBaseURL": defaultBaseURL}
	}
	if value != "" {
		defaultBaseURL["configured"] = true
		defaultBaseURL["source"] = source
		defaultBaseURL["host"] = hostFromURL(value)
	}
	return map[string]any{"defaultBaseURL": defaultBaseURL}
}

func docsConfigPath() string {
	if xdg := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME")); xdg != "" {
		return filepath.Join(expandUser(xdg), "ixf-toolbox", "config.json")
	}
	return filepath.Join(homeDir(), ".config", "ixf-toolbox", "config.json")
}

func hostFromURL(value string) string {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil {
		return ""
	}
	return parsed.Host
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
