package main

import (
	"errors"
	"fmt"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/tools/internal/verificationpolicy"
)

// commandSource identifies where the effective verification commands came from.
// The hash intentionally does not include this value: identity is defined by the
// commands that actually run, not by the surface that declared them (§6).
type commandSource string

const (
	commandSourceNone    commandSource = "none"
	commandSourcePR      commandSource = "pr"
	commandSourceComment commandSource = "comment"
)

type effectiveCommands struct {
	Source   commandSource
	Commands verificationpolicy.Commands
	Hash     string
}

// testTrigger reports whether a comment asks for verification.
//
// §5.1 requires the trigger to be the first non-empty line, exactly "/test".
// Scanning the whole comment is forbidden because it would let unrelated text
// such as "please /test" or a later quotation start credentialed execution.
func testTrigger(comment string) bool {
	for _, raw := range strings.Split(comment, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		return line == "/test"
	}
	return false
}

// resolveEffectiveCommands computes the commands a /test run must execute.
//
// A comment that declares a commands block fully replaces the PR declaration;
// merging is forbidden (§5.3). Any declared-but-invalid override fails closed
// and never falls back to the PR body (§5.4). Because the override replaces the
// PR declaration rather than supplementing it, a declared override is resolved
// before the PR body is consulted, so a stale or invalid PR declaration cannot
// veto an otherwise valid override.
func resolveEffectiveCommands(prBody, commentBody string, whitelist *verificationpolicy.Whitelist, cliName string) (effectiveCommands, error) {
	if testTrigger(commentBody) {
		commentDeclared, commentCommands, commentErr := parseDeclaration(commentBody, whitelist, cliName)
		if commentErr != nil {
			return effectiveCommands{}, commentErr
		}
		if commentDeclared {
			return effectiveCommands{
				Source:   commandSourceComment,
				Commands: commentCommands,
				Hash:     hashCommands(commentCommands),
			}, nil
		}
	}

	prDeclared, prCommands, prErr := parseDeclaration(prBody, whitelist, cliName)
	if prErr != nil {
		// An invalid PR declaration can never become effective commands.
		return effectiveCommands{}, prErr
	}
	if !prDeclared {
		return effectiveCommands{Source: commandSourceNone}, nil
	}
	return effectiveCommands{
		Source:   commandSourcePR,
		Commands: prCommands,
		Hash:     hashCommands(prCommands),
	}, nil
}

// parseDeclaration reports whether a body declares a commands block and returns
// the validated commands. Declared-but-empty and declared-but-invalid bodies
// return an error so callers can fail closed rather than silently ignore them.
func parseDeclaration(body string, whitelist *verificationpolicy.Whitelist, cliName string) (bool, verificationpolicy.Commands, error) {
	clean := htmlComment.ReplaceAllString(body, "")
	blocks, err := commandBlocks(clean)
	if err != nil {
		return true, verificationpolicy.Commands{}, err
	}
	if len(blocks) == 0 {
		return false, verificationpolicy.Commands{}, nil
	}
	if len(blocks) > 1 {
		return true, verificationpolicy.Commands{}, errors.New("Only one fenced commands block is allowed.")
	}
	var commands verificationpolicy.Commands
	index := 0
	for _, raw := range strings.Split(blocks[0], "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		index++
		command, err := verificationpolicy.ParseLine(line)
		if err != nil {
			return true, verificationpolicy.Commands{}, fmt.Errorf("Command %d is invalid: %v", index, err)
		}
		if err := whitelist.ValidateStatic(command, cliName); err != nil {
			return true, verificationpolicy.Commands{}, fmt.Errorf("Command %d is not allowed: %v", index, err)
		}
		commands.Commands = append(commands.Commands, command)
	}
	if len(commands.Commands) == 0 {
		return true, verificationpolicy.Commands{}, errors.New("The commands block must contain at least one command.")
	}
	return true, commands, nil
}

func hashCommands(commands verificationpolicy.Commands) string {
	if len(commands.Commands) == 0 {
		return ""
	}
	return verificationpolicy.NormalizeAndHash(commands)
}
