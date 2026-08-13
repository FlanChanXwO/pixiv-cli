# 原生以图搜图（reverse image search）可行性探索与方案设计

> 状态：**探索完成，证据已实测，待实施（本文档只记录，不实施）**
> 日期：2026-08-10/11
> 范围：为 pixiv-cli 增加原生"以图搜图"能力 —— 搜图引擎调用、结果建模、pixiv 富化。
> 结论：**SauceNAO（必须 api_key）为主引擎 + ascii2d 为可选引擎（FlareSolverr 本地/自部署云 + Firecrawl 低频云）**；ascii2d 本地文件上传需压缩到 FlareSolverr 1MB 硬顶内。

---

## 1. 背景与目标

pixiv-cli 目前是文本 CLI / MCP stdio server，无图片搜索能力。目标是为用户提供**原生级图片搜索**：输入一张图（或一个 pixiv 作品引用），输出结构化结果（相似度排序、命中索引、pixiv_id、作者、标题），并接入既有 `Record` / `pixiv.Artwork` 模型与富化能力。

区别于 WebView 类方案（如 Shaft 把搜图站塞进浏览器）：本方案做**原生数据管道** —— 结构化 JSON、相似度过滤、pixiv 富化、CLI/MCP 可编程消费。

## 2. 实测证据（2026-08-10/11）

### 2.1 SauceNAO

| 测试 | 结果 |
|---|---|
| `GET /search.php?api_key=…&url=…&output_type=2` | ✅ **200 JSON**，无 CF 挑战，3/3 稳定 |
| `db=5,6,51,52,53`（pixiv 家族）过滤 | ✅ 有效，返回 `pixiv_id`/`member_id`/`member_name`/`title`/`ext_urls`/`similarity` |
| multipart 文件上传 + `output_type=2` | ✅ **200 JSON**（**必须带 `output_type=2`**，否则返回 HTML "Sauce Found?" 页） |
| **匿名（无 api_key）** | ❌ **403 JSON：`{"status":-1,"message":"The anonymous account type does not permit API usage."}`** —— **策略上禁止匿名 API** |
| 匿名 URL 搜索 | ❌ 403 `cf-mitigated: challenge` |
| 限流 | `short_limit: 4/30s`（共享 IP 池）、`long_limit: 100/6h`；429 返回 HTML 非 JSON，须按状态码/头分类 |
| 上传大小 | 15MB 上限（Shaft 源码注释确认） |

**结论**：SauceNAO 必须 api_key。免费档够个人低频，5000/天为付费档。上传不压图、直传最稳。

### 2.2 ascii2d —— 全部自动化通道实测

| 通道 | 结果 |
|---|---|
| 原生 curl（GET `/search/url/`、POST `/search/uri` 带 CSRF） | ❌ 403 `cf-mitigated: challenge`（body 含 `managed` + `challenge-platform`） |
| curl_cffi 指纹伪造（chrome110/124/safari15_5）× 直连 + 本地 `:7890` 代理 | ❌ 全 403 |
| **FlareSolverr 3.5.0**（GET/POST，60-70s 超时） | ❌ 超时解不开 |
| **FlareSolverr 3.5.0 + 长超时 180s + 持久 session** | ✅ **3 次独立复现成功**（~17s 解挑战，热 session 后 2-3s/次） |
| CDP 真实 Chrome / Playwright 真浏览器 | ❌ 卡"请稍候" challenge |
| **Cloudflare Worker 反代**（CF 自家出口 + 完整浏览器头） | ❌ 首页 500（ascii2d 应用层封 CF IP）、搜索 403 challenge |
| **ScraperAPI 免费档**（render/premium/follow_redirects） | ❌ 对 CF 保护域直接 500（danbooru 同） |
| **Firecrawl**（浏览器渲染 + 代理池） | ✅ **2/2 复现穿透**（URL 搜索，返回真实结果页，含 pixiv 链接） |
| 真人手动浏览器 | ✅ 唯一确定可用 |

**关键结论**：
- ascii2d 是**双层防护**：CF managed 质询（`/search/*`）+ 应用层封 CF 出口 IP。
- **唯一免费且可自动化**的通道是 **FlareSolverr（真 Chromium 解 JS 质询）+ 自建/非 CF 出口**；**云免费**通道是 **Firecrawl**（浏览器渲染 + 自有代理池）。
- 文件上传端点 `/search/file` 真实存在（multipart：`utf8` + `authenticity_token` + `file`），**小图（<1MB）FlareSolverr 上传成功**；大图被 FlareSolverr 413 硬顶拦截。

