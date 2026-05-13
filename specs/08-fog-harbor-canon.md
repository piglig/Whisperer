# 08 — Fog Harbor v0.3.1 Canon

完整剧本设定。本文档是 GM-side 真相手册，**不在玩家可见的渲染中暴露**——但通过
`scenario.Truth` / `SNPC.Secret` / `SClue.Tier` 等字段注入 GM system prompt，让
LLM 据此扮演 NPC、布置线索。

## 主线：失踪案的表象

调查员（默认占位为记者 Lyra Marsh）应报社派遣，到访雾港调查 16 岁女孩露西·梅森的
失踪。表面看是一起边远小镇的常规失踪——海莲娜（露西母亲）形容露西「像离家出走」，
但镇上每隔几年就有这种失踪在发生。

## 真相（默认 / vance_pact variant）

雾港旧时是走私港，三十年前一次海难后，灯塔守与镇上少数知情者与栖息于灯塔下方礁洞
里的「深潜者」群落达成隐秘契约：每隔数年由镇上"输送"一名无人挂念的边缘人——孤女、
流浪汉、负债的水手——作为活祭。换取的是小镇免受外海风暴、瘟疫与外海未知存在的
侵扰。

- **范斯医生**是这一代人类一方主谋。坐诊掌握谁"无人挂念"。
- **罗克长官**受贿默许；银行存单藏在巡警所抽屉锁底。
- **玛丽莎**部分知情、被胁迫——账册某几页对应"祭品当夜"的范斯出诊时间。
- **欧林**是被收买的眼线，但已动摇；那晚他让灯塔的光"恰好移开"。
- **卡尔文神父**知情但守密——一生研究教区登记簿、保留古约信仰，认为这是镇与海的
  规矩，凡人不应妄动。
- **海莲娜**不知情，是真心相信"露西是离家出走"的悲恸母亲。
- 灯塔基座下的礁洞在月圆暴风夜的低潮才完全露出；那时深潜者长老会现身唱诵。
- 露西已被作为最近一名祭品安置在礁洞。

## NPC 秘密表（默认 vance_executes variant）

| ID | 名 | 角色 | secret |
|---|---|---|---|
| vance | 范斯医生 | 嫌疑人/共谋主谋 | 献祭契约的人类一方主谋 |
| marisa | 玛丽莎 | 酒馆老板娘 | 部分知情，被胁迫；账册即"祭品台账" |
| helena | 海莲娜 | 受害者母亲 | 不知情，唯一悲恸 |
| orin | 欧林 | 灯塔守 | 被收买的眼线，正在动摇 |
| rourke | 罗克 | 巡警长官 | 长期受贿默许 |
| father_calvin | 卡尔文神父 | 教士 | 知情但守密；"古约"信仰者 |
| **anna** | **安娜·里弗斯** | **酒馆女招待 / 下个候选祭品** | **范斯免费义诊她，下镇静剂；她不知道自己是下一个** |
| lucy | 露西 | 失踪者 | 已死（除 pact_broken 外） |
| deep_elder | 深潜者长老 | 终幕怪物 | 可被封印仪式劝退 |

Anna 是 v0.3.1 新增的"下一个受害者面孔"——给玩家具体的情感驱动（不止报仇已死的
Lucy，还要救眼前的 Anna）。`anna_taken` 触发器在 turn_ge:22 + 未对峙主谋 + Anna
关系 < 26 时杀掉她；玩家通过持续对话建立关系或早一步对峙主谋来拯救。

## 线索网（三层 + 1 红鲱，v0.3.1 共 16 条）

Three Clue Rule：每个关键结论至少 3 条独立线索路径，缺 1 条不卡死。

```
Tier 1（表层 — 失踪经过 / Lucy 已死的多重证据）
  ├── blood_letter   (码头公告)        — "潮位低于表，注意北岬"
  ├── tide_chart     (巡警所/海莲娜)    — 失踪夜潮位异常低
  ├── lucy_diary     (露西房间)         — 「他答应送我去远一点的港口」
  └── cloth_scrap    (码头退潮)         — 退潮露出布料碎片，海莲娜认得花纹 ★ NEW
        ↓
Tier 2（共谋 — 镇上人类参与 / 30 年模式 / 主谋身份）
  ├── ledger             (酒馆/夜话)       — 签名时间 == 范斯出诊
  ├── doctor_visits_log  (酒馆/玛丽莎)     — "祭品夜"的出诊对象规律
  ├── clinic_supplies    (酒馆账册另面)    — 镇静剂订单异常激增 ★ NEW
  ├── rourke_bribe       (巡警所抽屉)      — 银行存单署名 R.
  ├── parish_record      (教堂/神父)       — 三十年间 7 例规律失踪
  ├── empty_graves       (教堂墓地北角)    — 七座小石碑，"衣冠冢"过于整齐 ★ NEW
  └── anna_warning       (安娜亲口)        — 「医生这阵子对我特别好」 ★ NEW
        ↓
Tier 3（神话 — 克苏鲁存在）
  ├── strange_chant         (灯塔/夜)   — 多嗓子合鸣
  ├── reef_carvings         (礁洞)      — 螺旋符文 = 教区图样
  ├── sacrifice_chamber     (礁洞深处)   — 骨骸 + 露西证物
  └── deep_elder_sighting   (礁洞)      — 直接目击

Red herring
  └── marisa_exhusband (酒馆)            — 玛丽莎前夫旧仇，无关案情
```

**冗余分布**（Three Clue Rule）：
- "Lucy 已死" → sacrifice_chamber + lucy_diary + cloth_scrap（3 条）
- "30 年献祭模式" → parish_record + empty_graves + 老渔民闲聊（3 条，1 由 GM 即兴）
- "执行者身份"（vance_executes）→ doctor_visits_log + clinic_supplies + ledger（3 条）
- "下一个受害者" → anna_warning + clinic_supplies + 玩家与 Anna 的多次对话

