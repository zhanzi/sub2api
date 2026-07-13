# 账号数据导入

## 功能范围

管理员可在 `/admin/accounts` 的“更多操作 -> 导入”弹窗中通过以下方式导入账号与代理：

- 选择一个或多个 sub2api 导出的 JSON 文件。
- 拖入一个或多个 sub2api 导出的 JSON 文件。
- 直接粘贴一个完整导出对象。
- 直接粘贴由多个完整导出对象组成的 JSON 数组。

本功能不支持选择目录，也不支持直接粘贴账号对象数组。

## 粘贴格式

单个导出对象必须包含 `proxies` 和 `accounts` 数组；正常导出文件还会包含 `exported_at`：

```json
{
  "exported_at": "2026-07-13T00:00:00Z",
  "proxies": [],
  "accounts": []
}
```

批量粘贴时，数组中的每一项都必须是完整导出对象：

```json
[
  {
    "exported_at": "2026-07-13T00:00:00Z",
    "proxies": [],
    "accounts": []
  },
  {
    "exported_at": "2026-07-13T00:01:00Z",
    "proxies": [],
    "accounts": []
  }
]
```

空数组、非法 JSON、账号对象数组或任一结构不完整的数组项都会在请求前被拒绝。数组项错误按从 1 开始的序号提示，系统不会静默跳过坏数据。

## 导入行为

- 前端先校验每个导出对象，再合并 `proxies`、`accounts` 和 `skipped_shadows`。
- 每次导入只向现有 `POST /admin/accounts/data` 接口发送一个合并后的 payload。
- 文件输入和粘贴输入互相独立，每次只提交当前选中的输入方式。
- 导入继续使用现有重复数据规则：代理可按现有逻辑复用；账号尝试创建，冲突按单项失败返回，不静默覆盖。
- 导入内容可能包含账号凭据和代理认证信息，不会写入浏览器日志、缓存或 `localStorage`。

## 验收

在 `frontend` 目录执行：

```powershell
pnpm test:run -- src/__tests__/integration/data-import.spec.ts
pnpm typecheck
pnpm lint:check
pnpm build
```

验收时应确认：

- 单个导出对象和导出对象数组均可进入现有导入流程。
- 无效输入不会调用后端接口，并能定位文件名或数组项序号。
- 原有单文件、多文件、拖拽和部分成功后的列表刷新行为不受影响。
