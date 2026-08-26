package releaseworkflow

import (
	"strings"
	"testing"

	workflowyaml "github.com/FlanChanXwO/pixiv-cli/scripts/internal/workflow/yaml"
	"gopkg.in/yaml.v3"
)

// TestCheckWorkflowRejectsMissingContainerJobs 断言 release workflow 必须包含
// build_container 和 publish_container job。当前 checked-in workflow 缺少这些 job，
// checkWorkflow 返回 nil（不报错），因此测试失败——这是 Red 状态。
func TestCheckWorkflowRejectsMissingContainerJobs(t *testing.T) {
	t.Parallel()

	root := releaseWorkflowRoot(t)
	body, err := yaml.Marshal(root)
	if err != nil {
		t.Fatalf("marshal workflow: %v", err)
	}
	if err := checkWorkflow(body); err == nil ||
		!strings.Contains(err.Error(), "build_container") {
		t.Fatalf("policy error = %v, want rejection mentioning build_container", err)
	}
}

// TestContainerBuildJobRejectsPackagesWrite 断言 build_container 不持有 packages: write。
// Red 状态：checkWorkflow 因 build_container 不在 allowed job list 而拒绝，报 "must contain
// exactly the required keys" 而非 "packages"，测试因错误不匹配而失败。
func TestContainerBuildJobRejectsPackagesWrite(t *testing.T) {
	t.Parallel()

	root := releaseWorkflowRoot(t)
	jobs := requireMappingValue(t, root, "jobs")
	buildContainer := containerJobFixture(t)
	// 替换 permissions 为含 packages: write 的版本
	removeMappingValue(t, buildContainer, "permissions")
	appendMappingValue(t, buildContainer, "permissions", mappingNode(
		"contents", scalarNode("read"),
		"packages", scalarNode("write"),
	))
	appendMappingValue(t, jobs, "build_container", buildContainer)
	body, err := yaml.Marshal(root)
	if err != nil {
		t.Fatalf("marshal workflow: %v", err)
	}
	err = checkWorkflow(body)
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "packages") {
		t.Fatalf("policy error = %v, want rejection containing %q", err, "packages")
	}
}

// TestContainerBuildJobRejectsMovableRef 断言 build_container checkout 不可变 tag。
func TestContainerBuildJobRejectsMovableRef(t *testing.T) {
	t.Parallel()

	root := releaseWorkflowRoot(t)
	jobs := requireMappingValue(t, root, "jobs")
	buildContainer := containerJobFixture(t)
	steps := requireMappingValue(t, buildContainer, "steps")
	checkout := checkoutStepFromContainer(t, steps)
	with := requireMappingValue(t, checkout, "with")
	requireMappingValue(t, with, "ref").Value = "${{ github.event.repository.default_branch }}"
	appendMappingValue(t, jobs, "build_container", buildContainer)
	body, err := yaml.Marshal(root)
	if err != nil {
		t.Fatalf("marshal workflow: %v", err)
	}
	err = checkWorkflow(body)
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "immutable") {
		t.Fatalf("policy error = %v, want rejection containing %q", err, "immutable")
	}
}

// TestContainerBuildJobRejectsQEMU 断言 build_container 无 QEMU/setup-qemu 步骤。
func TestContainerBuildJobRejectsQEMU(t *testing.T) {
	t.Parallel()

	root := releaseWorkflowRoot(t)
	jobs := requireMappingValue(t, root, "jobs")
	buildContainer := containerJobFixture(t)
	steps := requireMappingValue(t, buildContainer, "steps")
	// 使用一个合法格式的 SHA 以避免 action reference 校验先于 container policy 报错
	qemuStep := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Content: []*yaml.Node{
		scalarNode("name"), scalarNode("Set up QEMU"),
		scalarNode("uses"), scalarNode("docker/setup-qemu-action@2f5e3a8e4f3e5a5e5a5e5a5e5a5e5a5a5a5a5a5e5"),
	}}
	steps.Content = append(steps.Content, qemuStep)
	appendMappingValue(t, jobs, "build_container", buildContainer)
	body, err := yaml.Marshal(root)
	if err != nil {
		t.Fatalf("marshal workflow: %v", err)
	}
	err = checkWorkflow(body)
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "qemu") {
		t.Fatalf("policy error = %v, want rejection containing %q", err, "qemu")
	}
}

