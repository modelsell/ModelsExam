# Usage Token 计数检查（报告 v19）

按不同输入长度采样，核对响应中的总输入用量、增长规律和所选基线差异。该检查发现的是计数异常或差异；单靠待测端点上报的 usage，无法证明真实计费量。

## 四组固定样本

| 样本 | 输入文本 | Probe |
| --- | --- | --- |
| 英文短文本 | 30 词 | `usage_tokens_en_30` |
| 英文中等文本 | 300 词 | `usage_tokens_en_300` |
| 英文长文本 | 1500 词 | `usage_tokens_en_1500` |
| 中文文本 | 1000 个汉字 | `usage_tokens_zh_1000` |

英文使用同一原创词序列的不同长度前缀，中文使用固定合成文本。四次请求使用相同的简短回复要求、`max_tokens: 32` 和 `thinking: disabled`，不附加显式缓存控制。文本及参数跨运行保持不变，便于另一次可信直连检测保存为基线。

只判定输入计数，不以是否输出 PONG 作为通过条件，不增加 CountTokens 请求。

每行展示：

`总输入 = input_tokens + cache_creation_input_tokens + cache_read_input_tokens`

普通输入必需，明确的负数、非整数、非有限值或超过 JavaScript 安全整数范围的计数不评分。未返回的可选缓存项按 0 参与此次合计；原始响应中的缺失状态仍保留。HTTP/响应结构无效、总输入非正或尚未采集时，显示原因，不能当作正常零值。

## 增长合理性

四组都有有效计数后才产生专项分数，覆盖率始终单独展示。英文三组总输入应严格递增；不满足时增长得分为 0。

设三组英文总输入为 `A30`、`A300`、`A1500`：

```
短中增长斜率 = (A300 - A30) / 270
中长增长斜率 = (A1500 - A300) / 1200
斜率一致度 r = min(两段斜率) / max(两段斜率)
```

差分减少相同 system、消息封装和固定前缀的影响。`r >= 0.8` 给 100 分，否则为 `round(100 × r / 0.8)`。20% 是本系统的经验容差，不是 Anthropic 或 AWS 承诺的误差范围。报告同时展示 `A1500 / A30` 的长短倍数，不把该倍数硬编码成真实性阈值。

中文只检查正的有效计数并参与基线对照。不同语言和模型的 tokenization 不同，不能要求“一词一个 Token”或按固定中英比例判定真假。

## 固定所选基线

只读取开始前选定且已冻结的 `baseline_id` / `baseline_type`，型号必须兼容。每行需要同一 probe、相同且非空的请求参数指纹，以及有效响应和计数；同条件重复样本取总输入中位数。不临时换基线，不把不同测试长度或旧题混为参考。

展示本次值、基线值、差值和差值百分比：

`差值百分比 = 100 × (本次总输入 - 基线总输入) / 基线总输入`

四行都能配对时计算基线一致性分数。每行差异在 `max(4 Token, 基线 × 5%)` 内给 100 分；超过容差时为 `round(100 × min(本次, 基线) / max(本次, 基线))`，四行取均值并四舍五入。4 Token / 5% 同样是产品经验容差。专项最终分取增长分和基线分的较低值。

没有完整可比基线时，仅展示增长合理性分和缺少参照的原因。旧基线通常没有这四组新样本，需要对可信渠道重新运行一次新检测并手动保存为基线；旧记录不补写、不虚构官方计数。

例如截图中的 `54 / 324 / 1524 / 2109` 可得到增长合理性 100 分、长短倍数约 28.22。若相同条件的基线是 `94 / 724 / 3524 / 4034`，会同时显示明显偏低的差异，而不是因为增长正常就认定计数真实。

## 流程和报告

v19 focused 流程为：连接 → 三次性能采样 → 四组 usage → 六道能力题 → 可选缓存、提示词、视觉、PDF 和 Bedrock。核心预算由 11 增为 15，默认含缓存为 23，默认再开启 Bedrock 为 30，全部选项为 35。鉴权、限流、单次超时和整轮 10 分钟上限保持生效。

usage 专项分作为协议维度的一个子项计入五维均分，历史列表与完整报告使用相同规则。正在采集、历史查看和 HTML 导出使用同一张表；JSON 同时保留原始 samples 和专项评估摘要。四组的原始计数仍列在 Token 台账中。

v18 及更早报告不新增未执行的题目或修改当时预算、分数；非 focused 的旧客户端流程保持原有行为。原有提示词注入与缓存读写测试保持各自的判定逻辑。

## 解释边界

固定倍数少报、多报可能仍呈线性增长，只有独立可信的同条件参照才能暴露这种差异。基线也可能受平台模板、tokenizer 或计量口径影响，差异不能直接定性为欺诈。请求内容或 usage 被同一个代理协调改写时，黑盒测试仍可被欺骗。

此专项检查输入计数，不能验证输出 Token 或实际扣费。实际计费真实性需独立上游调用记录或账单佐证。

## 官方参考

- [Anthropic 缓存与总输入口径](https://platform.claude.com/docs/en/build-with-claude/prompt-caching#tracking-cache-performance)
- [Anthropic Token counting](https://platform.claude.com/docs/en/build-with-claude/token-counting)
- [Anthropic 可选缓存计数类型](https://github.com/anthropics/anthropic-sdk-python/blob/main/src/anthropic/types/usage.py)
- [Amazon Bedrock TokenUsage 字段](https://docs.aws.amazon.com/bedrock/latest/APIReference/API_runtime_TokenUsage.html)
- [Amazon Bedrock Claude Messages 请求与响应](https://docs.aws.amazon.com/bedrock/latest/userguide/model-parameters-anthropic-claude-messages.html)
- [Amazon Bedrock 调用日志](https://docs.aws.amazon.com/bedrock/latest/userguide/model-invocation-logging.html)
