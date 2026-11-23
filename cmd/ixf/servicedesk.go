package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	ixfsd "github.com/serialq7ic4/ixf-toolbox/internal/servicedesk"
)

type serviceDeskEnvelope struct {
	Operation       string             `json:"operation"`
	ContractVersion int                `json:"contractVersion"`
	OK              bool               `json:"ok"`
	Result          any                `json:"result,omitempty"`
	Error           *ixfsd.DomainError `json:"error,omitempty"`
}

var serviceDeskClientLoader = loadServiceDeskClient
var serviceDeskClientForCredential = func(credential ixfsd.Credential) *ixfsd.Client {
	return ixfsd.NewClient(credential, ixfsd.ClientOptions{})
}
var serviceDeskRegistryLoader = func() ixfsd.RegistryLoader {
	return ixfsd.RegistryLoader{}
}

func runServiceDesk(args []string, stdout, stderr io.Writer) int {
	rows := [][2]string{
		{"doctor", "Inspect ServiceDesk credential and API readiness."},
		{"template", "Inspect a live ServiceDesk form contract."},
		{"profile", "Discover, validate, list, or show semantic profiles."},
		{"create", "Dry-run or apply a validated ticket creation."},
		{"list", "List the current user's recent or open tickets."},
		{"get", "Read one ticket by detail URL or explicit IDs."},
		{"withdraw", "Dry-run or apply withdrawal of the current user's ticket."},
	}
	if len(args) == 0 {
		fmt.Fprintln(stderr, "ERROR servicedesk requires a subcommand")
		printCommandHelp(stderr, "ixf servicedesk", rows)
		return 2
	}
	if isHelpArg(args[0]) {
		printCommandHelp(stdout, "ixf servicedesk", rows)
		return 0
	}
	switch args[0] {
	case "doctor":
		return runServiceDeskDoctor(args[1:], stdout, stderr)
	case "template":
		return runServiceDeskTemplate(args[1:], stdout, stderr)
	case "profile":
		return runServiceDeskProfile(args[1:], stdout, stderr)
	case "create":
		return runServiceDeskCreate(args[1:], stdout, stderr)
	case "list":
		return runServiceDeskList(args[1:], stdout, stderr)
	case "get":
		return runServiceDeskGet(args[1:], stdout, stderr)
	case "withdraw":
		return runServiceDeskWithdraw(args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "ERROR unsupported servicedesk subcommand: %s\n", args[0])
		return 2
	}
}

func serviceDeskCommonFlags(flags *flag.FlagSet) (*string, *bool) {
	return flags.String("api-key-file", "", ""), flags.Bool("json", false, "")
}

func loadServiceDeskClient(apiKeyFile string) (*ixfsd.Client, *ixfsd.DomainError) {
	credential, err := (ixfsd.CredentialLoader{}).Load(apiKeyFile)
	if err != nil {
		return nil, &ixfsd.DomainError{Code: "credential_unavailable", Message: "ServiceDesk API Key is not configured", Action: "set IXF_SERVICEDESK_API_KEY or create the 0600 credential file"}
	}
	return serviceDeskClientForCredential(credential), nil
}

func writeServiceDeskResult(stdout, stderr io.Writer, asJSON bool, operation string, result any, domainErr *ixfsd.DomainError) int {
	envelope := serviceDeskEnvelope{Operation: operation, ContractVersion: ixfsd.ContractVersion, OK: domainErr == nil, Result: result, Error: domainErr}
	if asJSON {
		writeJSON(stdout, envelope)
	} else if domainErr != nil {
		fmt.Fprintf(stderr, "ERROR %s: %s\n", domainErr.Code, domainErr.Message)
	} else {
		writePrettyJSON(stdout, result)
	}
	if domainErr != nil {
		return 1
	}
	return 0
}

