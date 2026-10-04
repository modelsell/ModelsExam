# Claude 能力检测 v3（报告 v19）

2026-09-16 设计能力题 v3，2026-09-17 精简为报告 v17，并在 v18 恢复可选 Bedrock 专项。2026-09-18 的 v19 新增 [四组 Usage Token 计数检查](model-check-usage-tokens.md)。基于 Claude 官方能力文档和主流评测方法，移除旧六道精确答案题，所有新发起的能力检测仅运行最新的 4 类、6 道原创任务。回答偏好采样已移除；Bedrock 边界诊断默认关闭，由操作者手动选择。目标是得到可复现的任务得分、实际调用性能及同条件基线差异，不把小题正确率、型号字段或工具指纹解释成真实模型身份认证。

## 能力范围与平台边界

| 官方能力 | 本系统的检测方式 | 能说明什么 |
| --- | --- | --- |
| 文本、指令遵循、多语言 | 固定多行格式约束；本轮不设多语言专项 | 指定约束下的完成质量，不是完整语言水平 |
| 推理、代码 | 推理只涉及最新数据抽取/关联任务；不设独立代码题 | 不推断通用推理或编程能力，不执行模型生成代码 |
| 结构化数据 | 两道 JSON 抽取任务 | 字段、类型、值和缺失信息处理；与原生约束解码支持分开 |
| 长上下文 | 384 条记录跨位置关联 | 此次输入规模内的检索和关联，不证明完整上下文窗口 |
| 自定义工具 | 自动选择工具及参数、工具结果回传 | 客户端工具调用与使用返回数据的能力 |
| 图像、PDF | 保留现有可选视觉/PDF 专项 | 针对测试素材的兼容性；不作为全面多模态能力排名 |
| Thinking、effort | 依模型/端点配置观察 | 空 thinking 或隐藏思考文本不构成能力扣分依据 |
| 缓存、流式、延迟 | 保留独立缓存及三次固定性能采样 | 请求计数一致性和此次运行性能，独立于能力题 |
| 托管 Web search 等平台功能 | 手动开启 Bedrock 边界诊断；始终保留实际核心请求的错误归因 | 观察平台功能支持，不将预期限制算作推理或工具能力低分 |

Claude 官方功能总表不代表每种模型、每个端点全部支持。AWS 运营的 Amazon Bedrock 与 Anthropic 运营的 Claude Platform on AWS 也不能混为一谈。当前 Claude 托管 `web_search` 不支持前者；AWS 自己的 Web Search 文档列出的 GPT/Responses 支持不能直接套用到 Claude。

官方还提供代码执行、网页抓取、Advisor、Computer/Browser use、Bash、文本编辑、Memory、Tool search、程序化工具调用、细粒度工具流、Skills、MCP、上下文压缩/编辑、批处理等功能。这些依赖平台托管服务或外部执行环境；本轮不声称已覆盖，也不因未测而扣核心能力分。需要新增专项时，应配置真实受控环境并验证执行后的结果，而不是仅检查工具名称或 HTTP 状态。

Bedrock 原生结构化输出必须区分旧 InvokeModel/Converse 和新 Mantle `/anthropic/v1/messages`。本套件只用普通提示词要求 JSON，不无条件发送 `output_config.format`。PDF 保留 base64 输入；旧 Converse 的完整视觉 PDF 有 citations 条件，不能把端点条件解释成 Claude 无 PDF 能力。

## 固定任务与程序评分

| 类别 | ID | 任务 / 评分方法 |
| --- | --- | --- |
| 指令遵循 | `instruction_format` | 三行、标题、排序值、结束标记分别判分 |
| 结构化数据 | `json_extraction` | 订单字段抽取与金额计算，检查完整 JSON、键集合、各字段类型和值 |
| 结构化数据 | `json_types` | 前导零字符串、false、数值 0、缺失值 null、排序数组，逐项检查 |
| 上下文 | `context_multihop` | 384 条记录的前、中、后部连接项目→负责人→站点→访问码，逐字段检查 |
| 工具 | `tool_selection` | `tool_choice:auto`，在库存/天气工具间选择正确函数，要求准确类型及参数 |
| 工具 | `tool_roundtrip` | 自动请求订单→保留真实 tool ID 回传固定模拟结果→检查最终 JSON 是否采用结果 |

所有新增数据均为原创合成数据，不复制公开基准原题。工具仅在本地提供固定虚构结果，不执行模型生成的命令、任意代码或外部网络操作。工具首轮没有正确调用时按 0 分处理，不伪造 tool_use、不偷偷追加修复请求；正确调用后才进行第二轮。第二轮回答按调用与最终答案断言给部分分。

JSON 校验解析整个响应，忽略对象键序和合法空白；多余说明、Markdown 包装、额外字段、错误类型、缺失值均影响对应断言。每题保留得分、是否全部正确、评分器版本、断言结果、最多 512 个字符的答案摘录。工具摘录只留名称/参数或最终答案，不保存思维文本、签名或 tool ID。

