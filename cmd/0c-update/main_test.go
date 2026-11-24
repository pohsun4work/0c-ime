package main

import (
	"os"
	"testing"
)

// 0.2.0 起引擎常駐對所有人生效（#120）：套用設定、登入、排程與安裝後都會叫起引擎。測試一律把啟動換成
// 不做事、把引擎的連線位置換到暫存目錄，go test 不會啟動或交接這台機器上真的引擎；
// 要測啟動流程的測試自己再換一次（engine_service_*_test.go）。
// 系統安裝的目錄也換到不存在的暫存位置：機器上裝了發行版套件（/usr/libexec/0c、/usr/share/0c）時，
// 測試不會讀到它（#239）；要測系統安裝的測試自己再設一次。
func TestMain(m *testing.M) {
	cleanup := isolateResidentEngine()
	startPrune = func(string) error { return nil } // 不在測試機上啟動真的背景清理（#179）
	code := m.Run()
	cleanup()
	os.Exit(code)
}
