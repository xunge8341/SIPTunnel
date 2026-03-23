package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"siptunnel/internal/testutil/netdegrade"
)

func main() {
	input := flag.String("input", "", "JSONL 文件，每行一个样本")
	tplPath := flag.String("template", "", "报告模板路径")
	outPath := flag.String("out", "", "输出 markdown 路径")
	flag.Parse()

	if strings.TrimSpace(*input) == "" || strings.TrimSpace(*tplPath) == "" || strings.TrimSpace(*outPath) == "" {
		fmt.Fprintln(os.Stderr, "usage: netemreport -input samples.jsonl -template template.md -out report.md")
		os.Exit(2)
	}

	samples, err := readSamples(*input)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read samples failed: %v\n", err)
		os.Exit(1)
	}
	templateText, err := os.ReadFile(*tplPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read template failed: %v\n", err)
		os.Exit(1)
	}

	summaries := netdegrade.Summarize(samples)
	rendered, err := netdegrade.Render(string(templateText), summaries)
	if err != nil {
		fmt.Fprintf(os.Stderr, "render report failed: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile(*outPath, []byte(rendered), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "write report failed: %v\n", err)
		os.Exit(1)
	}
}

func readSamples(path string) ([]netdegrade.Sample, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var out []netdegrade.Sample
	s := bufio.NewScanner(f)
	lineNo := 0
	for s.Scan() {
		lineNo++
		line := strings.TrimSpace(s.Text())
		if line == "" {
			continue
		}
		var sample netdegrade.Sample
		if err := json.Unmarshal([]byte(line), &sample); err != nil {
			return nil, fmt.Errorf("line %d: %w", lineNo, err)
		}
		out = append(out, sample)
	}
	if err := s.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
