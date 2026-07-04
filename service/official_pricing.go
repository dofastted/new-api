package service

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"gorm.io/gorm"
)

const (
	OfficialPricingProviderOpenAI   = "openai"
	OfficialPricingProviderClaude   = "claude"
	OfficialPricingProviderXAI      = "xai"
	OfficialPricingProviderGemini   = "gemini"
	OfficialPricingProviderGLM      = "glm"
	OfficialPricingProviderDeepSeek = "deepseek"

	OfficialPricingOpenAIURL   = "https://developers.openai.com/api/docs/pricing.md"
	OfficialPricingClaudeURL   = "https://platform.claude.com/docs/en/about-claude/pricing.md"
	OfficialPricingXAIURL      = "https://docs.x.ai/developers/models.md"
	OfficialPricingGeminiURL   = "https://ai.google.dev/gemini-api/docs/pricing"
	OfficialPricingGLMURL      = "https://docs.bigmodel.cn/cn/guide/models/text/glm-4.5"
	OfficialPricingDeepSeekURL = "https://api-docs.deepseek.com/quick_start/pricing"
	maxOfficialPricingBytes    = 10 << 20
)

type OfficialPricingSource struct {
	Provider string `json:"provider"`
	Name     string `json:"name"`
	URL      string `json:"url"`
}

type OfficialPricingSyncResult struct {
	SnapshotID   string                           `json:"snapshot_id"`
	EntriesCount int                              `json:"entries_count"`
	Sources      []OfficialPricingSource          `json:"sources"`
	Counts       map[string]int                   `json:"counts"`
	Errors       map[string]string                `json:"errors,omitempty"`
	MetadataSync *OfficialModelMetadataSyncResult `json:"metadata_sync,omitempty"`
}

type OfficialModelMetadataSyncResult struct {
	CreatedModels  int                    `json:"created_models"`
	UpdatedModels  int                    `json:"updated_models"`
	CreatedVendors int                    `json:"created_vendors"`
	SkippedModels  []string               `json:"skipped_models"`
	CreatedList    []string               `json:"created_list"`
	UpdatedList    []string               `json:"updated_list"`
	Source         OfficialMetadataSource `json:"source"`
}

type OfficialModelMetadataPreview struct {
	Missing   []OfficialModelMetadataItem `json:"missing"`
	Conflicts []OfficialMetadataConflict  `json:"conflicts"`
	Source    OfficialMetadataSource      `json:"source"`
}

type OfficialModelMetadataItem struct {
	ModelName     string `json:"model_name"`
	Vendor        string `json:"vendor"`
	PricingConfig string `json:"pricing_config,omitempty"`
}

type OfficialMetadataConflict struct {
	ModelName string                  `json:"model_name"`
	Fields    []OfficialConflictField `json:"fields"`
}

type OfficialConflictField struct {
	Field    string `json:"field"`
	Local    any    `json:"local"`
	Upstream any    `json:"upstream"`
}

type OfficialMetadataSource struct {
	Name       string                  `json:"name"`
	Providers  []OfficialPricingSource `json:"providers"`
	ModelCount int                     `json:"model_count"`
}

type officialVendorMetadata struct {
	Name string
	Icon string
}

type officialModelMetadataEntry struct {
	ModelName     string
	VendorName    string
	VendorIcon    string
	PricingConfig string
}

type OfficialTokenPricing struct {
	Provider             string
	Model                string
	SourceURL            string
	InputUSDPerMTok      float64
	OutputUSDPerMTok     float64
	CacheReadUSDPerMTok  *float64
	CacheWriteUSDPerMTok *float64
	CacheReadRatio       *float64
	CacheWriteRatio      *float64
}

func DefaultOfficialPricingSources() []OfficialPricingSource {
	return []OfficialPricingSource{
		{Provider: OfficialPricingProviderOpenAI, Name: "OpenAI 官方价格", URL: OfficialPricingOpenAIURL},
		{Provider: OfficialPricingProviderClaude, Name: "Claude 官方价格", URL: OfficialPricingClaudeURL},
		{Provider: OfficialPricingProviderXAI, Name: "xAI 官方价格", URL: OfficialPricingXAIURL},
		{Provider: OfficialPricingProviderGemini, Name: "Gemini 官方价格", URL: OfficialPricingGeminiURL},
		{Provider: OfficialPricingProviderGLM, Name: "GLM 官方价格", URL: OfficialPricingGLMURL},
		{Provider: OfficialPricingProviderDeepSeek, Name: "DeepSeek 官方价格", URL: OfficialPricingDeepSeekURL},
	}
}

func SyncOfficialPricing(ctx context.Context) (*OfficialPricingSyncResult, error) {
	sources := DefaultOfficialPricingSources()
	snapshotID := model.NewOfficialPricingSnapshotID()
	entries, result, err := FetchOfficialPricing(ctx, snapshotID, sources)
	if err != nil {
		_ = model.RecordFailedOfficialPricingSnapshot(snapshotID, officialPricingSourcesText(sources), err)
		return result, err
	}
	rows := officialPricingRowsFromEntries(snapshotID, entries)
	if len(rows) == 0 {
		err = fmt.Errorf("official pricing sync produced no rows")
		_ = model.RecordFailedOfficialPricingSnapshot(snapshotID, officialPricingSourcesText(sources), err)
		return result, err
	}
	if err := model.ReplaceActiveOfficialPricing(snapshotID, officialPricingSourcesText(sources), rows); err != nil {
		return result, err
	}
	if err := model.LoadActiveOfficialPricingIntoRuntime(); err != nil {
		return result, err
	}
	metadataResult, err := syncOfficialModelMetadataRows(rows)
	if err != nil {
		return result, err
	}
	if err := model.LoadModelPricingConfigsIntoRuntime(); err != nil {
		return result, err
	}
	model.RefreshPricing()
	result.MetadataSync = metadataResult
	result.EntriesCount = len(rows)
	return result, nil
}

func SyncOfficialModelMetadata(ctx context.Context) (*OfficialModelMetadataSyncResult, error) {
	rows, err := model.GetActiveOfficialPricingRows()
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		pricingResult, err := SyncOfficialPricing(ctx)
		if err != nil {
			return nil, err
		}
		if pricingResult != nil && pricingResult.MetadataSync != nil {
			return pricingResult.MetadataSync, nil
		}
		rows, err = model.GetActiveOfficialPricingRows()
		if err != nil {
			return nil, err
		}
	}
	metadataResult, err := syncOfficialModelMetadataRows(rows)
	if err != nil {
		return nil, err
	}
	if err := model.LoadModelPricingConfigsIntoRuntime(); err != nil {
		return nil, err
	}
	model.RefreshPricing()
	return metadataResult, nil
}

