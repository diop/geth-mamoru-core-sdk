package mempool

import (
	"context"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/event"
	"github.com/ethereum/go-ethereum/light"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/params"

	mamoru "github.com/Mamoru-Foundation/geth-mamoru-core-sdk"
	"github.com/Mamoru-Foundation/geth-mamoru-core-sdk/call_tracer"
)

type lightBlockChain interface {
	core.ChainContext

	GetBlockByHash(context.Context, common.Hash) (*types.Block, error)
	CurrentHeader() *types.Header
	Odr() light.OdrBackend

	SubscribeChainHeadEvent(ch chan<- core.ChainHeadEvent) event.Subscription
}

type LightSnifferBackend struct {
	txPool      TxPool
	chain       lightBlockChain
	chainConfig *params.ChainConfig

	newHeadEvent chan core.ChainHeadEvent
	newTxsEvent  chan core.NewTxsEvent

	TxSub   event.Subscription
	headSub event.Subscription

	ctx context.Context

	sniffer *mamoru.Sniffer
}

func NewLightSniffer(ctx context.Context, txPool TxPool, chain lightBlockChain, chainConfig *params.ChainConfig) *LightSnifferBackend {
	sb := &LightSnifferBackend{
		txPool:       txPool,
		chain:        chain,
		chainConfig:  chainConfig,
		newHeadEvent: make(chan core.ChainHeadEvent, 10),
		newTxsEvent:  make(chan core.NewTxsEvent, 1024),

		ctx: ctx,

		sniffer: mamoru.NewSniffer(),
	}
	sb.headSub = sb.SubscribeChainHeadEvent(sb.newHeadEvent)
	sb.TxSub = sb.SubscribeNewTxsEvent(sb.newTxsEvent)

	go sb.SnifferLoop()

	return sb
}

func (bc *LightSnifferBackend) SubscribeNewTxsEvent(ch chan<- core.NewTxsEvent) event.Subscription {
	return bc.txPool.SubscribeNewTxsEvent(ch)
}

// SubscribeChainHeadEvent registers a subscription of ChainHeadEvent.
func (bc *LightSnifferBackend) SubscribeChainHeadEvent(ch chan<- core.ChainHeadEvent) event.Subscription {
	return bc.chain.SubscribeChainHeadEvent(ch)
}

func (bc *LightSnifferBackend) SnifferLoop() {
	ctx, cancel := context.WithCancel(bc.ctx)

	defer func() {
		bc.headSub.Unsubscribe()
		bc.TxSub.Unsubscribe()
	}()

	for {
		select {
		case <-bc.ctx.Done():
		case <-bc.headSub.Err():
		case <-bc.TxSub.Err():
			cancel()
			return

		case newHead := <-bc.newHeadEvent:
			if newHead.Block != nil {
				go bc.processHead(ctx, newHead.Block.Header())
			}
		}
	}
}

func (bc *LightSnifferBackend) processHead(ctx context.Context, head *types.Header) {
	if ctx.Err() != nil || !bc.sniffer.CheckRequirements() {
		return
	}

	log.Info("Mamoru LightTxPool Sniffer start", "number", head.Number.Uint64(), "ctx", mamoru.CtxLightTxpool)
	startTime := time.Now()

	// Create tracer context
	tracer := mamoru.NewTracer(mamoru.NewFeed(bc.chainConfig))
	// Set tracer context Txpool
	tracer.SetTxpoolCtx()

	parentBlock, err := bc.chain.GetBlockByHash(ctx, head.ParentHash)
	if err != nil {
		log.Error("Mamoru parent block", "number", head.Number.Uint64(), "err", err, "ctx", mamoru.CtxLightTxpool)
		return
	}

	stateDb := light.NewState(ctx, parentBlock.Header(), bc.chain.Odr())

	newBlock, err := bc.chain.GetBlockByHash(ctx, head.Hash())
	if err != nil {
		log.Error("Mamoru current block", "number", head.Number.Uint64(), "err", err, "ctx", mamoru.CtxLightTxpool)
		return
	}
	tracer.FeedBlock(newBlock)

	callFrames, err := call_tracer.TraceBlock(ctx, call_tracer.NewTracerConfig(stateDb.Copy(), bc.chainConfig, bc.chain), newBlock)
	if err != nil {
		log.Error("Mamoru block trace", "number", head.Number.Uint64(), "err", err, "ctx", mamoru.CtxLightTxpool)
		return
	}

	for _, call := range callFrames {
		result := call.Result
		tracer.FeedCalTraces(result, head.Number.Uint64())
	}

	receipts, err := light.GetBlockReceipts(ctx, bc.chain.Odr(), newBlock.Hash(), newBlock.NumberU64())
	if err != nil {
		log.Error("Mamoru block receipt", "number", head.Number.Uint64(), "err", err, "ctx", mamoru.CtxLightTxpool)
		return
	}

	tracer.FeedTransactions(newBlock.Number(), newBlock.Transactions(), receipts)
	tracer.FeedEvents(receipts)

	// finish tracer context
	tracer.Send(startTime, newBlock.Number(), newBlock.Hash(), mamoru.CtxLightTxpool)
}
