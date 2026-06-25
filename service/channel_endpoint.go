package service

import (
	"strings"

	"github.com/QuantumNous/new-api/model"
)

type SelectedChannelEndpoint struct {
	Id    int
	URL   string
	Label string
}

func SelectChannelEndpoint(channel *model.Channel) (*SelectedChannelEndpoint, bool, error) {
	if channel == nil {
		return nil, false, nil
	}
	endpoints, err := model.GetEnabledChannelEndpoints(channel.Id)
	if err != nil {
		return nil, false, err
	}
	if len(endpoints) == 0 {
		return nil, false, nil
	}
	endpoint := endpoints[0]
	return &SelectedChannelEndpoint{
		Id:    endpoint.Id,
		URL:   strings.TrimSpace(endpoint.URL),
		Label: strings.TrimSpace(endpoint.Label),
	}, true, nil
}
