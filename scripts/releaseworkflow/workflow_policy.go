package main

import (
	"errors"
	"fmt"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/scripts/internal/workflowpolicy"
	"gopkg.in/yaml.v3"
)

func mustJobSteps(job *yaml.Node) []*yaml.Node {
	steps, err := jobSteps(job)
	if err != nil {
		return nil
	}
	return steps
}

func containsScalarFragment(node *yaml.Node, fragment string) bool {
	if node == nil {
		return false
	}
	if node.Kind == yaml.ScalarNode && strings.Contains(node.Value, fragment) {
		return true
	}
	for _, child := range node.Content {
		if containsScalarFragment(child, fragment) {
			return true
		}
	}
	return false
}

func countScalarFragment(node *yaml.Node, fragment string) int {
	if node == nil {
		return 0
	}
	count := 0
	if node.Kind == yaml.ScalarNode {
		count += strings.Count(node.Value, fragment)
	}
	for _, child := range node.Content {
		count += countScalarFragment(child, fragment)
	}
	return count
}

func stepIndexWithRunFragment(steps []*yaml.Node, fragment string) int {
	for index, step := range steps {
		if strings.Contains(requireRunValue(step), fragment) {
			return index
		}
	}
	return -1
}

func actionStepIndices(steps []*yaml.Node, action string) []int {
	var indices []int
	for index, step := range steps {
		uses, ok := workflowpolicy.MappingValue(step, "uses")
		if ok && uses.Kind == yaml.ScalarNode && uses.Value == action {
			indices = append(indices, index)
		}
	}
	return indices
}

func requireCanonicalNamedRunStep(step *yaml.Node, name, command string) error {
	if err := requireCanonicalRunStep(step, name, command); err != nil {
		return err
	}
	if err := workflowpolicy.RequireScalar(step, "name", name); err != nil {
		return fmt.Errorf("%s must keep its canonical name", name)
	}
	return nil
}

func requireCanonicalNamedRunStepInDirectory(step *yaml.Node, name, directory, command string) error {
	if err := requireOnlyMappingKeys(step, "name", "shell", "working-directory", "run"); err != nil || workflowpolicy.RequireScalar(step, "name", name) != nil || workflowpolicy.RequireScalar(step, "shell", "bash") != nil || workflowpolicy.RequireScalar(step, "working-directory", directory) != nil || workflowpolicy.RequireScalar(step, "run", command) != nil {
		return fmt.Errorf("%s must be the exact canonical step in %s", name, directory)
	}
	return nil
}

func requireCanonicalConditionalRunStep(step *yaml.Node, context, condition, canonical string) error {
	if err := requireOnlyMappingKeys(step, "name", "if", "shell", "run"); err != nil {
		return fmt.Errorf("%s must be the canonical conditional bash step", context)
	}
	if err := workflowpolicy.RequireScalar(step, "if", condition); err != nil {
		return fmt.Errorf("%s must use only the approved condition", context)
	}
	if err := workflowpolicy.RequireScalar(step, "shell", "bash"); err != nil || !equalCommands(splitCommands(requireRunValue(step)), splitCommands(canonical)) {
		return fmt.Errorf("%s must use the exact command sequence", context)
	}
	return nil
}

func mustMappingPath(root *yaml.Node, keys ...string) *yaml.Node {
	current := root
	for _, key := range keys {
		var ok bool
		current, ok = workflowpolicy.MappingValue(current, key)
		if !ok {
			return nil
		}
	}
	return current
}

func checkGlobalPermissions(root *yaml.Node) error {
	permissions, ok := workflowpolicy.MappingValue(root, "permissions")
	if !ok || permissions.Kind != yaml.MappingNode || len(permissions.Content) != 0 {
		return errors.New("global permissions must be an empty mapping")
	}
	return nil
}

func checkActionReferences(root *yaml.Node) error {
	var references []string
	invalidReference := false
	collectActionReferences(root, &references, &invalidReference)
	if len(references) == 0 {
		return errors.New("workflow must use at least one action")
	}
	if invalidReference {
		return errors.New("every action uses reference must be an owner/repo full 40-character lowercase SHA")
	}
	for _, reference := range references {
		if !actionReferencePattern.MatchString(reference) {
			return errors.New("every action uses reference must be an owner/repo full 40-character lowercase SHA")
		}
	}
	return nil
}

