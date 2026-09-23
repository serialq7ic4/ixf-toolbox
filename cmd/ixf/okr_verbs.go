package main

import (
	"fmt"
	"io"
	"strconv"

	ixfokr "github.com/serialq7ic4/ixf-toolbox/internal/okr"
)

// okrVerbArgs is the flag set shared by every OKR write verb. Parsing it in one
// place keeps the verbs differing only in intent, not in argument handling.
type okrVerbArgs struct {
	url         string
	cookies     string
	csrfURL     string
	objective   int
	expectTitle string
	title       string
	krs         []string
	confirm     int
	confirmSet  bool
	apply       bool
	dryRun      bool
}

func parseOKRVerbArgs(command string, args []string, stderr io.Writer) (okrVerbArgs, bool) {
	parsed := okrVerbArgs{cookies: defaultCookies, csrfURL: ixfokr.DefaultCSRFURL}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		value := func() (string, bool) {
			i++
			if i >= len(args) {
				fmt.Fprintf(stderr, "ERROR %s requires a value\n", arg)
				return "", false
			}
			return args[i], true
		}
		switch arg {
		case "--url":
			got, ok := value()
			if !ok {
				return parsed, false
			}
			parsed.url = got
		case "--cookies":
			got, ok := value()
			if !ok {
				return parsed, false
			}
			parsed.cookies = got
		case "--csrf-url":
			got, ok := value()
			if !ok {
				return parsed, false
			}
			parsed.csrfURL = got
		case "--objective":
			got, ok := value()
			if !ok {
				return parsed, false
			}
			number, err := strconv.Atoi(got)
			if err != nil {
				fmt.Fprintf(stderr, "ERROR --objective must be a 1-based index, got %q\n", got)
				return parsed, false
			}
			parsed.objective = number
		case "--expect-title":
			got, ok := value()
			if !ok {
				return parsed, false
			}
			parsed.expectTitle = got
		case "--title":
			got, ok := value()
			if !ok {
				return parsed, false
			}
			parsed.title = got
		case "--kr":
			got, ok := value()
			if !ok {
				return parsed, false
			}
			parsed.krs = append(parsed.krs, got)
		case "--confirm-kr-deletes":
			got, ok := value()
			if !ok {
				return parsed, false
			}
			number, err := strconv.Atoi(got)
			if err != nil {
				fmt.Fprintf(stderr, "ERROR --confirm-kr-deletes must be a number, got %q\n", got)
				return parsed, false
			}
			parsed.confirm = number
			parsed.confirmSet = true
		case "--apply":
			parsed.apply = true
		case "--dry-run":
			parsed.dryRun = true
		default:
			fmt.Fprintf(stderr, "ERROR unsupported %s flag: %s\n", command, arg)
			return parsed, false
		}
	}
	if parsed.url == "" {
		fmt.Fprintf(stderr, "ERROR %s requires --url\n", command)
		return parsed, false
	}
	if parsed.apply && parsed.dryRun {
		fmt.Fprintln(stderr, "ERROR --dry-run and --apply are mutually exclusive")
		return parsed, false
	}
	return parsed, true
}

func (a okrVerbArgs) config() ixfokr.VerbConfig {
	return ixfokr.VerbConfig{
		URL:              a.url,
		CookiesPath:      a.cookies,
		CSRFURL:          a.csrfURL,
		Objective:        a.objective,
		ExpectTitle:      a.expectTitle,
		Texts:            a.krs,
		Title:            a.title,
		ConfirmKRDeletes: a.confirm,
		Apply:            a.apply,
	}
}

func runOKRVerb(
	command string,
	args []string,
	stdout io.Writer,
	stderr io.Writer,
	help func(io.Writer),
	run func(ixfokr.VerbConfig) (map[string]any, error),
) int {
	if hasHelpArg(args) {
		help(stdout)
		return 0
	}
	parsed, ok := parseOKRVerbArgs(command, args, stderr)
	if !ok {
		return 2
	}
	payload, err := run(parsed.config())
	if err != nil {
		fmt.Fprintf(stderr, "ERROR %s\n", err)
		return 1
	}
	writeJSON(stdout, payload)
	if okValue, _ := payload["ok"].(bool); !okValue {
		return 1
	}
	return 0
}

