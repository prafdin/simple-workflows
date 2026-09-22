# Simple Workflows

Simple Workflows is a lightweight workflow engine for running tasks as containers in Kubernetes.

# About

This is a study project and I don't plan to use it in real world environments. I'm doing it just for fun.

# Features 

- Support YAML manifests for defining workflows
- Provide a REST API for setting up, starting, and checking workflows
- Use existing Docker images with your own task scripts

# Usage

Let's define a workflow
```yaml
name: My example workflow
image: docker.io/prafdin/simple-workflow-example:v0.0.1
```

Then, pass pass this workflow to the engine
```bash
curl -X POST \
  -F "file=@workflow.yaml" \
  http://localhost:8080/workflows
```

To list all existing workflows, call GET /workflows endpoint
```bash
curl -X GET \
  http://localhost:8080/workflows
```

Let's invoke the workflow:
```bash
curl -X GET \
  --data-urlencode "name=my example workflow" \
  http://localhost:8080/workflows/run
```
Workflows names are case-insensitive for this endpoint.

To check workflow status:
```bash
curl -X GET \
  --data-urlencode "name=my example workflow" \
  http://localhost:8080/workflows/status
```

To retrive the workflow output
```bash
curl -X GET \
  --data-urlencode "name=my example workflow" \
  http://localhost:8080/workflows/output
```

# Installation 

To install the application, first clone this repository and copy the sample overlay directory:
```
cp -r deploy/overlays/sample deploy/overlays/simple-workflows
```
Then, edit files in deploy/overlays/simple-workflows:
- Specify the correct MongoDB connection parameters in configmap-patch.yaml
- Specify the MongoDB credentials in secret-patch.yaml
- Specify the gateway reference for publishing the HTTP route in httproute-patch.yaml. If the Gateway lives in a different namespace than the HTTPRoute, set `namespace` on the `parentRefs` entry — the Gateway's own listener must also allow it via `allowedRoutes.namespaces` (e.g. `from: All`), since that setting is controlled by whoever owns the Gateway, not by this HTTPRoute
- Set the image tag to deploy under `images` in kustomization.yaml (`newTag`), matching a version published by the [Docker](.github/workflows/docker-publish.yml) workflow to `ghcr.io/prafdin/simple-workflows`

Finally, apply the manifest to your cluster:
```bash
kubectl apply -k deploy/overlays/simple-workflows
```

## Connecting to a mutual-TLS MongoDB (e.g. Percona Server for MongoDB)

If your MongoDB deployment requires clients to present their own certificate (mutual TLS — the default for the [Percona Server for MongoDB operator](https://docs.percona.com/percona-operator-for-mongodb/)), a CA file alone isn't enough: connections fail server-side with `no SSL certificate provided by peer; connection rejected`.

Start from `deploy/overlays/sample-with-mtls` instead of `deploy/overlays/sample` — it wires an initContainer that builds a client certificate PEM from a Secret (the app's distroless image has no shell to do this itself) and points `MONGO_URI` at it via `tlsCAFile`/`tlsCertificateKeyFile`. Before applying, create the `simple-workflows-mongo-ca` and `simple-workflows-mongo-client-cert` Secrets in your namespace — Secret volumes can't cross namespaces, so copy these from wherever your MongoDB operator generates them (e.g. PSMDB's `<cluster-name>-ssl` Secret holds `ca.crt`, `tls.crt`, `tls.key`).
