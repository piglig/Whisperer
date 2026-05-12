package rules

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"strconv"
	"strings"
	"unicode"
)

// MaxDiceCount 与 MaxDiceSides 限定可解析的骰子参数上限。
// 出于防御性目的，避免恶意/异常 LLM 输出导致大量计算或溢出。
const (
	MaxDiceCount = 100
	MaxDiceSides = 100
)

// ErrInvalidDiceExpression 在表达式解析失败时返回。
var ErrInvalidDiceExpression = errors.New("invalid dice expression")

// Roll 解析并求值一个骰子表达式，返回完整 trace。
//
// 语法（精简到本项目所需）:
//
//	expr := term (('+' | '-') term)*
//	term := dice | int
//	dice := int 'd' int
//
// 示例: "1d6", "2d6+3", "1d10-1", "5"。
func Roll(expression string, rng *rand.Rand) (DamageResult, error) {
	tokens, err := tokenize(expression)
	if err != nil {
		return DamageResult{}, err
	}
	if len(tokens) == 0 {
		return DamageResult{}, fmt.Errorf("%w: empty expression", ErrInvalidDiceExpression)
	}

	res := DamageResult{Expression: strings.ReplaceAll(expression, " ", "")}

	// 期望 token 序列以 term 开头，term/op 交替。
	sign := 1
	i := 0
	if tokens[0] == "+" || tokens[0] == "-" {
		if tokens[0] == "-" {
			sign = -1
		}
		i = 1
	}
	if i >= len(tokens) {
		return DamageResult{}, fmt.Errorf("%w: expression has no terms", ErrInvalidDiceExpression)
	}

	for i < len(tokens) {
		tok := tokens[i]
		if tok == "+" || tok == "-" {
			return DamageResult{}, fmt.Errorf("%w: unexpected operator at position %d", ErrInvalidDiceExpression, i)
		}

		rolls, modifier, err := evalTerm(tok, rng)
		if err != nil {
			return DamageResult{}, err
		}
		for _, r := range rolls {
			if sign < 0 {
				res.Rolls = append(res.Rolls, -r)
				res.Total -= r
			} else {
				res.Rolls = append(res.Rolls, r)
				res.Total += r
			}
		}
		res.Modifier += sign * modifier
		res.Total += sign * modifier

		i++
		if i >= len(tokens) {
			break
		}
		op := tokens[i]
		switch op {
		case "+":
			sign = 1
		case "-":
			sign = -1
		default:
			return DamageResult{}, fmt.Errorf("%w: expected operator, got %q", ErrInvalidDiceExpression, op)
		}
		i++
		if i >= len(tokens) {
			return DamageResult{}, fmt.Errorf("%w: trailing operator %q", ErrInvalidDiceExpression, op)
		}
	}

	return res, nil
}

// evalTerm 求值一个 term，返回掷骰列表与常数贡献。
// 对于纯整数 term，rolls 为 nil、modifier 为常数；
// 对于骰子 term，rolls 为各骰点数、modifier 为 0。
func evalTerm(tok string, rng *rand.Rand) ([]int, int, error) {
	if idx := strings.IndexByte(tok, 'd'); idx >= 0 {
		countStr := tok[:idx]
		sidesStr := tok[idx+1:]
		count, err := strconv.Atoi(countStr)
		if err != nil || count <= 0 {
			return nil, 0, fmt.Errorf("%w: bad dice count %q", ErrInvalidDiceExpression, countStr)
		}
		sides, err := strconv.Atoi(sidesStr)
		if err != nil || sides <= 0 {
			return nil, 0, fmt.Errorf("%w: bad dice sides %q", ErrInvalidDiceExpression, sidesStr)
		}
		if count > MaxDiceCount {
			return nil, 0, fmt.Errorf("%w: dice count %d exceeds max %d", ErrInvalidDiceExpression, count, MaxDiceCount)
		}
		if sides > MaxDiceSides {
			return nil, 0, fmt.Errorf("%w: dice sides %d exceeds max %d", ErrInvalidDiceExpression, sides, MaxDiceSides)
		}
		rolls := make([]int, count)
		for i := range count {
			rolls[i] = rng.IntN(sides) + 1
		}
		return rolls, 0, nil
	}
	n, err := strconv.Atoi(tok)
	if err != nil {
		return nil, 0, fmt.Errorf("%w: bad integer %q", ErrInvalidDiceExpression, tok)
	}
	return nil, n, nil
}

// tokenize 把表达式拆为一序列 token：term 字符串与单字符运算符 "+"/"-"。
func tokenize(expr string) ([]string, error) {
	expr = strings.ReplaceAll(expr, " ", "")
	if expr == "" {
		return nil, nil
	}

	var tokens []string
	var cur strings.Builder
	for _, r := range expr {
		switch {
		case r == '+' || r == '-':
			if cur.Len() > 0 {
				tokens = append(tokens, cur.String())
				cur.Reset()
			}
			tokens = append(tokens, string(r))
		case unicode.IsDigit(r) || r == 'd':
			cur.WriteRune(r)
		default:
			return nil, fmt.Errorf("%w: unexpected character %q", ErrInvalidDiceExpression, r)
		}
	}
	if cur.Len() > 0 {
		tokens = append(tokens, cur.String())
	}
	return tokens, nil
}
