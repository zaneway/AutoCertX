# AutoCertX GA一期与后续需求规划 V1.0

- 编写日期：2026-04-15
- 适用范围：首个可交付 GA 版本范围冻结、需求拆分、里程碑排期
- 关联文档：`doc/需求说明书v1.0.md`

## 1. 目标与冻结决策

### 1.1 一期 GA 目标

首个 GA 版本必须形成如下完整自动化闭环：

- 证书自动签发
- 证书自动续期
- 证书自动部署
- 部署后验证
- 存量证书发现与有效期检测

### 1.2 已冻结范围

- CA：Let's Encrypt
- 协议：ACME v2
- 挑战方式：HTTP-01、DNS-01（TXT）
- DNS Provider：阿里云 DNS
- 目标中间件：NGINX、Tomcat
- Tomcat 范围：仅支持 `JSSE + PKCS12`
- 密码算法：一期仅支持 `RSA`
- 执行模型：客户侧多 Agent 节点

### 1.3 一期成功标准

对于已纳管的 NGINX 和 Tomcat 服务，平台应支持：

- 提供完善的 Web 管理功能，支持登录认证、证书管理、查询统计和作业追踪
- 选择 Let's Encrypt 进行证书申请
- 根据域名和 challenge 类型完成自动验证
- 生成 RSA 私钥与 CSR
- 下载证书并自动部署到目标服务
- 验证部署结果并支持失败回滚
- 在续期窗口内自动完成更新
- 发现服务当前配置的证书及剩余有效期

## 2. 一期 GA 功能范围

### 2.1 控制面基础能力

#### P0 必做

- 多租户基础模型：租户、项目、环境
- 账号密码登录/注册
- 预置角色 + 基础 RBAC
- 基础审计日志
- Web 管理控制台
- 作业中心与任务状态跟踪
- OpenAPI 3.0 文档

#### 验收要求

- 用户可通过账号密码完成登录
- 不同租户的数据、作业、审计必须隔离
- 作业执行状态可查询、可重试、可审计
- Web 控制台必须提供登录认证、证书列表、证书详情、作业中心、基础查询统计能力

### 2.2 Agent 执行面

#### P0 必做

- 多 Agent 节点注册、心跳、版本上报
- Agent 分组、标签、用途标记
- 任务拉取执行模型
- 幂等执行、失败重试、断线恢复
- mTLS 双向认证
- Agent 本地私钥生成与本地部署执行

#### Agent 在一期承担的职责

- RSA 私钥生成
- CSR 生成
- HTTP-01 challenge 文件落地
- NGINX/Tomcat 证书部署
- NGINX/Tomcat 配置扫描
- 本地证书解析与有效期检测
- 部署结果和发现结果回传

### 2.3 Let's Encrypt / ACME 客户端管理

#### P0 必做

- 内置 ACME 客户端
- ACME 目录发现
- ACME 账户管理
- ACME 账户密钥生成/导入
- order / authorization / challenge / finalize / download 完整流程
- 证书下载与续期复用
- ACME 账户状态查询与审计

#### 一期约束

- 一期仅对接 Let's Encrypt
- 一期仅支持 RSA 证书
- 一期证书类型支持：
  - 单域名证书
  - 多 SAN 证书
  - 泛域名证书

#### 验收要求

- 能对同一租户管理多个 ACME 账户
- 能查看 challenge 状态、失败原因、重试状态
- 泛域名证书申请时只能选择 DNS-01（TXT）

### 2.4 域名验证能力

#### P0 必做

- HTTP-01 自动验证
- DNS-01（TXT）自动验证
- 阿里云 DNS 适配器
- challenge 信息展示
- challenge 状态跟踪

#### HTTP-01 一期要求

- 能在目标站点暴露 `/.well-known/acme-challenge/`
- 能展示 challenge 路径、token/value、验证状态
- 能在 challenge 结束后清理临时文件

#### DNS-01（TXT）一期要求

- 只支持阿里云 DNS
- 支持配置阿里云访问凭据
- 支持自动创建、更新、删除 `_acme-challenge` TXT 记录
- 支持展示记录名、记录值、状态和失败原因

#### 一期不做

- TLS-ALPN-01
- 第三方 DNS Provider 矩阵
- challenge 编排的高级策略路由

### 2.5 证书申请与自动续期

#### P0 必做

- 证书申请单创建
- 选择 CA、证书类型、challenge 方式
- 自动签发工作流
- 自动续期调度
- 幂等键、防重、失败重试
- 续期结果审计

#### 一期要求

- 新申请与续期共用同一套 ACME 工作流
- 续期默认复用原 challenge 方式
- 续期前后必须有明确状态流转
- 签发失败与续期失败必须可重试、可告警、可审计

### 2.6 目标部署连接器

#### P0 必做目标

- NGINX
- Tomcat（仅 `JSSE + PKCS12`）

#### NGINX 一期要求

- 证书文件写入
- 配置校验
- reload
- 部署后验证
- 回滚到上一版本

#### Tomcat 一期要求

- 支持将证书和私钥转换为 `PKCS12`
- 支持更新 `server.xml` 或关联的 keystore 配置引用
- 支持重载或受控重启
- 支持部署后验证
- 支持回滚到上一版本 keystore

