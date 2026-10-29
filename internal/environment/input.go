package environment

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// Codec 指明命令显式声明的 stdin 输入协议。
type Codec string

const (
	// NoInput 保留 stdin 给 MCP 等命令自有协议。
	NoInput Codec = "none"
	// TextValue 把整个 stdin 流映射成一个位置参数值。
	TextValue Codec = "text-value"
	// TextOrRecord 区分单个原始值与标准 NDJSON 记录流。
	TextOrRecord Codec = "text-or-record"
)

// Mode 记录 TextOrRecord 解析后得到文本还是记录。
type Mode string

const (
	// TextMode 表示 stdin 被解析成一个文本值。
	TextMode Mode = "text"
	// RecordMode 表示 stdin 被解析成 NDJSON 记录流。
	RecordMode Mode = "record"
)

// InputSpec 描述一条 Cobra 命令的显式输入契约。
//
// FillPosition 指定 stdin 填入的位置参数下标（从 0 开始）。只有一个必填或可选值的命令
// 通常填 0；"config set KEY [VALUE]" 这类命令填 1，保留显式的 KEY 作为第一个参数，只读取
// 省略的 VALUE。MaxArgs 为 -1 表示参数数量可变。
type InputSpec struct {
	Codec        Codec
	MinArgs      int
	MaxArgs      int
	FillPosition int
	Reader       io.Reader
	UsageError   func(error) error
	Enabled      func(*cobra.Command, []string) bool
	IsTTY        func(io.Reader) bool
}

// AnnotationKey 让架构检查确认命令声明了输入 codec，不需要解析 Use 字符串或
// 从帮助文本推断参数个数。
const AnnotationKey = "pixiv-cli.input-spec"

// State 保存一条命令实例解析后的输入。命令构造函数持有它；不按 *cobra.Command 建索引，
// 也不在多次执行之间共享。
type State struct {
	resolved *resolvedInput
}

type resolvedInput struct {
	args   []string
	reader io.Reader
	mode   Mode
}

// Args 返回 stdin 解析后的位置参数；解析尚未发生时返回调用方的切片，
// 例如测试直接调用 RunE 的场景。
func (s *State) Args(fallback []string) []string {
	if s == nil || s.resolved == nil {
		return fallback
	}
	return s.resolved.args
}

// Reader 返回 codec 选定的可重放 reader；不读取 stdin 的命令保留配置的 reader。
func (s *State) Reader(fallback io.Reader) io.Reader {
	if s == nil || s.resolved == nil || s.resolved.reader == nil {
		return fallback
	}
	return s.resolved.reader
}

// Mode 报告 TextOrRecord 流被判定为文本还是记录。
func (s *State) Mode() Mode {
	if s == nil || s.resolved == nil {
		return TextMode
	}
	return s.resolved.mode
}

// BindInput 把 InputSpec 挂到 cmd 上并返回本次命令实例的输入状态。它包装原有的位置参数校验和
// RunE，使业务代码看到解析后的参数切片，记录消费者则从 State.Reader 取得可重放 reader。
func BindInput(cmd *cobra.Command, spec InputSpec) *State {
	if cmd == nil {
		return nil
	}
	state := &State{}
	if cmd.Annotations == nil {
		cmd.Annotations = make(map[string]string)
	}
	cmd.Annotations[AnnotationKey] = annotationValue(spec)
	if spec.Codec == NoInput {
		// NoInput 只服务架构检查和命令树清单；MCP 等协议 owner 保留原来的 Args 和 RunE，
		// 通用解析器不会碰它们的输入流。
		return state
	}

	validate := cmd.Args
	if validate == nil {
		validate = cobra.ArbitraryArgs
	}
	run := cmd.RunE
	cmd.Args = func(command *cobra.Command, args []string) error {
		resolved, err := resolve(command, args, spec)
		if err != nil {
			return inputError(spec, err)
		}
		state.resolved = &resolved
		return validate(command, resolved.args)
	}
	if run != nil {
		cmd.RunE = func(command *cobra.Command, args []string) error {
			return run(command, state.Args(args))
		}
	}
	return state
}

func annotationValue(spec InputSpec) string {
	return string(spec.Codec) + ":" + strconv.Itoa(spec.MinArgs) + ":" + strconv.Itoa(spec.MaxArgs) + ":" + strconv.Itoa(spec.FillPosition)
}

func inputError(spec InputSpec, err error) error {
	if err == nil || spec.UsageError == nil {
		return err
	}
	return spec.UsageError(err)
}

