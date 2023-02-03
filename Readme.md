# Go package for Mamoru Tracer

## Usage
Install the package in the Ethereum based project

```shell
go get github.com/Mamoru-Foundation/geth-mamoru-core-sdk
```

Paste the following code into the Ethereum light client file `go-ethereum/light/lightchain.go`

```go
	//////////////////////////////////////////////////////////////////
	if !geth_mamoru_core_sdk.IsSnifferEnable() {
		return 0, nil
	}
	ctx := context.Background()

	lastBlock, err := lc.GetBlockByNumber(ctx, block.NumberU64())
	if err != nil {
		return 0, err
	}

	parentBlock, err := lc.GetBlockByHash(ctx, block.ParentHash())
	if err != nil {
		return 0, err
	}

	log.Info("Sniffer start", "number", lastBlock.NumberU64())

	stateDb := NewState(ctx, parentBlock.Header(), lc.Odr())
	receipts, err := GetBlockReceipts(ctx, lc.Odr(), lastBlock.Hash(), lastBlock.Number().Uint64())
	if err != nil {
		return 0, err
	}

	geth_mamoru_core_sdk.Trace(ctx,
		tracer.NewTracerConfig(stateDb.Copy(), lc.Config(), lc),
		lastBlock,
		receipts,
	)
	//////////////////////////////////////////////////////////////////
```

Build the project:

```shell
make geth
```

Run the following command to check if it works

```shell
 ./build/bin/geth --syncmode light --goerli
```
