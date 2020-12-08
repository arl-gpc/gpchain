package relayTypes

import (
	"fmt"
	"sync"
	"bytes"
	"errors"
	"strings"
	"net/url"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"

	"github.com/google/trillian"
    	"github.com/google/trillian/merkle"
	"github.com/google/trillian/merkle/hashers"

	"blockchain-service/blockchain"
)

type BloomMessage struct {
	Index uint64 `json:"index"` // Relay block index commiting to this bloom filter
	Filter []byte `json:"filter"` // Byte repersenation of bloom filter
}

type RelayBlock struct {
	Index uint64 `json:"index"`
	BlockMerkleRoot []byte `json:"root"` //Root of block merkle tree
	BloomFilterHash []byte `json:"bloom"` // Hash of bloomfilter bytes
	PreviousBlockHash []byte `json:"previous"`// Hash of previous relay block
}

type RelayBlockMessage struct {
	Block RelayBlock `json:"block"`
	SigList [][]byte `json:"siglist"` // RSA_SIG(SHA256(relayBlock))
	BlockHash []byte `json:"blockhash"`
}

// block bytes = [4 bytes for index] + [Merkle root as bytes] + [Bloom filter hash as bytes] + [Previous block hash as bytes]
func (rb *RelayBlock) Bytes() []byte {
	var blockData []byte
	indexAsBytes := bytes.NewBuffer([]byte{})
	binary.Write(indexAsBytes, binary.BigEndian, uint32(rb.Index))
	blockData = append(blockData, indexAsBytes.Bytes()...)
	blockData = append(blockData, rb.BlockMerkleRoot...)
	blockData = append(blockData, rb.BloomFilterHash...)
	blockData = append(blockData, rb.PreviousBlockHash...)
	return blockData
}

// block hash = sha256(block bytes)
func (rb *RelayBlock) Hash() []byte {
	sum := sha256.Sum256(rb.Bytes())
	return sum[:]
}

// Returns a block level merkle tree and a list of revocations for fabric block n
func ProcessBlock(n uint64, sdkLock *sync.Mutex, fSetup *blockchain.FabricSetup) (*merkle.InMemoryMerkleTree, *[][]byte, error) {
	//Get Block Information
	sdkLock.Lock()
	block, err := fSetup.GetBlock(n)
	sdkLock.Unlock()
	if err != nil {
		fmt.Printf("Could not handle block event: %s", err)
		return nil, nil, err
	}

	var revocations [][]byte
	var blockMerkleTree *merkle.InMemoryMerkleTree

	//If n == blockchain.BlockOffset, then the block being processed is the block published when the chaincode was instantiated. Else, standard block is being processed.
	if n != blockchain.BlockOffset {
		//Init hasher used by merkle tree
		strategy, ok := trillian.HashStrategy_value[*blockchain.HashStrategyFlag]
		if !ok {
			fmt.Printf("Unknown hash strategy: %s", *blockchain.HashStrategyFlag)
			return nil, nil, err
		}

		logHasher, err := hashers.NewLogHasher(trillian.HashStrategy(strategy))
		if err != nil {
			fmt.Printf("Could Not Create Log Hasher: %v\n", err)
			return nil, nil, err
		}

		//Init merkle tree
		blockMerkleTree = merkle.NewInMemoryMerkleTree(logHasher)

		for index, valid := range block.Metadata.Metadata[2] {
			if valid != 0 {
				//If tx was not accepted by peer, continue to next transaction
				fmt.Printf("Skipping Invalid Transaction\n")
				continue
			}

			for _, write := range block.Transactions[index].Writes{			
				var revokeJson [][]byte 
				var temp *blockchain.ProofFile
				//fmt.Printf("Write Set: %+v\n", write)
				
				//Parse Merkle Roots add to Block Merkle Tree
				rootString, err := url.QueryUnescape(write.KvRwSet.Writes[0].Key)
				if err != nil {
					fmt.Printf("Could not handle block event: %s\n", err)
					return nil, nil, err
				}
				fmt.Printf("relayTypes.go rootString = %s\n", rootString)
				blockMerkleTree.AddLeaf([]byte(rootString))
				
				//Parse Revocations and add to list
				if err = json.Unmarshal(write.KvRwSet.Writes[0].Value, &revokeJson); err != nil {
					fmt.Printf("Could not handle block event: %s\n", err)
					return nil, nil, err
				}
				
				for _,r := range revokeJson {
					if temp, err = blockchain.ParsePCN(r); err != nil{
						fmt.Printf("Could not handle block event: %s", err)
						return nil, nil, err
					}
					revocations = append(revocations, []byte(strings.Replace(temp.ProofList.Revoke.Cert, "REVOKE\n", "", 1)))
				}
				
			}
		}
	} else {
		logHasher, err := blockchain.InitHasher()
		if err != nil {
			fmt.Printf("%s\n", err)
			return nil, nil, err
		}

		blockMerkleTree = merkle.NewInMemoryMerkleTree(logHasher)

		//fmt.Printf("%+v\n", block)
		for index, valid := range block.Metadata.Metadata[2] {
			if valid != 0 {
				//If tx was not accepted by peer, continue to next transaction
				continue
			}
				
			for _, write := range block.Transactions[index].Writes{			
				var certs [][]byte
				fmt.Printf("Write Set: %+v\n", write)
					
				rootString, err := url.QueryUnescape(write.KvRwSet.Writes[0].Key)
				if err != nil {
					fmt.Printf("Could not handle block event: %s\n", err)
					return nil, nil, err
				}

				if rootString != "rootCerts" {
					fmt.Printf("Invalid Init Block!\n")
					return nil, nil, errors.New("Block is not formatted correctly. Key should be \"rootCerts\"\n")
				}	 		

				if err = json.Unmarshal(write.KvRwSet.Writes[0].Value, &certs); err != nil {
					fmt.Printf("Could not handle block event: %s\n", err)
					return nil, nil, err
				}

				for _,cert := range certs {
					blockMerkleTree.AddLeaf(cert)
				}
			}
		}
	}
	if n != blockchain.BlockOffset {
		return blockMerkleTree, &revocations, nil
	}
	return blockMerkleTree, nil, nil
}
