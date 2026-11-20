package update

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTrustsReleaseKeys(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := TrustsReleaseKeys(self); err != nil || !ok {
		t.Fatalf("測試執行檔內嵌了發行金鑰，應該信任：ok=%v err=%v", ok, err)
	}
	other := filepath.Join(t.TempDir(), "0c-update.exe")
	if err := os.WriteFile(other, []byte("p8+04oez+UEucOQTdHmA7sjI1Pixh5Q4VtepvVBqlb0="), 0o600); err != nil {
		t.Fatal(err)
	}
	if ok, err := TrustsReleaseKeys(other); err != nil || ok {
		t.Fatalf("只有官方金鑰的檔案不該算信任：ok=%v err=%v", ok, err)
	}
	if _, err := TrustsReleaseKeys(filepath.Join(t.TempDir(), "missing.exe")); err == nil {
		t.Fatal("檔案不存在時應回報錯誤")
	}
}
