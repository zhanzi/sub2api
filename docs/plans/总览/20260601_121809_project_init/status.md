# 状态交接

## 当前阶段

项目初始化认知已完成，后续进入具体功能前应先基于本目录补充专项计划或功能条目。

## 当前聚焦特性

- `F001 项目初始化认知`：已完成。
- 下一步默认聚焦：`F002 后续功能准入模板`，在用户提出第一个具体功能时补齐。

## Blockers

- 暂无编码阻塞。
- `docs/*` 被 `.gitignore` 忽略，本计划目录需要用 `git add -f` 纳入版本控制。

## 风险

- `DEV_GUIDE.md` 部分内容不是当前 fork 真源，尤其 fork 仓库名、Go 版本和 golangci-lint 版本。
- 数据库迁移有 checksum，不得编辑历史迁移。
- 支付、计费、配额、鉴权、上游账号调度、并发限流和外部 OAuth 都是高风险区域。

## Handoff

- done:
  - 已建立 `AGENTS.md` fork 工作流规则。
  - 已读取项目 README、开发指南、后端/前端依赖、CI、入口文件和迁移说明。
  - 已创建项目初始化计划目录。
- blocked:
  - 无。
- next:
  - 用户提出具体功能后，先从 `product/zhanzi` 切 `codex/<feature-slug>`。
  - 读取本文件和 `feature_list.json`。
  - 高风险功能先创建专项计划，再编码。
- risks:
  - 不要直接使用 `DEV_GUIDE.md` 的旧 fork 信息。
  - 不要修改已存在迁移文件。
  - 未运行验证命令前不得声明通过。
- commands:
  - `git status --short --branch`
  - `git remote -v`
  - `cd backend && go test -tags=unit ./...`
  - `cd backend && go test -tags=integration ./...`
  - `pnpm --dir frontend run lint:check`
  - `pnpm --dir frontend run typecheck`
  - `pnpm --dir frontend exec vitest run <spec>`
