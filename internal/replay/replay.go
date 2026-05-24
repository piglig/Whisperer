package replay

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/zhuzhenwu/whisperer/internal/agent"
	"github.com/zhuzhenwu/whisperer/internal/orchestrator"
	"github.com/zhuzhenwu/whisperer/internal/scenario"
)

const SchemaVersion = "replay.v1"

type Mark struct {
	Turn      int      `json:"turn"`
	Tags      []string `json:"tags"`
	Note      string   `json:"note,omitempty"`
	UpdatedAt string   `json:"updated_at"`
}

type Document struct {
	SchemaVersion string `json:"schema_version"`
	TracePath     string `json:"trace_path"`
	MarksPath     string `json:"marks_path"`
	SaveID        string `json:"save_id"`
	VariantID     string `json:"variant_id,omitempty"`
	Turns         []Turn `json:"turns"`
	Totals        Totals `json:"totals"`
	GeneratedAt   string `json:"generated_at"`
}

type Totals struct {
	Turns        int     `json:"turns"`
	InputTokens  int64   `json:"input_tokens"`
	OutputTokens int64   `json:"output_tokens"`
	CostUSD      float64 `json:"cost_usd"`
	Blocked      int     `json:"blocked"`
	SLAIssues    int     `json:"sla_issues"`
	JudgeIssues  int     `json:"judge_issues"`
	Marked       int     `json:"marked"`
}

type Turn struct {
	Number       int                       `json:"number"`
	Time         string                    `json:"time,omitempty"`
	PlayerInput  string                    `json:"player_input,omitempty"`
	GMOutput     string                    `json:"gm_output,omitempty"`
	Intent       string                    `json:"intent,omitempty"`
	Blocked      bool                      `json:"blocked"`
	BlockReasons []string                  `json:"block_reasons,omitempty"`
	Action       ActionDecision            `json:"action"`
	Tools        []ToolCall                `json:"tools,omitempty"`
	StateDiff    []StateChange             `json:"state_diff,omitempty"`
	SLA          SLAReport                 `json:"sla"`
	Judge        []JudgeCheck              `json:"judge,omitempty"`
	Ending       *Ending                   `json:"ending,omitempty"`
	Marks        []Mark                    `json:"marks,omitempty"`
	ScriptLine   string                    `json:"script_line,omitempty"`
	RawDecision  orchestrator.TurnDecision `json:"raw_decision"`
}

type ToolCall struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Iter   int    `json:"iter"`
	Target string `json:"target,omitempty"`
	Input  string `json:"input,omitempty"`
	Output string `json:"output,omitempty"`
}

type ActionDecision struct {
	Status             string   `json:"status"`
	ReasonCode         string   `json:"reason_code,omitempty"`
	PlayerFacingReason string   `json:"player_facing_reason,omitempty"`
	DebugReason        string   `json:"debug_reason,omitempty"`
	RequiredClues      []string `json:"required_clues,omitempty"`
	SuggestedActions   []string `json:"suggested_actions,omitempty"`
}

type StateChange struct {
	Kind   string `json:"kind"`
	ID     string `json:"id,omitempty"`
	Name   string `json:"name,omitempty"`
	From   string `json:"from,omitempty"`
	To     string `json:"to,omitempty"`
	Detail string `json:"detail,omitempty"`
}

type SLAReport struct {
	Passed       bool    `json:"passed"`
	EndingForced bool    `json:"ending_forced"`
	Violations   []Issue `json:"violations,omitempty"`
}

type Issue struct {
	Code    string `json:"code"`
	Message string `json:"message,omitempty"`
}

type JudgeCheck struct {
	Kind    string `json:"kind"`
	Target  string `json:"target,omitempty"`
	Passed  bool   `json:"passed"`
	Message string `json:"message,omitempty"`
}

type Ending struct {
	ID              string   `json:"id"`
	Kind            string   `json:"kind"`
	Description     string   `json:"description"`
	EvidenceStatus  string   `json:"evidence_status,omitempty"`
	CulpritName     string   `json:"culprit_name,omitempty"`
	FoundKeyClues   int      `json:"found_key_clues,omitempty"`
	MissingKeyClues []string `json:"missing_key_clues,omitempty"`
}

