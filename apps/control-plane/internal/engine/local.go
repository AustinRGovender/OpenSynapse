package engine

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
)

// localBackend runs k6 as a subprocess of the control plane.
type localBackend struct {
	cmd      *exec.Cmd
	dir      string
	api      string
	exited   chan struct{} // closed when the process has exited
	drained  chan struct{} // closed when its output has been read to the end
	exitCode int
}

func findFreePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

func (e *Engine) startLocal(rp *runProcess) (*localBackend, error) {
	dir, err := os.MkdirTemp("", "opensynapse-run-")
	if err != nil {
		return nil, fmt.Errorf("create run dir: %w", err)
	}
	scriptPath := filepath.Join(dir, "script.js")
	if err := os.WriteFile(scriptPath, []byte(rp.spec.Script), 0o600); err != nil {
		os.RemoveAll(dir)
		return nil, fmt.Errorf("write script: %w", err)
	}
	metricsPath := filepath.Join(dir, "metrics.json")

	apiPort, err := findFreePort()
	if err != nil {
		os.RemoveAll(dir)
		return nil, fmt.Errorf("allocate k6 API port: %w", err)
	}

	args := []string{
		"run",
		"--no-color",
		"--quiet",
		e.noSummaryFlag(),
		"--no-usage-report",
		fmt.Sprintf("--address=127.0.0.1:%d", apiPort),
		"--out", "json=" + metricsPath,
		scriptPath,
	}
	cmd := exec.Command(e.k6Path, args...)
	cmd.Dir = dir
	cmd.Env = baseEnv()
	for k, v := range runEnv(rp.spec) {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		os.RemoveAll(dir)
		return nil, fmt.Errorf("stderr pipe: %w", err)
	}
	cmd.Stdout = io.Discard

	b := &localBackend{
		cmd:     cmd,
		dir:     dir,
		api:     fmt.Sprintf("http://127.0.0.1:%d", apiPort),
		exited:  make(chan struct{}),
		drained: make(chan struct{}),
	}
	if err := cmd.Start(); err != nil {
		os.RemoveAll(dir)
		return nil, fmt.Errorf("start k6: %w", err)
	}
	rp.markStarted()

	go func() {
		s := bufio.NewScanner(stderr)
		s.Buffer(make([]byte, 64*1024), 1024*1024)
		for s.Scan() {
			rp.addLog(s.Text())
		}
	}()
	go func() {
		defer close(b.drained)
		b.tail(rp, metricsPath)
	}()
	go func() {
		if err := cmd.Wait(); err != nil {
			var exitErr *exec.ExitError
			if errors.As(err, &exitErr) {
				b.exitCode = exitErr.ExitCode()
			} else {
				b.exitCode = -1
			}
		}
		close(b.exited)
	}()
	return b, nil
}

// tail follows k6's JSON output file until the process has exited and the
// file is fully drained.
func (b *localBackend) tail(rp *runProcess, path string) {
	var f *os.File
	for {
		var err error
		if f, err = os.Open(path); err == nil {
			break
		}
		select {
		case <-b.exited:
			if f, err = os.Open(path); err != nil {
				return // k6 exited before producing output
			}
		case <-time.After(100 * time.Millisecond):
			continue
		}
		break
	}
	defer f.Close()

	r := bufio.NewReaderSize(f, 256*1024)
	var partial []byte
	exited := false
	for {
		chunk, err := r.ReadBytes('\n')
		partial = append(partial, chunk...)
		if err == nil {
			rp.ingest("", partial)
			partial = partial[:0]
			continue
		}
		if err != io.EOF {
			log.Printf("run %s: reading k6 output: %v", rp.spec.RunID, err)
			return
		}
		if exited {
			if len(bytes.TrimSpace(partial)) > 0 {
				rp.ingest("", partial)
			}
			return
		}
		select {
		case <-b.exited:
			exited = true // one more pass to drain what k6 flushed on exit
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func (b *localBackend) apis() []string { return []string{b.api} }

func (b *localBackend) wait() int {
	<-b.exited
	<-b.drained
	return b.exitCode
}

func (b *localBackend) interrupt() error {
	if runtime.GOOS == "windows" {
		return errors.New("interrupt is not supported on Windows")
	}
	return b.cmd.Process.Signal(os.Interrupt)
}

func (b *localBackend) kill() error { return b.cmd.Process.Kill() }

func (b *localBackend) cleanup() { os.RemoveAll(b.dir) }
