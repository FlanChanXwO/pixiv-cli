package releaseworkflow

import (
	"strings"
	"testing"

	workflowyaml "github.com/FlanChanXwO/pixiv-cli/scripts/internal/workflow/yaml"
	"gopkg.in/yaml.v3"
)

// TestCheckWorkflowAcceptsCheckedInWorkflowWithContainerJobs 断言当 release workflow
// 包含合规格器 build_container/publish_container job 时 policy 接受它。
// 当前 checked-in workflow 尚无容器 job，因此此测试先添加合规 job 再验证接受。
func TestCheckWorkflowAcceptsCheckedInWorkflowWithContainerJobs(t *testing.T) {
	t.Parallel()

	root := releaseWorkflowRoot(t)
	addContainerJobs(t, root)
	body, err := yaml.Marshal(root)
	if err != nil {
		t.Fatalf("marshal workflow: %v", err)
	}
	if err := checkWorkflow(body); err != nil {
		t.Fatalf("policy rejected workflow with compliant container jobs: %v", err)
	}
}

// TestCheckWorkflowRequiresContainerReleaseJobs 断言正式 release workflow 必须包含
// 两个容器 job。容器是发布路径的一等公民，不允许继续停留在“可选 fixture”状态。
func TestCheckWorkflowRequiresContainerReleaseJobs(t *testing.T) {
	t.Parallel()

	root := releaseWorkflowRoot(t)
	jobs := requireMappingValue(t, root, "jobs")
	for _, name := range []string{"build_container", "publish_container"} {
		if _, ok := workflowyaml.MappingValue(jobs, name); !ok {
			t.Fatalf("checked-in release workflow must contain the %q job", name)
		}
	}
}

// TestGitHubReleaseRequiresVerifiedContainerArtifacts 断言 GitHub Release 发布
// 必须等待两架构容器 artifact。GHCR 推送可以失败并重跑，但不能在容器构建未验证时先发 Release。
func TestGitHubReleaseRequiresVerifiedContainerArtifacts(t *testing.T) {
	t.Parallel()

	root := releaseWorkflowRoot(t)
	jobs := requireMappingValue(t, root, "jobs")
	addBuildContainerJob(t, jobs)
	publish := requireMappingValue(t, jobs, "publish")
	needs := requireMappingValue(t, publish, "needs")
	filteredNeeds := make([]*yaml.Node, 0, len(needs.Content))
	for _, dependency := range needs.Content {
		if dependency.Value != "build_container" {
			filteredNeeds = append(filteredNeeds, dependency)
		}
	}
	needs.Content = filteredNeeds
	body, err := yaml.Marshal(root)
	if err != nil {
		t.Fatalf("marshal workflow: %v", err)
	}
	err = checkWorkflow(body)
	if err == nil || !strings.Contains(err.Error(), "verified container") {
		t.Fatalf("policy error = %v, want rejection mentioning verified container", err)
	}
}

// TestContainerPublishJobRequiresGitHubRelease 断言 GHCR 发布必须在 GitHub Release
// 之后执行；这保证容器推送是 post-Release 恢复路径，而不是绕过 Release 门禁的旁路。
func TestContainerPublishJobRequiresGitHubRelease(t *testing.T) {
	t.Parallel()

	root := releaseWorkflowRoot(t)
	jobs := requireMappingValue(t, root, "jobs")
	addBuildContainerJob(t, jobs)
	publishContainer := containerPublishJobFixture()
	appendMappingValue(t, publishContainer, "name", scalarNode("Publish container image"))
	appendMappingValue(t, publishContainer, "needs", sequenceNode("build_container"))
	appendMappingValue(t, publishContainer, "runs-on", scalarNode("ubuntu-24.04"))
	appendMappingValue(t, publishContainer, "permissions", mappingNode(
		"contents", scalarNode("read"),
		"packages", scalarNode("write"),
	))
	appendMappingValue(t, publishContainer, "steps", &yaml.Node{
		Kind: yaml.SequenceNode,
		Tag:  "!!seq",
		Content: []*yaml.Node{
			canonicalPublishCheckout(t),
			runStepNode("Publish exact-version tag", `docker push ghcr.io/flanchanxwo/pixiv-cli:${RELEASE_TAG}`),
		},
	})
	setJobNode(t, jobs, "publish_container", publishContainer)
	body, err := yaml.Marshal(root)
	if err != nil {
		t.Fatalf("marshal workflow: %v", err)
	}
	err = checkWorkflow(body)
	if err == nil || !strings.Contains(err.Error(), "publish") {
		t.Fatalf("policy error = %v, want rejection requiring the publish dependency", err)
	}
}

