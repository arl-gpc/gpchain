package main

import (
	"fmt"
	"os"
	"os/signal"
	"os/exec"
	"log"
	"bytes"
	"sync"
	"errors"
	"time"
	"strings"
	"net/http"
	"net/url"
	"io/ioutil"
	"encoding/json"
	"encoding/base64"
	"encoding/pem"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"unicode"
	"crypto/sha256"
	
	"github.com/boltdb/bolt"
	"github.com/google/trillian/merkle"
	
	"blockchain-service/blockchain"
)


// Definition of the Fabric SDK properties
var fSetup = blockchain.FabricSetup{
	OrgAdmin:        "Admin", 
	OrgName:         "Org1", 
	ConfigFile:      "config.yaml",
	
	// Channel parameters 
	ChannelID:       "mychannel",
	ChannelConfig:   "../../../org1/basic-network/config/channel.tx",

	// User parameters
	UserName:        "Admin",
}

var db *bolt.DB
var dbLock sync.Mutex
var sdkLock sync.Mutex

type Workflow int

const(
	CREATED Workflow = 1 << iota //1
	SIGNED //2
	PUBLISHED //4
	REVOKED_PENDING //8
	REVOKED //16
	REVOKED_PUBLISHED //32
)

type csrData struct {
	C string
	S string
	L string
	O string
	Ou string
	Cn string
	Email string
}

type csrRequest struct {
	PemString string
	To string
	From string
}

type csrResponse struct {
	PemString string
	CsrData csrData
	Ca string
	Status Workflow
	PubValidationInfo blockchain.ValidationInfo
	BroadcastValidationInfo blockchain.ValidationInfo
	AttrString string
}

type signRequest struct {
	Ip string
	PemString []string
	Nonce string
}

type signResponse struct {
	Response string
	Csr string
	Revocation string
}

type dbValue struct {
	Data []byte
	To string
	From string 
	Status Workflow
	PubValidationInfo blockchain.ValidationInfo
	BroadcastValidationInfo blockchain.ValidationInfo
	PCN []byte
}

type dbEntry struct {
	Key []byte
	Value dbValue
}

