package edb

import (
	"context"
	"fmt"
	"math/big"

	"github.com/aj3423/edb/util"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/holiman/uint256"
	"github.com/pkg/errors"
)

func get_online_block_hash(
	client *ethclient.Client,
	blockNum uint64,
) (common.Hash, error) {
	var hash common.Hash

	if client == nil {
		return hash, fmt.Errorf("no block hash for: %d", blockNum)
	}
	// color.Blue("get block hash: %d", blockNum)

	block, e := client.BlockByNumber(
		context.Background(),
		new(big.Int).SetUint64(blockNum),
	)
	if e != nil {
		return hash, e
	}
	hash = block.Hash()
	// color.Green("value: %s", hash.Hex())

	return hash, nil
}
func get_online_storage(
	client *ethclient.Client,
	address common.Address,
	slot *uint256.Int,
	blockNum uint64,
) (*uint256.Int, error) {

	if client == nil {
		return nil, fmt.Errorf("no storage for: %s", address.String())
	}
	// color.Blue("get storage slot: %s, contract: %s",
	// 	slot.String(), address.String())

	if address == util.ZeroAddress || blockNum == 0 {
		return nil, errors.New("invalid AddressThis or Block.Number")
	}

	bs, e := client.StorageAt(
		context.Background(),
		address,
		common.BigToHash(slot.ToBig()),
		new(big.Int).SetUint64(blockNum),
	)
	if e != nil {
		return nil, e
	}
	val := uint256.NewInt(0).SetBytes(bs)
	// color.Green("value: %s", val.Hex())

	return val, nil
}

func get_online_code(
	client *ethclient.Client,
	address common.Address,
	blockNum uint64,
) ([]byte, error) {

	if client == nil {
		return nil, fmt.Errorf("no code for: %s", address.String())
	}
	// color.Blue("get contract code, address: %s, blockNum: %d",
	// address.Hex(), blockNum)
	if address == util.ZeroAddress || blockNum == 0 {
		return nil, errors.New("invalid AddressThis or Block.Number")
	}
	code, e := client.CodeAt(
		context.Background(), address, big.NewInt(int64(blockNum)))
	if e != nil {
		return nil, e
	}
	// color.Green("got %d bytes of code", len(code))
	return code, nil
}
func get_online_balance(
	client *ethclient.Client,
	address common.Address,
	blockNum uint64,
) (*big.Int, error) {
	if client == nil {
		return nil, fmt.Errorf("no balance for: %s", address.String())
	}
	// color.Blue("get balance, address: %s",
	// 	address.Hex())
	if address == util.ZeroAddress || blockNum == 0 {
		return nil, errors.New("invalid AddressThis or Block.Number")
	}

	bal, e := client.BalanceAt(context.Background(), address, big.NewInt(int64(blockNum)))
	if e != nil {
		return nil, e
	}

	// color.Green("blance: %s", bal.String())

	return bal, nil
}

func ContextFromTx(
	node_url string,
	tx_hash string,
) (*Context, error) {

	client, e := ethclient.Dial(node_url)
	if e != nil {
		return nil, e
	}

	chain_id, e := client.ChainID(context.Background())
	if e != nil {
		return nil, e
	}

	tx, _, e := client.TransactionByHash(context.Background(), common.HexToHash(tx_hash))
	if e != nil {
		return nil, e
	}
	receipt, e := client.TransactionReceipt(context.Background(), common.HexToHash(tx_hash))
	if e != nil {
		return nil, e
	}
	msg, e := tx.AsMessage(types.NewLondonSigner(big.NewInt(int64(chain_id.Uint64()))), nil)
	if e != nil {
		return nil, e
	}
	blockNum := receipt.BlockNumber
	block, e := client.BlockByNumber(context.Background(), blockNum)
	if e != nil {
		return nil, e
	}

	ctx := NewContext()

	ctx.ethClient = client

	ctx.Tx = Tx{
		Hash:     tx.Hash(),
		Origin:   msg.From(),
		GasPrice: tx.GasPrice().Uint64(),
	}
	baseFee := big.NewInt(0)
	if block.BaseFee() != nil {
		baseFee = block.BaseFee()
	}
	ctx.Block = Block{
		Number:     block.NumberU64(),
		Timestamp:  block.Time(),
		Difficulty: block.Difficulty().Uint64(),
		Coinbase:   block.Coinbase(),
		GasLimit:   block.GasLimit(),
		BaseFee:    baseFee.Uint64(),
	}
	ctx.BlockHashes[block.NumberU64()] = block.Hash()

	ctx.Chain = Chain{
		Id:      chain_id.Uint64(),
		NodeUrl: node_url,
	}
	to := *tx.To()

	ctx.Call().This = to
	ctx.Call().Msg = Msg{
		Data:   tx.Data(),
		Gas:    tx.Gas(),
		Sender: msg.From(),
		Value:  msg.Value(),
	}

	_, e = ensure_code(ctx, to)
	if e != nil {
		return nil, e
	}

	return ctx, nil
}

// get *Contract at addr, create if not exists
func ensure_contract_at(ctx *Context, addr common.Address) *Contract {
	contract, ok := ctx.Contracts[addr]
	if !ok {
		contract = NewContract()
		ctx.Contracts[addr] = contract
	}
	return contract
}

// get from local map first
// fetch online if not exists
func ensure_balance(ctx *Context, address common.Address) (*big.Int, error) {

	contract := ensure_contract_at(ctx, address)

	if contract.Balance != nil { // if code exists in local cache
		return contract.Balance, nil
	}

	bal, e := get_online_balance(ctx.ethClient, address, ctx.Block.Number-1) // block - 1
	if e != nil {
		return nil, e
	}

	// cache it
	contract.Balance = bal

	return bal, nil
}

// get from local map first
// fetch online if not exists
func ensure_code(ctx *Context, address common.Address) ([]byte, error) {
	var binary []byte

	contract := ensure_contract_at(ctx, address)

	if len(contract.Code.Binary) > 0 { // if code exists in local cache
		return contract.Code.Binary, nil
	}

	var e error
	binary, e = get_online_code(ctx.ethClient, address, ctx.Block.Number)
	if e != nil {
		return nil, e
	}

	// cache it
	if e := contract.Code.Set(binary); e != nil {
		return nil, e
	}

	return binary, nil
}

// get from local map first
// fetch online if not exists
func ensure_storage(
	ctx *Context,
	address common.Address,
	slot *uint256.Int,
) (*uint256.Int, error) {

	contract := ensure_contract_at(ctx, address)

	slotHash := common.BigToHash(slot.ToBig())

	val, ok := contract.Storage[slotHash]
	if ok { // if code exists in local cache
		return val, nil
	}

	var e error
	// from archive node we only get storage values after the block executed
	// so we need to query block-1 to get the storage before executed
	val, e = get_online_storage(
		ctx.ethClient, address, slot, ctx.Block.Number-1) // block.number-1
	if e != nil {
		return nil, e
	}

	// cache it
	contract.Storage[slotHash] = val

	return val, nil
}

// get from local map first
// fetch online if not exists
func ensure_block_hash(
	ctx *Context,
	blockNum uint64,
) (common.Hash, error) {

	hash, ok := ctx.BlockHashes[blockNum]
	if ok { // if code exists in local cache
		return hash, nil
	}

	var e error
	hash, e = get_online_block_hash(ctx.ethClient, blockNum)
	if e != nil {
		return hash, e
	}

	// cache it
	ctx.BlockHashes[blockNum] = hash

	return hash, nil
}
