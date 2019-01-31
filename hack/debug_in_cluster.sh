#!/bin/bash
set -e
tag=`git rev-parse --short HEAD`
IMG=dockerhub.qingcloud.com/magicsong/porter:$tag
echo "Building binary"
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -o bin/manager cmd/manager/main.go
echo "Binary build done, Build docker image, $IMG"
docker build -f deploy/Dockerfile -t ${IMG} bin/
echo "Docker image build done, try to push to registry"
docker push $IMG
echo "updating kustomize image patch file for manager resource"
sed -i'' -e 's@image: .*@image: '"${IMG}"'@' ./config/default/manager_image_patch.yaml
dockerconfig=`cat ~/.docker/config.json | base64 -w 0`
sed -i -e 's/dockerconfigjson:.*/dockerconfigjson: $dockerconfig/' ./config/default/manager_secret_patch.yaml
echo "Deploying for test"
kustomize build config/default -o deploy/release.yaml
kubectl apply -f deploy/release.yaml
kubectl delete pod controller-manager-0 -n porter-system
echo "Done! Let's roll"