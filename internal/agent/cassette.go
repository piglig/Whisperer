package agent

import (
	"errors"
	"fmt"
	"net/http"
	"os"

	"gopkg.in/dnaeon/go-vcr.v4/pkg/cassette"
	vcr "gopkg.in/dnaeon/go-vcr.v4/pkg/recorder"
)

// CassetteRecorder 包装 go-vcr v4 的 Recorder，让 Anthropic SDK 能透明走录放：
//
//   - 录制（WHISPERER_VCR_RECORD=1）：实际打 HTTP 出去，把请求/响应落到 yaml
//   - 回放（默认）：从 yaml 读 fixture，不上网
//
// 用途：CI 跑 LLM-集成测试不烧 token；开发者 review PR 时能在本地复现别人录下
// 的真实回合。把它跟 anthropic.NewClient 拼接需要 option.WithHTTPClient。
type CassetteRecorder struct {
	rec *vcr.Recorder
}

// CassetteOptions 是构造 CassetteRecorder 的输入。
type CassetteOptions struct {
	// Path 是 cassette 文件去掉 .yaml 后缀的路径。
	// 例： "testdata/cassettes/foo" → 实际文件 "testdata/cassettes/foo.yaml"
	Path string

	// Mode 显式指定模式；为 nil 时从 WHISPERER_VCR_RECORD 环境变量推断：
	//   - "1" / "true" → ModeRecordOnce
	//   - 其他          → ModeReplayOnly（CI-safe 默认）
	Mode *vcr.Mode
}

// NewCassetteRecorder 创建并初始化一个 recorder。Stop 必须在测试结束被调用以
// 把 episode 落盘（录制模式下）。
//
// 默认在落盘前抹掉认证类 header，避免真实 key 进入 fixture。
func NewCassetteRecorder(opts CassetteOptions) (*CassetteRecorder, error) {
	if opts.Path == "" {
		return nil, errors.New("cassette: Path is required")
	}
	mode := vcr.ModeReplayOnly
	if opts.Mode != nil {
		mode = *opts.Mode
	} else if recordEnabled() {
		mode = vcr.ModeRecordOnce
	}

	r, err := vcr.New(opts.Path,
		vcr.WithMode(mode),
		vcr.WithHook(stripAuthHeaders, vcr.BeforeSaveHook),
		vcr.WithMatcher(matchByMethodAndPath),
	)
	if err != nil {
		return nil, fmt.Errorf("cassette: open %s: %w", opts.Path, err)
	}

	return &CassetteRecorder{rec: r}, nil
}

// matchByMethodAndPath 是 cassette 匹配器：只比对 HTTP method + path。
//
// 默认匹配器对 body 做严格 equal，但 LLM SDK 每次发的请求 body 因为版本号 /
// 内部 trace id / 时间戳等微小差异都会不一致，导致 fixture 匹配失败。这里放宽
// 到 method+path 是 LLM 回放测试的常见做法（同等 fixture VCR.py 走这种匹配模式
// 的方案数不胜数）——副作用是同一 path 上多个不同 body 的请求要按 cassette 顺序
// 一一对应，不能错位。
func matchByMethodAndPath(r *http.Request, i cassette.Request) bool {
	if r.Method != i.Method {
		return false
	}
	if r.URL.Path != "" && i.URL != "" {
		// i.URL 是完整 URL，截到 path 比即可
		return r.URL.Path == extractPath(i.URL)
	}
	return r.URL.String() == i.URL
}

// extractPath 从完整 URL 取出 path 部分；解析失败时退化返回原串。
func extractPath(rawURL string) string {
	// 简化：找 "://" 后第一个 "/"
	if idx := indexAfter(rawURL, "://"); idx >= 0 {
		if slash := indexFrom(rawURL, "/", idx); slash >= 0 {
			return rawURL[slash:]
		}
	}
	return rawURL
}

func indexAfter(s, sep string) int {
	for i := 0; i+len(sep) <= len(s); i++ {
		if s[i:i+len(sep)] == sep {
			return i + len(sep)
		}
	}
	return -1
}

func indexFrom(s, sub string, from int) int {
	for i := from; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

// Client 返回一个 *http.Client，把它作为 anthropic SDK 的 HTTP 客户端即可让所有
// LLM 调用经由本 recorder。
func (c *CassetteRecorder) Client() *http.Client {
	return c.rec.GetDefaultClient()
}

// Mode 暴露当前模式（便于测试断言）。
func (c *CassetteRecorder) Mode() vcr.Mode {
	return c.rec.Mode()
}

// Stop 把 cassette 落盘并停止 recorder。即使 mode == replay 也要调用——会校验
// 所有 episode 都被消费过（防止漏跑导致 fixture 漂移）。
func (c *CassetteRecorder) Stop() error {
	return c.rec.Stop()
}

// recordEnabled 检查 WHISPERER_VCR_RECORD 环境变量。
func recordEnabled() bool {
	v := os.Getenv("WHISPERER_VCR_RECORD")
	return v == "1" || v == "true" || v == "TRUE"
}

// 写盘前要抹掉的请求头：覆盖 Anthropic 官方 + OpenRouter + 通用 OAuth。
var sensitiveRequestHeaders = []string{
	"x-api-key",
	"X-Api-Key",
	"Authorization",
	"openrouter-api-key",
	"OpenRouter-Api-Key",
	"anthropic-version",
	"anthropic-beta",
}

// stripAuthHeaders 是 vcr.BeforeSaveHook：在写盘前清掉认证 header。fixture 文件
// 永远不应该带真实 key 出门。
func stripAuthHeaders(i *cassette.Interaction) error {
	if i == nil {
		return nil
	}
	for _, h := range sensitiveRequestHeaders {
		if _, ok := i.Request.Headers[h]; ok {
			i.Request.Headers[http.CanonicalHeaderKey(h)] = []string{"REDACTED"}
		}
	}
	return nil
}
