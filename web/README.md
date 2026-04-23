# AutoCertX 前端原型

当前目录保存的是可直接浏览器打开的 HTML 原型，用于评审一期 GA 的页面信息架构、视觉方向和主业务链。

这套原型已经对齐：

- `doc/前端页面设计.md`
- `doc/一期GA详细设计.md`

## 当前结构

```text
web/
├── console/
│   ├── app.html                # 正式 Vue 前端入口
│   ├── package.json            # Vite + Vue 3 + TypeScript 工程配置
│   ├── src/
│   │   ├── app/               # 路由、布局、页面入口
│   │   └── shared/            # API client、鉴权、i18n、权限与 query 基线
│   ├── prototype.css          # 共享浅色主题与页面组件样式
│   ├── prototype.js           # 共享左侧菜单、顶部导航、版本/时间/角色信息
│   ├── index.html             # 仪表盘
│   ├── domains.html           # 域名管理
│   ├── assets.html            # 证书资产工作台
│   ├── requests.html          # 资产内发起申请向导
│   ├── ca-accounts.html       # CA 账户
│   ├── delivery.html          # 交付管理
│   ├── discoveries.html       # 发现结果
│   ├── jobs.html              # 作业中心
│   ├── audit.html             # 审计
│   └── settings.html          # 系统设置
└── README.md
```

## 正式前端工程

`T13` 已开始在 `web/console` 旁路搭建正式前端工程，当前策略是：

- 保留现有 `HTML` 原型文件作为设计输入，不移动、不覆盖
- 新增 `app.html` 作为 `Vite` 入口页，避免和原型 `index.html` 冲突
- 正式前端技术栈：
  - `Vue 3`
  - `TypeScript`
  - `Vite`
  - `Vue Router`
  - `Pinia`
  - `@tanstack/vue-query`
  - `vue-i18n`
- 当前已落地的正式能力：
  - 登录页
  - 路由守卫
  - 左侧导航与顶栏壳层
  - `zh-CN / en-US` 切换与偏好回写
  - API client、query key、权限映射基线
- 当前仍为占位容器的页面：
  - 治理页由 `T14` 接入真实业务组件
  - 运行页由 `T15` 接入真实业务组件

## 原型说明

### 统一壳层

所有页面都统一采用：

- 左侧菜单栏
- 顶部导航栏
- 右上角系统信息区
  - 当前系统版本
  - 当前时间
  - 当前角色
  - 当前租户 / 项目 / 环境
  - 语言切换（`中文 / EN`）
- 下方核心内容区

### 当前导航

左侧菜单已经按最新 IA 收敛为：

- 仪表盘
- 域名管理
- 证书资产
- CA 账户
- 交付管理
- 发现结果
- 作业中心
- 审计
- 系统设置

说明：

- `证书申请` 已并入 `证书资产`
- `部署目标` 与 `节点管理` 已合并为 `交付管理` 导航域
- `requests.html` 仍保留为资产内跳转的申请向导页面，不再作为一级菜单
- 原型已支持 `zh-CN / en-US` 双语切换，并通过本地存储记住当前语言选择

## 如何查看

直接用浏览器打开 `web/console/index.html` 即可。

建议按以下顺序浏览：

1. `index.html` 看整体视觉和控制台壳层
2. `assets.html` 看一期主工作台
3. `delivery.html` 看部署目标与节点管理的合并方式
4. `jobs.html`、`discoveries.html` 看排障与认领链路
5. `requests.html` 看资产内发起申请的向导页

正式 Vue 前端工程启动后，可通过以下方式运行：

1. 在 `web/console` 下安装依赖：`npm install`
2. 启动开发服务器：`npm run dev`
3. 打开 `app.html` 对应入口

## 技术说明

- 原型技术：纯 `HTML + CSS + JavaScript`
- 样式方案：共享 `prototype.css`
- 壳层逻辑：共享 `prototype.js`
- 字体：`IBM Plex Sans` + `JetBrains Mono`
- 布局方向：浅色系、风险治理台、信息密度高但结构清晰

## 后续演进

这套原型的目标不是长期维护静态页面，而是为正式前端工程提供设计输入。后续会迁移到：

- `Vue 3`
- `TypeScript`
- `Vite`
- `Vue Router`
- `Pinia`
- `@tanstack/vue-query`

迁移时建议保留：

- 当前导航收敛方式
- 当前页面信息分区
- 顶栏右侧的系统信息表达
- 资产工作台和交付工作台的双核心结构
