#!/usr/bin/env bash

set -Eeuo pipefail

REPOSITORY_REF="${REPOSITORY_REF:-}"
INSTALL_DIR="${INSTALL_DIR:-/opt/beidou-kai}"
CANVAS_HTTP_PORT="${CANVAS_HTTP_PORT:-3000}"
CANVAS_PUBLIC_ORIGIN="${CANVAS_PUBLIC_ORIGIN:-}"
CANVAS_IMAGE_TAG="${CANVAS_IMAGE_TAG:-}"
CANVAS_WEB_IMAGE="${CANVAS_WEB_IMAGE:-}"
CANVAS_BACKEND_IMAGE="${CANVAS_BACKEND_IMAGE:-}"
COMPOSE_FILE="docker-compose.deploy.yml"
COMPOSE_URL="${COMPOSE_URL:-}"

step() {
    printf '\n==> %s\n' "$1"
}

fail() {
    printf '\n安装失败：%s\n' "$1" >&2
    exit 1
}

require_root() {
    if [[ "${EUID}" -ne 0 ]]; then
        fail "请使用 README 中带 sudo 的一键安装命令"
    fi
    [[ "$(uname -s)" == "Linux" ]] || fail "一键部署脚本仅支持 Linux 服务器"
    [[ "$CANVAS_HTTP_PORT" =~ ^[0-9]+$ ]] || fail "CANVAS_HTTP_PORT 必须是 1 到 65535 的数字"
    ((CANVAS_HTTP_PORT >= 1 && CANVAS_HTTP_PORT <= 65535)) || fail "CANVAS_HTTP_PORT 必须是 1 到 65535 的数字"
}

validate_image_tag() {
    [[ "$CANVAS_IMAGE_TAG" =~ ^sha-[0-9a-f]{40}$ ]] || fail "CANVAS_IMAGE_TAG 必须来自 CI 发布凭据，格式为 sha-<40位小写提交哈希>，禁止使用 latest"
    [[ "$CANVAS_WEB_IMAGE" =~ ^ghcr\.io/xum74526-art/beidou-kai-web@sha256:[0-9a-f]{64}$ ]] || fail "CANVAS_WEB_IMAGE 必须是发布凭据中的 Web digest 引用"
    [[ "$CANVAS_BACKEND_IMAGE" =~ ^ghcr\.io/xum74526-art/beidou-kai-backend@sha256:[0-9a-f]{64}$ ]] || fail "CANVAS_BACKEND_IMAGE 必须是发布凭据中的 Backend digest 引用"
}

install_packages() {
    local packages=(ca-certificates curl openssl)

    if command -v apt-get >/dev/null 2>&1; then
        apt-get update
        DEBIAN_FRONTEND=noninteractive apt-get install -y "${packages[@]}"
    elif command -v dnf >/dev/null 2>&1; then
        dnf install -y "${packages[@]}"
    elif command -v yum >/dev/null 2>&1; then
        yum install -y "${packages[@]}"
    else
        fail "暂不支持当前 Linux 发行版，请先手动安装 Docker、curl 和 OpenSSL"
    fi
}

install_docker() {
    if command -v docker >/dev/null 2>&1 && docker compose version >/dev/null 2>&1; then
        return
    fi

    step "安装 Docker 和 Docker Compose"
    local installer
    installer="$(mktemp)"
    curl -fsSL https://get.docker.com -o "$installer"
    sh "$installer"
    rm -f "$installer"

    if command -v systemctl >/dev/null 2>&1; then
        systemctl enable --now docker
    elif command -v service >/dev/null 2>&1; then
        service docker start
    fi

    docker compose version >/dev/null 2>&1 || fail "Docker Compose 安装失败"
}

login_ghcr() {
    if [[ -n "${GHCR_USERNAME:-}" || -n "${GHCR_TOKEN:-}" ]]; then
        [[ -n "${GHCR_USERNAME:-}" && -n "${GHCR_TOKEN:-}" ]] || fail "GHCR_USERNAME 和 GHCR_TOKEN 必须同时配置"
        step "登录 GitHub Container Registry"
        # token 只通过 stdin 交给 Docker，避免出现在命令参数和进程列表中。
        printf '%s' "$GHCR_TOKEN" | docker login ghcr.io --username "$GHCR_USERNAME" --password-stdin
    fi
}

