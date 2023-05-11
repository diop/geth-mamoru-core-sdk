package mempool

import (
	"context"
	"math/big"

	"sync"
	"time"

	mamoru "github.com/Mamoru-Foundation/geth-mamoru-core-sdk"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/txpool"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/event"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/params"
)

type blockChain interface {
	core.ChainContext
	CurrentBlock() *types.Header
	GetBlock(hash common.Hash, number uint64) *types.Block
	StateAt(root common.Hash) (*state.StateDB, error)
	State() (*state.StateDB, error)

	SubscribeChainHeadEvent(ch chan<- core.ChainHeadEvent) event.Subscription
	SubscribeChainEvent(ch chan<- core.ChainEvent) event.Subscription
	SubscribeChainSideEvent(ch chan<- core.ChainSideEvent) event.Subscription
}

type SnifferBackend struct {
	txPool      TxPool
	chain       blockChain
	chainConfig *params.ChainConfig
	feeder      mamoru.Feeder

	newHeadEvent chan core.ChainHeadEvent
	newTxsEvent  chan core.NewTxsEvent

	chEv     chan core.ChainEvent
	chSideEv chan core.ChainSideEvent

	TxSub   event.Subscription
	headSub event.Subscription

	chEvSub     event.Subscription
	chSideEvSub event.Subscription

	ctx context.Context
	mu  sync.RWMutex
}

func NewSniffer(ctx context.Context, txPool TxPool, chain blockChain, chainConfig *params.ChainConfig, feeder mamoru.Feeder) *SnifferBackend {
	sb := &SnifferBackend{
		txPool:      txPool,
		chain:       chain,
		chainConfig: chainConfig,

		newTxsEvent:  make(chan core.NewTxsEvent, txpool.DefaultConfig.GlobalQueue),
		newHeadEvent: make(chan core.ChainHeadEvent, 10),

		chEv:     make(chan core.ChainEvent, 10),
		chSideEv: make(chan core.ChainSideEvent, 10),

		feeder: feeder,

		ctx: ctx,
		mu:  sync.RWMutex{},
	}
	sb.TxSub = sb.SubscribeNewTxsEvent(sb.newTxsEvent)
	sb.headSub = sb.SubscribeChainHeadEvent(sb.newHeadEvent)
	sb.chEvSub = sb.SubscribeChainEvent(sb.chEv)
	sb.chSideEvSub = sb.SubscribeChainSideEvent(sb.chSideEv)

	return sb
}

func (bc *SnifferBackend) SubscribeNewTxsEvent(ch chan<- core.NewTxsEvent) event.Subscription {
	return bc.txPool.SubscribeNewTxsEvent(ch)
}

// SubscribeChainHeadEvent registers a subscription of ChainHeadEvent.
func (bc *SnifferBackend) SubscribeChainHeadEvent(ch chan<- core.ChainHeadEvent) event.Subscription {
	return bc.chain.SubscribeChainHeadEvent(ch)
}

func (bc *SnifferBackend) SubscribeChainEvent(ch chan<- core.ChainEvent) event.Subscription {
	return bc.chain.SubscribeChainEvent(ch)
}

func (bc *SnifferBackend) SubscribeChainSideEvent(ch chan<- core.ChainSideEvent) event.Subscription {
	return bc.chain.SubscribeChainSideEvent(ch)
}

func (bc *SnifferBackend) SnifferLoop() {
	defer func() {
		bc.TxSub.Unsubscribe()
		bc.headSub.Unsubscribe()
		bc.chEvSub.Unsubscribe()
		bc.chSideEvSub.Unsubscribe()
	}()

	ctx, cancel := context.WithCancel(bc.ctx)
	var header = bc.chain.CurrentBlock()

	for {
		select {
		case <-bc.ctx.Done():
		case <-bc.TxSub.Err():
		case <-bc.headSub.Err():
		case <-bc.chEvSub.Err():
			cancel()
			return

		case newTx := <-bc.newTxsEvent:
			go bc.process(ctx, header, newTx.Txs)

		case newHead := <-bc.newHeadEvent:
			if newHead.Block != nil && newHead.Block.NumberU64() > header.Number.Uint64() {
				log.Info("New core.ChainHeadEvent", "number", newHead.Block.NumberU64(), "ctx", mamoru.CtxTxpool)
				bc.mu.RLock()
				header = newHead.Block.Header()
				bc.mu.RUnlock()
			}

		case newChEv := <-bc.chEv:
			if newChEv.Block != nil && newChEv.Block.NumberU64() > header.Number.Uint64() {
				log.Info("New core.ChainEvent", "number", newChEv.Block.NumberU64(), "ctx", mamoru.CtxTxpool)
				bc.mu.RLock()
				header = newChEv.Block.Header()
				bc.mu.RUnlock()
			}

		case newChSideEv := <-bc.chSideEv:
			if newChSideEv.Block != nil && newChSideEv.Block.NumberU64() > header.Number.Uint64() {
				log.Info("New core.ChainSideEvent", "number", newChSideEv.Block.NumberU64(), "ctx", mamoru.CtxTxpool)
				bc.mu.RLock()
				header = newChSideEv.Block.Header()
				bc.mu.RUnlock()
			}

		}
	}
}