// TestContainerBuildJobRejectsNonNativeRunner 断言 build_container 使用原生 Linux runner。
func TestContainerBuildJobRejectsNonNativeRunner(t *testing.T) {
	t.Parallel()

	root := releaseWorkflowRoot(t)
	jobs := requireMappingValue(t, root, "jobs")
	buildContainer := containerJobFixture(t)
	requireMappingValue(t, buildContainer, "runs-on").Value = "macos-15"
	appendMappingValue(t, jobs, "build_container", buildContainer)
	body, err := yaml.Marshal(root)
	if err != nil {
		t.Fatalf("marshal workflow: %v", err)
	}
	err = checkWorkflow(body)
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "runner") {
		t.Fatalf("policy error = %v, want rejection containing %q", err, "runner")
	}
}

// TestContainerBuildJobRejectsBuildProductionDependency 断言 build_container
// 在共享质量门禁后启动，与 build_production 并行而非串行依赖。
func TestContainerBuildJobRejectsBuildProductionDependency(t *testing.T) {
	t.Parallel()

	root := releaseWorkflowRoot(t)
	jobs := requireMappingValue(t, root, "jobs")
	buildContainer := containerJobFixture(t)
	// 替换 needs 为 build_production（串行依赖，应被拒绝）
	removeMappingValue(t, buildContainer, "needs")
	appendMappingValue(t, buildContainer, "needs", sequenceNode("build_production"))
	appendMappingValue(t, jobs, "build_container", buildContainer)
	body, err := yaml.Marshal(root)
	if err != nil {
		t.Fatalf("marshal workflow: %v", err)
	}
	err = checkWorkflow(body)
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "build_production") {
		t.Fatalf("policy error = %v, want rejection for build_container depending on build_production", err)
	}
}

// TestContainerPublishJobRequiresPackagesWrite 断言 publish_container 是唯一持有
// packages: write 的 job。
func TestContainerPublishJobRequiresPackagesWrite(t *testing.T) {
	t.Parallel()

	root := releaseWorkflowRoot(t)
	jobs := requireMappingValue(t, root, "jobs")
	publishContainer := containerPublishJobFixture()
	appendMappingValue(t, publishContainer, "name", scalarNode("Publish container image"))
	appendMappingValue(t, publishContainer, "needs", sequenceNode("build_container", "publish"))
	appendMappingValue(t, publishContainer, "runs-on", scalarNode("ubuntu-24.04"))
	// 故意缺少 packages: write，应被拒绝
	appendMappingValue(t, publishContainer, "permissions", mappingNode(
		"contents", scalarNode("read"),
	))
	appendMappingValue(t, jobs, "publish_container", publishContainer)
	body, err := yaml.Marshal(root)
	if err != nil {
		t.Fatalf("marshal workflow: %v", err)
	}
	err = checkWorkflow(body)
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "packages") {
		t.Fatalf("policy error = %v, want rejection containing %q", err, "packages")
	}
}

// TestContainerPublishJobRequiresBuildContainerDependency 断言 publish_container
// 依赖 build_container。
func TestContainerPublishJobRequiresBuildContainerDependency(t *testing.T) {
	t.Parallel()

	root := releaseWorkflowRoot(t)
	jobs := requireMappingValue(t, root, "jobs")
	publishContainer := containerPublishJobFixture()
	appendMappingValue(t, publishContainer, "name", scalarNode("Publish container image"))
	// 故意只依赖 publish，缺少 build_container
	appendMappingValue(t, publishContainer, "needs", scalarNode("publish"))
	appendMappingValue(t, publishContainer, "runs-on", scalarNode("ubuntu-24.04"))
	appendMappingValue(t, publishContainer, "permissions", mappingNode(
		"contents", scalarNode("read"),
		"packages", scalarNode("write"),
	))
	appendMappingValue(t, jobs, "publish_container", publishContainer)
	body, err := yaml.Marshal(root)
	if err != nil {
		t.Fatalf("marshal workflow: %v", err)
	}
	err = checkWorkflow(body)
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "build_container") {
		t.Fatalf("policy error = %v, want rejection containing %q", err, "build_container")
	}
}

