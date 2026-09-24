// Package verificationpolicy parses and validates PR-declared verification
// commands without invoking a shell.
package verificationpolicy

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

type Stage struct {
	Argv []string `json:"argv"`
}

type Command struct {
	Display string  `json:"display"`
	Stages  []Stage `json:"stages"`
}

type Commands struct {
	Commands []Command `json:"commands"`
}

type commandRule struct {
	allowAll       bool
	workspaceFiles bool
	allowPaths     [][]string
	denyPaths      [][]string
}

type Whitelist struct {
	rules map[string]*commandRule
}

func LoadWhitelist(path string) (*Whitelist, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	w := &Whitelist{rules: map[string]*commandRule{}}
	scanner := bufio.NewScanner(file)
	lineNumber := 0
	seen := map[string]struct{}{}
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			return nil, fmt.Errorf("line %d: expected command and rule", lineNumber)
		}
		name := fields[0]
		if err := validateExecutableName(name); err != nil {
			return nil, fmt.Errorf("line %d: %w", lineNumber, err)
		}
		rule := w.rules[name]
		if rule == nil {
			rule = &commandRule{}
			w.rules[name] = rule
		}
		key := strings.Join(fields, "\x00")
		if _, ok := seen[key]; ok {
			return nil, fmt.Errorf("line %d: duplicate rule", lineNumber)
		}
		seen[key] = struct{}{}
		switch {
		case len(fields) == 2 && fields[1] == "*":
			rule.allowAll = true
		case len(fields) == 2 && fields[1] == "@":
			rule.workspaceFiles = true
		case strings.HasPrefix(fields[1], "!"):
			path := append([]string{strings.TrimPrefix(fields[1], "!")}, fields[2:]...)
			if err := validateRulePath(path); err != nil {
				return nil, fmt.Errorf("line %d: %w", lineNumber, err)
			}
			rule.denyPaths = append(rule.denyPaths, path)
		case strings.HasPrefix(fields[1], "+"):
			path := append([]string{strings.TrimPrefix(fields[1], "+")}, fields[2:]...)
			if err := validateRulePath(path); err != nil {
				return nil, fmt.Errorf("line %d: %w", lineNumber, err)
			}
			rule.allowPaths = append(rule.allowPaths, path)
		default:
			return nil, fmt.Errorf("line %d: unknown rule syntax", lineNumber)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return w, nil
}

func validateExecutableName(name string) error {
	if name == "" || filepath.IsAbs(name) || strings.ContainsAny(name, "/\\") {
		return fmt.Errorf("invalid command name %q", name)
	}
	return nil
}

func validateRulePath(path []string) error {
	if len(path) == 0 {
		return errors.New("empty subcommand path")
	}
	for _, part := range path {
		if part == "" || strings.HasPrefix(part, "-") {
			return fmt.Errorf("invalid subcommand token %q", part)
		}
	}
	return nil
}

func (w *Whitelist) Validate(command Command, repositoryCLI, workspace string) error {
	return w.validate(command, repositoryCLI, workspace, true)
}

// ValidateStatic validates syntax and lexical path containment without
// requiring PR files to exist in the trusted base checkout.
func (w *Whitelist) ValidateStatic(command Command, repositoryCLI string) error {
	return w.validate(command, repositoryCLI, ".", false)
}

func (w *Whitelist) validate(command Command, repositoryCLI, workspace string, strictPaths bool) error {
	if len(command.Stages) == 0 {
		return errors.New("command has no stages")
	}
	for _, stage := range command.Stages {
		if len(stage.Argv) == 0 {
			return errors.New("pipeline contains an empty stage")
		}
		name := stage.Argv[0]
		rule := w.rules[name]
		if rule == nil {
			return fmt.Errorf("command %q is not in the trusted whitelist", name)
		}
		if !rule.allowAll && !rule.workspaceFiles && len(rule.allowPaths) == 0 {
			return fmt.Errorf("command %q has no allow rule", name)
		}
		for _, denied := range rule.denyPaths {
			if argvHasPath(stage.Argv[1:], denied) {
				return fmt.Errorf("%s subcommand %q is not allowed", name, strings.Join(denied, " "))
			}
		}
		if !rule.allowAll && len(rule.allowPaths) > 0 {
			allowed := false
			for _, path := range rule.allowPaths {
				if argvHasPath(stage.Argv[1:], path) {
					allowed = true
					break
				}
			}
			if !allowed {
				return fmt.Errorf("%s subcommand is not explicitly allowed", name)
			}
		}
		if name == repositoryCLI {
			if err := validateRepositoryCLI(stage.Argv[1:], workspace, strictPaths); err != nil {
				return err
			}
			continue
		}
		if err := validateHelper(name, stage.Argv[1:], rule.workspaceFiles, workspace, strictPaths); err != nil {
			return err
		}
	}
	return nil
}

