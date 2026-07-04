package operation_setting

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
)

var ErrorRewriteEnabled = true

type ErrorRewriteRule struct {
	Name        string   `json:"name"`
	StatusCodes string   `json:"status_codes"`
	Keywords    []string `json:"keywords"`
	Message     string   `json:"message"`
	StatusCode  int      `json:"status_code"`
	Enabled     bool     `json:"enabled"`
}

var DefaultErrorRewriteRules = []ErrorRewriteRule{
	{
		Name:        "quota-limited-429",
		StatusCodes: "429",
		Keywords:    []string{"exceeded your current quota", "credit balance"},
		Message:     "The service is temporarily unavailable. Please try again later.",
		StatusCode:  http.StatusServiceUnavailable,
		Enabled:     true,
	},
	{
		Name:        "generic-429",
		StatusCodes: "429",
		Message:     "We're experiencing high demand right now. Please retry in a moment.",
		StatusCode:  http.StatusTooManyRequests,
		Enabled:     true,
	},
	{
		Name:       "account-or-auth-unavailable",
		Keywords:   []string{"no available accounts", "auth_unavailable", "authentication token", "signing in again"},
		Message:    "The service is temporarily unavailable. Please try again later.",
		StatusCode: http.StatusServiceUnavailable,
		Enabled:    true,
	},
	{
		Name:       "claude-cli-required",
		Keywords:   []string{"official Claude CLI", "only accessible via the official Claude CLI"},
		Message:    "This model requires the official Claude Code CLI. Please use Claude Code and retry.",
		StatusCode: http.StatusForbidden,
		Enabled:    true,
	},
	{
		Name:       "codex-cli-required",
		Keywords:   []string{"codex 客户端限制", "请使用 /v1/responses"},
		Message:    "This model requires the official Codex CLI. Please use Codex (/v1/responses) and retry.",
		StatusCode: http.StatusForbidden,
		Enabled:    true,
	},
	{
		Name:        "permission-unavailable",
		StatusCodes: "403,502",
		Keywords:    []string{"access forbidden", "permission denied"},
		Message:     "The service is temporarily unavailable. Please try again later.",
		StatusCode:  http.StatusServiceUnavailable,
		Enabled:     true,
	},
	{
		Name:        "temporary-upstream-issue",
		StatusCodes: "500,502,503,504,524",
		Message:     "The service encountered a temporary issue. Please retry later.",
		StatusCode:  0,
		Enabled:     true,
	},
}

var ErrorRewriteRules = cloneErrorRewriteRules(DefaultErrorRewriteRules)

func ErrorRewriteRulesToJSONString() string {
	data, err := common.Marshal(ErrorRewriteRules)
	if err != nil {
		return "[]"
	}
	return string(data)
}

func UpdateErrorRewriteRulesByJSONString(value string) error {
	var rules []ErrorRewriteRule
	if err := common.Unmarshal([]byte(value), &rules); err != nil {
		return err
	}
	if err := ValidateErrorRewriteRules(rules); err != nil {
		return err
	}
	ErrorRewriteRules = normalizeErrorRewriteRules(rules)
	return nil
}

func ValidateErrorRewriteRulesJSON(value string) error {
	var rules []ErrorRewriteRule
	if err := common.Unmarshal([]byte(value), &rules); err != nil {
		return err
	}
	return ValidateErrorRewriteRules(rules)
}

func ValidateErrorRewriteRules(rules []ErrorRewriteRule) error {
	for i, rule := range rules {
		if strings.TrimSpace(rule.Message) == "" {
			return fmt.Errorf("error rewrite rule %d message is required", i+1)
		}
		if _, err := ParseHTTPStatusCodeRanges(rule.StatusCodes); err != nil {
			return fmt.Errorf("error rewrite rule %d status_codes invalid: %w", i+1, err)
		}
		if rule.StatusCode != 0 && (rule.StatusCode < 100 || rule.StatusCode > 599) {
			return fmt.Errorf("error rewrite rule %d status_code out of bounds", i+1)
		}
	}
	return nil
}

func ErrorRewriteRuleMatchesStatusCode(rule ErrorRewriteRule, statusCode int) bool {
	ranges, err := ParseHTTPStatusCodeRanges(rule.StatusCodes)
	if err != nil {
		return false
	}
	if len(ranges) == 0 {
		return true
	}
	if statusCode < 100 || statusCode > 599 {
		return false
	}
	for _, r := range ranges {
		if statusCode >= r.Start && statusCode <= r.End {
			return true
		}
	}
	return false
}

func cloneErrorRewriteRules(rules []ErrorRewriteRule) []ErrorRewriteRule {
	cloned := make([]ErrorRewriteRule, len(rules))
	for i, rule := range rules {
		cloned[i] = rule
		cloned[i].Keywords = append([]string(nil), rule.Keywords...)
	}
	return cloned
}

func normalizeErrorRewriteRules(rules []ErrorRewriteRule) []ErrorRewriteRule {
	normalized := cloneErrorRewriteRules(rules)
	for i := range normalized {
		normalized[i].Name = strings.TrimSpace(normalized[i].Name)
		normalized[i].StatusCodes = strings.TrimSpace(normalized[i].StatusCodes)
		normalized[i].Message = strings.TrimSpace(normalized[i].Message)
		keywords := make([]string, 0, len(normalized[i].Keywords))
		for _, keyword := range normalized[i].Keywords {
			keyword = strings.ToLower(strings.TrimSpace(keyword))
			if keyword != "" {
				keywords = append(keywords, keyword)
			}
		}
		normalized[i].Keywords = keywords
	}
	return normalized
}