### 2.3 FlareSolverr 413 上限

- **不可配置**：FlareSolverr 用 bottle 框架，`MEMFILE_MAX` **硬编码 1MB**（无 ENV/CLI/配置暴露），waitress 无更大上限接口。
- 改上限需 fork 源码自己 build 镜像 —— 不划算。
- **解法**：客户端本地压缩到 <1MB 再上传（ascii2d 是特征匹配，压缩不影响搜索质量到 1600px 档）。

### 2.4 竞品/社区印证

- **Shaft（CeuiLiSA/Pixiv-Shaft）** 源码 `ReverseImage.java`：只内置 SauceNAO + ascii2d 两个引擎，`DEFAULT_ENGINE = SauceNao`。注释记录**它曾用 OkHttp 原生上传，后因 Cloudflare JS 质询（#733）彻底走不通，退守 WebView 交给真浏览器** —— 印证 ascii2d/SauceNAO 上传端点的 CF 质询是行业性收紧。
- **PicImageSearch（718⭐）** #305：`curl_cffi + Cloudflare-Workers-Proxy` 曾可用（2025-06），但**本次实测 CF Worker 已被 ascii2d 应用层封 IP**，该通道已失效。
- **cq-picsearcher-bot（1599⭐）**：ascii2d 插件默认 `enableForAscii2d: false`，走 FlareSolverr（90s 超时）/自建 cf-bypasser —— 与我们的 FlareSolverr 通道一致。

## 3. 收敛后的架构

### 3.1 引擎与通道

```
pixiv source search <INPUT> --engine saucenao|ascii2d

saucenao:  原生直连（必须 api_key）        → 默认引擎，JSON API，抗大量，上传不压图
ascii2d:
  ├─ URL 搜索:  FlareSolverr 本地 :8191     → 大量/离线（docker run 一条命令，已验证）
  ├─ URL 搜索:  FlareSolverr 自部署云        → 大量/免费（Fly.io/Railway，无限量）
  ├─ URL 搜索:  Firecrawl Keyless/免费       → 低频/零安装（实测穿透，1000 次/月上限）
  └─ 文件上传:  本地压缩 <1MB → FlareSolverr multipart POST /search/file
```

- **Firecrawl 定位 = 零安装尝鲜通道，非主通道**（1000/月额度吃紧；大量使用走 FlareSolverr）。
- 客户端统一抽象为 `ImageSourceClient` 接口，本地/云 FlareSolverr 地址零差异，将来加通道只加 adapter。

### 3.2 CLI 输入形态（文本 CLI 用"引用"传图）

| 输入 | 解析 | 引擎支持 |
|---|---|---|
| pixiv 作品 ID（裸整数） | 复用 `parseEntityIDOrURL` 思路 | 全引擎 |
| pixiv 作品 URL | 现有 `sdk/pixiv.ParseURL` → `Reference{artwork, ID}` → 拉图 URL | 全引擎 |
| 任意图片 URL | http(s) 前缀 → 直接搜索 | 全引擎 |
| 本地文件路径 | 文件存在 + 图片扩展名 → 读字节 | SauceNAO ✅ / ascii2d ⚠️ 需压缩 |
| stdin `-` | `io.ReadAll(os.Stdin)` | 同本地文件 |

- **pixiv ID/URL 是原生建模核心**：拉作品封面/缩略图 URL（`Artwork.Cover`，天然小图）→ 喂搜索引擎 → 结果接 `pixiv download`，整条流水线是 CLI 自己的对象。
- macOS 剪贴板（`--clipboard`）平台相关，**不在 MVP**。

### 3.3 本地文件处理

```
本地文件 + SauceNAO:  直传 multipart（15MB 上限内，永不压缩）→ 识别质量最高
本地文件 + ascii2d:
  ① 解码 → 若 ≤1MB: 直传（绝大多数识图输入本就 <1MB）
  ② 若 1-5MB: 缩放最长边 ~1600px + q85 → 必过 1MB，识别无影响
  ③ 若 >5MB: 警告识别率风险 + 建议 SauceNAO 直传（不硬压）
```

