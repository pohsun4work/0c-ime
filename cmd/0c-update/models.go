package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"

	"github.com/0c-biggo/0c-ime/internal/autoupdate"
	"github.com/0c-biggo/0c-ime/internal/models"
	"github.com/0c-biggo/0c-ime/internal/neural"
	upstream "github.com/0c-biggo/0c-ime/models"
)

// 「模型」頁（#180）：安裝包內建的選字與補標點模型是哪一份（路徑、雜湊、Hugging Face 上游與授權）。只讀、不連網。

type modelView struct {
	Role       string `json:"role"`
	Label      string `json:"label"`
	State      string `json:"state"`
	Source     string `json:"source"`
	Version    string `json:"version"`
	Path       string `json:"path"`
	SHA256     string `json:"sha256"`
	Size       int64  `json:"size"`
	Repository string `json:"repository"`
	Revision   string `json:"revision"`
	URL        string `json:"url"`
	License    string `json:"license"`
	// Equivalent：本機模型不是 Hugging Face 那一份檔案，但權重相同、只差匯出資訊（models/equivalent.json，
	// 0.2.2 以前官網套件內建的那份）。上游資訊照樣附上。
	Equivalent bool `json:"equivalent"`
}

// modelsStatus 是 models status -json。Source 是 bundled（安裝包內建）或 none（找不到內建模型，統計選字）。
type modelsStatus struct {
	Schema int         `json:"schema"`
	OK     bool        `json:"ok"`
	Source string      `json:"source"`
	Models []modelView `json:"models"`
}

// modelsEnv 是 status 需要的環境；測試換成暫存目錄。
type modelsEnv struct {
	data string // 引擎用的字典；模型在 <data>/../neural
	lock []byte
	// equivalent 是 models/equivalent.json：與 models.lock 的 model.onnx 等價的雜湊。
	equivalent []byte
	dial       autoupdate.Dialer
}

func defaultModelsEnv() modelsEnv {
	return modelsEnv{data: dataPackDir(basePath()), lock: upstream.Lock, equivalent: upstream.Equivalent, dial: dialEngine}
}

func (e modelsEnv) status() (modelsStatus, error) {
	lock, err := models.ParseLock(e.lock)
	if err != nil {
		return modelsStatus{}, err
	}
	equivalent, _ := models.ParseEquivalents(e.equivalent) // 讀不到就只認 models.lock 本身
	bundled := neural.Find(e.data)
	st := modelsStatus{Schema: contractSchema, OK: true, Source: "none", Models: []modelView{}}
	if bundled.SelectSource == neural.SourceBundled {
		st.Source = neural.SourceBundled
	}
	engine := map[string]string{}
	if m := modelStatus(e.dial); m["state"] == "running" {
		engine[models.RoleSelect], _ = m["selection_state"].(string)
		engine[models.RolePunct], _ = m["punctuation_state"].(string)
	}
	for _, role := range models.Roles {
		v := modelView{Role: role, Label: models.Label(role), State: engine[role]}
		dir, found := bundled.SelectPackage, bundled.SelectSource == neural.SourceBundled
		if role == models.RolePunct {
			dir, found = bundled.PunctPackage, bundled.PunctSource == neural.SourceBundled
		}
		if found {
			v.Path, v.Source, v.SHA256 = dir, neural.SourceBundled, manifestModelSHA(dir)
			if fi, err := os.Stat(filepath.Join(dir, "model.onnx")); err == nil {
				v.Size = fi.Size()
			}
			// 本機的模型是 models.lock 記的上游版本，或與它等價的匯出時才附上 Hugging Face 的資訊；
			// 其他模型不冒用上游的網址。
			if match := lock.Match(role, v.SHA256, equivalent); match != models.MatchNone {
				m := lock.Models[role]
				v.Version, v.Repository, v.Revision, v.URL, v.License = m.Version, m.Repository, m.Revision, m.URL(), m.License
				v.Equivalent = match == models.MatchEquivalent
			}
		}
		st.Models = append(st.Models, v)
	}
	return st, nil
}

// manifestModelSHA 是模型 manifest 記的 model.onnx 雜湊（worker 載入時核對這個值；不必為了顯示讀 130 MB）。
func manifestModelSHA(dir string) string {
	var m struct {
		Files map[string]string `json:"files"`
	}
	if b, err := os.ReadFile(filepath.Join(dir, "manifest.json")); err == nil && json.Unmarshal(b, &m) == nil {
		return m.Files["model.onnx"]
	}
	return ""
}

func modelsUsage() error { return usageError("用法：0c-update models status [-json]") }

// modelsCommand 處理 0c-update models status。
func modelsCommand(args []string) (resultErr error) {
	if slices.ContainsFunc(args, isJSONFlag) {
		defer func() {
			if resultErr != nil && !errors.Is(resultErr, errReported) {
				resultErr = failJSON(os.Stdout, resultErr)
			}
		}()
	}
	if len(args) == 0 || args[0] != "status" {
		return modelsUsage()
	}
	flags := flag.NewFlagSet("models status", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	asJSON := flags.Bool("json", false, "")
	if err := flags.Parse(args[1:]); err != nil || flags.NArg() != 0 {
		return modelsUsage()
	}
	st, err := defaultModelsEnv().status()
	if err != nil {
		return err
	}
	if *asJSON {
		return emitJSON(os.Stdout, st)
	}
	printModels(os.Stdout, st)
	return nil
}

func printModels(w io.Writer, st modelsStatus) {
	if st.Source == neural.SourceBundled {
		fmt.Fprintln(w, "神經模型：安裝包內建")
	} else {
		fmt.Fprintln(w, "神經模型：找不到內建模型，使用統計選字")
	}
	for _, m := range st.Models {
		line := m.Label + "："
		if m.Path != "" {
			line += m.Path
		} else {
			line += "未安裝"
		}
		if m.URL != "" {
			line += "（" + m.URL + "，" + m.License + "）"
		}
		if m.Equivalent {
			line += "；權重與公開版相同，匯出資訊不同"
		}
		fmt.Fprintln(w, line)
	}
}
