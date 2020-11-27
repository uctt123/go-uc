















package client

import (
	"io"
	"math"
	"time"

	"github.com/ethereum/go-ethereum/les/utils"
	"github.com/ethereum/go-ethereum/rlp"
)

const (
	minResponseTime   = time.Millisecond * 50
	maxResponseTime   = time.Second * 10
	timeStatLength    = 32
	weightScaleFactor = 1000000
)







type (
	ResponseTimeStats struct {
		stats [timeStatLength]uint64
		exp   uint64
	}
	ResponseTimeWeights [timeStatLength]float64
)

var timeStatsLogFactor = (timeStatLength - 1) / (math.Log(float64(maxResponseTime)/float64(minResponseTime)) + 1)



func TimeToStatScale(d time.Duration) float64 {
	if d < 0 {
		return 0
	}
	r := float64(d) / float64(minResponseTime)
	if r > 1 {
		r = math.Log(r) + 1
	}
	r *= timeStatsLogFactor
	if r > timeStatLength-1 {
		return timeStatLength - 1
	}
	return r
}



func StatScaleToTime(r float64) time.Duration {
	r /= timeStatsLogFactor
	if r > 1 {
		r = math.Exp(r - 1)
	}
	return time.Duration(r * float64(minResponseTime))
}






func TimeoutWeights(timeout time.Duration) (res ResponseTimeWeights) {
	for i := range res {
		t := StatScaleToTime(float64(i))
		if t < 2*timeout {
			res[i] = math.Cos(math.Pi / 2 * float64(t) / float64(timeout))
		} else {
			res[i] = -1
		}
	}
	return
}


func (rt *ResponseTimeStats) EncodeRLP(w io.Writer) error {
	enc := struct {
		Stats [timeStatLength]uint64
		Exp   uint64
	}{rt.stats, rt.exp}
	return rlp.Encode(w, &enc)
}


func (rt *ResponseTimeStats) DecodeRLP(s *rlp.Stream) error {
	var enc struct {
		Stats [timeStatLength]uint64
		Exp   uint64
	}
	if err := s.Decode(&enc); err != nil {
		return err
	}
	rt.stats, rt.exp = enc.Stats, enc.Exp
	return nil
}


func (rt *ResponseTimeStats) Add(respTime time.Duration, weight float64, expFactor utils.ExpirationFactor) {
	rt.setExp(expFactor.Exp)
	weight *= expFactor.Factor * weightScaleFactor
	r := TimeToStatScale(respTime)
	i := int(r)
	r -= float64(i)
	rt.stats[i] += uint64(weight * (1 - r))
	if i < timeStatLength-1 {
		rt.stats[i+1] += uint64(weight * r)
	}
}



func (rt *ResponseTimeStats) setExp(exp uint64) {
	if exp > rt.exp {
		shift := exp - rt.exp
		for i, v := range rt.stats {
			rt.stats[i] = v >> shift
		}
		rt.exp = exp
	}
	if exp < rt.exp {
		shift := rt.exp - exp
		for i, v := range rt.stats {
			rt.stats[i] = v << shift
		}
		rt.exp = exp
	}
}



func (rt ResponseTimeStats) Value(weights ResponseTimeWeights, expFactor utils.ExpirationFactor) float64 {
	var v float64
	for i, s := range rt.stats {
		v += float64(s) * weights[i]
	}
	if v < 0 {
		return 0
	}
	return expFactor.Value(v, rt.exp) / weightScaleFactor
}


func (rt *ResponseTimeStats) AddStats(s *ResponseTimeStats) {
	rt.setExp(s.exp)
	for i, v := range s.stats {
		rt.stats[i] += v
	}
}


func (rt *ResponseTimeStats) SubStats(s *ResponseTimeStats) {
	rt.setExp(s.exp)
	for i, v := range s.stats {
		if v < rt.stats[i] {
			rt.stats[i] -= v
		} else {
			rt.stats[i] = 0
		}
	}
}







func (rt ResponseTimeStats) Timeout(failRatio float64) time.Duration {
	var sum uint64
	for _, v := range rt.stats {
		sum += v
	}
	s := uint64(float64(sum) * failRatio)
	i := timeStatLength - 1
	for i > 0 && s >= rt.stats[i] {
		s -= rt.stats[i]
		i--
	}
	r := float64(i) + 0.5
	if rt.stats[i] > 0 {
		r -= float64(s) / float64(rt.stats[i])
	}
	if r < 0 {
		r = 0
	}
	th := StatScaleToTime(r)
	if th > maxResponseTime {
		th = maxResponseTime
	}
	return th
}




type RtDistribution [timeStatLength][2]float64


func (rt ResponseTimeStats) Distribution(normalized bool, expFactor utils.ExpirationFactor) (res RtDistribution) {
	var mul float64
	if normalized {
		var sum uint64
		for _, v := range rt.stats {
			sum += v
		}
		if sum > 0 {
			mul = 1 / float64(sum)
		}
	} else {
		mul = expFactor.Value(float64(1)/weightScaleFactor, rt.exp)
	}
	for i, v := range rt.stats {
		res[i][0] = float64(StatScaleToTime(float64(i))) / float64(time.Second)
		res[i][1] = float64(v) * mul
	}
	return
}
