# 01 — Rules Engine（W1）

## Goals

- 用纯函数实现 CoC 7e 全部规则裁定（骰子、技能检定、SAN、对抗、战斗简化、成长）
- 所有结果对象包含完整 trace，便于上层落库与 SLA 校验
- 测试覆盖率 ≥ 85%
- 不依赖任何 internal/ 兄弟包；不做 IO

## Non-Goals

- 不实现护甲、闪避、格挡（与需求文档 Non-Goals 对齐）
- 不在本层判定临时疯狂的 INT/5 比较（由 orchestrator 二次判定，保持 rules 接口窄）
- 不做完整 impale 伤害规则（W7 抛光）；MVP 用"伤害骰最大值 + 加伤一次"近似

## Interface

完整类型定义见 `internal/rules/types.go`，函数签名见对应文件：

| 文件 | 导出函数 |
|---|---|
| `dice.go` | `Roll(expression string, rng *rand.Rand) (DamageResult, error)` |
| `skillcheck.go` | `RollSkill(name string, value int, diff Difficulty, rng *rand.Rand, bonus, penalty int) SkillCheckResult` |
| `sanity.go` | `SanityCheck(currentSAN int, lossPass, lossFail string, rng *rand.Rand) (SanityResult, error)` |
| `opposed.go` | `OpposedRoll(actorName string, actorValue int, targetName string, targetValue int, rng *rand.Rand) OpposedResult` |
| `combat.go` | `Initiative(combatants []Combatant, rng *rand.Rand) []string`<br>`Attack(att, def Combatant, rng *rand.Rand) (SkillCheckResult, *DamageResult, error)` |
| `growth.go` | `GrowthCheck(name string, oldValue int, rng *rand.Rand) GrowthCheckResult`<br>`SettleGrowth(used map[string]int, rng *rand.Rand) []GrowthCheckResult` |

## Algorithm

### 骰子表达式语法

```
expr := term (('+' | '-') term)*
term := dice | int
dice := int 'd' int
```

限制：N、M ∈ [1, 100]；不支持嵌套、保留高/低、爆炸、Fudge。

**为什么不用三方库**：`justinian/dice`、`travis-g/dice` 的结果类型不分离暴露 `Modifier`（仅有 `Total`），但 SLA trace 要求把 `Rolls []int` 与 `Modifier int` 分别记录。语法面上我们也只需要 `NdM±K`，三方库的 4d6kh3 / 爆炸骰 / Fudge 等等是噪音。自写解析器约 30 行 + 表驱动测试，比包装三方库更小。

### 技能检定（CoC 7e）

```
threshold = value                               // regular
if hard:    threshold = value / 2               // 整除
if extreme: threshold = value / 5

roll = resolveBonusPenalty(rng, bonus, penalty) // 见下

degree :=
  case roll == 1:                          critical_success
  case value <  50 && roll >= 96:          fumble
  case value >= 50 && roll == 100:         fumble
  case roll <= value/5:                    extreme_success
  case roll <= value/2:                    hard_success
  case roll <= value:                      regular_success
  default:                                 failure

success :=
  case difficulty == regular: degree ∈ {critical, extreme, hard, regular}
  case difficulty == hard:    degree ∈ {critical, extreme, hard}
  case difficulty == extreme: degree ∈ {critical, extreme}
  // fumble / failure 永远 success=false
```

**奖励/惩罚骰**：CoC 7e 规则：
- 个位 d10 掷一次（0..9，0 当 10）
- 十位 d10 掷 `1 + max(bonus, penalty)` 次（每次 0..9，十位 0 = 0）
- 奖励骰取最低十位、惩罚骰取最高十位
- bonus 与 penalty 同时给 → 取 `max - min` 后保留较大者方向（即抵消）
- 若组合后十位与个位都为 0 → roll = 100

### SAN 检定

```
roll = d100
success := roll <= currentSAN
loss := dice.Roll(lossPass) if success else dice.Roll(lossFail)
newSAN := max(0, currentSAN - loss)

triggeredIndefiniteInsanity := loss >= 5
triggeredTemporaryInsanity := false   // 由 orchestrator 拿调查员 INT 二次判定
```

### 对抗检定

```
actor = RollSkill(actorName, actorValue, regular, ...)
target = RollSkill(targetName, targetValue, regular, ...)

按 critical(4) > extreme(3) > hard(2) > regular(1) > failure(0) > fumble(-1) 比 degree
同 degree 比 skillValue
再相同 = tie
```

### 战斗

- `Initiative`：DEX 降序；同 DEX 各掷 d100
- `Attack`：用 `weaponSkill` 做 `RollSkill`；命中 → `Roll(weaponDamage)`；extreme/critical → 伤害骰最大值 + 一次加伤
- 不实现护甲、闪避、格挡

### 成长

- `GrowthCheck`：技能值 ≥ 99 → 不成长；否则 d100 > old → 提升 1d10
- `SettleGrowth`：迭代 `usedSkills`，结果按 `skillName` 字典序返回（确定性）

## Data

所有结果结构体字段导出 + JSON tag，见 `internal/rules/types.go`。

## Tests

- 表驱动 + testify `require/assert`
- 固定种子（`rand.New(rand.NewPCG(seed1, seed2))`）保证可重放
- 文件分布：`{dice,skillcheck,sanity,opposed,combat,growth}_test.go`、`helpers_test.go`
- 覆盖率门槛在 `Makefile` 的 `cover` target 内强制 ≥ 85%

## Open Questions

1. ✅ SAN 临时疯狂（INT/5）下放到 orchestrator（接口窄）
2. ⏳ Impale 伤害 W7 抛光时改完整版
3. ⏳ `runs/<save_id>/turns.jsonl` 格式 W3 接入 GM Agent 时定稿
