# ACME webhook for Websupport DNS

The ACME issuer type supports an optional 'webhook' solver, which can be used to implement custom [DNS01 challenge](https://letsencrypt.org/docs/challenge-types/#dns-01-challenge) solving logic. This is a implementation of such solver that supports DNS01 challenge with [websupport](https://www.websupport.sk/) DNS API.

More information about webhooks can be found: https://cert-manager.io/docs/configuration/acme/dns01/webhook/

> [!WARNING]
> Please note, that this is not official websupport cert-manager webhook, but rather community maintained one.

> [!NOTE]
> Currently, latest release supports these Kubernetes versions: `v1.22` → `v1.28`.

## Usage

You have to have a working installation of [cert-manager](https://cert-manager.io/) in your kubernetes cluster, before installing this webhook. You can follow these official [instructions](https://cert-manager.io/docs/installation/) to install it.

### Installation

This repository contains helm chart for deploying the webhook to kubernetes cluster. It is located in `deploy/` directory. You can build it by running by [Helm](https://helm.sh/), for example

```sh
helm template \
    --set image.repository=websupport-webhook \
    --set image.tag=latest \
    --namespace=cert-manager \
    cert-manager-webhook-websupport \
    deploy/cert-manager-webhook-websupport > manifest.yaml
```

or with `make` by running 

```sh
make rendered-manifest
```

The latter one will generated the same manifest as you would get with the `helm template` command and will output it to  `_out/` folder. Manifest can then be applied by running

```sh
kubectl apply -f _out/rendered_manifest.yaml
```

Or lastly you can install directly through Helm and [GitHub release](https://github.com/bratislava/cert-manager-webhook-websupport/releases)

```sh
helm repo add webhook-websupport https://github.com/bratislava/cert-manager-webhook-websupport/releases/download/<release-name>/
helm install cert-manager-webhook-websupport webhook-websupport/cert-manager-webhook-websupport  
```

This should install cert-manager into the cluster, to be able to issue certificates under it you need to create an `ClusterIssuer` under the cert-manager.

First you need to obtain websupport API credentials: https://www.websupport.sk/podpora/kb/api-keys/. Store them as a secret in the cluster

```sh
kubectl --namespace cert-manager create secret generic websupport-secret \
  --from-literal="ApiKey=<obtain-key>" \
  --from-literal="ApiSecret=<obtain-secret>"
```

and then create `ClusterIssuer` resource, with reference to your secret

```yaml
apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: letsencrypt-dns01
spec:
  acme:
    # You must replace this email address with your own.
    # Let's Encrypt will use this to contact you about expiring
    # certificates, and issues related to your account.
    email: contact@example.com
    server: https://acme-v02.api.letsencrypt.org/directory
    privateKeySecretRef:
      name: letsencrypt-issuer-account-key
    solvers:
    - dns01:
        webhook:
          config:
            apiKeySecretRef:
              name: websupport-secret
          groupName: cert-manager.io
          solverName: websupport-solver
```

Depending on the version of your kubernetes and/or your cert-manager you might need to grant additional permissions.

### Issue an certificate

Just create an `Certificate` resource, with issuer name, that you have given to your solver in previous step (in our example it is `letsencrypt-dns01`).

```sh
cat <<EOF | kubectl create --edit -f -
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: example-tls
spec:
  secretName: example-com-tls
  dnsNames:
  - example.com
  - "*.example.com"
  issuerRef:
    name: letsencrypt-dns01
    kind: ClusterIssuer
EOF
```

Or you can do it by annotating you ingress

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  annotations:
    # add an annotation indicating the issuer to use.
    cert-manager.io/cluster-issuer: letsencrypt-dns01
```

## Running the test suite

**:orange_book: Currently, we do not have automated testing, and running the test will just run the [example one](https://github.com/cert-manager/webhook-example). But we are always looking for contribution so please feel free to contribute :smile:.**

All DNS providers **must** run the DNS01 provider conformance testing suite,
else they will have undetermined behaviour when used with cert-manager.

**It is essential that you configure and run the test suite when creating a
DNS01 webhook.**

An example Go test file has been provided in [main_test.go](https://github.com/cert-manager/webhook-example/blob/master/main_test.go).

You can run the test suite with:

```bash
$ TEST_ZONE_NAME=example.com. make test
```

The example file has a number of areas you must fill in and replace with your
own options in order for tests to pass.
