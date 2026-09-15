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
- Specify the gateway reference for publishing the HTTP route in httproute-patch.yaml

Finally, apply the manifest to your cluster:
```bash
kubectl apply -k deploy/overlays/simple-workflows
```
