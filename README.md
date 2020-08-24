# MongoDB Kubernetes Sidecar
Container image: gcr.io/scware-project/mdb-sidecar

This sidecar runs in each mongo instance's pod and is responsible for the following:
- initializing and resizing the mongo replica set
- setting up the initial admin user and password
- creating a service to expose the replica set primary; service will be named ${RS_SVC}-primary

## How to use it
1. (Optional) Create a Namespace for this MongoDB Replica Set resources.
2. Create a ServiceAccount, a ClusterRole (allowing get/list/patch on Pods and create/get/list on Services), and a ClusterRoleBinding binding them together.
3. Create a StatefulSet with a pod template spec containing a mongo container and this sidecar container.
    1. Set the Pod template spec `serviceAccountName` to the ServiceAccount created in 2.
    2. Configure mongo containers' replica set configuration (via command flags, mounted ConfigMap volumes, etc).
    3. Configure the sidecar containers' environment variables. See Environment Variables below.
4. Create a headless Service for the StatefulSet.

### Environment Variables

| Environment Variable | Required | Default | Description |
| --- | --- | --- | --- |
| NS | NO |  | The Namespace to look up pods in. Not setting it will search for pods in all namespaces. |
| RS_SVC | YES |  | The headless service of the mongo StatefulSet Pods that makes up the mongo replica set. It is used for setting up the DNS configuration for the mongo pods, instead of the default pod IPs. Works only with the StatefulSets' stable network ID. |
| MDB_USER | YES | | The admin user to be created/used by sidecars. |
| MDB_PASS | YES | | The admin pass to be created/used by sidecars. |
| MDB_PORT | NO | 27017 | Configures the mongo port, allows the usage of non-standard ports. |

In its default configuration the sidecar uses the pods' IPs for the MongoDB replica names. Here is a trimmed example:
```
[ { _id: 1,
   name: '10.48.0.70:27017',
   stateStr: 'PRIMARY',
   ...},
 { _id: 2,
   name: '10.48.0.72:27017',
   stateStr: 'SECONDARY',
   ...},
 { _id: 3,
   name: '10.48.0.73:27017',
   stateStr: 'SECONDARY',
   ...} ]
```

If you want to use the StatefulSets' stable network ID, you have to make sure that you have the `KUBERNETES_MONGO_SERVICE_NAME`
environmental variable set. Then the MongoDB replica set node names could look like this:
```
[ { _id: 1,
   name: 'mongo-prod-0.mongodb.db-namespace.svc.cluster.local:27017',
   stateStr: 'PRIMARY',
   ...},
 { _id: 2,
   name: 'mongo-prod-1.mongodb.db-namespace.svc.cluster.local:27017',
   stateStr: 'SECONDARY',
   ...},
 { _id: 3,
   name: 'mongo-prod-2.mongodb.db-namespace.svc.cluster.local:27017',
   stateStr: 'SECONDARY',
   ...} ]
```
StatefulSet name: `mongo-prod`.
Headless service name: `mongodb`.
Namespace: `db-namespace`.

Read more about the stable network IDs
<a href="https://kubernetes.io/docs/concepts/abstractions/controllers/statefulsets/#stable-network-id">here</a>.

An example for a stable network pod ID looks like this:
`$(statefulset name)-$(ordinal).$(service name).$(namespace).svc.cluster.local`.
The `statefulset name` + the `ordinal` form the pod name, the `service name` is passed via `KUBERNETES_MONGO_SERVICE_NAME`,
the namespace is extracted from the pod metadata and the rest is static.

A thing to consider when running a cluster with the mongo-k8s-sidecar is that it will prefer the stateful set stable
network ID to the pod IP. It is however compatible with replica sets, configured with the pod IP as identifier - the sidecar
should not add an additional entry for it, nor alter the existing entries. The mongo-k8s-sidecar should only use the stable
network ID for new entries in the cluster.

Finally if you have a preconfigured replica set you have to make sure that:
- the names of the mongo nodes are their IPs
- the names of the mongo nodes are their stable network IDs (for more info see the link above)

Example of compatible mongo replica names:
```
10.48.0.72:27017 # Uses the default pod IP name
mongo-prod-0.mongodb.db-namespace.svc.cluster.local:27017 # Uses the stable network ID
```

Example of not compatible mongo replica names:
```
mongodb-service-0 # Uses some custom k8s service name. Risks being a duplicate entry for the same mongo.
```

If you run the sidecar alongside such a cluster, it may lead to a broken replica set, so make sure to test it well before
going to production with it (which applies for all software).

#### MongoDB Command
The following is an example of how you would update the mongo command enabling ssl and using a certificate obtained from a secret and mounted at /data/ssl/mongodb.pem

Command
```
        - name: my-mongo
          image: mongo
          command:
            - mongod
            - "--replSet"
            - heroku
            - "--bind_ip"
            - 0.0.0.0
            - "--smallfiles"
            - "--noprealloc"
            - "--sslMode"
            - "requireSSL"
            - "--sslPEMKeyFile"
            - "/data/ssl/mongodb.pem"
            - "--sslAllowConnectionsWithoutCertificates"
            - "--sslAllowInvalidCertificates"
            - "--sslAllowInvalidHostnames"
```

Volume & Volume Mount
```
          volumeMounts:
            - name: mongo-persistent-storage
              mountPath: /data/db
            - name: mongo-ssl
              mountPath: /data/ssl
        - name: mongo-sidecar
          image: cvallance/mongo-k8s-sidecar:latest
          env:
            - name: MONGO_SIDECAR_POD_LABELS
              value: "role=mongo,environment=prod"
            - name: MONGO_SSL_ENABLED
              value: 'true'
      volumes:
        - name: mongo-ssl
          secret:
            secretName: mongo
```

#### Creating Secret for SSL
Use the Makefile:

| Environment Variable | Required | Default | Description |
| --- | --- | --- | --- |
| MONGO_SECRET_NAME | NO | mongo-ssl | This is the name that the secret containing the SSL certificates will be created with. |
| KUBECTL_NAMESPACE | NO | default | This is the namespace in which the secret containing the SSL certificates will be created. |

```
export MONGO_SECRET_NAME=mongo-ssl
export KUBECTL_NAMESPACE=default
cd examples && make generate-certificate
```

or

Generate them on your own and push the secrets `kube create secret generic mongo --from-file=./keys`
where `keys` is a directory containing your SSL pem file named `mongodb.pem`

## Debugging

TODO: Instructions for cloning, mounting and watching

## Still to do

- Add tests!
- Add to circleCi
- Alter k8s call so that we don't have to filter in memory
