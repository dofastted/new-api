package controller

import (
	"net/http"

	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

// SyncUpstreamModels syncs model metadata from official provider pricing sources.
// It intentionally does not read third-party model catalogs: official pricing is
// the shared source for model IDs, vendors, and billing standards.
func SyncUpstreamModels(c *gin.Context) {
	result, err := service.SyncOfficialModelMetadata(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

// SyncUpstreamPreview exposes the official-source model set to the existing
// sync wizard. Official metadata is authoritative, so provider/model conflicts
// are not surfaced for manual third-party-style reconciliation.
func SyncUpstreamPreview(c *gin.Context) {
	result, err := service.PreviewOfficialModelMetadata(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}
