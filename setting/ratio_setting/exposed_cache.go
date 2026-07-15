package ratio_setting

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
)

const exposedDataTTL = 30 * time.Second

type exposedCache struct {
	data      gin.H
	expiresAt time.Time
}

var (
	exposedData atomic.Value
	rebuildMu   sync.Mutex
)

func InvalidateExposedDataCache() {
	exposedData.Store((*exposedCache)(nil))
}

func cloneGinH(src gin.H) gin.H {
	dst := make(gin.H, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func applyMetadataPricingToExposedData(data gin.H) {
	modelRatio, _ := data["model_ratio"].(map[string]float64)
	completionRatio, _ := data["completion_ratio"].(map[string]float64)
	cacheRatio, _ := data["cache_ratio"].(map[string]float64)
	createCacheRatio, _ := data["create_cache_ratio"].(map[string]float64)
	modelPrice, _ := data["model_price"].(map[string]float64)

	for model, price := range GetMetadataModelPriceCopy() {
		modelPrice[model] = price
		delete(modelRatio, model)
		delete(completionRatio, model)
		delete(cacheRatio, model)
		delete(createCacheRatio, model)
	}
	for model, ratio := range GetMetadataModelRatioCopy() {
		modelRatio[model] = ratio
		delete(modelPrice, model)
	}
	for model, ratio := range GetMetadataCompletionRatioCopy() {
		completionRatio[model] = ratio
	}
	for model, ratio := range GetMetadataCacheRatioCopy() {
		cacheRatio[model] = ratio
	}
	for model, ratio := range GetMetadataCreateCacheRatioCopy() {
		createCacheRatio[model] = ratio
	}
}

func applyOfficialPricingToExposedData(data gin.H) {
	modelRatio, _ := data["model_ratio"].(map[string]float64)
	completionRatio, _ := data["completion_ratio"].(map[string]float64)
	cacheRatio, _ := data["cache_ratio"].(map[string]float64)
	createCacheRatio, _ := data["create_cache_ratio"].(map[string]float64)
	modelPrice, _ := data["model_price"].(map[string]float64)

	for model, ratio := range GetOfficialModelRatioCopy() {
		modelRatio[model] = ratio
		delete(modelPrice, model)
	}
	for model, ratio := range GetOfficialCompletionRatioCopy() {
		completionRatio[model] = ratio
	}
	for model, ratio := range GetOfficialCacheRatioCopy() {
		cacheRatio[model] = ratio
	}
	for model, ratio := range GetOfficialCreateCacheRatioCopy() {
		createCacheRatio[model] = ratio
	}
}

func GetExposedData() gin.H {
	if c, ok := exposedData.Load().(*exposedCache); ok && c != nil && time.Now().Before(c.expiresAt) {
		return cloneGinH(c.data)
	}
	rebuildMu.Lock()
	defer rebuildMu.Unlock()
	if c, ok := exposedData.Load().(*exposedCache); ok && c != nil && time.Now().Before(c.expiresAt) {
		return cloneGinH(c.data)
	}
	newData := gin.H{
		"model_ratio":        GetModelRatioCopy(),
		"completion_ratio":   GetCompletionRatioCopy(),
		"cache_ratio":        GetCacheRatioCopy(),
		"create_cache_ratio": GetCreateCacheRatioCopy(),
		"model_price":        GetModelPriceCopy(),
	}
	applyOfficialPricingToExposedData(newData)
	applyMetadataPricingToExposedData(newData)
	exposedData.Store(&exposedCache{
		data:      newData,
		expiresAt: time.Now().Add(exposedDataTTL),
	})
	return cloneGinH(newData)
}
