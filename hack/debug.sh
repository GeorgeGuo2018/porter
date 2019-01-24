#!/bin/bash
set -e
git commit -a -m "auto commit in dev"
tag=`git rev-parse --short HEAD`
IMG=dockerhub.qingcloud.com/magicsong/porter:$tag
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -o bin/manager cmd/manager/main.go
docker build -f deploy/Dockerfile -t ${IMG} bin/
docker push $IMG
echo "updating kustomize image patch file for manager resource"
sed -i'' -e 's@image: .*@image: '"${IMG}"'@' ./config/default/manager_image_patch.yaml

kustomize build config/default -o release.yaml
kubectl apply -f release.yaml
kubectl delete pod controller-manager-0 -n porter-system
