package seo

// Chinese-first public content. Titles lead with the words people search
// ("模型评测", "API 中转站", "真伪检测") and keep the English name for the
// project. Every claim here describes what the exam does; none promises a
// result about any site.

// QA is one frequently asked question.
type QA struct{ Q, A string }

var nav = []Link{
	{"/", "开始检测"},
	{"/models", "模型榜单"},
	{"/baselines", "官方基线"},
	{"/records", "检测记录"},
	{"/get-badge", "获取徽章"},
	{"/method", "方法与独立性"},
	{"/integrate", "中转站接入"},
}

// FAQ is shown on the home page and published as FAQPage structured data.
var FAQ = []QA{
	{"ModelsExam 是什么？", "ModelsExam 是开源（AGPL-3.0）的独立检测服务：向你指定的 Claude、OpenAI 兼容或图像模型 API 地址发送一组固定请求，检查模型表现是否与声称的一致、协议是否符合官方、用量是否虚标，并生成公开的检测报告。"},
	{"怎么判断中转站的模型是不是真的？", "单靠让模型自报家门并不可靠。ModelsExam 综合身份问答、行为指纹和能力探测，再对照官方基线，并检查字段、流式、工具调用、错误格式和 Token 用量是否符合官方协议。便宜模型冒充高价模型，通常会在这些检查里露出破绽。"},
	{"检测需要填写 API Key 吗？密钥会泄露吗？", "需要。密钥只在本次检测中使用，不会写入报告，也不会出现在公开记录里。建议使用专门为检测创建、额度有限的密钥，检测后可以停用。"},
	{"得分 100 就代表可以放心购买吗？", "不能这样理解。分数只统计计分断言，描述的是某个地址在某一时刻的一次表现，是证据而不是保证，更不是排名或背书。徽章 30 天后过期，请以检测日期为准。"},
	{"榜单是怎么排序的？", "每个模型的榜单取每个站点最近一次已完成且有得分的检测，按得分从高到低排序，得分相同时较新的检测排在前面；超过 30 天的结果会标为已过期。榜单是检测结果的列表，不是对站点好坏的评价。"},
	{"我是站长，怎么展示检测徽章？", "在“获取徽章”页输入你的域名，复制脚本、图片或 Markdown 代码即可。徽章只在你自己的域名下显示，来自你域名下端点的最近一次检测，30 天后过期，并链接到公开报告。"},
	{"检测会产生费用吗？", "检测会向你填写的上游发送最多二十多次请求，可能产生上游服务商的少量费用，每次请求最长等待 90 秒，整轮检测最长 10 分钟。"},
}