//Init
func initFabricContext() error {
	// Initialization of the Fabric SDK from the previously set properties
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

func initDataStore() error{
	var err error
	db, err = bolt.Open("./data/data.db", 0600, nil)
	if err != nil {
		return err
	}
	err = db.Update(func(tx *bolt.Tx) error {
		if _, err := tx.CreateBucketIfNotExists([]byte("USERS")); err != nil {
			//Roll Back tx
			return err
		}
		return nil
	})
	return err
}

//Utils
/*
Verifies the following:
(1) Verify signer has permission to sign by making a call to the policy evaluator
(2) Check PM's local state to see if signer's cert has been revoked
*/
func permissionToSign(pcn *blockchain.ProofFile) error{
	//Write pcn struct to a []byte in file format	
	fileContent, err := pcn.ToFileFormat()
	if err != nil {
		return errors.New("Could not create file form pcn\n")
	}

	//Use URLEncoding(Sha256(current time)) as temp file name
	timeAsBin, err := time.Now().MarshalBinary()
	if err != nil {
		return errors.New(fmt.Sprintf("Could not get current time as []byte: %s\n", err))
	}
	timeHash := sha256.Sum256(timeAsBin)
	fileName := fmt.Sprintf("%s.pcn", url.QueryEscape(base64.StdEncoding.EncodeToString(timeHash[:])))
	
	//Write pcn to temp file
	if err = ioutil.WriteFile(fmt.Sprintf("./policy-eval/%s", fileName), fileContent, 0444); err != nil {
		return errors.New(fmt.Sprintf("Could not write to temp pcn file: %s\n", err))
	}

	//Call policy-eval to determine if pcn is a vaild cert chain
	fmt.Printf("Checking Policy...\n")
	cmd := exec.Command("./policy-eval/policy-eval", "-chain", fmt.Sprintf("./policy-eval/%s", fileName), "-pb", "./policy-eval/pb.txt")
	//Start cmd, wait for it to finish, return error
	returnCode := cmd.Run()
	
	//Delete temp file	
	if err := os.Remove(fmt.Sprintf("./policy-eval/%s", fileName)); err != nil {
		return errors.New(fmt.Sprintf("Could not remove temp pcn file: %s\n", err))
	}
	
	if returnCode != nil {
		return returnCode
	}
	
	fmt.Printf("Checking Revocation List...\n")
	//Check that singer's cert is not revoked
	return isRevoked(pcn.Certs[1])
}

/*
Verifies the following:
(1) Cert being revoked matches cert in PCN
(2) Verify signer has permission to revoke by making a call to the policy evaluator
(3) Verify the signature over "REVOKE<cert>" is valid
(4) Check PM's local state to see if signer's cert has been revoked
*/
func permissionToRevoke(pcn *blockchain.ProofFile, revoked *x509.Certificate) error{	
	//check that Cert in pcn.ProofList.Revoke.Cert matches the cert being revoked!	
	cert, _, err := pemToCert(strings.Replace(pcn.ProofList.Revoke.Cert, "REVOKE\n", "", 1))
	if err != nil {
		return err
	}

	if !bytes.Equal(cert.Raw, revoked.Raw) {
		return errors.New("Revocation message is for different cert!\n")
	}

	//Prepend pcn of revoking principle with certificate that is being revoked creating a new cert chain	
	revokedCertStr := bytes.NewBuffer([]byte(""))
	chainToEval, err := pcn.ToFileFormat()
	if err != nil {
		return err
	}
	block := &pem.Block{
		Type: "CERTIFICATE",
		Bytes: revoked.Raw,
	}
	
	if err := pem.Encode(revokedCertStr, block); err != nil {
		return nil
	}
	chainToEval = append(revokedCertStr.Bytes(), chainToEval...)

	//Use URLEncoding(Sha256(current time)) as temp file name
	timeAsBin, err := time.Now().MarshalBinary()
	if err != nil {
		return errors.New(fmt.Sprintf("Could not get current time as []byte: %s\n", err))
	}
	timeHash := sha256.Sum256(timeAsBin)
	fileName := fmt.Sprintf("%s.pcn", url.QueryEscape(base64.StdEncoding.EncodeToString(timeHash[:])))
	
	//Write new cert chain to temp file
	if err = ioutil.WriteFile(fmt.Sprintf("./policy-eval/%s", fileName), chainToEval, 0444); err != nil {
		return errors.New(fmt.Sprintf("Could not write to temp pcn file: %s\n", err))
	}
	fmt.Printf("Checking Policy...\n")

	//Call policy-eval to evaluate if the revoking principle has permission to revoke the cert being revoked
	cmd := exec.Command("./policy-eval/policy-eval", "-chain", fmt.Sprintf("./policy-eval/%s", fileName), "-pb", "./policy-eval/pb.txt")
	//Start cmd, wait for it to finish, return error
	returnCode := cmd.Run()

	//Delete temp file
	if err := os.Remove(fmt.Sprintf("./policy-eval/%s", fileName)); err != nil {
		return errors.New(fmt.Sprintf("Could not remove temp pcn file: %s\n", err))
	}
	if returnCode != nil {
		return returnCode
	}
	fmt.Printf("...Valid\n")

	//Check that revocation has been signed by the revoker
	fmt.Printf("Checking Signature...\n")
	err = pcn.Certs[0].CheckSignature(x509.SHA256WithRSA, []byte(pcn.ProofList.Revoke.Cert), pcn.ProofList.Revoke.Signature)
	if err != nil {
		return err
	}
	fmt.Printf("...Valid\n")

	//Check that revoker's cert is not included in revocation list
	fmt.Printf("Revocation List...\n")	
	if err = isRevoked(pcn.Certs[0]); err != nil {
		return err
	}
	fmt.Printf("...Valid\n")
	return nil
}

//Check PM's local state to see if the provided x509 has been revoked
func isRevoked(cert *x509.Certificate) error {
	rsaKey := cert.PublicKey.(*rsa.PublicKey)
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s%d", rsaKey.N.String(), rsaKey.E)))
	key := sum[:]
	dbLock.Lock()
	err := db.View(func(tx *bolt.Tx) error {
		root := tx.Bucket([]byte("USERS"))
		bucket := root.Bucket([]byte(strings.ToLower(cert.Subject.CommonName)))
		//PM's are aware of all revocations (since they listen for block events, adding revocations to their key value store).
		//If bucket == nil, revocation has not been published (PM is unaware of user). If bucket != nil revocation may have been published	
		if bucket != nil {
			resp := bucket.Get(key)
			//If resp == nil, revocation has not been published (PM's is aware of user but not of cert). If resp != nil revocation may be published
			if resp != nil {
				//Check status of entry
				var value dbValue
				err := json.Unmarshal(resp, &value)
				if err != nil {
					return err
				}
				if value.Status == REVOKED_PUBLISHED {
					return errors.New("Revoking cert found on PM's revocation list!\n")
				}
			}
		}
		return nil
	})
	dbLock.Unlock()
	return err
}

//Checks if cert is published. If so, returns the corresponding key-value-store entry
func isPublished(cert *x509.Certificate, proofPubJson string) (*dbEntry, error) {
	var rsaKey *rsa.PublicKey
	var value dbValue
	var returnValue *dbEntry
	published := false
	rsaKey = cert.PublicKey.(*rsa.PublicKey)
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s%d", rsaKey.N.String(), rsaKey.E)))
	key := sum[:]

	dbLock.Lock()
	err := db.Update(func(tx *bolt.Tx) error {
		root := tx.Bucket([]byte("USERS"))
		bucket, err:= root.CreateBucketIfNotExists([]byte(strings.ToLower(cert.Subject.CommonName)))
		if err != nil {
			return err
		}
		//Get Entry
		temp := bucket.Get(key)
		//If there is no entry, this certificate might be managed by another Permission Marshal. Construct dbValue and commit tx to check 
		//if the certificate was published by abother Permission Marshal.
		if temp == nil {
			fmt.Printf("Certificate Published by another PM\n")
			var proofPub blockchain.ValidationInfo
			if err := json.Unmarshal([]byte(proofPubJson), &proofPub); err != nil {
				//Rollback tx
				return err
			}
			value = dbValue{cert.Raw, cert.Issuer.CommonName, cert.Subject.CommonName, PUBLISHED, proofPub, blockchain.ValidationInfo{}, nil}
			return nil
		} else {
			fmt.Printf("Certificate Published by this PM\n")
			//Unmarhsal dbValue
			if err := json.Unmarshal(temp, &value); err != nil {
				//Rollback tx
				return err
			}
			return nil
		}
	})
	dbLock.Unlock()
	if err != nil {
		return nil, err
	}

	sdkLock.Lock()
	bci, err := fSetup.GetLedgerInfo()
	sdkLock.Unlock()
	if err != nil {
		return nil, err
	}
	
	fmt.Printf("Fetching Block %d of %d\n", value.PubValidationInfo.BlockIndex, bci.BCI.GetHeight()-1)
	if value.PubValidationInfo.BlockIndex <=-1 || uint64(value.PubValidationInfo.BlockIndex) > bci.BCI.GetHeight()-1{
		return nil , errors.New("Invalid block index in proof of publication\n")
	}
	sdkLock.Lock()
	block, err := fSetup.GetBlock(uint64(value.PubValidationInfo.BlockIndex))
	sdkLock.Unlock()
	if err != nil {
		return nil, err
	}
	for index, valid := range block.Metadata.Metadata[2] {
		if valid != 0 {
			//If tx was not accepted by peer, continue to next transaction
			continue
		}

		for _, write := range block.Transactions[index].Writes {			
			//Parse Merkle Roots and Revocations
			rootString, err := url.QueryUnescape(write.KvRwSet.Writes[0].Key)
			if err != nil {
				return nil, err
			}
			//If the current tx does not contain merkle root for published cert, continue
			if !bytes.Equal([]byte(rootString), value.PubValidationInfo.MerkleRoot) {
				continue;
			}
			fmt.Printf("MATCH: %+v, %+v\n", []byte(rootString), value.PubValidationInfo.MerkleRoot)
			if err = blockchain.VerifyMerkleProof(value.PubValidationInfo.LeafIndex, value.PubValidationInfo.NumLeaves, value.PubValidationInfo.MerkleRoot, value.Data, value.PubValidationInfo.Proof); err != nil {
				return nil, errors.New(fmt.Sprintf("Merkle Root Found for Certificate, but Could Not Verify Inclusion: %s", err))
			} else {
				fmt.Printf("Inclusion Verified\n")
				published = true
				returnValue = &dbEntry{key, value}
				break
			}
		}
	}

	if published {
		return returnValue, nil
	} else {
		return nil, errors.New("Merkle Root Not Found in Any Published Block!")
	}
}

func buildCsrResponse(buf *bytes.Buffer, name *pkix.Name, email []string, ca string, status Workflow, pubProof, broadcastProof blockchain.ValidationInfo, attrString string) csrResponse{
	return csrResponse{string(buf.Bytes()), csrData{name.Country[0],
		name.Province[0], name.Locality[0], name.Organization[0], name.OrganizationalUnit[0],
		name.CommonName, email[0]},ca, status, pubProof, broadcastProof, attrString}
}

func pemToCsr(s string) (*x509.CertificateRequest, error) {
	//Decode PEM
	block,_ := pem.Decode([]byte(s))
	if block == nil {
		return nil, errors.New("Could not decode PEM string\n")
	}
	//Using decoded data, create instance of csr struct
	csr, err := x509.ParseCertificateRequest(block.Bytes)
	if err != nil {
		return nil, err
	}
	return csr, nil
}

func pemToCert(s string) (*x509.Certificate, string, error) {
	//Extract base64 encoded portion of cert
	residue := ""
	start := strings.Index(s, "-----BEGIN CERTIFICATE-----") + int(len("-----BEGIN CERTIFICATE-----"))
	end := strings.Index(s, "-----END CERTIFICATE-----")
	certString := s[start:end]
	residue = s[end+int(len("-----END CERTIFICATE-----")):]
	//Decode base64 using standard encoding
	rawData, err := ioutil.ReadAll(base64.NewDecoder(base64.StdEncoding, strings.NewReader(certString)))
	if err != nil {
		return nil, "", err
	}
	//Using decoded data, create instance of certificate struct
	cert, err := x509.ParseCertificate(rawData)
	if err != nil {
		return nil, "", err
	}
	return cert, residue, nil
}

/*
Given an array of pki extenstions, parse and return attribute extension (1.3.6.1.5.5.7.10) value as a string.
Return error if attribute extenstion (1.3.6.1.5.5.7.10) is not found.
*/
func getAttrExtension(exts []pkix.Extension) (string, error) {
	attrString := ""
	//For all extensions in array	
	for _,ext := range exts {
		//Check if they are the attribute extension
		if ext.Id.String() == "1.3.6.1.5.5.7.10" {
			// If so, parse extenison to string of form (Attribute, CanConfer)
			//Removes all non-alphanumeric charcaters
			attrString = fmt.Sprintf("%s", strings.TrimFunc(string(ext.Value),func(r rune) bool {
				return !unicode.IsLetter(r) && !unicode.IsNumber(r)
			}))
			break;
		}
	}
	//If no attribute extension, return error. Else return attribute as string.
	if attrString == "" {
		return "", errors.New("Could not find extension 1.3.6.1.5.5.7.10\n")
	} else {
		return attrString, nil	
	}
}

/* 
Given an array of dbEntires, construct a merkle tree. 
dbEntries should contain the certificates the PM wishes to publish. 
*/
func buildTree(data []dbEntry) (*merkle.InMemoryMerkleTree, error) {
    err := errors.New("")
    var tree *merkle.InMemoryMerkleTree
	
	logHasher, err := blockchain.InitHasher()
    if err != nil {
        fmt.Printf("Could Not Create Log Hasher: %v\n", err)
        return nil, err
    }

    tree = merkle.NewInMemoryMerkleTree(logHasher)

    for _,element := range data {
		tree.AddLeaf([]byte(element.Value.Data))
	}
	tree.AddLeaf([]byte(fmt.Sprintf("%8d", time.Now().Unix())))
	
	tree.CurrentRoot()
	return tree, nil
}

/*
Construct merkle proofs for every leaf in merkle tree described in TreeEntryDescriptor.
*/
func proofArray(set []merkle.TreeEntryDescriptor) [][]byte {
	var returnArray [][]byte
	for _,elem := range set {
		returnArray = append(returnArray, elem.Value.Hash())
	}
	return returnArray
}

//REST Endpoint Handlers
func indexHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		w.WriteHeader(http.StatusNotFound);
		fmt.Fprintf(w, "%s", "404 Page Not Found")
		return
	}
	http.ServeFile(w, r, "app/index.html")
}

func fileHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Printf("Serving File: %s\n", r.URL.Path)
	http.ServeFile(w, r, fmt.Sprintf("app%s", r.URL.Path))
}

func markForRevocation(w http.ResponseWriter, r *http.Request) {
	var value *dbValue
	var data signRequest
	//Read contnents of http request body
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "Could not read body of HTTP request: %s\n", err)
		fmt.Printf("Could not read body of HTTP request: %s\n", err)
		return
	}
	err = json.Unmarshal(body, &data)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "Could not create JSON using body of request: %s\n", err)
		fmt.Printf("Could not create JSON using body of request: %s\n", err)
		return
	}

	//Create go x509 cert struct for cert being revoked
	cert, _, err := pemToCert(data.PemString[0])
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "Could not create x509 struct from PEM data: %s\n", err)
		fmt.Printf("Could not create x509 struct from PEM data: %s\n", err)
		return
	}
	rsaKey := cert.PublicKey.(*rsa.PublicKey)
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s%d", rsaKey.N.String(), rsaKey.E)))
	key := sum[:]

	dbLock.Lock()
	err = db.Update(func(tx *bolt.Tx) error {
		root := tx.Bucket([]byte("USERS"))
		bucket := root.Bucket([]byte(strings.ToLower(cert.Subject.CommonName)))
		if bucket == nil {
			//Rollback tx
			return errors.New(fmt.Sprintf("(Requestor) User %s does not exist\n", cert.Subject.CommonName))
		}
		//Get Entry
		temp := bucket.Get(key)
		if temp == nil {
			return errors.New("CSR not found in requestors kvs")
		}
		if err := json.Unmarshal(temp, &value); err != nil {
			return err
		}
		if value.Status != PUBLISHED {
			return errors.New("Cannot Revoke Entry Unless Status is PUBLISHED!\n")
		}

		value.Status = REVOKED_PENDING

		//Update Signee
		valueString, err := json.Marshal(value) 
		if err != nil {
			return err
		}
		
		if err = bucket.Put([]byte(key), valueString); err != nil {
			return err
		}
		
		//Update Signer
		bucket, err = root.CreateBucketIfNotExists([]byte(strings.ToLower(cert.Issuer.CommonName)))
		if err != nil {
			return err
		}
		if err = bucket.Put([]byte(key), valueString); err != nil {
			return err
		}
		
		return nil
	})
	dbLock.Unlock()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "Could not mark cert for revcation: %s\n", err)
		fmt.Printf("Could not mark cert for revcation: %s\n", err)
		return
	}
	fmt.Fprintf(w, "Cert makred for revcation\n")
	fmt.Printf("Cert makred for revcation\n")
	return
}