func collectActionReferences(node *yaml.Node, references *[]string, invalidReference *bool) {
	if node == nil {
		return
	}
	if node.Kind == yaml.MappingNode {
		for index := 0; index+1 < len(node.Content); index += 2 {
			key, value := node.Content[index], node.Content[index+1]
			if key.Value == "uses" {
				if value.Kind != yaml.ScalarNode {
					*invalidReference = true
				} else {
					*references = append(*references, value.Value)
				}
			}
			collectActionReferences(value, references, invalidReference)
		}
		return
	}
	for _, child := range node.Content {
		collectActionReferences(child, references, invalidReference)
	}
}

func requireNoWorkflowExecutionOverrides(root *yaml.Node) error {
	if _, exists := workflowpolicy.MappingValue(root, "defaults"); exists {
		return errors.New("workflow root must not declare defaults")
	}
	return nil
}

func requireNoEnvironment(job *yaml.Node, jobName string) error {
	if _, exists := workflowpolicy.MappingValue(job, "environment"); exists {
		return fmt.Errorf("%s must not declare an environment", jobName)
	}
	return nil
}

func requireRequiredJobExecution(job *yaml.Node, jobName string) error {
	for _, key := range []string{"if", "continue-on-error"} {
		if _, exists := workflowpolicy.MappingValue(job, key); exists {
			return fmt.Errorf("%s must not define if or continue-on-error", jobName)
		}
	}
	if _, exists := workflowpolicy.MappingValue(job, "defaults"); exists {
		return fmt.Errorf("%s must not declare defaults", jobName)
	}
	return nil
}

func requireExactActionStep(step *yaml.Node, context, action string, withValues map[string]string) error {
	if err := requireOnlyMappingKeys(step, "uses", "with"); err != nil {
		return fmt.Errorf("%s must be the exact pinned action step", context)
	}
	if err := workflowpolicy.RequireScalar(step, "uses", action); err != nil {
		return fmt.Errorf("%s must be the exact pinned action step", context)
	}
	with, ok := workflowpolicy.MappingValue(step, "with")
	if !ok || with.Kind != yaml.MappingNode || len(with.Content) != len(withValues)*2 {
		return fmt.Errorf("%s must be the exact pinned action step", context)
	}
	for key, value := range withValues {
		if err := workflowpolicy.RequireScalar(with, key, value); err != nil {
			return fmt.Errorf("%s must be the exact pinned action step", context)
		}
	}
	return nil
}

func requireCanonicalRunStep(step *yaml.Node, context, canonical string) error {
	if err := requireOnlyMappingKeys(step, "name", "shell", "run"); err != nil {
		return fmt.Errorf("%s must be the canonical direct bash step", context)
	}
	if err := workflowpolicy.RequireScalar(step, "shell", "bash"); err != nil {
		return fmt.Errorf("%s must be the canonical direct bash step", context)
	}
	if !equalCommands(splitCommands(requireRunValue(step)), splitCommands(canonical)) {
		return fmt.Errorf("%s must use the required direct command sequence", context)
	}
	return nil
}

func requireCanonicalRunStepWithEnvironment(step *yaml.Node, context, canonical string) error {
	if err := requireOnlyMappingKeys(step, "name", "shell", "env", "run"); err != nil {
		return fmt.Errorf("%s must be the canonical direct bash step", context)
	}
	if err := workflowpolicy.RequireScalar(step, "shell", "bash"); err != nil {
		return fmt.Errorf("%s must be the canonical direct bash step", context)
	}
	if !equalCommands(splitCommands(requireRunValue(step)), splitCommands(canonical)) {
		return fmt.Errorf("%s must use the required direct command sequence", context)
	}
	return nil
}

func requireOnlyMappingKeys(mapping *yaml.Node, keys ...string) error {
	if !workflowpolicy.HasExactMappingKeys(mapping, keys...) {
		return errors.New("must contain exactly the required keys")
	}
	return nil
}

func requireContentsPermission(job *yaml.Node, level string) error {
	permissions, ok := workflowpolicy.MappingValue(job, "permissions")
	if !ok || permissions.Kind != yaml.MappingNode || len(permissions.Content) != 2 {
		return fmt.Errorf("permissions must contain only contents: %s", level)
	}
	if err := workflowpolicy.RequireScalar(permissions, "contents", level); err != nil {
		return fmt.Errorf("permissions must contain only contents: %s", level)
	}
	return nil
}