// TestContainerBuildJobRejectsPackagesWrite 断言 build_container 不持有 packages: write。es: write。
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
	setJobNode(t, jobs, "build_container", buildContainer)
	// 添加合规的 publish_container 以便不因缺少它而报错
	addPublishContainerJob(t, jobs)
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
	setJobNode(t, jobs, "build_container", buildContainer)
	addPublishContainerJob(t, jobs)
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
		scalarNode("uses"), scalarNode("docker/setup-qemu-action@aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"),
	}}
	steps.Content = append(steps.Content, qemuStep)
	setJobNode(t, jobs, "build_container", buildContainer)
	addPublishContainerJob(t, jobs)
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
	setJobNode(t, jobs, "build_container", buildContainer)
	addPublishContainerJob(t, jobs)
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
	setJobNode(t, jobs, "build_container", buildContainer)
	addPublishContainerJob(t, jobs)
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
	appendMappingValue(t, publishContainer, "steps", &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"})
	setJobNode(t, jobs, "publish_container", publishContainer)
	// 添加合规的 build_container 以便不因缺少它而报错
	addBuildContainerJob(t, jobs)
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
	appendMappingValue(t, publishContainer, "steps", &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"})
	setJobNode(t, jobs, "publish_container", publishContainer)
	// 添加合规的 build_container 以便不因缺少它而报错
	addBuildContainerJob(t, jobs)
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

// addContainerJobs 向 workflow root 添加合规的 build_container 和 publish_container job。
func addContainerJobs(t *testing.T, root *yaml.Node) {
	t.Helper()
	jobs := requireMappingValue(t, root, "jobs")
	addBuildContainerJob(t, jobs)
	addPublishContainerJob(t, jobs)
}

// setJobNode 替换 checked-in workflow 中同名 job，避免正式容器 job 进入仓库后
// 旧的“追加 fixture”测试产生 duplicate key。
func setJobNode(t *testing.T, jobs *yaml.Node, name string, job *yaml.Node) {
	t.Helper()
	removeMappingValue(t, jobs, name)
	appendMappingValue(t, jobs, name, job)
}

func addBuildContainerJob(t *testing.T, jobs *yaml.Node) {
	t.Helper()
	setJobNode(t, jobs, "build_container", containerJobFixture(t))
}

func addPublishContainerJob(t *testing.T, jobs *yaml.Node) {
	t.Helper()
	publishContainer := containerPublishJobFixture()
	appendMappingValue(t, publishContainer, "name", scalarNode("Publish container image"))
	appendMappingValue(t, publishContainer, "needs", sequenceNode("build_container", "publish"))
	appendMappingValue(t, publishContainer, "runs-on", scalarNode("ubuntu-24.04"))
	appendMappingValue(t, publishContainer, "permissions", mappingNode(
		"contents", scalarNode("read"),
		"packages", scalarNode("write"),
	))
	// publish_container 需要有 steps 字段以通过 requireOnlyMappingKeys
	// 且必须包含 exact-version tag 推送步骤以满足 tag policy
	appendMappingValue(t, publishContainer, "steps", &yaml.Node{
		Kind: yaml.SequenceNode,
		Tag:  "!!seq",
		Content: []*yaml.Node{
			canonicalPublishCheckout(t),
			runStepNode("Publish exact-version tag", `docker push ghcr.io/flanchanxwo/pixiv-cli:${RELEASE_TAG}`),
		},
	})
	setJobNode(t, jobs, "publish_container", publishContainer)
}

// containerJobFixture 返回一个合规的 build_container job 节点。tainer job 节点。
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

// canonicalPublishCheckout 返回 publish_container 所需的轻量不可变 tag checkout。
func canonicalPublishCheckout(t *testing.T) *yaml.Node {
	t.Helper()
	return &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Content: []*yaml.Node{
		scalarNode("uses"), scalarNode(canonicalCheckoutAction),
		scalarNode("with"), mappingNode(
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

// TestContainerPublishJobIsOnlyPackagesWrite 断言 publish_container 是唯一持有
// packages: write 的 job——workflow 中没有任何其它 job 持有 packages: write。
// 这确保 registry 推送权限仅限于发布 job，构建 job 无法直接推送未验证镜像。
func TestContainerPublishJobIsOnlyPackagesWrite(t *testing.T) {
	t.Parallel()

	root := releaseWorkflowRoot(t)
	jobs := requireMappingValue(t, root, "jobs")

	// 添加合规的 build_container 和 publish_container
	addBuildContainerJob(t, jobs)
	addPublishContainerJob(t, jobs)

	// 遍历所有 job，确保只有 publish_container 持有 packages: write
	for i := 0; i+1 < len(jobs.Content); i += 2 {
		jobName := jobs.Content[i].Value
		job := jobs.Content[i+1]
		permissions, ok := workflowyaml.MappingValue(job, "permissions")
		if !ok {
			continue
		}
		packages, ok := workflowyaml.MappingValue(permissions, "packages")
		if !ok {
			continue
		}
		if packages.Kind == yaml.ScalarNode && packages.Value == "write" {
			if jobName != "publish_container" {
				t.Fatalf("job %q must not declare packages: write (only publish_container may)", jobName)
			}
		}
	}

	body, err := yaml.Marshal(root)
	if err != nil {
		t.Fatalf("marshal workflow: %v", err)
	}
	if err := checkWorkflow(body); err != nil {
		t.Fatalf("policy rejected workflow: %v", err)
	}

	// 现在给 build_container 添加 packages: write（已被其它测试覆盖），
	// 但这里额外验证全局唯一性：给一个非容器 job 添加 packages: write
	root2 := releaseWorkflowRoot(t)
	jobs2 := requireMappingValue(t, root2, "jobs")
	addBuildContainerJob(t, jobs2)
	addPublishContainerJob(t, jobs2)

	// 给 validate job 添加 packages: write（validate 不应持有此权限）
	validate := requireMappingValue(t, jobs2, "validate")
	validatePerms := requireMappingValue(t, validate, "permissions")
	appendMappingValue(t, validatePerms, "packages", scalarNode("write"))

	body2, err := yaml.Marshal(root2)
	if err != nil {
		t.Fatalf("marshal workflow: %v", err)
	}
	err = checkWorkflow(body2)
	if err == nil {
		t.Fatal("policy accepted workflow where validate job has packages: write")
	}
}

// TestContainerPublishJobAlwaysPublishesExactVersionTag 断言 publish_container
// 始终发布 exact-version tag（vX.Y.Z），不跳过或条件化版本 tag。
// 这是 release consistency boundary 的要求：每 release 必须有 exact-version tag。
func TestContainerPublishJobAlwaysPublishesExactVersionTag(t *testing.T) {
	t.Parallel()

	t.Run("accepts exact-version tag push", func(t *testing.T) {
		t.Parallel()
		root := releaseWorkflowRoot(t)
		jobs := requireMappingValue(t, root, "jobs")
		addBuildContainerJob(t, jobs)
		publishContainer := containerPublishJobFixture()
		appendMappingValue(t, publishContainer, "name", scalarNode("Publish container image"))
		appendMappingValue(t, publishContainer, "needs", sequenceNode("build_container", "publish"))
		appendMappingValue(t, publishContainer, "runs-on", scalarNode("ubuntu-24.04"))
		appendMappingValue(t, publishContainer, "permissions", mappingNode(
			"contents", scalarNode("read"),
			"packages", scalarNode("write"),
		))
		appendMappingValue(t, publishContainer, "steps", &yaml.Node{
			Kind: yaml.SequenceNode,
			Tag:  "!!seq",
			Content: []*yaml.Node{
				canonicalPublishCheckout(t),
				runStepNode("Publish exact-version tag", `docker push ghcr.io/flanchanxwo/pixiv-cli:${RELEASE_TAG}`),
			},
		})
		setJobNode(t, jobs, "publish_container", publishContainer)

		body, err := yaml.Marshal(root)
		if err != nil {
			t.Fatalf("marshal workflow: %v", err)
		}
		if err := checkWorkflow(body); err != nil {
			t.Fatalf("policy rejected workflow with exact-version tag push: %v", err)
		}
	})

	t.Run("rejects missing exact-version tag push", func(t *testing.T) {
		t.Parallel()
		root := releaseWorkflowRoot(t)
		jobs := requireMappingValue(t, root, "jobs")
		addBuildContainerJob(t, jobs)
		publishContainer := containerPublishJobFixture()
		appendMappingValue(t, publishContainer, "name", scalarNode("Publish container image"))
		appendMappingValue(t, publishContainer, "needs", sequenceNode("build_container", "publish"))
		appendMappingValue(t, publishContainer, "runs-on", scalarNode("ubuntu-24.04"))
		appendMappingValue(t, publishContainer, "permissions", mappingNode(
			"contents", scalarNode("read"),
			"packages", scalarNode("write"),
		))
		// steps 不包含 exact-version tag 推送——只有 latest
		appendMappingValue(t, publishContainer, "steps", &yaml.Node{
			Kind: yaml.SequenceNode,
			Tag:  "!!seq",
			Content: []*yaml.Node{
				runStepNode("Push latest only", `echo "no exact version"`),
			},
		})
		setJobNode(t, jobs, "publish_container", publishContainer)

		body, err := yaml.Marshal(root)
		if err != nil {
			t.Fatalf("marshal workflow: %v", err)
		}
		err = checkWorkflow(body)
		if err == nil || !strings.Contains(strings.ToLower(err.Error()), "exact-version") {
			t.Fatalf("policy error = %v, want rejection mentioning exact-version", err)
		}
	})
}

// TestContainerPublishJobRejectsRegistryPushRetryLoop 断言 GHCR 推送失败必须
// 保持 workflow failure；不允许用 retry loop 把暂时性或权限类错误伪装成成功。
func TestContainerPublishJobRejectsRegistryPushRetryLoop(t *testing.T) {
	t.Parallel()

	root := releaseWorkflowRoot(t)
	jobs := requireMappingValue(t, root, "jobs")
	addBuildContainerJob(t, jobs)
	publishContainer := containerPublishJobFixture()
	appendMappingValue(t, publishContainer, "name", scalarNode("Publish container image"))
	appendMappingValue(t, publishContainer, "needs", sequenceNode("build_container", "publish"))
	appendMappingValue(t, publishContainer, "runs-on", scalarNode("ubuntu-24.04"))
	appendMappingValue(t, publishContainer, "permissions", mappingNode(
		"contents", scalarNode("read"),
		"packages", scalarNode("write"),
	))
	appendMappingValue(t, publishContainer, "steps", &yaml.Node{
		Kind: yaml.SequenceNode,
		Tag:  "!!seq",
		Content: []*yaml.Node{
			runStepNode("Retry exact-version push", `for attempt in 1 2 3; do docker push ghcr.io/flanchanxwo/pixiv-cli:${RELEASE_TAG}; done`),
		},
	})
	setJobNode(t, jobs, "publish_container", publishContainer)

	body, err := yaml.Marshal(root)
	if err != nil {
		t.Fatalf("marshal workflow: %v", err)
	}
	err = checkWorkflow(body)
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "retry") {
		t.Fatalf("policy error = %v, want rejection mentioning retry", err)
	}
}

// TestContainerPublishJobLatestTagOnlyForStable 断言 latest tag 仅在 stable
// release 时推进；prerelease 不移动 latest。无条件推送会被 policy 拒绝。
func TestContainerPublishJobLatestTagOnlyForStable(t *testing.T) {
	t.Parallel()

	root := releaseWorkflowRoot(t)
	jobs := requireMappingValue(t, root, "jobs")
	addBuildContainerJob(t, jobs)
	// 添加一个 publish_container，无条件推进 latest（不区分 stable/prerelease）
	publishContainer := containerPublishJobFixture()
	appendMappingValue(t, publishContainer, "name", scalarNode("Publish container image"))
	appendMappingValue(t, publishContainer, "needs", sequenceNode("build_container", "publish"))
	appendMappingValue(t, publishContainer, "runs-on", scalarNode("ubuntu-24.04"))
	appendMappingValue(t, publishContainer, "permissions", mappingNode(
		"contents", scalarNode("read"),
		"packages", scalarNode("write"),
	))
	// steps 包含 exact-version tag 推送（满足 tag policy），
	// 但也包含无条件 latest tag 推送——应被拒绝
	appendMappingValue(t, publishContainer, "steps", &yaml.Node{
		Kind: yaml.SequenceNode,
		Tag:  "!!seq",
		Content: []*yaml.Node{
			runStepNode("Publish exact-version tag", `docker push ghcr.io/flanchanxwo/pixiv-cli:${RELEASE_TAG}`),
			runStepNode("Push latest unconditionally", `docker push ghcr.io/flanchanxwo/pixiv-cli:latest`),
		},
	})
	setJobNode(t, jobs, "publish_container", publishContainer)

	body, err := yaml.Marshal(root)
	if err != nil {
		t.Fatalf("marshal workflow: %v", err)
	}
	err = checkWorkflow(body)
	if err == nil {
		t.Fatal("policy accepted unconditionally pushing latest tag — must require stable-only gate")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "latest") {
		t.Fatalf("policy error = %v, want rejection mentioning latest", err)
	}
}

// TestContainerPublishJobPermissionsAreMinimal 断言发布 job 只能持有 GHCR 推送
// 所需的最小权限；任何额外写权限都会扩大 post-Release 恢复路径的爆炸半径。
func TestContainerPublishJobPermissionsAreMinimal(t *testing.T) {
	t.Parallel()

	root := releaseWorkflowRoot(t)
	jobs := requireMappingValue(t, root, "jobs")
	addBuildContainerJob(t, jobs)
	publishContainer := containerPublishJobFixture()
	appendMappingValue(t, publishContainer, "name", scalarNode("Publish container image"))
	appendMappingValue(t, publishContainer, "needs", sequenceNode("build_container", "publish"))
	appendMappingValue(t, publishContainer, "runs-on", scalarNode("ubuntu-24.04"))
	appendMappingValue(t, publishContainer, "permissions", mappingNode(
		"contents", scalarNode("write"),
		"packages", scalarNode("write"),
	))
	appendMappingValue(t, publishContainer, "steps", &yaml.Node{
		Kind: yaml.SequenceNode,
		Tag:  "!!seq",
		Content: []*yaml.Node{
			canonicalPublishCheckout(t),
			runStepNode("Publish exact-version tag", `docker push ghcr.io/flanchanxwo/pixiv-cli:${RELEASE_TAG}`),
		},
	})
	setJobNode(t, jobs, "publish_container", publishContainer)

	body, err := yaml.Marshal(root)
	if err != nil {
		t.Fatalf("marshal workflow: %v", err)
	}
	err = checkWorkflow(body)
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "minimal") {
		t.Fatalf("policy error = %v, want rejection mentioning minimal publish permissions", err)
	}
}

// TestContainerPublishJobChecksOutImmutableTag 断言 post-Release 分类和推送
// 也必须绑定触发 release 的不可变 tag，不能从 movable ref 恢复 registry 状态。
func TestContainerPublishJobChecksOutImmutableTag(t *testing.T) {
	t.Parallel()

	root := releaseWorkflowRoot(t)
	jobs := requireMappingValue(t, root, "jobs")
	addBuildContainerJob(t, jobs)
	publishContainer := containerPublishJobFixture()
	appendMappingValue(t, publishContainer, "name", scalarNode("Publish container image"))
	appendMappingValue(t, publishContainer, "needs", sequenceNode("build_container", "publish"))
	appendMappingValue(t, publishContainer, "runs-on", scalarNode("ubuntu-24.04"))
	appendMappingValue(t, publishContainer, "permissions", mappingNode(
		"contents", scalarNode("read"),
		"packages", scalarNode("write"),
	))
	// 故意没有 canonical checkout——policy 必须 fail closed。
	appendMappingValue(t, publishContainer, "steps", &yaml.Node{
		Kind: yaml.SequenceNode,
		Tag:  "!!seq",
		Content: []*yaml.Node{
			runStepNode("Publish exact-version tag", `docker push ghcr.io/flanchanxwo/pixiv-cli:${RELEASE_TAG}`),
		},
	})
	setJobNode(t, jobs, "publish_container", publishContainer)

	body, err := yaml.Marshal(root)
	if err != nil {
		t.Fatalf("marshal workflow: %v", err)
	}
	err = checkWorkflow(body)
	if err == nil || !strings.Contains(err.Error(), "immutable release tag") {
		t.Fatalf("policy error = %v, want rejection requiring immutable release tag checkout", err)
	}
}

// TestContainerPublishJobRequiresNewestStableGate 断言 latest 只能由仍是最新
// stable 的 immutable tag 推进；旧 stable rerun 不得把 latest 回滚。
func TestContainerPublishJobRequiresNewestStableGate(t *testing.T) {
	t.Parallel()

	root := releaseWorkflowRoot(t)
	jobs := requireMappingValue(t, root, "jobs")
	addBuildContainerJob(t, jobs)
	publishContainer := containerPublishJobFixture()
	appendMappingValue(t, publishContainer, "name", scalarNode("Publish container image"))
	appendMappingValue(t, publishContainer, "needs", sequenceNode("build_container", "publish"))
	appendMappingValue(t, publishContainer, "runs-on", scalarNode("ubuntu-24.04"))
	appendMappingValue(t, publishContainer, "permissions", mappingNode(
		"contents", scalarNode("read"),
		"packages", scalarNode("write"),
	))
	appendMappingValue(t, publishContainer, "steps", &yaml.Node{
		Kind: yaml.SequenceNode,
		Tag:  "!!seq",
		Content: []*yaml.Node{
			canonicalPublishCheckout(t),
			runStepNode("Publish exact-version tag", `docker push ghcr.io/flanchanxwo/pixiv-cli:${RELEASE_TAG}`),
			runStepNode("Advance latest only for a stable release", `channel=stable; case "$channel" in stable) docker manifest push ghcr.io/flanchanxwo/pixiv-cli:latest ;; esac`),
		},
	})
	setJobNode(t, jobs, "publish_container", publishContainer)

	body, err := yaml.Marshal(root)
	if err != nil {
		t.Fatalf("marshal workflow: %v", err)
	}
	err = checkWorkflow(body)
	if err == nil || !strings.Contains(err.Error(), "newest stable") {
		t.Fatalf("policy error = %v, want rejection requiring newest stable comparison", err)
	}
}
