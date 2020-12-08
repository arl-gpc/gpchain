#!/bin/bash

#Install GPCHAIN Blockchain specific software
echo "-----------------------------Blockchain software------------------------------"
echo "------------------------------------------------------------------------------"
echo "------------------------------------------------------------------------------"
echo "------------------------------------------------------------------------------"
echo "------------------------------------------------------------------------------"
echo "------------------------------------------------------------------------------"
echo "------------------------------------------------------------------------------"
echo "------------------------------------------------------------------------------"

#Install golang version 1.12.5 to $INSTALL_PATH saving environment variable to $ENV_PATH
INSTALL_PATH=/usr/local
wget https://dl.google.com/go/go1.12.5.linux-amd64.tar.gz && tar -C $INSTALL_PATH -xzf go1.12.5.linux-amd64.tar.gz && export PATH=$PATH:$INSTALL_PATH/go/bin

cat >> ~/.bashrc <<- EOM
#---getSoftwareDep.sh---
export PATH=\$PATH:$INSTALL_PATH/go/bin
#---
EOM

rm -f go1.12.5.linux-amd64.tar.gz

apt-get install docker -y
apt-get install docker-compose -y

apt-get install govendor -y

apt-get install node -y
apt-get install npm -y

echo "------------------------------------------------------------------------------"
echo "-----------------------------Setup Finished-----------------------------------"
echo "------------------------------------------------------------------------------"