func runServiceDeskDoctor(args []string, stdout, stderr io.Writer) int {
	if hasHelpArg(args) {
		printUsageHelp(stdout, "ixf servicedesk doctor [--business-id ID] [--api-key-file PATH] [--json]", nil)
		return 0
	}
	flags := flag.NewFlagSet("ixf servicedesk doctor", flag.ContinueOnError)
	flags.SetOutput(stderr)
	businessID := flags.Int("business-id", 0, "")
	apiKeyFile, asJSON := serviceDeskCommonFlags(flags)
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 {
		return 2
	}
	result := serviceDeskDiagnostics(*businessID, *apiKeyFile)
	if *asJSON {
		writeJSON(stdout, result)
	} else {
		writePrettyJSON(stdout, result)
	}
	return 0
}

func serviceDeskDiagnostics(explicitBusinessID int, apiKeyFile string) map[string]any {
	result := map[string]any{"configured": false, "apiReachable": false, "authenticated": false, "probeSkipped": true}
	credential, err := (ixfsd.CredentialLoader{}).Load(apiKeyFile)
	if err != nil {
		result["reason"] = "credential_unavailable"
		return result
	}
	result["configured"] = true
	result["credentialSource"] = credential.Source
	businessID, source, configured, err := (ixfsd.BusinessIDLoader{}).Load(explicitBusinessID)
	if err != nil {
		result["reason"] = "invalid_business_id"
		return result
	}
	if !configured {
		result["reason"] = "business_id_unconfigured"
		return result
	}
	result["businessId"] = businessID
	result["businessIdSource"] = source
	result["probeSkipped"] = false
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, probeErr := serviceDeskClientForCredential(credential).UserInfo(ctx, businessID)
	if probeErr != nil {
		classified := ixfsd.ClassifyError("doctor", probeErr)
		result["status"] = classified.Code
		return result
	}
	result["apiReachable"] = true
	result["authenticated"] = true
	result["status"] = "ready"
	return result
}

func serviceDeskRootDiagnostics() map[string]any { return serviceDeskDiagnostics(0, "") }

func runServiceDeskTemplate(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] != "inspect" {
		if len(args) > 0 && isHelpArg(args[0]) {
			printCommandHelp(stdout, "ixf servicedesk template", [][2]string{{"inspect", "Resolve a live form contract."}})
			return 0
		}
		fmt.Fprintln(stderr, "ERROR servicedesk template requires inspect")
		return 2
	}
	if hasHelpArg(args[1:]) {
		printUsageHelp(stdout, "ixf servicedesk template inspect (--url URL|--profile NAME) [--json]", nil)
		return 0
	}
	flags := flag.NewFlagSet("ixf servicedesk template inspect", flag.ContinueOnError)
	flags.SetOutput(stderr)
	urlValue := flags.String("url", "", "")
	profile := flags.String("profile", "", "")
	apiKeyFile, asJSON := serviceDeskCommonFlags(flags)
	if err := flags.Parse(args[1:]); err != nil || flags.NArg() != 0 {
		return 2
	}
	if (*urlValue == "") == (*profile == "") {
		fmt.Fprintln(stderr, "ERROR exactly one of --url or --profile is required")
		return 2
	}
	client, domainErr := serviceDeskClientLoader(*apiKeyFile)
	if domainErr != nil {
		return writeServiceDeskResult(stdout, stderr, *asJSON, "servicedesk_template_inspect", nil, domainErr)
	}
	request := ixfsd.InspectRequest{}
	if *urlValue != "" {
		target, err := ixfsd.ParseEditURL(*urlValue)
		if err != nil {
			return serviceDeskUsageError(stderr, err)
		}
		request.Target = target
	} else {
		overlay, status, err := serviceDeskRegistryLoader().Show(context.Background(), *profile)
		if err != nil {
			return writeServiceDeskResult(stdout, stderr, *asJSON, "servicedesk_template_inspect", nil, ixfsd.ClassifyError("registry", err))
		}
		request.Target = ixfsd.EditTarget{BusinessID: overlay.Target.BusinessID, ServiceID: overlay.Target.ServiceID}
		request.Profile = &ixfsd.ProfileContext{Overlay: overlay, Source: status.Source, Stale: status.Stale}
	}
	result, err := ixfsd.Inspect(context.Background(), client, request)
	if err != nil {
		return writeServiceDeskResult(stdout, stderr, *asJSON, "servicedesk_template_inspect", nil, ixfsd.ClassifyError("inspect", err))
	}
	return writeServiceDeskResult(stdout, stderr, *asJSON, "servicedesk_template_inspect", result, nil)
}

