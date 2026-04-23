(function () {
    const STORAGE_KEY = "autocertx.prototype.locale";
    const DEFAULT_LOCALE = "zh-CN";
    const SUPPORTED_LOCALES = ["zh-CN", "en-US"];

    const appMeta = {
        version: "v1.4.1",
        role: {
            "zh-CN": "平台工程师",
            "en-US": "Platform Engineer",
        },
        tenant: {
            "zh-CN": "示例租户",
            "en-US": "Example Tenant",
        },
        project: "global-edge",
        environment: {
            "zh-CN": "生产",
            "en-US": "Production",
        },
    };

    const shellTexts = {
        "zh-CN": {
            brandEyebrow: "AutoCertX Console",
            brandTitle: "证书生命周期控制台",
            brandDesc: "浅色系原型聚焦一期 GA 主闭环，统一收敛资产治理、执行治理、作业追踪和审计视角。",
            navSection: "主导航",
            summaryTitle: "当前视角",
            summaryDesc: "证书申请已并入 证书资产，部署目标与节点管理已合并为 交付管理 导航域。",
            versionLabel: "版本",
            timeLabel: "当前时间",
            roleLabel: "当前角色",
            contextLabel: "上下文",
            languageLabel: "语言",
            pageSuffix: "AutoCertX",
        },
        "en-US": {
            brandEyebrow: "AutoCertX Console",
            brandTitle: "Certificate Lifecycle Console",
            brandDesc: "The light-themed prototype focuses on the GA v1 main loop and unifies asset governance, execution governance, job troubleshooting, and audit views.",
            navSection: "Main Navigation",
            summaryTitle: "Current View",
            summaryDesc: "Certificate requests are now embedded into Certificate Assets, and Deployment Targets plus Nodes are grouped under Delivery.",
            versionLabel: "Version",
            timeLabel: "Current Time",
            roleLabel: "Current Role",
            contextLabel: "Context",
            languageLabel: "Language",
            pageSuffix: "AutoCertX",
        },
    };

    const navItems = [
        {
            key: "dashboard",
            href: "./index.html",
            badge: "",
            label: {
                "zh-CN": "仪表盘",
                "en-US": "Dashboard",
            },
        },
        {
            key: "domains",
            href: "./domains.html",
            badge: "",
            label: {
                "zh-CN": "域名管理",
                "en-US": "Domains",
            },
        },
        {
            key: "assets",
            href: "./assets.html",
            badge: "18",
            label: {
                "zh-CN": "证书资产",
                "en-US": "Certificate Assets",
            },
        },
        {
            key: "ca-accounts",
            href: "./ca-accounts.html",
            badge: "",
            label: {
                "zh-CN": "CA 账户",
                "en-US": "CA Accounts",
            },
        },
        {
            key: "delivery",
            href: "./delivery.html",
            badge: "6",
            label: {
                "zh-CN": "交付管理",
                "en-US": "Delivery",
            },
        },
        {
            key: "discoveries",
            href: "./discoveries.html",
            badge: "12",
            label: {
                "zh-CN": "发现结果",
                "en-US": "Discoveries",
            },
        },
        {
            key: "jobs",
            href: "./jobs.html",
            badge: "9",
            label: {
                "zh-CN": "作业中心",
                "en-US": "Jobs",
            },
        },
        {
            key: "audit",
            href: "./audit.html",
            badge: "",
            label: {
                "zh-CN": "审计",
                "en-US": "Audit",
            },
        },
        {
            key: "settings",
            href: "./settings.html",
            badge: "",
            label: {
                "zh-CN": "系统设置",
                "en-US": "Settings",
            },
        },
    ];

    const pageConfigs = {
        dashboard: {
            navKey: "dashboard",
            eyebrow: {
                "zh-CN": "运行总览",
                "en-US": "Runtime View",
            },
            title: {
                "zh-CN": "仪表盘",
                "en-US": "Dashboard",
            },
            description: {
                "zh-CN": "面向证书生命周期的总览工作台，聚焦到期风险、挑战失败、部署异常和发现未纳管问题。",
                "en-US": "An operations workbench for certificate lifecycle management, focused on expiration risks, challenge failures, deployment anomalies, and unmanaged discoveries.",
            },
            breadcrumbs: {
                "zh-CN": ["运行总览", "仪表盘"],
                "en-US": ["Operations", "Dashboard"],
            },
        },
        domains: {
            navKey: "domains",
            eyebrow: {
                "zh-CN": "治理对象",
                "en-US": "Governance",
            },
            title: {
                "zh-CN": "域名管理",
                "en-US": "Domains",
            },
            description: {
                "zh-CN": "治理域名资产、默认 challenge 策略、DNS 凭据绑定与验证历史，为后续申请和续期提供稳定输入。",
                "en-US": "Govern domain assets, default challenge policies, DNS credential bindings, and validation history to provide stable input for issuance and renewal.",
            },
            breadcrumbs: {
                "zh-CN": ["治理对象", "域名管理"],
                "en-US": ["Governance", "Domains"],
            },
        },
        assets: {
            navKey: "assets",
            eyebrow: {
                "zh-CN": "生命周期",
                "en-US": "Lifecycle",
            },
            title: {
                "zh-CN": "证书资产",
                "en-US": "Certificate Assets",
            },
            description: {
                "zh-CN": "统一承载证书资产列表、版本、部署、发现、审计与发起申请入口，是一期核心生命周期工作台。",
                "en-US": "A unified lifecycle workspace for certificate assets, versions, deployments, discoveries, audit records, and the embedded request entry.",
            },
            breadcrumbs: {
                "zh-CN": ["生命周期", "证书资产"],
                "en-US": ["Lifecycle", "Certificate Assets"],
            },
        },
        request: {
            navKey: "assets",
            eyebrow: {
                "zh-CN": "申请向导",
                "en-US": "Request Wizard",
            },
            title: {
                "zh-CN": "发起证书申请",
                "en-US": "Create Certificate Request",
            },
            description: {
                "zh-CN": "从证书资产工作台进入申请向导，按域名、签发参数、部署目标和风险复核完成创建。",
                "en-US": "Launch the issuance wizard from the certificate asset workspace and complete the request by selecting domains, issuance parameters, deployment targets, and risk checks.",
            },
            breadcrumbs: {
                "zh-CN": ["生命周期", "证书资产", "发起申请"],
                "en-US": ["Lifecycle", "Certificate Assets", "Create Request"],
            },
        },
        "ca-accounts": {
            navKey: "ca-accounts",
            eyebrow: {
                "zh-CN": "签发治理",
                "en-US": "Issuer",
            },
            title: {
                "zh-CN": "CA 账户",
                "en-US": "CA Accounts",
            },
            description: {
                "zh-CN": "管理 ACME 账户、目录地址、能力摘要和关联申请，作为签发治理的前置资源。",
                "en-US": "Manage ACME accounts, directory endpoints, capability summaries, and linked requests as prerequisite resources for issuance governance.",
            },
            breadcrumbs: {
                "zh-CN": ["治理对象", "CA 账户"],
                "en-US": ["Governance", "CA Accounts"],
            },
        },
        delivery: {
            navKey: "delivery",
            eyebrow: {
                "zh-CN": "执行治理",
                "en-US": "Execution Plane",
            },
            title: {
                "zh-CN": "交付管理",
                "en-US": "Delivery",
            },
            description: {
                "zh-CN": "统一承载部署目标和 Agent 节点视角，导航合并但对象模型不合并，用于部署与排障联动。",
                "en-US": "A unified navigation domain for deployment targets and agent nodes. Navigation is merged, but the underlying domain models remain separate for deployment and troubleshooting workflows.",
            },
            breadcrumbs: {
                "zh-CN": ["执行治理", "交付管理"],
                "en-US": ["Execution Governance", "Delivery"],
            },
        },
        discoveries: {
            navKey: "discoveries",
            eyebrow: {
                "zh-CN": "发现治理",
                "en-US": "Discovery",
            },
            title: {
                "zh-CN": "发现结果",
                "en-US": "Discoveries",
            },
            description: {
                "zh-CN": "查看 NGINX 与 Tomcat 配置扫描结果，识别未纳管证书、异常证书和可认领资产。",
                "en-US": "Inspect NGINX and Tomcat scan results to identify unmanaged certificates, invalid certificates, and claimable assets.",
            },
            breadcrumbs: {
                "zh-CN": ["运行对象", "发现结果"],
                "en-US": ["Runtime Objects", "Discoveries"],
            },
        },
        jobs: {
            navKey: "jobs",
            eyebrow: {
                "zh-CN": "任务排障",
                "en-US": "Jobs",
            },
            title: {
                "zh-CN": "作业中心",
                "en-US": "Jobs",
            },
            description: {
                "zh-CN": "统一查看异步任务、attempt 历史、错误证据和重试入口，是排障的第一入口。",
                "en-US": "A unified troubleshooting entry for asynchronous jobs, attempt history, error evidence, and retry actions.",
            },
            breadcrumbs: {
                "zh-CN": ["运行对象", "作业中心"],
                "en-US": ["Runtime Objects", "Jobs"],
            },
        },
        audit: {
            navKey: "audit",
            eyebrow: {
                "zh-CN": "审计视图",
                "en-US": "Audit",
            },
            title: {
                "zh-CN": "审计",
                "en-US": "Audit",
            },
            description: {
                "zh-CN": "按 actor、resource、action 和时间范围检索审计事件，追踪配置变更、凭据使用和自动化动作。",
                "en-US": "Query audit events by actor, resource, action, and time range to trace configuration changes, credential usage, and automated actions.",
            },
            breadcrumbs: {
                "zh-CN": ["治理对象", "审计"],
                "en-US": ["Governance", "Audit"],
            },
        },
        settings: {
            navKey: "settings",
            eyebrow: {
                "zh-CN": "控制面设置",
                "en-US": "Control Settings",
            },
            title: {
                "zh-CN": "系统设置",
                "en-US": "Settings",
            },
            description: {
                "zh-CN": "配置 Webhook、续期窗口和基础安全项，所有关键修改都应留下审计与变更上下文。",
                "en-US": "Configure webhooks, renewal windows, and baseline security settings. All critical changes must leave an audit trail and change context.",
            },
            breadcrumbs: {
                "zh-CN": ["治理对象", "系统设置"],
                "en-US": ["Governance", "Settings"],
            },
        },
    };

    const contentTextMap = {
        "en-US": {
            "查看作业中心": "View Jobs",
            "进入证书资产": "Open Certificate Assets",
            "域名资产": "Domain Assets",
            "其中 96 个启用 DNS-01 自动验证": "96 of them already use DNS-01 automation.",
            "证书资产": "Certificate Assets",
            "18 个资产在最近 30 天内完成签发": "18 assets were issued in the last 30 days.",
            "即将到期": "Expiring Soon",
            "3 个资产在 7 天内进入强提醒窗口": "3 assets enter the high-alert window within 7 days.",
            "未纳管发现": "Unmanaged Discoveries",
            "主要集中在 Tomcat 边缘节点": "Mostly concentrated on Tomcat edge nodes.",
            "风险工作台": "Risk Workbench",
            "优先处理会直接影响续期闭环和生产部署的异常。": "Prioritize anomalies that directly impact the renewal loop or production deployment.",
            "高优先级 4": "4 High Priority",
            "DNS-01 challenge 失败": "DNS-01 challenge failure",
            "`*.edge.example.com` 在阿里云 DNS TXT 写入阶段被 API 限流。": "`*.edge.example.com` hit AliDNS API throttling during TXT upsert.",
            "待处理": "Pending",
            "Tomcat 部署后验证失败": "Tomcat post-deploy verification failed",
            "`node-sh02-07` 已完成 PKCS12 覆盖，但应用侧尚未 reload。": "`node-sh02-07` already replaced the PKCS12 bundle, but the application has not reloaded yet.",
            "观察中": "Observing",
            "发现未纳管证书": "Unmanaged certificate discovered",
            "在 `gateway-admin` 服务上发现 2 份 RSA 证书未与资产台账匹配。": "Two RSA certificates on `gateway-admin` are not matched to managed assets.",
            "可认领": "Claimable",
            "交付面健康": "Delivery Health",
            "节点与目标联合视角，聚焦执行能力是否稳定。": "A joint view of nodes and targets focused on execution stability.",
            "在线 41 / 44": "41 / 44 Online",
            "部署目标覆盖率": "Deployment Target Coverage",
            "NGINX 目标 26 个，Tomcat 目标 12 个，自动验证开启率 92%。": "26 NGINX targets, 12 Tomcat targets, with 92% auto-verification enabled.",
            "正常": "Healthy",
            "异常节点": "Abnormal Nodes",
            "3 个节点版本落后于控制面协议基线，建议进入交付管理升级。": "3 nodes are behind the control-plane protocol baseline and should be upgraded from Delivery.",
            "最近 7 天签发与失败趋势": "Issuance and Failure Trend (7 Days)",
            "蓝绿色柱体表示成功签发，橙色和红色表示失败与重试。": "Blue-green bars indicate successful issuance. Orange and red indicate failures and retries.",
            "今日建议动作": "Recommended Actions Today",
            "按影响范围和恢复收益排序，便于当班处理。": "Sorted by impact radius and recovery value for today's operator.",
            "先处理 2 个失败 challenge": "Handle 2 failed challenges first",
            "避免泛域名申请积压，优先检查阿里云凭据限流与 TXT 残留。": "Avoid wildcard request backlog. Check AliDNS rate limits and TXT leftovers first.",
            "09:00 - 建议处理窗口": "09:00 - suggested handling window",
            "认领 3 条未纳管发现": "Claim 3 unmanaged findings",
            "可将发现记录映射到现有资产，减少重复申请和台账偏差。": "Map findings to existing assets to reduce duplicate requests and inventory drift.",
            "11:30 - 认领优先": "11:30 - claim first",
            "升级异常节点协议版本": "Upgrade out-of-date agent nodes",
            "3 个节点尚未支持最新部署回执格式，会影响证据回传完整性。": "3 nodes do not support the latest deployment receipt format, affecting evidence completeness.",
            "14:00 - 交付窗口": "14:00 - delivery window",

            "查看关联资产": "View Related Assets",
            "新建域名资产": "Create Domain Asset",
            "域名总数": "Total Domains",
            "95 个生产域名，33 个测试域名": "95 production domains and 33 test domains.",
            "默认 DNS-01": "Default DNS-01",
            "泛域名均已收敛到 DNS-01": "All wildcard domains are constrained to DNS-01.",
            "最近验证失败": "Recent Validation Failures",
            "近 24h 主要为 TXT 写入失败": "Mostly TXT write failures in the last 24 hours.",
            "已绑定资产": "Bound Assets",
            "平均每个域名关联 1.7 个资产": "Each domain is linked to 1.7 assets on average.",
            "搜索域名": "Search Domains",
            "Challenge": "Challenge",
            "全部": "All",
            "状态": "Status",
            "启用": "Enabled",
            "停用": "Disabled",
            "环境": "Environment",
            "生产": "Production",
            "预发": "Pre-production",
            "域名列表": "Domain List",
            "列表同时展示治理属性和运行反馈，避免在申请时才发现凭据或 challenge 配置不完整。": "The list shows governance attributes and runtime feedback together, so missing credentials or challenge configuration are visible before issuing.",
            "停用域名": "Disabled Domains",
            "域名": "Domain",
            "默认 Challenge": "Default Challenge",
            "DNS 凭据": "DNS Credential",
            "最近验证": "Latest Validation",
            "关联资产": "Linked Assets",
            "通过": "Passed",
            "失败": "Failed",
            "观察": "Observe",
            "最近验证记录": "Recent Validation Records",
            "按域名和 challenge 方式跟踪最近一次校验结果。": "Track the latest validation result by domain and challenge type.",
            "AliDNS TXT 写入后 ACME 查询仍未命中，建议检查 TTL 与残留记录。": "ACME still could not see the TXT after AliDNS upsert. Check TTL and stale records.",
            "HTTP-01 验证通过，challenge 文件由 `node-hz01-03` 正常托管。": "HTTP-01 validation passed. The challenge file is hosted correctly on `node-hz01-03`.",
            "近期 TXT 操作": "Recent TXT Operations",
            "便于安全管理员快速回看 DNS-01 自动写入动作。": "Helps security admins quickly review automated DNS-01 writes.",
            "CREATE / 成功": "CREATE / success",
            "UPSERT / 失败": "UPSERT / failed",
            "DELETE / 成功": "DELETE / success",

            "发起证书申请": "Create Certificate Request",
            "查看失败作业": "View Failed Jobs",
            "当前资产": "Managed Assets",
            "含 28 份泛域名与 63 份多 SAN 证书": "Including 28 wildcard and 63 multi-SAN certificates.",
            "高风险资产": "High-Risk Assets",
            "已进入续期窗口但最近作业失败": "Already in the renewal window and recently failed.",
            "最近续期成功": "Recent Successful Renewals",
            "近 7 天自动续期成功率 94%": "94% automatic renewal success rate over the last 7 days.",
            "待处理申请": "Open Requests",
            "2 个 challenge 中，4 个部署中": "2 in challenge stage, 4 in deployment stage.",
            "资产工作台": "Asset Workspace",
            "把资产列表、申请记录、部署状态和发现状态统一放在一个生命周期入口。": "Unify asset list, request records, deployment state, and discovery state under a single lifecycle entry.",
            "资产列表": "Assets",
            "申请记录": "Requests",
            "资产名": "Asset",
            "当前版本": "Current Version",
            "到期时间": "Expires At",
            "部署状态": "Deployment",
            "发现状态": "Discovery",
            "目标数": "Targets",
            "风险": "Risk",
            "高": "High",
            "中": "Medium",
            "低": "Low",
            "部分失败": "Partially Failed",
            "已匹配": "Matched",
            "成功": "Successful",
            "待部署": "Pending Deploy",
            "未扫描": "Not Scanned",
            "最近申请与续期": "Recent Requests and Renewals",
            "不再单独设置一级导航，申请记录直接归入资产工作台查看。": "Requests are no longer promoted to a top-level menu; request records now live inside the asset workspace.",
            "申请 `*.edge.example.com`": "Request `*.edge.example.com`",
            "当前处于 DNS-01 校验阶段，首个 Job 已创建并关联到阿里云 TXT 写入任务。": "Currently in DNS-01 validation. The first job has been created and linked to the AliDNS TXT upsert task.",
            "续期 `api.example.com` 成功": "Renewal for `api.example.com` succeeded",
            "新版本已部署到 2 个 NGINX 目标，部署后验证通过。": "The new version has been deployed to 2 NGINX targets and passed post-deployment verification.",
            "进入申请向导": "Open Request Wizard",
            "高风险提示": "High-Risk Hints",
            "优先清理会阻断闭环的资产异常。": "Prioritize asset issues that can block the closed loop.",
            "发现未匹配": "Unmatched Discoveries",
            "续期窗口内失败": "Failed During Renewal Window",
            "资产内申请入口说明": "Request Entry Inside Assets",
            "原一级菜单已取消，避免把生命周期工作流拆散。": "The old top-level request menu is removed to keep the lifecycle workflow intact.",
            "推荐操作链路为：": "Recommended flow:",

            "返回证书资产": "Back to Assets",
            "Step 1: 上下文与域名": "Step 1: Context and Domains",
            "步骤 1：上下文和域名": "Step 1: Context and Domains",
            "选择项目 / 环境": "Select Project / Environment",
            "选择主域名": "Select Primary Domain",
            "添加 SAN": "Add SAN",
            "步骤 2：签发参数": "Step 2: Issuance Parameters",
            "选择 CA": "Select CA",
            "选择证书类型": "Select Certificate Type",
            "选择 challenge": "Select Challenge",
            "显示 challenge 限制说明": "Show challenge restrictions",
            "步骤 3：部署目标": "Step 3: Deployment Target",
            "选择部署目标": "Select Deployment Target",
            "展示目标类型、节点选择器、安装方式": "Show target type, node selector, and installation mode",
            "步骤 4：提交确认": "Step 4: Review and Submit",
            "汇总 CN、SAN、CA、challenge、目标": "Summarize CN, SAN, CA, challenge, and targets",
            "风险提示": "Risk Notes",
            "提交": "Submit",
            "申请步骤": "Request Steps",
            "按域名、签发参数、部署目标和风险复核逐步收敛，保持和后端校验规则一致。": "Work through domains, issuance parameters, deployment targets, and risk review step by step, aligned with backend validation rules.",
            "上下文与域名": "Context & Domains",
            "签发参数": "Issuance Parameters",
            "部署目标": "Deployment Target",
            "复核并提交": "Review & Submit",
            "当前示例演示泛域名申请，因此后续 challenge 将限制为 DNS-01。": "This example demonstrates a wildcard request, so the challenge is constrained to DNS-01.",
            "项目": "Project",
            "主域名": "Primary Domain",
            "SAN 域名": "SAN Domains",
            "CA": "CA",
            "证书类型": "Certificate Type",
            "提交预览": "Submission Preview",
            "提交后会创建 `CertificateRequest` 与首个 Job。": "Submitting will create the `CertificateRequest` and the first job.",
            "规则提示": "Rules",
            "这些规则应与后端校验口径完全一致。": "These rules must match backend validation rules exactly.",
            "泛域名证书只能选择": "Wildcard certificates can only use ",
            "。当前使用阿里云凭据 `alidns-edge` 自动写入 TXT 记录，提交后将进入作业中心跟踪。": ". The current request uses the `alidns-edge` AliDNS credential for automated TXT upserts and will enter Jobs after submission.",
            "如果部署目标未配置回滚路径或节点标签不匹配，申请虽然可创建，但后续部署作业会进入失败或待人工处理状态。": "If the deployment target lacks a rollback path or node labels do not match, the request can still be created but the deployment job will fail or require manual handling.",
            "取消": "Cancel",
            "保存草稿": "Save Draft",
            "提交并查看作业": "Submit and View Jobs",

            "查看审计": "View Audit",
            "新增 CA 账户": "Add CA Account",
            "账户总数": "Accounts",
            "2 个生产，2 个测试": "2 production and 2 test accounts.",
            "健康账户": "Healthy Accounts",
            "最近目录探测通过": "Directory health checks passed recently.",
            "关联申请": "Linked Requests",
            "近 30 天累计签发请求": "Issuance requests in the last 30 days.",
            "需要关注": "Needs Attention",
            "测试目录响应延迟升高": "Staging directory latency is elevated.",
            "CA 账户列表": "CA Account List",
            "展示账户状态、目录地址、能力摘要和最近探测时间。": "Shows account state, directory URL, capability summary, and latest probe time.",
            "账户": "Account",
            "目录地址": "Directory URL",
            "能力": "Capabilities",
            "最近检查": "Last Checked",
            "Let's Encrypt Production": "Let's Encrypt Production",
            "Let's Encrypt Staging": "Let's Encrypt Staging",
            "测试": "Test",
            "账户能力摘要": "Capability Summary",
            "用于前端向导中选择 CA 时的能力提示。": "Capability hints for CA selection in the request wizard.",
            "支持 challenge": "Supported Challenges",
            "默认算法": "Default Algorithm",
            "证书类型": "Certificate Types",
            "最近关联申请": "Recent Linked Requests",
            "把账户治理和申请行为放在同一上下文里查看。": "Review account governance and request behavior in the same context.",
            "当前进入 challenge。": "Currently in challenge phase.",
            "目录探测耗时偏高但仍完成签发。": "Directory probing was slower than expected but issuance still completed.",

            "创建注册令牌": "Create Registration Token",
            "新建部署目标": "Add Deployment Target",
            "部署目标": "Deployment Targets",
            "NGINX 26，Tomcat 12": "26 NGINX targets and 12 Tomcat targets.",
            "节点在线": "Online Nodes",
            "3 个节点需要升级或排障": "3 nodes require upgrade or troubleshooting.",
            "最近部署失败": "Recent Deployment Failures",
            "主要为 Tomcat reload 不生效": "Mostly caused by Tomcat reload not taking effect.",
            "发现异常": "Discovery Anomalies",
            "其中 3 条可直接认领到现有资产": "3 of them can be directly claimed into existing assets.",
            "交付管理页签": "Delivery Tabs",
            "导航已合并，但仍保留部署目标和节点管理两个子视角，不合并领域模型与接口。": "Navigation is merged, but deployment targets and nodes remain separate sub-views and do not merge domain models or APIs.",
            "当前页面原型同时展示两个子域，实际产品中可通过页签切换为独立列表与详情页。": "This prototype shows both sub-domains together. In the final product they can be switched as dedicated lists and detail views.",
            "目标类型、安装路径、节点选择器和最近部署结果在这里统一治理。": "Target type, installation path, node selector, and latest deployment results are governed here.",
            "目标": "Target",
            "类型": "Type",
            "节点选择器": "Node Selector",
            "最近部署": "Latest Deployment",
            "待验证": "Awaiting Verification",
            "节点管理": "Nodes",
            "节点健康、协议版本、能力和最近任务结果在这里查看。": "Inspect node health, protocol version, capabilities, and recent task results here.",
            "节点": "Node",
            "版本": "Version",
            "标签": "Labels",
            "最近心跳": "Latest Heartbeat",
            "在线": "Online",
            "升级中": "Upgrading",
            "最近部署异常": "Recent Deployment Issues",
            "通过交付域直接联动作业与资产排障。": "Use the delivery domain to jump directly into jobs and asset troubleshooting.",
            "PKCS12 已写入，但应用未切换新证书，需进入作业中心查看回执。": "PKCS12 is already written, but the application has not switched to the new certificate. Check the job receipt.",
            "`node-hz01-11` 缺少配置回滚路径，自动部署被阻断。": "`node-hz01-11` is missing the configured rollback path, so automated deployment is blocked.",
            "节点健康摘要": "Node Health Summary",
            "节点数值越低，说明协议版本、能力或心跳存在问题。": "Lower values indicate issues with protocol version, capabilities, or heartbeat.",
            "协议兼容度": "Protocol Compatibility",
            "大多数节点已支持最新部署回执格式。": "Most nodes already support the latest deployment receipt format.",
            "发现能力覆盖率": "Discovery Capability Coverage",
            "Tomcat 发现能力仍有 2 个节点未启用。": "Tomcat discovery capability is still disabled on 2 nodes.",

            "查看交付管理": "View Delivery",
            "认领到资产": "Claim to Asset",
            "总发现记录": "Discovery Records",
            "包含 NGINX 与 Tomcat 配置扫描结果": "Includes NGINX and Tomcat scan results.",
            "已匹配": "Matched",
            "可回溯到现有证书资产": "Already linked to existing certificate assets.",
            "未纳管": "Unmanaged",
            "建议优先认领或补录": "Claim or register them first.",
            "异常": "Invalid",
            "包括无效证书与路径缺失": "Including invalid certificates and missing paths.",
            "搜索服务 / 节点 / 域名": "Search service / node / domain",
            "匹配状态": "Match Status",
            "目标类型": "Target Type",
            "发现记录": "Discovery Records",
            "展示节点、服务、路径、指纹、到期时间和匹配结果，便于认领或忽略。": "Shows node, service, path, fingerprint, expiry time, and matching result for claim or ignore actions.",
            "服务": "Service",
            "配置路径": "Config Path",
            "待认领队列": "Claim Queue",
            "优先认领可回收进台账的记录，减少重复申请。": "Claim records that can be folded back into the inventory first to reduce duplicate requests.",
            "发现于 `node-sh02-07`，建议认领到 `console-pre` 资产。": "Discovered on `node-sh02-07`. It should likely be claimed to the `console-pre` asset.",
            "待复核": "Needs Review",
            "证书 SAN 与现有资产高度相似，可考虑人工复核后认领。": "The certificate SANs are highly similar to an existing asset. Consider manual review before claiming.",
            "异常说明": "Issue Notes",
            "这类问题通常需要和交付管理或作业中心联合排查。": "These issues usually require joint troubleshooting with Delivery or Jobs.",
            "`legacy-proxy` 证书将在 7 天内到期，但既未匹配现有资产，也没有可用部署目标，建议优先补录或手工处理。": "The `legacy-proxy` certificate expires within 7 days, is not mapped to an existing asset, and has no usable deployment target. Register or handle it manually first.",

            "查看审计链路": "View Audit Trail",
            "重试选中作业": "Retry Selected Jobs",
            "运行中": "Running",
            "包含 challenge、部署、发现三类任务": "Includes challenge, deployment, and discovery jobs.",
            "4 个可重试，2 个需人工处理": "4 can be retried automatically, 2 require manual handling.",
            "平均 attempts": "Average Attempts",
            "重试主要集中在 DNS-01 作业": "Retries are mostly concentrated in DNS-01 jobs.",
            "排队中": "Queued",
            "等待节点拉取或 lease 回收": "Waiting for node pickup or lease reclamation.",
            "搜索 Job / 资源 / 错误码": "Search job / resource / error code",
            "失败": "Failed",
            "运行中": "Running",
            "作业列表": "Job List",
            "以失败、attempt 和错误证据为核心，承担统一排障入口角色。": "Built around failures, attempts, and evidence as the unified troubleshooting entry.",
            "Job": "Job",
            "资源": "Resource",
            "优先级": "Priority",
            "最近错误": "Latest Error",
            "作业 attempt 时间线": "Attempt Timeline",
            "这里模拟 `jobs + job_attempts` 分离后的排障视图。": "This simulates the troubleshooting view after separating `jobs` and `job_attempts`.",
            "3 attempts": "3 attempts",
            "PKCS12 写入成功，Tomcat reload 卡住超时，lease 到期后触发重试。": "PKCS12 write succeeded, Tomcat reload timed out, and retry was triggered after lease expiration.",
            "reload 完成，但部署后验证返回旧证书指纹，系统标记失败。": "Reload completed, but post-deployment verification still returned the old fingerprint, so the system marked it as failed.",
            "准备进入人工重试窗口，需检查应用是否真正引用新 keystore。": "Entering the manual retry window. Check whether the application actually points to the new keystore.",
            "错误证据摘要": "Error Evidence Summary",
            "保持错误码、原始消息和关联资源可快速串联。": "Keep error codes, original messages, and linked resources easy to correlate.",

            "查看关联作业": "View Related Jobs",
            "导出审计视图": "Export Audit View",
            "今日事件": "Events Today",
            "申请、TXT 操作、部署和设置变更均已入审计": "Requests, TXT operations, deployments, and settings changes are all audited.",
            "高风险动作": "High-Risk Actions",
            "停用节点、修改 Webhook、重试作业": "Disable node, update webhook, retry jobs.",
            "凭据使用": "Credential Usage",
            "主要为 AliDNS TXT 写入和 ACME 账户调用": "Mostly AliDNS TXT upserts and ACME account calls.",
            "可导出窗口": "Export Window",
            "按租户隔离导出": "Exported with tenant isolation.",
            "搜索 actor / resource / trace_id": "Search actor / resource / trace_id",
            "动作": "Action",
            "时间范围": "Time Range",
            "最近 24h": "Last 24h",
            "最近 7d": "Last 7d",
            "审计事件列表": "Audit Event List",
            "提供 actor、resource、action 和 trace 维度检索。": "Supports querying by actor, resource, action, and trace dimensions.",
            "时间": "Time",
            "结果": "Result",
            "manual": "manual",
            "事件详情": "Event Details",
            "右侧保留 request_id、trace_id 和 detail 摘要，便于和后端日志串联。": "Preserves request_id, trace_id, and detail summary to correlate with backend logs.",
            "detail_jsonb 摘要：使用凭据 `alidns-edge` 对 `_acme-challenge.edge.example.com` 执行 UPSERT，TTL=60，返回 requestId=4F8A23CE。": "detail_jsonb summary: credential `alidns-edge` executed UPSERT on `_acme-challenge.edge.example.com` with TTL=60 and returned requestId=4F8A23CE.",

            "查看配置审计": "View Configuration Audit",
            "保存配置": "Save Settings",
            "Webhook": "Webhook",
            "2 个启用，1 个测试中": "2 enabled and 1 under test.",
            "续期窗口": "Renewal Window",
            "提前 15 天进入告警提醒": "Escalate reminders 15 days before expiration.",
            "安全基线": "Security Baseline",
            "一期只允许 RSA，后续扩展预留": "GA v1 only allows RSA, with extension points reserved.",
            "Webhook 设置": "Webhook Settings",
            "配置系统事件通知的下游接收端，变更后必须留痕。": "Configure downstream receivers for system events. All changes must be audited.",
            "2 active": "2 active",
            "启用 / deployment_failed": "enabled / deployment_failed",
            "启用 / audit_exported": "enabled / audit_exported",
            "测试中 / dns_record_upsert": "testing / dns_record_upsert",
            "续期窗口设置": "Renewal Window Settings",
            "控制自动续期启动和告警升级的时间点。": "Control when automatic renewal starts and when alerts are escalated.",
            "自动续期启动": "Auto Renewal Starts",
            "到期前 30 天": "30 days before expiry",
            "高风险提醒": "High-Risk Alert",
            "到期前 15 天": "15 days before expiry",
            "失败重试间隔": "Retry Interval",
            "15 分钟 / 3 次": "15 min / 3 attempts",
            "人工升级阈值": "Manual Escalation Threshold",
            "连续失败 3 次": "3 consecutive failures",
            "基础安全设置": "Baseline Security Settings",
            "一期重点约束算法、账户认证和敏感操作留痕。": "GA v1 mainly constrains algorithms, account authentication, and sensitive action auditing.",
            "允许算法": "Allowed Algorithm",
            "登录方式": "Sign-In Method",
            "账号密码": "Username / Password",
            "高风险动作": "High-Risk Actions",
            "二次确认 + 审计": "Double confirmation + audit",
            "配置变更提示": "Configuration Change Notes",
            "原型中直接展示高风险变更的影响范围。": "The prototype directly shows the impact scope of high-risk changes.",
            "修改续期窗口将影响当前 214 个证书资产的后续调度计算。保存前应确认运维窗口和告警策略已经同步。": "Changing the renewal window affects scheduling for the current 214 certificate assets. Confirm maintenance windows and alert policies before saving.",
        },
    };

    let currentLocale = loadLocale();
    let contentTextNodes = [];
    let contentOptionNodes = [];
    let pageKey = "dashboard";

    function loadLocale() {
        const saved = window.localStorage.getItem(STORAGE_KEY);
        if (SUPPORTED_LOCALES.includes(saved)) {
            return saved;
        }
        return DEFAULT_LOCALE;
    }

    function saveLocale(locale) {
        window.localStorage.setItem(STORAGE_KEY, locale);
    }

    function getConfigForPage(key) {
        return pageConfigs[key] || pageConfigs.dashboard;
    }

    function tShell(locale, key) {
        return shellTexts[locale][key];
    }

    function tPage(config, locale, key) {
        return config[key][locale];
    }

    function renderSidebar(activeKey, locale) {
        const target = document.getElementById("sidebar-slot");
        if (!target) {
            return;
        }

        const navMarkup = navItems.map((item) => {
            const activeClass = item.key === activeKey ? "nav-item nav-item--active" : "nav-item";
            const badge = item.badge ? `<span class="nav-item__badge">${item.badge}</span>` : "";
            return `
                <a class="${activeClass}" href="${item.href}">
                    <span class="nav-item__left">
                        <span class="nav-item__icon"></span>
                        <span class="nav-item__name">${item.label[locale]}</span>
                    </span>
                    ${badge}
                </a>
            `;
        }).join("");

        target.innerHTML = `
            <div class="sidebar__panel">
                <section class="brand">
                    <div class="brand__eyebrow">${tShell(locale, "brandEyebrow")}</div>
                    <h1 class="brand__title">${tShell(locale, "brandTitle")}</h1>
                    <p class="brand__desc">${tShell(locale, "brandDesc")}</p>
                </section>

                <section class="nav-section">
                    <div class="nav-section__label">${tShell(locale, "navSection")}</div>
                    <div class="nav-list">
                        ${navMarkup}
                    </div>
                </section>

                <section class="sidebar__summary">
                    <h3>${tShell(locale, "summaryTitle")}</h3>
                    <p>${tShell(locale, "summaryDesc")}</p>
                </section>
            </div>
        `;
    }

    function renderHeader(config, locale) {
        const breadcrumbTarget = document.getElementById("topbar-context");
        const metaTarget = document.getElementById("topbar-meta");
        const heroTarget = document.getElementById("page-hero-text");
        const localeZhClass = locale === "zh-CN" ? "locale-switch__btn locale-switch__btn--active" : "locale-switch__btn";
        const localeEnClass = locale === "en-US" ? "locale-switch__btn locale-switch__btn--active" : "locale-switch__btn";

        if (breadcrumbTarget) {
            const breadcrumbHtml = tPage(config, locale, "breadcrumbs").map((item) => `<span>${item}</span>`).join("");
            breadcrumbTarget.innerHTML = `
                <div class="breadcrumbs">${breadcrumbHtml}</div>
                <div class="topbar__heading">
                    <h1>${tPage(config, locale, "title")}</h1>
                    <p>${tPage(config, locale, "description")}</p>
                </div>
            `;
        }

        if (heroTarget) {
            heroTarget.innerHTML = `
                <div class="page-hero__eyebrow">${tPage(config, locale, "eyebrow")}</div>
                <h2>${tPage(config, locale, "title")}</h2>
                <p>${tPage(config, locale, "description")}</p>
            `;
        }

        if (metaTarget) {
            metaTarget.innerHTML = `
                <div class="meta-chip"><span>${tShell(locale, "versionLabel")}</span><strong>${appMeta.version}</strong></div>
                <div class="meta-chip"><span>${tShell(locale, "timeLabel")}</span><strong id="current-time"></strong></div>
                <div class="meta-chip"><span>${tShell(locale, "roleLabel")}</span><strong>${appMeta.role[locale]}</strong></div>
                <div class="meta-chip"><span>${tShell(locale, "contextLabel")}</span><strong>${appMeta.tenant[locale]} / ${appMeta.project} / ${appMeta.environment[locale]}</strong></div>
                <div class="meta-chip meta-chip--locale">
                    <span>${tShell(locale, "languageLabel")}</span>
                    <span class="locale-switch">
                        <button type="button" class="${localeZhClass}" data-locale-select="zh-CN">中文</button>
                        <button type="button" class="${localeEnClass}" data-locale-select="en-US">EN</button>
                    </span>
                </div>
            `;
        }

        document.title = `${tPage(config, locale, "title")} - ${tShell(locale, "pageSuffix")}`;
    }

    function bindLocaleSwitch() {
        document.querySelectorAll("[data-locale-select]").forEach((button) => {
            button.addEventListener("click", function () {
                const nextLocale = this.getAttribute("data-locale-select");
                if (!SUPPORTED_LOCALES.includes(nextLocale) || nextLocale === currentLocale) {
                    return;
                }
                currentLocale = nextLocale;
                saveLocale(currentLocale);
                applyLocale();
            });
        });
    }

    function markNoTranslateContainers() {
        ["sidebar-slot", "topbar-context", "topbar-meta", "page-hero-text"].forEach((id) => {
            const el = document.getElementById(id);
            if (el) {
                el.dataset.noTranslate = "true";
            }
        });
    }

    function collectTranslatables() {
        if (contentTextNodes.length > 0 || contentOptionNodes.length > 0) {
            return;
        }

        const walker = document.createTreeWalker(document.body, NodeFilter.SHOW_TEXT, {
            acceptNode(node) {
                if (!node.parentElement) {
                    return NodeFilter.FILTER_REJECT;
                }
                const text = node.textContent;
                if (!text || !text.trim()) {
                    return NodeFilter.FILTER_REJECT;
                }
                const tagName = node.parentElement.tagName;
                if (["SCRIPT", "STYLE", "NOSCRIPT"].includes(tagName)) {
                    return NodeFilter.FILTER_REJECT;
                }
                if (node.parentElement.closest("[data-no-translate='true']")) {
                    return NodeFilter.FILTER_REJECT;
                }
                return NodeFilter.FILTER_ACCEPT;
            },
        });

        while (walker.nextNode()) {
            contentTextNodes.push({
                node: walker.currentNode,
                original: walker.currentNode.textContent,
            });
        }

        document.querySelectorAll("option").forEach((option) => {
            contentOptionNodes.push({
                node: option,
                original: option.textContent,
            });
        });
    }

    function translateWithMap(original, locale) {
        if (locale === "zh-CN") {
            return original;
        }

        const map = contentTextMap[locale] || {};
        const trimmed = original.trim();
        const translated = map[trimmed];
        if (!translated) {
            return original;
        }

        const leading = original.match(/^\s*/)[0];
        const trailing = original.match(/\s*$/)[0];
        return `${leading}${translated}${trailing}`;
    }

    function applyContentLocale(locale) {
        contentTextNodes.forEach((item) => {
            item.node.textContent = translateWithMap(item.original, locale);
        });

        contentOptionNodes.forEach((item) => {
            item.node.textContent = translateWithMap(item.original, locale);
        });
    }

    function tickTime(locale) {
        const target = document.getElementById("current-time");
        if (!target) {
            return;
        }
        const now = new Date();
        const formatter = new Intl.DateTimeFormat(locale, {
            year: "numeric",
            month: "2-digit",
            day: "2-digit",
            hour: "2-digit",
            minute: "2-digit",
            second: "2-digit",
            hour12: false,
        });
        target.textContent = formatter.format(now);
    }

    function applyLocale() {
        const config = getConfigForPage(pageKey);
        renderSidebar(config.navKey, currentLocale);
        renderHeader(config, currentLocale);
        bindLocaleSwitch();
        applyContentLocale(currentLocale);
        tickTime(currentLocale);
        document.documentElement.lang = currentLocale;
    }

    document.addEventListener("DOMContentLoaded", function () {
        pageKey = document.body.dataset.page || "dashboard";
        markNoTranslateContainers();
        applyLocale();
        collectTranslatables();
        applyContentLocale(currentLocale);
        tickTime(currentLocale);
        window.setInterval(function () {
            tickTime(currentLocale);
        }, 1000);
    });
})();
