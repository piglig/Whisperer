package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/zhuzhenwu/whisperer/internal/authoring"
	_ "github.com/zhuzhenwu/whisperer/internal/fogharbor"
	"github.com/zhuzhenwu/whisperer/internal/scenario"
)

func runScenarioSubcommand(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: whisperer scenario lint|playtest|verify [options]")
		os.Exit(2)
	}
	switch args[0] {
	case "lint":
		runScenarioLint(args[1:])
	case "playtest":
		runScenarioPlaytest(args[1:])
	case "verify":
		runScenarioVerify(args[1:])
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

func runScenarioPlaytest(args []string) {
	fs := flag.NewFlagSet("scenario playtest", flag.ExitOnError)
	scenarioID := fs.String("scenario", "fog_harbor", "bundled scenario id")
	pathText := fs.String("path", "mainline", "playtest path, interpreted by the scenario adapter")
	variantID := fs.String("variant", "", "variant id; empty uses first bundled variant")
	all := fs.Bool("all", false, "run mainline across every variant plus flee and dismissed")
	jsonOut := fs.Bool("json", false, "print machine-readable JSON")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "scenario playtest: %v\n", err)
		os.Exit(2)
	}
	adapter, err := authoring.Adapter(*scenarioID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "scenario playtest: %v\n", err)
		os.Exit(2)
	}

	ctx := context.Background()
	var reports []authoring.PlaytestReport
	if *all {
		reports, err = adapter.RunAllPlaytests(ctx)
	} else {
		path, pathErr := adapter.ValidatePath(*pathText)
		if pathErr != nil {
			fmt.Fprintf(os.Stderr, "scenario playtest: %v\n", pathErr)
			os.Exit(2)
		}
		var report authoring.PlaytestReport
		report, err = adapter.RunPlaytest(ctx, authoring.PlaytestOptions{
			VariantID: *variantID,
			Path:      path,
		})
		reports = []authoring.PlaytestReport{report}
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "scenario playtest: %v\n", err)
		os.Exit(1)
	}
	if *jsonOut {
		data, err := authoring.MarshalPlaytestJSON(reports, *all)
		if err != nil {
			fmt.Fprintf(os.Stderr, "scenario playtest: encode json: %v\n", err)
			os.Exit(1)
		}
		fmt.Fprintln(os.Stdout, string(data))
	} else {
		fmt.Fprintln(os.Stdout, authoring.FormatPlaytestReports(adapter.DisplayName(), reports))
	}
	if authoring.HasFailures(reports) {
		os.Exit(1)
	}
}

func runScenarioVerify(args []string) {
	fs := flag.NewFlagSet("scenario verify", flag.ExitOnError)
	scenarioID := fs.String("scenario", "fog_harbor", "bundled scenario id")
	jsonOut := fs.Bool("json", false, "print machine-readable JSON")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "scenario verify: %v\n", err)
		os.Exit(2)
	}
	adapter, err := authoring.Adapter(*scenarioID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "scenario verify: %v\n", err)
		os.Exit(2)
	}

	report, err := adapter.Verify(context.Background())
	if err != nil {
		fmt.Fprintf(os.Stderr, "scenario verify: %v\n", err)
		os.Exit(1)
	}
	if *jsonOut {
		data, err := authoring.MarshalVerifyJSON(report)
		if err != nil {
			fmt.Fprintf(os.Stderr, "scenario verify: encode json: %v\n", err)
			os.Exit(1)
		}
		fmt.Fprintln(os.Stdout, string(data))
	} else {
		fmt.Fprintln(os.Stdout, authoring.FormatVerifyReport(adapter.DisplayName(), report))
	}
	if !report.Passed {
		os.Exit(1)
	}
}
