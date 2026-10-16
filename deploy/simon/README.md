# simon.kai.com/kai/ 部署骨架

> 旧版部署参考：本目录内容从 KAI影原 main 保留，尚未适配上游 v1.6.6，尤其 `/kai/` 子路径、登录 Cookie、资源地址和镜像启动流程。不要直接按本文命令更新线上环境。当前升级范围仅为本地 SQLite 数据库及其测试；线上部署须另行适配并完成验证。

此目录只负责把当前代码和一份经过校验的 SQLite 部署副本接入公司站点的 `/kai/` 子路径。根站点继续由原服务提供；Compose 只在宿主机 `127.0.0.1:5173` 暴露网页容器，后端端口仅在 Compose 网络内可见。

## 部署边界

- 首期使用单个后端实例和 SQLite，不启动 PostgreSQL/Redis，也不能横向扩容后端。
- 前端必须使用当前分支的子路径适配，并以 `VITE_APP_BASE_PATH=/kai/` 构建。
- 数据目录必须是本地数据的独立部署副本。不要让服务器容器直接挂载开发机目录，也不要在数据库运行时只复制 `.db` 主文件。
- `.settings-key` 必须与数据库成套迁移，否则已加密的系统设置无法解密。使用本地文件存储时，相应资源文件也必须与数据库副本保持一致。
- 公开注册优先使用后台保存的设置；仅在数据库尚无该设置时才回退到服务器私有 `.env` 的 `CANVAS_REGISTRATION_ENABLED`。无论是否开放注册，预检始终要求管理员数量恰好为 1，新注册账号只能是普通用户。
- Compose 把服务器本地资源公网基址固定为 `https://simon.kai.com/kai`，生成的签名资源链接会进入 `/kai/api/public/resources/...`。
- 当前代码的数据库迁移会在后端首次启动时执行。启动前必须再保留一份不可写的部署副本，以便回滚。

## 1. 准备一致的数据副本

宿主机需要 Docker Engine、Docker Compose 插件和 `sqlite3` 命令行工具；Ubuntu 可使用 `sudo apt-get install -y sqlite3` 安装最后一项。

优先在源端使用 SQLite Online Backup API，例如 `sqlite3 source.db ".backup '/secure/stage/open_ai_canvas.db'"`。也可以先完全停止源后端，再复制数据库及同目录数据。不要从仍在写入的 WAL 数据库中单独复制 `open_ai_canvas.db`。

下面的清理只对离线部署副本执行，用于失效本地登录会话；它会保留现有公开注册设置：

```sh
sqlite3 -bail /secure/stage/open_ai_canvas.db < deploy/simon/snapshot-public-safety.sql
sh deploy/simon/preflight.sh /secure/stage/open_ai_canvas.db
```

预检要求：`PRAGMA quick_check` 为 `ok`、无外键违规、同目录存在恰好 32 字节的 `.settings-key`、数据库存在单管理员唯一索引、管理员数量恰好为 1 且处于启用状态并保留本地密码、登录会话与临时 OAuth/邮箱验证码均为 0。它不会修改数据库。是否开放公开注册优先由后台设置决定，只有缺失该设置时才使用服务器私有 `.env`；后端的单管理员约束仍应由当前代码和数据库迁移负责，预检不能替代运行时权限检查。

把通过预检的整套数据放到服务器私有目录，例如 `/srv/beidou-kai/data`。目录不得由 Web 根目录提供，也不要加入 Git 或镜像。确认运行后端镜像的 `app` 用户对该目录可读写。

## 2. 配置并启动容器

```sh
cp deploy/simon/.env.example deploy/simon/.env
# 编辑 deploy/simon/.env，至少确认 KAI_DATA_PATH 是绝对路径、公开注册策略以及每次上线的 KAI_IMAGE_TAG。

docker compose --env-file deploy/simon/.env -f deploy/simon/compose.yml config
sh deploy/simon/preflight.sh "$(grep '^KAI_DATA_PATH=' deploy/simon/.env | cut -d= -f2-)/open_ai_canvas.db"
docker compose --env-file deploy/simon/.env -f deploy/simon/compose.yml build
docker compose --env-file deploy/simon/.env -f deploy/simon/compose.yml up -d
curl --fail http://127.0.0.1:5173/api/health
```

不要把 `KAI_HTTP_PORT` 绑定到 `0.0.0.0`。Compose 文件已硬编码监听 `127.0.0.1`；公网只应通过现有 HTTPS Nginx 进入。

## 3. 接入现有 Nginx

把 `nginx-kai.conf` 中的 location 放入 `simon.kai.com` 源站现有的 `server {}`。公司负载均衡负责 TLS，源站当前监听 HTTP；片段把转发给应用的外部协议固定为 `https`，确保登录 Cookie 带 `Secure`。OAuth 回调和签名资源路径关闭源站访问日志，避免授权码、state 或资源签名进入默认日志。若修改了 `KAI_HTTP_PORT`，同步修改片段中的 `127.0.0.1:5173`。

```sh
sudo nginx -t
sudo systemctl reload nginx
curl --fail https://simon.kai.com/kai/api/health
```

片段只接管 `/kai` 和 `/kai/`，不会覆盖 `/`。`/kai/api/tasks/<id>/text-events` 单独关闭缓冲、缓存和 gzip；普通 API 不继承 SSE 的长超时策略。后端通过 `CANVAS_PUBLIC_BASE_PATH=/kai` 把 OAuth 回跳和登录 Cookie 收窄到 `/kai/`，避免影响公司站点其他页面。

### 公司首页左下角入口

把 `deploy/simon/company-site/kai-entry.js` 上传到公司首页当前 Web 根目录，并在现有 `index.html` 中只增加下面这一行：

```html
<script src="/kai-entry.js" defer></script>
```

脚本会复用现有 `#project-links` 容器，在 MatrixFuel 和 WOD 旁边追加唯一的“KAI影”入口，目标为 `/kai/`；容器尚未渲染时会等待，不会重建或覆盖公司首页。若线上首页没有该容器，应停止上线并先核对实际 DOM，不要临时复制一套悬浮入口。

## 4. 上线核对

1. 访问 `/kai/`、`/kai/login` 和一个刷新后的登录态路由，确认没有回到根站点。
2. 检查浏览器网络请求只访问 `/kai/assets/...` 与 `/kai/api/...`，不存在误发到根路径的 `/assets` 或 `/api`。
3. 登录保留的管理员账号，确认管理员入口可用；在后台确认公开注册已开启（数据库已有设置时不会被 `.env` 覆盖），再用一个普通测试账号确认注册成功且角色不是管理员。
4. 在数据库中再次执行 `SELECT COUNT(*) FROM users WHERE role='admin';`，结果必须为 `1`。
5. 检查上传、媒体读取、文本 SSE、登出后的 Cookie 清除以及容器重启后的数据持久化。

如后续启用 Linux.do 登录，把后台登记的回调地址设置为 `https://simon.kai.com/kai/oauth/linuxdo/callback`；根路径回调地址不适用于此部署。

## 回滚

先停止 Compose，再恢复首次启动前保存的 SQLite 数据目录副本与对应 `.settings-key`/资源文件，然后使用上一组镜像标签启动。不要把经过新版本迁移的数据库直接交给旧版程序。Nginx 回滚只需移除本目录的 location 并在 `nginx -t` 通过后 reload；首页入口回滚只需删除新增的脚本标签和 `kai-entry.js`。根站点始终保持独立。
