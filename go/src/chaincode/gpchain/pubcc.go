/*
 * This chaincode allows Permission Marshalls to publish transactions to the blockchain.
 * Each transaction must have 3 arguments:
 * 1. Merkle Tree of certificates
 * 2. List of revocations, where a revocation consists of:
      a. Merkle Path to a certificate
	  b. Certificate body
   3. Current Time
 *
 * Chaincode will endorse this if:
 * 1. Merkle Tree of certificates has leaves that are parsable x509 certificates
 * 2. Revocations refer to published certificates that are not expired
 * 3. Current Time = system time +- 12 hours
 *
 *
 * Peer will make a change to ledger consisting of:
 * 1. Add key = Merkle root, Value = {currentTime, list of certificate hashes}
 *
 */

 package main


import (
	"fmt"
	"time"
	"bytes"
	"errors"
	"strconv"
	"net/url"
	"encoding/json"
	"encoding/pem"
	"crypto/x509"

	"blockchain-service/blockchain"

	"github.com/hyperledger/fabric/core/chaincode/shim"
	"github.com/hyperledger/fabric/protos/peer"
)

// SimpleAsset implements a simple chaincode to manage an asset

type SimpleAsset struct {
}

// Init is called during chaincode instantiation to initialize any
// data. No initialization is required.

func (t *SimpleAsset) Init(stub shim.ChaincodeStubInterface) peer.Response {
	fn, args := stub.GetFunctionAndParameters()
	fmt.Printf("%s\n%+v\n", fn, args)
	var certs [][]byte
	for _,encodedString := range args {
		certString, err := url.QueryUnescape(encodedString)
		if err != nil {
			return shim.Error(fmt.Sprintf("Could not decode root cert: %s\n", err))
		}
		block,_ := pem.Decode([]byte(certString))
		if block == nil {
			return shim.Error("Could not parse root cert\n")
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return shim.Error("Could not build x509 struct\n")
		}
		fmt.Printf("Block Bytes == X509 Bytes: %t \n", bytes.Equal(block.Bytes, cert.Raw))
		certs = append(certs, cert.Raw)
	}
	
	certsJson, err := json.Marshal(certs)
	if err != nil {
		return shim.Error("Could not build cert json\n")
	}
	
	err = stub.PutState("rootCerts", []byte(certsJson))
	if err != nil {
		return shim.Error("Failed to set")
	}
	return shim.Success(nil)
}

// Invocations are routed to the proper function "pub" or "get"

func (t *SimpleAsset) Invoke(stub shim.ChaincodeStubInterface) peer.Response {
	// Extract the function and args from the transaction proposal
	fn, args := stub.GetFunctionAndParameters()

	var result string
	var err error
	
	if fn == "pub" {
		result, err = pub(stub, args)
	} else { // assume 'get' even if fn is nil
		result, err = get(stub, args)
	}
	if err != nil {
		return shim.Error(err.Error())
	}

	// Return the result as success payload
	fmt.Printf("Result: %s\n", result)
	return shim.Success([]byte(result))
}

// Publish a transaction

func pub(stub shim.ChaincodeStubInterface, args []string) (string, error) {
	var revocations []blockchain.Revocation
	var abbreviatedRevokeList [][]byte
	var timestamp int64
	timestampWindow := int64(1) // Number of minutes

	if len(args) != 3 {
		return "", fmt.Errorf("Incorrect arguments. Expecting merkleTree, revokeList, currentTime")
	}
	
	merkleRoot := args[0]

	if merkleRoot == "" {
	    return "", fmt.Errorf("Invalid Merkle Tree")
	}	
	
	temp, err := url.QueryUnescape(args[1])
	if err != nil {
		return "", err
	}
	
	fmt.Printf("Verifying Transaction Timestamp is Within %d Minute(s) of Current Time...\n", timestampWindow)
	timestamp, err = strconv.ParseInt(args[2], 10, 64)
	if err != nil {
		return "", errors.New("Could Not Parse Transaction Timestamp.")
	}
	now := time.Now().Unix()
	if now > (timestamp + (timestampWindow * 60)) || now < (timestamp - (timestampWindow * 60)) {
		return "", fmt.Errorf("Timestamp of transaction is not within %d minute(s) of current time.", timestampWindow)
	}
	fmt.Printf("...Confirmed\n")

	if err := json.Unmarshal([]byte(temp), &revocations); err != nil {
		return "", err
	}
	
	fmt.Printf("Verifying Revocations Correspond to Published Cert...\n")
	for _,r := range revocations {
		fmt.Printf("Revocation: %s\n", r.PCN)
		//abbreviatedRevokeList = append(abbreviatedRevokeList, r.CertData) //change to r.PCN
		abbreviatedRevokeList = append(abbreviatedRevokeList, r.PCN)
		if val, err := stub.GetState(url.QueryEscape(string(r.PubValidationInfo.MerkleRoot))); (val == nil && err == nil) {
			return "", errors.New(fmt.Sprintf("Merkle Root For Certificate Not Found in Ledger: %s", err))
		}
		if err = blockchain.VerifyMerkleProof(r.PubValidationInfo.LeafIndex, r.PubValidationInfo.NumLeaves, r.PubValidationInfo.MerkleRoot, r.CertData, r.PubValidationInfo.Proof); err != nil {
			return "", errors.New(fmt.Sprintf("Merkle Root For Certificate Found in Ledger, but Could Not Verify Inclusion: %s", err))
		}
	}
	fmt.Printf("...Confirmed\n")

	revokeJson, err := json.Marshal(abbreviatedRevokeList)
	if err != nil {
		return "", err
	}
	
	fmt.Printf("Merkle Root: %s\n", merkleRoot)
	fmt.Printf("Revocation List: %+v\n", abbreviatedRevokeList)
	
	
	err = stub.PutState(string(merkleRoot), []byte(revokeJson))
	
	if err != nil {
		return "", fmt.Errorf("Failed to set asset: %s", args[0])
	}
	
	return "Hooray", nil
}

// Get returns the paylaod for a merkle root

func get(stub shim.ChaincodeStubInterface, args []string) (string, error) {
	if len(args) != 1 {
		return "", fmt.Errorf("Incorrect arguments. Expecting a key")
	}

	value, err := stub.GetState(args[0])
	if err != nil {
		return "", fmt.Errorf("Failed to get asset: %s with error: %s", args[0], err)
	}
	if value == nil {
		return "", fmt.Errorf("Asset not found: %s", args[0])
	}
	return string(value), nil
}

// main function starts up the chaincode in the container during instantiate
func main() {
	if err := shim.Start(new(SimpleAsset)); err != nil {
		fmt.Printf("Error starting SimpleAsset chaincode: %s", err)
	}
}