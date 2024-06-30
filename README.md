# Sample for trust-manager

## Setup cluster

### 1. Create cluster by kind

```sh
kind create cluster --name tls-example
```

Ensure that the all componens are ready.

```
❯ kubectl get po -A            
NAMESPACE            NAME                                                READY   STATUS    RESTARTS   AGE
kube-system          coredns-7db6d8ff4d-98vc4                            1/1     Running   0          2m41s
kube-system          coredns-7db6d8ff4d-hgvkc                            1/1     Running   0          2m41s
kube-system          etcd-tls-example-control-plane                      1/1     Running   0          2m58s
kube-system          kindnet-wx8jc                                       1/1     Running   0          2m41s
kube-system          kube-apiserver-tls-example-control-plane            1/1     Running   0          2m57s
kube-system          kube-controller-manager-tls-example-control-plane   1/1     Running   0          2m57s
kube-system          kube-proxy-l7h2r                                    1/1     Running   0          2m41s
kube-system          kube-scheduler-tls-example-control-plane            1/1     Running   0          2m57s
local-path-storage   local-path-provisioner-988d74bc-lmkmx               1/1     Running   0          2m41s
```

### 2. Install cert-manager

```sh
helm repo add jetstack https://charts.jetstack.io --force-update
helm repo update
```

Install cert-manager by helm.

```sh
helm install cert-manager jetstack/cert-manager \
  --namespace cert-manager \
  --create-namespace \
  --set crds.enabled=true
```

Ensure that the all components of cert-manager are ready.

```
❯ kubectl get po -n cert-manager
NAME                                       READY   STATUS    RESTARTS   AGE
cert-manager-84489bc478-plr2w              1/1     Running   0          33s
cert-manager-cainjector-7477d56b47-svjf4   1/1     Running   0          33s
cert-manager-webhook-6d5cb854fc-rhptz      1/1     Running   0          33s
```

### 3. Install trust-manager

Install trust-manager by helm. It is contained in the charts from jetstack.io.

```sh
helm upgrade \
	--install \
	--namespace cert-manager \
	--wait \
	trust-manager jetstack/trust-manager
```

Ensure that the all components of trust-manager are ready.

```
❯ kubectl get po -n cert-manager -l app.kubernetes.io/name=trust-manager                     
NAME                             READY   STATUS    RESTARTS   AGE
trust-manager-7dc7cb97b4-kqkln   1/1     Running   0          57s
```

### 4. Create CA and its issuer

First, create your CA to use in your cluster.

```sh
kubectl apply -f manifests/01-internal-ca.yaml
```

Then, create issuer to issue certs using your CA.

```sh
kubectl apply -f manifests/02-internal-issuer.yaml
```

Ensure that your issuer is ready.

```
❯ kubectl get clusterissuer internal                        
NAME       READY   AGE
internal   True    3s
```

### 5. Create Bundle to distribute CA certs.

```sh
kubectl apply -f manifests/03-bundle.yaml
```

Ensure that the `internal-ca-bundle` config map is created.

```
❯ kubectl get cm internal-ca-bundle                        
NAME                 DATA   AGE
internal-ca-bundle   1      16s
```

## Test communication using TLS

Build sample gRPC server implementation and load it to kind cluster.

```sh
docker build . -f Dockerfile -t tls-example-server
kind load docker-image tls-example-server:latest --name tls-example
```

Build bastion image and load it to kind cluster.

```sh
docker build . -f Dockerfile.bastion -t tls-example-bastion
kind load docker-image tls-example-bastion:latest --name tls-example
```

### 0. Deploy bastion

```sh
kubectl apply -f manifests/04-bastion.yaml
```

Ensure that the bastion pod is ready.

```
❯ kubectl get po bastion                                                
NAME      READY   STATUS    RESTARTS   AGE
bastion   1/1     Running   0          34s
```

### 1. Test with plaintext

Deploy gRPC server that is not using TLS.

```sh
kubectl apply -f manifests/05-plain-server.yaml
```

Ensure that the server is ready.

```
❯ kubectl get po -n plaintext                                           
NAME                                READY   STATUS    RESTARTS   AGE
plaintext-server-59d759f797-77ljd   1/1     Running   0          10s
```

Login to your bastion. And run grpcurl as follows.

```sh
kubectl exec bastion -- grpcurl -plaintext api.plaintext.svc:80 grpc.health.v1.Health/Check
```

It will succeed.

```
❯ kubectl exec bastion -- grpcurl -plaintext api.plaintext.svc:80 grpc.health.v1.Health/Check
{
  "status": "SERVING"
}
```

However, it does not succeed using TLS.

```
❯ kubectl exec bastion -- grpcurl api.plaintext.svc:80 grpc.health.v1.Health/Check
E0630 17:58:37.425598   27263 websocket.go:296] Unknown stream id 1, discarding message
Failed to dial target host "api.plaintext.svc:80": tls: first record does not look like a TLS handshake
command terminated with exit code 1
```

### 2. Test with TLS (but skip verifying)

Firts, create certificate using `internal` issuer.

```sh
kubectl apply -f manifests/06-cert.yaml
```

Ensure that the certificate has been issued.

```
❯ kubectl get cert server -n secure      
NAME     READY   SECRET       AGE
server   True    server-tls   4s
```

Next, deploy the API server using TLS.

```sh
kubectl apply -f manifests/07-tls-server.yaml
```

Ensure that the server is ready.

```
❯ kubectl get po -n secure                     
NAME                          READY   STATUS    RESTARTS   AGE
tls-server-5465dd85cf-2q6v5   1/1     Running   0          9s
```

Running grpcurl for the server will fail due to `certificate signed by unknown authority`.

```
❯ kubectl exec bastion -- grpcurl api.secure.svc:443 grpc.health.v1.Health/Check
Failed to dial target host "api.secure.svc:443": tls: failed to verify certificate: x509: certificate signed by unknown authority
command terminated with exit code 1
```

We need to add `-insecure` to ignore the error. However, this is insecure!

```
❯ kubectl exec bastion -- grpcurl -insecure api.secure.svc:443 grpc.health.v1.Health/Check   
{
  "status": "SERVING"
}
```

### 3. Test with TLS and our CA

bastion mounts CA certs provided by trust-manager.

```
❯ kubectl get po bastion -o json | jq -r '.spec.containers[0].volumeMounts[0]'
{
  "mountPath": "/internal-ca-bundle",
  "name": "internal-ca-bundle",
  "readOnly": true
}
```

So we can use this to verify the certs from server.

```
❯ kubectl exec bastion -- grpcurl -cacert /internal-ca-bundle/trust-bundle.pem api.secure.svc:443 grpc.health.v1.Health/Check
{
  "status": "SERVING"
}
```

## Author

- pddg
