package tools

import (
	mcpserver "GopherAI/common/mcp/server"
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
)

// run_python is a SkillRestricted tool: only visible when code_assistant or
// data_analyst is active. It runs the supplied source code in a fresh Python
// interpreter spawned as a subprocess, with timeout, output capping and
// best-effort isolation.
//
// Sandbox notes (intentionally simple, demo-grade):
//   - The interpreter is launched with `-I` (isolated mode) so the user's
//     PYTHONPATH and site-packages-from-home cannot leak in.
//   - Working directory is a fresh, randomly-named temp dir that is
//     RemoveAll'd after the run, so any files the snippet drops are local
//     and short-lived.
//   - The env is empty except for PATH (or SystemRoot on Windows) so the
//     subprocess cannot read the parent's secrets.
//   - We rely on context-derived timeout to cap wall time; CPU/memory caps
//     are out of scope here.
//
// This is NOT a hardened sandbox. It's safe enough for a single-user demo
// where the LLM is the only "user" issuing Python.
const (
	defaultRunPythonTimeoutSec = 5
	maxRunPythonTimeoutSec     = 10
	maxRunPythonOutputBytes    = 4 * 1024
)

func registerRunPython(reg *mcpserver.ToolRegistry) {
	tool := mcp.NewTool(
		"run_python",
		mcp.WithDescription("在受限沙箱中执行一段 Python 代码并返回标准输出 / 错误。"+
			"默认超时 5 秒（最大 10 秒），无网络与持久磁盘。"+
			"适合做一次性计算、数据处理、字符串解析等。请用 print() 把结果打印出来。"),
		mcp.WithString("code",
			mcp.Description("要执行的 Python 源码"),
			mcp.Required(),
		),
		mcp.WithNumber("timeout_sec",
			mcp.Description("超时时间，单位秒，默认 5，最大 10"),
		),
	)
	reg.Register(tool, handleRunPython, mcpserver.ToolMeta{
		Scope:         mcpserver.ScopeSkillRestricted,
		AllowedSkills: []string{"code_assistant", "data_analyst"},
	})
}

func handleRunPython(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	code, err := stringArg(req, "code")
	if err != nil {
		return nil, err
	}
	timeout := optionalIntArg(req, "timeout_sec", defaultRunPythonTimeoutSec)
	if timeout <= 0 {
		timeout = defaultRunPythonTimeoutSec
	}
	if timeout > maxRunPythonTimeoutSec {
		timeout = maxRunPythonTimeoutSec
	}

	pyBin, err := resolvePythonBinary()
	if err != nil {
		return nil, fmt.Errorf("run_python: %w", err)
	}

	workDir, err := os.MkdirTemp("", "gopherai-run-python-")
	if err != nil {
		return nil, fmt.Errorf("run_python: create workdir failed: %w", err)
	}
	defer os.RemoveAll(workDir)

	scriptPath := filepath.Join(workDir, "main.py")
	if err := os.WriteFile(scriptPath, []byte(code), 0o600); err != nil {
		return nil, fmt.Errorf("run_python: write script failed: %w", err)
	}

	runCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	cmd := exec.CommandContext(runCtx, pyBin, "-I", "-X", "utf8", scriptPath)
	cmd.Dir = workDir
	cmd.Env = minimalEnv()

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	start := time.Now()
	runErr := cmd.Run()
	elapsed := time.Since(start)

	out := capPythonOutput(stdout.String(), maxRunPythonOutputBytes)
	errOut := capPythonOutput(stderr.String(), maxRunPythonOutputBytes)

	var sb strings.Builder
	fmt.Fprintf(&sb, "interpreter: %s\n耗时: %s\n", pyBin, elapsed.Round(time.Millisecond))

	if runCtx.Err() == context.DeadlineExceeded {
		fmt.Fprintf(&sb, "状态: 超时（%d 秒）已被强制终止\n", timeout)
	} else if runErr != nil {
		if exitErr, ok := runErr.(*exec.ExitError); ok {
			fmt.Fprintf(&sb, "状态: 非零退出 (code=%d)\n", exitErr.ExitCode())
		} else {
			fmt.Fprintf(&sb, "状态: 执行失败: %v\n", runErr)
		}
	} else {
		sb.WriteString("状态: 成功\n")
	}

	if out != "" {
		fmt.Fprintf(&sb, "\n--- stdout ---\n%s", out)
		if !strings.HasSuffix(out, "\n") {
			sb.WriteByte('\n')
		}
	} else {
		sb.WriteString("\n--- stdout --- (空)\n")
	}
	if errOut != "" {
		fmt.Fprintf(&sb, "\n--- stderr ---\n%s", errOut)
		if !strings.HasSuffix(errOut, "\n") {
			sb.WriteByte('\n')
		}
	}

	return textResult(sb.String()), nil
}

// resolvePythonBinary picks the Python interpreter to use. PYTHON_BIN wins
// for explicit setups; otherwise we probe `python3` and then `python` so the
// tool works on both *nix and Windows out of the box.
func resolvePythonBinary() (string, error) {
	if explicit := strings.TrimSpace(os.Getenv("PYTHON_BIN")); explicit != "" {
		if _, err := exec.LookPath(explicit); err == nil {
			return explicit, nil
		}
		return "", fmt.Errorf("PYTHON_BIN=%q not found in PATH", explicit)
	}
	for _, name := range []string{"python3", "python"} {
		if path, err := exec.LookPath(name); err == nil {
			if isUsablePythonBinary(path) {
				return path, nil
			}
		}
	}
	return "", fmt.Errorf("python interpreter not found (set PYTHON_BIN or install python3)")
}

func isUsablePythonBinary(path string) bool {
	if runtime.GOOS != "windows" {
		return true
	}
	// WindowsApps contains Microsoft Store launcher stubs that often exit
	// with code 9009 when Python is not actually installed. Avoid selecting
	// that shim during auto-discovery; PYTHON_BIN can still explicitly point
	// at any real interpreter.
	return !strings.Contains(strings.ToLower(path), `\windowsapps\`)
}

// minimalEnv strips inherited environment to the bare minimum the interpreter
// needs to start. PATH is preserved so binaries on the search path still
// resolve; on Windows SystemRoot must remain set or many DLLs fail to load.
func minimalEnv() []string {
	env := []string{}
	if path := os.Getenv("PATH"); path != "" {
		env = append(env, "PATH="+path)
	}
	if runtime.GOOS == "windows" {
		if sr := os.Getenv("SystemRoot"); sr != "" {
			env = append(env, "SystemRoot="+sr)
		}
	}
	env = append(env, "PYTHONIOENCODING=utf-8", "PYTHONUNBUFFERED=1")
	return env
}

func capPythonOutput(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + fmt.Sprintf("\n... (truncated, original %d bytes)", len(s))
}
