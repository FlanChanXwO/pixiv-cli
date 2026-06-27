package cmd

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	surveyterminal "github.com/AlecAivazis/survey/v2/terminal"
	"golang.org/x/term"
)

var (
	canPrompt     = defaultCanPrompt
	promptInput   = defaultPromptInput
	promptSecret  = defaultPromptSecret
	promptSelect  = defaultPromptSelect
	promptConfirm = defaultPromptConfirm
)

func defaultCanPrompt(a app) bool {
	in, okIn := a.in.(surveyterminal.FileReader)
	out, okOut := a.out.(surveyterminal.FileWriter)
	if !okIn || !okOut {
		return false
	}
	return term.IsTerminal(int(in.Fd())) && term.IsTerminal(int(out.Fd()))
}

func surveyAskOptions(a app) ([]survey.AskOpt, error) {
	in, okIn := a.in.(surveyterminal.FileReader)
	out, okOut := a.out.(surveyterminal.FileWriter)
	if !okIn || !okOut {
		return nil, errors.New("interactive prompt is only available on a TTY")
	}
	return []survey.AskOpt{survey.WithStdio(in, out, a.errOut)}, nil
}

func defaultPromptInput(a app, message, defaultValue string) (string, error) {
	opts, err := surveyAskOptions(a)
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

func defaultPromptSecret(a app, message string) (string, error) {
	opts, err := surveyAskOptions(a)
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

func defaultPromptSelect(a app, message string, options []string) (string, error) {
	if len(options) == 0 {
		return "", errors.New("no available options")
	}
	opts, err := surveyAskOptions(a)
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

func defaultPromptConfirm(a app, message string, defaultValue bool) (bool, error) {
	opts, err := surveyAskOptions(a)
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
