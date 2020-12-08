package blockchain

import (
	"os"
	"fmt"
	"time"
	"strings"
	"github.com/hyperledger/fabric-sdk-go/pkg/client/channel"
	"github.com/hyperledger/fabric-sdk-go/pkg/client/channel/invoke"
	"github.com/hyperledger/fabric-sdk-go/pkg/client/ledger"
	"github.com/hyperledger/fabric-sdk-go/pkg/client/event"
	"github.com/hyperledger/fabric-sdk-go/pkg/core/config"
	"github.com/hyperledger/fabric-sdk-go/pkg/fabsdk"
	"github.com/hyperledger/fabric-sdk-go/pkg/common/providers/fab"
	"github.com/hyperledger/fabric-sdk-go/third_party/github.com/hyperledger/fabric/protos/common"
)

// FabricSetup implementation
type FabricSetup struct {
	ConfigFile      			string
	OrgID           			string
	ChannelID       			string
	ChainCodeID     			string
	sdkInitialized     			bool
	channelClientInitialized  	bool
	ledgerClientInitialized  	bool
	eventClientInitialized  	bool
	ChannelConfig   			string
	ChaincodeGoPath 			string
	ChaincodePath   			string
	OrgAdmin        			string
	OrgName         			string
	UserName        			string
	channelClient          		channel.Client
	ledgerClient          		ledger.Client
	eventClient          		event.Client
	sdk             			*fabsdk.FabricSDK
}

// Initialize reads the configuration file and sets up the client, chain and event hub
func (setup *FabricSetup) Initialize() error {

	// Add parameters for the initialization
	if setup.sdkInitialized {
		return fmt.Errorf("sdk already initialized")
	}

	// Initialize the SDK with the configuration file
	sdk, err := fabsdk.New(config.FromFile(setup.ConfigFile))
	if err != nil {
		return fmt.Errorf("failed to create sdk: %v", err)
	}
	setup.sdk = sdk
	setup.sdkInitialized = true
	return nil
}

func (setup *FabricSetup) Close() {
	if setup.sdkInitialized {
		setup.sdk.Close()
	}
}

func (setup *FabricSetup) InitializeChannelClient() error {
	ctx := setup.sdk.ChannelContext(setup.ChannelID, fabsdk.WithUser(setup.UserName))
	client, err := channel.New(ctx)
	if err != nil {
		return fmt.Errorf("failed to create new channel client: %v", err)
	}		
	setup.channelClient = *client
	setup.channelClientInitialized = true;
	return nil
}

func (setup *FabricSetup) InitializeLedgerClient() error {
	ctx := setup.sdk.ChannelContext(setup.ChannelID, fabsdk.WithUser(setup.UserName))
	client, err := ledger.New(ctx)
	if err != nil {
		return fmt.Errorf("failed to create new channel client: %v", err)
	}		
	setup.ledgerClient = *client
	setup.ledgerClientInitialized = true;
	return nil
}

func (setup *FabricSetup) InitializeEventClient() error {
	ctx := setup.sdk.ChannelContext(setup.ChannelID, fabsdk.WithUser(setup.UserName))
	client, err := event.New(ctx)
	if err != nil {
		return fmt.Errorf("failed to create new channel client: %v", err)
	}		
	setup.eventClient = *client
	setup.eventClientInitialized = true;
	return nil
}


func (setup *FabricSetup) getBlock(blockNumber uint64) (*common.Block, error) {
	if !setup.ledgerClientInitialized {
		err := setup.InitializeLedgerClient()
		if err != nil {
			return nil,err
		}
	}
	blk, err := setup.ledgerClient.QueryBlock(blockNumber)
	if err != nil {
		return nil,err
	}
	return blk,nil
}

func (setup *FabricSetup) GetLedgerInfo() (*fab.BlockchainInfoResponse, error){
	if !setup.ledgerClientInitialized {
		err := setup.InitializeLedgerClient()
		if err != nil {
			return nil,err
		}
	}
	bci, err := setup.ledgerClient.QueryInfo()
	if err != nil {
		fmt.Printf("failed to query for blockchain info: %s\n", err)
		return nil, err
	}
	return bci, nil	
}

func (setup *FabricSetup) getCurrentBlock() (*common.Block, error) {
	if !setup.ledgerClientInitialized {
		err := setup.InitializeLedgerClient()
		if err != nil {
			return nil,err
		}
	}
	bci, err := setup.GetLedgerInfo()
	if err != nil {
		fmt.Printf("failed to get ledger info: %s\n", err)
		return nil, err
	}
	return setup.getBlock(bci.BCI.GetHeight()-1)
}

func (setup *FabricSetup) RegisterBlockListener()  (*fab.Registration, <-chan *fab.FilteredBlockEvent, error){
	if !setup.eventClientInitialized {
		err := setup.InitializeEventClient()
		if err != nil {
			return nil, nil, err
		}
	}

	registration, blockEventChannel, err := setup.eventClient.RegisterFilteredBlockEvent()
	if err != nil {
		fmt.Printf("failed to register block event: %s\n", err)
		return nil, nil, err
	}
	return &registration, blockEventChannel, nil
}

func (setup *FabricSetup) UnregisterBlockListener(reg *fab.Registration) {
	setup.eventClient.Unregister(*reg)
}

func (setup *FabricSetup) Pub(merkleRoot []byte, revocationJsonString []byte) (string, error) {
	if !setup.channelClientInitialized {
		err := setup.InitializeChannelClient()
		if err != nil {
			return "",err
		}
	}
	var args [][]byte
	args = append(args, merkleRoot)
	args = append(args, revocationJsonString)
	args = append(args, []byte(fmt.Sprintf("%d",time.Now().Unix())))

	setup.ChainCodeID = "pubcc"

	transientDataMap := make(map[string][]byte)
	transientDataMap["result"] = []byte("Invoke PUB")

	request := channel.Request{
		ChaincodeID: setup.ChainCodeID, 
		Fcn: "pub", 
		Args: args, 
		TransientMap: transientDataMap,
	}

	handlerInterface := invoke.NewExecuteHandler(&execHandler{})
	response, err := setup.channelClient.InvokeHandler(handlerInterface, request, channel.WithTargetEndpoints(strings.Split(os.Getenv("FabriPeerIps"),",")[:]...))	
	if err != nil {
		return "",fmt.Errorf("failed to invoke: %v", err)
	}
	return string(response.Payload), nil
}

type execHandler struct {
}

func (c *execHandler) Handle(context *invoke.RequestContext, clientContext *invoke.ClientContext) {
	// fmt.Printf("%+v\n", context)
}
