package service

import (
	"errors"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/types"
)

func RewriteUserFacingError(err *types.NewAPIError) *types.NewAPIError {
	if err == nil || !operation_setting.ErrorRewriteEnabled {
		return err
	}
	if err.GetErrorCode() == types.ErrorCodeCountTokenFailed || types.IsClientCanceledError(err) {
		return err
	}

	message := strings.ToLower(err.ErrorWithStatusCode())
	for _, rule := range operation_setting.ErrorRewriteRules {
		if !rule.Enabled || !operation_setting.ErrorRewriteRuleMatchesStatusCode(rule, err.StatusCode) {
			continue
		}
		if len(rule.Keywords) > 0 {
			matched, _ := AcSearch(message, rule.Keywords, true)
			if !matched {
				continue
			}
		}
		statusCode := rule.StatusCode
		if statusCode == 0 {
			statusCode = err.StatusCode
		}
		if statusCode == 0 {
			statusCode = http.StatusInternalServerError
		}
		errorType, errorCode := userFacingErrorIdentity(statusCode, err)
		return types.WithOpenAIError(types.OpenAIError{
			Message: rule.Message,
			Type:    errorType,
			Code:    errorCode,
		}, statusCode)
	}
	return err
}

func userFacingErrorIdentity(statusCode int, original *types.NewAPIError) (string, string) {
	switch {
	case statusCode == http.StatusTooManyRequests:
		return "rate_limit_error", "rate_limit_exceeded"
	case statusCode >= http.StatusInternalServerError && statusCode <= 599:
		return "server_error", "server_error"
	case statusCode == http.StatusForbidden:
		return "upstream_error", preservedErrorCode(original, "access_forbidden")
	case statusCode == http.StatusBadRequest:
		return "invalid_request_error", preservedErrorCode(original, string(types.ErrorCodeInvalidRequest))
	default:
		return "upstream_error", preservedErrorCode(original, string(types.ErrorCodeBadResponseStatusCode))
	}
}

func preservedErrorCode(original *types.NewAPIError, fallback string) string {
	if original == nil || original.GetErrorCode() == "" {
		return fallback
	}
	return string(original.GetErrorCode())
}

func RewriteUserFacingTaskError(statusCode int, code string, message string, localError bool) (int, string, string) {
	if localError {
		return statusCode, code, message
	}
	rewritten := RewriteUserFacingError(types.NewOpenAIError(
		errors.New(message),
		types.ErrorCode(code),
		statusCode,
	))
	if rewritten == nil {
		return statusCode, code, message
	}
	return rewritten.StatusCode, string(rewritten.GetErrorCode()), rewritten.Error()
}
