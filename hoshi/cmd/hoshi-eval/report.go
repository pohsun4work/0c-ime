package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/0c-biggo/0c-ime/internal/eval"
)

// budget 是每鍵比基準多花的上限（規格：每鍵最多比官方多 30 ms）。
const budget = 30 * time.Millisecond

func reportCommand(args []string) error {
	fs := flag.NewFlagSet("report", flag.ExitOnError)
	newPath := fs.String("new", "", "這次的結果 JSON")
	basePath := fs.String("base", "", "對照的結果 JSON（通常是官方基準）")
	out := fs.String("out", "", "報告（Markdown）")
	fs.Parse(args)
	if *newPath == "" || *out == "" {
		return fmt.Errorf("要指定 -new 與 -out")
	}
	cur, err := loadRun(*newPath)
	if err != nil {
		return err
	}
	var base *Run
	if *basePath != "" {
		b, err := loadRun(*basePath)
		if err != nil {
			return err
		}
		base = &b
	}
	return os.WriteFile(*out, []byte(renderReport(cur, base)), 0o644)
}

func loadRun(path string) (Run, error) {
	var r Run
	b, err := os.ReadFile(path)
	if err != nil {
		return r, err
	}
	if err := json.Unmarshal(b, &r); err != nil {
		return r, fmt.Errorf("%s：%w", path, err)
	}
	return r, nil
}

func (c CaseResult) ok() bool { return c.Got == c.Want }

func renderReport(cur Run, base *Run) string {
	var b strings.Builder
	p := func(format string, a ...any) { fmt.Fprintf(&b, format, a...) }
	runs := []Run{cur}
	if base == nil {
		p("# 評測：%s\n\n", cur.Label)
	} else {
		p("# 評測：%s（對照 %s）\n\n", cur.Label, base.Label)
		runs = append(runs, *base)
	}
	for _, r := range runs {
		p("- %s：引擎 %s，%s 跑，每句 %d 次；資料 %s（sha256 %.12s）\n", r.Label, r.Version,
			r.Started.Format("2006-01-02 15:04"), r.Repeat, filepath.Base(filepath.Dir(r.Data)), r.DataSHA)
		for _, n := range r.Neural {
			p("  - %s\n", n)
		}
	}
	p("\n「正確」是打完（不送出）畫面上的文字跟 want 完全一樣；␣ 是半形空白。")
	p("每鍵耗時是引擎處理那一鍵的時間（含神經選字），不含管道往返；每句跑好幾次時取每一鍵最快的一次。\n")
	if base != nil && base.Repeat != cur.Repeat {
		p("\n**兩次執行每句跑的次數不同（%d、%d），耗時比較會偏。**\n", cur.Repeat, base.Repeat)
	}

	var diffs []setDiff
	if base != nil {
		baseSets := map[string]SetResult{}
		for _, s := range base.Sets {
			baseSets[s.Name] = s
		}
		for _, s := range cur.Sets {
			diffs = append(diffs, diffSet(s, baseSets[s.Name]))
		}
	}

	p("\n## 總覽\n\n")
	if base == nil {
		p("| 語料 | 句數 | 正確 | 正確率 | 統計選字 | 重跑不同 |\n|---|---:|---:|---:|---:|---:|\n")
	} else {
		p("| 語料 | 句數 | 正確 | 正確率 | 對照正確率 | 變好 | 變差 | 跟對照不同 | 統計選字 | 重跑不同 |\n|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|\n")
	}
	for i, s := range cur.Sets {
		ok, fallback, unstable := 0, 0, 0
		for _, c := range s.Cases {
			if c.ok() {
				ok++
			}
			if c.Fallback != "" {
				fallback++
			}
			if c.Unstable != "" {
				unstable++
			}
		}
		name := s.Name + map[string]string{"baseline": "（要跟對照一樣）", "asus": "（華碩參考）"}[s.Match]
		p("| %s | %d | %d | %s |", name, len(s.Cases), ok, percent(ok, len(s.Cases)))
		if base != nil {
			d := diffs[i]
			p(" %s | %d | %d | %d |", percent(d.baseOK, d.paired), len(d.fixed), len(d.broken), len(d.changed))
		}
		p(" %d | %d |\n", fallback, unstable)
	}

	p("\n## 每鍵耗時（毫秒）\n\n")
	if base == nil {
		p("| 語料 | 送進引擎的鍵 | p50 | p95 | p99 | 最大 |\n|---|---:|---:|---:|---:|---:|\n")
	} else {
		p("| 語料 | 送進引擎的鍵 | p50 | p95 | p99 | 最大 | 比對照多 p95 | 比對照多最大 | 多超過 %d ms 的鍵 |\n|---|---:|---:|---:|---:|---:|---:|---:|---:|\n", budget.Milliseconds())
	}
	for i, s := range cur.Sets {
		var spent []time.Duration
		for _, c := range s.Cases {
			for _, st := range c.Steps {
				if st.Sent {
					spent = append(spent, time.Duration(st.EngineUS)*time.Microsecond)
				}
			}
		}
		p("| %s | %d | %s | %s | %s | %s |", s.Name, len(spent), ms(spent, 50), ms(spent, 95), ms(spent, 99), ms(spent, 100))
		if base != nil {
			extra := diffs[i].extra
			over := 0
			for _, v := range extra {
				if v > budget {
					over++
				}
			}
			p(" %s | %s | %d |", ms(extra, 95), ms(extra, 100), over)
		}
		p("\n")
	}

	for i, s := range cur.Sets {
		if base == nil {
			var bad []CaseResult
			for _, c := range s.Cases {
				if !c.ok() {
					bad = append(bad, c)
				}
			}
			caseTable(p, fmt.Sprintf("%s：錯 %d 句", s.Name, len(bad)), bad, nil)
			continue
		}
		d := diffs[i]
		caseTable(p, fmt.Sprintf("%s：變差 %d 句", s.Name, len(d.broken)), d.broken, d.baseGot)
		caseTable(p, fmt.Sprintf("%s：變好 %d 句", s.Name, len(d.fixed)), d.fixed, d.baseGot)
		if s.Match == "baseline" {
			caseTable(p, fmt.Sprintf("%s：跟對照不同 %d 句", s.Name, len(d.changed)), d.changed, d.baseGot)
		}
	}
	return b.String()
}

