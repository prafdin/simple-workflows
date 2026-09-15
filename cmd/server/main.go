package main

import (
	"context"
	"log"
	"net/http"
	"os"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/prafdin/simple-workflows/internal/api"
	"github.com/prafdin/simple-workflows/internal/k8sexec"
	"github.com/prafdin/simple-workflows/internal/mongostore"
)

func main() {
	ctx := context.Background()

	mongoURI := getenv("MONGO_URI", "mongodb://localhost:27017")
	mongoDatabase := getenv("MONGO_DATABASE", "simple_workflows")
	namespace := getenv("K8S_NAMESPACE", "default")
	listenAddr := getenv("LISTEN_ADDR", ":8080")

	mongoClient, err := mongo.Connect(ctx, options.Client().ApplyURI(mongoURI))
	if err != nil {
		log.Fatalf("could not connect to mongodb: %v", err)
	}
	collection := mongoClient.Database(mongoDatabase).Collection("workflows")
	store := mongostore.New(collection)

	k8sConfig, err := buildKubeConfig()
	if err != nil {
		log.Fatalf("could not build kubernetes config: %v", err)
	}
	clientset, err := kubernetes.NewForConfig(k8sConfig)
	if err != nil {
		log.Fatalf("could not build kubernetes clientset: %v", err)
	}
	runner := k8sexec.New(clientset, namespace)

	router := api.NewRouter(store, runner)

	log.Printf("listening on %s", listenAddr)
	if err := http.ListenAndServe(listenAddr, router); err != nil {
		log.Fatalf("server stopped: %v", err)
	}
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
