package handlers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"log"
	"net/http"
	"strconv"

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
var DockerRegistry string

func getReview(r *http.Request) (admission.AdmissionReview, error) {
	var rev admission.AdmissionReview

	err := json.NewDecoder(r.Body).Decode(&rev)
	if err != nil {
		return rev, err
	}

	return rev, nil
}

func hasImagePullSecrets(pod apiv1.Pod) bool {
	return len(pod.Spec.ImagePullSecrets) > 0
}

func getExistingSecret(namespace string) (bool, string, error) {
	secrets, err := Clientset.CoreV1().Secrets(namespace).List(context.TODO(), v1.ListOptions{})
	if err != nil {
		return false, "", err
	}

	hasSecret := false
	existingDockerConfig := ""
	for i := 0; i < len(secrets.Items); i++ {
		if secrets.Items[i].ObjectMeta.Name == "regcred" {
			hasSecret = true
			existingDockerConfig = string(secrets.Items[i].Data[".dockerconfigjson"])
		}
	}

	return hasSecret, existingDockerConfig, nil
}

func newCredentialsSecret(uid types.UID) (apiv1.Secret, error) {
	dockerConfig := k8s.DockerConfig{}
	dockerConfig.Auths = make(map[string]k8s.DockerAuth)
	dockerAuth := k8s.DockerAuth{}
	dockerAuth.Username = DockerUsername
	dockerAuth.Password = DockerPassword
	dockerAuth.Auth = base64.StdEncoding.EncodeToString([]byte(DockerUsername + ":" + DockerPassword))
	dockerConfig.Auths[DockerRegistry] = dockerAuth

	dockerConfigJSON, err := json.Marshal(dockerConfig)
	if err != nil {
		log.Printf("%s: failed to marshal DockerConfig", uid)
		return apiv1.Secret{}, err
	}

	newSecret := apiv1.Secret{}
	newSecret.Type = "kubernetes.io/dockerconfigjson"
	newSecret.Name = "regcred"
	newSecret.Data = make(map[string][]byte)
	newSecret.Data[".dockerconfigjson"] = []byte(dockerConfigJSON)

	return newSecret, nil
}

func createSecret(namespace string, uid types.UID, newSecret apiv1.Secret) error {
	log.Printf("%s: creating credentials in %s", uid, namespace)

	_, err := Clientset.CoreV1().Secrets(namespace).Create(context.TODO(), &newSecret, v1.CreateOptions{})
	if err != nil {
		log.Printf("%s: credential creation in %s failed", uid, namespace)
		return err
	}

	log.Printf("%s: credential creation in %s succeeded", uid, namespace)
	return err
}

func updateSecret(namespace string, uid types.UID, newSecret apiv1.Secret) error {
	log.Printf("%s: updating credentials in %s", uid, namespace)

	_, err := Clientset.CoreV1().Secrets(namespace).Update(context.TODO(), &newSecret, v1.UpdateOptions{})
	if err != nil {
		log.Printf("%s: credential update in %s failed", uid, namespace)
		return err
	}

	log.Printf("%s: credential update in %s succeeded", uid, namespace)
	return nil
}

func createOrUpdateSecret(namespace string, uid types.UID) error {
	hasSecret, existingDockerConfig, err := getExistingSecret(namespace)
	if err != nil {
		return err
	}

	newSecret, err := newCredentialsSecret(uid)
	if err != nil {
		return err
	}

	if !hasSecret {
		return createSecret(namespace, uid, newSecret)
	} else if existingDockerConfig != string(newSecret.Data[".dockerconfigjson"]) {
		return updateSecret(namespace, uid, newSecret)
	} else {
		log.Printf("%s: skipping credentials in %s, already exists", uid, namespace)
	}

	return nil
}

func generateResponseReview(req admission.AdmissionReview, pod apiv1.Pod) (*admission.AdmissionReview, error) {
	log.Printf("%s: Mutating pod with imagePullSecrets", req.Request.UID)

	responseReview := admission.AdmissionReview{}

	responseReview.Kind = "AdmissionReview"
	responseReview.APIVersion = "admission.k8s.io/v1beta1"
	responseReview.Response = &admission.AdmissionResponse{}
	responseReview.Response.UID = req.Request.UID
	responseReview.Response.Allowed = true
	patchType := admission.PatchTypeJSONPatch
	responseReview.Response.PatchType = &patchType

	patch, err := generatePatchResponse(pod)
	if err != nil {
		return nil, err
	}

	responseReview.Response.Patch = patch

	return &responseReview, nil
}

func generatePatchResponse(pod apiv1.Pod) ([]byte, error) {
	var patch []byte
	var err error
	if hasImagePullSecrets(pod) {
		patch, err = json.Marshal(generateAppendPatchResponse(len(pod.Spec.ImagePullSecrets)))
	} else {
		patch, err = json.Marshal(generateAddPatchResponse())
	}
	if err != nil {
		return nil, err
	}
	return patch, nil
}

func generateAddPatchResponse() []k8s.CreatePatchSpec {
	patchResponse := make([]k8s.CreatePatchSpec, 1)
	patchResponse[0].Op = "add"
	patchResponse[0].Path = "/spec/imagePullSecrets"
	patchResponse[0].Value = append(patchResponse[0].Value, make(map[string]string, 1))
	firstCred := patchResponse[0].Value[0]
	firstCred["name"] = "regcred"

	return patchResponse
}

func generateAppendPatchResponse(imagePullSecretCount int) []k8s.AppendPatchSpec {
	patchResponse := make([]k8s.AppendPatchSpec, 1)
	patchResponse[0].Op = "add"
	patchResponse[0].Path = "/spec/imagePullSecrets/" + strconv.Itoa(imagePullSecretCount)
	patchResponse[0].Value = map[string]string{}
	patchResponse[0].Value["name"] = "regcred"

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

	err = createOrUpdateSecret(namespace, req.Request.UID)
	if err != nil {
		log.Printf("%s failed to create secret, continuing to mutate: %s", req.Request.UID, err.Error())
	}

	var responseReview *admission.AdmissionReview
	log.Printf("%s: generating patch response for pod", req.Request.UID)
	responseReview, err = generateResponseReview(req, pod)

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