// TestContainerRegistryPath 断言容器 registry 路径常量正确。
func TestContainerRegistryPath(t *testing.T) {
	t.Parallel()

	if containerRegistry != "ghcr.io/flanchanxwo/pixiv-cli" {
		t.Fatalf("containerRegistry = %q, want ghcr.io/flanchanxwo/pixiv-cli", containerRegistry)
	}
}

// containerJobFixture 返回一个最小 build_container job 节点，
// 满足结构 contract 中最基本的字段，以便各个 mutation 测试可以在其上修改。
func containerJobFixture(t *testing.T) *yaml.Node {
	t.Helper()
	job := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	appendMappingValue(t, job, "name", scalarNode("Build container ${{ matrix.goos }}/${{ matrix.goarch }}"))
	appendMappingValue(t, job, "needs", sequenceNode("build"))
	appendMappingValue(t, job, "runs-on", scalarNode("${{ matrix.runner }}"))
	appendMappingValue(t, job, "permissions", mappingNode(
		"contents", scalarNode("read"),
	))
	appendMappingValue(t, job, "strategy", mappingNode(
		"fail-fast", scalarNode("false"),
		"matrix", containerMatrixFixture(t),
	))
	appendMappingValue(t, job, "steps", &yaml.Node{
		Kind: yaml.SequenceNode,
		Tag:  "!!seq",
		Content: []*yaml.Node{
			canonicalContainerCheckout(t),
		},
	})
	return job
}

// containerPublishJobFixture 返回一个空 publish_container job 节点，
// 由各测试自行填充字段。
func containerPublishJobFixture() *yaml.Node {
	return &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
}

// containerMatrixFixture 返回一个只含两个原生 Linux target 的 matrix 节点。
func containerMatrixFixture(t *testing.T) *yaml.Node {
	t.Helper()
	return mappingNode(
		"include", &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq", Content: []*yaml.Node{
			containerMatrixEntry("ubuntu-22.04", "linux", "amd64", "x86_64-unknown-linux-gnu", "1.96.1", "linux-amd64", "gcc"),
			containerMatrixEntry("ubuntu-22.04-arm", "linux", "arm64", "aarch64-unknown-linux-gnu", "1.96.1", "linux-arm64", "gcc"),
		}},
	)
}

func containerMatrixEntry(runner, goos, goarch, rustTarget, rustToolchain, artifact, cc string) *yaml.Node {
	return mappingNode(
		"runner", scalarNode(runner),
		"goos", scalarNode(goos),
		"goarch", scalarNode(goarch),
		"rust_target", scalarNode(rustTarget),
		"rust_toolchain", scalarNode(rustToolchain),
		"artifact", scalarNode(artifact),
		"cc", scalarNode(cc),
	)
}

// canonicalContainerCheckout 返回标准 checkout step（不可变 tag）。
func canonicalContainerCheckout(t *testing.T) *yaml.Node {
	t.Helper()
	return &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Content: []*yaml.Node{
		scalarNode("uses"), scalarNode(canonicalCheckoutAction),
		scalarNode("with"), mappingNode(
			"fetch-depth", scalarNode("0"),
			"persist-credentials", scalarNode("false"),
			"ref", scalarNode("${{ env.RELEASE_TAG }}"),
		),
	}}
}

// checkoutStepFromContainer 找到 container job steps 中的 checkout step。
func checkoutStepFromContainer(t *testing.T, steps *yaml.Node) *yaml.Node {
	t.Helper()
	for _, step := range steps.Content {
		uses, ok := workflowyaml.MappingValue(step, "uses")
		if ok && strings.HasPrefix(uses.Value, "actions/checkout@") {
			return step
		}
	}
	t.Fatal("container job fixture has no checkout step")
	return nil
}
