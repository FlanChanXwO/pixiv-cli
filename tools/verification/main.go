// Command verification executes trusted, pre-validated PR verification
// pipelines and renders sanitized cross-platform results.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"html"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/tools/internal/verificationpolicy"
)

const outputLimit = 16 * 1024

type stageResult struct {
	Command    string `json:"command"`
	Status     string `json:"status"`
	ExitCode   int    `json:"exit_code"`
	DurationMS int64  `json:"duration_ms"`
	Stderr     string `json:"stderr,omitempty"`
	pipeClosed bool
}

type commandResult struct {
	Command    string        `json:"command"`
	Status     string        `json:"status"`
	ExitCode   int           `json:"exit_code"`
	DurationMS int64         `json:"duration_ms"`
	Stdout     string        `json:"stdout,omitempty"`
	Stages     []stageResult `json:"stages,omitempty"`
}

type platformResult struct {
	Platform            string          `json:"platform"`
	Commands            []commandResult `json:"commands,omitempty"`
	InfrastructureError string          `json:"infrastructure_error,omitempty"`
}

type expectedMatrix struct {
	Include []struct {
		GOOS     string `json:"goos"`
		GOARCH   string `json:"goarch"`
		Artifact string `json:"artifact"`
	} `json:"include"`
}

func main() {
	if len(os.Args) < 2 {
		fatal(errors.New("expected init, execute, or render"))
	}
	switch os.Args[1] {
	case "init":
		initResult(os.Args[2:])
	case "execute":
		executeResult(os.Args[2:])
	case "render":
		renderResults(os.Args[2:])
	default:
		fatal(fmt.Errorf("unknown mode %q", os.Args[1]))
	}
}

func initResult(args []string) {
	set := flag.NewFlagSet("init", flag.ExitOnError)
	platform := set.String("platform", "", "platform name")
	output := set.String("output", "", "result file")
	_ = set.Parse(args)
	if *platform == "" || *output == "" {
		fatal(errors.New("platform and output are required"))
	}
	writeResult(*output, platformResult{
		Platform:            *platform,
		InfrastructureError: "Verification job did not reach command execution.",
	})
}

func executeResult(args []string) {
	set := flag.NewFlagSet("execute", flag.ExitOnError)
	platform := set.String("platform", "", "platform name")
	binary := set.String("binary", "", "PR-built repository CLI")
	commandsJSON := set.String("commands-json", "", "validated commands JSON")
	workspace := set.String("workspace", ".", "verification workspace")
	whitelistPath := set.String("whitelist", "tools/verification/command-whitelist.txt", "trusted whitelist")
	output := set.String("output", "", "result file")
	timeout := set.Duration("timeout", 2*time.Minute, "timeout for each declared command")
	_ = set.Parse(args)
	if *platform == "" || *binary == "" || *commandsJSON == "" || *output == "" {
		fatal(errors.New("platform, binary, commands-json, and output are required"))
	}
	var commands verificationpolicy.Commands
	if err := json.Unmarshal([]byte(*commandsJSON), &commands); err != nil {
		writeResult(*output, platformResult{Platform: *platform, InfrastructureError: "Trusted command payload could not be decoded."})
		return
	}
	w, err := verificationpolicy.LoadWhitelist(*whitelistPath)
	if err != nil {
		writeResult(*output, platformResult{Platform: *platform, InfrastructureError: "Trusted command whitelist could not be loaded."})
		return
	}
	for _, command := range commands.Commands {
		if err := w.Validate(command, "pixiv", *workspace); err != nil {
			writeResult(*output, platformResult{Platform: *platform, InfrastructureError: "Declared verification no longer satisfies the trusted policy: " + err.Error()})
			return
		}
	}
	if err := preflight(commands, *binary); err != nil {
		writeResult(*output, platformResult{Platform: *platform, InfrastructureError: err.Error()})
		return
	}

	result := platformResult{Platform: *platform}
	for _, command := range commands.Commands {
		result.Commands = append(result.Commands, runPipeline(command, *binary, *workspace, *timeout))
	}
	writeResult(*output, result)
}

func preflight(commands verificationpolicy.Commands, binary string) error {
	info, err := os.Stat(binary)
	if err != nil || info.IsDir() {
		return errors.New("PR-built repository CLI is unavailable")
	}
	seen := map[string]struct{}{}
	for _, command := range commands.Commands {
		for _, stage := range command.Stages {
			name := stage.Argv[0]
			if name == "pixiv" || isBuiltin(name) {
				continue
			}
			if _, ok := seen[name]; ok {
				continue
			}
			seen[name] = struct{}{}
			if _, err := exec.LookPath(name); err != nil {
				return fmt.Errorf("Required verification tool is unavailable: %s", name)
			}
		}
	}
	return nil
}