func runOKRObjective(args []string, stdout io.Writer, stderr io.Writer) int {
	rows := [][2]string{
		{"create", "Append a new Objective with its Key Results."},
		{"retitle", "Rewrite one Objective's title, leaving its Key Results alone."},
		{"delete", "Delete one whole Objective and its Key Results. Destructive."},
	}
	if len(args) == 0 {
		fmt.Fprintln(stderr, "ERROR okr objective requires a subcommand.")
		printCommandHelp(stderr, "ixf okr objective", rows)
		return 2
	}
	if isHelpArg(args[0]) {
		printCommandHelp(stdout, "ixf okr objective", rows)
		return 0
	}
	switch args[0] {
	case "create":
		return runOKRVerb("okr objective create", args[1:], stdout, stderr, printOKRObjectiveCreateHelp, ixfokr.CreateObjective)
	case "retitle":
		return runOKRVerb("okr objective retitle", args[1:], stdout, stderr, printOKRObjectiveRetitleHelp, ixfokr.RetitleObjective)
	case "delete":
		return runOKRVerb("okr objective delete", args[1:], stdout, stderr, printOKRObjectiveDeleteHelp, ixfokr.DeleteObjective)
	default:
		fmt.Fprintf(stderr, "ERROR unsupported okr objective subcommand: %s\n", args[0])
		printCommandHelp(stderr, "ixf okr objective", rows)
		return 2
	}
}

func runOKRKR(args []string, stdout io.Writer, stderr io.Writer) int {
	rows := [][2]string{
		{"add", "Append Key Results to an Objective, keeping the existing ones."},
		{"replace", "Replace an Objective's entire Key Result set. Destructive."},
		{"delete", "Remove named Key Results from an Objective. Destructive."},
	}
	if len(args) == 0 {
		fmt.Fprintln(stderr, "ERROR okr kr requires a subcommand.")
		printCommandHelp(stderr, "ixf okr kr", rows)
		return 2
	}
	if isHelpArg(args[0]) {
		printCommandHelp(stdout, "ixf okr kr", rows)
		return 0
	}
	switch args[0] {
	case "add":
		return runOKRVerb("okr kr add", args[1:], stdout, stderr, printOKRKRAddHelp, ixfokr.AddKRs)
	case "replace":
		return runOKRVerb("okr kr replace", args[1:], stdout, stderr, printOKRKRReplaceHelp, ixfokr.ReplaceKRs)
	case "delete":
		return runOKRVerb("okr kr delete", args[1:], stdout, stderr, printOKRKRDeleteHelp, ixfokr.DeleteKRs)
	default:
		fmt.Fprintf(stderr, "ERROR unsupported okr kr subcommand: %s\n", args[0])
		printCommandHelp(stderr, "ixf okr kr", rows)
		return 2
	}
}

var okrCommonOptions = [][2]string{
	{"--url URL", "OKR page URL."},
	{"--cookies PATH", "Read exported desktop session cookies from PATH."},
	{"--csrf-url URL", "Override the LGW CSRF token endpoint."},
	{"--dry-run", "Report the plan without writing. Default when --apply is absent."},
	{"--apply", "Perform the write."},
}

func okrHelpOptions(extra ...[2]string) [][2]string {
	options := append([][2]string{}, extra...)
	return append(options, okrCommonOptions...)
}

func printOKRTargetNote(w io.Writer) {
	fmt.Fprintln(w)
	fmt.Fprintln(w, "--objective is a 1-based index and --expect-title is the title you believe sits")
	fmt.Fprintln(w, "there. Indexes shift as Objectives are added, so the title is compared before")
	fmt.Fprintln(w, "anything is written and the command refuses on a mismatch. Copy both from")
	fmt.Fprintln(w, "`ixf okr inspect <okr-url>`.")
}

func printOKRObjectiveCreateHelp(w io.Writer) {
	printUsageHelp(w, "ixf okr objective create --url URL --title TEXT --kr TEXT [--kr TEXT ...] [--dry-run|--apply]",
		okrHelpOptions(
			[2]string{"--title TEXT", "Title of the new Objective."},
			[2]string{"--kr TEXT", "A Key Result. Repeat for each one; at least one is required."},
		))
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Appends a new Objective. It refuses an --objective that is not the append")
	fmt.Fprintln(w, "position, so it can never replace an existing Objective by accident.")
}

