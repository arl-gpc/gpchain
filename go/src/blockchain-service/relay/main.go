package main

import (
	"fmt"
	"flag"
	"sync"
	"bytes"
	"errors"
	"strings"
	"os"
	"os/signal"
	"time"
	"io/ioutil"
	"encoding/json"
//	"encoding/pem"
	"encoding/base64"
	"crypto"
	"crypto/sha256"
	"crypto/rsa"
	"crypto/x509"
	"crypto/rand"
	
	"github.com/willf/bloom"
	
	"blockchain-service/blockchain"
	
	mqtt "github.com/eclipse/paho.mqtt.golang"

	"blockchain-service/relay/blockRequestApi"
	"blockchain-service/relay/relayTypes"
)

var fSetup blockchain.FabricSetup //Must acquire sdkLock before using to be thread safe
var sdkLock, relayLock, updatingLock sync.Mutex

//n : number of items in bloom filter, p : probability of false positives, m : number of bits in the filter, k : number of hash functions
var n = uint(1000)
var p = 0.000001

//////////////////////////////////Must acquire relayLock before using to be thread safe//////////////////////////////////
var revocationList map[[32]byte]bool
var filter = bloom.NewWithEstimates(n, p)
var rsaKey *rsa.PrivateKey
var publisher mqtt.Client
var previousBlockHash = []byte("")
var relayBlockIndex = uint64(0)
/////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

var updating = false

const bloomTopic = "relay1-bloomfilters"
const blockTopic = "relay1-relayblocks"

//Implements crypto.SignerOpt interface (needed to sign realyBlock)
type signerOpt struct {
	h crypto.Hash
}

func (so signerOpt) HashFunc() crypto.Hash {
	return so.h
}

func pemToKey(private []byte) (*rsa.PrivateKey, error) {
	var key *rsa.PrivateKey
	var err error

	s := string(private)

	//Extract base64 encoded portion of cert
	//residue := ""
	start := strings.Index(s, "-----BEGIN PRIVATE KEY-----") + int(len("-----BEGIN PRIVATE KEY-----"))
	end := strings.Index(s, "-----END PRIVATE KEY-----")
	keyString := s[start:end]
	//residue = s[end+int(len("-----END PRIVATE KEY-----")):]
	//Decode base64 using standard encoding
	rawData, err := ioutil.ReadAll(base64.NewDecoder(base64.StdEncoding, strings.NewReader(keyString)))
	if err != nil {
		return nil, err
	}

	//Parse key to a PKCS8 Private Key
	keyInterface, err := x509.ParsePKCS8PrivateKey(rawData)
	if err != nil {
		return key, errors.New(fmt.Sprintf("Could not parse Private Key: %s", err))
	}
	
	//Cast interface to rsa.PrivateKey
	key = keyInterface.(*rsa.PrivateKey)

	return key, nil
}

//Updates internal state of the relay (bloom filter, previousBlockHash)
func update(n uint64, done chan bool) error {
	relayLock.Lock()
	myView := relayBlockIndex
	relayLock.Unlock()

	// If (n-blockchain.BlockOffset) != myView recurse (i.e. starting from the last known block, update internal state)
	if (n-blockchain.BlockOffset) != myView {
		update(n-1, done)
	}

	// time.Sleep(10 * time.Second)
	
	fmt.Printf("Updater: Processing Fabric Block: %d, Relay Block: %d\n", n, n-blockchain.BlockOffset)

	//Fetch block, build merkle tree for block, get list of revocations
	blockMerkleTree, revocations, err := relayTypes.ProcessBlock(n, &sdkLock, &fSetup)
	if err != nil {
		fmt.Printf("Could not update relay state: %s\n", err)
		return err
	}

	relayLock.Lock()
	
	//Add Revocation to Bloom Filter
	if revocations != nil {
		for _,revocation := range *revocations {
			sum := sha256.Sum256(revocation)
			filter.Add(sum[:])
		}
	}
	blockRoot := blockMerkleTree.CurrentRoot().Hash()
	var relayBlk relayTypes.RelayBlock
	fmt.Printf("%d %d\n", n, blockchain.BlockOffset)
	if n != blockchain.BlockOffset {	
		//Convert Bloom Filter to []byte
		filterBuffer := bytes.NewBuffer([]byte{})
		_, err = filter.WriteTo(filterBuffer)
		if err != nil {
			fmt.Printf("Error: Could not write bloom filter: %v\n", err)
			relayLock.Unlock()
			return err
		}
		filterBufferHash := sha256.Sum256(filterBuffer.Bytes())
	
		//Create Relay Block Message
		relayBlk = relayTypes.RelayBlock{relayBlockIndex, blockRoot, filterBufferHash[:], previousBlockHash}
	} else {
		relayBlk = relayTypes.RelayBlock{relayBlockIndex, blockRoot, []byte(""), []byte("")}
	}

	//fmt.Printf("Relay Block: %+v\n", relayBlk)

	//Update Global Vars 
	previousBlockHash = relayBlk.Hash()
	relayBlockIndex++

	relayLock.Unlock()
	done <- true
	return nil
}

