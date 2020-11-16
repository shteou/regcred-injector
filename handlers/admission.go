package handlers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"log"
	"net/http"

	"github.com/shteou/regcred-injector/k8s"

	admission "k8s.io/api/admission/v1beta1"
	apiv1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

var Clientset *kubernetes.Clientset
var DockerUsername string
var DockerPassword string

func getReview(r *http.Request) (admission.AdmissionReview, error) {
	var rev admission.AdmissionReview

	err := json.NewDecoder(r.Body).Decode(&rev)
	if err != nil {
		return rev, err
	}

	return rev, nil
}

func createSecret(namespace string) error {
	secrets, err := Clientset.CoreV1().Secrets(namespace).List(context.TODO(), v1.ListOptions{})
	if err != nil {
		return err
	}

	hasSecret := false
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
		dockerConfig := k8s.DockerConfig{}
		dockerConfig.Auths = make(map[string]k8s.DockerAuth)
		dockerAuth := k8s.DockerAuth{}
		dockerAuth.Username = DockerUsername
		dockerAuth.Password = DockerPassword
		dockerAuth.Auth = base64.StdEncoding.EncodeToString([]byte(DockerUsername + ":" + DockerPassword))
		dockerConfig.Auths["https://index.docker.io/v1/"] = dockerAuth

		dockerConfigJSON, err := json.Marshal(dockerConfig)
		if err != nil {
			return err
		}

		secret.Data[".dockerconfigjson"] = []byte(dockerConfigJSON)
		_, err = Clientset.CoreV1().Secrets(namespace).Create(context.TODO(), &secret, v1.CreateOptions{})
		if err != nil {
			return err
		}
	}
	return nil
}

func generateResponseReview(req admission.AdmissionReview, mutate bool) (*admission.AdmissionReview, error) {
	responseReview := admission.AdmissionReview{}

	responseReview.Kind = "AdmissionReview"
	responseReview.APIVersion = "admission.k8s.io/v1beta1"
	responseReview.Response = &admission.AdmissionResponse{}
	responseReview.Response.UID = req.Request.UID
	responseReview.Response.Allowed = true

	if mutate == true {
		log.Println("Mutating")
		patchType := admission.PatchTypeJSONPatch
		responseReview.Response.PatchType = &patchType
		patch, err := json.Marshal(generatePatchResponse())
		if err != nil {
			return nil, err
		}
		responseReview.Response.Patch = patch
	}

	return &responseReview, nil
}

func generatePatchResponse() []k8s.RegCredPatchSpec {
	patchResponse := make([]k8s.RegCredPatchSpec, 1)
	patchResponse[0].Op = "add"
	patchResponse[0].Path = "/spec/imagePullSecrets"
	patchResponse[0].Value = append(patchResponse[0].Value, make(map[string]string, 1))
	firstCred := patchResponse[0].Value[0]
	firstCred["name"] = "regcred"

	return patchResponse
}

func PodHandler(w http.ResponseWriter, r *http.Request) {
	req, err := getReview(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	log.Printf("Received a webhook request with UID %s", req.Request.UID)

	podObject, err := req.Request.Object.MarshalJSON()
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	log.Println(string(podObject))
	var pod apiv1.Pod
	err = json.Unmarshal(podObject, &pod)
	if err != nil {
		log.Printf("Failed to unmarshal pod JSON into pod object for pod %s", req.Request.UID)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	log.Printf("Identified target pod %s in namespace %s for UID %s", pod.ObjectMeta.Name, pod.ObjectMeta.Namespace, req.Request.UID)

	namespace := pod.ObjectMeta.Namespace
	if namespace == "" {
		namespace = pod.Namespace
	}
	log.Printf("Namespace was: %s", namespace)
	if namespace != "" {
		log.Printf("Creating secret in namespace %s", pod.Namespace)
		err = createSecret(pod.ObjectMeta.Namespace)
		if err != nil {
			log.Printf("Encountered an error creating the secret in %s: %s", pod.ObjectMeta.Namespace, err.Error())
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		log.Printf("Created secret in namespace %s for UID %s", pod.ObjectMeta.Namespace, req.Request.UID)
	}

	var responseReview *admission.AdmissionReview
	if pod.Kind == "Pod" {
		log.Println("Found a Pod, going to generate a patch response")
		responseReview, err = generateResponseReview(req, true)
	} else {
		responseReview, err = generateResponseReview(req, false)
	}

	if err != nil {
		log.Printf("Failed to generate response Review for UID %s", req.Request.UID)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	responseBytes, err := json.Marshal(responseReview)
	if err != nil {
		log.Printf("Failed to marshal response Review to bytes for UID %s", req.Request.UID)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	log.Printf("Successfully serviced request %s", req.Request.UID)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(responseBytes)
}
