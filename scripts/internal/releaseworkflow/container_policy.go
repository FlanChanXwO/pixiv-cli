package releaseworkflow

import (
	"errors"
	"fmt"
	"strings"

	workflowyaml "github.com/FlanChanXwO/pixiv-cli/scripts/internal/workflow/yaml"
	"gopkg.in/yaml.v3"
)

// containerRegistry 是 GHCR 上 pixiv-cli 容器镜像的仓库路径。
const containerRegistry = "ghcr.io/flanchanxwo/pixiv-cli"

// containerMatrixTargets 绑定两个原生 Linux 容器构建 target 与其 release matrix identity，
// 复用 build_production 的 Linux target 定义，确保 ABI provenance 一致。
var containerMatrixTargets = map[string]struct{}{
	"ubuntu-22.04|linux|amd64|x86_64-unknown-linux-gnu|linux-amd64|gcc":      {},
	"ubuntu-22.04-arm|linux|arm64|aarch64-unknown-linux-gnu|linux-arm64|gcc": {},
}

// checkContainerBuildJob 校验 build_container job 的容器构建 contract：
//   - 不持有 packages: write（registry 写入仅由 publish_container 持有）
//   - checkout 不可变 tag（与其它 release job 一致，不接受 movable ref）
//   - 使用原生 Linux runner（不允许 QEMU cross-build）
//   - 不依赖 build_production（在共享质量门禁后与 build_production 并行，而非串行）
//   - 无 QEMU/setup-qemu 步骤
//
// 当 job 不存在时不报错——容器 job 是可选的；一旦存在就必须满足此 contract。
func checkContainerBuildJob(job *yaml.Node) error {
	if job == nil {
		return nil
	}
	if err := requireRequiredJobExecution(job, "build_container job"); err != nil {
		return err
	}
	if err := requireNoEnvironment(job, "build_container job"); err != nil {
		return err
	}
	if err := requireOnlyMappingKeys(job, "name", "needs", "runs-on", "permissions", "strategy", "steps"); err != nil {
		return fmt.Errorf("build_container job: %w", err)
	}
	// build_container 不持有 packages: write——registry 推送由 publish_container 独占。
	if err := requireContainerBuildPermissions(job); err != nil {
		return err
	}
	// build_container 必须与 build_production 并行：在共享质量门禁（build）之后启动，
	// 不能串行依赖 build_production。
	if err := requireContainerBuildNeeds(job); err != nil {
		return err
	}
	// build_container 必须使用原生 Linux runner（matrix），不接受固定非 Linux runner。
	if err := workflowyaml.RequireScalar(job, "runs-on", "${{ matrix.runner }}"); err != nil {
		return fmt.Errorf("build_container job must use the native matrix runner")
	}
	steps, err := jobSteps(job)
	if err != nil || len(steps) == 0 {
		return errors.New("build_container job must contain steps")
	}
	// 第一步必须是绑定不可变 tag 的 canonical checkout。
	// 容器构建源必须从不可变 release tag 出发，不接受 movable ref。
	if err := requireCanonicalCheckout(steps[0], "build_container job", checkoutWithRequirement{"fetch-depth", "0"}, checkoutWithRequirement{"persist-credentials", "false"}, checkoutWithRequirement{"ref", "${{ env.RELEASE_TAG }}"}); err != nil {
		return fmt.Errorf("build_container job must checkout the immutable release tag: %w", err)
	}
	// 禁止 QEMU/setup-qemu 步骤——容器构建必须使用原生 runner 而非 cross-build。
	if err := rejectQEMUSteps(steps); err != nil {
		return err
	}
	// 验证 matrix 只含两个原生 Linux target。
	strategy, ok := workflowyaml.MappingValue(job, "strategy")
	if !ok || requireOnlyMappingKeys(strategy, "fail-fast", "matrix") != nil || workflowyaml.RequireScalar(strategy, "fail-fast", "false") != nil {
		return errors.New("build_container strategy must contain only fail-fast: false and the container matrix")
	}
	matrix := mustMappingPath(job, "strategy", "matrix")
	if matrix == nil || checkContainerMatrix(matrix) != nil {
		return errors.New("build_container matrix must contain exactly the two native Linux targets")
	}
	return nil
}

