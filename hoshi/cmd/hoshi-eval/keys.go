package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/0c-biggo/0c-ime/core/layout"
	"github.com/0c-biggo/0c-ime/core/lexicon"
	"github.com/0c-biggo/0c-ime/core/proto"
	"github.com/0c-biggo/0c-ime/core/zhuyin"
	"github.com/0c-biggo/0c-ime/internal/datadir"
	"github.com/0c-biggo/0c-ime/internal/eval"
)

// 輸入每行 id<TAB>文字[<TAB>備註]，# 開頭的行照抄；輸出 id、keys、want、note 四欄。
func keysCommand(args []string) error {
	fs := flag.NewFlagSet("keys", flag.ExitOnError)
	data := fs.String("data", "", "資料包目錄（標音用，例如已安裝版本的 data）")
	in := fs.String("in", "", "輸入檔")
	out := fs.String("out", "", "輸出的語料檔")
	fs.Parse(args)
	if *data == "" || *in == "" || *out == "" {
		return fmt.Errorf("要指定 -data、-in、-out")
	}
	eng, err := datadir.Open(*data)
	if err != nil {
		return err
	}
	rd := eval.NewReader(eng.Model.Lex.(*lexicon.Compact), eng.Model.LM)
	src, err := os.ReadFile(*in)
	if err != nil {
		return err
	}
	var b strings.Builder
	var bad []string
	for n, line := range strings.Split(strings.TrimRight(string(src), "\r\n"), "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" || strings.HasPrefix(line, "#") {
			b.WriteString(line + "\n")
			continue
		}
		cols := strings.Split(line, "\t")
		if len(cols) < 2 {
			return fmt.Errorf("%s:%d：要有 id 與文字", *in, n+1)
		}
		text := readText(cols[1])
		keys, err := textKeys(text, rd.Annotate)
		if err != nil {
			bad = append(bad, fmt.Sprintf("%s：%v", cols[0], err))
			continue
		}
		note := ""
		if len(cols) > 2 {
			note = cols[2]
		}
		fmt.Fprintf(&b, "%s\t%s\t%s\t%s\n", cols[0], formatKeys(keys), showSpaces(text), note)
	}
	if len(bad) > 0 {
		return fmt.Errorf("%d 句轉不出按鍵：\n  %s", len(bad), strings.Join(bad, "\n  "))
	}
	return writeNew(*out, b.String())
}

var ctrlPunctKey = func() map[rune]string {
	m := map[rune]string{}
	for _, c := range proto.CtrlPunctKeys {
		k := proto.CtrlPrefix + string(c)
		p, _ := proto.CtrlPunct(k)
		m[[]rune(p)[0]] = k
	}
	return m
}()

func textKeys(text string, annotate func(string) ([]zhuyin.Syllable, bool)) ([]string, error) {
	var keys []string
	rs := []rune(text)
	for i := 0; i < len(rs); {
		r := rs[i]
		if zhuyin.IsHan(r) {
			j := i
			for j < len(rs) && zhuyin.IsHan(rs[j]) {
				j++
			}
			syls, ok := annotate(string(rs[i:j]))
			if !ok {
				return nil, fmt.Errorf("「%s」字典標不出讀音", string(rs[i:j]))
			}
			for _, s := range syls {
				for _, k := range layout.Standard.Keys(s) {
					keys = append(keys, string(k))
				}
			}
			i = j
			continue
		}
		switch k, ok := ctrlPunctKey[r]; {
		case ok:
			keys = append(keys, k)
		case r == '{':
			return nil, errors.New("按鍵記法分不出 { 是字還是鍵名的開頭，這一句要手寫按鍵")
		case r >= 0x20 && r <= 0x7e:
			keys = append(keys, string(r))
		default:
			return nil, fmt.Errorf("不知道怎麼打 %q", r)
		}
		i++
	}
	return keys, nil
}

// writeNew 寫出新檔；檔案已存在時不蓋掉。
func writeNew(path, content string) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	_, err = f.WriteString(content)
	return errors.Join(err, f.Close())
}
