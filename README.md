### What is this toy?

It's an EVM debugger and analyzer, made of 3 components:
1. Basic debugger like "gdb", with basic debugging features such as "breakpoints" and "single step".
2. Low level instruction tracer, which logs all EVM operations, printing input/output arguments.
3. High level tracer to make the low level instruction more readable.


### Why this?

This toy was made to:
1. Learn EVM.
2. Learn Symbolic Execution.
3. Predict NFTs :)


### How to "Predict NFTs"?
Many NFTs use very simple algorithm to randomize the minting result, or the upgrading. Here's a demo NFT: 
```solidity
contract NFT {
	uint tokenId;

	function mint() external returns(bool) {
		tokenId ++;
		uint x = uint(keccak256(abi.encodePacked(
			block.timestamp, tokenId, msg.sender))) % 100;
		if (x > 5) {
			return false; // Common class
		} else {
			return true; // Rare class
		}
	}
}
```
It hashes `sha3(block.timestamp, tokenId, msg.sender)` and then `MOD 100`. If the result is less than 5 then it's a "Rare" one, so a user has 95% chance to mint a "Common" token.

But this is predictable, because:
1. The `block.timestamp` increases by 3 every 3 seconds(on BSC).
2. `tokenId` can be read from Storage.
3. `msg.sender` is just wallet address.

Many similar params: `DIFFICULTY`, `NUMBER`, etc.

**To predict this**:
1. Generate 1k+ wallets
2. For each wallet, calculate `sha3(timestamp+6, tokenId, walletAddress) % 100`. Because on BSC, transaction always mined after 2 blocks, which is 6 seconds. Do this on every block.
3. If wallet N got a "Rare" result, first, send enough Ether(or token) to N for minting(with higher gas fee to make sure it runs before the minting TX), and then send minting TX with N.
4. You will always get a "Rare" one.

### Why they still using simple algorithm?

Maybe because there is no perfect decompiler. The best one seems to be "panoramix", that used by "etherscan.com", but it still fails on many contracts.

### How to trace the algo:

1. Download Tx from a Archive Node, save it to json file:

```
>>> tx 0x__transaction_hash__ https://archive-node-rpc-url
```
If the archive node works, it will generate a "0x__transaction_hash__.json"

2. load the json
```
>>> load  0x__transaction_hash__.json
```
3. Start low/high tracer
```
>>> low 
>>> hi 
```
4. Run the Tx
```
>>> c
```
5. After it finishes executing, **op**timize it to high level code
```
>>> op
```

The low level instructions now printed in console and high level code saved to file. For the demo NFT, the result looks like:
![image](https://user-images.githubusercontent.com/4710875/174275874-dd449046-7823-4686-b5dd-f8d5e5d6c0e2.png)

See, what it does is comparing `SHA3(...) % 100` to 5.

It's also possible to stepping through the code and break at `SHA3` to check the memory input, but that's inefficient.

### About "Archive Node"

[Described here](https://geth.ethereum.org/docs/dapp/tracing). The **Archive** means it stores all the historical data, all the input/output memory/stack/gas/... for every single bytecode execution. The server requires much more resource than a normal **FullNode server**. Some provider enables the *tracing api* for visiting those data, but that is costy. 

Fortunately, some providers like nodereal.io/moralis.io/... provide free Archive Node without *tracing api* enabled, that's what we need for *edb*.

**Note**: *edb* doesn't work with Normal FullNode, it has to be **Archive Node**

### Solution
The `blockhash` is "random" enough, but you can't get it in your code because your code executes before it generated. Hence some NFT splits the `mint` into two steps `mint` and `open`. 
- When `mint`, it saves the current block number N
- When `open`, get `blockhash(N)` and use this along with other params to calculate the `SHA3`
You should limit the block number of `open` to the next 256 block since N, because you will alwase get 0 as block hash if it exceeds 256 block, which makes it predictable again. So if a user `open` after 256 block, always give him a "Common" class.

But even so, there're still 256 blocks you can try, means you have 256 chances to `open` a preferrable NFT. In practice, an NFT with 6 level, L1 to L6, we always get L2/L3, L1 is actually very "rare", but that's just the probability of that particular NFT.

Maybe limit it to 128 block or less.

Please fire an issure if you have any good solution.

### Install
Just download the prebuilt executables from release page

#### Or build it yourself:
1. Install Golang >= 1.18.1
2. Clone this repo: `git clone github.com/aj3423/edb`
3. Go to binary dir: `cd edb/main`
4. Run it with `go run .` or `go build .` to build.

### Command List

	help:                    Show this help
	mem [offset [size]]:     Show memory
	sto:                     Show Storage
	s:                       Show Stack items
	p [pc]:                  Show asm at current/target PC
	load [.json]:            Reload current .json file(default: sample.json)
	save [.json]:            Save context to current .json file(default: sample.json)
	tx <tx_hash> <node_url>: Generate .json file from archive node
	low:                     start low level trace
	hi:                      start high level trace
	op:                      Optimize and print result of high-level-trace
	log:                     Log every executed EVM instruction to file
	n:                       Single step
	c:                       Continue
	b l|d|op|pc:             Breakpoint list|delete|by opcode|by pc

### TODO
1. gas calculation (This is much more complex than I thought, pretty hopeless)
