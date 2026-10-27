// Package citest 覆盖仓库 CI 依赖的发布契约：安装器、change scope 分类、验证预过滤、
// action pin 形式和容器镜像。
//
// 包的组成：containerrelease 检查 Dockerfile 与 container publisher，installers 执行
// install.sh 与 install.cmd，prverificationworkflow 执行分类脚本与 /test 预过滤，
// workflowpins 校验 action pin 形式。目录里没有生产代码，这些测试都以真实脚本、真实
// YAML 与真实安装器为对象。
package citest
