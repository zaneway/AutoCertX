# AutoCertX

AutoCertX 是一个面向企业混合基础设施的证书生命周期管理平台，目标是提供 `SaaS 控制面 + 客户侧多 Agent 执行面` 的证书自动化能力，覆盖证书申请、验证、签发、部署、续期、发现、审计与资产可视化。

当前仓库已经从“纯需求文档”演进到“`文档驱动 + 工程骨架 + 契约冻结 + 基线业务实现 + 前端原型`”阶段，`T00/T01/T02/T03/T04/T05/T06` 已完成基线实现。当前控制面已具备认证与 RBAC、域名/DNS/CA 治理、Job/Lease/Scheduler、审计查询、同步 CSV 导出、系统设置和基础 Webhook 状态跟踪能力。若你正在评估项目范围、架构方向或一期交付边界，应优先阅读 [doc/需求说明书.md](/Users/zaneway/SynologyDrive/code-space/GolandProjects/learn/AutoCertX/doc/需求说明书.md:1)、[doc/GA一期与后续需求规划.md](/Users/zaneway/SynologyDrive/code-space/GolandProjects/learn/AutoCertX/doc/GA一期与后续需求规划.md:1) 和 [doc/一期GA详细设计.md](/Users/zaneway/SynologyDrive/code-space/GolandProjects/learn/AutoCertX/doc/一期GA详细设计.md:1)。

## 项目定位

- 面向中大型企业的企业级证书生命周期平台
- 产品主形态为 `SaaS`，同时支持用户侧部署
- 私钥不进入控制面，密钥生成和使用下沉到客户侧 Agent
- 控制面负责编排、策略、审计、资产台账和可视化
- 执行面负责密钥生成、challenge 执行、证书部署、配置扫描和发现回传

## 核心架构

### 控制面

- 多租户模型：租户、项目、环境
- Web 控制台
- ACME 客户端管理
- 证书资产台账
- 审计与作业中心
- 策略与权限控制

### 执行面

- 多 Agent 节点注册、心跳、任务拉取
- 本地私钥生成与 CSR 生成
- HTTP-01 challenge 文件处理
- DNS-01 challenge 执行协同
- NGINX / Tomcat 证书部署
- 本地配置扫描、证书发现、有效期检测

## 一期 GA 冻结范围

当前一期 GA 冻结范围为：

- `Let's Encrypt`
- `ACME`
- `HTTP-01`
- `DNS-01(TXT, 阿里云)`
- `RSA`
- `NGINX`
- `Tomcat(JSSE + PKCS12)`
- Web 控制台、作业中心、资产台账、发现与审计

更完整的范围、成功标准和里程碑定义见 [doc/GA一期与后续需求规划.md](/Users/zaneway/SynologyDrive/code-space/GolandProjects/learn/AutoCertX/doc/GA一期与后续需求规划.md:18)。

## 当前工程形态

当前仓库采用单仓组织，目标是承载以下四类内容：

- 产品与架构文档
- 控制面后端工程
- 客户侧 Agent 工程
- 前端控制台与接口契约

从工程形态上看，它不是“单一 Go 服务仓库”，而是一个面向一期 GA 的 `monorepo`：

- `Go 1.26.2` 作为后端与 Agent 主语言
- `Vue 3` 作为正式前端技术路线
- `SQL + migrations` 承载 PostgreSQL 初始化与后续 migration 基线
- `api/openapi` 作为控制面 API、Agent 协议和错误码契约冻结位置
- `internal/repository` 承载 PostgreSQL 迁移清单与持久化基线
- `doc` 作为需求、详细设计、选型和研发规划的事实源

## 当前工程结构

当前仓库目录已经按目标架构预分层，主要结构如下：

```text
.
├── api/
│   └── openapi/                 # 控制面 API、Agent 协议与共享契约冻结位置
├── cmd/
│   ├── agent/                   # Agent 启动入口
│   └── controlplane/            # 控制面启动入口
├── doc/                         # 需求、详细设计、页面设计、研发规划、选型分析
├── internal/
│   ├── agent/                   # 执行面内部模块边界
│   ├── app/
│   │   └── controlplane/        # 控制面进程装配、HTTP 路由、中间件、生命周期管理
│   ├── controlplane/            # 控制面结构规划说明
│   ├── repository/              # PostgreSQL 迁移清单与仓储基线
│   └── platform/                # 平台层：配置、日志、运行时、构建信息
├── migrations/                  # migration 包装文件与后续版本升级入口
├── pkg/
│   └── protocol/
│       └── acme/                # 内部 ACME 协议子系统
├── scripts/                     # 验证、初始化与研发辅助脚本
├── sql/
│   └── 001_init_schema.sql      # PostgreSQL 初始化基线 DDL
├── web/
│   ├── console/                 # 控制台原型与后续前端工程目录
│   └── README.md                # 前端原型说明
├── LICENSE
└── README.md
```