func LoadDocument(path string) (Document, error) {
	resolved, err := ResolvePath(path)
	if err != nil {
		return Document{}, err
	}
	entries, err := ReadEntries(resolved)
	if err != nil {
		return Document{}, err
	}
	marks, _ := ReadMarks(resolved)
	return BuildDocument(resolved, entries, marks), nil
}

func ResolvePath(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", errors.New("trace path is required")
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return path, nil
	}
	var files []string
	if err := filepath.WalkDir(path, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && filepath.Ext(p) == ".jsonl" {
			files = append(files, p)
		}
		return nil
	}); err != nil {
		return "", err
	}
	if len(files) == 0 {
		return "", fmt.Errorf("no .jsonl trace files under %s", path)
	}
	sort.Slice(files, func(i, j int) bool {
		ia, _ := os.Stat(files[i])
		ja, _ := os.Stat(files[j])
		return ia.ModTime().After(ja.ModTime())
	})
	return files[0], nil
}

func ReadEntries(path string) ([]orchestrator.TraceEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return DecodeEntries(f)
}

func DecodeEntries(r io.Reader) ([]orchestrator.TraceEntry, error) {
	var entries []orchestrator.TraceEntry
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	line := 0
	for scanner.Scan() {
		line++
		text := strings.TrimSpace(scanner.Text())
		if text == "" {
			continue
		}
		var entry orchestrator.TraceEntry
		if err := json.Unmarshal([]byte(text), &entry); err != nil {
			return nil, fmt.Errorf("line %d: %w", line, err)
		}
		entries = append(entries, entry)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, errors.New("trace has no entries")
	}
	return entries, nil
}

func MarksPath(tracePath string) string {
	return tracePath + ".marks.json"
}

func ReadMarks(tracePath string) (map[int][]Mark, error) {
	data, err := os.ReadFile(MarksPath(tracePath))
	if err != nil {
		return nil, err
	}
	var list []Mark
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, err
	}
	out := map[int][]Mark{}
	for _, mark := range list {
		out[mark.Turn] = append(out[mark.Turn], mark)
	}
	return out, nil
}

func BuildDocument(path string, entries []orchestrator.TraceEntry, marks map[int][]Mark) Document {
	doc := Document{
		SchemaVersion: SchemaVersion,
		TracePath:     path,
		MarksPath:     MarksPath(path),
		SaveID:        entries[0].SaveID,
		VariantID:     entries[0].VariantID,
		GeneratedAt:   time.Now().UTC().Format(time.RFC3339),
	}
	for _, entry := range entries {
		turn := buildTurn(entry, marks[entry.TurnNumber])
		doc.Turns = append(doc.Turns, turn)
		doc.Totals.InputTokens += entry.Result.Trace.InputTokens
		doc.Totals.OutputTokens += entry.Result.Trace.OutputTokens
		doc.Totals.CostUSD += entry.Result.Trace.TotalCostUSD
		if turn.Blocked {
			doc.Totals.Blocked++
		}
		if !turn.SLA.Passed || len(turn.SLA.Violations) > 0 {
			doc.Totals.SLAIssues++
		}
		for _, check := range turn.Judge {
			if !check.Passed {
				doc.Totals.JudgeIssues++
				break
			}
		}
		if len(turn.Marks) > 0 {
			doc.Totals.Marked++
		}
	}
	doc.Totals.Turns = len(doc.Turns)
	return doc
}

func buildTurn(entry orchestrator.TraceEntry, marks []Mark) Turn {
	res := entry.Result
	turn := Turn{
		Number:      entry.TurnNumber,
		PlayerInput: entry.UserInput,
		GMOutput:    res.Narrative,
		Intent:      string(res.Decision.Intent),
		Action: ActionDecision{
			Status:             string(res.Decision.ActionDecision.Status),
			ReasonCode:         res.Decision.ActionDecision.ReasonCode,
			PlayerFacingReason: res.Decision.ActionDecision.PlayerFacingReason,
			DebugReason:        res.Decision.ActionDecision.DebugReason,
			RequiredClues:      append([]string(nil), res.Decision.ActionDecision.RequiredClues...),
			SuggestedActions:   append([]string(nil), res.Decision.ActionDecision.SuggestedActions...),
		},
		Tools:       buildTools(res.Trace.ToolCalls),
		StateDiff:   buildStateDiff(res.Decision.StateChanges),
		SLA:         buildSLA(res),
		Judge:       buildJudge(res),
		Ending:      buildEnding(res.Ending, res.Report),
		Marks:       marks,
		ScriptLine:  strings.TrimSpace(entry.UserInput),
		RawDecision: res.Decision,
	}
	if entry.TraceAtMS > 0 {
		turn.Time = time.UnixMilli(entry.TraceAtMS).UTC().Format(time.RFC3339)
	}
	turn.Blocked, turn.BlockReasons = blockedReasons(res)
	return turn
}