//Handle block event for fabric block n
func handleEvent(n uint64) {
	var myView uint64
	done := make(chan bool)
	
	//Get current view of the relay (determiend by relayBlockIndex)
	relayLock.Lock()
	myView = relayBlockIndex
	relayLock.Unlock()

	fmt.Printf("HandleEvent Request for Fabric Block %d\n", n)

	if n > blockchain.BlockOffset {
		//Check if the updater needs to run, but isn't
		updatingLock.Lock()
		fmt.Printf("Handler %d: Have %d, Want: %d, Updating: %t\n", n, myView, n-blockchain.BlockOffset, updating)
		if (n-blockchain.BlockOffset) != myView && !updating {
		//Run the updater
		fmt.Printf("Starting Updater\n")
		updating = true
		go update(n-1, done)
		
		go func() {
			// fmt.Printf("Wait for %d to be %d\n", myView, (n-blockchain.BlockOffset))
			end := ((n-blockchain.BlockOffset) - myView)
			for i := uint64(0); i < end; i++ {
				<-done
			}
			updatingLock.Lock()
			updating = false;
			fmt.Printf("Updating Done\n")
			updatingLock.Unlock()
		}()
		}
		updatingLock.Unlock()

		relayLock.Lock()
		myView = relayBlockIndex
		relayLock.Unlock()

		//Wait for update to complete (if needed)
		for (n-blockchain.BlockOffset) != myView {
			// fmt.Printf("Handler %d: Have %d, Want: %d\n", n, myView, n-blockchain.BlockOffset)
			time.Sleep(10 * time.Millisecond)

			relayLock.Lock()
			myView = relayBlockIndex
			relayLock.Unlock()
		}

		//Fetch block, build merkle tree for block, get list of revocations
		blockMerkleTree, revocations, err := relayTypes.ProcessBlock(n, &sdkLock, &fSetup)
		if err != nil {
			fmt.Printf("Could not update relay state: %s\n", err)
			return
		}

		relayLock.Lock()
		//Add Revocation to Bloom Filter
		if revocations != nil {
			for _,revocation := range *revocations {
				sum := sha256.Sum256(revocation)
				revocationList[sum] = true
				filter.Add(sum[:])
			}
		}
		blockRoot := blockMerkleTree.CurrentRoot().Hash()

		//Convert Bloom Filter to []byte
		filterBuffer := bytes.NewBuffer([]byte{})
		_, err = filter.WriteTo(filterBuffer)
		if err != nil {
			fmt.Printf("Error: Could not write bloom filter: %v\n", err)
			relayLock.Unlock()
			return
		}
		filterBufferHash := sha256.Sum256(filterBuffer.Bytes())

		//Create Relay Block Message
		relayBlk := relayTypes.RelayBlock{relayBlockIndex, blockRoot, filterBufferHash[:], previousBlockHash}

		// RSA sig of block
		signedRelayBlock, err := rsaKey.Sign(rand.Reader, relayBlk.Hash(), signerOpt{crypto.SHA256}) 
		if err != nil {
			fmt.Printf("Could not sign relay block: %s\n", err)
			relayLock.Unlock()
			return
		}

		// Create Realy Block Message
		relayBlkMsg := relayTypes.RelayBlockMessage{relayBlk, [][]byte{signedRelayBlock}, relayBlk.Hash()}
		relayBlkMsgStr, err := json.Marshal(relayBlkMsg)
		if err != nil {
			fmt.Printf("Could not marshal relay block message: %s\n", err)
			relayLock.Unlock()
			return
		}

		//fmt.Printf("Relay Block: %+v\n", relayBlk)
		fmt.Printf("Relay Block String: %s\n", relayBlkMsgStr)

		//fmt.Printf("Current Block Hash: %+v\n", relayBlk.Hash())
		//fmt.Printf("Previous Block Hash: %+v\n", previousBlockHash)

		//Create Bloom Message
		bloomMsg := relayTypes.BloomMessage{relayBlockIndex, filterBuffer.Bytes()}
		bloomMsgStr, err := json.Marshal(bloomMsg)
		if err != nil {
			fmt.Printf("Could not marshal bloom message: %s\n", err)
			relayLock.Unlock()
			return
		} 
		
		fmt.Printf("Publishing Fabric Block: %d, Relay Block: %d\n", n, relayBlockIndex)
		//Publish Relay Block Message
		if token := publisher.Publish(blockTopic, byte(0), true, string(relayBlkMsgStr)); token.Error() != nil {
			fmt.Printf("Could not publish relay block message: %s\n", err)
			relayLock.Unlock()
			return
		}
		fmt.Printf("Published to topic: %s\n", blockTopic)

		//Publish Bloom Message
		if token := publisher.Publish(bloomTopic, byte(0), true, string(bloomMsgStr)); token.Error() != nil {
			fmt.Printf("Could not publish bloom message: %s\n", err)
			relayLock.Unlock()
			return
		}
		if err = ioutil.WriteFile("bloomFilter.txt", bloomMsgStr, 0644); err != nil {
			fmt.Printf("Could not write bloom filter file: %s\n", err)
			return
		}
		fmt.Printf("Published to topic: %s\n", bloomTopic)


		//Update Global Vars 
		previousBlockHash = relayBlk.Hash()
		relayBlockIndex++
		relayLock.Unlock()
	} else {
		if n == 0{
			fmt.Printf("Gensis Block\n")
		} else if n > 0 {
			fmt.Printf("Init Block\n")

			blockMerkleTree, _, err := relayTypes.ProcessBlock(n, &sdkLock, &fSetup)
			
			relayLock.Lock()
			blockRoot := blockMerkleTree.CurrentRoot().Hash()

			//Create Relay Block Message
			relayBlk := relayTypes.RelayBlock{relayBlockIndex, blockRoot, []byte(""), previousBlockHash}

			// RSA sig of block
			signedRelayBlock, err := rsaKey.Sign(rand.Reader, relayBlk.Hash(), signerOpt{crypto.SHA256}) 
			if err != nil {
				fmt.Printf("Could not sign relay block: %s\n", err)
				relayLock.Unlock()
				return
			}

			// Create Realy Block Message
			relayBlkMsg := relayTypes.RelayBlockMessage{relayBlk, [][]byte{signedRelayBlock}, relayBlk.Hash()}
			relayBlkMsgStr, err := json.Marshal(relayBlkMsg)
			if err != nil {
				fmt.Printf("Could not marshal relay block message: %s\n", err)
				relayLock.Unlock()
				return
			}

			fmt.Printf("Relay Block: %+v\n", relayBlk)

			fmt.Printf("Current Block Hash: %+v\n", relayBlk.Hash())
			fmt.Printf("Previous Block Hash: %+v\n", previousBlockHash)

			fmt.Printf("Publishing Fabric Block: %d, Relay Block: %d\n", n, relayBlockIndex)
			//Publish Relay Block Message
			if token := publisher.Publish(blockTopic, byte(0), true, string(relayBlkMsgStr)); token.Error() != nil {
				fmt.Printf("Could not publish relay block message: %s\n", err)
				relayLock.Unlock()
				return
			}
			fmt.Printf("Published to topic: %s\n", blockTopic)

			//Update Global Vars 
			previousBlockHash = relayBlk.Hash()
			relayBlockIndex++
			relayLock.Unlock()
		}
	}
}

