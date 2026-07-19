package main

import (
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/scripts/internal/workflowpolicy"
	"gopkg.in/yaml.v3"
)

func TestCheckWorkflowRequiresHomebrewReleaseGate(t *testing.T) {
	t.Parallel()

	root := releaseWorkflowRoot(t)
	for _, name := range []string{"render_homebrew_formula", "verify_homebrew_formula", "deploy_homebrew_tap"} {
		if _, ok := workflowpolicy.MappingValue(requireMappingValue(t, root, "jobs"), name); !ok {
			t.Errorf("release workflow is missing %q job", name)
		}
	}
	if err := checkWorkflow(mustMarshalYAML(t, root)); err != nil {
		t.Fatalf("release workflow policy rejected Homebrew release gate: %v", err)
	}
}

// Homebrew 6 不接受任意 workspace formula 路径；四平台门禁必须通过隔离 staging tap
// 的 tap-qualified 标识符安装。
func TestCheckWorkflowRequiresTapQualifiedStagingFormulaInstall(t *testing.T) {
	t.Parallel()

	root := releaseWorkflowRoot(t)
	step := stepWithRun(t, jobNode(t, root, "verify_homebrew_formula"), "brew install")
	replaceRunFragment(t, step, "brew install --debug-symbols --verbose --formula \"$staging_tap/$formula_name\"", "brew install --debug-symbols --verbose --formula \"staging-formula/$formula_name.rb\"")
	err := checkWorkflow(mustMarshalYAML(t, root))
	if err == nil || !strings.Contains(err.Error(), "Homebrew native install gate must use the required direct command sequence") {
		t.Fatalf("policy error = %v, want workspace-path Homebrew formula install rejection", err)
	}
}

// Homebrew 6 默认要求 tap trust；staging tap 只在 runner 本地临时存在，仍必须通过
// `brew trust --tap` 显式登记，不能用环境变量或 developer mode 绕过。
func TestCheckWorkflowRequiresStagingTapTrust(t *testing.T) {
	t.Parallel()

	root := releaseWorkflowRoot(t)
	step := stepWithRun(t, jobNode(t, root, "verify_homebrew_formula"), "brew trust --tap")
	removeRunFragment(t, step, "brew trust --tap \"$staging_tap\"")
	err := checkWorkflow(mustMarshalYAML(t, root))
	if err == nil || !strings.Contains(err.Error(), "Homebrew native install gate must use the required direct command sequence") {
		t.Fatalf("policy error = %v, want untrusted staging tap install rejection", err)
	}
}

// Linuxbrew 的 Resource staging 在 --keep-tmp 生效前会触发 Mktemp cleanup，且已在
// 短命 GitHub runner 上报出 chmod EINVAL。--debug-symbols 会让 Resource staging 的
// source cache 保留；它只适用于 Linux 发布验收，macOS 保持原本的安装命令。
func TestWorkflowUsesLinuxOnlyDebugSymbolsForHomebrewResourceStaging(t *testing.T) {
	t.Parallel()

	root := releaseWorkflowRoot(t)
	step := stepWithRun(t, jobNode(t, root, "verify_homebrew_formula"), "brew install")
	run := requireMappingValue(t, step, "run").Value
	for _, command := range []string{
		`eval "$(/home/linuxbrew/.linuxbrew/bin/brew shellenv)"`,
		`if [ '${{ matrix.os }}' = linux ]; then
  brew install --debug-symbols --verbose --formula "$staging_tap/$formula_name"
else
  brew install --formula "$staging_tap/$formula_name"
fi`,
	} {
		if !strings.Contains(run, command) {
			t.Fatalf("Homebrew install gate must retain Resource staging sources only on Linux and preserve the macOS install command; missing %q", command)
		}
	}
	for _, forbidden := range []string{"HOMEBREW_TEMP", "homebrew_temp=", "mkdir -p \"$homebrew_temp\"", "--keep-tmp"} {
		if strings.Contains(run, forbidden) {
			t.Fatalf("Homebrew install gate must not override Homebrew's temporary directory; found %q", forbidden)
		}
	}
}

