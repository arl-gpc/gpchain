#!/bin/bash
CHAINCODE_VERSION=1.33

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
./start-peer2.sh

# Now launch the CLI container in order to install
docker-compose -f ./docker-compose-peer2.yml up -d cli2


#----------------------------------------------------------------------------
docker exec -e "CORE_PEER_LOCALMSPID=Org2MSP" -e "CORE_PEER_MSPCONFIGPATH=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org2.example.com/users/Admin@org2.example.com/msp" cli2 peer chaincode install -n pubcc -v $CHAINCODE_VERSION -p "$CC_SRC_PATH_MNN" -l "$CC_RUNTIME_LANGUAGE"
echo "Press Enter:"
read _
docker exec -e "CORE_PEER_LOCALMSPID=Org2MSP" -e "CORE_PEER_MSPCONFIGPATH=/opt/gopath/src/github.com/hyperledger/fabric/peer/crypto/peerOrganizations/org2.example.com/users/Admin@org2.example.com/msp" cli2 peer chaincode instantiate -o orderer1.example.com:7050 -C mychannel -n pubcc -l "$CC_RUNTIME_LANGUAGE" -v $CHAINCODE_VERSION -c '{"Args":["","-----BEGIN%20CERTIFICATE-----%0AMIIDxzCCAq%2BgAwIBAgIUTSdeEoNjvd771CQgM8SrAfJ1JckwDQYJKoZIhvcNAQEL%0ABQAwXzELMAkGA1UEBhMCVVMxCzAJBgNVBAgMAlBBMRYwFAYDVQQHDA1TdGF0ZSBD%0Ab2xsZWdlMQ0wCwYDVQQKDARSb290MQ0wCwYDVQQLDARSb290MQ0wCwYDVQQDDARS%0Ab290MB4XDTE5MTEwMTE0MzQyMFoXDTI5MTAyOTE0MzQyMFowXzELMAkGA1UEBhMC%0AVVMxCzAJBgNVBAgMAlBBMRYwFAYDVQQHDA1TdGF0ZSBDb2xsZWdlMQ0wCwYDVQQK%0ADARSb290MQ0wCwYDVQQLDARSb290MQ0wCwYDVQQDDARSb290MIIBIjANBgkqhkiG%0A9w0BAQEFAAOCAQ8AMIIBCgKCAQEAwMi%2FUE5riCJD6WnNEFZnzpay%2FuEbkUmxVIiG%0ARLUdQfhRe9Lz3%2FZIwH6eQ3dDvOTu6b0IeaxbVTNbF26kZPilk5xiCcSfCi4WZgHH%0AUb6NqtZ1V7UhB9mc9%2BE6dPZnqw%2BKa4UW18EUiesWhCSgTH1Vi%2BOpLYE5ciQ%2BoZ71%0AwdCC0y%2B0AFNqcLfpiWIPQ%2FFRUd7gA4WxB8JtEyDd%2BqcZGZYB8ZanUA8Th9AM3481%0AlRFChs%2F1tLnqtd%2BVo04oWOF3cVb5q938sYN8RmV7e0SM4EwXoBTFpRBOQ9hvW4HU%0ADVA%2FpCFA1qeRvV1t16yQ3%2Fndxe%2BZuEnEqEN9FYY82F48gYBrJwIDAQABo3sweTAW%0ABgcrBgEFBQcKBAsMCVJvb3QsdHJ1ZTAdBgNVHQ4EFgQUOdf9wjH96UCxb25va3Lj%0A8tqqyUAwHwYDVR0jBBgwFoAUOdf9wjH96UCxb25va3Lj8tqqyUAwDwYDVR0TAQH%2F%0ABAUwAwEB%2FzAOBgNVHQ8BAf8EBAMCAYYwDQYJKoZIhvcNAQELBQADggEBAADTd9yh%0ARRqMpu1DCUJ7IVojnYQgqbazRhfLViRC2Tpl90wakdOhWhOv6A1ywJpm5f8DwFjB%0AwNxppvXALKprtiweeDCu9O%2B0FwwgniAqFGgFbbiTDK2g2tQVeCxaZYMe%2BJlPGNas%0AAmwnIWUOD63ZA9VBvSbzSz%2Bz4rPRhn6Ck0xvJqm%2FGJUK%2BegQyrVK0aREI%2BELc%2Fv5%0AXrNHkGWZqRof0muZHP6Ysv0iqRCRxmnAScuIJMicKgglQs4Gb%2Ff0tpVVXR6blezw%0AHQRYKRMTKoZrgTsSSe3M7L6F1lIcn9FYVuEwsvfK8E92%2B8C084UfKj%2FRl2CF4HR5%0APwzfudkKFqRhuTg%3D%0A-----END%20CERTIFICATE-----"]}' -P "AND ('Org1MSP.member','Org2MSP.member')" --tls --cafile $ORDERER_CA