func argvHasPath(argv, path []string) bool {
	if len(argv) < len(path) {
		return false
	}
	for index, want := range path {
		if argv[index] != want {
			return false
		}
	}
	return true
}

func validateRepositoryCLI(argv []string, workspace string, strictPaths bool) error {
	for index := 0; index < len(argv); index++ {
		arg := argv[index]
		switch arg {
		case "--proxy":
			return fmt.Errorf("%s is not allowed in PR verification", arg)
		case "--refresh":
			return errors.New("--refresh mutates local state and is not allowed")
		case "-o", "--output", "-d", "--directory":
			if index+1 >= len(argv) {
				return fmt.Errorf("%s requires a path", arg)
			}
			index++
			if err := validateWorkspacePath(workspace, argv[index], false, strictPaths); err != nil {
				return fmt.Errorf("%s: %w", arg, err)
			}
		default:
			if strings.HasPrefix(arg, "--proxy=") {
				return errors.New("--proxy is not allowed in PR verification")
			}
			if strings.HasPrefix(arg, "--output=") {
				if err := validateWorkspacePath(workspace, strings.TrimPrefix(arg, "--output="), false, strictPaths); err != nil {
					return fmt.Errorf("--output: %w", err)
				}
			}
			if strings.HasPrefix(arg, "--directory=") {
				if err := validateWorkspacePath(workspace, strings.TrimPrefix(arg, "--directory="), false, strictPaths); err != nil {
					return fmt.Errorf("--directory: %w", err)
				}
			}
		}
	}
	return nil
}

func validateHelper(name string, argv []string, workspaceFiles bool, workspace string, strictPaths bool) error {
	switch name {
	case "cat":
		if !workspaceFiles || len(argv) == 0 {
			return errors.New("cat requires at least one workspace-local path")
		}
		for _, arg := range argv {
			if strings.HasPrefix(arg, "-") {
				return errors.New("cat options are not supported in verification")
			}
			if err := validateWorkspacePath(workspace, arg, true, strictPaths); err != nil {
				return fmt.Errorf("cat: %w", err)
			}
		}
		return nil
	case "echo", "printf":
		return nil
	case "jq":
		return validateJQ(argv)
	case "grep":
		return validateGrep(argv)
	case "head", "tail":
		return validateHeadTail(name, argv)
	case "wc":
		return validateFlagsOnly(name, argv, "lwmcL")
	case "sort":
		return validateFlagsOnly(name, argv, "rnufo")
	case "uniq":
		return validateFlagsOnly(name, argv, "cdui")
	case "cut":
		return validateCut(argv)
	case "tr":
		return validateTR(argv)
	default:
		return fmt.Errorf("whitelisted helper %q has no trusted validator", name)
	}
}

func validateJQ(argv []string) error {
	positional := 0
	for _, arg := range argv {
		if strings.HasPrefix(arg, "-") {
			for _, flag := range strings.TrimLeft(arg, "-") {
				if !strings.ContainsRune("ercMsS", flag) {
					return fmt.Errorf("jq option %q is not supported in verification", arg)
				}
			}
			continue
		}
		positional++
	}
	if positional != 1 {
		return errors.New("jq verification usage requires exactly one filter and stdin input")
	}
	return nil
}

func validateGrep(argv []string) error {
	positional := 0
	for _, arg := range argv {
		if strings.HasPrefix(arg, "-") && arg != "-" {
			for _, flag := range strings.TrimLeft(arg, "-") {
				if !strings.ContainsRune("EFivqnc", flag) {
					return fmt.Errorf("grep option %q is not supported in verification", arg)
				}
			}
			continue
		}
		positional++
	}
	if positional != 1 {
		return errors.New("grep verification usage requires exactly one pattern and stdin input")
	}
	return nil
}

func validateHeadTail(name string, argv []string) error {
	for index := 0; index < len(argv); index++ {
		arg := argv[index]
		if arg == "-n" || arg == "-c" {
			index++
			if index >= len(argv) || strings.HasPrefix(argv[index], "-") {
				return fmt.Errorf("%s %s requires a value", name, arg)
			}
			continue
		}
		if strings.HasPrefix(arg, "-") && len(arg) > 1 {
			continue
		}
		return fmt.Errorf("%s accepts stdin only in verification", name)
	}
	return nil
}

func validateFlagsOnly(name string, argv []string, allowed string) error {
	for _, arg := range argv {
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			return fmt.Errorf("%s accepts stdin only in verification", name)
		}
		for _, flag := range strings.TrimLeft(arg, "-") {
			if !strings.ContainsRune(allowed, flag) {
				return fmt.Errorf("%s option %q is not supported in verification", name, arg)
			}
		}
	}
	return nil
}