func runServiceDeskProfile(args []string, stdout, stderr io.Writer) int {
	rows := [][2]string{{"discover", "Generate a local overlay draft."}, {"validate", "Validate an overlay against live metadata."}, {"list", "List shared semantic profiles."}, {"show", "Show one shared semantic profile."}}
	if len(args) == 0 {
		printCommandHelp(stderr, "ixf servicedesk profile", rows)
		return 2
	}
	if isHelpArg(args[0]) {
		printCommandHelp(stdout, "ixf servicedesk profile", rows)
		return 0
	}
	switch args[0] {
	case "list":
		if hasHelpArg(args[1:]) {
			printUsageHelp(stdout, "ixf servicedesk profile list [--json]", nil)
			return 0
		}
		flags := flag.NewFlagSet("profile list", flag.ContinueOnError)
		flags.SetOutput(stderr)
		asJSON := flags.Bool("json", false, "")
		if err := flags.Parse(args[1:]); err != nil || flags.NArg() != 0 {
			return 2
		}
		values, status, err := serviceDeskRegistryLoader().List(context.Background())
		if err != nil {
			return writeServiceDeskResult(stdout, stderr, *asJSON, "servicedesk_profile_list", nil, ixfsd.ClassifyError("registry", err))
		}
		return writeServiceDeskResult(stdout, stderr, *asJSON, "servicedesk_profile_list", map[string]any{"profiles": values, "registry": status}, nil)
	case "show":
		if hasHelpArg(args[1:]) {
			printUsageHelp(stdout, "ixf servicedesk profile show NAME [--json]", nil)
			return 0
		}
		flags := flag.NewFlagSet("profile show", flag.ContinueOnError)
		flags.SetOutput(stderr)
		asJSON := flags.Bool("json", false, "")
		name := ""
		nameSet := false
		flagArgs := make([]string, 0, len(args)-1)
		for _, arg := range args[1:] {
			if strings.HasPrefix(arg, "-") {
				flagArgs = append(flagArgs, arg)
				continue
			}
			if nameSet {
				return serviceDeskUsageError(stderr, fmt.Errorf("profile show requires exactly one name"))
			}
			name = arg
			nameSet = true
		}
		if err := flags.Parse(flagArgs); err != nil || flags.NArg() != 0 {
			return 2
		}
		if !nameSet {
			return serviceDeskUsageError(stderr, fmt.Errorf("profile show requires exactly one name"))
		}
		if !ixfsd.IsValidProfileName(name) {
			return serviceDeskUsageError(stderr, fmt.Errorf("profile name must be a lowercase hyphenated slug of at most 64 characters"))
		}
		value, status, err := serviceDeskRegistryLoader().Show(context.Background(), name)
		if err != nil {
			return writeServiceDeskResult(stdout, stderr, *asJSON, "servicedesk_profile_show", nil, ixfsd.ClassifyError("registry", err))
		}
		return writeServiceDeskResult(stdout, stderr, *asJSON, "servicedesk_profile_show", map[string]any{"profile": value, "registry": status}, nil)
	case "discover":
		return runServiceDeskProfileDiscover(args[1:], stdout, stderr)
	case "validate":
		return runServiceDeskProfileValidate(args[1:], stdout, stderr)
	default:
		fmt.Fprintln(stderr, "ERROR unsupported servicedesk profile subcommand")
		return 2
	}
}

