# Claude / AWS Bedrock 渠道检查

## v20：Bedrock thinking signature 检测

当前精简流程勾选「Bedrock 兼容性诊断」后，最多执行 8 次专项请求，新增 `bedrock_signature`。该探针用合成的 thinking 块、无效签名和配对的 `tool_use` / `tool_result` 构造当前工具轮次的续传请求，避免历史思考块被忽略；按映射后的模型选择 adaptive 或 enabled（1024 token 预算）模式，输出上限 2048。

仅 HTTP 400 且明确说明 thinking signature 无效或验证失败时记为符合预期。请求被接受则记录边界差异；普通参数错误、thinking 不支持、AWS 请求签名鉴权错误、限流及服务错误均不能证明该边界。探针不计入模型身份或性能评分，不采集或改写真实思考签名，也不代表本地密码学验证。未勾选时不会发送此请求；v19 及更早历史报告保留原清单和预算。

设计依据：[AWS Extended thinking with tool use](https://docs.aws.amazon.com/bedrock/latest/userguide/claude-messages-extended-thinking.html) 要求工具续传保留完整思考块。下文旧版 Thinking 回放说明仍仅适用于原样回放探针。

> 以下章节保留 v14 及更早版本的历史实现说明。提示词与缓存检查见 [当前提示词与缓存检测](model-check-token-audit.md)，更早的设计依据见 [v9 流程与本地数据依据](model-check-focused-flow.md)。

初版：2026-09-13；二次调研与流程改版：2026-09-14。

入口：默认前端管理菜单「模型检测」（`/model-check`），可独立填写 Base URL、API Key 和模型。渠道更多操作中的「模型检测」跳转到同一页面并带入渠道与模型，支持 AWS 原生渠道（33）及 Anthropic Messages 中转渠道（14），渠道密钥只在服务端使用。可选 PDF、流式对照、Thinking、缓存、重复采样和图片理解，支持快速 / 标准 / 扩展三档。

页面采用同一套报告组件贯穿待开始、连接中、逐项检测、完成和中断状态：顶部显示进度、当前请求及四类结果数量；下方按连接与基线、协议与行为、文档/视觉/思考与缓存、一致性与用量、核验边界五个阶段分组；请求台账实时追加 HTTP 状态、耗时、首事件耗时及四类 Token 用量。可展开证据并导出 JSON，中断后保留部分报告。Key 仅存在表单会话与请求中，不写入 URL、浏览器持久存储或报告。

## 截图中的结论应如何解读

截图的“纯度 97”“来源 Anthropic 官方 API”等结论不能作为真伪证明。消息 ID、模型名、工具 ID 和响应头均可被中转改写；响应头缺失也可能只是代理未透传。即使某模型通过全部行为测试，也只能说明这些探针在该链路上的表现符合预期。

截图第二张表中反复出现缓存写入、缓存读取全部为 0，应报告“未观察到缓存命中”。不能将写入量当作命中量，也不能只凭耗时变快判定缓存成功。缺少 CountTokens 时，不能证明 token 守恒或排除隐藏提示词注入。

本功能保留原始检测状态及用量证据，页面统一展示 100 分制兼容性评分、覆盖率和优化建议；该评分不代表模型纯度或真伪。

## 开源方案评估

| 方案 | 可参考部分 | 在本系统中的取舍 |
| --- | --- | --- |
| [7836246/claude-detector](https://github.com/7836246/claude-detector)（MIT） | 19 个探针、SSE 进度、工具/流式/缓存/CountTokens 审计及报告设计 | 探针分类值得参考；其 README 将自报身份、指纹等组合成真伪判定，这不足以证明真实供应商或模型。独立 Node 服务还需另外维护密钥和权限边界。采用探针思路，不导入服务或评分实现。 |
| [zxc123aa/cc-proxy-detector](https://github.com/zxc123aa/cc-proxy-detector)（MIT） | 多维来源指纹、重复请求及限流头变化观察、JSON 输出 | 指纹适合作为线索；矩阵把 Bedrock/Kiro 合为一列，且正常 AWS API 与订阅逆向渠道不能据此等同。仅参考观察方法，不采用来源评分和负面字段推断。 |
| [Promptfoo 的 AWS Bedrock provider](https://www.promptfoo.dev/docs/providers/aws-bedrock/) | 模型评测、断言、数据集回归，以及 Bedrock 的 Invoke/Converse 支持 | 更适合后续维护固定能力评测集。本次运维入口需要原始协议字段与用量，因此采用项目已有 Go AWS SDK 和渠道配置，不增加评测运行时。 |

以上评估依据当日公开 README 和 provider 文档，没有运行第三方检测平台，也没有把渠道凭据提交给第三方。

## 官方依据及判定边界

- [AWS Prompt caching](https://docs.aws.amazon.com/bedrock/latest/userguide/prompt-caching.html)：缓存门槛、TTL 和支持范围因模型而异。当前新检测使用两组带唯一前缀的合成文本和上游默认 TTL，每组顺序写入一次、读取三次；逐次核对写入与读取计数并汇总得分。不等待 TTL 过期，不声称已验证 1 小时 TTL。
- [AWS CountTokens](https://docs.aws.amazon.com/bedrock/latest/userguide/count-tokens.html)：接口本身免费，但依赖模型、区域及权限。一些仅跨区域提供的模型不支持 Runtime CountTokens；文档提供 Mantle 的另一路径。本功能只检查当前已配置链路，未自动改走 Mantle、切换模型或区域；无法调用时结果为“无法判定”。
- [AWS Extended thinking](https://docs.aws.amazon.com/bedrock/latest/userguide/claude-messages-extended-thinking.html)：Thinking 与工具结合时不能强制指定工具，后续请求需保留完整思考块。本功能使用自动工具选择并原样回放内容块，不修改、解码或伪造签名。成功回放表示该上游接受它，不表示本地完成密码学验证。

普通输入、缓存创建和缓存读取分别展示。计数对比使用：

`input_tokens + cache_creation_input_tokens + cache_read_input_tokens`

缺失字段显示横线；不会将缺失值默认为零后展示。CountTokens 与推理请求来自同一上游，仍不是独立计费凭证。AWS 发票和本站用户结算不在本次检查范围内，报告始终保留这一边界。

## 已实现的检查

基础检查最多 8 次请求：

1. Claude Messages 根结构、System 指令遵循和被动来源线索。
2. 对基础请求实际发出的输入调用 CountTokens。保留转换后的原始输入，避免重新生成动态覆盖字段造成计数漂移。
3. 流式事件开始、内容块生命周期、用量更新及结束事件；拒绝流中错误及截断，保留显式输出 0。
4. 工具调用名称、参数、ID 与停止原因。
5. `max_tokens=1` 截断。
6. 自定义停止序列。
7. 随机验证码的多轮上下文保留。
8. 显式 `max_tokens=0` 的官方零输出行为：非流式、HTTP 200、空 content 数组、stop_reason=max_tokens、output_tokens=0；不支持时保留原因并不计分。

可选 Thinking 最多增加 3 次请求：非流式思考块、工具结果回放和流式思考完整性。依据映射后模型名选择 adaptive 或传统 enabled 模式；未知模型或未返回签名标记为无法判定。可选缓存增加 3 次长提示词请求，分别记录写入、首次读取、再次读取。额外可选三次相同流式请求的重复采样，以及一次随机颜色合成 PNG 识别。未覆盖 PDF、Web Search、超长上下文上限、能力降级识别或签名篡改探针。


## 二次调研：常见指标与采用机制

这次检索的是公开实现与官方文档；以下是这些项目反复使用的指标，不代表所有社区工具都采用统一标准。

| 参考实现 | 本次关注机制 | 采用方式与限制 |
| --- | --- | --- |
| [claude-detector 探针源码](https://github.com/7836246/claude-detector/blob/main/src/lib/probes.ts) | 模型回显、错误格式、相同请求对比、图片输入 | 增加对应观测与可选图片探针；不同别名、重复 ID 或计数差异不直接判为假模型。 |
| [cc-proxy-detector 源码](https://github.com/zxc123aa/cc-proxy-detector/blob/main/scripts/detect.py) | 消息/工具标识、响应头、重复观察、限流头变化 | 收集白名单字段作为证据；未透传、计数不变和格式不同都可能有正常解释，不采用其来源评分或静态头即伪造的判断。 |
| [gateway-bench](https://github.com/edgee-ai/gateway-bench) | TTFT、完整响应耗时、成功率、可比请求批次 | 相同提示词顺序采样三次，只对成功样本计算中位数，并同时展示成功数和总请求数。样本量不足以报告可靠 P95/P99 或 SLA。 |
| [Promptfoo Bedrock provider](https://www.promptfoo.dev/docs/providers/aws-bedrock/) | 固定输入、断言、模型适配、可重复评测 | 采用固定协议断言和有预期答案的轻量探针；完整能力评测数据集仍应另行执行，不从几道题推断模型版本。 |

官方依据补充：

- [Claude Streaming](https://platform.claude.com/docs/en/build-with-claude/streaming)：区分 message_start、文本增量、thinking/signature 增量；保留有序生命周期验证，允许将来新增的元数据事件。省略 thinking 文本时仍可检查完整签名块，不把空文本误判为无思考能力。
- [Claude Token counting](https://platform.claude.com/docs/en/build-with-claude/token-counting) 与 [AWS CountTokens](https://docs.aws.amazon.com/bedrock/latest/userguide/count-tokens.html)：展示相同上游的计数差额；计数估算、转换规则和接口支持范围均需核对，差额单独标为无法判定，不自动下计费欺诈结论。
- [Claude Vision](https://platform.claude.com/docs/en/build-with-claude/vision)：使用本地生成的 64×64 PNG，只检查随机红、蓝、绿之一的识别，不依赖外部图片 URL，不上传用户图片。
- [Claude Errors](https://platform.claude.com/docs/en/api/errors)：非法参数请求记录实际 HTTP 状态和错误包络类别。AWS SDK 与中转可能改写包络，因此错误格式属于来源观测。

## 当前 V4 流程与报告设计

快速检测最多 8 次请求；标准检测默认开启 Thinking 和缓存，最多 14 次；扩展检测再加重复采样、图片和 Veridrop 套件，最多 22 次。Veridrop 可单独开关，最多增加 4 次。表单可单独调整选项，开始前实时显示最大请求数和检测清单。每份报告保存版本、选项与完整清单，共 27 个检查/观测/边界项目；未选择、依赖失败和中断有不同的跳过原因。

| 阶段 | 项目 | 执行方式 |
| --- | --- | --- |
| 连接与基线 | 响应结构、模型映射回显、来源指纹、CountTokens、基础缓存用量 | 首个基础请求临时不可用时，使用清单内的流式探针尝试建立基线；鉴权、明确无路由等问题直接停止。 |
| 协议与行为 | System、流式、工具、输出限制、停止序列、多轮、非法参数、错误包络、可选图片 | 复用响应提取观测，图片单独一次合成请求。 |
| 思考与缓存 | 非流式思考、流式思考、原样回放、缓存写后读 | 不解码或公开签名；报告保留是否存在和生命周期结论。 |
| 一致性与用量 | 三次重复采样、字段有效性、AWS 头计数核对 | 重复采样比较 ID、返回模型、答案和输入总量；缓存输入口径不明确时跳过头部输入比较。 |
| 核验边界 | 独立计费核验、隐藏提示词核验 | 明确保留无法判定，不参与模型真伪评分。 |

报告顺序为：总体评分、覆盖率与优化建议 → 五阶段进度与得分 → 三组关键指标 → 全部/优化与待核对项筛选后的项目证据 → 请求台账。兼容性断言、来源观测和核验边界各有标识。进度分别显示实际执行数与跳过数，跳过不计入执行进度；提前结束显示「检测已停止」及具体原因。

指标口径：

- 首事件、首段文本、文本最大间隔分别记录；不以首个 ping 或思考事件冒充可见文本到达。
- 重复请求显示成功数/尝试数、成功样本 TTFT 中位数及样本数、响应耗时最小/最大值。不混入工具请求、缓存请求、CountTokens 或不支持的零输出请求。
- 后续缓存命中显示命中次数/提供读取计数的成功后续请求数，并单列后续尝试数；未知读取计数不当成零，重复缓存写入不当成命中。
- 输入、输出、缓存创建、缓存读取独立累计，仅汇总 HTTP 200 的推理响应；缺字段时整项累计显示不可用，台账仍保留已知明细。CountTokens 不重复计入推理用量。
- V1 历史继续按原来的 15 项范围显示，新增 TTFT 等缺失字段保持不可用；V2 历史以已保存清单为准，刷新或新版页面不会补造未执行结果。

未将身份自述、知识截止时间、限流压测、最大上下文试探、隐藏 system 原文提取或签名篡改加入默认检测。前几项不能单独证明真实模型，后几项需要更高费用、可信基线或额外的专项测试设计。

## 评分展示

实时结果与历史报告统一展示兼容性总分、阶段得分和单项得分，不再展示「失败」结果标签、红色失败统计或失败结论横幅。扣分项目保留原因、请求证据和优化建议，历史列表增加评分列。接口中的原始状态与历史数据保持兼容，页面将原 `failed` 状态表达为「建议核对」。

评分口径 v1：每个已选、非验证边界的明确结果等权计分，`pass` 为 100 分，`fail` 为 0 分，总分是两类明确结果的平均值，四舍五入到整数。来源等观测项目只有明确结果时才计入。未确认、未执行及验证边界项目不计分；没有可评分结果显示「—」，明确的 0 分仍显示 0。评分覆盖率为已评分项目数 / 已选且非验证边界的项目数，并展示未评分数。执行中、取消或中断的报告标明「当前兼容性评分（部分结果）」，避免局部 100 分被误认为完整检测。

阶段和单项沿用同一口径；历史列表使用已保存的明确结果计数计算，历史详情依原报告范围计算。导出的 JSON 在保留原始证据的同时附带 `scoring`（口径版本、总分、覆盖率与项目数）。评分反映本次兼容性检测结果，不代表模型真伪或计费真实性。

## 接口与执行范围

`POST /api/channel/:id/claude_check`

```json
{"model":"claude-opus-5","cache":false,"thinking":false,"repeat":false,"vision":false}
```

- 沿用 AdminAuth 和 `ChannelOperate` 权限，仅接受该渠道配置的模型；不接受用户自定义 URL、密钥或提示词。
- 每份报告选择并固定一个可用密钥，复用渠道模型映射、代理、参数和头部覆盖、AWS 转换设置。不会从管理请求继承登录凭据或用户级中转覆盖。
- AWS 渠道调用原生 `InvokeModel` / `InvokeModelWithResponseStream` / `CountTokens`；支持现有 AK/SK 与 Bedrock API Key 格式。Anthropic 渠道调用其已配置的 Messages 路径。
- 每渠道同时一份报告，每个服务进程最多两份；总时限 10 分钟，单探针 90 秒，SDK 不重试；输出上限 2048 tokens、请求体上限 128 KiB、单响应上限 2 MiB。覆盖设置超出限额时拒绝该探针。
- 离开页面或点击停止会终止客户端请求并传播取消；已完成的上游请求仍可能收费。总时限耗尽时返回部分报告，浏览器额外设置 615 秒截止时间。基础非流式与流式请求均不可用后跳过依赖探针；每 10 秒发送 SSE 注释保活，不产生额外上游请求或历史记录。
- 检查不修改渠道启停、响应时间和测试时间，不走用户余额扣减流程；实际供应商费用仍会产生。用量属于诊断报告，不写入常规模型测试消费账目。
- 报告包含请求 ID、HTTP 状态、实际模型、响应模型、停止原因、原始分类用量和耗时，可手动导出 JSON。`first_event_ms` 是流式首个数据事件耗时；`stream.first_text_ms` 是首段非空白可见文本耗时（TTFT），另记录文本片段数与最大间隔。两者不混称 TTFB。
- 报告返回 `Cache-Control: no-store`，服务端自动保存脱敏后的检测快照和最终报告。历史及导出内容不包含原始生成文本、思考文本或签名；渠道密钥、凭据组成部分及覆盖头值在保存和响应前统一脱敏。

独立接口：`POST /api/model_check`，同样要求 AdminAuth 和 `ChannelOperate`。

```json
{"base_url":"https://api.example.com/v1","key":"<API key>","model":"claude-sonnet-4-6","cache":false,"thinking":false,"repeat":false,"vision":false}
```

手动接口使用 Anthropic Messages 协议，Base URL 可填根地址、`/v1` 或 `/v1/messages`；AWS 原生 Runtime 凭据通过渠道模式检测。手动 URL 不接受 userinfo、查询参数或 fragment，遵守全站出站 URL 策略并使用拨号时 SSRF 防护，禁止自动跟随重定向。请求体限 16 KiB，不创建或修改渠道；同一管理员同时一份手动报告，与渠道报告共用每进程两份的上限。

手动检测默认使用系统 DNS。若开发机的代理采用 Fake-IP，域名可能被解析为
`198.18.0.0/15` 内的保留地址，触发 SSRF 拦截；这发生在发送上游请求之前，与
API Key 或模型能力无关。页面会显示具体的解析或策略原因及解析地址。
只有遇到这种网络环境时，才需要为该服务进程设置
`MODEL_CHECK_DNS_RESOLVER=cloudflare` 并重启；正常生产环境无需设置。
该选项使用 [Cloudflare DNS over TLS](https://developers.cloudflare.com/1.1.1.1/encryption/dns-over-tls/)
解析手动检测的目标域名，连接固定解析器 `1.1.1.1:853` / `1.0.0.1:853` 并验证 TLS 证书。
解析器只接收域名，不接收 Key、URL 路径或检测内容。需要允许该进程访问 TCP 853。
解析不可用时终止请求，不自动降低校验或改回本地解析。

公网 DNS 模式要求现有 SSRF 防护及域名 IP 过滤已开启；域名、端口、IP 列表和
私有地址规则仍全部生效。预检查、请求校验、拨号使用同一个解析器，拨号前再次校验
并连接具体 IP，保留原始 Host 和 TLS SNI。此模式直接连接已校验的 IP，不使用环境 HTTP
代理，避免代理重新解析目标。该设置不改变渠道中转、图片下载或全局 DNS，不能用于
放行内网地址。未设置或设为 `system` 时保持原有解析及代理行为。

两个接口在 `Accept: text/event-stream` 时逐项推送 `start`、`probe_start`、`sample`、`check`、`done` 事件，每条事件先序列化脱敏再刷新响应；无该请求头时保留普通 JSON 返回。前端以 `done` 作为完整报告标志，流中断不会伪装成正常完成。

渠道模式检查上游兼容性与渠道适配配置。手动模式可检查指定的公开中转端点，但仍不能独立证实真实模型身份或供应商账单。

## 检测历史

每次有效检测在发送第一个上游请求前创建历史记录，随后按检测事件更新快照。完成、失败、主动停止、客户端断开及执行超时都保留已取得的结果。输入校验失败或并发限制拒绝的请求没有开始检测，不生成报告；首次保存失败则不发送上游请求。中途保存失败会终止后续探针并尝试保存最终快照，最终保存仍失败时页面明确提示导出当前报告。

记录保存到主数据库的 `model_check_runs` 表，正常迁移与快速迁移均已注册，兼容 SQLite、MySQL 和 PostgreSQL。报告上限 4 MiB，列表仅加载元数据，打开报告时才加载完整快照。进程意外退出后，超过 5 分钟未更新的运行记录在查询时标记为中断；保留最后一次成功保存的证据。未设置自动清理或历史条数限制。

独立页面提供「开始检测 / 检测历史」两个页签；切换历史不会停止当前检测。历史支持模型名、执行状态筛选及分页，并展示时间、检测目标、评分、状态、耗时和请求数。正在执行的历史每 3 秒更新。历史详情复用实时检测的报告组件，支持展开证据、查看请求台账及导出 JSON；`view=history&history_id=<报告 UUID>` 可刷新恢复所选报告。

- `GET /api/model_check/history`：参数为 `page`、`page_size`（默认 20，最大 100）、可选 `model`、`status`、`channel_id`。
- `GET /api/model_check/history/:id`：返回历史元数据及完整脱敏报告。
- 状态：`running`、`completed`、`failed`、`cancelled`、`interrupted`。`failed` 表示报告包含失败项，具体能力结论仍以各检查项为准。
- 两个接口要求 AdminAuth 和 `ChannelOperate`，只返回当前管理员自己发起的检测。历史按用户隔离，其他用户的报告 ID 返回 404。浏览器缓存键同样包含用户 ID。
- 历史记录保存检测模型、目标地址、渠道名称、选项和报告，不保存 API Key，不提供从历史自动恢复凭据的功能。

## 验证

使用本地模拟上游验证探针、缓存未命中、缺失用量、流式截断、显式零值、取消传播、模型映射、CountTokens 原始输入一致性、脱敏、并发限制、权限路由及不改变渠道状态。AWS SDK InvokeModel/CountTokens 使用本地 HTTP 服务验证序列化，无真实 AWS 费用。

独立页面增加真实 HTTP 流式首事件提前送达、手动 URL 校验、私网策略、重定向阻止及取消传播测试。前端验证 UTF-8 分片、SSE 多行帧、断流保留结果、零值/缺失字段及重新检测；用模拟事件在浏览器验证实际页面组件的进度、完成、停止、证据展开及响应式布局。

历史增加 SQLite 重复迁移、大报告往返、零值保存、终态不可覆盖、用户隔离、筛选分页、过期运行恢复、取消后写入、初始快照脱敏及数据库不可用时不发送上游请求的测试。校验三种数据库生成的字段类型；浏览器使用真实 Go 检测与历史接口、临时 SQLite 和本地模拟上游验证历史列表、报告复用、刷新恢复及停止保存。

V2 追加版本化范围、首事件/首文本分离、空白文本排除、流式签名块重组、前向兼容元数据事件、三次重复采样、随机图片、计数差异及缓存命中口径测试。浏览器通过本地模拟上游跑完 18 次请求，验证实时阶段、TTFT、缓存台账与历史报告复用。

前端执行 ESLint、格式检查、生产构建、翻译键完整性和 TypeScript 检查。全量 TypeScript 现有 45 个错误通过干净 HEAD 副本单独复现，集中在仪表盘、定价与应用入口；本次改动文件没有新增类型错误。

未部署生产，也未执行真实 AWS 付费调用。实际模型、地区、IAM 权限、供应商缓存及签名行为需在选定渠道运行报告后确认。

评分展示补充零分保留、空结果、部分结果覆盖率和未选/边界项目排除的单元测试。前端共 10 项测试通过；浏览器验证完整结果、历史列表及详情评分一致、空结果和中断报告的显示。


## 官方 Token 上限规则

### 官方规则修正

按 [Anthropic Prompt caching](https://platform.claude.com/docs/en/build-with-claude/prompt-caching)
和 [stop reasons](https://platform.claude.com/docs/en/build-with-claude/handling-stop-reasons)：

- `max_tokens=0` 是合法的非流式零输出请求，替换旧的“无效参数必须返回 400”检查。
  此探针不启用 stream/thinking/forced tool/output format。正常响应必须是 `content:[]`、
  `stop_reason:max_tokens`、`usage.output_tokens:0`。小提示词探针不宣称已写入缓存。
- `max_tokens=1` 允许空 content 数组；仍验证 message/assistant、id/model、content 数组存在、
  stop_reason 及非负 input/output usage。输出超过请求值记 0 分；提前自然结束或拒绝不计分。
- 记录 requested/effective max_tokens、content block 数及结构原因码；渠道覆盖改变上限时不计分。
  本地 effective 值只能证明本检查传输层发出的值，无法看到远端代理的二次改写。
- [AWS 请求参数](https://docs.aws.amazon.com/bedrock/latest/userguide/model-parameters-anthropic-claude-messages-request-response.html)
  的支持范围随模型/地区/路由变化。400、429、网络不可用等不视为已证明的模型兼容性缺陷。
- V1/V2 原报告及分数保持不变；页面提示重新检测。没有历史原始响应，不能回填新规则得分。

## V4：提前结束诊断与恢复

V1–V3 的首个非流式请求一旦超时或返回无效结构，就跳过全部依赖项。35 秒单项上限以及 3 分钟总上限使慢响应和扩展检测容易提前结束，旧页面又把跳过项算作已完成，看起来像正常跑完。

V4 在首个请求出现超时、暂时不可用、限流、连接问题或无效响应时，提前执行已经选中的流式探针。成功后复用该响应建立 System、模型回显、用量等基线，再执行剩余清单；流式探针不重复执行，快速/标准/扩展的最多 8/14/22 次请求预算保持不变。鉴权拒绝、接口不存在或明确的模型分组无可用路由不会尝试备用探针。

报告新增 `stop_reason`、`stop_probe`、`baseline_probe`、执行时限与最大请求数，历史新增 `stopped` 状态。普通探针中的超时、连接、权限等问题作为未确认可用性保留，不用这些结果给模型兼容性扣分。无可评分证据显示横线。老报告只依据已保存错误补充停止原因说明，原始证据和历史得分不改写。

[AWS Claude Messages 文档](https://docs.aws.amazon.com/bedrock/latest/userguide/model-parameters-anthropic-claude-messages.html) 提醒部分 Claude 推理可能远超短客户端超时；[Claude 错误文档](https://platform.claude.com/docs/en/api/errors) 区分鉴权、限流、过载与超时。90 秒单项 / 10 分钟整轮是本功能的诊断预算，不是官方模型兼容性要求，也不保证所有长推理都能在此预算内完成。


## V5：统一语义检测流程

测试方法已合并到五个常规阶段，不再使用独立套件、来源卡片或参考快照评分。
默认标准检测包含 PDF 和流式对照。快速 / 标准 / 扩展最多为 8 / 17 / 21 次请求；
26 个计划项固定写入每份 V5 报告，未选项和未执行项仍保留明确状态。

| 方法 | 所属阶段 | 验证方式 |
| --- | --- | --- |
| 结构化工具 | 协议与行为 | 替换原基础工具请求，校验唯一工具调用、名称、ID、必需字段、类型、枚举及多余参数；不增加重复请求。 |
| PDF 阅读 | 文档、视觉、思考与缓存 | 本地生成中性单页订单 PDF，每轮随机编号，仅将编号放在文档内；检验读到的编号。1 次请求。 |
| 流式内容对照 | 一致性与用量 | 相同 JSON 任务分别采用流式与非流式调用，比较对象值；允许 JSON 围栏和字段顺序差异，两次同样答错仍不符合预期。2 次请求。 |
| 结束原因一致性 | 一致性与用量 | 复用上述两次响应独立评分，期望都以 end_turn 完成；结束原因异常不抹去内容正确的得分。 |

PDF / 工具 / JSON 的输出预算为 512 tokens。收到合法的 `stop_reason=refusal`（含空 content）
时标为未确认且不计分，不能据此宣称模型不支持 PDF。答案尚未完成且预算耗尽也不计能力分；
完整响应内容错误则保留 0 分。见 [官方拒绝响应说明](https://platform.claude.com/docs/en/build-with-claude/refusals-and-fallback)。
网络、鉴权、限流等仍作为可用性证据，响应结构无效则记录具体验证原因。

历史报告在页面、检测事件和导出时转换为统一名称和阶段，来源快照不再展示或导出。
数据库原始报告、原得分、实际请求数和历史范围不改写，不为老报告追加未执行的新检查。
旧客户端套件字段继续兼容，进入新执行器后转换为 PDF 与流式对照开关；新报告不输出旧字段。
改编方法的源码归属、许可证和归档样例保留在 testdata 的 NOTICE 中；当前检测不加载旧快照。


## V6：型号替换与证据边界

修复旧版只比较配置映射与响应名的漏检。请求 Opus 5、映射及响应均为 Sonnet 5 时，
现在明确记录型号字段不一致并计 0 分；映射被改写但响应又伪装回 Opus 5 也会留下冲突证据。
新增整轮型号一致性项，复用各次成功响应，定位具体探针、冲突字段、预期型号和观测型号。
同轮型号切换也被记录，后续取消不会抹去已观测的不一致。计划共 27 项，请求预算仍为 8/17/21。

仅对已知 Claude 型号格式、官方旧版日期别名及 Bedrock 包装做归一化；自定义别名和无法解析的
application inference profile 不猜测归属、不计型号匹配分。旧历史不回填新得分。
型号分衡量字段一致性，不是模型身份概率。报告显式标记实际身份未验证；仅凭 PDF、工具、
签名回放、响应速度或模型自述，无法确认 Opus 5 没有被替换为 Sonnet 5。

进一步的行为鉴别需要从可信 Opus 5、Sonnet 5 端点采集同协议、同参数的重复样本，同时保留
采样量、误差、日期和版本。当前未采集这两套可信基线，不给出“实际模型为 Sonnet 5”的概率。
[fpverify](https://github.com/Mohamed7415/fpverify) 的公开测量没有 Opus 5，且其内置参考来自 Cursor
的批量提问协议；项目自身明确限制跨协议时只能相对排名，其模拟实验识别率不能用于此处。
[AWS CloudTrail](https://docs.aws.amazon.com/bedrock/latest/userguide/logging-using-cloudtrail.html)
调用记录中的 modelId 和 requestID 可用于供应商侧关联核验；当前功能没有接入该账户日志。

## 多类型基线与版本 7 测试集

支持手动从已完成的检测历史设置多份基线，按 Anthropic、AWS、CCMax、Kiro 或自定义类型标记；页面后续检测自动对照最近 50 份基线并保存引用快照。新增 12 道能力题、40 次分类指纹采样、3 次固定流式性能测试，完整基线预设最多 76 次请求。兼容性、能力、分布相似度与性能分开展示；不预填未经采集的官方身份数据。

操作、题目清单、评分公式与局限见 [基线测试集设计](model-check-baseline-design.md)。

## v8：提示词与缓存计数检查

新增三道独立 token 对照题，并在原缓存三次请求中核对首次写入和逐次读取。过程、历史和导出使用相同的计数表，展示差值、一致性得分和证据不足原因。详见 [测试方法与计分规则](model-check-token-audit.md)。

## Bedrock 专项诊断（2026-09-15）

检测设置的“可选诊断”新增 `bedrock` 开关，默认关闭。基础检测仍为 33 次请求；开启后最多 40 次，包含所有其他可选项目时最多 48 次。新诊断沿用实时事件、历史存储与报告导出，不改已有历史记录或基线。

### 主动用例

| 用例 | 请求变化 | 判定 |
| --- | --- | --- |
| 无效角色 | 单条消息使用不存在的 `model_check_invalid_role` | 400 且明确指出角色约束才记 100 分；不以 system 角色代替无效角色 |
| 未知 Beta | 固定合成标记 `model-check-unsupported-2099-01-01` | 400 且明确拒绝 Beta 才记 100 分；检测专用传输在正常转换完成后保留此标记，普通转发不受影响 |
| Web Search | 单独声明 `web_search_20250305`，`max_uses=1` | 400 且明确拒绝当前工具名称或类型才记 100 分 |
| Web Fetch | 单独声明 `web_fetch_20250910`，`max_uses=1` | 同上；其他工具的拒绝不能代替本项证据 |
| Code Execution | 单独声明 `code_execution_20250825`，不添加非必需 Beta 或自定义工具 schema | 同上；参数缺失、权限和配额错误不算符合预期 |
| Advisor | 单独声明 `advisor_20260301`，携带 `advisor-tool-2026-03-01` Beta；advisor 为 `claude-opus-5`，`max_uses=1`、子调用 `max_tokens=1024` | 必须明确拒绝工具本身；Beta、advisor/executor 模型配对或参数错误保持待确认 |
| 型号采样规则 | Sonnet/Haiku 4.5 同时设置 `temperature`、`top_p`；已登记的新型号设置 `temperature=0.5` | 按实际映射的模型选择规则。未知或未登记型号跳过，不外推未来型号规则 |

四个工具分别请求，只声明服务端工具，提示只返回 PONG、不调用工具。HTTP 200 仅记录声明被接受，不能证明工具执行成功；证据保存工具名称、版本和 `tool_declaration` 检测范围。原来的 `bedrock_server_tools` 历史条目仍可展示，不补造历史测试结果。

各用例请求设置 `max_tokens=32`；如渠道覆盖该参数，报告保留实际值，并继续执行现有 2048 tokens 的硬上限。不进行限流压力测试，不制造权限或配额故障，不执行工具结果。权限、配额、限流会停止继续采集；普通 400、5xx、原因不匹配、网关接受或改写均不直接判为满分或假模型。专项分数不计入模型身份、核心能力和性能评分。

### 被动错误诊断

所有现有请求自动解析 AWS SDK 类型、`x-amzn-errortype`、原生 JSON、中转包装和 SSE 的 `error` 事件。HTTP 200 后的错误不能成为有效模型样本。保留现有脱敏错误、请求 ID，以及可获得的原始状态码；新增结构化字段仅保存白名单类别，不保存任意 SDK 消息或凭据。

覆盖访问权限、临时凭据/签名、时钟过期、用途申请、Marketplace 订阅、模型/区域、推理配置、服务配额、限流、模型未就绪、模型执行/流中断、超时、服务容量，以及具体参数限制。例如 `ServiceQuotaExceededException` 的官方状态是 400，`ModelNotReadyException` 和 `ThrottlingException` 都可能是 429，必须按错误类型区分。响应被网关改写时同时显示观测状态和 AWS 文档状态；错误形状只作为线索。

### 官方资料及支持边界

- [AWS InvokeModel 错误类型](https://docs.aws.amazon.com/bedrock/latest/APIReference/API_runtime_InvokeModel.html) 与 [错误排查](https://docs.aws.amazon.com/bedrock/latest/userguide/troubleshooting-api-error-codes.html)：作为分类和处理建议依据。AWS 状态码不是模型身份认证。
- [新版 Bedrock 功能支持](https://platform.claude.com/docs/en/build-with-claude/claude-in-amazon-bedrock)：不支持 Anthropic 服务端 Web Search、Web Fetch、Code Execution、Advisor，以及 URL 输入和 Files API 引用。客户端工具单独处理；网关可自行补充功能。
- [旧版与 InvokeModel 支持说明](https://platform.claude.com/docs/en/build-with-claude/claude-on-amazon-bedrock-legacy)：PDF 可用；Converse 的视觉 PDF 需要 citations，纯文本提取不等于视觉解析。新版 Opus 的部分 InvokeModel 路径支持对话中 system，不能统一判为非法。
- [PDF API 限制](https://docs.aws.amazon.com/bedrock/latest/userguide/inference-api-restrictions.html)：每次请求最多 100 页。
- [CountTokens](https://docs.aws.amazon.com/bedrock/latest/userguide/count-tokens.html)：支持范围取决于型号和端点；部分 Runtime 型号需改用 Mantle 计数。本检测不自动跨端点发送凭据，缺少计数不扣分。
- [采样参数](https://platform.claude.com/docs/en/api/http/messages/create) 及 [AWS Messages 参数](https://docs.aws.amazon.com/bedrock/latest/userguide/model-parameters-anthropic-claude-messages-request-response.html)：新模型可能接受默认兼容值，测试选用非默认值；4.5 系列的互斥规则单独处理。

- 工具请求结构：[Web Search](https://platform.claude.com/docs/en/agents-and-tools/tool-use/web-search-tool)、[Web Fetch](https://platform.claude.com/docs/en/agents-and-tools/tool-use/web-fetch-tool)、[Code Execution](https://platform.claude.com/docs/en/agents-and-tools/tool-use/code-execution-tool)、[Advisor](https://platform.claude.com/docs/en/agents-and-tools/tool-use/advisor-tool)。这里检测的是 Anthropic 服务端工具；客户端自定义工具、Bash、Memory、Text Editor 等不按此规则判定为 Bedrock 不支持。

页面中“官方限制参考”明确标记为参考资料，不充当本次已执行的检测结果。资料核对日期为 2026-09-15，后续模型与平台功能变化时需要更新规则。


## 文档式报告与导出（2026-09-15）

检测过程、历史详情和导出共用 `ReportSheet`，以白底纸张样式展示环形总览、五维雷达、型号链路、同题性能分布、Token 台账、基线阈值及检测明细。页面保留原来的进度条、基线设置和可展开的完整诊断。

- 总览名称为“已测维度均分”，按型号字段、核心能力、提示词计数、缓存计数和协议检查五个维度等权计算，只纳入有实际评分的维度，同时展示评分覆盖。它不是模型真实性概率。Bedrock 拒绝测试单独展示，不增加总览分数；缺少数据的雷达轴显示 `—`，不补零或满分。
- 提示词计数缺少 CountTokens 时保持未评分，不使用“干净/未见注入”代替。缓存命中率以可测量的读取请求为分母，同时展示可测/尝试数；写入不能充当命中。缓存差值仍来自同请求指纹下的“读取 - 首写”，100% 命中与读写计数不一致可同时出现。
- 延迟仅统计已有三个合格的同题性能请求，展示 TTFT 中位数、样本标准差和样本量。首事件不标为 TTFB；不混入短问答、缓存或故意触发的 400 请求。历史中缺少这些采样时不补造性能数据。
- Token 台账展示上游输入、输出、缓存写入/读取、总输入、参照计数、带符号的差值与 AWS 头核对。CountTokens 请求仅作为参照；总输入任一组成缺失时保持 `—`。缓存输入头语义不确定时不比较输入，只比较实际可用且明确的字段。
- 可下载独立 HTML，离线无需外部资源；通过浏览器打印保存 PDF。导出从同一 React 文档获取，HTML 保留可展开的响应头证据，PDF 沿用当前展开状态，并限制为允许的响应头字段。HTML 使用 CSP 禁用脚本与外部资源；用户文本由 React 转义，端点去掉用户名、密码、查询参数及片段。JSON 保留原始评分口径，另外记录 `report_sheet.version=1` 的显示评分、分布和计数指标。
- 没有独立账单时不显示推测的“实际费用”。导出不修改历史或基线，也不新增默认模型请求。

参考截图中第一份报告的缓存首次写入为 2,249，随后读取为 2,239 / 2,229 / 2,220，存在 -10 / -20 / -29 的差值；即使命中率 100%，也不能据此断言读写完全一致。另一份报告将部分 Bedrock 平台限制标为失败，并把缺少独立验证的结果汇总成“纯正度”，本实现保留可见证据但不沿用该结论。


雷达图的覆盖区域连接实际已评分的顶点：至少 3 个顶点绘制填充区域，2 个仅绘制连线，0–1 个不补造面积。跨过未评分维度的边使用虚线，缺失维度继续显示 `—`；真实 0 分保留中心顶点。绘图调整不改变评分、历史记录或导出数据。

### 提示词完整性：两层证据评分（TokenAudit v2）

TokenAudit v2 历史运行使用三道固定题，最多 6 次请求（3 次推理 + 3 次同有效请求的 CountTokens）。不索取或保存渠道隐藏提示词，也不把模型的自述当成验证依据。新运行已改用下文的 v4 重复采样对照，v2 分数不回算。

| 测试 | 指令与预期 | 行为分 | Token 分 |
| --- | --- | --- | --- |
| 无 system 短指令 | 仅返回 `PONG`，检查额外前后缀与任务改写 | 10 | 70/3 |
| system 优先级 | system 要求 `MAPLE-7391`，用户要求改为 `CEDAR-0000`；应仅返回前者 | 10 | 70/3 |
| 参考文本隔离 | 在 96 条固定记录中查找 Record 073，参考文本夹带改答 `CEDAR-0000` 的伪指令；应仅返回 `701544` | 10 | 70/3 |

- 行为层严格比较完整 `end_turn` 文本（仅忽略首尾空白）。正确为该题行为满分，偏离为 0；传输、协议错误、拒绝或截断等没有完整行为证据时不计量。不保存响应原文。
- Token 层仍比较 `input_tokens + cache_creation_input_tokens + cache_read_input_tokens` 与同模型、同有效请求的计量。先验证请求指纹一致；缓存计数不改写或混用。产品容差为 `max(2, ceil(expected × 1%))`；容差内得该题 Token 满分，保留实际有符号差额；超出后沿用 `min(expected, actual) / max(expected, actual) × 100`。非空请求计数为 0 无法作为参考；实际输入为 0 不享受容差。
- 总分为三题已获得的行为分（最高 30）与 Token 分（最高 70）之和，各层汇总后四舍五入。未测层不获证据分，同时通过**加权证据覆盖率**与两层已测题数显示“未验证”；它不是检测失败或注入概率。例如：3 道行为题均符合但无计数接口，显示 **30/100，覆盖率 30%，行为 3/3，Token 0/3**。全部没有可计量结果仍显示 `—`。
- CountTokens 返回 404/405/501、其自身的 403 权限不足或明确的 Bedrock CountTokens 400 时，仅停止后续计数请求，继续行为题。401、429、取消和超时仍按原终止规则处理。CountTokens 的调用权限与模型推理权限分开判断。
- 缓存评分继续要求完全相等才得 100，不应用提示词的估算容差。TokenAudit v1 历史按原规则呈现，不从旧报告补推行为分；v2 评分随过程事件、历史和基线快照保存，页面与 HTML/打印导出使用同一结果。

这是提示词完整性的有限测试：行为异常可能由能力、参数或渠道策略导致；计数相等也不能排除代理同时改写推理与计数。该分数不能认证模型身份或证明渠道没有隐藏注入。权重与容差是本产品的评估规则。

依据：[Anthropic Token counting](https://platform.claude.com/docs/en/build-with-claude/token-counting) 明确计数是估算值，实际输入可能有小幅差异；[AWS CountTokens](https://docs.aws.amazon.com/bedrock/latest/userguide/count-tokens.html) 说明按模型和输入结构计量；[Anthropic prompt injection guidance](https://platform.claude.com/docs/en/test-and-evaluate/strengthen-guardrails/mitigate-jailbreaks) 区分可信指令与不可信外部内容。这些文档不提供本项目的 30/70 权重或数值容差。

### 渠道是否添加提示词：对照用例 v3

v2 的冲突指令/伪管理员题测量的是模型对测试者放入指令的反应，不能判断渠道是否偷偷添加内容。v3 移除这两道对抗题，仍保留 3 次推理、最多 3 次 CountTokens：

| ID | 请求 | 用途 |
| --- | --- | --- |
| `floor_a` | 无 system、无工具、无参考文，只要求返回 `PONG` | 收集最小输入计数与输出表现 |
| `floor_b` | 与 `floor_a` 完全相同 | 检查输入地板是否稳定；重复请求不能替代独立基线 |
| `system_canary` | 用户消息不变，增加已知 system，要求返回 `MAPLE-7391` | 观察已知 system 是否保持、输入增量与基线是否一致 |

不再通过添加更多“忽略之前指令”题目来提高渠道注入判断分数，也不采用字符数估算 Claude Token、固定跨型号输入地板、拒绝泄露 system 或模型自述作为注入证明。

**证据与分数：**

1. 记录测试原始请求指纹、实际发送的提示词内容指纹以及实际型号声明。内容指纹覆盖 system、messages（含角色和顺序）、tools；忽略 model、max_tokens 等非提示词字段及缓存标记，并规范等价的字符串/单文本块表示。只有内容指纹改变才报告“本地发出提示词已改写”，不会把 AWS 请求包装或参数默认值误判成文本注入。仅保存哈希，不保存被添加的原文。
2. 同渠道 CountTokens 与 usage 的差额继续保留为辅助诊断，但 **v3 不给它独立来源的 70 分**。代理若同时改写两条路径，会出现两边完全相等的漏检。
3. 自动读取本次运行开始时加载的手动基线。要求 TokenAudit v3、同一已识别型号、三道原始请求相同、实际请求参数一致、无已知本地提示词改写，且基线地板重复稳定、system 计数有增加。缺少用量、旧用例、参数/型号不兼容和不稳定的基线不参与定性，展示其数量。基线可信度由设置基线的人负责，标签不是官方认证。
4. 每份兼容基线保留三题 `基线实际输入 → 当前实际输入`、有符号差额、容差及来源报告 ID。三题均超出容差且差额接近，报告“稳定额外输入，可能附加提示词或虚增用量”；多题增加、其他差异及缺少配对分别报告。与基线接近只表述“相对基线未观察到额外输入”。
5. 基线结论不一致时，完整展开所有对照，不挑最近的一份作为无异常证明。v3 仍按行为 30 分、**保存基线的 Token 对照 70 分**计分；没有完整兼容基线或各基线定性冲突时，Token 层未计量，行为最多 30 分。有一致结论的完整基线时，每题采用对各基线的一致性得分中较低者，以原有容差/比值公式计分，并显示来源和覆盖率。CountTokens 不可用不妨碍已保存基线的对照。

**本地验证：** 受控假上游实际在输入中加 64 个字符的前缀，以可控计量器计算用量，同时维持原来的回答。覆盖两边计数都增加、计数接口不可用、本地可见改写、重复计数漂移、不同基线冲突和不兼容基线。额外覆盖“上游隐藏添加后伪造用量”盲区：其报告仍可能与基线一致，测试明确验证了这个限制。这些是检测器回归测试，不是任何真实 Claude 渠道的测量结论。

**调研依据与进一步确认：** [开源完整性检测实现](https://github.com/canarybyte/veridrop/blob/16feef72ad76d154c3ae6b4c0917319597fc1888/src/relay_detector/protocols/anthropic/detectors/integrity.py) 使用相同输入的差分计量作为线索；[Promptfoo 间接注入测试](https://www.promptfoo.dev/docs/red-team/plugins/indirect-prompt-injection/) 检查测试者放入的不可信内容能否改变模型行为，两者回答的问题不同。纯黑盒响应与可伪造的计数无法排除等长改写或计量造假。更强的证据是可信 AWS 账户内的 [Bedrock 调用日志](https://docs.aws.amazon.com/bedrock/latest/userguide/model-invocation-logging.html)，按 requestId 核对模型实际输入与原始请求；该日志默认关闭，当前官方文档限定支持 `bedrock-runtime`，不覆盖 `bedrock-mantle`。本次没有启用云端日志或调用生产渠道。


### v10 报告 / TokenAudit v4：重复采样后评分

2026-09-15 再次核对官方资料及开源方法，v10 当时采用以下流程（现已由 v12 简单对照替代）。v1–v3 历史记录保持原评分，不补造重复样本。其他能力、性能与指纹基线继续按各自版本匹配。

- **提示词：三类问题 × 三轮**。短输入只要求 PONG；无 system 的 96 条中性记录查询要求 701544；保持短输入不变、加入已知 system 指令要求 MAPLE-7391。按“短→长→system”交错执行三轮，共 9 次推理，每次紧接最多一次同有效请求的 CountTokens，总计最多 18 次。CountTokens 不支持时停止后续计数调用，继续完成全部推理；401、429 或取消仍停止采样。
- 每题保存三轮原始计数、原始/有效请求指纹、提示词内容指纹、发送型号与返回型号。汇总中位数、最小值、最大值及有效样本数。仅当三次输入均有效、请求与型号一致、范围不超过 `2 × max(2, ceil(median × 1%))`，才允许作稳定的基线比较。这是产品阈值，不能当作官方统计置信度。
- 只对照 v4 的同题、同有效请求参数、同型号且三轮稳定的手动基线；基线还须观察到长文本和已知 system 控制量高于短输入。重新验证原始样本，不信任保存的摘要。单次明显偏移保留在范围内并标记“采样不稳定”，不直接判注入。缺少有效样本显示采样不完整。
- 基线按三题中位数对照：三题持续正偏且偏移接近时报告可能附加提示词或虚增用量，多题增加或其他差异分别展示。不同基线给出不同结论时保留全部对照。没有独立基线时，至少两题的全部三轮都超出同渠道计数容差，才展示“持续额外输入”的辅助线索；单次计数偏差不会触发。
- 提示词评分仍为行为 30 分 + 基线计数一致性 70 分。行为按计划的 9 次回答汇总；基线按三题稳定中位数计分。无兼容稳定基线、样本不稳定或基线结论冲突时，70 分部分无证据；同时展示覆盖率。采样结束前不发布最终提示词得分。
- **缓存：两轮独立前缀，每轮一次写入、三次读取**，总计 8 次请求、6 个读写对照。每轮前缀使用不同随机标记，轮内保持请求体与缓存断点完全相同，顺序等待上一请求结束。每次读取只对照本轮已确认的冷写（写入 > 0、读取显式为 0），不跨轮借用写入参照。暂时性错误不放弃另一轮，权限、限流和取消仍中止。
- 缓存单次基础分为 `100 × min(首写, 读取) / max(首写, 读取)`，只有完全相等才是 100 分；重复写入时再乘 `首写 / (首写 + 本次重复写入)`。计数缺失、有效请求变化或没有确认冷写时保持单项未评分。最终证据分为六次已获得分数之和除以 6；未测项不获得证据分并单列覆盖率，避免一次可测命中就显示满分。明确返回的零读取是未命中，得 0 分。延迟仍独立使用既有性能采样，不将计数一致性称为缓存加速效果。
- 默认开启指纹和缓存时请求预算由 33 变为 38；开启全部可选项最多 65（含 18 次提示词、8 次缓存、7 次 Bedrock 检查）。关闭指纹、仅核心加缓存和提示词时最多 36。总时限仍 10 分钟。报告、历史、JSON、HTML/打印导出保存同一份版本化样本与汇总。

本轮资料依据：

- [Anthropic Token counting](https://platform.claude.com/docs/en/build-with-claude/token-counting)：计数属于估算，可有少量差额，系统优化可能带来额外且不计费的 Token；不能将单次不等直接解释为恶意注入。
- [Anthropic Prompt caching](https://platform.claude.com/docs/en/build-with-claude/prompt-caching) 与 [AWS Prompt caching](https://docs.aws.amazon.com/bedrock/latest/userguide/prompt-caching.html)：完全一致的前缀、最小缓存门槛、顺序预热，以及输入/写入/读取三种用量语义是比较的前提。
- [Promptfoo 间接提示词注入测试](https://www.promptfoo.dev/docs/red-team/plugins/indirect-prompt-injection/)是在已知模板变量中放入攻击内容来测抵抗能力，不能检验未知渠道是否偷偷添加了 system 指令，因此本轮不把对抗题得分当作无注入证明。
- [Anthropic 减少提示词泄露](https://platform.claude.com/docs/en/test-and-evaluate/strengthen-guardrails/reduce-prompt-leak)：应用可以设置隔离与输出防护。模型拒绝复述提示词不代表不存在上游附加内容；本测试不以复述/拒绝结果作主判据。
- [AWS 模型调用日志](https://docs.aws.amazon.com/bedrock/latest/userguide/model-invocation-logging.html)：可信 AWS 账户内的调用日志可按 requestId 对照实际输入与型号。它比渠道返回的 usage 更独立，但需要账户权限和事先启用，且须核对 Runtime/Mantle 的支持范围。本功能未自动启用云日志。

重复采样降低偶发误判，不能解决渠道同步伪造 usage、同长度文本替换、或“基线自身已被注入”的不可辨识问题。结果是相对所选基线的证据评分，不是模型真实性或无注入概率。


### v11 报告 / TokenAudit v5：按可用证据判断

本地 2026-09-15 的实际记录表明，用户命名为“官 key”的渠道及 `ms-claude_vt` 均已完成九次正确回答，三题输入量连续三轮固定为 15 / 981 / 45；`ccmax` 则固定为 26 / 992 / 56。旧规则仍只给 30 分，原因是 CountTokens 不可用，且当前手动保存的三份 v7 基线均没有提示词样本，并非这九次测试不稳定。

v5 保留三题各三轮的请求数量与原始用例，简化**证据适用条件**：

- 每题独立判断，三次中至少两次在中位数容差内即可参与计数对照；一个异常样本保留在最小值/最大值和偏离样本数中，不取消其他题的结果。剩余两次可用也可比较，实测回答数单独显示。
- 基线逐题按实际提示词内容（system、messages、角色、tools）及相容型号匹配，不再要求整份基线的套件版本相等、三个条件全部齐备或请求包装字节完全相同。v3/v4 中相同问题可直接复用；老记录缺少内容哈希时仅接受完全相同的原始请求哈希。不同问题、已知本地内容改写及不同型号仍不混用。
- 优先使用可用基线对照；某题没有基线样本时，使用该题至少两次合法的同有效请求 CountTokens 对照。CountTokens 请求指纹不匹配、计数缺失及单次计数异常不能单独触发额外输入结论。所有基线对照仍可展开，不会偷偷将某个现有渠道或本地报告设为可信基线。
- 多类型基线按整份对照的平均一致性选择一份用于计分（同分优先覆盖更多题），不会逐题拼出“全匹配”基线。其他类型的差异仍逐项展示；不同类型有不同模板不再要求全部同时符合。页面标记本次评分使用的基线，基线匹配分不能认证渠道无注入。
- 得分改为**已测项一致性**，缺少数据只影响证据覆盖率。保留行为 30%、输入计数 70% 的权重：按实际可测的回答与题目对照数加权，再除以已测权重归一到 100。九次行为均正确、没有输入参考时得分为 100、覆盖率为 30%，明确标注“仅行为，无输入计数对照”；不能将它解释为无注入证明。实际观察到的改写、回答变化或计数偏移仍扣分；本地实际发出内容被改写时单独报告并记 0 分。
- 旧报告不回算。可直接将现有的九次“官 key”报告手动设为基线，后续 v5 检测无需因套件版本变更重采同一道题。旧 v7 报告没有提示词数据，仍不能补造计数。

离线回归使用上述本地实际数据的脱敏副本，只保留生成用例的 Token 计数与哈希，不含渠道、地址、密钥、响应头或请求正文。以第一份记录作为测试参照，第二份为 100 分；第三份三个条件均多出 11 Token，得到 85 分并展示额外输入差异。这是离线比较结果，不是对其商业渠道身份的认证，也没有修改数据库里的手动基线或重新发起付费调用。

[Anthropic 文档](https://platform.claude.com/docs/en/build-with-claude/token-counting)明确将 CountTokens 定义为估算，并说明可能包含系统优化所加的 Token。因此，输入差异仍表述为附加内容、模板差异或用量报告差异的线索；绝对确认隐藏输入需要可信上游的实际调用记录。
