package rules

import "math/rand/v2"

// seededRand 给测试一个确定的随机源。
// 用 PCG（math/rand/v2 默认源），相同种子在同一 Go 版本下输出稳定。
func seededRand(seed uint64) *rand.Rand {
	return rand.New(rand.NewPCG(seed, seed^0x9E3779B97F4A7C15))
}
