package service

import (
	"regexp"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"
	ags "github.com/QuantumNous/new-api/setting/abuse_guard_setting"
)

// builtinPattern 是内置破限模式的定义。
//
// 性能关键:regex 类模式必须提供 anchors —— 一组小写字面子串,当且仅当其中至少一个
// 出现在文本中时该正则才有可能命中。所有 anchors 与 keyword 一起进入 AC 自动机(一次
// 线性扫描),只有命中 anchor 的正则才会真正执行。正常文本命中零 anchor,几乎零正则开销。
//
// 匹配统一在“已转小写”的文本上进行,因此模式字面量必须写成小写,且不使用 (?i)。
type builtinPattern struct {
	id      string
	kind    string
	pattern string
	weight  int
	anchors []string // 仅 regex 类需要
}

var builtinJailbreakPatterns = []builtinPattern{
	// 指令覆盖 / instruction override
	{id: "ovr_ignore_prev_en", kind: ags.PatternKindRegex, weight: 6, anchors: []string{"ignore"}, pattern: `ignore\s+(?:all\s+)?(?:previous|prior|above)\s+(?:instructions|prompts|rules|messages)`},
	{id: "ovr_disregard_en", kind: ags.PatternKindRegex, weight: 6, anchors: []string{"disregard"}, pattern: `disregard\s+(?:all\s+)?(?:previous|prior|above|your)\s+(?:instructions|rules|guidelines)`},
	{id: "ovr_forget_en", kind: ags.PatternKindRegex, weight: 5, anchors: []string{"forget"}, pattern: `forget\s+(?:everything|all|your\s+(?:instructions|rules|guidelines|training))`},
	{id: "ovr_override_en", kind: ags.PatternKindRegex, weight: 5, anchors: []string{"override"}, pattern: `override\s+(?:your\s+)?(?:system\s+)?(?:prompt|instructions|programming|settings)`},
	{id: "ovr_ignore_cn", kind: ags.PatternKindKeyword, weight: 6, pattern: "忽略之前的指令"},
	{id: "ovr_ignore_cn2", kind: ags.PatternKindKeyword, weight: 6, pattern: "无视以上所有"},
	{id: "ovr_ignore_cn3", kind: ags.PatternKindKeyword, weight: 5, pattern: "忘记你之前"},

	// 越狱人格 / jailbreak persona
	{id: "per_dan", kind: ags.PatternKindRegex, weight: 7, anchors: []string{"dan"}, pattern: `\bdan\b.{0,40}(?:mode|jailbreak|do\s+anything)`},
	{id: "per_do_anything", kind: ags.PatternKindRegex, weight: 6, anchors: []string{"do anything"}, pattern: `do\s+anything\s+now`},
	{id: "per_dev_mode", kind: ags.PatternKindRegex, weight: 6, anchors: []string{"developer mode"}, pattern: `developer\s+mode\s+(?:enabled|on|output|activated)`},
	{id: "per_jailbreak", kind: ags.PatternKindKeyword, weight: 5, pattern: "jailbreak"},
	{id: "per_no_restrictions_en", kind: ags.PatternKindRegex, weight: 6, anchors: []string{"no restriction", "no filter", "unfiltered", "unrestricted"}, pattern: `(?:you\s+are\s+now|act\s+as).{0,30}(?:no\s+restrictions?|no\s+filter|unfiltered|unrestricted)`},
	{id: "per_no_limit_cn", kind: ags.PatternKindKeyword, weight: 5, pattern: "没有任何限制"},
	{id: "per_no_limit_cn2", kind: ags.PatternKindKeyword, weight: 5, pattern: "不受任何限制"},
	{id: "per_no_limit_cn3", kind: ags.PatternKindKeyword, weight: 6, pattern: "解除所有限制"},
	{id: "per_evil_ai", kind: ags.PatternKindRegex, weight: 6, anchors: []string{"evil", "amoral", "unethical", "uncensored"}, pattern: `(?:evil|amoral|unethical|uncensored)\s+ai`},

	// 系统提示套取 / system prompt extraction
	{id: "sys_reveal_prompt_en", kind: ags.PatternKindRegex, weight: 6, anchors: []string{"system prompt", "initial instruction", "prompt above"}, pattern: `(?:reveal|show|print|repeat|output|tell\s+me)\s+.{0,20}(?:system\s+prompt|initial\s+instructions|the\s+prompt\s+above)`},
	{id: "sys_prompt_cn", kind: ags.PatternKindKeyword, weight: 5, pattern: "你的系统提示词"},
	{id: "sys_prompt_cn2", kind: ags.PatternKindKeyword, weight: 5, pattern: "重复上面的指令"},
	{id: "sys_verbatim_en", kind: ags.PatternKindRegex, weight: 6, anchors: []string{"verbatim"}, pattern: `repeat\s+the\s+(?:words|text)\s+above\s+verbatim`},

	// 道德/安全解除 / moral & safety suppression
	{id: "saf_no_ethics_en", kind: ags.PatternKindRegex, weight: 6, anchors: []string{"ethical", "moral", "safety", "legal"}, pattern: `(?:without|ignore|bypass|forget)\s+(?:any\s+)?(?:ethical|moral|safety|legal)\s+(?:concerns|guidelines|considerations|restrictions|filters)`},
	{id: "saf_no_warning_en", kind: ags.PatternKindRegex, weight: 4, anchors: []string{"warning", "disclaimer", "moralizing", "lecture"}, pattern: `(?:no|without)\s+(?:warnings?|disclaimers?|moralizing|lectures?)`},
	{id: "saf_hypothetical_en", kind: ags.PatternKindRegex, weight: 4, anchors: []string{"hypothetically"}, pattern: `hypothetically.{0,40}(?:no\s+rules|illegal|without\s+restrictions)`},
	{id: "saf_cn_no_moral", kind: ags.PatternKindKeyword, weight: 5, pattern: "不需要考虑道德"},
	{id: "saf_cn_no_law", kind: ags.PatternKindKeyword, weight: 5, pattern: "不用考虑法律"},

	// 编码混淆诱导 / encoding obfuscation
	{id: "enc_base64_decode", kind: ags.PatternKindRegex, weight: 5, anchors: []string{"base64"}, pattern: `(?:decode|decrypt)\s+(?:the\s+following\s+)?base64.{0,40}(?:execute|follow|do|run)`},
	{id: "enc_rot13", kind: ags.PatternKindRegex, weight: 4, anchors: []string{"rot13"}, pattern: `rot13.{0,30}(?:decode|execute|follow)`},

	// 虚构豁免包装 / fictional exemption framing
	{id: "fic_grandma", kind: ags.PatternKindRegex, weight: 5, anchors: []string{"grandma"}, pattern: `grandma.{0,60}(?:napalm|recipe|instructions|password|serial)`},
	{id: "fic_story_bypass", kind: ags.PatternKindRegex, weight: 3, anchors: []string{"story", "novel", "fiction"}, pattern: `(?:story|novel|fiction).{0,60}(?:how\s+to\s+(?:make|build|synthesize)|step[-\s]by[-\s]step)`},
}

