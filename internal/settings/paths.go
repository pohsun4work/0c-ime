package settings

import "path/filepath"

// Path 回傳設定檔的路徑：個人詞庫旁的 settings.json，指定其他個人詞庫目錄時也跟著，隔離測試或自訂設定互不影響。
func Path(userStore string) string { return filepath.Join(filepath.Dir(userStore), FileName) }
