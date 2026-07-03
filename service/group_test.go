package service

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// withAutoGroups sets the legacy auto-groups + usable-groups JSON for the
// duration of the test and restores the previous values on cleanup. It also
// seeds an enabled ProviderGroup row for each candidate so
// model.IsProviderGroupOnline reports them as online in the shared test DB,
// and clears any provider_group_auto_rules so GetRequestAutoGroup falls back
// to setting.GetAutoGroups() for the non-DB-backed cases. Provider groups are
// explicitly initialised so the test does not depend on whatever global
// state earlier tests left behind.
func withAutoGroups(t *testing.T, autoGroups, usableGroups string) {
	t.Helper()
	oldAuto := setting.AutoGroups2JsonString()
	oldUsable := setting.UserUsableGroups2JSONString()
	t.Cleanup(func() {
		require.NoError(t, setting.UpdateAutoGroupsByJsonString(oldAuto))
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(oldUsable))
	})
	require.NoError(t, setting.UpdateAutoGroupsByJsonString(autoGroups))
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(usableGroups))
	require.NoError(t, model.DB.Exec("DELETE FROM provider_group_auto_rules").Error)
	require.NoError(t, model.DB.Exec("DELETE FROM provider_group_channels").Error)
	require.NoError(t, model.DB.Exec("DELETE FROM provider_groups").Error)
	t.Cleanup(func() {
		model.DB.Exec("DELETE FROM provider_group_auto_rules")
		model.DB.Exec("DELETE FROM provider_group_channels")
		model.DB.Exec("DELETE FROM provider_groups")
	})
	var groupNames []string
	require.NoError(t, common.Unmarshal([]byte(autoGroups), &groupNames))
	for _, name := range groupNames {
		require.NoError(t, model.DB.Create(&model.ProviderGroup{
			Name: name, DisplayName: name, Status: model.ProviderGroupStatusEnabled, UsageRatio: 1,
		}).Error)
	}
}

func newAutoGroupContext(path string, body string) *gin.Context {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	var req *http.Request
	if body != "" {
		req = httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(http.MethodPost, path, nil)
	}
	c.Request = req
	return c
}

// TestGetRequestAutoGroupFiltersRouteScopedGroups guards the route-type
// intersection: when ContextKeyRouteAutoGroups restricts the candidate set,
// the result keeps only the intersection while preserving the auto order.
// The codex-pro candidate is retained here because the request carries a
// Codex-family marker, so the family filter must not drop it.
func TestGetRequestAutoGroupFiltersRouteScopedGroups(t *testing.T) {
	gin.SetMode(gin.TestMode)
	withAutoGroups(t,
		`["codex","codex-pro","codex-completions"]`,
		`{"codex":"Codex","codex-pro":"Codex Pro","codex-completions":"Codex Completions"}`,
	)

	ctx := newAutoGroupContext("/v1/responses", "")
	ctx.Request.Header.Set("User-Agent", "codex-cli/0.1")
	common.SetContextKey(ctx, constant.ContextKeyRouteAutoGroups, []string{"codex", "codex-pro"})

	groups := GetRequestAutoGroup(ctx, "default")

	require.Equal(t, []string{"codex", "codex-pro"}, groups)
}

// TestGetRequestAutoGroupFiltersClaudeMaxForNonClaudeCodeRequest asserts that
// a plain /v1/messages request (no Claude Code family markers) drops
// claude-max from the auto candidates but keeps the other candidates in order.
func TestGetRequestAutoGroupFiltersClaudeMaxForNonClaudeCodeRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	withAutoGroups(t,
		`["claude-max","claude-kiro","anthropic"]`,
		`{"claude-max":"Claude Max","claude-kiro":"Claude Kiro","anthropic":"Anthropic"}`,
	)

	ctx := newAutoGroupContext("/v1/messages", `{"model":"claude-3-5-sonnet","messages":[{"role":"user","content":"hi"}]}`)

	groups := GetRequestAutoGroup(ctx, "default")

	assert.Equal(t, []string{"claude-kiro", "anthropic"}, groups)
	assert.NotContains(t, groups, "claude-max")
}