func printOKRObjectiveRetitleHelp(w io.Writer) {
	printUsageHelp(w, "ixf okr objective retitle --url URL --objective N --expect-title OLD --title NEW [--dry-run|--apply]",
		okrHelpOptions(
			[2]string{"--objective N", "1-based index of the Objective to retitle."},
			[2]string{"--expect-title TEXT", "The title you expect at that index."},
			[2]string{"--title TEXT", "The new title."},
		))
	printOKRTargetNote(w)
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Key Results are not touched, and the post-write check asserts their count did")
	fmt.Fprintln(w, "not change.")
}

func printOKRObjectiveDeleteHelp(w io.Writer) {
	printUsageHelp(w, "ixf okr objective delete --url URL --objective N --expect-title TEXT --confirm-kr-deletes K [--dry-run|--apply]",
		okrHelpOptions(
			[2]string{"--objective N", "1-based index of the Objective to delete."},
			[2]string{"--expect-title TEXT", "The title you expect at that index."},
			[2]string{"--confirm-kr-deletes K", "The number of Key Results this will destroy. Must match exactly."},
		))
	printOKRTargetNote(w)
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Destructive. Deletes the Objective and every Key Result under it. One Objective")
	fmt.Fprintln(w, "per invocation. Run --dry-run first and copy diff.krsToDelete into")
	fmt.Fprintln(w, "--confirm-kr-deletes; the value cannot be guessed, which is the point.")
}

func printOKRKRAddHelp(w io.Writer) {
	printUsageHelp(w, "ixf okr kr add --url URL --objective N --expect-title TEXT --kr TEXT [--kr TEXT ...] [--dry-run|--apply]",
		okrHelpOptions(
			[2]string{"--objective N", "1-based index of the target Objective."},
			[2]string{"--expect-title TEXT", "The title you expect at that index."},
			[2]string{"--kr TEXT", "A Key Result to append. Repeat for each one."},
		))
	printOKRTargetNote(w)
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Existing Key Results are kept and keep their order. A Key Result already present")
	fmt.Fprintln(w, "is reported in diff.alreadyPresent and skipped rather than duplicated.")
}

func printOKRKRReplaceHelp(w io.Writer) {
	printUsageHelp(w, "ixf okr kr replace --url URL --objective N --expect-title TEXT --kr TEXT [--kr ...] --confirm-kr-deletes K [--dry-run|--apply]",
		okrHelpOptions(
			[2]string{"--objective N", "1-based index of the target Objective."},
			[2]string{"--expect-title TEXT", "The title you expect at that index."},
			[2]string{"--kr TEXT", "A Key Result in the new set. Repeat for each one."},
			[2]string{"--confirm-kr-deletes K", "The number of existing Key Results this will destroy. Must match exactly."},
		))
	printOKRTargetNote(w)
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Destructive. The Key Results given become the Objective's entire set and the")
	fmt.Fprintln(w, "previous ones are removed, in a single write. To keep the existing ones use")
	fmt.Fprintln(w, "`ixf okr kr add`; to remove without replacing use `ixf okr kr delete`.")
}

func printOKRKRDeleteHelp(w io.Writer) {
	printUsageHelp(w, "ixf okr kr delete --url URL --objective N --expect-title TEXT --kr TEXT [--kr ...] --confirm-kr-deletes K [--dry-run|--apply]",
		okrHelpOptions(
			[2]string{"--objective N", "1-based index of the target Objective."},
			[2]string{"--expect-title TEXT", "The title you expect at that index."},
			[2]string{"--kr TEXT", "The exact text of a Key Result to remove. Repeat for each one."},
			[2]string{"--confirm-kr-deletes K", "The number of Key Results this will destroy. Must match exactly."},
		))
	printOKRTargetNote(w)
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Destructive, and the only way to leave an Objective with no Key Results. A")
	fmt.Fprintln(w, "--kr that does not match an existing Key Result aborts the whole command rather")
	fmt.Fprintln(w, "than deleting the ones that did match, because a mismatch means the page has")
	fmt.Fprintln(w, "changed since it was inspected.")
}
