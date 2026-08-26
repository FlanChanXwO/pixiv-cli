// Package releaseworkflow 检查发布 workflow 的结构化安全与质量门禁。
package releaseworkflow

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	workflowyaml "github.com/FlanChanXwO/pixiv-cli/scripts/internal/workflow/yaml"
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

// Run 是 scripts/cmd/releaseworkflow 的入口 owner：解析参数并委托给 workflow 校验逻辑。
func Run(args []string) error {
	return checkWorkflowPath(args)
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
// requiredJobs 是 release workflow 必须包含的 job 名称集合。
var requiredJobs = []string{
	"validate", "e2e", "build", "build_production", "release_notes_audit",
	"verify_release_source", "publish", "render_homebrew_formula",
	"verify_homebrew_formula", "deploy_homebrew_tap",
}

// optionalJobs 是 release workflow 可选包含的 job 名称集合。
// build_container/publish_container 一旦存在须满足 container contract。
var optionalJobs = map[string]struct{}{
	"build_container":   {},
	"publish_container": {},
}

// requireJobsAllowlist 确认 workflow 包含全部必需 job，只含允许的 job，
// 容器 job 是可选的。
func requireJobsAllowlist(jobs *yaml.Node) error {
	allowed := make(map[string]struct{}, len(requiredJobs)+len(optionalJobs))
	for _, name := range requiredJobs {
		allowed[name] = struct{}{}
	}
	for name := range optionalJobs {
		allowed[name] = struct{}{}
	}
	seen := make(map[string]struct{}, len(jobs.Content)/2)
	for index := 0; index+1 < len(jobs.Content); index += 2 {
		key := jobs.Content[index]
		if key.Kind != yaml.ScalarNode {
			return errors.New("must contain exactly the required keys")
		}
		if _, ok := allowed[key.Value]; !ok {
			return errors.New("must contain exactly the required keys")
		}
		seen[key.Value] = struct{}{}
	}
	for _, name := range requiredJobs {
		if _, ok := seen[name]; !ok {
			return errors.New("must contain exactly the required keys")
		}
	}
	return nil
}

func checkWorkflow(body []byte) error {
	var document yaml.Node
	if err := yaml.Unmarshal(body, &document); err != nil {
		return fmt.Errorf("parse YAML: %w", err)
	}
	if err := workflowyaml.RejectAmbiguousYAML(&document); err != nil {
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
	if err := checkReleaseTagBinding(root); err != nil {
		return err
	}
	if err := checkGlobalPermissions(root); err != nil {
		return err
	}
	jobs, ok := workflowyaml.MappingValue(root, "jobs")
	if !ok || jobs.Kind != yaml.MappingNode {
		return errors.New("workflow must have a jobs mapping")
	}
	// 容器 job（build_container/publish_container）是可选的；允许列表而非严格列表。
	if err := requireJobsAllowlist(jobs); err != nil {
		return fmt.Errorf("workflow jobs: %w", err)
	}
	validate, ok := workflowyaml.MappingValue(jobs, "validate")
	if !ok || validate.Kind != yaml.MappingNode {
		return errors.New("workflow must have a validate job")
	}
	e2e, ok := workflowyaml.MappingValue(jobs, "e2e")
	if !ok || e2e.Kind != yaml.MappingNode {
		return errors.New("workflow must have an e2e job")
	}
	build, ok := workflowyaml.MappingValue(jobs, "build")
	if !ok || build.Kind != yaml.MappingNode {
		return errors.New("workflow must have a build job")
	}
	productionBuild, ok := workflowyaml.MappingValue(jobs, "build_production")
	if !ok || productionBuild.Kind != yaml.MappingNode {
		return errors.New("workflow must have a build_production job")
	}
	releaseNotesAudit, ok := workflowyaml.MappingValue(jobs, "release_notes_audit")
	if !ok || releaseNotesAudit.Kind != yaml.MappingNode {
		return errors.New("workflow must have a release_notes_audit job")
	}
	verifyReleaseSource, ok := workflowyaml.MappingValue(jobs, "verify_release_source")
	if !ok || verifyReleaseSource.Kind != yaml.MappingNode {
		return errors.New("workflow must have a verify_release_source job")
	}
	publish, ok := workflowyaml.MappingValue(jobs, "publish")
	if !ok || publish.Kind != yaml.MappingNode {
		return errors.New("workflow must have a publish job")
	}
	renderHomebrew, ok := workflowyaml.MappingValue(jobs, "render_homebrew_formula")
	if !ok || renderHomebrew.Kind != yaml.MappingNode {
		return errors.New("workflow must have a render_homebrew_formula job")
	}
	verifyHomebrew, ok := workflowyaml.MappingValue(jobs, "verify_homebrew_formula")
	if !ok || verifyHomebrew.Kind != yaml.MappingNode {
		return errors.New("workflow must have a verify_homebrew_formula job")
	}
	deployHomebrew, ok := workflowyaml.MappingValue(jobs, "deploy_homebrew_tap")
	if !ok || deployHomebrew.Kind != yaml.MappingNode {
		return errors.New("workflow must have a deploy_homebrew_tap job")
	}
	// 先报告任何越界 secret 引用，避免后续 checkout 形状校验掩盖真实的凭据泄露风险。
	preflightPublishSteps, _ := jobSteps(publish)
	preflightSigningIndex, _ := signingStepIndex(preflightPublishSteps)
	if err := checkSigningSecretReachability(validate, build, productionBuild, releaseNotesAudit, verifyReleaseSource, publish, preflightPublishSteps, preflightSigningIndex); err != nil {
		return err
	}
	if err := checkHomebrewSecretReachability(renderHomebrew, verifyHomebrew, deployHomebrew); err != nil {
		return err
	}
	if err := checkValidateJob(validate); err != nil {
		return err
	}
	if err := checkE2EJob(e2e); err != nil {
		return err
	}
	if err := checkBuildJob(build); err != nil {
		return err
	}
	if err := checkProductionBuildJob(productionBuild); err != nil {
		return err
	}
	if err := checkReleaseNotesAuditJob(releaseNotesAudit); err != nil {
		return err
	}
	if err := checkVerifyReleaseSourceJob(verifyReleaseSource); err != nil {
		return err
	}
	signingIndex, publishSteps, err := checkPublishJob(publish)
	if err != nil {
		return err
	}
	if err := checkSigningSecretReachability(validate, build, productionBuild, releaseNotesAudit, verifyReleaseSource, publish, publishSteps, signingIndex); err != nil {
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
	// 容器 job 是可选的：一旦存在就必须满足 container release contract。
	// build_container 不持有 packages: write；publish_container 是唯一持权 job。
	buildContainer, _ := workflowyaml.MappingValue(jobs, "build_container")
	if err := checkContainerBuildJob(buildContainer); err != nil {
		return err
	}
	publishContainer, _ := workflowyaml.MappingValue(jobs, "publish_container")
	if err := checkContainerPublishJob(publishContainer); err != nil {
		return err
	}
	return nil
}
