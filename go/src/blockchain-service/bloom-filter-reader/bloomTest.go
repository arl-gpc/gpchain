package main

import (
	"fmt"
	"flag"
	"os"
	"io/ioutil"
	//"encoding/base64"
	"encoding/json"
	"bytes"
	"github.com/willf/bloom"
)

// Exit Code 0: Data is part of bloom filter
// Exit Code 1: Data is not in bloom filter
// Exit Code 2: Error occurred

type filterFile struct {
	Index int
	Filter []byte
}

func main() {
	//n : number of items in bloom filter, p : probability of false positives, m : number of bits in the filter, k : number of hash functions
	var n = uint(1000)
	var p = 0.000001
	var filter = bloom.NewWithEstimates(n, p)
	var ff filterFile

	data := flag.String("data", "", "")
	filterPath := flag.String("filter", "", "")
	flag.Parse()

	filterFd, err := os.Open(*filterPath)
	if err != nil {
		fmt.Printf("Could not open bloom filter file: %s\n", err)
		os.Exit(2)
	}

	filterJson, err := ioutil.ReadAll(filterFd)
	if err != nil {
		fmt.Printf("Could not read bloom filter file: %s\n", err)
		os.Exit(2)
	}

	if err = json.Unmarshal(filterJson, &ff); err != nil {
		fmt.Printf("Could not json unmarshal bloom filter file: %s\n", err)
		os.Exit(2)
	}

	filterBuffer := bytes.NewBuffer(ff.Filter)

	_,err = filter.ReadFrom(filterBuffer)
	if err != nil {
		fmt.Printf("Could not read base64 decoded bloom filter: %s\n", err)
		os.Exit(2)
	}
	
	if filter.Test([]byte(*data)) {
		fmt.Printf("Data included in bloom filter!\n")
		os.Exit(0)
	} else {
		fmt.Printf("Data not included in bloom filter!\n")
		os.Exit(1)
	}
}