func PreviewOfficialModelMetadata(ctx context.Context) (*OfficialModelMetadataPreview, error) {
	rows, err := model.GetActiveOfficialPricingRows()
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		entries, _, err := FetchOfficialPricing(ctx, model.NewOfficialPricingSnapshotID(), DefaultOfficialPricingSources())
		if err != nil {
			return nil, err
		}
		rows = officialPricingRowsFromEntries("", entries)
	}
	entries, err := officialModelMetadataEntries(rows)
	if err != nil {
		return nil, err
	}
	preview, err := previewOfficialModelMetadataEntries(entries)
	if err != nil {
		return nil, err
	}
	return preview, nil
}

func FetchOfficialPricing(ctx context.Context, snapshotID string, sources []OfficialPricingSource) ([]OfficialTokenPricing, *OfficialPricingSyncResult, error) {
	result := &OfficialPricingSyncResult{
		SnapshotID: snapshotID,
		Sources:    sources,
		Counts:     map[string]int{},
		Errors:     map[string]string{},
	}
	allEntries := make([]OfficialTokenPricing, 0)
	client := GetHttpClient()
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	for _, source := range sources {
		entries, err := fetchOfficialPricingSource(ctx, client, source)
		if err != nil {
			result.Errors[source.Provider] = err.Error()
			continue
		}
		result.Counts[source.Provider] = len(entries)
		allEntries = append(allEntries, entries...)
	}
	if len(result.Errors) > 0 {
		return nil, result, fmt.Errorf("official pricing source failures: %v", result.Errors)
	}
	allEntries = dedupeOfficialPricing(allEntries)
	result.EntriesCount = len(allEntries)
	return allEntries, result, nil
}

func fetchOfficialPricingSource(ctx context.Context, client *http.Client, source OfficialPricingSource) ([]OfficialTokenPricing, error) {
	reqCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, source.URL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("%s returned %s", source.Name, resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxOfficialPricingBytes))
	if err != nil {
		return nil, err
	}
	switch source.Provider {
	case OfficialPricingProviderOpenAI:
		return ParseOpenAIOfficialTokenPricing(bytes.NewReader(body), source.URL)
	case OfficialPricingProviderClaude:
		return ParseClaudeOfficialTokenPricing(bytes.NewReader(body), source.URL)
	case OfficialPricingProviderXAI:
		return ParseXAIOfficialTokenPricing(bytes.NewReader(body), source.URL)
	case OfficialPricingProviderGemini:
		return ParseGeminiOfficialTokenPricing(bytes.NewReader(body), source.URL)
	case OfficialPricingProviderGLM:
		return ParseGLMOfficialTokenPricing(bytes.NewReader(body), source.URL)
	case OfficialPricingProviderDeepSeek:
		return ParseDeepSeekOfficialTokenPricing(bytes.NewReader(body), source.URL)
	default:
		return nil, fmt.Errorf("unsupported official pricing provider: %s", source.Provider)
	}
}

func officialPricingSourcesText(sources []OfficialPricingSource) string {
	parts := make([]string, 0, len(sources))
	for _, source := range sources {
		parts = append(parts, source.Provider+"="+source.URL)
	}
	return strings.Join(parts, "\n")
}

func officialTokenPricingToRow(snapshotID string, entry OfficialTokenPricing) model.OfficialModelPrice {
	modelRatio := entry.InputUSDPerMTok * float64(ratio_setting.USD) / 1000
	completionRatio := 0.0
	if entry.InputUSDPerMTok > 0 {
		completionRatio = entry.OutputUSDPerMTok / entry.InputUSDPerMTok
	}
	cacheRatio := entry.CacheReadRatio
	if cacheRatio == nil && entry.CacheReadUSDPerMTok != nil && entry.InputUSDPerMTok > 0 {
		ratio := *entry.CacheReadUSDPerMTok / entry.InputUSDPerMTok
		cacheRatio = &ratio
	}
	createCacheRatio := entry.CacheWriteRatio
	if createCacheRatio == nil && entry.CacheWriteUSDPerMTok != nil && entry.InputUSDPerMTok > 0 {
		ratio := *entry.CacheWriteUSDPerMTok / entry.InputUSDPerMTok
		createCacheRatio = &ratio
	}
	return model.OfficialModelPrice{
		SnapshotID:              snapshotID,
		Provider:                entry.Provider,
		ModelName:               strings.TrimSpace(entry.Model),
		SourceURL:               entry.SourceURL,
		InputUSDPerMTokens:      entry.InputUSDPerMTok,
		OutputUSDPerMTokens:     entry.OutputUSDPerMTok,
		CacheReadUSDPerMTokens:  entry.CacheReadUSDPerMTok,
		CacheWriteUSDPerMTokens: entry.CacheWriteUSDPerMTok,
		ModelRatio:              roundOfficialRatioValue(modelRatio),
		CompletionRatio:         roundOfficialRatioValue(completionRatio),
		CacheRatio:              roundOfficialRatioPointer(cacheRatio),
		CreateCacheRatio:        roundOfficialRatioPointer(createCacheRatio),
	}
}

func officialPricingRowsFromEntries(snapshotID string, entries []OfficialTokenPricing) []model.OfficialModelPrice {
	rows := make([]model.OfficialModelPrice, 0, len(entries))
	for _, entry := range entries {
		row := officialTokenPricingToRow(snapshotID, entry)
		if row.ModelName == "" {
			continue
		}
		rows = append(rows, row)
	}
	return rows
}

