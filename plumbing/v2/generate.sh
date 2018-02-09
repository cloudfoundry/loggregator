#!/bin/bash

dir_resolve()
{
    cd "$1" 2>/dev/null || return $?  # cd to desired directory; if fail, quell any error messages but return exit status
    echo "`pwd -P`" # output full, link-resolved path
}

set -e

TARGET=`dirname $0`
TARGET=`dir_resolve $TARGET`
cd $TARGET

go get github.com/golang/protobuf/{proto,protoc-gen-go}


tmp_dir=$(mktemp -d)
mkdir -p $tmp_dir/loggregator

cp *.proto $tmp_dir/loggregator

protoc $tmp_dir/loggregator/*.proto \
    --go_out=plugins=grpc,Menvelope.proto=code.cloudfoundry.org/go-loggregator/rpc/loggregator_v2,Mingress.proto=code.cloudfoundry.org/go-loggregator/rpc/loggregator_v2:. \
    --proto_path=$tmp_dir/loggregator \
    -I=$tmp_dir/loggregator \
    -I=$GOPATH/src/github.com/cloudfoundry/loggregator-api/v2/.

rm -r $tmp_dir
