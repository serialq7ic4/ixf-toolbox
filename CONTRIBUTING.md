# 贡献指南

感谢你帮助改进 `ixf-toolbox`。本项目处理私有文档和 OKR 工作流，因此贡献必须同时满足正确性、可审查性和数据安全要求。

## 适合贡献的方向

- 已授权文档读取、转换、分块和清理。
- Markdown 到文档发布及写后校验。
- OKR 读取、创建、修改、排序和安全校验。
- macOS / Windows 本地 cookie 导出可靠性。
- 错误契约、诊断、安装、更新、CI 和发布流程。
- 文档、合成 fixture 和 agent skill 指引。

默认不接受托管服务、遥测、后台 daemon、浏览器自动化或绕过权限模型的实现。

## 隐私规则

不要在 issue、PR、commit、测试、日志或截图中包含 cookie、CSRF、完整私有 URL、人员信息、真实内部标识、原始内部响应或私人内容。

测试必须使用合成 host、标识和文本。真实问题应缩减为最小脱敏 fixture。

## 开发与验证

```bash
go test ./...
go vet ./...
CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=$(cat VERSION)" -o /tmp/ixf-go ./cmd/ixf
scripts/smoke-go-binary.sh /tmp/ixf-go "$(cat VERSION)"
```

行为变更先写失败测试，再实现最小修复。影响用户的变更需要同步更新 README 和 `CHANGELOG.md`。

## Pull Request

PR 应说明：

- 问题和用户可感知的变化。
- 是否涉及实际远程写入或删除。
- 覆盖该行为的测试及执行结果。
- 隐私与脱敏检查。

Skill 应保持为 `ixf` 的轻量包装，不重复实现网络读写逻辑。
