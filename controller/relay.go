package controller

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	perfmetrics "github.com/QuantumNous/new-api/pkg/perf_metrics"
	"github.com/QuantumNous/new-api/relay"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/abuse_guard_setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/types"

	"github.com/bytedance/gopkg/util/gopool"
	"github.com/samber/lo"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

func relayHandler(c *gin.Context, info *relaycommon.RelayInfo) *types.NewAPIError {
	var err *types.NewAPIError
	switch info.RelayMode {
	case relayconstant.RelayModeImagesGenerations, relayconstant.RelayModeImagesEdits:
		err = relay.ImageHelper(c, info)
	case relayconstant.RelayModeAudioSpeech:
		fallthrough
	case relayconstant.RelayModeAudioTranslation:
		fallthrough
	case relayconstant.RelayModeAudioTranscription:
		err = relay.AudioHelper(c, info)
	case relayconstant.RelayModeRerank:
		err = relay.RerankHelper(c, info)
	case relayconstant.RelayModeEmbeddings:
		err = relay.EmbeddingHelper(c, info)
	case relayconstant.RelayModeResponses, relayconstant.RelayModeResponsesCompact:
		err = relay.ResponsesHelper(c, info)
	default:
		err = relay.TextHelper(c, info)
	}
	return err
}

func geminiRelayHandler(c *gin.Context, info *relaycommon.RelayInfo) *types.NewAPIError {
	var err *types.NewAPIError
	if strings.Contains(c.Request.URL.Path, "embed") {
		err = relay.GeminiEmbeddingHandler(c, info)
	} else {
		err = relay.GeminiHelper(c, info)
	}
	return err
}

func relayOnce(c *gin.Context, info *relaycommon.RelayInfo, relayFormat types.RelayFormat, ws *websocket.Conn) *types.NewAPIError {
	switch relayFormat {
	case types.RelayFormatOpenAIRealtime:
		return relay.WssHelper(c, info)
	case types.RelayFormatClaude:
		return relay.ClaudeHelper(c, info)
	case types.RelayFormatGemini:
		return geminiRelayHandler(c, info)
	default:
		return relayHandler(c, info)
	}
}

