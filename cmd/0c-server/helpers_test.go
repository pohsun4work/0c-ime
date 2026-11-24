package main

import (
	"bytes"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/Microsoft/go-winio"
	"golang.org/x/sys/windows"
)

var (
	buildEnv     []string // TestMain 換掉使用者目錄之前的環境：go build 要用原本的建置快取與模組目錄
	binDir       string   // TestMain 建立，測試結束時刪除
	buildOnce    sync.Once
	serverBinary string
	buildErr     error
	pipeSeq      atomic.Int64
)

// buildServer 每個測試行程只建置一次引擎。
func buildServer(t *testing.T) string {
	t.Helper()
	buildOnce.Do(func() {
		binary := filepath.Join(binDir, "0c-server.exe")
		cmd := exec.Command("go", "build", "-o", binary, ".")
		cmd.Env = buildEnv
		cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NO_WINDOW} // 測試行程沒有主控台時也不跳出視窗
		if out, err := cmd.CombinedOutput(); err != nil {
			buildErr = fmt.Errorf("%v\n%s", err, out)
			return
		}
		serverBinary = binary
	})
	if buildErr != nil {
		t.Fatal(buildErr)
	}
	return serverBinary
}

// testPipe 回傳這個測試行程專用、不會重複的管道名稱；測試一律明確指定 -socket，不碰使用者真正的管道。
func testPipe(name string) string {
	return fmt.Sprintf(`\\.\pipe\0c-test-%s-%d-%d`, name, os.Getpid(), pipeSeq.Add(1))
}

// dial 連上管道；管道不存在時立即失敗，所有執行個體都在忙時最多等 2 秒。
func dial(pipe string) (net.Conn, error) {
	timeout := 2 * time.Second
	return winio.DialPipe(pipe, &timeout)
}

// occupyPipe 在 pipe 上服務（接受連線就關掉），代替另一個正在服務的引擎，測試結束時關閉。winio 的管道
// 要有 Accept 在等才連得上，否則用戶端一直看到忙線。
func occupyPipe(t *testing.T, pipe string) {
	t.Helper()
	l, err := winio.ListenPipe(pipe, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()
}

func waitDial(t *testing.T, pipe string) {
	t.Helper()
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
		if c, err := dial(pipe); err == nil {
			c.Close()
			return
		}
	}
	t.Fatalf("連不上 %s", pipe)
}

// serverCommand 讓引擎自成一個行程群組、有自己不開視窗的主控台，stopServer 才能只對它送 Ctrl+Break。
func serverCommand(binary string, args ...string) *exec.Cmd {
	cmd := exec.Command(binary, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NEW_PROCESS_GROUP | windows.CREATE_NO_WINDOW}
	return cmd
}

// launch 啟動 cmd；測試結束時還在跑的引擎由 stopServer 收掉。
func launch(t *testing.T, cmd *exec.Cmd) {
	t.Helper()
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { stopServer(t, cmd) })
}

// startServer 啟動引擎；stderr 收進回傳的緩衝區，行程結束後才能讀。
func startServer(t *testing.T, binary string, args ...string) (*exec.Cmd, *bytes.Buffer) {
	t.Helper()
	cmd := serverCommand(binary, args...)
	var logs bytes.Buffer
	cmd.Stderr = &logs
	launch(t, cmd)
	return cmd, &logs
}

// stopServer 走結束訊號的路徑停掉引擎（Ctrl+Break 由 Go 轉成 SIGINT：存檔後結束）並等它結束；
// 已經結束的不再處理，可以重複呼叫。
func stopServer(t *testing.T, cmd *exec.Cmd) {
	t.Helper()
	if cmd.Process == nil || cmd.ProcessState != nil {
		return
	}
	sendErr := ctrlBreak(cmd.Process.Pid)
	if exited, _ := waitFor(cmd, 10*time.Second); !exited {
		t.Errorf("引擎收到 Ctrl+Break 後 10 秒內沒有結束，已強制結束（%v）", sendErr)
	}
}

// waitExit 等引擎自己結束；逾時就強制結束並判失敗。
func waitExit(t *testing.T, cmd *exec.Cmd, within time.Duration) error {
	t.Helper()
	exited, err := waitFor(cmd, within)
	if !exited {
		t.Fatalf("引擎 %v 內沒有結束", within)
	}
	return err
}

// waitFor 等 cmd 結束；逾時就強制結束，exited 回報 false。
func waitFor(cmd *exec.Cmd, within time.Duration) (exited bool, err error) {
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		return true, err
	case <-time.After(within):
		cmd.Process.Kill()
		<-done
		return false, nil
	}
}

const ctrlBreakEnv = "ZC_TEST_CTRL_BREAK_PID"

// ctrlBreak 對引擎送 Ctrl+Break。事件只能送給同一個主控台上的行程群組，引擎有自己的主控台（serverCommand），
// 所以由測試執行檔另開一個沒有主控台的子行程（ctrlBreakMain）附加上去再送；測試行程自己有沒有主控台、
// 輸出有沒有被導向都一樣，也不會送到跑測試的終端機。
func ctrlBreak(pid int) error {
	self, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.Command(self, "-test.run=^$")
	cmd.Env = append(os.Environ(), ctrlBreakEnv+"="+strconv.Itoa(pid))
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.DETACHED_PROCESS}
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%v %s", err, out)
	}
	return nil
}

func ctrlBreakMain(arg string) int {
	pid, err := strconv.ParseUint(arg, 10, 32)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if r, _, err := windows.NewLazySystemDLL("kernel32.dll").NewProc("AttachConsole").Call(uintptr(pid)); r == 0 {
		fmt.Fprintln(os.Stderr, "AttachConsole:", err)
		return 1
	}
	// 引擎剛好已經結束、群組不存在時，事件會送給這個主控台上的每個行程，包括自己。
	signal.Notify(make(chan os.Signal, 1), os.Interrupt)
	if err := windows.GenerateConsoleCtrlEvent(windows.CTRL_BREAK_EVENT, uint32(pid)); err != nil {
		fmt.Fprintln(os.Stderr, "GenerateConsoleCtrlEvent:", err)
		return 1
	}
	return 0
}
