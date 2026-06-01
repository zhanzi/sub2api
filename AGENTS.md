# sub2api fork 协作规则

本文件是当前 fork 工作区的 AI 协作规则真源，优先级高于全局规则和仓库内其他通用开发说明。所有 AI 助手和人工协作者在本仓库内规划、编码、同步上游或提交前，都必须先遵守本文件。

## 1. 远端真源

- `origin` 固定表示个人 fork：`https://github.com/zhanzi/sub2api.git`。
- `upstream` 固定表示开源上游：`https://github.com/Wei-Shaw/sub2api.git`。
- `DEV_GUIDE.md` 中提到的 `bayma888/sub2api-bmai` 是上游项目遗留信息，不是当前工作区的 fork 真源；不得据此修改本地 remote、推送目标或分支策略。
- 如果本地缺少 `upstream`，先执行：

```powershell
git remote add upstream https://github.com/Wei-Shaw/sub2api.git
git fetch upstream
```

## 2. 分支职责

- `main` 只用于跟随 `upstream/main`，不承载本 fork 的自定义业务功能。
- `product/zhanzi` 是长期产品集成分支，承载当前 fork 的私有功能、定制改动和项目级规则。
- 单个功能、修复或实验必须从 `product/zhanzi` 切短分支开发，优先使用 `codex/<feature-slug>`；人工开发也可使用 `feature/<feature-slug>` 或 `fix/<issue-slug>`。
- 除非用户明确要求，不得在 `main` 上直接开发、提交或合并业务功能。

## 3. 首次准备流程

在一个干净工作区中初始化 fork 工作流：

```powershell
git remote add upstream https://github.com/Wei-Shaw/sub2api.git
git fetch upstream
git checkout main
git merge upstream/main
git push origin main
git checkout -b product/zhanzi
git push -u origin product/zhanzi
```

如果 `upstream` 或 `product/zhanzi` 已存在，跳过对应创建步骤，先用只读命令确认当前状态：

```powershell
git remote -v
git branch --list main product/zhanzi
git status --short --branch
```

## 4. 日常开发流程

开始新功能前：

```powershell
git checkout product/zhanzi
git pull origin product/zhanzi
git checkout -b codex/<feature-slug>
```

功能完成并验证后，合回长期产品分支：

```powershell
git checkout product/zhanzi
git merge --no-ff codex/<feature-slug>
git push origin product/zhanzi
```

短分支内的提交应保持聚焦。不得把上游同步、功能开发、格式化重排、依赖升级和无关文档修改混在同一个提交或同一个 PR 中。

## 5. 同步上游流程

上游更新必须先进入 `main`，再由 `product/zhanzi` 通过 merge 吸收：

```powershell
git fetch upstream
git checkout main
git merge upstream/main
git push origin main
git checkout product/zhanzi
git merge main
git push origin product/zhanzi
```

`product/zhanzi` 合入上游更新默认使用 `git merge main`，不对长期产品分支做反复 rebase，避免改写共享历史。

## 6. 冲突与验证要求

- 合并上游时若发生冲突，必须优先保留上游通用修复，再重新套用本 fork 的定制逻辑。
- 冲突解决不得依赖中文展示文案或临时字符串判断业务语义，应以结构化字段、枚举、接口契约和测试为准。
- 涉及后端改动时，至少运行相关 `go test`；涉及前端改动时，使用 `pnpm`，不得改用 `npm`。
- 未运行验证命令，不得声明“完成”“通过”“可用”或“已修复”。

## 7. 文档与规则维护

- 本 fork 的长期流程、AI 协作规则和分支策略优先维护在本文件。
- 如需兼容 Claude Code 等工具，可在同层创建短 `CLAUDE.MD` 跳转到本文件，但不要复制维护第二份长规则。
- 若未来调整远端、长期产品分支名或上游同步策略，必须先更新本文件，再执行对应 Git 操作。