func Relay(c *gin.Context, relayFormat types.RelayFormat) {

	requestId := c.GetString(common.RequestIdKey)
	//group := common.GetContextKeyString(c, constant.ContextKeyUsingGroup)
	//originalModel := common.GetContextKeyString(c, constant.ContextKeyOriginalModel)

	var (
		newAPIError *types.NewAPIError
		ws          *websocket.Conn
	)

	if relayFormat == types.RelayFormatOpenAIRealtime {
		var err error
		ws, err = upgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			helper.WssError(c, ws, types.NewError(err, types.ErrorCodeGetChannelFailed, types.ErrOptionWithSkipRetry()).ToOpenAIError())
			return
		}
		defer ws.Close()
	}

	defer func() {
		if newAPIError != nil {
			logger.LogError(c, fmt.Sprintf("relay error: %s", common.LocalLogPreview(newAPIError.Error())))
			newAPIError = service.RewriteUserFacingError(newAPIError)
			newAPIError.SetMessage(common.MessageWithRequestId(newAPIError.Error(), requestId))
			switch relayFormat {
			case types.RelayFormatOpenAIRealtime:
				helper.WssError(c, ws, newAPIError.ToOpenAIError())
			case types.RelayFormatClaude:
				c.JSON(newAPIError.StatusCode, gin.H{
					"type":  "error",
					"error": newAPIError.ToClaudeError(),
				})
			default:
				c.JSON(newAPIError.StatusCode, gin.H{
					"error": newAPIError.ToOpenAIError(),
				})
			}
		}
	}()

	request, err := helper.GetAndValidateRequest(c, relayFormat)
	if err != nil {
		// Map "request body too large" to 413 so clients can handle it correctly
		if common.IsRequestBodyTooLargeError(err) || errors.Is(err, common.ErrRequestBodyTooLarge) {
			newAPIError = types.NewErrorWithStatusCode(err, types.ErrorCodeReadRequestBodyFailed, http.StatusRequestEntityTooLarge, types.ErrOptionWithSkipRetry())
		} else {
			newAPIError = types.NewError(err, types.ErrorCodeInvalidRequest)
		}
		return
	}

	relayInfo, err := relaycommon.GenRelayInfo(c, relayFormat, request, ws)
	if err != nil {
		newAPIError = types.NewError(err, types.ErrorCodeGenRelayInfoFailed)
		return
	}

	needSensitiveCheck := setting.ShouldCheckPromptSensitive()
	needAbuseGuard := abuse_guard_setting.GetAbuseGuardSetting().Enabled
	needCountToken := constant.CountToken
	// Avoid building huge CombineText (strings.Join) when token counting and content checks are all disabled.
	var meta *types.TokenCountMeta
	if needSensitiveCheck || needCountToken || needAbuseGuard {
		meta = request.GetTokenCountMeta()
	} else {
		meta = fastTokenCountMetaForPricing(request)
	}

	if needSensitiveCheck && meta != nil {
		contains, words := service.CheckSensitiveText(meta.CombineText)
		if contains {
			logger.LogWarn(c, fmt.Sprintf("user sensitive words detected: %s", strings.Join(words, ", ")))
			newAPIError = types.NewError(err, types.ErrorCodeSensitiveWordsDetected)
			return
		}
	}

	if needAbuseGuard && meta != nil {
		result := service.AbuseGuardCheck(service.AbuseGuardInput{
			UserId:      relayInfo.UserId,
			UserRole:    c.GetInt("role"),
			Group:       relayInfo.UsingGroup,
			ModelName:   relayInfo.OriginModelName,
			CombineText: meta.CombineText,
			RequestId:   requestId,
			Ip:          c.ClientIP(),
		})
		switch result.Decision {
		case service.AbuseBlock:
			logger.LogWarn(c, fmt.Sprintf("abuse guard blocked request for user %d model %s", relayInfo.UserId, relayInfo.OriginModelName))
			newAPIError = types.NewErrorWithStatusCode(errors.New(result.Message), types.ErrorCodeAbuseContentBlocked, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
			return
		case service.AbuseTempBanned:
			newAPIError = types.NewErrorWithStatusCode(errors.New(result.Message), types.ErrorCodeAbuseTempBanned, http.StatusForbidden, types.ErrOptionWithSkipRetry())
			return
		}
	}

	tokens, err := service.EstimateRequestToken(c, meta, relayInfo)
	if err != nil {
		newAPIError = types.NewError(err, types.ErrorCodeCountTokenFailed)
		return
	}

	relayInfo.SetEstimatePromptTokens(tokens)

	priceData, err := helper.ModelPriceHelper(c, relayInfo, tokens, meta)
	if err != nil {
		newAPIError = types.NewError(err, types.ErrorCodeModelPriceError, types.ErrOptionWithStatusCode(http.StatusBadRequest))
		return
	}

	// common.SetContextKey(c, constant.ContextKeyTokenCountMeta, meta)

	if priceData.FreeModel {
		logger.LogInfo(c, fmt.Sprintf("模型 %s 免费，跳过预扣费", relayInfo.OriginModelName))
	} else {
		newAPIError = service.PreConsumeBilling(c, priceData.QuotaToPreConsume, relayInfo)
		if newAPIError != nil {
			return
		}
	}

	defer func() {
		// Only return quota if downstream failed and quota was actually pre-consumed
		if newAPIError != nil {
			newAPIError = service.NormalizeViolationFeeError(newAPIError)
			if relayInfo.Billing != nil {
				relayInfo.Billing.Refund(c)
			}
			service.ChargeViolationFeeIfNeeded(c, relayInfo, newAPIError)
		}
	}()

	retryParam := &service.RetryParam{
		Ctx:           c,
		TokenGroup:    relayInfo.TokenGroup,
		ModelName:     relayInfo.OriginModelName,
		RequestPath:   c.Request.URL.Path,
		Retry:         common.GetPointer(0),
		AllowedGroups: relayInfo.SubscriptionProviderGroups,
	}
	relayInfo.RetryIndex = 0
	relayInfo.LastError = nil

	exhaustedRetryableChannels := false
	for {
		if retryParam.GetRetry() > common.RetryTimes {
			break
		}
		relayInfo.RetryIndex = retryParam.GetRetry()
		c.Set("retry_index", retryParam.GetRetry())
		channel, channelErr := getChannelWithRateLimit(c, relayInfo, retryParam)
		if channelErr != nil {
			logger.LogError(c, channelErr.Error())
			newAPIError = channelErr
			break
		}

		addUsedChannel(c, channel.Id)
		bodyStorage, bodyErr := common.GetBodyStorage(c)
		if bodyErr != nil {
			// Ensure consistent 413 for oversized bodies even when error occurs later (e.g., retry path)
			if common.IsRequestBodyTooLargeError(bodyErr) || errors.Is(bodyErr, common.ErrRequestBodyTooLarge) {
				newAPIError = types.NewErrorWithStatusCode(bodyErr, types.ErrorCodeReadRequestBodyFailed, http.StatusRequestEntityTooLarge, types.ErrOptionWithSkipRetry())
			} else {
				newAPIError = types.NewErrorWithStatusCode(bodyErr, types.ErrorCodeReadRequestBodyFailed, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
			}
			break
		}
		t429Retry := 0
		triedKeyIndexes := make(map[int]struct{})
		retriedAfter429WithinChannel := false
		for {
			if _, seekErr := bodyStorage.Seek(0, io.SeekStart); seekErr != nil {
				newAPIError = types.NewErrorWithStatusCode(seekErr, types.ErrorCodeReadRequestBodyFailed, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
				break
			}
			c.Request.Body = io.NopCloser(bodyStorage)

			newAPIError = relayOnce(c, relayInfo, relayFormat, ws)
			if newAPIError == nil {
				relayInfo.LastError = nil
				service.RecordChannelSuccess(channel.Id)
				return
			}

			newAPIError = service.NormalizeViolationFeeError(newAPIError)
			relayInfo.LastError = newAPIError
			if types.IsClientCanceledError(newAPIError) {
				logger.LogInfo(c, fmt.Sprintf("client canceled request, skip channel retry: %s", common.LocalLogPreview(newAPIError.Error())))
				break
			}

			processChannelError(c, *types.NewChannelError(channel.Id, channel.Type, channel.Name, channel.ChannelInfo.IsMultiKey, common.GetContextKeyString(c, constant.ContextKeyChannelKey), channel.GetAutoBan()), newAPIError)
			if !isTooManyRequestsError(newAPIError) {
				break
			}
			markChannelRateLimited(channel.Id)
			if !prepareTooManyRequestsRetry(c, channel, relayInfo.OriginModelName, triedKeyIndexes, t429Retry) {
				break
			}
			retriedAfter429WithinChannel = true
			t429Retry++
		}

		if len(retryParam.AllowedGroups) == 0 && len(relayInfo.SubscriptionProviderGroups) > 0 {
			retryParam.AllowedGroups = append([]string(nil), relayInfo.SubscriptionProviderGroups...)
		}
		if retriedAfter429WithinChannel {
			if shouldContinueAfterUpstreamRateLimit(c, newAPIError) {
				continue
			}
			break
		}
		if shouldContinueAfterUpstreamRateLimit(c, newAPIError) {
			continue
		}
		remainingRetries := common.RetryTimes - retryParam.GetRetry()
		retryableErr := shouldRetry(c, newAPIError, remainingRetries)
		if !retryableErr {
			if remainingRetries <= 0 && isRetryableChannelFailure(newAPIError) {
				exhaustedRetryableChannels = true
			}
			break
		}
		service.PrepareRetryAfterChannelFailure(retryParam, channel)
		retryParam.IncreaseRetry()
	}

	if exhaustedRetryableChannels {
		newAPIError = types.NewErrorWithStatusCode(
			fmt.Errorf("无可用渠道，请联系管理员"),
			types.ErrorCodeGetChannelFailed,
			http.StatusServiceUnavailable,
			types.ErrOptionWithSkipRetry(),
		)
	}

	useChannel := c.GetStringSlice("use_channel")
	if len(useChannel) > 1 {
		retryLogStr := fmt.Sprintf("重试：%s", strings.Trim(strings.Join(strings.Fields(fmt.Sprint(useChannel)), "->"), "[]"))
		logger.LogInfo(c, retryLogStr)
	}
	if newAPIError != nil {
		gopool.Go(func() {
			perfmetrics.RecordRelaySample(relayInfo, false, 0)
		})
	}
}

var upgrader = websocket.Upgrader{
	Subprotocols: []string{"realtime"}, // WS 握手支持的协议，如果有使用 Sec-WebSocket-Protocol，则必须在此声明对应的 Protocol TODO add other protocol
	CheckOrigin: func(r *http.Request) bool {
		return true // 允许跨域
	},
}

var tooManyRequestsRetrySleep = time.Sleep
var relayRateWaitForSlot = service.WaitForSlot

func isTooManyRequestsError(err *types.NewAPIError) bool {
	return err != nil && err.StatusCode == http.StatusTooManyRequests
}

func tooManyRequestsRetryDelay(retry int) time.Duration {
	delay := time.Second
	for range retry {
		if delay >= 8*time.Second {
			return 8 * time.Second
		}
		delay *= 2
	}
	return delay
}

func prepareTooManyRequestsRetry(c *gin.Context, channel *model.Channel, modelName string, triedKeyIndexes map[int]struct{}, retry int) bool {
	if channel == nil || !channel.ChannelInfo.IsMultiKey {
		return false
	}
	currentIndex := common.GetContextKeyInt(c, constant.ContextKeyChannelMultiKeyIndex)
	triedKeyIndexes[currentIndex] = struct{}{}
	if len(triedKeyIndexes) >= len(channel.GetKeys()) {
		return false
	}
	if setupErr := middleware.SetupContextForSelectedChannelExceptKeys(c, channel, modelName, triedKeyIndexes); setupErr != nil {
		logger.LogWarn(c, fmt.Sprintf("429 retry stayed on channel #%d but no alternate key is available: %s", channel.Id, common.LocalLogPreview(setupErr.Error())))
		return false
	}
	tooManyRequestsRetrySleep(tooManyRequestsRetryDelay(retry))
	return true
}

func addUsedChannel(c *gin.Context, channelId int) {
	useChannel := c.GetStringSlice("use_channel")
	useChannel = append(useChannel, fmt.Sprintf("%d", channelId))
	c.Set("use_channel", useChannel)
}

func fastTokenCountMetaForPricing(request dto.Request) *types.TokenCountMeta {
	if request == nil {
		return &types.TokenCountMeta{}
	}
	meta := &types.TokenCountMeta{
		TokenType: types.TokenTypeTokenizer,
	}
	switch r := request.(type) {
	case *dto.GeneralOpenAIRequest:
		maxCompletionTokens := lo.FromPtrOr(r.MaxCompletionTokens, uint(0))
		maxTokens := lo.FromPtrOr(r.MaxTokens, uint(0))
		if maxCompletionTokens > maxTokens {
			meta.MaxTokens = int(maxCompletionTokens)
		} else {
			meta.MaxTokens = int(maxTokens)
		}
	case *dto.OpenAIResponsesRequest:
		meta.MaxTokens = int(lo.FromPtrOr(r.MaxOutputTokens, uint(0)))
	case *dto.ClaudeRequest:
		meta.MaxTokens = int(lo.FromPtr(r.MaxTokens))
	case *dto.ImageRequest:
		// Pricing for image requests depends on ImagePriceRatio; safe to compute even when CountToken is disabled.
		return r.GetTokenCountMeta()
	default:
		// Best-effort: leave CombineText empty to avoid large allocations.
	}
	return meta
}

func getChannel(c *gin.Context, info *relaycommon.RelayInfo, retryParam *service.RetryParam) (*model.Channel, *types.NewAPIError) {
	if info.ChannelMeta == nil {
		autoBan := c.GetBool("auto_ban")
		autoBanInt := 1
		if !autoBan {
			autoBanInt = 0
		}
		return &model.Channel{
			Id:      c.GetInt("channel_id"),
			Type:    c.GetInt("channel_type"),
			Name:    c.GetString("channel_name"),
			AutoBan: &autoBanInt,
		}, nil
	}
	channel, selectGroup, err := service.CacheGetRandomSatisfiedChannel(retryParam)
	if channel != nil {
		service.AppendChannelSelectionTrace(c, channel, selectGroup, retryParam.GetRetry(), service.ChannelChainReasonRetrySelected, service.ChannelChainSelectionWeighted)
	}

	info.PriceData.GroupRatioInfo = helper.HandleGroupRatio(c, info)

	if err != nil {
		return nil, types.NewError(fmt.Errorf("获取分组 %s 下模型 %s 的可用渠道失败（retry）: %w", selectGroup, info.OriginModelName, err), types.ErrorCodeGetChannelFailed, types.ErrOptionWithSkipRetry())
	}
	if channel == nil {
		return nil, types.NewError(fmt.Errorf("分组 %s 下模型 %s 的可用渠道不存在（retry）", selectGroup, info.OriginModelName), types.ErrorCodeGetChannelFailed, types.ErrOptionWithSkipRetry())
	}

	newAPIError := middleware.SetupContextForSelectedChannel(c, channel, info.OriginModelName)
	if newAPIError != nil {
		return nil, newAPIError
	}
	return channel, nil
}

func getChannelWithRateLimit(c *gin.Context, info *relaycommon.RelayInfo, retryParam *service.RetryParam) (*model.Channel, *types.NewAPIError) {
	return selectRelayChannelWithRateLimit(c, retryParam, func() (*model.Channel, *types.NewAPIError) {
		return getChannel(c, info, retryParam)
	})
}

func selectRelayChannelWithRateLimit(c *gin.Context, retryParam *service.RetryParam, selector func() (*model.Channel, *types.NewAPIError)) (*model.Channel, *types.NewAPIError) {
	channel, limited, channelErr := trySelectRelayChannelWithRateLimit(c, retryParam, selector)
	if channel != nil || !limited {
		return channel, channelErr
	}

	budget := service.RateWaitBudget(c, setting.RateLimitWaitTimeoutSeconds)
	if budget <= 0 {
		setRateLimitRetryAfter(c)
		return nil, channelRateLimitAPIError()
	}

	var selected *model.Channel
	var selectErr *types.NewAPIError
	var stillLimited bool
	waitErr := relayRateWaitForSlot(c.Request.Context(), func() bool {
		retryParam.ResetRateLimitedChannelExclusions()
		selected, stillLimited, selectErr = trySelectRelayChannelWithRateLimit(c, retryParam, selector)
		if selectErr != nil && !stillLimited {
			return true
		}
		return selected != nil && selectErr == nil
	}, budget)
	if waitErr == nil && selected != nil {
		return selected, nil
	}
	if selectErr != nil && !stillLimited {
		return nil, selectErr
	}
	if errors.Is(waitErr, context.Canceled) || errors.Is(waitErr, context.DeadlineExceeded) {
		return nil, types.NewErrorWithStatusCode(waitErr, types.ErrorCodeGetChannelFailed, http.StatusRequestTimeout, types.ErrOptionWithSkipRetry())
	}
	setRateLimitRetryAfter(c)
	return nil, channelRateLimitAPIError()
}

func trySelectRelayChannelWithRateLimit(c *gin.Context, retryParam *service.RetryParam, selector func() (*model.Channel, *types.NewAPIError)) (*model.Channel, bool, *types.NewAPIError) {
	if selector == nil {
		return nil, false, types.NewError(errors.New("channel selector is nil"), types.ErrorCodeGetChannelFailed, types.ErrOptionWithSkipRetry())
	}

	limited := false
	for {
		channel, channelErr := selector()
		if channelErr != nil {
			if limited {
				return nil, true, nil
			}
			return nil, false, channelErr
		}
		if channel == nil {
			return nil, limited, nil
		}
		if tryAcquireSelectedChannelSlot(c, channel) {
			return channel, false, nil
		}
		limited = true
		if !retryParam.ExcludeRateLimitedChannel(channel.Id) {
			return nil, true, nil
		}
		logger.LogDebug(c, "channel #%d is rate limited or cooling down, selecting another channel", channel.Id)
	}
}

func tryAcquireSelectedChannelSlot(c *gin.Context, channel *model.Channel) bool {
	if channel == nil {
		return false
	}
	if service.IsChannelRateLimited(channel.Id) {
		return false
	}
	return service.TryAcquireChannelSlot(channel.Id, selectedChannelRateLimitRPM(c, channel))
}

func selectedChannelRateLimitRPM(c *gin.Context, channel *model.Channel) int {
	if channel == nil {
		return 0
	}
	rpm := channel.GetSetting().RateLimitRPM
	if rpm > 0 {
		return rpm
	}
	if contextChannelId := common.GetContextKeyInt(c, constant.ContextKeyChannelId); contextChannelId == channel.Id {
		if setting, ok := common.GetContextKeyType[dto.ChannelSettings](c, constant.ContextKeyChannelSetting); ok {
			return setting.RateLimitRPM
		}
	}
	return 0
}

func channelCircuitSettingForError(c *gin.Context, channelID int) dto.ChannelSettings {
	if common.GetContextKeyInt(c, constant.ContextKeyChannelId) != channelID {
		return dto.ChannelSettings{}
	}
	setting, _ := common.GetContextKeyType[dto.ChannelSettings](c, constant.ContextKeyChannelSetting)
	return setting
}

func channelRateLimitAPIError() *types.NewAPIError {
	return types.NewErrorWithStatusCode(
		errors.New(service.ChannelRateLimitBusyMessage()),
		types.ErrorCodeGetChannelFailed,
		http.StatusTooManyRequests,
		types.ErrOptionWithSkipRetry(),
	)
}

func setRateLimitRetryAfter(c *gin.Context) {
	if c == nil {
		return
	}
	retryAfter := setting.RateLimitWaitTimeoutSeconds
	if retryAfter <= 0 {
		retryAfter = 60
	}
	c.Header("Retry-After", fmt.Sprintf("%d", retryAfter))
}

func markChannelRateLimited(channelId int) {
	service.MarkChannelRateLimited(channelId, time.Duration(setting.ChannelRateLimitCooldownSeconds)*time.Second)
}

func shouldContinueAfterUpstreamRateLimit(c *gin.Context, err *types.NewAPIError) bool {
	if !isTooManyRequestsError(err) {
		return false
	}
	return canWaitForUpstreamRateLimit(c)
}

func shouldContinueAfterTaskUpstreamRateLimit(c *gin.Context, taskErr *dto.TaskError) bool {
	if taskErr == nil || taskErr.LocalError || !isTooManyRequestsTaskError(taskErr) {
		return false
	}
	return canWaitForUpstreamRateLimit(c)
}

func canWaitForUpstreamRateLimit(c *gin.Context) bool {
	if setting.RateLimitWaitTimeoutSeconds <= 0 || setting.ChannelRateLimitCooldownSeconds <= 0 {
		return false
	}
	if service.RateWaitBudget(c, setting.RateLimitWaitTimeoutSeconds) <= 0 {
		setRateLimitRetryAfter(c)
		return false
	}
	return true
}

func shouldRetry(c *gin.Context, openaiErr *types.NewAPIError, retryTimes int) bool {
	if openaiErr == nil {
		return false
	}
	if types.IsClientCanceledError(openaiErr) {
		return false
	}
	if service.ShouldSkipRetryAfterChannelAffinityFailure(c) {
		return false
	}
	if service.IsProviderFamilyAccessDeniedError(openaiErr) {
		return false
	}
	if isTooManyRequestsError(openaiErr) {
		return false
	}
	if types.IsChannelError(openaiErr) {
		return true
	}
	if types.IsSkipRetryError(openaiErr) {
		return false
	}
	if retryTimes <= 0 {
		return false
	}
	if _, ok := c.Get("specific_channel_id"); ok {
		return false
	}
	code := openaiErr.StatusCode
	if code >= 200 && code < 300 {
		return false
	}
	if code < 100 || code > 599 {
		return true
	}
	if operation_setting.IsAlwaysSkipRetryCode(openaiErr.GetErrorCode()) {
		return false
	}
	return operation_setting.ShouldRetryByStatusCode(code)
}

func isRetryableChannelFailure(openaiErr *types.NewAPIError) bool {
	if openaiErr == nil || types.IsClientCanceledError(openaiErr) || types.IsSkipRetryError(openaiErr) {
		return false
	}
	if service.IsProviderFamilyAccessDeniedError(openaiErr) {
		return false
	}
	if isTooManyRequestsError(openaiErr) {
		return false
	}
	if types.IsChannelError(openaiErr) {
		return true
	}
	code := openaiErr.StatusCode
	if code < 100 || code > 599 {
		return true
	}
	if code >= 200 && code < 300 {
		return false
	}
	if operation_setting.IsAlwaysSkipRetryCode(openaiErr.GetErrorCode()) {
		return false
	}
	return operation_setting.ShouldRetryByStatusCode(code)
}

func processChannelError(c *gin.Context, channelError types.ChannelError, err *types.NewAPIError) {
	logger.LogError(c, fmt.Sprintf("channel error (channel #%d, status code: %d): %s", channelError.ChannelId, err.StatusCode, common.LocalLogPreview(err.Error())))
	circuitDecision := service.ClassifyChannelCircuitFailure(channelCircuitSettingForError(c, channelError.ChannelId), err)
	circuitTrace := service.ChannelCircuitTrace{}
	if circuitDecision.ShouldRecord {
		status := service.RecordChannelFailure(channelError.ChannelId, circuitDecision.Category, circuitDecision.Policy)
		circuitTrace = service.ChannelCircuitTrace{
			Class:             circuitDecision.Category,
			State:             string(status.State),
			OpenUntil:         status.NextAttemptUnix,
			FallbackCandidate: "same_group_retry",
		}
	}
	service.AppendChannelFailureTraceWithCircuit(c, channelError.ChannelId, channelError.ChannelType, channelError.ChannelName, err, circuitTrace)
	// 不要使用context获取渠道信息，异步处理时可能会出现渠道信息不一致的情况
	// do not use context to get channel info, there may be inconsistent channel info when processing asynchronously
	// 429 仍需走 ShouldDisableChannel：默认状态码规则不含 429，纯限流不会禁用，
	// 但配额耗尽类关键词（如 "You exceeded your current quota"）以 429 返回时必须能自动禁用
	if service.ShouldDisableChannel(err) && channelError.AutoBan {
		gopool.Go(func() {
			service.DisableChannel(channelError, err.ErrorWithStatusCode())
		})
	}

	if constant.ErrorLogEnabled && types.IsRecordErrorLog(err) {
		// 保存错误日志到mysql中
		userId := c.GetInt("id")
		tokenName := c.GetString("token_name")
		modelName := c.GetString("original_model")
		tokenId := c.GetInt("token_id")
		userGroup := c.GetString("group")
		channelId := c.GetInt("channel_id")
		other := make(map[string]interface{})
		if c.Request != nil && c.Request.URL != nil {
			other["request_path"] = c.Request.URL.Path
		}
		other["error_type"] = err.GetErrorType()
		other["error_code"] = err.GetErrorCode()
		other["status_code"] = err.StatusCode
		other["channel_id"] = channelId
		other["channel_name"] = c.GetString("channel_name")
		other["channel_type"] = c.GetInt("channel_type")
		if requestFormat := common.GetContextKeyString(c, constant.ContextKeyRequestFormat); requestFormat != "" {
			other["request_format"] = requestFormat
		}
		if channelChain := service.ChannelChainForLog(c); len(channelChain) > 0 {
			other["channel_chain"] = channelChain
		}
		adminInfo := make(map[string]interface{})
		adminInfo["use_channel"] = c.GetStringSlice("use_channel")
		isMultiKey := common.GetContextKeyBool(c, constant.ContextKeyChannelIsMultiKey)
		if isMultiKey {
			adminInfo["is_multi_key"] = true
			adminInfo["multi_key_index"] = common.GetContextKeyInt(c, constant.ContextKeyChannelMultiKeyIndex)
		}
		service.AppendChannelAffinityAdminInfo(c, adminInfo)
		other["admin_info"] = adminInfo
		startTime := common.GetContextKeyTime(c, constant.ContextKeyRequestStartTime)
		if startTime.IsZero() {
			startTime = time.Now()
		}
		useTimeSeconds := int(time.Since(startTime).Seconds())
		model.RecordErrorLog(c, userId, channelId, modelName, tokenName, err.MaskSensitiveErrorWithStatusCode(), tokenId, useTimeSeconds, common.GetContextKeyBool(c, constant.ContextKeyIsStream), userGroup, other)
	}

}

func RelayMidjourney(c *gin.Context) {
	relayInfo, err := relaycommon.GenRelayInfo(c, types.RelayFormatMjProxy, nil, nil)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"description": fmt.Sprintf("failed to generate relay info: %s", err.Error()),
			"type":        "upstream_error",
			"code":        4,
		})
		return
	}

	var mjErr *dto.MidjourneyResponse
	switch relayInfo.RelayMode {
	case relayconstant.RelayModeMidjourneyNotify:
		mjErr = relay.RelayMidjourneyNotify(c)
	case relayconstant.RelayModeMidjourneyTaskFetch, relayconstant.RelayModeMidjourneyTaskFetchByCondition:
		mjErr = relay.RelayMidjourneyTask(c, relayInfo.RelayMode)
	case relayconstant.RelayModeMidjourneyTaskImageSeed:
		mjErr = relay.RelayMidjourneyTaskImageSeed(c)
	case relayconstant.RelayModeSwapFace:
		mjErr = relay.RelaySwapFace(c, relayInfo)
	default:
		mjErr = relay.RelayMidjourneySubmit(c, relayInfo)
	}
	//err = relayMidjourneySubmit(c, relayMode)
	log.Println(mjErr)
	if mjErr != nil {
		statusCode := http.StatusBadRequest
		if mjErr.Code == 30 {
			mjErr.Result = "当前分组负载已饱和，请稍后再试，或升级账户以提升服务质量。"
			statusCode = http.StatusTooManyRequests
		}
		c.JSON(statusCode, gin.H{
			"description": fmt.Sprintf("%s %s", mjErr.Description, mjErr.Result),
			"type":        "upstream_error",
			"code":        mjErr.Code,
		})
		channelId := c.GetInt("channel_id")
		logger.LogError(c, fmt.Sprintf("relay error (channel #%d, status code %d): %s", channelId, statusCode, fmt.Sprintf("%s %s", mjErr.Description, mjErr.Result)))
	}
}

