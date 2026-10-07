package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type runningServer struct {
	name string
	addr string
	cmd  *exec.Cmd
	log  *os.File
}

func (s *runningServer) Stop() {
	if s.cmd != nil && s.cmd.Process != nil {
		_ = s.cmd.Process.Kill()
		_, _ = s.cmd.Process.Wait()
	}

	if s.log != nil {
		_ = s.log.Close()
	}
}

func (s *runningServer) RSS() (uint64, error) {
	if s.cmd == nil || s.cmd.Process == nil {
		return 0, fmt.Errorf("server is not running")
	}

	return readRSS(s.cmd.Process.Pid)
}

func (s *runningServer) LogTail() string {
	if s.log == nil {
		return ""
	}

	data, err := os.ReadFile(s.log.Name())
	if err != nil {
		return ""
	}

	text := string(data)
	if len(text) > 2000 {
		text = text[len(text)-2000:]
	}

	return text
}

func readRSS(pid int) (uint64, error) {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/status", pid))
	if err != nil {
		return 0, err
	}

	for _, line := range strings.Split(string(data), "\n") {
		if !strings.HasPrefix(line, "VmRSS:") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 2 {
			return 0, fmt.Errorf("unexpected VmRSS line %q", line)
		}

		kb, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			return 0, err
		}

		return kb * 1024, nil
	}

	return 0, fmt.Errorf("VmRSS missing for pid %d", pid)
}

func startKairo(bin, addr string) (*runningServer, error) {
	logFile, err := os.CreateTemp("", "kairo-server-*.log")
	if err != nil {
		return nil, err
	}

	cmd := exec.Command(bin, "-addr", addr)
	cmd.Stdout = logFile
	cmd.Stderr = logFile

	server := &runningServer{name: "kairo", addr: addr, cmd: cmd, log: logFile}

	if err := cmd.Start(); err != nil {
		server.Stop()
		return nil, err
	}

	if err := waitReady(addr, 5*time.Second); err != nil {
		tail := server.LogTail()
		server.Stop()
		return nil, fmt.Errorf("%w\n%s", err, tail)
	}

	return server, nil
}

func startRedis(addr string) (*runningServer, error) {
	host, port, err := splitHostPort(addr)
	if err != nil {
		return nil, err
	}

	dir, err := os.MkdirTemp("", "kairo-redis-*")
	if err != nil {
		return nil, err
	}

	confPath := filepath.Join(dir, "redis.conf")
	conf := fmt.Sprintf(`bind %s
port %s
protected-mode no
daemonize no
appendonly no
save ""
dir %s
dbfilename dump.rdb
loglevel warning
io-threads 1
io-threads-do-reads no
`, host, port, dir)

	if err := os.WriteFile(confPath, []byte(conf), 0o644); err != nil {
		return nil, err
	}

	logFile, err := os.CreateTemp("", "kairo-redis-*.log")
	if err != nil {
		return nil, err
	}

	cmd := exec.Command("redis-server", confPath)
	cmd.Stdout = logFile
	cmd.Stderr = logFile

	server := &runningServer{name: "redis", addr: addr, cmd: cmd, log: logFile}

	if err := cmd.Start(); err != nil {
		server.Stop()
		return nil, err
	}

	if err := waitReady(addr, 5*time.Second); err != nil {
		tail := server.LogTail()
		server.Stop()
		return nil, fmt.Errorf("%w\n%s", err, tail)
	}

	return server, nil
}

func splitHostPort(addr string) (string, string, error) {
	host, port, ok := strings.Cut(addr, ":")
	if !ok || host == "" || port == "" {
		return "", "", fmt.Errorf("address %q must be host:port", addr)
	}

	return host, port, nil
}

func buildKairo(root string) (string, error) {
	dir, err := os.MkdirTemp("", "kairo-bin-*")
	if err != nil {
		return "", err
	}

	bin := filepath.Join(dir, "kairo")
	cmd := exec.Command("go", "build", "-o", bin, "./cmd/kairo")
	cmd.Dir = root
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("build kairo: %w", err)
	}

	return bin, nil
}