func runServiceDeskProfileDiscover(args []string, stdout, stderr io.Writer) int {
	if hasHelpArg(args) {
		printUsageHelp(stdout, "ixf servicedesk profile discover --url URL --name NAME --out FILE [--json]", nil)
		return 0
	}
	flags := flag.NewFlagSet("profile discover", flag.ContinueOnError)
	flags.SetOutput(stderr)
	urlValue := flags.String("url", "", "")
	name := flags.String("name", "", "")
	out := flags.String("out", "", "")
	apiKeyFile, asJSON := serviceDeskCommonFlags(flags)
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 {
		return 2
	}
	if *urlValue == "" || *name == "" || *out == "" {
		fmt.Fprintln(stderr, "ERROR --url, --name, and --out are required")
		return 2
	}
	target, err := ixfsd.ParseEditURL(*urlValue)
	if err != nil {
		return serviceDeskUsageError(stderr, err)
	}
	client, de := serviceDeskClientLoader(*apiKeyFile)
	if de != nil {
		return writeServiceDeskResult(stdout, stderr, *asJSON, "servicedesk_profile_discover", nil, de)
	}
	template, err := ixfsd.LoadTemplate(context.Background(), client, target)
	if err != nil {
		return writeServiceDeskResult(stdout, stderr, *asJSON, "servicedesk_profile_discover", nil, ixfsd.ClassifyError("inspect", err))
	}
	overlay := ixfsd.DiscoverOverlay(template, *name)
	encoded, err := json.MarshalIndent(overlay, "", "  ")
	if err != nil {
		return 1
	}
	encoded = append(encoded, '\n')
	if err := os.WriteFile(*out, encoded, 0600); err != nil {
		return writeServiceDeskResult(stdout, stderr, *asJSON, "servicedesk_profile_discover", nil, &ixfsd.DomainError{Code: "output_error", Message: "could not write profile draft", Action: "choose a writable output path"})
	}
	return writeServiceDeskResult(stdout, stderr, *asJSON, "servicedesk_profile_discover", map[string]any{"path": *out, "profile": overlay}, nil)
}

func runServiceDeskProfileValidate(args []string, stdout, stderr io.Writer) int {
	if hasHelpArg(args) {
		printUsageHelp(stdout, "ixf servicedesk profile validate --input FILE [--json]", nil)
		return 0
	}
	flags := flag.NewFlagSet("profile validate", flag.ContinueOnError)
	flags.SetOutput(stderr)
	input := flags.String("input", "", "")
	apiKeyFile, asJSON := serviceDeskCommonFlags(flags)
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 {
		return 2
	}
	if *input == "" {
		return serviceDeskUsageError(stderr, fmt.Errorf("--input is required"))
	}
	raw, err := os.ReadFile(*input)
	if err != nil {
		return serviceDeskUsageError(stderr, err)
	}
	overlay, err := ixfsd.DecodeOverlay(raw)
	if err != nil {
		return writeServiceDeskResult(stdout, stderr, *asJSON, "servicedesk_profile_validate", nil, ixfsd.ClassifyError("validate", err))
	}
	client, de := serviceDeskClientLoader(*apiKeyFile)
	if de != nil {
		return writeServiceDeskResult(stdout, stderr, *asJSON, "servicedesk_profile_validate", nil, de)
	}
	template, err := ixfsd.LoadTemplate(context.Background(), client, ixfsd.EditTarget{BusinessID: overlay.Target.BusinessID, ServiceID: overlay.Target.ServiceID})
	if err != nil {
		return writeServiceDeskResult(stdout, stderr, *asJSON, "servicedesk_profile_validate", nil, ixfsd.ClassifyError("validate", err))
	}
	resolved, err := ixfsd.ValidateOverlay(template, overlay)
	if err != nil {
		return writeServiceDeskResult(stdout, stderr, *asJSON, "servicedesk_profile_validate", nil, ixfsd.ClassifyError("validate", err))
	}
	return writeServiceDeskResult(stdout, stderr, *asJSON, "servicedesk_profile_validate", resolved, nil)
}

