// Command prmeta validates the pull request template and optional verification
// block using the trusted repository policy.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/tools/internal/verificationpolicy"
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
	TestOK       bool
	TestDesc     string
	Commands     verificationpolicy.Commands
	Hash         string
}

func main() {
	bodyPath := flag.String("body-file", "", "pull request body file")
	whitelistPath := flag.String("whitelist", "tools/verification/command-whitelist.txt", "trusted command whitelist")
	cliName := flag.String("cli", "pixiv", "repository CLI command name")
	workspace := flag.String("workspace", ".", "workspace used to validate declared file operands")
	githubOutput := flag.String("github-output", "", "GitHub Actions output file")
	commandsOutput := flag.String("commands-output", "", "optional commands JSON file")
	flag.Parse()

	if *bodyPath == "" {
		fatal(errors.New("body-file is required"))
	}
	body, err := os.ReadFile(*bodyPath)
	if err != nil {
		fatal(err)
	}
	w, err := verificationpolicy.LoadWhitelist(*whitelistPath)
	if err != nil {
		fatal(fmt.Errorf("load whitelist: %w", err))
	}
	validation := validate(string(body), w, *cliName, *workspace)
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
	fmt.Printf("template_ok=%t\ntest_ok=%t\nverification_hash=%s\n",
		validation.TemplateOK, validation.TestOK, validation.Hash)
	if !validation.TemplateOK || !validation.TestOK {
		os.Exit(1)
	}
}

func validate(body string, whitelist *verificationpolicy.Whitelist, cliName, workspace string) result {
	clean := htmlComment.ReplaceAllString(body, "")
	answer := result{TemplateOK: true, TestOK: true}
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
		answer.TemplateDesc = "Missing required section(s): " + strings.Join(missing, ", ")
	} else if uncheckedChecklist.MatchString(clean) {
		answer.TemplateOK = false
		answer.TemplateDesc = "Complete every required checklist item."
	} else {
		answer.TemplateDesc = "PR template is complete."
	}

	blocks, err := testBlocks(clean)
	if err != nil {
		answer.TestOK = false
		answer.TestDesc = err.Error()
		return answer
	}
	if len(blocks) == 0 {
		answer.TestDesc = "No live verification commands declared."
		return answer
	}
	if len(blocks) > 1 {
		answer.TestOK = false
		answer.TestDesc = "Verification may contain at most one fenced test block."
		return answer
	}
	for lineNumber, raw := range strings.Split(blocks[0], "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		command, err := verificationpolicy.ParseLine(line)
		if err != nil {
			answer.TestOK = false
			answer.TestDesc = fmt.Sprintf("Verification line %d: %v", lineNumber+1, err)
			return answer
		}
		if err := whitelist.ValidateStatic(command, cliName); err != nil {
			answer.TestOK = false
			answer.TestDesc = fmt.Sprintf("Verification line %d: %v", lineNumber+1, err)
			return answer
		}
		answer.Commands.Commands = append(answer.Commands.Commands, command)
	}
	if len(answer.Commands.Commands) == 0 {
		answer.TestOK = false
		answer.TestDesc = "The fenced test block must contain at least one command."
		return answer
	}
	answer.Hash = verificationpolicy.NormalizeAndHash(answer.Commands)
	answer.TestDesc = fmt.Sprintf("%d verification command(s) accepted.", len(answer.Commands.Commands))
	return answer
}

func testBlocks(body string) ([]string, error) {
	lines := strings.Split(body, "\n")
	var blocks []string
	var current []string
	inTest := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !inTest {
			if strings.HasPrefix(trimmed, "&#96;&#96;&#96;") {
				return nil, errors.New("HTML-escaped verification fences are not supported")
			}
			if len(trimmed) >= 7 && trimmed[:3] == string([]byte{96, 96, 96}) && strings.TrimSpace(trimmed[3:]) == "test" {
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
		return nil, errors.New("Verification test fence is not closed.")
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
		"template_ok=%t\ntemplate_desc=%s\ntest_ok=%t\ntest_desc=%s\nverification_hash=%s\ncommands=%s\ncommand_count=%d\n",
		validation.TemplateOK,
		oneLine(validation.TemplateDesc),
		validation.TestOK,
		oneLine(validation.TestDesc),
		validation.Hash,
		commands,
		len(validation.Commands.Commands),
	)
	return err
}

func oneLine(value string) string {
	return strings.NewReplacer("\r", " ", "\n", " ").Replace(value)
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(2)
}
