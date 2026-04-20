# Control Plane Layout

控制面后端不再建议按“业务模块平铺”方式直接展开，而应和详细设计第 24 章保持一致，采用分层结构。

推荐结构：

- `internal/app/controlplane`
  - 进程装配、HTTP 路由、中间件、生命周期管理
- `internal/domain`
  - 领域对象与状态机
- `internal/application`
  - command / query 用例层
- `internal/repository`
  - 持久化抽象与实现
- `internal/workflow`
  - 签发、挑战、部署、续期编排
- `internal/driver`
  - ACME、DNS、Agent transport 等外部适配
- `internal/scheduler`
  - `jobs + job_attempts + claim/lease` 调度机制

当前前端控制台是强聚合、强编排的治理台，不是纯 CRUD 后台，因此控制面必须显式保留：

- `query` 聚合层
- 领域边界
- 外部驱动层

不要直接把前端导航一一映射成后端大模块，尤其不要把 `交付管理` 实现成统一领域对象。

详细评估见：

- `doc/后端代码结构评估与重构建议.md`
