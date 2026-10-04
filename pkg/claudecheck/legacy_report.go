package claudecheck

// Legacy schemas preserve existing history; new reports do not populate references.
type BaselineSnapshot struct {
	Model                       string  `json:"model"`
	ResponseModel               string  `json:"response_model"`
	CollectedAt                 string  `json:"collected_at"`
	File                        string  `json:"file"`
	SHA256                      string  `json:"sha256"`
	PDFIdentifier               string  `json:"pdf_identifier"`
	ToolName                    string  `json:"tool_name"`
	ToolCity                    string  `json:"tool_city"`
	ToolUnit                    string  `json:"tool_unit"`
	IntegrityInputTokens        int64   `json:"integrity_input_tokens"`
	IntegrityOutputTokens       int64   `json:"integrity_output_tokens"`
	IntegrityStreamInputTokens  int64   `json:"integrity_stream_input_tokens"`
	IntegrityStreamOutputTokens int64   `json:"integrity_stream_output_tokens"`
	ConsistencyRuns             int     `json:"consistency_runs"`
	ConsistencyOutputTokens     []int64 `json:"consistency_output_tokens"`
	ConsistencyCV               float64 `json:"consistency_cv"`
}

type BaselineReference struct {
	SourceURL    string             `json:"source_url"`
	Commit       string             `json:"commit"`
	License      string             `json:"license"`
	MatchedModel string             `json:"matched_model,omitempty"`
	Snapshots    []BaselineSnapshot `json:"snapshots"`
}
