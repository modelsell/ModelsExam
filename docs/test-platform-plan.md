# 综合模型测试平台规划（v2：本次只加图像）

状态：规划稿（2026-10-04 修订）。**视频已移出范围**；本次迭代只在现有文本检测之外增加**图像**，并接入 OpenAI 官方内容真伪检测（Verify）。音频、向量、多模态理解等只在第 9 节列为后续，不在本次范围。本文尚未改动检测引擎。

## 1. 定位

现在的产品回答一个问题：这个 Base URL + Key + 模型，**协议是否合规、行为是否像官方、有没有被偷换或降级**。本次把「模型」从文本扩到图像生成与编辑，并用官方真伪检测给出「这张图是不是 OpenAI 的工具生成的」这一条独立证据。

沿用的边界声明不变：黑盒观测不能证明真实模型身份或供应商账单，报告里始终保留这条说明。

## 2. 现状（已读代码）

- 引擎：`pkg/claudecheck`（56 个 Go 文件，含测试）与 `pkg/openaicheck`（15 个 Go 文件，含测试），各自带一套 `Report / Check / Sample / Plan / Event` 类型，互不共享。
- 服务：`internal/server` 里 `POST /api/model_check`（Claude）与 `POST /api/model_check/openai`，各自一套并发槽（各 2）、每 IP 同时一个任务、SSE 进度流、保活。
- 存储：`model_check_runs` 单表，`transport` 字段区分 `anthropic_proxy` / `openai_api`，`report_json` 上限 4 MB；`history.go` 里用 `if transport == openai_api` 分支解码；公开列表只展示最近 100 条。
- 前端：`NewCheck` 用 Claude / OpenAI 两个 Tab，每个供应商一套表单与报告组件（`features/model-check/` 与 `features/model-check/openai/`）。
- 安全：`internal/ssrf` 在拨号时校验解析后的 IP，不走代理，不跟随重定向；Key 在报告、SSE、数据库、错误文本里统一脱敏；探针请求体只能由本地生成。
- 必须保留的做法：断言与观测分开；失败分层（无法判定 vs 失败）；请求从不重试；请求数上限事先可算；检测计划随报告落库。

结构性问题（本次只解决与图像相关的部分）：
1. 每新增一类检测就要复制 引擎类型 + 路由 + 历史分支 + 前端目录。
2. 报告以 JSON 内联保存，没有放图片的地方，而且历史公开可见。
3. 现有只有一个 Key 字段；真伪检测需要**第二个凭据**（见第 6 节）。

## 3. 设计原则

1. **平台层与套件层分离**：平台管路由、并发、存储、历史、导出；套件只管「发哪些探针、怎么判」。
2. **能力声明加探测，不硬编码**：同一模型的参数在不同资料里并不一致（第 11 节）。每个套件带可编辑的能力档案；声明支持的参数探测不符才算失败，声明不支持的只记观测。
3. **优先本地可验证的客观检查**：尺寸、格式、透明通道、主色、几何布局、遮罩外区域是否被改动，都用确定性代码判断。需要语义判断的放到可选的裁判模型，只记观测。
4. **成本先于功能**：图像按张计费，开始前给出最多张数、验真调用次数，并设硬上限。
5. **固定探针，禁止自定义 prompt**：调用方不能传 prompt，避免平台变成免费生图代理；同时每次运行在固定模板里**注入随机参数**（随机颜色值、随机形状位置），让上游无法靠「认出探针 prompt」做针对性路由，也让缓存和预存图片失效。
6. **生成物默认私有**：见第 7 节。

## 4. 总体架构（本次范围）

```
web (React)                       internal/server                     pkg/
 模态入口：文本 | 图像       ──►   /api/checks/*  (统一路由)    ──►   checkkit/   公共 Event Check Sample Plan Report 信封、脱敏（支持多个凭据）、runner 骨架
 套件描述驱动的通用表单            artifact store（图片产物）            suites/    注册表
 图像报告：画廊 + 验真卡片         store（runs 表扩展）                    claude/ openai/  现有引擎经适配器接入，不重写
                                   ssrf client（复用）                    image/    图像套件
                                   provenance client（固定 api.openai.com） media/   本地分析器：图像统计、遮罩比对、连通域计数、感知哈希
```

