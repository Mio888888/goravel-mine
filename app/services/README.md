# Services 目录

服务代码按能力域和模块两级组织：

- `access/`：认证、授权、Casbin、SSO。
- `application/`：跨模块应用编排和兼容 API 的实际实现。
- `platform/`：字典、日志、组织、参考案例、存储。
- `runtime/`：迁移锁、可观测性、队列、调度任务、密钥轮换。
- `tenancy/`：租户运行时基础能力。
- `facade.go`：原 `services.*` API 的兼容门面。

新增可复用服务应放入对应能力域的模块目录。只有跨模块编排允许进入
`application/`，根目录不得新增业务实现文件。
