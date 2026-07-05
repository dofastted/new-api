package service

import (
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/types"
)

const ChannelCircuitClassHighLoadTemporarilyUnavailable = "high_load_temporarily_unavailable"

type ChannelCircuitFailureDecision struct {
	ShouldRecord bool
	Category     string
	Policy       model.ChannelCircuitPolicy
}

func GetChannelCircuitStatus(channelID int) model.ChannelCircuitStatus {
	return model.GetChannelCircuitStatus(channelID)
}

func IsChannelCircuitOpen(channelID int) bool {
	return model.IsChannelCircuitOpen(channelID)
}

func RecordChannelSuccess(channelID int) model.ChannelCircuitStatus {
	return model.RecordChannelCircuitSuccess(channelID)
}

func RecordChannelFailure(channelID int, category string, policies ...model.ChannelCircuitPolicy) model.ChannelCircuitStatus {
	return model.RecordChannelCircuitFailure(channelID, category, policies...)
}

func ResetChannelCircuit(channelID int) model.ChannelCircuitStatus {
	return model.ResetChannelCircuit(channelID)
}

func ClassifyChannelCircuitFailure(channelSetting dto.ChannelSettings, err *types.NewAPIError) ChannelCircuitFailureDecision {
	if !ShouldRecordChannelCircuitFailure(err) {
		return ChannelCircuitFailureDecision{}
	}
	if channelSetting.CircuitBreaker == nil {
		return ChannelCircuitFailureDecision{
			ShouldRecord: true,
			Category:     string(err.GetErrorCode()),
			Policy:       model.ChannelCircuitPolicy{},
		}
	}
	if !channelSetting.CircuitBreaker.Enabled {
		return ChannelCircuitFailureDecision{}
	}
	for _, rule := range channelSetting.CircuitBreaker.Rules {
		if !channelCircuitRuleMatches(rule, err) {
			continue
		}
		category := strings.TrimSpace(rule.Class)
		if category == "" {
			category = strings.TrimSpace(rule.Name)
		}
		if category == "" {
			category = string(err.GetErrorCode())
		}
		return ChannelCircuitFailureDecision{
			ShouldRecord: true,
			Category:     category,
			Policy: model.ChannelCircuitPolicy{
				Name:                     strings.TrimSpace(rule.Name),
				FailureThreshold:         channelSetting.CircuitBreaker.FailureThreshold,
				OpenSeconds:              channelSetting.CircuitBreaker.OpenSeconds,
				HalfOpenSuccessThreshold: channelSetting.CircuitBreaker.HalfOpenSuccessThreshold,
			},
		}
	}
	return ChannelCircuitFailureDecision{}
}

func ShouldRecordChannelCircuitFailure(err *types.NewAPIError) bool {
	if err == nil || types.IsClientCanceledError(err) || types.IsSkipRetryError(err) {
		return false
	}
	if IsProviderFamilyAccessDeniedError(err) {
		return false
	}
	code := err.StatusCode
	if code == http.StatusTooManyRequests {
		return false
	}
	if code >= 400 && code < 500 && !types.IsChannelError(err) {
		return false
	}
	return true
}

func channelCircuitRuleMatches(rule dto.ChannelCircuitBreakerRule, err *types.NewAPIError) bool {
	matchedSelector := false
	if len(rule.StatusCodes) > 0 {
		matchedSelector = true
		if !containsStatusCode(rule.StatusCodes, err.StatusCode) {
			return false
		}
	}
	if len(rule.ErrorCodes) > 0 {
		matchedSelector = true
		if !containsErrorCode(rule.ErrorCodes, string(err.GetErrorCode())) {
			return false
		}
	}
	if len(rule.MessageContains) > 0 {
		matchedSelector = true
		if !containsMessageKeyword(rule.MessageContains, err.Error()) {
			return false
		}
	}
	return matchedSelector
}

func containsStatusCode(statusCodes []int, statusCode int) bool {
	for _, candidate := range statusCodes {
		if candidate == statusCode {
			return true
		}
	}
	return false
}

func containsErrorCode(errorCodes []string, errorCode string) bool {
	for _, candidate := range errorCodes {
		if strings.EqualFold(strings.TrimSpace(candidate), errorCode) {
			return true
		}
	}
	return false
}

func containsMessageKeyword(keywords []string, message string) bool {
	lowerMessage := strings.ToLower(message)
	for _, keyword := range keywords {
		keyword = strings.TrimSpace(keyword)
		if keyword != "" && strings.Contains(lowerMessage, strings.ToLower(keyword)) {
			return true
		}
	}
	return false
}
