DIR=$PWD/..
cd $DIR
rm -rf ./build
mkdir build

echo "Copying Fabric Network to build directory..."
cp -r ./{org1,org2} ./build
echo "...Done"
mkdir -p ./build/go/src/blockchain-service/

echo "Building policy-evaluator and bloom filter test"
go build ./go/src/blockchain-service/policy-evaluator/main.go
go build ./go/src/blockchain-service/bloom-filter-reader/bloomTest.go
echo "...Done"

echo "Building Permission Marshal..."
mkdir ./build/go/src/blockchain-service/permission-marshal ./build/go/src/blockchain-service/permission-marshal/data ./build/go/src/blockchain-service/permission-marshal/app
cp -r ./go/src/blockchain-service/permission-marshal/app/{assets,components,app.js,index.html,package.json,package-lock.json} ./build/go/src/blockchain-service/permission-marshal/app
cp -r ./go/src/blockchain-service/permission-marshal/{certs,policy-eval,crypto-config} ./build/go/src/blockchain-service/permission-marshal/
cp ./go/src/blockchain-service/permission-marshal/base.config.yaml ./build/go/src/blockchain-service/permission-marshal/config.yaml
cd ./go/src/blockchain-service/permission-marshal/
govendor update +vendor
cd $DIR
go build ./go/src/blockchain-service/permission-marshal/server.go
mv $DIR/go/src/blockchain-service/policy-evaluator/main $DIR/build/go/src/blockchain-service/permission-marshal/policy-eval/policy-eval
mv $DIR/go/src/blockchain-service/bloom-filter-reader/bloomTest $DIR/build/go/src/blockchain-service/permission-marshal/policy-eval/bloomTest
mv ./server ./build/go/src/blockchain-service/permission-marshal/
echo "...Done"

echo "Building Relay..."
mkdir ./build/go/src/blockchain-service/relay
cp -r ./go/src/blockchain-service/relay/{certs,crypto-config} ./build/go/src/blockchain-service/relay
cp ./go/src/blockchain-service/relay/base.config.1.yaml ./build/go/src/blockchain-service/relay/config.1.yaml
cp ./go/src/blockchain-service/policy-evaluator/pb.txt ./build/go/src/blockchain-service/relay/pb.txt
cd ./go/src/blockchain-service/relay/
govendor update +vendor
cd $DIR
go build ./go/src/blockchain-service/relay/main.go
mv ./main ./build/go/src/blockchain-service/relay/relay
echo "...Done"

echo "Copying Chaincode Source to build directory..."
cd ./go/src/chaincode/gpchain/
govendor update +vendor
cd $DIR
cp -r ./go/src/chaincode ./build/go/src
echo "...Done"

mkdir -p $DIR/build/org1_deploy/go/src/chaincode/
cp -r $DIR/build/org1 $DIR/build/org1_deploy
cp -r $DIR/build/go/src/chaincode/ $DIR/build/org1_deploy/go/src/

mkdir -p $DIR/build/org2_deploy/go/src/chaincode/
cp -r $DIR/build/org2 $DIR/build/org2_deploy
cp -r $DIR/build/go/src/chaincode/ $DIR/build/org2_deploy/go/src/

cp $DIR/scripts/setRuntimeEnvPeer.sh $DIR/build/org1_deploy/org1
cp $DIR/scripts/setRuntimeEnvPeer.sh $DIR/build/org2_deploy/org2

cp $DIR/scripts/getSoftwareDep.sh $DIR/build/org1_deploy
cp $DIR/scripts/getSoftwareDep.sh $DIR/build/org2_deploy

mkdir -p $DIR/build/pm_deploy/go/src/blockchain-service/
cp -r $DIR/build/go/src/blockchain-service/permission-marshal $DIR/build/pm_deploy/go/src/blockchain-service/
cp $DIR/scripts/setRuntimeEnvPM.sh $DIR/build/pm_deploy/

mkdir -p $DIR/build/relay_deploy/go/src/blockchain-service/
cp -r $DIR/build/go/src/blockchain-service/relay $DIR/build/relay_deploy/go/src/blockchain-service/
cp $DIR/scripts/setRuntimeEnvRelay.sh $DIR/build/relay_deploy/

