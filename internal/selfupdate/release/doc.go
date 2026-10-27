// Package release 查询 GitHub Releases 并应用 SemVer 渠道策略。
//
// 包的组成：[GitHubReleaseClient] 由 [NewGitHubReleaseClient] 构造，[Release] 与
// [ReleaseAsset] 是 GitHub 数据，[ReleaseCache] 与 [ReleaseChecker] 是端口，
// [ReleaseCheckOptions] 与 [ReleaseCheckResult] 描述一次检查。
package release