func buildTools(calls []agent.ToolCall) []ToolCall {
	out := make([]ToolCall, 0, len(calls))
	for _, call := range calls {
		status := "ok"
		if call.IsError {
			status = "error"
		}
		out = append(out, ToolCall{
			Name:   call.Name,
			Status: status,
			Iter:   call.Iter,
			Target: toolTarget(call),
			Input:  compactJSON(call.Input),
			Output: compactJSON(call.Output),
		})
	}
	return out
}

func buildStateDiff(changes []orchestrator.DecisionChange) []StateChange {
	out := make([]StateChange, 0, len(changes))
	for _, change := range changes {
		out = append(out, StateChange(change))
	}
	return out
}

func buildSLA(res orchestrator.TurnResult) SLAReport {
	out := SLAReport{
		Passed:       res.SLAReport.Passed,
		EndingForced: res.SLAReport.EndingForced,
	}
	for _, violation := range res.SLAReport.Violations {
		out.Violations = append(out.Violations, Issue{
			Code:    string(violation.Code),
			Message: violation.Message,
		})
	}
	return out
}

func buildJudge(res orchestrator.TurnResult) []JudgeCheck {
	out := []JudgeCheck{}
	for _, check := range res.SLAReport.JudgeChecks {
		out = append(out, JudgeCheck{
			Kind:    check.Kind,
			Target:  check.Target,
			Passed:  check.Passed,
			Message: check.Message,
		})
	}
	return out
}

func buildEnding(ending *scenario.Ending, report *scenario.CaseReport) *Ending {
	if ending == nil {
		return nil
	}
	out := &Ending{
		ID:          ending.ID,
		Kind:        ending.Kind,
		Description: ending.Description,
	}
	if report != nil {
		out.EvidenceStatus = report.EvidenceStatus
		out.CulpritName = report.CulpritName
		out.FoundKeyClues = len(report.FoundKeyClues)
		for _, clue := range report.MissingKeyClues {
			out.MissingKeyClues = append(out.MissingKeyClues, firstNonEmpty(clue.Description, clue.ID))
		}
	}
	return out
}

func blockedReasons(res orchestrator.TurnResult) (bool, []string) {
	reasons := []string{}
	for _, check := range res.Decision.Checks {
		if check.Passed {
			continue
		}
		reasons = append(reasons, firstNonEmpty(check.Message, check.Code))
	}
	for _, call := range res.Trace.ToolCalls {
		if call.IsError {
			reasons = append(reasons, call.Name+": "+toolErrorMessage(call.Output))
		}
	}
	if len(res.Trace.ToolCalls) == 0 && len(res.Decision.Mechanics) == 0 && len(res.Decision.Checks) > 0 {
		for _, check := range res.Decision.Checks {
			if !check.Passed {
				reasons = append(reasons, firstNonEmpty(check.Message, check.Code))
			}
		}
	}
	return len(reasons) > 0, dedupe(reasons)
}

func WriteHTML(path string, doc Document) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("html path is required")
	}
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	html, err := RenderHTML(doc)
	if err != nil {
		return err
	}
	return os.WriteFile(path, []byte(html), 0o644)
}

func RenderHTML(doc Document) (string, error) {
	payload, err := json.Marshal(doc)
	if err != nil {
		return "", err
	}
	var b bytes.Buffer
	err = viewerTemplate.Execute(&b, struct {
		Title string
		Data  template.HTML
	}{
		Title: "Whisperer Replay",
		Data:  template.HTML(payload),
	})
	if err != nil {
		return "", err
	}
	return b.String(), nil
}