## 三幕节奏

### 第一幕（1–4 回合）：抵港委托
- 渡船到码头 → 公告栏（blood_letter）→ 巡警所（罗克 + 海莲娜）

### 第二幕（5–14 回合）：镇内调查
玩家在 4–5 个调查点之间穿梭。Tier 1→2 的转折点通常是夜访酒馆（pub_after_dark）或与
神父建立关系（church_records_unlocked）。

强制紧张拐点 A：在范斯诊所翻找时被范斯回来撞见 → `潜行` 或 `话术` 检定。失败：rourke
关系 –10、ledger 暂被销毁需另寻。

时间压力：
- `turn_ge:8 + 未找 rourke_bribe` → rourke 关系 –10（rourke_warns）
- `turn_ge:12 + helena 未死` → helena 关系 –10（helena_despairs）

### 第三幕（15–25 回合）：礁洞对决
- 触发条件：`lighthouse_storm` fired + `turn_ge: 14` → `reef_cave_open`
- 拐点 B 礁洞下行：`攀爬` 或 `闪避`，失败 1d6 物理 + 灯熄延迟
- 礁洞内 SAN 高烈度损失：reef_carvings (1/1d6)、sacrifice_chamber (1d3/1d10)、deep_elder_sighting (1/1d6)
- 拐点 C 礁洞坍塌：`运动` 检定，失败把 success 结局降级为 flee_with_truth

## 结局判定表（默认 variant）

| 结局 | 类型 | 触发条件（核心） |
|---|---|---|
| `pact_broken` | 完美成功 | solved 条件 + church_records_unlocked + father_calvin ≥15 |
| `solved` | 标准成功 | sacrifice_chamber + reef_carvings + ledger + culprit_confronted |
| `flee_with_truth` | 部分成功 | Tier1+2 主线证据齐 + 未访 reef_cave |
| `victim_dies` | 失败 | helena 死 OR **anna 死** OR turn_ge:25 且 lucy_diary 未找 |
| `dismissed` | 失败 | rourke 关系 ≤ –20 |
| 系统级死亡页 | 失败（不入 YAML） | 调查员 HP/SAN 归零 by orchestrator |

## SAN 节点（共 8）

1. blood_letter（0/1）
2. lucy_diary（0/1）
3. parish_record（1/1d3）
4. strange_chant（0/1d4）
5. reef_carvings（1/1d6）
6. deep_elder_sighting（1/1d6）
7. sacrifice_chamber（1d3/1d10）
8. 战斗中目睹 NPC 死亡（GM 自由触发，1/1d6）

## 三个 Variant（v0.3.1 角色站位轮换）

设计原则：variant **不改世界**——三十年献祭契约、7 例历史祭品、深潜者存在、礁洞
仪式跨 variant 一致。variant 只改"当代执行链条里谁在主动、谁被骗、谁挣扎倒戈"。
这避免了 v0.3.0 的世界一致性崩溃（"为什么 marisa 复仇案里礁洞还有 7 具骨骸"）。

### `vance_executes`（weight 2，默认）
当代执行者是范斯医生。罗克受贿、calvin 守密、marisa 被胁、orin 是动摇中的眼线。

- `culprit_confronted`：clue_found doctor_visits_log + location_visited pub（默认）

### `calvin_directs`（weight 1）
当代精神主导是 calvin 神父。每代他"指引"镇上的医师"成为执行者"——范斯是被指引
的最近一位，自以为是医学伦理灰色地带，实际是教区多代传承的隐秘仪式。

- 共谋链条：calvin 主导 / vance 不知更高一环 / rourke 受贿（款项绕道堂会账户）
- `culprit_confronted`：clue_found parish_record + location_visited church
- 对峙时神父坦然不抗，第三天审讯室内被发现已自尽

### `rourke_runs`（weight 1）
当代主谋是罗克长官。他握有 vance 早年一桩医疗事故的把柄，胁迫范斯成为执行者。
calvin 反对但守密保留登记簿副本——他更愿意配合调查员（church 关系阈值降低）。

- 共谋链条：rourke 主谋 / vance 被胁 / calvin 反对 / marisa 被胁牌照 / orin 被胁旧账
- `culprit_confronted`：clue_found rourke_bribe + location_visited station
- `church_records_unlocked` 不要求 calvin 关系阈值（他主动配合）

## 跨周目 meta（runs/meta.json）

记录：
```json
{
  "completed_variants": ["vance_pact"],
  "completed_endings": ["pact_broken"],
  "discovered_truths": ["sacrifice_chamber", "deep_elder_sighting"],
  "play_count": 3
}
```

GM prompt 注入"玩家先验"段，允许的暗示（非剧透）：
- 长居者（神父 / 老渔民 / 守塔人）：偶尔愣神 1 拍、半句"我好像见过你"
- 结局页元叙述："你已活过 N 次"
- 不可让 NPC 直接说出上局真相，关系阈值仍按本局推动

## 重开性矩阵

| 维度 | 数量 | 组合贡献 |
|---|---|---|
| variant（真凶轮换） | 3 | × 3 主线 |
| 结局分支 | 5 | × 5 = 15 不同结局体验 |
| 关键词解锁的 NPC 隐藏知识 | 8 NPC × 平均 3 phrase entries | 玩家每局尝试不同语言路径 |
| 跨周目 meta 暗示 | NPC 在第 2/3 局对玩家有"似曾相识"反应 | 外层叙事循环 |

保守估算：3 variant × 平均 2 不同结局路径 ≈ **10–15 局新鲜感**
