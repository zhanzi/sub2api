# 任务清单

## F001 项目初始化认知

- [x] 读取 fork 工作流规则和 Git 状态。
- [x] 读取 README、开发指南、后端/前端 manifest、CI、入口文件和迁移说明。
- [x] 识别 `DEV_GUIDE.md` 与当前 CI 的差异。
- [x] 建立项目上下文、阶段计划、功能真源和交接状态。

## F002 后续功能准入模板

- [ ] 新增功能前先描述目标、非目标、交付物和验收方式。
- [ ] 判断是否涉及资金、权限、配额、敏感信息、外部系统、迁移或生产资源。
- [ ] 高风险功能先创建专项计划，再进入编码。
- [ ] 编码型功能包含“注释补齐与规范自检”。

## F003 验证命令基线确认

- [ ] 后端普通改动优先运行 `cd backend && go test -tags=unit ./...`。
- [ ] 涉及数据库、Redis、迁移或集成链路时运行 `cd backend && go test -tags=integration ./...`。
- [ ] 前端改动优先运行 `pnpm --dir frontend run lint:check`、`pnpm --dir frontend run typecheck` 和相关 vitest。
- [ ] 依赖、构建或发布相关改动按 CI 补充完整验证。