func syncOfficialModelMetadataRows(rows []model.OfficialModelPrice) (*OfficialModelMetadataSyncResult, error) {
	entries, err := officialModelMetadataEntries(rows)
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("official model metadata sync has no model entries")
	}

	result := &OfficialModelMetadataSyncResult{
		SkippedModels: []string{},
		CreatedList:   []string{},
		UpdatedList:   []string{},
		Source:        officialMetadataSource(len(entries)),
	}
	err = model.DB.Transaction(func(tx *gorm.DB) error {
		vendorIDs, createdVendors, err := ensureOfficialMetadataVendors(tx, entries)
		if err != nil {
			return err
		}
		result.CreatedVendors = createdVendors

		existing, err := loadOfficialMetadataModels(tx, entries)
		if err != nil {
			return err
		}
		now := common.GetTimestamp()
		for _, entry := range entries {
			vendorID := vendorIDs[entry.VendorName]
			current, ok := existing[entry.ModelName]
			if !ok {
				item := model.Model{
					ModelName:     entry.ModelName,
					VendorID:      vendorID,
					PricingConfig: entry.PricingConfig,
					Status:        1,
					SyncOfficial:  1,
					NameRule:      model.NameRuleExact,
					CreatedTime:   now,
					UpdatedTime:   now,
				}
				if err := tx.Create(&item).Error; err != nil {
					return err
				}
				result.CreatedModels++
				result.CreatedList = append(result.CreatedList, entry.ModelName)
				continue
			}
			updates := officialMetadataModelUpdates(current, entry, vendorID, now)
			if len(updates) == 0 {
				continue
			}
			if err := tx.Model(&model.Model{}).Where("id = ?", current.Id).Updates(updates).Error; err != nil {
				return err
			}
			result.UpdatedModels++
			result.UpdatedList = append(result.UpdatedList, entry.ModelName)
		}
		staleResult := tx.Model(&model.Model{}).
			Where("vendor_id IN ?", officialMetadataVendorIDs(vendorIDs)).
			Where("model_name NOT IN ?", officialMetadataModelNames(entries)).
			Where("icon <> ? OR description <> ? OR tags <> ? OR endpoints <> ? OR pricing_config <> ?", "", "", "", "", "").
			Updates(map[string]any{
				"icon":           "",
				"description":    "",
				"tags":           "",
				"endpoints":      "",
				"pricing_config": "",
				"updated_time":   now,
			})
		if staleResult.Error != nil {
			return staleResult.Error
		}
		if staleResult.RowsAffected > 0 {
			result.UpdatedModels += int(staleResult.RowsAffected)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func officialMetadataVendorIDs(vendorIDs map[string]int) []int {
	ids := make([]int, 0, len(vendorIDs))
	for _, id := range vendorIDs {
		if id != 0 {
			ids = append(ids, id)
		}
	}
	return ids
}

func officialMetadataModelNames(entries []officialModelMetadataEntry) []string {
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.ModelName != "" {
			names = append(names, entry.ModelName)
		}
	}
	return names
}

func previewOfficialModelMetadataEntries(entries []officialModelMetadataEntry) (*OfficialModelMetadataPreview, error) {
	if len(entries) == 0 {
		return nil, fmt.Errorf("official model metadata preview has no model entries")
	}
	existing, err := loadOfficialMetadataModels(model.DB, entries)
	if err != nil {
		return nil, err
	}
	missing := make([]OfficialModelMetadataItem, 0)
	for _, entry := range entries {
		if _, ok := existing[entry.ModelName]; ok {
			continue
		}
		missing = append(missing, OfficialModelMetadataItem{
			ModelName:     entry.ModelName,
			Vendor:        entry.VendorName,
			PricingConfig: entry.PricingConfig,
		})
	}
	return &OfficialModelMetadataPreview{
		Missing:   missing,
		Conflicts: []OfficialMetadataConflict{},
		Source:    officialMetadataSource(len(entries)),
	}, nil
}

func officialModelMetadataEntries(rows []model.OfficialModelPrice) ([]officialModelMetadataEntry, error) {
	entries := make([]officialModelMetadataEntry, 0, len(rows))
	seen := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		modelName := strings.TrimSpace(row.ModelName)
		if modelName == "" {
			continue
		}
		if _, ok := seen[modelName]; ok {
			continue
		}
		vendor, ok := officialPricingProviderVendor(row.Provider)
		if !ok {
			continue
		}
		pricingConfig, err := officialModelPricingConfig(row)
		if err != nil {
			return nil, err
		}
		seen[modelName] = struct{}{}
		entries = append(entries, officialModelMetadataEntry{
			ModelName:     modelName,
			VendorName:    vendor.Name,
			VendorIcon:    vendor.Icon,
			PricingConfig: pricingConfig,
		})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].ModelName < entries[j].ModelName
	})
	return entries, nil
}

func officialModelPricingConfig(row model.OfficialModelPrice) (string, error) {
	cfg := model.ModelPricingConfig{
		Mode:            model.ModelPricingModePerToken,
		Ratio:           float64Pointer(row.ModelRatio),
		CompletionRatio: float64Pointer(row.CompletionRatio),
	}
	if row.CacheRatio != nil {
		cfg.CacheRatio = float64Pointer(*row.CacheRatio)
	}
	if row.CreateCacheRatio != nil {
		cfg.CreateCacheRatio = float64Pointer(*row.CreateCacheRatio)
	}
	payload, err := common.Marshal(cfg)
	if err != nil {
		return "", fmt.Errorf("marshal official pricing config for %s: %w", row.ModelName, err)
	}
	return string(payload), nil
}

func float64Pointer(value float64) *float64 {
	return &value
}

func officialPricingProviderVendor(provider string) (officialVendorMetadata, bool) {
	switch provider {
	case OfficialPricingProviderOpenAI:
		return officialVendorMetadata{Name: "OpenAI", Icon: "OpenAI"}, true
	case OfficialPricingProviderClaude:
		return officialVendorMetadata{Name: "Anthropic", Icon: "Claude.Color"}, true
	case OfficialPricingProviderXAI:
		return officialVendorMetadata{Name: "xAI", Icon: "XAI"}, true
	case OfficialPricingProviderGemini:
		return officialVendorMetadata{Name: "Google", Icon: "Gemini.Color"}, true
	case OfficialPricingProviderGLM:
		return officialVendorMetadata{Name: "智谱", Icon: "Zhipu.Color"}, true
	case OfficialPricingProviderDeepSeek:
		return officialVendorMetadata{Name: "DeepSeek", Icon: "DeepSeek.Color"}, true
	default:
		return officialVendorMetadata{}, false
	}
}

func officialMetadataSource(modelCount int) OfficialMetadataSource {
	return OfficialMetadataSource{
		Name:       "official-pricing",
		Providers:  DefaultOfficialPricingSources(),
		ModelCount: modelCount,
	}
}

