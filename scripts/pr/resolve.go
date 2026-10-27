package main

import (
	"errors"
	"fmt"
	"strings"
)

// commandSource 记录有效验证命令来自 PR 正文、评论还是缺失。命令身份摘要不含来源：
// 同一批命令无论写在哪里都对应同一次执行。
type commandSource string

const (
	commandSourceNone    commandSource = "none"
	commandSourcePR      commandSource = "pr"
	commandSourceComment commandSource = "comment"
)

type effectiveCommands struct {
	Source   commandSource
	Commands Commands
	Hash     string
}

// testTrigger 判断评论是否请求验证：首个非空行必须精确等于 /test。扫描整条评论会让
// “please /test”或后文引用中的 /test 也启动需要凭据的执行。
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

// resolveEffectiveCommands 计算一次 /test 必须执行的命令。评论里的 commands 块完全替换
// PR 声明，不做合并；声明了但无效的 override 直接失败，不回落到 PR 正文。override 先于
// PR 正文解析，因此陈旧或无效的 PR 声明不会否决有效的 override。
func resolveEffectiveCommands(prBody, commentBody string, whitelist *Whitelist, cliName string) (effectiveCommands, error) {
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
		// 无效的 PR 声明不能变成有效命令。
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

// parseDeclaration 返回正文是否声明了 commands 块以及校验后的命令。声明为空或无效都
// 返回错误，让调用方 fail closed，而不是静默忽略。
func parseDeclaration(body string, whitelist *Whitelist, cliName string) (bool, Commands, error) {
	clean := htmlComment.ReplaceAllString(body, "")
	blocks, err := commandBlocks(clean)
	if err != nil {
		return true, Commands{}, err
	}
	if len(blocks) == 0 {
		return false, Commands{}, nil
	}
	if len(blocks) > 1 {
		return true, Commands{}, errors.New("Only one fenced commands block is allowed.")
	}
	var commands Commands
	index := 0
	for _, raw := range strings.Split(blocks[0], "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		index++
		command, err := ParseLine(line)
		if err != nil {
			return true, Commands{}, fmt.Errorf("Command %d is invalid: %v", index, err)
		}
		if err := whitelist.ValidateStatic(command, cliName); err != nil {
			return true, Commands{}, fmt.Errorf("Command %d is not allowed: %v", index, err)
		}
		commands.Commands = append(commands.Commands, command)
	}
	if len(commands.Commands) == 0 {
		return true, Commands{}, errors.New("The commands block must contain at least one command.")
	}
	return true, commands, nil
}

func hashCommands(commands Commands) string {
	if len(commands.Commands) == 0 {
		return ""
	}
	return NormalizeAndHash(commands)
}
