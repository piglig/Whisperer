package orchestrator

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/zhuzhenwu/whisperer/internal/secrets"
)

// TraceWriter 把每回合的 TurnResult 追加到 JSONL 文件，便于离线分析（按 variant
// 算胜率、按 SLA code 看违规分布、复现回合等）。
//
// 文件路径：<dir>/<save_id>/<session_ts>.jsonl
//
//   - dir 是 orchestrator.Config.TraceDir
//   - session_ts 是 Orchestrator 进程启动时刻（让一次连续会话落同一个文件，便于
//     按时间顺序 cat / jq）
//
// 一行一个 TraceEntry。文件末尾不补 newline 之外的内容；可被 `jq -c` / `pandas
// read_json(lines=True)` / `awk` 直接消费。
//
// 写失败仅 log，不阻塞 RunTurn——观测层故障不应让玩家这一回合丢失。
type TraceWriter struct {
	mu          sync.Mutex
	dir         string
	saveID      string
	sessionFile string
	disabled    bool
}

// TraceEntry 是 JSONL 中的一行结构。turn_number 在 JSONL 内冗余便于流处理；
// trace_at 是写入时刻 UTC 毫秒，便于排序。
type TraceEntry struct {
	TraceAtMS  int64       `json:"trace_at_ms"`
	SaveID     string      `json:"save_id"`
	VariantID  string      `json:"variant_id,omitempty"`
	TurnNumber int         `json:"turn_number"`
	Result     TurnResult  `json:"result"`
	UserInput  string      `json:"user_input,omitempty"`
	Extra      interface{} `json:"extra,omitempty"`
}

// NewTraceWriter 构造一个 TraceWriter。dir 为空或 "-" → disabled，所有写入是 no-op。
//
// 不在此处建文件——延迟到第一次 Append 才真正打开/创建，避免空会话留下空文件。
func NewTraceWriter(dir, saveID string) *TraceWriter {
	w := &TraceWriter{dir: dir, saveID: saveID}
	if dir == "" || dir == "-" {
		w.disabled = true
		return w
	}
	stamp := time.Now().UTC().Format("20060102T150405Z")
	w.sessionFile = filepath.Join(dir, saveID, stamp+".jsonl")
	return w
}

// Path 返回当次会话写入的 jsonl 路径；disabled 时返回空。
func (w *TraceWriter) Path() string {
	if w == nil || w.disabled {
		return ""
	}
	return w.sessionFile
}

// Append 把一回合的结果以 JSONL 行追加到文件。线程安全。disabled 时直接返回 nil。
func (w *TraceWriter) Append(entry TraceEntry) error {
	if w == nil || w.disabled {
		return nil
	}
	w.mu.Lock()
	defer w.mu.Unlock()

	if entry.TraceAtMS == 0 {
		entry.TraceAtMS = time.Now().UTC().UnixMilli()
	}
	if entry.SaveID == "" {
		entry.SaveID = w.saveID
	}
	// 在落盘前抹掉看起来像 API key 的子串。玩家可能在 TUI 里粘错；GM 也可能把
	// system prompt 里出现过的 key 反刍出来——trace 文件可能被 share 出去做
	// debug，这里是最后一道防线。
	entry.UserInput = secrets.Redact(entry.UserInput)
	entry.Result.Narrative = secrets.Redact(entry.Result.Narrative)
	entry.Result.Trace.Narrative = secrets.Redact(entry.Result.Trace.Narrative)

	if err := os.MkdirAll(filepath.Dir(w.sessionFile), 0o755); err != nil {
		return fmt.Errorf("trace mkdir: %w", err)
	}
	f, err := os.OpenFile(w.sessionFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("trace open: %w", err)
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(entry); err != nil {
		return fmt.Errorf("trace encode: %w", err)
	}
	return nil
}

// ValidateTraceDir 在 orchestrator.New 验证阶段调用——保证 dir 路径在文件系统上
// 至少能创建（提前失败优于第一回合才暴露）。空字符串 / "-" 跳过。
func ValidateTraceDir(dir string) error {
	if dir == "" || dir == "-" {
		return nil
	}
	if strings.HasPrefix(dir, "~") {
		return errors.New("trace dir cannot start with '~' — expand it via shell first")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("trace dir not writable: %w", err)
	}
	return nil
}