type setDiff struct {
	paired, baseOK         int
	fixed, broken, changed []CaseResult
	baseGot                map[string]string
	extra                  []time.Duration // 同一句、同一鍵（兩邊都送進引擎）這次比對照多花的時間
}

func diffSet(cur, base SetResult) setDiff {
	d := setDiff{baseGot: map[string]string{}}
	byID := map[string]CaseResult{}
	for _, c := range base.Cases {
		byID[c.ID] = c
	}
	for _, c := range cur.Cases {
		o, ok := byID[c.ID]
		if !ok {
			continue
		}
		d.paired++
		d.baseGot[c.ID] = o.Got
		if o.ok() {
			d.baseOK++
		}
		switch {
		case c.ok() && !o.ok():
			d.fixed = append(d.fixed, c)
		case !c.ok() && o.ok():
			d.broken = append(d.broken, c)
		}
		if c.Got != o.Got {
			d.changed = append(d.changed, c)
		}
		if len(o.Steps) == len(c.Steps) {
			for i, st := range c.Steps {
				if st.Sent && o.Steps[i].Sent {
					d.extra = append(d.extra, time.Duration(st.EngineUS-o.Steps[i].EngineUS)*time.Microsecond)
				}
			}
		}
	}
	return d
}

func caseTable(p func(string, ...any), title string, cases []CaseResult, baseGot map[string]string) {
	if len(cases) == 0 {
		return
	}
	p("\n<details><summary>%s</summary>\n\n", title)
	if baseGot == nil {
		p("| id | 按鍵 | 要 | 得到 | 備註 |\n|---|---|---|---|---|\n")
	} else {
		p("| id | 按鍵 | 要 | 這次 | 對照 | 備註 |\n|---|---|---|---|---|---|\n")
	}
	for _, c := range cases {
		row := []string{c.ID, "`" + c.Keys + "`", cell(c.Want), cell(c.Got)}
		if baseGot != nil {
			row = append(row, cell(baseGot[c.ID]))
		}
		row = append(row, cell(c.Note))
		p("| %s |\n", strings.Join(row, " | "))
	}
	p("\n</details>\n")
}

var cellEscape = strings.NewReplacer("|", `\|`, "\n", "⏎", " ", spaceMark)

func cell(s string) string { return cellEscape.Replace(s) }

func percent(n, total int) string {
	if total == 0 {
		return "—"
	}
	return fmt.Sprintf("%.1f%%", 100*float64(n)/float64(total))
}

// ms 是第 q 百分位，以毫秒顯示。
func ms(v []time.Duration, q float64) string {
	if len(v) == 0 {
		return "—"
	}
	return fmt.Sprintf("%.1f", float64(eval.Percentile(v, q))/float64(time.Millisecond))
}