不全局设置 temperature=0，也不要求非空 thinking。文本题沿用关闭 thinking、最多 512 输出 token 的固定轻量配置；两道新增工具题参照官方工具示例，不传 thinking，使用模型默认设置、最多 1024 输出 token，并关闭并行工具调用（每题只需要一个工具）。官方明确指出 Opus 5 关闭 thinking 时偶尔会把工具调用输出为纯文本，因此工具题不强制关闭思考。这些上限是本套件配置，并非模型能力上限；实际请求被中间层改写导致 profile 不同时不判分。

## 分数、覆盖与预算

- 每题得分为通过断言数 / 断言总数 × 100，四舍五入。工具首轮必须正确调用才得分。
- 能力分为可评分题的均分。六道题按题目等权；四个类别分别有 1/2/1/2 题，类别卡片单独显示均分及覆盖。不能把少量满分解释成完整能力达标。
- 有效回答但答错、无必要工具调用计 0；接口拒绝、限流、超时、无效响应、截断、实际请求 profile 变化不计分，明确展示原因。中断后其余题记录未采集。未评分不会补满分或用 `correct` 伪造数值。
- 正常核心流程最多 15 次：基础连接 1 + 固定性能 3 + Usage Token 4 + 能力 7（6 题，其中工具闭环两次）。
- 缓存 +8、提示词 +3、视觉 +1、PDF +1，Bedrock 可选诊断最多 +7。默认只开启缓存及核心检测，最多 **23 次**；默认组合勾选 Bedrock 后最多 **30 次**；全部选项开启最多 **35 次**（不含 Bedrock 的全部计分项为28次）。核心加缓存及提示词最多26次。退役 `fingerprint` 开关即使传入 `true` 也不会执行或增加预算。
- 鉴权/限流停止、单次超时和整次 10 分钟上限保持生效。预算是上限；失败的工具首轮、提前终止可能减少实际调用数。

报告五维均分继续使用已有的型号字段、能力、提示词、缓存、协议维度；缺少数据的维度不参与均分。能力区展示总分、有效题数、四类得分及可展开的逐题判分证据。实时过程、历史查看和 HTML 导出共用同一报告组件。历史列表分数由同一规则计算，前后端共享用例检查一致性。

v17 起 JSON 导出的 `scoring.version=2`、`method=measured_dimensions`，总分与纸面报告的五维均分一致；同条件基线对照仅导出本次明确选择的快照及能力/性能结果，不生成退役指纹排名。v18 可选 Bedrock 的计划、结论和实际请求证据随原始报告导出，不纳入核心总分。旧版导出的版本化语义与历史原始证据保留。

## 移除非计分采样

字母选择偏好和中文动物选择偏好来自旧的独立行为指纹采样，未计入上述六道能力题。过去每类交错采样 10 次，以分类频数与所选基线计算 Jensen–Shannon 分布相似度，但没有参与能力分或报告总分。

这类方法有研究依据，但本实现只有两类提示词，尚未用已知真实后端的独立同模型/不同模型样本完成误判率校准。它观察的是当前请求配置和服务链路下的回答偏好；相似度不是模型身份概率，也不是能力或质量分数。

2026-09-16 只读复核本地 58 份历史报告，其中 23 份保存了这两类采样，合计 460 次请求。字母有效样本为 222/230，动物为 230/230；6 份基线仅 3 份具有两类完整数据。每轮采样耗时中位数 51.3 秒，额外 20 次请求约占当时整轮请求数的 46.5%（中位数）。在请求型号、profile 相同且两类完整的记录中，同渠道 39 对相似度范围为 47.1–97.9%，跨渠道 81 对最高为 96.9%。这些历史记录没有控制后端变更及运行环境，不能换算为识别准确率；但目前的重叠和波动不支持将相似度作为真伪判据。

因此 v17 从检测入口、计划和执行中移除回答偏好采样，不再保留实验性开关或独立观察区；v18 继续保持移除。AWS 计数头核对等冗余摘要也从主报告移除。实际核心请求的响应、错误、计数及历史原始台账仍可追溯，已保存报告和基线快照不改写。

保留的请求分别支撑型号/基础协议分、六题能力分、缓存/提示词分或同条件基线性能评分。性能三次固定采样继续提供实际延迟与输出速率；未选基线时不虚构性能达标分。视觉和 PDF 各一题，作为可选的协议计分项。缓存的写入、读取及提示词重复采样是形成最终计分所需的证据，不按单次请求是否直接有分删除。

接口拒绝、限流、超时、截断或缺少可比数据导致核心项暂不可评分时，仍保留该项与原因；不隐藏异常，也不补造满分。

## 可选 Bedrock 诊断

v18 恢复操作者明确要求保留的 Bedrock 专项，默认关闭。勾选后最多执行七次：无效消息角色、未知 Beta、Web search、Web fetch、Code execution、Advisor 工具声明，以及已知型号的采样参数边界。未知型号缺少采样规则时跳过该项；鉴权、限流和整轮超时仍会停止后续请求。