func toolTarget(call agent.ToolCall) string {
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

func toolErrorMessage(raw json.RawMessage) string {
	var payload struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(raw, &payload); err == nil && payload.Error != "" {
		return payload.Error
	}
	return compactJSON(raw)
}

func compactJSON(raw json.RawMessage) string {
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

func dedupe(values []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

var viewerTemplate = template.Must(template.New("viewer").Parse(`<!doctype html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Title}}</title>
<style>
:root{color-scheme:light;--bg:#f6f4ee;--panel:#fffdf8;--ink:#1d1b18;--muted:#756f65;--line:#d8d0c2;--accent:#0f766e;--bad:#b42318;--warn:#a15c07;--good:#287a3e;--code:#24211d}
*{box-sizing:border-box}body{margin:0;background:var(--bg);color:var(--ink);font:14px/1.45 ui-sans-serif,system-ui,-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif}
button,input,textarea,select{font:inherit}button{border:1px solid var(--line);background:var(--panel);color:var(--ink);border-radius:6px;padding:7px 9px;cursor:pointer}button.active{background:var(--accent);border-color:var(--accent);color:white}
.app{display:grid;grid-template-columns:300px minmax(360px,1fr) 380px;height:100vh;min-height:620px}
.sidebar,.detail{border-right:1px solid var(--line);overflow:auto}.detail{border-right:0;border-left:1px solid var(--line);background:#fbf7ef}
.top{position:sticky;top:0;background:rgba(246,244,238,.94);backdrop-filter:blur(8px);z-index:2;border-bottom:1px solid var(--line);padding:14px}
h1{font-size:18px;margin:0 0 8px}.meta{color:var(--muted);font-size:12px}.stats{display:grid;grid-template-columns:repeat(2,1fr);gap:6px;margin-top:12px}.stat{border:1px solid var(--line);border-radius:6px;padding:8px;background:var(--panel)}.stat b{display:block;font-size:18px}
.filters{display:flex;gap:6px;flex-wrap:wrap;margin-top:12px}.turn-list{padding:8px}.turn-btn{display:block;width:100%;text-align:left;margin:0 0 6px;padding:10px;border-radius:6px}.turn-btn .row{display:flex;align-items:center;justify-content:space-between;gap:10px}.pill{display:inline-flex;align-items:center;border:1px solid var(--line);border-radius:999px;padding:1px 7px;font-size:12px;color:var(--muted);white-space:nowrap}.pill.bad{color:var(--bad);border-color:#e5aaa4}.pill.warn{color:var(--warn);border-color:#e7c080}.pill.good{color:var(--good);border-color:#a8d4b4}.snippet{margin-top:5px;color:var(--muted);white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
.main{overflow:auto}.content{max-width:980px;margin:0 auto;padding:22px 28px 36px}.section{margin-bottom:22px}.section h2{font-size:13px;text-transform:uppercase;letter-spacing:0;color:var(--muted);margin:0 0 8px}.prose{white-space:pre-wrap;background:var(--panel);border:1px solid var(--line);border-radius:6px;padding:14px}.empty{color:var(--muted);font-style:italic}.grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(220px,1fr));gap:10px}.item{border:1px solid var(--line);background:var(--panel);border-radius:6px;padding:10px}.item h3{font-size:14px;margin:0 0 6px}.kv{color:var(--muted);font-size:12px}.code{font-family:ui-monospace,SFMono-Regular,Menlo,Consolas,monospace;background:var(--code);color:#f7f3eb;border-radius:6px;padding:10px;white-space:pre-wrap;overflow:auto}
.detail-inner{padding:14px}.panel{border:1px solid var(--line);background:var(--panel);border-radius:6px;padding:12px;margin-bottom:12px}.panel h2{font-size:14px;margin:0 0 8px}.list{margin:0;padding-left:18px}.mark-form{display:grid;gap:8px}.mark-form textarea{min-height:72px;resize:vertical;border:1px solid var(--line);border-radius:6px;padding:8px;background:white}.tag-row{display:flex;gap:6px;flex-wrap:wrap}.tag{user-select:none}.tag input{margin-right:5px}.toolbar{display:flex;gap:8px;flex-wrap:wrap}.hidden{display:none!important}
@media(max-width:980px){.app{grid-template-columns:1fr;height:auto}.sidebar,.detail{height:auto;border:0;border-bottom:1px solid var(--line)}.main{min-height:520px}.content{padding:18px}}
</style>
</head>
<body>
<div class="app">
  <aside class="sidebar">
    <div class="top">
      <h1>Whisperer Replay</h1>
      <div class="meta" id="meta"></div>
      <div class="stats" id="stats"></div>
      <div class="filters">
        <button data-filter="all" class="active">全部</button>
        <button data-filter="blocked">拦截</button>
        <button data-filter="issues">问题</button>
        <button data-filter="marked">标记</button>
      </div>
    </div>
    <div class="turn-list" id="turnList"></div>
  </aside>
  <main class="main"><div class="content" id="content"></div></main>
  <aside class="detail"><div class="detail-inner" id="detail"></div></aside>
</div>
<template id="replay-data">{{.Data}}</template>
<script>
const doc=JSON.parse(document.getElementById('replay-data').textContent);
let selected=doc.turns[0]?.number||0;let filter='all';
const $=id=>document.getElementById(id);const esc=s=>(s??'').toString().replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
function localMarks(){try{return JSON.parse(localStorage.getItem('whisperer.replay.marks.'+doc.trace_path)||'{}')}catch{return {}}}
function saveLocalMarks(v){localStorage.setItem('whisperer.replay.marks.'+doc.trace_path,JSON.stringify(v))}
function turnMarks(t){return [...(t.marks||[]),...(localMarks()[t.number]||[])]}
function hasIssue(t){return !t.sla.passed||(t.sla.violations||[]).length>0||(t.judge||[]).some(j=>!j.passed)}
function visible(t){if(filter==='blocked')return t.blocked;if(filter==='issues')return hasIssue(t);if(filter==='marked')return turnMarks(t).length>0;return true}
function renderShell(){ $('meta').innerHTML=esc(doc.save_id)+' '+(doc.variant_id?'/ '+esc(doc.variant_id):'')+'<br>'+esc(doc.trace_path);$('stats').innerHTML=[['回合',doc.totals.turns],['拦截',doc.totals.blocked],['SLA',doc.totals.sla_issues],['Judge',doc.totals.judge_issues],['标记',doc.totals.marked],['成本','$'+doc.totals.cost_usd.toFixed(4)]].map(x=>'<div class="stat"><b>'+x[1]+'</b><span>'+x[0]+'</span></div>').join('');document.querySelectorAll('[data-filter]').forEach(b=>b.onclick=()=>{filter=b.dataset.filter;document.querySelectorAll('[data-filter]').forEach(x=>x.classList.toggle('active',x===b));renderList()})}
function renderList(){const rows=doc.turns.filter(visible).map(t=>'<button class="turn-btn '+(t.number===selected?'active':'')+'" data-turn="'+t.number+'"><span class="row"><b>Turn '+t.number+'</b><span>'+(t.blocked?'<span class="pill bad">blocked</span>':'')+(hasIssue(t)?'<span class="pill warn">issue</span>':'')+(turnMarks(t).length?'<span class="pill good">mark</span>':'')+'</span></span><div class="snippet">'+esc(t.player_input||t.gm_output||'empty')+'</div></button>').join('')||'<div class="empty">没有符合筛选的回合</div>';$('turnList').innerHTML=rows;document.querySelectorAll('[data-turn]').forEach(b=>b.onclick=()=>{selected=Number(b.dataset.turn);renderAll()})}
function current(){return doc.turns.find(t=>t.number===selected)||doc.turns[0]}
function renderMain(){const t=current();if(!t)return;const a=t.action||{};$('content').innerHTML='<div class="section"><h2>玩家输入</h2><div class="prose">'+esc(t.player_input||'')+'</div></div><div class="section"><h2>GM 输出</h2><div class="prose">'+esc(t.gm_output||'')+'</div></div><div class="section"><h2>为什么被拦截</h2>'+(t.blocked?'<div class="item"><h3>'+esc(a.reason_code||'blocked')+'</h3><p>'+esc(a.player_facing_reason||t.block_reasons.join('\\n'))+'</p><div class="kv">'+esc(a.debug_reason||'')+'</div>'+((a.required_clues||[]).length?'<div class="kv">required clues: '+esc(a.required_clues.join(', '))+'</div>':'')+((a.suggested_actions||[]).length?'<ul class="list">'+a.suggested_actions.map(r=>'<li>'+esc(r)+'</li>').join('')+'</ul>':'')+'</div>':'<div class="empty">本回合没有拦截。</div>')+'</div><div class="section"><h2>状态 Diff</h2>'+cards(t.state_diff,c=>'<h3>'+esc(c.kind)+' '+esc(c.name||c.id||'')+'</h3><div class="kv">'+esc(c.detail||[c.from,c.to].filter(Boolean).join(' -> ')||'changed')+'</div>')+'</div><div class="section"><h2>结局报告</h2>'+(t.ending?'<div class="item"><h3>'+esc(t.ending.kind)+' / '+esc(t.ending.id)+'</h3><p>'+esc(t.ending.description)+'</p><div class="kv">'+esc(t.ending.evidence_status||'')+' '+esc(t.ending.culprit_name||'')+'</div></div>':'<div class="empty">本回合未触发结局。</div>')+'</div>'}
function cards(list,fn){return list&&list.length?'<div class="grid">'+list.map(x=>'<div class="item">'+fn(x)+'</div>').join('')+'</div>':'<div class="empty">无记录</div>'}
function renderDetail(){const t=current();if(!t)return;const marks=turnMarks(t);$('detail').innerHTML='<div class="panel"><h2>回合</h2><div class="kv">Turn '+t.number+' · '+esc(t.intent||'unknown')+'<br>'+esc(t.time||'')+'</div></div><div class="panel"><h2>Tool Calls</h2>'+cards(t.tools,c=>'<h3>'+esc(c.name)+' <span class="pill '+(c.status==='error'?'bad':'good')+'">'+esc(c.status)+'</span></h3><div class="kv">iter='+c.iter+' '+(c.target?'target='+esc(c.target):'')+'</div>'+(c.input?'<div class="code">'+esc(c.input)+'</div>':'')+(c.output?'<div class="code">'+esc(c.output)+'</div>':''))+'</div><div class="panel"><h2>SLA</h2><div class="pill '+(t.sla.passed?'good':'bad')+'">'+(t.sla.passed?'pass':'fail')+'</div>'+(t.sla.violations||[]).map(v=>'<div class="item"><b>'+esc(v.code)+'</b><div>'+esc(v.message)+'</div></div>').join('')+'</div><div class="panel"><h2>Judge</h2>'+cards(t.judge,j=>'<h3>'+esc(j.kind)+' <span class="pill '+(j.passed?'good':'bad')+'">'+(j.passed?'pass':'fail')+'</span></h3><div class="kv">'+esc(j.target||'')+'</div><div>'+esc(j.message||'')+'</div>')+'</div><div class="panel"><h2>标记</h2>'+(marks.length?marks.map(m=>'<div class="item"><b>'+esc((m.tags||[]).join(', '))+'</b><div>'+esc(m.note||'')+'</div></div>').join(''):'<div class="empty">无标记</div>')+'<div class="mark-form"><div class="tag-row">'+['不好玩','漂移','误判','节奏慢','信息不足'].map(x=>'<label class="tag"><input type="checkbox" value="'+x+'">'+x+'</label>').join('')+'</div><textarea id="markNote" placeholder="记录这一回合的问题"></textarea><button id="saveMark">保存到浏览器</button></div></div><div class="panel"><h2>导出</h2><div class="toolbar"><button id="copyScript">复制 playtest script</button><button id="downloadMarks">导出标记 JSON</button></div></div>';document.getElementById('saveMark').onclick=()=>{const tags=[...document.querySelectorAll('.tag input:checked')].map(x=>x.value);if(!tags.length)return;const all=localMarks();all[t.number]=[{turn:t.number,tags,note:document.getElementById('markNote').value,updated_at:new Date().toISOString()}];saveLocalMarks(all);renderAll()};document.getElementById('copyScript').onclick=()=>navigator.clipboard?.writeText(doc.turns.map(x=>x.script_line?'# Turn '+x.number+'\n'+x.script_line:'').filter(Boolean).join('\n\n'));document.getElementById('downloadMarks').onclick=()=>download('replay-marks.json',JSON.stringify(localMarks(),null,2))}
function download(name,text){const a=document.createElement('a');a.href=URL.createObjectURL(new Blob([text],{type:'application/json'}));a.download=name;a.click();URL.revokeObjectURL(a.href)}
function renderAll(){renderShell();renderList();renderMain();renderDetail()}
renderAll();
</script>
</body>
</html>`))