func runServiceDeskCreate(args []string, stdout, stderr io.Writer) int {
	if hasHelpArg(args) {
		printUsageHelp(stdout, "ixf servicedesk create (--url URL|--profile NAME) --input FILE [--dry-run|--apply] [--json]", nil)
		return 0
	}
	flags := flag.NewFlagSet("servicedesk create", flag.ContinueOnError)
	flags.SetOutput(stderr)
	urlValue := flags.String("url", "", "")
	profile := flags.String("profile", "", "")
	input := flags.String("input", "", "")
	apply := flags.Bool("apply", false, "")
	dryRun := flags.Bool("dry-run", false, "")
	apiKeyFile, asJSON := serviceDeskCommonFlags(flags)
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 {
		return 2
	}
	if (*urlValue == "") == (*profile == "") || *input == "" || *apply && *dryRun {
		return serviceDeskUsageError(stderr, fmt.Errorf("exactly one target, --input, and only one mode are required"))
	}
	raw, err := os.ReadFile(*input)
	if err != nil {
		return serviceDeskUsageError(stderr, err)
	}
	ticketInput, err := ixfsd.DecodeTicketInput(raw)
	if err != nil {
		return writeServiceDeskResult(stdout, stderr, *asJSON, "servicedesk_create", nil, ixfsd.ClassifyError("create", err))
	}
	request := ixfsd.CreateRequest{Profile: *profile, Input: ticketInput, Apply: *apply}
	if *urlValue != "" {
		target, err := ixfsd.ParseEditURL(*urlValue)
		if err != nil {
			return serviceDeskUsageError(stderr, err)
		}
		request.Target = &target
	}
	client, de := serviceDeskClientLoader(*apiKeyFile)
	if de != nil {
		return writeServiceDeskResult(stdout, stderr, *asJSON, "servicedesk_create", nil, de)
	}
	result, de := ixfsd.Create(context.Background(), ixfsd.Dependencies{Client: client, Registry: serviceDeskRegistryLoader()}, request)
	return writeServiceDeskResult(stdout, stderr, *asJSON, "servicedesk_create", result, de)
}

func runServiceDeskList(args []string, stdout, stderr io.Writer) int {
	if hasHelpArg(args) {
		printUsageHelp(stdout, "ixf servicedesk list --mine --business-id ID [--view recent|open] [--limit N] [--json]", nil)
		return 0
	}
	flags := flag.NewFlagSet("servicedesk list", flag.ContinueOnError)
	flags.SetOutput(stderr)
	mine := flags.Bool("mine", false, "")
	businessID := flags.Int("business-id", 0, "")
	view := flags.String("view", "recent", "")
	limit := flags.Int("limit", 20, "")
	cursor := flags.String("cursor", "", "")
	apiKeyFile, asJSON := serviceDeskCommonFlags(flags)
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 {
		return 2
	}
	if !*mine {
		return serviceDeskUsageError(stderr, fmt.Errorf("--mine is required"))
	}
	if *businessID <= 0 {
		return serviceDeskUsageError(stderr, fmt.Errorf("--business-id must be positive"))
	}
	if *limit <= 0 {
		return serviceDeskUsageError(stderr, fmt.Errorf("--limit must be positive"))
	}
	viewName := strings.ToLower(strings.TrimSpace(*view))
	if viewName != "recent" && viewName != "open" {
		return serviceDeskUsageError(stderr, fmt.Errorf("--view must be recent or open"))
	}
	client, de := serviceDeskClientLoader(*apiKeyFile)
	if de != nil {
		return writeServiceDeskResult(stdout, stderr, *asJSON, "servicedesk_list", nil, de)
	}
	result, de := ixfsd.ListTickets(context.Background(), client, ixfsd.ListRequest{BusinessID: *businessID, Mine: *mine, View: viewName, Limit: *limit, Cursor: *cursor})
	return writeServiceDeskResult(stdout, stderr, *asJSON, "servicedesk_list", result, de)
}

