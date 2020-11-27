















package client

import (
	"io"

	"github.com/ethereum/go-ethereum/les/utils"
	"github.com/ethereum/go-ethereum/rlp"
)

const basketFactor = 1000000 









type referenceBasket struct {
	basket    requestBasket
	reqValues []float64 
}










type serverBasket struct {
	basket   requestBasket
	rvFactor float64
}

type (
	
	
	
	requestBasket struct {
		items []basketItem
		exp   uint64
	}
	
	
	
	
	
	basketItem struct {
		amount, value uint64
	}
)



func (b *requestBasket) setExp(exp uint64) {
	if exp > b.exp {
		shift := exp - b.exp
		for i, item := range b.items {
			item.amount >>= shift
			item.value >>= shift
			b.items[i] = item
		}
		b.exp = exp
	}
	if exp < b.exp {
		shift := b.exp - exp
		for i, item := range b.items {
			item.amount <<= shift
			item.value <<= shift
			b.items[i] = item
		}
		b.exp = exp
	}
}



func (s *serverBasket) init(size int) {
	if s.basket.items == nil {
		s.basket.items = make([]basketItem, size)
	}
}



func (s *serverBasket) add(reqType, reqAmount uint32, reqCost uint64, expFactor utils.ExpirationFactor) {
	s.basket.setExp(expFactor.Exp)
	i := &s.basket.items[reqType]
	i.amount += uint64(float64(uint64(reqAmount)*basketFactor) * expFactor.Factor)
	i.value += uint64(float64(reqCost) * s.rvFactor * expFactor.Factor)
}



func (s *serverBasket) updateRvFactor(rvFactor float64) {
	s.rvFactor = rvFactor
}




func (s *serverBasket) transfer(ratio float64) requestBasket {
	res := requestBasket{
		items: make([]basketItem, len(s.basket.items)),
		exp:   s.basket.exp,
	}
	for i, v := range s.basket.items {
		ta := uint64(float64(v.amount) * ratio)
		tv := uint64(float64(v.value) * ratio)
		if ta > v.amount {
			ta = v.amount
		}
		if tv > v.value {
			tv = v.value
		}
		s.basket.items[i] = basketItem{v.amount - ta, v.value - tv}
		res.items[i] = basketItem{ta, tv}
	}
	return res
}



func (r *referenceBasket) init(size int) {
	r.reqValues = make([]float64, size)
	r.normalize()
	r.updateReqValues()
}




func (r *referenceBasket) add(newBasket requestBasket) {
	r.basket.setExp(newBasket.exp)
	
	var (
		totalCost  uint64
		totalValue float64
	)
	for i, v := range newBasket.items {
		totalCost += v.value
		totalValue += float64(v.amount) * r.reqValues[i]
	}
	if totalCost > 0 {
		
		scaleValues := totalValue / float64(totalCost)
		for i, v := range newBasket.items {
			r.basket.items[i].amount += v.amount
			r.basket.items[i].value += uint64(float64(v.value) * scaleValues)
		}
	}
	r.updateReqValues()
}



func (r *referenceBasket) updateReqValues() {
	r.reqValues = make([]float64, len(r.reqValues))
	for i, b := range r.basket.items {
		if b.amount > 0 {
			r.reqValues[i] = float64(b.value) / float64(b.amount)
		} else {
			r.reqValues[i] = 0
		}
	}
}


func (r *referenceBasket) normalize() {
	var sumAmount, sumValue uint64
	for _, b := range r.basket.items {
		sumAmount += b.amount
		sumValue += b.value
	}
	add := float64(int64(sumAmount-sumValue)) / float64(sumValue)
	for i, b := range r.basket.items {
		b.value += uint64(int64(float64(b.value) * add))
		r.basket.items[i] = b
	}
}



func (r *referenceBasket) reqValueFactor(costList []uint64) float64 {
	var (
		totalCost  float64
		totalValue uint64
	)
	for i, b := range r.basket.items {
		totalCost += float64(costList[i]) * float64(b.amount) 
		totalValue += b.value
	}
	if totalCost < 1 {
		return 0
	}
	return float64(totalValue) * basketFactor / totalCost
}


func (b *basketItem) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, []interface{}{b.amount, b.value})
}


func (b *basketItem) DecodeRLP(s *rlp.Stream) error {
	var item struct {
		Amount, Value uint64
	}
	if err := s.Decode(&item); err != nil {
		return err
	}
	b.amount, b.value = item.Amount, item.Value
	return nil
}


func (r *requestBasket) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, []interface{}{r.items, r.exp})
}


func (r *requestBasket) DecodeRLP(s *rlp.Stream) error {
	var enc struct {
		Items []basketItem
		Exp   uint64
	}
	if err := s.Decode(&enc); err != nil {
		return err
	}
	r.items, r.exp = enc.Items, enc.Exp
	return nil
}






func (r requestBasket) convertMapping(oldMapping, newMapping []string, initBasket requestBasket) requestBasket {
	nameMap := make(map[string]int)
	for i, name := range oldMapping {
		nameMap[name] = i
	}
	rc := requestBasket{items: make([]basketItem, len(newMapping))}
	var scale, oldScale, newScale float64
	for i, name := range newMapping {
		if ii, ok := nameMap[name]; ok {
			rc.items[i] = r.items[ii]
			oldScale += float64(initBasket.items[i].amount) * float64(initBasket.items[i].amount)
			newScale += float64(rc.items[i].amount) * float64(initBasket.items[i].amount)
		}
	}
	if oldScale > 1e-10 {
		scale = newScale / oldScale
	} else {
		scale = 1
	}
	for i, name := range newMapping {
		if _, ok := nameMap[name]; !ok {
			rc.items[i].amount = uint64(float64(initBasket.items[i].amount) * scale)
			rc.items[i].value = uint64(float64(initBasket.items[i].value) * scale)
		}
	}
	return rc
}
