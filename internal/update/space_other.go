//go:build !windows

package update

import "errors"

// availableSpace 只有 Windows 實作。這個檔案只為了讓 cmd/0c-release（簽索引、續簽）能在 hoshi-release.yml
// 的 Linux runner 上建置；更新器本身只在 Windows 執行，其他平台下載時一律回報無法確認空間。
func availableSpace(string) (uint64, error) { return 0, errors.ErrUnsupported }
