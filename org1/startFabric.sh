#!/bin/bash
CHAINCODE_VERSION=1.36

#
# Copyright IBM Corp All Rights Reserved
#
# SPDX-License-Identifier: Apache-2.0
#
# Exit on first error
set -e

# don't rewrite paths for Windows Git Bash users
export MSYS_NO_PATHCONV=1

starttime=$(date +%s)

ORDERER_CA=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/ordererOrganizations/example.com/tlsca/tlsca.example.com-cert.pem
CC_RUNTIME_LANGUAGE=golang
CC_SRC_PATH_MNN=chaincode/gpchain


# clean the keystore
rm -rf ./hfc-key-store

# launch network; create channel and join peer to channel
cd basic-network
./start.sh

# Now launch the CLI container in order to install, instantiate chaincode
docker-compose -f ./docker-compose.yml up -d cli

#----------------------------------------------------------------------------
docker exec -e "CORE_PEER_LOCALMSPID=Org1MSP" -e "CORE_PEER_MSPCONFIGPATH=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org1.example.com/users/Admin@org1.example.com/msp" cli peer chaincode install -n pubcc -v $CHAINCODE_VERSION -p "$CC_SRC_PATH_MNN" -l "$CC_RUNTIME_LANGUAGE"
echo "Press Enter: "
read
docker exec -e "CORE_PEER_LOCALMSPID=Org1MSP" -e "CORE_PEER_MSPCONFIGPATH=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org1.example.com/users/Admin@org1.example.com/msp" cli peer chaincode instantiate -o orderer0.example.com:7050 -C mychannel -n pubcc -l "$CC_RUNTIME_LANGUAGE" -v $CHAINCODE_VERSION -c '{"Args":["","-----BEGIN%20CERTIFICATE-----%0AMIIDyTCCArGgAwIBAgIUCQkgnMVRWn07RGFL1AAozSs5fwwwDQYJKoZIhvcNAQEL%0ABQAwXzELMAkGA1UEBhMCVVMxCzAJBgNVBAgMAlBBMRYwFAYDVQQHDA1TdGF0ZSBD%0Ab2xsZWdlMQ0wCwYDVQQKDARSb290MQ0wCwYDVQQLDARSb290MQ0wCwYDVQQDDARS%0Ab290MB4XDTIwMDIxMjE4MDU0NVoXDTIxMDIxMTE4MDU0NVowXzELMAkGA1UEBhMC%0AVVMxCzAJBgNVBAgMAlBBMRYwFAYDVQQHDA1TdGF0ZSBDb2xsZWdlMQ0wCwYDVQQK%0ADARSb290MQ0wCwYDVQQLDARSb290MQ0wCwYDVQQDDARSb290MIIBIjANBgkqhkiG%0A9w0BAQEFAAOCAQ8AMIIBCgKCAQEAw0s5T0R5EASJ%2BtOXILSgO40Wvkzyq%2F86Erbh%0AWgRPnAG3ppVYJts%2Bb46YyL%2F4eSvDMAtLgMlyI5oLExr9L56v7WM7Ck7OEsmLrV3q%0Apvctf2T%2BSLYcDUB3TUDsXatSRizWtthi9UMayeKAxgBoromfKS7oFY7UNhM%2FaSWd%0ASmfdvTSCkMqdHNKZA2od7MikAgMP4DlK%2Fl%2BOecAP%2FhLnh4QPB1ZF18%2BUvSyoaSbX%0A9D7VFpe%2FSfl8%2FU9Of9m39eWmvmq8aFmpNCGGE6mjXEqP9bT%2FoklTuJMFZTI7omPS%0AUMIR%2Be00f6bbAeqNX8XUt4c2D%2FhgSoCbCP7EtzlimUq2VfFx7wIDAQABo30wezAY%0ABgcrBgEFBQcKBA0MC1Jvb3RfZ3JhbnRzMB0GA1UdDgQWBBQR5UHT7qy4mu%2BGs5AG%0AcuqU0xmouTAfBgNVHSMEGDAWgBQR5UHT7qy4mu%2BGs5AGcuqU0xmouTAPBgNVHRMB%0AAf8EBTADAQH%2FMA4GA1UdDwEB%2FwQEAwIBhjANBgkqhkiG9w0BAQsFAAOCAQEAuZks%0AzZ8PosSPzf8QjDaUOZShPEqmhtiwqcTHIYMFcH%2Folf9iSWP8uLqMIkFO58uc42YZ%0Af9KzaQmb8p8Pzq9W9A0a28lx%2FbR4X3PXh53YEspqJR8ssHypsjaEFtiKhTdKSKfA%0AF%2BOnXYv0jumOO5vF8wNhBKANiGLw1adM%2BUJTmaJrYztYJ4MkGMHzUltTdJFSRUOl%0Aovfl0smtvK4H94exFxX2rkzbTfurIstSuS%2BCs7HaLmXsEc5mYnAD5xFsEAvlAiDC%0Atv8fjwMvk09ZB%2FkvmGIdevJaHgJZJ2je0vKnzOj73Yvq30PEEZk%2B6YxxbsO%2BXFb8%0AZ4OjWivvZhu5S0X9aQ%3D%3D%0A-----END%20CERTIFICATE-----"]}' -P "AND ('Org1MSP.member','Org2MSP.member')" --tls --cafile $ORDERER_CA --peerAddresses peer0.org1.example.com:7051 peer0.org2.example.com:7051 --tlsRootCertFiles /opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org1.example.com/tlsca/tlsca.org1.example.com-cert.pem /opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org2.example.com/tlsca/tlsca.org2.example.com-cert.pem

cat <<EOF
EOF