### 4.1 套件接口

```go
type Suite interface {
    ID() string                      // "claude.messages" "openai.chat" "image.openai"
    Modality() string                // text | image
    Describe() Descriptor            // 选项、档位、能力档案默认值、成本估算，供前端渲染表单
    Validate(Options) error
    Plan(Options) []PlanItem         // 随报告落库，与现在一致
    Budget(Options) Budget           // {MaxRequests, MaxImages, MaxVerifyCalls, MaxDuration}
    Run(ctx, Options, Transport, Observer) Report
}
```

- `checkkit` 从 `openaicheck` 抽公共类型（它的类型最干净）；`claudecheck` 经适配器接入，旧报告 JSON 不变，旧历史继续可读。
- 路由：保留 `/api/model_check` 与 `/api/model_check/openai`；新增 `GET /api/checks/suites`、`POST /api/checks`、`GET /api/checks/:id`、`GET /api/checks/:id/artifacts/:aid`。`history.go` 的 `if transport == ...` 改为按 `suite` 在注册表里找解码器。
- 图像检测沿用「请求即任务」的 SSE 模型（单张图通常几十秒），套件自带超时：单探针 180 秒、整轮 15 分钟。**不做脱离请求的任务队列**（那是视频才需要的，已随视频移出）。
- 并发槽：图像单独一个槽，容量 2；每 IP 同时一个任务的规则保留。

### 4.2 数据模型（在 `model_check_runs` 上扩展，`AutoMigrate` 即可）

| 新字段 | 用途 |
|---|---|
| `suite` | 套件 ID，取代按 `transport` 分支 |
| `modality` | 列表筛选与图标 |
| `artifact_count` | 列表上显示「含 N 张图」 |
| `provenance` | 验真摘要：`trusted_c2pa` / `synthid` / `none` / `unavailable`，用于列表徽章 |

新增 `model_check_artifacts` 表：`id, run_id, check_id, mime, bytes, width, height, sha256, storage_key, created_at, expires_at`。二进制不进数据库、不进 `report_json`。

## 5. 图像套件（`image.openai`）

接口形态：同步的 OpenAI Images（`POST /v1/images/generations`、`POST /v1/images/edits`）。队列型（fal、Replicate 的提交、查询、取结果三段式）作为第二个适配器，放在后续。

### 5.1 协议与参数（断言，随能力档案判定）

| ID | 做什么 | 判定 |
|---|---|---|
| `img_generate_basic` | 最小请求：model、prompt、size=1024x1024、quality=low | 200；`data[]` 非空；有 `b64_json` 或 `url`；能解码 |
| `img_response_shape` | 响应与错误体形状 | `usage` 若存在则字段齐全且 total=input+output；错误体含 `error.message/type/code` |
| `img_size_matrix` | 方图、竖图、横图；档案声明支持自定义尺寸时再加一个自定义尺寸 | **实际解码宽高等于请求值**；静默缩放到 1024 单独记录 |
| `img_size_invalid` | 越界尺寸（非 16 的倍数、超最大边、比例超限） | 4xx 且错误体合规；若 200 记录「接受了非法尺寸」 |
| `img_quality_levels` | low / medium / high 各一张（full 档） | 参数被接受；体积与高频能量应随档位非递减，仅观测 |
| `img_output_format` | png / jpeg / webp（含 `output_compression`） | 魔数与请求一致；jpeg 无 alpha |
| `img_background` | `background=transparent` + png | PNG 含 alpha，且有一定比例像素 alpha<255 |
| `img_n` | n=2 | 返回数量等于 n；两张 sha256 不同 |
| `img_stream_partial` | `stream=true` + `partial_images` | 事件类型与顺序合规；最终图存在；partial 数不超过请求值 |
| `img_seed_repeat` | 同 prompt、同 seed 两次 | 感知哈希距离，纯观测 |
| `img_error_shape` | 缺少必填字段 | 4xx 且错误体合规 |
| `img_moderation_shape` | 只记录上游自己返回内容过滤错误时的结构；**不主动发送违规 prompt** | 观测 |

