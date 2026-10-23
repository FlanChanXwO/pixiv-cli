// Terminal prompts shared by every command owner.

package command

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	surveyterminal "github.com/AlecAivazis/survey/v2/terminal"
	"golang.org/x/term"
)

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

// CanPrompt reports whether both input and output are terminals.
func (e Env) CanPrompt() bool {
	return terminalCanPrompt(e.In, e.Out)
}

func (e Env) PromptInput(message, defaultValue string) (string, error) {
	opts, err := surveyAskOptions(e.In, e.Out, e.Err)
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

func (e Env) PromptSecret(message string) (string, error) {
	opts, err := surveyAskOptions(e.In, e.Out, e.Err)
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

func (e Env) PromptSelect(message string, options []string) (string, error) {
	if len(options) == 0 {
		return "", errors.New("no available options")
	}
	opts, err := surveyAskOptions(e.In, e.Out, e.Err)
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

func (e Env) PromptConfirm(message string, defaultValue bool) (bool, error) {
	opts, err := surveyAskOptions(e.In, e.Out, e.Err)
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
