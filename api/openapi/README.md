# OpenAPI Contracts

该目录用于冻结一期控制面 REST API、Agent 协议、共享错误码和后续生成物。

当前结构：

- `openapi.json`
  - 一期 OpenAPI 初版
- `errors.json`
  - 共享错误码目录
- `contracts_test.go`
  - 契约覆盖与静态校验

约束：

- 协议字段、状态字段、错误码禁止写入中文
- 面向控制台的接口与 Agent 协议统一采用 `REST + JSON`
- OpenAPI 作为 `T02` 冻结输入，后续任务只允许消费，不应擅自改动语义