#### 一期不做

- Tomcat `JKS`
- Tomcat APR / Native 模式
- NGINX/Tomcat 以外的目标连接器

### 2.7 证书发现与有效期检测

#### P0 必做

- NGINX 配置扫描
- Tomcat 配置扫描
- 本地证书解析
- 服务与证书绑定关系建立
- 剩余有效期计算
- 未纳管/异常证书识别

#### NGINX 扫描范围

- `ssl_certificate`
- `ssl_certificate_key`
- `server_name`
- 关联证书文件与服务实例

#### Tomcat 扫描范围

- `server.xml`
- `SSLHostConfig`
- `Certificate`
- `keystoreFile`
- `keystoreType`

#### 验收要求

- 平台能输出“哪个节点的哪个服务用了哪张证书、剩余多少天到期”
- 对丢失证书文件、解析失败、配置异常给出明确状态
- 发现结果能区分已纳管和未纳管证书

### 2.8 资产台账与审计

#### P0 必做

- 证书资产列表与详情
- 签发记录、部署记录、续期记录
- 发现来源与发现状态
- 到期时间线
- 审计查询与导出

#### 一期要求

- 资产详情必须串起申请、签发、部署、发现、续期
- 任意一次失败都必须保留结构化错误信息

### 2.9 通知与告警

#### P0 必做

- 站内待办
- Webhook
- 到期告警
- challenge 失败告警
- 部署失败告警

#### 一期不做

- 邮件通知
- 钉钉/飞书/企业微信

## 3. 一期 GA 非目标

- Vault PKI
- 自建 CA 个性化适配
- F5、防火墙、云服务连接器
- HAProxy、Kubernetes Secret、SSH
- OIDC/SAML
- ECDSA、SM2、ECC、抗量子算法
- 除阿里云以外的 DNS Provider
- 复杂计费和商业化结算

## 4. 后续需求规划

### 4.1 二期

#### CA 与认证

- Vault PKI
- OIDC / SAML
- 邀请注册、组织级身份集成

#### 挑战与域名验证

- 更多 DNS Provider
- challenge 策略路由
- 更复杂的自动验证编排

#### 目标连接器

- HAProxy
- Kubernetes Secret
- SSH

#### 通知能力

- 邮件
- 钉钉
- 飞书
- 企业微信

#### 算法能力

- SM2
- ECDSA / ECC

### 4.2 三期及以后

- 个性化自建 CA 适配
- F5 / 防火墙类设备
- 云服务集成
- 抗量子算法预研与适配
- 更复杂的证书发现（网络级 TLS 探测、K8s 扫描、云资源扫描）
- 商业版高级治理能力

## 5. 一期排期建议

### 5.1 里程碑拆分

#### M1：控制面与 Agent 基础（第 1-2 周）

- 多租户基础模型
- 账号密码登录/注册
- 基础 RBAC
- 审计骨架
- Agent 注册、心跳、mTLS
- 任务框架

#### M2：ACME 与 challenge（第 3-4 周）

- 内置 ACME 客户端
- Let's Encrypt 对接
- ACME 账户管理
- HTTP-01 自动验证
- 阿里云 DNS-01（TXT）自动验证

#### M3：签发、续期与资产（第 5-6 周）

- 自动签发流程
- 自动续期调度
- 证书资产台账
- challenge 状态跟踪
- 作业中心

#### M4：部署连接器（第 7-8 周）

- NGINX 部署、验证、回滚
- Tomcat `JSSE + PKCS12` 部署、验证、回滚
- Webhook

#### M5：发现、告警与 GA 收口（第 9-10 周）

- NGINX/Tomcat 配置扫描
- 证书发现与有效期检测
- 到期/部署/challenge 告警
- 稳定性修复
- GA 验收与文档收口

### 5.2 发布门槛

GA 发布前必须满足：

- Let's Encrypt 证书可自动签发
- HTTP-01、DNS-01（TXT, 阿里云）可自动验证
- NGINX/Tomcat 可自动部署与自动续期
- 发现链路可识别 NGINX/Tomcat 已配置证书
- 失败回滚、失败审计、失败告警可用
- 多 Agent 节点场景下不出现重复签发或重复部署

## 6. 工程风险与前置依赖

### 6.1 前置依赖

- 阿里云 DNS 访问凭据与最小权限策略
- Let’s Encrypt 测试/生产环境切换机制
- Tomcat `PKCS12` 导入与重载策略
- NGINX/Tomcat 目标机文件权限与 reload/restart 权限

### 6.2 一期主要风险

- Let’s Encrypt 频率限制影响联调和回归
- DNS-01 受阿里云 API、TTL、生效延迟影响
- Tomcat `PKCS12` 部署兼容性高于 NGINX
- 多 Agent 节点的幂等和任务互斥实现复杂

### 6.3 风险应对

- 默认接入 ACME staging 环境做联调
- DNS challenge 增加传播等待和重试机制
- Tomcat 一期只支持 `JSSE + PKCS12`
- 所有签发、部署、续期任务使用幂等键和状态机约束
