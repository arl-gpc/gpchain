#!/bin/bash
DIR=$PWD/..

read -p $'Enter Org1 Peer0 IP\n> ' -i 10.0.1.22 -e Org1Peer0Ip
echo $'\n'

read -p $'Enter Org1 Peer0 Port\n> ' -i 7051 -e Org1Peer0Port
echo $'\n'

read -p $'Enter Org1 Peer0 Port\n> ' -i 7053 -e Org1Peer0EventPort
echo $'\n'

read -p $'Enter Org2 Peer0 IP\n> ' -i 10.0.1.4 -e Org2Peer0Ip
echo $'\n'

read -p $'Enter Org2 Peer0 Port\n> ' -i 7051 -e Org2Peer0Port
echo $'\n'

read -p $'Enter Org2 Peer0 Port\n> ' -i 7053 -e Org2Peer0EventPort
echo $'\n'

read -p $'Enter Orderer0 Ip:Port\n> ' -i 10.0.1.22:7050 -e Orderer0Ip
echo $'\n'

read -p $'Enter Orderer1 Ip:Port\n> ' -i 10.0.1.4:7050 -e Orderer1Ip
echo $'\n'

#Setup Environment variables
echo "Setting Network Environment Variables..."
cat >> $HOME/.bashrc <<- EOM
#---setDevelEnv.sh---
export FabriPeerIps=$Org1Peer0Ip:$Org1Peer0Port,$Org2Peer0Ip:$Org2Peer0Port
export Org1Peer0Ip=$Org1Peer0Ip
export Org1Peer0Port=$Org1Peer0Port
export Org1Peer0EventPort=$Org1Peer0EventPort
export Org2Peer0Ip=$Org2Peer0Ip
export Org2Peer0Port=$Org2Peer0Port
export Org2Peer0EventPort=$Org2Peer0EventPort
export Orderer0Ip=$Orderer0Ip
export Orderer1Ip=$Orderer1Ip
#---
EOM
echo "...Done"

echo "Setting org1 .env File..."
cat > $DIR/org1/basic-network/.env <<-EOM
COMPOSE_PROJECT_NAME=net
ORG1_IP=$Org1Peer0Ip
ORG2_IP=$Org2Peer0Ip
EOM
echo "...Done"

echo "Setting org2 .env File..."
cat > $DIR/org2/basic-network/.env <<-EOM
COMPOSE_PROJECT_NAME=net
ORG1_IP=$Org1Peer0Ip
ORG2_IP=$Org2Peer0Ip
EOM
echo "...Done"

echo "Setting permission-marshal config.yaml File..."
cp $DIR/go/src/blockchain-service/permission-marshal/base.config.yaml $DIR/go/src/blockchain-service/permission-marshal/config.yaml 
perl -pi.bak -e 's/\$\{Org1Peer0Ip\}/'$Org1Peer0Ip'/g' $DIR/go/src/blockchain-service/permission-marshal/config.yaml
perl -pi.bak -e 's/\$\{Org1Peer0Port\}/'$Org1Peer0Port'/g' $DIR/go/src/blockchain-service/permission-marshal/config.yaml
perl -pi.bak -e 's/\$\{Org1Peer0EventPort\}/'$Org1Peer0EventPort'/g' $DIR/go/src/blockchain-service/permission-marshal/config.yaml

perl -pi.bak -e 's/\$\{Org2Peer0Ip\}/'$Org2Peer0Ip'/g' $DIR/go/src/blockchain-service/permission-marshal/config.yaml
perl -pi.bak -e 's/\$\{Org2Peer0Port\}/'$Org2Peer0Port'/g' $DIR/go/src/blockchain-service/permission-marshal/config.yaml
perl -pi.bak -e 's/\$\{Org2Peer0EventPort\}/'$Org2Peer0EventPort'/g' $DIR/go/src/blockchain-service/permission-marshal/config.yaml

perl -pi.bak -e 's/\$\{Orderer0Ip\}/'$Orderer0Ip'/g' $DIR/go/src/blockchain-service/permission-marshal/config.yaml
perl -pi.bak -e 's/\$\{Orderer1Ip\}/'$Orderer1Ip'/g' $DIR/go/src/blockchain-service/permission-marshal/config.yaml
echo "...Done"

echo "Setting relay config.1.yaml File..."
cp $DIR/go/src/blockchain-service/relay/base.config.1.yaml $DIR/go/src/blockchain-service/relay/config.1.yaml
perl -pi.bak -e 's/\$\{Org1Peer0Ip\}/'$Org1Peer0Ip'/g' $DIR/go/src/blockchain-service/relay/config.1.yaml
perl -pi.bak -e 's/\$\{Org1Peer0Port\}/'$Org1Peer0Port'/g' $DIR/go/src/blockchain-service/relay/config.1.yaml
perl -pi.bak -e 's/\$\{Org1Peer0EventPort\}/'$Org1Peer0EventPort'/g' $DIR/go/src/blockchain-service/relay/config.1.yaml

perl -pi.bak -e 's/\$\{Org2Peer0Ip\}/'$Org2Peer0Ip'/g' $DIR/go/src/blockchain-service/relay/config.1.yaml
perl -pi.bak -e 's/\$\{Org2Peer0Port\}/'$Org2Peer0Port'/g' $DIR/go/src/blockchain-service/relay/config.1.yaml
perl -pi.bak -e 's/\$\{Org2Peer0EventPort\}/'$Org2Peer0EventPort'/g' $DIR/go/src/blockchain-service/relay/config.1.yaml

perl -pi.bak -e 's/\$\{Orderer0Ip\}/'$Orderer0Ip'/g' $DIR/go/src/blockchain-service/relay/config.1.yaml
perl -pi.bak -e 's/\$\{Orderer1Ip\}/'$Orderer1Ip'/g' $DIR/go/src/blockchain-service/relay/config.1.yaml
echo "...Done"
