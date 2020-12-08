set -ev
# don't rewrite paths for Windows Git Bash users
export MSYS_NO_PATHCONV=1
export FABRIC_START_TIMEOUT=50
ORDERER_CA=./orderer/tlsca.example.com-cert.pem

docker-compose -f docker-compose-peer2.yml down
docker-compose -f docker-compose-peer2.yml up -d orderer1.example.com

sleep ${FABRIC_START_TIMEOUT}

docker-compose -f docker-compose-peer2.yml up -d peer0.org2.example.com ca2.example.com

sleep ${FABRIC_START_TIMEOUT}

docker exec -e "CORE_PEER_MSPCONFIGPATH=/etc/hyperledger/msp/users/Admin@org2.example.com/msp" peer0.org2.example.com peer channel fetch config -o orderer1.example.com:7050 -c mychannel --tls --cafile $ORDERER_CA
docker exec -e "CORE_PEER_MSPCONFIGPATH=/etc/hyperledger/msp/users/Admin@org2.example.com/msp" peer0.org2.example.com peer channel join -b mychannel_config.block