func (bc *SnifferBackend) process(ctx context.Context, header *types.Header, txs types.Transactions) {
	if !mamoru.IsSnifferEnable() || !mamoru.Connect() || ctx.Err() != nil {
		return
	}

	log.Info("Mamoru TxPool Sniffer start", "txs", txs.Len(), "number", header.Number.Uint64(), "ctx", mamoru.CtxTxpool)
	startTime := time.Now()

	// Create tracer context
	tracer := mamoru.NewTracer(bc.feeder)

	var receipts types.Receipts

	stateDb, err := bc.chain.StateAt(header.Root)
	if err != nil {
		log.Error("Mamoru State", "err", err, "ctx", mamoru.CtxTxpool)
	}

	stateDb = stateDb.Copy()

	for index, tx := range txs {
		calltracer, err := mamoru.NewCallTracer(false)
		if err != nil {
			log.Error("Mamoru Call tracer", "err", err, "ctx", mamoru.CtxTxpool)
		}

		chCtx := core.ChainContext(bc.chain)
		author, _ := types.LatestSigner(bc.chainConfig).Sender(tx)
		gasPool := new(core.GasPool).AddGas(tx.Gas())

		var gasUsed = new(uint64)
		*gasUsed = header.GasUsed

		stateDb.SetTxContext(tx.Hash(), index)
		from, err := types.Sender(types.LatestSigner(bc.chainConfig), tx)
		if err != nil {
			log.Error("types.Sender", "err", err, "number", header.Number.Uint64(), "ctx", mamoru.CtxTxpool)
		}
		if tx.Nonce() > stateDb.GetNonce(from) {
			stateDb.SetNonce(from, tx.Nonce())
		}
		log.Info("ApplyTransaction", "tx.hash", tx.Hash().String(), "tx.nonce", tx.Nonce(),
			"stNonce", stateDb.GetNonce(from), "number", header.Number.Uint64(), "ctx", mamoru.CtxTxpool)

		receipt, err := core.ApplyTransaction(bc.chainConfig, chCtx, &author, gasPool, stateDb, header, tx,
			gasUsed, vm.Config{Debug: true, Tracer: calltracer, NoBaseFee: true})
		if err != nil {
			log.Error("Mamoru Apply Transaction", "err", err, "number", header.Number.Uint64(),
				"tx.hash", tx.Hash().String(), "ctx", mamoru.CtxTxpool)
			break
		}

		cleanReceiptAndLogs(receipt)

		receipts = append(receipts, receipt)

		callFrames, err := calltracer.GetResult()
		if err != nil {
			log.Error("Mamoru tracer result", "err", err, "number", header.Number.Uint64(),
				"ctx", mamoru.CtxTxpool)
			break
		}

		tracer.FeedCalTraces(callFrames, header.Number.Uint64())
	}

	//tracer.FeedBlock(block)
	tracer.FeedTransactions(header.Number, txs, receipts)
	tracer.FeedEvents(receipts)
	tracer.Send(startTime, header.Number, header.Hash(), mamoru.CtxTxpool)
}

func cleanReceiptAndLogs(receipt *types.Receipt) {
	receipt.BlockNumber = big.NewInt(0)
	receipt.BlockHash = common.Hash{}
	for _, l := range receipt.Logs {
		l.BlockNumber = 0
		l.BlockHash = common.Hash{}
	}
}