prepare_environment() {
    mkdir -p "$INSTALL_DIR"
    cd "$INSTALL_DIR"

    if [[ -f .env ]]; then
        grep -Eq '^POSTGRES_PASSWORD=.+$' .env || fail "现有 .env 缺少 POSTGRES_PASSWORD"
        grep -Eq '^DATABASE_URL=.+$' .env || fail "现有 .env 缺少 DATABASE_URL"

        local configured_http_port
        configured_http_port="$(sed -n 's/^CANVAS_HTTP_PORT=//p' .env | tail -n 1)"
        if [[ -n "$configured_http_port" ]]; then
            [[ "$configured_http_port" =~ ^[0-9]+$ ]] || fail ".env 中的 CANVAS_HTTP_PORT 无效"
            ((configured_http_port >= 1 && configured_http_port <= 65535)) || fail ".env 中的 CANVAS_HTTP_PORT 无效"
            CANVAS_HTTP_PORT="$configured_http_port"
        fi

        local configured_cors_origins
        configured_cors_origins="$(sed -n 's/^CANVAS_CORS_ORIGINS=//p' .env | tail -n 1)"
        [[ -n "$configured_cors_origins" ]] || fail ".env 中的 CANVAS_CORS_ORIGINS 为空，请填写浏览器实际访问 Origin（例如 http://服务器IP:${CANVAS_HTTP_PORT}）"

        local configured_image_tag
        configured_image_tag="$(sed -n 's/^CANVAS_IMAGE_TAG=//p' .env | tail -n 1)"
        [[ -n "$configured_image_tag" ]] || fail ".env 缺少 CI 发布凭据中的 CANVAS_IMAGE_TAG"
        CANVAS_IMAGE_TAG="$configured_image_tag"
        CANVAS_WEB_IMAGE="$(sed -n 's/^CANVAS_WEB_IMAGE=//p' .env | tail -n 1)"
        CANVAS_BACKEND_IMAGE="$(sed -n 's/^CANVAS_BACKEND_IMAGE=//p' .env | tail -n 1)"
        validate_image_tag
        return
    fi

    step "生成 PostgreSQL 随机密码和部署配置"
    validate_image_tag
    local database_password
    local local_ip
    database_password="$(openssl rand -hex 32)"
    local_ip="$(hostname -I 2>/dev/null | awk '{print $1}')"
    [[ -n "$CANVAS_PUBLIC_ORIGIN" ]] || CANVAS_PUBLIC_ORIGIN="http://${local_ip:-127.0.0.1}:${CANVAS_HTTP_PORT}"
    [[ "$CANVAS_PUBLIC_ORIGIN" =~ ^https?://[^/]+$ ]] || fail "CANVAS_PUBLIC_ORIGIN 必须是完整 Origin，且不能包含路径"
    umask 077
    cat >.env <<EOF
POSTGRES_DB=open_ai_canvas
POSTGRES_USER=open_ai_canvas
POSTGRES_PASSWORD=${database_password}
DATABASE_URL=postgresql://open_ai_canvas:${database_password}@postgres:5432/open_ai_canvas?sslmode=disable
CANVAS_HTTP_PORT=${CANVAS_HTTP_PORT}
CANVAS_IMAGE_TAG=${CANVAS_IMAGE_TAG}
CANVAS_RELEASE_REVISION=${CANVAS_IMAGE_TAG#sha-}
CANVAS_WEB_IMAGE=${CANVAS_WEB_IMAGE}
CANVAS_BACKEND_IMAGE=${CANVAS_BACKEND_IMAGE}
CANVAS_REGISTRATION_ENABLED=false
CANVAS_ALLOW_PRIVATE_UPSTREAMS=false
CANVAS_ALLOWED_PRIVATE_UPSTREAM_HOSTS=
CANVAS_CORS_ORIGINS=${CANVAS_PUBLIC_ORIGIN}
CANVAS_PUBLIC_BASE_URL=${CANVAS_PUBLIC_ORIGIN}
CANVAS_TRUSTED_PROXIES=172.16.0.0/12
EOF
}

download_compose() {
    step "下载 GHCR 镜像部署配置"
    local temporary_file
    local compose_url="$COMPOSE_URL"
    local repository_ref="${REPOSITORY_REF:-${CANVAS_IMAGE_TAG#sha-}}"
    [[ -n "$compose_url" ]] || compose_url="https://raw.githubusercontent.com/xum74526-art/kai--/${repository_ref}/${COMPOSE_FILE}"
    temporary_file="$(mktemp "${INSTALL_DIR}/.docker-compose.deploy.XXXXXX")"
    curl -fsSL "$compose_url" -o "$temporary_file"
    mv "$temporary_file" "$COMPOSE_FILE"
}

start_services() {
    step "拉取并启动 GHCR 网页与后端镜像"
    if ! docker compose --env-file .env -f "$COMPOSE_FILE" pull; then
        fail "GHCR 镜像拉取失败；如果容器包尚未公开，请通过 GHCR_USERNAME 和 GHCR_TOKEN 登录后重试"
    fi
    verify_image_bundle
    docker compose --env-file .env -f "$COMPOSE_FILE" up -d --remove-orphans --wait --wait-timeout 600
}

verify_image_bundle() {
    local expected_revision="${CANVAS_IMAGE_TAG#sha-}"
    local web_revision
    local backend_revision

    web_revision="$(docker image inspect --format '{{ index .Config.Labels "org.opencontainers.image.revision" }}' "$CANVAS_WEB_IMAGE")"
    backend_revision="$(docker image inspect --format '{{ index .Config.Labels "org.opencontainers.image.revision" }}' "$CANVAS_BACKEND_IMAGE")"
    [[ "$web_revision" == "$expected_revision" ]] || fail "Web 镜像 revision 与发布标签不一致"
    [[ "$backend_revision" == "$expected_revision" ]] || fail "Backend 镜像 revision 与发布标签不一致"
    [[ "$web_revision" == "$backend_revision" ]] || fail "前后端镜像不是同一条提交，拒绝部署"
}

print_result() {
    local local_ip
    local_ip="$(hostname -I 2>/dev/null | awk '{print $1}')"
    [[ -n "$local_ip" ]] || local_ip="服务器IP"

    printf '\n部署完成。\n'
    local configured_origin
    configured_origin="$(sed -n 's/^CANVAS_CORS_ORIGINS=//p' .env | tail -n 1)"
    printf '访问地址：%s\n' "${configured_origin:-http://${local_ip}:${CANVAS_HTTP_PORT}}"
    printf '安装目录：%s\n' "$INSTALL_DIR"
    printf '查看状态：cd %q && docker compose --env-file .env -f %s ps\n' "$INSTALL_DIR" "$COMPOSE_FILE"
    printf '\n新注册账号默认为普通用户；请通过受控流程初始化管理员。公网长期使用前请配置 HTTPS。\n'
}

main() {
    require_root
    step "安装服务器基础工具"
    install_packages
    install_docker
    login_ghcr
    prepare_environment
    download_compose
    start_services
    print_result
}

main "$@"
