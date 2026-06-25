package types

// ChannelDecisionContext stores non-sensitive selector details for one decision.
type ChannelDecisionContext struct {
	TotalCandidates     int     `json:"total_candidates,omitempty"`
	FilteredCandidates  int     `json:"filtered_candidates,omitempty"`
	Priority            int64   `json:"priority,omitempty"`
	Weight              int     `json:"weight,omitempty"`
	TotalWeight         int     `json:"total_weight,omitempty"`
	SelectedProbability float64 `json:"selected_probability,omitempty"`
}

// ChannelChainEntry records one channel routing decision without raw prompts or secrets.
type ChannelChainEntry struct {
	ChannelId     int                    `json:"channel_id,omitempty"`
	ChannelName   string                 `json:"channel_name,omitempty"`
	ChannelType   int                    `json:"channel_type,omitempty"`
	Group         string                 `json:"group,omitempty"`
	Reason        string                 `json:"reason,omitempty"`
	Selection     string                 `json:"selection,omitempty"`
	Attempt       int                    `json:"attempt,omitempty"`
	RetryIndex    int                    `json:"retry_index,omitempty"`
	CircuitState  string                 `json:"circuit_state,omitempty"`
	Endpoint      string                 `json:"endpoint,omitempty"`
	ErrorCode     string                 `json:"error_code,omitempty"`
	ErrorCategory string                 `json:"error_category,omitempty"`
	Decision      ChannelDecisionContext `json:"decision,omitempty"`
}