type compiledRegex struct {
	id     string
	weight int
	re     *regexp.Regexp
}

type patternSet struct {
	keywordDict    []string                    // AC 输入:keyword 词 + 所有 regex anchor
	keywordByWord  map[string]compiledPattern  // 词 -> keyword 模式(仅 keyword 类)
	anchorToRegex  map[string][]*compiledRegex // anchor 词 -> 候选正则
	alwaysRunRegex []*compiledRegex            // 无 anchor 的自定义正则,总是执行
}

type compiledPattern struct {
	id     string
	weight int
}

var (
	patternSetCache *patternSet
	patternSetKey   string
	patternSetMu    sync.RWMutex
)

func buildPatternSet(s *ags.AbuseGuardSetting) *patternSet {
	disabled := make(map[string]struct{}, len(s.DisabledBuiltinIDs))
	for _, id := range s.DisabledBuiltinIDs {
		disabled[id] = struct{}{}
	}

	ps := &patternSet{
		keywordByWord: make(map[string]compiledPattern),
		anchorToRegex: make(map[string][]*compiledRegex),
	}
	dictSeen := make(map[string]struct{})
	addToDict := func(word string) {
		if _, ok := dictSeen[word]; !ok {
			dictSeen[word] = struct{}{}
			ps.keywordDict = append(ps.keywordDict, word)
		}
	}

	addKeyword := func(id, word string, weight int) {
		word = strings.ToLower(strings.TrimSpace(word))
		if word == "" {
			return
		}
		addToDict(word)
		ps.keywordByWord[word] = compiledPattern{id: id, weight: weight}
	}
	addRegex := func(id, pat string, weight int, anchors []string) {
		re, err := regexp.Compile(pat)
		if err != nil {
			common.SysError("abuse_guard: skip invalid regex pattern " + id + ": " + err.Error())
			return
		}
		cr := &compiledRegex{id: id, weight: weight, re: re}
		if len(anchors) == 0 {
			ps.alwaysRunRegex = append(ps.alwaysRunRegex, cr)
			return
		}
		for _, a := range anchors {
			a = strings.ToLower(strings.TrimSpace(a))
			if a == "" {
				continue
			}
			addToDict(a)
			ps.anchorToRegex[a] = append(ps.anchorToRegex[a], cr)
		}
	}

	for _, p := range builtinJailbreakPatterns {
		if _, ok := disabled[p.id]; ok {
			continue
		}
		if p.kind == ags.PatternKindKeyword {
			addKeyword(p.id, p.pattern, p.weight)
		} else {
			addRegex(p.id, p.pattern, p.weight, p.anchors)
		}
	}
	for _, p := range s.CustomPatterns {
		weight := p.Weight
		if weight <= 0 {
			weight = 1
		}
		if p.Kind == ags.PatternKindRegex {
			// 自定义正则无 anchor,总是执行(数量少,由管理员负责);匹配基于已小写文本。
			addRegex(p.ID, strings.ToLower(p.Pattern), weight, nil)
		} else {
			addKeyword(p.ID, p.Pattern, weight)
		}
	}
	return ps
}