func initSKD() error {
	fSetup = blockchain.FabricSetup{
		OrgAdmin:        "Admin", 
		OrgName:         "Org1", 
		ConfigFile:      "config.1.yaml",
		
		// Channel parameters 
		ChannelID:       "mychannel",
		ChannelConfig:   "../../../org1/basic-network/config/channel.tx",
	
		// User parameters
		UserName:        "Admin",
	}

	fmt.Printf("Initializing Fabric SDK...\n")
	err := fSetup.Initialize()
	if err != nil{
		fmt.Printf("...Unable to initialize the Fabric SDK: %v\n", err)
		return err
	}
	fmt.Printf("...Fabric SDK Initialized\n")
	fmt.Printf("Initializing Ledger Client...\n")
	
	if err = fSetup.InitializeLedgerClient(); err != nil {
		fmt.Printf("...Unable to initialize ledger client: \nError: %v\n", err)
		return err
	}
	fmt.Printf("...Ledger Client Initialized\n")
	fmt.Printf("Initializing Event Client...\n")
	
	if err = fSetup.InitializeEventClient(); err != nil {
		fmt.Printf("...Unable to initialize event client: \nError: %v\n", err)
		return err
	}
	fmt.Printf("...Event Client Initialized\n\n")
	return nil
}

func initPublisher(brokerIP string) error {
	fmt.Printf("Initializing MQTT Publisher...\n")
	opts := mqtt.NewClientOptions()
	opts.AddBroker("tcp://"+brokerIP)
	opts.SetClientID("relay1")
	opts.SetCleanSession(false)
	publisher = mqtt.NewClient(opts)

	token := publisher.Connect()
	return token.Error()
}

