package model

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestModelResolveAuthorityLevel(t *testing.T) {
	require.Equal(t, AuthorityLevelFallback, (*Model)(nil).ResolveAuthorityLevel())
	require.Equal(t, AuthorityLevelManual, (&Model{SyncOfficial: 0}).ResolveAuthorityLevel())
	require.Equal(t, AuthorityLevelOfficial, (&Model{SyncOfficial: 1}).ResolveAuthorityLevel())
	require.Equal(t, AuthorityLevelFallback, (&Model{SyncOfficial: 1, AuthorityLevel: AuthorityLevelFallback}).ResolveAuthorityLevel())
}