func getPatternSet(s *ags.AbuseGuardSetting) *patternSet {
	key := patternSetSignature(s)

	patternSetMu.RLock()
	if patternSetCache != nil && patternSetKey == key {
		cached := patternSetCache
		patternSetMu.RUnlock()
		return cached
	}
	patternSetMu.RUnlock()

	patternSetMu.Lock()
	defer patternSetMu.Unlock()
	if patternSetCache != nil && patternSetKey == key {
		return patternSetCache
	}
	ps := buildPatternSet(s)
	patternSetCache = ps
	patternSetKey = key
	return ps
}

func patternSetSignature(s *ags.AbuseGuardSetting) string {
	var b strings.Builder
	b.WriteString(strings.Join(s.DisabledBuiltinIDs, ","))
	b.WriteByte('|')
	for _, p := range s.CustomPatterns {
		b.WriteString(p.ID)
		b.WriteByte(':')
		b.WriteString(p.Kind)
		b.WriteByte(':')
		b.WriteString(p.Pattern)
		b.WriteByte(';')
	}
	return b.String()
}

type patternHit struct {
	ID     string
	Weight int
}

// scan 对已小写文本执行模式匹配:一次 AC 扫描得到全部命中词与 anchor,再仅对命中 anchor
// 的正则做确认。返回去重命中(同一模式 ID 计一次)与权重和。
func (ps *patternSet) scan(lowerText string) ([]patternHit, int) {
	seen := make(map[string]int)
	candidates := make(map[*compiledRegex]struct{})

	if len(ps.keywordDict) > 0 {
		if ok, words := AcSearch(lowerText, ps.keywordDict, false); ok {
			for _, w := range words {
				lw := strings.ToLower(w)
				if cp, isKeyword := ps.keywordByWord[lw]; isKeyword {
					if _, dup := seen[cp.id]; !dup {
						seen[cp.id] = cp.weight
					}
				}
				for _, cr := range ps.anchorToRegex[lw] {
					candidates[cr] = struct{}{}
				}
			}
		}
	}

	for cr := range candidates {
		if _, dup := seen[cr.id]; dup {
			continue
		}
		if cr.re.MatchString(lowerText) {
			seen[cr.id] = cr.weight
		}
	}
	for _, cr := range ps.alwaysRunRegex {
		if _, dup := seen[cr.id]; dup {
			continue
		}
		if cr.re.MatchString(lowerText) {
			seen[cr.id] = cr.weight
		}
	}

	hits := make([]patternHit, 0, len(seen))
	total := 0
	for id, w := range seen {
		hits = append(hits, patternHit{ID: id, Weight: w})
		total += w
	}
	return hits, total
}
