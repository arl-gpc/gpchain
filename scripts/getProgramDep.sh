DIR=$PWD/..
echo $DIR
echo "Downloading Permission Marshal Web App Dependencies..."
cd $DIR/go/src/blockchain-service/permission-marshal/app
npm install
echo "...Done"

echo "Downloading Blockchain Utils Dependencies..."
cd $DIR/go/src/blockchain-service/blockchain
govendor sync
echo "...Done"

echo "Downloading Bloom Filter Reader Dependencies..."
cd $DIR/go/src/blockchain-service/bloom-filter-reader
govendor sync
echo "...Done"

echo "Downloading Permission Marshal Dependencies..."
cd $DIR/go/src/blockchain-service/permission-marshal
#govendor update +program
govendor sync
govendor update +external
echo "...Done"

echo "Downloading Relay Dependencies..."
cd $DIR/go/src/blockchain-service/relay
#govendor update +program
govendor sync
govendor update +external
echo "...Done"

echo "Downloading Relay Receiver Dependencies..."
cd $DIR/go/src/blockchain-service/relay-receviver
govendor sync
echo "...Done"

echo "Downloading pubcc Dependencies..."
cd $DIR/go/src/chaincode/gpchain
govendor sync
govendor update +external
echo "...Done"
