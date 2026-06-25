package service

import (
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/types"
)

func GetChannelCircuitStatus(channelID int) model.ChannelCircuitStatus {
	return model.GetChannelCircuitStatus(channelID)
}

func IsChannelCircuitOpen(channelID int) bool {
	return model.IsChannelCircuitOpen(channelID)
}

func RecordChannelSuccess(channelID int) {
	model.RecordChannelCircuitSuccess(channelID)
}

func RecordChannelFailure(channelID int, category string) {
	model.RecordChannelCircuitFailure(channelID, category)
}

func ResetChannelCircuit(channelID int) {
	model.ResetChannelCircuit(channelID)
}

func ShouldRecordChannelCircuitFailure(err *types.NewAPIError) bool {
	if err == nil || types.IsClientCanceledError(err) || types.IsSkipRetryError(err) {
		return false
	}
	code := err.StatusCode
	if code >= 400 && code < 500 && !types.IsChannelError(err) {
		return false
	}
	return true
}
