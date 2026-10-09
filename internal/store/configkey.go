package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"

	"model-check/common"
)

// ConfigKey identifies a check configuration: protocol, endpoint, model and
// the options that change what is tested. False, empty and null options are
// dropped so an old report and a new one with the same choices match.
func ConfigKey(transport, endpoint, model string, options json.RawMessage) string {
	clean := map[string]any{}
	var raw map[string]any
	if len(options) > 0 && common.Unmarshal(options, &raw) == nil {
		for k, v := range raw {
			switch x := v.(type) {
			case nil:
				continue
			case bool:
				if !x {
					continue
				}
			case string:
				if x == "" {
					continue
				}
			}
			if k == "model" {
				continue
			}
			clean[k] = v
		}
	}
	data, _ := json.Marshal(clean) // map keys are sorted
	sum := sha256.Sum256([]byte(transport + "\n" + strings.TrimRight(endpoint, "/") + "\n" + model + "\n" + string(data)))
	return hex.EncodeToString(sum[:16])
}

// ReportConfigKey reads endpoint, model and options from a stored report.
func ReportConfigKey(transport, reportJSON string) string {
	var doc struct {
		Endpoint string          `json:"endpoint"`
		Model    string          `json:"model"`
		Options  json.RawMessage `json:"options"`
	}
	_ = common.UnmarshalJsonStr(reportJSON, &doc)
	return ConfigKey(transport, doc.Endpoint, doc.Model, doc.Options)
}

// backfillConfigKeys fills the key of runs stored before it existed.
func (s *Store) backfillConfigKeys(ctx context.Context) error {
	for {
		rows := []Run{}
		if err := s.db.WithContext(ctx).Select("id", "transport", "report_json").
			Where("config_key = ? OR config_key IS NULL", "").Limit(200).Find(&rows).Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		for _, r := range rows {
			if err := s.db.WithContext(ctx).Model(&Run{}).Where("id = ?", r.ID).
				Update("config_key", ReportConfigKey(r.Transport, r.ReportJSON)).Error; err != nil {
				return err
			}
		}
	}
}
