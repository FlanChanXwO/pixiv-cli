# Dockerfile — pixiv-cli 容器镜像
#
# 从与原生生产构建相同的不可变 release tag 构建版本化 Linux 二进制后，
# 拷贝到 pinned Debian slim runtime。容器使用同一 pixiv 二进制和 ~/.pixiv-cli
# 状态命名空间，不创建 Docker 专有产品行为。
#
# 构建上下文预期包含 dist/pixiv（或 dist/pixiv.exe on Windows，但容器只支持 Linux）。
# CI 构建时 COPY --from=build 或 COPY dist/pixiv，取决于构建方式。
#
# 使用方式：
#   docker build -t pixiv-cli .
#   docker run --rm pixiv-cli --version
#   docker run --rm -i pixiv-cli mcp
#   docker run --rm -v pixiv-cli-state:/home/pixiv/.pixiv-cli -v "$PWD:/work" pixiv-cli download <url>

# Debian bookworm-slim (glibc 2.36)，pinned by immutable multi-arch manifest digest。
# 不可变 digest 确保构建可复现；不使用可变 tag。
FROM debian@sha256:88200866dfff7ea7f5cbcb6ec7c8a701889efe6fe859fe64d6990e4b07ea4171 AS runtime

# 安装运行时必要材料：CA 证书用于 HTTPS 连接，tzdata 用于时区处理。
# Debian slim 默认不含这些；pixiv CLI 需要连接 Pixiv API（HTTPS）。
RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        ca-certificates \
        tzdata \
    && rm -rf /var/lib/apt/lists/*

# 创建专用非 root 用户 pixiv（UID 1000），HOME=/home/pixiv。
# 容器不以 root 运行，避免提权风险。
RUN useradd --home-dir /home/pixiv --create-home --shell /usr/sbin/nologin --uid 1000 pixiv

# 预创建状态目录并固定属主；首次挂载空命名 volume 时 Docker 会继承该 ownership。
RUN mkdir -p /home/pixiv/.pixiv-cli && chown -R pixiv:pixiv /home/pixiv

# 拷贝预构建的版本化 pixiv 二进制到 /usr/local/bin/pixiv。
# 二进制从构建上下文的 dist/pixiv 拷入。
COPY dist/pixiv /usr/local/bin/pixiv
RUN chmod 0755 /usr/local/bin/pixiv

# 携带项目与第三方许可证，保证容器分发满足保留版权和许可声明的要求。
COPY LICENSE THIRD_PARTY_LICENSES.md /usr/share/doc/pixiv-cli/
COPY third_party/licenses/ /usr/share/doc/pixiv-cli/third_party/licenses/
RUN chmod -R a+rX /usr/share/doc/pixiv-cli

# 设置 HOME 环境变量，使 pixiv CLI 本地状态路径解析到用户 home 目录下。
ENV HOME=/home/pixiv

# /work 是默认工作目录，用于下载/文件操作 bind mount。
WORKDIR /work

# OCI provenance 元数据标签。
# CI 必须注入触发 release 的不可变 tag 与 tag commit；本地构建可显式传入对应值。
ARG REVISION
ARG VERSION
LABEL org.opencontainers.image.source="https://github.com/FlanChanXwO/pixiv-cli"
LABEL org.opencontainers.image.revision="${REVISION}"
LABEL org.opencontainers.image.version="${VERSION}"
LABEL org.opencontainers.image.licenses="MIT"

# 切换到非 root 用户。
USER pixiv

# pixiv CLI 入口点——不使用 wrapper script，直接执行二进制。
ENTRYPOINT ["/usr/local/bin/pixiv"]
