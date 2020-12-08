package blockchain

import(
	"fmt"
	"sync"
	"flag"
	"errors"
	"time"
	"bytes"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"

	"github.com/google/trillian"
	"github.com/google/trillian/merkle"
	"github.com/google/trillian/merkle/hashers"
    _ "github.com/google/trillian/merkle/rfc6962" // Load hashers

	"github.com/hyperledger/fabric-sdk-go/third_party/github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric-sdk-go/pkg/common/providers/fab"
	p1 "github.com/hyperledger/fabric-sdk-go/third_party/github.com/hyperledger/fabric/protos/peer"
	p2 "github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/rwsetutil"
	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/protos/utils"
)

//Fabric block number = Relay block number + 1 (Fabric block 0 = genesis block, Fabric block 1 = chaincode instantiation)
const BlockOffset = uint64(1)

type Transaction struct {
	Writes []*rwsetutil.NsRwSet
}

type Block struct {
	Transactions []*Transaction
	Header *common.BlockHeader
	Metadata *common.BlockMetadata
}

type ValidationInfo struct {
	LeafIndex int64 `json:"index"`
	BlockIndex int64 `json:"height"`
	NumLeaves int64 `json:"numLeaves"`
	MerkleRoot []byte `json:"merkleRoot,omitempty"`
	Proof [][]byte `json:"hashes"`
}

type ValidatorRevokeInfo struct {
 	Signature []byte `json:"signature,omitempty"`
	Cert string `json:"message,omitempty"`
}

type ProofList struct {
	ProofList []ValidationInfo `json:"proofList"`
	Revoke ValidatorRevokeInfo `json:"revoke,omitempty"`
}

type ProofFile struct {
	Certs []*x509.Certificate
	ProofList *ProofList
}

type Revocation struct {
	CertData []byte
	PubValidationInfo ValidationInfo
	BroadcastValidationInfo ValidationInfo
	PCN []byte
}

func (p *ProofFile) AddMerkleProof(v *ValidationInfo) error {
	if len(p.Certs) != len(p.ProofList.ProofList) +1 {
		return errors.New("Cannot add entry to this file")
	}
	p.ProofList.ProofList = append([]ValidationInfo{*v}, p.ProofList.ProofList...)
	return nil
}

func (p *ProofFile) Print() {
	fmt.Printf("Certificates:\n")
	for _,cert := range p.Certs {
		fmt.Printf("\t%s\n", cert.Subject)
	}
	fmt.Printf("Merkle Proofs:\n")
	for _,proof := range p.ProofList.ProofList {
		fmt.Printf("\t%+v\n", proof)
	}
	return
}

func (p *ProofFile) ToFileFormat() ([]byte, error) {
	var returnString []byte	
	write := bytes.NewBuffer([]byte{})
	for _,cert := range p.Certs {
		block := &pem.Block{
			Type: "CERTIFICATE",
			Bytes: cert.Raw,
		}
	
		if err := pem.Encode(write, block); err != nil {
			return nil, err
		}
		returnString = append(returnString, []byte(write.String())...)
		write = bytes.NewBuffer([]byte{})
	}
	jsonString, err := json.MarshalIndent(p.ProofList, "", "    ")
	if err != nil {
		return nil, err
	}
	returnString = append(returnString, jsonString...)
	return returnString, nil
}

func getX509Chain(data []byte) ([]*x509.Certificate, []byte, error) {
	var certs []*x509.Certificate
	var residue []byte
	var temp *pem.Block
	//Decode PEM encoded Cert Chain
	for temp, residue = pem.Decode(data); temp != nil; temp, residue = pem.Decode(residue) {
		if temp == nil || temp.Type != "CERTIFICATE" {
			return nil, nil, errors.New("Could not decode PEM string\n")
		}
		cert, err := x509.ParseCertificate(temp.Bytes)
		if err != nil {
			return nil, nil, err
		}
		certs = append(certs, cert)
	}
	return certs,residue, nil
}

func getMekerkleProofArray(data []byte) (*ProofList, error) {
	var proofList ProofList
	if err := json.Unmarshal(data, &proofList); err != nil {
		return nil, err
	}
	return &proofList, nil
}

func ParsePCN(data []byte) (*ProofFile, error) {
	certs, residue, err := getX509Chain(data)
	if err != nil {
		return nil, fmt.Errorf("Could not parse certs: %s\n", err)
	}
	proofArray, err := getMekerkleProofArray(residue)
	if err != nil {
		return nil, fmt.Errorf("Could not parse json section: %s\n", err)
	}
	return &ProofFile{certs, proofArray}, nil
}

func (b *Block) Print() {
	fmt.Printf("Block Header: %+v\n", b.Header)
	fmt.Printf("Transactions:\n")
	for index,tx := range b.Transactions {
		fmt.Printf("\tTransaction: %d\n", index)
		for _,write := range tx.Writes {
			fmt.Printf("\t\tWrite: %s\n", write)
		}
	}
	b.PrintMetadata()
	fmt.Printf("\n")
}

func (b *Block) PrintMetadata() {
	fmt.Printf("Block Metadata:\n")
	for index, element := range b.Metadata.Metadata {
		fmt.Printf("\t%s:\n", common.BlockMetadataIndex_name[int32(index)])
		fmt.Printf("\t\t%x\n", element)
	}
}

