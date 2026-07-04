package service

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	ags "github.com/QuantumNous/new-api/setting/abuse_guard_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSampleHitDeterministic(t *testing.T) {
	// 0% never samples, 100% always samples
	assert.False(t, sampleHit("req-1", 0))
	assert.True(t, sampleHit("req-1", 100))
	// same requestId + rate is deterministic
	assert.Equal(t, sampleHit("req-abc", 5), sampleHit("req-abc", 5))

	// approximate rate over many ids
	hit := 0
	total := 20000
	for i := 0; i < total; i++ {
		if sampleHit("request-id-"+string(rune(i))+itoa(i), 10) {
			hit++
		}
	}
	ratio := float64(hit) / float64(total)
	assert.InDelta(t, 0.10, ratio, 0.03, "sampling ratio should approximate configured rate")
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var b [20]byte
	p := len(b)
	for i > 0 {
		p--
		b[p] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		p--
		b[p] = '-'
	}
	return string(b[p:])
}

func TestIsFirstRequestMemoryFallback(t *testing.T) {
	common.RedisEnabled = false
	abuseFirstMem.Range(func(k, _ any) bool { abuseFirstMem.Delete(k); return true })

	assert.True(t, isFirstRequest(9001), "first call is first request")
	assert.False(t, isFirstRequest(9001), "second call is not")
	assert.True(t, isFirstRequest(9002), "different user is first")
}

func TestShouldReviewGates(t *testing.T) {
	common.RedisEnabled = false
	abuseFirstMem.Range(func(k, _ any) bool { abuseFirstMem.Delete(k); return true })

	noKey := &ags.AbuseGuardSetting{SampleRatePercent: 100}
	assert.False(t, shouldReview(noKey, 1, "r1", true), "no moderation key disables review")

	s := &ags.AbuseGuardSetting{ModerationAPIKey: "sk", SampleRatePercent: 0}
	assert.True(t, shouldReview(s, 100, "r1", true), "forced review bypasses sampling")
	assert.True(t, shouldReview(s, 100, "r1", false), "first request always reviewed")
	assert.False(t, shouldReview(s, 100, "r2", false), "subsequent with 0% sampling not reviewed")
}

func TestCallModerationFlagged(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/moderations", r.URL.Path)
		assert.Equal(t, "Bearer sk-test", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"results":[{"flagged":true,"categories":{"violence":true,"hate":false},"category_scores":{"violence":0.9,"hate":0.1}}]}`))
	}))
	defer srv.Close()

	s := &ags.AbuseGuardSetting{ModerationAPIKey: "sk-test", ModerationBaseURL: srv.URL, ModerationModel: "omni-moderation-latest"}
	flagged, category, err := callModeration(s, "some text")
	require.NoError(t, err)
	assert.True(t, flagged)
	assert.Equal(t, "violence", category)
}

func TestCallModerationNotFlagged(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"results":[{"flagged":false,"categories":{},"category_scores":{}}]}`))
	}))
	defer srv.Close()

	s := &ags.AbuseGuardSetting{ModerationAPIKey: "sk", ModerationBaseURL: srv.URL}
	flagged, _, err := callModeration(s, "hello")
	require.NoError(t, err)
	assert.False(t, flagged)
}

func TestCallModerationServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	s := &ags.AbuseGuardSetting{ModerationAPIKey: "sk", ModerationBaseURL: srv.URL}
	_, _, err := callModeration(s, "hello")
	assert.Error(t, err, "non-200 status is an error, caller degrades silently")
}

func TestTopCategoryPicksHighestScore(t *testing.T) {
	cat := topCategory(
		map[string]float64{"violence": 0.3, "sexual": 0.8, "hate": 0.5},
		map[string]bool{"violence": true, "sexual": true, "hate": false},
	)
	assert.Equal(t, "sexual", cat)
}
