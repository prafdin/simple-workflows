package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.opentelemetry.io/contrib/instrumentation/go.mongodb.org/mongo-driver/mongo/otelmongo"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/prafdin/simple-workflows/internal/api"
	"github.com/prafdin/simple-workflows/internal/k8sexec"
	"github.com/prafdin/simple-workflows/internal/metrics"
	"github.com/prafdin/simple-workflows/internal/mongostore"
	"github.com/prafdin/simple-workflows/internal/tracing"
)

func main() {
	ctx := context.Background()

	provider, shutdown, err := tracing.Setup(ctx)
	if err != nil {
		log.Fatalf("could not set up tracing: %v", err)
	}
	defer shutdown(context.Background())

	mongoURI := getenv("MONGO_URI", "mongodb://localhost:27017")
	mongoDatabase := getenv("MONGO_DATABASE", "simple_workflows")
	mongoUsername := getenv("MONGO_USERNAME", "")
	mongoPassword := getenv("MONGO_PASSWORD", "")
	namespace := getenv("K8S_NAMESPACE", "default")
	listenAddr := getenv("LISTEN_ADDR", ":8080")
	metricsAddr := getenv("METRICS_ADDR", ":9090")

	mongoClient, err := mongo.Connect(ctx, buildClientOptions(mongoURI, mongoUsername, mongoPassword, provider))
	if err != nil {
		log.Fatalf("could not connect to mongodb: %v", err)
	}
	collection := mongoClient.Database(mongoDatabase).Collection("workflows")
	store := mongostore.New(collection)

	k8sConfig, err := buildKubeConfig()
	if err != nil {
		log.Fatalf("could not build kubernetes config: %v", err)
	}
	k8sConfig.Wrap(func(rt http.RoundTripper) http.RoundTripper {
		return otelhttp.NewTransport(rt, otelhttp.WithTracerProvider(provider), otelhttp.WithPropagators(propagation.TraceContext{}))
	})
	clientset, err := kubernetes.NewForConfig(k8sConfig)
	if err != nil {
		log.Fatalf("could not build kubernetes clientset: %v", err)
	}
	runner := k8sexec.New(clientset, namespace)

	telemetry := metrics.New(store)
	watcher, err := k8sexec.NewWatcher(clientset, namespace, telemetry, provider)
	if err != nil {
		log.Fatalf("could not create job watcher: %v", err)
	}
	if err := watcher.Run(ctx, 30*time.Second); err != nil {
		log.Fatalf("could not start job watcher: %v", err)
	}

	metricsMux := http.NewServeMux()
	metricsMux.Handle("GET /metrics", telemetry.Handler())
	go func() {
		log.Printf("serving metrics on %s", metricsAddr)
		if err := http.ListenAndServe(metricsAddr, metricsMux); err != nil {
			log.Fatalf("metrics server stopped: %v", err)
		}
	}()

	router := api.NewRouter(store, runner, telemetry, provider)

	log.Printf("listening on %s", listenAddr)
	if err := http.ListenAndServe(listenAddr, router); err != nil {
		log.Fatalf("server stopped: %v", err)
	}
}

func buildClientOptions(uri, username, password string, provider trace.TracerProvider) *options.ClientOptions {
	opts := options.Client().ApplyURI(uri).SetMonitor(otelmongo.NewMonitor(otelmongo.WithTracerProvider(provider)))
	if username != "" {
		opts = opts.SetAuth(options.Credential{Username: username, Password: password})
	}
	return opts
}

func buildKubeConfig() (*rest.Config, error) {
	if config, err := rest.InClusterConfig(); err == nil {
		return config, nil
	}
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	return clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, &clientcmd.ConfigOverrides{}).ClientConfig()
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
