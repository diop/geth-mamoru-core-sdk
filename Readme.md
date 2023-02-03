# geth-mamoru-core-sdk

## Usage
Install the package in the Ethereum based project

```shell
go get github.com/Mamoru-Foundation/geth-mamoru-core-sdk
```

Add to import `geth-mamoru-core-sdk` in file `go-ethereum/light/lightchain.go`

```go
import (
	...
	geth_mamoru_core_sdk "github.com/Mamoru-Foundation/geth-mamoru-core-sdk"
	"github.com/Mamoru-Foundation/geth-mamoru-core-sdk/tracer"
	...
```

Paste the following code into the Ethereum light client file `go-ethereum/light/lightchain.go`

```go
	//////////////////////////////////////////////////////////////////
	//Check if sniffer is enabled
	if !geth_mamoru_core_sdk.IsSnifferEnable() {
		return 0, nil
	}
	ctx := context.Background()
    
	//Collecting data about the last block
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
    
	//Starting the trace handler
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

Export Mamoru Environment Variables

```shell
export MAMORU_CHAIN_TYPE=<chain-type>
export MAMORU_SNIFFER_ENABLE=true
export MAMORU_PRIVATE_KEY=<private-key>
export MAMORU_CHAIN_ID=<validations-chain-id>
export MAMORU_ENDPOINT="https://validation-chain-url"
```


Run the following command to check if it works

```shell
 ./build/bin/geth --syncmode light --goerli
```

It's really easy, enjoy