package selfupdate

import (
	"fmt"
	"os"
	"os/exec"
)

// Spawn 以相同工作目录和参数启动新版本，父进程退出后由系统接管。
func Spawn(spec LaunchSpec) error {
	cmd := exec.Command(spec.Path, spec.Args...)
	cmd.Dir = spec.Dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动新版本失败: %w", err)
	}
	return cmd.Process.Release()
}
