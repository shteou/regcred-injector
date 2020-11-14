package main

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	admission "k8s.io/api/admission/v1beta1"
)

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

	patchResponse := []map[string]string{{
		"op":    "add",
		"path":  "/spec/imagePullSecrets/-",
		"value": "{\"name\": \"regcredsecret\"}",
	}}

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
	r := mux.NewRouter()
	r.HandleFunc("/admission", PodHandler)
	r.HandleFunc("/status", StatusHandler)
	http.Handle("/", r)

	srv := &http.Server{
		Handler:      r,
		Addr:         "127.0.0.1:8080",
		WriteTimeout: 15 * time.Second,
		ReadTimeout:  15 * time.Second,
	}

	log.Fatal(srv.ListenAndServe())
}