func parseServiceDeskDetailTarget(urlValue string, businessID int, ticketID string) (ixfsd.DetailTarget, error) {
	if urlValue != "" {
		if businessID != 0 || ticketID != "" {
			return ixfsd.DetailTarget{}, fmt.Errorf("--url and explicit IDs are mutually exclusive")
		}
		return ixfsd.ParseDetailURL(urlValue)
	}
	if businessID <= 0 || ticketID == "" {
		return ixfsd.DetailTarget{}, fmt.Errorf("--business-id and --ticket-id are required")
	}
	parsedID, err := strconv.Atoi(ticketID)
	if err != nil || parsedID <= 0 {
		return ixfsd.DetailTarget{}, fmt.Errorf("--ticket-id must be positive")
	}
	return ixfsd.DetailTarget{BusinessID: businessID, TicketID: ticketID}, nil
}

func runServiceDeskGet(args []string, stdout, stderr io.Writer) int {
	if hasHelpArg(args) {
		printUsageHelp(stdout, "ixf servicedesk get (--url URL|--business-id ID --ticket-id ID) [--json]", nil)
		return 0
	}
	flags := flag.NewFlagSet("servicedesk get", flag.ContinueOnError)
	flags.SetOutput(stderr)
	urlValue := flags.String("url", "", "")
	businessID := flags.Int("business-id", 0, "")
	ticketID := flags.String("ticket-id", "", "")
	apiKeyFile, asJSON := serviceDeskCommonFlags(flags)
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 {
		return 2
	}
	target, err := parseServiceDeskDetailTarget(*urlValue, *businessID, *ticketID)
	if err != nil {
		return serviceDeskUsageError(stderr, err)
	}
	client, de := serviceDeskClientLoader(*apiKeyFile)
	if de != nil {
		return writeServiceDeskResult(stdout, stderr, *asJSON, "servicedesk_get", nil, de)
	}
	result, de := ixfsd.GetTicket(context.Background(), client, target)
	return writeServiceDeskResult(stdout, stderr, *asJSON, "servicedesk_get", result, de)
}

func runServiceDeskWithdraw(args []string, stdout, stderr io.Writer) int {
	if hasHelpArg(args) {
		printUsageHelp(stdout, "ixf servicedesk withdraw (--url URL|--business-id ID --ticket-id ID) --reason TEXT [--dry-run|--apply] [--json]", nil)
		return 0
	}
	flags := flag.NewFlagSet("servicedesk withdraw", flag.ContinueOnError)
	flags.SetOutput(stderr)
	urlValue := flags.String("url", "", "")
	businessID := flags.Int("business-id", 0, "")
	ticketID := flags.String("ticket-id", "", "")
	reason := flags.String("reason", "", "")
	apply := flags.Bool("apply", false, "")
	dryRun := flags.Bool("dry-run", false, "")
	apiKeyFile, asJSON := serviceDeskCommonFlags(flags)
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 {
		return 2
	}
	if *apply && *dryRun {
		return serviceDeskUsageError(stderr, fmt.Errorf("--dry-run and --apply are mutually exclusive"))
	}
	if strings.TrimSpace(*reason) == "" {
		return serviceDeskUsageError(stderr, fmt.Errorf("--reason is required"))
	}
	target, err := parseServiceDeskDetailTarget(*urlValue, *businessID, *ticketID)
	if err != nil {
		return serviceDeskUsageError(stderr, err)
	}
	client, de := serviceDeskClientLoader(*apiKeyFile)
	if de != nil {
		return writeServiceDeskResult(stdout, stderr, *asJSON, "servicedesk_withdraw", nil, de)
	}
	result, de := ixfsd.WithdrawTicket(context.Background(), client, ixfsd.WithdrawRequest{Target: target, Reason: *reason, Apply: *apply})
	return writeServiceDeskResult(stdout, stderr, *asJSON, "servicedesk_withdraw", result, de)
}

func serviceDeskUsageError(stderr io.Writer, err error) int {
	fmt.Fprintf(stderr, "ERROR %s\n", err)
	return 2
}
