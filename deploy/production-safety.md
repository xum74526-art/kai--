# 生产发布与数据保护

> 旧版部署参考：本目录内容从 KAI影原 main 保留，尚未适配上游 v1.6.6，尤其 `/kai/` 子路径、登录 Cookie、资源地址和镜像启动流程。不要直接按本文命令更新线上环境。当前升级范围仅为本地 SQLite 数据库及其测试；线上部署须另行适配并完成验证。

本文说明当前单机 Docker Compose 部署的最小安全流程。它不会改变注册策略、管理员数据或数据库类型；Simon 部署继续使用单实例 SQLite，通用服务器部署继续支持 PostgreSQL/Redis。

## 1. 可部署镜像

`Quality checks` 必须先通过后端测试、前端完整测试与生产构建、Canvas Agent 测试与构建。只有成功的 `main` 推送才会触发镜像候选构建。

Web 和 Backend 候选镜像分别构建完成后，发布工作流才把二者提升为同一个可识别的提交标签：

```text
sha-<40 位 Git 提交哈希>
```

成功后下载 Actions 产物 `deployable-images-<revision>`，把其中 `release.env` 保存到服务器私有配置。标签本身仍可能被 Registry 重新指向，因此它只用于识别版本；生产 Compose 实际按两张镜像各自不可变的 digest 引用拉取。镜像安装脚本还会核对两张镜像的 OCI `org.opencontainers.image.revision`，任一不一致都会在启动前停止。

源码构建安装会以当前检出的完整提交生成 `source-<revision>` 标签，并在启动前执行同样的前后端 revision 核对。

Backend 镜像的健康检查使用 `/api/ready`：数据库、任务领取状态、必需本地目录，以及启用时的 Redis 任一不可用都会返回 503。Compose 给后端 45 秒停机宽限期，覆盖默认 30 秒的请求与在途任务排空窗口；`/api/health` 仍只表示进程存活。

Simon 源码部署还需在私有 `deploy/simon/.env` 中设置：

```sh
KAI_IMAGE_TAG=release-001
KAI_BUILD_REVISION=0123456789abcdef0123456789abcdef01234567
```

其中 `KAI_BUILD_REVISION` 必须填写 `git rev-parse HEAD` 输出的实际值。每次发布使用新标签，保留上一组镜像，禁止复用旧标签。

## 2. 上线前一致性备份

备份主机需要 Bash、Python 3、`tar` 和 `realpath`；SQLite 模式还需 `sqlite3`，PostgreSQL 模式还需 `pg_dump` / `pg_restore`。这些工具只用于生成和校验归档，不会自动修改生产数据库。

备份覆盖业务数据库、`.settings-key` 和后端本地媒体目录。数据库与这些文件必须属于同一个静默时点，因此先停止 Backend 接收写入；Web 可以保留维护页，但不能继续提交生成或上传。

备份目录必须位于应用数据目录之外。脚本不会修改源数据，也不会自动停止容器；只有操作者明确设置 `CANVAS_BACKUP_QUIESCED=yes` 才会执行。

### SQLite

SQLite 部署先停止后端，再执行：

```sh
docker compose --env-file deploy/simon/.env -f deploy/simon/compose.yml stop backend

CANVAS_DATABASE_DRIVER=sqlite \
CANVAS_DATA_PATH=/srv/beidou-kai/data \
CANVAS_BACKUP_DIR=/srv/beidou-kai-backups \
CANVAS_RELEASE_REVISION="$(git rev-parse HEAD)" \
CANVAS_BACKUP_QUIESCED=yes \
bash deploy/backup-production.sh
```

脚本使用 SQLite Online Backup API 生成独立数据库文件，并执行 `quick_check` 与外键检查。不要从运行中的 WAL 数据库只复制 `.db` 主文件。

### PostgreSQL

PostgreSQL 容器不应为备份临时暴露公网端口。先停止 Backend，然后从 Compose 网络内导出自包含 dump：

```sh
docker compose --env-file .env -f docker-compose.deploy.yml stop backend
umask 077
backend_container="$(docker compose --env-file .env -f docker-compose.deploy.yml ps --all -q backend)"
backend_data_path="$(docker inspect --format '{{range .Mounts}}{{if eq .Destination "/data"}}{{.Source}}{{end}}{{end}}' "$backend_container")"
docker compose --env-file .env -f docker-compose.deploy.yml exec -T postgres \
  pg_dump -U open_ai_canvas -d open_ai_canvas --format=custom --no-owner --no-acl \
  > /srv/beidou-kai-backups/postgres.pending.dump

CANVAS_DATABASE_DRIVER=postgres \
CANVAS_DATA_PATH="$backend_data_path" \
CANVAS_BACKUP_DIR=/srv/beidou-kai-backups \
CANVAS_POSTGRES_DUMP_PATH=/srv/beidou-kai-backups/postgres.pending.dump \
CANVAS_RELEASE_REVISION="${CANVAS_RELEASE_REVISION}" \
CANVAS_BACKUP_QUIESCED=yes \
bash deploy/backup-production.sh
rm -f /srv/beidou-kai-backups/postgres.pending.dump
```

上述命令从 Backend 容器挂载信息解析实际数据路径，兼容命名 volume 和显式宿主机目录。生产 `.env` 仍建议把 `CANVAS_DATA_PATH` 设置为受限权限的绝对宿主机目录，便于审计和恢复。备份成功并完成校验后才能重新启动 Backend。

## 3. 隔离恢复校验

每个备份包都包含版本元数据和全部文件的 SHA-256 校验和。校验脚本只解压到临时目录，不写入生产数据目录：

```sh
bash deploy/verify-production-backup.sh /srv/beidou-kai-backups/beidou-kai-*.tar.gz
```

SQLite 会再次执行完整性与外键检查。PostgreSQL 默认验证 dump 目录；要做真实恢复演练，先单独创建名称以 `_restore_verify` 结尾的隔离数据库，再执行：

```sh
POSTGRES_VERIFY_URL='postgresql://verify_user:password@127.0.0.1:5432/kai_restore_verify?sslmode=require' \
POSTGRES_RESTORE_VERIFY_CONFIRM=restore-isolated-database \
bash deploy/verify-production-backup.sh /srv/beidou-kai-backups/beidou-kai-postgres-*.tar.gz
```

校验库必须是新建且 `public` schema 中没有任何表；脚本拒绝非空数据库，也拒绝与 `DATABASE_URL` 相同的地址，不会替操作者执行清空。不要把生产地址作为校验地址。

## 4. 回滚边界

1. 上线前记录当前镜像标签、revision、备份包路径和校验结果。
2. 新版本只在 `/api/ready` 通过后接收流量；登录、素材读取、上传、任务创建与 SSE 仍需做部署后冒烟。
3. 若只发生容器启动问题且没有执行数据库迁移，可以重新启动上一组固定镜像。
4. 一旦新版本执行过数据库迁移，必须停止后端，把备份解压到新的隔离数据目录或恢复到新的数据库，再校验；不要在原目录上覆盖部分文件。
5. SQLite 回滚要同时切换完整数据目录和对应 `.settings-key`；PostgreSQL 回滚要同时恢复 dump、本地媒体目录和 `.settings-key`。
6. 校验通过后再切换 Compose 的数据路径和旧镜像标签。保留失败现场，直到业务与数据核对完成。

备份至少保留每日、每周两级周期，并定期在隔离环境演练恢复。只有“能成功生成备份”而没有恢复演练，不能视为已经具备回滚能力。
