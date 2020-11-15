package main

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	admission "k8s.io/api/admission/v1beta1"
	apiv1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

var clientset *kubernetes.Clientset
var dockerUsername string
var dockerPassword string

func StatusHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func extractReview(r *http.Request) (admission.AdmissionReview, error) {
	var rev admission.AdmissionReview

	err := json.NewDecoder(r.Body).Decode(&rev)
	if err != nil {
		return rev, err
	}

	return rev, nil
}

type RegCredPatchSpec struct {
	Op    string              `json:"op"`
	Path  string              `json:"path"`
	Value []map[string]string `json:"value"`
}

type DockerAuth struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Auth     string `json:"auth"`
}

type DockerConfig struct {
	Auths map[string]DockerAuth `json:"auths":`
}

func createSecret(namespace string) error {
	secrets, err := clientset.CoreV1().Secrets(namespace).List(context.TODO(), v1.ListOptions{})
	if err != nil {
		return err
	}

	hasSecret := false
	log.Printf("Found %v secrets", len(secrets.Items))
	for i := 0; i < len(secrets.Items); i++ {
		if secrets.Items[i].ObjectMeta.Name == "regcred" {
			hasSecret = true
		}
	}

	if hasSecret == false {
		secret := apiv1.Secret{}
		secret.Type = "kubernetes.io/dockerconfigjson"
		secret.Name = "regcred"
		secret.Data = make(map[string][]byte)
		dockerConfig := DockerConfig{}
		dockerConfig.Auths = make(map[string]DockerAuth)
		dockerAuth := DockerAuth{}
		dockerAuth.Username = dockerUsername
		dockerAuth.Password = dockerPassword
		dockerAuth.Auth = base64.StdEncoding.EncodeToString([]byte(dockerUsername + ":" + dockerPassword))
		dockerConfig.Auths["https://index.docker.io/v1/"] = dockerAuth

		dockerConfigJSON, err := json.Marshal(dockerConfig)
		if err != nil {
			return err
		}

		secret.Data[".dockerconfigjson"] = []byte(dockerConfigJSON)
		_, err = clientset.CoreV1().Secrets(namespace).Create(context.TODO(), &secret, v1.CreateOptions{})
		if err != nil {
			return err
		}
	}
	return nil
}

func PodHandler(w http.ResponseWriter, r *http.Request) {
	req, err := extractReview(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	responseReview := admission.AdmissionReview{}

	responseReview.Kind = "AdmissionReview"
	responseReview.APIVersion = "admission.k8s.io/v1beta1"

	responseReview.Response = &admission.AdmissionResponse{}
	responseReview.Response.UID = req.Request.UID
	responseReview.Response.Allowed = true
	patchType := admission.PatchTypeJSONPatch
	responseReview.Response.PatchType = &patchType

	patchResponse := make([]RegCredPatchSpec, 1)
	patchResponse[0].Op = "add"
	patchResponse[0].Path = "/spec/imagePullSecrets"
	patchResponse[0].Value = append(patchResponse[0].Value, make(map[string]string, 1))
	firstCred := patchResponse[0].Value[0]
	firstCred["name"] = "regcred"

	responseReview.Response.Patch, err = json.Marshal(&patchResponse)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	responseBytes, err := json.Marshal(responseReview)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(responseBytes)
}

func main() {
	var tlsConf *tls.Config
	keyPair, err := tls.LoadX509KeyPair("certs/regcred-injector-crt.pem", "certs/regcred-injector-key.pem")
	if err != nil {
		log.Fatal(err)
	}

	serverName, found := os.LookupEnv("SERVER_NAME")
	if !found {
		log.Fatal("Unable to read SERVER_NAME environment variable")
	}
	dockerUsername, found = os.LookupEnv("DOCKER_USERNAME")
	if !found {
		log.Fatal("Unable to read DOCKER_USERNAME environment variable")
	}
	dockerPassword, found = os.LookupEnv("DOCKER_PASSWORD")
	if !found {
		log.Fatal("Unable to read DOCKER_PASSWORD environment variable")
	}

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

	log.Printf("%v+", tlsConf)

	r := mux.NewRouter()
	r.HandleFunc("/admission", PodHandler)
	r.HandleFunc("/status", StatusHandler)
	loggingHandler := handlers.LoggingHandler(os.Stdout, r)

	http.Handle("/", r)

	srv := &http.Server{
		Handler:      loggingHandler,
		Addr:         "0.0.0.0:8443",
		TLSConfig:    tlsConf,
		TLSNextProto: make(map[string]func(*http.Server, *tls.Conn, http.Handler), 0),
		WriteTimeout: 15 * time.Second,
		ReadTimeout:  15 * time.Second,
	}

	log.Fatal(srv.ListenAndServeTLS("certs/regcred-injector-crt.pem", "certs/regcred-injector-key.pem"))
}