### 5.2 客观内容（断言，全部本地判定，参数每次随机）

| ID | prompt 模板（固定骨架 + 随机参数） | 本地判定 |
|---|---|---|
| `img_solid_color` | 「纯色背景，RGB(r, g, b)，无其他内容」 | 平均色与随机的 (r, g, b) 距离在阈值内 |
| `img_split_layout` | 「左半 RGB(…)、右半 RGB(…)，中间一条竖直分界」 | 左右半区平均色分别接近指定色 |
| `img_centered_shape` | 「RGB(…) 背景正中一个 RGB(…) 正方形」 | 中心区为前景色、四角为背景色 |
| `img_count_shapes` | 「白底上恰好 k 个互不相连的黑色圆」（k 随机 2 到 5） | 阈值化后连通域数量等于 k（Go 标准库 `image` 即可） |
| `img_not_blank` | 以上所有产物 | 方差非零、非全黑 / 全白、非纯噪声 |

### 5.3 编辑（`/v1/images/edits`，multipart，素材全部本地合成）

| ID | 做什么 | 判定 |
|---|---|---|
| `img_edit_basic` | 512×512 色块图 + 简单编辑指令 | 200；输出可解码；尺寸合规 |
| `img_edit_mask` | 带遮罩的编辑 | **遮罩保护区域像素与原图差异低于阈值**（客观，能抓出忽略遮罩的实现） |
| `img_edit_multi` | 多张参考图 | 接受多个 `image[]`；输出可解码 |
| `img_edit_fidelity` | `input_fidelity=high` | 参数被接受；保真度仅观测 |

### 5.4 观测（只记录，不计分）

- 性能：各尺寸与档位的端到端耗时、首个 partial 耗时、n>1 吞吐。
- usage：同尺寸同档位多次请求的 `output_tokens` 是否稳定，只作计量一致性线索，不当计费凭证。
- 语义对齐与文字渲染：可选裁判模型（用户另填一个视觉模型端点，默认关闭），报告写明裁判模型名，结果只算观测。

### 5.5 套件分档与预算（初稿，按真实价格校准）

| 档位 | 内容 | 最多生成 | 验真调用 |
|---|---|---|---|
| basic | 基础、格式、不空白、错误体、`img_solid_color` | 约 3 张 | 开启验真时 2 次（1 张生成图 + 1 张阴性对照） |
| standard | + 尺寸矩阵、透明背景、n=2、其余客观内容 | 约 10 张 | 开启验真时 3 到 5 次 |
| full | + 质量档位、流式、seed、编辑 4 项、非法尺寸、验真的格式矩阵 | 约 22 张 | 开启验真时最多 8 次 |

内容类检查默认用最低质量与最小尺寸；高质量与大尺寸只在专门项里各跑一次。开始前显示「最多 X 张图、Y 次验真」，full 档需要二次确认。

### 5.6 能力档案

```json
{
  "family": "gpt-image-2",
  "sizes": {"fixed": [], "custom": {"multiple_of": 16, "max_edge": 3840, "max_ratio": 3}},
  "quality": ["low", "medium", "high"],
  "output_format": ["png", "jpeg", "webp"],
  "background": ["auto", "transparent", "opaque"],
  "supports": {"stream": true, "edits": true, "mask": true, "input_fidelity": true, "n_max": 10}
}
```

默认档案按模型系列内置，表单高级设置里可改。自称 gpt-image-2 的中转若拒绝自定义尺寸、或把大图静默缩回 1024，是有价值的指纹线索，但仍只当观测。

## 6. 官方真伪检测（OpenAI Verify）接入

### 6.1 它是什么、不是什么

