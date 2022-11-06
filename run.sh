#!/bin/bash

rm -rf ./repo || true
rm ./speakeasy || true

set -a
source <(cat .env | sed -e '/^#/d;/^\s*$/d' -e "s/'/'\\\''/g" -e "s/=\(.*\)/='\1'/g")
set +a

go run .