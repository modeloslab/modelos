// Copyright (c) 2025-2026 The Pearl Research Labs developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package netsync

import (
	"container/list"
	"math/big"

	"github.com/modelos/modelos/node/blockchain"
)

// findBestBlockMsg scans the message queue for the highest priority blockMsg.
// Returns the chosen element, or the front element if no candidate qualifies.
//
// Priority:
//  1. Parent is current tip (oldest wins).
//  2. Parent is known in block index (hardest difficulty wins).
func findBestBlockMsg(chain *blockchain.BlockChain, queue *list.List) *list.Element {
	if queue.Len() == 1 {
		return queue.Front()
	}
	tipHash := chain.BestSnapshot().Hash

	for e := queue.Front(); e != nil; e = e.Next() {
		bmsg, ok := e.Value.(*blockMsg)
		if !ok {
			continue
		}
		header := bmsg.block.MsgBlock().BlockHeader()
		if header.PrevBlock.IsEqual(&tipHash) {
			return e
		}
	}
	var bestTarget *big.Int
	best := queue.Front()
	for e := queue.Front(); e != nil; e = e.Next() {
		bmsg, ok := e.Value.(*blockMsg)
		if !ok {
			continue
		}
		header := bmsg.block.MsgBlock().BlockHeader()
		target := blockchain.CompactToBig(header.Bits)
		if (bestTarget == nil || target.Cmp(bestTarget) < 0) && chain.BlockInIndex(&header.PrevBlock) {
			best = e
			bestTarget = target
		}
	}
	return best
}