func runPipeline(command verificationpolicy.Command, binary, workspace string, timeout time.Duration) commandResult {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	started := time.Now()
	results := make([]stageResult, len(command.Stages))
	pipes := make([]struct {
		reader *io.PipeReader
		writer *io.PipeWriter
	}, max(0, len(command.Stages)-1))
	for index := range pipes {
		pipes[index].reader, pipes[index].writer = io.Pipe()
	}
	var finalOutput cappedBuffer
	var wg sync.WaitGroup
	for index, stage := range command.Stages {
		index := index
		stage := stage
		var stdin io.Reader
		if index > 0 {
			stdin = pipes[index-1].reader
		}
		var stdout io.Writer = &finalOutput
		var closeWriter *io.PipeWriter
		if index < len(pipes) {
			stdout = pipes[index].writer
			closeWriter = pipes[index].writer
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			if index > 0 {
				defer pipes[index-1].reader.Close()
			}
			if closeWriter != nil {
				defer closeWriter.Close()
			}
			results[index] = runStage(ctx, stage, binary, workspace, stdin, stdout)
		}()
	}
	wg.Wait()
	normalizeDownstreamPipeClose(results)

	overallExit := 0
	status := "passed"
	for index := len(results) - 1; index >= 0; index-- {
		if results[index].ExitCode != 0 {
			overallExit = results[index].ExitCode
			status = "failed"
			break
		}
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		status = "failed"
		if overallExit == 0 {
			overallExit = 124
		}
	}
	return commandResult{
		Command:    command.Display,
		Status:     status,
		ExitCode:   overallExit,
		DurationMS: time.Since(started).Milliseconds(),
		Stdout:     sanitize(finalOutput.String()),
		Stages:     results,
	}
}

func runStage(ctx context.Context, stage verificationpolicy.Stage, binary, workspace string, stdin io.Reader, stdout io.Writer) stageResult {
	started := time.Now()
	result := stageResult{Command: strings.Join(stage.Argv, " "), Status: "passed"}
	var stderr cappedBuffer
	name := stage.Argv[0]
	args := append([]string(nil), stage.Argv[1:]...)
	var err error
	switch name {
	case "cat":
		err = builtinCat(workspace, args, stdout)
	case "echo":
		_, err = io.WriteString(stdout, strings.Join(args, " ")+"\n")
	case "printf":
		err = builtinPrintf(args, stdout)
	default:
		program := name
		if name == "pixiv" {
			program = binary
		}
		command := exec.CommandContext(ctx, program, args...)
		command.Dir = workspace
		command.Stdin = stdin
		command.Stdout = stdout
		command.Stderr = &stderr
		command.Env = safeEnvironment()
		err = command.Run()
	}
	result.DurationMS = time.Since(started).Milliseconds()
	result.Stderr = sanitize(stderr.String())
	if err != nil {
		result.Status = "failed"
		result.ExitCode = exitCode(err)
		if result.Stderr == "" {
			result.Stderr = sanitize(err.Error())
		}
		result.pipeClosed = errors.Is(err, io.ErrClosedPipe) ||
			(runtime.GOOS != "windows" && result.ExitCode == 141) ||
			strings.TrimSpace(result.Stderr) == "io: read/write on closed pipe"
	}
	return result
}

// normalizeDownstreamPipeClose 保留 pipefail，但允许 head 等成功消费者主动提前关闭输入。
// 只有右侧所有 stage 都成功时才规范化上游 SIGPIPE/closed-pipe；真实下游失败仍保持失败。
func normalizeDownstreamPipeClose(results []stageResult) {
	downstreamPassed := true
	for index := len(results) - 1; index >= 0; index-- {
		result := &results[index]
		if index < len(results)-1 && downstreamPassed && result.pipeClosed {
			result.Status = "passed"
			result.ExitCode = 0
		}
		if result.ExitCode != 0 {
			downstreamPassed = false
		}
	}
}

func isBuiltin(name string) bool {
	return name == "cat" || name == "echo" || name == "printf"
}

func builtinCat(workspace string, paths []string, out io.Writer) error {
	for _, raw := range paths {
		path := filepath.Join(workspace, filepath.Clean(raw))
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(out, file)
		closeErr := file.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
	}
	return nil
}

func builtinPrintf(args []string, out io.Writer) error {
	if len(args) == 0 {
		return nil
	}
	format := args[0]
	values := args[1:]
	var rendered strings.Builder
	valueIndex := 0
	for index := 0; index < len(format); index++ {
		ch := format[index]
		if ch == '\\' && index+1 < len(format) {
			index++
			switch format[index] {
			case 'n':
				rendered.WriteByte('\n')
			case 't':
				rendered.WriteByte('\t')
			case '\\':
				rendered.WriteByte('\\')
			default:
				return fmt.Errorf("printf escape \\%c is not supported", format[index])
			}
			continue
		}
		if ch == '%' && index+1 < len(format) {
			index++
			switch format[index] {
			case '%':
				rendered.WriteByte('%')
			case 's':
				if valueIndex >= len(values) {
					return errors.New("printf %s requires an argument")
				}
				rendered.WriteString(values[valueIndex])
				valueIndex++
			default:
				return fmt.Errorf("printf format %%%c is not supported", format[index])
			}
			continue
		}
		rendered.WriteByte(ch)
	}
	if valueIndex != len(values) {
		return errors.New("printf has unused arguments")
	}
	_, err := io.WriteString(out, rendered.String())
	return err
}

