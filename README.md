<p align="center">
  <img src="web/public/logo.svg" width="240" alt="KAI影 Logo">
</p>

<h1 align="center">KAI影</h1>

<p align="center">让一个故事，从文字走向银幕。</p>

<p align="center">
  <a href="https://github.com/xum74526-art/kai--">代码仓库</a> ·
  <a href="docs/content/docs/overview/features.mdx">功能清单</a> ·
  <a href="docs/index.md">文档索引</a> ·
  <a href="https://github.com/xum74526-art/kai--/issues">问题反馈</a>
</p>

KAI影是面向 AI 影视、短剧和视觉创作的工作台。把故事、角色、场景、参考素材和生成结果放在同一个创作空间，通过自由画布、项目工作台和 AI 助手逐步完成作品。

本仓库维护 KAI影的产品界面、原版 Logo、影片展示和功能定制。模型渠道、邮箱、存储及账号配置由部署者管理，不随源码公开。

> 项目仍在快速开发。升级前请备份数据库、媒体文件和配置密钥；不要让旧版本代码直接连接已经升级的数据目录。

## 核心能力

- **即时创作**：通过文本、图片和参考素材发起图片、视频、音频或文本生成，查看任务状态与历史结果。
- **自由画布**：用节点与连线组织想法，整理镜头、角色和素材，支持缩放、框选、导入导出及只读分享。
- **短剧项目**：围绕剧本、章节、角色、场景和分镜组织创作，逐步推进制作流程。
- **AI 助手与技能**：在画布内讨论创意、读取上下文和使用技能；实际操作受模型能力、权限与审批控制。
- **素材与任务管理**：管理历史文件、生成任务、失败恢复、资源引用和账号存储空间。
- **渠道与管理后台**：配置模型协议、系统渠道、积分、邮件服务、对象存储、站点外观和安全策略。
- **KAI影品牌展示**：登录页保留完整 Logo 和七段影片；自定义视频、封面及外观配置继续优先，不被默认展示覆盖。

完整范围和验收边界见[功能清单](docs/content/docs/overview/features.mdx)及[待测试清单](docs/content/docs/progress/pending-test.mdx)。

## 本地启动

需要 Bun、Go 及 Node.js；运行版本和依赖以仓库锁文件、[本地开发文档](docs/content/docs/backend/local-development.mdx)为准。

```bash
git clone https://github.com/xum74526-art/kai--.git kai
cd kai
```

Windows PowerShell 在仓库根目录执行：

```powershell
.\scripts\start-local.ps1
```

该脚本检查端口、准备依赖并启动前后端。开发数据使用 Git 忽略的 `.local/project-workbench-debug`。已有账号时必须复用原数据目录和 `.settings-key`，不要另建空库。

打开 <http://localhost:3000>。前端默认通过 `/api` 访问本地后端；先登录，再配置自己的模型渠道。

Docker 本地构建：

```bash
docker compose -f docker-compose.local.yml up -d --build
```

源码热更新、迁移与多工作树共用数据的细节见[本地开发文档](docs/content/docs/backend/local-development.mdx)。

## 服务器部署

源码部署从本仓库获取安装脚本：

```bash
curl -fsSL https://raw.githubusercontent.com/xum74526-art/kai--/main/scripts/install-server.sh | sudo bash
```

执行前应审阅脚本与 `.env.example`，准备独立的数据目录和备份。默认安装目录仍为 `/opt/open-ai-canvas`，这是沿用的技术路径，不代表产品名称；不要仅为改名迁移已有数据库、数据卷或服务目录。

生产编排使用 `docker-compose.deploy.yml`，源码构建叠加 `docker-compose.build.yml`。公网只开放网页入口，后端、PostgreSQL 和 Redis 留在内部网络。

镜像安装和在线更新只能使用 **KAI影仓库已成功发布的 Release 与镜像摘要**。没有可用 Release 时使用源码部署，不要改用上游镜像覆盖 KAI影定制。更新、备份、迁移和回退要求见[在线更新文档](docs/content/docs/backend/system-update.mdx)。

## 配置与安全

- 真实 API Key、SMTP 授权码、OSS/COS 凭据和登录密码不进入仓库、日志或截图。
- 模型、邮箱和存储配置由部署者自行设置；源码更新不代表这些外部服务已经配置可用。
- 生产环境使用 HTTPS，并明确设置 `CANVAS_CORS_ORIGINS`；默认保持 `CANVAS_REGISTRATION_ENABLED=false`。
- 首次管理员注册在受控网络完成。后端端口不直接暴露公网。
- 外部模型访问保留 SSRF 防护；本地开发只通过 `CANVAS_ALLOWED_PRIVATE_UPSTREAM_HOSTS` 精确放行可信地址。
- 备份必须覆盖数据库、媒体文件和 `.settings-key`；限制数据目录与备份的读取权限。
- `yingce.plugin/v2`、`YINGCE_AGENT_*`、内部包名及既有存储 key 属于协议或运行标识，保留它们以免影响已保存配置和插件。

漏洞报告方式见[安全策略](SECURITY.md)。

## 文档与参与

- [文档索引](docs/index.md)
- [功能清单](docs/content/docs/overview/features.mdx)
- [本地开发](docs/content/docs/backend/local-development.mdx)
- [后端 API](docs/content/docs/backend/http-api.mdx)
- [数据库结构](docs/content/docs/backend/backend-database.mdx)
- [插件系统](docs/content/docs/plugins/plugin-system.mdx)
- [待测试清单](docs/content/docs/progress/pending-test.mdx)
- [更新日志](CHANGELOG.md)
- [贡献指南](CONTRIBUTING.md)

欢迎在 [KAI影 Issues](https://github.com/xum74526-art/kai--/issues) 提交建议与问题。请提供版本、复现步骤和脱敏截图，不要附上账号密钥或生产数据库。

## 开源来源与许可

KAI影基于 [Open AI Canvas](https://github.com/ddcat-ai/open-ai-canvas) 开发，感谢原作者和贡献者。产品品牌、仓库维护与本地定制以 KAI影为准；上游团队和赞助商不作为 KAI影团队或赞助商展示。

代码按 [MIT 许可证](LICENSE) 使用，并保留原始版权声明。第三方技能、依赖和演示媒体按各自的许可使用，详见随附的 `THIRD-PARTY-LICENSES.md` 及[演示素材署名](web/public/welcome/credits.html)。