func TestCheckWorkflowRejectsHomebrewReleaseGateMutations(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name   string
		want   string
		mutate func(t *testing.T, root *yaml.Node)
	}{
		{
			name: "render bypasses published checksums",
			want: "render_homebrew_formula job: needs must equal \"publish\"",
			mutate: func(t *testing.T, root *yaml.Node) {
				requireMappingValue(t, jobNode(t, root, "render_homebrew_formula"), "needs").Value = "verify_release_source"
			},
		},
		{
			name: "install bypasses rendered formula",
			want: "verify_homebrew_formula job: needs must equal \"render_homebrew_formula\"",
			mutate: func(t *testing.T, root *yaml.Node) {
				requireMappingValue(t, jobNode(t, root, "verify_homebrew_formula"), "needs").Value = "publish"
			},
		},
		{
			name: "tap deploy bypasses install matrix",
			want: "deploy_homebrew_tap job: needs must equal \"verify_homebrew_formula\"",
			mutate: func(t *testing.T, root *yaml.Node) {
				requireMappingValue(t, jobNode(t, root, "deploy_homebrew_tap"), "needs").Value = "render_homebrew_formula"
			},
		},
		{
			name: "publish exports a different checksum file",
			want: "publish job must upload the verified release/checksums.txt after publishing",
			mutate: func(t *testing.T, root *yaml.Node) {
				steps := requireMappingValue(t, jobNode(t, root, "publish"), "steps")
				upload := steps.Content[len(steps.Content)-1]
				requireMappingValue(t, requireMappingValue(t, upload, "with"), "path").Value = "dist/checksums.txt"
			},
		},
		{
			name: "publish mutates checksums after Release verification",
			want: "must preserve the verified asset set before exporting checksums",
			mutate: func(t *testing.T, root *yaml.Node) {
				step := stepWithRun(t, jobNode(t, root, "publish"), "gh release edit")
				replaceRunFragment(t, step, "gh release edit \"$RELEASE_TAG\" --draft=false", "printf tampered > release/checksums.txt\ngh release edit \"$RELEASE_TAG\" --draft=false")
			},
		},
		{
			name: "checksum artifact is not immediately downstream of publication",
			want: "must upload the verified release/checksums.txt after publishing",
			mutate: func(t *testing.T, root *yaml.Node) {
				steps := requireMappingValue(t, jobNode(t, root, "publish"), "steps")
				steps.Content = append(steps.Content, runStepNode("Intervening mutation opportunity", "true"))
			},
		},
		{
			name: "render downloads a different artifact",
			want: "verified checksums download must be the exact pinned action step",
			mutate: func(t *testing.T, root *yaml.Node) {
				step := requireMappingValue(t, jobNode(t, root, "render_homebrew_formula"), "steps").Content[2]
				requireMappingValue(t, requireMappingValue(t, step, "with"), "name").Value = "unverified-checksums"
			},
		},
		{
			name: "stable channel renders beta formula",
			want: "Homebrew formula render step must use the required direct command sequence",
			mutate: func(t *testing.T, root *yaml.Node) {
				replaceRunFragment(t, stepWithRun(t, jobNode(t, root, "render_homebrew_formula"), "formula_name=pixiv-cli"), "formula_name=pixiv-cli\n", "formula_name=pixiv-cli-beta\n")
			},
		},
		{
			name: "formula output artifact renamed",
			want: "staging formula artifact must be the exact pinned action step",
			mutate: func(t *testing.T, root *yaml.Node) {
				steps := requireMappingValue(t, jobNode(t, root, "render_homebrew_formula"), "steps")
				upload := steps.Content[len(steps.Content)-1]
				requireMappingValue(t, requireMappingValue(t, upload, "with"), "name").Value = "mutable-formula"
			},
		},
		{
			name: "native install matrix loses arm64 Linux",
			want: "matrix must contain exactly the four native targets",
			mutate: func(t *testing.T, root *yaml.Node) {
				strategy := requireMappingValue(t, jobNode(t, root, "verify_homebrew_formula"), "strategy")
				include := requireMappingValue(t, requireMappingValue(t, strategy, "matrix"), "include")
				requireMappingValue(t, include.Content[len(include.Content)-1], "runner").Value = "ubuntu-24.04"
			},
		},
		{
			name: "Linux Homebrew activation removed",
			want: "Homebrew native install gate must use the required direct command sequence",
			mutate: func(t *testing.T, root *yaml.Node) {
				removeRunFragment(t, stepWithRun(t, jobNode(t, root, "verify_homebrew_formula"), "/home/linuxbrew/.linuxbrew/bin/brew"), "eval \"$(/home/linuxbrew/.linuxbrew/bin/brew shellenv)\"")
			},
		},
		{
			name: "Linux Homebrew Resource staging debug symbols are removed",
			want: "Homebrew native install gate must use the required direct command sequence",
			mutate: func(t *testing.T, root *yaml.Node) {
				replaceRunFragment(t, stepWithRun(t, jobNode(t, root, "verify_homebrew_formula"), "brew install --debug-symbols"), "brew install --debug-symbols --verbose --formula \"$staging_tap/$formula_name\"", "brew install --verbose --formula \"$staging_tap/$formula_name\"")
			},
		},
		{
			name: "Linux Homebrew Resource staging retains obsolete keep tmp",
			want: "Homebrew native install gate must use the required direct command sequence",
			mutate: func(t *testing.T, root *yaml.Node) {
				replaceRunFragment(t, stepWithRun(t, jobNode(t, root, "verify_homebrew_formula"), "brew install --debug-symbols"), "brew install --debug-symbols --verbose --formula \"$staging_tap/$formula_name\"", "brew install --debug-symbols --keep-tmp --verbose --formula \"$staging_tap/$formula_name\"")
			},
		},
		{
			name: "Linux Homebrew staging install is not verbose",
			want: "Homebrew native install gate must use the required direct command sequence",
			mutate: func(t *testing.T, root *yaml.Node) {
				replaceRunFragment(t, stepWithRun(t, jobNode(t, root, "verify_homebrew_formula"), "brew install --debug-symbols"), "brew install --debug-symbols --verbose --formula \"$staging_tap/$formula_name\"", "brew install --debug-symbols --formula \"$staging_tap/$formula_name\"")
			},
		},
		{
			name: "Linux Homebrew debug symbols leave the Linux conditional",
			want: "Homebrew native install gate must use the required direct command sequence",
			mutate: func(t *testing.T, root *yaml.Node) {
				replaceRunFragment(t, stepWithRun(t, jobNode(t, root, "verify_homebrew_formula"), "brew install --debug-symbols"), `if [ '${{ matrix.os }}' = linux ]; then
  brew install --debug-symbols --verbose --formula "$staging_tap/$formula_name"
else
  brew install --formula "$staging_tap/$formula_name"
fi`, `brew install --debug-symbols --verbose --formula "$staging_tap/$formula_name"`)
			},
		},
		{
			name: "Linux Homebrew debug symbols leak to macOS",
			want: "Homebrew native install gate must use the required direct command sequence",
			mutate: func(t *testing.T, root *yaml.Node) {
				replaceRunFragment(t, stepWithRun(t, jobNode(t, root, "verify_homebrew_formula"), "brew install --debug-symbols"), "brew install --formula \"$staging_tap/$formula_name\"", "brew install --debug-symbols --verbose --formula \"$staging_tap/$formula_name\"")
			},
		},
		{
			name: "staging formula install is not tap qualified",
			want: "Homebrew native install gate must use the required direct command sequence",
			mutate: func(t *testing.T, root *yaml.Node) {
				replaceRunFragment(t, stepWithRun(t, jobNode(t, root, "verify_homebrew_formula"), "brew install --debug-symbols"), "brew install --debug-symbols --verbose --formula \"$staging_tap/$formula_name\"", "brew install --debug-symbols --verbose --formula \"staging-formula/$formula_name.rb\"")
			},
		},
		{
			name: "staging formula is copied into the trusted tap namespace",
			want: "Homebrew native install gate must use the required direct command sequence",
			mutate: func(t *testing.T, root *yaml.Node) {
				replaceRunFragment(t, stepWithRun(t, jobNode(t, root, "verify_homebrew_formula"), "staging_tap="), "staging_tap=pixiv-cli-release/staging", "staging_tap=FlanChanXwO/tap")
			},
		},
		{
			name: "staging tap trust is removed",
			want: "Homebrew native install gate must use the required direct command sequence",
			mutate: func(t *testing.T, root *yaml.Node) {
				removeRunFragment(t, stepWithRun(t, jobNode(t, root, "verify_homebrew_formula"), "brew trust --tap"), "brew trust --tap \"$staging_tap\"")
			},
		},
		{
			name: "staging tap trust is redirected",
			want: "Homebrew native install gate must use the required direct command sequence",
			mutate: func(t *testing.T, root *yaml.Node) {
				replaceRunFragment(t, stepWithRun(t, jobNode(t, root, "verify_homebrew_formula"), "brew trust --tap"), "brew trust --tap \"$staging_tap\"", "brew trust --tap FlanChanXwO/tap")
			},
		},
		{
			name: "version JSON assertion removed",
			want: "Homebrew native install gate must use the required direct command sequence",
			mutate: func(t *testing.T, root *yaml.Node) {
				removeRunFragment(t, stepWithRun(t, jobNode(t, root, "verify_homebrew_formula"), "pixiv version --json"), "pixiv version --json")
			},
		},
		{
			name: "install gate references a secret",
			want: "Homebrew render and install jobs must not reference secrets",
			mutate: func(t *testing.T, root *yaml.Node) {
				appendMappingValue(t, requireMappingValue(t, jobNode(t, root, "verify_homebrew_formula"), "steps").Content[1], "env", mappingNode("LEAK", scalarNode("${{ secrets.HOMEBREW_TAP_DEPLOY_KEY }}")))
			},
		},
		{
			name: "tap deploy is unprotected",
			want: "deploy_homebrew_tap environment must be release",
			mutate: func(t *testing.T, root *yaml.Node) {
				requireMappingValue(t, jobNode(t, root, "deploy_homebrew_tap"), "environment").Value = "unprotected"
			},
		},
		{
			name: "tap deploy runs after a failed install gate",
			want: "deploy_homebrew_tap job must not define if or continue-on-error",
			mutate: func(t *testing.T, root *yaml.Node) {
				appendMappingValue(t, jobNode(t, root, "deploy_homebrew_tap"), "if", scalarNode("always()"))
			},
		},
		{
			name: "tap deploy elevates token permission",
			want: "permissions must contain only contents: read",
			mutate: func(t *testing.T, root *yaml.Node) {
				permissions := requireMappingValue(t, jobNode(t, root, "deploy_homebrew_tap"), "permissions")
				requireMappingValue(t, permissions, "contents").Value = "write"
			},
		},
		{
			name: "tap secret is reachable before final push",
			want: "must not reference secrets outside its final push step",
			mutate: func(t *testing.T, root *yaml.Node) {
				steps := requireMappingValue(t, jobNode(t, root, "deploy_homebrew_tap"), "steps")
				appendMappingValue(t, steps.Content[1], "env", mappingNode("EARLY_KEY", scalarNode("${{ secrets.HOMEBREW_TAP_DEPLOY_KEY }}")))
			},
		},
		{
			name: "tap push adds an unrelated secret",
			want: "tap final push step must declare only HOMEBREW_TAP_DEPLOY_KEY",
			mutate: func(t *testing.T, root *yaml.Node) {
				steps := requireMappingValue(t, jobNode(t, root, "deploy_homebrew_tap"), "steps")
				env := requireMappingValue(t, steps.Content[len(steps.Content)-1], "env")
				appendMappingValue(t, env, "UNRELATED", scalarNode("${{ secrets.UNRELATED }}"))
			},
		},
		{
			name: "tap clone uses SSH before verification",
			want: "tap one-formula commit step must use the required direct command sequence",
			mutate: func(t *testing.T, root *yaml.Node) {
				replaceRunFragment(t, stepWithRun(t, jobNode(t, root, "deploy_homebrew_tap"), "git clone https://"), "https://github.com/FlanChanXwO/homebrew-tap.git", "git@github.com:FlanChanXwO/homebrew-tap.git")
			},
		},
		{
			name: "tap commit stages the whole repository",
			want: "tap one-formula commit step must use the required direct command sequence",
			mutate: func(t *testing.T, root *yaml.Node) {
				replaceRunFragment(t, stepWithRun(t, jobNode(t, root, "deploy_homebrew_tap"), "git -C \"$tap_dir\" add"), "git -C \"$tap_dir\" add -- \"Formula/$formula_name.rb\"", "git -C \"$tap_dir\" add -- .")
			},
		},
		{
			name: "strict host key checking disabled",
			want: "tap final protected push step must use the required direct command sequence",
			mutate: func(t *testing.T, root *yaml.Node) {
				replaceRunFragment(t, stepWithRun(t, jobNode(t, root, "deploy_homebrew_tap"), "StrictHostKeyChecking=yes"), "StrictHostKeyChecking=yes", "StrictHostKeyChecking=no")
			},
		},
		{
			name: "tap private key loses required trailing newline",
			want: "tap final protected push step must use the required direct command sequence",
			mutate: func(t *testing.T, root *yaml.Node) {
				replaceRunFragment(t, stepWithRun(t, jobNode(t, root, "deploy_homebrew_tap"), "HOMEBREW_TAP_DEPLOY_KEY"), "printf '%s\\n' \"$HOMEBREW_TAP_DEPLOY_KEY\"", "printf '%s' \"$HOMEBREW_TAP_DEPLOY_KEY\"")
			},
		},
		{
			name: "tap push changes the exact SSH remote",
			want: "tap final protected push step must use the required direct command sequence",
			mutate: func(t *testing.T, root *yaml.Node) {
				replaceRunFragment(t, stepWithRun(t, jobNode(t, root, "deploy_homebrew_tap"), "remote set-url"), "git@github.com:FlanChanXwO/homebrew-tap.git", "git@github.com:attacker/homebrew-tap.git")
			},
		},
		{
			name: "known hosts replaced by ssh-keyscan",
			want: "tap final protected push step must use the required direct command sequence",
			mutate: func(t *testing.T, root *yaml.Node) {
				replaceRunFragment(t, stepWithRun(t, jobNode(t, root, "deploy_homebrew_tap"), "UserKnownHostsFile="), "UserKnownHostsFile=$GITHUB_WORKSPACE/templates/homebrew/github.com-known-hosts", "UserKnownHostsFile=$RUNNER_TEMP/known_hosts; ssh-keyscan github.com")
			},
		},
		{
			name: "push does not target main from the verified commit",
			want: "tap final protected push step must use the required direct command sequence",
			mutate: func(t *testing.T, root *yaml.Node) {
				replaceRunFragment(t, stepWithRun(t, jobNode(t, root, "deploy_homebrew_tap"), "push origin HEAD:main"), "push origin HEAD:main", "push origin HEAD")
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := releaseWorkflowRoot(t)
			test.mutate(t, root)
			err := checkWorkflow(mustMarshalYAML(t, root))
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("policy error = %v, want rejection containing %q", err, test.want)
			}
		})
	}
}

// Task34 将 formula 渲染、四个平台实际安装和 tap 写入串成发布后的强制门禁；删除最终
// deploy job 必须被结构化策略拒绝，避免安装成功后 workflow 静默结束而未更新 tap。
func TestCheckWorkflowRequiresHomebrewTapPublicationGate(t *testing.T) {
	t.Parallel()

	root := releaseWorkflowRoot(t)
	removeMappingValue(t, requireMappingValue(t, root, "jobs"), "deploy_homebrew_tap")
	err := checkWorkflow(mustMarshalYAML(t, root))
	if err == nil || !strings.Contains(err.Error(), "workflow jobs") {
		t.Fatalf("policy error = %v, want missing Homebrew tap publication gate rejection", err)
	}
}
