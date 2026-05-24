package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/zhuzhenwu/whisperer/internal/agent"
	"github.com/zhuzhenwu/whisperer/internal/orchestrator"
	replayview "github.com/zhuzhenwu/whisperer/internal/replay"
	"github.com/zhuzhenwu/whisperer/internal/scenario"
)

type replayOptions struct {
	Path         string
	Turn         int
	ShowTools    bool
	ExportScript string
	HTML         string
	Mark         string
	Note         string
}

type replayMark struct {
	Turn      int      `json:"turn"`
	Tags      []string `json:"tags"`
	Note      string   `json:"note,omitempty"`
	UpdatedAt string   `json:"updated_at"`
}

func runReplaySubcommand(args []string) {
	fs := flag.NewFlagSet("replay", flag.ExitOnError)
	turn := fs.Int("turn", 0, "show only one turn number")
	showTools := fs.Bool("tools", true, "show tool calls")
	exportScript := fs.String("export-script", "", "write player inputs as a playtest script")
	htmlOut := fs.String("html", "", "write an interactive local replay viewer HTML file")
	mark := fs.String("mark", "", "mark --turn with tags: bad, drift, misjudge, or comma-separated values")
	note := fs.String("note", "", "note stored with --mark")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "replay: %v\n", err)
		os.Exit(2)
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: whisperer replay [--turn N] [--tools=false] [--html viewer.html] <trace.jsonl | trace-dir>")
		os.Exit(2)
	}
	out, err := RenderReplay(replayOptions{
		Path:         fs.Arg(0),
		Turn:         *turn,
		ShowTools:    *showTools,
		ExportScript: *exportScript,
		HTML:         *htmlOut,
		Mark:         *mark,
		Note:         *note,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "replay: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintln(os.Stdout, out)
}

func RenderReplay(opts replayOptions) (string, error) {
	path, err := resolveReplayPath(opts.Path)
	if err != nil {
		return "", err
	}
	entries, err := readTraceEntries(path)
	if err != nil {
		return "", err
	}
	if opts.Turn > 0 {
		entries = filterReplayTurn(entries, opts.Turn)
		if len(entries) == 0 {
			return "", fmt.Errorf("turn %d not found in %s", opts.Turn, path)
		}
	}
	if opts.ExportScript != "" {
		if err := exportReplayScript(opts.ExportScript, entries); err != nil {
			return "", err
		}
		return "playtest script exported: " + opts.ExportScript, nil
	}
	if opts.HTML != "" {
		marks, _ := replayview.ReadMarks(path)
		doc := replayview.BuildDocument(path, entries, marks)
		if err := replayview.WriteHTML(opts.HTML, doc); err != nil {
			return "", err
		}
		return "replay viewer exported: " + opts.HTML, nil
	}
	if opts.Mark != "" {
		if opts.Turn <= 0 {
			return "", errors.New("--mark requires --turn")
		}
		if err := writeReplayMark(path, opts.Turn, opts.Mark, opts.Note); err != nil {
			return "", err
		}
		return fmt.Sprintf("turn %d marked: %s", opts.Turn, opts.Mark), nil
	}
	marks, _ := readReplayMarks(path)
	var b strings.Builder
	writeReplayHeader(&b, path, entries)
	for _, entry := range entries {
		writeReplayEntry(&b, entry, opts.ShowTools, marks[entry.TurnNumber])
	}
	return strings.TrimRight(b.String(), "\n"), nil
}

func resolveReplayPath(path string) (string, error) {
	return replayview.ResolvePath(path)
}

func readTraceEntries(path string) ([]orchestrator.TraceEntry, error) {
	return replayview.ReadEntries(path)
}

func filterReplayTurn(entries []orchestrator.TraceEntry, turn int) []orchestrator.TraceEntry {
	var out []orchestrator.TraceEntry
	for _, entry := range entries {
		if entry.TurnNumber == turn {
			out = append(out, entry)
		}
	}
	return out
}

func writeReplayHeader(b *strings.Builder, path string, entries []orchestrator.TraceEntry) {
	first := entries[0]
	last := entries[len(entries)-1]
	totalIn, totalOut := int64(0), int64(0)
	totalCost := 0.0
	for _, entry := range entries {
		totalIn += entry.Result.Trace.InputTokens
		totalOut += entry.Result.Trace.OutputTokens
		totalCost += entry.Result.Trace.TotalCostUSD
	}
	fmt.Fprintf(b, "Whisperer Replay\n")
	fmt.Fprintf(b, "file: %s\n", path)
	fmt.Fprintf(b, "save: %s\n", first.SaveID)
	if first.VariantID != "" {
		fmt.Fprintf(b, "variant: %s\n", first.VariantID)
	}
	fmt.Fprintf(b, "turns: %d", len(entries))
	if first.TurnNumber != last.TurnNumber {
		fmt.Fprintf(b, " (%d -> %d)", first.TurnNumber, last.TurnNumber)
	}
	fmt.Fprintf(b, "\n")
	fmt.Fprintf(b, "tokens: in %d / out %d · cost $%.4f\n", totalIn, totalOut, totalCost)
	fmt.Fprintf(b, "\n")
}

func writeReplayEntry(b *strings.Builder, entry orchestrator.TraceEntry, showTools bool, marks []replayMark) {
	res := entry.Result
	fmt.Fprintf(b, "=== Turn %d ===\n", entry.TurnNumber)
	if entry.TraceAtMS > 0 {
		fmt.Fprintf(b, "time: %s\n", time.UnixMilli(entry.TraceAtMS).UTC().Format(time.RFC3339))
	}
	if entry.UserInput != "" {
		fmt.Fprintf(b, "\n[player]\n%s\n", indentBlock(entry.UserInput, "  "))
	}
	if res.Narrative != "" {
		fmt.Fprintf(b, "\n[gm]\n%s\n", indentBlock(res.Narrative, "  "))
	}
	writeReplayMarks(b, marks)
	writeReplayDecision(b, res)
	if showTools {
		writeReplayTools(b, res.Trace.ToolCalls)
	}
	writeReplaySLA(b, res)
	writeReplayJudge(b, res)
	if res.Ending != nil {
		writeReplayEnding(b, res.Ending, res.Report)
	}
	fmt.Fprintf(b, "\n")
}

func exportReplayScript(path string, entries []orchestrator.TraceEntry) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("export path is required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil && filepath.Dir(path) != "." {
		return err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# Exported from Whisperer trace\n")
	for _, entry := range entries {
		input := strings.TrimSpace(entry.UserInput)
		if input == "" {
			continue
		}
		fmt.Fprintf(&b, "\n# Turn %d\n%s\n", entry.TurnNumber, input)
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

func writeReplayMark(tracePath string, turn int, rawTags, note string) error {
	tags := parseReplayTags(rawTags)
	if len(tags) == 0 {
		return errors.New("mark tag is required")
	}
	path := replayMarksPath(tracePath)
	marks, err := readReplayMarksList(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	updated := replayMark{
		Turn:      turn,
		Tags:      tags,
		Note:      strings.TrimSpace(note),
		UpdatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	replaced := false
	for i := range marks {
		if marks[i].Turn == turn {
			marks[i] = updated
			replaced = true
			break
		}
	}
	if !replaced {
		marks = append(marks, updated)
	}
	sort.Slice(marks, func(i, j int) bool { return marks[i].Turn < marks[j].Turn })
	data, err := json.MarshalIndent(marks, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func readReplayMarks(tracePath string) (map[int][]replayMark, error) {
	list, err := readReplayMarksList(replayMarksPath(tracePath))
	if err != nil {
		return nil, err
	}
	out := map[int][]replayMark{}
	for _, mark := range list {
		out[mark.Turn] = append(out[mark.Turn], mark)
	}
	return out, nil
}

func readReplayMarksList(path string) ([]replayMark, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var marks []replayMark
	if err := json.Unmarshal(data, &marks); err != nil {
		return nil, err
	}
	return marks, nil
}

func replayMarksPath(tracePath string) string {
	return tracePath + ".marks.json"
}

func parseReplayTags(raw string) []string {
	seen := map[string]bool{}
	var tags []string
	for _, part := range strings.Split(raw, ",") {
		tag := strings.TrimSpace(part)
		if tag == "" || seen[tag] {
			continue
		}
		seen[tag] = true
		tags = append(tags, tag)
	}
	return tags
}

func writeReplayMarks(b *strings.Builder, marks []replayMark) {
	if len(marks) == 0 {
		return
	}
	fmt.Fprintf(b, "\n[marks]\n")
	for _, mark := range marks {
		fmt.Fprintf(b, "  - %s", strings.Join(mark.Tags, ", "))
		if mark.Note != "" {
			fmt.Fprintf(b, ": %s", mark.Note)
		}
		fmt.Fprintf(b, "\n")
	}
}

func writeReplayDecision(b *strings.Builder, res orchestrator.TurnResult) {
	fmt.Fprintf(b, "\n[recap]\n")
	if res.Decision.Intent != "" {
		fmt.Fprintf(b, "  intent: %s\n", res.Decision.Intent)
	}
	if len(res.Decision.StateChanges) == 0 {
		fmt.Fprintf(b, "  state: no tracked state changes\n")
	} else {
		fmt.Fprintf(b, "  state changes:\n")
		for _, change := range res.Decision.StateChanges {
			fmt.Fprintf(b, "    - %s%s%s\n", change.Kind, replayNameSuffix(change.Name, change.ID), replayChangeSuffix(change))
		}
	}
	if len(res.Decision.Checks) > 0 {
		fmt.Fprintf(b, "  checks:\n")
		for _, check := range res.Decision.Checks {
			status := "pass"
			if !check.Passed {
				status = "fail"
			}
			msg := strings.TrimSpace(check.Message)
			if msg != "" {
				msg = " · " + msg
			}
			fmt.Fprintf(b, "    - %s: %s%s\n", check.Code, status, msg)
		}
	}
}

func writeReplayTools(b *strings.Builder, calls []agent.ToolCall) {
	fmt.Fprintf(b, "\n[tools]\n")
	if len(calls) == 0 {
		fmt.Fprintf(b, "  none\n")
		return
	}
	for _, call := range calls {
		status := "ok"
		if call.IsError {
			status = "error"
		}
		fmt.Fprintf(b, "  - %s [%s] iter=%d", call.Name, status, call.Iter)
		if target := replayToolTarget(call); target != "" {
			fmt.Fprintf(b, " target=%s", target)
		}
		fmt.Fprintf(b, "\n")
		if in := compactReplayJSON(call.Input); in != "" {
			fmt.Fprintf(b, "      input: %s\n", truncateReplay(in, 180))
		}
		if out := compactReplayJSON(call.Output); out != "" {
			fmt.Fprintf(b, "      output: %s\n", truncateReplay(out, 180))
		}
	}
}

func writeReplaySLA(b *strings.Builder, res orchestrator.TurnResult) {
	fmt.Fprintf(b, "\n[sla]\n")
	fmt.Fprintf(b, "  passed: %v\n", res.SLAReport.Passed)
	if res.SLAReport.EndingForced {
		fmt.Fprintf(b, "  ending forced: true\n")
	}
	for _, violation := range res.SLAReport.Violations {
		fmt.Fprintf(b, "  - %s: %s\n", violation.Code, violation.Message)
	}
}

func writeReplayJudge(b *strings.Builder, res orchestrator.TurnResult) {
	fmt.Fprintf(b, "\n[judge]\n")
	if len(res.SLAReport.JudgeChecks) == 0 {
		fmt.Fprintf(b, "  not recorded\n")
		return
	}
	for _, check := range res.SLAReport.JudgeChecks {
		status := "pass"
		if !check.Passed {
			status = "fail"
		}
		target := ""
		if check.Target != "" {
			target = " target=" + check.Target
		}
		msg := ""
		if check.Message != "" {
			msg = " · " + check.Message
		}
		fmt.Fprintf(b, "  - %s: %s%s%s\n", check.Kind, status, target, msg)
	}
}

func writeReplayEnding(b *strings.Builder, ending *scenario.Ending, report *scenario.CaseReport) {
	fmt.Fprintf(b, "\n[ending]\n")
	fmt.Fprintf(b, "  %s / %s: %s\n", ending.Kind, ending.ID, ending.Description)
	if report == nil {
		return
	}
	if report.EvidenceStatus != "" {
		fmt.Fprintf(b, "  evidence: %s\n", report.EvidenceStatus)
	}
	if report.CulpritName != "" {
		fmt.Fprintf(b, "  culprit: %s\n", report.CulpritName)
	}
	if len(report.FoundKeyClues) > 0 {
		fmt.Fprintf(b, "  found key clues: %d\n", len(report.FoundKeyClues))
	}
	if len(report.MissingKeyClues) > 0 {
		fmt.Fprintf(b, "  missing key clues:\n")
		for _, clue := range report.MissingKeyClues {
			fmt.Fprintf(b, "    - %s\n", firstReplayNonEmpty(clue.Description, clue.ID))
		}
	}
}

func replayNameSuffix(name, id string) string {
	value := firstReplayNonEmpty(name, id)
	if value == "" {
		return ""
	}
	return " " + value
}

func replayChangeSuffix(change orchestrator.DecisionChange) string {
	if change.Detail != "" {
		return ": " + change.Detail
	}
	if change.From != "" || change.To != "" {
		return ": " + firstReplayNonEmpty(change.From, "?") + " -> " + firstReplayNonEmpty(change.To, "?")
	}
	return ""
}

func replayToolTarget(call agent.ToolCall) string {
	var input map[string]any
	if err := json.Unmarshal(call.Input, &input); err != nil {
		return ""
	}
	for _, key := range []string{"location_id", "npc_id", "clue_id", "item_id", "investigator_id"} {
		if value, ok := input[key].(string); ok && value != "" {
			return value
		}
	}
	return ""
}

func compactReplayJSON(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return string(raw)
	}
	b, err := json.Marshal(v)
	if err != nil {
		return string(raw)
	}
	return string(b)
}

func indentBlock(text, prefix string) string {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	for i, line := range lines {
		lines[i] = prefix + line
	}
	return strings.Join(lines, "\n")
}

func truncateReplay(text string, limit int) string {
	if limit <= 0 || len(text) <= limit {
		return text
	}
	return text[:limit] + "..."
}

func firstReplayNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
