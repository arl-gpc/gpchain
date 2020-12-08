package main

import (
	"fmt"
	"encoding/base64"
	"encoding/json"
	"bytes"
	"strings"
	"github.com/willf/bloom"
	"flag"
	"crypto/sha256"
	"io/ioutil"
)

type bloomMessage struct {
	Index uint64 `json:"index"` // Relay block index commiting to this bloom filter
	Filter []byte `json:"filter"` // Byte repersenation of bloom filter
}	

func main() {
	//n : number of items in bloom filter, p : probability of false positives, m : number of bits in the filter, k : number of hash functions
	var n = uint(1000)
	var p = 0.000001
	var filter = bloom.NewWithEstimates(n, p)
	pem := flag.String("pem", "", "")
	flag.Parse()

	s := *pem
	start := strings.Index(s, "-----BEGIN CERTIFICATE-----") + int(len("-----BEGIN CERTIFICATE-----"))
	end := strings.Index(s, "-----END CERTIFICATE-----") 
		
	base64Str := s[start:end]
		
	certData, err := base64.StdEncoding.DecodeString(base64Str)
	if err!= nil {
		fmt.Printf("Error: Could not decode cert: %v\n", err)
		return
	}

	certHash := sha256.Sum256(certData)
	filter.Add(certHash[:])
	filterBuffer := bytes.NewBuffer([]byte{})
	_, err = filter.WriteTo(filterBuffer)
	if err != nil {
		fmt.Printf("Error: Could not write bloom filter: %v\n", err)
		return
	}
	
	bloomMsg := bloomMessage{1, filterBuffer.Bytes()}
	bloomMsgStr, err := json.Marshal(bloomMsg)
	if err != nil {
		fmt.Printf("Error: create json bloom message: %v\n", err)
		return
	}

	if err = ioutil.WriteFile("bloomFilter.txt", bloomMsgStr, 0644); err != nil {
		fmt.Printf("Could not write bloom filter file: %s\n", err)
		return
	}		
}
