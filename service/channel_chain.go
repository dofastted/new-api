package service

import (
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

const (
	ChannelChainReasonSelected      = "selected"
	ChannelChainReasonAffinityReuse = "affinity_reuse"
	ChannelChainReasonRetrySelected = "retry_selected"
	ChannelChainReasonFailure       = "failure"
	ChannelChainSelectionWeighted   = "weighted"
	ChannelChainSelectionAffinity   = "affinity"
	ChannelChainSelectionSpecific   = "specific_channel"
	ChannelChainCircuitStateClosed  = "closed"
)

func AppendChannelChainEntry(c *gin.Context, entry types.ChannelChainEntry) {
	if c == nil {
		return
	}
	entry.ChannelName = sanitizeChannelChainText(entry.ChannelName)
	entry.Endpoint = sanitizeChannelChainEndpoint(entry.Endpoint)
	entry.ErrorCode = sanitizeChannelChainText(entry.ErrorCode)
	entry.ErrorCategory = sanitizeChannelChainText(entry.ErrorCategory)
	chain := GetChannelChain(c)
	chain = append(chain, entry)
	common.SetContextKey(c, constant.ContextKeyChannelChain, chain)
}

func GetChannelChain(c *gin.Context) []types.ChannelChainEntry {
	if c == nil {
		return nil
	}
	if chain, ok := common.GetContextKeyType[[]types.ChannelChainEntry](c, constant.ContextKeyChannelChain); ok {
		return chain
	}
	return nil
}

func AppendChannelSelectionTrace(c *gin.Context, channel *model.Channel, group string, retry int, reason string, selection string) {
	if channel == nil {
		return
	}
	AppendChannelChainEntry(c, types.ChannelChainEntry{
		ChannelId:    channel.Id,
		ChannelName:  channel.Name,
		ChannelType:  channel.Type,
		Group:        group,
		Reason:       reason,
		Selection:    selection,
		Attempt:      retry + 1,
		RetryIndex:   retry,
		CircuitState: ChannelChainCircuitStateClosed,
		Decision: types.ChannelDecisionContext{
			Priority: channel.GetPriority(),
			Weight:   channel.GetWeight(),
		},
	})
}

func AppendChannelFailureTrace(c *gin.Context, channelId int, channelType int, channelName string, err *types.NewAPIError) {
	entry := types.ChannelChainEntry{
		ChannelId:    channelId,
		ChannelName:  channelName,
		ChannelType:  channelType,
		Reason:       ChannelChainReasonFailure,
		Selection:    "relay_failure",
		Group:        common.GetContextKeyString(c, constant.ContextKeyUsingGroup),
		RetryIndex:   c.GetInt("retry_index"),
		CircuitState: ChannelChainCircuitStateClosed,
	}
	if err != nil {
		entry.ErrorCode = string(err.GetErrorCode())
		entry.ErrorCategory = string(err.GetErrorType())
	}
	AppendChannelChainEntry(c, entry)
}

func ChannelChainForLog(c *gin.Context) []types.ChannelChainEntry {
	chain := GetChannelChain(c)
	if len(chain) == 0 {
		return nil
	}
	out := make([]types.ChannelChainEntry, len(chain))
	copy(out, chain)
	return out
}

func sanitizeChannelChainText(value string) string {
	value = strings.ReplaceAll(value, "\n", " ")
	value = strings.ReplaceAll(value, "\r", " ")
	return strings.TrimSpace(value)
}

func sanitizeChannelChainEndpoint(value string) string {
	value = sanitizeChannelChainText(value)
	if value == "" {
		return ""
	}
	if idx := strings.Index(value, "?"); idx != -1 {
		value = value[:idx]
	}
	return value
}
