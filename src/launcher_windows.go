//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

const createNewConsole = 0x00000010

type systemLauncher struct{}

func (systemLauncher) GUI(exe string, args []string, dir string, env []string) error {
	var cmd *exec.Cmd
	low := strings.ToLower(exe)
	if strings.HasSuffix(low, ".html") || strings.HasSuffix(low, ".htm") { // es. CyberChef: si apre nel browser, offline
		return shellOpen(0, exe, "")
	}
	if strings.HasSuffix(low, ".cmd") || strings.HasSuffix(low, ".bat") {
		cmd = exec.Command("cmd.exe", append([]string{"/c", exe}, args...)...)
		cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNewConsole}
	} else {
		cmd = exec.Command(exe, args...)
	}
	cmd.Dir, cmd.Env = dir, env
	return startDetached(cmd)
}

// Console apre una nuova finestra cmd.exe con lo strumento nel PATH e un esempio d'uso.
func (systemLauncher) Console(title, hint, dir string, pathDirs []string, env []string, scratch string) error {
	os.MkdirAll(scratch, 0o755)
	script := filepath.Join(scratch, "console-banner.cmd")
	var b strings.Builder
	b.WriteString("@echo off\r\n")
	fmt.Fprintf(&b, "title AFIP Portable - %s\r\n", cmdEscape(title))
	fmt.Fprintf(&b, "set \"PATH=%s;%%PATH%%\"\r\n", strings.Join(pathDirs, ";"))
	b.WriteString("echo.\r\n")
	fmt.Fprintf(&b, "echo  %s - avvio registrato nel registro AFIP Portable\r\n", cmdEscape(title))
	fmt.Fprintf(&b, "echo  Cartella di lavoro: %s\r\n", cmdEscape(dir))
	for _, h := range strings.Split(hint, "  |  ") {
		if h = strings.TrimSpace(h); h != "" {
			fmt.Fprintf(&b, "echo  Esempio: %s\r\n", cmdEscape(h))
		}
	}
	b.WriteString("echo.\r\n")
	if err := os.WriteFile(script, []byte(b.String()), 0o644); err != nil {
		return err
	}
	cmd := exec.Command("cmd.exe", "/k", script)
	cmd.Dir, cmd.Env = dir, env
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNewConsole}
	return startDetached(cmd)
}

func (systemLauncher) OpenURL(url string) error     { return shellOpen(0, url, "") }
func (systemLauncher) OpenFolder(path string) error { return shellOpen(0, path, "") }
