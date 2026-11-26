package usage

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sync"

	"evergate/internal/store"
)

// catalog.json 随二进制内置的预设价格目录（USD / 百万 token），只作兜底：渠道+模型覆盖 → 全局默认 → 预设目录。
//
//go:embed catalog.json
var catalogJSON []byte

// CatalogEntry 一条预设价格：按预设 id + 上游模型名匹配（供应商按上游模型计费）。
type CatalogEntry struct {
	PresetID               string `json:"preset_id"`
	Model                  string `json:"model"`
	InputMicrosPerMillion  int64  `json:"input_micros_per_million"`
	OutputMicrosPerMillion int64  `json:"output_micros_per_million"`
	// 缓存价只在厂商公开了明确的缓存读写价时填写；缺省（0）计费时按输入价全价计
	CacheReadMicrosPerMillion  int64  `json:"cache_read_micros_per_million,omitempty"`
	CacheWriteMicrosPerMillion int64  `json:"cache_write_micros_per_million,omitempty"`
	SourceURL                  string `json:"source_url"`
}

func (e CatalogEntry) price() store.ModelPrice {
	return store.ModelPrice{
		InputMicrosPerMillion: e.InputMicrosPerMillion, OutputMicrosPerMillion: e.OutputMicrosPerMillion,
		CacheReadMicrosPerMillion: e.CacheReadMicrosPerMillion, CacheWriteMicrosPerMillion: e.CacheWriteMicrosPerMillion,
	}
}

// priceInput 目录价只带厂商明确公开的缓存价；没有的一律不写，避免把 0 当成显式设置。
func (e CatalogEntry) priceInput() PriceInput {
	return PriceInput{
		Input: e.InputMicrosPerMillion, Output: e.OutputMicrosPerMillion,
		CacheRead: optionalPositive(e.CacheReadMicrosPerMillion), CacheWrite: optionalPositive(e.CacheWriteMicrosPerMillion),
	}
}

func optionalPositive(v int64) *int64 {
	if v <= 0 {
		return nil
	}
	return &v
}

type catalogFile struct {
	AsOf    string         `json:"as_of"`
	Note    string         `json:"note"`
	Entries []CatalogEntry `json:"entries"`
}

var (
	catalogOnce sync.Once
	catalogData catalogFile
	catalogErr  error
	catalogIdx  map[string]map[string]CatalogEntry
)

// loadCatalog 解析内置目录并按 preset_id → model 建索引；JSON 非法属于构建期错误，直接 panic 更容易暴露。
func loadCatalog() (catalogFile, map[string]map[string]CatalogEntry, error) {
	catalogOnce.Do(func() {
		if err := json.Unmarshal(catalogJSON, &catalogData); err != nil {
			catalogErr = fmt.Errorf("parse embedded price catalog: %w", err)
			return
		}
		catalogIdx = make(map[string]map[string]CatalogEntry)
		for _, entry := range catalogData.Entries {
			if entry.PresetID == "" || entry.Model == "" {
				catalogErr = fmt.Errorf("price catalog entry missing preset_id or model: %+v", entry)
				return
			}
			if catalogIdx[entry.PresetID] == nil {
				catalogIdx[entry.PresetID] = map[string]CatalogEntry{}
			}
			catalogIdx[entry.PresetID][entry.Model] = entry
		}
	})
	return catalogData, catalogIdx, catalogErr
}

// Catalog 返回内置目录的日期与全部条目（管理台"按预设查看"用）。
func Catalog() (asOf string, note string, entries []CatalogEntry, err error) {
	data, _, err := loadCatalog()
	return data.AsOf, data.Note, data.Entries, err
}

// catalogLookup 按 preset_id + 上游模型查内置价格。
func catalogLookup(presetID, model string) (CatalogEntry, bool) {
	_, idx, err := loadCatalog()
	if err != nil || presetID == "" || model == "" {
		return CatalogEntry{}, false
	}
	entry, ok := idx[presetID][model]
	return entry, ok
}
