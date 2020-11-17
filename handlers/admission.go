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
	"k8s.io/apimachinery/pkg/types"
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

func createSecret(namespace string, uid types.UID) error {
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
		log.Printf("%s: creating credentials in %s", uid, namespace)

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
			log.Printf("%s: credential creation in %s failed", uid, namespace)
			return err
		}
		log.Printf("%s: credential creation in %s succeeded", uid, namespace)
	} else {
		log.Printf("%s: skipping credentials in %s, already exists", uid, namespace)
	}
	return nil
}

func generateResponseReview(req admission.AdmissionReview) (*admission.AdmissionReview, error) {
	log.Printf("%s: Mutating pod with imagePullSecrets", req.Request.UID)

	responseReview := admission.AdmissionReview{}

	responseReview.Kind = "AdmissionReview"
	responseReview.APIVersion = "admission.k8s.io/v1beta1"
	responseReview.Response = &admission.AdmissionResponse{}
	responseReview.Response.UID = req.Request.UID
	responseReview.Response.Allowed = true
	patchType := admission.PatchTypeJSONPatch
	responseReview.Response.PatchType = &patchType
	patch, err := json.Marshal(generatePatchResponse())
	if err != nil {
		return nil, err
	}
	responseReview.Response.Patch = patch

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

	namespace := req.Request.Namespace
	log.Printf("%s: received a webhook request for namespace %s", req.Request.UID, namespace)

	podObject, err := req.Request.Object.MarshalJSON()
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var pod apiv1.Pod
	err = json.Unmarshal(podObject, &pod)
	if err != nil {
		log.Printf("%s: failed to unmarshal pod JSON into pod object", req.Request.UID)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	err = createSecret(namespace, req.Request.UID)
	if err != nil {
		log.Printf("%s failed to create secret, continuing to mutate: %s", req.Request.UID, err.Error())
	}

	var responseReview *admission.AdmissionReview
	log.Printf("%s: generating patch response for pod", req.Request.UID)
	responseReview, err = generateResponseReview(req)

	if err != nil {
		log.Printf("%s: failed to generate response Review", req.Request.UID)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	responseBytes, err := json.Marshal(responseReview)
	if err != nil {
		log.Printf("%s: failed to marshal response Review to bytes", req.Request.UID)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	log.Printf("%s: request successful", req.Request.UID)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(responseBytes)
}
