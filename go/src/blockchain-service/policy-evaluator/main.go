package main

import (
	"fmt"
	"flag"
	"os"
	"time"
	"errors"
	"strings"
	"encoding/pem"
	"encoding/json"
	"io/ioutil"
	"crypto/x509"
	"unicode"
)

type Attribute struct {
	Value string
	CanConfer bool
}

type policyNode struct {
	Parent *policyNode
	Children []*policyNode
	Value string
}

type policyBook struct {
	Trees []*policyNode
}

var stringToAttr map[string]*policyNode

func (pn policyNode) printSubTree(n int) {
	for i:= 0; i < n; i++ {
		fmt.Printf("\t")
	}

	for i:= 0; i < n; i++ {
		fmt.Printf("\t")
	}
	fmt.Printf("Value: %s\n", pn.Value)

	if len(pn.Children) != 0 {
		for _,child := range pn.Children {
			child.printSubTree(n+1)
		}
	}
}

func (pn policyNode) canAssign(node *policyNode, canConfer bool) error {
	if !canConfer {
		return errors.New(fmt.Sprintf("Attribute (%s, %t) cannot grant children\n", pn.Value, canConfer))
	}
	for i := 0; i < len(pn.Children); i++ {
		if pn.Children[i] == node {
			return nil
		}
	}
	return errors.New(fmt.Sprintf("%s cannot assign %s!", pn.Value, node.Value))
}

func (pb policyBook) Print() {
	for _,policyNode := range pb.Trees {
		policyNode.printSubTree(0)
	}
}

func (pn *policyNode) mapSubTree(m map[string]*policyNode) map[string]*policyNode {
	if len(pn.Children) != 0 {
		for _,child := range pn.Children {
			child.mapSubTree(m)
		}
	}
	m[pn.Value] = pn
	return m
}
func (pb *policyBook) Map() map[string]*policyNode {
	m := map[string]*policyNode{}
	for _,policyNode := range pb.Trees {
		policyNode.mapSubTree(m)
	}
	return m
}

func (pn *policyNode) getAttrs(path string) []string {
	var attrs []string
	var currentPath string
	if len(path) == 0 {
		currentPath = pn.Value
	} else {
		currentPath = fmt.Sprintf("%s.%s", path, pn.Value)
	}
	if len(pn.Children) != 0 {
		for _,child := range pn.Children {
			attrs = append(attrs, child.getAttrs(currentPath)...)
		}
	}
	return append(attrs, currentPath)
}

func (pb *policyBook) PrintAttr() error{
	var attrs []string
	for _,policyNode := range pb.Trees {
		attrs = append(attrs, policyNode.getAttrs("")...)
	}
	jsonStr, err := json.Marshal(attrs)
	if err != nil {
		return err
	}
	fmt.Printf("%s\n", jsonStr)
	return nil
}

func findIndexOfPair(str string, open, close rune) int {
	opens := -1
	for i,c := range str {
		if c == open {
			opens++
		} else if c == close {
			if opens == 0 {
				return i
			} else {
				opens--
			}
		}
	}
	return 0
}

func parseSet(s string) ([]*policyNode, error) {
	var siblings []*policyNode
	if s[0] != '{' {
		return nil, errors.New(fmt.Sprintf("Invalid character got '%c', expected '{'", s[0]))
	}
	if s[1] == '}' {
		return nil, nil
	}

	var end int
	for next := s[1:]; len(next) != 0 && next != ")"; next = next[end+1:] {
		end = findIndexOfPair(next, '(', ')')+1
		if end == 0 {
			return nil, errors.New("Could not find ')', check syntax of policy book")
		}
		sib, err := parsePair(next[0:end])
		if err != nil {
			return nil, err
		}
		siblings = append(siblings, sib)
	}
	return siblings, nil
}

func parsePair(s string) (*policyNode, error) {
	var children []*policyNode
	var err error
	if s[0] != '(' {
		return nil, errors.New(fmt.Sprintf("Invalid character got '%c', expected '('", s[0]))
	}
	key := s[1:strings.Index(s, ",")]
	if children, err = parseSet(s[strings.Index(s, ",")+1:]); err != nil {
		return nil, err
	}
	node := &policyNode{nil, children, key}
	for _, child := range children {
		child.Parent = node
	}
	return node, nil
}

func buildPolicyBook(pbName string) policyBook{
	//Get file descriptor for pb file
	fd, err := os.Open(pbName)
	if err != nil {
		fmt.Printf("Could not open policy book: %s\n", err)
		os.Exit(1)
	}
	
	//Read file as string
	pString, err := ioutil.ReadAll(fd)
	if err != nil {
		fmt.Printf("Could not read 509 certificate chain: %s\n", err)
		os.Exit(1)
	}
	fd.Close()

	rootNode, err := parsePair(strings.ReplaceAll(string(pString), " ", ""))
	pBook := policyBook{[]*policyNode{rootNode}}
	stringToAttr = (&pBook).Map()

	return pBook
}

func decodeCertChain(chain []byte) ([]*x509.Certificate, error){
	var certs []*x509.Certificate

	//Decode PEM encoded Cert Chain
	for temp, residue := pem.Decode(chain); temp != nil; temp, residue = pem.Decode(residue) {
		if temp == nil || temp.Type != "CERTIFICATE" {
			return nil, errors.New("Could not decode PEM string\n")
		}
		cert, err := x509.ParseCertificate(temp.Bytes)
		if err != nil {
			return nil, err
		}
		certs = append(certs, cert)
	}
	return certs, nil
}

