package service

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
)

func TestSelectChannelEndpointFallsBackWhenPoolEmpty(t *testing.T) {
	oldDB := model.DB
	model.DB = nil
	t.Cleanup(func() { model.DB = oldDB })

	channel := &model.Channel{Id: 42}
	endpoint, found, err := SelectChannelEndpoint(channel)

	require.NoError(t, err)
	require.False(t, found)
	require.Nil(t, endpoint)
}