func acceptSignRequest(w http.ResponseWriter, r *http.Request) {
	//Read and Parse data from HTTP Request
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		fmt.Fprintf(w, "%s", err)
		return
	}

	pcn, err := blockchain.ParsePCN(body)
	if err != nil {
		fmt.Printf("Could not parse response pcn from signing app\n")
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "Could not parse response pcn from signing app: %s\n", err)
	}
	//Check if CA had permission to sign csr
	if err = permissionToSign(pcn); err == nil {
		fmt.Printf("...Valid\n")
		//Convert PEM string to Certificate
		cert := pcn.Certs[0]
		var rsaKey *rsa.PublicKey
		rsaKey = cert.PublicKey.(*rsa.PublicKey)
		sum := sha256.Sum256([]byte(fmt.Sprintf("%s%d", rsaKey.N.String(), rsaKey.E)))
		key := sum[:]
		var temp []byte
		var value dbValue
		// Obtain db lock
		dbLock.Lock()
		//Start Read Write Transaction
		err = db.Update(func(tx *bolt.Tx) error {
			//Set bucket to Requestor's bucket
			root := tx.Bucket([]byte("USERS"))
			bucket := root.Bucket([]byte(strings.ToLower(cert.Subject.CommonName)))
			if bucket == nil {
				//Rollback tx
				return errors.New(fmt.Sprintf("(Requestor) User %s does not exist\n", cert.Subject.CommonName))
			}
			//Get Entry
			temp = bucket.Get(key)
			if temp == nil {
				//Rollback tx
				return errors.New("CSR not found in requestors kvs")
			}
			if err := json.Unmarshal(temp, &value); err != nil {
				//Rollback tx
				return err
			}
			if value.Status != CREATED {
				return errors.New("Cannot Sign Entry Unless Status is CREATED!\n")
			}
			//Update Status to SIGNED
			value.Status = SIGNED
			value.Data = cert.Raw
			value.PCN, err = pcn.ToFileFormat()
			if err != nil {
				return err
			}
			//Convert to JSON
			valueString, err := json.Marshal(value)
			if err != nil {
				//Rollback tx
				return err
			}
			//Write Back to KVS
			if err = bucket.Put([]byte(key), valueString); err != nil {
				//Rollback tx
				return err
			}
			//Set bucket to CA's bucket
			bucket = root.Bucket([]byte(strings.ToLower(cert.Issuer.CommonName)))
			if bucket == nil {
				//Rollback tx
				return errors.New(fmt.Sprintf("(CA) User %s does not exist\n", cert.Issuer.CommonName))
			}
			//Write Back to KVS
			if err = bucket.Put([]byte(key), valueString); err != nil {
				//Rollback tx
				return err
			}
			//Commit tx
			return nil
		})
		//Release db lock
		dbLock.Unlock()
		//Handle Error
		if err != nil {
			fmt.Printf("Cannot Update kvs: %s", err)
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, "%s", err)
			return
		}

		//Return Success
		fmt.Fprintf(w, "Valid")
		return
	} else {
		//Handle Permission Denied
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "CA Does not Have Permission to Sing CSR")
		fmt.Printf("CA Does not Have Permission to Sing CSR: %s\n", err)
		return
	}
}

func acceptRevocation(w http.ResponseWriter, r *http.Request) {
	var entry *dbEntry
	//Read contnents of http request body
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "Could not read body of HTTP request: %s\n", err)
		fmt.Printf("Could not read body of HTTP request: %s\n", err)
		return
	}
	
	//Parse PCN from signing app's response. PCN file returned is the PCN of the revoking enitity.		
	pcn, err := blockchain.ParsePCN(body)
	if err != nil {
		fmt.Printf("Could not parse response pcn from signing app: %s\n", err)
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "Could not parse response pcn from signing app: %s\n", err)
		return
	}
	cert, _, err := pemToCert(strings.Replace(pcn.ProofList.Revoke.Cert, "REVOKE\n", "", 1))
	if err != nil {
		fmt.Printf("Could not parse revoked cert from pcn: %s\n", err)
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "Could not parse revoked cert from pcn: %s\n", err)
		return
	}
	//Check if the revoking entity has permission to revoke the cert being revoked 
	if err = permissionToRevoke(pcn, cert); err == nil {
		//Check to see if the cert being revoked has been published.
		if entry, err = isPublished(cert, ""); err == nil {
			//If signingApp approved AND revoking entitiy has permission to revoke AND the cert being revoked is published	isPublished			
			dbLock.Lock()
			err = db.Update(func(tx *bolt.Tx) error {
				root := tx.Bucket([]byte("USERS"))
				bucket := root.Bucket([]byte(strings.ToLower(cert.Subject.CommonName)))
				
				key := entry.Key
				value := entry.Value
				//Mark cert's entry in key value store as revoked.					
				value.Status = REVOKED
				value.PCN, err = pcn.ToFileFormat()
				if err != nil {
					return err
				}
				valueString, err := json.Marshal(value) 
				if err != nil {
					return err
				}
				if err = bucket.Put([]byte(key), valueString); err != nil {
					return err
				}
				bucket, err = root.CreateBucketIfNotExists([]byte(strings.ToLower(cert.Issuer.CommonName)))
				if err != nil {
					return err
				}
				if err = bucket.Put([]byte(key), valueString); err != nil {
					return err
				}
				return nil
			})
			dbLock.Unlock()
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				fmt.Fprintf(w, "Could not set status to REVOKED: %s", err)
				fmt.Printf("%s\n", err)
				return
			}
		} else {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, "Could not verify certificate is published: %s\n", err)
			fmt.Printf("Could not verify certificate is published: %s\n", err)
			return
		}
	} else {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "User does not have permission to revoke certificate!: %s\n", err)
		fmt.Printf("User does not have permission to revoke certificate!: %s\n", err)
		return
	}	
}

