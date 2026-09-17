package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/doctor-agent/internal/agent"
	"github.com/doctor-agent/internal/config"
)

const defaultSampleAnswers = "evals/sample_answers.json"

func main() {
	questionsPath := flag.String("questions", "evals/questions.json", "golden set JSON 路径")
	answersPath := flag.String("answers", "", "离线回答文件路径 ({\"question_id\": \"answer\"}); 省略时尝试 sample_answers.json")
	online := flag.Bool("online", false, "在线模式：真实调用 agent（需要 ANTHROPIC_API_KEY）")
	reportPath := flag.String("report", "", "将完整评测报告写入 JSON 文件")
	flag.Parse()

	qs, err := LoadQuestionSet(*questionsPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ %v\n", err)
		os.Exit(2)
	}
	fmt.Printf("评测集: %s v%s (%d 题)\n\n", qs.Meta.Name, qs.Meta.Version, len(qs.Questions))

	var report *Report
	if *online {
		cfg := config.Load()
		ag, err := agent.New(cfg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "❌ 初始化 agent 失败: %v\n", err)
			os.Exit(2)
		}
		fmt.Println("在线模式：正在逐题运行 agent（需要 API key，耗时较长）...")
		report = runOnline(qs, ag)
	} else {
		answers, err := loadAnswers(*answersPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "❌ %v\n", err)
			fmt.Fprintln(os.Stderr, "提示: 提供 -answers 文件，或使用 -online 模式真实调用 agent。")
			os.Exit(2)
		}
		report = RunOffline(qs, answers)
	}

	if *reportPath != "" {
		data, _ := json.MarshalIndent(report, "", "  ")
		if err := os.WriteFile(*reportPath, data, 0644); err != nil {
			fmt.Fprintf(os.Stderr, "⚠️ 写入报告失败: %v\n", err)
		} else {
			fmt.Printf("\n报告已写入 %s\n", *reportPath)
		}
	}

	fmt.Print(FormatReport(report))

	// Non-zero exit when any item fails, so CI can gate on it.
	if report.Passed < report.Total {
		os.Exit(1)
	}
}

// runOnline evaluates every question against the real agent pipeline.
func runOnline(qs *QuestionSet, ag *agent.Agent) *Report {
	report := &Report{}
	for _, q := range qs.Questions {
		sess := ag.GetOrCreateSession("eval-" + q.ID)
		resp, err := ag.ProcessMessage(context.Background(), sess, q.Question)
		answer := ""
		if err != nil {
			answer = fmt.Sprintf("(agent 错误: %v)", err)
		} else {
			answer = resp.Text
		}
		res := evaluate(q, answer, qs.DefaultMustNot)
		report.Total++
		if res.Passed {
			report.Passed++
		}
		accumulate(q, res, report)
		report.Items = append(report.Items, res)
	}
	return report
}

// loadAnswers reads the offline answer map, falling back to the sample file.
func loadAnswers(path string) (map[string]string, error) {
	if path == "" {
		if _, err := os.Stat(defaultSampleAnswers); err == nil {
			path = defaultSampleAnswers
		} else {
			return nil, fmt.Errorf("未提供 -answers 且不存在默认样例 %s", defaultSampleAnswers)
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取答案文件 %s: %w", path, err)
	}
	var answers map[string]string
	if err := json.Unmarshal(data, &answers); err != nil {
		return nil, fmt.Errorf("解析答案文件 %s: %w", path, err)
	}
	return answers, nil
}