func convertTranscationAction(ta *p1.TransactionAction) *p2.TransactionAction {
	return &p2.TransactionAction{ta.Header, ta.Payload, ta.XXX_NoUnkeyedLiteral, ta.XXX_unrecognized, ta.XXX_sizecache}
}

func (setup *FabricSetup) GetBlock(blockNumber uint64) (*Block, error){
	var blk *common.Block
	var err error

	if blockNumber == 0{
		blk, err = setup.getCurrentBlock()
		if err != nil {
			fmt.Printf("Could not get block: \nError: %v\n", err)
			return nil, err
		}
	} else {
		blk, err = setup.getBlock(blockNumber)
		if err != nil {
			fmt.Printf("Could not get block: \nError: %v\n", err)
			return nil, err
		}
	}

	
	var myBlock Block;
	myBlock.Transactions = make([]*Transaction, 0, len(blk.Data.Data))
	myBlock.Header = blk.GetHeader()
	myBlock.Metadata = blk.GetMetadata()
	
	for _, data := range blk.Data.Data {
		var myTransaction Transaction
		env := common.Envelope{}
		payload := common.Payload{}
		tx := p1.Transaction{}
		chaincodeAction := &p2.ChaincodeAction{}
		
		if err = proto.Unmarshal(data, &env); err != nil {
			fmt.Printf("Could unmarshal to envelope")
			return nil, err
		}
		
		if err = proto.Unmarshal(env.GetPayload(), &payload); err != nil {
			fmt.Printf("Could unmarshal to payload")
			return nil, err
		}
		
		if err = proto.Unmarshal(payload.GetData(), &tx); err != nil {
			fmt.Printf("Could unmarshal to transaction")
			return nil, err
		}

		//Assumes one write per action
		myTransaction.Writes = make([]*rwsetutil.NsRwSet, 0, len(tx.GetActions()))
		rwset := &rwsetutil.TxRwSet{}
		for _, action := range tx.GetActions() {
			if _, chaincodeAction, err = utils.GetPayloads(convertTranscationAction(action)); err != nil {
				fmt.Printf("Could get payloads: %s", err)
				return nil, err
			}
			if err = rwset.FromProtoBytes(chaincodeAction.GetResults()); err != nil {
				fmt.Printf("Could not get read write set: %s", err)
				return nil, err
			}
			//index 0 is the read set, index 1 is the write set
			myTransaction.Writes = append(myTransaction.Writes, rwset.NsRwSets[1])
		}
		myBlock.Transactions = append(myBlock.Transactions, &myTransaction)
	}
	return &myBlock, nil
}

var HashStrategyFlag = flag.String("hash_strategy", "RFC6962_SHA256", "The log hashing strategy to use")

func InitHasher() (hashers.LogHasher, error) {
    strategy, ok := trillian.HashStrategy_value[*HashStrategyFlag]
	if !ok {
        fmt.Printf("Unknown hash strategy: %s", *HashStrategyFlag)
        return nil, errors.New(fmt.Sprintf("Unknown hash strategy: %s", *HashStrategyFlag))
	}

    logHasher, err := hashers.NewLogHasher(trillian.HashStrategy(strategy))
    if err != nil {
        fmt.Printf("Could Not Create Log Hasher: %v\n", err)
        return nil, err
	}
	return  logHasher, nil
}

func VerifyMerkleProof(leafIndex, treeSize int64, root, leaf []byte, proofSet [][]byte) error {
	logHasher, err := InitHasher()
    if err != nil {
        fmt.Printf("Could Not Create Log Hasher: %v\n", err)
        return err
	}
		
	verifier := merkle.NewLogVerifier(logHasher)
	leafHash := logHasher.HashLeaf(leaf)
	fmt.Printf("Leaf Hash: %+v\n", leafHash)

	return verifier.VerifyInclusionProof(leafIndex, treeSize, proofSet, root, leafHash)
}

type handleEvent func(uint64)

func BlockListener(m *sync.Mutex, fSetup *FabricSetup, fn handleEvent, stop, done chan bool) {
	defer func () {
		done <- true
	}()
	attempts := 0
	var reg *fab.Registration
	var blockEventChannel <-chan *fab.FilteredBlockEvent
	var err error
	for err = errors.New(""); err != nil && attempts < 200; attempts++ {
		if attempts > 0 {
			time.Sleep(1 * time.Second)
		}
		fmt.Printf("Attempt %d to register block listener\n", attempts)
		m.Lock()
		reg, blockEventChannel, err = fSetup.RegisterBlockListener()
		m.Unlock()
	}
	if err != nil {
		fmt.Printf("Could not register block listener: %s\n", err)
		return
	}
	fmt.Printf("Block listener registered\n")
	defer func() {
		m.Lock()
		fSetup.UnregisterBlockListener(reg)
		m.Unlock()
	}()
	
	fmt.Printf("Waiting for block events...\n")
	for true {
		select {
			case event, isOpen := <-blockEventChannel:
				if !isOpen {
					fmt.Printf("Channel closed while waiting\n")
					return 
				}
				if event.FilteredBlock == nil {
					fmt.Printf("Block is nil\n")
					return
				}
				fmt.Printf("----------------------Got block event----------------------\n")
				go fn(event.FilteredBlock.Number)
			case <-stop:
				done <- true
				return
		}
	}
}