func csrHandler(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path{
	case "/csr/new": 
		newCsr(w,r)
	case "/csr/post/signed":
		acceptSignRequest(w,r)
	case "/csr/get/signed":
		getCsr(w,r,true, SIGNED|PUBLISHED)
	case "/csr/get/to_sign":
		getCsr(w,r,true, CREATED)
	case "/csr/get/my_csrs":
		getCsr(w,r,false, CREATED|SIGNED|PUBLISHED|REVOKED_PUBLISHED)
	default:
		w.WriteHeader(http.StatusNotFound);
		fmt.Fprintf(w, "%s", "404 Page Not Found")
		return
	}
}

func revokeHandler(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/revoke/post": 
		acceptRevocation(w,r)
	case "/revoke/mark": 
		markForRevocation(w,r)
	case "/revoke/get": 
		getCsr(w, r, true, REVOKED_PENDING)
	default:
		w.WriteHeader(http.StatusNotFound);
		fmt.Fprintf(w, "%s", "404 Page Not Found")
		return
	}
}

func getAttributes(w http.ResponseWriter, r *http.Request) {
	//Have policy-eval return an array of attributes	
	cmd := exec.Command("./policy-eval/policy-eval", "-printAttr", "-pb", "./policy-eval/pb.txt")
	result, err := cmd.Output()
	fmt.Printf("%s\n", result)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "Could not get attributes from policy evaluator: %s\n", err)
		fmt.Printf("Could not get attributes from policy evaluator: %s\n", err)
		return
	}
	fmt.Fprintf(w,"%s", result)
	return
}

