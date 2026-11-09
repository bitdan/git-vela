package app

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"

	"golang.org/x/sys/windows"
)

func openProjectLocation(path string, folder bool, action string) error {
	system := filepath.Join(os.Getenv("SystemRoot"), "System32")
	var command *exec.Cmd
	switch action {
	case "explorer":
		args := []string{path}
		if !folder {
			args = []string{"/select," + path}
		}
		command = exec.Command(filepath.Join(os.Getenv("SystemRoot"), "explorer.exe"), args...)
	case "terminal":
		if !folder {
			path = filepath.Dir(path)
		}
		if terminal, err := exec.LookPath("wt.exe"); err == nil {
			command = exec.Command(terminal, "-d", path)
		} else {
			command = exec.Command(filepath.Join(system, "cmd.exe"), "/D", "/K")
			command.Dir = path
			command.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NEW_CONSOLE}
		}
	default:
		return errors.New("unknown project location action")
	}
	if err := command.Start(); err != nil {
		return err
	}
	return command.Process.Release()
}
