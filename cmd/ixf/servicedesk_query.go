package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"

	ixfsd "github.com/serialq7ic4/ixf-toolbox/internal/servicedesk"
)

var serviceDeskQueryRunner = ixfsd.QueryTickets

func runServiceDeskList(args []string, stdout, stderr io.Writer) int {
	return runServiceDeskQueryCommand("list", args, stdout, stderr)
}

func runServiceDeskSearch(args []string, stdout, stderr io.Writer) int {
	return runServiceDeskQueryCommand("search", args, stdout, stderr)
}

func runServiceDeskQueryCommand(command string, args []string, stdout, stderr io.Writer) int {
	if hasHelpArg(args) {
		if command == "search" {
			printUsageHelp(stdout, "ixf servicedesk search --mine --query TEXT [--role any|todo|submitted] [--state all|open] [--business-id ID] [--limit N] [--json]", nil)
		} else {
			printUsageHelp(stdout, "ixf servicedesk list --mine --role todo|submitted [--state all|open] [--business-id ID] [--limit N] [--json]", nil)
		}
		return 0
	}
	flags := flag.NewFlagSet("servicedesk "+command, flag.ContinueOnError)
	flags.SetOutput(stderr)
	mine := flags.Bool("mine", false, "")
	defaultRole := ""
	if command == "search" {
		defaultRole = string(ixfsd.QueryRoleAny)
	}
	roleValue := flags.String("role", defaultRole, "")
	stateValue := flags.String("state", string(ixfsd.QueryStateAll), "")
	queryValue := flags.String("query", "", "")
	businessID := flags.Int("business-id", 0, "")
	limit := flags.Int("limit", 20, "")
	apiKeyFile, asJSON := serviceDeskCommonFlags(flags)
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 {
		return 2
	}
	stateExplicit := false
	flags.Visit(func(current *flag.Flag) {
		if current.Name == "state" {
			stateExplicit = true
		}
	})
	if !*mine {
		return serviceDeskUsageError(stderr, fmt.Errorf("--mine is required"))
	}
	role := ixfsd.QueryRole(strings.ToLower(strings.TrimSpace(*roleValue)))
	if command == "list" {
		if role != ixfsd.QueryRoleSubmitted && role != ixfsd.QueryRoleTodo {
			return serviceDeskUsageError(stderr, fmt.Errorf("--role must be submitted or todo"))
		}
	} else if role != ixfsd.QueryRoleSubmitted && role != ixfsd.QueryRoleTodo && role != ixfsd.QueryRoleAny {
		return serviceDeskUsageError(stderr, fmt.Errorf("--role must be any, submitted, or todo"))
	}
	state := ixfsd.QueryStateFilter(strings.ToLower(strings.TrimSpace(*stateValue)))
	if state != ixfsd.QueryStateAll && state != ixfsd.QueryStateOpen {
		return serviceDeskUsageError(stderr, fmt.Errorf("--state must be all or open"))
	}
	if role == ixfsd.QueryRoleTodo && stateExplicit {
		return serviceDeskUsageError(stderr, fmt.Errorf("--state is not supported with --role todo"))
	}
	query := strings.TrimSpace(*queryValue)
	if command == "search" && query == "" {
		return serviceDeskUsageError(stderr, fmt.Errorf("--query must be non-empty"))
	}
	if command == "list" && query != "" {
		return serviceDeskUsageError(stderr, fmt.Errorf("--query is only supported by servicedesk search"))
	}
	if *businessID < 0 {
		return serviceDeskUsageError(stderr, fmt.Errorf("--business-id must be positive"))
	}
	if *limit <= 0 {
		return serviceDeskUsageError(stderr, fmt.Errorf("--limit must be positive"))
	}
	operation := "servicedesk_" + command
	resolvedBusinessID, _, _, err := (ixfsd.BusinessIDLoader{}).Load(*businessID)
	if err != nil {
		domainErr := &ixfsd.DomainError{Code: "invalid_request", Message: "ServiceDesk business ID override is invalid", Action: "remove or correct --business-id, IXF_SERVICEDESK_BUSINESS_ID, or the private default business ID file"}
		return writeServiceDeskResult(stdout, stderr, *asJSON, operation, nil, domainErr)
	}
	client, domainErr := serviceDeskClientLoader(*apiKeyFile)
	if domainErr != nil {
		return writeServiceDeskResult(stdout, stderr, *asJSON, operation, nil, domainErr)
	}
	result, domainErr := serviceDeskQueryRunner(context.Background(), client, ixfsd.QueryRequest{
		BootstrapBusinessID: resolvedBusinessID,
		Mine:                *mine,
		Role:                role,
		State:               state,
		Query:               query,
		Limit:               *limit,
	})
	return writeServiceDeskResult(stdout, stderr, *asJSON, operation, result, domainErr)
}
