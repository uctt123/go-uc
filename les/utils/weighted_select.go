















package utils

import (
	"math/rand"
)

type (
	
	WeightedRandomSelect struct {
		root *wrsNode
		idx  map[WrsItem]int
		wfn  WeightFn
	}
	WrsItem  interface{}
	WeightFn func(interface{}) uint64
)


func NewWeightedRandomSelect(wfn WeightFn) *WeightedRandomSelect {
	return &WeightedRandomSelect{root: &wrsNode{maxItems: wrsBranches}, idx: make(map[WrsItem]int), wfn: wfn}
}



func (w *WeightedRandomSelect) Update(item WrsItem) {
	w.setWeight(item, w.wfn(item))
}


func (w *WeightedRandomSelect) Remove(item WrsItem) {
	w.setWeight(item, 0)
}


func (w *WeightedRandomSelect) IsEmpty() bool {
	return w.root.sumWeight == 0
}


func (w *WeightedRandomSelect) setWeight(item WrsItem, weight uint64) {
	idx, ok := w.idx[item]
	if ok {
		w.root.setWeight(idx, weight)
		if weight == 0 {
			delete(w.idx, item)
		}
	} else {
		if weight != 0 {
			if w.root.itemCnt == w.root.maxItems {
				
				newRoot := &wrsNode{sumWeight: w.root.sumWeight, itemCnt: w.root.itemCnt, level: w.root.level + 1, maxItems: w.root.maxItems * wrsBranches}
				newRoot.items[0] = w.root
				newRoot.weights[0] = w.root.sumWeight
				w.root = newRoot
			}
			w.idx[item] = w.root.insert(item, weight)
		}
	}
}





func (w *WeightedRandomSelect) Choose() WrsItem {
	for {
		if w.root.sumWeight == 0 {
			return nil
		}
		val := uint64(rand.Int63n(int64(w.root.sumWeight)))
		choice, lastWeight := w.root.choose(val)
		weight := w.wfn(choice)
		if weight != lastWeight {
			w.setWeight(choice, weight)
		}
		if weight >= lastWeight || uint64(rand.Int63n(int64(lastWeight))) < weight {
			return choice
		}
	}
}

const wrsBranches = 8 


type wrsNode struct {
	items                    [wrsBranches]interface{}
	weights                  [wrsBranches]uint64
	sumWeight                uint64
	level, itemCnt, maxItems int
}


func (n *wrsNode) insert(item WrsItem, weight uint64) int {
	branch := 0
	for n.items[branch] != nil && (n.level == 0 || n.items[branch].(*wrsNode).itemCnt == n.items[branch].(*wrsNode).maxItems) {
		branch++
		if branch == wrsBranches {
			panic(nil)
		}
	}
	n.itemCnt++
	n.sumWeight += weight
	n.weights[branch] += weight
	if n.level == 0 {
		n.items[branch] = item
		return branch
	}
	var subNode *wrsNode
	if n.items[branch] == nil {
		subNode = &wrsNode{maxItems: n.maxItems / wrsBranches, level: n.level - 1}
		n.items[branch] = subNode
	} else {
		subNode = n.items[branch].(*wrsNode)
	}
	subIdx := subNode.insert(item, weight)
	return subNode.maxItems*branch + subIdx
}



func (n *wrsNode) setWeight(idx int, weight uint64) uint64 {
	if n.level == 0 {
		oldWeight := n.weights[idx]
		n.weights[idx] = weight
		diff := weight - oldWeight
		n.sumWeight += diff
		if weight == 0 {
			n.items[idx] = nil
			n.itemCnt--
		}
		return diff
	}
	branchItems := n.maxItems / wrsBranches
	branch := idx / branchItems
	diff := n.items[branch].(*wrsNode).setWeight(idx-branch*branchItems, weight)
	n.weights[branch] += diff
	n.sumWeight += diff
	if weight == 0 {
		n.itemCnt--
	}
	return diff
}


func (n *wrsNode) choose(val uint64) (WrsItem, uint64) {
	for i, w := range n.weights {
		if val < w {
			if n.level == 0 {
				return n.items[i].(WrsItem), n.weights[i]
			}
			return n.items[i].(*wrsNode).choose(val)
		}
		val -= w
	}
	panic(nil)
}
