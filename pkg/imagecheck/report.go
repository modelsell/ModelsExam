package imagecheck

import (
	"fmt"
	"sort"
	"strings"
)

// Titles are the Chinese check names used in the Markdown export.
var Titles = map[string]string{
	"img_generate_basic":     "基础生成",
	"img_response_shape":     "响应与 usage 形状",
	"img_size_exact":         "尺寸与请求一致",
	"img_error_shape":        "错误响应形状",
	"img_solid_color":        "纯色生成",
	"img_split_layout":       "左右分区布局",
	"img_centered_shape":     "居中形状",
	"img_count_shapes":       "形状计数",
	"img_output_format":      "输出格式",
	"img_size_matrix":        "尺寸矩阵",
	"img_background":         "透明背景",
	"img_n":                  "n 多张",
	"img_quality_levels":     "质量档位",
	"img_stream_partial":     "流式 partial",
	"img_size_invalid":       "非法尺寸",
	"img_edit_basic":         "图片编辑",
	"img_edit_mask":          "遮罩编辑",
	"img_prov_control":       "验真阴性对照",
	"img_prov_c2pa":          "C2PA 内容凭证",
	"img_prov_synthid":       "SynthID 水印",
	"img_prov_model_match":   "凭证模型一致",
	"img_prov_format_matrix": "格式与信号保留",
	"img_prov_baseline":      "官方直连对照",
	"img_usage_fields":       "usage 一致性",
	"img_performance":        "耗时观测",
}

var statusText = map[string]string{"pass": "通过", "fail": "失败", "inconclusive": "无法判定", "skipped": "跳过"}

// StripThumbs removes the live-only thumbnails. Anything persisted or
// published goes through it first: generated images stay private by default.
func StripThumbs(report *Report) {
	for i := range report.Images {
		report.Images[i].Thumb = ""
	}
}

// Markdown renders a report without thumbnails.
func Markdown(report Report) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# 图像检测报告\n\n- 模型：`%s`\n- 端点：%s\n- 开始时间：%s\n- 耗时：%d ms\n", report.Model, report.Endpoint, report.StartedAt, report.DurationMS)
	fmt.Fprintf(&b, "- 请求数：%d，图片数（按请求计）：%d\n", report.RequestsRun, report.ImagesBilled)
	if report.Score != nil {
		fmt.Fprintf(&b, "- 接口一致性得分：%d（只统计断言，不代表模型真伪）\n", *report.Score)
	}
	if p := report.Provenance; p != nil && p.Enabled {
		fmt.Fprintf(&b, "- 官方验真：%s（验真调用 %d 次）\n", p.Level, p.VerifyCalls)
	}
	b.WriteString("\n| 检测项 | 类型 | 结果 | 代码 | 证据 |\n|---|---|---|---|---|\n")
	for _, c := range report.Checks {
		title := Titles[c.ID]
		if title == "" {
			title = c.ID
		}
		fmt.Fprintf(&b, "| %s (`%s`) | %s | %s | `%s` | %s |\n", title, c.ID, c.Kind, statusText[c.Status], c.Code, evidenceLine(c.Evidence))
	}
	b.WriteString("\n> 黑盒观测不能证明真实模型身份或供应商账单。官方验真未检出不等于不是 OpenAI 模型。\n")
	return b.String()
}

func evidenceLine(ev map[string]any) string {
	if len(ev) == 0 {
		return ""
	}
	keys := make([]string, 0, len(ev))
	for k := range ev {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%v", k, ev[k]))
	}
	return strings.ReplaceAll(strings.Join(parts, "; "), "|", "/")
}
