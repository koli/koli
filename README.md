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
  chmod +x /usr/local/bin/koli
  ```

Create the koli namespace system and start the package installation:

```bash
kubectl config use-context minikube
kubectl create namespace koli-system
kubectl create -f https://gist.githubusercontent.com/sandromello/12ebb0763ecc1028900592b8a01f313c/raw/05be69874e48aaf793d7cc1b89a450f76b631f1c/bundle.yml
```

Wait for the minio pod starts

```bash
$ kubectl get pods --namespace=koli-system -w
NAME                          READY     STATUS              RESTARTS   AGE
api-router-2487936394-doq5o   0/1       ContainerCreating   0          1m
controller-2698305122-x1dxw   0/1       ContainerCreating   0          1m
crafter-2230611587-nxthm      0/1       ContainerCreating   0          1m
minio-u9dz4                   1/1       Running             0          1m
```

Then expose the minio service and patch the services `api-router` and `minio` to your minikube IP:

```bash
kubectl create -f https://gist.githubusercontent.com/sandromello/119edb64ea7ffb053ac49e83b07ae740/raw/c0c6240ec7d94e055f865b72548abceb7dfad0a0/minio-expose.yml
MINIKUBE_IP=$(minikube ip)
kubectl --namespace koli-system patch svc api-router -p '{"spec":{"deprecatedPublicIPs": ["'$MINIKUBE_IP'"],"externalIPs": ["'$MINIKUBE_IP'"]}}'
kubectl --namespace koli-system patch svc minio -p '{"spec":{"deprecatedPublicIPs": ["'$MINIKUBE_IP'"],"externalIPs": ["'$MINIKUBE_IP'"]}}'
```

> If you get error of existent `clusterIP`, list all services to identify which one is using it: 
> 
> `$ kubectl get svc --all-namespace`
> 
> Delete the conflicting service:
>
> `$ kubectl delete svc <service-name> --namespace <namespace>`

Wait for all pods to get into the `Running` status.
Then configure your kubeconfig:

```bash
kubectl config set-cluster orion --server=https://$(minikube ip):6443 --insecure-skip-tls-verify=true
kubectl config use-context minikube 
kubectl config set-credentials koli --token=$(kubectl get secrets -o yaml |grep token: |awk {'print $2'} | base64 -D)
kubectl config set-context koli --cluster orion --user koli
kubectl config use-context koli
```

Once you done with kubectl you must insert crafter hostname in your `/etc/hosts`

```bash
echo "$(minikube ip) crafter-orion.kolibox.io" |sudo tee -a /etc/hosts > /dev/null 
```

After that you can start interacting with the `koli` command line

```bash
koli create namespace default
koli create deploy my-app
git push koli master
(...)
```