func ensureOfficialMetadataVendors(tx *gorm.DB, entries []officialModelMetadataEntry) (map[string]int, int, error) {
	vendorNames := make([]string, 0)
	vendorByName := make(map[string]officialVendorMetadata)
	for _, entry := range entries {
		if _, ok := vendorByName[entry.VendorName]; ok {
			continue
		}
		vendorByName[entry.VendorName] = officialVendorMetadata{Name: entry.VendorName, Icon: entry.VendorIcon}
		vendorNames = append(vendorNames, entry.VendorName)
	}
	var existing []model.Vendor
	if err := tx.Where("name IN ?", vendorNames).Find(&existing).Error; err != nil {
		return nil, 0, err
	}
	vendorIDs := make(map[string]int, len(vendorNames))
	for _, item := range existing {
		vendorIDs[item.Name] = item.Id
	}
	created := 0
	now := common.GetTimestamp()
	for _, name := range vendorNames {
		if _, ok := vendorIDs[name]; ok {
			continue
		}
		metadata := vendorByName[name]
		vendor := model.Vendor{
			Name:        metadata.Name,
			Icon:        metadata.Icon,
			Status:      1,
			CreatedTime: now,
			UpdatedTime: now,
		}
		if err := tx.Create(&vendor).Error; err != nil {
			return nil, created, err
		}
		vendorIDs[name] = vendor.Id
		created++
	}
	return vendorIDs, created, nil
}

func loadOfficialMetadataModels(tx *gorm.DB, entries []officialModelMetadataEntry) (map[string]model.Model, error) {
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.ModelName)
	}
	var models []model.Model
	if err := tx.Where("model_name IN ?", names).Find(&models).Error; err != nil {
		return nil, err
	}
	byName := make(map[string]model.Model, len(models))
	for _, item := range models {
		byName[item.ModelName] = item
	}
	return byName, nil
}

func officialMetadataModelUpdates(current model.Model, entry officialModelMetadataEntry, vendorID int, now int64) map[string]any {
	updates := make(map[string]any)
	if current.VendorID != vendorID {
		updates["vendor_id"] = vendorID
	}
	if current.Icon != "" {
		updates["icon"] = ""
	}
	if current.Description != "" {
		updates["description"] = ""
	}
	if current.Tags != "" {
		updates["tags"] = ""
	}
	if current.Endpoints != "" {
		updates["endpoints"] = ""
	}
	if current.PricingConfig != entry.PricingConfig {
		updates["pricing_config"] = entry.PricingConfig
	}
	if current.NameRule != model.NameRuleExact {
		updates["name_rule"] = model.NameRuleExact
	}
	if len(updates) > 0 {
		updates["sync_official"] = 1
		updates["updated_time"] = now
	}
	return updates
}

func dedupeOfficialPricing(entries []OfficialTokenPricing) []OfficialTokenPricing {
	byKey := make(map[string]OfficialTokenPricing, len(entries))
	for _, entry := range entries {
		modelName := strings.TrimSpace(entry.Model)
		if modelName == "" || !isValidOfficialCost(entry.InputUSDPerMTok) || !isValidOfficialCost(entry.OutputUSDPerMTok) {
			continue
		}
		if entry.InputUSDPerMTok == 0 && entry.OutputUSDPerMTok > 0 {
			continue
		}
		entry.Model = modelName
		byKey[entry.Provider+"/"+modelName] = entry
	}
	keys := make([]string, 0, len(byKey))
	for key := range byKey {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]OfficialTokenPricing, 0, len(keys))
	for _, key := range keys {
		out = append(out, byKey[key])
	}
	return out
}

func ConvertOfficialTokenPricingToRatioData(entries []OfficialTokenPricing) (map[string]any, error) {
	modelRatioMap := make(map[string]any)
	completionRatioMap := make(map[string]any)
	cacheRatioMap := make(map[string]any)
	createCacheRatioMap := make(map[string]any)
	for _, entry := range dedupeOfficialPricing(entries) {
		row := officialTokenPricingToRow("", entry)
		if row.InputUSDPerMTokens == 0 {
			modelRatioMap[row.ModelName] = 0.0
			continue
		}
		modelRatioMap[row.ModelName] = row.ModelRatio
		completionRatioMap[row.ModelName] = row.CompletionRatio
		if row.CacheRatio != nil {
			cacheRatioMap[row.ModelName] = *row.CacheRatio
		}
		if row.CreateCacheRatio != nil {
			createCacheRatioMap[row.ModelName] = *row.CreateCacheRatio
		}
	}
	if len(modelRatioMap) == 0 {
		return nil, fmt.Errorf("no valid official pricing entries found")
	}
	converted := make(map[string]any)
	converted["model_ratio"] = modelRatioMap
	if len(completionRatioMap) > 0 {
		converted["completion_ratio"] = completionRatioMap
	}
	if len(cacheRatioMap) > 0 {
		converted["cache_ratio"] = cacheRatioMap
	}
	if len(createCacheRatioMap) > 0 {
		converted["create_cache_ratio"] = createCacheRatioMap
	}
	return converted, nil
}

func ConvertOpenAIOfficialPricingToRatioData(reader io.Reader) (map[string]any, error) {
	entries, err := ParseOpenAIOfficialTokenPricing(reader, OfficialPricingOpenAIURL)
	if err != nil {
		return nil, err
	}
	return ConvertOfficialTokenPricingToRatioData(entries)
}

func ConvertClaudeOfficialPricingToRatioData(reader io.Reader) (map[string]any, error) {
	entries, err := ParseClaudeOfficialTokenPricing(reader, OfficialPricingClaudeURL)
	if err != nil {
		return nil, err
	}
	return ConvertOfficialTokenPricingToRatioData(entries)
}

func ConvertGeminiOfficialPricingToRatioData(reader io.Reader) (map[string]any, error) {
	entries, err := ParseGeminiOfficialTokenPricing(reader, OfficialPricingGeminiURL)
	if err != nil {
		return nil, err
	}
	return ConvertOfficialTokenPricingToRatioData(entries)
}

func ConvertGLMOfficialPricingToRatioData(reader io.Reader) (map[string]any, error) {
	entries, err := ParseGLMOfficialTokenPricing(reader, OfficialPricingGLMURL)
	if err != nil {
		return nil, err
	}
	return ConvertOfficialTokenPricingToRatioData(entries)
}