// TestGetRequestAutoGroupKeepsClaudeMaxForClaudeCodeFamilyRequest asserts that
// a /v1/messages request carrying Claude Code family markers (the User-Agent
// header containing "claude-code") retains claude-max alongside other
// candidates, preserving order.
func TestGetRequestAutoGroupKeepsClaudeMaxForClaudeCodeFamilyRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	withAutoGroups(t,
		`["claude-max","claude-kiro","anthropic"]`,
		`{"claude-max":"Claude Max","claude-kiro":"Claude Kiro","anthropic":"Anthropic"}`,
	)

	ctx := newAutoGroupContext("/v1/messages", `{"model":"claude-3-5-sonnet","messages":[{"role":"user","content":"hi"}]}`)
	ctx.Request.Header.Set("User-Agent", "claude-code/1.0")

	groups := GetRequestAutoGroup(ctx, "default")

	assert.Equal(t, []string{"claude-max", "claude-kiro", "anthropic"}, groups)
}

// TestGetRequestAutoGroupKeepsClaudeMaxForClaudeCodeToolsBody covers the
// body-based family detection: a /v1/messages request whose tools list names
// at least two Claude Code tools (bash, read) is treated as Claude Code
// family and keeps claude-max even without a distinguishing header.
func TestGetRequestAutoGroupKeepsClaudeMaxForClaudeCodeToolsBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	withAutoGroups(t,
		`["claude-max","claude-kiro"]`,
		`{"claude-max":"Claude Max","claude-kiro":"Claude Kiro"}`,
	)

	body := `{"model":"claude-3-5-sonnet","messages":[{"role":"user","content":"hi"}],"tools":[{"name":"bash"},{"name":"read"}]}`
	ctx := newAutoGroupContext("/v1/messages", body)

	groups := GetRequestAutoGroup(ctx, "default")

	assert.Equal(t, []string{"claude-max", "claude-kiro"}, groups)
}

// TestGetRequestAutoGroupFiltersCodexProForNonCodexRequest asserts that a
// plain /v1/responses request (no Codex family markers) drops codex-pro but
// keeps the plain codex candidate, preserving order.
func TestGetRequestAutoGroupFiltersCodexProForNonCodexRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	withAutoGroups(t,
		`["codex","codex-pro","openai"]`,
		`{"codex":"Codex","codex-pro":"Codex Pro","openai":"OpenAI"}`,
	)

	ctx := newAutoGroupContext("/v1/responses", `{"model":"gpt-4.1","input":"hi"}`)

	groups := GetRequestAutoGroup(ctx, "default")

	assert.Equal(t, []string{"codex", "openai"}, groups)
	assert.NotContains(t, groups, "codex-pro")
}

// TestGetRequestAutoGroupKeepsCodexProForCodexFamilyRequest asserts that a
// /v1/responses request carrying a Codex family marker (User-Agent containing
// "codex") retains codex-pro alongside codex, preserving order.
func TestGetRequestAutoGroupKeepsCodexProForCodexFamilyRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	withAutoGroups(t,
		`["codex","codex-pro","openai"]`,
		`{"codex":"Codex","codex-pro":"Codex Pro","openai":"OpenAI"}`,
	)

	ctx := newAutoGroupContext("/v1/responses", `{"model":"gpt-5","input":"hi"}`)
	ctx.Request.Header.Set("User-Agent", "codex-cli/0.1")

	groups := GetRequestAutoGroup(ctx, "default")

	assert.Equal(t, []string{"codex", "codex-pro", "openai"}, groups)
}

// TestGetRequestAutoGroupKeepsCodexProForCodexBodyMarker covers body-based
// Codex family detection: a /v1/responses request whose body carries a
// prompt_cache_key is treated as Codex family and keeps codex-pro even
// without a distinguishing header.
func TestGetRequestAutoGroupKeepsCodexProForCodexBodyMarker(t *testing.T) {
	gin.SetMode(gin.TestMode)
	withAutoGroups(t,
		`["codex","codex-pro"]`,
		`{"codex":"Codex","codex-pro":"Codex Pro"}`,
	)

	body := `{"model":"gpt-5","input":"hi","prompt_cache_key":"codex-task-abc"}`
	ctx := newAutoGroupContext("/v1/responses", body)

	groups := GetRequestAutoGroup(ctx, "default")

	assert.Equal(t, []string{"codex", "codex-pro"}, groups)
}