func resolve(command *cobra.Command, args []string, spec InputSpec) (resolvedInput, error) {
	resolved := resolvedInput{
		args:   append([]string(nil), args...),
		reader: spec.Reader,
		mode:   modeForCodec(spec.Codec),
	}
	if spec.Codec == NoInput || !enabled(command, spec, args) || !shouldRead(command, args, spec) {
		return resolved, nil
	}
	if spec.Reader == nil {
		return resolvedInput{}, errors.New("stdin input reader is not configured")
	}

	switch spec.Codec {
	case TextValue:
		value, err := ReadValue(spec.Reader)
		if err != nil {
			return resolvedInput{}, fmt.Errorf("read stdin value: %w", err)
		}
		if value != "" {
			resolved.args = append(resolved.args, value)
		}
		resolved.mode = TextMode
		resolved.reader = bytes.NewReader(nil)
		return resolved, nil
	case TextOrRecord:
		mode, reader, value, err := resolveTextOrRecord(spec.Reader)
		if err != nil {
			return resolvedInput{}, fmt.Errorf("read stdin input: %w", err)
		}
		resolved.mode = mode
		resolved.reader = reader
		if mode == TextMode && value != "" {
			resolved.args = append(resolved.args, value)
		}
		return resolved, nil
	default:
		return resolvedInput{}, fmt.Errorf("unsupported stdin codec %q", spec.Codec)
	}
}

func enabled(command *cobra.Command, spec InputSpec, args []string) bool {
	return spec.Enabled == nil || spec.Enabled(command, args)
}

func shouldRead(command *cobra.Command, args []string, spec InputSpec) bool {
	if len(args) != spec.FillPosition {
		return false
	}
	if spec.MaxArgs >= 0 && len(args) >= spec.MaxArgs {
		return false
	}
	// 缺少两个以上必填位置参数时保留 Cobra 原有的使用错误，不猜测 stdin 对应哪一个值。
	if spec.MinArgs > spec.FillPosition+1 {
		return false
	}
	checker := spec.IsTTY
	if checker == nil {
		checker = IsTTY
	}
	return !checker(spec.Reader)
}

func resolveTextOrRecord(reader io.Reader) (Mode, io.Reader, string, error) {
	prefix := make([]byte, 0, 1)
	for {
		var one [1]byte
		n, err := reader.Read(one[:])
		if n > 0 {
			prefix = append(prefix, one[0])
			if !isJSONWhitespace(one[0]) {
				if one[0] == '{' {
					return RecordMode, io.MultiReader(bytes.NewReader(prefix), reader), "", nil
				}
				body, readErr := io.ReadAll(io.MultiReader(bytes.NewReader(prefix), reader))
				if readErr != nil {
					return TextMode, nil, "", readErr
				}
				return TextMode, bytes.NewReader(nil), string(stripOneTrailingLineEnding(body)), nil
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return TextMode, bytes.NewReader(nil), string(stripOneTrailingLineEnding(prefix)), nil
			}
			return TextMode, nil, "", err
		}
		if n == 0 {
			continue
		}
	}
}

func modeForCodec(codec Codec) Mode {
	if codec == TextOrRecord {
		return TextMode
	}
	return TextMode
}

// IsTTY 只把真正的终端文件当作 TTY；内存和管道 reader 按非 TTY 处理，
// 让嵌入式调用方得到管道语义。
func IsTTY(reader io.Reader) bool {
	file, ok := reader.(*os.File)
	return ok && term.IsTerminal(int(file.Fd()))
}

// ReadValue 读完整个流并只去掉一个行尾 LF 或 CRLF；不去空白、不拆词、不设大小上限。
func ReadValue(reader io.Reader) (string, error) {
	if reader == nil {
		return "", errors.New("stdin input reader is nil")
	}
	body, err := io.ReadAll(reader)
	if err != nil {
		return "", err
	}
	return string(stripOneTrailingLineEnding(body)), nil
}

// FirstNonWhitespace 返回第一个非 JSON 空白的字节。记录 codec 只需要 JSON 定义的前导
// 空白集合；按字节判断可以避免解码或改写不透明的输入内容。
func FirstNonWhitespace(body []byte) (byte, bool) {
	for _, one := range body {
		if !isJSONWhitespace(one) {
			return one, true
		}
	}
	return 0, false
}

func isJSONWhitespace(one byte) bool {
	switch one {
	case ' ', '\t', '\n', '\r':
		return true
	default:
		return false
	}
}

// stripOneTrailingLineEnding 只去掉末尾的一个 LF 或 CRLF，其余字节原样保留。
func stripOneTrailingLineEnding(body []byte) []byte {
	if bytes.HasSuffix(body, []byte("\r\n")) {
		return body[:len(body)-2]
	}
	if bytes.HasSuffix(body, []byte("\n")) {
		return body[:len(body)-1]
	}
	return body
}
