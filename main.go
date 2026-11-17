package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
)

// AppKit only works on the main thread.
func init() { runtime.LockOSThread() }

func main() {
	loadLang()
	flag.Usage = func() { fmt.Fprint(os.Stderr, tr("usage")) }
	size := flag.Float64("size", 4, "")
	fg := flag.Bool("fg", false, "")
	flag.Parse()

	switch flag.Arg(0) {
	case "":
	case "stop":
		stop()
		return
	case "status":
		if pid, ok := running(); ok {
			fmt.Printf(tr("alive"), pid)
		} else {
			fmt.Println(tr("absent"))
		}
		return
	case "autostart":
		on := flag.Arg(1) != "off"
		if err := setAutostart(on); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Println(tr("autostart") + onoff(on))
		return
	case "icon":
		// Internal command for make app: renders the icons for the .icns file.
		if err := exportIcons(flag.Arg(1)); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	default:
		flag.Usage()
		os.Exit(2)
	}

	if pid, ok := running(); ok {
		fmt.Printf(tr("already"), pid)
		return
	}

	if !*fg && os.Getenv("MANSUR_CHILD") == "" && !inBundle() {
		exe, err := os.Executable()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		cmd := exec.Command(exe, os.Args[1:]...)
		cmd.Env = append(os.Environ(), "MANSUR_CHILD=1")
		cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
		if err := cmd.Start(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Println(tr("moved_in"))
		return
	}

	writePid()
	runApp(*size, removePid)
}

func inBundle() bool {
	exe, _ := os.Executable()
	return strings.Contains(exe, ".app/Contents/MacOS/")
}

func pidPath() string {
	dir, err := os.UserCacheDir()
	if err != nil {
		dir = os.TempDir()
	}
	return filepath.Join(dir, "mansur", "mansur.pid")
}

func writePid() {
	_ = os.MkdirAll(filepath.Dir(pidPath()), 0o700)
	_ = os.WriteFile(pidPath(), []byte(strconv.Itoa(os.Getpid())), 0o600)
}

// removePid deletes the pid file only if it is ours: on restart the old Mansur
// exits after the new one has already written its pid.
func removePid() {
	if b, err := os.ReadFile(pidPath()); err == nil && strings.TrimSpace(string(b)) == strconv.Itoa(os.Getpid()) {
		_ = os.Remove(pidPath())
	}
}

func running() (int, bool) {
	b, err := os.ReadFile(pidPath())
	if err != nil {
		return 0, false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil || pid <= 1 || syscall.Kill(pid, 0) != nil {
		return 0, false
	}
	// A pid from a stale file may now belong to another program, so check that it really is Mansur.
	out, err := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "comm=").Output()
	if err != nil || filepath.Base(strings.TrimSpace(string(out))) != "mansur" {
		return 0, false
	}
	return pid, true
}

func stop() {
	pid, ok := running()
	if !ok {
		fmt.Println(tr("not_here"))
		return
	}
	if err := syscall.Kill(pid, syscall.SIGTERM); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	_ = os.Remove(pidPath())
	fmt.Println(tr("bye"))
}