func validateCut(argv []string) error {
	for index := 0; index < len(argv); index++ {
		arg := argv[index]
		switch arg {
		case "-d", "-f", "-c", "-b":
			index++
			if index >= len(argv) {
				return fmt.Errorf("cut %s requires a value", arg)
			}
		case "-s":
		case "--complement":
		default:
			if strings.HasPrefix(arg, "--delimiter=") || strings.HasPrefix(arg, "--fields=") ||
				strings.HasPrefix(arg, "--characters=") || strings.HasPrefix(arg, "--bytes=") {
				continue
			}
			return fmt.Errorf("cut argument %q is not supported in verification", arg)
		}
	}
	return nil
}

func validateTR(argv []string) error {
	positional := 0
	for _, arg := range argv {
		if strings.HasPrefix(arg, "-") {
			for _, flag := range strings.TrimLeft(arg, "-") {
				if !strings.ContainsRune("cdst", flag) {
					return fmt.Errorf("tr option %q is not supported in verification", arg)
				}
			}
			continue
		}
		positional++
	}
	if positional < 1 || positional > 2 {
		return errors.New("tr verification usage requires one or two character sets and stdin input")
	}
	return nil
}

func validateWorkspacePath(workspace, raw string, mustExist, strict bool) error {
	if filepath.IsAbs(raw) {
		return errors.New("absolute paths are not allowed")
	}
	clean := filepath.Clean(raw)
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return errors.New("path escapes the verification workspace")
	}
	if !strict {
		return nil
	}
	if workspace == "" {
		workspace = "."
	}
	root, err := filepath.Abs(workspace)
	if err != nil {
		return err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return err
	}
	target := filepath.Join(root, clean)
	if mustExist {
		target, err = filepath.EvalSymlinks(target)
		if err != nil {
			return err
		}
	} else {
		parent := filepath.Dir(target)
		parent, err = filepath.EvalSymlinks(parent)
		if err != nil {
			return err
		}
		target = filepath.Join(parent, filepath.Base(target))
	}
	relative, err := filepath.Rel(root, target)
	if err != nil {
		return err
	}
	if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return errors.New("path escapes the verification workspace")
	}
	return nil
}

func ParseLine(line string) (Command, error) {
	var stages []Stage
	var argv []string
	var token strings.Builder
	inSingle := false
	inDouble := false
	escaped := false
	hasToken := false
	flushToken := func() {
		if hasToken {
			argv = append(argv, token.String())
			token.Reset()
			hasToken = false
		}
	}
	flushStage := func() error {
		flushToken()
		if len(argv) == 0 {
			return errors.New("pipeline contains an empty stage")
		}
		stages = append(stages, Stage{Argv: append([]string(nil), argv...)})
		argv = nil
		return nil
	}

	runes := []rune(strings.TrimSpace(line))
	for index := 0; index < len(runes); index++ {
		ch := runes[index]
		if escaped {
			token.WriteRune(ch)
			hasToken = true
			escaped = false
			continue
		}
		if ch == '\\' && !inSingle {
			escaped = true
			hasToken = true
			continue
		}
		if ch == '\'' && !inDouble {
			inSingle = !inSingle
			hasToken = true
			continue
		}
		if ch == '"' && !inSingle {
			inDouble = !inDouble
			hasToken = true
			continue
		}
		if !inSingle && !inDouble {
			if unicode.IsSpace(ch) {
				flushToken()
				continue
			}
			switch ch {
			case '|':
				if index+1 < len(runes) && runes[index+1] == '|' {
					return Command{}, errors.New("shell operator || is not allowed")
				}
				if err := flushStage(); err != nil {
					return Command{}, err
				}
				continue
			case ';', '&', '<', '>', '`':
				return Command{}, fmt.Errorf("shell metacharacter %q is not allowed", ch)
			case '$':
				if index+1 < len(runes) && runes[index+1] == '(' {
					return Command{}, errors.New("command substitution is not allowed")
				}
			}
		}
		token.WriteRune(ch)
		hasToken = true
	}
	if escaped || inSingle || inDouble {
		return Command{}, errors.New("unterminated quote or escape")
	}
	if err := flushStage(); err != nil {
		return Command{}, err
	}
	return Command{Display: strings.TrimSpace(line), Stages: stages}, nil
}

func NormalizeAndHash(commands Commands) string {
	var lines []string
	for _, command := range commands.Commands {
		var stages []string
		for _, stage := range command.Stages {
			stages = append(stages, strings.Join(stage.Argv, "\x1f"))
		}
		lines = append(lines, strings.Join(stages, "\x1e"))
	}
	sum := sha256.Sum256([]byte(strings.Join(lines, "\n")))
	return hex.EncodeToString(sum[:])
}