func ConvertXAIOfficialPricingToRatioData(reader io.Reader) (map[string]any, error) {
	entries, err := ParseXAIOfficialTokenPricing(reader, OfficialPricingXAIURL)
	if err != nil {
		return nil, err
	}
	return ConvertOfficialTokenPricingToRatioData(entries)
}

func ConvertDeepSeekOfficialPricingToRatioData(reader io.Reader) (map[string]any, error) {
	entries, err := ParseDeepSeekOfficialTokenPricing(reader, OfficialPricingDeepSeekURL)
	if err != nil {
		return nil, err
	}
	return ConvertOfficialTokenPricingToRatioData(entries)
}

var openAIPricingRowPattern = regexp.MustCompile(`\["([^"]+)",\s*([^,\]]+),\s*([^,\]]+),\s*([^\]]+)\]`)

func ParseOpenAIOfficialTokenPricing(reader io.Reader, sourceURL string) ([]OfficialTokenPricing, error) {
	bodyBytes, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read OpenAI pricing response: %w", err)
	}
	section := openAIStandardPricingSection(string(bodyBytes))
	matches := openAIPricingRowPattern.FindAllStringSubmatch(section, -1)
	if len(matches) == 0 {
		return nil, fmt.Errorf("no OpenAI pricing rows found")
	}
	entries := make([]OfficialTokenPricing, 0, len(matches))
	for _, match := range matches {
		input, ok := parseOptionalOfficialPriceToken(match[2])
		if !ok {
			continue
		}
		cachedInput, hasCachedInput := parseOptionalOfficialPriceToken(match[3])
		output, ok := parseOptionalOfficialPriceToken(match[4])
		if !ok {
			continue
		}
		var cacheReadRatio *float64
		var cacheReadPrice *float64
		if hasCachedInput && input > 0 {
			ratio := cachedInput / input
			cacheReadRatio = &ratio
			cacheReadPrice = &cachedInput
		}
		entries = append(entries, OfficialTokenPricing{
			Provider:            OfficialPricingProviderOpenAI,
			Model:               normalizeOpenAIModelName(match[1]),
			SourceURL:           sourceURL,
			InputUSDPerMTok:     input,
			OutputUSDPerMTok:    output,
			CacheReadUSDPerMTok: cacheReadPrice,
			CacheReadRatio:      cacheReadRatio,
		})
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("no valid OpenAI pricing entries found")
	}
	return entries, nil
}

func parseOptionalOfficialPriceToken(token string) (float64, bool) {
	trimmed := strings.TrimSpace(token)
	trimmed = strings.Trim(trimmed, `"`)
	if trimmed == "" || trimmed == "-" || trimmed == "null" {
		return 0, false
	}
	value, err := strconv.ParseFloat(trimmed, 64)
	return value, err == nil && isValidOfficialCost(value)
}

func normalizeOpenAIModelName(raw string) string {
	modelName := strings.TrimSpace(raw)
	if idx := strings.Index(modelName, " ("); idx >= 0 {
		modelName = modelName[:idx]
	}
	return strings.TrimSpace(modelName)
}

func openAIStandardPricingSection(content string) string {
	start := strings.Index(content, `tier="standard"`)
	if start < 0 {
		return content
	}
	section := content[start:]
	if end := strings.Index(section, `data-value="batch"`); end >= 0 {
		return section[:end]
	}
	return section
}

func ParseClaudeOfficialTokenPricing(reader io.Reader, sourceURL string) ([]OfficialTokenPricing, error) {
	bodyBytes, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read Claude pricing response: %w", err)
	}
	lines := strings.Split(string(bodyBytes), "\n")
	entries := make([]OfficialTokenPricing, 0)
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "| Claude ") {
			continue
		}
		cells := strings.Split(trimmed, "|")
		if len(cells) < 7 {
			continue
		}
		modelIDs := claudeModelIDs(cells[1])
		if len(modelIDs) == 0 {
			continue
		}
		input, inputOK := parseUSDPerMTok(cells[2])
		cacheWrite, cacheWriteOK := parseUSDPerMTok(cells[3])
		cacheRead, cacheReadOK := parseUSDPerMTok(cells[5])
		output, outputOK := parseUSDPerMTok(cells[6])
		if !inputOK || !outputOK {
			continue
		}
		var cacheReadRatio *float64
		var cacheReadPrice *float64
		if cacheReadOK && input > 0 {
			ratio := cacheRead / input
			cacheReadRatio = &ratio
			cacheReadPrice = &cacheRead
		}
		var cacheWriteRatio *float64
		var cacheWritePrice *float64
		if cacheWriteOK && input > 0 {
			ratio := cacheWrite / input
			cacheWriteRatio = &ratio
			cacheWritePrice = &cacheWrite
		}
		for _, modelID := range modelIDs {
			entries = append(entries, OfficialTokenPricing{
				Provider:             OfficialPricingProviderClaude,
				Model:                modelID,
				SourceURL:            sourceURL,
				InputUSDPerMTok:      input,
				OutputUSDPerMTok:     output,
				CacheReadUSDPerMTok:  cacheReadPrice,
				CacheWriteUSDPerMTok: cacheWritePrice,
				CacheReadRatio:       cacheReadRatio,
				CacheWriteRatio:      cacheWriteRatio,
			})
		}
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("no valid Claude pricing entries found")
	}
	return entries, nil
}

func parseUSDPerMTok(cell string) (float64, bool) {
	cleaned := strings.TrimSpace(cell)
	if cleaned == "" || cleaned == "-" {
		return 0, false
	}
	idx := strings.Index(cleaned, "$ ")
	if idx >= 0 {
		cleaned = cleaned[idx+1:]
	} else if idx = strings.Index(cleaned, "$"); idx >= 0 {
		cleaned = cleaned[idx+1:]
	}
	cleaned = strings.TrimSpace(cleaned)
	parts := strings.Fields(cleaned)
	if len(parts) == 0 {
		return 0, false
	}
	value, err := strconv.ParseFloat(strings.TrimPrefix(parts[0], "$"), 64)
	return value, err == nil && isValidOfficialCost(value)
}