func RelayNotImplemented(c *gin.Context) {
	err := types.OpenAIError{
		Message: "API not implemented",
		Type:    "new_api_error",
		Param:   "",
		Code:    "api_not_implemented",
	}
	c.JSON(http.StatusNotImplemented, gin.H{
		"error": err,
	})
}

func RelayNotFound(c *gin.Context) {
	err := types.OpenAIError{
		Message: fmt.Sprintf("Invalid URL (%s %s)", c.Request.Method, c.Request.URL.Path),
		Type:    "invalid_request_error",
		Param:   "",
		Code:    "",
	}
	c.JSON(http.StatusNotFound, gin.H{
		"error": err,
	})
}

func RelayTaskFetch(c *gin.Context) {
	relayInfo, err := relaycommon.GenRelayInfo(c, types.RelayFormatTask, nil, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, &dto.TaskError{
			Code:       "gen_relay_info_failed",
			Message:    err.Error(),
			StatusCode: http.StatusInternalServerError,
		})
		return
	}
	if taskErr := relay.RelayTaskFetch(c, relayInfo.RelayMode); taskErr != nil {
		respondTaskError(c, taskErr)
	}
}

func RelayTask(c *gin.Context) {
	relayInfo, err := relaycommon.GenRelayInfo(c, types.RelayFormatTask, nil, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, &dto.TaskError{
			Code:       "gen_relay_info_failed",
			Message:    err.Error(),
			StatusCode: http.StatusInternalServerError,
		})
		return
	}

	if taskErr := relay.ResolveOriginTask(c, relayInfo); taskErr != nil {
		respondTaskError(c, taskErr)
		return
	}

	var result *relay.TaskSubmitResult
	var taskErr *dto.TaskError
	defer func() {
		if taskErr != nil && relayInfo.Billing != nil {
			relayInfo.Billing.Refund(c)
		}
	}()

	retryParam := &service.RetryParam{
		Ctx:           c,
		TokenGroup:    relayInfo.TokenGroup,
		ModelName:     relayInfo.OriginModelName,
		RequestPath:   c.Request.URL.Path,
		Retry:         common.GetPointer(0),
		AllowedGroups: relayInfo.SubscriptionProviderGroups,
	}

	for {
		if retryParam.GetRetry() > common.RetryTimes {
			break
		}
		var channel *model.Channel

		if lockedCh, ok := relayInfo.LockedChannel.(*model.Channel); ok && lockedCh != nil {
			var channelErr *types.NewAPIError
			channel, channelErr = selectRelayChannelWithRateLimit(c, retryParam, func() (*model.Channel, *types.NewAPIError) {
				if retryParam.GetRetry() > 0 {
					if setupErr := middleware.SetupContextForSelectedChannel(c, lockedCh, relayInfo.OriginModelName); setupErr != nil {
						return nil, setupErr
					}
				}
				return lockedCh, nil
			})
			if channelErr != nil {
				taskErr = service.TaskErrorWrapperLocal(channelErr.Err, "get_channel_failed", channelErr.StatusCode)
				break
			}
		} else {
			var channelErr *types.NewAPIError
			channel, channelErr = getChannelWithRateLimit(c, relayInfo, retryParam)
			if channelErr != nil {
				logger.LogError(c, channelErr.Error())
				taskErr = service.TaskErrorWrapperLocal(channelErr.Err, "get_channel_failed", channelErr.StatusCode)
				break
			}
		}

		addUsedChannel(c, channel.Id)
		bodyStorage, bodyErr := common.GetBodyStorage(c)
		if bodyErr != nil {
			if common.IsRequestBodyTooLargeError(bodyErr) || errors.Is(bodyErr, common.ErrRequestBodyTooLarge) {
				taskErr = service.TaskErrorWrapperLocal(bodyErr, "read_request_body_failed", http.StatusRequestEntityTooLarge)
			} else {
				taskErr = service.TaskErrorWrapperLocal(bodyErr, "read_request_body_failed", http.StatusBadRequest)
			}
			break
		}
		t429Retry := 0
		triedKeyIndexes := make(map[int]struct{})
		retriedAfter429WithinChannel := false
		for {
			if _, seekErr := bodyStorage.Seek(0, io.SeekStart); seekErr != nil {
				taskErr = service.TaskErrorWrapperLocal(seekErr, "read_request_body_failed", http.StatusBadRequest)
				break
			}
			c.Request.Body = io.NopCloser(bodyStorage)

			result, taskErr = relay.RelayTaskSubmit(c, relayInfo)
			if taskErr == nil {
				break
			}

			if !taskErr.LocalError {
				processChannelError(c,
					*types.NewChannelError(channel.Id, channel.Type, channel.Name, channel.ChannelInfo.IsMultiKey,
						common.GetContextKeyString(c, constant.ContextKeyChannelKey), channel.GetAutoBan()),
					types.NewOpenAIError(taskErr.Error, types.ErrorCodeBadResponseStatusCode, taskErr.StatusCode))
			}
			if taskErr.LocalError || !isTooManyRequestsTaskError(taskErr) {
				break
			}
			markChannelRateLimited(channel.Id)
			if !prepareTooManyRequestsRetry(c, channel, relayInfo.OriginModelName, triedKeyIndexes, t429Retry) {
				break
			}
			retriedAfter429WithinChannel = true
			t429Retry++
		}
		if taskErr == nil {
			break
		}

		if retriedAfter429WithinChannel {
			if shouldContinueAfterTaskUpstreamRateLimit(c, taskErr) {
				continue
			}
			break
		}
		if shouldContinueAfterTaskUpstreamRateLimit(c, taskErr) {
			continue
		}
		if len(retryParam.AllowedGroups) == 0 && len(relayInfo.SubscriptionProviderGroups) > 0 {
			retryParam.AllowedGroups = append([]string(nil), relayInfo.SubscriptionProviderGroups...)
		}
		if !shouldRetryTaskRelay(c, channel.Id, taskErr, common.RetryTimes-retryParam.GetRetry()) {
			break
		}
		service.PrepareRetryAfterChannelFailure(retryParam, channel)
		retryParam.IncreaseRetry()
	}

	useChannel := c.GetStringSlice("use_channel")
	if len(useChannel) > 1 {
		retryLogStr := fmt.Sprintf("重试：%s", strings.Trim(strings.Join(strings.Fields(fmt.Sprint(useChannel)), "->"), "[]"))
		logger.LogInfo(c, retryLogStr)
	}

	// ── 成功：结算 + 日志 + 插入任务 ──
	if taskErr == nil {
		if settleErr := service.SettleBilling(c, relayInfo, result.Quota); settleErr != nil {
			common.SysError("settle task billing error: " + settleErr.Error())
		}
		service.LogTaskConsumption(c, relayInfo)

		task := model.InitTask(result.Platform, relayInfo)
		task.PrivateData.UpstreamTaskID = result.UpstreamTaskID
		task.PrivateData.BillingSource = relayInfo.BillingSource
		task.PrivateData.SubscriptionId = relayInfo.SubscriptionId
		task.PrivateData.TokenId = relayInfo.TokenId
		task.PrivateData.BillingContext = &model.TaskBillingContext{
			ModelPrice:      relayInfo.PriceData.ModelPrice,
			GroupRatio:      relayInfo.PriceData.GroupRatioInfo.GroupRatio,
			ModelRatio:      relayInfo.PriceData.ModelRatio,
			OtherRatios:     relayInfo.PriceData.OtherRatios,
			OriginModelName: relayInfo.OriginModelName,
			PerCallBilling:  common.StringsContains(constant.TaskPricePatches, relayInfo.OriginModelName) || relayInfo.PriceData.UsePrice,
		}
		task.Quota = result.Quota
		task.Data = result.TaskData
		task.Action = relayInfo.Action
		if insertErr := task.Insert(); insertErr != nil {
			common.SysError("insert task error: " + insertErr.Error())
		}
	}

	if taskErr != nil {
		respondTaskError(c, taskErr)
	}
}