func jobSteps(job *yaml.Node) ([]*yaml.Node, error) {
	steps, ok := workflowpolicy.MappingValue(job, "steps")
	if !ok || steps.Kind != yaml.SequenceNode || len(steps.Content) == 0 {
		return nil, errors.New("must contain a non-empty steps sequence")
	}
	return steps.Content, nil
}

func hasCommandInWorkingDirectory(job *yaml.Node, directory, command string) bool {
	steps, err := jobSteps(job)
	if err != nil {
		return false
	}
	for _, step := range steps {
		workingDirectory, hasWorkingDirectory := workflowpolicy.MappingValue(step, "working-directory")
		run, hasRun := workflowpolicy.MappingValue(step, "run")
		if !hasWorkingDirectory || !hasRun || workingDirectory.Value != directory {
			continue
		}
		for _, line := range splitCommands(run.Value) {
			if line == command {
				return true
			}
		}
	}
	return false
}

func hasCommand(job *yaml.Node, directory, command string) bool {
	if directory != "" {
		return hasCommandInWorkingDirectory(job, directory, command)
	}
	steps, err := jobSteps(job)
	if err != nil {
		return false
	}
	for _, step := range steps {
		workingDirectory, hasWorkingDirectory := workflowpolicy.MappingValue(step, "working-directory")
		if hasWorkingDirectory && (workingDirectory.Kind != yaml.ScalarNode || workingDirectory.Value != ".") {
			continue
		}
		if hasStepCommand(step, command) {
			return true
		}
	}
	return false
}

func rootStepWithRunFragment(job *yaml.Node, fragment string) (*yaml.Node, bool) {
	steps, err := jobSteps(job)
	if err != nil {
		return nil, false
	}
	for _, step := range steps {
		workingDirectory, hasWorkingDirectory := workflowpolicy.MappingValue(step, "working-directory")
		if hasWorkingDirectory && (workingDirectory.Kind != yaml.ScalarNode || workingDirectory.Value != ".") {
			continue
		}
		run, hasRun := workflowpolicy.MappingValue(step, "run")
		if hasRun && run.Kind == yaml.ScalarNode && strings.Contains(run.Value, fragment) {
			return step, true
		}
	}
	return nil, false
}

func requireRunFragments(step *yaml.Node, context string, fragments ...string) error {
	run := requireRunValue(step)
	if run == "" {
		return fmt.Errorf("%s must have a run command", context)
	}
	for _, fragment := range fragments {
		if !strings.Contains(run, fragment) {
			return fmt.Errorf("%s must contain %s", context, fragment)
		}
	}
	return nil
}

func requireRunValue(step *yaml.Node) string {
	run, hasRun := workflowpolicy.MappingValue(step, "run")
	if !hasRun || run.Kind != yaml.ScalarNode {
		return ""
	}
	return run.Value
}

func commandIndexAfter(commands []string, want string, after int) int {
	for index := after + 1; index < len(commands); index++ {
		if commands[index] == want {
			return index
		}
	}
	return -1
}

func countCommand(commands []string, want string) int {
	count := 0
	for _, command := range commands {
		if command == want {
			count++
		}
	}
	return count
}

func countRunFragment(step *yaml.Node, fragment string) int {
	return strings.Count(requireRunValue(step), fragment)
}

func hasShellArgument(step *yaml.Node, argument string) bool {
	run, hasRun := workflowpolicy.MappingValue(step, "run")
	if !hasRun || run.Kind != yaml.ScalarNode {
		return false
	}
	for _, line := range strings.Split(run.Value, "\n") {
		line = strings.TrimSpace(line)
		line = strings.TrimSpace(strings.TrimSuffix(line, "\\"))
		if line == argument {
			return true
		}
	}
	return false
}

func hasStepCommand(step *yaml.Node, command string) bool {
	run, hasRun := workflowpolicy.MappingValue(step, "run")
	if !hasRun || run.Kind != yaml.ScalarNode {
		return false
	}
	for _, line := range splitCommands(run.Value) {
		if line == command {
			return true
		}
	}
	return false
}

func splitCommands(run string) []string {
	var commands []string
	for _, line := range strings.Split(run, "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") {
			commands = append(commands, line)
		}
	}
	return commands
}

func equalCommands(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for index := range want {
		if got[index] != want[index] {
			return false
		}
	}
	return true
}