这组结果用于检查平台限制和常见错误返回，独立显示诊断结论、HTTP 状态及可展开证据，不计入能力或五维总分。符合规则的预期拒绝不算模型能力低分。实时过程、历史报告及 HTML/JSON 导出都保留本次执行结果；未开启且没有实际诊断证据时不展示空诊断卡片。

## 基线与历史兼容

当前题库为 `benchmark.version=3`，focused 报告版本为 19。v1 原六题、v2 混合十二题的历史报告继续使用当时的评分规则；不修改历史报告或基线快照，也不在新检测中重新执行退役题目。旧 API 未指定 focused 时，其余兼容逻辑保持，新请求同样停用回答偏好采样，Bedrock 专项按显式选项执行，能力测试使用最新六题。

v3 与 v2 基线仅比较六道相同新题，要求 ID、实际请求 profile、评分器版本均一致，支持部分分。v3 与仅有旧六题的 v1 基线没有共同能力题，应显示不可比较；需要重新检测并手动设为基线，不能用不同题目的分数强行对比。性能、缓存及 Token 台账仍独立比较，不要求新基线包含已经退役的偏好采样。

历史请求预算按保存的选项和版本计算：v14 原默认38/全部50，v15 原默认45/全部57；v16 曾默认开启偏好采样为39、随后关闭为19，全开51；v17 默认19/全部24；v18 默认19/全部31；v19 新增四组 usage，默认23/全部35。版本及范围调整不改变已保存报告的选项、实际请求数或分数。

工具闭环使用初始请求、固定工具结果和评分器构建任务级 profile，排除随机 tool ID；每轮真实请求仍检查完整的有效请求 profile，Token 台账匹配规则不放宽。保存基线时保留题目、分数、profile 和评分器，去掉答案摘录/逐断言数据以维持 32 KiB 上限；完整证据仍保留于来源报告，保存操作不改变原报告。

默认能力题每题测一次，固定性能仍测三次。该结果不代表多次运行稳定性；严肃渠道评估应重复同条件套件并保留每次记录，不能用多次中的最好一次代替总体表现。未来增加题目或修改题干、工具结果、评分语义必须升级相应 profile/评分器版本。

## 方法来源（2026-09-16 查阅）

- [Claude 官方能力总览](https://platform.claude.com/docs/en/build-with-claude/overview)、[官方评估设计](https://platform.claude.com/docs/en/test-and-evaluate/develop-tests)：任务成功条件可测、优先程序评分，覆盖边界；避免仅凭 API 200 判断能力。
- [IFEval 论文](https://arxiv.org/abs/2311.07911)：把指令拆成可程序验证的约束。
- [BFCL 多轮评测](https://gorilla.cs.berkeley.edu/blogs/13_bfcl_v3_multi_turn.html)、[Claude 工具调用](https://platform.claude.com/docs/en/agents-and-tools/tool-use/overview)：评估函数/参数并完成结果回传。
- [NVIDIA RULER](https://github.com/NVIDIA/RULER)：使用多位置、合成数据和关联任务；单一 needle 不足以代表上下文能力。
- [Promptfoo 断言](https://www.promptfoo.dev/docs/configuration/expected-outputs/)、[lm-evaluation-harness 任务配置](https://github.com/EleutherAI/lm-evaluation-harness/blob/main/docs/task_guide.md)：版本化题集、参数和程序判分，确保比较可复现。
- [HumanEval](https://github.com/openai/human-eval)、[SWE-bench](https://www.swebench.com/SWE-bench/)：编程基准依赖执行测试环境；当前套件不提供代码题，也不冒用这些基准名称或分数。
- [One Token Is Enough](https://arxiv.org/abs/2607.10252)：预印本探索单词回答分布指纹，使用更丰富的任务、语言及重复采样，并评估同模型与不同模型分布的判别误差；不能将论文结果直接套用为本项目两类、每类 10 次采样的识别准确率。
- [Claude Thinking](https://platform.claude.com/docs/en/build-with-claude/thinking)、[原生结构化输出](https://platform.claude.com/docs/en/build-with-claude/structured-outputs)、[新 Bedrock 端点](https://platform.claude.com/docs/en/build-with-claude/claude-in-amazon-bedrock)、[AWS 结构化输出](https://docs.aws.amazon.com/bedrock/latest/userguide/structured-output.html)：依模型和端点处理功能限制。
- [Claude Vision](https://platform.claude.com/docs/en/build-with-claude/vision)、[PDF](https://platform.claude.com/docs/en/build-with-claude/pdf-support)、[Claude Web search](https://platform.claude.com/docs/en/agents-and-tools/tool-use/web-search-tool)、[AWS Web Search](https://docs.aws.amazon.com/bedrock/latest/userguide/web-search.html)：专项素材、来源和平台支持范围。
