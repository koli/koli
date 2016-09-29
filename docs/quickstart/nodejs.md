# NodeJS

## Prepare the app
In this step, you will prepare a simple application that can be deployed.
To clone the sample application so that you have a local version of the code that you can then deploy to Heroku, execute the following commands in your local command shell or terminal:

```bash
$ git clone https://github.com/heroku/node-js-getting-started.git
$ cd node-js-getting-started
```

You now have a functioning git repository that contains a simple application as well as a package.json file, which is used by Node’s dependency manager.
Log in to report a problem

## Deploy the app
In this step you will deploy the app to Koli.
Create an app on Koli, which prepares Koli to receive your source code.

```bash
$ koli create deploy my-nodejs-app
deployment "my-nodejs-app" created
```

Check if deploy was created:

```
koli get deploy
NAME            DESIRED   CURRENT   UP-TO-DATE   AVAILABLE   PAUSED    AGE
my-nodejs-app   1         0         0            0           true      1m
```

Now you can start the build and deploy of your application with a single command:

```bash
$  git push koli master
Counting objects: 459, done.
Delta compression using up to 8 threads.
Compressing objects: 100% (356/356), done.
Writing objects: 100% (459/459), 225.78 KiB | 0 bytes/s, done.
Total 459 (delta 75), reused 450 (delta 69)
remote: Resolving deltas: 100% (75/75), done.
remote: 2016/09/29 23:28:09 Running GIT Hook
remote: 2016/09/29 23:28:09 read [0000000000000000000000000000000000000000,6983eab2c75d65e35d253fe1c758db7aa8828d7f,refs/heads/master]
remote: 2016/09/29 23:28:09 GITHOME: /opt/21/21-cainelli AppName: node-js-getting-started Repo: node-js-getting-started
remote: 2016/09/29 23:28:09 running [git archive --format=tgz --output=node-js-getting-started.tgz 6983eab2] in directory /opt/21/21-cainelli/node-js-getting-started
remote: 2016/09/29 23:28:09 running [tar -xzf node-js-getting-started.tgz -C /opt/21/21-cainelli/node-js-getting-started/build/tmp574518245/] in directory /opt/21/21-cainelli/node-js-getting-started
remote: 2016/09/29 23:28:09 Uploading tar to 21/21-cainelli/node-js-getting-started/node-js-getting-started.tgz
remote: 2016/09/29 23:28:09 Starting build... but first, coffee!
remote: 2016/09/29 23:28:11 2016/09/29 23:28:10 Successfully copied 21/21-cainelli/node-js-getting-started/node-js-getting-started.tgz to /tmp/slug.tgz
-----> Node.js app detected

-----> Creating runtime environment

       NPM_CONFIG_LOGLEVEL=error
       NPM_CONFIG_PRODUCTION=true
       NODE_ENV=production
       NODE_MODULES_CACHE=true

-----> Installing binaries
       engines.node (package.json):  5.9.1
       engines.npm (package.json):   unspecified (use default)

       Downloading and installing node 5.9.1...
       Using default npm version: 3.7.3

-----> Restoring cache
       Skipping cache restore (new runtime signature)

-----> Building dependencies
       Installing node modules (package.json)
       node-js-getting-started@0.2.5 /tmp/build
       +-- ejs@2.4.1
       `-- express@4.13.3
       +-- accepts@1.2.13
       | +-- mime-types@2.1.12
       | | `-- mime-db@1.24.0
       | `-- negotiator@0.5.3
       +-- array-flatten@1.1.1
       +-- content-disposition@0.5.0
       +-- content-type@1.0.2
       +-- cookie@0.1.3
       +-- cookie-signature@1.0.6
       +-- debug@2.2.0
       | `-- ms@0.7.1
       +-- depd@1.0.1
       +-- escape-html@1.0.2
       +-- etag@1.7.0
       +-- finalhandler@0.4.0
       | `-- unpipe@1.0.0
       +-- fresh@0.3.0
       +-- merge-descriptors@1.0.0
       +-- methods@1.1.2
       +-- on-finished@2.3.0
       | `-- ee-first@1.1.1
       +-- parseurl@1.3.1
       +-- path-to-regexp@0.1.7
       +-- proxy-addr@1.0.10
       | +-- forwarded@0.1.0
       | `-- ipaddr.js@1.0.5
       +-- qs@4.0.0
       +-- range-parser@1.0.3
       +-- send@0.13.0
       | +-- destroy@1.0.3
       | +-- http-errors@1.3.1
       | | `-- inherits@2.0.3
       | +-- mime@1.3.4
       | `-- statuses@1.2.1
       +-- serve-static@1.10.3
       | +-- escape-html@1.0.3
       | `-- send@0.13.2
       |   +-- depd@1.1.0
       |   `-- destroy@1.0.4
       +-- type-is@1.6.13
       | `-- media-typer@0.3.0
       +-- utils-merge@1.0.0
       `-- vary@1.0.1


-----> Caching build
       Clearing previous node cache
       Saving 2 cacheDirectories (default):
       - node_modules
       - bower_components (nothing to cache)

-----> Build succeeded!
       +-- ejs@2.4.1
       `-- express@4.13.3

-----> Discovering process types
       Procfile declares types -> web
       Default process types for Node.js -> web
-----> Compiled slug size is 13M
remote: 2016/09/29 23:28:39 2016/09/29 23:28:39 Successfully copied 21/21-cainelli/node-js-getting-started/slugs/slug-v1.tgz to /tmp/slug.tgz
remote: 2016/09/29 23:28:39 2016/09/29 23:28:39 Successfully copied 21/21-cainelli/node-js-getting-started/slugs/Procfile-v1 to /tmp/build/Procfile
remote: 2016/09/29 23:28:39 2016/09/29 23:28:39 Successfully copied 21/21-cainelli/node-js-getting-started/slugs/latest/slug.tgz to /tmp/slug.tgz
remote: 2016/09/29 23:28:39 2016/09/29 23:28:39 Successfully copied 21/21-cainelli/node-js-getting-started/slugs/latest/Procfile to /tmp/build/Procfile
-----> Deploy updated successfully
remote: 2016/09/29 23:28:43 Build completed
To http://crafter-orion.kolibox.io:7080/21-cainelli/node-js-getting-started
 * [new branch]      master -> master

```

Once sucessfully deployed check if the pod is running:

```bash
$ koli get pods
NAME                            READY     STATUS    RESTARTS   AGE
my-nodejs-app-999502812-cbc0u   1/1       Running   0          1m
``` 

With the `pod name` you can port-forward and see if the app is running properly

```bash
$ koli port-forward my-nodejs-app-999502812-cbc0u 5000:5000
Forwarding from 127.0.0.1:5000 -> 5000
Forwarding from [::1]:5000 -> 5000
```

Open your browser at http://127.0.0.1:5000

Thanks heroku