- **压缩算法**：解码归一化 → 档位缩放（最长边 1024 起，逐档减半）→ JPEG 质量递减（88→45）→ 每步检查 ≤1MB → 兜底 512px+q40（信息量固定，必然收敛）。
- **关键认识**：识图输入图通常本就是缩略图级（截图/聊天保存/平台压缩），≤1MB 是绝大多数情况；压缩只是兜底，不是主路径。
- 用 Go stdlib `image` + `golang.org/x/image/draw`（同 org，合理），零新第三方依赖。

### 3.4 代码结构（遵循既有边界规则）

```
sdk/saucenao            # 仿 sdk/fanbox：只依赖 sdk，Open/OpenWith，模型，SearchByURL/SearchByFile
                        #   newError via sdk.NewError(product="saucenao") 复用 sdk.Reason
                        #   from响应头动态读额度（short/long remaining）
sdk/ascii2d             # 仿 sdk/fanbox：FlareSolverr 协议通道 + Firecrawl 后端（统一 ImageSourceClient）
                        #   解析器用 golang.org/x/net/html 移植 Python 实验的 .item-box/.detail-box 逻辑
internal/application/imgsrc  # 跨 provider 桥接 + pixiv 富化（复用 pixivapp.SDKService.GetArtwork）
                        #   sdk/saucenao 与 sdk/ascii2d 互不 import（规则：产品 SDK 不互相 import）
internal/record         # RecordFromArtworkSafe（pixiv 命中）+ RecordFromImageSourceHit（非 pixiv）
internal/cli/pixiv      # source search 命令，parseEntityIDOrURL 扩展，printJSON/记录循环复用
internal/mcpserver/pixiv# source_image tool，typed addTool 注册
```

### 3.5 结果可信度与验证（"搜到的图是不是我想要的"）

**直答**：不能 100% 自动确定，但可把"是不是想要的"拆成两个可判定问题：
- **SAME（是否同一张/出处）** —— 可客观验证（像素级比较）
- **RELATED（是否相关作品）** —— 语义模糊，无法自动化确定，只能辅助 + 人眼确认

**实测铁证：相似度排序 ≠ 出处正确**。拿 pixiv 源图（113968075）全索引搜 SauceNAO：

| 命中 | 相似度 | 实际 |
|---|---|---|
| Yande.re 镜像（同图，重编码） | **93%** | 同一张图，但搬运站 |
| Pixiv 原图 113968075 | **39%** | **真正的出处**，但排第三 |

逐字解读：真正的 pixiv 出处因像素被 booru 重编码，在 pixiv 库只匹配 39%，反被 93% 的镜像压过。**"按相似度取最高"会把错的排前面** —— 必须靠信号重排。ascii2d color 搜索同样会混入"调色相似但完全不同"的作品。

**四层信号，逐层逼近"是不是想要的"**：

| 层 | 做法 | 成本 | 治什么 |
|---|---|---|---|
| ① 域名过滤 | 只留 pixiv 命中 | 免费 | 平台噪音（Twitter/Danbooru 等） |
| **② pixiv 命中重排** | 带 `pixiv_id` 的命中提升优先，pixiv 命中内再按相似度排 | 免费，**MVP 必做** | 镜像 93% 压过原图 39% 的错序 |
| **③ 感知哈希验证 `--verify`** | 下载 top-N 命中的 pixiv 真实图（复用 `OpenResource`）→ 与查询图做 **dHash** 比对 | 中等（要下载 top-N 图，默认关） | **客观定 SAME**：dHash 对缩放/JPEG 重压免疫，恰好治 booru 重编码 |
| **④ 交叉引擎一致 `--confirm`** | SauceNAO + ascii2d bovw 对同一 `pixiv_id` 达成一致 | 贵（双引擎耗 2 倍额度） | 两个独立算法一致 → 高置信 |

**置信度分级输出**（CLI 为每个命中标注）：

```
[high]   113968075 すばらしき新世界 — dHash 与查询图 98% 一致 + 双引擎同 id → 就是出处
[medium] 59268682  【刀剣乱腐】LOG3  — pixiv 命中 + 相似度达标，dHash 不一致 → 相关但可能不是同图
[low]    (无 pixiv_id)             — 低于阈值 → 仅供参考
```

- **HIGH**：双引擎同 id 或 dHash ≥0.95 → 客观"就是它"
- **MEDIUM**：pixiv_id + 相似度达标 → "大概率相关，人眼确认"
- **LOW**：无 pixiv 命中 → 标注"可能不是同一张"

