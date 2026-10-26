// PR metadata gate：校验模板与 commands 声明，并解析 /test 的有效命令。
// 命令白名单与验证执行在 policy.go、verify.go。
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"regexp"
	"strings"
)

var (
	headingChanges      = regexp.MustCompile("(?m)^##\\s+变更点\\s*/\\s*Changes\\s*$")
	headingVerification = regexp.MustCompile("(?m)^##\\s+验证步骤\\s*/\\s*Verification\\s*$")
	headingChecklist    = regexp.MustCompile("(?m)^##\\s+检查清单\\s*/\\s*Checklist\\s*$")
	uncheckedChecklist  = regexp.MustCompile("(?m)^\\s*-\\s*\\[\\s\\]\\s+")
	htmlComment         = regexp.MustCompile("(?s)<!--.*?-->")
)

type result struct {
	TemplateOK   bool
	TemplateDesc string
	CommandsOK   bool
	CommandsDesc string
	Commands     Commands
	Hash         string
}

// commandFence is the only fenced block prmeta recognises. Every other fence
// language (including the retired `test` fence) is ordinary Markdown.
const commandFence = "commands"

// htmlEscapedFence is the entity-encoded form of ```; only the commands
// language is rejected so callers cannot smuggle a declaration past detection.
const htmlEscapedFence = "&#96;&#96;&#96;"

func runMeta(args []string) {
	bodyPath := flag.String("body-file", "", "pull request body file")
	whitelistPath := flag.String("whitelist", "scripts/pr/command-whitelist.txt", "trusted command whitelist")
	cliName := flag.String("cli", "pixiv", "repository CLI command name")
	githubOutput := flag.String("github-output", "", "GitHub Actions output file")
	commandsOutput := flag.String("commands-output", "", "optional commands JSON file")
	flag.CommandLine.Parse(args)

	if *bodyPath == "" {
		fatal(errors.New("body-file is required"))
	}
	body, err := os.ReadFile(*bodyPath)
	if err != nil {
		fatal(err)
	}
	w, err := LoadWhitelist(*whitelistPath)
	if err != nil {
		fatal(fmt.Errorf("load whitelist: %w", err))
	}

	validation := validate(string(body), w, *cliName)
	if *commandsOutput != "" {
		encoded, err := json.Marshal(validation.Commands)
		if err != nil {
			fatal(err)
		}
		if err := os.WriteFile(*commandsOutput, append(encoded, '\n'), 0o600); err != nil {
			fatal(err)
		}
	}
	if err := writeGitHubOutput(*githubOutput, validation); err != nil {
		fatal(err)
	}
	fmt.Printf("template_ok=%t\ncommands_ok=%t\nverification_hash=%s\n",
		validation.TemplateOK, validation.CommandsOK, validation.Hash)
	if !validation.TemplateOK || !validation.CommandsOK {
		os.Exit(1)
	}
}

// runTrigger 只判定 /test 触发，不加载白名单：触发判定不能依赖策略配置。
func runTrigger(args []string) {
	commentPath := flag.String("comment-file", "", "trigger comment body file")
	githubOutput := flag.String("github-output", "", "GitHub Actions output file")
	flag.CommandLine.Parse(args)
	if *commentPath == "" {
		fatal(errors.New("comment-file is required with trigger"))
	}
	body, err := os.ReadFile(*commentPath)
	if err != nil {
		fatal(err)
	}
	trigger := testTrigger(string(body))
	if err := writeTriggerOutput(*githubOutput, trigger); err != nil {
		fatal(err)
	}
	fmt.Printf("trigger=%t\n", trigger)
}

// runResolve 解析 /test 的有效命令；无效 override 返回 resolve_ok=false 而不是退出，
// 让受信聚合器发布失败结果与 reaction，且绝不回落到 PR body。
func runResolve(args []string) {
	bodyPath := flag.String("body-file", "", "pull request body file")
	commentPath := flag.String("comment-file", "", "trigger comment body file")
	whitelistPath := flag.String("whitelist", "scripts/pr/command-whitelist.txt", "trusted command whitelist")
	cliName := flag.String("cli", "pixiv", "repository CLI command name")
	githubOutput := flag.String("github-output", "", "GitHub Actions output file")
	commandsOutput := flag.String("commands-output", "", "optional commands JSON file")
	flag.CommandLine.Parse(args)
	if *bodyPath == "" {
		fatal(errors.New("body-file is required"))
	}
	if *commentPath == "" {
		fatal(errors.New("comment-file is required with resolve"))
	}
	body, err := os.ReadFile(*bodyPath)
	if err != nil {
		fatal(err)
	}
	comment, err := os.ReadFile(*commentPath)
	if err != nil {
		fatal(err)
	}
	w, err := LoadWhitelist(*whitelistPath)
	if err != nil {
		fatal(fmt.Errorf("load whitelist: %w", err))
	}
	effective, err := resolveEffectiveCommands(string(body), string(comment), w, *cliName)
	overrideDeclared := commentDeclaresCommands(string(comment))
	if err != nil {
		if writeErr := writeResolveOutput(*githubOutput, effective, overrideDeclared, false, err.Error()); writeErr != nil {
			fatal(writeErr)
		}
		fmt.Printf("resolve_ok=false\nresolve_desc=%s\n", oneLine(err.Error()))
		return
	}
	if effective.Source != commandSourceNone && *commandsOutput != "" {
		encoded, err := json.Marshal(effective.Commands)
		if err != nil {
			fatal(err)
		}
		if err := os.WriteFile(*commandsOutput, append(encoded, '\n'), 0o600); err != nil {
			fatal(err)
		}
	}
	if err := writeResolveOutput(*githubOutput, effective, overrideDeclared, true, ""); err != nil {
		fatal(err)
	}
	fmt.Printf("effective_source=%s\neffective_hash=%s\neffective_count=%d\n",
		effective.Source, effective.Hash, len(effective.Commands.Commands))
}