func claudeModelIDs(label string) []string {
	switch strings.ToLower(cleanClaudeModelLabel(label)) {
	case "claude fable 5":
		return []string{"claude-fable-5"}
	case "claude mythos 5", "claude mythos preview":
		return []string{"claude-mythos-5"}
	case "claude opus 4.8":
		return expandClaudeOpusVariantIDs("claude-opus-4-8")
	case "claude opus 4.7":
		return expandClaudeOpusVariantIDs("claude-opus-4-7")
	case "claude opus 4.6":
		return expandClaudeOpusVariantIDs("claude-opus-4-6")
	case "claude opus 4.5":
		return []string{"claude-opus-4-5", "claude-opus-4-5-20251101", "claude-opus-4-5-20251101-thinking"}
	case "claude opus 4.1":
		return []string{"claude-opus-4-1", "claude-opus-4-1-20250805", "claude-opus-4-1-20250805-thinking"}
	case "claude opus 4":
		return []string{"claude-opus-4", "claude-opus-4-20250514", "claude-opus-4-20250514-thinking"}
	case "claude sonnet 5":
		return []string{"claude-sonnet-5"}
	case "claude sonnet 5 starting september 1, 2026":
		return nil
	case "claude sonnet 4.6":
		return []string{"claude-sonnet-4-6", "claude-sonnet-4-6-thinking"}
	case "claude sonnet 4.5":
		return []string{"claude-sonnet-4-5", "claude-sonnet-4-5-20250929", "claude-sonnet-4-5-20250929-thinking"}
	case "claude sonnet 4":
		return []string{"claude-sonnet-4", "claude-sonnet-4-20250514", "claude-sonnet-4-20250514-thinking"}
	case "claude haiku 4.5":
		return []string{"claude-haiku-4-5", "claude-haiku-4-5-20251001"}
	case "claude haiku 3.5":
		return []string{"claude-3-5-haiku", "claude-3-5-haiku-20241022"}
	default:
		return nil
	}
}

func cleanClaudeModelLabel(raw string) string {
	modelName := strings.TrimSpace(raw)
	if idx := strings.Index(modelName, "("); idx >= 0 {
		modelName = strings.TrimSpace(modelName[:idx])
	}
	if idx := strings.Index(modelName, "["); idx >= 0 {
		modelName = strings.TrimSpace(modelName[:idx])
	}
	return strings.TrimSpace(modelName)
}

func expandClaudeOpusVariantIDs(base string) []string {
	variants := []string{base, base + "-thinking", base + "-max", base + "-high", base + "-medium", base + "-low"}
	if strings.HasPrefix(base, "claude-opus-4-7") || strings.HasPrefix(base, "claude-opus-4-8") {
		variants = append(variants, base+"-xhigh")
	}
	return variants
}

func ParseGeminiOfficialTokenPricing(reader io.Reader, sourceURL string) ([]OfficialTokenPricing, error) {
	bodyBytes, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read Gemini pricing response: %w", err)
	}
	content := stripHTML(string(bodyBytes))
	entries := parseGeminiMarkdownPricing(content, sourceURL)
	if len(entries) == 0 {
		entries = parseGeminiNormalizedPricing(content, sourceURL)
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("no valid Gemini pricing entries found")
	}
	return entries, nil
}

func parseGeminiMarkdownPricing(content string, sourceURL string) []OfficialTokenPricing {
	lines := strings.Split(content, "\n")
	entries := make([]OfficialTokenPricing, 0)
	modelIDs := make([]string, 0)
	inStandard := false
	standardSeen := false
	var input, output, cacheRead float64
	var inputOK, outputOK, cacheOK bool
	flush := func() {
		if len(modelIDs) == 0 || !inputOK || !outputOK || strings.Contains(strings.Join(modelIDs, ","), "imagen-") || strings.Contains(strings.Join(modelIDs, ","), "veo-") {
			return
		}
		var cacheReadRatio *float64
		var cacheReadPrice *float64
		if cacheOK && input > 0 {
			ratio := cacheRead / input
			cacheReadRatio = &ratio
			cacheReadPrice = &cacheRead
		}
		for _, modelID := range modelIDs {
			entries = append(entries, OfficialTokenPricing{
				Provider:            OfficialPricingProviderGemini,
				Model:               modelID,
				SourceURL:           sourceURL,
				InputUSDPerMTok:     input,
				OutputUSDPerMTok:    output,
				CacheReadUSDPerMTok: cacheReadPrice,
				CacheReadRatio:      cacheReadRatio,
			})
		}
	}
	resetPrices := func() {
		inStandard = false
		input, output, cacheRead = 0, 0, 0
		inputOK, outputOK, cacheOK = false, false, false
	}
	for _, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		if strings.HasPrefix(line, "## ") {
			flush()
			modelIDs = nil
			resetPrices()
			standardSeen = false
			continue
		}
		ids := geminiModelIDsFromLine(line)
		if len(ids) > 0 {
			modelIDs = ids
			resetPrices()
			standardSeen = false
			continue
		}
		if strings.HasPrefix(line, "### ") {
			sectionName := strings.TrimSpace(strings.TrimPrefix(line, "### "))
			if inStandard {
				flush()
			}
			resetPrices()
			inStandard = strings.EqualFold(sectionName, "Standard")
			if inStandard {
				standardSeen = true
			}
			continue
		}
		if len(modelIDs) == 0 {
			continue
		}
		allowDirectTable := !standardSeen
		if strings.HasPrefix(line, "| Input price") && (inStandard || (allowDirectTable && !inputOK)) {
			input, inputOK = parseFirstUSDFromPricingLine(line)
			continue
		}
		if strings.HasPrefix(line, "| Output price") && (inStandard || (allowDirectTable && !outputOK)) && !strings.Contains(strings.ToLower(line), "per image") && !strings.Contains(strings.ToLower(line), "per second") {
			output, outputOK = parseFirstUSDFromPricingLine(line)
			continue
		}
		if strings.HasPrefix(line, "| Context caching price") && (inStandard || (allowDirectTable && !cacheOK)) {
			cacheRead, cacheOK = parseFirstUSDFromPricingLine(line)
		}
	}
	flush()
	return entries
}

