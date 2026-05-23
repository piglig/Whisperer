package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/zhuzhenwu/whisperer/internal/fogharbor"
)

func runFogHarborSubcommand(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: whisperer fog-harbor playtest|verify [options]")
		os.Exit(2)
	}
	switch args[0] {
	case "playtest":
		runFogHarborPlaytest(args[1:])
	case "verify":
		runFogHarborVerify(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown fog-harbor subcommand %q\n", args[0])
		os.Exit(2)
	}
}

func runFogHarborPlaytest(args []string) {
	fs := flag.NewFlagSet("fog-harbor playtest", flag.ExitOnError)
	pathText := fs.String("path", string(fogharbor.PathMainline), "playtest path: mainline | flee | dismissed")
	variantID := fs.String("variant", "", "variant id; empty uses first bundled variant")
	all := fs.Bool("all", false, "run mainline across every variant plus flee and dismissed")
	jsonOut := fs.Bool("json", false, "print machine-readable JSON")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "fog-harbor playtest: %v\n", err)
		os.Exit(2)
	}

	ctx := context.Background()
	var reports []fogharbor.PlaytestReport
	var err error
	if *all {
		reports, err = fogharbor.RunAllPlaytests(ctx)
	} else {
		path, pathErr := fogharbor.ValidatePath(*pathText)
		if pathErr != nil {
			fmt.Fprintf(os.Stderr, "fog-harbor playtest: %v\n", pathErr)
			os.Exit(2)
		}
		var report fogharbor.PlaytestReport
		report, err = fogharbor.RunPlaytest(ctx, fogharbor.PlaytestOptions{
			VariantID: *variantID,
			Path:      path,
		})
		reports = []fogharbor.PlaytestReport{report}
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "fog-harbor playtest: %v\n", err)
		os.Exit(1)
	}
	if *jsonOut {
		data, err := fogharbor.MarshalJSONReports(reports, *all)
		if err != nil {
			fmt.Fprintf(os.Stderr, "fog-harbor playtest: encode json: %v\n", err)
			os.Exit(1)
		}
		fmt.Fprintln(os.Stdout, string(data))
	} else {
		fmt.Fprintln(os.Stdout, fogharbor.FormatPlaytestReports(reports))
	}
	if fogharbor.HasFailures(reports) {
		os.Exit(1)
	}
}

func runFogHarborVerify(args []string) {
	fs := flag.NewFlagSet("fog-harbor verify", flag.ExitOnError)
	jsonOut := fs.Bool("json", false, "print machine-readable JSON")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "fog-harbor verify: %v\n", err)
		os.Exit(2)
	}

	report, err := fogharbor.Verify(context.Background())
	if err != nil {
		fmt.Fprintf(os.Stderr, "fog-harbor verify: %v\n", err)
		os.Exit(1)
	}
	if *jsonOut {
		data, err := fogharbor.MarshalVerifyJSON(report)
		if err != nil {
			fmt.Fprintf(os.Stderr, "fog-harbor verify: encode json: %v\n", err)
			os.Exit(1)
		}
		fmt.Fprintln(os.Stdout, string(data))
	} else {
		fmt.Fprintln(os.Stdout, fogharbor.FormatVerifyReport(report))
	}
	if !report.Passed {
		os.Exit(1)
	}
}
