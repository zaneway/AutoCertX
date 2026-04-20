# ACME Protocol Package

该目录保留给内部 `ACME` 协议子系统。

- 一期基础实现选用 `golang.org/x/crypto/acme`
- 业务层不得直接依赖第三方 ACME 原生类型
- 私有扩展字段和后续 CA 适配应收敛在该目录内
