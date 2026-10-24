// Terminal prompts shared by every command owner.

package command

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"golang.org/x/term"
)

var errPromptNotTTY = errors.New("interactive prompt is only available on a TTY")

// terminalFD requires both streams to be terminals: prompts write to out, and a
// password is read from the terminal's file descriptor without echoing.
func terminalFD(in io.Reader, out io.Writer) (int, error) {
	reader, okIn := in.(interface{ Fd() uintptr })
	writer, okOut := out.(interface{ Fd() uintptr })
	if !okIn || !okOut {
		return 0, errPromptNotTTY
	}
	fd := int(reader.Fd())
	if !term.IsTerminal(fd) || !term.IsTerminal(int(writer.Fd())) {
		return 0, errPromptNotTTY
	}
	return fd, nil
}

// CanPrompt reports whether both input and output are terminals.
func (e Env) CanPrompt() bool {
	_, err := terminalFD(e.In, e.Out)
	return err == nil
}

func (e Env) PromptInput(message, defaultValue string) (string, error) {
	if _, err := terminalFD(e.In, e.Out); err != nil {
		return "", err
	}
	reader := bufio.NewReader(e.In)
	label := fmt.Sprintf("%s: ", message)
	if defaultValue != "" {
		label = fmt.Sprintf("%s (%s): ", message, defaultValue)
	}
	for {
		line, err := readLine(reader, e.Out, label)
		if err != nil {
			return "", err
		}
		value := line
		if value == "" {
			value = defaultValue
		}
		if value = strings.TrimSpace(value); value != "" {
			return value, nil
		}
		if err := writePrompt(e.Out, "value cannot be empty\n"); err != nil {
			return "", err
		}
	}
}

func (e Env) PromptSecret(message string) (string, error) {
	fd, err := terminalFD(e.In, e.Out)
	if err != nil {
		return "", err
	}
	for {
		if err := writePrompt(e.Out, "%s: ", message); err != nil {
			return "", err
		}
		raw, readErr := term.ReadPassword(fd)
		if err := writePrompt(e.Out, "\n"); err != nil {
			return "", err
		}
		if readErr != nil {
			return "", readErr
		}
		if value := strings.TrimSpace(string(raw)); value != "" {
			return value, nil
		}
		if err := writePrompt(e.Out, "value cannot be empty\n"); err != nil {
			return "", err
		}
	}
}

func (e Env) PromptSelect(message string, options []string) (string, error) {
	if len(options) == 0 {
		return "", errors.New("no available options")
	}
	if _, err := terminalFD(e.In, e.Out); err != nil {
		return "", err
	}
	for i, option := range options {
		if err := writePrompt(e.Out, "  %d) %s\n", i+1, option); err != nil {
			return "", err
		}
	}
	reader := bufio.NewReader(e.In)
	label := fmt.Sprintf("%s [1-%d]: ", message, len(options))
	for {
		line, err := readLine(reader, e.Out, label)
		if err != nil {
			return "", err
		}
		if n, convErr := strconv.Atoi(line); convErr == nil && n >= 1 && n <= len(options) {
			return strings.TrimSpace(options[n-1]), nil
		}
		if err := writePrompt(e.Out, "enter a number between 1 and %d\n", len(options)); err != nil {
			return "", err
		}
	}
}

func (e Env) PromptConfirm(message string, defaultValue bool) (bool, error) {
	if _, err := terminalFD(e.In, e.Out); err != nil {
		return false, err
	}
	reader := bufio.NewReader(e.In)
	label := fmt.Sprintf("%s (Y/n): ", message)
	if !defaultValue {
		label = fmt.Sprintf("%s (y/N): ", message)
	}
	for {
		line, err := readLine(reader, e.Out, label)
		if err != nil {
			return false, err
		}
		switch strings.ToLower(line) {
		case "":
			return defaultValue, nil
		case "y", "yes":
			return true, nil
		case "n", "no":
			return false, nil
		}
		if err := writePrompt(e.Out, "%q is not a valid answer, please try again.\n", line); err != nil {
			return false, err
		}
	}
}

// readLine returns one line without its terminator. A final line that ends at
// EOF is still an answer, matching a terminal's last line before close.
func readLine(reader *bufio.Reader, w io.Writer, prompt string) (string, error) {
	if err := writePrompt(w, "%s", prompt); err != nil {
		return "", err
	}
	line, err := reader.ReadString('\n')
	if err != nil && !(errors.Is(err, io.EOF) && line != "") {
		return "", err
	}
	line = strings.TrimSuffix(line, "\n")
	return strings.TrimSuffix(line, "\r"), nil
}

func writePrompt(w io.Writer, format string, args ...any) error {
	_, err := fmt.Fprintf(w, format, args...)
	return err
}