var pages = []Page{
	{
		Path:        "/",
		Title:       siteName + " 模型评测｜Claude / OpenAI API 中转站真伪检测与榜单",
		Description: "独立、开源的大模型 API 检测：输入地址和密钥，检测 Claude、OpenAI 兼容和图像模型是否为真、协议是否符合官方、用量是否虚标。公开报告、模型榜单、站点目录。Independent, open-source conformance and authenticity tests for model APIs.",
		H1:          siteName + "：大模型 API 真伪与协议一致性检测",
		FAQ:         FAQ,
		Lead:        "只做真模型，不做假货，不掺水。输入任意 Claude、OpenAI 兼容或图像模型的 API 地址，ModelsExam 发送一组固定请求，如实记录返回了什么，并附上证据。每份报告公开，只统计断言得分，不为任何服务商背书。 Independent exams for Claude, OpenAI-compatible and image model APIs.",
		Home:        true,
		MaxRecords:  10,
		Sections: []Section{
			{Heading: "ModelsExam 徽章代表什么", Items: []string{
				"只做真模型，不做假货，不掺水：检查端点的真实行为——身份问答、行为指纹和能力探测，低价模型冒充高价模型通常会在这里露出破绽。",
				"符合官方协议，不虚标：同一组请求遵循官方约定的字段、流式、工具调用和错误格式；上报的 Token 用量会与实际发送内容比对，虚增的数字会被标出。",
				"服务稳定，检测日期看得见：徽章只和最近一次检测一样新，30 天后过期；每份报告都列出延迟和失败的请求。",
				"开源，结果公开：代码采用 AGPL-3.0，所有记录公开。",
			}},
			{Heading: "检测怎么做", Items: []string{
				"选择你的端点使用的协议：Claude（Anthropic Messages）、OpenAI 兼容（Chat Completions 与 Responses）或图像生成（OpenAI Images）。",
				"填写 Base URL 和 API Key，来源可以选官方 Key、AWS Bedrock 或其他类型；系统会读取该站的 /v1/models，点选要检测的模型。",
				"ModelsExam 发送一组固定请求，记录端点实际返回的内容并附上证据。",
				"阅读公开报告：每个结果只描述一个端点在某一时刻的表现，不是排名，也不是背书。",
			}},
			{Heading: "如何读懂徽章", Items: []string{
				"符合（100 分）：所有计分检查均通过。",
				"基本符合（80 到 99 分）：大部分计分检查通过，报告会列出未通过项。",
				"建议核对（低于 80 分）：计分检查未通过。",
				"未完成：检测在所有检查完成前已停止，因此不给出结论。",
			}, Body: []string{"观测项（如延迟、缓存）和来源验证结果会出现在报告里，但不会改变徽章。"}},
			{Heading: "官方基线", Body: []string{"基线是在服务商自己的端点上直接跑的同一套检测。把中转站的报告放在旁边对照，就能看出它和源头有哪里不同。"}, Items: []string{
				"Anthropic API（api.anthropic.com）：Claude 一致性检测。",
				"AWS Bedrock：Claude 一致性检测。",
				"OpenAI API（api.openai.com）：OpenAI 兼容检测。",
				"OpenAI Images（api.openai.com）：图像检测。",
			}},
			{Heading: "站长：在你的网站展示保真徽章", Body: []string{"输入你的域名，复制代码：适合 new-api 首页的脚本、图片或 Markdown。徽章显示你域名下端点的最近一次检测结果，链接到公开报告，只在你自己的域名下提供，检测 30 天后过期。支持亮色、暗色和自动主题。"}},
		},
	},
	{
		Path:        "/models",
		Title:       "模型评测榜单：Claude、GPT 等 API 中转站检测得分一览 | " + siteName,
		Description: "按模型汇总各 API 中转站最近一次公开检测的得分、来源和检测日期，每个结果链接到完整报告。榜单是检测结果列表，不是排名认证。",
		H1:          "模型评测榜单",
		Lead:        "每个模型一张榜：取每个站点最近一次已完成且有得分的检测，按得分从高到低排列，得分相同较新的在前，超过 30 天的标为已过期。这是检测结果的列表，不是对站点好坏的评价。",
		MaxRecords:  0,
		Sections: []Section{{Heading: "榜单规则", Items: []string{
			"每个站点在一张榜上只出现一次，取它最近一次完成的检测。",
			"得分只统计计分断言，观测项和来源验证不计入。",
			"每一行都有检测日期，并链接到公开报告。",
		}}},
	},
	{
		Path:        "/get-badge",
		Title:       "获取 ModelsExam 保真徽章：new-api 首页接入脚本 | " + siteName,
		Description: "输入你的域名，生成专属脚本、图片或 Markdown 代码，在你的网站展示最近一次检测结果。徽章只在你自己的域名下显示，30 天后过期。",
		H1:          "获取你的保真徽章",
		Lead:        "输入域名，复制代码，在你的网站展示最近一次检测结果。徽章 30 天后过期，只提供给你域名下的页面。",
		Sections: []Section{
			{Heading: "徽章如何防盗用", Items: []string{
				"徽章来自域名（或其子域名）下端点最近一次已完成的检测。",
				"脚本和数据只提供给你域名下的页面，别的网站无法加载你的徽章。",
				"检测 30 天后徽章会如实显示“检测已过期”，直到你再次检测。",
				"徽章始终链接到公开报告，任何人（包括你）都无法修改结果。",
			}},
			{Heading: "三种接入方式", Items: []string{
				"脚本（推荐）：粘贴一行 script，适合允许脚本的自定义页脚、主题或模板。",
				"图片：凡是能渲染 HTML 的地方都能用，会自动更新。",
				"Markdown：适合 README、公告或任何支持 Markdown 的页面。",
			}, Body: []string{"徽章支持亮色、暗色和跟随访客的自动主题。截图无法被阻止，访客可以通过可见的域名和报告链接自行核对。"}},
		},
	},
	{
		Path:        "/baselines",
		Title:       "官方基线：在 Anthropic、AWS Bedrock、OpenAI 官方端点上的参照检测 | " + siteName,
		Description: "在 Anthropic、AWS Bedrock 和 OpenAI 官方端点上直接运行的同一套检测，用来对照中转站的检测报告，看出它和源头的差异。",
		H1:          "官方基线",
		Lead:        "基线是在服务商自己的端点上直接跑的同一套检测。把中转站的报告放在旁边对照，就能看出它和源头有哪里不同。",
		Sections: []Section{
			{Heading: "参照端点", Items: []string{
				"Anthropic API（api.anthropic.com）：在 Anthropic 官方端点上运行的 Claude 一致性检测。",
				"AWS Bedrock：在 Bedrock 上运行的 Claude 一致性检测。",
				"OpenAI API（api.openai.com）：在 OpenAI 官方端点上运行的 OpenAI 兼容检测。",
				"OpenAI Images（api.openai.com）：图像检测，含可选的来源验证结果。",
			}, Body: []string{"基线只有在指向真实的公开报告后才会作为结果列出；在此之前显示“参考检测待发布”。"}},
		},
	},
	{
		Path:        "/records",
		Title:       "公开检测记录：API 中转站模型评测历史 | " + siteName,
		Description: "ModelsExam 公开的检测记录列表，每条记录显示网站名称、模型、结果和检测日期，并链接到完整报告。",
		H1:          "公开检测记录",
		Lead:        "最近的公开检测记录，每条带一枚只对应该次检测的徽章。",
		MaxRecords:  50,
	},
	{
		Path:        "/method",
		Title:       "检测方法与独立性：ModelsExam 检查什么、怎么计分 | " + siteName,
		Description: "ModelsExam 检查什么、分数如何计算，以及为什么结果是关于一次检测的证据，而不是排名或认证。",
		H1:          "方法与独立性",
		Lead:        "检查什么、怎么计分，以及为什么结果是一次检测的证据，而不是排名或认证。",
		Sections: []Section{
			{Heading: "检查什么", Items: []string{
				"身份问答、行为指纹和能力探测，判断端点背后的模型表现是否和它声称的一致。",
				"协议一致性：请求与响应字段、流式、工具调用、结构化输出和错误格式。",
				"Token 用量：端点上报的用量与实际发送的内容比对。",
				"延迟、缓存、失败请求等观测项，会列在报告里。",
			}},
			{Heading: "怎么计分", Body: []string{"分数只统计计分断言：100 分表示所有计分检查通过（符合），80 到 99 分为基本符合，低于 80 分为建议核对。提前停止的检测没有结论。观测项和来源验证结果不会改变徽章。"}},
			{Heading: "独立性", Body: []string{"ModelsExam 是独立的开源项目（AGPL-3.0），与 Anthropic、OpenAI、亚马逊云科技没有关联。结果描述的是某个端点和模型在检测时刻的表现，不是排名、认证或背书。"}},
		},
	},
	{
		Path:        "/integrate",
		Title:       "中转站接入：一个链接带用户去检测 API | " + siteName,
		Description: "API 中转站可以在控制台放一个链接，把 Base URL、Key 和模型预先填进 ModelsExam 检测表单，用户核对后自己开始检测。Key 放在 # 后面，不会发给服务器。",
		H1:          "中转站接入",
		Lead:        "在你的控制台加一个「去 ModelsExam 检测」链接：用户点开后，检测表单里的 Base URL、Key 和模型已经填好，核对无误后由用户自己点击开始检测。",
		Sections: []Section{
			{Heading: "链接格式", Body: []string{"https://modelsexam.com/#type=claude&base_url=<Base URL>&key=<API Key>&model=<模型>。参数放在 # 后面，浏览器不会把它们发给服务器；也支持 ?base_url=… 查询参数。每个值都要做 URL 编码。"}, Items: []string{
				"type：claude、openai 或 image，打开哪种检测，默认 claude。",
				"base_url：你的 API 地址，可以带 /v1 后缀。",
				"key：用户的 API Key，页面上以掩码显示。",
				"model：要检测的模型，用户仍可改选。",
			}},
			{Heading: "Key 与隐私", Items: []string{
				"页面读取链接后立即从地址栏移除 Base URL 和 Key，不会留在浏览历史里。",
				"不会自动开始检测，必须由用户点击「开始检测」。",
				"Key 只用于这一次检测，不写入报告或公开记录。",
			}},
		},
	},
}
