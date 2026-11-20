// hoshi-eval 是 fork 的按鍵層級評測：把語料的按鍵逐鍵送進一個獨立的 0c-server（官方或 fork 建置），
// 用真的神經模型，記下每一鍵後的畫面與耗時，再比較兩次執行。說明見 hoshi/eval/README.md。
//
//	hoshi-eval keys   -data DIR -in TEXT.tsv -out CASES.tsv      中文句（含英數）轉成按鍵
//	hoshi-eval run    -server EXE -data DIR -out RUN.json CASES.tsv...
//	hoshi-eval report -new RUN.json [-base RUN.json] -out REPORT.md
package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	var err error
	switch os.Args[1] {
	case "keys":
		err = keysCommand(os.Args[2:])
	case "run":
		err = runCommand(os.Args[2:])
	case "report":
		err = reportCommand(os.Args[2:])
	default:
		usage()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "hoshi-eval:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "用法：hoshi-eval keys|run|report …（-h 看各自的參數）")
	os.Exit(2)
}
