#!/bin/bash

#Setup Environment variables
cat >> $HOME/.bashrc <<- EOM
#---setBuildEnv.sh---
export GOROOT=/usr/local/go
export GOPATH=$PWD/../go
export FABPATH=$HOME/fabric-samples

export PATH=\$PATH:\$GOROOT/bin
export PATH=\$PATH:\$GOPATH/bin
export PATH=\$PATH:\$FABPATH/bin
#---
EOM

