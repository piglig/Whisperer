package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/zhuzhenwu/whisperer/internal/scenario"
)

func runScenarioSubcommand(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: whisperer scenario lint [--scenario fog_harbor | --all] [--json]")
		os.Exit(2)
	}
	switch args[0] {
	case "lint":
		runScenarioLint(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown scenario subcommand %q\n", args[0])
		os.Exit(2)
	}
}

func runScenarioLint(args []string) {
	fs := flag.NewFlagSet("scenario lint", flag.ExitOnError)
	scenarioID := fs.String("scenario", "fog_harbor", "bundled scenario id")
	all := fs.Bool("all", false, "lint every bundled scenario")
	jsonOut := fs.Bool("json", false, "print machine-readable JSON")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "scenario lint: %v\n", err)
		os.Exit(2)
	}
	reports, err := lintBundledScenarios(*scenarioID, *all)
	if err != nil {
		fmt.Fprintf(os.Stderr, "scenario lint: %v\n", err)
		os.Exit(1)
	}
	if *jsonOut {
		writeScenarioLintJSON(reports, *all)
	} else {
		if *all {
			fmt.Fprintln(os.Stdout, scenario.FormatLintReports(reports))
		} else {
			fmt.Fprintln(os.Stdout, scenario.FormatLintReport(reports[0]))
		}
	}
	if scenario.HasLintErrors(reports) {
		os.Exit(1)
	}
}

func lintBundledScenarios(scenarioID string, all bool) ([]scenario.LintReport, error) {
	if all {
		infos, err := scenario.ListBundled()
		if err != nil {
			return nil, err
		}
		reports := make([]scenario.LintReport, 0, len(infos))
		for _, info := range infos {
			scn, err := scenario.LoadBundled(info.ID)
			if err != nil {
				return nil, err
			}
			reports = append(reports, scenario.Lint(scn))
		}
		return reports, nil
	}
	scn, err := scenario.LoadBundled(scenarioID)
	if err != nil {
		return nil, err
	}
	return []scenario.LintReport{scenario.Lint(scn)}, nil
}

func writeScenarioLintJSON(reports []scenario.LintReport, forceArray bool) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	var err error
	if len(reports) == 1 && !forceArray {
		err = enc.Encode(reports[0])
	} else {
		err = enc.Encode(reports)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "scenario lint: encode json: %v\n", err)
		os.Exit(1)
	}
}
