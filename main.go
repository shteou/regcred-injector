package main

import (
	"crypto/tls"
	"log"
	"net/http"
	"os"
	"time"

	ghandlers "github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/shteou/regcred-injector/handlers"
)

func main() {
	var tlsConf *tls.Config
	keyPair, err := tls.LoadX509KeyPair("certs/regcred-injector-crt.pem", "certs/regcred-injector-key.pem")
	if err != nil {
		log.Fatal(err)
	}

	config, err := rest.InClusterConfig()
	if err != nil {
		log.Fatal(err)
	}
	handlers.Clientset, err = kubernetes.NewForConfig(config)
	if err != nil {
		log.Fatal(err)
	}

	serverName, found := os.LookupEnv("SERVER_NAME")
	if !found {
		log.Fatal("Unable to read SERVER_NAME environment variable")
	}

	handlers.DockerUsername, found = os.LookupEnv("DOCKER_USERNAME")
	if !found {
		log.Fatal("Unable to read DOCKER_USERNAME environment variable")
	}
	handlers.DockerPassword, found = os.LookupEnv("DOCKER_PASSWORD")
	if !found {
		log.Fatal("Unable to read DOCKER_PASSWORD environment variable")
	}
	handlers.DockerRegistry, found = os.LookupEnv("DOCKER_REGISTRY")
	if !found {
		log.Fatal("Unable to read DOCKER_REGISTRY environment variable")
	}

	log.SetOutput(os.Stdout)

	tlsConf = &tls.Config{
		Certificates:             []tls.Certificate{keyPair},
		ServerName:               serverName,
		MinVersion:               tls.VersionTLS12,
		CurvePreferences:         []tls.CurveID{tls.CurveP521, tls.CurveP384, tls.CurveP256},
		PreferServerCipherSuites: true,
		CipherSuites: []uint16{
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
			tls.TLS_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_RSA_WITH_AES_256_CBC_SHA,
		},
	}

	r := mux.NewRouter()
	r.HandleFunc("/admission", handlers.PodHandler)
	r.HandleFunc("/status", handlers.StatusHandler)
	loggingHandler := ghandlers.LoggingHandler(os.Stdout, r)

	http.Handle("/", r)

	srv := &http.Server{
		Handler:           loggingHandler,
		Addr:              "0.0.0.0:8443",
		TLSConfig:         tlsConf,
		TLSNextProto:      make(map[string]func(*http.Server, *tls.Conn, http.Handler), 0),
		WriteTimeout:      30 * time.Second,
		ReadTimeout:       20 * time.Second,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	log.Fatal(srv.ListenAndServeTLS("certs/regcred-injector-crt.pem", "certs/regcred-injector-key.pem"))
}
