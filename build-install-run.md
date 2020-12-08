#Build, Install, Run Global Permission Chain

--
--

# Devloping and Building Global Permission Chain

**What does this do?**

* Pulls repository from git.
* Downloads and installs Go, Docker, Docker-Compose, Govendor, NodeJS, Node Package Manager.
* Sets GOPATH, GOROOT, and FABPATH environment variables and adds them to the PATH.
* Downloads Go and Node dependencies needed for all components of project.
* Configure org1/basic-newtwork/.env with user provided IPs.
* Configure org2/basic-newtwork/.env with user provided IPs.
* Configures permission-marshal/config.yaml with user provided IPs.
* Configures relay/config.1.yaml with user provided IPs.
* Builds all components of the project and places them in the gpchain/build directory.

**Instructions**

* *Make sure line-endings are correct for your OS with the following line:*
* git config --global core.autocrlf input
* git clone https://github.com/arl-gpc/gpchain.git
* cd gpchain
* git fetch
* git checkout Deployment_Kit
* cd ./scripts
* sudo ./getSoftwareDep.sh
* *The following appends lines to /home/username/.bashrc so erase them if you do it twice.*
* ./setBuildEnv.sh
* bash
* ./getProgramDep.sh
* *The following appends lines to /home/username/.bashrc so erase them if you do it twice.*
* ./setDevelEnv.sh
* bash
* ./package.sh

**Situations that require additional steps**

If you modify ./org1/basic-network/configtx.yaml or ./org1/basic-network/crypto-config.yaml it is necessary to perform 2 additional steps prior to running "./scripts/package.sh".

1. Run ./org1/basic-network/generate.sh
2. Copy ./org1/basic-network/crypto-config over to ./org2/basic-network/crypto-config

This is because the contents of ./org1/basic-network/config and ./org1/basic-network/crypto-config must be regenerated to match the new configuration.

Todo: What about Relay and Permission Marshal? They have crypto-config directories too, with some but not all of the crypto-config that the peers have.


---
---

# Install Org1 and Org2

* *Copy org1_deploy or org2_deploy to appropriate machine.*
* cd ./orgN_deploy
* sudo su
* *The following line is not necessary if you already ran it on this computer.*
* ./getSoftwareDep.sh
* cd ./orgN
* *Note: Run this next line with --configOnly option if /root/.bashrc is already*
* *configured.*
* *Note2: 
* ./setRuntimeEnvPeer.sh [--configOnly]
* bash

---
---

# Install Permission Marshall

* *Copy pm_deploy to appropriate machine*
* *Might need to make sure node dependencies are loaded:*
* cd pm_deploy/go/src/blockchain-service/permission-marshal/app/
* npm install
* cd pm_deploy
* *Note: Run this next line with --configOnly option if /home/username/.bashrc is already*
* *configured.*
* ./setRuntimeEnvPM.sh [--configOnly]
* bash

---
---

# Install Relay

* *Copy relay_deploy to appropriate machine.*
* cd relay_deploy
* *Note: Run this next line with --configOnly option if /home/username/.bashrc is already*
* *configured.*
* ./setRuntimeEnvRelay.sh [--configOnly]
* bash

---
---

# Running Global Permission Chain

**Open Terminals**

Open 5 terminals.

1. org1-peer-host
2. org2-peer-host
3. permission-marshal-host
4. broker-host
5. relay-host

---

**Shut down components if necessary**

Peer 1
* org1-peer-host: cd org1_deploy/org1/basic-network/
* org1-peer-host: sudo ./teardown.sh

Peer 2
* org2-peer-host: cd org2_deploy/org2/basic-network/
* org2-peer-host: sudo teardown.sh

Relay
* kill relay-pid

Permission Marshal
* kill permission-marshal-pid

---

**Start up Peer 1**

* org1-peer-host: sudo su
* org1-peer-host: cd org1_deploy/org1/
* org1-peer-host: ./startFabric.sh
* (Do not press enter to continue until relay and permission marshal have started.)

**To get logs from Peer**

* docker container ls | grep fabric-peer
* *Grab the CONTAINER ID from the output and use it here:
* docker logs CONTAINER_ID --tail 5000
* *Note that peer outputs to stderr so to output to file you can do*
* docker logs CONTAINER_ID &> log.txt

**To get logs from Chaincode**

* docker container ls | grep chaincode
* *Grab the CONTAINER ID of the correct chaincode from the output and use it here:
* docker logs CONTAINER_ID --tail 5000
* *Note that hyperledger outputs to stderr so to output to file you can do*
* docker logs CONTAINER_ID &> log.txt

**Start up Peer 2**

* org2-peer-host: sudo su
* org2-peer-host: cd org2_deploy/org2/
* org2-peer-host: ./startFabric.sh
* (Do not press enter to continue until relay and permission marshal have started.)

---

**Start up Broker**

* broker-host: activemq-\*/bin/activemq start

---

**Start up Relay**

* relay-host: sudo su
* relay-host: cd relay_deploy/go/src/blockchain-service/relay/
* *The next command has a default of -broker localhost:1883*
* relay-host: ./relay [-broker <brokerIP>:<brokerPort>] > log.txt &
* relay-host: disown
* relay-host: tail -f log.txt

---

**Start up Permission Marshal**

* permission-marshal-host: sudo su
* permission-marshal-host: cd pm_deploy/go/src/blockchain-service/permission-marshal/
* permission-marshal-host: ./server > log.txt &
* permission-marshal-host: disown
* permission-marshal-host: tail -f log.txt

---

**Issue first block**

* org1-peer-host: [Enter]
* org2-peer-host: [Enter]