// commentDeclaresCommands reports whether the comment carries a commands fence.
// It is used only for reporting; validation always goes through the single
// trusted parser.
func commentDeclaresCommands(commentBody string) bool {
	clean := htmlComment.ReplaceAllString(commentBody, "")
	blocks, err := commandBlocks(clean)
	if err != nil {
		return true
	}
	return len(blocks) > 0
}

func validate(body string, whitelist *Whitelist, cliName string) result {
	clean := htmlComment.ReplaceAllString(body, "")
	answer := result{TemplateOK: true, CommandsOK: true}
	var missing []string
	if !headingChanges.MatchString(clean) {
		missing = append(missing, "Changes")
	}
	if !headingVerification.MatchString(clean) {
		missing = append(missing, "Verification")
	}
	if !headingChecklist.MatchString(clean) {
		missing = append(missing, "Checklist")
	}
	if len(missing) > 0 {
		answer.TemplateOK = false
		answer.TemplateDesc = "Missing required PR section(s): " + strings.Join(missing, ", ") + "."
	} else if uncheckedChecklist.MatchString(clean) {
		answer.TemplateOK = false
		answer.TemplateDesc = "Complete all required checklist items."
	} else {
		answer.TemplateDesc = "Required PR sections and checklist are complete."
	}

	// A single trusted parser serves both the PR-body gate and comment overrides;
	// a second parser or policy for override handling could disagree with it.
	declared, commands, err := parseDeclaration(clean, whitelist, cliName)
	if err != nil {
		answer.CommandsOK = false
		answer.CommandsDesc = err.Error()
		return answer
	}
	if !declared {
		// Zero declared blocks is valid: consumers treat it as no default commands.
		answer.CommandsDesc = "No default verification commands declared."
		return answer
	}
	answer.Commands = commands
	answer.Hash = hashCommands(commands)
	answer.CommandsDesc = commandCountDescription(len(commands.Commands))
	return answer
}

// commandCountDescription renders the stable gate wording. Exactly one
// declared command reads in the singular; every other count is plural.
func commandCountDescription(count int) string {
	if count == 1 {
		return "1 default verification command accepted."
	}
	return fmt.Sprintf("%d default verification commands accepted.", count)
}

func commandBlocks(body string) ([]string, error) {
	lines := strings.Split(body, "\n")
	var blocks []string
	var current []string
	inTest := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !inTest {
			if strings.HasPrefix(trimmed, htmlEscapedFence) &&
				strings.TrimSpace(strings.TrimPrefix(trimmed, htmlEscapedFence)) == commandFence {
				return nil, errors.New("HTML-escaped commands fences are not supported")
			}
			if len(trimmed) >= 7 && trimmed[:3] == string([]byte{96, 96, 96}) && strings.TrimSpace(trimmed[3:]) == commandFence {
				inTest = true
				current = nil
			}
			continue
		}
		if trimmed == string([]byte{96, 96, 96}) {
			blocks = append(blocks, strings.Join(current, "\n"))
			inTest = false
			continue
		}
		current = append(current, line)
	}
	if inTest {
		return nil, errors.New("The commands block is not closed.")
	}
	return blocks, nil
}

func writeGitHubOutput(path string, validation result) error {
	if path == "" {
		return nil
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	commands, err := json.Marshal(validation.Commands)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(file,
		"template_ok=%t\ntemplate_desc=%s\ncommands_ok=%t\ncommands_desc=%s\nverification_hash=%s\ncommands=%s\ncommand_count=%d\n",
		validation.TemplateOK,
		oneLine(validation.TemplateDesc),
		validation.CommandsOK,
		oneLine(validation.CommandsDesc),
		validation.Hash,
		commands,
		len(validation.Commands.Commands),
	)
	return err
}

func oneLine(value string) string {
	return strings.NewReplacer("\r", " ", "\n", " ").Replace(value)
}

func writeTriggerOutput(path string, trigger bool) error {
	if path == "" {
		return nil
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = fmt.Fprintf(file, "trigger=%t\n", trigger)
	return err
}

// writeResolveOutput publishes the effective-command outcome. override_declared
// distinguishes "no comment declaration" from "declaration rejected", which the
// workflow needs in order to fail closed only when the user actually asked for
// an override; it is kept separate from the PR commands gate.
func writeResolveOutput(path string, effective effectiveCommands, overrideDeclared, ok bool, description string) error {
	if path == "" {
		return nil
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	commands, err := json.Marshal(effective.Commands)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(file,
		"resolve_ok=%t\nresolve_desc=%s\neffective_source=%s\neffective_hash=%s\neffective_count=%d\noverride_declared=%t\neffective_commands=%s\n",
		ok,
		oneLine(description),
		effective.Source,
		effective.Hash,
		len(effective.Commands.Commands),
		overrideDeclared,
		commands,
	)
	return err
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(2)
}