// respondTaskError 统一输出 Task 错误响应（含 429 限流提示改写）
func respondTaskError(c *gin.Context, taskErr *dto.TaskError) {
	taskErr.StatusCode, taskErr.Code, taskErr.Message = service.RewriteUserFacingTaskError(taskErr.StatusCode, taskErr.Code, taskErr.Message, taskErr.LocalError)
	c.JSON(taskErr.StatusCode, taskErr)
}

func isTooManyRequestsTaskError(taskErr *dto.TaskError) bool {
	return taskErr != nil && taskErr.StatusCode == http.StatusTooManyRequests
}

func shouldRetryTaskRelay(c *gin.Context, channelId int, taskErr *dto.TaskError, retryTimes int) bool {
	if taskErr == nil {
		return false
	}
	if service.ShouldSkipRetryAfterChannelAffinityFailure(c) {
		return false
	}
	if retryTimes <= 0 {
		return false
	}
	if _, ok := c.Get("specific_channel_id"); ok {
		return false
	}
	if isTooManyRequestsTaskError(taskErr) {
		return false
	}
	if taskErr.StatusCode == 307 {
		return true
	}
	if taskErr.StatusCode/100 == 5 {
		// 超时不重试
		if operation_setting.IsAlwaysSkipRetryStatusCode(taskErr.StatusCode) {
			return false
		}
		return true
	}
	if taskErr.StatusCode == http.StatusBadRequest {
		return false
	}
	if taskErr.StatusCode == 408 {
		// azure处理超时不重试
		return false
	}
	if taskErr.LocalError {
		return false
	}
	if taskErr.StatusCode/100 == 2 {
		return false
	}
	return true
}
