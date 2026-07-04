package operation_setting

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultErrorRewriteRules(t *testing.T) {
	require.Len(t, DefaultErrorRewriteRules, 7)
	assert.Equal(t, "429", DefaultErrorRewriteRules[0].StatusCodes)
	assert.Equal(t, "We're experiencing high demand right now. Please retry in a moment.", DefaultErrorRewriteRules[1].Message)
	assert.Equal(t, "This model requires the official Claude Code CLI. Please use Claude Code and retry.", DefaultErrorRewriteRules[3].Message)
	assert.Equal(t, "This model requires the official Codex CLI. Please use Codex (/v1/responses) and retry.", DefaultErrorRewriteRules[4].Message)
	assert.Equal(t, 0, DefaultErrorRewriteRules[6].StatusCode)
}

func TestUpdateErrorRewriteRulesByJSONString(t *testing.T) {
	oldRules := cloneErrorRewriteRules(ErrorRewriteRules)
	t.Cleanup(func() {
		ErrorRewriteRules = oldRules
	})

	jsonValue := `[{"name":"any-status","status_codes":"","keywords":["UPSTREAM"],"message":"Temporary issue.","status_code":0,"enabled":true}]`
	require.NoError(t, UpdateErrorRewriteRulesByJSONString(jsonValue))
	require.Len(t, ErrorRewriteRules, 1)
	assert.Equal(t, []string{"upstream"}, ErrorRewriteRules[0].Keywords)
	assert.True(t, ErrorRewriteRuleMatchesStatusCode(ErrorRewriteRules[0], http.StatusBadRequest))

	assert.NoError(t, ValidateErrorRewriteRulesJSON(ErrorRewriteRulesToJSONString()))
	assert.Error(t, ValidateErrorRewriteRulesJSON(`[{"name":"bad","status_codes":"99","message":"x","enabled":true}]`))
}