func parseGeminiNormalizedPricing(content string, sourceURL string) []OfficialTokenPricing {
	normalized := strings.Join(strings.Fields(content), " ")
	modelMatches := geminiModelIDPattern.FindAllStringIndex(normalized, -1)
	entries := make([]OfficialTokenPricing, 0)
	seen := make(map[string]struct{}, len(modelMatches))
	for _, match := range modelMatches {
		modelID := normalized[match[0]:match[1]]
		if !isGeminiPricingModelID(modelID) {
			continue
		}
		if _, ok := seen[modelID]; ok {
			continue
		}
		end := match[0] + 5000
		if end > len(normalized) {
			end = len(normalized)
		}
		input, output, cacheRead, cacheOK, ok := parseGeminiStandardPrices(normalized[match[0]:end])
		if !ok {
			continue
		}
		var cacheReadRatio *float64
		var cacheReadPrice *float64
		if cacheOK && input > 0 {
			ratio := cacheRead / input
			cacheReadRatio = &ratio
			cacheReadPrice = &cacheRead
		}
		seen[modelID] = struct{}{}
		entries = append(entries, OfficialTokenPricing{
			Provider:            OfficialPricingProviderGemini,
			Model:               modelID,
			SourceURL:           sourceURL,
			InputUSDPerMTok:     input,
			OutputUSDPerMTok:    output,
			CacheReadUSDPerMTok: cacheReadPrice,
			CacheReadRatio:      cacheReadRatio,
		})
	}
	return entries
}

func isGeminiPricingModelID(modelID string) bool {
	return strings.HasPrefix(modelID, "gemini-") &&
		!strings.HasPrefix(modelID, "gemini-api") &&
		!strings.Contains(modelID, "/")
}

var geminiModelIDPattern = regexp.MustCompile(`gemini-[a-z0-9][a-z0-9.-]*`)
var geminiStandardPricePattern = regexp.MustCompile(`(?i)Standard\s+.*?Input price\s+[^$]*\$\s*([0-9]+(?:\.[0-9]+)?).*?Output price[^$]*\$\s*([0-9]+(?:\.[0-9]+)?)`)
var geminiCacheReadPricePattern = regexp.MustCompile(`(?i)Context caching price\s+[^$]*\$\s*([0-9]+(?:\.[0-9]+)?)`)

func parseGeminiStandardPrices(content string) (input float64, output float64, cacheRead float64, cacheOK bool, ok bool) {
	match := geminiStandardPricePattern.FindStringSubmatch(content)
	if len(match) < 3 {
		return 0, 0, 0, false, false
	}
	input, err := strconv.ParseFloat(match[1], 64)
	if err != nil || !isValidOfficialCost(input) {
		return 0, 0, 0, false, false
	}
	output, err = strconv.ParseFloat(match[2], 64)
	if err != nil || !isValidOfficialCost(output) {
		return 0, 0, 0, false, false
	}
	cacheMatch := geminiCacheReadPricePattern.FindStringSubmatch(content)
	if len(cacheMatch) >= 2 {
		cacheRead, err = strconv.ParseFloat(cacheMatch[1], 64)
		cacheOK = err == nil && isValidOfficialCost(cacheRead)
	}
	return input, output, cacheRead, cacheOK, true
}

var codeModelPattern = regexp.MustCompile("`([^`]+)`")

func geminiModelIDsFromLine(line string) []string {
	if !strings.Contains(line, "gemini-") {
		return nil
	}
	matches := codeModelPattern.FindAllStringSubmatch(line, -1)
	ids := make([]string, 0, len(matches))
	for _, match := range matches {
		for _, part := range strings.Split(match[1], ",") {
			modelID := strings.TrimSpace(part)
			if strings.HasPrefix(modelID, "gemini-") {
				ids = append(ids, modelID)
			}
		}
	}
	return ids
}

var usdAmountPattern = regexp.MustCompile(`\$\s*([0-9]+(?:\.[0-9]+)?)`)

func parseFirstUSDFromPricingLine(line string) (float64, bool) {
	matches := usdAmountPattern.FindStringSubmatch(line)
	if len(matches) < 2 {
		return 0, false
	}
	value, err := strconv.ParseFloat(matches[1], 64)
	return value, err == nil && isValidOfficialCost(value)
}

func ParseGLMOfficialTokenPricing(reader io.Reader, sourceURL string) ([]OfficialTokenPricing, error) {
	bodyBytes, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read GLM pricing response: %w", err)
	}
	content := stripHTML(string(bodyBytes))
	inputCNY, outputCNY, ok := parseGLMSeriesCNYPricing(content)
	if !ok {
		return nil, fmt.Errorf("no GLM token pricing rows found")
	}
	modelIDs := glmModelIDsFromContent(content)
	if len(modelIDs) == 0 {
		return nil, fmt.Errorf("no GLM model ids found")
	}
	inputUSD := inputCNY / ratio_setting.USD2RMB
	outputUSD := outputCNY / ratio_setting.USD2RMB
	entries := make([]OfficialTokenPricing, 0, len(modelIDs))
	for _, modelID := range modelIDs {
		entries = append(entries, OfficialTokenPricing{
			Provider:         OfficialPricingProviderGLM,
			Model:            modelID,
			SourceURL:        sourceURL,
			InputUSDPerMTok:  inputUSD,
			OutputUSDPerMTok: outputUSD,
		})
	}
	return entries, nil
}

var glmPricePattern = regexp.MustCompile(`输入\s*([0-9]+(?:\.[0-9]+)?)\s*元/百万\s*tokens[，,]\s*输出\s*([0-9]+(?:\.[0-9]+)?)\s*元/百万\s*tokens`)

func parseGLMSeriesCNYPricing(content string) (float64, float64, bool) {
	matches := glmPricePattern.FindStringSubmatch(content)
	if len(matches) < 3 {
		return 0, 0, false
	}
	input, inputErr := strconv.ParseFloat(matches[1], 64)
	output, outputErr := strconv.ParseFloat(matches[2], 64)
	return input, output, inputErr == nil && outputErr == nil && isValidOfficialCost(input) && isValidOfficialCost(output)
}

var glmNamePattern = regexp.MustCompile(`GLM-[0-9]+(?:\.[0-9]+)*(?:-[A-Za-z0-9]+)?`)

func glmModelIDsFromContent(content string) []string {
	matches := glmNamePattern.FindAllString(content, -1)
	seen := map[string]bool{}
	out := make([]string, 0, len(matches))
	for _, match := range matches {
		modelID := strings.ToLower(strings.TrimSpace(match))
		if modelID == "" || seen[modelID] || !strings.HasPrefix(modelID, "glm-4.5") || strings.Contains(modelID, "4.5v") {
			continue
		}
		seen[modelID] = true
		out = append(out, modelID)
	}
	sort.Strings(out)
	return out
}

