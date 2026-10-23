package auth

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	surveyterminal "github.com/AlecAivazis/survey/v2/terminal"
	"golang.org/x/term"
)

// 以下终端提示实现只依赖调用方给出的流。命令自身通过注入的 Deps 使用提示能力，
// 因此整个 CLI 只有 composition root 一处替换点，不需要包级可变 seam。

func terminalCanPrompt(in io.Reader, out io.Writer) bool {
	reader, okIn := in.(surveyterminal.FileReader)
	writer, okOut := out.(surveyterminal.FileWriter)
	if !okIn || !okOut {
		return false
	}
	return term.IsTerminal(int(reader.Fd())) && term.IsTerminal(int(writer.Fd()))
}

func surveyAskOptions(in io.Reader, out, errOut io.Writer) ([]survey.AskOpt, error) {
	reader, okIn := in.(surveyterminal.FileReader)
	writer, okOut := out.(surveyterminal.FileWriter)
	if !okIn || !okOut {
		return nil, errors.New("interactive prompt is only available on a TTY")
	}
	return []survey.AskOpt{survey.WithStdio(reader, writer, errOut)}, nil
}

func terminalPromptInput(in io.Reader, out, errOut io.Writer, message, defaultValue string) (string, error) {
	opts, err := surveyAskOptions(in, out, errOut)
	if err != nil {
		return "", err
	}
	value := ""
	prompt := &survey.Input{Message: message, Default: defaultValue}
	if err := survey.AskOne(prompt, &value, append(opts, survey.WithValidator(nonEmptyAnswer))...); err != nil {
		return "", err
	}
	return strings.TrimSpace(value), nil
}

func terminalPromptSecret(in io.Reader, out, errOut io.Writer, message string) (string, error) {
	opts, err := surveyAskOptions(in, out, errOut)
	if err != nil {
		return "", err
	}
	value := ""
	prompt := &survey.Password{Message: message}
	if err := survey.AskOne(prompt, &value, append(opts, survey.WithValidator(nonEmptyAnswer))...); err != nil {
		return "", err
	}
	return strings.TrimSpace(value), nil
}

func terminalPromptSelect(in io.Reader, out, errOut io.Writer, message string, options []string) (string, error) {
	if len(options) == 0 {
		return "", errors.New("no available options")
	}
	opts, err := surveyAskOptions(in, out, errOut)
	if err != nil {
		return "", err
	}
	value := ""
	prompt := &survey.Select{Message: message, Options: options}
	if err := survey.AskOne(prompt, &value, opts...); err != nil {
		return "", err
	}
	return strings.TrimSpace(value), nil
}

func terminalPromptConfirm(in io.Reader, out, errOut io.Writer, message string, defaultValue bool) (bool, error) {
	opts, err := surveyAskOptions(in, out, errOut)
	if err != nil {
		return false, err
	}
	value := false
	prompt := &survey.Confirm{Message: message, Default: defaultValue}
	if err := survey.AskOne(prompt, &value, opts...); err != nil {
		return false, err
	}
	return value, nil
}

func nonEmptyAnswer(value any) error {
	text, ok := value.(string)
	if !ok {
		return fmt.Errorf("unexpected prompt value type %T", value)
	}
	if strings.TrimSpace(text) == "" {
		return errors.New("value cannot be empty")
	}
	return nil
}

func writePrompt(w io.Writer, format string, args ...any) {
	_, _ = fmt.Fprintf(w, format, args...)
}