func checkPolicy(pb policyBook, certs []*x509.Certificate) error {
	var attributeChain []Attribute
	
	//Verify certs are valid x509
	for i := 0; i < len(certs); i++ {
		var next *x509.Certificate
		cert := certs[i]
		fmt.Printf("Verifying %s's Certificate Has Not Expired...\n", cert.Subject.CommonName)
		now := time.Now().Unix()
		if now > cert.NotAfter.Unix() || now < cert.NotBefore.Unix() {
			return errors.New("Certificate chain contains expired certificate.\n")
		}
		fmt.Printf("...Confirmed\n")
		if i != len(certs) -1 {
			next = certs[i+1]
			fmt.Printf("Verifying %s signed %s's Certificate...\n", next.Subject.CommonName, cert.Subject.CommonName)
			if err := cert.CheckSignatureFrom(next); err != nil {
				return err
			}
			fmt.Printf("...Confirmed\n")
		} 

		hasExt := false
		for _,ext := range cert.Extensions {
			if ext.Id.String() == "1.3.6.1.5.5.7.10" {
				attrArray := strings.Split(string(ext.Value), "_")
				fmt.Printf("Attr Array: %+v\n", attrArray)
				var canConfer bool
				if len(attrArray) > 1 && attrArray[1] == "grants" {
					canConfer = true
				} else {
					canConfer = false
				}
				attributeChain = append(attributeChain, Attribute{strings.TrimFunc(attrArray[0],func(r rune) bool {
					return !unicode.IsLetter(r) && !unicode.IsNumber(r)
				}), canConfer})
				fmt.Printf("Attr Chain: %+v\n", attributeChain)
				hasExt = true
				break;
			}
		}
		if !hasExt {
			return errors.New(fmt.Sprintf("Certificate with subject %+v does not have Attribute Certificate Extension!", cert.Subject))
		}
	}

	//Verify policy
	for i:= 0; i < len(attributeChain); i++ {
		attr := attributeChain[i]
		if i != len(attributeChain)-1 {
			fmt.Printf("Verifying %s,%t can assign %s,%t...\n", attributeChain[i+1].Value, attributeChain[i+1].CanConfer, attr.Value, attr.CanConfer)
			if !strings.HasPrefix(attr.Value, attributeChain[i+1].Value) {
				return errors.New("Invalid attribute path")
			}
			currentString := strings.Split(attr.Value, ".")[len(strings.Split(attr.Value, "."))-1]
			nextString := strings.Split(attributeChain[i+1].Value, ".")[len(strings.Split(attributeChain[i+1].Value, "."))-1]
			next := stringToAttr[nextString]
			if err := next.canAssign(stringToAttr[currentString], attributeChain[i+1].CanConfer); err != nil {
				return err
			}
			fmt.Printf("...Confirmed\n")
		} else {
			fmt.Printf("Verifying attribute chain ends with Root...\n")
			if attr.Value != "Root" {
				return errors.New("Attribute chain is not terminated by Root")
			}
			fmt.Printf("...Confirmed\n")
		}
	}
	fmt.Printf("Certificate Chain Validated\n")
	return nil
}

func main() {
	permissionChain := flag.String("chain", "chain.cert.pem", "")
	// root := flag.String("root", "ca.cert.pem", "")
	pbName := flag.String("pb", "pb.txt", "")
	printAttr := flag.Bool("printAttr", false, "")
	flag.Parse()
	
	//Load Policy Book
	pb := buildPolicyBook(*pbName)
	
	if *printAttr {
		if err := pb.PrintAttr(); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}

	fmt.Printf("Loaded Policy Book:\n")
	pb.Print()
	

	//Get file descriptor x509 certificate chain
	fd, err := os.Open(*permissionChain)
	if err != nil {
		fmt.Printf("Could not open x509 certificate chain: %s\n", err)
		os.Exit(1)
	}
	
	//Read file as string
	pemString, err := ioutil.ReadAll(fd)
	if err != nil {
		fmt.Printf("Could not read 509 certificate chain: %s\n", err)
		os.Exit(1)
	}
	fd.Close()
	
	certs, err := decodeCertChain(pemString)
	if err != nil {
		fmt.Printf("%s\n", err)
		os.Exit(1)
	}
	
	//Get file descriptor root x509 certificate
	// fd, err = os.Open(*root)
	// if err != nil {
	// 	fmt.Printf("Could not open root x509 certificate: %s\n", err)
	// 	os.Exit(1)
	// }
	
	// //Read file as string
	// pemString, err = ioutil.ReadAll(fd)
	// if err != nil {
	// 	fmt.Printf("Could not read root x509 certificate: %s\n", err)
	// 	os.Exit(1)
	// }
	fd.Close()

	// temp, err := decodeCertChain(pemString)
	// if err != nil {
	// 	fmt.Printf("Could not parse root x509 cert to go struct")
	// 	os.Exit(1)
	// }
	// rootCert := temp[0]

	if err := checkPolicy(pb, certs); err != nil {
		fmt.Printf("...Denied\n%s\n", err)
		os.Exit(1)
	}
	os.Exit(0)
}