func main() {
	broker := flag.String("broker", "localhost:1883", "Broker IP address, default is localhost:1883")
	flag.Parse()
	fmt.Printf("Starting Relay with broker: %s\n", *broker)
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)

	stopBlockListener := make(chan bool)
	blockListenerStopped := make(chan bool)
	stopBlockRequestApi := make(chan bool)
	blockRequestApiStopped := make(chan bool)

	//Init revocationList
	revocationList = make(map[[32]byte]bool)

	//Get file descriptor for key.pem
	keyFile, err := os.Open("certs/key.pem")
	if err != nil {
		fmt.Printf("Could not open rsa key pair: %s\n", err)
		return
	}
	
	//Read file as string
	keyData, err := ioutil.ReadAll(keyFile)
	if err != nil {
		fmt.Printf("Could not read rsa key pair: %s\n", err)
		return
	}

	//Get golang RSA Key Struct from PEM string
	if rsaKey, err = pemToKey(keyData); err != nil {
		fmt.Printf("Could Not Parse RSA Key: %s\n", err)
		return
	}

	if err := initSKD(); err != nil {
		fmt.Printf("Could not init fabric sdk: %s\n", err)
		return
	}
	defer func() {
		sdkLock.Lock()
		fSetup.Close()
		sdkLock.Unlock()
	}()

	if err := initPublisher(*broker); err != nil {
		fmt.Printf("Could not init mqtt publisher client: %s\n", err)
		return
	}
	defer func() {
		relayLock.Lock()
		publisher.Disconnect(uint(250))
		relayLock.Unlock()
	}()
	fmt.Printf("...MQTT Publisher Initialized\n\n")

	//Run Cleanup Code on Ctrl + c
	go func(){
		<-c
		fmt.Printf("\nShutting Down...\n")

		fmt.Printf("Signaling Block Listener Routine to Stop...\n")
		close(stopBlockListener)
		<-blockListenerStopped
		fmt.Printf("...Block Listener Routine Stopped\n")
		
		fmt.Printf("Closing Fabric SDK...\n")
		sdkLock.Lock()
		fSetup.Close()
		sdkLock.Unlock()
		fmt.Printf("...Fabric SDK Closed\n")
		
		fmt.Printf("Disconnecting MQTT Publisher...\n")
		relayLock.Lock()
		publisher.Disconnect(uint(250))
		relayLock.Unlock()
		fmt.Printf("...MQTT Publisher Disconnected\n")
		
		fmt.Printf("...Shutdown Complete\n")
		os.Exit(1)
	}()

	go blockRequestApi.StartBlockRequestListener(&sdkLock, &fSetup, *rsaKey, stopBlockRequestApi, blockRequestApiStopped)

	//On block publish, handleEvent is run in a new thread
	blockchain.BlockListener(&sdkLock, &fSetup, handleEvent, stopBlockListener, blockListenerStopped)
}
