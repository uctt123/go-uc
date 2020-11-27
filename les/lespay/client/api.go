















package client

import (
	"time"

	"github.com/ethereum/go-ethereum/common/mclock"
	"github.com/ethereum/go-ethereum/les/utils"
	"github.com/ethereum/go-ethereum/p2p/enode"
)


type PrivateClientAPI struct {
	vt *ValueTracker
}


func NewPrivateClientAPI(vt *ValueTracker) *PrivateClientAPI {
	return &PrivateClientAPI{vt}
}


func parseNodeStr(nodeStr string) (enode.ID, error) {
	if id, err := enode.ParseID(nodeStr); err == nil {
		return id, nil
	}
	if node, err := enode.Parse(enode.ValidSchemes, nodeStr); err == nil {
		return node.ID(), nil
	} else {
		return enode.ID{}, err
	}
}



func (api *PrivateClientAPI) RequestStats() []RequestStatsItem {
	return api.vt.RequestStats()
}







func (api *PrivateClientAPI) Distribution(nodeStr string, normalized bool) (RtDistribution, error) {
	var expFactor utils.ExpirationFactor
	if !normalized {
		expFactor = utils.ExpFactor(api.vt.StatsExpirer().LogOffset(mclock.Now()))
	}
	if nodeStr == "" {
		return api.vt.RtStats().Distribution(normalized, expFactor), nil
	}
	if id, err := parseNodeStr(nodeStr); err == nil {
		return api.vt.GetNode(id).RtStats().Distribution(normalized, expFactor), nil
	} else {
		return RtDistribution{}, err
	}
}








func (api *PrivateClientAPI) Timeout(nodeStr string, failRate float64) (float64, error) {
	if nodeStr == "" {
		return float64(api.vt.RtStats().Timeout(failRate)) / float64(time.Second), nil
	}
	if id, err := parseNodeStr(nodeStr); err == nil {
		return float64(api.vt.GetNode(id).RtStats().Timeout(failRate)) / float64(time.Second), nil
	} else {
		return 0, err
	}
}



func (api *PrivateClientAPI) Value(nodeStr string, timeout float64) (float64, error) {
	wt := TimeoutWeights(time.Duration(timeout * float64(time.Second)))
	expFactor := utils.ExpFactor(api.vt.StatsExpirer().LogOffset(mclock.Now()))
	if nodeStr == "" {
		return api.vt.RtStats().Value(wt, expFactor), nil
	}
	if id, err := parseNodeStr(nodeStr); err == nil {
		return api.vt.GetNode(id).RtStats().Value(wt, expFactor), nil
	} else {
		return 0, err
	}
}
