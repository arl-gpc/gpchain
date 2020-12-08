DIR=$PWD
echo $DIR
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
	cat >> $HOME/.bashrc <<- EOM
	#---setRuntimeEnvPM.sh---
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
fi

perl -pi.bak -e 's/\$\{Org1Peer0Ip\}/'$Org1Peer0Ip'/g' $DIR/go/src/blockchain-service/permission-marshal/config.yaml
perl -pi.bak -e 's/\$\{Org1Peer0Port\}/'$Org1Peer0Port'/g' $DIR/go/src/blockchain-service/permission-marshal/config.yaml
perl -pi.bak -e 's/\$\{Org1Peer0EventPort\}/'$Org1Peer0EventPort'/g' $DIR/go/src/blockchain-service/permission-marshal/config.yaml

perl -pi.bak -e 's/\$\{Org2Peer0Ip\}/'$Org2Peer0Ip'/g' $DIR/go/src/blockchain-service/permission-marshal/config.yaml
perl -pi.bak -e 's/\$\{Org2Peer0Port\}/'$Org2Peer0Port'/g' $DIR/go/src/blockchain-service/permission-marshal/config.yaml
perl -pi.bak -e 's/\$\{Org2Peer0EventPort\}/'$Org2Peer0EventPort'/g' $DIR/go/src/blockchain-service/permission-marshal/config.yaml

perl -pi.bak -e 's/\$\{Orderer0Ip\}/'$Orderer0Ip'/g' $DIR/go/src/blockchain-service/permission-marshal/config.yaml
perl -pi.bak -e 's/\$\{Orderer1Ip\}/'$Orderer1Ip'/g' $DIR/go/src/blockchain-service/permission-marshal/config.yaml


