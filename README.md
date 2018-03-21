[![Build Status](https://travis-ci.org/koli/koli.svg?branch=master)](https://travis-ci.org/koli/koli)

<img src="https://s3.amazonaws.com/kolihub/logos/01.png" width="350">

---

[![Join the chat at https://gitter.im/koli/koli](https://badges.gitter.im/koli/koli.svg)](https://gitter.im/koli/koli?utm_source=badge&utm_medium=badge&utm_campaign=pr-badge&utm_content=badge)

Koli it's a Plataform as Service (PaaS) built on top of Kubernetes, providing tools
that enables easy management and deployment of applications.

*__Note__: This project is in __alpha status__ and is It is subject to architectural changes.
We will change or remove this notice when development status changes.*

TODO

## Quick Start / MacOS Only - (Deprecated)

- Install [kbox](https://github.com/kolihub/kbox)
- Install [kubectl](https://storage.googleapis.com/kubernetes-release/release/v1.4.3/bin/linux/amd64/kubectl)

```bash
kbox setup # Option 3 / Default Option / Default Option
# Wait for all pods and containers starts
kubect get pods -n kube-system -w # On the same shell!

# Install helm (http://helm.sh)
cd /tmp/ && curl http://storage.googleapis.com/kubernetes-helm/helm-v2.0.0-beta.1-darwin-amd64.tar.gz  |tar -xf - && mv darwin-amd64/helm /usr/local/bin/helm
rm -rf ~/.helm && helm init  # Wait for the tiller pod starts  
helm repo add kolihub-alpha https://kolihub.github.io/charts

# Create the namespace koli-system
kubectl create ns koli-system
# Make sure the SYSTEM_TOKEN is set. E.G.: echo $SYSTEM_TOKEN
SYSTEM_TOKEN=$(kubectl get secrets -n kube-system -o yaml |grep -i token: |awk {'print $2'})
helm install kolihub-alpha/koli --set systemToken=$SYSTEM_TOKEN --namespace=koli-system

# Wait for all pods to start
kubectl get pods -n koli-system -w

[START A NEW SHELL]

# Configure koli to interact with the platform
kubectl config set-cluster orion --server=https://$(kbox ip):30443 --insecure-skip-tls-verify=true 
kubectl config set-credentials koli --token=dummy
kubectl config set-context koli --cluster orion --user koli
kubectl config use-context koli

# Download the koli toolbelt command line
curl -o /usr/local/bin/koli https://dl.dropboxusercontent.com/sh/sqae3geyqsgab0z/AABtbZn64-W4eS3eyeRz3IcDa/koli-darwin-amd64-v0.2.0-alpha 
chmod +x /usr/local/bin/koli

koli login # user: guest password: ilok
GUEST_TOKEN=$(koli config view -o jsonpath='{.users[?(@.name == "koli")].user.token}') # Make sure the variable is set with a token

# Configure an addon named "redis"
curl https://gist.githubusercontent.com/sandromello/218ee91f3e45f58448d46acc384d2bc5/raw/95fc1f401b5eab66de3fca1ca501192326c73565/addon-redis.json > /tmp/redis.json 
curl http://192.168.64.2:30080/addons/redis -H "Authorization: Bearer $GUEST_TOKEN" \
  -H 'Content-Type: application/json' -H 'Host: controller.kolihub.io' \
  -XPOST --data-binary @/tmp/redis.json

# Interact with the Koli Platform
koli login # user: guest password: ilok
koli create ns default
koli create addon redis
koli create links redis
(...)
```
