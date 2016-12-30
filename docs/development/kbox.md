# Dev Environment WIP


Make sure you have `git-lfs` installed.

```bash
$ brew install git-lfs
```

You also need `virtualenv` installed.

```bash
$ sudo pip install virtualenv
```

Clone [kbox](https://github.com/kolihub/kbox) and [controller](https://github.com/kolihub/kbox) repositories:

```bash
$ git clone https://github.com/kolihub/kbox.git
$ git clone https://github.com/kolihub/controller.git
```

Create `controller/environment.sh` file:

> Make the proper adustments

```bash
#export SECRET_PATH=/Users/san/Downloads/saccount
export ENVIRONMENT=development
export PYTHONPATH=/Users/cainelli/Documents/workspace/kolihub/controller/rootfs
export KUBERNETES_SERVICE_HOST=192.168.64.2
export KUBERNETES_SERVICE_PORT=5443
```

Install development environment:

```bash
$ cd controller/
$ make setup-venv
$ make up
```

Install `corectl`:

  - **macOS 10.10.3** Yosemite or later 
  - Mac 2010 or later for this to work.
  - **Note: [Corectl App](https://github.com/TheNewNormal/corectl.app) must be installed, which will serve as `corectld` server daemon control.**
  - [iTerm2](https://www.iterm2.com/) is required, if not found the app will install it by itself.
  - Download [Corectl App](https://github.com/TheNewNormal/corectl.app) `latest dmg` from the [Releases Page](https://github.com/TheNewNormal/corectl.app/releases) and install it to `/Applications` folder, it allows to start/stop/update [corectl](https://github.com/TheNewNormal/corectl) tools needed to run CoreOS VMs on macOS

Now on the kbox project copy the `kbox` bin to your `/usr/local/bin/` path.

```bash
$ cd ../
$ cp kbox/bin/kbox /usr/local/bin/
$ chmod +x /usr/local/bin/kbox
```

Create a symlink to `/opt/` dir:

```bash
$ sudo ln -s ${PWD}/kbox /opt/kbox
```

Now you can run the installer and configure the vm resources:

```bash
$ kbox setup

Setting up Kubernetes Solo Cluster on macOS

Reading ssh key from /Users/cainelli/.ssh/id_rsa.pub

/Users/cainelli/.ssh/id_rsa.pub found, updating configuration files ...

Set CoreOS Release Channel:
 1)  Alpha (may not always function properly)
 2)  Beta
 3)  Stable (recommended)

Select an option: 3


Please type VM's RAM size in GBs followed by [ENTER]:
[default is 2]: 4

Changing VM's RAM to 4GB...


Please type Data disk size in GBs followed by [ENTER]:
[default is 30]:

Creating 30GB sparse disk (QCow2)...
-
Created 30GB Data disk


Starting VM ...
```

Make sure you have a similar output at the end.
```
kubectl cluster-info:
Kubernetes master is running at http://192.168.64.2:8080
KubeDNS is running at http://192.168.64.2:8080/api/v1/proxy/namespaces/kube-system/services/kube-dns
kubernetes-dashboard is running at http://192.168.64.2:8080/api/v1/proxy/namespaces/kube-system/services/kubernetes-dashboard

To further debug and diagnose cluster problems, use 'kubectl cluster-info dump'.

Cluster version:
Client version: v1.4.3
Server version: v1.4.3

kubectl get nodes:
NAME        STATUS    AGE
k8solo-01   Ready     13s

Also you can install Deis Workflow PaaS (https://deis.com) with 'install_deis' command ...
```

Otherwise you can try destroy it and run the setup again

```bash
# your machine
$ kbox destroy
```



```bash
# your machine kbox shell
$ kbox shell
$ kubectl config use-context kube-solo
$ SYSTEMTOKEN=$(kubectl get secrets -n kube-system -o yaml |grep -i token: |awk {'print $2'} |base64 -D) python3
import shelve, os
db = shelve.open('config.db')
db['rbacsuperuser'] = os.environ['SYSTEMTOKEN']
db.close()
```


```bash
# your machine native shell
$ cd controller/
$ make up
source venv/bin/activate \
		  && source environment.sh \
		  && python -m api.app
 * Running on http://0.0.0.0:8000/ (Press CTRL+C to quit)
 * Restarting with stat
 * Debugger is active!
 * Debugger pin code: 235-835-594
```

Try to `curl` from coreos vm to the 8000 port:

```bash
#coreos vm
core@k8solo-01 ~ $ curl http://10.20.30.128:8000
<!DOCTYPE HTML PUBLIC "-//W3C//DTD HTML 3.2 Final//EN">
<title>404 Not Found</title>
<h1>Not Found</h1>
<p>The requested URL was not found on the server.  If you entered the URL manually please check your spelling and try again.</p>
```

udpdate to your IP address in the following file `/etc/systemd/system/kube-apiserver.service`
```bash
# coreos vm
ExecStartPre=/opt/sbin/webhook-cfg-writer.sh /data/kubernetes/authentication-webhook.cfg http://192.168.0.101:8000/webhook-auth
```

And restart the required services

```bash
# coreos vm
$ sudo systemctl daemon-reload
$ sudo systemctl restart kube-apiserver
```

Now finally configure `koli` CLI
```bash
# your machine native shell
$ kubectl config set-cluster orion --server=https://$(kbox ip):5443 --insecure-skip-tls-verify=true 
$ kubectl config set-credentials koli --token=za
$ kubectl config set-context koli --cluster orion --user koli
$ kubectl config use-context koli
```

Download `koli` CLI
```bash
$ curl -O https://github.com/kolihub/koli/releases/download/v0.1.1-alpha/koli-darwin-v0.1.1-alpha /usr/local/bin/koli
$ chmod +x /usr/local/bin/koli
```

```bash
# your machine native shell
$ curl http://127.0.0.1:8000/users -H 'Content-Type: application/json' -XPOST \
    -d '{"username": "cainelli", "email": "cainelli@kolihub.io", "password": "ko+li", "type": "regular"}'
```

Build `api-router` image

```
$ git clone ...
$ VERSION=v0.2.0-alpha make build
```

```bash
# your machine shell
$ kubectl config use-context kube-solo
$ kubectl create ns koli-system
$ cat <<EOF | kubectl create -f -
apiVersion: extensions/v1beta1
kind: Deployment
metadata:
  name: api-router
  namespace: koli-system
spec:
  replicas: 1
  template:
    metadata:
      labels:
        app: api-router
    spec:
      containers:
      - name: api-router
        image: 'quay.io/koli/api-router:v0.2.0-alpha'
        ports:
        - containerPort: 80
        - containerPort: 443
---
kind: Service
apiVersion: v1
metadata:
  name: api-router
  labels:
    app: api-router
  namespace: koli-system
spec:
  ports:
  - protocol: TCP
    name: http
    port: 7080
    targetPort: 80
  - protocol: TCP
    name: https
    port: 5443
    targetPort: 443
  externalIPs:
  - 192.168.64.2
  deprecatedPublicIPs:
  - 192.168.64.2
  selector:
    app: api-router
EOF
```

```bash
$ koli login
```

```bash
$ curl -XPOST  http://127.0.0.1:8000/addons/redis -H 'Authorization: Bearer []' \
    -H 'Content-Type: application/json' --data-binary @redis.json
```

```
$ kubectl -n 4779-default run busybox --rm -ti --image=busybox /bin/sh
```
