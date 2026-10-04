# evals — 反幻觉题集与判分器

改检索、改提示词、改知识数据之前先跑这里。它测的是**答案文本**，不是检索质量；真实检索质量看 `internal/knowledge` 的召回门。

## 文件

| 文件 | 作用 |
|---|---|
| `main.go` | CLI。flags（:17-20）：`-questions`、`-answers`、`-online`、`-report`。退出码：加载/agent/答案文件失败 **exit 2**（:26、:36、:45），`Passed < Total` **exit 1**（:62-64），全过才 0 |
| `eval.go` | `Meta` / `Question` / `QuestionSet`、`LoadQuestionSet`、`RunOffline`、`FormatReport`。判分族见下 |
| `questions.json` | 中文题 **77**（其中 57 题有样例答案） |
| `sample_answers.json` | **57** 个答案键，覆盖 57/77；缺 20 个（`consumer-med-*`、`consumer-lab-*`、`consumer-daily-*`、`redflag-child-*`、`redflag-preg-*`、`redflag-adult-*`），无孤儿答案 |
| `questions_en.json` | MedQA 200 + PubMedQA 99 = **299** |
| `questions_cmb.json` / `questions_cmexam.json` | 各 **200** 中文单选题，靠 `Question.ExpectedOption` 字母判分，**只在 `-online` 模式有效** |
| `add_consumer_questions.py` | 幂等（按 id 去重）往 `questions.json` 追加消费者真实问法与红旗题 —— 那 20 个无样例答案的题就是它加进来的 |

## 判分族（`eval.go`）

禁用正则 :132-157 → 拒答 :160-173 → 要点覆盖 ≥ ceil(60%) :176-195 → `ExpectedOption` :198-211 → 引用 `[N]` :214-230（拒答题、emergency、mcq_en、pubmedqa_en 以及 ≤40 rune 的答案免引用）。

## 用法

```bash
go run ./evals                      # 离线：拿样例答案判分
go run ./evals -online              # 在线：每题真跑 agent（需 API key，很慢）
go run ./evals -questions evals/questions_cmb.json -online
go run ./evals -answers my.json -report out.json
```

## 两条如实记录

1. **离线默认跑现在必然非零退出**：实测 `go run ./evals` = 77 题 57 过、exit 1（20 题无样例答案 + 4 题过度拒答）。所以它**不能直接当 CI 门** —— `.github/workflows/ci.yml` 里也确实没有任何 evals 步骤。
2. 在线基线（2026-08-08）：中文 36 题通过率 72.2%，拒答 7/7 正确，模型 Zhipu glm-4-flash；英文 299 题与 Claude 基线未跑。

新增题目：写进对应 `questions*.json`（中文口语题保持中文，不要把领域内容翻成英文）。
