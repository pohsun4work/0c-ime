package update

import (
	"bytes"
	"os"
)

// TrustsReleaseKeys 回報 path 這個更新器執行檔是否信任這一版的每一把發行金鑰。Go 把金鑰字串原樣放進執行檔，
// 官方的更新器只有官方金鑰，fork 用這個分辨留在電腦上的更新器是不是自己的。
func TrustsReleaseKeys(path string) (bool, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	for _, key := range releaseKeyText {
		if !bytes.Contains(b, []byte(key)) {
			return false, nil
		}
	}
	return true, nil
}
