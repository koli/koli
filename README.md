## Koli PaaS

Koli it's a Plataform as Service (PaaS) built on top of Kubernetes, providing tools
that enables easy management and deployment of applications.

It's a working in progress project. 

## Development

> The `vendor` folder in kubernetes `k8s.io/kubernetes/vendor` must be purged, otherwise
> it will bump into this [problem](https://github.com/golang/go/issues/12432)

## Quick Start / MacOS Only

- Install [minikube](https://github.com/kubernetes/minikube) (If you don't have a kubernetes cluster running)
- Install [kubectl](http://kubernetes.io/docs/getting-started-guides/minikube/#download-kubectl)
- Install the koli command line

  ```bash
  curl -o /usr/local/bin/koli https://dl.dropboxusercontent.com/s/nvzqduss1ozwq8d/koli-darwin-amd64?dl=0
  ```

- Download the [cluster certificate](https://dl.dropboxusercontent.com/s/q74md7wcgzaw8qf/koli-ca.pem?dl=0)

Get the a service account token from the default namespace

```bash
# Assuming this is a fresh installation, copy the output of the command
kubectl get secrets -o yaml |grep token: |awk {'print $2'} | base64 -D
```

Create the koli namespace system and start the package installation

```bash
kubectl create namespace koli-system
kubectl create -f https://gist.githubusercontent.com/sandromello/12ebb0763ecc1028900592b8a01f313c/raw/05be69874e48aaf793d7cc1b89a450f76b631f1c/bundle.yml
```

> Wait for the minio pod starts, then execute the next command!

```bash
kubectl create -f https://gist.githubusercontent.com/sandromello/119edb64ea7ffb053ac49e83b07ae740/raw/c0c6240ec7d94e055f865b72548abceb7dfad0a0/minio-expose.yml
```

> If you get error of existent `clusterIP`, list all svc: `$ kubectl get svc --all-namespace`
> and delete the conflicting service: `$ kubectl delete svc <service-name> --namespace <namespace>`

Wait for all pods to get into the `Running` status.
Then configure your kubeconfig:

```bash
kubectl config set-cluster orion --server=https://<MINIKUBE-HOST>:6443 --certificate-authority=/path/to/cluster/cert/ca.pem
kubectl config set-credentials koli --token=<OUTPUT-TOKEN-FROM-FIRST-COMMAND>
kubectl config set-context koli --cluster orion --user koli
kubectl config use-context koli
```

After that you can start interacting with the `koli` command line

```bash
koli create namespace default
koli create deploy my-app
git push koli master
(...)
```