// checkContainerPublishJob 校验 publish_container job 的 GHCR 发布 contract：
//   - 持有 packages: write（registry 推送唯一持权 job）
//   - 依赖 build_container 和 publish（容器 artifact 已验证且 GitHub Release 已发布）
//
// 当 job 不存在时不报错——容器 job 是可选的；一旦存在就必须满足此 contract。
func checkContainerPublishJob(job *yaml.Node) error {
	if job == nil {
		return nil
	}
	if err := requireRequiredJobExecution(job, "publish_container job"); err != nil {
		return err
	}
	if err := requireOnlyMappingKeys(job, "name", "needs", "runs-on", "permissions", "steps"); err != nil {
		return fmt.Errorf("publish_container job: %w", err)
	}
	// publish_container 必须持有 packages: write——这是唯一允许 registry 推送的权限。
	if err := requireContainerPublishPermissions(job); err != nil {
		return err
	}
	// publish_container 必须依赖 build_container 和 publish：前者提供两架构已验证
	// artifact，后者保证 GHCR 推送是 GitHub Release 之后的可重跑恢复路径。
	if err := requireContainerPublishNeeds(job); err != nil {
		return err
	}
	// publish_container 的 steps 必须包含 exact-version tag 推送（${RELEASE_TAG}）。
	if err := requireExactVersionTagPush(job); err != nil {
		return err
	}
	// latest tag 推送必须受 stable-only gate 约束，不能无条件推送。
	if err := rejectUnconditionalLatestTagPush(job); err != nil {
		return err
	}
	return nil
}

// requireContainerBuildPermissions 确保 build_container 的 permissions 只含 contents: read，
// 不含 packages: write。
func requireContainerBuildPermissions(job *yaml.Node) error {
	permissions, ok := workflowyaml.MappingValue(job, "permissions")
	if !ok || permissions.Kind != yaml.MappingNode {
		return errors.New("build_container job must declare permissions")
	}
	if err := requireOnlyMappingKeys(permissions, "contents"); err != nil {
		return errors.New("build_container permissions must not include packages: write")
	}
	if err := workflowyaml.RequireScalar(permissions, "contents", "read"); err != nil {
		return errors.New("build_container permissions must not include packages: write")
	}
	return nil
}

// requireContainerBuildNeeds 确保 build_container 依赖 build（共享质量门禁），
// 而非 build_production（串行依赖会破坏并行性）。
func requireContainerBuildNeeds(job *yaml.Node) error {
	needs, ok := workflowyaml.MappingValue(job, "needs")
	if !ok || needs.Kind != yaml.SequenceNode {
		return errors.New("build_container job must depend on build")
	}
	for _, dep := range needs.Content {
		if dep.Kind != yaml.ScalarNode {
			return errors.New("build_container job must depend on build")
		}
		if dep.Value == "build_production" {
			return errors.New("build_container job must not depend on build_production")
		}
	}
	// 必须依赖 build（共享质量门禁）
	foundBuild := false
	for _, dep := range needs.Content {
		if dep.Value == "build" {
			foundBuild = true
		}
	}
	if !foundBuild {
		return errors.New("build_container job must depend on build")
	}
	return nil
}

// requireContainerPublishPermissions 确保 publish_container 持有 packages: write。
func requireContainerPublishPermissions(job *yaml.Node) error {
	permissions, ok := workflowyaml.MappingValue(job, "permissions")
	if !ok || permissions.Kind != yaml.MappingNode {
		return errors.New("publish_container job must declare packages: write")
	}
	packages, ok := workflowyaml.MappingValue(permissions, "packages")
	if !ok || packages.Kind != yaml.ScalarNode || packages.Value != "write" {
		return errors.New("publish_container job must declare packages: write")
	}
	return nil
}

// requireContainerPublishNeeds 确保 publish_container 依赖 build_container。
func requireContainerPublishNeeds(job *yaml.Node) error {
	needs, ok := workflowyaml.MappingValue(job, "needs")
	if !ok {
		return errors.New("publish_container job must depend on build_container")
	}
	var deps []string
	if needs.Kind == yaml.ScalarNode {
		deps = append(deps, needs.Value)
	} else if needs.Kind == yaml.SequenceNode {
		for _, dep := range needs.Content {
			if dep.Kind == yaml.ScalarNode {
				deps = append(deps, dep.Value)
			}
		}
	}
	foundBuildContainer := false
	foundGitHubRelease := false
	for _, dep := range deps {
		switch dep {
		case "build_container":
			foundBuildContainer = true
		case "publish":
			foundGitHubRelease = true
		}
	}
	if !foundBuildContainer || !foundGitHubRelease {
		return errors.New("publish_container job must depend on build_container and publish")
	}
	return nil
}

