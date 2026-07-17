// Command releaseworkflow 检查发布 workflow 的结构化安全与质量门禁。
package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/scripts/internal/workflowpolicy"
	"gopkg.in/yaml.v3"
)

// actionReferencePattern 只接受远端 action 的不可变完整对象 ID，避免可移动 tag 改写发布供应链。
var actionReferencePattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+@[0-9a-f]{40}$`)

const canonicalCheckoutAction = "actions/checkout@34e114876b0b11c390a56381ad16ebd13914f8d5"

const (
	setupGoAction          = "actions/setup-go@40f1582b2485089dde7abd97c1529aa768e1baff"
	uploadArtifactAction   = "actions/upload-artifact@ea165f8d65b6e75b540449e92b4886f43607fa02"
	downloadArtifactAction = "actions/download-artifact@d3f86a106a0bac45b974a628896c90dbdf5c8093"
	githubKnownHostsLine   = "github.com ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIOMqqnkVzrm0SdG6UOoqKLsabgH5C9okWi0dh2l9GKJl\n"
	testRustInstallCommand = "rustup toolchain install '${{ matrix.rust_toolchain }}' --profile minimal --component 'clippy,rustfmt' --target '${{ matrix.rust_target }}' --no-self-update"
	prodRustInstallCommand = "rustup toolchain install '${{ matrix.rust_toolchain }}' --profile minimal --target '${{ matrix.rust_target }}' --no-self-update"
)

func main() {
	if err := checkWorkflowPath(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "release workflow policy: %v\n", err)
		os.Exit(1)
	}
}

func checkWorkflowPath(arguments []string) error {
	if len(arguments) != 2 || arguments[0] != "--workflow" {
		return errors.New("usage: releaseworkflow --workflow PATH")
	}
	body, err := os.ReadFile(arguments[1])
	if err != nil {
		return fmt.Errorf("read workflow: %w", err)
	}
	if err := checkWorkflow(body); err != nil {
		return err
	}
	repositoryRoot := filepath.Clean(filepath.Join(filepath.Dir(arguments[1]), "..", ".."))
	knownHosts, err := os.ReadFile(filepath.Join(repositoryRoot, "templates", "homebrew", "github.com-known-hosts"))
	if err != nil {
		return fmt.Errorf("read pinned GitHub known_hosts: %w", err)
	}
	return checkPinnedGitHubKnownHosts(knownHosts)
}

func checkPinnedGitHubKnownHosts(body []byte) error {
	// Git for Windows 可将未标记文本 fixture checkout 为 CRLF；known_hosts
	// 的单条 ED25519 记录不受该表示影响，比较前只规范化 CRLF，仍拒绝额外内容。
	if strings.ReplaceAll(string(body), "\r\n", "\n") != githubKnownHostsLine {
		return errors.New("GitHub known_hosts fixture must contain only the pinned official ED25519 host key")
	}
	return nil
}

func checkWorkflow(body []byte) error {
	var document yaml.Node
	if err := yaml.Unmarshal(body, &document); err != nil {
		return fmt.Errorf("parse YAML: %w", err)
	}
	if err := workflowpolicy.RejectAmbiguousYAML(&document); err != nil {
		return err
	}
	if document.Kind != yaml.DocumentNode || len(document.Content) != 1 {
		return errors.New("workflow must contain exactly one YAML document")
	}
	root := document.Content[0]
	if root.Kind != yaml.MappingNode {
		return errors.New("workflow root must be a mapping")
	}
	if err := requireNoWorkflowExecutionOverrides(root); err != nil {
		return err
	}
	if err := checkActionReferences(root); err != nil {
		return err
	}
	if err := checkTagTrigger(root); err != nil {
		return err
	}
	if err := checkGlobalPermissions(root); err != nil {
		return err
	}
	jobs, ok := workflowpolicy.MappingValue(root, "jobs")
	if !ok || jobs.Kind != yaml.MappingNode {
		return errors.New("workflow must have a jobs mapping")
	}
	if err := requireOnlyMappingKeys(jobs, "validate", "build", "build_production", "verify_release_source", "publish", "render_homebrew_formula", "verify_homebrew_formula", "deploy_homebrew_tap"); err != nil {
		return fmt.Errorf("workflow jobs: %w", err)
	}
	validate, ok := workflowpolicy.MappingValue(jobs, "validate")
	if !ok || validate.Kind != yaml.MappingNode {
		return errors.New("workflow must have a validate job")
	}
	build, ok := workflowpolicy.MappingValue(jobs, "build")
	if !ok || build.Kind != yaml.MappingNode {
		return errors.New("workflow must have a build job")
	}
	productionBuild, ok := workflowpolicy.MappingValue(jobs, "build_production")
	if !ok || productionBuild.Kind != yaml.MappingNode {
		return errors.New("workflow must have a build_production job")
	}
	verifyReleaseSource, ok := workflowpolicy.MappingValue(jobs, "verify_release_source")
	if !ok || verifyReleaseSource.Kind != yaml.MappingNode {
		return errors.New("workflow must have a verify_release_source job")
	}
	publish, ok := workflowpolicy.MappingValue(jobs, "publish")
	if !ok || publish.Kind != yaml.MappingNode {
		return errors.New("workflow must have a publish job")
	}
	renderHomebrew, ok := workflowpolicy.MappingValue(jobs, "render_homebrew_formula")
	if !ok || renderHomebrew.Kind != yaml.MappingNode {
		return errors.New("workflow must have a render_homebrew_formula job")
	}
	verifyHomebrew, ok := workflowpolicy.MappingValue(jobs, "verify_homebrew_formula")
	if !ok || verifyHomebrew.Kind != yaml.MappingNode {
		return errors.New("workflow must have a verify_homebrew_formula job")
	}
	deployHomebrew, ok := workflowpolicy.MappingValue(jobs, "deploy_homebrew_tap")
	if !ok || deployHomebrew.Kind != yaml.MappingNode {
		return errors.New("workflow must have a deploy_homebrew_tap job")
	}
	// 先报告任何越界 secret 引用，避免后续 checkout 形状校验掩盖真实的凭据泄露风险。
	preflightPublishSteps, _ := jobSteps(publish)
	preflightSigningIndex, _ := signingStepIndex(preflightPublishSteps)
	if err := checkSigningSecretReachability(validate, build, productionBuild, verifyReleaseSource, publish, preflightPublishSteps, preflightSigningIndex); err != nil {
		return err
	}
	if err := checkHomebrewSecretReachability(renderHomebrew, verifyHomebrew, deployHomebrew); err != nil {
		return err
	}
	if err := checkValidateJob(validate); err != nil {
		return err
	}
	if err := checkBuildJob(build); err != nil {
		return err
	}
	if err := checkProductionBuildJob(productionBuild); err != nil {
		return err
	}
	if err := checkRecoveryPolicy(root); err != nil {
		return err
	}
	if err := checkVerifyReleaseSourceJob(verifyReleaseSource); err != nil {
		return err
	}
	signingIndex, publishSteps, err := checkPublishJob(publish)
	if err != nil {
		return err
	}
	if err := checkSigningSecretReachability(validate, build, productionBuild, verifyReleaseSource, publish, publishSteps, signingIndex); err != nil {
		return err
	}
	if err := checkRenderHomebrewJob(renderHomebrew); err != nil {
		return err
	}
	if err := checkVerifyHomebrewJob(verifyHomebrew); err != nil {
		return err
	}
	if err := checkDeployHomebrewJob(deployHomebrew); err != nil {
		return err
	}
	return nil
}