来源：[OpenAI「验证 OpenAI 生成的内容」](https://openai.com/zh-Hans-CN/research/verify/) 与 [Content Provenance 开发者文档](https://developers.openai.com/api/docs/guides/content-provenance)。

- 检查上传的图像（或音频）里有没有**OpenAI 工具生成的信号**，覆盖 ChatGPT、OpenAI API、Codex 的产出。两种信号：
  - **C2PA Content Credentials**：签名元数据，带签发者、模型、生成时间。可被剥离。
  - **SynthID 水印**：嵌入像素的不可见水印，设计上能抵抗裁剪、滤镜、有损压缩。
- API：`POST /v1/content_provenance_checks`，Bearer 鉴权，multipart 字段 `file`，每次一个文件，≤ 50 MiB，图像支持 PNG / JPEG / WebP。
- 响应：`results[]` 里 `type=c2pa`（`outcome`、`validation_state` 为 trusted / valid / invalid / not_present、`issuer`、`model`、`generated_at`）与 `type=synthid`（`outcome`）。
- 错误：400（文件格式错误或被拦截）、404（组织无权限）、429（限流，带 `Retry-After`）。文档写明限流严格、可申请提额；零数据保留（ZDR）组织不适用。
- **它不能做的**：检测不到其他公司模型生成的内容；`not_detected` 不能证明不是 AI 生成，也不能证明不是 OpenAI 的；元数据剥离、格式转换、压缩、裁剪会削弱信号；文档要求把结果当作证据而不是定论。页面还强调它「并不能判定内容是否准确」。

### 6.2 在本平台里的用法：给中转站多一条独立证据

中转站号称提供 gpt-image-2，我们拿到它返回的**原始字节**（不重新编码、不转存），送去官方验真：

| 验真结果 | 含义 | 报告里的处理 |
|---|---|---|
| C2PA `detected` 且 `trusted`，issuer 为 OpenAI | 这张图由 OpenAI 工具生成，签名可信 | 强证据，**通过** |
| C2PA 的 `model` 与请求模型不一致 | 上游给的图来自别的 OpenAI 模型 | **失败**（断言），附两个模型名 |
| C2PA `valid` 但不 `trusted`，或 `invalid` | 有凭证但签名链不被信任或已损坏 | 观测，写明 `validation_state` |
| SynthID `detected`（C2PA 缺失） | 元数据被剥离，但像素水印还在 | 强证据，通过，并提示「元数据已被中转剥离」 |
| 两者都 `not_detected` | **不能说明是假的**：可能被重新编码，可能不是 OpenAI 模型，也可能该模型或版本不带信号 | 观测「未检出」，不扣分；是否构成问题要看下面的对照 |
| 验证服务不可用（404 无权限、429、超时） | 没测成 | 「无法判定」，不怪被测端点 |

### 6.3 检测项

| ID | 类型 | 做什么 |
|---|---|---|
| `img_prov_control` | 断言 | **阴性对照**：对本地合成的纯色 PNG 调用验真，两种信号都必须是 `not_detected`；否则说明验证链路异常或结果不可信，本次所有验真结果降级为「无法判定」。不消耗生成费用 |
| `img_prov_c2pa` | 断言 | 对 `img_generate_basic` 产出的原始图验真，读取 C2PA 结果，按 6.2 判定 |
| `img_prov_synthid` | 断言 | 同一次验真响应里的 SynthID 结果，按 6.2 判定 |
| `img_prov_model_match` | 断言 | C2PA 检出时，`model` 与请求模型（容许日期快照后缀）是否一致 |
| `img_prov_format_matrix` | 观测（full） | 分别请求 png / jpeg / webp 各一张并验真，看哪种格式保留了信号，用来解释「未检出」是格式转换造成的还是上游本来就没有 |
| `img_prov_baseline` | 观测（可选，默认关） | **官方直连对照**：用户另填一个官方 OpenAI Key，同样的固定请求直连 `api.openai.com` 生成一张图并验真，与中转结果并排。直连检出而中转两种信号都未检出，才是有意义的异常线索，报告写作「中转未保留官方信号：可能转存重编码，或未使用 OpenAI 模型」。多花 1 张生成费 |

一次验真调用同时返回 C2PA 与 SynthID，所以 `img_prov_c2pa` 与 `img_prov_synthid` 共用一次请求。

### 6.4 凭据、隐私与安全

- **两个凭据，严格隔离**：`key` 只发给被测端点；`verify_key`（官方 OpenAI Key）只发给固定主机 `api.openai.com`。两者都不会出现在对方的请求里。脱敏器要同时登记两个密钥，报告、SSE、数据库、错误文本里统一替换。
- 验真主机写死，不接受用户填写，避免被当成 SSRF 或密钥外发通道。开发和测试用环境变量 `MODEL_CHECK_VERIFY_BASE_URL` 指向 fake 服务，生产环境不开放。
- 上传到 OpenAI 的只有本次探针产出的图和本地合成的对照图，内容由固定模板加随机参数生成，不含用户数据。界面上要明示「图片将上传到 OpenAI 做验真」，并提示 ZDR 组织不适用。
- 验真需要组织有 Content Provenance 权限，404 时提示去申请，并按「无法判定」处理。
- 限流严格：验真调用串行；沿用「请求从不重试」，收到 429 就记录 `Retry-After` 并把依赖它的项标为无法判定。
- 验真不配 `verify_key` 时，整组 `img_prov_*` 标「未请求」，其余图像检测照常。
- 抗针对性路由：中转可能认出我们的探针去走真官方，其余流量走便宜模型。随机参数能压低这种风险，但**不能杜绝**，报告里保留这条说明。

## 7. 产物（图片）的存储与隐私

现在整个历史是公开的，这对生成图不可接受（内容审核、版权、滥用）。

- **默认私有**：产物只有创建任务的浏览器（`mc_owner` cookie）能下载；公开历史只显示指标与验真徽章，不显示图片。
- 公开分享：由 owner 在报告页手动开启，默认关闭。
- 存储后端：抽象成 `ArtifactStore`，首版是本地目录（容器里挂卷），预留 S3 兼容。
- 保留期：默认 7 天后清理（`expires_at`）；报告里的缩略图（≤ 256 px、JPEG）可随报告保留。
- 限制：单张 ≤ 20 MB；单次检测产物总量 ≤ 200 MB；超出则中止并记录。
- 上游若返回 `url` 而不是 `b64_json`，下载必须走 `internal/ssrf` 客户端；图片 CDN 常用 302 跳转，现在客户端不跟随重定向，需要加一个**逐跳重新校验**的受限重定向（最多 3 跳，每跳都做拨号时 IP 校验）。
- 脱敏除 Key 外，还要去掉签名 URL 的查询串。
- 不保存用户上传的图片（编辑素材全部本地合成）；不提供自定义 prompt。

## 8. 前端（本次范围）

- 导航：`开始检测`、`检查记录`；顶部或首屏增加模态切换「文本 | 图像」，替代现在 Claude / OpenAI 两个 Tab 的一级位置；文本下保留 Claude / OpenAI 两个协议。
- 套件描述驱动的通用表单：前端调 `GET /api/checks/suites`，按 `Descriptor` 渲染 Base URL、Key、模型、档位、开关和成本提示。图像表单里额外有「官方验真」折叠区：`verify_key` 输入、「官方直连对照」开关，以及上传到 OpenAI 的隐私提示。
- 现有 Claude、OpenAI 的专用报告组件保留并挂进注册表，不为统一而重写。
- 图像报告：画廊（点击放大，叠加主色、连通域等像素证据）、验真卡片（C2PA 的 `validation_state`、`issuer`、`model`、`generated_at`，SynthID 是否检出，对照结果并排）、产物下载与「公开分享」开关。
- 开始前的确认面板：最多请求数、图片张数、验真次数；full 档二次确认。
- 检查记录：增加模态筛选；图像行显示验真徽章（官方信号已检出 / 未检出 / 无法判定），缩略图仅 owner 可见。
- 国际化：所有新键补齐 7 个语言文件。`docs/claude-bedrock-check.md` 提到过翻译键完整性检查，但本仓库 `web/scripts` 里没有对应脚本，需要先补上。
- 目录：新功能放 `web/src/features/checks/image/` 与 `features/shared/media/`，旧目录先不动。

## 9. 不在本次范围

视频、音频（TTS / STT）、向量与重排、视频与音频理解、渠道对比页、跨套件基线、定时回归、队列型图像适配器、脱离请求的任务队列。其中音频将来可以直接复用本文的验真通道：Verify API 同样支持音频的 SynthID（≤ 60 秒）。

## 10. 路线图

| 阶段 | 内容 | 验收 |
|---|---|---|
| P0（已完成） | 前端开发端口改为 5178；「检查记录」文案在源码里已有，需重新构建才出现在嵌入版里 | `bun run dev` 在 5178 打开；导航显示「检查记录」 |
| P1 平台骨架 | 抽 `checkkit`；套件注册表与 `Descriptor`；统一路由；`runs` 表加字段；多凭据脱敏器；前端「模态」切换与通用表单；Claude、OpenAI 经适配器接入，行为不变；补翻译键检查脚本 | 现有 Go 与前端测试全部通过；旧历史可打开；旧路由可用 |
| P2 图像核心 | `media/` 本地分析器；图像套件 basic → full（5.1 到 5.3）；产物存储与私有下载；能力档案；画廊与确认面板 | fake 图像上游的一致性测试通过；真实 gpt-image 端点跑通 basic 与 standard |
| P3 官方验真 | provenance 客户端；`img_prov_*` 全部检测项；验真卡片与徽章；官方直连对照 | fake Verify 服务覆盖 6.2 每一行；用真实 `verify_key` 对官方直连图跑通检出，对本地合成图验证阴性对照 |

建议先做 P1，因为它决定后面的成本；P2、P3 之间的依赖只有「需要先有图可验」，P3 的 fake 测试可以和 P2 并行写。

## 11. 测试策略

- fake 上游：沿用 `openaicheck/fake_test.go` 的做法，新增 fake Images 服务（可配置返回错尺寸、忽略遮罩、静默降级、返回 url 与跳转）和 fake Verify 服务（可配置 trusted / valid / invalid / SynthID / 429 / 404 / model 不一致）。
- 本地分析器用黄金图（程序生成的固定图）做单元测试，阈值写成常量并有边界用例。
- 报告语义测试：保证「未检出」永远不会让断言失败、不会降低得分；验证服务不可用永远落在「无法判定」。
- 脱敏测试：两个 Key 分别出现在响应错误体、URL、头部里，报告与 SSE 中都不能残留。
- 前端：单元测试覆盖验真徽章的状态映射、成本确认面板、翻译键完整性。

## 12. 资料与可信度

| 事项 | 来源 | 可信度与处理 |
|---|---|---|
| Verify 的功能、两种信号、局限 | [OpenAI 官方页面](https://openai.com/zh-Hans-CN/research/verify/) | 官方，已读 |
| 验真 API 的路径、字段、限制、错误码 | [官方开发者文档](https://developers.openai.com/api/docs/guides/content-provenance) | 官方；**由抓取工具摘要得到**，实现前要对照原文逐字段核对，尤其 `validation_state` 枚举与限流 |
| 文档里「DALL-E 模型会嵌入信号」，但没有明确 gpt-image-2、编辑接口是否都带信号 | 同上 | **未确认**。所以设计里加了官方直连对照，用实测回答，而不是靠假设 |
| OpenAI 宣布在 ChatGPT、Codex、API 生成的图像中加入 C2PA 与 SynthID | [PetaPixel 报道](https://petapixel.com/2026/05/20/openai-gets-serious-about-detecting-fake-images/)（URL 日期为 2026-05-20） | 二手报道，与官方页面一致 |
| gpt-image-2 参数 | [Azure Foundry 文档](https://learn.microsoft.com/en-us/azure/foundry/openai/how-to/dall-e)、[fal 文档](https://fal.ai/models/openai/gpt-image-2/api)、[apidog 指南](https://apidog.com/blog/gpt-image-2-api/) | **三者互相不一致**：Azure 写 `quality` 为 low / medium / high、尺寸任意（16 的倍数、最长边 ≤ 3840、比例 ≤ 3:1）；apidog 写 `quality` 为 standard / high、有 `thinking` 参数、最大 2000×2000；fal 写 low / medium / high / auto。OpenAI 官方图像文档这次没读到。默认值待读官方文档后定，所以采用能力档案加探测 |
| 图像编辑参数（`image[]`、`mask`、`input_fidelity`、`stream`、`partial_images`、`output_format`、`output_compression`） | 同上 Azure 文档 | 仅一个来源，实现前对照官方 |
| 客观图像评测思路（GenEval 的对象、计数、颜色、位置） | [GenEval](https://neurips.cc/virtual/2023/poster/73566) | 只借用「可程序化判定」的思路，自己用本地几何判定实现 |

## 13. 需要你决定的事

1. **验真凭据从哪来**：由用户在表单里自填官方 OpenAI Key，还是部署方在服务端配置一个共用的 Key？后者会让所有访客消耗同一个组织的限流额度，我建议默认由用户自填。
2. **图像和验真要不要加访问控制**：图像检测与官方验真都花真钱或占限流额度，而现在没有账号。不加的话只能靠每 IP 限流和每日上限。
3. **生成图存放**：默认私有加 7 天清理，是否可以？本地挂卷够用，还是需要 S3 兼容存储？
4. **首批要测哪些模型和中转**：目前按 OpenAI Images 协议设计；如果你主要测 gpt-image-2 的中转，这套就够用。
5. **裁判模型**：文字渲染与语义对齐是否需要（默认关闭、用户自填视觉模型端点）？
6. **成本上限**：每次检测与每日的最多张数、验真次数各给多少？


## 14. 实现状态（已落地，与上文设计的差异）

已实现：`pkg/media`、`pkg/provenance`、`pkg/imagecheck`、`internal/server/image.go`（`POST /api/model_check/image`，历史 transport 为 `image_api`）、前端「图像」Tab、历史抽屉与导出。与设计不同之处：

- 只有一个 `verify_key`（官方 OpenAI Key），仅发往 Verify 主机；被测端点的 Key 仅发往被测端点。开启「官方直连对照」时，`verify_key` 也用于向 api.openai.com 发起一次生成。开启对照会隐含开启验真阶段。
- 没有做通用的套件注册表 / `checkkit` 重构，也没有 `GET /api/checks/suites`；图像功能独立成包，前端按 `openai/` 的结构镜像为 `image/`。
- 蒙版编辑是观测项，不计分；验真类检查（kind=`provenance`）不计入得分，只在报告里单独展示。
- 生成图不落库：缩略图（最多 24 张、192px）只出现在实时 SSE 流里，入库前剥离；历史里只留尺寸、格式、哈希。
- 能力档案（Profile）按模型名给默认值，服务端可校验；UI 暂未提供手动编辑。
- 预算：质量档位探测的请求数 = 档位数；前端按默认档案估算，精确上限以报告 `limits` 为准。
- 已验证：Go 纯包（media / provenance / imagecheck）在容器里单测通过；前端 `tsc --noEmit` 与全部 123 个单测通过。gin/gorm 胶水代码（`internal/server`）只对桩验证过，需在本机跑 `go build ./... && make test`。
- i18n：新增键已补 en、zh；其余 5 个语言暂回退英文。

## 15. 前端改版：中立检测机构的形象与报告徽章

- 首页：衬线标题 + 定位声明；右侧「如何读懂徽章」图例；协议选择改为三张单选卡（Claude / OpenAI 兼容 / 图像生成）；之后依次是检查记录、覆盖范围表、方法与独立性声明、页脚免责声明。
- 徽章（`components/report-badge.tsx`，规则在 `lib/badge.ts`，有单测）：只由运行状态与计分（仅断言）决定。100 符合；80–99 基本符合；低于 80 建议核对；停止/取消/中断 → 未完成（不给结论，无论分数）；运行中；未评分。观测项与验真结果不改变徽章。
- 列表：每行显示协议标签（Claude / OpenAI / 图像）与徽章；报告抽屉顶部复用大号徽章并注明“仅对应本次检测”。
- 覆盖范围表只列已实现的协议；其他原生协议（如 Gemini 原生）明确标注“暂未覆盖”。后续新增协议只需在表中加一行，并在平台选择里加一张卡。