**诚实边界**：
- RELATED 意图（找相似风格/角色）**永远无法自动化确定** —— 语义判断，人眼是最终裁决。CLI 的职责是摆出 HIGH/MEDIUM/LOW + 富化元数据（标题/作者/tags/缩略图），让人 1-2 秒确认。
- **裁剪图**：查询图是原图局部时 dHash 全图比对会失败 → 降级为 MEDIUM + 标注"可能是裁剪/局部"，不假装能确定。
- 实施优先级：**②重排进 MVP**（零成本治 93%/39% 病）；**③ `--verify` opt-in**（默认关）；**④ `--confirm` 后置**（双引擎贵）。

### 3.6 Firecrawl 通道的会话复用（不每次都走它）

Firecrawl 本质是"浏览器渲染 + 代理池"的**无状态 scrape**，理论上每次调用都付一次渲染/额度。但我们实测它**能穿透 ascii2d 的 CF 质询** —— 关键在于：**穿透后能否拿到会话凭证（cookie / `cf_clearance`）复用，让后续请求直连**，从而只在必要请求时才走 Firecrawl。

**实测依据**：Firecrawl scrape 的响应 `metadata` 带 `csrf-token`（`authenticity_token`）—— 说明 ascii2d 的会话状态确实经过 Firecrawl 的浏览器。但要真正"拿到 cookie 复用"，需要：
1. **Firecrawl 是否返回 Set-Cookie / 能否持久会话** —— 免费/Keyless 档 `actions` 交互被拒、响应头策略未知；**需实测确认能否取到 `cf_clearance` 或等价会话凭证**。
2. **如果取到**：架构变为「**Firecrawl 只做冷启动**（解一次质询拿会话）→ 后续搜索直连 ascii2d（带 cookie + 匹配 UA）」，**Firecrawl 额度只花在 session 建立**，搜索本身不耗它。
3. **如果取不到**：退化为「Firecrawl 每次搜索都走」—— 那 1000/月上限就成了硬约束，Firecrawl 仅作低频尝鲜。

**结论（设计决策）**：MVP 的 Firecrawl 通道**先按"必要请求才走"设计** —— 尝试从首次 scrape 提取 `cf_clearance`/cookie 供后续直连复用（与 FlareSolverr 的 session 复用同构）；若实测取不到，则明确标注 Firecrawl 为"每次搜索付费、仅低频"。这需要在实施时补一次「Firecrawl → 提取 cookie → 直连复用」的验证实验。

## 4. 边界与安全

- **不提交 api_key / 不缓存图**：`saucenao_api_key` 存 config DB（secret，不进 `--json`）。
- **本地图片不外发**：SauceNAO 直传 / ascii2d 压缩后上传；不做图床中转。
- **匿名模式**：SauceNAO 匿名实测被拒（API 层禁止），设计上支持匿名尝试但**优雅失败**并提示配 key。
- **限制与超时**：不新增无依据固定值。SauceNAO 动态读响应头额度；ascii2d 冷启动 ~17s 解挑战、热 session 2-3s/次 —— 写进文档与代码注释。
- **大图识别率**：>5MB 图不硬压，警告 + 建议 SauceNAO 直传；ascii2d 特征匹配对低分辨率素材有固有边界，文档写明。
- **Firecrawl 地区限制**：`actions` 交互当前地区不可用，仅用普通 scrape（URL 搜索）；文件上传走 FlareSolverr。
- **富化需本地已登录 pixiv 账号**（无匿名 Web 规则）；未登录时**降级返回原始命中**并提示（默认，可改）。

## 5. 待实施清单（后续，不在此文档实施）

- [ ] `sdk/saucenao`：Open/OpenWith、Hit/Header/SearchResponse 模型、SearchByURL/SearchByFile、错误分类、httptest 桩 + key 实测测试
- [ ] `sdk/ascii2d`：ImageSourceClient 接口、FlareSolverr 通道（session 复用 + 180s 超时）、Firecrawl 后端、`.item-box` 解析器、本地压缩
- [ ] `internal/application/imgsrc`：桥接 + 富化（pixivapp.SDKService.GetArtwork）
- [ ] `internal/record`：RecordFromImageSourceHit
- [ ] CLI `pixiv source search`：输入文法、`--engine`/`--api-key`/`--min-similarity`/`--top`/`--no-enrich`
- [ ] MCP `source_image` tool
- [ ] 文档：双语文档、`public-api-inventory.md`、`skills/pixiv-cli/`、README 路由
- [ ] 更新 `docs/maintainers/architecture.md` 与 `public-api-inventory.md`（新增公开 SDK 包）