func newCsr(w http.ResponseWriter, r *http.Request) {
	var data csrRequest
	
	//Read and Parse data from HTTP Request
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		fmt.Fprintf(w, "%s", err)
		return
	}
	err = json.Unmarshal(body, &data)
	if err != nil {
		fmt.Fprintf(w, "%s", err)
		return
	}

	//Convert PEM string to CSR
	csr, err := pemToCsr(data.PemString)
	if err != nil {
		fmt.Printf("Could not convert PEM to CSR: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "%s", err)
		return
	}

	//Save csr to proper kvs
	
	// key = hash(RSA Pub Key)
	csrBytes := csr.Raw
	var key *rsa.PublicKey
	key = csr.PublicKey.(*rsa.PublicKey)
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s%d", key.N.String(), key.E)))
	entry := dbEntry{sum[:], dbValue{csrBytes, strings.ToLower(data.To), strings.ToLower(data.From), CREATED, blockchain.ValidationInfo{}, blockchain.ValidationInfo{}, nil}}
	
	valueString, err := json.Marshal(entry.Value)
	if err != nil {
		fmt.Printf("Could not marshal json: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "%s", err)
		return
	}

	//Obtain lock for DB
	dbLock.Lock()
	//Start Read Write Transaction
	err = db.Update(func(tx *bolt.Tx) error {
		//Set bucket to CA's bucket
		root := tx.Bucket([]byte("USERS"))
		bucket, err := root.CreateBucketIfNotExists([]byte(entry.Value.To))
		if err != nil {
			return err
		}
		//PUT(hash(RSA Pub Key), dbValue) to CA's bucket
		if err = bucket.Put([]byte(entry.Key), valueString); err != nil {
			//Rollback tx
			return err
		}
		
		//Set bucket to Requestor's bucket
		bucket, err = root.CreateBucketIfNotExists([]byte(entry.Value.From))
		if err != nil {
			return err
		}
		//PUT(hash(RSA Pub Key), dbValue) to Requestor's bucket
		if err = bucket.Put([]byte(entry.Key), valueString); err != nil {
			//Rollback tx
			return err
		}
		//Commit tx
		return nil
	})
	//Release lock for DB
	dbLock.Unlock()
	//Error handling
	if err != nil {
		fmt.Printf("Could not add CSR to CSR data store: %s", err)
		w.WriteHeader(http.StatusInternalServerError);
		fmt.Fprintf(w, "%s", err)
		return
	}

	//Send Response
	fmt.Fprintf(w, "Success")
}

func getCsr(w http.ResponseWriter, r *http.Request, signer bool, status Workflow) {
	//TODO Authenticate CA
	
	//Get CA name from HTTP request
	q := r.URL.Query()
	user := strings.ToLower(q.Get("user"))
	var csrRawData []*dbEntry
	var csrDataResponses []*csrResponse
	var responseJson []byte
	
	//Obtain lock for DB
	dbLock.Lock()
	//Start a Read Only Transaction
	err := db.View(func(tx *bolt.Tx) error {
		//Set bucket to CA's bucket
		root := tx.Bucket([]byte("USERS"))
		bucket := root.Bucket([]byte(user))
		if bucket == nil {
			return errors.New(fmt.Sprintf("User %s does not exist\n", user))
		}
		//Create iterator
		it := bucket.Cursor()
		//Iterate over all CSRs in the CA's bucket adding data to csrRawData
		var value dbValue
		for k,v := it.First(); k != nil; k, v = it.Next() {
			if err := json.Unmarshal(v, &value); err != nil {
				return err
			}
			if signer {
				if value.To == user && (value.Status & status != 0) {
					csrRawData = append(csrRawData, &dbEntry{k, value})
				}
			} else {
				if value.From == user && (value.Status & status != 0) {
					csrRawData = append(csrRawData, &dbEntry{k, value})
				}
			}
		}
		return nil
	})
	//Release lock for DB
	dbLock.Unlock()
	//Error Handling
	if err != nil {
		fmt.Printf("Could not get CSRs for user %s: %s", user, err)
		w.WriteHeader(http.StatusInternalServerError);
		fmt.Fprintf(w, "%s", err)
		return
	}

	//Process Data
	for _,entry := range csrRawData {
		//Convert raw csr data to PEM encoded data
		if (entry.Value.Status & CREATED) != 0 {
			buffer := bytes.NewBufferString("")
			pem.Encode(buffer, &pem.Block{Type: "CERTIFICATE REQUEST", Bytes: entry.Value.Data})
			
			//Parse csr raw data into golang x509 class
			csr, err := x509.ParseCertificateRequest(entry.Value.Data)
			if err != nil {
				fmt.Printf("Could not parse stored CSR: %s", err)
				w.WriteHeader(http.StatusInternalServerError);
				fmt.Fprintf(w, "%s", err)
				return
			}
			attrString, err := getAttrExtension(csr.Extensions)
			if err != nil {
				fmt.Printf("Could not parse extension from stored CSR: %s", err)
				w.WriteHeader(http.StatusInternalServerError);
				fmt.Fprintf(w, "Could not parse extension from stored CSR: %s", err)	
			}
			//Build CSR response
			response := buildCsrResponse(buffer, &(csr.Subject), csr.EmailAddresses, entry.Value.To, entry.Value.Status, entry.Value.PubValidationInfo, entry.Value.BroadcastValidationInfo, attrString)
			csrDataResponses = append(csrDataResponses, &response)
		} else {
			buffer := bytes.NewBufferString("")
			//pem.Encode(buffer, &pem.Block{Type: "CERTIFICATE", Bytes: entry.Value.Data})
			n ,err := buffer.Write(entry.Value.PCN)
			if n != len(entry.Value.PCN) || err != nil {
				fmt.Printf("Could not write PCN to buffer: %s", err)
				w.WriteHeader(http.StatusInternalServerError);
				fmt.Fprintf(w, "Could not write PCN to buffer: %s", err)
				return
			}
			//Parse csr raw data into golang x509 class
			cert, err := x509.ParseCertificate(entry.Value.Data)
			if err != nil {
				fmt.Printf("Could not parse stored CSR: %s", err)
				w.WriteHeader(http.StatusInternalServerError);
				fmt.Fprintf(w, "Could not parse stored CSR: %s", err)
				return
			}
			attrString, err := getAttrExtension(cert.Extensions)
			if err != nil {
				fmt.Printf("Could not parse extension from stored Cert: %s", err)
				w.WriteHeader(http.StatusInternalServerError);
				fmt.Fprintf(w, "Could not parse extension from stored Cert: %s", err)	
			}
			
			response := buildCsrResponse(buffer, &(cert.Subject), []string{""}, entry.Value.To, entry.Value.Status, entry.Value.PubValidationInfo, entry.Value.BroadcastValidationInfo,attrString)
			csrDataResponses = append(csrDataResponses, &response)
		}
	}

	//Marshal golang struct to JSON
	responseJson, err = json.Marshal(csrDataResponses)
	if err != nil {
		fmt.Printf("Could not marshal json: %s", err)
		w.WriteHeader(http.StatusInternalServerError);
		fmt.Fprintf(w, "%s", err)
		return
	}

	//Return JSON CSR data
	fmt.Fprintf(w, "%s", responseJson)
}

//Fabric Interaction

//Every 5 seconds check if there are new certs / revocations to publish
func batcher(stop, done chan bool) {
	for true {
		select {
			default:
				var certBatch []dbEntry
				var revokeBatch []blockchain.Revocation
				time.Sleep(5 * time.Second)
				fmt.Printf("------------------------------------BATCH-------------------------------------\n")
				//Start Read Transaction
				dbLock.Lock()
				err := db.View(func(tx *bolt.Tx) error {
					root := tx.Bucket([]byte("USERS"))
					//Create user iterator
					outter := root.Cursor()
					//Iterate over all users
					for user,_ := outter.First(); user != nil; user,_ = outter.Next() {
						fmt.Printf("User: %s\n", user)
						//Create CSR iterator for current user
						inner := root.Bucket(user).Cursor()
						//Iterate over all CSRs for user
						for k,v := inner.First(); k != nil; k,v = inner.Next() {
							var value dbValue
							//Convert JSON string to dbValue
							if err := json.Unmarshal(v, &value); err != nil {
								return err
							}
							//Only add CSRs requeted by current user that have been signed by a CA to the certBatch
							if (value.Status & SIGNED != 0) && strings.ToLower(value.From) == string(user) {
								fmt.Printf("\tPub CSR Hash: %x\n", k)
								// Add to certBatch
								certBatch = append(certBatch, dbEntry{k, value})
							} else  if (value.Status & REVOKED != 0) && strings.ToLower(value.From) == string(user) {
								// Add to revokeBatch
								fmt.Printf("\tRevoke CSR Hash: %x\n", k)
								fmt.Printf("\tStatus: %v\n", value.Status)
								revokeBatch = append(revokeBatch, blockchain.Revocation{value.Data, value.PubValidationInfo, blockchain.ValidationInfo{}, value.PCN})
							}
						}
					}
					return nil
				})
				dbLock.Unlock()
				if err == nil {
					//Success

					//If no new certs / revokes, continue
					if len(certBatch) == 0 && len(revokeBatch) == 0 {
						continue
					}

					//Compute Merkle Tree
					tree, err := buildTree(certBatch)
					if err != nil {
						fmt.Printf("%s\n", err)
						continue
					}

					//convert Revocations to JSON Object
					jsonString, err := json.Marshal(revokeBatch)
					if err != nil {
						fmt.Printf("Could not marshal revocations to JSON: %s\n", err)
						continue
					}

					//Invoke Chaincode
					sdkLock.Lock()
					_,err = fSetup.Pub([]byte(url.QueryEscape(string(tree.CurrentRoot().Hash()))), []byte(url.QueryEscape(string(jsonString))))
					sdkLock.Unlock()					
					if err != nil {
						fmt.Printf("Could not invoke pubcc: %s\n", err)
						fmt.Printf("%+v\n",revokeBatch)
						continue
					}
					
					//Only writeback Validation Info when chaincode returns success
					for index,_ := range certBatch {
						certBatch[index].Value.PubValidationInfo = blockchain.ValidationInfo{int64(index), int64(-1), tree.LeafCount(), tree.CurrentRoot().Hash(), proofArray(tree.PathToCurrentRoot(int64(index)+1))}
					}
					
					dbLock.Lock()
					err = db.Update(func(tx *bolt.Tx) error {
						root := tx.Bucket([]byte("USERS"))
						for _,elem := range certBatch {
							valueString, err := json.Marshal(elem.Value)
							if err != nil {
								//Rollback tx
								return err
							}
							//Writeback to Requestor
							bucket := root.Bucket([]byte(strings.ToLower(elem.Value.From)))
							if err = bucket.Put(elem.Key, valueString); err != nil {
								//Rollback tx
								return err
							}
							//Writeback to CA
							bucket = root.Bucket([]byte(strings.ToLower(elem.Value.To)))
							if err = bucket.Put(elem.Key, valueString); err != nil {
								//Rollback tx
								return err
							}
						}
						return nil
					})
					dbLock.Unlock()
					if err != nil {
						fmt.Printf("Could Not Writeback Validation Info")
						continue;	
					}
				} else {
					//Error
					fmt.Printf("Could Not Batch Signed Certs and Revocations: %s", err)
				}
			case <- stop:
				done <- true
				return
		}
	}
}

//Handles Fabric Block Event
func handleEvent(n uint64) {
	var merkleRoots [][]byte
	var revocations [][]byte
	
	logHasher, err := blockchain.InitHasher()
    if err != nil {
        fmt.Printf("Could Not Create Log Hasher: %v\n", err)
        return
    }

    blockMerkleTree := merkle.NewInMemoryMerkleTree(logHasher)
	
	//Get Block Information
	sdkLock.Lock()
	block, err := fSetup.GetBlock(uint64(0))
	sdkLock.Unlock()
	if err != nil {
		fmt.Printf("Could not handle block event: %s\n", err)
		return
	}

	for index, valid := range block.Metadata.Metadata[2] {
		if valid != 0 {
			//If tx was not accepted by peer, continue to next transaction
			continue
		}

		for _, write := range block.Transactions[index].Writes {			
			var revokeJson [][]byte //[]blockchain.ProofFile
			var temp *blockchain.ProofFile
			//Parse Merkle Roots and Revocations
			rootString, err := url.QueryUnescape(write.KvRwSet.Writes[0].Key)
			if err != nil {
				fmt.Printf("Could not handle block event: %s", err)
				return
			}
			if err = json.Unmarshal(write.KvRwSet.Writes[0].Value, &revokeJson); err != nil {
				fmt.Printf("Could not handle block event: %s", err)
				return
			}

			for _,r := range revokeJson {
				if temp, err = blockchain.ParsePCN(r); err != nil{
					fmt.Printf("Could not handle block event: %s", err)
					return
				}
				fmt.Printf("%+v\n", temp)
				revocations = append(revocations, []byte(strings.Replace(temp.ProofList.Revoke.Cert, "REVOKE\n", "", 1)))
			}
			fmt.Printf("Merkle Root: %x\n", rootString)
			blockMerkleTree.AddLeaf([]byte(rootString))
			merkleRoots = append(merkleRoots, []byte(rootString))
		}
	}
	fmt.Printf("Block Merkle Tree Root: %x\n", blockMerkleTree.CurrentRoot().Hash())

	//Start Write Transaction
	dbLock.Lock()
	err = db.Update(func(tx *bolt.Tx) error {
		root := tx.Bucket([]byte("USERS"))
		//Create user iterator
		outter := root.Cursor()
		var value dbValue
		for merkleRootLeafIndex,merkleRootLeaf := range merkleRoots { 
			//Iterate over all users
			for user,_ := outter.First(); user != nil; user,_ = outter.Next() {
				//Create CSR iterator for current user
				inner := root.Bucket(user).Cursor()
				//Iterate over all CSRs for user
				for k,v := inner.First(); k != nil; k,v = inner.Next() {
					//Convert JSON string to dbValue
					if err := json.Unmarshal(v, &value); err != nil {
						return err
					}
					if (value.Status & SIGNED != 0) && bytes.Equal(value.PubValidationInfo.MerkleRoot, merkleRootLeaf) {
						var hashes [][]byte
						fmt.Printf("Marking As Published\n")						
						//Update DB entry with published info						
						value.Status = PUBLISHED
						value.PubValidationInfo.BlockIndex = int64(n)
						value.BroadcastValidationInfo = blockchain.ValidationInfo{int64(merkleRootLeafIndex), int64(n - blockchain.BlockOffset), blockMerkleTree.LeafCount(), blockMerkleTree.CurrentRoot().Hash(), proofArray(blockMerkleTree.PathToCurrentRoot(int64(merkleRootLeafIndex)+1))}
						//Create PCNS						
						hashes = append(hashes, proofArray(blockMerkleTree.PathToCurrentRoot(int64(merkleRootLeafIndex)+1))...)
						hashes = append(hashes, blockMerkleTree.CurrentRoot().Hash())
						temp, err := blockchain.ParsePCN([]byte(value.PCN))
						if err != nil {
							return err
						}
						temp.AddMerkleProof(&blockchain.ValidationInfo{int64(merkleRootLeafIndex), int64(n - blockchain.BlockOffset), blockMerkleTree.LeafCount(), nil, hashes})
						value.PCN, err = temp.ToFileFormat()
						if err != nil {
							return err
						} 
						valueString, err := json.Marshal(value)
						if err != nil {
							return err
						}
						if err = root.Bucket(user).Put(k,valueString); err != nil {
							return err
						}
					}
				}
			}
		}
		return nil
	})
	dbLock.Unlock()
	if err != nil {
		fmt.Printf("Could not handle block event: %s", err)
		return
	}

	//Add all revocations to local state
	for _,revocation := range revocations {
		cert, _, err := pemToCert(string(revocation))
		if err != nil {
			fmt.Printf("Error Processing Revocation\n")			
			return
		}
		var rsaKey *rsa.PublicKey
		rsaKey = cert.PublicKey.(*rsa.PublicKey)
		sum := sha256.Sum256([]byte(fmt.Sprintf("%s%d", rsaKey.N.String(), rsaKey.E)))
		key := sum[:]	

		dbLock.Lock()
		err = db.Update(func(tx *bolt.Tx) error {
			root := tx.Bucket([]byte("USERS"))
			bucket, err:= root.CreateBucketIfNotExists([]byte(strings.ToLower(cert.Subject.CommonName)))
			if err != nil {
				return err
			}
			resp := bucket.Get(key)
			// Cert managed by another PM, Create new entry in PM's key value store
			if resp == nil {
				fmt.Printf("User managed by another PM, Create new entry in PM's key value store\n")
				value := dbValue{cert.Raw, cert.Issuer.CommonName, cert.Subject.CommonName, REVOKED_PUBLISHED, blockchain.ValidationInfo{}, blockchain.ValidationInfo{}, nil}
				jsonStr, err := json.Marshal(value)
				if err != nil {
					return err
				}
				err = bucket.Put(key, jsonStr)
				if err != nil {
					return err
				}

				bucket, err:= root.CreateBucketIfNotExists([]byte(strings.ToLower(cert.Subject.CommonName)))
				if err != nil {
					return err
				}
				resp := bucket.Get(key)
				if resp == nil {
					err = bucket.Put(key, jsonStr)
					if err != nil {
						return err
					}
				} else {
					var value dbValue
					if err := json.Unmarshal(resp, &value); err != nil {
						return err
					}
					value.Status = REVOKED_PUBLISHED
					jsonStr, err := json.Marshal(value)
					if err != nil {
						return err
					}
					err = bucket.Put(key, jsonStr)
					if err != nil {
						return err
					}
				}	
			} else {
				// Cert managed by this PM, Update PM's key value store					
				//Update subject's entry				
				fmt.Printf("User managed by this PM, Update PM's key value store\n")
				var value dbValue
				if err := json.Unmarshal(resp, &value); err != nil {
					return err
				}
				value.Status = REVOKED_PUBLISHED
				jsonStr, err := json.Marshal(value)
				if err != nil {
					return err
				}
				err = bucket.Put(key, jsonStr)
				if err != nil {
					return err
				}
				//Update CA's entry
				bucket = root.Bucket([]byte(strings.ToLower(cert.Issuer.CommonName)))
				if err != nil {
					return err
				}
				err = bucket.Put(key, jsonStr)
				if err != nil {
					return err
				}
			}
			return nil
		})
		dbLock.Unlock()
		if err != nil {
			fmt.Printf("Could not handle block event: %s", err)
			return
		}
	}
}

func main() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)

	stopBatcher := make(chan bool)
	bathcerStopped := make(chan bool)
	stopBlockListener := make(chan bool)
	blockListenerStopped := make(chan bool)

	if err := initFabricContext(); err != nil {
		fmt.Printf("Could Not init fabric context: %s", err)
		//return
	}
	defer func() {
		sdkLock.Lock()
		fSetup.Close()
		sdkLock.Unlock()
	}()

	if err := initDataStore(); err != nil {
		fmt.Printf("Coud Not init data store: %s", err)
		return
	}
	defer func() {
		dbLock.Lock()
		db.Close()
		dbLock.Unlock()
	}()

	// Start Batcher
	go batcher(stopBatcher, bathcerStopped)

	// Start Block Listener
	go blockchain.BlockListener(&sdkLock, &fSetup, handleEvent, stopBlockListener, blockListenerStopped)

	//Run Cleanup Code on Ctrl + c
	go func(){
		<-c
		fmt.Printf("\nShutting Down...\n")
		
		fmt.Printf("Signaling Batcher Routine to Stop...\n")
		close(stopBatcher)
		<-bathcerStopped
		fmt.Printf("...Batcher Routine Stopped\n")

		fmt.Printf("Signaling Block Listener Routine to Stop...\n")
		close(stopBlockListener)
		<-blockListenerStopped
		fmt.Printf("...Block Listener Routine Stopped\n")
		
		fmt.Printf("Closing Fabric SDK...\n")
		sdkLock.Lock()
		fSetup.Close()
		sdkLock.Unlock()
		
		fmt.Printf("...Fabric SDK Closed\n")
		fmt.Printf("Closing boltDb...\n")
		dbLock.Lock()
		db.Close()
		dbLock.Unlock()
		
		fmt.Printf("...boltDb Closed\n")
		fmt.Printf("...Shutdown Complete\n")
		os.Exit(1)
	}()

	serveMux := http.NewServeMux()

	serveMux.HandleFunc("/", indexHandler)
	serveMux.HandleFunc("/app.js", fileHandler)
	serveMux.HandleFunc("/favicon.io", fileHandler)
	serveMux.HandleFunc("/node_modules/", fileHandler)
	serveMux.HandleFunc("/components/", fileHandler)
	serveMux.HandleFunc("/assets/", fileHandler)
	serveMux.HandleFunc("/csr/", csrHandler)
	serveMux.HandleFunc("/revoke/", revokeHandler)
	serveMux.HandleFunc("/getAttr", getAttributes)
	fmt.Println("Listening on Port 8080")
	log.Fatal(http.ListenAndServeTLS(":8080", "certs/gpchain-webserver.crt", "certs/gpchain-webserver.key", serveMux))
}
