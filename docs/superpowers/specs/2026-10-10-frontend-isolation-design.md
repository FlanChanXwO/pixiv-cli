# 前端源码统一隔离设计

状态：用户已批准并要求实施。保持现有页面行为、MCP 协议、URI、HTTP 路由和单文件 CLI 分发。

## 源码和 owner

Gallery 的 HTML、CSS、JS 和 Node fixture 位于 `frontend/mcp-apps/gallery/`；登录模板和 CSS 位于 `frontend/pages/login/`；owner 授权同意页模板和 CSS 位于 `frontend/pages/oauth/`。不引入框架、npm、Web UI 或 Wails 工程，Homebrew 模板保留原位置。

Go 保留资源注册、模板数据、html/template 转义、CSP、媒体 origin 注入、canonical action、CSRF 和授权状态。三个现有 owner 的 `generated/` 目录通过 go:embed 嵌入构建产物，精确忽略且不提交。

## 生成契约

标准库命令 `go run ./scripts/cmd/frontendassets` 使用固定映射。Gallery 以唯一的样式及脚本占位符内联 CSS/JS；授权页内联 CSS；登录模板及 CSS 原字节复制。先读取并验证所有输入再写产物，缺失输入、重复或缺失占位符明确失败。生成确定且可重复；`--check` 验证缺失或过期产物，不写文件。

## 构建链

干净检出必须先生成，再测试、vet 或编译。build、build-platform、race 脚本和 Quality、Release、Platform 工作流接入生成；交叉编译前以宿主 GOOS/GOARCH 运行生成器。生成资源会被 release 的干净产物检查显式排除，其他机器产物仍禁止。frontend 和生成工具的改动触发完整 Quality、Platform、Container 分类。Docker 仍只使用预构建二进制。

## 验收

先运行 Gallery、登录和授权基线。生成器以相关失败测试开始，覆盖干净生成、确定性、缺失输入、异常占位符和只读过期检查。Node fixture 直接执行 Gallery JS；资源测试验证内联内容及媒体 CSP。保留登录转义、callback 提交、授权 canonical action、CSRF、Allow/Deny、owner secret 脱敏回归。

从不含 generated 资源的临时源码检出验证生成、测试、构建和打包，再执行全量 test、race、vet、格式和构建检查。同步双语开发、架构文档和 AGENTS 验证顺序。保留工作树已有改动，不重启 MCP 服务或隧道。

## 实施前审核

主线程已核对三个 owner 和构建入口。安全边界不迁移到前端；生成器无运行时网络和外部依赖。Release 现有 `git clean -ndx` 检查必须仅排除三个生成目录，否则新生成资源会阻止发布。当前子代理 API 不支持用户要求的 fork_turns 参数，设计审核在主线程完成。
