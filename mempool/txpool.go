package mempool

import (
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/event"
)

type TxPool interface {
	SubscribeNewTxsEvent(chan<- core.NewTxsEvent) event.Subscription
}
