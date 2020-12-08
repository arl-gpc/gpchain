package blockRequestApi

import (
	"fmt"
	"sync"
	"bytes"
	"path"
	"crypto"
	"strconv"
	"net/http"
	"crypto/rsa"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"github.com/willf/bloom"
	"blockchain-service/blockchain"
	"blockchain-service/relay/relayTypes"
)


//Implements crypto.SignerOpt interface (needed to sign realyBlock)
type signerOpt struct {
	h crypto.Hash
}

func (so signerOpt) HashFunc() crypto.Hash {
	return so.h
}

//n : number of items in bloom filter, p : probability of false positives, m : number of bits in the filter, k : number of hash functions
const (
	n = uint(1000)
	p = 0.000001
)

var sdkLock *sync.Mutex
var fSetup *blockchain.FabricSetup
var rsaKey *rsa.PrivateKey

func createChain(previousBlock *relayTypes.RelayBlock, filter *bloom.BloomFilter, current, stop uint64) (*relayTypes.RelayBlock, error) {
	var currentBlock relayTypes.RelayBlock
	currentBlock.Index = current
	if previousBlock != nil {
		currentBlock.PreviousBlockHash = previousBlock.Hash()
	} else {
		currentBlock.PreviousBlockHash = []byte("")
	}

	blockMerkleTree, revocations, err  := relayTypes.ProcessBlock(current + blockchain.BlockOffset, sdkLock, fSetup)
	if err != nil {
		fmt.Printf("Could not update relay state: %s\n", err)
		return nil, err
	}

	blockRoot := blockMerkleTree.CurrentRoot().Hash()
	currentBlock.BlockMerkleRoot = blockRoot

	//Add Revocation to Bloom Filter
	if revocations != nil {
		for _,revocation := range *revocations {
			sum := sha256.Sum256(revocation)
			filter.Add(sum[:])
		}

		//Convert Bloom Filter to []byte
		filterBuffer := bytes.NewBuffer([]byte{})
		_, err = filter.WriteTo(filterBuffer)
		if err != nil {
			fmt.Printf("Error: Could not write bloom filter: %v\n", err)
			return nil, err
		}
		filterBufferHash := sha256.Sum256(filterBuffer.Bytes())
		currentBlock.BloomFilterHash = filterBufferHash[:]
	} else {
		currentBlock.BloomFilterHash = []byte("")
	}
	
	if current == stop {
		return &currentBlock, nil
	}

	return createChain(&currentBlock, filter, current+1, stop)
}

func getRelayBlock(w http.ResponseWriter, r *http.Request) {
	blkNumStr := r.URL.Query()["blockNumber"]
	if len(blkNumStr) == 0 {
		fmt.Fprintf(w, "Block Number not Provided!\n")
		return
	}
	
	isNum, err := path.Match("[0-9]*", blkNumStr[0])
	if !isNum || err != nil {
		fmt.Fprintf(w, "Block Number is not a Number!\n")
		return
	}

	blkNum, err := strconv.ParseUint(blkNumStr[0], 10, 64)
	if err != nil {
		fmt.Fprintf(w, "Could not parse provided block number to type uint64\n")
		return
	}
	filter := bloom.NewWithEstimates(n, p)
	relayBlk, err := createChain(nil, filter, uint64(0), blkNum)
	if err != nil {
		fmt.Fprintf(w, "Could compute relayblock: %s\n", err)
		return
	}

	// RSA sig of block
	signedRelayBlock, err := rsaKey.Sign(rand.Reader, relayBlk.Hash(), signerOpt{crypto.SHA256}) 
	if err != nil {
		fmt.Printf("Could not sign relay block: %s\n", err)
		return
	}
	
	// Create Realy Block Message
	relayBlkMsg := relayTypes.RelayBlockMessage{*relayBlk, [][]byte{signedRelayBlock}, relayBlk.Hash()}
	relayBlkMsgStr, err := json.Marshal(relayBlkMsg)
	if err != nil {
		fmt.Printf("Could not marshal relay block message: %s\n", err)
		return
	}

	fmt.Fprintf(w, string(relayBlkMsgStr))
	return
}

func getCurrentHeight(w http.ResponseWriter, r *http.Request) {
	bci, err := fSetup.GetLedgerInfo()
	if err != nil {
		fmt.Fprintf(w, "Could not get max height!\n")
		return
	}
	fmt.Fprintf(w, "%d", bci.BCI.GetHeight()-2)
	return
}

func StartBlockRequestListener(m *sync.Mutex, f *blockchain.FabricSetup, key rsa.PrivateKey, stop, done chan bool) {
	sdkLock = m
	fSetup = f
	rsaKey = &key
	defer func () {
		done <- true
	}()

	go func() {
		httpServeMux := http.NewServeMux()
		httpServeMux.HandleFunc("/blocks", getRelayBlock)
		httpServeMux.HandleFunc("/currentHeight", getCurrentHeight)
		fmt.Printf("Block Request Listener Started on Port :8081\n")
		http.ListenAndServe(":8081", httpServeMux)
	}()

	<-stop
}
