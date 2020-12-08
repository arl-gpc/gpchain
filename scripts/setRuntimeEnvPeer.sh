#!/bin/bash

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
ARG="none"
CHECK="--configOnly"
if [ -n "$1" ]; 
then
	ARG=$1
fi

if [[ "$ARG" != "$CHECK" ]];
then
	echo "Setting Environment Variables..."
	cat >> $HOME/.bashrc <<- EOM
	#---setRuntimeEnvPeer.sh---
	export GOROOT=/usr/local/go
	export GOPATH=$PWD/go
	export FABPATH=$HOME/fabric-samples

	export PATH=\$PATH:\$GOROOT/bin
	export PATH=\$PATH:\$GOPATH/bin
	export PATH=\$PATH:\$FABPATH/bin
	#---
	EOM

	echo "...Done"
fi

echo "Setting .env File..."
cat > ./basic-network/.env <<-EOM
COMPOSE_PROJECT_NAME=net
ORG1_IP=$Org1Peer0Ip
ORG2_IP=$Org2Peer0Ip
EOM
echo "...Done"