### 各目录职责

`doc/`
- 项目的需求事实源和设计事实源
- 当前最重要的研发输入都在这里冻结

`cmd/controlplane`
- 控制面可执行程序入口
- 后续负责装配 HTTP 服务、调度器、作业处理器、配置和日志

`cmd/agent`
- Agent 可执行程序入口
- 后续负责装配注册、心跳、轮询、执行器和本地证据回传

`internal/platform`
- 与具体业务无关的底层能力
- 当前已包含构建信息、配置、日志和运行时封装

`internal/app/controlplane`
- 控制面应用装配层
- 当前已落地：
  - `bootstrap`
  - `http`
  - `middleware`
  - `wiring`
- 负责把平台层基础能力装配成可运行的控制面进程骨架

`internal/controlplane`
- 控制面主业务目录
- 当前作为“后续业务模块分层规划说明”保留，不再作为平铺业务实现目录扩张
- 现阶段以“分层结构”作为正式口径，而不是继续按旧的业务模块平铺展开
- 推荐收敛为：
  - `internal/app/controlplane`
  - `internal/domain`
  - `internal/application`
  - `internal/repository`
  - `internal/workflow`
  - `internal/driver`
  - `internal/scheduler`
- 详细评估见 [doc/后端代码结构评估与重构建议.md](/Users/zaneway/SynologyDrive/code-space/GolandProjects/learn/AutoCertX/doc/后端代码结构评估与重构建议.md:1)

`internal/agent`
- Agent 侧执行能力目录
- 当前按能力边界预留，后续展开：
  - `bootstrap`
  - `poller`
  - `executor`
  - `keymgr`
  - `challenge/http01`
  - `deploy/nginx`
  - `deploy/tomcat`
  - `verify/nginx`
  - `verify/tomcat`
  - `discover/nginx`
  - `discover/tomcat`
  - `evidence`
  - `reporter`

`pkg/protocol/acme`
- 内部 ACME 协议子系统
- 一期以标准 ACME 为主，后续承接私有扩展字段、流程和算法扩展
- 业务层不应直接依赖第三方 ACME 原生类型

`api/openapi`
- 控制面 REST API、Agent 协议和共享契约的冻结位置
- 当前已落地：
  - `openapi.json`
  - `errors.json`
  - `contracts_test.go`
  - `README.md`
- 后续用于生成 OpenAPI 文档、SDK 或共享模型

`sql/`
- 当前保存 PostgreSQL 初始化 schema
- 后续可按 migration 工具拆分为初始化、索引、种子和升级脚本

`migrations/`
- 当前保存首个迁移包装文件 `0001_init_schema.sql`
- 与 `sql/001_init_schema.sql` 形成“设计事实源 + 可执行迁移入口”的组合

`internal/repository`
- 当前已落地 `postgres/manifest.go` 与 `manifest_test.go`
- 用于冻结迁移清单、表结构基线和后续仓储实现入口

`scripts/`
- 当前已落地 `verify_ddl.sh`
- 用于真库初始化校验和后续集成/E2E 辅助脚本

`web/console`
- 当前包含低保真 HTML 页面原型
- 后续正式前端工程将迁移为 `Vue 3 + TypeScript + Vite + Vue Router + Pinia + @tanstack/vue-query`

### 当前已存在的工程骨架

目前仓库内已经存在以下可识别的实现骨架：

- Go 启动入口：
  - `cmd/controlplane/main.go`
  - `cmd/agent/main.go`
- 平台层基础代码：
  - `internal/platform/config`
  - `internal/platform/logging`
  - `internal/platform/runtime`
  - `internal/platform/buildinfo`
- 控制面装配骨架：
  - `internal/app/controlplane/bootstrap`
  - `internal/app/controlplane/http`
  - `internal/app/controlplane/middleware`
  - `internal/app/controlplane/wiring`
- 数据库初始化：
  - `sql/001_init_schema.sql`
- 迁移与仓储基线：
  - `migrations/0001_init_schema.sql`
  - `internal/repository/postgres/manifest.go`
  - `internal/repository/postgres/manifest_test.go`
  - `scripts/verify_ddl.sh`