func safeEnvironment() []string {
	allowed := map[string]struct{}{
		"HOME": {}, "USERPROFILE": {}, "PATH": {}, "SystemRoot": {}, "WINDIR": {},
		"TEMP": {}, "TMP": {}, "TMPDIR": {}, "LANG": {}, "LC_ALL": {},
		"PIXIV_LOG_LEVEL": {}, "PIXIV_LOG_FORMAT": {},
	}
	var environment []string
	for _, item := range os.Environ() {
		key, _, ok := strings.Cut(item, "=")
		if !ok {
			continue
		}
		if _, ok := allowed[key]; ok {
			environment = append(environment, item)
		}
	}
	return environment
}

func exitCode(err error) int {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		code := exitErr.ExitCode()
		if code >= 0 {
			return code
		}
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return 124
	}
	return 1
}

type cappedBuffer struct {
	buffer    bytes.Buffer
	omitted   int
	maxLength int
}

func (b *cappedBuffer) Write(value []byte) (int, error) {
	limit := b.maxLength
	if limit == 0 {
		limit = outputLimit
	}
	remaining := limit - b.buffer.Len()
	if remaining > 0 {
		write := len(value)
		if write > remaining {
			write = remaining
		}
		_, _ = b.buffer.Write(value[:write])
		b.omitted += len(value) - write
	} else {
		b.omitted += len(value)
	}
	return len(value), nil
}

func (b *cappedBuffer) String() string {
	if b.omitted == 0 {
		return b.buffer.String()
	}
	return b.buffer.String() + fmt.Sprintf("\n... %d byte(s) omitted ...", b.omitted)
}

var (
	ansiPattern   = regexp.MustCompile("\\x1b\\[[0-9;?]*[ -/]*[@-~]")
	authPattern   = regexp.MustCompile("(?i)(authorization|cookie|password|refresh[_ -]?token)\\s*[:=]\\s*[^\\s]+")
	bearerPattern = regexp.MustCompile("(?i)bearer\\s+[A-Za-z0-9._~+/-]+")
	jwtPattern    = regexp.MustCompile("[A-Za-z0-9_-]{16,}\\.[A-Za-z0-9_-]{16,}\\.[A-Za-z0-9_-]{16,}")
)

func sanitize(value string) string {
	value = ansiPattern.ReplaceAllString(value, "")
	value = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == '\t' || r >= 0x20 {
			return r
		}
		return -1
	}, value)
	value = authPattern.ReplaceAllString(value, "$1=[REDACTED]")
	value = bearerPattern.ReplaceAllString(value, "Bearer [REDACTED]")
	value = jwtPattern.ReplaceAllString(value, "[REDACTED_TOKEN]")
	return strings.TrimSpace(value)
}

func writeResult(path string, result platformResult) {
	body, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		fatal(err)
	}
	if err := os.WriteFile(path, append(body, '\n'), 0o600); err != nil {
		fatal(err)
	}
}

func renderResults(args []string) {
	set := flag.NewFlagSet("render", flag.ExitOnError)
	resultsDir := set.String("results-dir", "", "directory containing platform JSON results")
	expectedJSON := set.String("expected-matrix-json", "", "trusted expected matrix JSON")
	commit := set.String("commit", "", "verified commit")
	runURL := set.String("run-url", "", "GitHub Actions run URL")
	output := set.String("output", "", "rendered Markdown path")
	githubOutput := set.String("github-output", "", "GitHub Actions output file")
	_ = set.Parse(args)
	if *resultsDir == "" || *expectedJSON == "" || *output == "" {
		fatal(errors.New("results-dir, expected-matrix-json, and output are required"))
	}
	var expected expectedMatrix
	if err := json.Unmarshal([]byte(*expectedJSON), &expected); err != nil {
		fatal(err)
	}
	results := map[string]platformResult{}
	entries, err := os.ReadDir(*resultsDir)
	if err != nil {
		fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		body, err := os.ReadFile(filepath.Join(*resultsDir, entry.Name()))
		if err != nil {
			continue
		}
		var result platformResult
		if json.Unmarshal(body, &result) == nil && result.Platform != "" {
			results[result.Platform] = result
		}
	}
	markdown, overall := render(expected, results, *commit, *runURL)
	if err := os.WriteFile(*output, []byte(markdown), 0o600); err != nil {
		fatal(err)
	}
	if *githubOutput != "" {
		file, err := os.OpenFile(*githubOutput, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o600)
		if err != nil {
			fatal(err)
		}
		defer file.Close()
		if _, err := fmt.Fprintf(file, "overall=%s\n", overall); err != nil {
			fatal(err)
		}
	}
}