var xAIPricingRowPattern = regexp.MustCompile(`(?m)^\|\s*(grok-[^|]+?)\s*\|\s*[^|]+\|\s*\$?\s*([0-9]+(?:\.[0-9]+)?)\s*\|\s*\$?\s*([0-9]+(?:\.[0-9]+)?)\s*\|`)

func ParseXAIOfficialTokenPricing(reader io.Reader, sourceURL string) ([]OfficialTokenPricing, error) {
	bodyBytes, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read xAI pricing response: %w", err)
	}
	matches := xAIPricingRowPattern.FindAllStringSubmatch(string(bodyBytes), -1)
	entries := make([]OfficialTokenPricing, 0, len(matches))
	for _, match := range matches {
		input, inputErr := strconv.ParseFloat(match[2], 64)
		output, outputErr := strconv.ParseFloat(match[3], 64)
		if inputErr != nil || outputErr != nil || !isValidOfficialCost(input) || !isValidOfficialCost(output) {
			continue
		}
		entries = append(entries, OfficialTokenPricing{
			Provider:         OfficialPricingProviderXAI,
			Model:            strings.TrimSpace(match[1]),
			SourceURL:        sourceURL,
			InputUSDPerMTok:  input,
			OutputUSDPerMTok: output,
		})
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("no valid xAI pricing entries found")
	}
	return entries, nil
}

func ParseDeepSeekOfficialTokenPricing(reader io.Reader, sourceURL string) ([]OfficialTokenPricing, error) {
	bodyBytes, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read DeepSeek pricing response: %w", err)
	}
	content := stripHTML(string(bodyBytes))
	modelIDs := deepSeekPrimaryModelIDs(content)
	cacheHitPrices, cacheHitOK := parseDeepSeekPriceRow(content, "INPUT TOKENS (CACHE HIT)")
	cacheMissPrices, cacheMissOK := parseDeepSeekPriceRow(content, "INPUT TOKENS (CACHE MISS)")
	outputPrices, outputOK := parseDeepSeekPriceRow(content, "OUTPUT TOKENS")
	if len(modelIDs) == 0 || !cacheHitOK || !cacheMissOK || !outputOK || len(cacheMissPrices) < len(modelIDs) || len(outputPrices) < len(modelIDs) || len(cacheHitPrices) < len(modelIDs) {
		return nil, fmt.Errorf("no valid DeepSeek pricing rows found")
	}
	entries := make([]OfficialTokenPricing, 0, len(modelIDs)+2)
	for idx, modelID := range modelIDs {
		input := cacheMissPrices[idx]
		output := outputPrices[idx]
		cacheRead := cacheHitPrices[idx]
		if !isValidOfficialCost(input) || !isValidOfficialCost(output) || !isValidOfficialCost(cacheRead) {
			continue
		}
		var cacheReadRatio *float64
		var cacheReadPrice *float64
		if input > 0 {
			ratio := cacheRead / input
			cacheReadRatio = &ratio
			cacheReadPrice = &cacheRead
		}
		for _, name := range append([]string{modelID}, deepSeekModelAliases(modelID)...) {
			entries = append(entries, OfficialTokenPricing{
				Provider:            OfficialPricingProviderDeepSeek,
				Model:               name,
				SourceURL:           sourceURL,
				InputUSDPerMTok:     input,
				OutputUSDPerMTok:    output,
				CacheReadUSDPerMTok: cacheReadPrice,
				CacheReadRatio:      cacheReadRatio,
			})
		}
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("no valid DeepSeek pricing entries found")
	}
	return entries, nil
}

var deepSeekNamePattern = regexp.MustCompile(`deepseek-[a-z0-9]+(?:-[a-z0-9]+)*`)

func deepSeekPrimaryModelIDs(content string) []string {
	normalized := strings.Join(strings.Fields(content), " ")
	upper := strings.ToUpper(normalized)
	start := strings.Index(upper, "MODEL ")
	end := strings.Index(upper, " BASE URL")
	if start >= 0 && end > start {
		return uniqueDeepSeekModelIDs(normalized[start:end])
	}
	return uniqueDeepSeekModelIDs(normalized)
}

func uniqueDeepSeekModelIDs(content string) []string {
	matches := deepSeekNamePattern.FindAllString(strings.ToLower(content), -1)
	seen := map[string]bool{}
	out := make([]string, 0, len(matches))
	for _, match := range matches {
		if seen[match] || match == "deepseek-chat" || match == "deepseek-reasoner" {
			continue
		}
		seen[match] = true
		out = append(out, match)
	}
	return out
}

func parseDeepSeekPriceRow(content string, label string) ([]float64, bool) {
	normalized := strings.Join(strings.Fields(content), " ")
	pattern := regexp.MustCompile(`(?i)(?:1M\s+)?` + regexp.QuoteMeta(label) + `\s+\$?([0-9]+(?:\.[0-9]+)?)\s+\$?([0-9]+(?:\.[0-9]+)?)`)
	match := pattern.FindStringSubmatch(normalized)
	if len(match) < 3 {
		return nil, false
	}
	values := make([]float64, 0, 2)
	for _, raw := range match[1:] {
		value, err := strconv.ParseFloat(raw, 64)
		if err != nil || !isValidOfficialCost(value) {
			return nil, false
		}
		values = append(values, value)
	}
	return values, true
}

func deepSeekModelAliases(modelID string) []string {
	if modelID == "deepseek-v4-flash" {
		return []string{"deepseek-chat", "deepseek-reasoner"}
	}
	return nil
}

var htmlTagPattern = regexp.MustCompile(`<[^>]+>`)

func stripHTML(content string) string {
	withoutTags := htmlTagPattern.ReplaceAllString(content, " ")
	withoutTags = strings.ReplaceAll(withoutTags, "&nbsp;", " ")
	withoutTags = strings.ReplaceAll(withoutTags, "&amp;", "&")
	return withoutTags
}

func isValidOfficialCost(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= 0
}

func roundOfficialRatioPointer(value *float64) *float64 {
	if value == nil {
		return nil
	}
	rounded := roundOfficialRatioValue(*value)
	return &rounded
}

func roundOfficialRatioValue(value float64) float64 {
	if value == 0 {
		return 0
	}
	rounded, err := strconv.ParseFloat(strconv.FormatFloat(value, 'f', 8, 64), 64)
	if err != nil {
		return value
	}
	return rounded
}