- 契约冻结产物：
  - `api/openapi/openapi.json`
  - `api/openapi/errors.json`
  - `api/openapi/contracts_test.go`
- 前端页面原型：
  - `web/console/index.html`
  - `web/console/domains.html`
  - `web/console/requests.html`
  - `web/console/audit.html`
  - `web/console/ca-accounts.html`
  - `web/console/delivery.html`
  - `web/console/discoveries.html`
  - `web/console/jobs.html`
  - `web/console/settings.html`

需要注意：

- 当前 `web/console/*.html` 是原型页面，不代表最终前端工程组织
- 最新页面信息架构和导航设计，以 [doc/前端页面设计.md](/Users/zaneway/SynologyDrive/code-space/GolandProjects/learn/AutoCertX/doc/前端页面设计.md:1) 为准
- 最新前端导航已经收敛为 `证书资产` 内发起申请、`交付管理` 聚合部署目标和节点管理

## 推荐阅读顺序

如果你是首次进入仓库，建议按以下顺序阅读：

1. [doc/需求说明书.md](/Users/zaneway/SynologyDrive/code-space/GolandProjects/learn/AutoCertX/doc/需求说明书.md:1)
2. [doc/GA一期与后续需求规划.md](/Users/zaneway/SynologyDrive/code-space/GolandProjects/learn/AutoCertX/doc/GA一期与后续需求规划.md:1)
3. [doc/一期GA详细设计.md](/Users/zaneway/SynologyDrive/code-space/GolandProjects/learn/AutoCertX/doc/一期GA详细设计.md:1)
4. [doc/前端页面设计.md](/Users/zaneway/SynologyDrive/code-space/GolandProjects/learn/AutoCertX/doc/前端页面设计.md:1)
5. [doc/开源组件选型与扩展性设计.md](/Users/zaneway/SynologyDrive/code-space/GolandProjects/learn/AutoCertX/doc/开源组件选型与扩展性设计.md:1)
6. [doc/AI系统开发规划.md](/Users/zaneway/SynologyDrive/code-space/GolandProjects/learn/AutoCertX/doc/AI系统开发规划.md:1)

## 仓库状态

当前仓库处于“详细设计已冻结、工程骨架已预留、业务代码逐步落地”阶段：

- 文档已形成较完整的一期 GA 交付边界和详细设计
- `T00` 已完成：Go 工程入口、控制面装配壳层、平台层和目录边界已建立
- `T01` 已完成：SQL 初始化脚本、migration 包装文件、仓储基线与真库 DDL 校验脚本已生成
- `T02` 已完成：OpenAPI、错误码和契约测试已冻结
- `T03` 已完成：身份、租户上下文、RBAC、语言偏好与 `/api/v1/auth/*` 基线已落地
- `T04` 已完成：域名、DNS 凭据、CA 账户治理与对应治理 API 基线已落地
- `T05` 已完成：`job` 域模型、planner/worker/reaper、PostgreSQL `SKIP LOCKED` 仓储与调度集成验证已落地
- `T06` 已完成：审计域、系统设置域、`/api/v1/audit-events*`、`/api/v1/settings/*`、同步 CSV 导出产物、导出记录状态跟踪与基础 Webhook 状态机已落地
- 前端已有页面原型，但正式 Vue 3 工程尚未完整迁移
- `README` 反映的是当前需求、设计和工程骨架状态，而不是全部功能均已实现的状态

## 许可说明

当前仓库包含的许可证文件为 `GNU LGPL v2.1`，详见 [LICENSE](/Users/zaneway/SynologyDrive/code-space/GolandProjects/learn/AutoCertX/LICENSE:1)。

需要注意：

- 需求文档中提到了“开源基础版 + 商业高级版”以及可能的授权限制策略
- 但**仓库当前实际生效的许可**以 `LICENSE` 文件为准
- 如果后续需要引入“双许可证”或“限制直接商用/SaaS 转售”的授权模型，应通过明确的许可证变更或附加授权文件落地，而不是仅停留在需求描述中

这部分属于产品与法律策略，不应由 `README` 单独重定义。

## 适合谁阅读这个仓库

- 正在评估企业证书生命周期平台产品方向的人
- 正在做证书自动化、ACME、部署编排、发现扫描方案设计的人
- 需要先冻结一期 GA 范围，再进入架构设计和工程实现的人
- 准备基于当前详细设计，推进控制面、Agent 和前端并行研发的人