func render(expected expectedMatrix, results map[string]platformResult, commit, runURL string) (string, string) {
	type row struct {
		Platform string
		Result   platformResult
		State    string
		Passed   int
		Failed   int
	}
	var rows []row
	overall := "passed"
	for _, item := range expected.Include {
		platform := item.GOOS + "/" + item.GOARCH
		result, ok := results[platform]
		if !ok {
			result = platformResult{Platform: platform, InfrastructureError: "No verification result artifact was produced."}
		}
		state := "✅ Passed"
		passed, failed := 0, 0
		if result.InfrastructureError != "" {
			state = "⚠️ Infrastructure failure"
			overall = "infrastructure"
		} else {
			for _, command := range result.Commands {
				if command.Status == "passed" {
					passed++
				} else {
					failed++
				}
			}
			if failed > 0 {
				state = "❌ Failed"
				if overall == "passed" {
					overall = "failed"
				}
			}
		}
		rows = append(rows, row{Platform: platform, Result: result, State: state, Passed: passed, Failed: failed})
	}

	var out strings.Builder
	out.WriteString("<!-- pr-test-result -->\n\n## Verification\n\n")
	if commit != "" {
		short := commit
		if len(short) > 7 {
			short = short[:7]
		}
		fmt.Fprintf(&out, "Commit: `%s`\n", html.EscapeString(short))
	}
	switch overall {
	case "passed":
		out.WriteString("Overall: ✅ Passed\n\n")
	case "failed":
		out.WriteString("Overall: ❌ Failed\n\n")
	default:
		out.WriteString("Overall: ⚠️ Infrastructure failure\n\n")
	}
	out.WriteString("| Platform | Result | Passed | Failed |\n| --- | --- | ---: | ---: |\n")
	for _, row := range rows {
		fmt.Fprintf(&out, "| %s | %s | %d | %d |\n", row.Platform, row.State, row.Passed, row.Failed)
	}
	for _, row := range rows {
		if row.State == "✅ Passed" {
			continue
		}
		fmt.Fprintf(&out, "\n### %s — %s\n\n", row.Platform, row.State)
		if row.Result.InfrastructureError != "" {
			fmt.Fprintf(&out, "<details>\n<summary>Execution result</summary>\n\n**Relevant output**\n\n<pre>%s</pre>\n\n</details>\n",
				html.EscapeString(row.Result.InfrastructureError))
			continue
		}
		for _, command := range row.Result.Commands {
			if command.Status == "passed" {
				continue
			}
			fmt.Fprintf(&out, "<code>%s</code> — ❌ Failed\n\n", html.EscapeString(command.Command))
			out.WriteString("<details>\n<summary>Execution result</summary>\n\n")
			fmt.Fprintf(&out, "**Exit code:** `%d`  \n**Duration:** `%s`\n\n",
				command.ExitCode, formatDuration(command.DurationMS))
			if len(command.Stages) > 1 {
				out.WriteString("**Pipeline**\n\n| Stage | Result | Exit code | Duration |\n| --- | --- | ---: | ---: |\n")
				for _, stage := range command.Stages {
					state := "✅ Passed"
					if stage.Status != "passed" {
						state = "❌ Failed"
					}
					fmt.Fprintf(&out, "| <code>%s</code> | %s | `%d` | `%s` |\n",
						html.EscapeString(stage.Command), state, stage.ExitCode, formatDuration(stage.DurationMS))
				}
				out.WriteString("\n")
			}
			relevant := command.Stdout
			for index := len(command.Stages) - 1; index >= 0; index-- {
				if command.Stages[index].Status != "passed" && command.Stages[index].Stderr != "" {
					relevant = command.Stages[index].Stderr
					break
				}
			}
			if relevant == "" {
				relevant = "The command failed without diagnostic output."
			}
			fmt.Fprintf(&out, "**Relevant output**\n\n<pre>%s</pre>\n\n</details>\n\n", html.EscapeString(relevant))
		}
	}
	if runURL != "" {
		fmt.Fprintf(&out, "\n[View full GitHub Actions logs](%s)\n", runURL)
	}
	return out.String(), overall
}

func formatDuration(milliseconds int64) string {
	if milliseconds < 1000 {
		return strconv.FormatInt(milliseconds, 10) + "ms"
	}
	return fmt.Sprintf("%.1fs", float64(milliseconds)/1000)
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(2)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