// clearProviderGroupTables wipes the provider-group tables for the DB-backed
// route-rule tests so they do not depend on whatever state earlier tests left
// behind. The service TestMain (in task_billing_test.go) migrates these
// tables; if it did not, the queries would error and these tests would fail
// loudly rather than silently pass.
func clearProviderGroupTables(t *testing.T) {
	t.Helper()
	require.NoError(t, model.DB.Exec("DELETE FROM provider_group_auto_rules").Error)
	require.NoError(t, model.DB.Exec("DELETE FROM provider_group_channels").Error)
	require.NoError(t, model.DB.Exec("DELETE FROM provider_groups").Error)
	t.Cleanup(func() {
		model.DB.Exec("DELETE FROM provider_group_auto_rules")
		model.DB.Exec("DELETE FROM provider_group_channels")
		model.DB.Exec("DELETE FROM provider_groups")
	})
}

// seedMessagesAutoRules inserts provider_group_auto_rules for the "messages"
// route type with the given candidate order and marks each candidate group
// online (enabled ProviderGroup row). This exercises the
// model.GetProviderAutoGroups(routeType) path in GetRequestAutoGroup rather
// than the legacy setting fallback.
func seedMessagesAutoRules(t *testing.T, candidates ...string) {
	t.Helper()
	clearProviderGroupTables(t)
	for i, name := range candidates {
		require.NoError(t, model.DB.Create(&model.ProviderGroup{
			Name: name, DisplayName: name, Status: model.ProviderGroupStatusEnabled, UsageRatio: 1,
		}).Error)
		require.NoError(t, model.DB.Create(&model.ProviderGroupAutoRule{
			RouteType:      model.ProviderRouteTypeMessages,
			CandidateGroup: name,
			SortOrder:      i,
			Enabled:        true,
		}).Error)
	}
}

// TestGetRequestAutoGroupDBMessagesRuleFiltersClaudeMax asserts the DB-backed
// path: when provider_group_auto_rules for the "messages" route type list
// [claude-max, claude-kiro], a plain /v1/messages request (no Claude Code
// family markers) drops claude-max via the family filter and returns only
// claude-kiro. This proves the family filter composes with
// GetProviderAutoGroups(routeType), not just the setting fallback.
func TestGetRequestAutoGroupDBMessagesRuleFiltersClaudeMax(t *testing.T) {
	gin.SetMode(gin.TestMode)
	seedMessagesAutoRules(t, "claude-max", "claude-kiro")

	ctx := newAutoGroupContext("/v1/messages", `{"model":"claude-3-5-sonnet","messages":[{"role":"user","content":"hi"}]}`)

	groups := GetRequestAutoGroup(ctx, "default")

	assert.Equal(t, []string{"claude-kiro"}, groups)
	assert.NotContains(t, groups, "claude-max")
}

// TestGetRequestAutoGroupDBMessagesRuleKeepsClaudeMaxForClaudeCode asserts
// that with the same DB-backed messages rules, a /v1/messages request
// carrying a Claude Code family header retains both claude-max and
// claude-kiro in the seeded order.
func TestGetRequestAutoGroupDBMessagesRuleKeepsClaudeMaxForClaudeCode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	seedMessagesAutoRules(t, "claude-max", "claude-kiro")

	ctx := newAutoGroupContext("/v1/messages", `{"model":"claude-3-5-sonnet","messages":[{"role":"user","content":"hi"}]}`)
	ctx.Request.Header.Set("User-Agent", "claude-code/1.0")

	groups := GetRequestAutoGroup(ctx, "default")

	assert.Equal(t, []string{"claude-max", "claude-kiro"}, groups)
}
