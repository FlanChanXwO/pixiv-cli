package pipeline

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	record "github.com/FlanChanXwO/pixiv-cli/internal/shared/record"
	"github.com/FlanChanXwO/pixiv-cli/internal/utils/parse"
)

// PipelineDiagnosticError 表示错误已经按管道契约写入 stderr。它让根命令保留
// 非零退出状态，同时不再附加普通的 "error:" 文本破坏逐行诊断。
type PipelineDiagnosticError struct{}

func (*PipelineDiagnosticError) Error() string { return "pipeline records failed" }

type recordDiagnostic struct {
	Kind      string `json:"kind"`
	Operation string `json:"operation"`
	Line      int64  `json:"line"`
	ID        string `json:"id,omitempty"`
	Type      string `json:"type,omitempty"`
	Code      string `json:"code"`
	Message   string `json:"message"`
}

// recordActionError 让动作处理器用稳定 code 标记一条已解析记录的失败原因。
// 原始错误仅写入诊断 message，避免伪造成功或吞掉远端失败。
type recordActionError struct {
	code string
	err  error
}

func (e *recordActionError) Error() string { return e.err.Error() }
func (e *recordActionError) Unwrap() error { return e.err }

func NewRecordActionError(code string, err error) error {
	return &recordActionError{code: code, err: err}
}

// fatalRecordPipelineError 表示 stdout/stderr I/O 等无法以单条诊断恢复的失败。
// 这类错误必须原样返回，不能误报成一条输入记录的动作失败。
type fatalRecordPipelineError struct{ err error }

func (e *fatalRecordPipelineError) Error() string { return e.err.Error() }
func (e *fatalRecordPipelineError) Unwrap() error { return e.err }

func FatalRecordPipeline(err error) error { return &fatalRecordPipelineError{err: err} }

// consumeNDJSONRecords 逐行解析 NDJSON 并顺序处理记录。它不使用 Scanner，避免
// 合法的大记录受隐藏 token 限制；每次回调结束后才读取下一行，以便取消后不启动新任务。
func ConsumeNDJSONRecords(ctx context.Context, in io.Reader, errOut io.Writer, operation string, failFast bool, consume func(context.Context, record.Record) error) error {
	reader := bufio.NewReader(in)
	lineNumber := int64(0)
	failed := false
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		line, readErr := reader.ReadBytes('\n')
		// 非 EOF 读取错误表示该次 ReadBytes 返回的数据不具备完整流边界；在有
		// 副作用的动作命令中绝不能把这些字节当成可执行记录。EOF 则允许携带
		// 最后一行合法 NDJSON，仍按正常路径处理。
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			return fmt.Errorf("read NDJSON input: %w", readErr)
		}
		if len(line) > 0 {
			lineNumber++
			parsedRecord, recordErr := record.ParseRecordJSON(bytes.TrimSpace(line))
			if recordErr != nil {
				failed = true
				if err := writeRecordDiagnostic(errOut, operation, lineNumber, line, "invalid_record", recordErr); err != nil {
					return err
				}
				if failFast {
					return &PipelineDiagnosticError{}
				}
			} else if err := consume(ctx, parsedRecord); err != nil {
				if ctxErr := ctx.Err(); ctxErr != nil {
					return ctxErr
				}
				var fatalErr *fatalRecordPipelineError
				if errors.As(err, &fatalErr) {
					return fatalErr.err
				}
				failed = true
				code := "action_failed"
				var actionErr *recordActionError
				if errors.As(err, &actionErr) && actionErr.code != "" {
					code = actionErr.code
				}
				if err := writeRecordDiagnostic(errOut, operation, lineNumber, line, code, err); err != nil {
					return err
				}
				if failFast {
					return &PipelineDiagnosticError{}
				}
			}
		}
		if readErr == nil {
			continue
		}
		// 非 nil 的非 EOF 错误已在处理 line 前返回；剩余情况仅为 EOF。
		break
	}
	if failed {
		return &PipelineDiagnosticError{}
	}
	return nil
}

func RecordFailureStrategy(value string) (bool, error) {
	switch value {
	case "skip":
		return false, nil
	case "fail-fast":
		return true, nil
	default:
		return false, errors.New("on-error must be one of: skip, fail-fast")
	}
}

func writeRecordDiagnostic(out io.Writer, operation string, lineNumber int64, line []byte, code string, cause error) error {
	diagnostic := recordDiagnostic{
		Kind:      "record_error",
		Operation: operation,
		Line:      lineNumber,
		Code:      code,
		Message:   cause.Error(),
	}
	diagnostic.ID, diagnostic.Type = diagnosticRecordIdentity(line)
	return json.NewEncoder(out).Encode(diagnostic)
}

func diagnosticRecordIdentity(line []byte) (string, string) {
	decoder := json.NewDecoder(bytes.NewReader(line))
	decoder.UseNumber()
	var fields map[string]any
	if err := decoder.Decode(&fields); err != nil {
		return "", ""
	}
	var id string
	switch value := fields["id"].(type) {
	case string:
		id = value
	case json.Number:
		id = value.String()
	}
	typ, _ := fields["type"].(string)
	return id, typ
}

func RequiredRecordID(value record.Record) (int64, error) {
	id, err := parse.PositiveInt64(value.ID(), "record id")
	if err != nil {
		return 0, fmt.Errorf("record id must be a positive integer: %w", err)
	}
	return id, nil
}