// requirePublishAfterContainerBuild 保证 GitHub Release 发布等待已验证容器 artifact。
// GitHub Release 与 GHCR 虽然无法原子提交，但 Release 不得先于容器构建完成；
// GHCR 推送失败仍可通过重跑 publish_container 恢复。
func requirePublishAfterContainerBuild(publish, buildContainer *yaml.Node) error {
	if buildContainer == nil {
		return nil
	}
	needs, ok := workflowyaml.MappingValue(publish, "needs")
	if !ok || needs.Kind != yaml.SequenceNode {
		return errors.New("publish job must depend on build_container for verified container artifacts")
	}
	for _, dep := range needs.Content {
		if dep.Kind == yaml.ScalarNode && dep.Value == "build_container" {
			return nil
		}
	}
	return errors.New("publish job must depend on build_container for verified container artifacts")
}

// requireExactVersionTagPush 确保 publish_container 的 steps 中包含推送
// exact-version tag（使用 ${RELEASE_TAG}）的步骤。每 release 必须发布
// exact-version container tag，这是 release consistency boundary 的要求。
func requireExactVersionTagPush(job *yaml.Node) error {
	steps, err := jobSteps(job)
	if err != nil {
		return errors.New("publish_container job must push the exact-version tag")
	}
	for _, step := range steps {
		run, ok := workflowyaml.MappingValue(step, "run")
		if !ok || run.Kind != yaml.ScalarNode {
			continue
		}
		if strings.Contains(run.Value, "docker push") && strings.Contains(run.Value, "${RELEASE_TAG}") {
			return nil
		}
	}
	return errors.New("publish_container job must always push the exact-version tag")
}

// rejectUnconditionalLatestTagPush 确保如果 publish_container 推送 latest tag，
// 该推送必须受 stable-only gate 约束（使用 channel classifier 区分 stable/prerelease），
// 而非无条件推送。latest tag 只在 stable release 时推进。
func rejectUnconditionalLatestTagPush(job *yaml.Node) error {
	steps, err := jobSteps(job)
	if err != nil {
		return nil
	}
	for _, step := range steps {
		run, ok := workflowyaml.MappingValue(step, "run")
		if !ok || run.Kind != yaml.ScalarNode {
			continue
		}
		if !strings.Contains(run.Value, "latest") {
			continue
		}
		// 如果步骤包含 latest 但没有 stable gate（channel classifier），
		// 则是无条件推送，应被拒绝。
		hasStableGate := strings.Contains(run.Value, "stable") || strings.Contains(run.Value, "channel") || strings.Contains(run.Value, "prerelease")
		if !hasStableGate {
			return errors.New("publish_container job must not push latest tag unconditionally (requires stable-only gate)")
		}
	}
	return nil
}

// rejectQEMUSteps 禁止任何 QEMU/setup-qemu action 步骤。
func rejectQEMUSteps(steps []*yaml.Node) error {
	for _, step := range steps {
		uses, ok := workflowyaml.MappingValue(step, "uses")
		if !ok || uses.Kind != yaml.ScalarNode {
			continue
		}
		if strings.Contains(strings.ToLower(uses.Value), "qemu") {
			return errors.New("build_container job must not use QEMU setup")
		}
	}
	return nil
}

// checkContainerMatrix 验证容器 matrix 只含两个原生 Linux target。
func checkContainerMatrix(matrix *yaml.Node) error {
	if err := requireOnlyMappingKeys(matrix, "include"); err != nil {
		return errors.New("container matrix must contain only the two native Linux targets")
	}
	include, ok := workflowyaml.MappingValue(matrix, "include")
	if !ok || include.Kind != yaml.SequenceNode || len(include.Content) != len(containerMatrixTargets) {
		return errors.New("container matrix must contain exactly the two native Linux targets")
	}
	seen := make(map[string]struct{}, len(include.Content))
	for _, entry := range include.Content {
		if entry.Kind != yaml.MappingNode {
			return errors.New("container matrix must contain exactly the two native Linux targets")
		}
		if err := requireOnlyMappingKeys(entry, "runner", "goos", "goarch", "rust_target", "rust_toolchain", "artifact", "cc"); err != nil {
			return errors.New("container matrix entries must contain only the canonical release target fields")
		}
		fields := make([]string, 0, 6)
		for _, key := range []string{"runner", "goos", "goarch", "rust_target", "artifact", "cc"} {
			value, ok := workflowyaml.MappingValue(entry, key)
			if !ok || value.Kind != yaml.ScalarNode {
				return errors.New("container matrix must contain exactly the two native Linux targets")
			}
			fields = append(fields, value.Value)
		}
		identity := strings.Join(fields, "|")
		if _, expected := containerMatrixTargets[identity]; !expected {
			return errors.New("container matrix must contain exactly the two native Linux targets")
		}
		if _, duplicate := seen[identity]; duplicate {
			return errors.New("container matrix must contain exactly the two native Linux targets")
		}
		seen[identity] = struct{}{}
	}
	return nil
}
